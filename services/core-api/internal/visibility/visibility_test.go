package visibility

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewRejectsMalformedActor(t *testing.T) {
	for _, actorID := range []string{"not-a-uuid", "  "} {
		policy, err := WithResearch(actorID, true)
		if actorID == "  " {
			if err != nil || !policy.Anonymous() || policy.Research() {
				t.Fatalf("blank actor should be anonymous without research role: %+v %v", policy, err)
			}
			continue
		}
		if err != ErrForbidden {
			t.Fatalf("expected forbidden for %q, got %v", actorID, err)
		}
	}
}

func TestAccessAllowedExcludesMissingAndHidden(t *testing.T) {
	for _, access := range []Access{AccessPublic, AccessOwner, AccessCollaborator, AccessResearch} {
		if !access.Allowed() {
			t.Fatalf("expected %s to be allowed", access)
		}
	}
	for _, access := range []Access{AccessMissing, AccessHidden} {
		if access.Allowed() {
			t.Fatalf("expected %s to be denied", access)
		}
	}
}

func TestParamsAllocatesSequentialPlaceholders(t *testing.T) {
	params := NewParams()
	first := params.Add("first")
	second := params.Add(uuid.Nil)
	if first != "$1" || second != "$2" {
		t.Fatalf("unexpected placeholders: %s %s", first, second)
	}
	if castUUID(second) != "$2::uuid" {
		t.Fatalf("unexpected cast placeholder: %s", castUUID(second))
	}
	if params.Len() != 2 {
		t.Fatalf("unexpected parameter count: %d", params.Len())
	}
	args := params.Args()
	if len(args) != 2 || args[0] != "first" || args[1] != uuid.Nil {
		t.Fatalf("unexpected arguments: %#v", args)
	}
	if exists := params.Exists("people", first); exists != `EXISTS (SELECT 1 FROM people WHERE id = $1)` {
		t.Fatalf("unexpected existence expression: %s", exists)
	}
}

func TestAnonymousPolicyPredicatesStayPublicOnly(t *testing.T) {
	policy := Anonymous()
	params := NewParams()
	person := policy.PersonPredicate(params, "p.id")
	claim := policy.ClaimPredicate(params, "c.id")
	question := policy.QuestionPredicate(params, "q.id")
	if strings.Contains(person, "TRUE") || strings.Contains(claim, "TRUE") || strings.Contains(question, "TRUE") {
		t.Fatalf("anonymous policy must not grant unconditional access: %s | %s | %s", person, claim, question)
	}
	if !strings.Contains(person, "vis_version.state = 'published'") || !strings.Contains(person, "vis_tree.visibility = 'public'") {
		t.Fatalf("person predicate must require a published public tree: %s", person)
	}
	if !strings.Contains(claim, "vis_source.visibility = 'public'") || !strings.Contains(claim, "vis_source.visibility = 'private'") {
		t.Fatalf("claim predicate must apply the all-or-nothing source rule: %s", claim)
	}
	if !strings.Contains(question, "question_sources") {
		t.Fatalf("question predicate must require a public source link: %s", question)
	}
}

func TestResearchRoleCannotBypassPeople(t *testing.T) {
	policy, err := WithResearch("10000000-0000-0000-0000-000000000001", true)
	if err != nil {
		t.Fatal(err)
	}
	params := NewParams()
	if person := policy.PersonPredicate(params, "p.id"); strings.Contains(person, "TRUE") {
		t.Fatalf("research role must not bypass person visibility: %s", person)
	}
	if tree := policy.TreePredicate(params, "t.id"); strings.Contains(tree, "TRUE") {
		t.Fatalf("research role must not bypass tree visibility: %s", tree)
	}
	if tree := policy.TreePredicate(params, "t.id"); !strings.Contains(tree, "vis_version.state = 'published'") {
		t.Fatalf("the public tree grant must require a published version: %s", tree)
	}
	if source := policy.SourcePredicate(params, "s.id"); !strings.Contains(source, "TRUE") {
		t.Fatalf("research role must reach research-only sources: %s", source)
	}
	if claim := policy.ClaimPredicate(params, "c.id"); !strings.Contains(claim, "TRUE") {
		t.Fatalf("research role must reach research-only claims: %s", claim)
	}
}

func TestPolicyPredicatesReferenceAllocatedActor(t *testing.T) {
	policy, err := New("10000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	params := NewParams()
	params.Add("search text")
	policy.ClaimPredicate(params, "c.id")
	actorPlaceholders := 0
	for _, arg := range params.Args() {
		if value, ok := arg.(uuid.UUID); ok && value.String() == "10000000-0000-0000-0000-000000000001" {
			actorPlaceholders++
		}
	}
	if actorPlaceholders != 1 || params.Len() != 2 {
		t.Fatalf("expected one actor parameter after the caller parameter, got %#v", params.Args())
	}
	if !strings.Contains(params.Args()[1].(uuid.UUID).String(), "10000000") {
		t.Fatalf("unexpected actor argument: %#v", params.Args()[1])
	}
}

type policyFixture struct {
	ownerID               uuid.UUID
	collaboratorID        uuid.UUID
	researcherID          uuid.UUID
	unrelatedResearcherID uuid.UUID
	adminID               uuid.UUID
	draftPersonID         uuid.UUID
	publishedPersonID     uuid.UUID
	unpublishedTreeID     uuid.UUID
	unpublishedDraftID    uuid.UUID
	publicSourceID        uuid.UUID
	privateSourceID       uuid.UUID
	publicClaimID         uuid.UUID
	privateClaimID        uuid.UUID
	emptyClaimID          uuid.UUID
	publicQuestionID      uuid.UUID
	researchQuestionID    uuid.UUID
	passageIDs            []uuid.UUID
}

func TestPolicyAuthorizationMatrixAgainstDatabase(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	// Registered before the fixture cleanup so cleanups run in reverse order and the
	// fixture rows are removed while the pool is still open.
	t.Cleanup(pool.Close)
	fixture := seedPolicyFixture(t, ctx, pool)

	policies := []struct {
		name   string
		policy Policy
	}{
		{"anonymous", Anonymous()},
		{"unrelated researcher", mustPolicy(t, fixture.unrelatedResearcherID.String(), true)},
		{"owner", mustPolicy(t, fixture.ownerID.String(), false)},
		{"collaborator", mustPolicy(t, fixture.collaboratorID.String(), false)},
		{"researcher", mustPolicy(t, fixture.researcherID.String(), true)},
		{"admin", mustPolicy(t, fixture.adminID.String(), true)},
	}

	personCases := []struct {
		label  string
		id     uuid.UUID
		expect map[string]Access
	}{
		{"published person", fixture.publishedPersonID, map[string]Access{
			"anonymous": AccessPublic, "owner": AccessPublic, "collaborator": AccessPublic,
			"researcher": AccessPublic, "unrelated researcher": AccessPublic, "admin": AccessPublic,
		}},
		{"private draft person", fixture.draftPersonID, map[string]Access{
			"anonymous": AccessHidden, "owner": AccessOwner, "collaborator": AccessCollaborator,
			"researcher": AccessHidden, "unrelated researcher": AccessHidden, "admin": AccessHidden,
		}},
	}
	for _, current := range personCases {
		for _, subject := range policies {
			access, accessErr := subject.policy.Person(ctx, pool, current.id)
			if accessErr != nil {
				t.Fatalf("%s reading %s: %v", subject.name, current.label, accessErr)
			}
			if access != current.expect[subject.name] {
				t.Fatalf("%s reading %s = %s, want %s", subject.name, current.label, access, current.expect[subject.name])
			}
		}
	}

	sourceCases := []struct {
		label  string
		id     uuid.UUID
		expect map[string]Access
	}{
		{"public source", fixture.publicSourceID, map[string]Access{
			"anonymous": AccessPublic, "owner": AccessPublic, "collaborator": AccessPublic,
			"researcher": AccessPublic, "unrelated researcher": AccessPublic, "admin": AccessPublic,
		}},
		{"private source", fixture.privateSourceID, map[string]Access{
			"anonymous": AccessHidden, "owner": AccessOwner, "collaborator": AccessHidden,
			"researcher": AccessResearch, "unrelated researcher": AccessResearch, "admin": AccessResearch,
		}},
	}
	for _, current := range sourceCases {
		for _, subject := range policies {
			access, accessErr := subject.policy.Source(ctx, pool, current.id)
			if accessErr != nil {
				t.Fatalf("%s reading %s: %v", subject.name, current.label, accessErr)
			}
			if access != current.expect[subject.name] {
				t.Fatalf("%s reading %s = %s, want %s", subject.name, current.label, access, current.expect[subject.name])
			}
		}
	}

	claimCases := []struct {
		label  string
		id     uuid.UUID
		expect map[string]Access
	}{
		{"public claim", fixture.publicClaimID, map[string]Access{
			"anonymous": AccessPublic, "owner": AccessPublic, "collaborator": AccessPublic,
			"researcher": AccessPublic, "unrelated researcher": AccessPublic, "admin": AccessPublic,
		}},
		{"private evidence claim", fixture.privateClaimID, map[string]Access{
			"anonymous": AccessHidden, "owner": AccessOwner, "collaborator": AccessHidden,
			"researcher": AccessResearch, "unrelated researcher": AccessResearch, "admin": AccessResearch,
		}},
		{"claim without evidence", fixture.emptyClaimID, map[string]Access{
			"anonymous": AccessHidden, "owner": AccessOwner, "collaborator": AccessHidden,
			"researcher": AccessResearch, "unrelated researcher": AccessResearch, "admin": AccessResearch,
		}},
	}
	for _, current := range claimCases {
		for _, subject := range policies {
			access, accessErr := subject.policy.Claim(ctx, pool, current.id)
			if accessErr != nil {
				t.Fatalf("%s reading %s: %v", subject.name, current.label, accessErr)
			}
			if access != current.expect[subject.name] {
				t.Fatalf("%s reading %s = %s, want %s", subject.name, current.label, access, current.expect[subject.name])
			}
		}
	}

	questionCases := []struct {
		label  string
		id     uuid.UUID
		expect map[string]Access
	}{
		{"public question", fixture.publicQuestionID, map[string]Access{
			"anonymous": AccessPublic, "owner": AccessPublic, "collaborator": AccessPublic,
			"researcher": AccessPublic, "unrelated researcher": AccessPublic, "admin": AccessPublic,
		}},
		{"research-only question", fixture.researchQuestionID, map[string]Access{
			"anonymous": AccessHidden, "owner": AccessOwner, "collaborator": AccessHidden,
			"researcher": AccessResearch, "unrelated researcher": AccessResearch, "admin": AccessResearch,
		}},
	}
	for _, current := range questionCases {
		for _, subject := range policies {
			access, accessErr := subject.policy.Question(ctx, pool, current.id)
			if accessErr != nil {
				t.Fatalf("%s reading %s: %v", subject.name, current.label, accessErr)
			}
			if access != current.expect[subject.name] {
				t.Fatalf("%s reading %s = %s, want %s", subject.name, current.label, access, current.expect[subject.name])
			}
		}
	}

	// A public tree is public data only once a version is published. Until then the
	// tree stays closed to anonymous callers and to unrelated researchers, while the
	// owner and the collaborators keep their drafts.
	treeCases := map[string]Access{
		"anonymous":            AccessHidden,
		"unrelated researcher": AccessHidden,
		"owner":                AccessOwner,
		"collaborator":         AccessCollaborator,
		"researcher":           AccessHidden,
		"admin":                AccessHidden,
	}
	publishedTreeID := uuid.New()
	publishedVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة عامة منشورة', 'public', $2)`, publishedTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'published', $3)`, publishedVersionID, publishedTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	for _, subject := range policies {
		access, accessErr := subject.policy.Tree(ctx, pool, publishedTreeID)
		if accessErr != nil || access != AccessPublic {
			t.Fatalf("%s reading a published public tree = %s (%v), want %s", subject.name, access, accessErr, AccessPublic)
		}
	}
	for _, subject := range policies {
		access, accessErr := subject.policy.Tree(ctx, pool, fixture.unpublishedTreeID)
		if accessErr != nil {
			t.Fatalf("%s reading a public tree with no published version: %v", subject.name, accessErr)
		}
		if access != treeCases[subject.name] {
			t.Fatalf("%s reading a public tree with no published version = %s, want %s", subject.name, access, treeCases[subject.name])
		}
		access, accessErr = subject.policy.Tree(ctx, pool, uuid.New())
		if accessErr != nil || access != AccessMissing {
			t.Fatalf("%s reading a missing tree = %s (%v), want missing", subject.name, access, accessErr)
		}
	}

	for _, subject := range policies {
		access, accessErr := subject.policy.Person(ctx, pool, uuid.New())
		if accessErr != nil || access != AccessMissing {
			t.Fatalf("%s reading a deleted person = %s (%v), want missing", subject.name, access, accessErr)
		}
		if access, accessErr = subject.policy.Claim(ctx, pool, uuid.Nil); accessErr != nil || access != AccessMissing {
			t.Fatalf("%s reading a nil claim = %s (%v), want missing", subject.name, access, accessErr)
		}
	}

	// A caller may alias the same tables the policy joins. The predicate must keep
	// its own vis_ aliases, otherwise the inner reference binds to the caller's
	// alias and a private row becomes public.
	aliasParams := NewParams()
	aliasQuery := `SELECT count(*) FROM sources s WHERE ` + Anonymous().SourcePredicate(aliasParams, "s.id")
	var publicSources int
	if err := pool.QueryRow(ctx, aliasQuery, aliasParams.Args()...).Scan(&publicSources); err != nil {
		t.Fatal(err)
	}
	var expectedPublic int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sources WHERE visibility = 'public'`).Scan(&expectedPublic); err != nil {
		t.Fatal(err)
	}
	if publicSources != expectedPublic {
		t.Fatalf("aliased source predicate returned %d rows, want %d", publicSources, expectedPublic)
	}

	loaded, err := Load(ctx, pool, fixture.researcherID.String())
	if err != nil || !loaded.Research() {
		t.Fatalf("Load did not resolve the research role: %+v %v", loaded, err)
	}
	loaded, err = Load(ctx, pool, fixture.ownerID.String())
	if err != nil || loaded.Research() {
		t.Fatalf("Load granted the research role to a registered owner: %+v %v", loaded, err)
	}
	loaded, err = Load(ctx, pool, "")
	if err != nil || !loaded.Anonymous() {
		t.Fatalf("Load did not produce the anonymous policy: %+v %v", loaded, err)
	}
}

func mustPolicy(t *testing.T, actorID string, research bool) Policy {
	t.Helper()
	policy, err := WithResearch(actorID, research)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func seedPolicyFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *policyFixture {
	t.Helper()
	fixture := &policyFixture{}
	fixture.ownerID = uuid.New()
	fixture.collaboratorID = uuid.New()
	fixture.researcherID = uuid.New()
	fixture.unrelatedResearcherID = uuid.New()
	fixture.adminID = uuid.New()
	for index, entry := range []struct {
		id   uuid.UUID
		role string
	}{{fixture.ownerID, "registered"}, {fixture.collaboratorID, "registered"}, {fixture.researcherID, "researcher"}, {fixture.unrelatedResearcherID, "researcher"}, {fixture.adminID, "admin"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'مختبِر الرؤية')`, entry.id, fmt.Sprintf("visibility-%d-%s@dawha.test", index, entry.role)); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, $2)`, entry.id, entry.role); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanupPolicyFixture(t, pool, fixture)
	})

	privateTreeID := uuid.New()
	privateVersionID := uuid.New()
	publicTreeID := uuid.New()
	publicVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة اختبار الرؤية الخاصة', 'private', $2), ($3, 'شجرة اختبار الرؤية العامة', 'public', $2)`, privateTreeID, fixture.ownerID, publicTreeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3), ($4, $5, 1, 'published', $3)`, privateVersionID, privateTreeID, fixture.ownerID, publicVersionID, publicTreeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'view', $3)`, privateTreeID, fixture.collaboratorID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.unpublishedTreeID = uuid.New()
	fixture.unpublishedDraftID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة عامة غير منشورة', 'public', $2)`, fixture.unpublishedTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3)`, fixture.unpublishedDraftID, fixture.unpublishedTreeID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'view', $3)`, fixture.unpublishedTreeID, fixture.collaboratorID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.draftPersonID = uuid.New()
	fixture.publishedPersonID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, 'شخصية مسودة اختبار الرؤية', 'شخصية مسودة اختبار الرؤية', $3), ($2, 'شخصية منشورة اختبار الرؤية', 'شخصية منشورة اختبار الرؤية', $3)`, fixture.draftPersonID, fixture.publishedPersonID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخصية مسودة اختبار الرؤية', 1), ($4, $5, $6, 'شخصية منشورة اختبار الرؤية', 1)`, uuid.New(), privateVersionID, fixture.draftPersonID, uuid.New(), publicVersionID, fixture.publishedPersonID); err != nil {
		t.Fatal(err)
	}

	fixture.publicSourceID = uuid.New()
	fixture.privateSourceID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر اختبار الرؤية العام', 'book', 'public', $3), ($2, 'مصدر اختبار الرؤية الخاص', 'book', 'private', $3)`, fixture.publicSourceID, fixture.privateSourceID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	publicPassageID := uuid.New()
	privatePassageID := uuid.New()
	fixture.passageIDs = []uuid.UUID{publicPassageID, privatePassageID}
	if _, err := pool.Exec(ctx, `INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar) VALUES ($1, $3, 1, 'مقطع اختبار الرؤية العام', 'مقطع اختبار الرؤية العام'), ($2, $4, 1, 'مقطع اختبار الرؤية الخاص', 'مقطع اختبار الرؤية الخاص')`, publicPassageID, privatePassageID, fixture.publicSourceID, fixture.privateSourceID); err != nil {
		t.Fatal(err)
	}
	publicStatementID := uuid.New()
	privateStatementID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, review_status, created_by) VALUES ($1, $3, $5, 'عبارة اختبار الرؤية العامة', 'accepted', $7), ($2, $4, $6, 'عبارة اختبار الرؤية الخاصة', 'accepted', $7)`, publicStatementID, privateStatementID, fixture.publicSourceID, fixture.privateSourceID, publicPassageID, privatePassageID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}

	fixture.publicClaimID = uuid.New()
	fixture.privateClaimID = uuid.New()
	fixture.emptyClaimID = uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES
			($1, 'person', $4, 'father_of', 'person', $5, 'supported', $6),
			($2, 'person', $4, 'father_of', 'person', $5, 'supported', $6),
			($3, 'person', $4, 'father_of', 'person', $5, 'unresolved', $6)`, fixture.publicClaimID, fixture.privateClaimID, fixture.emptyClaimID, fixture.publishedPersonID, fixture.draftPersonID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO claim_evidence (claim_id, source_statement_id, relation, created_by) VALUES ($1, $2, 'supports', $3), ($4, $5, 'supports', $3)`, fixture.publicClaimID, publicStatementID, fixture.ownerID, fixture.privateClaimID, privateStatementID); err != nil {
		t.Fatal(err)
	}

	fixture.publicQuestionID = uuid.New()
	fixture.researchQuestionID = uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO open_questions (id, title_ar, status, created_by) VALUES ($1, 'سؤال اختبار الرؤية العام', 'open', $3), ($2, 'سؤال اختبار الرؤية البحثي', 'open', $3)`, fixture.publicQuestionID, fixture.researchQuestionID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO question_sources (question_id, source_id, role) VALUES ($1, $2, 'supporting')`, fixture.publicQuestionID, fixture.publicSourceID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// cleanupPolicyFixture removes the fixture in foreign key order so the shared test
// database is left exactly as it was found.
func cleanupPolicyFixture(t *testing.T, pool *pgxpool.Pool, fixture *policyFixture) {
	t.Helper()
	ctx := context.Background()
	actors := []uuid.UUID{fixture.ownerID, fixture.collaboratorID, fixture.researcherID, fixture.unrelatedResearcherID, fixture.adminID}
	statements := []string{
		`DELETE FROM question_sources WHERE question_id IN (SELECT id FROM open_questions WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM question_claims WHERE question_id IN (SELECT id FROM open_questions WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM open_questions WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM claim_evidence WHERE claim_id IN (SELECT id FROM claims WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM claims WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM source_statements WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM sources WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM tree_nodes WHERE person_id IN (SELECT id FROM people WHERE created_by = ANY($1::uuid[]))`,
		`DELETE FROM tree_versions WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM trees WHERE owner_id = ANY($1::uuid[])`,
		`DELETE FROM people WHERE created_by = ANY($1::uuid[])`,
		`DELETE FROM users WHERE id = ANY($1::uuid[])`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, actors); err != nil {
			t.Errorf("cleanup failed for %q: %v", statement, err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM source_passages WHERE id = ANY($1::uuid[])`, fixture.passageIDs); err != nil {
		t.Errorf("cleanup failed for source passages: %v", err)
	}
}

// TestReferenceFamiliesAreScopedByVisibility pins the rule the reference families
// were given in db/migrations/0039_reference_visibility.sql. A published row is
// public exactly as it was before the column existed; a research-only row is
// readable by the identity write role set and by nobody else - including an actor
// who merely registered.
func TestReferenceFamiliesAreScopedByVisibility(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	t.Cleanup(pool.Close)

	researcherID := uuid.New()
	collaboratorID := uuid.New()
	registeredID := uuid.New()
	// A unique address per run, so a rerun with -count=N does not collide with the
	// accounts an earlier run left behind.
	tag := strings.ReplaceAll(researcherID.String(), "-", "")[:12]
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, display_name_ar) VALUES
			($1, $4, 'باحث'),
			($2, $5, 'متعامل'),
			($3, $6, 'مسجل')
	`, researcherID, collaboratorID, registeredID,
		"reference-researcher-"+tag+"@example.test",
		"reference-collaborator-"+tag+"@example.test",
		"reference-registered-"+tag+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'researcher'), ($2, 'collaborator'), ($3, 'registered')`, researcherID, collaboratorID, registeredID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		for _, statement := range []string{
			`DELETE FROM places WHERE created_by IN ($1, $2, $3)`,
			`DELETE FROM families WHERE created_by IN ($1, $2, $3)`,
			`DELETE FROM audit_log WHERE actor_id IN ($1, $2, $3)`,
			`DELETE FROM user_roles WHERE user_id IN ($1, $2, $3)`,
			`DELETE FROM users WHERE id IN ($1, $2, $3)`,
		} {
			if _, err := pool.Exec(cleanup, statement, researcherID, collaboratorID, registeredID); err != nil {
				t.Errorf("cleanup failed for %q: %v", statement, err)
			}
		}
	})

	publicPlaceID := uuid.New()
	researchPlaceID := uuid.New()
	publicFamilyID := uuid.New()
	researchFamilyID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility, created_by) VALUES
			($1, 'مكان منشور', 'مكان منشور', 'city', 'public', $3),
			($2, 'مكان بحثي', 'مكان بحثي', 'city', 'private', $3)
	`, publicPlaceID, researchPlaceID, researcherID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO families (id, canonical_name_ar, normalized_name_ar, visibility, created_by) VALUES
			($1, 'عائلة منشورة', 'عائلة منشورة', 'public', $3),
			($2, 'عائلة بحثية', 'عائلة بحثية', 'private', $3)
	`, publicFamilyID, researchFamilyID, researcherID); err != nil {
		t.Fatal(err)
	}

	policies := map[string]Policy{}
	policies["anonymous"] = Anonymous()
	for name, id := range map[string]uuid.UUID{"researcher": researcherID, "collaborator": collaboratorID, "registered": registeredID} {
		policy, err := Load(ctx, pool, id.String())
		if err != nil {
			t.Fatal(err)
		}
		policies[name] = policy
	}
	// The privileged flag is the identity write role set and nothing wider: a
	// registered account does not hold it even though it is a real account.
	if !policies["researcher"].CanWriteReferences() || !policies["collaborator"].CanWriteReferences() {
		t.Fatal("a research or collaborator role does not hold the identity write role set")
	}
	if policies["registered"].CanWriteReferences() || policies["anonymous"].CanWriteReferences() {
		t.Fatal("a registered or anonymous reader holds the identity write role set")
	}

	cases := []struct {
		kind     string
		public   uuid.UUID
		research uuid.UUID
	}{
		{kind: "place", public: publicPlaceID, research: researchPlaceID},
		{kind: "family", public: publicFamilyID, research: researchFamilyID},
	}
	for _, testCase := range cases {
		for _, actor := range []string{"anonymous", "registered"} {
			access, err := policies[actor].Reference(ctx, pool, testCase.kind, testCase.public)
			if err != nil {
				t.Fatal(err)
			}
			if access != AccessPublic {
				t.Fatalf("%s read of a published %s = %q, want %q", actor, testCase.kind, access, AccessPublic)
			}
			access, err = policies[actor].Reference(ctx, pool, testCase.kind, testCase.research)
			if err != nil {
				t.Fatal(err)
			}
			if access != AccessHidden {
				t.Fatalf("%s read of a research-only %s = %q, want %q", actor, testCase.kind, access, AccessHidden)
			}
		}
		for _, actor := range []string{"researcher", "collaborator"} {
			access, err := policies[actor].Reference(ctx, pool, testCase.kind, testCase.research)
			if err != nil {
				t.Fatal(err)
			}
			if !access.Allowed() {
				t.Fatalf("%s could not read a research-only %s: %q", actor, testCase.kind, access)
			}
		}
		// A missing row and an unknown family both answer as missing, so neither can
		// be used to find out what exists.
		if access, err := policies["researcher"].Reference(ctx, pool, testCase.kind, uuid.New()); err != nil || access != AccessMissing {
			t.Fatalf("a missing %s = (%q, %v), want (%q, nil)", testCase.kind, access, err, AccessMissing)
		}
		if access, err := policies["researcher"].Reference(ctx, pool, "source", publicPlaceID); err != nil || access != AccessMissing {
			t.Fatalf("an unknown reference family = (%q, %v), want (%q, nil)", access, err, AccessMissing)
		}
	}
}
