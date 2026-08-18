//go:build darwin || ios

package translation

/*
#include <stdlib.h>
#include "apple_bridge.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type appleResponse struct {
	Texts []string `json:"texts"`
	Error string   `json:"error"`
}

type appleFrontendRequest struct {
	ID     uint64
	Engine string
}

type appleCancellationMarker struct {
	ID uint64
}

var (
	appleRequestSequence      atomic.Uint64
	appleRequests             sync.Map
	appleFrontendRequests     sync.Map
	appleCancelledRequests    sync.Map
	appleCancellationSequence atomic.Uint64
)

func appleLibraryPath() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Frameworks", "DKSTTranslation.dylib"))
}

func platformCapabilities() Capabilities {
	path := C.CString(appleLibraryPath())
	defer C.free(unsafe.Pointer(path))
	appleAvailable := C.dkst_apple_translation_available(path) == 1
	googleAvailable := C.dkst_google_translation_available() == 1
	engines := make([]string, 0, 2)
	if appleAvailable {
		engines = append(engines, "apple-translation")
	}
	if googleAvailable {
		engines = append(engines, "google-mlkit")
	}
	return Capabilities{
		Available:      appleAvailable || googleAvailable,
		Backend:        "apple-translation",
		MinimumVersion: "iOS/iPadOS 15.5 (Google ML Kit), iOS/iPadOS 18.0 or macOS 15.0 (Apple)",
		Engines:        engines,
	}
}

func platformTranslate(request Request) ([]string, error) {
	engine := request.Engine
	if engine == "" {
		engine = "apple-translation"
	}
	backendName := "Apple Translation"
	if engine == "google-mlkit" {
		backendName = "Google ML Kit"
	}
	capabilities := platformCapabilities()
	engineAvailable := false
	for _, available := range capabilities.Engines {
		if available == engine {
			engineAvailable = true
			break
		}
	}
	if !engineAvailable {
		if engine == "google-mlkit" {
			return nil, errors.New("Google ML Kit translation is available only in the iPhone and iPad build")
		}
		return nil, errors.New("Apple Translation requires iOS/iPadOS 18 or macOS 15")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	id := appleRequestSequence.Add(1)
	response := make(chan appleResponse, 1)
	appleRequests.Store(id, response)
	defer appleRequests.Delete(id)
	if request.RequestID != "" {
		appleFrontendRequests.Store(request.RequestID, appleFrontendRequest{ID: id, Engine: engine})
		if _, cancelled := appleCancelledRequests.LoadAndDelete(request.RequestID); cancelled {
			appleFrontendRequests.Delete(request.RequestID)
			return nil, errors.New("Translation cancelled")
		}
		defer func() {
			if value, ok := appleFrontendRequests.Load(request.RequestID); ok && value.(appleFrontendRequest).ID == id {
				appleFrontendRequests.Delete(request.RequestID)
			}
		}()
	}

	startTime := time.Now()
	totalChars := 0
	for _, text := range request.Texts {
		totalChars += len(text)
	}
	fmt.Printf("[AppleTranslation/Go] Request #%d dispatched (engine: %s, texts: %d, chars: %d, %s -> %s)\n",
		id, engine, len(request.Texts), totalChars, request.SourceLanguage, request.TargetLanguage)

	requestJSON := C.CString(string(payload))
	defer C.free(unsafe.Pointer(requestJSON))
	if engine == "google-mlkit" {
		if C.dkst_google_translation_submit(requestJSON, C.uint64_t(id)) != 1 {
			return nil, errors.New("Google ML Kit translation bridge could not be loaded")
		}
	} else {
		path := C.CString(appleLibraryPath())
		defer C.free(unsafe.Pointer(path))
		if C.dkst_apple_translation_submit(path, requestJSON, C.uint64_t(id)) != 1 {
			return nil, errors.New("Apple Translation bridge could not be loaded")
		}
	}

	select {
	case result := <-response:
		elapsed := time.Since(startTime)
		if result.Error != "" {
			fmt.Printf("[AppleTranslation/Go] Request #%d failed after %v: %s\n", id, elapsed, result.Error)
			return nil, errors.New(result.Error)
		}
		if len(result.Texts) != len(request.Texts) {
			fmt.Printf("[AppleTranslation/Go] Request #%d incomplete after %v (%d/%d texts)\n", id, elapsed, len(result.Texts), len(request.Texts))
			return nil, errors.New(backendName + " returned an incomplete batch")
		}
		fmt.Printf("[AppleTranslation/Go] Request #%d completed successfully in %v\n", id, elapsed)
		return result.Texts, nil
	case <-time.After(10 * time.Minute):
		fmt.Printf("[AppleTranslation/Go] Request #%d timed out\n", id)
		return nil, errors.New(backendName + " timed out")
	}
}

func platformLanguages() ([]string, error) {
	appleAvailable := false
	for _, engine := range platformCapabilities().Engines {
		if engine == "apple-translation" {
			appleAvailable = true
			break
		}
	}
	if !appleAvailable {
		return nil, errors.New("Apple Translation requires iOS/iPadOS 18 or macOS 15")
	}
	id := appleRequestSequence.Add(1)
	response := make(chan appleResponse, 1)
	appleRequests.Store(id, response)
	defer appleRequests.Delete(id)
	path := C.CString(appleLibraryPath())
	defer C.free(unsafe.Pointer(path))
	if C.dkst_apple_translation_languages(path, C.uint64_t(id)) != 1 {
		return nil, errors.New("Apple Translation bridge could not list languages")
	}
	select {
	case result := <-response:
		if result.Error != "" {
			return nil, errors.New(result.Error)
		}
		return result.Texts, nil
	case <-time.After(30 * time.Second):
		return nil, errors.New("Apple Translation language lookup timed out")
	}
}

func platformCancel(requestID string) {
	if requestID == "" {
		return
	}
	value, ok := appleFrontendRequests.Load(requestID)
	if !ok {
		marker := appleCancellationMarker{ID: appleCancellationSequence.Add(1)}
		appleCancelledRequests.Store(requestID, marker)
		time.AfterFunc(time.Minute, func() {
			if current, exists := appleCancelledRequests.Load(requestID); exists && current == marker {
				appleCancelledRequests.Delete(requestID)
			}
		})
		return
	}
	request := value.(appleFrontendRequest)
	if request.Engine == "google-mlkit" {
		C.dkst_google_translation_cancel(C.uint64_t(request.ID))
		return
	}
	path := C.CString(appleLibraryPath())
	defer C.free(unsafe.Pointer(path))
	C.dkst_apple_translation_cancel(path, C.uint64_t(request.ID))
}

//export DKSTAppleTranslationDidComplete
func DKSTAppleTranslationDidComplete(requestID C.uint64_t, resultJSON *C.char) {
	value, ok := appleRequests.Load(uint64(requestID))
	if !ok {
		return
	}
	result := appleResponse{}
	if resultJSON == nil {
		result.Error = "Apple Translation returned no result"
	} else if err := json.Unmarshal([]byte(C.GoString(resultJSON)), &result); err != nil {
		result.Error = "Apple Translation returned an invalid result"
	}
	value.(chan appleResponse) <- result
}
