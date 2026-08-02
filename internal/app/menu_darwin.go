//go:build darwin
// +build darwin

package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func GetMenu(app *application.App) *application.Menu {
	appMenu := app.NewMenu()

	// macOS Native App Menu
	appMenu.AddRole(application.AppMenu)

	// File Menu
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.Add("Open File…").SetAccelerator("CmdOrCtrl+o").OnClick(func(_ *application.Context) {
		app.Event.Emit("menu:open-file")
	})
	fileMenu.AddSeparator()
	fileMenu.Add("Quit").SetAccelerator("CmdOrCtrl+q").OnClick(func(_ *application.Context) {
		app.Quit()
	})

	// Edit Menu (Essential for Clipboard on macOS)
	appMenu.AddRole(application.EditMenu)

	viewMenu := appMenu.AddSubmenu("View")
	viewMenu.Add("Decrease Font Size").SetAccelerator("CmdOrCtrl+-").OnClick(func(_ *application.Context) {
		app.Event.Emit("menu:font-decrease")
	})
	viewMenu.Add("Increase Font Size").SetAccelerator("CmdOrCtrl+=").OnClick(func(_ *application.Context) {
		app.Event.Emit("menu:font-increase")
	})
	viewMenu.Add("Actual Size").SetAccelerator("CmdOrCtrl+0").OnClick(func(_ *application.Context) {
		app.Event.Emit("menu:font-reset")
	})

	translateMenu := appMenu.AddSubmenu("Translate")
	translateMenu.Add("Translate").SetAccelerator("CmdOrCtrl+t").OnClick(func(_ *application.Context) {
		app.Event.Emit("menu:translate")
	})

	// Window Menu
	appMenu.AddRole(application.WindowMenu)

	return appMenu
}
