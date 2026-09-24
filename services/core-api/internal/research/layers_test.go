package research

import "testing"

func TestEvidencePackageValidatesEveryLayer(t *testing.T) {
	packageValue := EvidencePackage{
		SourceStatements: []Reference{{Layer: SourceStatement, Type: "source_statement", ID: "s1"}},
		ResearchClaims:   []Reference{{Layer: ResearchClaim, Type: "claim", ID: "c1"}},
		OpenQuestions:    []Reference{{Layer: OpenQuestion, Type: "open_question", ID: "q1"}},
	}

	if err := packageValue.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestFindingDoesNotBecomeInterpretationByIdentity(t *testing.T) {
	packageValue := EvidencePackage{
		PlatformFindings:    []Reference{{Layer: PlatformFinding, Type: "platform_finding", ID: "f1"}},
		TreeInterpretations: []Reference{{Layer: TreeInterpretation, Type: "tree_interpretation", ID: "f1"}},
	}

	if !packageValue.FindingIsInterpretation() {
		t.Fatal("expected a same-id finding to be detected as a layer collision")
	}
}
