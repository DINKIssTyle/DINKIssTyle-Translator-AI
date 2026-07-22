// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

// Package pdfengine defines the clean boundary between the translation app and
// PDF analysis/composition. Implementations receive translated block text from
// the app; they never own translation provider, glossary, or style decisions.
package pdfengine

type TextRegion struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type TextBlock struct {
	ID         string       `json:"id"`
	PageNumber int          `json:"pageNumber"`
	BlockIndex int          `json:"blockIndex"`
	X          float64      `json:"x"`
	Y          float64      `json:"y"`
	Width      float64      `json:"width"`
	Height     float64      `json:"height"`
	FontSize   float64      `json:"fontSize"`
	Role       string       `json:"role"`
	Text       string       `json:"text"`
	Regions    []TextRegion `json:"regions,omitempty"`
}

type Page struct {
	PageNumber int         `json:"pageNumber"`
	Width      float64     `json:"width"`
	Height     float64     `json:"height"`
	Text       string      `json:"text"`
	Blocks     []TextBlock `json:"blocks"`
}

type Document struct {
	Name         string `json:"name"`
	DataBase64   string `json:"dataBase64"`
	PageCount    int    `json:"pageCount"`
	Pages        []Page `json:"pages"`
	SourceText   string `json:"sourceText"`
	ExtractedLen int    `json:"extractedLength"`
}

type ComposeRequest struct {
	SourceName       string `json:"sourceName"`
	SourceDataBase64 string `json:"sourceDataBase64"`
	Pages            []Page `json:"pages"`
	TranslatedText   string `json:"translatedText"`
}

type Result struct {
	Name       string `json:"name"`
	DataBase64 string `json:"dataBase64"`
	PageCount  int    `json:"pageCount"`
	Warning    string `json:"warning,omitempty"`
}

type Engine interface {
	Name() string
	Analyze(path string) (Document, error)
	Compose(request ComposeRequest) (Result, error)
}
