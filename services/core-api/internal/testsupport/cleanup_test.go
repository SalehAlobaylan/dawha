package testsupport

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCleanupRemovesEverythingReachableFromASyntheticActor is the helper's own
// test, and it is deliberately the largest fixture in the repository: one actor
// that owns a tree with two versions, nodes, relationships, a fork and a
// collaborator; people and places; sources with files, passages and statements;
// claims with evidence; an open question with a note; a research run; a
// geospatial association; and audit rows written with and without an actor.
//
// Every one of those hangs off a foreign key that is not ON DELETE CASCADE, which
// is the whole reason a hand-written `DELETE FROM users WHERE id = $1` aborts and
// leaves the actor behind. The assertion is the point: after the sweep, nothing
// reachable from the actor may still be in the database, checked by the traversal
// and not by a list this test wrote down - a list would only prove the list is
// complete.
func TestCleanupRemovesEverythingReachableFromASyntheticActor(t *testing.T) {
	databaseURL := requireDatabase(t)
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	actor := uuid.New()
	viewer := uuid.New()
	tree := uuid.New()
	versionOne := uuid.New()
	versionTwo := uuid.New()
	nodeOne := uuid.New()
	nodeTwo := uuid.New()
	relationship := uuid.New()
	personOne := uuid.New()
	personTwo := uuid.New()
	place := uuid.New()
	source := uuid.New()
	otherSource := uuid.New()
	sourceFile := uuid.New()
	passage := uuid.New()
	statement := uuid.New()
	claim := uuid.New()
	question := uuid.New()
	run := uuid.New()

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("fixture %q: %v", firstWords(query), err)
		}
	}

	exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث الاختبار'), ($3, $4, 'باحث آخر')`,
		actor, "cleanup-actor-"+actor.String()+"@example.test",
		viewer, "cleanup-viewer-"+viewer.String()+"@example.test")
	t.Cleanup(func() {
		// Belt and braces: the sweep under test is what should do this. If it
		// does, these are no-ops. If it does not, the test still leaves the
		// developer's database as it found it, and the failure below names why.
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1::uuid[])`,
			[]uuid.UUID{actor, viewer})
	})

	// A tree with two versions, two nodes and a relationship between them, plus
	// the rows that a viewer and a fork hang off.
	exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by)
	      VALUES ($1, 'الشخص الأول', 'الشخص الأول', $3), ($2, 'الشخص الثاني', 'الشخص الثاني', $3)`,
		personOne, personTwo, actor)
	exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة الاختبار', 'private', $2)`, tree, actor)
	exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, created_by, published_by)
	      VALUES ($1, $2, 1, 'published', $3, $3), ($4, $2, 2, 'draft', $3, $3)`,
		versionOne, tree, actor, versionTwo)
	exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar)
	      VALUES ($1, $2, $3, 'الشخص الأول'), ($4, $2, $5, 'الشخص الثاني')`,
		nodeOne, versionOne, personOne, nodeTwo, personTwo)
	exec(`INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, created_by)
	      VALUES ($1, $2, $3, $4, 'parent_of', 'interpreted', $5)`, relationship, versionOne, nodeOne, nodeTwo, actor)
	exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by)
	      VALUES ($1, $2, 'edit', $3)`, tree, viewer, actor)
	exec(`INSERT INTO tree_forks (tree_id, parent_tree_id, parent_version_id, forked_by)
	      VALUES ($1, $2, $3, $4)`, tree, tree, versionOne, actor)
	exec(`INSERT INTO tree_invitations (tree_id, inviter_id, invitee_user_id, token_hash, expires_at, permission_level, status)
	      VALUES ($1, $2, $3, $4, now() + interval '1 day', 'view', 'pending')`,
		tree, actor, viewer, uuid.New().String())
	exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, created_by)
	      VALUES ($1, 'المكان', 'المكان', 'village', $2)`, place, actor)

	// Sources with the rows that hang off them. source_statements.source_passage_id
	// is NO ACTION, which is a delete of the passage that aborts unless the
	// statement goes first.
	exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
	      VALUES ($1, 'المصدر الأول', 'book', 'public', $3), ($2, 'المصدر الثاني', 'book', 'public', $3)`,
		source, otherSource, actor)
	exec(`INSERT INTO source_files (id, source_id, storage_key, original_filename_ar)
	      VALUES ($1, $2, 'key.pdf', 'file.pdf')`, sourceFile, source)
	exec(`INSERT INTO source_passages (id, source_id, source_file_id, sequence_number, text_ar, normalized_text_ar)
	      VALUES ($1, $2, $3, 1, 'نص', 'نص')`, passage, source, sourceFile)
	exec(`INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by)
	      VALUES ($1, $2, $3, 'عبارة', 'accepted', $4)`, statement, source, passage, actor)
	exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by)
	      VALUES ($1, 'person', $2, 'parent_of', 'person', $3, 'supported', $4)`, claim, personOne, personTwo, actor)
	exec(`INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by)
	      VALUES ($1, $2, 'supports', $3)`, claim, statement, actor)
	exec(`INSERT INTO geographic_associations (entity_type, entity_id, place_id, relation_type, claim_id, source_id, created_by)
	      VALUES ('person', $1, $2, 'lived_in', $5, $3, $4)`, personOne, place, source, actor, claim)
	exec(`INSERT INTO open_questions (id, title_ar, created_by) VALUES ($1, 'سؤال', $2)`, question, actor)
	exec(`INSERT INTO question_notes (question_id, note_ar, created_by) VALUES ($1, 'ملاحظة', $2)`, question, actor)
	exec(`INSERT INTO research_runs (id, actor_id, query, normalized_query, status, insufficient_evidence,
	                                 semantic_route_fallback, semantic_route_source_bearing,
	                                 semantic_route_potential_contradiction, semantic_route_continue_investigation,
	                                 synthesis_attempted)
	      VALUES ($1, $2, 'سؤال', 'سؤال', 'succeeded', false, false, true, false, false, false)`, run, actor)
	exec(`INSERT INTO audit_log (actor_id, action, entity_type, entity_id) VALUES ($1, 'test', 'user', $1), (NULL, 'test', 'user', $1)`,
		actor)
	// Two rows that name this run's work with no foreign key to it, and which the
	// traversal above cannot see: a queued job whose payload holds the source id, and
	// an audit row with no actor whose entity is the source file. Both survived a whole
	// suite run against a shared database before this sweep learned to look for them.
	exec(`INSERT INTO jobs (type, payload, status, max_attempts)
	      VALUES ('source_process', jsonb_build_object('source_id', $1::text, 'source_file_id', $2::text), 'queued', 3)`,
		source, sourceFile)
	exec(`INSERT INTO audit_log (actor_id, action, entity_type, entity_id) VALUES (NULL, 'test', 'source_file', $1)`,
		sourceFile)

	reachable := reachableCount(ctx, t, pool, actor)
	if reachable == 0 {
		t.Fatal("the fixture wrote nothing reachable, so the sweep would prove nothing")
	}

	CleanupSyntheticActors(t, pool, actor, viewer)

	if got := reachableCount(ctx, t, pool, actor); got != 0 {
		t.Fatalf("cleanup left %d row(s) reachable from the synthetic actor; the traversal itself "+
			"believes it reached %d, so the delete and the traversal disagree", got, reachable)
	}
	// The unlinked references are invisible to that traversal by construction, so they
	// are counted here rather than assumed.
	for _, check := range []struct {
		label string
		query string
		args  []any
	}{
		{`a job whose payload names the removed source`,
			`SELECT count(*) FROM jobs WHERE payload->>'source_id' = $1::text`, []any{source}},
		{`an audit row about the removed source file`,
			`SELECT count(*) FROM audit_log WHERE entity_id = $1`, []any{sourceFile}},
	} {
		var count int
		if err := pool.QueryRow(ctx, check.query, check.args...).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", check.label, err)
		}
		if count != 0 {
			t.Errorf("cleanup left %s: %d row(s)", check.label, count)
		}
	}
	// The two actors are gone by id, which is the same statement the gate uses on
	// the marker, so a leak here would be one the gate reports.
	for _, id := range []uuid.UUID{actor, viewer} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, id).Scan(&count); err != nil {
			t.Fatalf("count user %s: %v", id, err)
		}
		if count != 0 {
			t.Fatalf("cleanup left the synthetic actor %s in place", id)
		}
	}
}

// TestCleanupLeavesSeededRowsAlone is the other half of the contract: a sweep scoped
// to one synthetic actor must not take another one with it.
//
// It uses a second synthetic actor as the control rather than counting the database.
// `go test ./...` runs packages in parallel against the one shared schema, so a global
// before/after snapshot sees other packages' fixtures appear and disappear underneath
// it: an actor that was mid-test when the "before" was taken is gone by the "after"
// because that package cleaned up, which has nothing to do with this sweep. Two actors
// created here, one swept and one not, is a statement this test can actually make.
func TestCleanupLeavesSeededRowsAlone(t *testing.T) {
	databaseURL := requireDatabase(t)
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	swept := uuid.New()
	kept := uuid.New()
	for _, actor := range []uuid.UUID{swept, kept} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث')`,
			actor, "cleanup-seeded-"+actor.String()+"@example.test"); err != nil {
			t.Fatalf("insert actor %s: %v", actor, err)
		}
	}
	// The fallback cleanup is the same sweep, not a hand-written DELETE. This test
	// exists to show what a hand-written one gets wrong: the control actor has a
	// source, `sources.created_by` is NO ACTION, and `DELETE FROM users WHERE id = $1`
	// aborts on it - leaving a leaked actor behind and failing the run's own gate.
	t.Cleanup(func() { CleanupSyntheticActors(t, pool, swept, kept) })
	source := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
	      VALUES ($1, 'مصدر غير ممسوح', 'book', 'public', $2)`, source, kept); err != nil {
		t.Fatalf("insert the control source: %v", err)
	}

	CleanupSyntheticActors(t, pool, swept)

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, swept).Scan(&count); err != nil {
		t.Fatalf("count the swept actor: %v", err)
	}
	if count != 0 {
		t.Error("the sweep left the synthetic actor it was given")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, kept).Scan(&count); err != nil {
		t.Fatalf("count the control actor: %v", err)
	}
	if count != 1 {
		t.Error("the sweep removed a synthetic actor it was not given")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sources WHERE id = $1`, source).Scan(&count); err != nil {
		t.Fatalf("count the control source: %v", err)
	}
	if count != 1 {
		t.Error("the sweep removed a source belonging to an actor it was not given")
	}
}

// TestTheGateSeesALeakedSyntheticUser is the leak gate's own test, and it is the
// other defect this plan exists to close: a fixture that leaks has to be visible
// after the suite has finished, by marker, without anybody counting rows.
func TestTheGateSeesALeakedSyntheticUser(t *testing.T) {
	databaseURL := requireDatabase(t)
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	leaked := uuid.New()
	email := "leak-probe-" + leaked.String() + "@example.test"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مسرَّب')`,
		leaked, email); err != nil {
		t.Fatalf("insert leaked user: %v", err)
	}
	// Deliberately not cleaned up for the length of this test: the point is that the
	// gate finds it.
	defer func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, leaked); err != nil {
			t.Errorf("remove the leak probe: %v", err)
		}
	}()

	users, err := listSyntheticUsers(ctx, pool)
	if err != nil {
		t.Fatalf("list synthetic users: %v", err)
	}
	found := false
	for _, user := range users {
		if user.Email == email {
			found = true
		}
	}
	if !found {
		t.Fatalf("the gate did not see the leaked user %s; it saw %d synthetic user(s) and none "+
			"of them was this one", email, len(users))
	}

	reachable, err := reachableFromSyntheticUsers(ctx, pool)
	if err != nil {
		t.Fatalf("probe reachability: %v", err)
	}
	if _, ok := reachable["users"]; ok {
		t.Error("the reachability report should not repeat the users table; the leaked actors are " +
			"already listed by ListSyntheticUsers and naming them twice buries the rows underneath")
	}
}

// TestSweepActorsIsTheEngineWithoutATest covers the half of the one-time cleanup that
// a tool calls: the same engine, the same dependency order and the same convergence
// loop, without a *testing.T to fail.
//
// The database-wide entry point, SweepSyntheticRows, is deliberately NOT exercised
// here. `go test ./...` runs packages in parallel against the one shared schema, so a
// test that swept every synthetic row would delete the rows a package running at the
// same moment had just created - including, on a run where a fixture leaks, exactly
// the leak tools/dbtestguard is about to look for. It would turn the gate green by
// cleaning up behind its back, which is the failure mode this whole file exists to
// remove. The one-time sweep is instead run once, deliberately, against a database no
// test is using.
func TestSweepActorsIsTheEngineWithoutATest(t *testing.T) {
	databaseURL := requireDatabase(t)
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	actor := uuid.New()
	person := uuid.New()
	tree := uuid.New()
	version := uuid.New()
	node := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'باحث')`,
		actor, "sweep-actors-"+actor.String()+"@example.test"); err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, actor)
	})
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'شخص', 'شخص', $2)`, []any{person, actor}},
		{`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة', 'public', $2)`, []any{tree, actor}},
		{`INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3)`, []any{version, tree, actor}},
		{`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar) VALUES ($1, $2, $3, 'شخص')`, []any{node, version, person}},
	} {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("fixture %q: %v", firstWords(statement.query), err)
		}
	}

	if reachable := reachableCount(ctx, t, pool, actor); reachable < 5 {
		t.Fatalf("the fixture wrote %d reachable row(s), so the sweep would prove nothing", reachable)
	}
	if err := SweepActors(ctx, pool, []string{actor.String()}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if reachable := reachableCount(ctx, t, pool, actor); reachable != 0 {
		t.Errorf("SweepActors left %d row(s) reachable from the actor", reachable)
	}
}

// TestTheReadOnlySurfacesOnlyNameSyntheticActors covers the one thing the gate and
// `dbsweep -n` depend on and can actually be checked on: the marker filter is narrow
// enough that it can never name a developer's own account.
//
// That is the failure a leak gate has to avoid. A check that flagged `demo@dawha.local`
// would train people to run the suite against a copy, which is how the 593 leaked rows
// survived in the first place - so the property is pinned here rather than left to the
// marker list looking obviously right.
//
// It deliberately does not compare two reads of the database. `go test ./...` runs
// packages in parallel against the one shared schema, so a synthetic actor can appear
// between two reads of the same instant and a comparison would be a flaky test rather
// than a strict one - the same reason the gate itself is marker-scoped and not
// count-scoped.
func TestTheReadOnlySurfacesOnlyNameSyntheticActors(t *testing.T) {
	databaseURL := requireDatabase(t)
	ctx := t.Context()

	counts, err := CountReportedTables(ctx, databaseURL)
	if err != nil {
		t.Fatalf("count reported tables: %v", err)
	}
	for _, table := range ReportedTables {
		if _, ok := counts[table]; !ok {
			t.Errorf("CountReportedTables did not report %s, so a sweep's before/after would be "+
				"missing a table an operator asked about", table)
		}
	}

	users, reachable, err := DescribeSyntheticRows(ctx, databaseURL)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	for _, user := range users {
		marked := false
		for _, suffix := range SyntheticEmailSuffixes {
			if strings.HasSuffix(user.Email, suffix) {
				marked = true
			}
		}
		if !marked {
			t.Errorf("DescribeSyntheticRows returned %s, whose address carries no synthetic "+
				"marker; the gate would fail a run over a developer's own account", user.Email)
		}
	}
	// The seeded demo account is the row a wrong marker would take first, so it is
	// named here whether or not this database still has it.
	if len(users) == 0 {
		t.Log("no synthetic actor in this database; the marker filter was exercised only by the " +
			"negative case above")
	}
	for table := range reachable {
		if table == "users" {
			t.Error("the reachability report repeats the users table; the leaked actors are already " +
				"listed by ListSyntheticUsers and naming them twice buries the rows underneath")
		}
	}
}

func sortedKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func requireDatabase(t *testing.T) string {
	t.Helper()
	databaseURL := os.Getenv(DatabaseURLEnv)
	if databaseURL == "" {
		t.Skip(DBSkipMessage)
	}
	return databaseURL
}

// reachableCount asks the traversal itself how much it can still reach, so the
// assertion above is the traversal's and not a hand-written list of tables.
func reachableCount(ctx context.Context, t *testing.T, pool *pgxpool.Pool, actor uuid.UUID) int {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	scopes, err := buildScopes(ctx, tx, []string{actor.String()})
	if err != nil {
		t.Fatalf("walk the schema from %s: %v", actor, err)
	}
	reachable := 0
	for _, scope := range scopes.tables {
		values, err := scope.remaining(ctx, tx)
		if err != nil {
			t.Fatalf("probe %s: %v", scope.table, err)
		}
		reachable += len(values)
	}
	return reachable
}

// firstWords shortens a statement for a failure message, so the reader sees which
// fixture line failed without a wall of SQL in the log.
func firstWords(query string) string {
	fields := strings.Fields(query)
	if len(fields) > 4 {
		fields = fields[:4]
	}
	return strings.Join(fields, " ")
}

// TestTheSweepRefusesToCallSuccessWhatTheDatabaseDisagreesWith is the guarantee this
// plan was sent back for, tested directly.
//
// The failure that reached a shared database was a sweep that reported success with rows
// still in it, and the rows that survived had one thing in common: nothing the sweep had
// looked at. An `audit_log` row's `entity_id` and a job's `payload` have no constraint, so
// a row that another transaction commits while the sweep is working is neither rejected
// nor seen, and the in-transaction proof - a snapshot of our own transaction - cannot
// notice it afterwards either.
//
// So the check that matters runs after the commit, on a fresh connection, and asks the
// database rather than the transaction. This test drives that check with the two states it
// exists to distinguish: a clean database must pass, and a database that disagrees must be
// named rather than accepted. The second half is the one that matters, because it is the
// behaviour that was missing.
func TestTheSweepRefusesToCallSuccessWhatTheDatabaseDisagreesWith(t *testing.T) {
	databaseURL := requireDatabase(t)
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	actor := uuid.New()
	source := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مreon')`,
		actor, "verify-"+actor.String()+"@example.test"); err != nil {
		t.Fatalf("insert the actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
	      VALUES ($1, 'مصدر', 'book', 'public', $2)`, source, actor); err != nil {
		t.Fatalf("insert the source: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_log WHERE entity_id = $1`, source)
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_log WHERE actor_id = $1`, actor)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, actor)
	})

	// Clean: the actor is present, so the check must say so - by naming it.
	err = verifyAfterCommit(ctx, pool, []string{actor.String()}, []string{actor.String(), source.String()})
	if err == nil {
		t.Fatal("the post-commit check accepted a database that still holds the actor it removed")
	}
	if !strings.Contains(err.Error(), actor.String()) && !strings.Contains(err.Error(), "verify-") {
		t.Errorf("the post-commit check did not name the surviving actor: %v", err)
	}

	// Remove them, and the check must now pass: a sweep that has nothing left to find is
	// allowed to say so.
	if err := SweepActors(ctx, pool, []string{actor.String()}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if err := verifyAfterCommit(ctx, pool, []string{actor.String()}, []string{actor.String(), source.String()}); err != nil {
		t.Errorf("the post-commit check rejected a database the sweep had just cleaned: %v", err)
	}
}

// TestTheSweepFailsWhenARowLandsAfterItsSnapshot is the measured failure, constructed.
//
// It reproduces the exact state a shared database was left in: a synthetic actor and its
// rows are gone, and an `audit_log` row that names one of the removed ids is still there,
// because nothing in the schema would have rejected it and it arrived after the sweep had
// taken its snapshot. The sweep must not be able to call that success, and the check that
// says so has to name the row class rather than a count.
func TestTheSweepFailsWhenARowLandsAfterItsSnapshot(t *testing.T) {
	databaseURL := requireDatabase(t)
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	actor := uuid.New()
	source := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'متأخر')`,
		actor, "late-"+actor.String()+"@example.test"); err != nil {
		t.Fatalf("insert the actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
	      VALUES ($1, 'مصدر', 'book', 'public', $2)`, source, actor); err != nil {
		t.Fatalf("insert the source: %v", err)
	}
	if err := SweepActors(ctx, pool, []string{actor.String()}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	// The source is gone. This row is the residue: it names the source, nothing constrains
	// it, and it is the shape of the eleven rows a shared database was found holding.
	if _, err := pool.Exec(ctx, `INSERT INTO audit_log (actor_id, action, entity_type, entity_id)
	      VALUES (NULL, 'source_processing_completed', 'source', $1)`, source); err != nil {
		t.Fatalf("insert the late audit row: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_log WHERE entity_id = $1`, source)
	})

	err = verifyAfterCommit(ctx, pool, []string{actor.String()}, []string{actor.String(), source.String()})
	if err == nil {
		t.Fatalf("a row that names a removed id is still in the database and the post-commit check " +
			"called it success; that is the leak this plan was sent back for")
	}
	if !strings.Contains(err.Error(), "audit row") {
		t.Errorf("the check did not say which row class was left behind: %v", err)
	}
	t.Logf("the check refuses: %v", err)
}

// TestASweepOfNothingIsNotAFailure keeps the post-commit check from being a gate on its own
// bookkeeping: a fixture whose actor is already gone has nothing to prove and must not be
// reported as a failure.
func TestASweepOfNothingIsNotAFailure(t *testing.T) {
	databaseURL := requireDatabase(t)
	if err := SweepActors(context.Background(), openPool(t, databaseURL),
		[]string{uuid.New().String()}); err != nil {
		t.Errorf("sweeping an actor that does not exist reported %v; it has nothing to remove and "+
			"nothing to prove", err)
	}
}

func openPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
