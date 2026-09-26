// Package visibility is the single resource visibility policy for the core API.
//
// Every public read path (dictionary, search, claims, research history) resolves
// visibility through Policy so the same rules are applied everywhere:
//
//   - public published data: a person appears in a published version of a public
//     tree, a source is published, a claim rests on at least one public source
//     statement and carries no private source, a question links public evidence.
//   - data owned by or collaborated on by the actor: the actor created the record,
//     or collaborates on a tree that contains the person.
//   - research-only data visible to authorized roles: researcher, moderator and
//     admin roles reach research records, but never through a blanket bypass of
//     global people rows.
//   - reference families: a family, tribe, branch or place is public when its
//     visibility column says so, and research-only otherwise, in which case it is
//     readable by the identity write role set alone. These four tables had no
//     visibility column before db/migrations/0039_reference_visibility.sql, so
//     every row of them was public; the backfill preserved that and the write
//     surface no longer adds to it.
//   - missing or deleted records: reported as AccessMissing so direct-ID reads can
//     answer with one status that does not leak existence.
package visibility

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrForbidden is returned when an actor identifier cannot be interpreted.
var ErrForbidden = errors.New("visibility actor is not permitted")

// Access is the reason a resource is readable. AccessMissing and AccessHidden are
// both unreadable; the difference exists so callers can keep existence and
// permission apart internally while answering callers with a single status.
type Access string

const (
	AccessPublic       Access = "public"
	AccessOwner        Access = "owner"
	AccessCollaborator Access = "collaborator"
	AccessResearch     Access = "research"
	AccessMissing      Access = "missing"
	AccessHidden       Access = "hidden"
)

func (a Access) String() string {
	return string(a)
}

// Allowed reports whether the access level may be read. Missing records and denied
// records are both not allowed.
func (a Access) Allowed() bool {
	return a != AccessMissing && a != AccessHidden
}

// Executor is the query surface Policy needs. Both *pgxpool.Pool and pgx.Tx
// satisfy it, so the policy can be resolved inside or outside a transaction.
type Executor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Policy is the resolved visibility scope of one actor. The zero value is the
// anonymous policy: only public data is readable.
type Policy struct {
	actorID  uuid.UUID
	research bool
	// referenceWrite is the platform role set internal/auth calls IdentityWrite.
	// It is resolved separately from research because the research flag is
	// deliberately narrower - it excludes the platform collaborator role - and
	// widening it would change every existing source, claim and question
	// predicate at once. The reference families get their own flag so they can be
	// scoped by exactly the role their write surface requires, and no other.
	referenceWrite bool
}

// Anonymous returns the policy for a caller without a session.
func Anonymous() Policy {
	return Policy{}
}

// New builds a policy for an actor identifier without touching the database. The
// research role stays false until Load confirms it from user_roles.
func New(actorID string) (Policy, error) {
	trimmed := strings.TrimSpace(actorID)
	if trimmed == "" {
		return Policy{}, nil
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return Policy{}, ErrForbidden
	}
	return Policy{actorID: parsed}, nil
}

// WithResearch returns a policy with the research role resolved by the caller. It
// exists for callers that already resolved the actor's roles. An anonymous caller
// never carries the research role, so the flag is ignored without a session.
func WithResearch(actorID string, research bool) (Policy, error) {
	policy, err := New(actorID)
	if err != nil {
		return Policy{}, err
	}
	if policy.Anonymous() {
		return policy, nil
	}
	policy.research = research
	return policy, nil
}

// Load builds a policy and resolves the actor's research role from user_roles.
func Load(ctx context.Context, executor Executor, actorID string) (Policy, error) {
	policy, err := New(actorID)
	if err != nil {
		return Policy{}, err
	}
	if policy.Anonymous() {
		return policy, nil
	}
	roles, err := resolveRoles(ctx, executor, policy.actorID)
	if err != nil {
		return Policy{}, err
	}
	policy.research = roles.research
	policy.referenceWrite = roles.referenceWrite
	return policy, nil
}

// CanViewResearchHistory reports whether the policy may read research run
// metadata. It is the research role under the name the history endpoints need.
func (p Policy) CanViewResearchHistory() bool {
	return p.research
}

// ActorID returns the actor identifier, or uuid.Nil for anonymous callers.
func (p Policy) ActorID() uuid.UUID {
	return p.actorID
}

// Anonymous reports whether the policy has no actor.
func (p Policy) Anonymous() bool {
	return p.actorID == uuid.Nil
}

// Research reports whether the policy carries a research-only role.
func (p Policy) Research() bool {
	return p.research
}

// CanWriteReferences reports whether the policy holds the role set the identity
// write surface requires. It is the read-side twin of that gate: a reference row
// that has not been published is research, and the people who may write research
// are the people who may read it.
func (p Policy) CanWriteReferences() bool {
	return p.referenceWrite
}

// roleScopes is the two role answers Load needs, read in one round trip so the
// policy costs the same number of queries as it did before the reference families
// were scoped.
type roleScopes struct {
	research       bool
	referenceWrite bool
}

func resolveRoles(ctx context.Context, executor Executor, actorID uuid.UUID) (roleScopes, error) {
	var scopes roleScopes
	err := executor.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $1 AND ur.role IN ('researcher', 'moderator', 'admin')),
			EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $1 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin'))
	`, actorID).Scan(&scopes.research, &scopes.referenceWrite)
	return scopes, err
}

// grant is one named reason a record may be read. The same fragments build the
// direct-ID decision and the list predicate, so the two can never drift apart.
type grant struct {
	access Access
	sql    string
}

func resolve(ctx context.Context, executor Executor, params *Params, table string, id string, grants []grant) (Access, error) {
	selects := make([]string, 0, len(grants)+1)
	selects = append(selects, params.Exists(table, id))
	for _, current := range grants {
		selects = append(selects, current.sql)
	}
	flags := make([]bool, len(grants))
	targets := make([]any, 0, len(grants)+1)
	var exists bool
	targets = append(targets, &exists)
	for index := range flags {
		targets = append(targets, &flags[index])
	}
	if err := executor.QueryRow(ctx, `SELECT `+strings.Join(selects, ", "), params.Args()...).Scan(targets...); err != nil {
		return AccessHidden, err
	}
	if !exists {
		return AccessMissing, nil
	}
	for index, current := range grants {
		if flags[index] {
			return current.access, nil
		}
	}
	return AccessHidden, nil
}

// Person reports whether the policy may read a person record. A research role is
// not a blanket bypass: an unpublished person stays hidden from everyone except
// the actor who created it and collaborators on a tree that contains it.
func (p Policy) Person(ctx context.Context, executor Executor, personID uuid.UUID) (Access, error) {
	if personID == uuid.Nil {
		return AccessMissing, nil
	}
	params := &Params{}
	reference := params.Add(personID)
	grants := personGrants(reference, castUUID(params.Add(p.actorID)))
	return resolve(ctx, executor, params, "people", reference, grants)
}

// Source reports whether the policy may read a source record.
func (p Policy) Source(ctx context.Context, executor Executor, sourceID uuid.UUID) (Access, error) {
	if sourceID == uuid.Nil {
		return AccessMissing, nil
	}
	params := &Params{}
	reference := params.Add(sourceID)
	grants := sourceGrants(reference, castUUID(params.Add(p.actorID)), p.research)
	return resolve(ctx, executor, params, "sources", reference, grants)
}

// Claim reports whether the policy may read a claim record. A claim is public only
// when it rests on at least one public source and carries no private source, which
// keeps the all-or-nothing source rule used by graph retrieval.
func (p Policy) Claim(ctx context.Context, executor Executor, claimID uuid.UUID) (Access, error) {
	if claimID == uuid.Nil {
		return AccessMissing, nil
	}
	params := &Params{}
	reference := params.Add(claimID)
	grants := claimGrants(reference, castUUID(params.Add(p.actorID)), p.research)
	return resolve(ctx, executor, params, "claims", reference, grants)
}

// Question reports whether the policy may read an open question record.
func (p Policy) Question(ctx context.Context, executor Executor, questionID uuid.UUID) (Access, error) {
	if questionID == uuid.Nil {
		return AccessMissing, nil
	}
	params := &Params{}
	reference := params.Add(questionID)
	grants := questionGrants(reference, castUUID(params.Add(p.actorID)), p.research)
	return resolve(ctx, executor, params, "open_questions", reference, grants)
}

// Tree reports whether the policy may read a tree record.
func (p Policy) Tree(ctx context.Context, executor Executor, treeID uuid.UUID) (Access, error) {
	if treeID == uuid.Nil {
		return AccessMissing, nil
	}
	params := &Params{}
	reference := params.Add(treeID)
	grants := treeGrants(reference, castUUID(params.Add(p.actorID)))
	return resolve(ctx, executor, params, "trees", reference, grants)
}

// referenceTables is the closed set of reference families. A caller names a kind
// and the policy looks the table up here, so no statement in this package is ever
// assembled from a caller string and an unknown kind is refused rather than
// interpolated.
var referenceTables = map[string]string{
	"family": "families",
	"tribe":  "tribes",
	"branch": "branches",
	"place":  "places",
}

// ReferenceTable reports the table backing a reference kind. It is exported so the
// services that read a reference family can ask the policy which table the kind
// means instead of repeating the map.
func ReferenceTable(kind string) (string, bool) {
	table, ok := referenceTables[strings.ToLower(strings.TrimSpace(kind))]
	return table, ok
}

// Reference reports whether the policy may read a reference row: a family, a
// tribe, a branch or a place.
//
// A published reference row is public, exactly as it was before it had a
// visibility column. A research-only one is not: it is readable by an actor who
// holds the identity write role, because that is the role whose write surface
// produces it, and by nobody else. A caller that cannot name the kind gets the
// anonymous answer, so an unknown kind can never widen the scope.
func (p Policy) Reference(ctx context.Context, executor Executor, kind string, id uuid.UUID) (Access, error) {
	table, known := ReferenceTable(kind)
	if id == uuid.Nil || !known {
		return AccessMissing, nil
	}
	params := &Params{}
	reference := params.Add(id)
	return resolve(ctx, executor, params, table, reference, referenceGrants(table, reference, p.referenceWrite))
}

// ReferencePredicate returns a SQL boolean expression that is true when a row of
// the named reference family is readable under the policy. kindReference is a SQL
// expression yielding the row id, for example "f.id".
func (p Policy) ReferencePredicate(params *Params, kind, kindReference string) string {
	table, known := ReferenceTable(kind)
	if !known {
		// An unknown family reads as nothing rather than as everything. The safe
		// direction has to be the one a typo lands in.
		return "FALSE"
	}
	return combine(referenceGrants(table, kindReference, p.referenceWrite), false)
}

// PersonPredicate returns a SQL boolean expression that is true when a person row
// is readable under the policy. personReference is a SQL expression yielding a
// person id, for example "p.id".
func (p Policy) PersonPredicate(params *Params, personReference string) string {
	return combine(personGrants(personReference, castUUID(params.Add(p.actorID))), p.research && roleBypassesPeople)
}

// SourcePredicate returns a SQL boolean expression that is true when a source row
// is readable under the policy.
func (p Policy) SourcePredicate(params *Params, sourceReference string) string {
	return combine(sourceGrants(sourceReference, castUUID(params.Add(p.actorID)), p.research), p.research)
}

// ClaimPredicate returns a SQL boolean expression that is true when a claim row is
// readable under the policy. claimReference is a SQL expression yielding a claim
// id, for example "c.id".
func (p Policy) ClaimPredicate(params *Params, claimReference string) string {
	return combine(claimGrants(claimReference, castUUID(params.Add(p.actorID)), p.research), p.research)
}

// QuestionPredicate returns a SQL boolean expression that is true when an open
// question row is readable under the policy.
func (p Policy) QuestionPredicate(params *Params, questionReference string) string {
	return combine(questionGrants(questionReference, castUUID(params.Add(p.actorID)), p.research), p.research)
}

// TreePredicate returns a SQL boolean expression that is true when a tree row is
// readable under the policy.
func (p Policy) TreePredicate(params *Params, treeReference string) string {
	return combine(treeGrants(treeReference, castUUID(params.Add(p.actorID))), p.research && roleBypassesTrees)
}

// PublicSourcePredicate returns a SQL boolean expression that is true only for
// published sources. The deliberately public source dependency graph features use
// it so their public contract does not follow the actor scope.
func PublicSourcePredicate(params *Params, sourceReference string) string {
	return "(" + publicSourceGrant(sourceReference).sql + ")"
}

// A research role must not become a blanket bypass over the global people and tree
// tables. Publishing a public tree version is what makes a person public, and the
// actor scope is what opens a private tree, so neither kind grants "TRUE".
const (
	roleBypassesPeople = false
	roleBypassesTrees  = false
)

func combine(grants []grant, roleBypassesEverything bool) string {
	terms := make([]string, 0, len(grants)+1)
	for _, current := range grants {
		terms = append(terms, current.sql)
	}
	if roleBypassesEverything {
		terms = append(terms, "TRUE")
	}
	return "(" + strings.Join(terms, " OR ") + ")"
}

// The fragments below alias every table with a "vis_" prefix. A predicate is
// embedded in the caller's query, so a plain alias such as "s" or "c" could be
// shadowed by the caller's own alias of the same name and silently turn a scoped
// check into a global one.

func personGrants(personReference, actorReference string) []grant {
	return []grant{
		{access: AccessPublic, sql: fmt.Sprintf(`EXISTS (
			SELECT 1 FROM tree_nodes vis_node
			JOIN tree_versions vis_version ON vis_version.id = vis_node.tree_version_id AND vis_version.state = 'published'
			JOIN trees vis_tree ON vis_tree.id = vis_version.tree_id
			WHERE vis_node.person_id = %s AND vis_tree.visibility = 'public')`, personReference)},
		{access: AccessOwner, sql: fmt.Sprintf(`EXISTS (SELECT 1 FROM people vis_person WHERE vis_person.id = %s AND vis_person.created_by = %s)`, personReference, actorReference)},
		{access: AccessCollaborator, sql: fmt.Sprintf(`EXISTS (
			SELECT 1 FROM tree_nodes vis_node
			JOIN tree_versions vis_version ON vis_version.id = vis_node.tree_version_id
			JOIN trees vis_tree ON vis_tree.id = vis_version.tree_id
			JOIN tree_collaborators vis_collaborator ON vis_collaborator.tree_id = vis_tree.id
			WHERE vis_node.person_id = %s AND vis_collaborator.user_id = %s)`, personReference, actorReference)},
	}
}

// referenceGrants is the published-version rule for the reference families. It is
// the same two-value rule sources use, and the table name comes from the closed
// map above rather than from a request. The privileged grant is the identity write
// role set: a research-only reference row is readable by the people whose write
// surface produces it, and by nobody else. There is deliberately no owner or
// collaborator grant - a reference row belongs to no single interpretation, and a
// right over one tree says nothing about it.
func referenceGrants(table, reference string, privileged bool) []grant {
	grants := []grant{{
		access: AccessPublic,
		sql:    fmt.Sprintf(`EXISTS (SELECT 1 FROM %s vis_reference WHERE vis_reference.id = %s AND vis_reference.visibility = 'public')`, table, reference),
	}}
	if privileged {
		grants = append(grants, grant{access: AccessResearch, sql: "TRUE"})
	}
	return grants
}

func publicSourceGrant(sourceReference string) grant {
	return grant{access: AccessPublic, sql: fmt.Sprintf(`EXISTS (SELECT 1 FROM sources vis_source WHERE vis_source.id = %s AND vis_source.visibility = 'public')`, sourceReference)}
}

func sourceGrants(sourceReference, actorReference string, research bool) []grant {
	grants := []grant{
		publicSourceGrant(sourceReference),
		{access: AccessOwner, sql: fmt.Sprintf(`EXISTS (SELECT 1 FROM sources vis_source WHERE vis_source.id = %s AND vis_source.created_by = %s)`, sourceReference, actorReference)},
	}
	if research {
		grants = append(grants, grant{access: AccessResearch, sql: "TRUE"})
	}
	return grants
}

func publicClaimGrant(claimReference string) grant {
	return grant{access: AccessPublic, sql: fmt.Sprintf(`(
		EXISTS (SELECT 1 FROM claim_evidence vis_evidence
			LEFT JOIN source_statements vis_statement ON vis_statement.id = vis_evidence.source_statement_id
			LEFT JOIN source_passages vis_passage ON vis_passage.id = vis_evidence.source_passage_id
			LEFT JOIN sources vis_source ON vis_source.id = COALESCE(vis_statement.source_id, vis_passage.source_id)
			WHERE vis_evidence.claim_id = %s AND vis_source.visibility = 'public')
		AND NOT EXISTS (SELECT 1 FROM claim_evidence vis_evidence
			LEFT JOIN source_statements vis_statement ON vis_statement.id = vis_evidence.source_statement_id
			LEFT JOIN source_passages vis_passage ON vis_passage.id = vis_evidence.source_passage_id
			LEFT JOIN sources vis_source ON vis_source.id = COALESCE(vis_statement.source_id, vis_passage.source_id)
			WHERE vis_evidence.claim_id = %s AND vis_source.visibility = 'private')
		AND NOT EXISTS (SELECT 1 FROM claim_counter_evidence vis_counter
			LEFT JOIN source_statements vis_statement ON vis_statement.id = vis_counter.source_statement_id
			LEFT JOIN source_passages vis_passage ON vis_passage.id = vis_counter.source_passage_id
			LEFT JOIN sources vis_source ON vis_source.id = COALESCE(vis_statement.source_id, vis_passage.source_id)
			WHERE vis_counter.claim_id = %s AND vis_source.visibility = 'private'))`, claimReference, claimReference, claimReference)}
}

func claimGrants(claimReference, actorReference string, research bool) []grant {
	grants := []grant{
		publicClaimGrant(claimReference),
		{access: AccessOwner, sql: fmt.Sprintf(`EXISTS (SELECT 1 FROM claims vis_claim WHERE vis_claim.id = %s AND vis_claim.created_by = %s)`, claimReference, actorReference)},
	}
	if research {
		grants = append(grants, grant{access: AccessResearch, sql: "TRUE"})
	}
	return grants
}

func publicQuestionGrant(questionReference string) grant {
	linkedClaim := fmt.Sprintf("(SELECT vis_link.claim_id FROM question_claims vis_link WHERE vis_link.question_id = %s)", questionReference)
	return grant{access: AccessPublic, sql: fmt.Sprintf(`(
		EXISTS (SELECT 1 FROM question_sources vis_link JOIN sources vis_source ON vis_source.id = vis_link.source_id WHERE vis_link.question_id = %s AND vis_source.visibility = 'public')
		OR %s)`, questionReference, publicClaimGrant(linkedClaim).sql)}
}

func questionGrants(questionReference, actorReference string, research bool) []grant {
	grants := []grant{
		publicQuestionGrant(questionReference),
		{access: AccessOwner, sql: fmt.Sprintf(`EXISTS (SELECT 1 FROM open_questions vis_question WHERE vis_question.id = %s AND vis_question.created_by = %s)`, questionReference, actorReference)},
	}
	if research {
		grants = append(grants, grant{access: AccessResearch, sql: "TRUE"})
	}
	return grants
}

// treeGrants keeps the same all-or-nothing rule the person grants use: a tree is
// public only once a version of it is published. A public tree whose versions are
// all drafts still holds unpublished interpretations, so it stays closed to
// anonymous callers. The owner and collaborator grants open the draft to the people
// working on it.
func treeGrants(treeReference, actorReference string) []grant {
	return []grant{
		{access: AccessPublic, sql: fmt.Sprintf(`EXISTS (
			SELECT 1 FROM trees vis_tree
			JOIN tree_versions vis_version ON vis_version.tree_id = vis_tree.id AND vis_version.state = 'published'
			WHERE vis_tree.id = %s AND vis_tree.visibility = 'public')`, treeReference)},
		{access: AccessOwner, sql: fmt.Sprintf(`EXISTS (SELECT 1 FROM trees vis_tree WHERE vis_tree.id = %s AND vis_tree.owner_id = %s)`, treeReference, actorReference)},
		{access: AccessCollaborator, sql: fmt.Sprintf(`EXISTS (SELECT 1 FROM trees vis_tree JOIN tree_collaborators vis_collaborator ON vis_collaborator.tree_id = vis_tree.id WHERE vis_tree.id = %s AND vis_collaborator.user_id = %s)`, treeReference, actorReference)},
	}
}
