//go:build android

package translation

import "errors"

func platformCapabilities() Capabilities {
	return Capabilities{Available: true, Backend: "android-mlkit", MinimumVersion: "Android 6.0", Engines: []string{"android-mlkit"}}
}

func platformTranslate(Request) ([]string, error) {
	return nil, errors.New("Android translation is provided by the ML Kit WebView bridge")
}

func platformLanguages() ([]string, error) { return []string{}, nil }

func platformCancel(string) {}
