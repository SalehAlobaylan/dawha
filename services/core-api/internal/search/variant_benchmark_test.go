package search

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The variant benchmark (plan 028-E): search speaks يثرب → المدينة without a
// line of search code changing. The place branch already scores
// historical_place_names.normalized_name_ar ILIKE at 85 and trigram
// similarity at 70 (service.go), so each fixture below certifies that one
// imported row is what makes the query land — measured recall, not faith.
//
// The fixture set is the committed subset `infra/local/name-variants.json`
// (Wikidata CC0, sha256 in its header), narrowed to the 12 pairs that
// exercise substring-only, diacritic and historic-name arms. The test seeds
// its own canonical places (the demo seed has no مكة/المدينة/الطائف place,
// so those two cannot be demo asserts) and inserts the variant rows with a
// source, exactly as the builder's seed does.
func TestVariantBenchmarkRetrievesCanonicalPlace(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.Register(t, fixture, "صاحب المفاتيح")
	fixture.GrantRole(owner.User.ID, "researcher")

	// The Wikidata sources row and the canonical places. Sources metadata
	// carries the CC0 provenance the builder records.
	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
		VALUES ($1, 'أسماء بديلة من ويكي بيانات (CC0)', 'website', 'public',
		        jsonb_build_object('license', 'CC0', 'endpoint', 'https://query.wikidata.org/sparql', 'fetched_at', '2026-11-08'), $2)`,
		sourceID, owner.User.ID)

	canonicals := []struct {
		name string
		id   uuid.UUID
	}{
		{"الرياض", uuid.UUID{}},         // demo place: reuse by name lookup below
		{"الأحساء", uuid.UUID{}},        // demo place
		{"العلا", uuid.UUID{}},          // demo place
		{"مكة المكرمة", uuid.New()},     // seeded here
		{"المدينة المنورة", uuid.New()}, // seeded here
		{"الطائف", uuid.New()},          // seeded here
	}
	placeByName := map[string]uuid.UUID{}
	for _, canonical := range canonicals {
		if canonical.id == (uuid.UUID{}) {
			continue
		}
		fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility, created_by)
			VALUES ($1, $2, $3, 'city', ST_SetSRID(ST_MakePoint(46.6, 24.6), 4326), 'public', $4)`,
			canonical.id, canonical.name, identity.NormalizeArabicName(canonical.name), owner.User.ID)
	}
	// Demo places already exist under their canonical names; resolve them.
	for _, name := range []string{"الرياض", "الأحساء", "العلا"} {
		placeByName[name] = mustPlaceUUID(t, fixture, name)
	}
	for _, canonical := range canonicals {
		if canonical.id != (uuid.UUID{}) {
			placeByName[canonical.name] = canonical.id
		}
	}

	fixtures := []struct {
		query     string
		canonical string
		nameType  string
	}{
		{"يثرب", "المدينة المنورة", "historical"},
		{"طيبة", "المدينة المنورة", "historical"},
		{"المدينة", "المدينة المنورة", "alternative"},
		{"مدينة رسول الله", "المدينة المنورة", "alternative"},
		{"بكة", "مكة المكرمة", "historical"},
		{"أم القرى", "مكة المكرمة", "historical"},
		{"البلد الأمين", "مكة المكرمة", "historical"},
		{"اليمامة", "الرياض", "historical"},
		{"هفوف", "الأحساء", "alternative"},
		{"مدينة العلا", "العلا", "alternative"},
		{"محافظة العلا", "العلا", "alternative"},
		{"طائف", "الطائف", "alternative"},
	}
	for _, variant := range fixtures {
		placeID := placeByName[variant.canonical]
		if placeID == (uuid.UUID{}) {
			t.Fatalf("fixture canonical %q has no place", variant.canonical)
		}
		normalized := identity.NormalizeArabicName(variant.query)
		// The display spelling the builder ships (name_ar) and the query may
		// differ only in hamza/ة: the indexed value is the normaliser's seat.
		fixture.Exec(`INSERT INTO historical_place_names (place_id, name_ar, normalized_name_ar, name_type, source_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (place_id, name_type, normalized_name_ar) DO NOTHING`,
			placeID, variant.query, normalized, variant.nameType, sourceID)
		// Every imported row carries a source: checked directly, not assumed.
		var rows int
		if err := fixture.Pool().QueryRow(fixture.Ctx(),
			`SELECT count(*) FROM historical_place_names WHERE place_id = $1 AND normalized_name_ar = $2 AND source_id IS NOT NULL`,
			placeID, normalized).Scan(&rows); err != nil {
			t.Fatalf("check source_id for %q: %v", variant.query, err)
		}
		if rows != 1 {
			t.Fatalf("variant %q (§ %s) rows = %d, want 1 sourced row", variant.query, variant.canonical, rows)
		}
	}

	// Duplicate impossible: a second insert of the same triple is refused by
	// the 0062 unique index (not silently doubled).
	second := fixtures[0]
	_, err := fixture.Pool().Exec(fixture.Ctx(), `INSERT INTO historical_place_names (place_id, name_ar, normalized_name_ar, name_type, source_id)
		VALUES ($1, $2, $3, $4, $5)`,
		placeByName[second.canonical], second.query, identity.NormalizeArabicName(second.query), second.nameType, sourceID)
	if err == nil {
		t.Fatalf("duplicate variant insert succeeded; the unique guard is not holding")
	}

	service := NewService(fixture.Pool())
	hits := 0
	for _, variant := range fixtures {
		t.Run(variant.query, func(t *testing.T) {
			response, err := service.Search(fixture.Ctx(), Input{Query: variant.query, Kind: "names", Limit: 20})
			if err != nil {
				t.Fatalf("search %q: %v", variant.query, err)
			}
			var names *Group
			for i := range response.Groups {
				if response.Groups[i].Key == "names" {
					names = &response.Groups[i]
					break
				}
			}
			if names == nil {
				t.Fatalf("no names group for %q", variant.query)
			}
			wantID := placeByName[variant.canonical].String()
			found := false
			for _, item := range names.Items {
				if item.ID == wantID && item.Kind == "place" {
					if item.Score < 85 {
						t.Fatalf("variant %q scored %v (< 85): the variant arm did not fire", variant.query, item.Score)
					}
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("variant %q did not retrieve canonical %q (ids: %v)", variant.query, variant.canonical, names.Items)
			}
			hits++
		})
	}
	if hits != len(fixtures) {
		t.Fatalf("benchmark recall = %d/%d", hits, len(fixtures))
	}
	t.Logf("variant benchmark: %d/%d retrieved", hits, len(fixtures))
}

func mustPlaceUUID(t *testing.T, fixture *testsupport.Fixture, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := fixture.Pool().QueryRow(fixture.Ctx(), `SELECT id FROM places WHERE canonical_name_ar = $1 ORDER BY created_at, id LIMIT 1`, name).Scan(&id); err != nil {
		t.Fatalf("resolve place %q: %v", name, err)
	}
	return id
}
