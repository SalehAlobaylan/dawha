package geospatialintelligence

import (
	"math"
	"sort"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
)

type analysisInput struct {
	Scope          Scope
	Places         []placeRecord
	Names          []historicalNameRecord
	Statements     []statementRecord
	Sources        map[string]sourceRecord
	Associations   []associationRecord
	Migrations     []migrationRecord
	SpatialEdges   []spatialEdge
	RadiusKM       float64
	MaximumRecords int
}

type findingDraft struct {
	Type          string
	TitleAR       string
	ExplanationAR string
	Severity      string
	EntityIDs     []string
	PlaceIDs      []string
	SourceIDs     []string
	ClaimIDs      []string
	Signals       map[string]any
}

type placeMatch struct {
	Place      placeRecord
	Historical bool
	ValidFrom  string
	ValidTo    string
}

func analyze(input analysisInput) (Report, []findingDraft) {
	report := Report{
		PlaceResolution:          PlaceResolution{Mentions: []PlaceMention{}},
		Disambiguation:           []DisambiguationCandidate{},
		Clusters:                 []SpatialCluster{},
		MigrationHypotheses:      []MigrationHypothesis{},
		GeographicContradictions: []GeographicContradiction{},
		SourceGeography:          []SourceGeography{},
		Limitations:              []string{"يقتصر التحليل على المصادر العامة المستقلة المقبولة."},
	}
	placeIndex := buildPlaceIndex(input.Places, input.Names)
	mentions, disambiguation, sourceGeography := resolveMentions(input, placeIndex)
	report.PlaceResolution.Mentions = mentions
	report.Disambiguation = disambiguation
	report.SourceGeography = sourceGeography
	report.PlaceResolution.MentionCount = len(mentions)
	for _, mention := range mentions {
		if mention.Resolution == "resolved" {
			report.PlaceResolution.ResolvedCount++
		}
	}
	for _, source := range sourceGeography {
		report.PlaceResolution.UnresolvedCount += source.UnresolvedCount
	}
	report.Clusters = buildClusters(input)
	report.MigrationHypotheses = buildMigrationHypotheses(input)
	findings := buildContradictions(input)
	for _, finding := range findings {
		report.GeographicContradictions = append(report.GeographicContradictions, GeographicContradiction{
			ID:            stableID("geographic-finding", finding.Type, strings.Join(finding.PlaceIDs, ","), strings.Join(finding.EntityIDs, ",")),
			Type:          finding.Type,
			Layer:         "platform_inferred",
			Status:        "needs_review",
			Severity:      finding.Severity,
			TitleAR:       finding.TitleAR,
			ExplanationAR: finding.ExplanationAR,
			EntityIDs:     finding.EntityIDs,
			PlaceIDs:      finding.PlaceIDs,
			SourceIDs:     finding.SourceIDs,
			ClaimIDs:      finding.ClaimIDs,
		})
	}
	if len(input.Places) == 0 {
		report.Limitations = append(report.Limitations, "لا توجد مواضع هندسية قابلة للمطابقة.")
	}
	if len(input.Statements) > 0 && len(mentions) >= MaximumReportItems {
		report.Limitations = append(report.Limitations, "تم تجاوز حد الإشارات المكانية المعروضة؛ قد تكون القائمة جزئية.")
	}
	if len(input.SpatialEdges) >= MaximumRecords {
		report.Limitations = append(report.Limitations, "تم تجاوز حد مسارات التشابه المكاني؛ قد تكون المجموعة غير مكتملة.")
	}
	if len(input.Statements) == 0 && len(input.Associations) == 0 && len(input.Migrations) == 0 {
		report.Limitations = append(report.Limitations, "لا توجد بيانات جغرافية مؤهلة كافية لإجراء تحليل.")
	}
	return report, findings
}

func buildPlaceIndex(places []placeRecord, names []historicalNameRecord) map[string][]placeMatch {
	index := make(map[string][]placeMatch)
	byID := make(map[string]placeRecord, len(places))
	for _, place := range places {
		byID[place.ID] = place
		addPlaceMatch(index, place.Normalized, place, false, "", "")
		addPlaceMatch(index, identity.NormalizeArabicName(place.Name), place, false, "", "")
	}
	for _, name := range names {
		place, found := byID[name.PlaceID]
		if !found {
			place = placeRecord{ID: name.PlaceID, Name: name.Name}
		}
		addPlaceMatch(index, name.Normalized, place, true, name.ValidFrom, name.ValidTo)
	}
	for key := range index {
		sort.SliceStable(index[key], func(left, right int) bool {
			if index[key][left].Place.ID == index[key][right].Place.ID {
				return index[key][left].Historical == false
			}
			return index[key][left].Place.ID < index[key][right].Place.ID
		})
	}
	return index
}

func addPlaceMatch(index map[string][]placeMatch, normalized string, place placeRecord, historical bool, validFrom, validTo string) {
	normalized = strings.TrimSpace(normalized)
	if normalized == "" || len([]rune(normalized)) < 4 {
		return
	}
	for _, existing := range index[normalized] {
		if existing.Place.ID == place.ID && existing.Historical == historical {
			return
		}
	}
	index[normalized] = append(index[normalized], placeMatch{Place: place, Historical: historical, ValidFrom: validFrom, ValidTo: validTo})
}

func resolveMentions(input analysisInput, index map[string][]placeMatch) ([]PlaceMention, []DisambiguationCandidate, []SourceGeography) {
	keys := make([]string, 0, len(index))
	for key := range index {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	mentions := make([]PlaceMention, 0)
	ambiguous := make([]DisambiguationCandidate, 0)
	sourceStats := make(map[string]*SourceGeography)
	for _, statement := range input.Statements {
		if len(mentions) >= MaximumReportItems {
			break
		}
		source := sourceStats[statement.SourceID]
		if source == nil {
			source = &SourceGeography{SourceID: statement.SourceID, SourceTitle: statement.SourceTitle, MatchedPlaceIDs: []string{}, PlaceNames: []string{}, DependencyStatus: "unknown", SourceLayer: "source_backed", QualificationNote: "المصدر عام ومستقل."}
			if record, found := input.Sources[statement.SourceID]; found {
				source.DependencyStatus = record.DependencyStatus
			}
			sourceStats[statement.SourceID] = source
		}
		source.StatementCount++
		normalizedText := identity.NormalizeArabicName(statement.Text)
		matchedKeys := make([]string, 0)
		for _, key := range keys {
			if strings.Contains(normalizedText, key) {
				matchedKeys = append(matchedKeys, key)
			}
		}
		if len(matchedKeys) == 0 {
			source.UnresolvedCount++
			continue
		}
		for _, key := range matchedKeys {
			if len(mentions) >= MaximumReportItems {
				break
			}
			candidates := uniquePlaceMatches(index[key])
			if len(candidates) == 0 {
				source.UnresolvedCount++
				continue
			}
			mention := PlaceMention{
				ID:             stableID("place-mention", statement.ID, key),
				Mention:        key,
				NormalizedName: key,
				SourceID:       statement.SourceID,
				SourceTitle:    statement.SourceTitle,
				StatementID:    statement.ID,
				PassageID:      statement.PassageID,
				Resolution:     "resolved",
				Reason:         "exact_normalized_match",
				SourceLayer:    "source_backed",
			}
			for _, candidate := range candidates {
				mention.CandidateIDs = append(mention.CandidateIDs, candidate.Place.ID)
				if candidate.Historical {
					mention.HistoricalName = true
					mention.ValidFrom = candidate.ValidFrom
					mention.ValidTo = candidate.ValidTo
				}
			}
			if len(candidates) == 1 {
				mention.PlaceID = candidates[0].Place.ID
				mention.PlaceName = candidates[0].Place.Name
				source.ResolvedMentions++
				source.MatchedPlaceIDs = appendUnique(source.MatchedPlaceIDs, mention.PlaceID)
				source.PlaceNames = appendUnique(source.PlaceNames, mention.PlaceName)
			} else {
				mention.Resolution = "unresolved"
				mention.Reason = "multiple_places_or_historical_names"
				source.UnresolvedCount++
				ambiguous = append(ambiguous, DisambiguationCandidate{
					ID:          stableID("place-disambiguation", statement.ID, key),
					Mention:     key,
					SourceID:    statement.SourceID,
					StatementID: statement.ID,
					Status:      "unresolved",
					Reason:      "multiple_places_or_historical_names",
					Candidates:  placeCandidates(candidates),
				})
			}
			mentions = append(mentions, mention)
		}
	}
	items := make([]SourceGeography, 0, len(sourceStats))
	for _, item := range sourceStats {
		sort.Strings(item.MatchedPlaceIDs)
		sort.Strings(item.PlaceNames)
		items = append(items, *item)
	}
	sort.Slice(items, func(left, right int) bool { return items[left].SourceID < items[right].SourceID })
	return mentions, ambiguous, items
}

func uniquePlaceMatches(values []placeMatch) []placeMatch {
	items := make([]placeMatch, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		if seen[value.Place.ID] {
			continue
		}
		seen[value.Place.ID] = true
		items = append(items, value)
	}
	return items
}

func placeCandidates(values []placeMatch) []PlaceCandidate {
	items := make([]PlaceCandidate, 0, len(values))
	for _, value := range values {
		items = append(items, PlaceCandidate{PlaceID: value.Place.ID, PlaceName: value.Place.Name, PlaceType: value.Place.PlaceType, ValidFrom: value.ValidFrom, ValidTo: value.ValidTo})
	}
	sort.Slice(items, func(left, right int) bool { return items[left].PlaceID < items[right].PlaceID })
	return items
}

func buildClusters(input analysisInput) []SpatialCluster {
	byID := make(map[string]associationRecord)
	for _, association := range input.Associations {
		if association.Latitude != nil && association.Longitude != nil {
			byID[association.ID] = association
		}
	}
	parent := make(map[string]string)
	var find func(string) string
	find = func(value string) string {
		if parent[value] == "" {
			parent[value] = value
			return value
		}
		if parent[value] != value {
			parent[value] = find(parent[value])
		}
		return parent[value]
	}
	union := func(first, second string) {
		firstRoot, secondRoot := find(first), find(second)
		if firstRoot != secondRoot {
			parent[secondRoot] = firstRoot
		}
	}
	for _, edge := range input.SpatialEdges {
		if _, firstOK := byID[edge.FirstID]; !firstOK {
			continue
		}
		if _, secondOK := byID[edge.SecondID]; !secondOK {
			continue
		}
		union(edge.FirstID, edge.SecondID)
	}
	components := make(map[string][]associationRecord)
	for id, association := range byID {
		root := find(id)
		components[root] = append(components[root], association)
	}
	items := make([]SpatialCluster, 0)
	for _, component := range components {
		placeIDs := make([]string, 0)
		for _, association := range component {
			placeIDs = appendUnique(placeIDs, association.PlaceID)
		}
		if len(component) < 2 || len(placeIDs) < 2 {
			continue
		}
		sort.Slice(component, func(left, right int) bool { return component[left].ID < component[right].ID })
		centerLatitude, centerLongitude := 0.0, 0.0
		for _, association := range component {
			centerLatitude += *association.Latitude
			centerLongitude += *association.Longitude
		}
		centerLatitude /= float64(len(component))
		centerLongitude /= float64(len(component))
		radius := 0.0
		sourceIDs := make([]string, 0)
		evidenceIDs := make([]string, 0)
		entityIDs := make([]string, 0)
		associationIDs := make([]string, 0)
		placeNames := make([]string, 0)
		sourceBackedCount, inferredCount := 0, 0
		for _, association := range component {
			radius = math.Max(radius, distanceKM(centerLatitude, centerLongitude, *association.Latitude, *association.Longitude))
			sourceIDs = appendUnique(sourceIDs, association.SourceID)
			evidenceIDs = appendUnique(evidenceIDs, association.EvidenceID)
			entityIDs = appendUnique(entityIDs, association.EntityID)
			associationIDs = appendUnique(associationIDs, association.ID)
			placeIDs = appendUnique(placeIDs, association.PlaceID)
			placeNames = appendUnique(placeNames, association.PlaceName)
			if (association.Status == "documented" || association.Status == "interpreted") && association.EvidenceStatus == "accepted" {
				sourceBackedCount++
			} else {
				inferredCount++
			}
		}
		items = append(items, SpatialCluster{ID: stableID("spatial-cluster", strings.Join(associationIDs, ",")), Layer: "platform_inferred", Status: "platform_hypothesis", PlaceIDs: placeIDs, PlaceNames: placeNames, AssociationIDs: associationIDs, EntityIDs: entityIDs, SourceIDs: sourceIDs, EvidenceIDs: evidenceIDs, CenterLatitude: centerLatitude, CenterLongitude: centerLongitude, RadiusKM: radius, SourceBackedCount: sourceBackedCount, InferredCount: inferredCount, ExplanationAR: "تجمع مكاني أولي بين الإشارات؛ يبقى استنتاجاً ولا يثبت حداً تاريخياً."})
	}
	sort.Slice(items, func(left, right int) bool { return items[left].ID < items[right].ID })
	if len(items) > MaximumReportItems {
		items = items[:MaximumReportItems]
	}
	return items
}

func buildMigrationHypotheses(input analysisInput) []MigrationHypothesis {
	items := make([]MigrationHypothesis, 0)
	seen := make(map[string]bool)
	for _, migration := range input.Migrations {
		if migration.FromPlaceID == "" || migration.ToPlaceID == "" {
			continue
		}
		key := migration.SubjectType + ":" + migration.SubjectID + ":" + migration.FromPlaceID + ":" + migration.ToPlaceID
		if seen[key] {
			continue
		}
		seen[key] = true
		layer := "platform_inferred"
		if (migration.Status == "documented" || migration.Status == "interpreted") && migration.EvidenceStatus == "accepted" {
			layer = "source_backed"
		}
		items = append(items, MigrationHypothesis{ID: stableID("migration", migration.ID), SubjectType: migration.SubjectType, SubjectID: migration.SubjectID, SubjectName: migration.SubjectName, Sequence: []PlaceReference{{PlaceID: migration.FromPlaceID, PlaceName: migration.FromPlaceName, Layer: layer}, {PlaceID: migration.ToPlaceID, PlaceName: migration.ToPlaceName, Layer: layer}}, SourceIDs: compactStrings(migration.SourceID), EvidenceIDs: compactStrings(migration.EvidenceID), ClaimIDs: compactStrings(migration.ClaimID), AssociationIDs: []string{}, TreeVersionID: input.Scope.TreeVersionID, Layer: layer, Status: "platform_hypothesis", ExplanationAR: "مسار ممكن مبني على إشارة جغرافية؛ لا يثبت مساراً تاريخياً نهائياً."})
	}
	if input.Scope.EntityType == "person" {
		byEntity := make(map[string][]associationRecord)
		for _, association := range input.Associations {
			if association.TimeFrom == "" || association.PlaceID == "" {
				continue
			}
			byEntity[association.EntityID] = append(byEntity[association.EntityID], association)
		}
		for entityID, associations := range byEntity {
			sort.Slice(associations, func(left, right int) bool { return associations[left].TimeFrom < associations[right].TimeFrom })
			sequence := make([]associationRecord, 0)
			for _, association := range associations {
				if len(sequence) == 0 || sequence[len(sequence)-1].PlaceID != association.PlaceID {
					sequence = append(sequence, association)
				}
			}
			if len(sequence) < 2 {
				continue
			}
			key := entityID + ":inferred:" + sequence[0].PlaceID + ":" + sequence[len(sequence)-1].PlaceID
			if seen[key] {
				continue
			}
			seen[key] = true
			refs := make([]PlaceReference, 0, len(sequence))
			sourceIDs, claimIDs, associationIDs := make([]string, 0), make([]string, 0), make([]string, 0)
			for _, association := range sequence {
				refs = append(refs, PlaceReference{PlaceID: association.PlaceID, PlaceName: association.PlaceName, Layer: "platform_inferred"})
				sourceIDs = appendUnique(sourceIDs, association.SourceID)
				claimIDs = appendUnique(claimIDs, association.ClaimID)
				associationIDs = appendUnique(associationIDs, association.ID)
			}
			items = append(items, MigrationHypothesis{ID: stableID("migration-inferred", key), SubjectType: "person", SubjectID: entityID, SubjectName: sequence[0].EntityName, Sequence: refs, SourceIDs: sourceIDs, EvidenceIDs: []string{}, ClaimIDs: claimIDs, AssociationIDs: associationIDs, TreeVersionID: input.Scope.TreeVersionID, Layer: "platform_inferred", Status: "platform_hypothesis", ExplanationAR: "تسلسل محتمل استُنتج من توالي حضورات موثقة؛ يبقى فرضية تحتاج مراجعة."})
		}
	}
	sort.Slice(items, func(left, right int) bool { return items[left].ID < items[right].ID })
	if len(items) > MaximumReportItems {
		items = items[:MaximumReportItems]
	}
	return items
}

func buildContradictions(input analysisInput) []findingDraft {
	findings := make([]findingDraft, 0)
	if input.Scope.EntityType != "person" {
		return findings
	}
	associations := input.Associations
	for left := 0; left < len(associations); left++ {
		for right := left + 1; right < len(associations); right++ {
			first, second := associations[left], associations[right]
			if first.EntityID != second.EntityID || first.PlaceID == second.PlaceID || first.Latitude == nil || first.Longitude == nil || second.Latitude == nil || second.Longitude == nil {
				continue
			}
			if !rangesOverlap(first.TimeFrom, first.TimeTo, second.TimeFrom, second.TimeTo) {
				continue
			}
			distance := distanceKM(*first.Latitude, *first.Longitude, *second.Latitude, *second.Longitude)
			if distance <= input.RadiusKM {
				continue
			}
			placeIDs := []string{first.PlaceID, second.PlaceID}
			sort.Strings(placeIDs)
			findings = append(findings, findingDraft{Type: FindingTypeConflict, TitleAR: "حضور جغرافي متعارض في فترات متداخلة", ExplanationAR: "تظهر إشارتان جغرافيتان متباعدتان لنفس الكيان في فترات متداخلة؛ يلزم فحص المصادر قبل الحكم بتعارض فعلي.", Severity: "medium", EntityIDs: []string{first.EntityID}, PlaceIDs: placeIDs, SourceIDs: compactStrings(first.SourceID, second.SourceID), ClaimIDs: compactStrings(first.ClaimID, second.ClaimID), Signals: map[string]any{"distanceKm": distance, "radiusKm": input.RadiusKM, "associationIds": []string{first.ID, second.ID}}})
		}
	}
	return uniqueFindings(findings)
}

func uniqueFindings(values []findingDraft) []findingDraft {
	items := make([]findingDraft, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		key := stableID("finding", value.Type, strings.Join(value.EntityIDs, ","), strings.Join(value.PlaceIDs, ","))
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, value)
	}
	if len(items) > MaximumReportItems {
		items = items[:MaximumReportItems]
	}
	return items
}

func rangesOverlap(firstFrom, firstTo, secondFrom, secondTo string) bool {
	if firstFrom == "" || firstTo == "" || secondFrom == "" || secondTo == "" {
		return true
	}
	return firstFrom <= secondTo && secondFrom <= firstTo
}

func distanceKM(firstLatitude, firstLongitude, secondLatitude, secondLongitude float64) float64 {
	const earthRadiusKM = 6371.0088
	firstLatitudeRadians := firstLatitude * math.Pi / 180
	secondLatitudeRadians := secondLatitude * math.Pi / 180
	latitudeDelta := (secondLatitude - firstLatitude) * math.Pi / 180
	longitudeDelta := (secondLongitude - firstLongitude) * math.Pi / 180
	value := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) + math.Cos(firstLatitudeRadians)*math.Cos(secondLatitudeRadians)*math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)
	return earthRadiusKM * 2 * math.Atan2(math.Sqrt(value), math.Sqrt(1-value))
}

func stableID(parts ...string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.Join(parts, "|"))).String()
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func compactStrings(values ...string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = appendUnique(items, value)
	}
	return items
}
