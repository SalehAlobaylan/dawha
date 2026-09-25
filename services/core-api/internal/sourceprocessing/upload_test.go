package sourceprocessing

import (
	"errors"
	"strings"
	"testing"
)

// TestSupportedContentTypesMatrix is the V1 format decision in one place: every
// media type the product advertises is accepted, and every format outside the
// text extractor is refused. The same matrix decides the API error copy, the
// worker guard, and the UI copy, so a new row here is a product decision.
func TestSupportedContentTypesMatrix(t *testing.T) {
	cases := []struct {
		contentType string
		supported   bool
	}{
		{"text/plain", true},
		{"text/plain; charset=utf-8", true},
		{"TEXT/PLAIN", true},
		{"text/markdown", true},
		{"text/csv", true},
		{"text/html", true},
		{"text/tab-separated-values", true},
		{"text/xml", true},
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"application/xml", true},
		{"application/pdf", false},
		{"image/jpeg", false},
		{"image/png", false},
		{"image/tiff", false},
		{"application/octet-stream", false},
		{"application/msword", false},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", false},
		{"application/zip", false},
		{"audio/mpeg", false},
		{"video/mp4", false},
		{"application/x-sql-dump", false},
		{"", false},
		{"   ", false},
	}
	for _, testCase := range cases {
		if got := IsSupportedContentType(testCase.contentType); got != testCase.supported {
			t.Fatalf("IsSupportedContentType(%q) = %t, want %t", testCase.contentType, got, testCase.supported)
		}
	}
}

// TestSupportedContentTypesRefusedByValidator drives the same matrix through
// the upload boundary, which is the decision a caller actually meets.
func TestSupportedContentTypesRefusedByValidator(t *testing.T) {
	cases := []struct {
		contentType string
		content     string
		supported   bool
	}{
		{"text/plain", "نص عربي", true},
		{"text/markdown; charset=utf-8", "# عنوان", true},
		{"text/csv", "a,b\n1,2", true},
		{"application/json", `{"key":"قيمة"}`, true},
		{"application/xml", "<doc>نص</doc>", true},
		{"text/xml", "<doc>نص</doc>", true},
		{"application/pdf", "%PDF-1.7\nbinary", false},
		{"image/png", "\x89PNG\r\n\x1a\n", false},
		{"image/jpeg", "\xFF\xD8\xFF\xE0", false},
		{"image/tiff", "II*\x00binary", false},
		{"application/octet-stream", string([]byte{0x00, 0x01, 0x02, 0xff}), false},
		{"application/zip", "PK\x03\x04\x00\x00", false},
	}
	for _, testCase := range cases {
		_, err := validateUploadInput(UploadInput{Filename: "source.bin", ContentType: testCase.contentType, Content: []byte(testCase.content)})
		if testCase.supported && err != nil {
			t.Fatalf("validateUploadInput(%q) = %v, want accepted", testCase.contentType, err)
		}
		if !testCase.supported {
			if !errors.Is(err, ErrUnsupportedContent) {
				t.Fatalf("validateUploadInput(%q) = %v, want ErrUnsupportedContent", testCase.contentType, err)
			}
		}
	}
}

func TestUnsupportedContentErrorNamesTheSupportedFormats(t *testing.T) {
	_, err := validateUploadInput(UploadInput{Filename: "scan.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.7")})
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("error = %v, want ErrUnsupportedContent", err)
	}
	if !errors.Is(err, ErrUnsupportedDocument) {
		t.Fatalf("error = %v, want it to satisfy ErrUnsupportedDocument", err)
	}
	var unsupported *UnsupportedContentError
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %v, want an *UnsupportedContentError", err)
	}
	for _, fragment := range []string{"application/pdf", "text/*", "application/json", "application/xml"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("message %q does not mention %q", err.Error(), fragment)
		}
	}
	if !strings.Contains(UnsupportedFormatSummary(), "text/*") {
		t.Fatalf("summary %q does not name the supported formats", UnsupportedFormatSummary())
	}
}

func TestValidateUploadInputSniffsContentAgainstTheDeclaredType(t *testing.T) {
	// A text/plain claim over PDF bytes is refused, not stored on trust.
	_, err := validateUploadInput(UploadInput{Filename: "report.txt", ContentType: "text/plain", Content: []byte("%PDF-1.7\n\x00\x01binary")})
	if !errors.Is(err, ErrUnsupportedContent) || !strings.Contains(err.Error(), "application/pdf") {
		t.Fatalf("pdf claimed as text = %v, want a mismatch naming application/pdf", err)
	}
	// The same refusal holds when the client declares nothing at all.
	_, err = validateUploadInput(UploadInput{Filename: "report.txt", Content: []byte("%PDF-1.7\n\x00\x01binary")})
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("undeclared pdf = %v, want ErrUnsupportedContent", err)
	}
	// An undeclared text file is read from its content instead of being refused.
	input, err := validateUploadInput(UploadInput{Filename: "notes.txt", ContentType: "application/octet-stream", Content: []byte("نص عربي")})
	if err != nil {
		t.Fatalf("undeclared text = %v, want accepted", err)
	}
	if input.ContentType != "text/plain" {
		t.Fatalf("content type = %q, want the sniffed text/plain", input.ContentType)
	}
}

func TestValidateUploadInputKeepsFilenameAndSizeRules(t *testing.T) {
	cases := []struct {
		name  string
		input UploadInput
	}{
		{"empty filename", UploadInput{ContentType: "text/plain", Content: []byte("نص")}},
		{"whitespace filename", UploadInput{Filename: "   ", ContentType: "text/plain", Content: []byte("نص")}},
		{"invalid utf-8 filename", UploadInput{Filename: "\xff\xfe.txt", ContentType: "text/plain", Content: []byte("نص")}},
		{"long filename", UploadInput{Filename: strings.Repeat("س", 256), ContentType: "text/plain", Content: []byte("نص")}},
		{"empty content", UploadInput{Filename: "source.txt", ContentType: "text/plain"}},
		{"oversized content", UploadInput{Filename: "source.txt", ContentType: "text/plain", Content: make([]byte, MaxUploadBytes+1)}},
	}
	for _, testCase := range cases {
		if _, err := validateUploadInput(testCase.input); !errors.Is(err, ErrValidation) {
			t.Fatalf("%s = %v, want ErrValidation", testCase.name, err)
		}
	}
	// Invalid UTF-8 contradicts the declared text format, so it is answered with
	// the format contract rather than a bare validation error.
	if _, err := validateUploadInput(UploadInput{Filename: "source.txt", ContentType: "text/plain", Content: []byte("\xff\xfe\xfd")}); !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("invalid utf-8 content = %v, want ErrUnsupportedContent", err)
	}
}

func TestValidateUploadInputNormalizesAcceptedInput(t *testing.T) {
	input, err := validateUploadInput(UploadInput{Filename: "  مصدر.txt ", ContentType: "text/plain; charset=utf-8", Content: []byte("نص")})
	if err != nil {
		t.Fatal(err)
	}
	if input.Filename != "مصدر.txt" || input.ContentType != "text/plain" {
		t.Fatalf("unexpected input: %+v", input)
	}
}

func TestSafeFilenameKeepsStorageKeysInsideTheSource(t *testing.T) {
	cases := map[string]string{
		"مصدر.txt":         "مصدر.txt",
		"../../etc/passwd": "passwd",
		"a/b\\c.txt":       "b_c.txt",
		"..":               "source",
		"  ":               "source",
	}
	for input, expected := range cases {
		if got := safeFilename(input); got != expected {
			t.Fatalf("safeFilename(%q) = %q, want %q", input, got, expected)
		}
	}
}
