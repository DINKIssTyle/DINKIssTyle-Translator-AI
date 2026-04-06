package debugsync

import (
	"os"
	"path/filepath"
	"sync"
)

var fileMu sync.Mutex

func stateFilePath() string {
	return filepath.Join(os.TempDir(), "dkst-translator-ai-debug-studio-state.json")
}

func Read() (string, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	data, err := os.ReadFile(stateFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func Write(state string) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	return os.WriteFile(stateFilePath(), []byte(state), 0o600)
}
