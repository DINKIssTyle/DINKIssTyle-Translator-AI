package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dinkisstyle-translator/internal/file"
	"dinkisstyle-translator/internal/llm"
	"dinkisstyle-translator/internal/pdfengine"
)

const (
	pdfCheckpointVersion       = 1
	pdfCheckpointDirectoryName = "pdf-checkpoint"
	pdfCheckpointManifestName  = "manifest.json"
	pdfCheckpointSourceName    = "source.pdf"
	pdfCheckpointPartialName   = "translated-partial.pdf"
	pdfCheckpointPartialMeta   = "translated-partial.json"
)

type pdfCheckpointManifest struct {
	Version          int              `json:"version"`
	SourceName       string           `json:"sourceName"`
	SourceSHA256     string           `json:"sourceSha256"`
	PageCount        int              `json:"pageCount"`
	Pages            []pdfengine.Page `json:"pages"`
	SourceText       string           `json:"sourceText"`
	ExtractedLen     int              `json:"extractedLength"`
	CompletedPages   int              `json:"completedPages"`
	TranslatedText   string           `json:"translatedText"`
	Status           string           `json:"status"`
	UpdatedAt        time.Time        `json:"updatedAt"`
	TargetLanguage   string           `json:"targetLanguage,omitempty"`
	TranslationModel string           `json:"translationModel,omitempty"`
	CleanShutdown    bool             `json:"cleanShutdown"`
}

type pdfCheckpointPartialMetadata struct {
	CompletedPages int `json:"completedPages"`
}

type PDFCheckpointRecovery struct {
	Found          bool             `json:"found"`
	Document       file.PDFDocument `json:"document"`
	Result         file.PDFResult   `json:"result"`
	CompletedPages int              `json:"completedPages"`
	TotalPages     int              `json:"totalPages"`
	Status         string           `json:"status"`
	Error          string           `json:"error,omitempty"`
}

type pdfPageReadyPayload struct {
	PageNumber     int            `json:"pageNumber"`
	CompletedPages int            `json:"completedPages"`
	TotalPages     int            `json:"totalPages"`
	Result         file.PDFResult `json:"result"`
}

type pdfCheckpointStore struct {
	dir string
}

func newPDFCheckpointStore() *pdfCheckpointStore {
	return &pdfCheckpointStore{dir: filepath.Join(getAppConfigDir(), pdfCheckpointDirectoryName)}
}

func (s *pdfCheckpointStore) manifestPath() string {
	return filepath.Join(s.dir, pdfCheckpointManifestName)
}

func (s *pdfCheckpointStore) sourcePath() string {
	return filepath.Join(s.dir, pdfCheckpointSourceName)
}

func (s *pdfCheckpointStore) partialPath() string {
	return filepath.Join(s.dir, pdfCheckpointPartialName)
}

func (s *pdfCheckpointStore) partialMetadataPath() string {
	return filepath.Join(s.dir, pdfCheckpointPartialMeta)
}

func (s *pdfCheckpointStore) prepare(document file.PDFDocument, req llm.TranslationRequest) (pdfCheckpointManifest, error) {
	source, err := base64.StdEncoding.DecodeString(document.DataBase64)
	if err != nil || len(source) < 5 || string(source[:5]) != "%PDF-" {
		return pdfCheckpointManifest{}, errors.New("cannot checkpoint an invalid PDF source")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return pdfCheckpointManifest{}, err
	}
	if err := writeFileAtomically(s.sourcePath(), source, 0o600); err != nil {
		return pdfCheckpointManifest{}, fmt.Errorf("save checkpoint source: %w", err)
	}
	manifest := pdfCheckpointManifest{
		Version:          pdfCheckpointVersion,
		SourceName:       document.Name,
		SourceSHA256:     checksumHex(source),
		PageCount:        document.PageCount,
		Pages:            document.Pages,
		SourceText:       document.SourceText,
		ExtractedLen:     document.ExtractedLen,
		Status:           "in_progress",
		UpdatedAt:        time.Now(),
		TargetLanguage:   req.TargetLang,
		TranslationModel: req.Settings.Model,
		CleanShutdown:    false,
	}
	if err := s.saveManifest(manifest); err != nil {
		return pdfCheckpointManifest{}, err
	}
	return manifest, nil
}

func (s *pdfCheckpointStore) saveManifest(manifest pdfCheckpointManifest) error {
	manifest.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomically(s.manifestPath(), data, 0o600)
}

func (s *pdfCheckpointStore) load() (pdfCheckpointManifest, []byte, error) {
	raw, err := os.ReadFile(s.manifestPath())
	if err != nil {
		return pdfCheckpointManifest{}, nil, err
	}
	var manifest pdfCheckpointManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return pdfCheckpointManifest{}, nil, fmt.Errorf("read PDF checkpoint manifest: %w", err)
	}
	if manifest.Version != pdfCheckpointVersion || manifest.PageCount <= 0 || len(manifest.Pages) == 0 {
		return pdfCheckpointManifest{}, nil, errors.New("unsupported or incomplete PDF checkpoint")
	}
	source, err := os.ReadFile(s.sourcePath())
	if err != nil {
		return pdfCheckpointManifest{}, nil, fmt.Errorf("read PDF checkpoint source: %w", err)
	}
	if checksumHex(source) != manifest.SourceSHA256 {
		return pdfCheckpointManifest{}, nil, errors.New("PDF checkpoint source checksum does not match")
	}
	return manifest, source, nil
}

func (s *pdfCheckpointStore) savePartial(result file.PDFResult, completedPages int) error {
	data, err := base64.StdEncoding.DecodeString(result.DataBase64)
	if err != nil || len(data) < 5 || string(data[:5]) != "%PDF-" {
		return errors.New("partial translated PDF data is invalid")
	}
	if err := writeFileAtomically(s.partialPath(), data, 0o600); err != nil {
		return err
	}
	meta, err := json.Marshal(pdfCheckpointPartialMetadata{CompletedPages: completedPages})
	if err != nil {
		return err
	}
	return writeFileAtomically(s.partialMetadataPath(), meta, 0o600)
}

func (s *pdfCheckpointStore) loadPartial(completedPages int, sourceName string) (file.PDFResult, bool) {
	metaRaw, err := os.ReadFile(s.partialMetadataPath())
	if err != nil {
		return file.PDFResult{}, false
	}
	var meta pdfCheckpointPartialMetadata
	if json.Unmarshal(metaRaw, &meta) != nil || meta.CompletedPages != completedPages {
		return file.PDFResult{}, false
	}
	data, err := os.ReadFile(s.partialPath())
	if err != nil || len(data) < 5 || string(data[:5]) != "%PDF-" {
		return file.PDFResult{}, false
	}
	return file.PDFResult{
		Name:       recoveredTranslatedPDFName(sourceName),
		DataBase64: base64.StdEncoding.EncodeToString(data),
		PageCount:  completedPages,
	}, true
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".checkpoint-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func checksumHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func recoveredTranslatedPDFName(sourceName string) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(sourceName)), filepath.Ext(sourceName))
	if base == "" || base == "." {
		base = "translation"
	}
	return base + "-translated.pdf"
}

type pdfTranslationSink struct {
	app      *App
	store    *pdfCheckpointStore
	document file.PDFDocument
	manifest pdfCheckpointManifest
}

func (a *App) translatePDF(req llm.TranslationRequest) error {
	a.pdfRunMu.Lock()
	defer a.pdfRunMu.Unlock()

	a.pdfMu.Lock()
	document := a.activePDF
	a.pdfMu.Unlock()
	if document.PageCount <= 0 || len(document.Pages) == 0 {
		return errors.New("open a PDF before starting PDF translation")
	}

	store := newPDFCheckpointStore()
	a.pdfCheckpointMu.Lock()
	manifest, err := store.prepare(document, req)
	a.pdfCheckpointMu.Unlock()
	if err != nil {
		return fmt.Errorf("prepare PDF translation checkpoint: %w", err)
	}

	sink := &pdfTranslationSink{app: a, store: store, document: document, manifest: manifest}
	_, _, err = a.llm.TranslateTextStream(req, sink)
	if err != nil {
		sink.markInterrupted()
	}
	return err
}

func (s *pdfTranslationSink) Token(token string) {
	s.app.wails.Event.Emit("translation:token", token)
}

func (s *pdfTranslationSink) Chunk(payload llm.TranslationChunkPayload) {
	s.app.wails.Event.Emit("translation:chunk", payload)
}

func (s *pdfTranslationSink) Page(payload llm.TranslationPagePayload) {
	completedPages := s.completedPagesIncludingEmptyFollowers(payload.PageNumber)
	result, err := s.commit(payload.TranslatedText, completedPages, "in_progress")
	if err != nil {
		s.app.wails.Event.Emit("translation:pdf-page-error", map[string]any{
			"pageNumber": payload.PageNumber,
			"message":    err.Error(),
		})
		return
	}
	s.app.wails.Event.Emit("translation:pdf-page", pdfPageReadyPayload{
		PageNumber:     payload.PageNumber,
		CompletedPages: completedPages,
		TotalPages:     s.document.PageCount,
		Result:         result,
	})
}

func (s *pdfTranslationSink) Clear() {
	s.app.wails.Event.Emit("translation:clear")
}

func (s *pdfTranslationSink) Complete(payload llm.TranslationCompletePayload) {
	if s.manifest.CompletedPages < s.document.PageCount {
		result, err := s.commit(payload.Text, s.document.PageCount, "completed")
		if err != nil {
			s.app.wails.Event.Emit("translation:pdf-page-error", map[string]any{
				"pageNumber": s.document.PageCount,
				"message":    err.Error(),
			})
		} else {
			s.app.wails.Event.Emit("translation:pdf-page", pdfPageReadyPayload{
				PageNumber:     s.document.PageCount,
				CompletedPages: s.document.PageCount,
				TotalPages:     s.document.PageCount,
				Result:         result,
			})
		}
	} else {
		s.manifest.TranslatedText = payload.Text
		s.manifest.Status = "completed"
		s.app.pdfCheckpointMu.Lock()
		_ = s.store.saveManifest(s.manifest)
		s.app.pdfCheckpointMu.Unlock()
	}
	s.app.wails.Event.Emit("translation:complete", payload)
}

func (s *pdfTranslationSink) Progress(payload llm.TranslationProgressPayload) {
	s.app.wails.Event.Emit("translation:progress", payload)
}

func (s *pdfTranslationSink) Stats(payload llm.TranslationStatsPayload) {
	s.app.wails.Event.Emit("translation:stats", payload)
}

func (s *pdfTranslationSink) Debug(direction string, endpoint string, payload string) {
	s.app.wails.Event.Emit("translation:debug", map[string]string{
		"direction": direction,
		"endpoint":  endpoint,
		"payload":   payload,
	})
}

func (s *pdfTranslationSink) completedPagesIncludingEmptyFollowers(pageNumber int) int {
	completed := pageNumber
	for completed < len(s.document.Pages) && len(s.document.Pages[completed].Blocks) == 0 {
		completed++
	}
	if completed > s.document.PageCount {
		return s.document.PageCount
	}
	return completed
}

func (s *pdfTranslationSink) commit(translatedText string, completedPages int, status string) (file.PDFResult, error) {
	if completedPages <= 0 {
		return file.PDFResult{}, errors.New("no completed PDF pages to checkpoint")
	}
	if completedPages > len(s.document.Pages) {
		completedPages = len(s.document.Pages)
	}
	s.manifest.CompletedPages = completedPages
	s.manifest.TranslatedText = translatedText
	s.manifest.Status = status

	s.app.pdfCheckpointMu.Lock()
	defer s.app.pdfCheckpointMu.Unlock()
	if err := s.store.saveManifest(s.manifest); err != nil {
		return file.PDFResult{}, fmt.Errorf("save page checkpoint: %w", err)
	}
	result, err := file.BuildTranslatedPDF(file.PDFCreateRequest{
		SourceName:       s.document.Name,
		SourceDataBase64: s.document.DataBase64,
		Pages:            s.document.Pages[:completedPages],
		TranslatedText:   translatedText,
	})
	if err != nil {
		return file.PDFResult{}, fmt.Errorf("build translated pages 1-%d: %w", completedPages, err)
	}
	if err := s.store.savePartial(result, completedPages); err != nil {
		return file.PDFResult{}, fmt.Errorf("save translated pages 1-%d: %w", completedPages, err)
	}
	return result, nil
}

func (s *pdfTranslationSink) markInterrupted() {
	if s.manifest.CompletedPages <= 0 {
		return
	}
	s.manifest.Status = "interrupted"
	s.app.pdfCheckpointMu.Lock()
	_ = s.store.saveManifest(s.manifest)
	s.app.pdfCheckpointMu.Unlock()
}

func (a *App) RecoverPDFCheckpoint() (PDFCheckpointRecovery, error) {
	store := newPDFCheckpointStore()
	a.pdfCheckpointMu.Lock()
	defer a.pdfCheckpointMu.Unlock()

	manifest, source, err := store.load()
	if errors.Is(err, os.ErrNotExist) {
		return PDFCheckpointRecovery{}, nil
	}
	if err != nil {
		return PDFCheckpointRecovery{}, err
	}
	if !shouldRecoverPDFCheckpoint(manifest) {
		return PDFCheckpointRecovery{}, nil
	}
	document := file.PDFDocument{
		Name:         manifest.SourceName,
		DataBase64:   base64.StdEncoding.EncodeToString(source),
		PageCount:    manifest.PageCount,
		Pages:        manifest.Pages,
		SourceText:   manifest.SourceText,
		ExtractedLen: manifest.ExtractedLen,
	}
	recovery := PDFCheckpointRecovery{
		Found:          true,
		Document:       document,
		CompletedPages: manifest.CompletedPages,
		TotalPages:     manifest.PageCount,
		Status:         manifest.Status,
	}
	if partial, ok := store.loadPartial(manifest.CompletedPages, manifest.SourceName); ok {
		recovery.Result = partial
	} else {
		result, buildErr := file.BuildTranslatedPDF(file.PDFCreateRequest{
			SourceName:       document.Name,
			SourceDataBase64: document.DataBase64,
			Pages:            document.Pages[:manifest.CompletedPages],
			TranslatedText:   manifest.TranslatedText,
		})
		if buildErr != nil {
			recovery.Error = buildErr.Error()
		} else {
			recovery.Result = result
			_ = store.savePartial(result, manifest.CompletedPages)
		}
	}
	a.pdfMu.Lock()
	a.activePDF = document
	a.pdfMu.Unlock()
	return recovery, nil
}

func shouldRecoverPDFCheckpoint(manifest pdfCheckpointManifest) bool {
	return manifest.CompletedPages > 0 && !manifest.CleanShutdown
}

func (a *App) markPDFCheckpointCleanShutdown() {
	store := newPDFCheckpointStore()
	a.pdfCheckpointMu.Lock()
	defer a.pdfCheckpointMu.Unlock()

	manifest, _, err := store.load()
	if err != nil {
		return
	}
	manifest.CleanShutdown = true
	_ = store.saveManifest(manifest)
}

var _ interface {
	Token(string)
	Chunk(llm.TranslationChunkPayload)
	Page(llm.TranslationPagePayload)
	Clear()
	Complete(llm.TranslationCompletePayload)
	Progress(llm.TranslationProgressPayload)
	Stats(llm.TranslationStatsPayload)
	Debug(string, string, string)
} = (*pdfTranslationSink)(nil)
