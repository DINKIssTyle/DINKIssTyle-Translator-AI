//go:build production

package app

func (a *App) ReadDebugStudioState() (string, error) {
	return "", nil
}

func (a *App) WriteDebugStudioState(state string) error {
	return nil
}

func (a *App) OpenDebugStudioWindow() error {
	return nil
}
