// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"dinkisstyle-translator/internal/file"
	"dinkisstyle-translator/internal/litertlm"
	"dinkisstyle-translator/internal/llm"
	"dinkisstyle-translator/internal/translation"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App struct
type App struct {
	wails             *application.App
	mode              string
	assets            fs.FS
	llm               *llm.Client
	liteRT            *litertlm.Manager
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
	downloadMu        sync.Mutex
	downloadCancel    context.CancelFunc
}

// NewApp creates a new App application struct
func NewApp(mode string, assets fs.FS, wailsApp *application.App) *App {
	return &App{
		wails:             wailsApp,
		mode:              mode,
		assets:            assets,
		llm:               llm.NewClient(),
		liteRT:            litertlm.NewManager(),
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
	_ = a.liteRT.Close()
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
	var err error
	settings, err = a.prepareProviderSettings(settings)
	if err != nil {
		return nil, err
	}
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
	var err error
	req.Settings, err = a.prepareProviderSettings(req.Settings)
	if err != nil {
		return err
	}
	if req.DocumentType == "pdf" {
		return a.translatePDF(req)
	}
	return a.llm.Translate(req)
}

func (a *App) prepareProviderSettings(settings llm.ProviderSettings) (llm.ProviderSettings, error) {
	if !strings.EqualFold(settings.Mode, "litertlm") {
		return settings, nil
	}
	endpoint, modelID, err := a.liteRT.Ensure(context.Background(), litertlm.Config{
		ModelPath:   settings.LiteRTModelPath,
		RuntimePath: settings.LiteRTRuntimePath,
		RuntimeMode: settings.LiteRTRuntimeMode,
		Port:        settings.LiteRTPort,
	})
	if err != nil {
		return settings, err
	}
	settings.Endpoint = endpoint
	if strings.TrimSpace(settings.Model) == "" {
		settings.Model = modelID
	}
	return settings, nil
}

func (a *App) SelectLiteRTLMModel() (string, error) {
	return a.wails.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title: "Select LiteRT-LM Model",
		Filters: []application.FileFilter{
			{DisplayName: "LiteRT-LM Models (*.litertlm)", Pattern: "*.litertlm"},
			{DisplayName: "Legacy AI Edge Models (*.bin)", Pattern: "*.bin"},
		},
	}).PromptForSingleSelection()
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

func (a *App) GetPlatformCapabilities() (translation.Capabilities, error) {
	return translation.NewService().Capabilities(), nil
}

func (a *App) ListLocalLiteRTModels() ([]litertlm.LocalModelInfo, error) {
	return litertlm.ListLocalModels()
}

func (a *App) GetLiteRTModelsDir() (string, error) {
	return litertlm.GetModelsDir()
}

func (a *App) ImportLiteRTModel() (litertlm.LocalModelInfo, error) {
	if a.wails == nil || a.wails.Dialog == nil {
		return litertlm.LocalModelInfo{}, errors.New("file dialog unavailable")
	}
	selectedPath, err := a.wails.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title: "Import LiteRT-LM Model (.litertlm)",
		Filters: []application.FileFilter{
			{DisplayName: "LiteRT-LM Models (*.litertlm)", Pattern: "*.litertlm"},
			{DisplayName: "Legacy AI Edge Models (*.bin)", Pattern: "*.bin"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		return litertlm.LocalModelInfo{}, err
	}
	if strings.TrimSpace(selectedPath) == "" {
		return litertlm.LocalModelInfo{}, nil
	}
	return litertlm.ImportModelFile(selectedPath)
}

func (a *App) ImportLiteRTModelFromPath(sourcePath string) (litertlm.LocalModelInfo, error) {
	return litertlm.ImportModelFile(sourcePath)
}

func (a *App) DeleteLiteRTModel(modelPathOrName string) (bool, error) {
	if err := litertlm.DeleteModelFile(modelPathOrName); err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) ConfirmDeleteLiteRTModel(modelName string) (bool, error) {
	if a.wails == nil || a.wails.Dialog == nil {
		return true, nil
	}
	confirmed := false
	dialog := a.wails.Dialog.Question().
		SetTitle("Delete LiteRT-LM Model").
		SetMessage(fmt.Sprintf("Are you sure you want to delete the model %q from storage?", modelName))
	dialog.AddButton("Delete").OnClick(func() { confirmed = true })
	cancel := dialog.AddButton("Cancel")
	dialog.SetDefaultButton(cancel)
	dialog.SetCancelButton(cancel)
	dialog.Show()
	return confirmed, nil
}

func (a *App) DownloadLiteRTModel(repoOrURL string, token string) (string, error) {
	a.downloadMu.Lock()
	if a.downloadCancel != nil {
		a.downloadCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.downloadCancel = cancel
	a.downloadMu.Unlock()

	defer func() {
		a.downloadMu.Lock()
		a.downloadCancel = nil
		a.downloadMu.Unlock()
	}()

	return litertlm.DownloadModel(ctx, repoOrURL, token, func(progress litertlm.DownloadProgress) {
		if a.wails != nil && a.wails.Event != nil {
			a.wails.Event.Emit("litert:download-progress", progress)
		}
	})
}

func (a *App) CancelLiteRTDownload() bool {
	a.downloadMu.Lock()
	defer a.downloadMu.Unlock()
	if a.downloadCancel != nil {
		a.downloadCancel()
		a.downloadCancel = nil
		if a.wails != nil && a.wails.Event != nil {
			a.wails.Event.Emit("litert:download-progress", litertlm.DownloadProgress{
				Status: "cancelled",
			})
		}
		return true
	}
	return false
}

