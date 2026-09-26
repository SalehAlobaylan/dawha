// One cleanup path for every fixture that creates a synthetic actor.
//
// This exists because of a specific, measured failure. Fixtures across nine
// packages insert an actor into the shared public schema and then delete it with
// a hand-written statement or a `defer pool.Exec(...)` whose error nobody reads.
// The delete aborts on a foreign key the fixture did not know about - an
// audit_log row the service wrote, a geographic_association, a review - and the
// error is discarded. The test passes, the row stays, and the developer's
// database slowly fills with people who never existed. The browser suite stopped
// doing this years ago; apps/web/e2e/cleanup.sql is the proven shape, and this is
// the Go equivalent of it, written once so no package can get it subtly wrong.
//
// What it does, in one transaction:
//
//  1. Every foreign key whose ON DELETE action is not CASCADE is followed by
//     hand, level by level, outwards from the synthetic actors. CASCADE is left
//     to cascade. The graph is read from pg_constraint rather than written out by
//     hand, so a migration that adds a table changes what is deleted instead of
//     silently becoming a leak.
//  2. The reachable rows are deleted deepest level first, so a child is always
//     gone before its parent.
//  3. The whole pass runs again until one changes no rows. Two rows that
//     reference each other, or a RESTRICT that only clears once a neighbour is
//     gone, resolve themselves over two passes instead of aborting.
//  4. A final traversal inside the same transaction refuses to commit if a single
//     row reachable from the actors is still there, and names the table and the
//     ids. A clean delete that skipped a table is otherwise indistinguishable from
//     a delete that had nothing to do.
//
// A delete that cannot be made to work is a loud failure rather than a partial
// clean: the sweep is one transaction, so a failure leaves the database exactly
// as it was found instead of half-cleaned.
//
// The scope is the synthetic actors, never a row count. Packages run in parallel
// and `go test ./...` interleaves them, so "users had 580 rows before and 580
// after" is a race that can fail for a reason unrelated to any test - and plan 005
// already lost a day to exactly that. The markers below are the contract instead,
// and tools/dbtestguard checks them by name.
package testsupport

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SyntheticEmailSuffixes are the reserved address domains a test actor's email
// ends in. A row in users whose address ends in one of these is a test row, full
// stop, and both the cleanup below and the gate in tools/dbtestguard are scoped by
// exactly this list.
//
//   - `@example.test` is the marker the fixtures that write to the shared schema
//     have always used, and the domain the 593 leaked rows carried.
//   - `@dawha.test` is the marker the visibility and handler fixtures use for
//     their own actors. Those fixtures clean up today; including the domain means
//     the day one of them stops, the gate catches it rather than the leak sitting
//     there until somebody notices the count.
//
// `@example.invalid` is deliberately NOT here. That is the browser suite's marker,
// cleaned up by apps/web/e2e/cleanup.sql on every Playwright teardown, so counting
// it would make a `go test` run fail over a row an e2e run left and already
// reported. The two suites have separate teardowns and separate gates.
var SyntheticEmailSuffixes = []string{"@example.test", "@dawha.test"}

// SyntheticUser is one actor a fixture created, as the gate reports it.
type SyntheticUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// SyntheticEmailPredicate renders a WHERE fragment matching a column against every
// reserved marker, with one placeholder per marker. It is exported so the gate, the
// sweep and the gate's own tests cannot drift apart from the list they share.
func SyntheticEmailPredicate(column string) (string, []any) {
	clauses := make([]string, 0, len(SyntheticEmailSuffixes))
	args := make([]any, 0, len(SyntheticEmailSuffixes))
	for index, suffix := range SyntheticEmailSuffixes {
		clauses = append(clauses, fmt.Sprintf("%s LIKE $%d", column, index+1))
		args = append(args, "%"+suffix)
	}
	return strings.Join(clauses, " OR "), args
}

// Database is what the sweep needs from its caller: a connection or a pool it can
// read and run one transaction on. *pgxpool.Pool and *pgx.Conn both satisfy it,
// which is how the same engine serves a test's pool and a tool that has no pool.
type Database interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CleanupSyntheticActors removes every row reachable from the actors a fixture
// created, and fails the test if any of it survives.
//
// It is meant for t.Cleanup, so it runs after the assertions of the test that
// registered it:
//
//	defer testsupport.CleanupSyntheticActors(t, pool, ownerID, viewerID)
//
// Everything a fixture writes hangs off its actors, so naming the actors is
// enough and no fixture has to enumerate its own tables - which is how a
// hand-written delete ends up missing one. A fixture that reuses a seeded id -
// granting the demo user a role, for instance - creates no actor of its own and
// has nothing to register here; those keep the defers they already have, and they
// are not what this gate is about.
func CleanupSyntheticActors(t *testing.T, pool Database, actors ...uuid.UUID) {
	t.Helper()
	if len(actors) == 0 {
		return
	}
	ids := make([]string, 0, len(actors))
	for _, actor := range actors {
		ids = append(ids, actor.String())
	}
	if err := SweepActors(context.Background(), pool, ids); err != nil {
		t.Errorf("testsupport cleanup: %v", err)
	}
}

// SweepActors removes every row reachable from the given users, in dependency
// order, looping to convergence, and returns an error naming whatever survived.
//
// It is CleanupSyntheticActors without the *testing.T, so the fixtures and the tool
// that sweeps a database nobody is running a test in share one implementation and
// cannot disagree about what clean means.
func SweepActors(ctx context.Context, pool Database, actorIDs []string) error {
	_, _, _, err := sweep(ctx, pool, actorIDs)
	return err
}

// ListSyntheticUsers returns every user in the database whose address carries a
// synthetic marker. That query is the whole of the leak gate: a fixture that
// leaves one of these behind has leaked, and the gate names it.
//
// It takes a connection string rather than a pool because its callers are the gate
// and a command-line tool, neither of which has a pool, and a second exported
// spelling for "hand me your pool" would be one more thing to keep in step.
func ListSyntheticUsers(ctx context.Context, databaseURL string) ([]SyntheticUser, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.Background())
	return listSyntheticUsers(ctx, conn)
}

func listSyntheticUsers(ctx context.Context, q Database) ([]SyntheticUser, error) {
	predicate, args := SyntheticEmailPredicate("email")
	rows, err := q.Query(ctx, `SELECT id::text, email FROM users WHERE `+predicate+` ORDER BY email`, args...)
	if err != nil {
		return nil, fmt.Errorf("list synthetic users: %w", err)
	}
	defer rows.Close()
	users := []SyntheticUser{}
	for rows.Next() {
		var user SyntheticUser
		if err := rows.Scan(&user.ID, &user.Email); err != nil {
			return nil, fmt.Errorf("list synthetic users: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// ListReachableFromSyntheticUsers reports, per table, the rows still reachable from
// a synthetic user.
//
// It is the gate's second question. "A synthetic row is left" is the failure; "and
// here is everything it is still holding up" is what turns the failure into
// something a fixture author can act on rather than a bare count.
func ListReachableFromSyntheticUsers(ctx context.Context, databaseURL string) (map[string][]string, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.Background())
	return reachableFromSyntheticUsers(ctx, conn)
}

func reachableFromSyntheticUsers(ctx context.Context, q Database) (map[string][]string, error) {
	seed, err := listSyntheticUsers(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(seed) == 0 {
		return map[string][]string{}, nil
	}
	ids := make([]string, 0, len(seed))
	for _, user := range seed {
		ids = append(ids, user.ID)
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	tx, err := q.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin reachability probe: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	scopes, err := buildScopes(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	reachable := map[string][]string{}
	for _, scope := range scopes.tables {
		if scope.table == "users" {
			continue
		}
		values, err := scope.remaining(ctx, tx)
		if err != nil {
			return nil, err
		}
		if len(values) > 0 {
			reachable[scope.table] = values
		}
	}
	return reachable, nil
}

// ReportedTables are the tables a sweep's before/after report always shows, in this
// order, whether or not the sweep touched them. A count an operator has to ask for is
// a count they will not check.
var ReportedTables = []string{"users", "people", "trees", "sources", "claims", "audit_log"}

// SweepReport is what a sweep of a whole database found and removed.
type SweepReport struct {
	// Seeded is how many users the sweep started from, all of them synthetic.
	Seeded int `json:"seeded"`
	// Emails are those users, so an operator can see exactly what went.
	Emails []string `json:"emails"`
	// DeletedPerTable counts the rows removed, per table.
	DeletedPerTable map[string]int `json:"deleted_per_table"`
	// Passes is how many delete passes it took to reach a pass that changed
	// nothing. One means the dependency order was right first time.
	Passes int `json:"passes"`
	// SkippedCascadeConstraints is how many foreign keys are ON DELETE CASCADE and
	// point at a parent column the traversal cannot join on, so their child rows were
	// removed by the database rather than followed. Non-zero means the proof that
	// nothing survived covers those rows by the database's cascade rather than by this
	// traversal - which is sound, because a cascade child cannot outlive its parent, but
	// it is worth being able to see.
	SkippedCascadeConstraints int `json:"skipped_cascade_constraints"`
	// Before and After count ReportedTables.
	Before map[string]int `json:"before"`
	After  map[string]int `json:"after"`
	// Orphans is the number of rows left pointing at a parent that is gone, and
	// SoftConstraints the number of foreign keys that are deferrable or NOT VALID.
	// Both must be zero: an ordered delete is only proof of absence because every
	// constraint in this schema is immediate and validated, and the sweep refuses to
	// commit on the strength of an assumption it has not checked.
	Orphans         int `json:"orphans"`
	SoftConstraints int `json:"soft_constraints"`
}

// SweepSyntheticRows removes every synthetic row from the database at databaseURL
// and reports what it did. It is scoped by the markers above, so it cannot touch a
// seeded row however many of them there are.
func SweepSyntheticRows(ctx context.Context, databaseURL string) (*SweepReport, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.Background())

	before, err := countTables(ctx, conn, ReportedTables)
	if err != nil {
		return nil, err
	}
	seed, err := listSyntheticUsers(ctx, conn)
	if err != nil {
		return nil, err
	}
	report := &SweepReport{
		Seeded:          len(seed),
		Emails:          []string{},
		DeletedPerTable: map[string]int{},
		Before:          before,
		After:           map[string]int{},
	}
	ids := make([]string, 0, len(seed))
	for _, user := range seed {
		ids = append(ids, user.ID)
		report.Emails = append(report.Emails, user.Email)
	}
	sort.Strings(report.Emails)

	deleted, passes, skipped, err := sweep(ctx, conn, ids)
	if err != nil {
		return nil, err
	}
	report.DeletedPerTable = deleted
	report.Passes = passes
	report.SkippedCascadeConstraints = skipped

	if report.After, err = countTables(ctx, conn, ReportedTables); err != nil {
		return nil, err
	}
	if report.Orphans, report.SoftConstraints, err = proveNoOrphans(ctx, conn); err != nil {
		return nil, err
	}
	left, err := listSyntheticUsers(ctx, conn)
	if err != nil {
		return nil, err
	}
	if len(left) > 0 {
		return nil, fmt.Errorf("sweep: %d synthetic user(s) survived the delete: %s",
			len(left), strings.Join(userEmails(left), ", "))
	}
	return report, nil
}

func userEmails(users []SyntheticUser) []string {
	out := make([]string, 0, len(users))
	for _, user := range users {
		out = append(out, user.Email)
	}
	return out
}

func countTables(ctx context.Context, q Database, tables []string) (map[string]int, error) {
	counts := map[string]int{}
	for _, table := range tables {
		var count int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = count
	}
	return counts, nil
}

// maxSweepPasses bounds the convergence loop. Two is the ordinary case: one pass to
// delete in dependency order, one to prove nothing moved. A pathological cycle - two
// rows each RESTRICTing the other - would otherwise spin, and a loud failure naming
// both rows is worth more than a hang.
const maxSweepPasses = 8

// maxSweepLevels bounds how far the traversal walks. The deepest chain in this schema
// is users -> trees -> tree_versions -> tree_nodes -> suggestions -> suggestion_reviews
// -> suggestion_change_sets, which is six hops; the CASCADE edges the traversal also
// follows add a couple more. Twenty-four is generous, and it exists so that a
// self-referential graph stops with a "did not converge" failure rather than spinning.
const maxSweepLevels = 24

// sweep is the engine. It runs the deletes and the proof that nothing survived as
// one transaction, so a failure leaves the database exactly as it was found.
func sweep(ctx context.Context, pool Database, actorIDs []string) (map[string]int, int, int, error) {
	deleted := map[string]int{}
	edges, skippedCascade, err := loadEdges(ctx, pool)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(actorIDs) == 0 {
		return deleted, 0, skippedCascade, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("begin cleanup: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	passes := 0
	for pass := 1; pass <= maxSweepPasses; pass++ {
		scopes, err := buildScopesWithEdges(ctx, tx, actorIDs, edges)
		if err != nil {
			return nil, 0, 0, err
		}
		removed := 0
		// The unlinked references go first: they name rows that are about to be
		// deleted, and the traversal that proves nothing survived looks at the tables
		// they belong to.
		unlinked, err := deleteUnlinkedReferences(ctx, tx, scopes)
		if err != nil {
			return nil, 0, 0, err
		}
		for table, count := range unlinked {
			deleted[table] += count
			removed += count
		}
		changed, err := deleteScopes(ctx, tx, scopes)
		if err != nil {
			return nil, 0, 0, err
		}
		for table, count := range changed {
			deleted[table] += count
			removed += count
		}
		passes = pass
		if removed == 0 {
			break
		}
	}
	if passes == maxSweepPasses {
		return deleted, passes, skippedCascade, fmt.Errorf("cleanup did not converge in %d passes, so the dependency "+
			"graph has a cycle the traversal cannot order; nothing was committed", maxSweepPasses)
	}

	// The proof, inside the same transaction.
	leftover, err := buildScopesWithEdges(ctx, tx, actorIDs, edges)
	if err != nil {
		return nil, 0, 0, err
	}
	if remaining := leftover.total(); remaining > 0 {
		described, err := leftover.describe(ctx, tx)
		if err != nil {
			return nil, 0, 0, err
		}
		return nil, 0, 0, fmt.Errorf("cleanup left %d row(s) reachable from the synthetic actors: %s",
			remaining, described)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, 0, fmt.Errorf("commit cleanup: %w", err)
	}
	committed = true
	return deleted, passes, skippedCascade, nil
}

// cleanupEdge is one referential edge of the schema, and the traversal follows all of
// them, CASCADE included.
//
// Following a CASCADE edge looks redundant - the parent delete would take the child
// anyway - and it is not. The traversal's output is also what proves nothing survived,
// and a row the database deletes for you is still a row this run was responsible for.
// A source file is the case that matters: `source_files.source_id` is CASCADE, so the
// traversal never sees the file, and an `audit_log` row that names the file therefore
// looks unreachable and survives every run. Including the edge puts the file in the
// scope set, which is the set the unlinked-reference sweep and the final proof read.
type cleanupEdge struct {
	ChildTable   string
	ChildColumn  string
	ParentTable  string
	ParentColumn string
	// Action is pg_constraint.confdeltype: `c` CASCADE, `a` NO ACTION, `r` RESTRICT,
	// `n` SET NULL. Recorded rather than acted on, so a reader can tell which edges
	// the database would have handled by itself.
	Action string
}

// loadEdges reads the referential edges of the public schema that the traversal can
// follow, which is every edge whose parent column is the parent's `id`.
//
// It fails when a non-CASCADE edge points at a parent column that is not the parent's
// `id`, because the traversal identifies a parent's rows by `id` and would otherwise
// under-collect without saying so. A CASCADE edge is allowed to be skipped: the row
// disappears with its parent either way, so the traversal missing it costs only a
// deeper proof, never a leaked row. The count of skipped CASCADE edges is returned so
// a report can say the proof was not total.
func loadEdges(ctx context.Context, q Database) ([]cleanupEdge, int, error) {
	rows, err := q.Query(ctx, `
		SELECT child.relname, child_attr.attname, parent.relname, parent_attr.attname, c.confdeltype::text
		FROM pg_constraint c
		JOIN pg_class child ON child.oid = c.conrelid
		JOIN pg_class parent ON parent.oid = c.confrelid
		JOIN pg_namespace ns ON ns.oid = c.connamespace
		JOIN LATERAL unnest(c.conkey, c.confkey) AS k(child_attnum, parent_attnum) ON true
		JOIN pg_attribute child_attr ON child_attr.attrelid = c.conrelid AND child_attr.attnum = k.child_attnum
		JOIN pg_attribute parent_attr ON parent_attr.attrelid = c.confrelid AND parent_attr.attnum = k.parent_attnum
		WHERE c.contype = 'f'
		  AND ns.nspname = 'public'
		ORDER BY child.relname, child_attr.attname`)
	if err != nil {
		return nil, 0, fmt.Errorf("read foreign keys: %w", err)
	}
	defer rows.Close()
	edges := []cleanupEdge{}
	skippedCascade := map[string]bool{}
	for rows.Next() {
		var edge cleanupEdge
		if err := rows.Scan(&edge.ChildTable, &edge.ChildColumn,
			&edge.ParentTable, &edge.ParentColumn, &edge.Action); err != nil {
			return nil, 0, fmt.Errorf("read foreign keys: %w", err)
		}
		if edge.ParentColumn != "id" {
			if edge.Action == "c" {
				// Counted per constraint, not per column pair, so the number a report
				// prints is a number of foreign keys a reader can look up.
				skippedCascade[edge.ChildTable+" -> "+edge.ParentTable] = true
				continue
			}
			return nil, 0, fmt.Errorf(
				"cleanup cannot follow %s.%s -> %s.%s: the parent column is not the parent's id, "+
					"so rows reachable through that edge would be missed. Extend the traversal in "+
					"internal/testsupport/cleanup.go before a migration adds it.",
				edge.ChildTable, edge.ChildColumn, edge.ParentTable, edge.ParentColumn)
		}
		edges = append(edges, edge)
	}
	return edges, len(skippedCascade), rows.Err()
}

// scopeTable is one real table and the temp table holding the rows of it that are
// reachable from the synthetic actors.
type scopeTable struct {
	table string
	// alias is the temp table's name, unique within the transaction.
	alias string
	// columns are the table's identity columns: `id` where it has one, otherwise its
	// uuid primary key. A row is identified by all of them, so a join table without an
	// id is still addressable.
	columns []string
	// count is how many rows the temp table currently holds.
	count int
	// maxLevel is the deepest level a row of this table was found at.
	maxLevel int
}

// idExpression is the row's `id`, or NULL for a table that has none. The traversal
// only ever follows edges out of a parent keyed by `id` - loadEdges guarantees it - so
// a composite-keyed table only ever needs to be a leaf.
func (s *scopeTable) idExpression(alias string) string {
	if len(s.columns) == 1 && s.columns[0] == "id" {
		return alias + ".id"
	}
	return "NULL::uuid"
}

// keyExpression renders the row's identity as a text array, which is how a row is
// stored in the temp table and compared against it again. Text rather than uuid so a
// composite key needs no special case.
func (s *scopeTable) keyExpression(alias string) string {
	parts := make([]string, 0, len(s.columns))
	for _, column := range s.columns {
		parts = append(parts, alias+"."+pgx.Identifier{column}.Sanitize()+"::text")
	}
	return "ARRAY[" + strings.Join(parts, ", ") + "]"
}

func (s *scopeTable) create() string {
	return `CREATE TEMP TABLE ` + pgx.Identifier{s.alias}.Sanitize() +
		` (level int NOT NULL, id uuid, key text[] NOT NULL) ON COMMIT DROP`
}

func (s *scopeTable) drop() string {
	return `DROP TABLE IF EXISTS ` + pgx.Identifier{s.alias}.Sanitize()
}

// deleteAt removes this table's rows that were found at one level.
func (s *scopeTable) deleteAt(level int) string {
	return `DELETE FROM ` + pgx.Identifier{s.table}.Sanitize() + ` AS real WHERE EXISTS (` +
		`SELECT 1 FROM ` + pgx.Identifier{s.alias}.Sanitize() + ` AS scope` +
		` WHERE scope.level = ` + strconv.Itoa(level) +
		` AND scope.key = ` + s.keyExpression("real") + `)`
}

// remaining returns the identities of this table's rows that are still in the
// database, for the gate's report and for the sweep's own final proof.
func (s *scopeTable) remaining(ctx context.Context, q Database) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT scope.key::text FROM `+
		pgx.Identifier{s.alias}.Sanitize()+` AS scope WHERE EXISTS (SELECT 1 FROM `+
		pgx.Identifier{s.table}.Sanitize()+` AS real WHERE `+s.keyExpression("real")+` = scope.key)`+
		` ORDER BY 1 LIMIT 20`)
	if err != nil {
		return nil, fmt.Errorf("probe %s: %w", s.table, err)
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("probe %s: %w", s.table, err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *scopeTable) total() int { return s.count }

func (scopes *scopeSet) total() int {
	total := 0
	for _, scope := range scopes.tables {
		total += scope.count
	}
	return total
}

// describe names what is still reachable, so a failure says which table and which
// ids rather than only how many.
func (scopes *scopeSet) describe(ctx context.Context, q Database) (string, error) {
	parts := []string{}
	for _, scope := range scopes.tables {
		if scope.count == 0 {
			continue
		}
		values, err := scope.remaining(ctx, q)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("%s: %d row(s) e.g. %s", scope.table, scope.count,
			strings.Join(values, ", ")))
	}
	return strings.Join(parts, "; "), nil
}

type scopeSet struct {
	tables []*scopeTable
	byName map[string]*scopeTable
}

func (s *scopeSet) add(scope *scopeTable) *scopeTable {
	s.tables = append(s.tables, scope)
	s.byName[scope.table] = scope
	return scope
}

func (s *scopeSet) level() int {
	deepest := 0
	for _, scope := range s.tables {
		if scope.count > 0 && scope.maxLevel > deepest {
			deepest = scope.maxLevel
		}
	}
	return deepest
}

// buildScopes walks the graph outwards from the actors until it stops finding
// anything new, and returns one scope table per table it reached. The temp tables
// are created here and dropped at COMMIT, so each call starts from a clean set.
func buildScopes(ctx context.Context, tx pgx.Tx, actorIDs []string) (*scopeSet, error) {
	edges, _, err := loadEdges(ctx, tx)
	if err != nil {
		return nil, err
	}
	return buildScopesWithEdges(ctx, tx, actorIDs, edges)
}

func buildScopesWithEdges(ctx context.Context, tx pgx.Tx, actorIDs []string, edges []cleanupEdge) (*scopeSet, error) {
	scopes := &scopeSet{byName: map[string]*scopeTable{}}
	users := scopes.add(&scopeTable{table: "users", alias: "p012_scope_0", columns: []string{"id"}})
	if _, err := tx.Exec(ctx, users.drop()); err != nil {
		return nil, fmt.Errorf("reset scope for users: %w", err)
	}
	if _, err := tx.Exec(ctx, users.create()); err != nil {
		return nil, fmt.Errorf("create scope for users: %w", err)
	}
	tag, err := tx.Exec(ctx,
		`INSERT INTO p012_scope_0 (level, id, key) SELECT 0, id, ARRAY[id::text] FROM users WHERE id::text = ANY($1::text[])`,
		actorIDs)
	if err != nil {
		return nil, fmt.Errorf("seed scope: %w", err)
	}
	users.count = int(tag.RowsAffected())
	if users.count == 0 {
		return scopes, nil
	}

	aliasOf := map[string]string{users.table: users.alias}
	for level := 1; level <= maxSweepLevels; level++ {
		// One statement per reachable child table per level, with every edge out of
		// an already-reached parent OR'd into its WHERE. Fewer round trips than one
		// statement per edge, and a table is only touched when a parent of it
		// actually holds rows.
		conditions := map[string][]string{}
		for _, edge := range edges {
			parent, ok := scopes.byName[edge.ParentTable]
			if !ok || parent.count == 0 {
				continue
			}
			child, ok := scopes.byName[edge.ChildTable]
			if !ok {
				identity, err := identityColumns(ctx, tx, edge.ChildTable)
				if err != nil {
					return nil, err
				}
				child = scopes.add(&scopeTable{
					table:   edge.ChildTable,
					alias:   "p012_scope_" + strconv.Itoa(len(scopes.tables)),
					columns: identity,
				})
				if _, err := tx.Exec(ctx, child.drop()); err != nil {
					return nil, fmt.Errorf("reset scope for %s: %w", child.table, err)
				}
				if _, err := tx.Exec(ctx, child.create()); err != nil {
					return nil, fmt.Errorf("create scope for %s: %w", child.table, err)
				}
				aliasOf[child.table] = child.alias
			}
			conditions[child.table] = append(conditions[child.table], fmt.Sprintf(
				`real.%s IN (SELECT id FROM %s WHERE id IS NOT NULL)`,
				pgx.Identifier{edge.ChildColumn}.Sanitize(), pgx.Identifier{parent.alias}.Sanitize()))
		}
		if len(conditions) == 0 {
			break
		}
		added := 0
		for _, child := range scopes.tables {
			where, ok := conditions[child.table]
			if !ok {
				continue
			}
			key := child.keyExpression("real")
			statement := `INSERT INTO ` + pgx.Identifier{child.alias}.Sanitize() +
				` (level, id, key) SELECT ` + strconv.Itoa(level) + `, ` + child.idExpression("real") +
				`, ` + key + ` FROM ` + pgx.Identifier{child.table}.Sanitize() + ` AS real` +
				` WHERE (` + strings.Join(where, " OR ") + `)` +
				` AND NOT EXISTS (SELECT 1 FROM ` + pgx.Identifier{child.alias}.Sanitize() +
				` AS seen WHERE seen.key = ` + key + `)`
			tag, err := tx.Exec(ctx, statement)
			if err != nil {
				return nil, fmt.Errorf("collect reachable %s: %w", child.table, err)
			}
			found := int(tag.RowsAffected())
			if found > 0 {
				child.count += found
				child.maxLevel = level
				added += found
			}
		}
		if added == 0 {
			break
		}
	}
	return scopes, nil
}

// deleteScopes removes every reachable row, deepest level first, and reports how many
// rows went per table.
//
// A child is always deleted before its parent, which is what a RESTRICT or NO ACTION
// foreign key needs. That is a guarantee about the *depth* of a row and not about the
// order two rows at the same depth were found in, so a statement can still be blocked
// - a claim and the geographic association that cites it are both one step from the
// actor that owns them, and which one goes first is an accident of the table list.
// Each statement therefore runs behind a savepoint: one that is blocked is rolled
// back on its own and left for the next pass, by which time the row blocking it is
// gone. Without the savepoint the first blocked statement would abort the transaction
// and throw away the deletes that already worked.
func deleteScopes(ctx context.Context, tx pgx.Tx, scopes *scopeSet) (map[string]int, error) {
	deleted := map[string]int{}
	for level := scopes.level(); level >= 0; level-- {
		for _, scope := range scopes.tables {
			if scope.count == 0 || scope.maxLevel < level {
				continue
			}
			if _, err := tx.Exec(ctx, `SAVEPOINT p012_sweep_delete`); err != nil {
				return nil, fmt.Errorf("savepoint: %w", err)
			}
			tag, err := tx.Exec(ctx, scope.deleteAt(level))
			if err != nil {
				if _, rollbackErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT p012_sweep_delete`); rollbackErr != nil {
					return nil, fmt.Errorf("rollback to savepoint: %w", rollbackErr)
				}
				continue
			}
			if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT p012_sweep_delete`); err != nil {
				return nil, fmt.Errorf("release savepoint: %w", err)
			}
			affected := int(tag.RowsAffected())
			if affected > 0 {
				deleted[scope.table] += affected
				scope.count -= affected
			}
		}
	}
	return deleted, nil
}

// deleteUnlinkedReferences removes the rows that name a fixture's rows without a
// foreign key to them, which the traversal above cannot see by construction.
//
// There are exactly two shapes of that in this schema, and both are measured rather
// than imagined: running the whole suite against a database that had been accumulating
// test rows for months left 15 `jobs` rows whose source had been deleted out from
// under them and 22 `audit_log` rows about a source file that no longer existed.
//
//   - `jobs.payload` is jsonb. A queued source job names its source by payload key
//     rather than by column, so no constraint ties it to anything and deleting the
//     source leaves the job pointing at a uuid that is not in the database.
//   - `audit_log.entity_id` is polymorphic: one column for every entity type in the
//     product, with no constraint. A background worker and a signed-out visitor both
//     write audit rows with no actor at all, so `actor_id` finds nothing and the row
//     outlives the thing it is about.
//
// Both are handled by value rather than by a hand-written list of keys, which is the
// point: a job payload key list would be a list to keep in step with the code that
// writes them, and a new key would be a new leak. Every uuid anywhere in the payload is
// compared against the ids of the rows this run is deleting, so a new key is free.
//
// A row of this shape has no foreign key, so deleting it cannot be blocked and its
// order does not matter. It is not part of the convergence loop for the same reason.
func deleteUnlinkedReferences(ctx context.Context, tx pgx.Tx, scopes *scopeSet) (map[string]int, error) {
	deleted := map[string]int{}
	auditBranches := []string{}
	jobBranches := []string{}
	for _, scope := range scopes.tables {
		if scope.count == 0 {
			continue
		}
		alias := pgx.Identifier{scope.alias}.Sanitize()
		auditBranches = append(auditBranches, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM %s AS scope WHERE scope.id = audit.entity_id)`, alias))
		jobBranches = append(jobBranches, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM %s AS scope
			         WHERE scope.id IN (SELECT value::uuid FROM jsonb_each_text(job.payload)
			                            WHERE value ~ '^[0-9a-fA-F-]{36}$'))`, alias))
	}
	if len(auditBranches) > 0 {
		statement := `DELETE FROM audit_log AS audit WHERE ` + strings.Join(auditBranches, " OR ")
		tag, err := tx.Exec(ctx, statement)
		if err != nil {
			return nil, fmt.Errorf("delete audit rows about the rows being removed: %w", err)
		}
		if affected := int(tag.RowsAffected()); affected > 0 {
			deleted["audit_log"] += affected
		}
	}
	if len(jobBranches) > 0 {
		statement := `DELETE FROM jobs AS job WHERE ` + strings.Join(jobBranches, " OR ")
		tag, err := tx.Exec(ctx, statement)
		if err != nil {
			return nil, fmt.Errorf("delete jobs about the rows being removed: %w", err)
		}
		if affected := int(tag.RowsAffected()); affected > 0 {
			deleted["jobs"] += affected
		}
	}
	return deleted, nil
}

// identityColumns returns the columns that identify a row of table: `id` when it has
// one, otherwise its uuid primary key.
//
// A table with neither cannot be addressed by the traversal, and the traversal is the
// thing that decides what a fixture left behind, so this fails loudly rather than
// quietly under-deleting.
func identityColumns(ctx context.Context, q Database, table string) ([]string, error) {
	var hasID *string
	if err := q.QueryRow(ctx, `SELECT attname FROM pg_attribute
		WHERE attrelid = $1::regclass AND attname = 'id' AND attnum > 0`, table).Scan(&hasID); err == nil {
		return []string{"id"}, nil
	}
	var key *string
	if err := q.QueryRow(ctx, `
		SELECT string_agg(a.attname, ',' ORDER BY k.ord)
		FROM pg_index i
		JOIN unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord) ON true
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
		WHERE i.indrelid = $1::regclass AND i.indisprimary AND a.atttypid = 'uuid'::regtype`,
		table).Scan(&key); err != nil {
		return nil, fmt.Errorf("read the primary key of %s: %w", table, err)
	}
	if key == nil || *key == "" {
		return nil, fmt.Errorf("cleanup cannot address rows of %s: it has neither an id nor a "+
			"uuid primary key, so a row reachable from a synthetic actor could neither be "+
			"deleted nor named. Give the table a uuid primary key, or extend the traversal in "+
			"internal/testsupport/cleanup.go", table)
	}
	return strings.Split(*key, ","), nil
}

// orphanProbe counts a child row whose parent is gone. A NULL foreign key is not an
// orphan: 22 seeded audit rows and 4 seeded sources carry a NULL actor, which is a
// fact about those rows rather than a dangling reference. Only a non-NULL value with
// no parent counts.
const orphanProbe = `
	SELECT
	    (SELECT count(*) FROM tree_versions v WHERE v.tree_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM trees t WHERE t.id = v.tree_id))
	  + (SELECT count(*) FROM tree_nodes n WHERE n.tree_version_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM tree_versions v WHERE v.id = n.tree_version_id))
	  + (SELECT count(*) FROM tree_relationships r WHERE r.tree_version_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM tree_versions v WHERE v.id = r.tree_version_id))
	  + (SELECT count(*) FROM tree_nodes n WHERE n.person_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM people p WHERE p.id = n.person_id))
	  + (SELECT count(*) FROM source_files f WHERE f.source_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sources s WHERE s.id = f.source_id))
	  + (SELECT count(*) FROM source_passages p WHERE p.source_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sources s WHERE s.id = p.source_id))
	  + (SELECT count(*) FROM source_statements s WHERE s.source_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sources src WHERE src.id = s.source_id))
	  + (SELECT count(*) FROM tree_invitations i WHERE i.inviter_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = i.inviter_id))
	  + (SELECT count(*) FROM tree_collaborators c WHERE c.invited_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.invited_by))
	  + (SELECT count(*) FROM sources s WHERE s.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = s.created_by))
	  + (SELECT count(*) FROM open_questions q WHERE q.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = q.created_by))
	  + (SELECT count(*) FROM question_notes n WHERE n.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = n.created_by))
	  + (SELECT count(*) FROM claims c WHERE c.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.created_by))
	  + (SELECT count(*) FROM people p WHERE p.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = p.created_by))
	  + (SELECT count(*) FROM audit_log a WHERE a.actor_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = a.actor_id))
	  + (SELECT count(*) FROM trees t WHERE t.owner_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = t.owner_id))`

// proveNoOrphans is the sweep's last check, and it is the same two questions
// apps/web/e2e/cleanup.sql asks. Every foreign key in this schema is immediate and
// validated, which is what makes an ordered delete a proof of absence rather than a
// hope: PostgreSQL verifies such a constraint at the end of every statement, so a
// completed delete cannot have left a child pointing at a row that is gone. A
// deferrable or NOT VALID constraint would void that argument, so the count of those
// is checked rather than assumed.
func proveNoOrphans(ctx context.Context, q Database) (int, int, error) {
	var softConstraints int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM pg_constraint
		WHERE contype = 'f' AND connamespace = 'public'::regnamespace
		  AND (condeferrable OR NOT convalidated)`).Scan(&softConstraints); err != nil {
		return 0, 0, fmt.Errorf("count deferrable foreign keys: %w", err)
	}
	if softConstraints > 0 {
		return 0, softConstraints, fmt.Errorf("cleanup: %d foreign keys are deferrable or NOT VALID, "+
			"so a successful delete no longer proves the absence of orphans; nothing was committed",
			softConstraints)
	}
	var orphans int
	if err := q.QueryRow(ctx, orphanProbe).Scan(&orphans); err != nil {
		return 0, softConstraints, fmt.Errorf("probe for orphans: %w", err)
	}
	return orphans, softConstraints, nil
}

// DescribeSyntheticRows is the read-only half of the gate, for a human running
// dbsweep -n: it lists the synthetic actors a database is holding and the rows
// still reachable from them, without changing anything.
func DescribeSyntheticRows(ctx context.Context, databaseURL string) ([]SyntheticUser, map[string][]string, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.Background())
	users, err := listSyntheticUsers(ctx, conn)
	if err != nil {
		return nil, nil, err
	}
	reachable, err := reachableFromSyntheticUsers(ctx, conn)
	if err != nil {
		return nil, nil, err
	}
	return users, reachable, nil
}

// CountReportedTables counts ReportedTables in the database at databaseURL, so a
// caller can print a before/after without reaching for its own SQL.
func CountReportedTables(ctx context.Context, databaseURL string) (map[string]int, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.Background())
	return countTables(ctx, conn, ReportedTables)
}
