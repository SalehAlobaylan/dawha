package dictionary

import (
	"context"
	"errors"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("dictionary database is unavailable")
	ErrNotFound            = errors.New("dictionary resource not found")
	ErrValidation          = errors.New("dictionary input is invalid")
	ErrForbidden           = errors.New("dictionary actor is not permitted")
)

type IndexItem struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	NameAR      string `json:"nameAr"`
	SecondaryAR string `json:"secondaryAr,omitempty"`
	Status      string `json:"status,omitempty"`
	Count       int    `json:"count"`
}

type IndexResponse struct {
	Kind  string      `json:"kind"`
	Query string      `json:"query"`
	Items []IndexItem `json:"items"`
}

type AliasView struct {
	ValueAR string `json:"valueAr"`
	Type    string `json:"type"`
}

type ReferenceView struct {
	ID         string `json:"id"`
	NameAR     string `json:"nameAr"`
	DetailAR   string `json:"detailAr,omitempty"`
	Status     string `json:"status,omitempty"`
	SourceType string `json:"sourceType,omitempty"`
}

type TreeReference struct {
	ID         string `json:"id"`
	NameAR     string `json:"nameAr"`
	VersionID  string `json:"versionId"`
	VersionNum int    `json:"versionNumber"`
}

type ClaimReference struct {
	ID            string `json:"id"`
	SubjectType   string `json:"subjectType"`
	SubjectID     string `json:"subjectId"`
	Predicate     string `json:"predicate"`
	ObjectType    string `json:"objectType"`
	ObjectID      string `json:"objectId"`
	Status        string `json:"status"`
	EvidenceCount int    `json:"evidenceCount"`
}

type QuestionReference struct {
	ID        string `json:"id"`
	TitleAR   string `json:"titleAr"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	NoteCount int    `json:"noteCount"`
}

type Detail struct {
	Kind            string              `json:"kind"`
	ID              string              `json:"id"`
	NameAR          string              `json:"nameAr"`
	DescriptionAR   string              `json:"descriptionAr,omitempty"`
	Aliases         []AliasView         `json:"aliases"`
	HistoricalNames []ReferenceView     `json:"historicalNames"`
	Branches        []ReferenceView     `json:"branches"`
	Places          []ReferenceView     `json:"places"`
	Families        []ReferenceView     `json:"families"`
	People          []ReferenceView     `json:"people"`
	Tribes          []ReferenceView     `json:"tribes"`
	PublishedTrees  []TreeReference     `json:"publishedTrees"`
	Claims          []ClaimReference    `json:"claims"`
	Sources         []ReferenceView     `json:"sources"`
	Questions       []QuestionReference `json:"questions"`
	Migrations      []ReferenceView     `json:"migrations"`
}

type Service struct {
	Pool *pgxpool.Pool
}

type dbExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

func (s *Service) ListIndex(ctx context.Context, kind, search, actorID string) (IndexResponse, error) {
	if err := s.ready(); err != nil {
		return IndexResponse{}, err
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return IndexResponse{}, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if !validIndexKind(kind) {
		return IndexResponse{}, ErrValidation
	}
	search = identity.NormalizeArabicName(strings.TrimSpace(search))
	query, args := indexQuery(kind, search, policy)
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return IndexResponse{}, err
	}
	defer rows.Close()
	items := make([]IndexItem, 0)
	for rows.Next() {
		item, err := scanIndex(rows)
		if err != nil {
			return IndexResponse{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return IndexResponse{}, err
	}
	return IndexResponse{Kind: kind, Query: search, Items: items}, nil
}

func (s *Service) Get(ctx context.Context, kind, id, actorID string) (Detail, error) {
	if err := s.ready(); err != nil {
		return Detail{}, err
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return Detail{}, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "families" && kind != "tribes" && kind != "people" && kind != "places" {
		return Detail{}, ErrValidation
	}
	resourceID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return Detail{}, ErrNotFound
	}
	// A person with no published public tree, and no actor scope, must not be
	// readable by direct id either. A denied person answers exactly like a missing
	// one so the endpoint cannot be used as an existence oracle.
	if kind == "people" {
		access, accessErr := policy.Person(ctx, s.Pool, resourceID)
		if accessErr != nil {
			return Detail{}, accessErr
		}
		if !access.Allowed() {
			return Detail{}, ErrNotFound
		}
	}
	var result Detail
	switch kind {
	case "families":
		result, err = s.familyDetail(ctx, resourceID)
	case "tribes":
		result, err = s.tribeDetail(ctx, resourceID)
	case "people":
		result, err = s.personDetail(ctx, policy, resourceID)
	case "places":
		result, err = s.placeDetail(ctx, policy, resourceID)
	}
	if err != nil {
		return Detail{}, err
	}
	if result.Aliases == nil {
		result.Aliases = []AliasView{}
	}
	if result.HistoricalNames == nil {
		result.HistoricalNames = []ReferenceView{}
	}
	if result.Branches == nil {
		result.Branches = []ReferenceView{}
	}
	if result.Places == nil {
		result.Places = []ReferenceView{}
	}
	if result.Families == nil {
		result.Families = []ReferenceView{}
	}
	if result.People == nil {
		result.People = []ReferenceView{}
	}
	if result.Tribes == nil {
		result.Tribes = []ReferenceView{}
	}
	if result.PublishedTrees == nil {
		result.PublishedTrees = []TreeReference{}
	}
	if result.Claims == nil {
		result.Claims = []ClaimReference{}
	}
	if result.Sources == nil {
		result.Sources = []ReferenceView{}
	}
	if result.Questions == nil {
		result.Questions = []QuestionReference{}
	}
	if result.Migrations == nil {
		result.Migrations = []ReferenceView{}
	}
	return result, nil
}

func (s *Service) familyDetail(ctx context.Context, id uuid.UUID) (Detail, error) {
	var item Detail
	var description pgtype.Text
	if err := s.Pool.QueryRow(ctx, `SELECT id, canonical_name_ar, description_ar FROM families WHERE id = $1`, id).Scan(&item.ID, &item.NameAR, &description); err != nil {
		return Detail{}, mapNotFound(err)
	}
	item.Kind = "family"
	item.DescriptionAR = textValue(description)
	var err error
	if item.Aliases, err = s.aliases(ctx, s.Pool, "family_aliases", "family_id", id); err != nil {
		return Detail{}, err
	}
	if item.Branches, err = s.references(ctx, s.Pool, `SELECT b.id, b.canonical_name_ar, f.canonical_name_ar, NULL::text FROM branches b JOIN families f ON f.id = b.family_id WHERE b.family_id = $1 ORDER BY b.canonical_name_ar`, id); err != nil {
		return Detail{}, err
	}
	if item.Places, err = s.references(ctx, s.Pool, `SELECT p.id, p.canonical_name_ar, p.place_type, NULL::text FROM places p JOIN families f ON f.origin_place_id = p.id WHERE f.id = $1`, id); err != nil {
		return Detail{}, err
	}
	return item, nil
}

func (s *Service) tribeDetail(ctx context.Context, id uuid.UUID) (Detail, error) {
	var item Detail
	var description pgtype.Text
	if err := s.Pool.QueryRow(ctx, `SELECT id, canonical_name_ar, description_ar FROM tribes WHERE id = $1`, id).Scan(&item.ID, &item.NameAR, &description); err != nil {
		return Detail{}, mapNotFound(err)
	}
	item.Kind = "tribe"
	item.DescriptionAR = textValue(description)
	var err error
	if item.Aliases, err = s.aliases(ctx, s.Pool, "tribe_aliases", "tribe_id", id); err != nil {
		return Detail{}, err
	}
	return item, nil
}

func (s *Service) personDetail(ctx context.Context, policy visibility.Policy, id uuid.UUID) (Detail, error) {
	var item Detail
	var status string
	var notes pgtype.Text
	if err := s.Pool.QueryRow(ctx, `SELECT id, canonical_name_ar, identity_status, notes_ar FROM people WHERE id = $1`, id).Scan(&item.ID, &item.NameAR, &status, &notes); err != nil {
		return Detail{}, mapNotFound(err)
	}
	item.Kind = "person"
	item.DescriptionAR = textValue(notes)
	var err error
	if item.Aliases, err = s.personAliases(ctx, s.Pool, policy, id); err != nil {
		return Detail{}, err
	}
	// A place reaches this page through an association or a claim, and either can be
	// research-only. Both branches apply the same rule the place page uses: an
	// association backed by a source is kept only while that source is visible, and a
	// claim is kept only while the claim itself is. An association with no source is a
	// platform record, so it stays.
	placeParams := visibility.NewParams()
	personRef := placeParams.Add(id)
	associationSource := policy.SourcePredicate(placeParams, "ga.source_id")
	placeClaim := policy.ClaimPredicate(placeParams, "c.id")
	if item.Places, err = s.references(ctx, s.Pool, `
		SELECT DISTINCT p.id, p.canonical_name_ar, p.place_type, ga.status
		FROM places p JOIN geographic_associations ga ON ga.place_id = p.id
		WHERE ga.entity_type = 'person' AND ga.entity_id = `+personRef+` AND (ga.source_id IS NULL OR `+associationSource+`)
		UNION
		SELECT DISTINCT p.id, p.canonical_name_ar, p.place_type, c.status
		FROM places p JOIN claims c ON c.place_id = p.id
		WHERE `+placeClaim+` AND ((c.subject_type = 'person' AND c.subject_id = `+personRef+`) OR (c.object_type = 'person' AND c.object_id = `+personRef+`))
		ORDER BY 2
	`, placeParams.Args()...); err != nil {
		return Detail{}, err
	}
	if item.PublishedTrees, err = s.publishedTrees(ctx, s.Pool, id); err != nil {
		return Detail{}, err
	}
	if item.Claims, err = s.claims(ctx, s.Pool, policy, id); err != nil {
		return Detail{}, err
	}
	if item.Sources, err = s.personSources(ctx, s.Pool, policy, id); err != nil {
		return Detail{}, err
	}
	if item.Questions, err = s.personQuestions(ctx, s.Pool, policy, id); err != nil {
		return Detail{}, err
	}
	return item, nil
}

func (s *Service) placeDetail(ctx context.Context, policy visibility.Policy, id uuid.UUID) (Detail, error) {
	var item Detail
	var description pgtype.Text
	var placeType string
	if err := s.Pool.QueryRow(ctx, `SELECT id, canonical_name_ar, place_type, notes_ar FROM places WHERE id = $1`, id).Scan(&item.ID, &item.NameAR, &placeType, &description); err != nil {
		return Detail{}, mapNotFound(err)
	}
	item.Kind = "place"
	item.DescriptionAR = textValue(description)
	var err error
	if item.HistoricalNames, err = s.references(ctx, s.Pool, `SELECT h.id, h.name_ar, h.name_type, NULL::text FROM historical_place_names h WHERE h.place_id = $1 ORDER BY h.name_ar`, id); err != nil {
		return Detail{}, err
	}
	peopleParams := visibility.NewParams()
	placeIDRef := peopleParams.Add(id)
	peoplePredicate := policy.PersonPredicate(peopleParams, "p.id")
	if item.People, err = s.references(ctx, s.Pool, `
		SELECT DISTINCT p.id, p.canonical_name_ar, p.identity_status, ga.status
		FROM people p JOIN geographic_associations ga ON ga.entity_id = p.id
		WHERE ga.place_id = `+placeIDRef+` AND ga.entity_type = 'person' AND `+peoplePredicate+`
		ORDER BY 2
	`, peopleParams.Args()...); err != nil {
		return Detail{}, err
	}
	if item.Families, err = s.references(ctx, s.Pool, `SELECT f.id, f.canonical_name_ar, f.description_ar, NULL::text FROM families f WHERE f.origin_place_id = $1 ORDER BY f.canonical_name_ar`, id); err != nil {
		return Detail{}, err
	}
	if item.Tribes, err = s.references(ctx, s.Pool, `SELECT DISTINCT t.id, t.canonical_name_ar, t.description_ar, ga.status FROM tribes t JOIN geographic_associations ga ON ga.entity_id = t.id WHERE ga.place_id = $1 AND ga.entity_type = 'tribe' ORDER BY 2`, id); err != nil {
		return Detail{}, err
	}
	if item.Claims, err = s.placeClaims(ctx, s.Pool, policy, id); err != nil {
		return Detail{}, err
	}
	if item.Sources, err = s.placeSources(ctx, s.Pool, policy, id); err != nil {
		return Detail{}, err
	}
	if item.Questions, err = s.placeQuestions(ctx, s.Pool, policy, id); err != nil {
		return Detail{}, err
	}
	migrationParams := visibility.NewParams()
	placeIDRef = migrationParams.Add(id)
	sourcePredicate := policy.SourcePredicate(migrationParams, "m.source_id")
	if item.Migrations, err = s.references(ctx, s.Pool, `SELECT m.id, COALESCE(pf.canonical_name_ar, pt.canonical_name_ar, 'هجرة'), m.status, m.certainty FROM migration_events m LEFT JOIN places pf ON pf.id = m.from_place_id LEFT JOIN places pt ON pt.id = m.to_place_id WHERE (m.from_place_id = `+placeIDRef+` OR m.to_place_id = `+placeIDRef+`) AND (m.source_id IS NULL OR `+sourcePredicate+`) ORDER BY m.time_from NULLS LAST`, migrationParams.Args()...); err != nil {
		return Detail{}, err
	}
	_ = placeType
	return item, nil
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func validIndexKind(kind string) bool {
	return kind == "families" || kind == "tribes" || kind == "branches" || kind == "people" || kind == "places" || kind == "sources" || kind == "questions" || kind == "disputed-claims"
}

// indexQuery builds the public index list. Every kind declares its public
// membership rule here: people, claims, questions and sources join through the
// central visibility policy, while families, tribes, branches and places have no
// visibility column and stay public. Each case builds its own query so a predicate
// never allocates a placeholder that the executed statement does not use.
func indexQuery(kind, search string, policy visibility.Policy) (string, []any) {
	params := visibility.NewParams()
	term := params.Add(search)
	switch kind {
	case "families":
		return `SELECT f.id, 'family', f.canonical_name_ar, COALESCE((SELECT fa.value_ar FROM family_aliases fa WHERE fa.family_id = f.id ORDER BY fa.created_at LIMIT 1), ''), NULL::text, (SELECT count(*) FROM branches b WHERE b.family_id = f.id) FROM families f WHERE (` + fmtCondition("f.normalized_name_ar", term) + ` OR EXISTS (SELECT 1 FROM family_aliases fa WHERE fa.family_id = f.id AND fa.normalized_value_ar ILIKE '%' || ` + term + ` || '%')) ORDER BY f.canonical_name_ar LIMIT 100`, params.Args()
	case "tribes":
		return `SELECT t.id, 'tribe', t.canonical_name_ar, COALESCE((SELECT ta.value_ar FROM tribe_aliases ta WHERE ta.tribe_id = t.id ORDER BY ta.created_at LIMIT 1), ''), NULL::text, (SELECT count(*) FROM geographic_associations ga WHERE ga.entity_type = 'tribe' AND ga.entity_id = t.id) FROM tribes t WHERE (` + fmtCondition("t.normalized_name_ar", term) + ` OR EXISTS (SELECT 1 FROM tribe_aliases ta WHERE ta.tribe_id = t.id AND ta.normalized_value_ar ILIKE '%' || ` + term + ` || '%')) ORDER BY t.canonical_name_ar LIMIT 100`, params.Args()
	case "branches":
		return `SELECT b.id, 'branch', b.canonical_name_ar, f.canonical_name_ar, NULL::text, (SELECT count(*) FROM branches child WHERE child.parent_branch_id = b.id) FROM branches b JOIN families f ON f.id = b.family_id WHERE (` + fmtCondition("b.normalized_name_ar", term) + ` OR f.normalized_name_ar ILIKE '%' || ` + term + ` || '%') ORDER BY b.canonical_name_ar LIMIT 100`, params.Args()
	case "places":
		return `SELECT p.id, 'place', p.canonical_name_ar, COALESCE((SELECT hp.name_ar FROM historical_place_names hp WHERE hp.place_id = p.id ORDER BY hp.created_at LIMIT 1), ''), p.place_type, (SELECT count(*) FROM historical_place_names hp WHERE hp.place_id = p.id) FROM places p WHERE (` + fmtCondition("p.normalized_name_ar", term) + ` OR EXISTS (SELECT 1 FROM historical_place_names hp WHERE hp.place_id = p.id AND hp.name_ar ILIKE '%' || ` + term + ` || '%')) ORDER BY p.canonical_name_ar LIMIT 100`, params.Args()
	case "sources":
		sourcePredicate := policy.SourcePredicate(params, "s.id")
		return `SELECT s.id, 'source', s.title_ar, COALESCE(s.author_ar, ''), s.source_type, (SELECT count(*) FROM source_statements ss WHERE ss.source_id = s.id) FROM sources s WHERE ` + sourcePredicate + ` AND (` + term + ` = '' OR s.title_ar ILIKE '%' || ` + term + ` || '%' OR COALESCE(s.author_ar, '') ILIKE '%' || ` + term + ` || '%') ORDER BY s.title_ar LIMIT 100`, params.Args()
	case "people":
		personPredicate := policy.PersonPredicate(params, "p.id")
		// Every alias reference in this statement follows one rule: an alias whose
		// source is not visible to the actor may not be shown, counted, or used as a
		// search term. Without the third gate an alias taken from a private source
		// still worked as a probe: a hit proved the alias exists and belongs to that
		// person.
		aliasSource := policy.SourcePredicate(params, "pa.source_id")
		visibleAlias := `(pa.source_id IS NULL OR ` + aliasSource + `)`
		return `SELECT p.id, 'person', p.canonical_name_ar, COALESCE((SELECT pa.value_ar FROM person_aliases pa WHERE pa.person_id = p.id AND ` + visibleAlias + ` ORDER BY pa.created_at LIMIT 1), ''), p.identity_status, (SELECT count(*) FROM person_aliases pa WHERE pa.person_id = p.id AND ` + visibleAlias + `) FROM people p WHERE ` + personPredicate + ` AND (` + fmtCondition("p.normalized_name_ar", term) + ` OR EXISTS (SELECT 1 FROM person_aliases pa WHERE pa.person_id = p.id AND ` + visibleAlias + ` AND pa.normalized_value_ar ILIKE '%' || ` + term + ` || '%')) ORDER BY p.canonical_name_ar LIMIT 100`, params.Args()
	case "questions":
		questionPredicate := policy.QuestionPredicate(params, "q.id")
		return `SELECT q.id, 'question', q.title_ar, COALESCE(q.description_ar, ''), q.status, (SELECT count(*) FROM question_notes qn WHERE qn.question_id = q.id) FROM open_questions q WHERE ` + questionPredicate + ` AND (` + term + ` = '' OR q.title_ar ILIKE '%' || ` + term + ` || '%' OR COALESCE(q.description_ar, '') ILIKE '%' || ` + term + ` || '%') ORDER BY q.updated_at DESC LIMIT 100`, params.Args()
	case "disputed-claims":
		claimPredicate := policy.ClaimPredicate(params, "c.id")
		// The subject and object names come from people rows, so the person policy
		// belongs in the join condition. An unreadable person does not join, the
		// COALESCE fallback reports the id the row already exposes, and the name of a
		// private draft person cannot reach the label or match the search term.
		subjectPerson := policy.PersonPredicate(params, "ps.id")
		objectPerson := policy.PersonPredicate(params, "po.id")
		return `SELECT c.id, 'claim', COALESCE(ps.canonical_name_ar, c.subject_id::text) || ' ← ' || COALESCE(po.canonical_name_ar, c.object_id::text), c.predicate, c.status, (SELECT count(*) FROM claim_evidence ce WHERE ce.claim_id = c.id) FROM claims c LEFT JOIN people ps ON c.subject_type = 'person' AND ps.id = c.subject_id AND ` + subjectPerson + ` LEFT JOIN people po ON c.object_type = 'person' AND po.id = c.object_id AND ` + objectPerson + ` WHERE ` + claimPredicate + ` AND c.status IN ('disputed', 'contested') AND (` + term + ` = '' OR COALESCE(ps.canonical_name_ar, c.subject_id::text) ILIKE '%' || ` + term + ` || '%' OR COALESCE(po.canonical_name_ar, c.object_id::text) ILIKE '%' || ` + term + ` || '%') ORDER BY c.updated_at DESC LIMIT 100`, params.Args()
	}
	return "", nil
}

func fmtCondition(column, parameter string) string {
	return parameter + " = '' OR " + column + " ILIKE '%' || " + parameter + " || '%'"
}

func scanIndex(row pgx.Row) (IndexItem, error) {
	var item IndexItem
	var secondary pgtype.Text
	var status pgtype.Text
	if err := row.Scan(&item.ID, &item.Kind, &item.NameAR, &secondary, &status, &item.Count); err != nil {
		return IndexItem{}, err
	}
	item.SecondaryAR = textValue(secondary)
	item.Status = textValue(status)
	return item, nil
}

// aliases reads the aliases of a family or a tribe. Neither table carries a source
// column, so there is nothing to scope and the rows stay public.
func (s *Service) aliases(ctx context.Context, q dbExecutor, table, column string, id uuid.UUID) ([]AliasView, error) {
	rows, err := q.Query(ctx, `SELECT value_ar, alias_type FROM `+table+` WHERE `+column+` = $1 ORDER BY created_at, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AliasView, 0)
	for rows.Next() {
		var item AliasView
		if err := rows.Scan(&item.ValueAR, &item.Type); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// personAliases reads the aliases of a person. person_aliases can record the source a
// spelling was taken from, so an alias derived from a research-only source must not
// reach a public reader: on a published person page that alias is a way to disclose a
// private source. An alias with no source is a platform record and stays.
func (s *Service) personAliases(ctx context.Context, q dbExecutor, policy visibility.Policy, personID uuid.UUID) ([]AliasView, error) {
	params := visibility.NewParams()
	personRef := params.Add(personID)
	sourcePredicate := policy.SourcePredicate(params, "pa.source_id")
	rows, err := q.Query(ctx, `SELECT pa.value_ar, pa.alias_type FROM person_aliases pa WHERE pa.person_id = `+personRef+` AND (pa.source_id IS NULL OR `+sourcePredicate+`) ORDER BY pa.created_at, pa.id`, params.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AliasView, 0)
	for rows.Next() {
		var item AliasView
		if err := rows.Scan(&item.ValueAR, &item.Type); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) references(ctx context.Context, q dbExecutor, query string, args ...any) ([]ReferenceView, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ReferenceView, 0)
	for rows.Next() {
		var item ReferenceView
		var detail, status pgtype.Text
		if err := rows.Scan(&item.ID, &item.NameAR, &detail, &status); err != nil {
			return nil, err
		}
		item.DetailAR = textValue(detail)
		item.Status = textValue(status)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) publishedTrees(ctx context.Context, q dbExecutor, personID uuid.UUID) ([]TreeReference, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT t.id, t.name_ar, v.id, v.version_number
		FROM trees t JOIN tree_versions v ON v.tree_id = t.id AND v.state = 'published'
		JOIN tree_nodes n ON n.tree_version_id = v.id
		WHERE t.visibility = 'public' AND n.person_id = $1
		ORDER BY t.name_ar, v.version_number DESC
	`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TreeReference, 0)
	for rows.Next() {
		var item TreeReference
		if err := rows.Scan(&item.ID, &item.NameAR, &item.VersionID, &item.VersionNum); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) claims(ctx context.Context, q dbExecutor, policy visibility.Policy, personID uuid.UUID) ([]ClaimReference, error) {
	params := visibility.NewParams()
	personRef := params.Add(personID)
	predicate := policy.ClaimPredicate(params, "c.id")
	return s.claimRefs(ctx, q, `SELECT c.id, c.subject_type, c.subject_id, c.predicate, c.object_type, c.object_id, c.status, (SELECT count(*) FROM claim_evidence ce WHERE ce.claim_id = c.id) FROM claims c WHERE `+predicate+` AND ((c.subject_type = 'person' AND c.subject_id = `+personRef+`) OR (c.object_type = 'person' AND c.object_id = `+personRef+`)) ORDER BY c.updated_at DESC`, params.Args()...)
}

func (s *Service) placeClaims(ctx context.Context, q dbExecutor, policy visibility.Policy, placeID uuid.UUID) ([]ClaimReference, error) {
	params := visibility.NewParams()
	placeRef := params.Add(placeID)
	predicate := policy.ClaimPredicate(params, "c.id")
	return s.claimRefs(ctx, q, `SELECT c.id, c.subject_type, c.subject_id, c.predicate, c.object_type, c.object_id, c.status, (SELECT count(*) FROM claim_evidence ce WHERE ce.claim_id = c.id) FROM claims c WHERE `+predicate+` AND c.place_id = `+placeRef+` ORDER BY c.updated_at DESC`, params.Args()...)
}

func (s *Service) claimRefs(ctx context.Context, q dbExecutor, query string, args ...any) ([]ClaimReference, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ClaimReference, 0)
	for rows.Next() {
		var item ClaimReference
		var subjectID, objectID pgtype.UUID
		if err := rows.Scan(&item.ID, &item.SubjectType, &subjectID, &item.Predicate, &item.ObjectType, &objectID, &item.Status, &item.EvidenceCount); err != nil {
			return nil, err
		}
		item.SubjectID = uuidString(subjectID)
		item.ObjectID = uuidString(objectID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) personSources(ctx context.Context, q dbExecutor, policy visibility.Policy, personID uuid.UUID) ([]ReferenceView, error) {
	params := visibility.NewParams()
	personRef := params.Add(personID)
	predicate := policy.SourcePredicate(params, "s.id")
	return s.references(ctx, q, `SELECT DISTINCT s.id, s.title_ar, s.source_type, ce.relation FROM sources s JOIN source_statements ss ON ss.source_id = s.id JOIN claim_evidence ce ON ce.source_statement_id = ss.id JOIN claims c ON c.id = ce.claim_id WHERE `+predicate+` AND ((c.subject_type = 'person' AND c.subject_id = `+personRef+`) OR (c.object_type = 'person' AND c.object_id = `+personRef+`)) ORDER BY s.title_ar`, params.Args()...)
}

func (s *Service) placeSources(ctx context.Context, q dbExecutor, policy visibility.Policy, placeID uuid.UUID) ([]ReferenceView, error) {
	params := visibility.NewParams()
	placeRef := params.Add(placeID)
	predicate := policy.SourcePredicate(params, "s.id")
	return s.references(ctx, q, `SELECT DISTINCT s.id, s.title_ar, s.source_type, ga.status FROM sources s JOIN geographic_associations ga ON ga.source_id = s.id WHERE `+predicate+` AND ga.place_id = `+placeRef+` UNION SELECT DISTINCT s.id, s.title_ar, s.source_type, m.status FROM sources s JOIN migration_events m ON m.source_id = s.id WHERE `+predicate+` AND (m.from_place_id = `+placeRef+` OR m.to_place_id = `+placeRef+`) ORDER BY 2`, params.Args()...)
}

func (s *Service) personQuestions(ctx context.Context, q dbExecutor, policy visibility.Policy, personID uuid.UUID) ([]QuestionReference, error) {
	params := visibility.NewParams()
	personRef := params.Add(personID)
	predicate := policy.QuestionPredicate(params, "q.id")
	return s.questionRefs(ctx, q, `SELECT DISTINCT q.id, q.title_ar, q.status, q.priority, (SELECT count(*) FROM question_notes qn WHERE qn.question_id = q.id) FROM open_questions q JOIN question_claims qc ON qc.question_id = q.id JOIN claims c ON c.id = qc.claim_id WHERE `+predicate+` AND ((c.subject_type = 'person' AND c.subject_id = `+personRef+`) OR (c.object_type = 'person' AND c.object_id = `+personRef+`)) ORDER BY q.title_ar`, params.Args()...)
}

func (s *Service) placeQuestions(ctx context.Context, q dbExecutor, policy visibility.Policy, placeID uuid.UUID) ([]QuestionReference, error) {
	params := visibility.NewParams()
	placeRef := params.Add(placeID)
	predicate := policy.QuestionPredicate(params, "q.id")
	return s.questionRefs(ctx, q, `SELECT DISTINCT q.id, q.title_ar, q.status, q.priority, (SELECT count(*) FROM question_notes qn WHERE qn.question_id = q.id) FROM open_questions q JOIN question_claims qc ON qc.question_id = q.id JOIN claims c ON c.id = qc.claim_id WHERE `+predicate+` AND c.place_id = `+placeRef+` ORDER BY q.title_ar`, params.Args()...)
}

func (s *Service) questionRefs(ctx context.Context, q dbExecutor, query string, args ...any) ([]QuestionReference, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QuestionReference, 0)
	for rows.Next() {
		var item QuestionReference
		if err := rows.Scan(&item.ID, &item.TitleAR, &item.Status, &item.Priority, &item.NoteCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
