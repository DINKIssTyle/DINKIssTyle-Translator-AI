//go:build windows

package translation

import "errors"

func platformCapabilities() Capabilities {
	return Capabilities{Available: false, Backend: "unsupported"}
}

func platformTranslate(Request) ([]string, error) {
	return nil, errors.New("Windows native translation is unsupported")
}

func platformLanguages() ([]string, error) { return []string{}, nil }

func platformCancel(string) {}
