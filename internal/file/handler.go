// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dinkisstyle-translator/internal/pdfengine"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type FileHandler struct {
	ctx       context.Context
	pdfEngine pdfengine.Engine
}

type OpenedDocument struct {
	Kind string      `json:"kind"`
	Name string      `json:"name"`
	Text string      `json:"text,omitempty"`
	PDF  PDFDocument `json:"pdf"`
}

func NewFileHandler() *FileHandler {
	return &FileHandler{pdfEngine: newCleanroomPDFEngine()}
}

func (f *FileHandler) SetContext(ctx context.Context) {
	f.ctx = ctx
}

// OpenFile opens a system dialog to select a text/markdown file
func (f *FileHandler) OpenFile() (string, error) {
	selection, err := runtime.OpenFileDialog(f.ctx, runtime.OpenDialogOptions{
		Title: "Select Text File",
		Filters: []runtime.FileFilter{
			{DisplayName: "Text Files (*.txt;*.md)", Pattern: "*.txt;*.md"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	if selection == "" {
		return "", nil
	}

	content, err := os.ReadFile(selection)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// OpenDocument opens text, Markdown, and PDF files from one picker and returns
// the appropriate document representation for the selected extension.
func (f *FileHandler) OpenDocument() (OpenedDocument, error) {
	selection, err := runtime.OpenFileDialog(f.ctx, runtime.OpenDialogOptions{
		Title: "Open File",
		Filters: []runtime.FileFilter{
			{DisplayName: "Supported Documents (*.txt;*.md;*.pdf)", Pattern: "*.txt;*.md;*.pdf"},
			{DisplayName: "Text and Markdown (*.txt;*.md)", Pattern: "*.txt;*.md"},
			{DisplayName: "PDF Documents (*.pdf)", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		return OpenedDocument{}, err
	}
	if selection == "" {
		return OpenedDocument{}, nil
	}

	switch strings.ToLower(filepath.Ext(selection)) {
	case ".pdf":
		document, err := f.pdfEngine.Analyze(selection)
		if err != nil {
			return OpenedDocument{}, err
		}
		return OpenedDocument{Kind: "pdf", Name: filepath.Base(selection), PDF: document}, nil
	case ".txt", ".md":
		content, err := os.ReadFile(selection)
		if err != nil {
			return OpenedDocument{}, err
		}
		return OpenedDocument{Kind: "text", Name: filepath.Base(selection), Text: string(content)}, nil
	default:
		return OpenedDocument{}, fmt.Errorf("unsupported document type: %s", filepath.Ext(selection))
	}
}

// SaveFile saves text to a file (optional but good for translation results)
func (f *FileHandler) SaveFile(content string) (string, error) {
	selection, err := runtime.SaveFileDialog(f.ctx, runtime.SaveDialogOptions{
		Title:           "Save Translation",
		DefaultFilename: "translation.txt",
		Filters: []runtime.FileFilter{
			{DisplayName: "Text Files (*.txt)", Pattern: "*.txt"},
			{DisplayName: "Markdown Files (*.md)", Pattern: "*.md"},
		},
	})
	if err != nil {
		return "", err
	}
	if selection == "" {
		return "", nil
	}

	err = os.WriteFile(selection, []byte(content), 0644)
	if err != nil {
		return "", err
	}

	return selection, nil
}
