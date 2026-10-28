package sourceprocessing

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"hash/fnv"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

//go:embed name_candidate_common.json
var commonNameJSON []byte

var namePreparationSlots = make(chan struct{}, 4)

type FrozenJudgeIdentity struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
}

func nameJudgeIdentity() FrozenJudgeIdentity {
	provider := strings.TrimSpace(os.Getenv("NAME_JUDGE_PROVIDER"))
	if provider == "" {
		provider = "liquid"
	}
	model := strings.TrimSpace(os.Getenv("LIQUID_MODEL"))
	if model == "" {
		model = "d1:3b"
	}
	prompt := strings.TrimSpace(os.Getenv("LIQUID_PROMPT_VERSION"))
	if prompt == "" {
		prompt = "name-judge-liquid-v1"
	}
	return FrozenJudgeIdentity{provider, model, prompt}
}

type NameReaderEvidence struct {
	Value         string `json:"value"`
	Variant       string `json:"variant"`
	ReaderVersion string `json:"readerVersion"`
}
type NamePreparationContext struct {
	PrivacyPolicy     string                    `json:"privacyPolicy"`
	JudgeMode         string                    `json:"judgeMode"`
	JudgeSample       float64                   `json:"judgeSample"`
	Region            Region                    `json:"region"`
	TightBounds       ChartTileBounds           `json:"tightBounds"`
	ExposureID        string                    `json:"exposureId"`
	Reading           string                    `json:"reading"`
	Alternatives      []string                  `json:"alternatives"`
	Readers           []NameReaderEvidence      `json:"readers"`
	Lexicon           NameLexicon               `json:"lexicon"`
	Crop              JudgeCrop                 `json:"crop"`
	Frame             *NamePageFrame            `json:"frame,omitempty"`
	Versions          map[string]string         `json:"versions"`
	Diagnostics       []string                  `json:"diagnostics"`
	Eligible          bool                      `json:"eligible"`
	EligibilityReason string                    `json:"eligibilityReason"`
	FirstExposedAt    time.Time                 `json:"firstExposedAt"`
	CanPrepare        bool                      `json:"canPrepare"`
	Preparation       *NameCandidatePreparation `json:"preparation,omitempty"`
}
type NameCandidatePreparation struct {
	ID        string                        `json:"id"`
	Signature string                        `json:"signature"`
	Snapshot  []SuggestionCandidateSnapshot `json:"snapshot"`
}
type PrepareNameCandidatesInput struct {
	ExposureID  string                        `json:"exposureId"`
	PoolVersion string                        `json:"poolVersion"`
	Snapshot    []SuggestionCandidateSnapshot `json:"snapshot"`
}
type NameCandidateProvenance struct {
	Value           string               `json:"value"`
	Sources         []string             `json:"sources"`
	ReaderVariants  []NameReaderEvidence `json:"readerVariants"`
	LexiconRevision string               `json:"lexiconRevision,omitempty"`
}

func (s *Service) lockReviewOccurrence(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	file, status, _, superseded, err := s.lockReviewRow(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if superseded {
		return "", ErrSuggestionSuperseded
	}
	if status != "suggested" {
		return "", ErrSuggestionAlreadyDecided
	}
	return file.String(), nil
}

// Lineage is undirected for exposure/history: split, merge and repair can never
// erase a parent's or sibling's previous human decision. UNION bounds cycles.
const nameLineageSQL = `WITH RECURSIVE lineage(id) AS (
 SELECT $1::uuid UNION SELECT CASE WHEN e.parent_id=l.id THEN e.child_id ELSE e.parent_id END
 FROM lineage l JOIN source_name_occurrence_lineage e ON e.parent_id=l.id OR e.child_id=l.id
) `

func nameReviewHistory(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	var decisions, exposure, untracked bool
	err := tx.QueryRow(ctx, nameLineageSQL+`SELECT
 EXISTS(SELECT 1 FROM source_tree_suggestion_decisions d JOIN lineage l ON l.id=d.suggestion_id),
 EXISTS(SELECT 1 FROM source_name_review_exposures e JOIN lineage l ON l.id=e.suggestion_id WHERE e.suggestion_id<>$1),
 EXISTS(SELECT 1 FROM source_name_untracked_exposures e JOIN lineage l ON l.id=e.suggestion_id)`, id).Scan(&decisions, &exposure, &untracked)
	if decisions {
		return "prior_human_decision", err
	}
	if exposure {
		return "prior_lineage_exposure", err
	}
	if untracked {
		return "untracked_review_exposure", err
	}
	return "", err
}
func (s *Service) BeginNameReview(ctx context.Context, id, actorID string) (NamePreparationContext, error) {
	result := NamePreparationContext{}
	source, err := s.loadSuggestionContext(ctx, id, actorID, false)
	if err != nil {
		return result, err
	}
	select {
	case namePreparationSlots <- struct{}{}:
		defer func() { <-namePreparationSlots }()
	default:
		return result, ErrQueueUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	file, err := s.lockReviewOccurrence(ctx, tx, id)
	if err != nil {
		return result, err
	}
	var encoded []byte
	var deadline time.Time
	var sealed *time.Time
	err = tx.QueryRow(ctx, `SELECT id::text,context,created_at,deadline,sealed_at FROM source_name_review_exposures WHERE suggestion_id=$1`, id).Scan(&result.ExposureID, &encoded, &result.FirstExposedAt, &deadline, &sealed)
	if err == nil {
		exposure, created := result.ExposureID, result.FirstExposedAt
		if err = json.Unmarshal(encoded, &result); err != nil {
			return result, err
		}
		result.ExposureID = exposure
		result.FirstExposedAt = created
		var prep NameCandidatePreparation
		var snapshot []byte
		err = tx.QueryRow(ctx, `SELECT id::text,signature,snapshot FROM source_name_candidate_preparations WHERE exposure_id=$1`, exposure).Scan(&prep.ID, &prep.Signature, &snapshot)
		if err == nil {
			if err = json.Unmarshal(snapshot, &prep.Snapshot); err != nil {
				return result, err
			}
			result.Preparation = &prep
			result.CanPrepare = false
		} else if errors.Is(err, pgx.ErrNoRows) {
			result.CanPrepare = sealed == nil && time.Now().Before(deadline)
		} else {
			return result, err
		}
		return result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	result.PrivacyPolicy = "local_text_only"
	result.JudgeMode = s.nameJudgeSettings().Mode
	result.JudgeSample = s.nameJudgeSettings().Sample
	// Re-read machine evidence under the occurrence lock. A local reader may
	// have completed between authorization and this transaction.
	var reader string
	var alternativesJSON, regionJSON []byte
	if err = tx.QueryRow(ctx, `SELECT s.suggested_text_ar,s.alternatives,s.region,s.page_number,s.reading_model,f.checksum_sha256 FROM source_tree_suggestions s JOIN source_files f ON f.id=s.source_file_id WHERE s.id=$1`, id).Scan(&source.suggestedText, &alternativesJSON, &regionJSON, &source.pageNumber, &reader, &source.checksum); err != nil {
		return result, err
	}
	if json.Unmarshal(alternativesJSON, &source.alternatives) != nil || json.Unmarshal(regionJSON, &source.region) != nil {
		return result, ErrValidation
	}
	result.Lexicon, err = loadNameLexicon(ctx, tx, file)
	if err != nil {
		return result, err
	}
	result.Reading = boundedJudgeReading(source.suggestedText)
	result.Alternatives = boundedJudgeNames(source.alternatives, 5)
	result.Readers = []NameReaderEvidence{}
	result.Diagnostics = []string{}
	if validJudgeNameValue(result.Reading, true) {
		result.Readers = append(result.Readers, NameReaderEvidence{result.Reading, "primary", ""})
	}
	for _, v := range result.Alternatives {
		if !validJudgeNameValue(v, true) {
			continue
		}
		result.Readers = append(result.Readers, NameReaderEvidence{v, "primary-alternative", ""})
	}
	for i := range result.Readers {
		result.Readers[i].ReaderVersion = reader
	}
	rows, err := tx.Query(ctx, `SELECT variants,reader_version,outcome FROM source_name_readings WHERE suggestion_id=$1 ORDER BY created_at DESC,id DESC LIMIT 3`, id)
	if err != nil {
		return result, err
	}
	readerVersions := map[string]bool{reader: true}
	for rows.Next() {
		var variants []byte
		var version, outcome string
		if err = rows.Scan(&variants, &version, &outcome); err != nil {
			rows.Close()
			return result, err
		}
		if outcome == "reading_unavailable" {
			result.Diagnostics = append(result.Diagnostics, "local_reading_unavailable")
			continue
		}
		var vv []LocalVariantReading
		if json.Unmarshal(variants, &vv) != nil {
			rows.Close()
			return result, ErrValidation
		}
		for _, v := range vv {
			if validJudgeNameValue(v.RawText, true) && len(result.Readers) < 24 {
				result.Readers = append(result.Readers, NameReaderEvidence{v.RawText, v.Variant, version})
				readerVersions[version] = true
			}
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(readerVersions) > 1 {
		versions := []string{}
		for v := range readerVersions {
			versions = append(versions, v)
		}
		sort.Strings(versions)
		reader = "reader-set-v1+" + hashNameValue(versions)[:16]
	}
	if reader == "" {
		reader = "legacy"
	}
	identity := nameJudgeIdentity()
	result.Versions = map[string]string{"evidence": NameEvidenceVersion, "pool": NamePoolVersion, "normalizer": "ar-name-keys-v1", "generation": NameGenerationVersion, "lexicon": NameLexiconVersion, "renderer": NameRendererVersion, "geometry": NameGeometryVersion, "preprocessing": NamePreprocessingVersion, "reader": reader, "judgeModel": identity.Model, "judgePrompt": identity.Prompt}
	frame, frameErr := loadNameFrame(ctx, tx, source)
	if frameErr == nil {
		bounds, boundsErr := suggestionCropBounds(source.region, frame.Width, frame.Height, "tight")
		if boundsErr != nil {
			return result, boundsErr
		}
		result.Frame = &frame
		result.Region = source.region
		result.TightBounds = bounds
		result.Crop = JudgeCrop{bounds.Width, bounds.Height, frame.Width, frame.Height, int(math.Round(source.region.Height * float64(frame.Height)))}
	} else if !errors.Is(frameErr, pgx.ErrNoRows) {
		return result, frameErr
	}
	reason, err := nameReviewHistory(ctx, tx, id)
	if err != nil {
		return result, err
	}
	var partition string
	var contract []byte
	var registeredBefore bool
	err = tx.QueryRow(ctx, `SELECT g.partition,g.contract,m.created_at<clock_timestamp() FROM source_name_evaluation_members m JOIN source_name_evaluation_groups g ON g.id=m.group_id WHERE m.source_file_id=$1`, file).Scan(&partition, &contract, &registeredBefore)
	if errors.Is(err, pgx.ErrNoRows) {
		partition = "unregistered"
	} else if err != nil {
		return result, err
	}
	if reason == "" && partition != "held_out" {
		reason = partition + "_source"
	}
	if reason == "" {
		var expected map[string]string
		if json.Unmarshal(contract, &expected) != nil || hashNameValue(expected) != hashNameValue(result.Versions) || !registeredBefore {
			reason = "cohort_contract_mismatch"
		}
	}
	if reason == "" && result.Lexicon.Truncated {
		reason = "truncated_file_context"
	}
	if reason == "" && result.Frame == nil {
		reason = "geometry_unavailable"
	}
	if reason == "" && s.nameJudgeSettings().Mode == "active" {
		reason = "active_review"
	}
	result.Eligible = reason == ""
	if reason == "" {
		reason = "first_held_out_exposure"
	}
	result.EligibilityReason = reason
	if result.Reading == "" {
		result.Diagnostics = append(result.Diagnostics, "empty_machine_reading")
	}
	if len(result.Readers) == 0 {
		result.Diagnostics = append(result.Diagnostics, "unsupported_reading")
	}
	if result.Lexicon.Truncated {
		result.Diagnostics = append(result.Diagnostics, "truncated_file_context")
	}
	if result.Frame == nil {
		result.Diagnostics = append(result.Diagnostics, "geometry_unavailable")
	}
	result.CanPrepare = true
	err = tx.QueryRow(ctx, `INSERT INTO source_name_review_exposures(suggestion_id,lexicon_id,context,eligible,eligibility_reason) VALUES($1,$2,$3,$4,$5) RETURNING id::text,created_at`, id, result.Lexicon.ID, mustJSON(result), result.Eligible, reason).Scan(&result.ExposureID, &result.FirstExposedAt)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (s *Service) SealNameReview(ctx context.Context, id, actorID string) error {
	if _, err := s.loadSuggestionContext(ctx, id, actorID, false); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO source_name_untracked_exposures(suggestion_id) VALUES($1) ON CONFLICT(suggestion_id) DO NOTHING`, id)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE source_name_review_exposures SET sealed_at=COALESCE(sealed_at,clock_timestamp()) WHERE suggestion_id=$1`, id)
	return err
}
func candidateProvenance(snapshot []SuggestionCandidateSnapshot, c NamePreparationContext) ([]NameCandidateProvenance, error) {
	var common []string
	if json.Unmarshal(commonNameJSON, &common) != nil {
		return nil, ErrValidation
	}
	out := []NameCandidateProvenance{}
	for _, item := range snapshot {
		if item.Key != nameCanonical(item.Value) || item.CompactKey != nameCompact(item.Key) || item.ParticleKey != nameParticle(item.Key) {
			return nil, ErrValidation
		}
		evidence := NameCandidateProvenance{Value: item.Value, Sources: item.Sources, ReaderVariants: []NameReaderEvidence{}}
		expectedCount := 0
		for _, source := range item.Sources {
			supported := false
			switch source {
			case "reader":
				for _, v := range c.Readers {
					if v.Value == item.Value {
						supported = true
						evidence.ReaderVariants = append(evidence.ReaderVariants, v)
					}
				}
			case "file":
				for _, entry := range c.Lexicon.Entries {
					if item.Key == entry.Key {
						for _, v := range entry.Variants {
							if v == item.Value {
								supported = true
								expectedCount = entry.Count
								evidence.LexiconRevision = c.Lexicon.Revision
							}
						}
					}
				}
			case "common":
				for _, v := range common {
					if v == item.Value {
						supported = true
						break
					}
				}
			default:
				return nil, ErrValidation
			}
			// Equivalent supported display spellings can merge provenance; neither a
			// typed string nor a visual-confusion invention creates a new source.
			if !supported {
				switch source {
				case "reader":
					for _, v := range c.Readers {
						if nameCanonical(v.Value) == item.Key || nameCompact(nameCanonical(v.Value)) == item.CompactKey {
							supported = true
							evidence.ReaderVariants = append(evidence.ReaderVariants, v)
						}
					}
				case "file":
					for _, e := range c.Lexicon.Entries {
						if e.Key == item.Key || e.CompactKey == item.CompactKey {
							supported = true
							expectedCount = e.Count
							evidence.LexiconRevision = c.Lexicon.Revision
						}
					}
				case "common":
					for _, v := range common {
						if nameCanonical(v) == item.Key || nameCompact(nameCanonical(v)) == item.CompactKey {
							supported = true
							break
						}
					}
				}
			}
			if !supported {
				return nil, ErrValidation
			}
		}
		if (expectedCount == 0 && item.FileCount != 0) || (expectedCount > 0 && (item.FileCount < 1 || item.FileCount > c.Lexicon.TotalOccurrences)) {
			return nil, ErrValidation
		}
		out = append(out, evidence)
	}
	return out, nil
}
func sampledNameShadow(id string, sample float64) bool {
	if sample <= 0 {
		return false
	}
	if sample >= 1 {
		return true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return float64(h.Sum32())/math.Exp2(32) < sample
}
func (s *Service) PrepareNameCandidates(ctx context.Context, id, actorID string, in PrepareNameCandidatesInput) (NameCandidatePreparation, error) {
	out := NameCandidatePreparation{}
	if in.PoolVersion != NamePoolVersion {
		return out, ErrValidation
	}
	source, err := s.loadSuggestionContext(ctx, id, actorID, false)
	if err != nil {
		return out, err
	}
	if _, err = uuid.Parse(in.ExposureID); err != nil {
		return out, ErrValidation
	}
	ev, err := validateDecisionEvidence(SuggestionDecision{CandidateSnapshot: in.Snapshot, InputMethod: "typed", NormalizerVersion: "ar-name-keys-v1", CandidatePoolVersion: NamePoolVersion})
	if err != nil {
		return out, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err = s.lockReviewOccurrence(ctx, tx, id); err != nil {
		return out, err
	}
	var encoded []byte
	var eligible, open bool
	var reason string
	err = tx.QueryRow(ctx, `SELECT context,eligible,eligibility_reason,sealed_at IS NULL AND deadline>clock_timestamp() FROM source_name_review_exposures WHERE id=$1 AND suggestion_id=$2 FOR UPDATE`, in.ExposureID, id).Scan(&encoded, &eligible, &reason, &open)
	if err != nil {
		return out, ErrConflict
	}
	var c NamePreparationContext
	if json.Unmarshal(encoded, &c) != nil {
		return out, ErrValidation
	}
	signature := NamePreparationSignature(in.ExposureID, in.PoolVersion, ev.candidateSnapshot)
	var existing []byte
	err = tx.QueryRow(ctx, `SELECT id::text,signature,snapshot FROM source_name_candidate_preparations WHERE exposure_id=$1`, in.ExposureID).Scan(&out.ID, &out.Signature, &existing)
	if err == nil {
		if out.Signature != signature {
			return out, ErrConflict
		}
		_ = json.Unmarshal(existing, &out.Snapshot)
		return out, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if !open {
		return out, ErrConflict
	}
	// History may have appeared through a related occurrence since begin. A late
	// finalization cannot retain a stale claim of blind eligibility.
	history, err := nameReviewHistory(ctx, tx, id)
	if err != nil {
		return out, err
	}
	if history != "" {
		eligible = false
		reason = history
	}
	provenance, err := candidateProvenance(in.Snapshot, c)
	if err != nil {
		return out, err
	}
	shown := 0
	for _, candidate := range in.Snapshot {
		if candidate.Shown {
			shown++
		}
	}
	if shown > 8 {
		return out, ErrValidation
	}
	out.Signature = signature
	out.Snapshot = in.Snapshot
	err = tx.QueryRow(ctx, `INSERT INTO source_name_candidate_preparations(exposure_id,suggestion_id,signature,snapshot,provenance,versions,eligible,eligibility_reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, in.ExposureID, id, signature, ev.candidateSnapshot, mustJSON(provenance), mustJSON(c.Versions), eligible, reason).Scan(&out.ID)
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `UPDATE source_name_review_exposures SET sealed_at=clock_timestamp() WHERE id=$1`, in.ExposureID); err != nil {
		return out, err
	}
	settings := s.nameJudgeSettings()
	if settings.Mode == "shadow" && s.NameJudge != nil && s.Jobs != nil && sampledNameShadow(id, settings.Sample) && len(in.Snapshot) > 0 && c.Frame != nil && nameJudgeIdentity().Provider == "liquid" {
		// The optional outbox must not discard a valid initial pool. PostgreSQL
		// errors are isolated with a savepoint; failure remains a missing request.
		nested, nestedErr := tx.Begin(ctx)
		if nestedErr != nil {
			return out, nestedErr
		}
		if enqueueErr := s.enqueueFrozenNameJudge(ctx, nested, out, c, source); enqueueErr != nil {
			_ = nested.Rollback(ctx)
		} else if err = nested.Commit(ctx); err != nil {
			return out, err
		}
	}
	return out, tx.Commit(ctx)
}

// The new queue withholds unprepared reading text for a registered held-out
// source. A legacy queue exposure is separately recorded and fails eligibility.
func (s *Service) NameReviewQueue(ctx context.Context, fileID, actorID, status string, limit, offset int) ([]SuggestionView, error) {
	views, err := s.ListSuggestions(ctx, fileID, actorID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	var held bool
	err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_name_evaluation_members m JOIN source_name_evaluation_groups g ON g.id=m.group_id WHERE m.source_file_id=$1 AND g.partition='held_out')`, fileID).Scan(&held)
	if err != nil {
		return nil, err
	}
	if held {
		for i := range views {
			views[i].SuggestedTextAR = ""
			views[i].Alternatives = []string{}
		}
	}
	return views, nil
}
func (s *Service) RecordLegacyNameExposure(ctx context.Context, views []SuggestionView) error {
	ids := []uuid.UUID{}
	for _, v := range views {
		if v.Status == "suggested" {
			if id, err := uuid.Parse(v.ID); err == nil {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO source_name_untracked_exposures(suggestion_id) SELECT unnest($1::uuid[]) ON CONFLICT(suggestion_id) DO NOTHING`, ids)
	return err
}
