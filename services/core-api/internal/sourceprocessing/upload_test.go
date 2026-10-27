package sourceprocessing

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestSupportedContentTypesMatrix keeps the shared upload contract aligned:
// every advertised format is accepted, and unsupported formats are refused.
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
		{"application/pdf", true},
		{"image/jpeg", true},
		{"image/jpg", true},
		{"image/png", true},
		{"image/gif", true},
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
	pngContent := string(testImageBytes(t, "png"))
	jpegContent := string(testImageBytes(t, "jpeg"))
	gifContent := string(testImageBytes(t, "gif"))
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
		{"application/pdf", "%PDF-1.7\nbinary", true},
		{"image/png", pngContent, true},
		{"image/jpeg", jpegContent, true},
		{"image/jpg", jpegContent, true},
		{"image/gif", gifContent, true},
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
	_, err := validateUploadInput(UploadInput{Filename: "archive.zip", ContentType: "application/zip", Content: []byte("PK\x03\x04")})
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
	for _, fragment := range []string{"application/zip", "text/*", "application/json", "application/xml", "application/pdf", "image/png"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("message %q does not mention %q", err.Error(), fragment)
		}
	}
	if !strings.Contains(UnsupportedFormatSummary(), "text/*") {
		t.Fatalf("summary %q does not name the supported formats", UnsupportedFormatSummary())
	}
}

func TestValidateUploadInputSniffsContentAgainstTheDeclaredType(t *testing.T) {
	// A text/plain claim over PDF bytes is refused instead of being stored under
	// a misleading type. The accepted format list names the detected PDF type.
	_, err := validateUploadInput(UploadInput{Filename: "report.txt", ContentType: "text/plain", Content: []byte("%PDF-1.7\n\x00\x01binary")})
	if !errors.Is(err, ErrUnsupportedContent) || !strings.Contains(err.Error(), "application/pdf") {
		t.Fatalf("pdf claimed as text = %v, want a type mismatch naming application/pdf", err)
	}
	// The same sniffing applies when the client declares nothing at all.
	input, err := validateUploadInput(UploadInput{Filename: "report.txt", Content: []byte("%PDF-1.7\n\x00\x01binary")})
	if err != nil || input.ContentType != "application/pdf" {
		t.Fatalf("undeclared pdf = %+v, %v, want accepted application/pdf", input, err)
	}
	// An undeclared text file is read from its content instead of being refused.
	input, err = validateUploadInput(UploadInput{Filename: "notes.txt", ContentType: "application/octet-stream", Content: []byte("نص عربي")})
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

// TestValidateUploadInputAcceptsARealFamilyChart pins the size bound to an
// input that exists in the world: the scanned chart of آل مقبل is
// 61,079,149 bytes, and it was refused when the bound was 25 MiB. A bound that
// refuses the real thing is not a bound on abuse, it is a refusal of the
// product's own material. The 25 MiB bound was raised on 2026-10-22 for
// exactly this input.
func TestValidateUploadInputAcceptsARealFamilyChart(t *testing.T) {
	if testing.Short() {
		t.Skip("the real chart is 61 MB; skipped in -short runs")
	}
	const realChartBytes = 61_079_149
	content := make([]byte, realChartBytes)
	copy(content, "%PDF-1.7")
	input, err := validateUploadInput(UploadInput{Filename: "شجرة أسر آل مقبل.pdf", ContentType: "application/pdf", Content: content})
	if err != nil {
		t.Fatalf("a real 61 MB family chart = %v, want accepted", err)
	}
	if input.ContentType != "application/pdf" {
		t.Fatalf("content type = %q, want application/pdf", input.ContentType)
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
// offer an extension the configured extraction paths cannot read.
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
	refusedExtensions := []string{".tif", ".tiff", ".bmp", ".heic", ".webp", ".doc", ".docx", ".xls", ".xlsx", ".zip", ".gz", ".bin", ".exe", ".mp3", ".mp4"}
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

// testImageBytes encodes a small real image in one of the accepted raster
// formats. The upload boundary decodes the header, so a fixture has to be a
// real file rather than a signature prefix.
func testImageBytes(t *testing.T, format string) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 16; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(20 + x*4), G: uint8(60 + y*5), B: 40, A: 255})
		}
	}
	var buffer bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buffer, canvas)
	case "jpeg":
		err = jpeg.Encode(&buffer, canvas, nil)
	case "gif":
		err = gif.Encode(&buffer, canvas, nil)
	default:
		t.Fatalf("no encoder for %q", format)
	}
	if err != nil {
		t.Fatalf("could not encode a %s fixture: %v", format, err)
	}
	return buffer.Bytes()
}

// pngHeaderWithSize patches a PNG's IHDR dimensions and CRC, so the upload
// boundary can be tested against an image that announces more pixels than the
// worker will decode.
func pngHeaderWithSize(t *testing.T, width, height uint32) []byte {
	t.Helper()
	data := testImageBytes(t, "png")
	if len(data) < 33 || string(data[12:16]) != "IHDR" {
		t.Fatal("the PNG fixture does not start with an IHDR chunk")
	}
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[20:24], height)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

// TestValidateUploadInputAcceptsRasterCharts pins the raster arm of the source
// contract: JPEG, PNG and GIF are documents, and the bytes decide whether a
// declaration is honest. The GIF matters because a published tree is often one
// GIF embedded in a web page.
func TestValidateUploadInputAcceptsRasterCharts(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		data := testImageBytes(t, format)
		input, err := validateUploadInput(UploadInput{Filename: "شجرة." + format, ContentType: "image/" + format, Content: data})
		if err != nil {
			t.Fatalf("a real %s chart = %v, want accepted", format, err)
		}
		if input.ContentType != "image/"+format {
			t.Fatalf("content type = %q, want image/%s", input.ContentType, format)
		}
	}
	// A scanner or browser that calls a JPEG image/jpg is naming the same
	// format, and the alias is normalised rather than refused.
	jpegData := testImageBytes(t, "jpeg")
	input, err := validateUploadInput(UploadInput{Filename: "شجرة.jpg", ContentType: "image/jpg", Content: jpegData})
	if err != nil || input.ContentType != "image/jpeg" {
		t.Fatalf("image/jpg alias = %+v, %v, want accepted as image/jpeg", input, err)
	}
	// Undeclared images are detected from their bytes, like undeclared PDFs.
	input, err = validateUploadInput(UploadInput{Filename: "شجرة", ContentType: undeclaredContentType, Content: jpegData})
	if err != nil || input.ContentType != "image/jpeg" {
		t.Fatalf("undeclared jpeg = %+v, %v, want sniffed image/jpeg", input, err)
	}
	// A PNG declaration over JPEG bytes is refused rather than stored under a
	// misleading type.
	if _, err := validateUploadInput(UploadInput{Filename: "شجرة.png", ContentType: "image/png", Content: jpegData}); !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("png declared over jpeg bytes = %v, want a contradiction", err)
	}
	// Truncated bytes with a raster signature are not a readable image.
	if _, err := validateUploadInput(UploadInput{Filename: "شجرة.png", ContentType: "image/png", Content: []byte("\x89PNG\r\n\x1a\n")}); !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("truncated png = %v, want ErrUnsupportedContent", err)
	}
	// The pixel bound is a worker-memory bound, so it is enforced at upload.
	_, err = validateUploadInput(UploadInput{Filename: "panorama.png", ContentType: "image/png", Content: pngHeaderWithSize(t, 9000, 9000)})
	if !errors.Is(err, ErrUnsupportedContent) || !strings.Contains(err.Error(), "megapixels") {
		t.Fatalf("a 81-megapixel image = %v, want a refusal naming the pixel bound", err)
	}
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
