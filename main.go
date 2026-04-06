// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package main

import (
	"embed"
	"os"

	"dinkisstyle-translator/internal/app"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	mode := "main"
	windowTitle := "DKST Translator AI"
	windowWidth := 1200
	windowHeight := 800
	if len(os.Args) > 1 && os.Args[1] == "--debug-studio-window" {
		mode = "debug-studio"
		windowTitle = "DKST Translator AI Debug Studio"
		windowWidth = 1640
		windowHeight = 1080
	}

	// Create an instance of the app structure
	a := app.NewApp(mode, assets)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  windowTitle,
		Width:  windowWidth,
		Height: windowHeight,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        a.Startup,
		OnShutdown:       a.Shutdown,
		Bind: []interface{}{
			a,
		},
		Menu: app.GetMenu(a),
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
			About: &mac.AboutInfo{
				Title:   "DKST Translator AI",
				Message: "Professional AI Local LLM Translation Tool built with Wails.",
			},
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
