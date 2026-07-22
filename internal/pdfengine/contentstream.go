// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package pdfengine

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// BuildTextFreeBackground removes page-level PDF text objects while retaining
// images, vector graphics, annotations and page geometry. pdfcpu is used only
// for standards-compliant object IO; text-object recognition is implemented
// here from the PDF content-stream grammar.
func BuildTextFreeBackground(source []byte) ([]byte, int, error) {
	if len(source) < 5 || string(source[:5]) != "%PDF-" {
		return nil, 0, errors.New("invalid PDF source")
	}
	api.DisableConfigDir()
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.OPTIMIZE
	// gofpdi, used by the compositor, is most reliable with classic xref tables.
	conf.WriteObjectStream = false
	conf.WriteXRefStream = false
	ctx, err := api.ReadAndValidate(bytes.NewReader(source), conf)
	if err != nil {
		return nil, 0, fmt.Errorf("parse PDF structure: %w", err)
	}

	removedTotal := 0
	for pageNumber := 1; pageNumber <= ctx.PageCount; pageNumber++ {
		pageDict, _, _, err := ctx.PageDict(pageNumber, false)
		if err != nil {
			return nil, removedTotal, fmt.Errorf("read page %d: %w", pageNumber, err)
		}
		content, err := ctx.PageContent(pageDict, pageNumber)
		if err != nil {
			// A page without a content stream needs no cleanup.
			continue
		}
		cleaned, removed, err := StripTextObjects(content)
		if err != nil {
			return nil, removedTotal, fmt.Errorf("clean page %d text: %w", pageNumber, err)
		}
		removedTotal += removed
		stream, err := ctx.NewStreamDictForBuf(cleaned)
		if err != nil {
			return nil, removedTotal, fmt.Errorf("create page %d content: %w", pageNumber, err)
		}
		if err := stream.Encode(); err != nil {
			return nil, removedTotal, fmt.Errorf("encode page %d content: %w", pageNumber, err)
		}
		ref, err := ctx.IndRefForNewObject(*stream)
		if err != nil {
			return nil, removedTotal, fmt.Errorf("store page %d content: %w", pageNumber, err)
		}
		pageDict.Update("Contents", *ref)
	}

	var output bytes.Buffer
	if err := api.WriteContext(ctx, &output); err != nil {
		return nil, removedTotal, fmt.Errorf("write text-free PDF: %w", err)
	}
	return output.Bytes(), removedTotal, nil
}

// StripTextObjects removes complete BT ... ET text objects from a decoded PDF
// page content stream. Literal strings, hex strings, comments and inline image
// payloads are skipped while scanning so embedded BT/ET bytes are not mistaken
// for operators.
func StripTextObjects(content []byte) ([]byte, int, error) {
	result := make([]byte, 0, len(content))
	removed := 0
	for i := 0; i < len(content); {
		if content[i] == '%' {
			end := scanComment(content, i)
			result = append(result, content[i:end]...)
			i = end
			continue
		}
		if content[i] == '(' {
			end, err := scanLiteralString(content, i)
			if err != nil {
				return nil, removed, err
			}
			result = append(result, content[i:end]...)
			i = end
			continue
		}
		if matchPDFToken(content, i, "BI") {
			end := scanInlineImage(content, i)
			result = append(result, content[i:end]...)
			i = end
			continue
		}
		if matchPDFToken(content, i, "BT") {
			end, err := findTextObjectEnd(content, i+2)
			if err != nil {
				return nil, removed, err
			}
			result = append(result, '\n')
			removed++
			i = end
			continue
		}
		result = append(result, content[i])
		i++
	}
	return result, removed, nil
}

func findTextObjectEnd(content []byte, start int) (int, error) {
	for i := start; i < len(content); {
		switch content[i] {
		case '%':
			i = scanComment(content, i)
		case '(':
			end, err := scanLiteralString(content, i)
			if err != nil {
				return 0, err
			}
			i = end
		case '<':
			i = scanHexString(content, i)
		default:
			if matchPDFToken(content, i, "ET") {
				return i + 2, nil
			}
			i++
		}
	}
	return 0, errors.New("unterminated PDF text object")
}

func scanLiteralString(content []byte, start int) (int, error) {
	depth := 0
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
		}
	}
	return 0, errors.New("unterminated PDF literal string")
}

func scanHexString(content []byte, start int) int {
	if start+1 < len(content) && content[start+1] == '<' {
		return start + 1
	}
	for i := start + 1; i < len(content); i++ {
		if content[i] == '>' {
			return i + 1
		}
	}
	return len(content)
}

func scanComment(content []byte, start int) int {
	for i := start; i < len(content); i++ {
		if content[i] == '\n' || content[i] == '\r' {
			return i + 1
		}
	}
	return len(content)
}

func scanInlineImage(content []byte, start int) int {
	// Inline image data begins after the ID operator and ends at a whitespace
	// delimited EI operator. This conservative scan protects binary payloads.
	for i := start + 2; i < len(content); i++ {
		if !matchPDFToken(content, i, "ID") {
			continue
		}
		for j := i + 2; j < len(content)-1; j++ {
			if matchPDFToken(content, j, "EI") {
				return j + 2
			}
		}
		return len(content)
	}
	return start + 2
}

func matchPDFToken(content []byte, start int, token string) bool {
	if start < 0 || start+len(token) > len(content) || string(content[start:start+len(token)]) != token {
		return false
	}
	if start > 0 && !isPDFDelimiter(content[start-1]) {
		return false
	}
	end := start + len(token)
	return end == len(content) || isPDFDelimiter(content[end])
}

func isPDFDelimiter(value byte) bool {
	switch value {
	case 0, '\t', '\n', '\f', '\r', ' ', '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}
