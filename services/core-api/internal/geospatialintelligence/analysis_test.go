package geospatialintelligence

import "testing"

func TestResolveMentionsKeepsAmbiguousHistoricalNamesUnresolved(t *testing.T) {
	places := []placeRecord{
		{ID: "place-1", Name: "الرياض", Normalized: "الرياض", PlaceType: "city"},
		{ID: "place-2", Name: "الرياض", Normalized: "الرياض", PlaceType: "region"},
	}
	names := []historicalNameRecord{{PlaceID: "place-1", Name: "رياض قديم", Normalized: "رياض قديم", NameType: "historical"}}
	input := analysisInput{Statements: []statementRecord{{ID: "statement-1", SourceID: "source-1", Text: "ذكر الرياض ثم رياض قديم"}}}
	mentions, ambiguous, sources := resolveMentions(input, buildPlaceIndex(places, names))
	if len(mentions) != 2 || len(ambiguous) != 1 || len(sources) != 1 {
		t.Fatalf("unexpected mention resolution: mentions=%d ambiguous=%d sources=%d", len(mentions), len(ambiguous), len(sources))
	}
	if mentions[0].Resolution != "unresolved" || mentions[0].PlaceID != "" || len(mentions[0].CandidateIDs) != 2 {
		t.Fatalf("ambiguous name was resolved: %+v", mentions[0])
	}
	if sources[0].UnresolvedCount != 1 {
		t.Fatalf("unexpected unresolved source count: %+v", sources[0])
	}
}

func TestBuildClustersUsesSpatialEdgesAndSeparatesLayers(t *testing.T) {
	input := analysisInput{
		Associations: []associationRecord{
			{ID: "association-1", EntityID: "person-1", PlaceID: "place-1", PlaceName: "الرياض", SourceID: "source-1", EvidenceID: "evidence-1", EvidenceStatus: "accepted", Status: "documented", Latitude: float64Pointer(24.7), Longitude: float64Pointer(46.7)},
			{ID: "association-2", EntityID: "person-1", PlaceID: "place-2", PlaceName: "الأحساء", SourceID: "source-1", EvidenceID: "evidence-2", EvidenceStatus: "needs_review", Status: "interpreted", Latitude: float64Pointer(25.3), Longitude: float64Pointer(49.5)},
		},
		SpatialEdges: []spatialEdge{{FirstID: "association-1", SecondID: "association-2", Distance: 100}},
		RadiusKM:     250,
	}
	clusters := buildClusters(input)
	if len(clusters) != 1 || clusters[0].Layer != "platform_inferred" || clusters[0].Status != "platform_hypothesis" || clusters[0].SourceBackedCount != 1 || clusters[0].InferredCount != 1 {
		t.Fatalf("unexpected cluster: %+v", clusters)
	}
}

func TestBuildMigrationHypothesisAndContradictionAreLabeled(t *testing.T) {
	input := analysisInput{
		Scope: Scope{EntityType: "person"},
		Associations: []associationRecord{
			{ID: "association-1", EntityID: "person-1", EntityName: "الشخص", PlaceID: "place-1", PlaceName: "الرياض", TimeFrom: "1200-01-01", TimeTo: "1205-01-01", SourceID: "source-1", ClaimID: "claim-1", Status: "documented", EvidenceStatus: "accepted", Latitude: float64Pointer(24.7), Longitude: float64Pointer(46.7)},
			{ID: "association-2", EntityID: "person-1", EntityName: "الشخص", PlaceID: "place-2", PlaceName: "العلا", TimeFrom: "1200-01-01", TimeTo: "1205-01-01", SourceID: "source-2", ClaimID: "claim-2", Status: "interpreted", EvidenceStatus: "accepted", Latitude: float64Pointer(26.6), Longitude: float64Pointer(38.1)},
		},
		RadiusKM: 250,
	}
	hypotheses := buildMigrationHypotheses(input)
	if len(hypotheses) != 1 || hypotheses[0].Status != "platform_hypothesis" || len(hypotheses[0].Sequence) != 2 {
		t.Fatalf("unexpected migration hypothesis: %+v", hypotheses)
	}
	findings := buildContradictions(input)
	if len(findings) != 1 || findings[0].Type != FindingTypeConflict || len(findings[0].PlaceIDs) != 2 {
		t.Fatalf("unexpected geographic finding: %+v", findings)
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}
