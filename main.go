// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package main

import (
	"embed"
	"log"

	"dinkisstyle-translator/internal/app"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	mode := "main"
	windowTitle := "DKST Translator AI"
	windowWidth := 1200
	windowHeight := 800
	if isDebugStudioRequested() {
		mode = "debug-studio"
		windowTitle = "DKST Translator AI Debug Studio"
		windowWidth = 1640
		windowHeight = 1080
	}

	wailsApp := application.New(application.Options{
		Name:        "DKST Translator AI",
		Description: "Professional AI Local LLM Translation Tool built with Wails.",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	a := app.NewApp(mode, assets, wailsApp)
	wailsApp.RegisterService(application.NewService(a))
	wailsApp.Menu.Set(app.GetMenu(wailsApp))
	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:               mode,
		Title:              windowTitle,
		Width:              windowWidth,
		Height:             windowHeight,
		MinWidth:           350,
		MinHeight:          700,
		BackgroundColour:   application.NewRGB(27, 38, 54),
		UseApplicationMenu: true,
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarDefault,
		},
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
