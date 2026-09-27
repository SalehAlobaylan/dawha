// Package embeddingbackfill writes embeddings onto source passages that do not
// have one, so the vector leg in internal/research has something to score.
//
// # Why this is not the processing worker
//
// `retrieveVectorPassages` in internal/research/retrieval.go requires
// `sp.embedding IS NOT NULL`, and only one writer in this repository populated
// that column: `internal/sourceprocessing/processor.go`, which stores the
// embedding of a page it has just extracted out of an uploaded file. That writer
// cannot be pointed at seeded passages. It looks its work up through
// `source_processing_runs` joined to `source_files` (processor.go:206-238),
// the seed creates neither, and it INSERTs a fresh passage per page rather than
// updating an existing one (processor.go:449). Making the worker backfill
// existing passages would be a change to the production processing path made for
// the benefit of a measurement, which is the wrong direction for a change to
// travel.
//
// So this is route (b) of plan 014: a small, explicit backfill that calls the
// SAME contract the worker calls. It uses the same `ai.Client.Embed` with the
// same dimension constant (sourceprocessing.EmbeddingDimensions), the same
// Arabic normalization the worker applies before embedding
// (`identity.NormalizeArabicName`, processor.go:254), and the same four
// refusals the worker applies to what comes back (processor.go:262-276). The
// only thing it does differently is which rows it writes.
//
// # What it promises
//
//   - Rows whose embedding is already set are never touched, so a second run is
//     a no-op and a rerun after a partial failure resumes rather than re-embeds.
//   - It refuses to finish while any passage in scope still has no embedding,
//     and it reads `vector_dims` back for everything it wrote. A backfill that
//     reports success over a half-embedded corpus is worse than one that fails:
//     the failure it hides is a measurement coming out wrong rather than out.
//   - A passage whose `normalized_text_ar` is not what normalizing `text_ar`
//     produces is refused, not guessed at. That column is what a query is
//     compared against and what the worker embeds, so a disagreement means it
//     is not knowable which of the two a query would ever match.
//   - The scope is explicit and reported. `SeedOnly` is the corpus the seed
//     created, identified by the `{"synthetic":true}` marker every seeded source
//     carries, so this cannot silently rewrite a developer's own rows.
package embeddingbackfill

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// Dimensions is the width every embedding in this column has, and the value the
// service asks the provider for. It is the worker's constant, not a second one:
// two constants would be two numbers that could disagree.
const Dimensions = sourceprocessing.EmbeddingDimensions

// ErrIncomplete is what a run returns when it finished its own writes and the
// scope still holds an unembedded passage. It is a distinct error so a caller
// can tell "the backfill broke" from "the backfill worked and there is more to
// do", which are different situations and neither of them is success.
var ErrIncomplete = errors.New("passages in scope still have no embedding")

// Coverage is the state of a scope before and after a run.
type Coverage struct {
	Total    int `json:"total"`
	Embedded int `json:"embedded"`
	Missing  int `json:"missing"`
}

// Service embeds passages through the AI provider. The provider is the
// `ai.Provider` interface rather than a concrete client so a test can hand this
// package a provider that returns a deliberately wrong vector - which is the
// only way to prove the refusals below are refusals and not comments.
type Service struct {
	Pool     *pgxpool.Pool
	Provider ai.Provider
	// Dimensions is the width to ask for and to verify. Zero means Dimensions.
	Dimensions int
}

// New builds a service over a pool and a provider.
func New(pool *pgxpool.Pool, provider ai.Provider) *Service {
	return &Service{Pool: pool, Provider: provider, Dimensions: Dimensions}
}

func (s *Service) dimensions() int {
	if s.Dimensions > 0 {
		return s.Dimensions
	}
	return Dimensions
}

// Scope selects which passages a run covers. It is a type rather than a boolean
// so the three places that need it - Coverage, Pending and the verification
// after a write - cannot disagree about which rows are in play.
type Scope struct {
	all bool
}

// SeedScope is the corpus db/seeds/ created: every passage whose source carries
// the `{"synthetic":true}` marker the seed writes into `sources.metadata`.
//
// It is a marker rather than a list of ids so adding a seed file does not mean
// remembering to update a list, and so pointing this at a database with real
// work in it does not rewrite the real work.
func SeedScope() *Scope { return &Scope{} }

// AllScope is every passage in the database, whatever created it.
func AllScope() *Scope { return &Scope{all: true} }

func (s *Scope) predicate() string {
	if s.all {
		return "true"
	}
	return "COALESCE(src.metadata->>'synthetic', 'false') = 'true'"
}

// String names the scope for a log line, so the recorded output of a run says
// which of the two scopes it covered.
func (s *Scope) String() string { return s.description() }

func (s *Scope) description() string {
	if s.all {
		return "every passage in the database"
	}
	return "the seeded corpus (sources marked synthetic)"
}

// SeedOnly reports whether this scope is the seeded corpus rather than
// everything. It is reported in the command's output so a reader can tell which
// of the two a recorded run used.
func (s *Scope) SeedOnly() bool { return !s.all }

const coverageQuery = `
SELECT count(*), count(sp.embedding)
FROM source_passages sp
JOIN sources src ON src.id = sp.source_id
WHERE `

// Coverage reports how much of a scope carries an embedding.
func (s *Service) Coverage(ctx context.Context, scope *Scope) (Coverage, error) {
	if s == nil || s.Pool == nil {
		return Coverage{}, errors.New("embedding backfill requires a database pool")
	}
	var state Coverage
	if err := s.Pool.QueryRow(ctx, coverageQuery+scope.predicate()).Scan(&state.Total, &state.Embedded); err != nil {
		return Coverage{}, err
	}
	state.Missing = state.Total - state.Embedded
	return state, nil
}

// Passage is one row that needs an embedding.
type Passage struct {
	ID         string
	Text       string
	Normalized string
}

// Pending lists the passages in a scope that still have no embedding, in a
// stable order so a resumed run embeds them in the same sequence.
func (s *Service) Pending(ctx context.Context, scope *Scope) ([]Passage, error) {
	if s == nil || s.Pool == nil {
		return nil, errors.New("embedding backfill requires a database pool")
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT sp.id::text, sp.text_ar, sp.normalized_text_ar
		FROM source_passages sp
		JOIN sources src ON src.id = sp.source_id
		WHERE sp.embedding IS NULL AND `+scope.predicate()+`
		ORDER BY sp.source_id, sp.sequence_number, sp.id
	`)
	if err != nil {
		return nil, err
	}
	pending := make([]Passage, 0, 64)
	for rows.Next() {
		var item Passage
		var text, normalized pgtype.Text
		if err := rows.Scan(&item.ID, &text, &normalized); err != nil {
			rows.Close()
			return nil, err
		}
		item.Text = text.String
		item.Normalized = normalized.String
		pending = append(pending, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pending, nil
}

// Result is what one run did.
type Result struct {
	Scope   string `json:"scope"`
	Before  Coverage
	Written int `json:"written"`
	After   Coverage
	Model   string `json:"model,omitempty"`
	// Verify is the per-passage check of what landed, kept in the result rather
	// than in a log line so a caller that wants to assert on it can.
	Verified int `json:"verified"`
}

// Run embeds every pending passage in the scope and then refuses to report
// success unless the scope is fully embedded and every row it wrote holds a
// vector of the right width.
func (s *Service) Run(ctx context.Context, scope *Scope) (Result, error) {
	result := Result{Scope: scope.description()}
	if s == nil || s.Pool == nil || s.Provider == nil {
		return result, errors.New("embedding backfill requires a database pool and an AI provider")
	}
	before, err := s.Coverage(ctx, scope)
	if err != nil {
		return result, err
	}
	result.Before = before

	pending, err := s.Pending(ctx, scope)
	if err != nil {
		return result, err
	}
	for _, item := range pending {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		normalized, err := s.embeddingText(item)
		if err != nil {
			return result, err
		}
		vector, model, err := s.embed(ctx, normalized)
		if err != nil {
			return result, fmt.Errorf("passage %s: %w", item.ID, err)
		}
		if err := s.write(ctx, item.ID, vector); err != nil {
			return result, err
		}
		if result.Model == "" {
			result.Model = model
		}
		result.Written++
		result.Verified++
	}

	after, err := s.Coverage(ctx, scope)
	if err != nil {
		return result, err
	}
	result.After = after
	if after.Missing != 0 {
		return result, fmt.Errorf("%w: %d of %d passages in scope have no embedding, so this run did not complete the backfill", ErrIncomplete, after.Missing, after.Total)
	}
	return result, nil
}

// embeddingText is the text that gets embedded, and the refusal that stops a
// passage whose two text columns disagree.
//
// The worker embeds the NORMALIZED text, not the raw page (processor.go:254-258),
// and a query is normalized before it is embedded too (rag_service.go:61-62). A
// vector built from the raw text is a vector the query path can never reproduce,
// so this is load-bearing rather than cosmetic. When the stored normalized text
// is present it must equal what the normalizer produces from the raw text: a
// column that says something else is a row whose matchability is unknown, and
// an unknown is not something to guess at.
func (s *Service) embeddingText(item Passage) (string, error) {
	recomputed := identity.NormalizeArabicName(item.Text)
	if item.Normalized == "" {
		if recomputed == "" {
			return "", fmt.Errorf("neither text_ar nor normalized_text_ar has content to embed")
		}
		return recomputed, nil
	}
	if recomputed != "" && recomputed != item.Normalized {
		return "", fmt.Errorf("normalized_text_ar is %q but normalizing text_ar gives %q; the column and the text disagree, so which one a query would match is not knowable", item.Normalized, recomputed)
	}
	return item.Normalized, nil
}

// embed applies the worker's refusals to whatever the provider returned, so a
// vector this package stores is a vector the worker would have stored: the
// right width, finite, and above a zero norm. pgvector will store a zero vector
// happily, which is precisely why it is refused - `<=>` against it is
// undefined and the row would look embedded while scoring nothing.
func (s *Service) embed(ctx context.Context, normalized string) ([]float32, string, error) {
	response, err := s.Provider.Embed(ctx, ai.EmbeddingRequest{Text: normalized, Dimensions: s.dimensions()})
	if err != nil {
		return nil, "", err
	}
	if len(response.Embedding) != s.dimensions() {
		return nil, "", fmt.Errorf("the provider returned %d dimensions and %d were requested", len(response.Embedding), s.dimensions())
	}
	values := make([]float32, len(response.Embedding))
	norm := 0.0
	for index, value := range response.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, "", fmt.Errorf("the provider returned a non-finite value at index %d", index)
		}
		values[index] = float32(value)
		norm += value * value
	}
	if norm <= 1e-12 {
		return nil, "", errors.New("the provider returned a zero-length vector")
	}
	return values, response.Model, nil
}

// write stores the vector and proves what landed.
//
// The dimension is read back with `vector_dims` rather than assumed from the
// value that was sent, because "the write succeeded" and "the column holds a
// 1536-dimension vector" are different claims, and only the second is what this
// package exists to establish. The `embedding IS NULL` guard in the WHERE
// clause is what makes a concurrent second run a no-op instead of a duplicate
// write, and a run that loses the race says so rather than counting a row it
// did not write.
func (s *Service) write(ctx context.Context, id string, vector []float32) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE source_passages SET embedding = $2 WHERE id = $1 AND embedding IS NULL`, id, pgvector.NewVector(vector))
	if err != nil {
		return fmt.Errorf("write the embedding for passage %s: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("passage %s: the guarded update touched %d rows, wanted exactly 1", id, tag.RowsAffected())
	}
	var present bool
	var dimensions int
	if err := s.Pool.QueryRow(ctx, `SELECT embedding IS NOT NULL, COALESCE(vector_dims(embedding), 0) FROM source_passages WHERE id = $1`, id).Scan(&present, &dimensions); err != nil {
		return fmt.Errorf("verify the embedding for passage %s: %w", id, err)
	}
	if !present {
		return fmt.Errorf("passage %s: the row reports no embedding after the write", id)
	}
	if dimensions != s.dimensions() {
		return fmt.Errorf("passage %s: the stored vector has %d dimensions, wanted %d", id, dimensions, s.dimensions())
	}
	return nil
}
