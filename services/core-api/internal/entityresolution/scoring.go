package entityresolution

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type entitySnapshot struct {
	ID           uuid.UUID
	EntityType   EntityType
	Name         string
	Normalized   string
	Aliases      []string
	Gender       string
	BirthFrom    pgtype.Date
	BirthTo      pgtype.Date
	DeathFrom    pgtype.Date
	DeathTo      pgtype.Date
	OriginPlace  string
	Fathers      map[string]struct{}
	Grandfathers map[string]struct{}
	Children     map[string]struct{}
	Siblings     map[string]struct{}
	Spouses      map[string]struct{}
	Places       map[string]struct{}
	Sources      map[string]struct{}
	Trees        map[string]struct{}
	Branches     map[string]struct{}
}

type blockKey struct {
	EntityType EntityType
	EntityID   string
	Kind       string
	Key        string
}

type generatedCandidate struct {
	Left               *entitySnapshot
	Right              *entitySnapshot
	MatchClass         MatchClass
	Score              float64
	ScoreComponents    map[string]float64
	MatchingSignals    []Signal
	ConflictingSignals []Signal
	ExplanationAR      string
}

type embeddingCache struct {
	values  map[string][]float64
	model   string
	aiCalls int
}

func newEntitySnapshot(id uuid.UUID, entityType EntityType, name, normalized string) *entitySnapshot {
	return &entitySnapshot{ID: id, EntityType: entityType, Name: name, Normalized: normalized, Fathers: set(), Grandfathers: set(), Children: set(), Siblings: set(), Spouses: set(), Places: set(), Sources: set(), Trees: set(), Branches: set()}
}

func set(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func addSet(target map[string]struct{}, values ...string) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			target[value] = struct{}{}
		}
	}
}

func setValues(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (s *Service) generateCandidates(ctx context.Context, snapshots []*entitySnapshot) ([]generatedCandidate, []blockKey, string) {
	blocks := make([]blockKey, 0)
	byBlock := make(map[string][]*entitySnapshot)
	seenBlocks := make(map[string]struct{})
	for _, snapshot := range snapshots {
		for _, block := range snapshotBlocks(snapshot) {
			block.EntityType = snapshot.EntityType
			block.EntityID = snapshot.ID.String()
			key := string(block.EntityType) + "|" + block.Kind + "|" + block.Key
			if _, exists := seenBlocks[key+"|"+block.EntityID]; !exists {
				blocks = append(blocks, block)
				seenBlocks[key+"|"+block.EntityID] = struct{}{}
			}
			byBlock[key] = append(byBlock[key], snapshot)
		}
	}
	pairs := make(map[string][2]*entitySnapshot)
	for _, members := range byBlock {
		if len(members) > 250 {
			members = members[:250]
		}
		for leftIndex := 0; leftIndex < len(members); leftIndex++ {
			for rightIndex := leftIndex + 1; rightIndex < len(members); rightIndex++ {
				left, right := members[leftIndex], members[rightIndex]
				if left.ID == right.ID || left.EntityType != right.EntityType {
					continue
				}
				if left.ID.String() > right.ID.String() {
					left, right = right, left
				}
				pairs[left.ID.String()+"|"+right.ID.String()] = [2]*entitySnapshot{left, right}
			}
		}
	}
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 1000 {
		keys = keys[:1000]
	}
	cache := &embeddingCache{values: make(map[string][]float64)}
	candidates := make([]generatedCandidate, 0, len(keys))
	for _, key := range keys {
		pair := pairs[key]
		candidate := scorePair(ctx, s, pair[0], pair[1], cache)
		candidates = append(candidates, candidate)
	}
	return candidates, blocks, cache.model
}

func snapshotBlocks(snapshot *entitySnapshot) []blockKey {
	blocks := make([]blockKey, 0)
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			blocks = append(blocks, blockKey{Kind: kind, Key: value})
		}
	}
	add("name", snapshot.Normalized)
	for _, alias := range snapshot.Aliases {
		add("alias", alias)
	}
	for _, value := range setValues(snapshot.Fathers) {
		add("father", value)
	}
	for _, value := range setValues(snapshot.Grandfathers) {
		add("grandfather", value)
	}
	for _, value := range setValues(snapshot.Places) {
		add("place", value)
	}
	for _, value := range setValues(snapshot.Trees) {
		add("tree", value)
	}
	for _, value := range setValues(snapshot.Branches) {
		add("branch", value)
	}
	for _, year := range dateBlockYears(snapshot.BirthFrom, snapshot.BirthTo) {
		add("date", "birth:"+itoa(year))
	}
	for _, year := range dateBlockYears(snapshot.DeathFrom, snapshot.DeathTo) {
		add("date", "death:"+itoa(year))
	}
	return uniqueBlocks(blocks)
}

func uniqueBlocks(values []blockKey) []blockKey {
	seen := make(map[string]struct{}, len(values))
	result := make([]blockKey, 0, len(values))
	for _, value := range values {
		key := value.Kind + "|" + value.Key
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func dateBlockYears(from, to pgtype.Date) []int {
	if !from.Valid && !to.Valid {
		return nil
	}
	start, end := from.Time.Year(), to.Time.Year()
	if !from.Valid {
		start = end
	}
	if !to.Valid {
		end = start
	}
	if start > end {
		start, end = end, start
	}
	first := (start / 10) * 10
	last := (end / 10) * 10
	if last-first > 30 {
		last = first + 30
	}
	result := make([]int, 0, (last-first)/10+1)
	for year := first; year <= last; year += 10 {
		result = append(result, year)
	}
	return result
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}

func scorePair(ctx context.Context, service *Service, left, right *entitySnapshot, cache *embeddingCache) generatedCandidate {
	nameScore, nameConflict := nameSimilarity(left, right)
	components := map[string]float64{"name": nameScore}
	matching := make([]Signal, 0, 6)
	conflicting := make([]Signal, 0, 4)
	if nameScore >= 0.72 {
		matching = append(matching, Signal{Kind: "name", Detail: "تشابه الاسم أو أحد الألقاب", Score: nameScore})
	} else if nameConflict {
		conflicting = append(conflicting, Signal{Kind: "name", Detail: "الاسم لا يتطابق بما يكفي", Score: nameScore})
	}

	embeddingScore := embeddingSimilarity(ctx, service, left, right, cache)
	if embeddingScore != nil {
		semanticScore := math.Max(0, math.Min(1, *embeddingScore))
		components["embedding"] = semanticScore
		if semanticScore >= 0.72 {
			matching = append(matching, Signal{Kind: "embedding", Detail: "تشابه دلالي في الاسم", Score: semanticScore})
		} else if *embeddingScore < 0.35 {
			conflicting = append(conflicting, Signal{Kind: "embedding", Detail: "التمثيل الدلالي لا يدل على تطابق", Score: semanticScore})
		}
	}

	relationshipScore, relationshipAvailable, relationshipConflict := relationshipSimilarity(left, right)
	if relationshipAvailable {
		components["relationship"] = relationshipScore
		if relationshipScore >= 0.45 {
			matching = append(matching, Signal{Kind: "relationship", Detail: "تداخل في القرابة أو الصلة", Score: relationshipScore})
		} else if relationshipConflict {
			conflicting = append(conflicting, Signal{Kind: "relationship", Detail: "علاقة قرابة متعارضة", Score: relationshipScore})
		}
	}

	geographyScore, geographyAvailable := setSimilarity(left.Places, right.Places)
	if geographyAvailable {
		components["geography"] = geographyScore
		if geographyScore > 0 {
			matching = append(matching, Signal{Kind: "geography", Detail: "تشابه في المواضع المعروفة", Score: geographyScore})
		}
	}

	chronologyScore, chronologyAvailable, chronologyConflict := chronologySimilarity(left, right)
	if chronologyAvailable {
		components["chronology"] = chronologyScore
		if chronologyScore >= 0.5 {
			matching = append(matching, Signal{Kind: "chronology", Detail: "توافق في النطاق الزمني", Score: chronologyScore})
		} else if chronologyConflict {
			conflicting = append(conflicting, Signal{Kind: "chronology", Detail: "النطاق الزمني غير متوافق", Score: chronologyScore})
		}
	}

	sourceScore, sourceAvailable := setSimilarity(left.Sources, right.Sources)
	if sourceAvailable {
		components["source_context"] = sourceScore
		if sourceScore > 0 {
			matching = append(matching, Signal{Kind: "source_context", Detail: "تشابه في سياق المصدر", Score: sourceScore})
		}
	}

	if left.Gender != "" && right.Gender != "" && left.Gender != right.Gender {
		conflicting = append(conflicting, Signal{Kind: "gender", Detail: "الصفات المسجلة غير متطابقة", Score: 0})
	}

	weights := map[string]float64{"name": 0.30, "embedding": 0.15, "relationship": 0.25, "geography": 0.10, "chronology": 0.10, "source_context": 0.10}
	total, totalWeight := 0.0, 0.0
	for name, value := range components {
		weight := weights[name]
		total += value * weight
		totalWeight += weight
	}
	score := 0.0
	if totalWeight > 0 {
		score = total / totalWeight
	}
	hardConflict := relationshipConflict || chronologyConflict || (left.Gender != "" && right.Gender != "" && left.Gender != right.Gender)
	if hardConflict && score >= 0.78 {
		score = 0.49
		conflicting = append(conflicting, Signal{Kind: "hard_conflict", Detail: "توجد قرينة صريحة تمنع اعتبارهما نسخة واحدة", Score: score})
	}
	class := LikelyDifferent
	switch {
	case score >= 0.75:
		class = StrongCandidate
	case score >= 0.55:
		class = PossibleMatch
	}
	explanation := buildExplanation(left, right, score, class, matching, conflicting)
	return generatedCandidate{Left: left, Right: right, MatchClass: class, Score: score, ScoreComponents: components, MatchingSignals: matching, ConflictingSignals: conflicting, ExplanationAR: explanation}
}

func stringSimilarity(left, right string) float64 {
	left = identity.NormalizeArabicName(left)
	right = identity.NormalizeArabicName(right)
	if left == right {
		if left == "" {
			return 0
		}
		return 1
	}
	if left == "" || right == "" {
		return 0
	}
	leftRunes, rightRunes := []rune(left), []rune(right)
	rows := make([][]int, len(leftRunes)+1)
	for index := range rows {
		rows[index] = make([]int, len(rightRunes)+1)
		rows[index][0] = index
	}
	for column := range rightRunes {
		rows[0][column+1] = column + 1
	}
	for leftIndex := 1; leftIndex <= len(leftRunes); leftIndex++ {
		for rightIndex := 1; rightIndex <= len(rightRunes); rightIndex++ {
			cost := 1
			if leftRunes[leftIndex-1] == rightRunes[rightIndex-1] {
				cost = 0
			}
			rows[leftIndex][rightIndex] = minInt(rows[leftIndex-1][rightIndex]+1, rows[leftIndex][rightIndex-1]+1, rows[leftIndex-1][rightIndex-1]+cost)
		}
	}
	distance := float64(rows[len(leftRunes)][len(rightRunes)])
	charScore := 1 - distance/float64(maxInt(len(leftRunes), len(rightRunes)))
	leftTokens, rightTokens := tokenSet(left), tokenSet(right)
	tokenScore := 0.0
	if len(leftTokens) > 0 && len(rightTokens) > 0 {
		intersection := 0
		for token := range leftTokens {
			if _, exists := rightTokens[token]; exists {
				intersection++
			}
		}
		tokenScore = float64(intersection) / float64(len(leftTokens)+len(rightTokens)-intersection)
	}
	return math.Max(0, math.Max(charScore, tokenScore))
}

func tokenSet(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, token := range strings.Fields(value) {
		result[token] = struct{}{}
	}
	return result
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func maxInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

func nameSimilarity(left, right *entitySnapshot) (float64, bool) {
	leftValues := append([]string{left.Normalized}, left.Aliases...)
	rightValues := append([]string{right.Normalized}, right.Aliases...)
	best := 0.0
	for _, leftValue := range leftValues {
		for _, rightValue := range rightValues {
			if value := stringSimilarity(leftValue, rightValue); value > best {
				best = value
			}
		}
	}
	return best, best < 0.45
}

func relationshipSimilarity(left, right *entitySnapshot) (float64, bool, bool) {
	pairs := []struct {
		left, right map[string]struct{}
		weight      float64
	}{
		{left.Fathers, right.Fathers, 0.40},
		{left.Grandfathers, right.Grandfathers, 0.25},
		{left.Children, right.Children, 0.15},
		{left.Siblings, right.Siblings, 0.10},
		{left.Spouses, right.Spouses, 0.10},
	}
	score, weight, available := 0.0, 0.0, false
	conflict := false
	for _, pair := range pairs {
		if len(pair.left) == 0 || len(pair.right) == 0 {
			continue
		}
		available = true
		value, overlap := jaccard(pair.left, pair.right)
		score += value * pair.weight
		weight += pair.weight
		if !overlap {
			conflict = true
		}
	}
	if !available {
		return 0, false, false
	}
	return score / weight, true, conflict
}

func geographySimilarity(left, right *entitySnapshot) (float64, bool) {
	value, _ := jaccard(left.Places, right.Places)
	return value, len(left.Places) > 0 && len(right.Places) > 0
}

func chronologySimilarity(left, right *entitySnapshot) (float64, bool, bool) {
	leftFrom, leftTo, leftOK := dateRange(left.BirthFrom, left.BirthTo)
	rightFrom, rightTo, rightOK := dateRange(right.BirthFrom, right.BirthTo)
	if !leftOK || !rightOK {
		leftFrom, leftTo, leftOK = dateRange(left.DeathFrom, left.DeathTo)
		rightFrom, rightTo, rightOK = dateRange(right.DeathFrom, right.DeathTo)
	}
	if !leftOK || !rightOK {
		return 0, false, false
	}
	leftStart, leftEnd := timeRange(leftFrom, leftTo)
	rightStart, rightEnd := timeRange(rightFrom, rightTo)
	if !leftStart.After(rightEnd) && !rightStart.After(leftEnd) {
		return 1, true, false
	}
	gap := 0.0
	if leftEnd.Before(rightStart) {
		gap = rightStart.Sub(leftEnd).Hours() / 24 / 365
	} else {
		gap = leftStart.Sub(rightEnd).Hours() / 24 / 365
	}
	score := math.Max(0, 1-gap/80)
	return score, true, true
}

func dateRange(from, to pgtype.Date) (time.Time, time.Time, bool) {
	if !from.Valid && !to.Valid {
		return time.Time{}, time.Time{}, false
	}
	if from.Valid {
		return from.Time, valueOrDate(to, from.Time), true
	}
	return to.Time, to.Time, true
}

func valueOrDate(value pgtype.Date, fallback time.Time) time.Time {
	if value.Valid {
		return value.Time
	}
	return fallback
}

func timeRange(from, to time.Time) (time.Time, time.Time) {
	if to.Before(from) {
		from, to = to, from
	}
	return from, to
}

func embeddingSimilarity(ctx context.Context, service *Service, left, right *entitySnapshot, cache *embeddingCache) *float64 {
	leftVector := embeddingFor(ctx, service, left.EntityType, left.Name, cache)
	rightVector := embeddingFor(ctx, service, right.EntityType, right.Name, cache)
	if len(leftVector) == 0 || len(rightVector) == 0 {
		return nil
	}
	value := cosine(leftVector, rightVector)
	return &value
}

func embeddingFor(ctx context.Context, service *Service, entityType EntityType, name string, cache *embeddingCache) []float64 {
	key := string(entityType) + "|" + identity.NormalizeArabicName(name)
	if value, exists := cache.values[key]; exists {
		return value
	}
	var vector []float64
	if service != nil && service.AI != nil && cache.aiCalls < 20 {
		cache.aiCalls++
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		response, err := service.AI.Embed(callCtx, ai.EmbeddingRequest{Text: name, Dimensions: 1536})
		cancel()
		if err == nil && len(response.Embedding) == 1536 && validVector(response.Embedding) {
			vector = make([]float64, len(response.Embedding))
			copy(vector, response.Embedding)
			if cache.model == "" {
				cache.model = response.Model
			}
		}
	}
	if len(vector) == 0 {
		vector = localVector(name)
		if cache.model == "" {
			cache.model = "local-character-v1"
		}
	}
	cache.values[key] = vector
	return vector
}

func validVector(values []float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func localVector(value string) []float64 {
	vector := make([]float64, 64)
	normalized := identity.NormalizeArabicName(value)
	for _, token := range strings.Fields(normalized) {
		digest := sha256.Sum256([]byte(token))
		index := int(binary.LittleEndian.Uint32(digest[:4]) % uint32(len(vector)))
		vector[index] += 1
	}
	for index, character := range normalized {
		digest := sha256.Sum256([]byte{byte(character), byte(index % 251)})
		vector[int(binary.LittleEndian.Uint32(digest[:4])%uint32(len(vector)))] += 0.25
	}
	norm := 0.0
	for _, item := range vector {
		norm += item * item
	}
	if norm == 0 {
		return vector
	}
	norm = math.Sqrt(norm)
	for index := range vector {
		vector[index] /= norm
	}
	return vector
}

func cosine(left, right []float64) float64 {
	if len(left) != len(right) {
		return 0
	}
	dot, leftNorm, rightNorm := 0.0, 0.0, 0.0
	for index := range left {
		dot += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func setSimilarity(left, right map[string]struct{}) (float64, bool) {
	value, _ := jaccard(left, right)
	return value, len(left) > 0 && len(right) > 0
}

func jaccard(left, right map[string]struct{}) (float64, bool) {
	if len(left) == 0 || len(right) == 0 {
		return 0, false
	}
	intersection := 0
	for value := range left {
		if _, exists := right[value]; exists {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0, false
	}
	return float64(intersection) / float64(union), intersection > 0
}

func buildExplanation(left, right *entitySnapshot, score float64, class MatchClass, matching, conflicting []Signal) string {
	parts := []string{}
	if len(matching) > 0 {
		parts = append(parts, "إشارات متطابقة: "+strings.Join(signalDetails(matching), "، "))
	}
	if len(conflicting) > 0 {
		parts = append(parts, "إشارات متعارضة: "+strings.Join(signalDetails(conflicting), "، "))
	}
	if len(parts) == 0 {
		parts = append(parts, "لا توجد إشارات كافية لفرض مطابقة")
	}
	return left.Name + " و" + right.Name + ": " + strings.Join(parts, ". ") + ". النتيجة " + classLabel(class) + " بدرجة فرز " + itoa(int(math.Round(score*100))) + "٪."
}

func signalDetails(signals []Signal) []string {
	result := make([]string, 0, len(signals))
	for _, signal := range signals {
		result = append(result, signal.Detail)
	}
	return result
}

func classLabel(value MatchClass) string {
	switch value {
	case StrongCandidate:
		return "مرشح قوي"
	case PossibleMatch:
		return "تطابق محتمل"
	default:
		return "على الأرجح مختلف"
	}
}
