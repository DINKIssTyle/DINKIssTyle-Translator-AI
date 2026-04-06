//go:build !darwin
// +build !darwin

package app

import (
	"github.com/wailsapp/wails/v2/pkg/menu"
)

func GetMenu(app *App) *menu.Menu {
	AppMenu := menu.NewMenu()

	// On non-macOS, the app menu is less common.
	return AppMenu
}
