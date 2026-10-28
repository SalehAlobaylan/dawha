package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const NamePoolVersion = "ar-name-pool-v2"
const NameEvidenceVersion = "name-review-v2"
const NameGenerationVersion = "local-evidence-v1"
const NameLexiconVersion = "file-name-lexicon-v1"
const maxLexiconEntries = 2048
const maxLexiconSpellings = 10000
const maxLexiconResponseBytes = 1 << 20

type NameLexiconEntry struct {
	Value             string   `json:"value"`
	Key               string   `json:"key"`
	CompactKey        string   `json:"compactKey"`
	ParticleKey       string   `json:"particleKey"`
	Count             int      `json:"count"`
	Variants          []string `json:"variants"`
	VariantsTruncated bool     `json:"variantsTruncated"`
}
type NameLexicon struct {
	ID               string             `json:"-"`
	Revision         string             `json:"revision"`
	Version          string             `json:"version"`
	AsOf             time.Time          `json:"asOf"`
	Entries          []NameLexiconEntry `json:"entries"`
	TotalOccurrences int                `json:"totalOccurrences"`
	TotalSpellings   int                `json:"totalSpellings"`
	Truncated        bool               `json:"truncated"`
	OmittedEntries   int                `json:"omittedEntries"`
	InvalidSpellings int                `json:"invalidSpellings"`
}

func (s *Service) NameLexicon(ctx context.Context, fileID, actorID string) (NameLexicon, error) {
	file, err := uuid.Parse(fileID)
	if err != nil {
		return NameLexicon{}, ErrNotFound
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return NameLexicon{}, err
	}
	if !allowed {
		return NameLexicon{}, ErrForbidden
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return NameLexicon{}, err
	}
	defer tx.Rollback(ctx)
	lex, err := loadNameLexicon(ctx, tx, fileID)
	if err != nil {
		return lex, err
	}
	return lex, tx.Commit(ctx)
}

// Aggregate the latest human decision of each occurrence, never machine fallback
// text. A bounded response is distinct from silently truncating decision rows.
func loadNameLexicon(ctx context.Context, tx pgx.Tx, fileID string) (NameLexicon, error) {
	result := NameLexicon{Version: NameLexiconVersion, Entries: []NameLexiconEntry{}}
	rows, err := tx.Query(ctx, `WITH latest AS (
 SELECT DISTINCT ON(d.suggestion_id) d.suggestion_id,d.decision,d.decided_text_ar,d.created_at,d.id
 FROM source_tree_suggestion_decisions d JOIN source_tree_suggestions s ON s.id=d.suggestion_id
 WHERE s.source_file_id=$1 ORDER BY d.suggestion_id,d.created_at DESC,d.id DESC
 ), spellings AS (SELECT btrim(decided_text_ar) AS name,count(*) AS n,max(created_at) AS cutoff
 FROM latest WHERE decision IN ('accepted','corrected') AND btrim(COALESCE(decided_text_ar,''))<>'' GROUP BY btrim(decided_text_ar))
 SELECT name,n,count(*) OVER(),sum(n) OVER(),max(cutoff) OVER() FROM spellings ORDER BY name COLLATE "C" LIMIT $2`, fileID, maxLexiconSpellings+1)
	if err != nil {
		return result, err
	}
	type spelling struct {
		Value string
		Count int
	}
	groups := map[string][]spelling{}
	counts := map[string]int{}
	var cutoff time.Time
	scanned := 0
	for rows.Next() {
		var value string
		var n int
		if err = rows.Scan(&value, &n, &result.TotalSpellings, &result.TotalOccurrences, &cutoff); err != nil {
			rows.Close()
			return result, err
		}
		scanned++
		if scanned > maxLexiconSpellings {
			result.Truncated = true
			continue
		}
		if !validJudgeNameValue(value, true) {
			result.InvalidSpellings++
			continue
		}
		key := nameCanonical(value)
		groups[key] = append(groups[key], spelling{value, n})
		counts[key] += n
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result.OmittedEntries = max(0, len(keys)-maxLexiconEntries)
	if result.OmittedEntries > 0 {
		result.Truncated = true
	}
	// The identity includes the complete scanned aggregate and cutoff, even if a
	// bounded response drops entries. AsOf is assigned only on first persistence.
	identity := hashNameValue(struct {
		Version          string
		Groups           map[string][]spelling
		Counts           map[string]int
		Cutoff           time.Time
		Total, Spellings int
	}{NameLexiconVersion, groups, counts, cutoff, result.TotalOccurrences, result.TotalSpellings})
	encodedBytes := 4096
	for _, key := range keys[:min(len(keys), maxLexiconEntries)] {
		variants := groups[key]
		sort.Slice(variants, func(i, j int) bool {
			if variants[i].Count != variants[j].Count {
				return variants[i].Count > variants[j].Count
			}
			return variants[i].Value < variants[j].Value
		})
		if len(variants) > 8 {
			result.Truncated = true
		}
		values := []string{}
		for _, v := range variants[:min(len(variants), 8)] {
			values = append(values, v.Value)
		}
		entry := NameLexiconEntry{Value: variants[0].Value, Key: key, CompactKey: nameCompact(key), ParticleKey: nameParticle(key), Count: counts[key], Variants: values, VariantsTruncated: len(variants) > 8}
		encodedBytes += len(mustJSON(entry)) + 1
		if encodedBytes > maxLexiconResponseBytes {
			result.Truncated = true
			break
		}
		result.Entries = append(result.Entries, entry)
	}
	result.OmittedEntries = len(keys) - len(result.Entries)
	if result.InvalidSpellings > 0 {
		result.Truncated = true
	}
	result.Revision = identity
	var encoded []byte
	err = tx.QueryRow(ctx, `SELECT id::text,context FROM source_name_lexicon_revisions WHERE source_file_id=$1 AND revision=$2`, fileID, identity).Scan(&result.ID, &encoded)
	if err == nil {
		err = json.Unmarshal(encoded, &result)
		return result, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&result.AsOf); err != nil {
		return result, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO source_name_lexicon_revisions(source_file_id,revision,context) VALUES($1,$2,$3) ON CONFLICT(source_file_id,revision) DO UPDATE SET revision=EXCLUDED.revision RETURNING id::text,context`, fileID, identity, mustJSON(result)).Scan(&result.ID, &encoded)
	if err == nil {
		err = json.Unmarshal(encoded, &result)
	}
	return result, err
}
func lexiconJudgeNames(lex NameLexicon) []string {
	entries := append([]NameLexiconEntry(nil), lex.Entries...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Key < entries[j].Key
	})
	result := []string{}
	for _, entry := range entries {
		if len(result) == 20 {
			break
		}
		result = append(result, entry.Value)
	}
	return result
}

// Registration is immutable. Rendering groups cannot straddle partitions or
// contracts. The operator must identify related renderings; matching checksum
// and source identity also enforce known relationships locally.
type NameEvaluationRegistration struct {
	Partition string            `json:"partition"`
	GroupKey  string            `json:"groupKey"`
	Contract  map[string]string `json:"contract"`
}

func (s *Service) NameEvaluationContract(ctx context.Context) map[string]string {
	return map[string]string{"pool": NamePoolVersion, "normalizer": "ar-name-keys-v1", "evidence": NameEvidenceVersion, "generation": NameGenerationVersion, "lexicon": NameLexiconVersion, "renderer": NameRendererVersion, "geometry": NameGeometryVersion, "preprocessing": NamePreprocessingVersion, "reader": s.nameReadingConfig(ctx).Reader, "judgeModel": nameJudgeIdentity().Model, "judgePrompt": nameJudgeIdentity().Prompt}
}
func (s *Service) RegisterNameEvaluation(ctx context.Context, fileID, actorID string, in NameEvaluationRegistration) error {
	file, err := uuid.Parse(fileID)
	if err != nil {
		return ErrNotFound
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	if (in.Partition != "development" && in.Partition != "held_out") || !validSHA256(in.GroupKey) {
		return ErrValidation
	}
	expected := s.NameEvaluationContract(ctx)
	if len(in.Contract) != len(expected) {
		return ErrValidation
	}
	for k, v := range expected {
		if in.Contract[k] == "" || (in.Partition == "held_out" && in.Contract[k] != v) {
			return ErrValidation
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(253025)`); err != nil {
		return err
	}
	var sourceID, checksum string
	var fresh bool
	if err = tx.QueryRow(ctx, `SELECT source_id::text,checksum_sha256,created_at>(SELECT installed_at FROM source_name_review_boundary WHERE version=$2) FROM source_files WHERE id=$1 FOR UPDATE`, fileID, NameEvidenceVersion).Scan(&sourceID, &checksum, &fresh); err != nil {
		return err
	}
	var existingGroup, partition string
	var contract []byte
	err = tx.QueryRow(ctx, `SELECT g.group_key,g.partition,g.contract FROM source_name_evaluation_members m JOIN source_name_evaluation_groups g ON g.id=m.group_id WHERE m.source_file_id=$1`, fileID).Scan(&existingGroup, &partition, &contract)
	if err == nil {
		if existingGroup != in.GroupKey || partition != in.Partition || FrozenNameJudgeSignature(contract) != hashNameValue(in.Contract) {
			return ErrConflict
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var conflict bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_name_evaluation_members m JOIN source_name_evaluation_groups g ON g.id=m.group_id JOIN source_files f ON f.id=m.source_file_id WHERE (f.source_id=$1 OR f.checksum_sha256=$2) AND g.group_key<>$3)`, sourceID, checksum, in.GroupKey).Scan(&conflict)
	if err != nil {
		return err
	}
	if conflict {
		return ErrConflict
	}
	if in.Partition == "held_out" {
		var legacyRendering bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_files f WHERE f.id<>$3 AND (f.source_id=$1 OR f.checksum_sha256=$2) AND (f.created_at<=(SELECT installed_at FROM source_name_review_boundary WHERE version=$4) OR EXISTS(SELECT 1 FROM source_tree_suggestions s WHERE s.source_file_id=f.id AND (EXISTS(SELECT 1 FROM source_tree_suggestion_decisions d WHERE d.suggestion_id=s.id) OR EXISTS(SELECT 1 FROM source_name_review_exposures e WHERE e.suggestion_id=s.id)))))`, sourceID, checksum, fileID, NameEvidenceVersion).Scan(&legacyRendering)
		if err != nil {
			return err
		}
		if legacyRendering {
			return ErrConflict
		}
		var seen bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_tree_suggestions s WHERE (s.source_file_id=$1 OR s.source_file_id IN (SELECT source_file_id FROM source_name_evaluation_members m JOIN source_name_evaluation_groups g ON g.id=m.group_id WHERE g.group_key=$2)) AND (EXISTS(SELECT 1 FROM source_tree_suggestion_decisions d WHERE d.suggestion_id=s.id) OR EXISTS(SELECT 1 FROM source_name_review_exposures e WHERE e.suggestion_id=s.id)))`, fileID, in.GroupKey).Scan(&seen)
		if err != nil {
			return err
		}
		if seen || !fresh {
			return ErrConflict
		}
	}
	var group string
	err = tx.QueryRow(ctx, `INSERT INTO source_name_evaluation_groups(group_key,partition,contract,created_by) VALUES($1,$2,$3,$4) ON CONFLICT(group_key) DO NOTHING RETURNING id::text`, in.GroupKey, in.Partition, mustJSON(in.Contract), actorID).Scan(&group)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id::text,partition,contract FROM source_name_evaluation_groups WHERE group_key=$1`, in.GroupKey).Scan(&group, &partition, &contract)
		if err == nil && (partition != in.Partition || FrozenNameJudgeSignature(contract) != hashNameValue(in.Contract)) {
			return ErrConflict
		}
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO source_name_evaluation_members(source_file_id,group_id) VALUES($1,$2)`, fileID, group); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
