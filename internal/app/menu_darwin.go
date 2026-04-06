//go:build darwin
// +build darwin

package app

import (
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func GetMenu(app *App) *menu.Menu {
	AppMenu := menu.NewMenu()

	// macOS Native App Menu
	AppMenu.Append(menu.AppMenu())

	// Edit Menu (Essential for Clipboard on macOS)
	AppMenu.Append(menu.EditMenu())

	// File Menu
	fileMenu := AppMenu.AddSubmenu("File")
	fileMenu.AddText("Open Text File", keys.CmdOrCtrl("o"), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "menu:open-file")
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("Quit", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		runtime.Quit(app.ctx)
	})

	viewMenu := AppMenu.AddSubmenu("View")
	viewMenu.AddText("Decrease Font Size", keys.CmdOrCtrl("-"), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "menu:font-decrease")
	})
	viewMenu.AddText("Increase Font Size", keys.CmdOrCtrl("="), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "menu:font-increase")
	})
	viewMenu.AddText("Actual Size", keys.CmdOrCtrl("0"), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "menu:font-reset")
	})

	translateMenu := AppMenu.AddSubmenu("Translate")
	translateMenu.AddText("Translate", keys.CmdOrCtrl("t"), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "menu:translate")
	})

	// Window Menu
	AppMenu.Append(menu.WindowMenu())

	return AppMenu
}
