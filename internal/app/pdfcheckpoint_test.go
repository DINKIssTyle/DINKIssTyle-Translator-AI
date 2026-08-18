package app

import (
	"encoding/base64"
	"path/filepath"
	"testing"

	"dinkisstyle-translator/internal/file"
	"dinkisstyle-translator/internal/llm"
	"dinkisstyle-translator/internal/pdfengine"
)

func TestPDFCheckpointManifestPreservesCompletedTranslation(t *testing.T) {
	store := &pdfCheckpointStore{dir: filepath.Join(t.TempDir(), "checkpoint")}
	source := []byte("%PDF-checkpoint-test")
	document := file.PDFDocument{
		Name:       "source.pdf",
		DataBase64: base64.StdEncoding.EncodeToString(source),
		PageCount:  2,
		Pages: []pdfengine.Page{
			{PageNumber: 1, Width: 595, Height: 842},
			{PageNumber: 2, Width: 595, Height: 842},
		},
		SourceText:   "source text",
		ExtractedLen: 11,
	}
	manifest, err := store.prepare(document, llm.TranslationRequest{TargetLang: "Korean"})
	if err != nil {
		t.Fatal(err)
	}
	manifest.CompletedPages = 1
	manifest.TranslatedText = "[[DKST_PDF_BLOCK:0001:0001]]\n번역"
	manifest.Status = "interrupted"
	if err := store.saveManifest(manifest); err != nil {
		t.Fatal(err)
	}

	recovered, recoveredSource, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if string(recoveredSource) != string(source) {
		t.Fatalf("source mismatch: %q", recoveredSource)
	}
	if recovered.CompletedPages != 1 || recovered.TranslatedText != manifest.TranslatedText {
		t.Fatalf("checkpoint lost page progress: %#v", recovered)
	}
	if recovered.Status != "interrupted" {
		t.Fatalf("unexpected status %q", recovered.Status)
	}
}

func TestPDFCheckpointRecoveryRequiresAbnormalExit(t *testing.T) {
	manifest := pdfCheckpointManifest{
		PageCount:      5,
		CompletedPages: 3,
		Status:         "interrupted",
	}
	if !shouldRecoverPDFCheckpoint(manifest) {
		t.Fatal("an abnormal exit checkpoint with incompleted pages should be recoverable")
	}
	manifest.CleanShutdown = true
	if shouldRecoverPDFCheckpoint(manifest) {
		t.Fatal("a checkpoint closed by normal app shutdown must not be restored")
	}

	// Completed status must not be recovered
	manifest.CleanShutdown = false
	manifest.Status = "completed"
	if shouldRecoverPDFCheckpoint(manifest) {
		t.Fatal("a completed translation must not be restored as a checkpoint")
	}

	// All pages completed must not be recovered
	manifest.Status = "in_progress"
	manifest.CompletedPages = 5
	if shouldRecoverPDFCheckpoint(manifest) {
		t.Fatal("a checkpoint with all pages completed must not be restored")
	}
}
