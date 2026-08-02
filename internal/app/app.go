// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package app

import (
	"context"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"dinkisstyle-translator/internal/file"
	"dinkisstyle-translator/internal/llm"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App struct
type App struct {
	wails             *application.App
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
	pdfMu             sync.Mutex
	activePDF         file.PDFDocument
	pdfRunMu          sync.Mutex
	pdfCheckpointMu   sync.Mutex
}

// NewApp creates a new App application struct
func NewApp(mode string, assets fs.FS, wailsApp *application.App) *App {
	return &App{
		wails:             wailsApp,
		mode:              mode,
		assets:            assets,
		llm:               llm.NewClient(),
		webLLM:            llm.NewClient(),
		file:              file.NewFileHandler(wailsApp),
		hostSettings:      loadPersistedHostProviderSettings(),
		webServerSettings: loadPersistedWebServerSettings(),
		webSessions:       make(map[string]time.Time),
	}
}

// ServiceStartup is called by Wails when the service starts.
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.llm.SetRuntime(ctx, a.wails.Event.Emit)
	a.webLLM.SetRuntime(ctx, a.wails.Event.Emit)
	if a.mode == "main" && a.webServerSettings.Enabled {
		_ = a.applyWebServerSettings()
	}
	return nil
}

func (a *App) ServiceShutdown() error {
	a.llm.CancelTranslation()
	a.pdfRunMu.Lock()
	a.pdfRunMu.Unlock()
	a.markPDFCheckpointCleanShutdown()

	a.webServerMu.Lock()
	defer a.webServerMu.Unlock()
	_ = a.stopWebServerLocked()
	return nil
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
	if req.DocumentType == "pdf" {
		return a.translatePDF(req)
	}
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

func (a *App) OpenDocument() (file.OpenedDocument, error) {
	document, err := a.file.OpenDocument()
	if err != nil || document.Kind != "pdf" || document.PDF.PageCount == 0 {
		return document, err
	}
	a.pdfMu.Lock()
	a.activePDF = document.PDF
	a.pdfMu.Unlock()
	return document, nil
}

func (a *App) OpenPDF() (file.PDFDocument, error) {
	document, err := a.file.OpenPDF()
	if err != nil || document.PageCount == 0 {
		return document, err
	}
	a.pdfMu.Lock()
	a.activePDF = document
	a.pdfMu.Unlock()
	return document, nil
}

func (a *App) CreateTranslatedPDF(req file.PDFCreateRequest) (file.PDFResult, error) {
	return file.BuildTranslatedPDF(req)
}

// SaveFile saves text to a file
func (a *App) SaveFile(content string) (string, error) {
	return a.file.SaveFile(content)
}

func (a *App) SavePDF(dataBase64 string, defaultFilename string) (string, error) {
	return a.file.SavePDF(dataBase64, defaultFilename)
}

func (a *App) ConfirmClearSource() (bool, error) {
	confirmed := false
	dialog := a.wails.Dialog.Question().
		SetTitle("Clear Source Text").
		SetMessage("Clear the source text?")
	dialog.AddButton("Clear").OnClick(func() { confirmed = true })
	cancel := dialog.AddButton("Cancel")
	dialog.SetDefaultButton(cancel)
	dialog.SetCancelButton(cancel)
	dialog.Show()
	return confirmed, nil
}

func (a *App) GetWindowMode() string {
	return a.mode
}
