//go:build !darwin
// +build !darwin

package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func GetMenu(app *application.App) *application.Menu {
	appMenu := app.NewMenu()

	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.Add("Open File...").SetAccelerator("CmdOrCtrl+o").OnClick(func(_ *application.Context) {
		app.Event.Emit("menu:open-file")
	})

	return appMenu
}
