package main

import (
	"encoding/json"
	sp "github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"strings"
	"testing"
	"time"
)

func preparedEvidenceFixture() (decisionRow, initialEvidence) {
	at := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	exposed := at
	prepared := at.Add(time.Millisecond)
	requested := at.Add(2 * time.Millisecond)
	decided := at.Add(3 * time.Millisecond)
	finished := at.Add(time.Hour)
	registered := at.Add(-time.Hour)
	versions := map[string]string{"pool": sp.NamePoolVersion, "normalizer": normalizerVersion, "evidence": sp.NameEvidenceVersion, "generation": sp.NameGenerationVersion, "lexicon": sp.NameLexiconVersion, "renderer": sp.NameRendererVersion, "geometry": sp.NameGeometryVersion, "preprocessing": sp.NamePreprocessingVersion, "reader": "reader-fixture-v2", "judgeModel": "d1:3b", "judgePrompt": "name-judge-liquid-v1"}
	snapshot, _ := json.Marshal([]candidateSnapshot{{Value: "محمد", Key: "محمد", CompactKey: "محمد", ParticleKey: "محمد", Sources: []string{"reader"}, Shown: true, Position: 1}})
	context := sp.NamePreparationContext{PrivacyPolicy: "local_text_only", JudgeMode: "shadow", JudgeSample: 1, Reading: "محمد", Readers: []sp.NameReaderEvidence{{Value: "محمد", Variant: "raw-psm7", ReaderVersion: "reader-fixture-v2"}}, Lexicon: sp.NameLexicon{Revision: strings.Repeat("a", 64), Version: sp.NameLexiconVersion, Entries: []sp.NameLexiconEntry{{Value: "نادر", Key: "نادر", Count: 2}}}, Versions: versions, Frame: &sp.NamePageFrame{Width: 600, Height: 400, Space: "native_raster", Renderer: sp.NameRendererVersion}, Crop: sp.JudgeCrop{WidthPx: 120, HeightPx: 40, SourceWidthPx: 600, SourceHeightPx: 400, NameHeightPx: 30}}
	request, _ := json.Marshal(map[string]any{"state": sp.FrozenNameJudgeState(context), "candidates": []sp.JudgeCandidate{{Value: "محمد", Source: "reader", Rank: 1}}, "mode": "shadow"})
	signature := sp.FrozenNameJudgeSignature(request)
	choice := "محمد"
	e := initialEvidence{Schema: sp.NameEvidenceVersion, PreparationID: "preparation-1", ExposureID: "exposure-1", SuggestionID: "occurrence-1", ActualSuggestionID: "occurrence-1", Snapshot: snapshot, Versions: versions, Context: context, LexiconRevision: context.Lexicon.Revision, ExposureAt: &exposed, PreparationAt: &prepared, RequestAt: &requested, ResultAt: &finished, RegistrationAt: &registered, RequestPayload: request, RequestIdentity: sp.FrozenJudgeIdentity{Provider: "liquid", Model: "d1:3b", Prompt: "name-judge-liquid-v1"}, RequestSignature: signature, ResultSignature: signature, Result: sp.NameJudgeResult{Choice: &choice, Probabilities: map[string]float64{"محمد": .8, "unclear": .2}, Confidence: .8, Provider: "liquid", Model: "d1:3b", PromptVersion: "name-judge-liquid-v1"}, Eligible: true, Partition: "held_out", RegistrationContract: versions}
	e.Signature = sp.NamePreparationSignature(e.ExposureID, sp.NamePoolVersion, snapshot)
	row := decisionRow{ID: "decision-1", Decision: "corrected", DecidedText: "محمد", DecisionAt: decided, FileID: "source-1", SourceClass: "clean_print", Judge: judgeOutput{Mode: "active", Choice: "browser-forged"}}
	return row, e
}
func TestInitialEvidenceJoinsLateServerOutputAndRejectsBrowserSubstitution(t *testing.T) {
	row, e := preparedEvidenceFixture()
	applyInitialEvidence(&row, e)
	if !row.InitialValid || !row.BlindEligible || row.InitialGap != "" || row.JudgeGap != "" || row.Judge.Mode != "shadow" || row.Judge.Choice != "محمد" {
		t.Fatalf("valid frozen evidence rejected: %+v", row)
	}
	if row.PoolVersion != sp.NamePoolVersion || row.PageWidth != 600 {
		t.Fatal("saved version/frame not used")
	}
	e.ResultAt = nil
	row, _ = preparedEvidenceFixture()
	applyInitialEvidence(&row, e)
	if row.Judge.present() || row.JudgeGap != "missing_server_result" {
		t.Fatal("browser output replaced missing server result")
	}
}
func TestInitialEvidenceRejectsLateMismatchedOrLeakingInputs(t *testing.T) {
	cases := map[string]func(*initialEvidence){
		"late preparation":   func(e *initialEvidence) { x := e.PreparationAt.Add(time.Hour); e.PreparationAt = &x },
		"late request":       func(e *initialEvidence) { x := e.RequestAt.Add(time.Hour); e.RequestAt = &x },
		"snapshot signature": func(e *initialEvidence) { e.Signature = strings.Repeat("0", 64) },
		"request signature":  func(e *initialEvidence) { e.RequestSignature = strings.Repeat("0", 64) },
		"result signature":   func(e *initialEvidence) { e.ResultSignature = strings.Repeat("0", 64) },
		"wrong occurrence":   func(e *initialEvidence) { e.SuggestionID = "another-occurrence" },
		"wrong lexicon":      func(e *initialEvidence) { e.LexiconRevision = strings.Repeat("b", 64) },
		"wrong model":        func(e *initialEvidence) { e.Result.Model = "changed-model" },
		"provider image": func(e *initialEvidence) {
			var r map[string]any
			_ = json.Unmarshal(e.RequestPayload, &r)
			r["image_base64"] = "private-image"
			e.RequestPayload, _ = json.Marshal(r)
			e.RequestSignature = sp.FrozenNameJudgeSignature(e.RequestPayload)
			e.ResultSignature = e.RequestSignature
		},
		"context rebuilt after answer": func(e *initialEvidence) {
			var r map[string]any
			_ = json.Unmarshal(e.RequestPayload, &r)
			r["state"].(map[string]any)["file_context"] = []string{"known-answer-after-freeze"}
			e.RequestPayload, _ = json.Marshal(r)
			e.RequestSignature = sp.FrozenNameJudgeSignature(e.RequestPayload)
			e.ResultSignature = e.RequestSignature
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			row, e := preparedEvidenceFixture()
			mutate(&e)
			applyInitialEvidence(&row, e)
			if row.Judge.present() {
				t.Fatal("invalid immutable relation counted")
			}
			if row.InitialGap == "" && row.JudgeGap == "" {
				t.Fatal("missing diagnostic")
			}
		})
	}
}
func TestKnownOrUnregisteredTruthCannotPassHeldOutGate(t *testing.T) {
	for _, kind := range []string{"development", "reopened", "late registration", "active review"} {
		t.Run(kind, func(t *testing.T) {
			row, e := preparedEvidenceFixture()
			switch kind {
			case "development":
				e.Partition = "development"
			case "reopened":
				e.Eligible = false
			case "active review":
				e.DecisionGap = "active_review"
			case "late registration":
				late := e.ExposureAt.Add(time.Second)
				e.RegistrationAt = &late
			}
			applyInitialEvidence(&row, e)
			if row.BlindEligible {
				t.Fatal("known truth was admitted as unseen")
			}
			result := summarizeWithClassMap([]decisionRow{row}, "", true)
			if result.ShadowGate.JudgedWithTruth != 0 {
				t.Fatal("development or history counted")
			}
		})
	}
}
func TestRetriesCountOnceAndDistinctPhysicalOccurrencesCountTwice(t *testing.T) {
	row, e := preparedEvidenceFixture()
	applyInitialEvidence(&row, e)
	retry := row
	retry.ID = "decision-2"
	retry.DecisionAt = row.DecisionAt.Add(time.Hour)
	other, otherEvidence := preparedEvidenceFixture()
	other.ID = "decision-3"
	otherEvidence.ExposureID = "exposure-2"
	otherEvidence.PreparationID = "preparation-2"
	otherEvidence.SuggestionID = "occurrence-2"
	otherEvidence.ActualSuggestionID = "occurrence-2"
	otherEvidence.Signature = sp.NamePreparationSignature(otherEvidence.ExposureID, sp.NamePoolVersion, otherEvidence.Snapshot)
	applyInitialEvidence(&other, otherEvidence)
	result := summarizeWithClassMap([]decisionRow{retry, other, row}, "", true)
	if result.Decisions != 2 || result.DuplicateObservations != 1 || result.ShadowGate.JudgedWithTruth != 2 {
		t.Fatalf("occurrence accounting changed: %+v", result)
	}
}
func TestValidEmptyPreparationRemainsANoCandidateOutcome(t *testing.T) {
	row, e := preparedEvidenceFixture()
	e.Snapshot = json.RawMessage(`[]`)
	e.Signature = sp.NamePreparationSignature(e.ExposureID, sp.NamePoolVersion, e.Snapshot)
	e.RequestAt = nil
	e.ResultAt = nil
	applyInitialEvidence(&row, e)
	result := summarizeWithClassMap([]decisionRow{row}, "", true)
	if result.MissingSnapshot != 0 || result.WithSnapshot != 1 || result.NoCandidates.Matches != 1 {
		t.Fatal("no-candidate result became a missing snapshot")
	}
}

func TestRejectedInitialSnapshotsNeverContributeQualityMetrics(t *testing.T) {
	for _, kind := range []string{"position", "key", "duplicate", "hidden position", "too many", "partial JSON"} {
		t.Run(kind, func(t *testing.T) {
			row, e := preparedEvidenceFixture()
			var candidates []candidateSnapshot
			if err := json.Unmarshal(e.Snapshot, &candidates); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "position":
				candidates[0].Position = 9
			case "key":
				candidates[0].Key = "wrong"
			case "duplicate":
				candidates = append(candidates, candidates[0])
			case "hidden position":
				candidates[0].Shown = false
			case "too many":
				for len(candidates) < 13 {
					candidates = append(candidates, candidates[0])
				}
			}
			e.Snapshot, _ = json.Marshal(candidates)
			if kind == "partial JSON" {
				// A type error can leave a partially decoded nonempty slice.
				e.Snapshot = append(e.Snapshot[:len(e.Snapshot)-1], []byte(`,{"position":"bad"}]`)...)
			}
			e.Signature = sp.NamePreparationSignature(e.ExposureID, sp.NamePoolVersion, e.Snapshot)
			applyInitialEvidence(&row, e)
			result := summarizeWithClassMap([]decisionRow{row}, "", true)
			if row.InitialValid || row.InitialGap != "invalid_preparation" || len(row.Candidates) != 0 {
				t.Fatalf("rejected snapshot retained: %+v", row)
			}
			if result.WithSnapshot != 0 || result.Recall.Matches != 0 || result.MissingSnapshot != 1 {
				t.Fatalf("invalid snapshot contributed metrics: %+v", result)
			}
		})
	}
}

func TestNewLegacyClientsCannotBypassThePreparationBoundary(t *testing.T) {
	boundary := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	old := decisionRow{DecisionAt: boundary.Add(-time.Hour), Candidates: []candidateSnapshot{{Value: "محمد", Key: "محمد", Shown: true, Position: 1}}, PoolVersion: "ar-name-pool-v1", NormalizerVersion: normalizerVersion, Judge: judgeOutput{Mode: "shadow", Model: "old-model", PromptVersion: "old-prompt", Choice: "محمد"}}
	applyInitialEvidence(&old, initialEvidence{BoundaryAt: &boundary})
	if old.EvidenceVersion != "" || len(old.Candidates) != 1 || old.PoolVersion != "ar-name-pool-v1" || old.Judge.Model != "old-model" {
		t.Fatal("historical evidence reinterpreted")
	}
	recent := old
	recent.DecisionAt = boundary.Add(time.Hour)
	applyInitialEvidence(&recent, initialEvidence{BoundaryAt: &boundary})
	if recent.EvidenceVersion != sp.NameEvidenceVersion || recent.InitialGap != "missing_preparation" || recent.Judge.present() {
		t.Fatal("a new legacy client bypassed initial preparation")
	}
}
