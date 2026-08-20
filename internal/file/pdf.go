// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package file

import (
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"dinkisstyle-translator/internal/pdfengine"
	pdfreader "github.com/ledongthuc/pdf"
	"github.com/signintech/gopdf"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed fonts/NanumGothic.ttf
var embeddedNanumGothic []byte

const (
	pdfPageMarkerFormat  = "[[DKST_PDF_PAGE:%04d]]"
	pdfBlockMarkerFormat = "[[DKST_PDF_BLOCK:%04d:%04d]]"
)

var pdfPageMarkerPattern = regexp.MustCompile(`\[\[DKST_PDF_PAGE:(\d+)\]\]`)
var pdfBlockMarkerPattern = regexp.MustCompile(`\[\[DKST_PDF_BLOCK:(\d+):(\d+)\]\]`)

type PDFTextBlock = pdfengine.TextBlock
type PDFTextRegion = pdfengine.TextRegion
type PDFPage = pdfengine.Page
type PDFDocument = pdfengine.Document
type PDFCreateRequest = pdfengine.ComposeRequest
type PDFResult = pdfengine.Result

type cleanroomPDFEngine struct{}

func newCleanroomPDFEngine() pdfengine.Engine {
	return cleanroomPDFEngine{}
}

func (cleanroomPDFEngine) Name() string { return "dkst-cleanroom" }

func (cleanroomPDFEngine) Analyze(path string) (pdfengine.Document, error) {
	return analyzePDF(path)
}

func (cleanroomPDFEngine) Compose(request pdfengine.ComposeRequest) (pdfengine.Result, error) {
	return composeTranslatedPDF(request)
}

var defaultPDFEngine = newCleanroomPDFEngine()

func (f *FileHandler) OpenPDF() (PDFDocument, error) {
	selection, err := f.app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title: "Select PDF Document",
		Filters: []application.FileFilter{
			{DisplayName: "PDF Documents (*.pdf)", Pattern: "*.pdf"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		return PDFDocument{}, err
	}
	if selection == "" {
		return PDFDocument{}, nil
	}
	return f.pdfEngine.Analyze(selection)
}

func ReadPDF(path string) (PDFDocument, error) {
	return defaultPDFEngine.Analyze(path)
}

func analyzePDF(path string) (PDFDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PDFDocument{}, err
	}
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		return PDFDocument{}, errors.New("the selected file is not a valid PDF document")
	}

	file, reader, err := pdfreader.Open(path)
	if err != nil {
		return PDFDocument{}, fmt.Errorf("open PDF: %w", err)
	}
	defer file.Close()

	pageCount := reader.NumPage()
	if pageCount <= 0 {
		return PDFDocument{}, errors.New("the PDF does not contain any pages")
	}

	pages := make([]PDFPage, 0, pageCount)
	var source strings.Builder
	extractedLen := 0
	for pageNumber := 1; pageNumber <= pageCount; pageNumber++ {
		page := reader.Page(pageNumber)
		width, height := pageDimensions(page)
		text, blocks, pageErr := extractPDFPageLayout(page, pageNumber, width, height)
		if pageErr != nil {
			return PDFDocument{}, fmt.Errorf("extract text from page %d: %w", pageNumber, pageErr)
		}
		text = strings.TrimSpace(text)
		pages = append(pages, PDFPage{
			PageNumber: pageNumber,
			Width:      width,
			Height:     height,
			Text:       text,
			Blocks:     blocks,
		})
		extractedLen += utf8.RuneCountInString(text)
		if source.Len() > 0 {
			source.WriteString("\n\n")
		}
		source.WriteString(fmt.Sprintf(pdfPageMarkerFormat, pageNumber))
		for _, block := range blocks {
			source.WriteString("\n\n")
			source.WriteString(fmt.Sprintf(pdfBlockMarkerFormat, pageNumber, block.BlockIndex))
			source.WriteString("\n")
			source.WriteString(fmt.Sprintf("[[DKST_PDF_ROLE:%s]]", block.Role))
			source.WriteString("\n")
			source.WriteString(block.Text)
		}
	}

	if extractedLen == 0 {
		return PDFDocument{}, errors.New("no selectable text was found in this PDF; scanned image PDFs require OCR before translation")
	}

	return PDFDocument{
		Name:         filepath.Base(path),
		DataBase64:   base64.StdEncoding.EncodeToString(data),
		PageCount:    pageCount,
		Pages:        pages,
		SourceText:   source.String(),
		ExtractedLen: extractedLen,
	}, nil
}

func pageDimensions(page pdfreader.Page) (float64, float64) {
	box := inheritedPDFValue(page.V, "CropBox")
	if box.Len() != 4 {
		box = inheritedPDFValue(page.V, "MediaBox")
	}
	width, height := 595.0, 842.0
	if box.Len() == 4 {
		x0, y0 := box.Index(0).Float64(), box.Index(1).Float64()
		x1, y1 := box.Index(2).Float64(), box.Index(3).Float64()
		if candidate := math.Abs(x1 - x0); candidate > 10 {
			width = candidate
		}
		if candidate := math.Abs(y1 - y0); candidate > 10 {
			height = candidate
		}
	}
	rotation := inheritedPDFValue(page.V, "Rotate").Int64() % 360
	if rotation < 0 {
		rotation += 360
	}
	if rotation == 90 || rotation == 270 {
		width, height = height, width
	}
	return width, height
}

func inheritedPDFValue(value pdfreader.Value, key string) pdfreader.Value {
	for current := value; !current.IsNull(); current = current.Key("Parent") {
		if found := current.Key(key); !found.IsNull() {
			return found
		}
	}
	return pdfreader.Value{}
}

type pdfTextLine struct {
	x         float64
	top       float64
	width     float64
	height    float64
	fontSize  float64
	fragments []pdfreader.Text
}

type pdfTextBlockBuilder struct {
	x        float64
	top      float64
	right    float64
	bottom   float64
	fontSize float64
	lines    []pdfTextLine
}

func extractPDFPageText(page pdfreader.Page) (string, error) {
	width, height := pageDimensions(page)
	text, _, err := extractPDFPageLayout(page, 1, width, height)
	return text, err
}

func extractPDFPageLayout(page pdfreader.Page, pageNumber int, pageWidth, pageHeight float64) (result string, blocks []PDFTextBlock, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ""
			blocks = nil
			err = fmt.Errorf("%v", recovered)
		}
	}()
	if page.V.IsNull() || page.V.Key("Contents").IsNull() {
		return "", nil, nil
	}

	fragments := append([]pdfreader.Text(nil), page.Content().Text...)
	fragments = filterPDFTextFragments(fragments)
	if len(fragments) == 0 {
		return "", nil, nil
	}
	sort.SliceStable(fragments, func(i, j int) bool {
		if math.Abs(fragments[i].Y-fragments[j].Y) > 1.5 {
			return fragments[i].Y > fragments[j].Y
		}
		return fragments[i].X < fragments[j].X
	})

	lines := make([]pdfTextLine, 0)
	for _, fragment := range fragments {
		tolerance := math.Max(1.5, fragment.FontSize*0.18)
		lineIndex := -1
		for index := len(lines) - 1; index >= 0 && index >= len(lines)-4; index-- {
			baseline := pageHeight - lines[index].top - lines[index].height*0.78
			if math.Abs(baseline-fragment.Y) <= tolerance && pdfFragmentBelongsToLine(lines[index], fragment) {
				lineIndex = index
				break
			}
		}
		estimatedWidth := float64(utf8.RuneCountInString(fragment.S)) * math.Max(fragment.FontSize, 8) * 0.42
		fragmentWidth := fragment.W
		if fragmentWidth < estimatedWidth*0.2 {
			fragmentWidth = estimatedWidth
		}
		fragmentHeight := math.Max(2, fragment.FontSize*1.18)
		fragmentTop := pageHeight - fragment.Y - fragment.FontSize*0.86
		if lineIndex < 0 {
			lines = append(lines, pdfTextLine{
				x: fragment.X, top: fragmentTop, width: fragmentWidth, height: fragmentHeight,
				fontSize: fragment.FontSize, fragments: []pdfreader.Text{fragment},
			})
			continue
		}
		line := &lines[lineIndex]
		line.fragments = append(line.fragments, fragment)
		left := math.Min(line.x, fragment.X)
		right := math.Max(line.x+line.width, fragment.X+fragmentWidth)
		line.x, line.width = left, right-left
		line.top = math.Min(line.top, fragmentTop)
		line.height = math.Max(line.height, fragmentHeight)
		if fragment.FontSize > line.fontSize {
			line.fontSize = fragment.FontSize
		}
	}

	for index := range lines {
		sort.SliceStable(lines[index].fragments, func(i, j int) bool {
			return lines[index].fragments[i].X < lines[index].fragments[j].X
		})
		lineText := joinPDFLineFragments(lines[index].fragments)
		estimatedWidth := float64(utf8.RuneCountInString(lineText)) * math.Max(lines[index].fontSize, 8) * 0.42
		rawLeft, rawRight := math.MaxFloat64, -math.MaxFloat64
		for _, fragment := range lines[index].fragments {
			rawLeft = math.Min(rawLeft, fragment.X)
			rawRight = math.Max(rawRight, fragment.X+math.Max(0, fragment.W))
		}
		rawWidth := rawRight - rawLeft
		if rawWidth >= estimatedWidth*0.25 {
			lines[index].x = rawLeft
			lines[index].width = rawWidth
		} else {
			// Some CID fonts expose every glyph at the same extraction X position.
			// In that case the raw span is unusable and a conservative estimate is
			// safer than a near-zero text box.
			lines[index].width = estimatedWidth
		}
		lines[index].width = math.Min(lines[index].width, math.Max(1, pageWidth-lines[index].x))
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if math.Abs(lines[i].top-lines[j].top) > 2 {
			return lines[i].top < lines[j].top
		}
		return lines[i].x < lines[j].x
	})

	built := make([]pdfTextBlockBuilder, 0, len(lines))
	for _, line := range lines {
		bestIndex, bestGap := -1, math.MaxFloat64
		for index := range built {
			candidate := &built[index]
			gap := line.top - candidate.bottom
			fontMax := math.Max(candidate.fontSize, line.fontSize)
			fontRatio := fontMax / math.Max(1, math.Min(candidate.fontSize, line.fontSize))
			leftAligned := math.Abs(candidate.x-line.x) <= fontMax*0.9
			overlap := horizontalOverlap(candidate.x, candidate.right, line.x, line.x+line.width)
			if gap < -fontMax*0.35 || gap > fontMax*0.72 || fontRatio > 1.32 || (!leftAligned && overlap < 0.45) {
				continue
			}
			if gap < bestGap {
				bestIndex, bestGap = index, gap
			}
		}
		if bestIndex < 0 {
			built = append(built, pdfTextBlockBuilder{
				x: line.x, top: line.top, right: line.x + line.width, bottom: line.top + line.height,
				fontSize: line.fontSize, lines: []pdfTextLine{line},
			})
			continue
		}
		block := &built[bestIndex]
		block.x = math.Min(block.x, line.x)
		block.top = math.Min(block.top, line.top)
		block.right = math.Max(block.right, line.x+line.width)
		block.bottom = math.Max(block.bottom, line.top+line.height)
		block.fontSize = math.Max(block.fontSize, line.fontSize)
		block.lines = append(block.lines, line)
	}

	fontSizes := make([]float64, 0, len(built))
	for _, block := range built {
		fontSizes = append(fontSizes, block.fontSize)
	}
	sort.Float64s(fontSizes)
	medianFontSize := 10.0
	if len(fontSizes) > 0 {
		medianFontSize = fontSizes[len(fontSizes)/2]
	}
	sort.SliceStable(built, func(i, j int) bool {
		if math.Abs(built[i].top-built[j].top) > math.Max(built[i].fontSize, built[j].fontSize) {
			return built[i].top < built[j].top
		}
		return built[i].x < built[j].x
	})

	var plainText strings.Builder
	for _, candidate := range built {
		sort.SliceStable(candidate.lines, func(i, j int) bool { return candidate.lines[i].top < candidate.lines[j].top })
		text := joinPDFBlockLines(candidate.lines)
		if text == "" {
			continue
		}
		role := "body"
		if candidate.fontSize >= medianFontSize*1.32 {
			role = "heading"
		} else if candidate.fontSize <= medianFontSize*0.84 || utf8.RuneCountInString(text) < 55 {
			role = "caption"
		}
		blockIndex := len(blocks) + 1
		blocks = append(blocks, PDFTextBlock{
			ID: fmt.Sprintf("p%04d-b%04d", pageNumber, blockIndex), PageNumber: pageNumber, BlockIndex: blockIndex,
			X: math.Max(0, candidate.x), Y: math.Max(0, candidate.top),
			Width: math.Max(8, candidate.right-candidate.x), Height: math.Max(4, candidate.bottom-candidate.top),
			FontSize: candidate.fontSize, Role: role, Text: text, Regions: pdfTextLineRegions(candidate.lines, pageWidth, pageHeight),
		})
		if plainText.Len() > 0 {
			plainText.WriteString("\n\n")
		}
		plainText.WriteString(text)
	}
	return plainText.String(), blocks, nil
}

func pdfTextLineRegions(lines []pdfTextLine, pageWidth, pageHeight float64) []PDFTextRegion {
	regions := make([]PDFTextRegion, 0, len(lines))
	for _, line := range lines {
		x := math.Max(0, line.x)
		y := math.Max(0, line.top)
		width := math.Min(math.Max(2, line.width), math.Max(2, pageWidth-x))
		height := math.Min(math.Max(2, line.height), math.Max(2, pageHeight-y))
		regions = append(regions, PDFTextRegion{X: x, Y: y, Width: width, Height: height})
	}
	return regions
}

func horizontalOverlap(leftA, rightA, leftB, rightB float64) float64 {
	overlap := math.Max(0, math.Min(rightA, rightB)-math.Max(leftA, leftB))
	return overlap / math.Max(1, math.Min(rightA-leftA, rightB-leftB))
}

func joinPDFBlockLines(lines []pdfTextLine) string {
	var builder strings.Builder
	for _, line := range lines {
		lineText := strings.TrimSpace(joinPDFLineFragments(line.fragments))
		if lineText == "" {
			continue
		}
		if builder.Len() > 0 {
			current := builder.String()
			if strings.HasSuffix(current, "-") && !strings.HasSuffix(current, " -") {
				builder.Reset()
				builder.WriteString(strings.TrimSuffix(current, "-"))
			} else {
				builder.WriteByte(' ')
			}
		}
		builder.WriteString(lineText)
	}
	return strings.TrimSpace(builder.String())
}

func filterPDFTextFragments(fragments []pdfreader.Text) []pdfreader.Text {
	filtered := make([]pdfreader.Text, 0, len(fragments))
	for _, fragment := range fragments {
		fragment.S = strings.ReplaceAll(fragment.S, "\u0000", "")
		fragment.S = strings.ReplaceAll(fragment.S, "\uFFFD", "")
		// ledongthuc/pdf reports rotated edge credits from some magazine PDFs as
		// zero-size, scrambled fragments. Rotated text needs a dedicated transform;
		// treating these fragments as horizontal boxes creates page-wide blocks.
		if fragment.S == "" || fragment.FontSize < 1 {
			continue
		}
		filtered = append(filtered, fragment)
	}
	return filtered
}

func pdfFragmentBelongsToLine(line pdfTextLine, fragment pdfreader.Text) bool {
	lineRight := line.x + line.width
	fragmentWidth := fragment.W
	if fragmentWidth <= 0 {
		fragmentWidth = float64(utf8.RuneCountInString(fragment.S)) * math.Max(fragment.FontSize, 8) * 0.42
	}
	fragmentRight := fragment.X + fragmentWidth
	if fragmentRight < line.x || fragment.X > lineRight {
		gap := math.Max(fragment.X-lineRight, line.x-fragmentRight)
		fontSize := math.Max(1, math.Max(line.fontSize, fragment.FontSize))
		// Normal inter-word gaps are a small fraction of the font size. A much
		// larger gap on the same baseline is almost always a column boundary.
		return gap <= math.Max(4, fontSize*1.45)
	}
	return true
}

func joinPDFLineFragments(fragments []pdfreader.Text) string {
	var builder strings.Builder
	var previous pdfreader.Text
	var lastRune rune
	hasLastRune := false
	pendingSpace := false
	for _, fragment := range fragments {
		text := strings.TrimSpace(fragment.S)
		if text == "" {
			if builder.Len() > 0 {
				pendingSpace = true
			}
			continue
		}
		if hasLastRune && (pendingSpace || needsPDFFragmentSpace(previous, fragment, lastRune, text)) {
			builder.WriteByte(' ')
		}
		builder.WriteString(text)
		pendingSpace = false
		lastRune, _ = utf8.DecodeLastRuneInString(text)
		hasLastRune = true
		previous = fragment
	}
	return builder.String()
}

func needsPDFFragmentSpace(previous, current pdfreader.Text, last rune, next string) bool {
	if next == "" || strings.HasPrefix(next, " ") {
		return false
	}
	first, _ := utf8.DecodeRuneInString(next)
	if isCJKRune(last) || isCJKRune(first) || unicode.IsPunct(first) || strings.ContainsRune("([{/'\"", last) {
		return false
	}
	previousWidth := previous.W
	if previousWidth <= 0 {
		previousWidth = float64(utf8.RuneCountInString(previous.S)) * math.Max(previous.FontSize, 8) * 0.45
	}
	gap := current.X - (previous.X + previousWidth)
	fontSize := math.Max(8, math.Max(previous.FontSize, current.FontSize))
	return gap > fontSize*0.12
}

func isCJKRune(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana)
}

func BuildTranslatedPDF(request PDFCreateRequest) (PDFResult, error) {
	return defaultPDFEngine.Compose(request)
}

func composeTranslatedPDF(request PDFCreateRequest) (PDFResult, error) {
	hasBlocks := false
	for _, page := range request.Pages {
		if len(page.Blocks) > 0 {
			hasBlocks = true
			break
		}
	}
	if strings.TrimSpace(request.SourceDataBase64) == "" || !hasBlocks {
		return buildReflowTranslatedPDF(request)
	}
	return buildLayoutPreservingTranslatedPDF(request)
}

func buildLayoutPreservingTranslatedPDF(request PDFCreateRequest) (result PDFResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = PDFResult{}
			err = fmt.Errorf("compose translated PDF layout: %v", recovered)
		}
	}()
	if len(request.Pages) == 0 {
		return PDFResult{}, errors.New("source PDF page information is missing")
	}
	sourceData, err := base64.StdEncoding.DecodeString(request.SourceDataBase64)
	if err != nil || len(sourceData) < 5 || string(sourceData[:5]) != "%PDF-" {
		return PDFResult{}, errors.New("source PDF data is invalid")
	}
	translatedByBlock, recoveredBlocks := splitTranslatedPDFBlocks(request.TranslatedText)
	backgroundData := sourceData
	textFreeBackground := false
	backgroundWarning := ""
	if cleanedSource, removedObjects, cleanErr := pdfengine.BuildTextFreeBackground(sourceData); cleanErr == nil && removedObjects > 0 {
		backgroundData = cleanedSource
		textFreeBackground = true
	} else if cleanErr != nil {
		backgroundWarning = "High-fidelity text cleanup was unavailable; used opaque block patches."
	}
	fontPath, err := findPDFFont()
	if err != nil {
		return PDFResult{}, err
	}

	document := &gopdf.GoPdf{}
	document.Start(gopdf.Config{Unit: gopdf.UnitPT, PageSize: normalizedPageSize(request.Pages[0])})
	document.SetInfo(gopdf.PdfInfo{
		Title: translatedPDFName(request.SourceName), Author: "DKST Translator AI",
		Subject: "Layout-preserving translated PDF", Creator: "DKST Translator AI",
		Producer: "DKST Translator AI", CreationDate: time.Now(),
	})
	if err := document.AddTTFFont("translation", fontPath); err != nil {
		return PDFResult{}, fmt.Errorf("load PDF font %s: %w", filepath.Base(fontPath), err)
	}
	sourceFile, err := os.CreateTemp("", "dkst-pdf-source-*.pdf")
	if err != nil {
		return PDFResult{}, fmt.Errorf("prepare source PDF: %w", err)
	}
	sourcePath := sourceFile.Name()
	defer os.Remove(sourcePath)
	if _, err := sourceFile.Write(backgroundData); err != nil {
		sourceFile.Close()
		return PDFResult{}, fmt.Errorf("prepare source PDF: %w", err)
	}
	if err := sourceFile.Close(); err != nil {
		return PDFResult{}, fmt.Errorf("prepare source PDF: %w", err)
	}

	totalBlocks, renderedBlocks, clippedBlocks := 0, 0, 0
	for _, page := range request.Pages {
		pageSize := normalizedPageSize(page)
		document.AddPageWithOption(gopdf.PageOption{PageSize: &pageSize})
		templateID := document.ImportPage(sourcePath, page.PageNumber, "/MediaBox")
		document.UseImportedTemplate(templateID, 0, 0, pageSize.W, pageSize.H)

		for _, block := range page.Blocks {
			totalBlocks++
			translated := strings.TrimSpace(translatedByBlock[block.ID])
			if translated == "" {
				continue
			}
			renderResult, err := drawTranslatedPDFBlock(document, pageSize, block, translated, !textFreeBackground)
			if err != nil {
				return PDFResult{}, fmt.Errorf("render block %s: %w", block.ID, err)
			}
			if renderResult.Clipped {
				clippedBlocks++
			}
			renderedBlocks++
		}
	}

	data, err := document.GetBytesPdfReturnErr()
	if err != nil {
		return PDFResult{}, fmt.Errorf("build translated PDF: %w", err)
	}
	result = PDFResult{
		Name: translatedPDFName(request.SourceName), DataBase64: base64.StdEncoding.EncodeToString(data),
		PageCount: len(request.Pages),
	}
	if renderedBlocks != totalBlocks {
		unmatchedDetail := "unmatched blocks retain their original text."
		if textFreeBackground {
			unmatchedDetail = "unmatched blocks were left blank rather than restoring source text."
		}
		result.Warning = fmt.Sprintf("Placed %d of %d translated text blocks; %s", renderedBlocks, totalBlocks, unmatchedDetail)
	} else if recoveredBlocks > totalBlocks {
		result.Warning = fmt.Sprintf("Placed all %d text blocks; duplicate structural markers were ignored.", totalBlocks)
	}
	if clippedBlocks > 0 {
		result.Warning = joinPDFWarnings(result.Warning, fmt.Sprintf("Clipped %d text blocks to prevent text from crossing their layout boundaries.", clippedBlocks))
	}
	result.Warning = joinPDFWarnings(result.Warning, backgroundWarning)
	return result, nil
}

func joinPDFWarnings(warnings ...string) string {
	filled := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if trimmed := strings.TrimSpace(warning); trimmed != "" {
			filled = append(filled, trimmed)
		}
	}
	return strings.Join(filled, " ")
}

func splitTranslatedPDFBlocks(text string) (map[string]string, int) {
	matches := pdfBlockMarkerPattern.FindAllStringSubmatchIndex(text, -1)
	result := make(map[string]string, len(matches))
	for index, match := range matches {
		pageNumber, blockNumber := 0, 0
		_, _ = fmt.Sscanf(text[match[2]:match[3]], "%d", &pageNumber)
		_, _ = fmt.Sscanf(text[match[4]:match[5]], "%d", &blockNumber)
		end := len(text)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		rawSegment := text[match[1]:end]
		cleaned := pdfPageMarkerPattern.ReplaceAllString(rawSegment, "")
		cleaned = cleanTranslatedBlockText(cleaned)
		id := fmt.Sprintf("p%04d-b%04d", pageNumber, blockNumber)
		if previous := result[id]; len([]rune(previous)) > len([]rune(cleaned)) {
			continue
		}
		result[id] = cleaned
	}
	return result, len(matches)
}

func cleanTranslatedBlockText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "*#` \t\r\n")
	text = strings.TrimPrefix(text, "-->")
	text = strings.TrimPrefix(text, ":")
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimSuffix(text, "<!--")
	return strings.TrimSpace(text)
}

type pdfBlockRenderResult struct {
	Clipped bool
}

type pdfBlockTypography struct {
	MaxFontSize    float64
	MinFontSize    float64
	LineHeightRate float64
}

type translatedPDFTextFit struct {
	FontSize   float64
	Lines      []string
	LineHeight float64
	Clipped    bool
}

type translatedPDFPlacedLine struct {
	X          float64
	Y          float64
	Width      float64
	Text       string
	LineHeight float64
}

type translatedPDFRegionFit struct {
	FontSize float64
	Lines    []translatedPDFPlacedLine
	Clipped  bool
}

func drawTranslatedPDFBlock(document *gopdf.GoPdf, pageSize gopdf.Rect, block PDFTextBlock, text string, coverSource bool) (pdfBlockRenderResult, error) {
	padding := math.Max(1.25, math.Min(3, block.FontSize*0.18))
	x := math.Max(0, block.X-padding)
	y := math.Max(0, block.Y-padding)
	width := math.Min(pageSize.W-x, block.Width+padding*2)
	height := math.Min(pageSize.H-y, block.Height+padding*2)
	if width <= 2 || height <= 2 {
		return pdfBlockRenderResult{}, nil
	}

	if coverSource {
		// Basic fallback for PDFs whose content streams cannot be rewritten safely.
		document.SetFillColor(255, 255, 255)
		if len(block.Regions) > 0 {
			for _, region := range block.Regions {
				regionX := math.Max(0, region.X-padding)
				regionY := math.Max(0, region.Y-padding)
				regionWidth := math.Min(pageSize.W-regionX, region.Width+padding*2)
				regionHeight := math.Min(pageSize.H-regionY, region.Height+padding*2)
				if regionWidth > 0 && regionHeight > 0 {
					document.RectFromUpperLeftWithStyle(regionX, regionY, regionWidth, regionHeight, "F")
				}
			}
		} else {
			document.RectFromUpperLeftWithStyle(x, y, width, height, "F")
		}
	}
	document.SetTextColor(30, 32, 36)

	typography := translatedPDFTypography(block)
	if len(block.Regions) > 0 {
		fit, err := fitTranslatedPDFTextRegions(document, text, block.Regions, typography)
		if err != nil {
			return pdfBlockRenderResult{}, err
		}
		if len(fit.Lines) == 0 {
			return pdfBlockRenderResult{Clipped: fit.Clipped}, nil
		}
		if err := document.SetFont("translation", "", fit.FontSize); err != nil {
			return pdfBlockRenderResult{}, err
		}
		for _, line := range fit.Lines {
			document.SetXY(line.X, line.Y)
			if err := document.Cell(&gopdf.Rect{W: line.Width, H: line.LineHeight}, line.Text); err != nil {
				return pdfBlockRenderResult{}, err
			}
		}
		return pdfBlockRenderResult{Clipped: fit.Clipped}, nil
	}
	fit, err := fitTranslatedPDFText(
		document,
		text,
		width-padding*2,
		height-padding*2,
		typography.MaxFontSize,
		typography.MinFontSize,
		typography.LineHeightRate,
	)
	if err != nil {
		return pdfBlockRenderResult{}, err
	}
	if len(fit.Lines) == 0 {
		return pdfBlockRenderResult{Clipped: fit.Clipped}, nil
	}
	if err := document.SetFont("translation", "", fit.FontSize); err != nil {
		return pdfBlockRenderResult{}, err
	}
	document.SetXY(x+padding, y+padding)
	for _, line := range fit.Lines {
		document.SetX(x + padding)
		if err := document.Cell(&gopdf.Rect{W: width - padding*2, H: fit.LineHeight}, line); err != nil {
			return pdfBlockRenderResult{}, err
		}
		document.Br(fit.LineHeight)
	}
	return pdfBlockRenderResult{Clipped: fit.Clipped}, nil
}

func fitTranslatedPDFTextRegions(document *gopdf.GoPdf, text string, regions []PDFTextRegion, typography pdfBlockTypography) (translatedPDFRegionFit, error) {
	for size := typography.MaxFontSize; size >= typography.MinFontSize; size -= 0.5 {
		if err := document.SetFont("translation", "", size); err != nil {
			return translatedPDFRegionFit{}, err
		}
		lines, complete, err := layoutTranslatedPDFTextInRegions(document, text, regions, size, typography.LineHeightRate)
		if err != nil {
			return translatedPDFRegionFit{}, err
		}
		if complete {
			return translatedPDFRegionFit{FontSize: size, Lines: lines}, nil
		}
	}

	if err := document.SetFont("translation", "", typography.MinFontSize); err != nil {
		return translatedPDFRegionFit{}, err
	}
	lines, complete, err := layoutTranslatedPDFTextInRegions(document, text, regions, typography.MinFontSize, typography.LineHeightRate)
	if err != nil {
		return translatedPDFRegionFit{}, err
	}
	if complete {
		return translatedPDFRegionFit{FontSize: typography.MinFontSize, Lines: lines}, nil
	}
	if len(lines) > 0 {
		last := &lines[len(lines)-1]
		limited, err := limitTranslatedPDFLines(document, []string{last.Text, "overflow"}, last.Width, 1)
		if err != nil {
			return translatedPDFRegionFit{}, err
		}
		if len(limited) > 0 {
			last.Text = limited[0]
		}
	}
	return translatedPDFRegionFit{FontSize: typography.MinFontSize, Lines: lines, Clipped: true}, nil
}

func layoutTranslatedPDFTextInRegions(document *gopdf.GoPdf, text string, regions []PDFTextRegion, fontSize, lineHeightRate float64) ([]translatedPDFPlacedLine, bool, error) {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	position := 0
	lineHeight := fontSize * lineHeightRate
	placed := make([]translatedPDFPlacedLine, 0, len(regions))
	for _, region := range regions {
		if position >= len(runes) {
			break
		}
		if region.Width <= 1 || region.Height <= 1 {
			continue
		}
		lineCapacity := int(math.Floor((region.Height + 0.01) / lineHeight))
		for lineIndex := 0; lineIndex < lineCapacity && position < len(runes); lineIndex++ {
			line, next, err := takeTranslatedPDFLine(document, runes, position, region.Width)
			if err != nil {
				return nil, false, err
			}
			if next <= position {
				break
			}
			if line != "" {
				placed = append(placed, translatedPDFPlacedLine{
					X: region.X, Y: region.Y + float64(lineIndex)*lineHeight,
					Width: region.Width, Text: line, LineHeight: lineHeight,
				})
			}
			position = next
		}
	}
	for position < len(runes) && unicode.IsSpace(runes[position]) {
		position++
	}
	return placed, position >= len(runes), nil
}

func takeTranslatedPDFLine(document *gopdf.GoPdf, runes []rune, start int, width float64) (string, int, error) {
	for start < len(runes) && unicode.IsSpace(runes[start]) {
		start++
	}
	if start >= len(runes) {
		return "", start, nil
	}
	lastSpace := -1
	for end := start + 1; end <= len(runes); end++ {
		candidate := string(runes[start:end])
		measured, err := document.MeasureTextWidth(candidate)
		if err != nil {
			return "", start, err
		}
		if measured > width {
			if lastSpace > start {
				return strings.TrimSpace(string(runes[start:lastSpace])), lastSpace + 1, nil
			}
			if end == start+1 {
				return candidate, end, nil
			}
			return strings.TrimSpace(string(runes[start : end-1])), end - 1, nil
		}
		if unicode.IsSpace(runes[end-1]) {
			lastSpace = end - 1
		}
	}
	return strings.TrimSpace(string(runes[start:])), len(runes), nil
}

func translatedPDFTypography(block PDFTextBlock) pdfBlockTypography {
	baseSize := math.Min(36, math.Max(4, block.FontSize))
	typography := pdfBlockTypography{
		MaxFontSize:    baseSize,
		MinFontSize:    math.Max(4.5, baseSize*0.56),
		LineHeightRate: 1.12,
	}
	switch block.Role {
	case "heading":
		typography.MinFontSize = math.Max(6, baseSize*0.62)
		typography.LineHeightRate = 1.06
	case "caption":
		typography.MaxFontSize = math.Min(18, baseSize)
		typography.MinFontSize = math.Max(3.75, baseSize*0.54)
		typography.LineHeightRate = 1.08
	}
	if typography.MinFontSize > typography.MaxFontSize {
		typography.MinFontSize = typography.MaxFontSize
	}
	return typography
}

func fitTranslatedPDFText(document *gopdf.GoPdf, text string, width, height, maxSize, minSize, lineHeightRate float64) (translatedPDFTextFit, error) {
	if width <= 0 || height <= 0 {
		return translatedPDFTextFit{Clipped: strings.TrimSpace(text) != ""}, nil
	}
	if lineHeightRate <= 0 {
		lineHeightRate = 1.12
	}
	for size := maxSize; size >= minSize; size -= 0.5 {
		if err := document.SetFont("translation", "", size); err != nil {
			return translatedPDFTextFit{}, err
		}
		lines, err := wrapTranslatedPDFText(document, text, width)
		if err != nil {
			return translatedPDFTextFit{}, err
		}
		lineHeight := size * lineHeightRate
		if float64(len(lines))*lineHeight <= height {
			return translatedPDFTextFit{FontSize: size, Lines: lines, LineHeight: lineHeight}, nil
		}
	}

	// A block must never paint outside its source layout rectangle. If even the
	// readable minimum size does not fit, retain as much text as the box can
	// safely hold and mark the result so the caller can report the overflow.
	effectiveMinSize := math.Min(minSize, height/lineHeightRate)
	effectiveMinSize = math.Max(2.5, effectiveMinSize)
	if effectiveMinSize*lineHeightRate > height {
		return translatedPDFTextFit{Clipped: strings.TrimSpace(text) != ""}, nil
	}
	if err := document.SetFont("translation", "", effectiveMinSize); err != nil {
		return translatedPDFTextFit{}, err
	}
	lines, err := wrapTranslatedPDFText(document, text, width)
	if err != nil {
		return translatedPDFTextFit{}, err
	}
	lineHeight := effectiveMinSize * lineHeightRate
	maxLines := int(math.Floor((height + 0.01) / lineHeight))
	if maxLines < 1 {
		return translatedPDFTextFit{Clipped: len(lines) > 0}, nil
	}
	if len(lines) <= maxLines {
		return translatedPDFTextFit{FontSize: effectiveMinSize, Lines: lines, LineHeight: lineHeight}, nil
	}
	limited, err := limitTranslatedPDFLines(document, lines, width, maxLines)
	if err != nil {
		return translatedPDFTextFit{}, err
	}
	return translatedPDFTextFit{
		FontSize: effectiveMinSize, Lines: limited, LineHeight: lineHeight, Clipped: true,
	}, nil
}

func limitTranslatedPDFLines(document *gopdf.GoPdf, lines []string, width float64, maxLines int) ([]string, error) {
	if maxLines <= 0 || len(lines) == 0 {
		return nil, nil
	}
	if len(lines) <= maxLines {
		return append([]string(nil), lines...), nil
	}
	limited := append([]string(nil), lines[:maxLines]...)
	last := strings.TrimSpace(limited[len(limited)-1])
	for {
		candidate := strings.TrimSpace(last) + "..."
		measured, err := document.MeasureTextWidth(candidate)
		if err != nil {
			return nil, err
		}
		if measured <= width || last == "" {
			limited[len(limited)-1] = candidate
			return limited, nil
		}
		runes := []rune(last)
		last = strings.TrimSpace(string(runes[:len(runes)-1]))
	}
}

func wrapTranslatedPDFText(document *gopdf.GoPdf, text string, width float64) ([]string, error) {
	if width <= 0 {
		return []string{strings.TrimSpace(text)}, nil
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return []string{""}, nil
	}
	lines := make([]string, 0)
	current := ""
	for _, r := range []rune(text) {
		candidate := current + string(r)
		measured, err := document.MeasureTextWidth(candidate)
		if err != nil {
			return nil, err
		}
		if measured <= width || current == "" {
			current = candidate
			continue
		}
		trimmed := strings.TrimSpace(current)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
		current = string(r)
	}
	if trimmed := strings.TrimSpace(current); trimmed != "" {
		lines = append(lines, trimmed)
	}
	if len(lines) == 0 {
		lines = append(lines, " ")
	}
	return lines, nil
}

func buildReflowTranslatedPDF(request PDFCreateRequest) (PDFResult, error) {
	if len(request.Pages) == 0 {
		return PDFResult{}, errors.New("source PDF page information is missing")
	}
	if strings.TrimSpace(request.TranslatedText) == "" {
		return PDFResult{}, errors.New("translated text is empty")
	}

	translatedPages, markerCount := splitTranslatedPDFPages(request.TranslatedText, request.Pages)
	fontPath, err := findPDFFont()
	if err != nil {
		return PDFResult{}, err
	}

	defaultSize := normalizedPageSize(request.Pages[0])
	document := &gopdf.GoPdf{}
	document.Start(gopdf.Config{Unit: gopdf.UnitPT, PageSize: defaultSize})
	document.SetInfo(gopdf.PdfInfo{
		Title:        translatedPDFName(request.SourceName),
		Author:       "DKST Translator AI",
		Subject:      "Translated PDF",
		Creator:      "DKST Translator AI",
		Producer:     "DKST Translator AI",
		CreationDate: time.Now(),
	})
	if err := document.AddTTFFont("translation", fontPath); err != nil {
		return PDFResult{}, fmt.Errorf("load PDF font %s: %w", filepath.Base(fontPath), err)
	}
	if err := document.SetFont("translation", "", 11); err != nil {
		return PDFResult{}, fmt.Errorf("set PDF font: %w", err)
	}

	physicalPages := 0
	for index, sourcePage := range request.Pages {
		pageText := ""
		if index < len(translatedPages) {
			pageText = strings.TrimSpace(translatedPages[index])
		}
		if pageText == "" {
			pageText = " "
		}
		added, err := writeTranslatedPage(document, sourcePage, pageText)
		if err != nil {
			return PDFResult{}, fmt.Errorf("compose translated page %d: %w", sourcePage.PageNumber, err)
		}
		physicalPages += added
	}

	data, err := document.GetBytesPdfReturnErr()
	if err != nil {
		return PDFResult{}, fmt.Errorf("build translated PDF: %w", err)
	}
	result := PDFResult{
		Name:       translatedPDFName(request.SourceName),
		DataBase64: base64.StdEncoding.EncodeToString(data),
		PageCount:  physicalPages,
	}
	if markerCount != len(request.Pages) {
		result.Warning = fmt.Sprintf("Recovered %d of %d PDF page markers; page text was redistributed where necessary.", markerCount, len(request.Pages))
	}
	return result, nil
}

func splitTranslatedPDFPages(text string, pages []PDFPage) ([]string, int) {
	matches := pdfPageMarkerPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return distributeTextAcrossPages(strings.TrimSpace(text), pages), 0
	}
	if len(matches) != len(pages) {
		unmarked := strings.TrimSpace(pdfPageMarkerPattern.ReplaceAllString(text, ""))
		return distributeTextAcrossPages(unmarked, pages), len(matches)
	}

	byPage := make(map[int]string, len(matches))
	for index, match := range matches {
		pageNumberText := text[match[2]:match[3]]
		pageNumber := 0
		_, _ = fmt.Sscanf(pageNumberText, "%d", &pageNumber)
		start := match[1]
		end := len(text)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		byPage[pageNumber] = strings.TrimSpace(text[start:end])
	}

	result := make([]string, len(pages))
	for index, page := range pages {
		if translated, ok := byPage[page.PageNumber]; ok {
			result[index] = translated
		} else {
			unmarked := strings.TrimSpace(pdfPageMarkerPattern.ReplaceAllString(text, ""))
			return distributeTextAcrossPages(unmarked, pages), len(matches)
		}
	}
	return result, len(matches)
}

func distributeTextAcrossPages(text string, pages []PDFPage) []string {
	result := make([]string, len(pages))
	if len(pages) == 0 || text == "" {
		return result
	}
	paragraphs := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\f' })
	cleaned := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if trimmed := strings.TrimSpace(paragraph); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return result
	}

	totalWeight := 0
	weights := make([]int, len(pages))
	for index, page := range pages {
		weights[index] = maxInt(1, utf8.RuneCountInString(page.Text))
		totalWeight += weights[index]
	}
	paragraphIndex := 0
	for pageIndex := range pages {
		remainingPages := len(pages) - pageIndex
		remainingParagraphs := len(cleaned) - paragraphIndex
		if remainingParagraphs <= 0 {
			break
		}
		take := int(math.Round(float64(len(cleaned)) * float64(weights[pageIndex]) / float64(totalWeight)))
		take = maxInt(1, take)
		if take > remainingParagraphs-(remainingPages-1) {
			take = maxInt(1, remainingParagraphs-(remainingPages-1))
		}
		if pageIndex == len(pages)-1 {
			take = remainingParagraphs
		}
		end := minInt(len(cleaned), paragraphIndex+take)
		result[pageIndex] = strings.Join(cleaned[paragraphIndex:end], "\n\n")
		paragraphIndex = end
	}
	return result
}

func writeTranslatedPage(document *gopdf.GoPdf, sourcePage PDFPage, text string) (int, error) {
	pageSize := normalizedPageSize(sourcePage)
	margin := math.Max(34, math.Min(pageSize.W, pageSize.H)*0.065)
	footerHeight := 24.0
	lineHeight := 16.5
	contentWidth := pageSize.W - margin*2
	contentBottom := pageSize.H - margin - footerHeight
	pageNumber := sourcePage.PageNumber
	continuation := 0
	physicalPages := 0

	addPage := func() error {
		size := pageSize
		document.AddPageWithOption(gopdf.PageOption{PageSize: &size})
		physicalPages++
		document.SetTextColor(112, 119, 132)
		if err := document.SetFont("translation", "", 8.5); err != nil {
			return err
		}
		document.SetXY(margin, margin*0.48)
		header := fmt.Sprintf("Translated page %d", pageNumber)
		if continuation > 0 {
			header += fmt.Sprintf(" - continued %d", continuation)
		}
		if err := document.Cell(&gopdf.Rect{W: contentWidth, H: 12}, header); err != nil {
			return err
		}
		document.SetTextColor(42, 45, 52)
		if err := document.SetFont("translation", "", 11); err != nil {
			return err
		}
		document.SetXY(margin, margin)
		return nil
	}
	if err := addPage(); err != nil {
		return 0, err
	}

	paragraphs := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			if document.GetY()+lineHeight*0.65 < contentBottom {
				document.Br(lineHeight * 0.65)
				document.SetX(margin)
			}
			continue
		}
		lines, err := document.SplitTextWithWordWrap(paragraph, contentWidth)
		if err != nil {
			lines, err = document.SplitText(paragraph, contentWidth)
		}
		if err != nil {
			return physicalPages, err
		}
		for _, line := range lines {
			if document.GetY()+lineHeight > contentBottom {
				if err := writeTranslatedFooter(document, pageSize, margin, pageNumber); err != nil {
					return physicalPages, err
				}
				continuation++
				if err := addPage(); err != nil {
					return physicalPages, err
				}
			}
			document.SetX(margin)
			if err := document.Cell(&gopdf.Rect{W: contentWidth, H: lineHeight}, line); err != nil {
				return physicalPages, err
			}
			document.Br(lineHeight)
		}
		document.Br(lineHeight * 0.45)
	}
	if err := writeTranslatedFooter(document, pageSize, margin, pageNumber); err != nil {
		return physicalPages, err
	}
	return physicalPages, nil
}

func writeTranslatedFooter(document *gopdf.GoPdf, pageSize gopdf.Rect, margin float64, sourcePage int) error {
	if err := document.SetFont("translation", "", 8); err != nil {
		return err
	}
	document.SetTextColor(112, 119, 132)
	document.SetXY(margin, pageSize.H-margin*0.72)
	footer := fmt.Sprintf("DKST Translator AI  |  Source page %d", sourcePage)
	if err := document.Cell(&gopdf.Rect{W: pageSize.W - margin*2, H: 12}, footer); err != nil {
		return err
	}
	document.SetTextColor(42, 45, 52)
	return document.SetFont("translation", "", 11)
}

func normalizedPageSize(page PDFPage) gopdf.Rect {
	width, height := page.Width, page.Height
	if width < 100 || width > 5000 {
		width = 595
	}
	if height < 100 || height > 5000 {
		height = 842
	}
	return gopdf.Rect{W: width, H: height}
}

func validatePDFFont(path string) bool {
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return false
	}
	d := &gopdf.GoPdf{}
	d.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	d.AddPage()
	if err := d.AddTTFFont("test", path); err != nil {
		return false
	}
	if err := d.SetFont("test", "", 12); err != nil {
		return false
	}
	w, err := d.MeasureTextWidth("한글")
	return err == nil && w > 15
}

func findPDFFont() (string, error) {
	// 1. Embedded fallback font: extract to temp cache and validate
	if len(embeddedNanumGothic) > 0 {
		cachePath := filepath.Join(os.TempDir(), "dkst-nanumgothic.ttf")
		if info, err := os.Stat(cachePath); err != nil || info.Size() != int64(len(embeddedNanumGothic)) {
			_ = os.WriteFile(cachePath, embeddedNanumGothic, 0o644)
		}
		if validatePDFFont(cachePath) {
			return cachePath, nil
		}
	}

	candidates := []string{}

	// 2. Check app bundle or executable resources directory
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "NanumGothic.ttf"),
			filepath.Join(exeDir, "fonts", "NanumGothic.ttf"),
			filepath.Join(exeDir, "..", "Resources", "fonts", "NanumGothic.ttf"),
			filepath.Join(exeDir, "..", "Resources", "NanumGothic.ttf"),
			filepath.Join(exeDir, "..", "Resources", "fonts", "NotoSansCJK-Regular.ttf"),
			filepath.Join(exeDir, "..", "Resources", "fonts", "Arial Unicode.ttf"),
			filepath.Join(exeDir, "fonts", "NotoSansCJK-Regular.ttf"),
			filepath.Join(exeDir, "fonts", "Arial Unicode.ttf"),
		)
	}
	if docs := getDocumentsDir(); docs != "" {
		candidates = append(candidates,
			filepath.Join(docs, "fonts", "NanumGothic.ttf"),
			filepath.Join(docs, "NanumGothic.ttf"),
			filepath.Join(docs, "fonts", "NotoSansCJK-Regular.ttf"),
			filepath.Join(docs, "fonts", "Arial Unicode.ttf"),
		)
	}

	// 3. OS-specific system font locations
	switch runtime.GOOS {
	case "darwin", "ios":
		candidates = append(candidates,
			"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
			"/System/Library/Fonts/Supplemental/NanumGothic.ttf",
			"/System/Library/Fonts/Supplemental/NotoSansKR-Regular.ttf",
			"/Library/Fonts/NanumGothic.ttf",
			"/Library/Fonts/Arial Unicode.ttf",
		)
	case "android":
		candidates = append(candidates,
			"/system/fonts/NotoSansCJK-Regular.ttc",
			"/system/fonts/NotoSansKR-Regular.otf",
			"/system/fonts/Roboto-Regular.ttf",
			"/system/fonts/DroidSansFallback.ttf",
		)
	case "windows":
		windowsDir := os.Getenv("WINDIR")
		if windowsDir == "" {
			windowsDir = `C:\Windows`
		}
		candidates = append(candidates,
			filepath.Join(windowsDir, "Fonts", "malgun.ttf"),
			filepath.Join(windowsDir, "Fonts", "msgothic.ttf"),
			filepath.Join(windowsDir, "Fonts", "arialuni.ttf"),
			filepath.Join(windowsDir, "Fonts", "msyh.ttc"),
			filepath.Join(windowsDir, "Fonts", "arial.ttf"),
			filepath.Join(windowsDir, "Fonts", "segoeui.ttf"),
		)
	default:
		candidates = append(candidates,
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/truetype/nanum/NanumGothic.ttf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
			"/usr/share/fonts/truetype/freefont/FreeSans.ttf",
		)
	}

	for _, candidate := range candidates {
		if validatePDFFont(candidate) {
			return candidate, nil
		}
	}

	return "", errors.New("no compatible Unicode TrueType font was found; install Noto Sans CJK, Nanum Gothic or Arial Unicode to create translated PDFs")
}

func translatedPDFName(sourceName string) string {
	base := strings.TrimSpace(filepath.Base(sourceName))
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" || base == "." {
		base = "translation"
	}
	return base + "-translated.pdf"
}

func (f *FileHandler) SavePDF(dataBase64, defaultFilename string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return "", fmt.Errorf("decode translated PDF: %w", err)
	}
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		return "", errors.New("translated PDF data is invalid")
	}
	if strings.TrimSpace(defaultFilename) == "" {
		defaultFilename = "translation.pdf"
	}
	if f.app != nil && f.app.Dialog != nil {
		selection, err := f.app.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
			Title:    "Save Translated PDF",
			Filename: filepath.Base(defaultFilename),
			Filters: []application.FileFilter{
				{DisplayName: "PDF Documents (*.pdf)", Pattern: "*.pdf"},
			},
		}).PromptForSingleSelection()
		if err == nil && selection != "" {
			if !strings.EqualFold(filepath.Ext(selection), ".pdf") {
				selection += ".pdf"
			}
			if err := os.WriteFile(selection, data, 0644); err != nil {
				return "", err
			}
			return selection, nil
		}
	}

	// Fallback to Documents directory on mobile / headless
	docs := getDocumentsDir()
	if docs != "" {
		target := filepath.Join(docs, filepath.Base(defaultFilename))
		if err := os.WriteFile(target, data, 0644); err != nil {
			return "", err
		}
		return target, nil
	}

	return "", errors.New("cannot determine save location")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
