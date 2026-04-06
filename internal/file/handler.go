// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package file

import (
	"context"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type FileHandler struct {
	ctx context.Context
}

func NewFileHandler() *FileHandler {
	return &FileHandler{}
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
