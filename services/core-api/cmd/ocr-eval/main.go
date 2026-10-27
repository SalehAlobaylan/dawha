// Command ocr-eval runs the family-tree reading pipeline over the private corpus and
// writes a prediction file in the shape tools/ocr-eval/evaluate.py scores.
//
// It is the arm that the benchmark has never had: the localiser finds where the names
// are, the reader reads each one in isolation, and the result is scored against the
// same gold as every other arm.
//
// The corpus is private and lives outside the repository, so this command takes the
// manifest path rather than assuming one. See docs/ocr-benchmark-format.md.
//
//	ocr-eval --manifest ~/.dawha/corpus/manifest.json \
//	          --output-dir ~/.dawha/corpus/predictions \
//	          --reader-url http://localhost:8000
//
// Without a reader URL it still runs, and reports the carriers it found without
// reading them. That is useful on its own: the number of carriers is the localiser's
// recall, measured against nothing, and it is what the two-rendering consistency
// check compares.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
)

type manifestEntry struct {
	DocumentID  string `json:"document_id"`
	SourceClass string `json:"source_class"`
	Split       string `json:"split"`
	Rendering   string `json:"rendering_of"`
	Pages       []struct {
		PageID    string `json:"page_id"`
		ImagePath string `json:"image_path"`
	} `json:"pages"`
}

type predictionNode struct {
	ID     string `json:"id"`
	PageID string `json:"page_id"`
	NameAR string `json:"name_ar"`
	Status string `json:"status"`
	Region region `json:"region"`
}

type region struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type predictionPage struct {
	PageID    string `json:"page_id"`
	Processed bool   `json:"processed"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type predictionDocument struct {
	DocumentID        string           `json:"document_id"`
	SourceClass       string           `json:"source_class"`
	Split             string           `json:"split"`
	Rendering         string           `json:"rendering_of,omitempty"`
	Pages             []predictionPage `json:"pages"`
	Nodes             []predictionNode `json:"nodes"`
	Branches          []any            `json:"branches"`
	Connectors        []any            `json:"connectors"`
	Edges             []any            `json:"edges"`
	ProcessingSeconds float64          `json:"processing_seconds"`
	CostUSD           float64          `json:"cost_usd"`
	ProviderCalls     int              `json:"provider_call_count"`
	OCROperations     int              `json:"ocr_operation_count"`
	InputImageBytes   int64            `json:"input_image_bytes"`
}

// decodePNG decodes a rendered page image.
func decodePNG(data []byte) (image.Image, error) {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	return decoded, err
}

func main() {
	manifestPath := flag.String("manifest", "", "the private corpus manifest")
	outputDir := flag.String("output-dir", "", "where the prediction file is written")
	pipeline := flag.String("pipeline", "carrier_vision", "the arm name")
	readerURL := flag.String("reader-url", "", "the AI service base URL; without it the carriers are reported unread")
	comparePath := flag.String("compare", "", "compare the renderings in a predictions JSONL; no manifest or output directory is needed")
	limit := flag.Int("limit", 0, "read at most this many pages per document; zero means all of them")
	maxCarriers := flag.Int("max-carriers", 0, "read at most this many carriers per page; zero means all of them")
	sample := flag.Int("sample", 0, "read an evenly spaced sample of at most this many name groups per page; zero means all of them")
	flag.Parse()

	if *comparePath != "" {
		if err := compareRenderings(*comparePath); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-eval: %v\n", err)
			os.Exit(2)
		}
		return
	}
	if *manifestPath == "" || *outputDir == "" {
		fmt.Fprintln(os.Stderr, "ocr-eval: --manifest and --output-dir are required")
		flag.Usage()
		os.Exit(2)
	}
	raw, err := os.ReadFile(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ocr-eval: %v\n", err)
		os.Exit(2)
	}
	var manifest []manifestEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		fmt.Fprintf(os.Stderr, "ocr-eval: the manifest is not valid JSON: %v\n", err)
		os.Exit(2)
	}
	base := filepath.Dir(*manifestPath)

	var reader sourceprocessing.ChartVisionProvider
	if *readerURL != "" {
		reader = sourceprocessing.NewHTTPChartVisionProvider(*readerURL)
	}

	localiser := sourceprocessing.DefaultCarrierLocaliserConfig()
	readerConfig := sourceprocessing.DefaultCarrierReaderConfig()

	documents := make([]predictionDocument, 0, len(manifest))
	for _, entry := range manifest {
		document := predictionDocument{
			DocumentID:  entry.DocumentID,
			SourceClass: entry.SourceClass,
			Split:       entry.Split,
			Rendering:   entry.Rendering,
			Branches:    []any{},
			Connectors:  []any{},
			Edges:       []any{},
		}
		started := time.Now()
		for index, page := range entry.Pages {
			if *limit > 0 && index >= *limit {
				break
			}
			data, err := os.ReadFile(filepath.Join(base, page.ImagePath))
			if err != nil {
				fmt.Fprintf(os.Stderr, "ocr-eval: %s/%s: %v\n", entry.DocumentID, page.PageID, err)
				continue
			}
			document.InputImageBytes += int64(len(data))
			decoded, err := decodePNG(data)
			if err != nil {
				continue
			}
			bounds := decoded.Bounds()
			document.Pages = append(document.Pages, predictionPage{
				PageID:    page.PageID,
				Processed: true,
				Width:     bounds.Dx(),
				Height:    bounds.Dy(),
			})

			regions, err := sourceprocessing.DetectNameCarriers(data, localiser)
			if err != nil {
				continue
			}
			groups := make([]sourceprocessing.PageRegion, 0, len(regions))
			for _, region := range regions {
				if region.Kind == "name_group" {
					groups = append(groups, region)
				}
			}
			if *maxCarriers > 0 && len(groups) > *maxCarriers {
				groups = groups[:*maxCarriers]
			}
			if *sample > 0 && len(groups) > *sample {
				groups = evenlySpacedRegions(groups, *sample)
			}
			if reader == nil || len(groups) == 0 {
				// Without a reader the carriers are still reported, as detected
				// occurrences with no transcription. That measures the
				// localiser's DETECTION recall, which is a different and
				// separately useful number.
				for index, group := range groups {
					document.Nodes = append(document.Nodes, predictionNode{
						ID:     fmt.Sprintf("%s-%s-%06d", entry.DocumentID, page.PageID, index),
						PageID: page.PageID,
						NameAR: "",
						Status: "detected",
						Region: region{
							X:      group.X,
							Y:      group.Y,
							Width:  group.Width,
							Height: group.Height,
						},
					})
				}
				continue
			}
			readings, err := sourceprocessing.ReadCarriers(context.Background(), reader, decoded, groups, readerConfig)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ocr-eval: %s/%s: %v\n", entry.DocumentID, page.PageID, err)
				continue
			}
			for index, reading := range readings {
				if index >= len(groups) {
					break
				}
				group := groups[index]
				document.Nodes = append(document.Nodes, predictionNode{
					ID:     fmt.Sprintf("%s-%s-%06d", entry.DocumentID, page.PageID, index),
					PageID: page.PageID,
					NameAR: reading.TextAR,
					Status: reading.Status,
					Region: region{
						X:      group.X,
						Y:      group.Y,
						Width:  group.Width,
						Height: group.Height,
					},
				})
			}
			document.ProviderCalls += readerConfig.Views
		}
		document.ProcessingSeconds = time.Since(started).Seconds()
		sort.Slice(document.Nodes, func(i, j int) bool { return document.Nodes[i].ID < document.Nodes[j].ID })
		documents = append(documents, document)
		fmt.Fprintf(os.Stderr, "ocr-eval: %s: %d pages, %d names\n", entry.DocumentID, len(document.Pages), len(document.Nodes))
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ocr-eval: %v\n", err)
		os.Exit(2)
	}
	target := filepath.Join(*outputDir, *pipeline+".jsonl")
	file, err := os.Create(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ocr-eval: %v\n", err)
		os.Exit(2)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, document := range documents {
		if err := encoder.Encode(document); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-eval: %v\n", err)
			os.Exit(2)
		}
	}
	fmt.Fprintf(os.Stderr, "ocr-eval: wrote %s\n", target)
}

// evenlySpacedRegions takes a deterministic sample across the page instead of
// the first N groups. The first groups of a chart are the title and the
// ancestor stack, so a prefix is a biased sample of a family tree; a stride
// reaches the branches.
func evenlySpacedRegions(regions []sourceprocessing.PageRegion, sample int) []sourceprocessing.PageRegion {
	if sample <= 0 || len(regions) <= sample {
		return regions
	}
	sampled := make([]sourceprocessing.PageRegion, 0, sample)
	for index := 0; index < sample; index++ {
		sampled = append(sampled, regions[index*len(regions)/sample])
	}
	return sampled
}

// compareRenderings pairs the documents in a predictions file that are
// renderings of the same facts, and reports name-level agreement. This is the
// measurement an answer key would otherwise provide: two independent readings
// of the same names, compared to each other.
func compareRenderings(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	byRendering := map[string][]predictionDocument{}
	decoder := json.NewDecoder(file)
	for {
		var document predictionDocument
		if err := decoder.Decode(&document); err != nil {
			break
		}
		if document.Rendering == "" {
			continue
		}
		byRendering[document.Rendering] = append(byRendering[document.Rendering], document)
	}
	renderings := make([]string, 0, len(byRendering))
	for rendering := range byRendering {
		renderings = append(renderings, rendering)
	}
	sort.Strings(renderings)
	compared := 0
	for _, rendering := range renderings {
		documents := byRendering[rendering]
		if len(documents) < 2 {
			continue
		}
		left := documentNames(documents[0])
		right := documentNames(documents[1])
		agreement := sourceprocessing.CompareCarrierTexts(left, right, 0.8)
		fmt.Printf("pair-check: rendering %q: %d vs %d readings, %d matched (%d exact, %d fuzzy), agreement %.1f%%\n",
			rendering, agreement.LeftReadings, agreement.RightReadings, agreement.Matched,
			agreement.ExactMatched, agreement.FuzzyMatched, agreement.Agreement*100)
		printReadingSample("left only", agreement.LeftOnly)
		printReadingSample("right only", agreement.RightOnly)
		compared++
	}
	if compared == 0 {
		return fmt.Errorf("%s has no rendering pair to compare", path)
	}
	return nil
}

func documentNames(document predictionDocument) []string {
	names := make([]string, 0, len(document.Nodes))
	for _, node := range document.Nodes {
		if node.NameAR != "" {
			names = append(names, node.NameAR)
		}
	}
	return names
}

func printReadingSample(label string, readings []string) {
	if len(readings) == 0 {
		return
	}
	limit := len(readings)
	if limit > 8 {
		limit = 8
	}
	fmt.Printf("  %s (%d): %s\n", label, len(readings), strings.Join(readings[:limit], " · "))
}
