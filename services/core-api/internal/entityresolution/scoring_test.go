package entityresolution

import (
	"context"
	"testing"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func testSnapshot(id uuid.UUID, name string) *entitySnapshot {
	return newEntitySnapshot(id, EntityPerson, name, normalizeForTest(name))
}

func normalizeForTest(value string) string {
	return identity.NormalizeArabicName(value)
}

func TestStringSimilarityNormalizesArabicNames(t *testing.T) {
	if value := stringSimilarity("عَبْدُ الله", "عبدالله"); value < 0.8 {
		t.Fatalf("expected normalized names to match, got %f", value)
	}
	if value := stringSimilarity("محمد بن سعد", "سعد بن عامر"); value >= 0.8 {
		t.Fatalf("expected different names to remain distinct, got %f", value)
	}
}

func TestSnapshotBlocksIncludeDeterministicSignals(t *testing.T) {
	snapshot := testSnapshot(uuid.New(), "عبدالله بن محمد")
	snapshot.Aliases = []string{"عبد الله"}
	snapshot.Fathers = set("father-1")
	snapshot.Grandfathers = set("grandfather-1")
	snapshot.Places = set("place-1")
	snapshot.BirthFrom = dateForTest(1120)
	blocks := snapshotBlocks(snapshot)
	seen := make(map[string]bool)
	for _, block := range blocks {
		seen[block.Kind+"|"+block.Key] = true
	}
	for _, key := range []string{"name|" + snapshot.Normalized, "alias|عبد الله", "father|father-1", "grandfather|grandfather-1", "place|place-1", "date|birth:1120"} {
		if !seen[key] {
			t.Fatalf("missing block %q in %+v", key, blocks)
		}
	}
}

func TestGenerateCandidatesCanonicalizesPairs(t *testing.T) {
	leftID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	rightID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	left := testSnapshot(leftID, "عبدالله بن محمد")
	right := testSnapshot(rightID, "عبدالله بن محمد")
	candidates, blocks, model := (&Service{}).generateCandidates(context.Background(), []*entitySnapshot{left, right})
	if len(candidates) != 1 || len(blocks) != 2 || model != "local-character-v1" {
		t.Fatalf("unexpected generation result: candidates=%+v blocks=%+v model=%q", candidates, blocks, model)
	}
	if candidates[0].Left.ID != rightID || candidates[0].Right.ID != leftID {
		t.Fatalf("pair was not canonicalized: %+v", candidates[0])
	}
}

func TestScorePairUsesExplainableSignals(t *testing.T) {
	left := testSnapshot(uuid.New(), "عبدالله بن محمد")
	right := testSnapshot(uuid.New(), "عبدالله بن محمد")
	left.Fathers = set("father-1")
	right.Fathers = set("father-1")
	left.Places = set("place-1")
	right.Places = set("place-1")
	left.BirthFrom = dateForTest(1120)
	left.BirthTo = dateForTest(1140)
	right.BirthFrom = dateForTest(1125)
	right.BirthTo = dateForTest(1145)
	candidate := scorePair(context.Background(), &Service{}, left, right, &embeddingCache{values: map[string][]float64{}})
	if candidate.MatchClass != StrongCandidate {
		t.Fatalf("expected strong candidate, got %s with score %f", candidate.MatchClass, candidate.Score)
	}
	if candidate.ScoreComponents["relationship"] != 1 || candidate.ScoreComponents["geography"] != 1 {
		t.Fatalf("expected matching relationship and geography components: %+v", candidate.ScoreComponents)
	}
	if len(candidate.MatchingSignals) < 3 || candidate.ExplanationAR == "" {
		t.Fatalf("expected explainable signals, got %+v", candidate)
	}
}

func TestScorePairCapsExplicitConflicts(t *testing.T) {
	left := testSnapshot(uuid.New(), "عبدالله بن محمد")
	right := testSnapshot(uuid.New(), "عبدالله بن محمد")
	left.Gender = "male"
	right.Gender = "female"
	left.BirthFrom = dateForTest(1100)
	left.BirthTo = dateForTest(1110)
	right.BirthFrom = dateForTest(1200)
	right.BirthTo = dateForTest(1210)
	candidate := scorePair(context.Background(), &Service{}, left, right, &embeddingCache{values: map[string][]float64{}})
	if candidate.MatchClass != LikelyDifferent || candidate.Score >= 0.55 {
		t.Fatalf("expected explicit conflicts to remain different: %+v", candidate)
	}
}

func TestRelationshipDirectionMatchesTreeSemantics(t *testing.T) {
	parentID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	childID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	parent := testSnapshot(parentID, "محمد")
	child := testSnapshot(childID, "عبدالله")
	applyRelation(map[uuid.UUID]*entitySnapshot{parentID: parent, childID: child}, parentID, "parent_of", childID)
	if _, ok := child.Fathers[parentID.String()]; !ok {
		t.Fatalf("expected child father relationship: %+v", child.Fathers)
	}
	if _, ok := parent.Children[childID.String()]; !ok {
		t.Fatalf("expected parent child relationship: %+v", parent.Children)
	}
}

func TestReviewDecision(t *testing.T) {
	if decision, status, err := reviewDecision("approve"); err != nil || decision != "approve" || status != ReviewApproved {
		t.Fatalf("unexpected approve mapping: %q %q %v", decision, status, err)
	}
	if _, _, err := reviewDecision("unknown"); err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func dateForTest(year int) pgtype.Date {
	return pgtype.Date{Time: time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC), Valid: true}
}
