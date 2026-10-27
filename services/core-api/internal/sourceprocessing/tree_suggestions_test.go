package sourceprocessing

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestJudgeEvidenceKeepsOnlyBoundedNamesAndKnownFields(t *testing.T) {
	values := map[string]struct{}{"محمد": {}}
	valid := json.RawMessage(`{
		"mode":"shadow","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1",
		"choice":"محمد","probabilities":{"محمد":0.6,"اسم من خيار سابق":0.3,"unclear":0.1},
		"confidence":0.6,"abstained":false,"shown":false,"latencyMs":12
	}`)
	if err := validateJudgeOutput(valid, values); err != nil {
		t.Fatalf("a bounded name from the judge's option set was rejected: %v", err)
	}
	liquid := json.RawMessage(`{
		"mode":"shadow","provider":"liquid","model":"d1:3b","promptVersion":"name-judge-liquid-v1",
		"choice":"محمد","probabilities":{"محمد":0.6,"unclear":0.4},
		"confidence":0.6,"abstained":false,"shown":false,"latencyMs":12
	}`)
	if err := validateJudgeOutput(liquid, values); err != nil {
		t.Fatalf("the local Liquid provider was rejected: %v", err)
	}

	for name, raw := range map[string]json.RawMessage{
		"unknown provider":      json.RawMessage(`{"mode":"shadow","provider":"other","model":"d1:3b","promptVersion":"name-judge-liquid-v1","choice":"محمد","probabilities":{"محمد":0.6,"unclear":0.4},"confidence":0.6,"abstained":false,"shown":false,"latencyMs":12}`),
		"source id probability": json.RawMessage(`{"mode":"shadow","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1","choice":"محمد","probabilities":{"source_id_1":0.6,"unclear":0.4},"confidence":0.6,"abstained":false,"shown":false,"latencyMs":12}`),
		"unknown field":         json.RawMessage(`{"mode":"shadow","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1","choice":"محمد","confidence":0.6,"abstained":false,"shown":false,"latencyMs":12,"apiKey":"secret"}`),
		"shadow shown":          json.RawMessage(`{"mode":"shadow","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1","choice":"محمد","confidence":0.6,"abstained":false,"shown":true,"latencyMs":12}`),
		"active hidden":         json.RawMessage(`{"mode":"active","provider":"jev","model":"jev-pinned","promptVersion":"name-judge-v1","choice":"محمد","confidence":0.6,"abstained":false,"shown":false,"latencyMs":12}`),
	} {
		if err := validateJudgeOutput(raw, values); !errors.Is(err, ErrValidation) {
			t.Fatalf("%s = %v, want ErrValidation", name, err)
		}
	}
}

func TestNameHeightSummaryWarnsWhenTheSourceIsTooSmall(t *testing.T) {
	// The warning is the only accuracy signal a run can produce without an
	// answer key: it says whether the names were big enough in the original for
	// any reader to have a fair chance. It must not fire on a page of readable
	// names, and it must not be silent on a screenshot where every name is a
	// few pixels tall.
	cases := []struct {
		name    string
		heights []int
		median  int
		small   bool
	}{
		{name: "no proposals is not a verdict", heights: nil, median: 0, small: false},
		{name: "one tiny name", heights: []int{6}, median: 6, small: true},
		{name: "one readable name", heights: []int{36}, median: 36, small: false},
		{name: "a screenshot of tiny names", heights: []int{5, 6, 7, 8, 9}, median: 7, small: true},
		{name: "source-resolution chart names", heights: []int{30, 34, 36, 38, 42}, median: 36, small: false},
		{name: "one large name cannot hide a page of small ones", heights: []int{5, 6, 7, 60}, median: 7, small: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			median, small := summarizeNameHeights(testCase.heights)
			if median != testCase.median || small != testCase.small {
				t.Fatalf("summarizeNameHeights(%v) = (%d, %t), want (%d, %t); the warning must follow the typical name, not the largest one", testCase.heights, median, small, testCase.median, testCase.small)
			}
		})
	}
}
