package research

import "errors"

type Layer string

const (
	SourceStatement    Layer = "source_statement"
	ResearchClaim      Layer = "research_claim"
	TreeInterpretation Layer = "tree_interpretation"
	PlatformFinding    Layer = "platform_finding"
	OpenQuestion       Layer = "open_question"
)

type Reference struct {
	Layer Layer
	Type  string
	ID    string
}

type EvidencePackage struct {
	SourceStatements    []Reference
	ResearchClaims      []Reference
	TreeInterpretations []Reference
	PlatformFindings    []Reference
	OpenQuestions       []Reference
}

func (p EvidencePackage) Validate() error {
	all := [][]Reference{p.SourceStatements, p.ResearchClaims, p.TreeInterpretations, p.PlatformFindings, p.OpenQuestions}
	for _, group := range all {
		for _, reference := range group {
			if reference.Type == "" || reference.ID == "" || reference.Layer == "" {
				return errors.New("research references must preserve layer, type, and id")
			}
		}
	}
	return nil
}

func (p EvidencePackage) FindingIsInterpretation() bool {
	for _, finding := range p.PlatformFindings {
		for _, interpretation := range p.TreeInterpretations {
			if finding.ID == interpretation.ID {
				return true
			}
		}
	}
	return false
}
