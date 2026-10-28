package main

import (
	"testing"
	"time"
)

func TestKeysForUsesThePublishedNameKeys(t *testing.T) {
	keys := keysFor("آل الصباح")
	if keys.Canonical != "ال الصباح" {
		t.Fatalf("canonical = %q", keys.Canonical)
	}
	if keys.Compact != "الالصباح" {
		t.Fatalf("compact = %q", keys.Compact)
	}
	if keys.Particle != "صباح" {
		t.Fatalf("particle = %q", keys.Particle)
	}
}

func TestCandidateMatchesAcrossSpacingAndParticleKeys(t *testing.T) {
	spaced := candidateSnapshot{Value: "عبد الله", Key: "عبد الله"}
	if !candidateMatches(spaced, keysFor("عبدالله")) {
		t.Fatal("a compound should match its compact key")
	}

	family := candidateSnapshot{Value: "آل الصباح", Key: "ال الصباح"}
	if !candidateMatches(family, keysFor("الصباح")) {
		t.Fatal("a family name should match its particle key")
	}
}

func TestSummarizeCountsRecallRankingAndMissingEvidence(t *testing.T) {
	rows := []decisionRow{
		{
			FileID:      "file-1",
			FileName:    "chart.png",
			Decision:    "accepted",
			DecidedText: "محمد",
			Candidates: []candidateSnapshot{
				{Value: "محمد", Key: "محمد", Shown: true, Position: 1},
				{Value: "أحمد", Key: "احمد", Shown: true, Position: 2},
			},
			InputMethod:       "reader",
			NormalizerVersion: normalizerVersion,
			DurationMS:        1000,
		},
		{
			FileID:      "file-1",
			FileName:    "chart.png",
			Decision:    "corrected",
			DecidedText: "جاسم",
			Candidates: []candidateSnapshot{
				{Value: "علي", Key: "علي", Shown: true, Position: 1},
				{Value: "خالد", Key: "خالد", Shown: true, Position: 2},
			},
			InputMethod:       "typed",
			NormalizerVersion: normalizerVersion,
			DurationMS:        3000,
		},
		{
			FileID:      "file-2",
			FileName:    "old.png",
			Decision:    "accepted",
			DecidedText: "محمد",
			InputMethod: "legacy",
			DurationMS:  500,
		},
	}

	result := summarize(rows, "")
	if result.Decisions != 3 || result.WithSnapshot != 2 || result.MissingSnapshot != 1 {
		t.Fatalf("coverage = %+v", result)
	}
	if result.Recall.Matches != 1 || result.Recall.Total != 2 {
		t.Fatalf("recall = %+v", result.Recall)
	}
	if result.RecallAny.Matches != 1 || result.RecallAny.Total != 2 {
		t.Fatalf("recall any shown = %+v", result.RecallAny)
	}
	if result.NoCandidates.Matches != 0 || result.NoCandidates.Total != 2 {
		t.Fatalf("no candidates = %+v", result.NoCandidates)
	}
	if result.AllWrong.Matches != 1 || result.AllWrong.Total != 2 {
		t.Fatalf("all wrong = %+v", result.AllWrong)
	}
	if result.Top1.Matches != 1 || result.Top1.Total != 2 {
		t.Fatalf("top-1 = %+v", result.Top1)
	}
	if result.Top3.Matches != 1 || result.Top3.Total != 2 {
		t.Fatalf("top-3 = %+v", result.Top3)
	}
	if result.Typed.Matches != 1 || result.Typed.Total != 2 {
		t.Fatalf("typed = %+v", result.Typed)
	}
	if result.MedianMS == nil || *result.MedianMS != 2000 {
		t.Fatalf("median = %v", result.MedianMS)
	}
	if result.Recall.Measurable {
		t.Fatal("two decisions must not be reported as a measurable rate")
	}
	if len(result.Files) != 2 {
		t.Fatalf("files = %+v", result.Files)
	}
	if result.Files[0].Recall.Matches != 1 || result.Files[0].Recall.Total != 2 {
		t.Fatalf("file recall = %+v", result.Files[0])
	}
	if result.Files[0].RecallAny.Matches != 1 || result.Files[0].RecallAny.Total != 2 {
		t.Fatalf("file recall any = %+v", result.Files[0])
	}
}

func TestSummarizeDoesNotReinterpretASnapshotFromAnotherNormalizer(t *testing.T) {
	rows := []decisionRow{{
		FileID:            "file-1",
		FileName:          "chart.png",
		Decision:          "accepted",
		DecidedText:       "محمد",
		Candidates:        []candidateSnapshot{{Value: "محمد", Key: "محمد", Shown: true, Position: 1}},
		NormalizerVersion: "ar-name-keys-v0",
	}}

	result := summarize(rows, "")
	if result.Decisions != 1 || result.WithSnapshot != 0 || result.StaleNormalizer != 1 {
		t.Fatalf("coverage = %+v", result)
	}
	if result.Recall.Total != 0 {
		t.Fatalf("a stale snapshot must not enter the rate: %+v", result.Recall)
	}
}

func TestSummarizeCountsAnEmptyShownSnapshotAsNoCandidates(t *testing.T) {
	rows := []decisionRow{{
		FileID:            "file-1",
		Decision:          "typed",
		DecidedText:       "جاسم",
		Candidates:        []candidateSnapshot{{Value: "علي", Key: "علي", Shown: false}},
		NormalizerVersion: normalizerVersion,
	}}

	result := summarize(rows, "")
	if result.WithSnapshot != 1 || result.NoCandidates.Matches != 1 || result.NoCandidates.Total != 1 {
		t.Fatalf("coverage = %+v", result)
	}
	if result.Recall.Total != 1 || result.Recall.Matches != 0 {
		t.Fatalf("recall = %+v", result.Recall)
	}
	if result.AllWrong.Total != 0 {
		t.Fatalf("nothing was shown, so there is no wrong shown candidate: %+v", result.AllWrong)
	}
}

func TestSummarizeReportsCandidateSourcesJudgeAbstentionAndCalibration(t *testing.T) {
	confidence := 0.9
	lowConfidence := 0.3
	rows := []decisionRow{
		{
			FileID: "file-1", Decision: "accepted", DecidedText: "محمد",
			Candidates: []candidateSnapshot{
				{Value: "محمد", Sources: []string{"reader", "common"}, Shown: true, Position: 1},
				{Value: "أحمد", Sources: []string{"ai"}, Shown: true, Position: 2},
			},
			InputMethod: "reader", NormalizerVersion: normalizerVersion,
			Judge: judgeOutput{
				Mode: "shadow", Provider: "jev", Choice: "محمد", Confidence: &confidence,
				Probabilities: map[string]float64{"محمد": 0.8, "أحمد": 0.15, "unclear": 0.05},
			},
		},
		{
			FileID: "file-1", Decision: "corrected", DecidedText: "جاسم",
			Candidates: []candidateSnapshot{
				{Value: "علي", Sources: []string{"ai"}, Shown: true, Position: 1},
			},
			InputMethod: "typed", NormalizerVersion: normalizerVersion,
			Judge: judgeOutput{
				Mode: "shadow", Provider: "jev", Choice: "unclear", Abstained: true, Confidence: &lowConfidence,
				Probabilities: map[string]float64{"علي": 0.3, "unclear": 0.7},
			},
		},
	}

	result := summarize(rows, "")
	if result.JudgeTotal != 2 || result.JudgeAbstain.Matches != 1 || result.JudgeAbstain.Total != 2 {
		t.Fatalf("judge total/abstain = %+v / %+v", result.JudgeTotal, result.JudgeAbstain)
	}
	if result.JudgeAbstention.TruthPresent.Matches != 0 || result.JudgeAbstention.TruthPresent.Total != 1 {
		t.Fatalf("truth present abstention = %+v", result.JudgeAbstention.TruthPresent)
	}
	if result.JudgeAbstention.TruthAbsent.Matches != 1 || result.JudgeAbstention.TruthAbsent.Total != 1 {
		t.Fatalf("truth absent abstention = %+v", result.JudgeAbstention.TruthAbsent)
	}
	if result.JudgeComparison.CandidatePresent != 1 || result.JudgeComparison.JudgeTop1.Matches != 1 ||
		result.JudgeComparison.BaselineTop1.Matches != 1 {
		t.Fatalf("judge comparison = %+v", result.JudgeComparison)
	}
	if result.JudgeComparison.Lift.Measurable || result.JudgeComparison.Lift.Difference != 0 {
		t.Fatalf("small lift = %+v", result.JudgeComparison.Lift)
	}
	if len(result.Calibration) != 1 {
		t.Fatalf("calibration = %+v", result.Calibration)
	}
	if result.Calibration[0].Bucket != "0.8-1.0" || result.Calibration[0].Correct != 1 {
		t.Fatalf("high calibration = %+v", result.Calibration[0])
	}

	bySource := make(map[string]sourceCoverageReport)
	for _, source := range result.SourceCoverage {
		bySource[source.Source] = source
	}
	if bySource["reader"].ContainsTruth.Matches != 1 || bySource["reader"].Offered.Matches != 1 {
		t.Fatalf("reader coverage = %+v", bySource["reader"])
	}
	if bySource["reader"].JudgeTop1.Matches != 1 || bySource["reader"].JudgeTop1.Total != 1 ||
		bySource["reader"].BaselineTop1.Matches != 1 || bySource["reader"].JudgeAbstain.Matches != 0 {
		t.Fatalf("reader judge subset = %+v", bySource["reader"])
	}
	if bySource["ai"].Offered.Matches != 2 || bySource["ai"].ContainsTruth.Matches != 0 {
		t.Fatalf("ai coverage = %+v", bySource["ai"])
	}
	if bySource["common"].ContainsTruth.Matches != 1 || bySource["common"].JudgeTop1.Matches != 1 || bySource["file"].Offered.Matches != 0 {
		t.Fatalf("common/file coverage = %+v / %+v", bySource["common"], bySource["file"])
	}
}

func TestSummarizeBuildsTheRequiredBreakdowns(t *testing.T) {
	rows := []decisionRow{
		{
			FileID: "file-1", Decision: "accepted", DecidedText: "محمد",
			Candidates:        []candidateSnapshot{{Value: "محمد", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion, PageNumber: 2,
			ReaderConfidence: 0.9, DecisionAt: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC),
			MimeType: "image/png", PageWidth: 1200, PageHeight: 900,
			SourceClass: "clean_print",
		},
		{
			FileID: "file-2", Decision: "corrected", DecidedText: "جاسم",
			Candidates:        []candidateSnapshot{{Value: "علي", Sources: []string{"ai"}, Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion, PageNumber: 1,
			ReaderConfidence: 0.3, DecisionAt: time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC),
			MimeType: "application/pdf", PageWidth: 600, PageHeight: 800,
			SourceClass: "low_resolution_screenshot",
		},
	}
	result := summarizeWithClassMap(rows, "", true)
	dimensions := make(map[string]breakdownReport)
	for _, breakdown := range result.Breakdowns {
		dimensions[breakdown.Dimension] = breakdown
	}
	for _, name := range []string{"fileClass", "resolution", "page", "readerConfidence", "date"} {
		if len(dimensions[name].Groups) == 0 {
			t.Fatalf("missing breakdown %q: %+v", name, result.Breakdowns)
		}
	}
	if dimensions["fileClass"].Groups[0].Group != "image" || dimensions["resolution"].Groups[0].Group != "medium" {
		t.Fatalf("file class/resolution = %+v / %+v", dimensions["fileClass"].Groups, dimensions["resolution"].Groups)
	}
	if dimensions["readerConfidence"].Groups[0].Group != "high" || dimensions["page"].Groups[0].Group != "page 1" {
		t.Fatalf("confidence/page = %+v / %+v", dimensions["readerConfidence"].Groups, dimensions["page"].Groups)
	}
}

func TestShadowGateNeedsThreeHundredDecisionsAndAllExpertClasses(t *testing.T) {
	makeRows := func(class string, count int) []decisionRow {
		rows := make([]decisionRow, 0, count)
		confidence := 0.8
		for index := 0; index < count; index++ {
			rows = append(rows, decisionRow{
				FileID: class, Decision: "accepted", DecidedText: "محمد",
				Candidates:        []candidateSnapshot{{Value: "محمد", Shown: true, Position: 1}},
				NormalizerVersion: normalizerVersion, SourceClass: class,
				DecisionAt: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC),
				Judge:      judgeOutput{Mode: "shadow", Provider: "jev", Choice: "محمد", Confidence: &confidence},
			})
		}
		return rows
	}
	rows := append(append(makeRows("clean_print", 100), makeRows("calligraphic_dense", 100)...), makeRows("low_resolution_screenshot", 100)...)
	result := summarizeWithClassMap(rows, "", true)
	if result.ShadowGate.JudgedWithTruth != 300 || !result.ShadowGate.Ready {
		t.Fatalf("gate = %+v", result.ShadowGate)
	}
	withoutClasses := summarize(rows, "")
	if withoutClasses.ShadowGate.Ready {
		t.Fatalf("a gate without human classes must not be ready: %+v", withoutClasses.ShadowGate)
	}
	short := summarizeWithClassMap(rows[:299], "", true)
	if short.ShadowGate.Ready {
		t.Fatalf("299 decisions must not pass: %+v", short.ShadowGate)
	}
	for _, invalid := range []struct {
		name   string
		change func(*decisionRow)
	}{
		{"active", func(row *decisionRow) { row.Judge.Mode = "active" }},
		{"shown", func(row *decisionRow) { row.Judge.Shown = true }},
		{"used", func(row *decisionRow) { row.Judge.Used = true }},
		{"unregistered", func(row *decisionRow) { row.SourceClass = "" }},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			changed := append([]decisionRow(nil), rows...)
			invalid.change(&changed[0])
			gate := summarizeWithClassMap(changed, "", true).ShadowGate
			if gate.Ready || gate.JudgedWithTruth != 299 {
				t.Fatalf("invalid shadow decision counted: %+v", gate)
			}
		})
	}

	wrong := make([]decisionRow, 0, 300)
	for index := 0; index < 300; index++ {
		row := rows[index]
		row.Candidates = []candidateSnapshot{{Value: "خالد", Shown: true, Position: 1}}
		wrong = append(wrong, row)
	}
	lowRecall := summarizeWithClassMap(wrong, "", true)
	if lowRecall.ShadowGate.Ready {
		t.Fatalf("a candidate pool that misses the truth must block the judge: %+v", lowRecall.ShadowGate)
	}
}

func TestSummarizeCountsWhetherTheActiveJudgeRowWasUsed(t *testing.T) {
	confidence := 0.8
	rows := []decisionRow{
		{
			FileID: "file-1", Decision: "accepted", DecidedText: "محمد",
			Candidates:        []candidateSnapshot{{Value: "محمد", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion,
			Judge:             judgeOutput{Mode: "active", Provider: "jev", Shown: true, Used: true, Choice: "محمد", Confidence: &confidence},
		},
		{
			FileID: "file-1", Decision: "accepted", DecidedText: "محمد",
			Candidates:        []candidateSnapshot{{Value: "محمد", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion,
			Judge:             judgeOutput{Mode: "active", Provider: "jev", Shown: true, Used: false, Choice: "محمد", Confidence: &confidence},
		},
		{
			FileID: "file-1", Decision: "accepted", DecidedText: "محمد",
			Candidates:        []candidateSnapshot{{Value: "محمد", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion,
			Judge:             judgeOutput{Mode: "shadow", Provider: "jev", Shown: false, Choice: "محمد", Confidence: &confidence},
		},
	}
	result := summarize(rows, "")
	if result.JudgeActiveShown != 2 || result.JudgeUsed != 1 || result.JudgeUseRate.Total != 2 || result.JudgeUseRate.Matches != 1 {
		t.Fatalf("judge use = shown %d used %d rate %+v", result.JudgeActiveShown, result.JudgeUsed, result.JudgeUseRate)
	}
}

func TestJudgeTop3UsesProbabilityOrderAndIgnoresUnclear(t *testing.T) {
	truth := keysFor("جاسم")
	if judgeTop3Contains(map[string]float64{"جاسم": 0.1, "علي": 0.5, "خالد": 0.2, "سالم": 0.15, "unclear": 0.05}, truth) {
		t.Fatal("a fourth-ranked candidate must not count as top-3")
	}
	if !judgeTop3Contains(map[string]float64{"جاسم": 0.3, "علي": 0.6, "unclear": 0.1}, truth) {
		t.Fatal("a third-ranked candidate should count as top-3")
	}
}

func TestPairedBootstrapIsDeterministic(t *testing.T) {
	pairs := []pair{
		{JudgeCorrect: true, BaselineCorrect: false},
		{JudgeCorrect: true, BaselineCorrect: true},
		{JudgeCorrect: false, BaselineCorrect: false},
	}
	lowerA, upperA, differenceA := pairedBootstrap(pairs, 500)
	lowerB, upperB, differenceB := pairedBootstrap(pairs, 500)
	if lowerA != lowerB || upperA != upperB || differenceA != differenceB {
		t.Fatalf("bootstrap is not deterministic: %v/%v/%v vs %v/%v/%v", lowerA, upperA, differenceA, lowerB, upperB, differenceB)
	}
	if differenceA != 1.0/3.0 {
		t.Fatalf("difference = %v", differenceA)
	}
}

func TestShownCandidatesMeasuresOnlyTheFirstEightForRecallAt8(t *testing.T) {
	candidates := make([]candidateSnapshot, 0, 10)
	for index := 1; index <= 10; index++ {
		candidates = append(candidates, candidateSnapshot{
			Value:    "candidate",
			Key:      "candidate",
			Shown:    true,
			Position: index,
		})
	}
	if got := len(shownCandidates(candidates, 8)); got != 8 {
		t.Fatalf("shown @8 = %d", got)
	}
	if got := len(shownCandidates(candidates, 0)); got != 10 {
		t.Fatalf("shown any = %d", got)
	}
}

func TestMixedVersionsCannotJointlySatisfyShadowGate(t *testing.T) {
	for _, dimension := range []string{"pool", "reader", "model", "prompt"} {
		t.Run(dimension, func(t *testing.T) {
			rows := make([]decisionRow, 300)
			confidence := 0.8
			for index := range rows {
				rows[index] = decisionRow{FileID: expertSourceClasses[index%3], SourceClass: expertSourceClasses[index%3], Decision: "accepted", DecidedText: "محمد", Candidates: []candidateSnapshot{{Value: "محمد", Shown: true, Position: 1}}, NormalizerVersion: normalizerVersion, PoolVersion: "pool-v1", ReaderVersion: "reader-v1", Judge: judgeOutput{Mode: "shadow", Provider: "jev", Model: "judge-v1", PromptVersion: "prompt-v1", Choice: "محمد", Confidence: &confidence}}
			}
			if !summarizeWithClassMap(rows, "", true).ShadowGate.Ready {
				t.Fatal("single coherent fixture must qualify")
			}
			for index := 150; index < len(rows); index++ {
				switch dimension {
				case "pool":
					rows[index].PoolVersion = "pool-v2"
				case "reader":
					rows[index].ReaderVersion = "reader-v2"
				case "model":
					rows[index].Judge.Model = "judge-v2"
				case "prompt":
					rows[index].Judge.PromptVersion = "prompt-v2"
				}
			}
			mixed := summarizeWithClassMap(rows, "", true)
			if mixed.ShadowGate.Ready || !mixed.MixedVersions || len(mixed.Cohorts) != 2 {
				t.Fatalf("mixed cohorts passed: %+v", mixed.ShadowGate)
			}
			for _, cohort := range mixed.Cohorts {
				if cohort.Decisions != 150 || cohort.ShadowGate.Ready {
					t.Fatalf("cohort incorrectly combined: %+v", cohort.ShadowGate)
				}
			}
			selected := selectCohort(rows, cohortVersion{Pool: "pool-v1", Reader: "reader-v1", Model: "judge-v1", Prompt: "prompt-v1"})
			if len(selected) != 150 || summarizeWithClassMap(selected, "", true).MixedVersions {
				t.Fatal("explicit selection did not isolate saved evidence")
			}
		})
	}
}
func TestMissingVersionRemainsLegacy(t *testing.T) {
	rows := []decisionRow{{}, {PoolVersion: "new"}}
	selected := selectCohort(rows, cohortVersion{Pool: "legacy"})
	if len(selected) != 1 || selected[0].PoolVersion != "" {
		t.Fatal("legacy row was silently versioned")
	}
}

func TestTruthPolicyCountsOneOccurrenceAcrossReopenedRevisions(t *testing.T) {
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	rows := []decisionRow{
		{
			ID: "decision-1", SuggestionID: "occurrence-1", ReviewRevision: 1,
			FileID: "file-1", Decision: "accepted", DecidedText: "محمد",
			Candidates:        []candidateSnapshot{{Value: "محمد", Key: "محمد", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion, DecisionAt: base,
		},
		{
			ID: "decision-2", SuggestionID: "occurrence-1", ReviewRevision: 2,
			FileID: "file-1", Decision: "corrected", DecidedText: "أحمد",
			Candidates:        []candidateSnapshot{{Value: "خالد", Key: "خالد", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion, DecisionAt: base.Add(time.Hour),
		},
		{
			ID: "decision-3", SuggestionID: "occurrence-2", ReviewRevision: 1,
			FileID: "file-1", Decision: "accepted", DecidedText: "علي",
			Candidates:        []candidateSnapshot{{Value: "علي", Key: "علي", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion, DecisionAt: base.Add(2 * time.Hour),
		},
	}
	first := summarizeWithTruth(rows, "", false, "first", "")
	if first.Decisions != 2 || first.CollapsedRevisions != 1 || first.DuplicateObservations != 0 {
		t.Fatalf("first policy invented observations: %+v", first)
	}
	if first.Top1.Matches != 2 {
		t.Fatalf("first policy did not keep the original answers: %+v", first.Top1)
	}
	cutoff := base.Add(3 * time.Hour).Format(time.RFC3339)
	latest := summarizeWithTruth(rows, "", false, "latest", cutoff)
	if latest.Decisions != 2 || latest.CollapsedRevisions != 1 || latest.Cutoff != cutoff {
		t.Fatalf("latest policy invented observations: %+v", latest)
	}
	// The reopened occurrence contributes its latest answer (أحمد, absent from
	// the revision's candidates), not the superseded first answer. The second
	// occurrence still matches once, so a match count of two would mean the
	// first answer was counted.
	if latest.Top1.Matches != 1 {
		t.Fatalf("latest policy did not use the revision selected at the cutoff: %+v", latest.Top1)
	}
}

func TestSupersededGeometryIsReportedButNeverCounted(t *testing.T) {
	rows := []decisionRow{
		{
			ID: "decision-1", SuggestionID: "parent", ReviewRevision: 1, Superseded: true,
			FileID: "file-1", Decision: "accepted", DecidedText: "محمد",
			Candidates:        []candidateSnapshot{{Value: "محمد", Key: "محمد", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion,
		},
		{
			ID: "decision-2", SuggestionID: "child", ReviewRevision: 1,
			FileID: "file-1", Decision: "corrected", DecidedText: "أحمد",
			Candidates:        []candidateSnapshot{{Value: "خالد", Key: "خالد", Shown: true, Position: 1}},
			NormalizerVersion: normalizerVersion,
		},
	}
	result := summarizeWithTruth(rows, "", false, "first", "")
	if result.Decisions != 1 || result.SupersededDecisions != 1 {
		t.Fatalf("superseded parent counted as an observation: %+v", result)
	}
}
