package sourceprocessing

import (
	"context"
	"io"
	"strings"
	"unicode/utf8"
)

type TextExtractor struct {
	MaxPageRunes int
	MaxBytes     int64
}

func NewTextExtractor() *TextExtractor {
	return &TextExtractor{MaxPageRunes: 3500, MaxBytes: 50 << 20}
}

func (e *TextExtractor) Extract(ctx context.Context, input ExtractInput) ([]Page, error) {
	if e == nil || input.Reader == nil || !isTextContentType(input.ContentType) {
		return nil, ErrUnsupportedDocument
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	maxBytes := e.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 50 << 20
	}
	data, err := io.ReadAll(io.LimitReader(input.Reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes || !utf8.Valid(data) {
		return nil, ErrUnsupportedDocument
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	pageTexts := strings.Split(text, "\f")
	maxRunes := e.MaxPageRunes
	if maxRunes <= 0 {
		maxRunes = 3500
	}
	pages := make([]Page, 0, len(pageTexts))
	for pageIndex, pageText := range pageTexts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pageNumber := pageIndex + 1
		runes := []rune(pageText)
		for start := 0; start < len(runes); start += maxRunes {
			end := start + maxRunes
			if end > len(runes) {
				end = len(runes)
			}
			segment := strings.TrimSpace(string(runes[start:end]))
			if segment == "" {
				continue
			}
			pages = append(pages, Page{Number: pageNumber, Text: segment, StartOffset: start, EndOffset: end})
		}
	}
	if len(pages) == 0 || len(pages) > MaxPages {
		return nil, ErrValidation
	}
	return pages, nil
}

func isTextContentType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	return strings.HasPrefix(value, "text/") || value == "application/json" || value == "application/xml"
}

func pageCount(pages []Page) int {
	if len(pages) == 0 {
		return 0
	}
	return pages[len(pages)-1].Number
}

var _ Extractor = (*TextExtractor)(nil)
