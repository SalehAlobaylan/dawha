package sourceprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ReadingSettings struct {
	Mode          string `json:"mode"`
	HostedEnabled bool   `json:"hostedEnabled"`
}

func ReadingSettingsFromEnvironment(getenv func(string) string) ReadingSettings {
	mode := strings.ToLower(strings.TrimSpace(getenv("NAME_READER_MODE")))
	if mode == "" {
		mode = "local"
	}
	if mode != "local" && mode != "hosted" && mode != "off" {
		mode = "off"
	}
	enabled := strings.ToLower(strings.TrimSpace(getenv("CHART_VISION_ENABLED")))
	return ReadingSettings{Mode: mode, HostedEnabled: mode == "hosted" && (enabled == "true" || enabled == "1" || enabled == "yes" || enabled == "on")}
}

type LocalReaderStatus struct {
	ArabicDataVersion string              `json:"arabicDataVersion,omitempty"`
	Configured        bool                `json:"configured"`
	Ready             bool                `json:"ready"`
	Reason            string              `json:"reason"`
	Version           string              `json:"version"`
	Languages         []string            `json:"languages"`
	Tools             []ReadingToolStatus `json:"tools"`
}

type ReadingToolStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
}

type HostedReaderStatus struct {
	Enabled          bool   `json:"enabled"`
	Configured       bool   `json:"configured"`
	AllowedForSource bool   `json:"allowedForSource"`
	Ready            bool   `json:"ready"`
	Reason           string `json:"reason"`
	Version          string `json:"version"`
}

type ReadingCapabilities struct {
	Mode         string             `json:"mode"`
	LocalReader  LocalReaderStatus  `json:"localReader"`
	HostedReader HostedReaderStatus `json:"hostedReader"`
}

// ReadingStatusProvider probes configuration through a bounded metadata GET.
// It never receives a file identifier, image, name, or user credential.
type ReadingStatusProvider interface {
	ReadingStatus(context.Context) (HostedReaderStatus, error)
}

type readingStatusClient struct {
	baseURL string
	client  *http.Client
}

func NewReadingStatusProvider(baseURL string) ReadingStatusProvider {
	return &readingStatusClient{baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *readingStatusClient) ReadingStatus(ctx context.Context) (HostedReaderStatus, error) {
	result := HostedReaderStatus{Reason: "temporarily_unavailable", Version: ChartVisionModel}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/vision/chart/capabilities", nil)
	if err != nil {
		return result, ErrChartVisionUnavailable
	}
	response, err := c.client.Do(request)
	if err != nil {
		return result, ErrChartVisionUnavailable
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(body) > 8192 || response.StatusCode != http.StatusOK {
		return result, ErrChartVisionUnavailable
	}
	var status struct {
		Enabled    bool   `json:"enabled"`
		Configured bool   `json:"configured"`
		Reason     string `json:"reason"`
		Version    string `json:"version"`
	}
	if json.Unmarshal(body, &status) != nil || status.Version != ChartVisionModel {
		return result, ErrChartVisionUnavailable
	}
	result.Enabled, result.Configured = status.Enabled, status.Enabled && status.Configured
	if !result.Enabled {
		result.Reason = "disabled_by_policy"
	} else if !result.Configured {
		result.Reason = "not_configured"
	} else {
		result.Reason = "configured"
	}
	return result, nil
}

type readingCapabilityCache struct {
	mu     sync.Mutex
	at     time.Time
	local  LocalReaderStatus
	hosted HostedReaderStatus
}

// ProbeLocalReadingTools reports installed tools, not OCR quality. The local
// crop adapter is deliberately still unavailable until Plan 024 implements it.
func ProbeLocalReadingTools(ctx context.Context) LocalReaderStatus {
	status := LocalReaderStatus{Reason: "configured", Version: LocalCropReaderVersion, Languages: []string{}, Tools: []ReadingToolStatus{}}
	extractor := NewPDFExtractor()
	for _, tool := range []struct{ name, binary, flag string }{{"pdfinfo", extractor.PDFInfo, "-v"}, {"pdftotext", extractor.PDFToText, "-v"}, {"pdftoppm", extractor.PDFToPPM, "-v"}, {"tesseract", extractor.Tesseract, "--version"}} {
		entry := ReadingToolStatus{Name: tool.name}
		probeCtx, cancel := context.WithTimeout(ctx, time.Second)
		output, err := probeTool(probeCtx, tool.binary, tool.flag)
		cancel()
		// Poppler prints its version to stderr. Resolve it without returning
		// arbitrary command output or configured filesystem paths.
		if _, pathErr := exec.LookPath(tool.binary); pathErr == nil {
			entry.Available = err == nil
			if tool.name != "tesseract" && err == nil {
				entry.Available = true
			}
			fields := strings.Fields(string(output))
			if len(fields) >= 2 && fields[0] == tool.name {
				token := fields[1]
				if token == "version" && len(fields) > 2 {
					token = fields[2]
				}
				if validToolVersion(token) {
					entry.Version = token
				}
			}
		}
		if !entry.Available {
			status.Reason = "tool_missing"
		}
		status.Tools = append(status.Tools, entry)
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	output, err := runBounded(probeCtx, time.Second, extractor.Tesseract, "--list-langs")
	cancel()
	if err == nil {
		status.ArabicDataVersion = installedArabicDataVersion(string(output))
		for _, line := range strings.Split(string(output), "\n") {
			language := strings.TrimSpace(line)
			if language == "ara" || language == "eng" {
				status.Languages = append(status.Languages, language)
			}
		}
	}
	if status.Reason != "tool_missing" && !strings.Contains(" "+strings.Join(status.Languages, " ")+" ", " ara ") {
		status.Reason = "language_missing"
	}
	return status
}

func validToolVersion(value string) bool {
	if value == "" || len(value) > 64 || value[0] < '0' || value[0] > '9' {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !strings.ContainsRune(".+_-", r) {
			return false
		}
	}
	return true
}

func (s *Service) readingSettings() ReadingSettings {
	settings := s.ReaderSettings
	if settings.Mode == "" {
		settings.Mode = "local"
	}
	if settings.Mode != "local" && settings.Mode != "hosted" && settings.Mode != "off" {
		settings.Mode = "off"
	}
	settings.HostedEnabled = settings.Mode == "hosted" && settings.HostedEnabled
	return settings
}

// ReadingConfiguration is aggregate-only. Readiness describes a working adapter,
// not the presence of Tesseract or an allocated HTTP client.
func (s *Service) ReadingConfiguration(ctx context.Context) ReadingCapabilities {
	settings := s.readingSettings()
	s.readingCache.mu.Lock()
	defer s.readingCache.mu.Unlock()
	if time.Since(s.readingCache.at) > 30*time.Second {
		probe := s.LocalReadingProbe
		if probe == nil {
			probe = ProbeLocalReadingTools
		}
		s.readingCache.local = probe(ctx)
		s.readingCache.hosted = HostedReaderStatus{Reason: "disabled_by_policy", Version: ChartVisionModel}
		if settings.HostedEnabled {
			s.readingCache.hosted = HostedReaderStatus{Enabled: true, Reason: "temporarily_unavailable", Version: ChartVisionModel}
			if s.ReaderStatus != nil {
				if status, err := s.ReaderStatus.ReadingStatus(ctx); err == nil {
					s.readingCache.hosted = status
				}
			}
		}
		s.readingCache.at = time.Now()
	}
	local := s.readingCache.local
	local.Configured = settings.Mode == "local"
	local.Ready = local.Configured && local.Reason == "configured"
	if local.Ready {
		local.Reason = "configured"
		local.Version = LocalCropReaderVersion
	}
	if !local.Configured {
		local.Reason = "disabled_by_policy"
	}
	hosted := s.readingCache.hosted
	if !settings.HostedEnabled {
		hosted = HostedReaderStatus{Reason: "disabled_by_policy", Version: ChartVisionModel}
	}
	return ReadingCapabilities{Mode: settings.Mode, LocalReader: local, HostedReader: hosted}
}

func (s *Service) SourceReadingCapabilities(ctx context.Context, fileID, actorID string) (ReadingCapabilities, error) {
	if s == nil || s.Pool == nil {
		return ReadingCapabilities{}, ErrDatabaseUnavailable
	}
	fileUUID, err := uuid.Parse(fileID)
	if err != nil {
		return ReadingCapabilities{}, ErrNotFound
	}
	allowed, err := s.canReview(ctx, fileUUID, actorID)
	if err != nil {
		return ReadingCapabilities{}, err
	}
	if !allowed {
		return ReadingCapabilities{}, ErrForbidden
	}
	var approved bool
	// Approval is a source-side policy fact, never a provider request field.
	if err := s.Pool.QueryRow(ctx, `SELECT COALESCE(s.metadata->'hostedChartVisionApproved' = 'true'::jsonb,false) FROM source_files f JOIN sources s ON s.id=f.source_id WHERE f.id=$1`, fileUUID).Scan(&approved); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReadingCapabilities{}, ErrNotFound
		}
		return ReadingCapabilities{}, ErrDatabaseUnavailable
	}
	result := s.ReadingConfiguration(ctx)
	result.HostedReader.AllowedForSource = approved && s.readingSettings().HostedEnabled
	result.HostedReader.Ready = result.HostedReader.Enabled && result.HostedReader.Configured && result.HostedReader.AllowedForSource
	if s.readingSettings().HostedEnabled && !approved {
		result.HostedReader.Reason = "disabled_by_policy"
	}
	return result, nil
}

func probeTool(ctx context.Context, binary string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = documentToolEnvironment()
	output := &boundedBuffer{limit: 8192}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	return output.Bytes(), err
}

// Check policy before opening an original or rendering a provider-bound crop.
func (s *Service) requireHostedSuggestionReader(ctx context.Context, suggestionID, actorID string) error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	id, err := uuid.Parse(suggestionID)
	if err != nil {
		return ErrNotFound
	}
	var fileID string
	err = s.Pool.QueryRow(ctx, `SELECT source_file_id::text FROM source_tree_suggestions WHERE id=$1`, id).Scan(&fileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return ErrDatabaseUnavailable
	}
	capabilities, err := s.SourceReadingCapabilities(ctx, fileID, actorID)
	if err != nil {
		return err
	}
	reader := capabilities.HostedReader
	if !reader.Enabled || !reader.AllowedForSource {
		return ErrChartVisionPolicy
	}
	if reader.Reason == "temporarily_unavailable" {
		return ErrChartVisionUnavailable
	}
	if !reader.Configured {
		return ErrChartVisionSetup
	}
	return nil
}

// ReadingPipelineVersions identifies current algorithms, not the versions of
// historical evidence. name-eval always reads the saved versions instead.
func ReadingPipelineVersions() map[string]string {
	return map[string]string{"renderer": NameRendererVersion, "reader": LocalCropReaderVersion, "localReader": LocalCropReaderVersion, "pool": NamePoolVersion, "normalizer": "ar-name-keys-v1"}
}

func installedArabicDataVersion(listing string) string {
	directory := os.Getenv("TESSDATA_PREFIX")
	if directory == "" {
		parts := strings.Split(listing, "\"")
		if len(parts) >= 3 {
			directory = parts[1]
		}
	}
	if directory == "" {
		return "unknown"
	}
	file, err := os.Open(filepath.Join(directory, "ara.traineddata"))
	if err != nil {
		return "unavailable"
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, (64<<20)+1))
	if err != nil || size > 64<<20 {
		return "unavailable"
	}
	return hex.EncodeToString(hash.Sum(nil))
}
