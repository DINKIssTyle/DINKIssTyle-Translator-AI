package llm

import "testing"

func TestBuildPDFBlockChunksCarriesPageNumbersAcrossSplitBlocks(t *testing.T) {
	source := `[[DKST_PDF_PAGE:0001]]

[[DKST_PDF_BLOCK:0001:0001]]
[[DKST_PDF_ROLE:body]]
First page text that is deliberately long enough to split.

[[DKST_PDF_PAGE:0002]]

[[DKST_PDF_BLOCK:0002:0001]]
[[DKST_PDF_ROLE:caption]]
Second page text.`

	chunks := buildPDFBlockChunks(source, 18, true)
	if len(chunks) < 3 {
		t.Fatalf("expected a split first block and a second-page block, got %d chunks", len(chunks))
	}
	seenSecondPage := false
	for index, chunk := range chunks {
		if chunk.PDFPageNumber != 1 && chunk.PDFPageNumber != 2 {
			t.Fatalf("chunk %d has invalid PDF page number %d", index, chunk.PDFPageNumber)
		}
		if chunk.PDFPageNumber == 2 {
			seenSecondPage = true
		} else if seenSecondPage {
			t.Fatalf("page 1 chunk appeared after page 2 at index %d", index)
		}
	}
	if !seenSecondPage {
		t.Fatal("expected at least one page 2 chunk")
	}
}
