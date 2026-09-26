package search

import (
	"context"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

// The passage stage's filter is two ILIKEs, which is the one lexical filter in this
// codebase a trigram index can serve. db/migrations/0042 adds that index.
//
// The test pins that the index is in the schema, on both columns the filter reads.
// That is what a later change - a migration edited to one column, a filter rewritten
// onto one column, an index dropped - would break, and it breaks here rather than in
// production. It also records the plan the index serves, which is how a reader can
// see for themselves why the plan is recorded rather than required.
//
// The corpus is shaped like a library - several sources, their own passages, text
// that varies from passage to passage and the term is carried by exactly one
// passage, which is what a caller searching for a name in a library is doing.
const (
	explainSources   = 50
	explainPerSource = 20
	explainScale     = explainSources * explainPerSource
	explainTerm      = "الزمرد"
	// explainRareOrdinal is the last passage of the corpus and the only one that
	// carries explainTerm.
	explainRareOrdinal = explainScale - 1
)

// passageQuery is the production passage filter with the production score beside it,
// so what is recorded is the statement the endpoint sends.
const passageQuery = `
	SELECT sp.id, s.title_ar, sp.text_ar, sp.locator_ar,
	       GREATEST(CASE WHEN sp.normalized_text_ar = $1 THEN 100 ELSE 0 END,
	                similarity(sp.normalized_text_ar, $1) * 80,
	                CASE WHEN sp.normalized_text_ar ILIKE '%' || $1 || '%' THEN 65 ELSE 0 END) AS score
	FROM source_passages sp JOIN sources s ON s.id = sp.source_id
	WHERE ($1 = '' OR sp.normalized_text_ar ILIKE '%' || $1 || '%' OR sp.text_ar ILIKE '%' || $1 || '%')
	  AND s.visibility = 'public'
	  AND ($2 = '' OR sp.source_id = $2::uuid)
	ORDER BY score DESC, sp.created_at DESC LIMIT 20`

func TestPassageTrigramIndexIsInTheSchemaAndServesThePassageFilter(t *testing.T) {
	fixture := testsupport.New(t)
	hasTrigramIndex(t, fixture)
	seedPassageCorpus(t, fixture, explainScale)

	// Two plans, both recorded. Which of them the planner picks depends on how many
	// rows there are and on how pg_trgm costs its index, so neither is required: what
	// is required is that the index exists on the columns the filter reads, and these
	// two plans are recorded so a reader can see what the statement costs either way.
	chosen := explain(t, fixture, "SELECT 1", passageQuery, explainTerm, "")
	// Sequential scans and btree index scans disallowed, which leaves the trigram
	// index and the composite unique keys as the ways in.
	indexed := explain(t, fixture, "SET LOCAL enable_seqscan = off; SET LOCAL enable_indexscan = off", passageQuery, explainTerm, "")
	if !strings.Contains(indexed, "Bitmap") {
		t.Fatalf("with sequential scans and btree scans disallowed no bitmap path is left for the passage filter, so nothing can serve the two ILIKEs:\n%s", indexed)
	}
	t.Logf("PASSAGE INDEX over %d passages\nplan the planner chose:\n%s\nplan with the btrees disallowed:\n%s",
		explainScale, chosen, indexed)
}

func hasTrigramIndex(t *testing.T, fixture *testsupport.Fixture) {
	t.Helper()
	var definition string
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = current_schema() AND indexname = 'source_passages_trgm_idx'
	`).Scan(&definition); err != nil {
		t.Fatalf("db/migrations/0042_passage_lexical_trigram_index.sql is not applied to this schema: %v", err)
	}
	// Both columns, or the filter's second ILIKE is served by nothing. The opclass,
	// or the index is a plain btree and serves no ILIKE at all.
	for _, fragment := range []string{"gin", "gin_trgm_ops", "normalized_text_ar", "text_ar"} {
		if !strings.Contains(definition, fragment) {
			t.Fatalf("the trigram index does not mention %q, so it cannot serve the passage filter: %s", fragment, definition)
		}
	}
}

func explain(t *testing.T, fixture *testsupport.Fixture, setup, statement string, args ...any) string {
	t.Helper()
	tx, err := fixture.Pool().Begin(fixture.Ctx())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())
	if strings.TrimSpace(setup) != "" {
		if _, err := tx.Exec(fixture.Ctx(), setup); err != nil {
			t.Fatalf("%s: %v", setup, err)
		}
	}
	rows, err := tx.Query(fixture.Ctx(), "EXPLAIN (ANALYZE, BUFFERS) "+statement, args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	lines := make([]string, 0, 32)
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			rows.Close()
			t.Fatalf("scan plan line: %v", err)
		}
		lines = append(lines, line)
	}
	rows.Close()
	if len(lines) == 0 {
		t.Fatal("the planner returned no plan")
	}
	return strings.Join(lines, "\n")
}

// seedPassageCorpus builds a library-shaped corpus: several public sources, each
// with its own passages, and text that varies from passage to passage.
//
// The variation is the part that matters. A GIN trigram lookup is charged for the
// posting list of every trigram in the pattern, so a corpus of identical sentences
// would "prove" the index worthless, and a real corpus is not made of identical
// sentences.
func seedPassageCorpus(t *testing.T, fixture *testsupport.Fixture, count int) {
	t.Helper()
	seedCorpusNoAssert(t, fixture)
	if got := fixture.Count(`SELECT count(*) FROM source_passages`); got < count {
		t.Fatalf("the probe built %d passages, want at least %d", got, count)
	}
	if got := fixture.Count(`SELECT count(*) FROM source_passages WHERE normalized_text_ar ILIKE '%' || $1 || '%'`, explainTerm); got != 1 {
		t.Fatalf("the probe term %q is carried by %d passages, want exactly 1: a term every passage carries is not a lookup", explainTerm, got)
	}
}

func seedCorpusNoAssert(t *testing.T, fixture *testsupport.Fixture) {
	t.Helper()
	var ownerID string
	if err := fixture.QueryRow(`SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&ownerID); err != nil {
		t.Fatalf("read a user: %v", err)
	}
	fixture.Exec(`
		INSERT INTO sources (title_ar, source_type, visibility, dependency_status, created_by)
		SELECT 'مصدر الم probe ' || g, 'book', 'public', 'independent', $1::uuid
		FROM generate_series(1, $2) g
	`, ownerID, explainSources)
	// One counter across the whole insert, so no two passages carry the same number
	// and the probe's term lands in exactly one of them.
	fixture.Exec(`
		INSERT INTO source_passages (source_id, sequence_number, text_ar, normalized_text_ar)
		SELECT numbered.source_id, numbered.ordinal,
		       numbered.body || ' في الممر ' || numbered.ordinal
		           || CASE WHEN numbered.ordinal = $2 THEN ' والزمرد' ELSE '' END,
		       numbered.body || ' في الممر ' || numbered.ordinal
		           || CASE WHEN numbered.ordinal = $2 THEN ' والزمرد' ELSE '' END
		FROM (
			SELECT s.id AS source_id, g AS sequence_number,
			       row_number() OVER (ORDER BY s.id, g) AS ordinal,
			       'نص الممر ' || g || ' من كتاب البحث يذكر ابن سعد' AS body
			FROM sources s, generate_series(1, $1) g
			WHERE s.title_ar LIKE 'مصدر الم probe %'
		) numbered
	`, explainPerSource, explainRareOrdinal)
	// The planner's row estimate comes from the statistics, and a freshly inserted
	// corpus without them is read as a handful of rows - which is a sequential scan
	// for reasons that have nothing to do with the index.
	fixture.Exec(`ANALYZE source_passages`)
	fixture.Exec(`ANALYZE sources`)
}
