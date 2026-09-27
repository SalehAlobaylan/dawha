package researchagent

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// Two defects in this package shared one cause: a step's report was assembled
// from the arguments it was handed rather than from what the database did with
// them. `search_sources` was handed a NULL source id and read it as "exclude
// everything"; `persistStage` was handed a slice of references and counted it
// without asking which of them survived the unique constraint. Both look correct
// in the Go source and are wrong in the answer a reader gets.
//
// The two tests below run the real code against a real PostgreSQL. Neither of
// them can be satisfied by the query or the insert being "obviously right":
// search_sources has to return rows, and the step's count has to equal the rows
// that exist.

// qualifiedSourceStatement seeds one accepted statement on a public,
// independent, dependency-free source: exactly the shape search_sources is
// allowed to return.
func qualifiedSourceStatement(t *testing.T, fixture *testsupport.Fixture, title, text string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	var sourceID uuid.UUID
	if err := fixture.QueryRow(`
		INSERT INTO sources (title_ar, source_type, visibility, dependency_status)
		VALUES ($1, 'manuscript', 'public', 'independent') RETURNING id
	`, "مصدر مؤهل "+fixture.Unique("t")).Scan(&sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	var statementID uuid.UUID
	if err := fixture.QueryRow(`
		INSERT INTO source_statements (source_id, statement_text_ar, review_status)
		VALUES ($1, $2, 'accepted') RETURNING id
	`, sourceID, text).Scan(&statementID); err != nil {
		t.Fatalf("insert source statement: %v", err)
	}
	return sourceID, statementID
}

// TestSearchSourcesReadsAnAbsentSourceIDAsNoSourceConstraint is the pin for the
// NULL. The stage used to filter with a comparison against the empty string, and
// a NULL parameter makes that comparison NULL, so a run scoped to a person with
// no source id found no source statements at all - it answered "there is
// nothing here" about material it had never been asked to exclude.
//
// The three assertions are the three directions: with the source id, the
// statement is found; with no source id, it is still found; with SOME OTHER
// source id, it is not. The third one is what stops the fix from being "drop the
// filter", which would pass the first two.
func TestSearchSourcesReadsAnAbsentSourceIDAsNoSourceConstraint(t *testing.T) {
	fixture := testsupport.New(t)
	sourceID, statementID := qualifiedSourceStatement(t, fixture, "سجل",

		// The term has to be one the decomposer would actually keep: three runes
		// or more, so this is the shape a real question produces.
		"ذكرت الهججرة موضع المدينة في الأحساء.")
	// A second qualified source, so "the stage found material" cannot be satisfied
	// by the stage returning the one statement it was pointed at.
	otherSourceID, otherStatementID := qualifiedSourceStatement(t, fixture, "سجل آخر", "ذكرت الهججرة موضع المدينة في الأحساء.")
	ctx := fixture.Ctx()

	terms := []string{"الهججرة"}
	base := stageContext{
		ActorUUID: uuid.New(),
		Terms:     terms,
		Input:     RunInput{Question: "أين كانت الهجرة؟", EntityType: "person"},
	}

	scoped := base
	scoped.Input.SourceID = sourceID.String()
	withSource, err := searchSources(ctx, fixture.Pool(), scoped)
	if err != nil {
		t.Fatalf("search scoped to the source: %v", err)
	}
	if len(withSource) != 1 || withSource[0].ReferenceID != statementID.String() {
		t.Fatalf("scoped to its own source, the stage returned %+v, want the one statement it was pointed at", withSource)
	}

	// The defect. No source id at all, which arrives at the query as NULL.
	unscoped := base
	unscoped.Input.SourceID = ""
	withoutSource, err := searchSources(ctx, fixture.Pool(), unscoped)
	if err != nil {
		t.Fatalf("search with no source id: %v", err)
	}
	if !cites(withoutSource, statementID.String()) || !cites(withoutSource, otherStatementID.String()) {
		t.Fatalf("a run with no source id found %d statements and did not include both qualified ones: an absent source id means no source constraint, not no material", len(withoutSource))
	}

	// And the constraint is still a constraint.
	elsewhere := base
	elsewhere.Input.SourceID = otherSourceID.String()
	other, err := searchSources(ctx, fixture.Pool(), elsewhere)
	if err != nil {
		t.Fatalf("search scoped to another source: %v", err)
	}
	if cites(other, statementID.String()) {
		t.Fatalf("a run scoped to one source returned another source's statement: %+v", other)
	}
}

func cites(items []EvidenceRef, statementID string) bool {
	for _, item := range items {
		if item.ReferenceID == statementID {
			return true
		}
	}
	return false
}

// TestAStepReportsOnlyTheEvidenceItWrote is the pin for the count.
//
// research_agent_evidence is UNIQUE on (run_id, reference_type, reference_id,
// stance) and that key does not include step_id. Two stages that reach the same
// claim therefore produce one row, carrying the first step's id, and the second
// step used to report two. The count has to come from the inserts.
//
// The fixture deliberately contains a claim that two stages cite: inspect_
// chronology and compare_claims both read the disputed claim, and the run
// asserts the conflict actually happened, so this test cannot pass by finding a
// fixture with nothing to conflict over.
func TestAStepReportsOnlyTheEvidenceItWrote(t *testing.T) {
	fixture := testsupport.New(t)
	ctx := fixture.Ctx()
	pool := fixture.Pool()
	actorID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث العدّ')`, actorID, fixture.Email()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher')`, actorID); err != nil {
		t.Fatal(err)
	}
	questionID := uuid.New()
	personID, placeID, sourceID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, priority, created_by) VALUES ($1, 'ما موضع الهجرة؟', 'open', 'high', $2)`, questionID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, 'عبد الله', 'عبد الله', 'reviewed', $2)`, personID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, created_by) VALUES ($1, 'الأحساء', 'الأحساء', 'city', $2)`, placeID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, dependency_status, visibility, created_by) VALUES ($1, 'مصدر مستقل', 'book', 'independent', 'public', $2)`, sourceID, actorID); err != nil {
		t.Fatal(err)
	}
	statementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'ذكرت الهجرة موضع الأحساء.', 'accepted', $3)`, statementID, sourceID, actorID); err != nil {
		t.Fatal(err)
	}
	// A DISPUTED claim with accepted evidence. Both stages read it: inspect_
	// chronology takes every claim about the subject, and compare_claims takes
	// every claim about the subject and classifies a disputed one as context
	// rather than as support - so both produce (claim, id, context) and the
	// second insert is swallowed.
	disputedClaimID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
		VALUES ($1, 'person', $2, 'lived_in', 'place', $3, 'disputed', 'ادعاء متنازع عليه', $4)
	`, disputedClaimID, personID, placeID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3)`, disputedClaimID, statementID, actorID); err != nil {
		t.Fatal(err)
	}
	// A second claim, SUPPORTED, so compare_claims has a row of its own to write
	// and the collision above is a collision rather than an empty stage.
	supportedClaimID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
		VALUES ($1, 'person', $2, 'lived_in', 'place', $3, 'supported', 'ادعاء مدعوم', $4)
	`, supportedClaimID, personID, placeID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3)`, supportedClaimID, statementID, actorID); err != nil {
		t.Fatal(err)
	}

	queue := jobs.NewService(pool)
	service := NewService(pool).WithQueue(queue)
	accepted, err := service.StartRun(ctx, actorID.String(), RunInput{
		Question:   "أين كانت الهجرة؟",
		QuestionID: questionID.String(),
		EntityType: "person",
		EntityID:   personID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	run := driveAgentRun(t, pool, queue, service, accepted.ID)

	// Every step's reported count is the number of rows that carry its id.
	rows, err := pool.Query(ctx, `SELECT id::text, stage, evidence_count FROM research_agent_steps WHERE run_id = $1 ORDER BY step_order`, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type stepCount struct {
		stage string
		want  int
		got   int
	}
	steps := make([]stepCount, 0, 11)
	for rows.Next() {
		var id, stage string
		var evidenceCount int
		if err := rows.Scan(&id, &stage, &evidenceCount); err != nil {
			t.Fatal(err)
		}
		steps = append(steps, stepCount{stage: stage, want: countRunRows(t, pool, `SELECT count(*) FROM research_agent_evidence WHERE step_id = $1`, id), got: evidenceCount})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 11 {
		t.Fatalf("the run recorded %d steps, want 11", len(steps))
	}
	for _, step := range steps {
		if step.got != step.want {
			t.Fatalf("step %s reported %d evidence items and holds %d rows: a step must count what it wrote, not what it was offered", step.stage, step.got, step.want)
		}
	}

	// The step-level counts and the run-level count are the same evidence seen
	// from two places, so they have to add up to the table.
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM research_agent_evidence WHERE run_id = $1`, accepted.ID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	summed := 0
	for _, step := range steps {
		summed += step.got
	}
	if summed != total {
		t.Fatalf("the step counts sum to %d and the run holds %d evidence rows", summed, total)
	}
	if run.EvidenceCount != total {
		t.Fatalf("the run reports %d evidence items and the table holds %d", run.EvidenceCount, total)
	}

	// And the fixture was the hard case: both stages read the disputed claim, the
	// row exists once, and each stage still wrote evidence of its own - so the
	// counts above are not passing on a run with nothing to collide over.
	holders := countRunRows(t, pool, `SELECT count(*) FROM research_agent_evidence WHERE run_id = $1 AND reference_type = 'claim' AND reference_id = $2 AND stance = 'context'`, accepted.ID, disputedClaimID)
	if holders != 1 {
		t.Fatalf("the disputed claim is cited by %d steps, want the one that wrote the row: this fixture is supposed to make two stages collide", holders)
	}
	rowsPerStep := func(stage string) int {
		return countRunRows(t, pool, `SELECT count(*) FROM research_agent_evidence WHERE step_id = (SELECT id FROM research_agent_steps WHERE run_id = $1 AND stage = $2)`, accepted.ID, stage)
	}
	if rowsPerStep(StageInspectChronology) < 1 || rowsPerStep(StageCompareClaims) < 1 {
		t.Fatalf("both stages are supposed to have written evidence of their own: inspect_chronology holds %d rows, compare_claims holds %d. The fixture no longer produces the collision it was written for",
			rowsPerStep(StageInspectChronology), rowsPerStep(StageCompareClaims))
	}
}
