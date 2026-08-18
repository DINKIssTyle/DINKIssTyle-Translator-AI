//go:build !android && !darwin && !ios && !windows

package translation

import "errors"

func platformCapabilities() Capabilities {
	return Capabilities{Available: false, Backend: "unsupported"}
}

func platformTranslate(Request) ([]string, error) {
	return nil, errors.New("native translation is not available on this platform")
}

func platformLanguages() ([]string, error) { return []string{}, nil }

func platformCancel(string) {}
