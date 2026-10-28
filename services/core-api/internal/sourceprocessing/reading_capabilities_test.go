package sourceprocessing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type stubReadingStatus struct {
	calls  int
	status HostedReaderStatus
	err    error
}

func (p *stubReadingStatus) ReadingStatus(context.Context) (HostedReaderStatus, error) {
	p.calls++
	return p.status, p.err
}
func TestReadingSettingsFailClosed(t *testing.T) {
	for _, test := range []struct {
		mode, flag, want string
		enabled          bool
	}{{"", "true", "local", false}, {"off", "true", "off", false}, {"local", "true", "local", false}, {"typo", "true", "off", false}, {"hosted", "false", "hosted", false}, {"hosted", "true", "hosted", true}} {
		settings := ReadingSettingsFromEnvironment(func(key string) string {
			if key == "NAME_READER_MODE" {
				return test.mode
			}
			return test.flag
		})
		if settings.Mode != test.want || settings.HostedEnabled != test.enabled {
			t.Fatalf("settings = %+v", settings)
		}
	}
}
func TestDisabledReadingNeverProbesHostedEvenWithCredentials(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "fixture-credential")
	for _, mode := range []string{"local", "off", "typo"} {
		provider := &stubReadingStatus{}
		service := Service{ReaderSettings: ReadingSettings{Mode: mode, HostedEnabled: true}, ReaderStatus: provider, LocalReadingProbe: func(context.Context) LocalReaderStatus {
			return LocalReaderStatus{Reason: "local_reader_not_implemented"}
		}}
		capabilities := service.ReadingConfiguration(context.Background())
		if provider.calls != 0 || capabilities.HostedReader.Enabled || capabilities.LocalReader.Ready {
			t.Fatalf("unsafe capabilities: %+v", capabilities)
		}
	}
}
func TestReadingHealthIsCachedAndDistinguishedFromConfiguration(t *testing.T) {
	provider := &stubReadingStatus{err: ErrChartVisionUnavailable}
	probes := 0
	service := Service{ReaderSettings: ReadingSettings{Mode: "hosted", HostedEnabled: true}, ReaderStatus: provider, LocalReadingProbe: func(context.Context) LocalReaderStatus { probes++; return LocalReaderStatus{} }}
	for i := 0; i < 2; i++ {
		status := service.ReadingConfiguration(context.Background())
		if status.HostedReader.Reason != "temporarily_unavailable" || status.HostedReader.Ready {
			t.Fatalf("status=%+v", status)
		}
	}
	if probes != 1 || provider.calls != 1 {
		t.Fatalf("cache probes=%d hosted=%d", probes, provider.calls)
	}
}
func TestReadingStatusUsesOnlyBoundedMetadataGET(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/v1/vision/chart/capabilities" || r.ContentLength > 0 || r.URL.RawQuery != "" {
			t.Errorf("unexpected metadata probe")
		}
		fmt.Fprintf(w, `{"enabled":true,"configured":true,"version":%q}`, ChartVisionModel)
	}))
	defer server.Close()
	status, err := NewReadingStatusProvider(server.URL).ReadingStatus(context.Background())
	if err != nil || !status.Configured || status.Ready || status.AllowedForSource || calls != 1 {
		t.Fatalf("status=%+v error=%v", status, err)
	}
}
func TestLocalReadingProbeReportsMissingArabicAndNeverReady(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"pdfinfo", "pdftotext", "pdftoppm", "tesseract"} {
		content := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then echo eng; else echo '%s 1.2.3' >&2; fi\n", name)
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
		key := map[string]string{"pdfinfo": "PDFINFO_BINARY", "pdftotext": "PDFTOTEXT_BINARY", "pdftoppm": "PDFTOPPM_BINARY", "tesseract": "TESSERACT_BINARY"}[name]
		t.Setenv(key, path)
	}
	status := ProbeLocalReadingTools(context.Background())
	if status.Ready || status.Reason != "language_missing" {
		t.Fatalf("status=%+v", status)
	}
	for _, tool := range status.Tools {
		if !tool.Available || tool.Version != "1.2.3" {
			t.Fatalf("tool=%+v", tool)
		}
	}
	t.Setenv("PDFINFO_BINARY", filepath.Join(directory, "missing"))
	if status = ProbeLocalReadingTools(context.Background()); status.Reason != "tool_missing" {
		t.Fatalf("status=%+v", status)
	}
}

type batchBehavior struct {
	calls int
	run   func(context.Context, []ChartCropInput) ([]ChartNameReading, error)
}

func (*batchBehavior) LocateTile(context.Context, string, []byte) (ChartTileReading, error) {
	panic("unexpected locate")
}
func (*batchBehavior) SuggestNames(context.Context, []byte) ([]string, error) {
	panic("unexpected candidates")
}
func (p *batchBehavior) ReadBatch(ctx context.Context, inputs []ChartCropInput) ([]ChartNameReading, error) {
	p.calls++
	return p.run(ctx, inputs)
}
func TestPermanentBatchFailuresNeverFanOut(t *testing.T) {
	for _, failure := range []error{ErrChartVisionSetup, ErrChartVisionPolicy, ErrChartVisionRejected, context.Canceled, context.DeadlineExceeded} {
		provider := &batchBehavior{run: func(context.Context, []ChartCropInput) ([]ChartNameReading, error) { return nil, failure }}
		_, err := readInBatches(context.Background(), provider, make([]ChartCropInput, 8))
		if !errors.Is(err, failure) || provider.calls != 1 {
			t.Fatalf("failure=%v error=%v calls=%d", failure, err, provider.calls)
		}
	}
}
func TestTransientBatchFailuresHaveABoundedBudget(t *testing.T) {
	provider := &batchBehavior{run: func(context.Context, []ChartCropInput) ([]ChartNameReading, error) {
		return nil, ErrChartVisionUnavailable
	}}
	_, err := readInBatches(context.Background(), provider, make([]ChartCropInput, 8))
	if !errors.Is(err, ErrChartVisionUnavailable) || provider.calls != 2 {
		t.Fatalf("error=%v calls=%d", err, provider.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = readInBatches(ctx, provider, make([]ChartCropInput, 8))
	if !errors.Is(err, context.Canceled) || provider.calls != 2 {
		t.Fatalf("canceled error=%v calls=%d", err, provider.calls)
	}
}
func TestValidationFallbackPreservesOtherCropsButNotAnOutage(t *testing.T) {
	inputs := []ChartCropInput{{OccurrenceID: "good"}, {OccurrenceID: "bad"}}
	provider := &batchBehavior{run: func(_ context.Context, items []ChartCropInput) ([]ChartNameReading, error) {
		if len(items) > 1 || items[0].OccurrenceID == "bad" {
			return nil, ErrChartVisionOutputInvalid
		}
		return []ChartNameReading{{OccurrenceID: "good", Status: "readable", TextAR: "محمد"}}, nil
	}}
	rows, err := readInBatches(context.Background(), provider, inputs)
	if err != nil || len(rows) != 2 || rows[0].Status != "readable" || rows[1].Status != "unreadable" || provider.calls != 3 {
		t.Fatalf("rows=%+v error=%v calls=%d", rows, err, provider.calls)
	}
	provider.calls = 0
	provider.run = func(_ context.Context, items []ChartCropInput) ([]ChartNameReading, error) {
		if len(items) > 1 {
			return nil, ErrChartVisionOutputInvalid
		}
		return nil, ErrChartVisionSetup
	}
	if _, err = readInBatches(context.Background(), provider, inputs); !errors.Is(err, ErrChartVisionSetup) || provider.calls != 2 {
		t.Fatalf("outage error=%v calls=%d", err, provider.calls)
	}
}
