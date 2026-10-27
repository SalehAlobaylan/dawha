package sourceprocessing

import (
	"sort"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
)

// Measuring a reader without an answer key
//
// There is no gold transcription of a family chart: the chart is the authority,
// and a second transcription does not exist. What does exist is a second
// RENDERING of the same facts - the drawn chart and the photograph of the same
// chart. The two pages look different, but the names on them are the same
// names. So the measurable quantity is AGREEMENT: read both renderings
// independently, pair the readings by name, and count what only one side saw.
//
// Agreement is not accuracy. Two renderings read by the same reader can agree on
// the same mistake. It is a lower bound on consistency, and it is the only
// name-level signal the corpus can give without a human answer key. Reviewer
// corrections are the other signal, and they are recorded in the decision
// ledger rather than here.
//
// The earlier localiser check compared COUNTS across renderings. Counts agree
// while the names differ, so this compares the names themselves.

// CarrierTextAgreement is the result of reading two renderings of the same
// chart and pairing what was read.
type CarrierTextAgreement struct {
	LeftReadings  int `json:"left_readings"`
	RightReadings int `json:"right_readings"`
	Matched       int `json:"matched"`
	ExactMatched  int `json:"exact_matched"`
	FuzzyMatched  int `json:"fuzzy_matched"`
	// Agreement is the Dice coefficient over paired readings:
	// 2*matched / (left + right). It treats a name missed on either side as a
	// disagreement, because a reader that returns different names from the same
	// facts is not consistent even if it returned the right number of them.
	Agreement float64 `json:"agreement"`
	// LeftOnly and RightOnly are the unpaired readings, in the order they were
	// read. They are the evidence a person checks.
	LeftOnly  []string `json:"left_only,omitempty"`
	RightOnly []string `json:"right_only,omitempty"`
}

// CompareCarrierTexts pairs the names read from two renderings of the same
// chart. minimumSimilarity is the trigram similarity at which two spellings are
// treated as the same name; exact normalised equality is always a match. A
// threshold of zero means exact matches only.
func CompareCarrierTexts(left, right []string, minimumSimilarity float64) CarrierTextAgreement {
	leftReadings := usableCarrierTexts(left)
	rightReadings := usableCarrierTexts(right)
	agreement := CarrierTextAgreement{
		LeftReadings:  len(leftReadings),
		RightReadings: len(rightReadings),
	}
	if len(leftReadings) == 0 && len(rightReadings) == 0 {
		return agreement
	}
	type candidate struct {
		left, right int
		score       float64
		exact       bool
	}
	candidates := make([]candidate, 0)
	for leftIndex, leftText := range leftReadings {
		for rightIndex, rightText := range rightReadings {
			exact := leftText == rightText
			score := 0.0
			if exact {
				score = 1
			} else if minimumSimilarity > 0 {
				score = arabicNameSimilarity(leftText, rightText)
				if score < minimumSimilarity {
					continue
				}
			} else {
				continue
			}
			candidates = append(candidates, candidate{left: leftIndex, right: rightIndex, score: score, exact: exact})
		}
	}
	// Best pairs first, then stable by position. A greedy one-to-one pairing is
	// enough here: the alternative is a full assignment problem, and the names
	// on one chart are not dense enough for the choice to change the count.
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].left != candidates[j].left {
			return candidates[i].left < candidates[j].left
		}
		return candidates[i].right < candidates[j].right
	})
	usedLeft := make([]bool, len(leftReadings))
	usedRight := make([]bool, len(rightReadings))
	for _, pair := range candidates {
		if usedLeft[pair.left] || usedRight[pair.right] {
			continue
		}
		usedLeft[pair.left] = true
		usedRight[pair.right] = true
		agreement.Matched++
		if pair.exact {
			agreement.ExactMatched++
		} else {
			agreement.FuzzyMatched++
		}
	}
	for index, text := range leftReadings {
		if !usedLeft[index] {
			agreement.LeftOnly = append(agreement.LeftOnly, text)
		}
	}
	for index, text := range rightReadings {
		if !usedRight[index] {
			agreement.RightOnly = append(agreement.RightOnly, text)
		}
	}
	agreement.Agreement = float64(2*agreement.Matched) / float64(len(leftReadings)+len(rightReadings))
	return agreement
}

// usableCarrierTexts normalises the readings and drops the ones that carry no
// name. A status of "unreadable" produces an empty text and is not a reading.
func usableCarrierTexts(readings []string) []string {
	usable := make([]string, 0, len(readings))
	for _, reading := range readings {
		normalized := identity.NormalizeArabicName(reading)
		if normalized == "" {
			continue
		}
		usable = append(usable, normalized)
	}
	return usable
}

// arabicNameSimilarity is the Jaccard similarity over character trigrams of the
// normalised names. Trigrams tolerate the differences OCR actually makes - a
// split word, one wrong letter, a dropped diacritic that survived
// normalisation - while keeping two genuinely different names apart.
func arabicNameSimilarity(left, right string) float64 {
	leftGrams := nameTrigrams(left)
	rightGrams := nameTrigrams(right)
	if len(leftGrams) == 0 || len(rightGrams) == 0 {
		return 0
	}
	intersection := 0
	for gram, count := range leftGrams {
		if other, ok := rightGrams[gram]; ok {
			if count < other {
				intersection += count
			} else {
				intersection += other
			}
		}
	}
	union := 0
	for _, count := range leftGrams {
		union += count
	}
	for _, count := range rightGrams {
		union += count
	}
	union -= intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func nameTrigrams(value string) map[string]int {
	// Spaces are removed before the trigrams are built: an OCR reading can
	// split a word that the other rendering joined, and that is a difference in
	// spacing, not in the name. Diacritics are already gone by normalisation.
	runes := []rune(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	if len(runes) == 0 {
		return nil
	}
	grams := map[string]int{}
	if len(runes) < 3 {
		grams[string(runes)] = 1
		return grams
	}
	for index := 0; index+3 <= len(runes); index++ {
		grams[string(runes[index:index+3])]++
	}
	return grams
}
