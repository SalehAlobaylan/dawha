package sourceprocessing

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
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

// TestSourceFormatPanelMatchesTheSharedMatrix is the tie between the Go matrix
// and the copy a user reads before choosing a file. The panel is TypeScript and
// cannot import the Go list, so this test is what keeps the two from drifting:
// every media type the panel advertises has to be inside the matrix, every
// media type in the matrix has to reach the panel, and the file picker may not
// offer an extension the extractor cannot read.
func TestSourceFormatPanelMatchesTheSharedMatrix(t *testing.T) {
	panel := readSourceFormatPanel(t)
	for _, supported := range SupportedContentTypes() {
		if !strings.Contains(panel, supported) {
			t.Fatalf("the source processing panel never names the supported format %q", supported)
		}
	}

	advertised := panelMediaTypes(t, panel)
	if len(advertised) == 0 {
		t.Fatal("the source processing panel has no media type list to check")
	}
	for _, mediaType := range advertised {
		if !IsSupportedContentType(mediaType) {
			t.Fatalf("the source processing panel advertises %q, which the format contract refuses", mediaType)
		}
	}
	for _, supported := range SupportedContentTypes() {
		if !slices.Contains(advertised, supported) {
			t.Fatalf("the source processing panel is missing the supported format %q", supported)
		}
	}

	accept := panelAcceptAttribute(t, panel)
	// The rendered picker has to read the checked list, otherwise a hand-typed
	// accept attribute is a second copy the matrix never sees.
	if !regexp.MustCompile(`accept=\{SUPPORTED_UPLOAD_ACCEPT\}`).MatchString(panel) {
		t.Fatal("the source file input must take its accept list from SUPPORTED_UPLOAD_ACCEPT")
	}
	refusedExtensions := []string{".pdf", ".png", ".jpg", ".jpeg", ".tif", ".tiff", ".doc", ".docx", ".xls", ".xlsx", ".zip", ".gz", ".bin", ".exe", ".mp3", ".mp4"}
	for _, entry := range strings.Split(accept, ",") {
		entry = strings.TrimSpace(entry)
		if !strings.HasPrefix(entry, ".") {
			if !IsSupportedContentType(entry) {
				t.Fatalf("the accept attribute advertises the media type %q, which the format contract refuses", entry)
			}
			continue
		}
		for _, refused := range refusedExtensions {
			if strings.EqualFold(entry, refused) {
				t.Fatalf("the accept attribute offers %q, a format the extractor cannot read", entry)
			}
		}
	}
}

// panelMediaTypes reads the media type list the panel mirrors, so a row added to
// the panel without a row in the Go matrix is a test failure rather than a
// promise the API does not keep.
func panelMediaTypes(t *testing.T, panel string) []string {
	t.Helper()
	block := regexp.MustCompile(`SUPPORTED_UPLOAD_MEDIA_TYPES\s*=\s*\[([^\]]*)\]`).FindStringSubmatch(panel)
	if len(block) != 2 {
		t.Fatal("the source processing panel no longer declares SUPPORTED_UPLOAD_MEDIA_TYPES")
	}
	quoted := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(block[1], -1)
	mediaTypes := make([]string, 0, len(quoted))
	for _, entry := range quoted {
		mediaTypes = append(mediaTypes, entry[1])
	}
	return mediaTypes
}

func panelAcceptAttribute(t *testing.T, panel string) string {
	t.Helper()
	accept := regexp.MustCompile(`SUPPORTED_UPLOAD_ACCEPT\s*=\s*"([^"]*)"`).FindStringSubmatch(panel)
	if len(accept) != 2 {
		t.Fatal("the source processing panel no longer declares SUPPORTED_UPLOAD_ACCEPT")
	}
	return accept[1]
}

// readSourceFormatPanel walks up from this file to the module root, then two
// levels up to the repository root. It needs neither the database nor the
// network, so the contract stays checked on a bare checkout.
func readSourceFormatPanel(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	directory := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			path := filepath.Join(directory, "..", "..", "apps", "web", "src", "components", "SourceProcessingPanel.tsx")
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("the source processing panel is not readable at %s: %v", path, err)
			}
			return string(content)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("cannot locate the module root")
		}
		directory = parent
	}
}
