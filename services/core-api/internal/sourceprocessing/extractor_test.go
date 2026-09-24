package sourceprocessing

import (
	"context"
	"strings"
	"testing"
)

func TestTextExtractorPreservesPagesAndSegments(t *testing.T) {
	extractor := &TextExtractor{MaxPageRunes: 20, MaxBytes: 1024}
	pages, err := extractor.Extract(context.Background(), ExtractInput{
		Reader:      strings.NewReader("السطر الأول\f\fالسطر الثاني"),
		ContentType: "text/plain; charset=utf-8",
		Filename:    "source.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(pages))
	}
	if pages[0].Number != 1 || pages[1].Number != 3 {
		t.Fatalf("unexpected page numbers: %+v", pages)
	}
	if pages[0].Text != "السطر الأول" || pages[1].Text != "السطر الثاني" {
		t.Fatalf("unexpected page text: %+v", pages)
	}
}

func TestTextExtractorSegmentsLongPages(t *testing.T) {
	extractor := &TextExtractor{MaxPageRunes: 4, MaxBytes: 1024}
	pages, err := extractor.Extract(context.Background(), ExtractInput{Reader: strings.NewReader("123456789"), ContentType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("segments = %d, want 3", len(pages))
	}
}

func TestTextExtractorRejectsUnsupportedContent(t *testing.T) {
	_, err := NewTextExtractor().Extract(context.Background(), ExtractInput{
		Reader:      strings.NewReader("binary"),
		ContentType: "application/pdf",
		Filename:    "source.pdf",
	})
	if err != ErrUnsupportedDocument {
		t.Fatalf("error = %v, want ErrUnsupportedDocument", err)
	}
}

func TestValidateUploadInput(t *testing.T) {
	input, err := validateUploadInput(UploadInput{Filename: "  مصدر.txt ", ContentType: "text/plain; charset=utf-8", Content: []byte("نص")})
	if err != nil {
		t.Fatal(err)
	}
	if input.Filename != "مصدر.txt" || input.ContentType != "text/plain" {
		t.Fatalf("unexpected input: %+v", input)
	}
	if _, err := validateUploadInput(UploadInput{Filename: "source.txt", ContentType: "text/plain"}); err != ErrValidation {
		t.Fatalf("empty content error = %v", err)
	}
}

func TestParseJobPayload(t *testing.T) {
	payload, err := parseJobPayload([]byte(`{"source_id":"00000000-0000-0000-0000-000000000001","source_file_id":"00000000-0000-0000-0000-000000000002"}`))
	if err != nil {
		t.Fatal(err)
	}
	if payload.SourceID == "" || payload.SourceFileID == "" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if _, err := parseJobPayload([]byte(`{"source_id":"bad"}`)); err != ErrValidation {
		t.Fatalf("invalid payload error = %v", err)
	}
}

func TestCandidateStatusDefaultsToReview(t *testing.T) {
	if candidateStatus("accepted") != "needs_review" || candidateStatus("unreviewed") != "unreviewed" {
		t.Fatal("unexpected candidate status normalization")
	}
}
