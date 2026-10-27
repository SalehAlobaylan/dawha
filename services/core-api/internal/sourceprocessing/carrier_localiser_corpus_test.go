package sourceprocessing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The corpus manifest, as tools/ocr-eval reads it.
// The manifest is a JSON array of documents, as tools/ocr-eval reads it.
type corpusManifestEntry struct {
	DocumentID string `json:"document_id"`
	Rendering  string `json:"rendering_of"`
	Pages      []struct {
		PageID    string `json:"page_id"`
		ImagePath string `json:"image_path"`
	} `json:"pages"`
}

type corpusDocument struct {
	DocumentID string
	Rendering  string
	Pages      []string
}

func readCorpus(t *testing.T) []corpusDocument {
	t.Helper()
	directory := strings.TrimSpace(os.Getenv("DAWHA_CORPUS_DIR"))
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("DAWHA_CORPUS_DIR is unset and the home directory is unavailable, so there is no corpus to measure against.")
		}
		directory = filepath.Join(home, ".dawha", "corpus")
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Skipf("DAWHA_CORPUS_DIR is set but %s cannot be read, so there is no corpus to measure against: %v", manifestPath, err)
	}
	var manifest []corpusManifestEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("the corpus manifest at %s is not valid JSON: %v", manifestPath, err)
	}
	documents := make([]corpusDocument, 0, len(manifest))
	for _, document := range manifest {
		entry := corpusDocument{DocumentID: document.DocumentID, Rendering: document.Rendering}
		for _, page := range document.Pages {
			entry.Pages = append(entry.Pages, filepath.Join(directory, page.ImagePath))
		}
		documents = append(documents, entry)
	}
	return documents
}

// The paired consistency check.
//
// Two renderings of the same family carry the same names. Run the localiser over
// both and the number of carriers it finds must agree - that agreement is the
// only accuracy signal available before a reader exists, and it needs no answer
// key.
//
// The tolerance is 15 %. It is not a claim about accuracy: it is a guard against
// the localiser silently drifting, and against a threshold change that happens to
// suit one rendering and not the other.
func TestNameCarrierLocaliserAgreesAcrossTwoRenderingsOfTheSameFacts(t *testing.T) {
	documents := readCorpus(t)
	byRendering := map[string][]corpusDocument{}
	for _, document := range documents {
		if document.Rendering == "" {
			continue
		}
		byRendering[document.Rendering] = append(byRendering[document.Rendering], document)
	}

	checked := 0
	for rendering, group := range byRendering {
		if len(group) < 2 {
			continue
		}
		counts := make([]int, 0, len(group))
		nameCounts := make([]int, 0, len(group))
		for _, document := range group {
			total := 0
			names := 0
			for _, page := range document.Pages {
				data, err := os.ReadFile(page)
				if err != nil {
					t.Fatalf("the corpus page %s cannot be read: %v", page, err)
				}
				regions, err := DetectNameCarriers(data, DefaultCarrierLocaliserConfig())
				if err != nil {
					t.Fatalf("the localiser failed on %s: %v", page, err)
				}
				total += len(regions)
				for _, region := range regions {
					if region.Kind == "name_group" {
						names++
					}
				}
			}
			counts = append(counts, total)
			nameCounts = append(nameCounts, names)
		}
		lowest, highest := counts[0], counts[0]
		for _, count := range counts {
			if count < lowest {
				lowest = count
			}
			if count > highest {
				highest = count
			}
		}
		if lowest == 0 {
			t.Fatalf("rendering %q: the localiser found no carriers in any of its %d documents", rendering, len(counts))
		}
		spread := float64(highest-lowest) / float64(lowest)
		if spread > 0.15 {
			t.Errorf("rendering %q: the localiser found %d regions in one rendering and %d in another, a spread of %.1f%%, which is past the 15%% the two renderings of the same facts should agree within",
				rendering, lowest, highest, spread*100)
		}

		// The NAME GROUPS are the number that matters, because a name group is
		// one name. The carriers are the coloured shapes, and on a painted
		// tree the leaves touch, so a carrier is often a dozen of them.
		nameLowest, nameHighest := nameCounts[0], nameCounts[0]
		for _, count := range nameCounts {
			if count < nameLowest {
				nameLowest = count
			}
			if count > nameHighest {
				nameHighest = count
			}
		}
		if nameLowest == 0 {
			t.Fatalf("rendering %q: the localiser found no name groups in any of its %d documents", rendering, len(nameCounts))
		}
		nameSpread := float64(nameHighest-nameLowest) / float64(nameLowest)
		if nameSpread > 0.15 {
			t.Errorf("rendering %q: the localiser found %d names in one rendering and %d in another, a spread of %.1f%%, which is past the 15%% the two renderings of the same facts should agree within",
				rendering, nameLowest, nameHighest, nameSpread*100)
		}
		t.Logf("rendering %q: %d regions and %d names, in agreement within %.1f%%",
			rendering, lowest, nameLowest, nameSpread*100)
		checked++
	}
	if checked == 0 {
		t.Skip("the corpus has no group of two or more documents that are renderings of the same facts, so there is no pair to compare.")
	}
}
