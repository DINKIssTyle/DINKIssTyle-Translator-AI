// Package translation exposes the operating-system translation backend to the
// translator application. This package translates ordered batches of plain text.
package translation

import (
	"errors"
	"strings"
)

// Capabilities describes the backend compiled for the current platform.
type Capabilities struct {
	Available      bool     `json:"available"`
	Backend        string   `json:"backend"`
	MinimumVersion string   `json:"minimumVersion"`
	Engines        []string `json:"engines,omitempty"`
}

// Request is an ordered batch of text nodes from one document.
type Request struct {
	RequestID      string   `json:"requestID,omitempty"`
	Engine         string   `json:"engine,omitempty"`
	Texts          []string `json:"texts"`
	SourceLanguage string   `json:"sourceLanguage"`
	TargetLanguage string   `json:"targetLanguage"`
}

// Service is exposed to the application for native translation.
type Service struct{}

// NewService creates the translation service.
func NewService() *Service { return &Service{} }

// Capabilities returns the backend selected by the current Go build target.
func (s *Service) Capabilities() Capabilities { return platformCapabilities() }

// Languages returns every language currently supported by the platform backend.
func (s *Service) Languages() ([]string, error) { return platformLanguages() }

// Cancel asks the platform backend to stop an in-flight translation request.
func (s *Service) Cancel(requestID string) { platformCancel(strings.TrimSpace(requestID)) }

// Translate translates a batch while preserving its input order.
func (s *Service) Translate(request Request) ([]string, error) {
	if len(request.Texts) == 0 {
		return []string{}, nil
	}
	sourceBase := normaliseLanguage(request.SourceLanguage)
	targetBase := normaliseLanguage(request.TargetLanguage)
	request.SourceLanguage = platformLanguage(request.SourceLanguage)
	request.TargetLanguage = platformLanguage(request.TargetLanguage)
	if request.TargetLanguage == "" || request.TargetLanguage == "und" {
		return nil, errors.New("a target language is required")
	}
	if sourceBase != "" && sourceBase == targetBase {
		return append([]string(nil), request.Texts...), nil
	}
	return platformTranslate(request)
}

func platformLanguage(value string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(strings.ToLower(value)), "_", "-")
	if cleaned == "" || cleaned == "auto" || cleaned == "und" || cleaned == "automatic" {
		return ""
	}
	base := normaliseLanguage(cleaned)
	if base != "" {
		// If subtag exists like zh-hans or pt-br, keep valid subtag
		parts := strings.Split(cleaned, "-")
		if len(parts) > 1 && (base == "zh" || base == "pt") {
			return base + "-" + parts[1]
		}
		return base
	}
	return cleaned
}

func normaliseLanguage(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if index := strings.IndexAny(value, "-_"); index >= 0 {
		value = value[:index]
	}
	switch value {
	case "auto", "automatic", "und", "detect":
		return ""
	case "en", "eng", "english":
		return "en"
	case "ko", "kor", "korean":
		return "ko"
	case "ja", "jpn", "japanese":
		return "ja"
	case "zh", "zho", "chi", "chinese":
		return "zh"
	case "de", "deu", "ger", "german":
		return "de"
	case "fr", "fra", "fre", "french":
		return "fr"
	case "es", "spa", "spanish":
		return "es"
	case "it", "ita", "italian":
		return "it"
	case "ru", "rus", "russian":
		return "ru"
	case "pt", "por", "portuguese":
		return "pt"
	case "ar", "ara", "arabic":
		return "ar"
	case "hi", "hin", "hindi":
		return "hi"
	case "vi", "vie", "vietnamese":
		return "vi"
	case "th", "tha", "thai":
		return "th"
	case "id", "ind", "indonesian":
		return "id"
	case "nl", "nld", "dut", "dutch":
		return "nl"
	case "pl", "pol", "polish":
		return "pl"
	case "tr", "tur", "turkish":
		return "tr"
	case "uk", "ukr", "ukrainian":
		return "uk"
	}
	return value
}
