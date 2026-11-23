// Package regionworkbench is the tile-task lane over a plate's proposals
// (plan 030-F, the Tasking-Manager pattern at plate scale). A plate square
// holding proposals becomes a tile task; a reviewer claims the tile, reads its
// crops through the 030-E candidate lane, and submits; a second reviewer
// independently does the same; the tile retires when two passes by different
// reviewers agree, and returns to the queue — with the disagreement named —
// when they do not. A claimant can also defer (release) their seat; the
// deferred tile returns through the same claim, and the history keeps the
// round trip (the plan-026 deferral shape).
//
// The workbench writes bookkeeping only: statuses, claims, events, and
// submit-time snapshots derived from the candidate lane's own decision
// history. It never writes a reading, never re-decides a candidate, and never
// promotes anything — every candidate-level answer still travels the 030-E
// decide surface, and nothing here renders to readers.
package regionworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("region workbench database is unavailable")
	ErrUnauthenticated     = errors.New("region workbench requires a signed-in session")
	ErrForbidden           = errors.New("region workbench is moderator-readable")
	ErrNotFound            = errors.New("tile task not found")
	ErrValidation          = errors.New("region workbench input is invalid")
	// ErrConflict is an action a tile's current state cannot take: claiming a
	// seated tile, submitting without the seat, submitting a retired tile.
	ErrConflict = errors.New("tile task is not in a state for that action")
	// ErrSelfReview is the two-person retirement refusing one person counted
	// twice: the tile's own submitter cannot claim it back.
	ErrSelfReview = errors.New("a tile cannot be claimed by its own submitter")
	// ErrIncompletePass is a submit whose reviewer left candidates in the tile
	// undecided. It carries the missing ids so the workbench can point at the
	// work, not just refuse.
	ErrIncompletePass = IncompletePassError{}
)

// IncompletePassError names the candidates a submit's pass did not answer.
// Bounded like every list: a tile older than the member cap names the first
// missing ids only.
type IncompletePassError struct {
	Missing []string
}

func (e IncompletePassError) Error() string {
	return "tile pass is incomplete: " + lenMissing(e.Missing) + " candidates untouched"
}

func lenMissing(missing []string) string {
	return strconv.Itoa(len(missing))
}

// Actions are the tile lane's closed verb set, the decision-history table's
// own CHECK. claim/release/submit are a reviewer's moves; confirm and
// invalidate are what the comparison of two passes produced.
const (
	ActionClaim      = "claim"
	ActionRelease    = "release"
	ActionSubmit     = "submit"
	ActionConfirm    = "confirm"
	ActionInvalidate = "invalidate"
)

const (
	ListLimit = 100
	// MaxTileCandidates bounds one tile detail response (the queue grid keeps
	// tiles small by construction; the bound is the response's guarantee, and
	// a tile that grows past it is refused honest service rather than served a
	// silent truncation on a submission).
	MaxTileCandidates = 50
	MaxNoteRunes      = 1000
)

func validStatus(status string) bool {
	switch status {
	case "open", "in_progress", "submitted", "retired":
		return true
	}
	return false
}

// TileBox is the tile's square in the plate's own full-resolution pixels —
// the same shape the candidate lane's crop boxes use.
type TileBox struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type Tile struct {
	ID        string  `json:"id"`
	SourceID  string  `json:"sourceId"`
	PlateKey  string  `json:"plateKey"`
	TileID    string  `json:"tileId"`
	BoxPx     TileBox `json:"boxPx"`
	Status    string  `json:"status"`
	ClaimedBy string  `json:"claimedBy,omitempty"`
	ClaimedAt string  `json:"claimedAt,omitempty"`
	// Passes is the distinct-submitter count the tile's submit events record —
	// the queue shows how far a tile already is without opening it.
	Passes    int    `json:"passes"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type TileListResponse struct {
	Tiles     []Tile `json:"tiles"`
	Status    string `json:"status"`
	Limit     int    `json:"limit"`
	Truncated bool   `json:"truncated"`
}

// TileSummaryResponse is the header's own counts: how many tiles sit in each
// lane today (open waiters, passes awaiting review, retired reads).
type TileSummaryResponse struct {
	Counts   map[string]int `json:"counts"`
	Statuses []string       `json:"statuses"`
}

// TileCandidate is one proposal inside the tile, rendered as the queue shows
// it: what it is, where its box sits, and whether a reviewer has answered it.
// The workbench adds nothing to the answer.
type TileCandidate struct {
	ID           string  `json:"id"`
	OccurrenceID string  `json:"occurrenceId"`
	CropPx       CropBox `json:"cropPx"`
	Status       string  `json:"status"`
	LabelTextAR  string  `json:"labelTextAr"`
	LabelReadBy  string  `json:"labelReadBy,omitempty"`
	DecidedBy    string  `json:"decidedBy,omitempty"`
	AnsweredByMe bool    `json:"answeredByMe"`
}

// CropBox mirrors the candidate lane's box; the workbench renders it but the
// candidate lane owns its meaning.
type CropBox struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type TileDetailResponse struct {
	Tile        Tile            `json:"tile"`
	ClaimedByMe bool            `json:"claimedByMe"`
	Candidates  []TileCandidate `json:"candidates"`
	Limit       int             `json:"limit"`
	Truncated   bool            `json:"truncated"`
}

// SubmitResult is what a submit changed: the tile after, what the comparison
// decided, and the other submitter's id when there was one. A first submit
// carries no agreement; a second carries it complete with the verdict lists.
type SubmitResult struct {
	Tile           Tile      `json:"tile"`
	Action         string    `json:"action"`
	OtherSubmitter string    `json:"otherSubmitter,omitempty"`
	Agreement      Agreement `json:"agreement,omitempty"`
}

type TileEvent struct {
	ID           string `json:"id"`
	TaskID       string `json:"taskId"`
	ReviewerID   string `json:"reviewerId"`
	ReviewerName string `json:"reviewerName,omitempty"`
	Action       string `json:"action"`
	FromStatus   string `json:"fromStatus"`
	ToStatus     string `json:"toStatus"`
	NoteAR       string `json:"noteAr,omitempty"`
	Snapshot     string `json:"snapshot,omitempty"`
	CreatedAt    string `json:"createdAt"`
}

type TileHistoryResponse struct {
	TaskID    string      `json:"taskId"`
	History   []TileEvent `json:"history"`
	Limit     int         `json:"limit"`
	Truncated bool        `json:"truncated"`
}

type ListParams struct {
	Status   string
	PlateKey string
}

type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

func (s *Service) actorModerator(ctx context.Context, actorID string) (uuid.UUID, bool, error) {
	if s.Pool == nil {
		return uuid.Nil, false, ErrDatabaseUnavailable
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil || actorUUID == uuid.Nil {
		return uuid.Nil, false, ErrUnauthenticated
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, actorUUID).Scan(&exists); err != nil {
		return uuid.Nil, false, err
	}
	if !exists {
		return uuid.Nil, false, ErrUnauthenticated
	}
	// The same moderator set every review lane answers to; the workbench is
	// the same adjudication authority, not a new power.
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return uuid.Nil, false, err
	}
	return actorUUID, allowed, nil
}

const tileColumns = `
	t.id::text, t.source_id::text, t.plate_key, t.tile_id, t.box_px, t.status,
	COALESCE(t.claimed_by::text, ''), t.claimed_at,
	(SELECT count(DISTINCT e.reviewer_id) FROM region_tile_task_events e
	   WHERE e.task_id = t.id AND e.action = 'submit'),
	t.created_at, t.updated_at`

func scanTile(scan func(dest ...any) error) (Tile, error) {
	var tile Tile
	var boxBytes []byte
	var box TileBox
	var passes int
	var claimedAt, createdAt, updatedAt pgtype.Timestamptz
	if err := scan(&tile.ID, &tile.SourceID, &tile.PlateKey, &tile.TileID, &boxBytes, &tile.Status,
		&tile.ClaimedBy, &claimedAt, &passes, &createdAt, &updatedAt); err != nil {
		return Tile{}, err
	}
	if err := json.Unmarshal(boxBytes, &box); err != nil {
		return Tile{}, ErrValidation
	}
	tile.BoxPx = box
	tile.Passes = passes
	if claimedAt.Valid {
		tile.ClaimedAt = claimedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	if createdAt.Valid {
		tile.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	if updatedAt.Valid {
		tile.UpdatedAt = updatedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	return tile, nil
}

// List returns one status lane of tile tasks, bounded. Moderator-only. The
// default lane is `open`: the queue of tiles nobody has worked, including the
// tiles an invalidation returned.
func (s *Service) List(ctx context.Context, actorID string, params ListParams) (TileListResponse, error) {
	var err error
	var allowed bool
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return TileListResponse{}, err
	}
	if !allowed {
		return TileListResponse{}, ErrForbidden
	}
	if params.Status == "" {
		params.Status = "open"
	}
	if !validStatus(params.Status) {
		return TileListResponse{}, ErrValidation
	}
	plateKey := strings.TrimSpace(params.PlateKey)

	rows, err := s.Pool.Query(ctx, `
		SELECT `+tileColumns+`
		FROM region_tile_tasks t
		WHERE t.status = $1
		  AND ($2::text = '' OR t.plate_key = $2)
		ORDER BY t.created_at, t.id
		LIMIT $3`, params.Status, plateKey, ListLimit+1)
	if err != nil {
		return TileListResponse{}, err
	}
	defer rows.Close()

	response := TileListResponse{Tiles: []Tile{}, Status: params.Status, Limit: ListLimit}
	for rows.Next() {
		tile, err := scanTile(rows.Scan)
		if err != nil {
			return TileListResponse{}, err
		}
		response.Tiles = append(response.Tiles, tile)
	}
	if err := rows.Err(); err != nil {
		return TileListResponse{}, err
	}
	if len(response.Tiles) > ListLimit {
		response.Tiles = response.Tiles[:ListLimit]
		response.Truncated = true
	}
	return response, nil
}

// Get returns one tile with the proposals inside it, bounded. The membership
// is derived from the crop boxes against the tile's own square (the crop box
// centre falls inside), so a tile never shows a stale member list.
func (s *Service) Get(ctx context.Context, actorID, taskID string) (TileDetailResponse, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return TileDetailResponse{}, err
	}
	if !allowed {
		return TileDetailResponse{}, ErrForbidden
	}
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return TileDetailResponse{}, ErrValidation
	}
	tile, err := s.getTile(ctx, taskUUID)
	if err != nil {
		return TileDetailResponse{}, err
	}
	candidates, truncated, err := s.tileCandidates(ctx, taskUUID)
	if err != nil {
		return TileDetailResponse{}, err
	}
	verdicts, err := s.passVerdicts(ctx, candidates, reviewer)
	if err != nil {
		return TileDetailResponse{}, err
	}
	for i := range candidates {
		_, candidates[i].AnsweredByMe = verdicts[candidates[i].ID]
	}
	return TileDetailResponse{
		Tile:        tile,
		ClaimedByMe: tile.Status == "in_progress" && tile.ClaimedBy == reviewer.String(),
		Candidates:  candidates,
		Limit:       MaxTileCandidates,
		Truncated:   truncated,
	}, nil
}

func (s *Service) getTile(ctx context.Context, id uuid.UUID) (Tile, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+tileColumns+` FROM region_tile_tasks t WHERE t.id = $1`, id)
	tile, err := scanTile(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Tile{}, ErrNotFound
		}
		return Tile{}, err
	}
	return tile, nil
}

// tileCandidates is the membership query shared by Get and Submit: the tile's
// own square, over the candidates' crop-box centres. One row over the bound
// sets the truncated flag and, for submissions, the caller refuses.
func (s *Service) tileCandidates(ctx context.Context, taskID uuid.UUID) ([]TileCandidate, bool, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id::text, c.occurrence_id, c.crop_px, c.status, c.label_text_ar,
		       COALESCE(c.label_read_by::text, ''), COALESCE(c.decided_by::text, '')
		FROM external_region_candidates c
		JOIN region_tile_tasks t ON t.id = $1
		WHERE c.source_id = t.source_id AND c.plate_key = t.plate_key
		  AND (c.crop_px->>'x')::int + (c.crop_px->>'w')::int / 2 >= (t.box_px->>'x')::int
		  AND (c.crop_px->>'x')::int + (c.crop_px->>'w')::int / 2 <  (t.box_px->>'x')::int + (t.box_px->>'w')::int
		  AND (c.crop_px->>'y')::int + (c.crop_px->>'h')::int / 2 >= (t.box_px->>'y')::int
		  AND (c.crop_px->>'y')::int + (c.crop_px->>'h')::int / 2 <  (t.box_px->>'y')::int + (t.box_px->>'h')::int
		ORDER BY c.rank NULLS LAST, c.created_at, c.id
		LIMIT $2`, taskID, MaxTileCandidates+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	candidates := []TileCandidate{}
	for rows.Next() {
		var candidate TileCandidate
		var cropBytes []byte
		if err := rows.Scan(&candidate.ID, &candidate.OccurrenceID, &cropBytes, &candidate.Status,
			&candidate.LabelTextAR, &candidate.LabelReadBy, &candidate.DecidedBy); err != nil {
			return nil, false, err
		}
		if err := json.Unmarshal(cropBytes, &candidate.CropPx); err != nil {
			return nil, false, ErrValidation
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := false
	if len(candidates) > MaxTileCandidates {
		candidates = candidates[:MaxTileCandidates]
		truncated = true
	}
	return candidates, truncated, nil
}

// Claim seats one reviewer on an open (or submitted) tile. While the tile is
// `in_progress` the seat is held: nobody else can claim it. A reviewer who
// already submitted a pass cannot claim the same tile again — their
// submitting twice would be one person counted twice, and that is exactly
// what two-person retirement exists to refuse.
func (s *Service) Claim(ctx context.Context, actorID, taskID string) (Tile, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return Tile{}, err
	}
	if !allowed {
		return Tile{}, ErrForbidden
	}
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return Tile{}, ErrValidation
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Tile{}, err
	}
	defer tx.Rollback(ctx)

	var fromStatus string
	var seatHolder pgtype.UUID
	var previouslySubmitted bool
	// FOR UPDATE serialises the race: two claimants read the seat one after
	// the other, the second sees the first's write.
	err = tx.QueryRow(ctx, `SELECT t.status, t.claimed_by,
			EXISTS (SELECT 1 FROM region_tile_task_events e
			  WHERE e.task_id = t.id AND e.action = 'submit' AND e.reviewer_id = $2)
		FROM region_tile_tasks t WHERE t.id = $1 FOR UPDATE`, taskUUID, reviewer).Scan(&fromStatus, &seatHolder, &previouslySubmitted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Tile{}, ErrNotFound
		}
		return Tile{}, err
	}
	if previouslySubmitted && fromStatus == "submitted" {
		// This reviewer's own pass is the tile's standing, unanswered-by-
		// anyone-else result: claiming it back would be one person counted
		// twice. Once the tile is open again (released, or invalidated by a
		// disagreeing pass) the same reviewer may claim the rework — settling
		// a disagreement is exactly their job.
		return Tile{}, ErrSelfReview
	}
	// Legitimate claims: an unseated tile, or resuming a seat this same
	// reviewer already holds (a re-opened tab is the same work, not a second
	// worker).
	resumingSeat := fromStatus == "in_progress" && seatHolder.Valid && uuid.UUID(seatHolder.Bytes) == reviewer
	if fromStatus != "open" && fromStatus != "submitted" && !resumingSeat {
		return Tile{}, ErrConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE region_tile_tasks SET
		status = 'in_progress', claimed_by = $2, claimed_at = now(), updated_at = now()
	WHERE id = $1 AND status = $3`, taskUUID, reviewer, fromStatus)
	if err != nil {
		return Tile{}, err
	}
	if tag.RowsAffected() != 1 {
		return Tile{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO region_tile_task_events
		(task_id, reviewer_id, action, from_status, to_status, note_ar)
		VALUES ($1, $2, 'claim', $3, 'in_progress', '')`, taskUUID, reviewer, fromStatus); err != nil {
		return Tile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Tile{}, err
	}
	return s.getTile(ctx, taskUUID)
}

// Release defers a held seat: the tile returns to the queue with the note,
// and whoever comes next claims it through the same door (the plan-026
// deferral shape: history keeps the round trip, nothing is lost).
func (s *Service) Release(ctx context.Context, actorID, taskID, note string) (Tile, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return Tile{}, err
	}
	if !allowed {
		return Tile{}, ErrForbidden
	}
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return Tile{}, ErrValidation
	}
	if !validateRuneBound(note, MaxNoteRunes) {
		return Tile{}, ErrValidation
	}
	note = strings.TrimSpace(note)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Tile{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE region_tile_tasks SET
		status = 'open', claimed_by = NULL, claimed_at = NULL, updated_at = now()
	WHERE id = $1 AND status = 'in_progress' AND claimed_by = $2`, taskUUID, reviewer)
	if err != nil {
		return Tile{}, err
	}
	if tag.RowsAffected() != 1 {
		return Tile{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO region_tile_task_events
		(task_id, reviewer_id, action, from_status, to_status, note_ar)
		VALUES ($1, $2, 'release', 'in_progress', 'open', $3)`, taskUUID, reviewer, note); err != nil {
		return Tile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Tile{}, err
	}
	return s.getTile(ctx, taskUUID)
}

// Submit closes one reviewer's pass over a tile. The reviewer's verdicts are
// what they already decided through the candidate lane — the service derives
// them from the decision history and refuses an incomplete pass by naming
// the untouched candidates. A first submit leaves the tile `submitted` (one
// pass recorded, its seat released so the second pass can claim); a second
// submit compares the passes: agreement retires the tile, divergence or
// shared deferral returns it to the queue with the difference named.
func (s *Service) Submit(ctx context.Context, actorID, taskID, note string) (SubmitResult, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return SubmitResult{}, err
	}
	if !allowed {
		return SubmitResult{}, ErrForbidden
	}
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return SubmitResult{}, ErrValidation
	}
	if !validateRuneBound(note, MaxNoteRunes) {
		return SubmitResult{}, ErrValidation
	}
	note = strings.TrimSpace(note)

	// The seat check first: a submit belongs to the tile's current claimant,
	// no one else.
	var fromStatus string
	err = s.Pool.QueryRow(ctx, `SELECT status FROM region_tile_tasks
		WHERE id = $1 AND claimed_by = $2`, taskUUID, reviewer).Scan(&fromStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmitResult{}, ErrConflict
		}
		return SubmitResult{}, err
	}
	if fromStatus != "in_progress" {
		return SubmitResult{}, ErrConflict
	}

	// The tile's membership, over the bound: a tile that grew past the
	// response cap cannot be honestly submitted from a bounded read, so the
	// grid needs rebuilding — refused, never silently truncated into a pass.
	candidates, truncated, err := s.tileCandidates(ctx, taskUUID)
	if err != nil {
		return SubmitResult{}, err
	}
	if truncated {
		return SubmitResult{}, ErrValidation
	}

	myVerdicts, err := s.passVerdicts(ctx, candidates, reviewer)
	if err != nil {
		return SubmitResult{}, err
	}
	// Completeness: a pass answers every proposal in the tile, even the
	// answer being "defer". The missing ones are named, bounded.
	missing := []string{}
	for _, candidate := range candidates {
		if _, answered := myVerdicts[candidate.ID]; !answered {
			missing = append(missing, candidate.ID)
		}
	}
	if len(missing) > 0 {
		return SubmitResult{}, IncompletePassError{Missing: missing}
	}

	// The other submitter, when there is one: the tile's latest submit event
	// by a different reviewer. (A tile reaches this line only once per pass —
	// the seat gates it — so "latest by another" is the whole prior history.)
	otherSubmitter := uuid.Nil
	var priorSnapshot []byte
	err = s.Pool.QueryRow(ctx, `SELECT e.reviewer_id, e.snapshot FROM region_tile_task_events e
		WHERE e.task_id = $1 AND e.action = 'submit' AND e.reviewer_id <> $2
		ORDER BY e.sequence_number DESC LIMIT 1`, taskUUID, reviewer).Scan(&otherSubmitter, &priorSnapshot)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return SubmitResult{}, err
	}

	snapshot := map[string]any{
		"verdicts":    verdictSlice(candidates, myVerdicts),
		"byCandidate": myVerdicts,
	}
	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		return SubmitResult{}, err
	}

	result := SubmitResult{Action: ActionSubmit}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SubmitResult{}, err
	}
	defer tx.Rollback(ctx)

	if otherSubmitter == uuid.Nil {
		tag, err := tx.Exec(ctx, `UPDATE region_tile_tasks SET
			status = 'submitted', claimed_by = NULL, claimed_at = NULL, updated_at = now()
		WHERE id = $1 AND status = 'in_progress' AND claimed_by = $2`, taskUUID, reviewer)
		if err != nil {
			return SubmitResult{}, err
		}
		if tag.RowsAffected() != 1 {
			return SubmitResult{}, ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO region_tile_task_events
			(task_id, reviewer_id, action, from_status, to_status, note_ar, snapshot)
			VALUES ($1, $2, 'submit', 'in_progress', 'submitted', $3, $4::jsonb)`,
			taskUUID, reviewer, note, string(snapshotBytes)); err != nil {
			return SubmitResult{}, err
		}
	} else {
		// The second pass: compare against the first submitter's verdicts.
		// The pass itself is recorded as a submit event first — every pass is
		// a submit, whatever its comparison then produces — so the pass count
		// and the next other-submitter lookup see the whole history, and the
		// confirm/invalidate records the comparison's own outcome beside it.
		var prior struct {
			ByCandidate Verdicts  `json:"byCandidate"`
			Verdicts    []Verdict `json:"verdicts"`
		}
		if err := json.Unmarshal(priorSnapshot, &prior); err != nil {
			return SubmitResult{}, err
		}
		theirVerdicts := prior.ByCandidate
		if theirVerdicts == nil && len(prior.Verdicts) == len(candidates) {
			// Legacy snapshots stored verdicts in the tile query's order.
			theirVerdicts = Verdicts{}
			for i, candidate := range candidates {
				theirVerdicts[candidate.ID] = prior.Verdicts[i]
			}
		}
		ids := make([]string, len(candidates))
		for i, candidate := range candidates {
			ids[i] = candidate.ID
		}
		agreement := Compare(ids, otherSubmitter.String(), reviewer.String(), theirVerdicts, myVerdicts)
		result.OtherSubmitter = otherSubmitter.String()
		result.Agreement = agreement
		toStatus := "open"
		outcome := ActionInvalidate
		if agreement.Retires {
			toStatus = "retired"
			outcome = ActionConfirm
		}
		tag, err := tx.Exec(ctx, `UPDATE region_tile_tasks SET
			status = $3, claimed_by = NULL, claimed_at = NULL, updated_at = now()
		WHERE id = $1 AND status = 'in_progress' AND claimed_by = $2`, taskUUID, reviewer, toStatus)
		if err != nil {
			return SubmitResult{}, err
		}
		if tag.RowsAffected() != 1 {
			return SubmitResult{}, ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO region_tile_task_events
			(task_id, reviewer_id, action, from_status, to_status, note_ar, snapshot)
			VALUES ($1, $2, 'submit', 'in_progress', $3, $4, $5::jsonb)`,
			taskUUID, reviewer, toStatus, note, string(snapshotBytes)); err != nil {
			return SubmitResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO region_tile_task_events
			(task_id, reviewer_id, action, from_status, to_status, note_ar, snapshot)
			VALUES ($1, $2, $3, 'in_progress', $4, $5, $6::jsonb)`,
			taskUUID, reviewer, outcome, toStatus, note, string(snapshotBytes)); err != nil {
			return SubmitResult{}, err
		}
		result.Action = outcome
	}
	if err := tx.Commit(ctx); err != nil {
		return SubmitResult{}, err
	}

	tile, err := s.getTile(ctx, taskUUID)
	if err != nil {
		return SubmitResult{}, err
	}
	result.Tile = tile
	return result, nil
}

// passVerdicts derives one reviewer's verdicts for a tile's candidates from
// the candidate lane's decision history — their latest decision per
// candidate, with the reading that decision carried in its diff. The
// workbench never re-asks; the history is the record.
func (s *Service) passVerdicts(ctx context.Context, candidates []TileCandidate, reviewer uuid.UUID) (Verdicts, error) {
	if len(candidates) == 0 {
		return Verdicts{}, nil
	}
	ids := make([]string, len(candidates))
	for i, candidate := range candidates {
		ids[i] = candidate.ID
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT ON (d.candidate_id)
		       d.candidate_id::text, d.action,
		       COALESCE(d.diff->'label_text_ar'->>'to', ''),
		       COALESCE(d.diff->'era_label'->>'to', '')
		FROM external_region_candidate_decisions d
		WHERE d.candidate_id = ANY($1::uuid[]) AND d.reviewer_id = $2
		ORDER BY d.candidate_id, d.sequence_number DESC`, ids, reviewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	verdicts := Verdicts{}
	for rows.Next() {
		var candidateID, action, reading, era string
		if err := rows.Scan(&candidateID, &action, &reading, &era); err != nil {
			return nil, err
		}
		// Reopening retracts this reviewer's previous answer until they decide again.
		if action == "reopen" {
			continue
		}
		verdicts[candidateID] = Verdict{Action: action, Reading: reading, Era: era}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return verdicts, nil
}

func verdictSlice(candidates []TileCandidate, verdicts Verdicts) []Verdict {
	ordered := make([]Verdict, 0, len(candidates))
	for _, candidate := range candidates {
		ordered = append(ordered, verdicts[candidate.ID])
	}
	return ordered
}

func validateRuneBound(value string, limit int) bool {
	return len([]rune(value)) <= limit
}

// History returns the tile's append-only trail, bounded: claims, releases,
// submits, the confirm and the invalidations — the same audit discipline the
// candidate and pairing lanes keep.
func (s *Service) History(ctx context.Context, actorID, taskID string) (TileHistoryResponse, error) {
	var err error
	var allowed bool
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return TileHistoryResponse{}, err
	}
	if !allowed {
		return TileHistoryResponse{}, ErrForbidden
	}
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return TileHistoryResponse{}, ErrValidation
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM region_tile_tasks WHERE id = $1)`, taskUUID).Scan(&exists); err != nil {
		return TileHistoryResponse{}, err
	}
	if !exists {
		return TileHistoryResponse{}, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id::text, e.task_id::text, e.reviewer_id::text, COALESCE(u.display_name_ar, ''),
		       e.action, e.from_status, e.to_status, COALESCE(e.note_ar, ''), COALESCE(e.snapshot::text, ''), e.created_at
		FROM region_tile_task_events e
		LEFT JOIN users u ON u.id = e.reviewer_id
		WHERE e.task_id = $1
		ORDER BY e.sequence_number
		LIMIT $2`, taskUUID, ListLimit+1)
	if err != nil {
		return TileHistoryResponse{}, err
	}
	defer rows.Close()
	response := TileHistoryResponse{TaskID: taskUUID.String(), History: []TileEvent{}, Limit: ListLimit}
	for rows.Next() {
		event := TileEvent{}
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&event.ID, &event.TaskID, &event.ReviewerID, &event.ReviewerName, &event.Action,
			&event.FromStatus, &event.ToStatus, &event.NoteAR, &event.Snapshot, &createdAt); err != nil {
			return TileHistoryResponse{}, err
		}
		if createdAt.Valid {
			event.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		response.History = append(response.History, event)
	}
	if err := rows.Err(); err != nil {
		return TileHistoryResponse{}, err
	}
	if len(response.History) > ListLimit {
		response.History = response.History[:ListLimit]
		response.Truncated = true
	}
	return response, nil
}

// Summary returns bounded per-status counts of tile tasks: one grouped query,
// four numbers, no per-lane scans. The workbench's header shows progress and
// its lanes know their size; none of it renders to readers.
func (s *Service) Summary(ctx context.Context, actorID string) (TileSummaryResponse, error) {
	allowed := false
	var err error
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return TileSummaryResponse{}, err
	}
	if !allowed {
		return TileSummaryResponse{}, ErrForbidden
	}

	rows, err := s.Pool.Query(ctx, `SELECT status, count(*)::int FROM region_tile_tasks GROUP BY status`)
	if err != nil {
		return TileSummaryResponse{}, err
	}
	defer rows.Close()

	response := TileSummaryResponse{Counts: map[string]int{}, Statuses: []string{"open", "in_progress", "submitted", "retired"}}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return TileSummaryResponse{}, err
		}
		response.Counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return TileSummaryResponse{}, err
	}
	return response, nil
}
