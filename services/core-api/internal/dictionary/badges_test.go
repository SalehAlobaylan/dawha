package dictionary

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// TestTribeBadgeTalliesMatchAnIndependentDerivation is the 031-B acceptance:
// the badge trio equals what a hand-written query over the same fixture yields
// for the same reader. The derivation is written out with literals and the
// fixture's known grants — reusing claimBadgeSelect would be the code under
// test explaining itself.
//
// The fixture carries the semantics the badges promise (stipulation 1): two
// receipts from one source are one receipt (DISTINCT sources, never passage
// counts); a source may stand on both sides of one claim (supporting and
// contradicting are different tallies, not a partition); a claim whose
// evidence set carries a private source is not public as a unit, so none of
// its rows reach the plain reader; an evidence-less claim tallies zeros and
// stays visible — بلا شواهد is a state, not an absence.
func TestTribeBadgeTalliesMatchAnIndependentDerivation(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.Register(t, fixture, "مالك لوحة العهد")
	fixture.GrantRole(owner.User.ID, "researcher")
	reader := actor.Register(t, fixture, "قارئ لوحة العهد")

	tribe := uuid.New()
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'قبيلة العهود', 'قبيله العهود', 'public', $2)`, tribe, owner.User.ID)

	// Three public sources and the owner's private one.
	publicSourceA := uuid.New()
	publicSourceB := uuid.New()
	publicSourceC := uuid.New()
	privateSource := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر أ', 'book', 'public', $2)`, publicSourceA, owner.User.ID)
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر ب', 'book', 'public', $2)`, publicSourceB, owner.User.ID)
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر ج', 'book', 'public', $2)`, publicSourceC, owner.User.ID)
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر الخصوصية', 'book', 'private', $2)`, privateSource, owner.User.ID)

	passages := map[string]uuid.UUID{
		"a1":      uuid.New(),
		"a2":      uuid.New(),
		"b":       uuid.New(),
		"c1":      uuid.New(),
		"c2":      uuid.New(),
		"private": uuid.New(),
	}
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'نص أ', 'نص أ')`, passages["a1"], publicSourceA)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 2, 'نص أ الثاني', 'نص أ الثاني')`, passages["a2"], publicSourceA)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'نص ب', 'نص ب')`, passages["b"], publicSourceB)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'نص ج', 'نص ج')`, passages["c1"], publicSourceC)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 2, 'نص ج الثاني', 'نص ج الثاني')`, passages["c2"], publicSourceC)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'نص الخصوصية', 'نص الخصوصية')`, passages["private"], privateSource)

	newClaim := func(status, notes string) uuid.UUID {
		id := uuid.New()
		fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
			VALUES ($1, 'tribe', $2, 'originated_in', 'place', $3, $4, $5, $6)`,
			id, tribe, uuid.New(), status, notes, owner.User.ID)
		return id
	}
	newEvidence := func(claim uuid.UUID, relation string, passage uuid.UUID) {
		fixture.Exec(`INSERT INTO claim_evidence (id, claim_id, source_passage_id, relation, created_by)
			VALUES ($1, $2, $3, $4, $5)`, uuid.New(), claim, passage, relation, owner.User.ID)
	}

	// ClaimAlpha: two supports (source أ — two passages, one source), one
	// support (source ب), one contextualize (source أ again), one contradicts
	// (the private source — which makes the claim a non-public unit).
	claimAlpha := newClaim("documented", "الادعاء الأول")
	newEvidence(claimAlpha, "supports", passages["a1"])
	newEvidence(claimAlpha, "supports", passages["a2"])
	newEvidence(claimAlpha, "supports", passages["b"])
	newEvidence(claimAlpha, "contextualizes", passages["a1"])
	newEvidence(claimAlpha, "contradicts", passages["private"])

	// ClaimBeta: two supports and one refutes, all from ONE source (ج) — the
	// same source may stand on both sides; three receipts, two tallies of one.
	claimBeta := newClaim("documented", "الادعاء الثاني")
	newEvidence(claimBeta, "supports", passages["c1"])
	newEvidence(claimBeta, "supports", passages["c2"])
	newEvidence(claimBeta, "refutes", passages["c2"])

	// ClaimGamma: no evidence at all — zeros that stay visible.
	claimGamma := newClaim("documented", "الادعاء الثالث")

	// ClaimDelta: documented with two supports, one of them private — not a
	// public unit, and two supporting sources for its own reader (the owner).
	claimDelta := newClaim("documented", "الادعاء المخصوم")
	newEvidence(claimDelta, "supports", passages["b"])
	newEvidence(claimDelta, "supports", passages["private"])

	service := NewService(fixture.Pool())

	// The PLAIN READER: the claim-system's unit rule is the privacy wall; what
	// they get is Beta (all-public evidence) and the visible zero (Gamma).
	readerDetail, err := service.Get(fixture.Ctx(), "tribes", tribe.String(), reader.User.ID)
	if err != nil {
		t.Fatalf("reader detail: %v", err)
	}
	if readerDetail.BadgeCaption != EvidenceBadgeCaption {
		t.Fatalf("badge caption = %q", readerDetail.BadgeCaption)
	}
	readerClaims := map[uuid.UUID]ClaimReference{}
	for _, claim := range readerDetail.Claims {
		readerClaims[uuid.MustParse(claim.ID)] = claim
	}
	if _, present := readerClaims[claimAlpha]; present {
		t.Fatalf("the claim carrying a private contaminant leaked to a plain reader")
	}
	if _, present := readerClaims[claimDelta]; present {
		t.Fatalf("the private-backed claim leaked to a plain reader")
	}
	if len(readerDetail.Claims) != 1 {
		t.Fatalf("reader claims = %d, want just Beta: the claim-system's own rule keeps an evidence-less claim out of the public dossier, and the private-backed claims keep themselves out", len(readerDetail.Claims))
	}
	readerBeta := readerClaims[claimBeta]
	if readerBeta.SupportingSources != 1 || readerBeta.ContradictingSources != 1 || readerBeta.MentionOnlySources != 0 {
		t.Fatalf("reader Beta badges = %+v, want {1, 1, 0}", readerBeta)
	}

	// THE OWNER: their private source is theirs to read, so the badges count it;
	// and the evidence-less claim is theirs own, so its zeros are theirs to see
	// (بلا شواهد is the owner's honest state as well).
	ownerDetail, err := service.Get(fixture.Ctx(), "tribes", tribe.String(), owner.User.ID)
	if err != nil {
		t.Fatalf("owner detail: %v", err)
	}
	ownerClaims := map[uuid.UUID]ClaimReference{}
	for _, claim := range ownerDetail.Claims {
		ownerClaims[uuid.MustParse(claim.ID)] = claim
	}
	if len(ownerClaims) != 4 {
		t.Fatalf("owner claims = %d, want all four", len(ownerClaims))
	}
	ownerGamma := ownerClaims[claimGamma]
	if ownerGamma.SupportingSources != 0 || ownerGamma.ContradictingSources != 0 || ownerGamma.MentionOnlySources != 0 {
		t.Fatalf("owner Gamma badges = %+v, want the honest zeros", ownerGamma)
	}
	if ownerAlpha := ownerClaims[claimAlpha]; ownerAlpha.SupportingSources != 2 || ownerAlpha.ContradictingSources != 1 || ownerAlpha.MentionOnlySources != 1 {
		t.Fatalf("owner Alpha badges = %+v, want {2, 1, 1}", ownerAlpha)
	}
	// The DISTINCT pin in its purest form: Beta carries THREE receipt rows from
	// ONE source; the badge is one per side — the same source on both sides.
	if ownerBeta := ownerClaims[claimBeta]; ownerBeta.SupportingSources != 1 || ownerBeta.ContradictingSources != 1 || ownerBeta.MentionOnlySources != 0 {
		t.Fatalf("owner Beta badges = %+v, want one supporting and one contradicting source", ownerBeta)
	}
	if delta := ownerClaims[claimDelta]; delta.SupportingSources != 2 {
		t.Fatalf("owner Delta badges = %+v, want 2 supporting sources", delta)
	}

	// The independent derivation for one claim, written the way the page's
	// caption words it: distinct sources with public-or-own grants.
	var supports int
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `
		SELECT count(DISTINCT src.id)
		FROM claim_evidence ce
		JOIN claims c ON c.id = ce.claim_id
		LEFT JOIN source_passages sp ON sp.id = ce.source_passage_id
		LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
		LEFT JOIN sources src ON src.id = COALESCE(sp.source_id, ss.source_id)
		WHERE c.id = $1 AND ce.relation = 'supports'
		  AND (src.id IS NULL OR src.visibility = 'public' OR src.created_by = $2)`,
		claimAlpha, owner.User.ID).Scan(&supports); err != nil {
		t.Fatalf("comparator: %v", err)
	}
	if supports != 2 {
		t.Fatalf("the independent derivation reads %d supporting sources; the badge must equal it", supports)
	}
}

// TestBadgeTalliesCollapseReuseLineages is the 034-B acceptance in its purest
// form: two sources' passages inside ONE reuse cluster count as ONE lineage.
// Three supporting passages from three sources — but two of them are in one
// reuse cluster — so the badge reads 2 lineages, not 3. The tallies' change
// is never silent: the response's tallySemantics field says so.
func TestBadgeTalliesCollapseReuseLineages(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.Register(t, fixture, "مالك الشارات")
	fixture.GrantRole(owner.User.ID, "researcher")

	sourceA := uuid.New()
	sourceB := uuid.New()
	sourceC := uuid.New()
	tribeID := uuid.New()
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'قبيلة الشارات', 'قبيله الشارات', 'public', $2)`, tribeID, owner.User.ID)
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'المصدر أ', 'book', 'public', $2)`, sourceA, owner.User.ID)
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'المصدر ب', 'book', 'public', $2)`, sourceB, owner.User.ID)
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'المصدر ج', 'book', 'public', $2)`, sourceC, owner.User.ID)

	passages := map[string]uuid.UUID{
		"a": uuid.New(),
		"b": uuid.New(),
		"c": uuid.New(),
	}
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'نص أ', 'نص أ')`, passages["a"], sourceA)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'نص ب', 'نص ب')`, passages["b"], sourceB)
	fixture.Exec(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, $2, 1, 'نص ج', 'نص ج')`, passages["c"], sourceC)

	claim := uuid.New()
	fixture.Exec(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
		VALUES ($1, 'tribe', $2, 'originated_in', 'place', $3, 'documented', 'الادعاء المُجمّع', $4)`,
		claim, tribeID, uuid.New(), owner.User.ID)

	// THREE supporting evidence rows from three sources — but أ and ب are in
	// one reuse cluster, so the badge reads TWO lineages, not three.
	for _, passage := range []string{"a", "b", "c"} {
		fixture.Exec(`INSERT INTO claim_evidence (id, claim_id, source_passage_id, relation, created_by)
			VALUES ($1, $2, $3, 'supports', $4)`, uuid.New(), claim, passages[passage], owner.User.ID)
	}

	// The reuse cluster: أ + ب's passages are one LINEAGE (the engine's own
	// committed artifacts carry the same shape's member rows).
	clusterID := "reuse-" + uuid.New().String()[:12]
	fixture.Exec(`INSERT INTO text_reuse_clusters (id, method, threshold, pair_count)
		VALUES ($1, 'jaccard_shingle_5', 0.55, 1)`, clusterID)
	for _, passage := range []string{"a", "b"} {
		fixture.Exec(`INSERT INTO text_reuse_members (cluster_id, passage_id, similarity, excerpt)
			VALUES ($1, $2, 1.0, 'نص أ/ب')`, clusterID, passages[passage])
	}

	service := NewService(fixture.Pool())
	detail, err := service.Get(fixture.Ctx(), "tribes", tribeID.String(), owner.User.ID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	// THE COLLAPSE: three passages of THREE sources yield TWO lineages — the
	// cluster (أ+ب) is one, and ج is its own singleton — so supporting = 2,
	// and the semantics field carries the version that explains it.
	if len(detail.Claims) == 0 {
		t.Fatalf("no claims reached the reader")
	}
	var actual ClaimReference
	for _, item := range detail.Claims {
		if item.ID == claim.String() {
			actual = item
			break
		}
	}
	if actual.SupportingSources != 2 {
		t.Fatalf("supporting = %d, want 2 lineages (the cluster counts once, the singleton once)", actual.SupportingSources)
	}
	if detail.TallySemantics != TallySemanticsIndependence {
		t.Fatalf("tallySemantics = %q, want the versioned response", detail.TallySemantics)
	}
}
