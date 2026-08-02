//go:build !production

package app

import (
	"os"
	"os/exec"

	"dinkisstyle-translator/internal/debugsync"
)

func (a *App) ReadDebugStudioState() (string, error) {
	return debugsync.Read()
}

func (a *App) WriteDebugStudioState(state string) error {
	return debugsync.Write(state)
}

func (a *App) OpenDebugStudioWindow() error {
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(executablePath, "--debug-studio-window")
	return cmd.Start()
}
