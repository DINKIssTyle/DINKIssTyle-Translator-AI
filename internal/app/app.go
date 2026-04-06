// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package app

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"dinkisstyle-translator/internal/debugsync"
	"dinkisstyle-translator/internal/file"
	"dinkisstyle-translator/internal/llm"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx               context.Context
	mode              string
	assets            fs.FS
	llm               *llm.Client
	webLLM            *llm.Client
	file              *file.FileHandler
	hostSettings      llm.ProviderSettings
	webServer         *http.Server
	webServerMu       sync.Mutex
	webServerPort     string
	webServerTLS      bool
	webServerSettings persistedWebServerSettings
	webSessions       map[string]time.Time
}

// NewApp creates a new App application struct
func NewApp(mode string, assets fs.FS) *App {
	return &App{
		mode:              mode,
		assets:            assets,
		llm:               llm.NewClient(),
		webLLM:            llm.NewClient(),
		file:              file.NewFileHandler(),
		hostSettings:      loadPersistedHostProviderSettings(),
		webServerSettings: loadPersistedWebServerSettings(),
		webSessions:       make(map[string]time.Time),
	}
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.llm.SetContext(ctx)
	a.webLLM.SetContext(ctx)
	a.file.SetContext(ctx)
	if a.mode == "main" && a.webServerSettings.Enabled {
		_ = a.applyWebServerSettings()
	}
}

func (a *App) Shutdown(context.Context) {
	a.webServerMu.Lock()
	defer a.webServerMu.Unlock()
	_ = a.stopWebServerLocked()
}

// GetModels returns the list of models from the configured provider endpoint.
func (a *App) GetModels(settings llm.ProviderSettings) ([]llm.ModelInfo, error) {
	return a.llm.ListModels(settings)
}

func (a *App) GetHostProviderSettings() (llm.ProviderSettings, error) {
	return a.hostSettings, nil
}

func (a *App) SaveHostProviderSettings(settings llm.ProviderSettings) error {
	a.hostSettings = settings
	return savePersistedHostProviderSettings(settings)
}

// Translate performs the translation via the configured provider.
func (a *App) Translate(req llm.TranslationRequest) error {
	return a.llm.Translate(req)
}

// CancelTranslation aborts the active translation request, if any.
func (a *App) CancelTranslation() {
	a.llm.CancelTranslation()
}

// OpenFile opens a text file and returns its content
func (a *App) OpenFile() (string, error) {
	return a.file.OpenFile()
}

// SaveFile saves text to a file
func (a *App) SaveFile(content string) (string, error) {
	return a.file.SaveFile(content)
}

func (a *App) ConfirmClearSource() (bool, error) {
	selection, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Clear Source Text",
		Message:       "Clear the source text?",
		Buttons:       []string{"Clear", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return false, err
	}
	return selection == "Clear", nil
}

func (a *App) GetWindowMode() string {
	return a.mode
}

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
