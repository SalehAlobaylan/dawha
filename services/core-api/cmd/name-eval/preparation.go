package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	sp "github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
)

type initialEvidence struct {
	BoundaryAt           *time.Time                `json:"boundaryAt"`
	GroupKey             string                    `json:"groupKey"`
	DecisionGap          string                    `json:"decisionGap"`
	Schema               string                    `json:"schema"`
	PreparationID        string                    `json:"preparationId"`
	ExposureID           string                    `json:"exposureId"`
	SuggestionID         string                    `json:"suggestionId"`
	ActualSuggestionID   string                    `json:"actualSuggestionId"`
	Signature            string                    `json:"signature"`
	Snapshot             json.RawMessage           `json:"snapshot"`
	Versions             map[string]string         `json:"versions"`
	Context              sp.NamePreparationContext `json:"context"`
	LexiconRevision      string                    `json:"lexiconRevision"`
	ExposureAt           *time.Time                `json:"exposureAt"`
	PreparationAt        *time.Time                `json:"preparationAt"`
	RequestAt            *time.Time                `json:"requestAt"`
	ResultAt             *time.Time                `json:"resultAt"`
	RegistrationAt       *time.Time                `json:"registrationAt"`
	RequestPayload       json.RawMessage           `json:"requestPayload"`
	RequestIdentity      sp.FrozenJudgeIdentity    `json:"requestIdentity"`
	RequestSignature     string                    `json:"requestSignature"`
	ResultSignature      string                    `json:"resultSignature"`
	Result               sp.NameJudgeResult        `json:"result"`
	LatencyMS            int                       `json:"latencyMs"`
	Eligible             bool                      `json:"eligible"`
	Partition            string                    `json:"partition"`
	RegistrationContract map[string]string         `json:"registrationContract"`
}

func applyInitialEvidence(row *decisionRow, e initialEvidence) {
	row.SourceGroup = e.GroupKey
	row.SuggestionID = e.ActualSuggestionID
	row.ReadingDiagnostics = e.Context.Diagnostics
	row.Partition = e.Partition
	if row.Partition == "" {
		row.Partition = "legacy"
	}
	if e.Schema == "" {
		// A legacy client cannot create blind evidence for an enrolled new file.
		// Old ledger snapshots otherwise retain their exact original interpretation.
		if e.Partition != "held_out" && (e.BoundaryAt == nil || row.DecisionAt.Before(*e.BoundaryAt)) {
			return
		}
		e.Schema = sp.NameEvidenceVersion
	}
	row.EvidenceVersion = e.Schema
	row.InitialGap = "missing_preparation"
	row.InitialValid = false
	row.Judge = judgeOutput{}
	row.ExposureID = e.ExposureID
	row.BlindEligible = false
	versions := e.Versions
	if len(versions) == 0 {
		versions = e.RegistrationContract
	}
	rawVersions, _ := json.Marshal(versions)
	row.RecipeVersion = sp.FrozenNameJudgeSignature(rawVersions)
	row.PoolVersion = versions["pool"]
	row.ReaderVersion = versions["reader"]
	row.GenerationVersion = versions["generation"]
	row.RendererVersion = versions["renderer"]
	row.LexiconVersion = versions["lexicon"]
	row.FrozenModel = versions["judgeModel"]
	row.FrozenPrompt = versions["judgePrompt"]
	row.Candidates = nil
	row.NormalizerVersion = versions["normalizer"]
	if e.PreparationID == "" {
		return
	}
	row.InitialGap = "late_preparation"
	if e.ExposureAt == nil || e.PreparationAt == nil || !e.PreparationAt.Before(row.DecisionAt) || e.ExposureAt.After(*e.PreparationAt) {
		return
	}
	row.InitialGap = "invalid_preparation"
	if e.Schema != sp.NameEvidenceVersion || e.SuggestionID != e.ActualSuggestionID || e.ExposureID == "" || e.Signature != sp.NamePreparationSignature(e.ExposureID, versions["pool"], e.Snapshot) {
		return
	}
	if versions["pool"] != sp.NamePoolVersion || versions["evidence"] != sp.NameEvidenceVersion || versions["generation"] != sp.NameGenerationVersion || versions["normalizer"] != normalizerVersion || versions["renderer"] != sp.NameRendererVersion || versions["geometry"] != sp.NameGeometryVersion || versions["preprocessing"] != sp.NamePreprocessingVersion || versions["lexicon"] != sp.NameLexiconVersion || versions["reader"] == "" {
		return
	}
	if len(e.Context.Lexicon.Revision) != 64 || e.Context.Lexicon.Revision != e.LexiconRevision || !equalEvidenceJSON(e.Context.Versions, versions) {
		return
	}
	var candidates []candidateSnapshot
	if json.Unmarshal(e.Snapshot, &candidates) != nil || len(candidates) > 12 {
		return
	}
	values := map[string]bool{}
	positions := map[int]bool{}
	shown := 0
	for _, c := range candidates {
		if c.Value == "" || c.Key != canonicalName(c.Value) || c.CompactKey != compactName(c.Key) || c.ParticleKey != particleName(c.Key) || values[c.Value] || len(c.Sources) == 0 {
			return
		}
		values[c.Value] = true
		if c.Shown {
			shown++
			if c.Position < 1 || c.Position > 8 || positions[c.Position] {
				return
			}
			positions[c.Position] = true
		} else if c.Position != 0 {
			return
		}
	}
	if shown > 8 {
		return
	}
	row.Candidates = candidates
	row.InitialGap = ""
	if e.Context.Frame != nil {
		row.PageWidth = float64(e.Context.Frame.Width)
		row.PageHeight = float64(e.Context.Frame.Height)
	}
	row.InitialValid = true
	row.ContextTruncated = e.Context.Lexicon.Truncated
	row.BlindEligible = e.Context.PrivacyPolicy == "local_text_only" && e.DecisionGap == "" && e.Eligible && e.Partition == "held_out" && e.RegistrationAt != nil && !e.RegistrationAt.After(*e.ExposureAt) && equalEvidenceJSON(e.RegistrationContract, versions) && !row.ContextTruncated
	row.JudgeGap = "missing_frozen_request"
	if len(row.Candidates) == 0 {
		row.JudgeGap = "no_candidates"
		return
	}
	if e.RequestAt == nil {
		return
	}
	row.JudgeGap = "late_judge_request"
	if !e.RequestAt.Before(row.DecisionAt) || e.RequestAt.Before(*e.PreparationAt) {
		return
	}
	row.JudgeGap = "invalid_judge_request"
	if e.RequestSignature == "" || e.RequestSignature != sp.FrozenNameJudgeSignature(e.RequestPayload) {
		return
	}
	var request struct {
		State      sp.JudgeState       `json:"state"`
		Candidates []sp.JudgeCandidate `json:"candidates"`
		Mode       string              `json:"mode"`
	}
	decoder := json.NewDecoder(bytes.NewReader(e.RequestPayload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request.Mode != "shadow" || len(request.Candidates) != len(row.Candidates) || len(request.State.FileContext) > 20 || len(request.State.Alternatives) > 5 {
		return
	}
	if e.Context.Frame == nil || e.Context.Crop.WidthPx < 1 || e.Context.Crop.HeightPx < 1 || !equalEvidenceJSON(request.State, sp.FrozenNameJudgeState(e.Context)) {
		return
	}
	for i, c := range request.Candidates {
		if c.Value != row.Candidates[i].Value || c.Source != row.Candidates[i].Sources[0] || c.Rank != i+1 {
			return
		}
	}
	if e.RequestIdentity.Provider != "liquid" || e.RequestIdentity.Model != row.FrozenModel || e.RequestIdentity.Prompt != row.FrozenPrompt {
		return
	}
	row.JudgeGap = "missing_server_result"
	if e.ResultAt == nil {
		return
	}
	row.JudgeGap = "invalid_server_result"
	if sp.ValidateTextOnlyNameJudgeResult(e.Result, request.Candidates) != nil || e.ResultAt.Before(*e.RequestAt) || e.ResultSignature != e.RequestSignature || e.Result.Model != row.FrozenModel || e.Result.PromptVersion != row.FrozenPrompt || e.Result.Provider != e.RequestIdentity.Provider {
		return
	}
	choice := "unclear"
	if e.Result.Choice != nil {
		choice = *e.Result.Choice
	}
	if !e.Result.Abstained && !values[choice] {
		return
	}
	confidence := e.Result.Confidence
	row.Judge = judgeOutput{Mode: "shadow", Provider: e.Result.Provider, Model: e.Result.Model, PromptVersion: e.Result.PromptVersion, Choice: choice, Probabilities: e.Result.Probabilities, Confidence: &confidence, Abstained: e.Result.Abstained, Readable: e.Result.Readable, Shown: false, Used: false, LatencyMS: e.LatencyMS}
	row.JudgeGap = ""
}
func equalEvidenceJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return sp.FrozenNameJudgeSignature(x) == sp.FrozenNameJudgeSignature(y)
}

// uniqueObservations selects one human truth revision per physical occurrence.
// first is the original cohort semantics; latest chooses the most recent answer
// inside the pinned window. Superseded geometry is excluded from both policies,
// because a split or merge parent is not an independent observation beside its
// descendants. Retry rows and later revisions are counted separately so a
// collapsed revision cannot inflate the denominator.
func uniqueObservations(rows []decisionRow, truthPolicy string) ([]decisionRow, int, int, int) {
	copyRows := append([]decisionRow(nil), rows...)
	sort.SliceStable(copyRows, func(i, j int) bool {
		if copyRows[i].DecisionAt.Equal(copyRows[j].DecisionAt) {
			if copyRows[i].ReviewRevision != copyRows[j].ReviewRevision {
				return copyRows[i].ReviewRevision < copyRows[j].ReviewRevision
			}
			return copyRows[i].ID < copyRows[j].ID
		}
		return copyRows[i].DecisionAt.Before(copyRows[j].DecisionAt)
	})
	superseded := 0
	kept := make([]decisionRow, 0, len(copyRows))
	for index, row := range copyRows {
		if row.Superseded {
			superseded++
			continue
		}
		if row.SuggestionID == "" {
			row.SuggestionID = row.ID
			if row.SuggestionID == "" {
				row.SuggestionID = fmt.Sprintf("row-%d", index)
			}
		}
		kept = append(kept, row)
	}
	groups := make(map[string][]decisionRow)
	order := make([]string, 0, len(kept))
	for _, row := range kept {
		if _, exists := groups[row.SuggestionID]; !exists {
			order = append(order, row.SuggestionID)
		}
		groups[row.SuggestionID] = append(groups[row.SuggestionID], row)
	}
	selected := make([]decisionRow, 0, len(groups))
	duplicates := 0
	collapsed := 0
	for _, key := range order {
		group := groups[key]
		chosen := group[0]
		if truthPolicy == "latest" {
			chosen = group[len(group)-1]
		}
		selected = append(selected, chosen)
		for _, row := range group {
			if row.ID == chosen.ID {
				continue
			}
			if row.ReviewRevision > 0 {
				collapsed++
			} else {
				duplicates++
			}
		}
	}
	// A repeated exposure across distinct ids is still a retry, not a second
	// physical occurrence. Keep the earliest selected row for the exposure.
	seen := map[string]bool{}
	out := make([]decisionRow, 0, len(selected))
	for _, row := range selected {
		if row.EvidenceVersion == sp.NameEvidenceVersion && row.ExposureID != "" {
			if seen[row.ExposureID] {
				duplicates++
				continue
			}
			seen[row.ExposureID] = true
		}
		out = append(out, row)
	}
	return out, duplicates, superseded, collapsed
}
