package geography

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The corridor lane's own contract tests (plan 033-B/C): the schema's own
// rules, pinned where they bite, plus the corridor read's bounded shape.
//
// 1. NO CORRIDOR WITHOUT source_id — the column is NOT NULL; the insert that
//    skips it is refused at the door.
// 2. The legs' day counts are POSITIVE — zero and negative are refused.
// 3. The sequence's HEAD carries no leg (no mode, no day count).
// 4. The read answers its subject's own rows only, bounded + truncated.

func TestCorridorSchemaRulesBiteAtTheDoor(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.Register(t, fixture, "مراجع الممر")
	fixture.GrantRole(owner.User.ID, "researcher")

	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر المرحلة', 'book', 'public', $2)`, sourceID, owner.User.ID)
	tribeID := uuid.New()
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'قبيلة قواعد الممر', 'قبيلة قواعد الممر', 'public', $2)`, tribeID, owner.User.ID)
	placeID := uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility)
		VALUES ($1, 'موضع القاعدة', 'موضع القاعدة', 'city', ST_SetSRID(ST_MakePoint(45, 25), 4326), 'public')`, placeID)

	// Rule 1: no corridor without its source.
	if _, err := fixture.Pool().Exec(fixture.Ctx(), `INSERT INTO movement_corridors (subject_type, subject_id) VALUES ('tribe', $1)`, tribeID); err == nil {
		t.Fatalf("a corridor without a source_id was accepted — the plan's own acceptance failed at the door")
	}

	// The honest corridor with its source, then the leg rules.
	corridorID := uuid.New()
	fixture.Exec(`INSERT INTO movement_corridors (id, subject_type, subject_id, season, valid_from, valid_to, source_id, created_by)
		VALUES ($1, 'tribe', $2, 'summer', 1840, 1880, $3, $4)`, corridorID, tribeID, sourceID, owner.User.ID)

	// Rule 2: zero day-counts refuse (the legs are positive integers).
	if _, err := fixture.Pool().Exec(fixture.Ctx(), `INSERT INTO movement_corridor_stops (corridor_id, sequence_number, place_id, day_count_from_prev)
		VALUES ($1, 2, $2, 0)`, corridorID, placeID); err == nil {
		t.Fatalf("a zero day-count leg was accepted — the stage rule failed")
	}
	// Rule 3: the head carries no leg.
	if _, err := fixture.Pool().Exec(fixture.Ctx(), `INSERT INTO movement_corridor_stops (corridor_id, sequence_number, place_id, day_count_from_prev, mode_from_prev)
		VALUES ($1, 1, $2, 3, 'camel')`, corridorID, placeID); err == nil {
		t.Fatalf("the sequence's own head carried a leg — the head rule failed")
	}
	// The honest legs: the head's own stop, then a leg the sources name.
	fixture.Exec(`INSERT INTO movement_corridor_stops (corridor_id, sequence_number, place_id) VALUES ($1, 1, $2)`, corridorID, placeID)
	fixture.Exec(`INSERT INTO movement_corridor_stops (corridor_id, sequence_number, place_id, day_count_from_prev, mode_from_prev)
		VALUES ($1, 2, $2, 18, 'camel')`, corridorID, placeID)
}

func TestSubjectCorridorsReadsItsOwnSubjectBounded(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.Register(t, fixture, "قارئ الممر")
	fixture.GrantRole(owner.User.ID, "researcher")

	sourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
		VALUES ($1, 'مصدر المسالِك', 'book', 'public', $2)`, sourceID, owner.User.ID)
	tribeID := uuid.New()
	fixture.Exec(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
		VALUES ($1, 'قبيلة قراءة الممر', 'قبيلة قراءة الممر', 'public', $2)`, tribeID, owner.User.ID)

	placeIDs := make([]string, 0, 3)
	for index := 0; index < 3; index++ {
		placeID := uuid.New()
		fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility)
			VALUES ($1, $2, $2, 'city', ST_SetSRID(ST_MakePoint($3, 25), 4326), 'public')`,
			placeID, "موضع الممر "+string(rune('أ'+index)), float64(40+index))
		placeIDs = append(placeIDs, placeID.String())
	}

	for corridorIndex := 0; corridorIndex < MaxSubjectCorridors+2; corridorIndex++ {
		corridorID := uuid.New()
		fixture.Exec(`INSERT INTO movement_corridors (id, subject_type, subject_id, title_ar, season, source_id, created_by)
			VALUES ($1, 'tribe', $2, $3, 'summer', $4, $5)`,
			corridorID, tribeID, "مرحلة القراءة "+string(rune('أ'+corridorIndex)), sourceID, owner.User.ID)
		for sequence, placeID := range placeIDs {
			legDay := sequence + 3
			legMode := "camel"
			if sequence == 0 {
				legDay, legMode = 0, ""
			}
			fixture.Exec(`INSERT INTO movement_corridor_stops (corridor_id, sequence_number, place_id, day_count_from_prev, mode_from_prev)
				VALUES ($1, $2, $3,
					CASE WHEN $4 = 0 THEN NULL ELSE $4 END,
					CASE WHEN $5 = '' THEN NULL ELSE $5 END)`,
				corridorID, sequence+1, placeID, legDay, legMode)
		}
	}

	service := NewService(fixture.Pool())
	response, err := service.SubjectCorridors(fixture.Ctx(), "tribe", tribeID.String(), owner.User.ID, "", "")
	if err != nil {
		t.Fatalf("corridor read: %v", err)
	}
	if len(response.Corridors) != MaxSubjectCorridors {
		t.Fatalf("corridors = %d, want the bound (%d) with truncation stated", len(response.Corridors), MaxSubjectCorridors)
	}
	if !response.Truncated {
		t.Fatalf("the read was cut without saying so")
	}
	// The stops ride their corridor's own order, the head's legless, the legs'
	// own day counts positive and their own mode the vocabulary's words.
	first := response.Corridors[0]
	if len(first.Stops) != 3 {
		t.Fatalf("stops = %d, want the three readable places", len(first.Stops))
	}
	if first.Stops[0].DayCountFromPrev != nil || first.Stops[0].ModeFromPrev != "" {
		t.Fatalf("the head's own leg rode the read: %+v", first.Stops[0])
	}
	if first.Stops[1].DayCountFromPrev == nil || *first.Stops[1].DayCountFromPrev != 4 || first.Stops[1].ModeFromPrev != "camel" {
		t.Fatalf("the first leg lost its own day count or mode: %+v", first.Stops[1])
	}

	// The 033-C filter words narrow honestly: the season's own column, the
	// mode from ANY leg.
	seasonRead, err := service.SubjectCorridors(fixture.Ctx(), "tribe", tribeID.String(), owner.User.ID, "winter", "")
	if err != nil {
		t.Fatalf("the corridor read refused the honest season: %v", err)
	}
	if len(seasonRead.Corridors) != 0 {
		t.Fatalf("season=winter returned %d corridors over a summer-only fixture", len(seasonRead.Corridors))
	}
	modeRead, err := service.SubjectCorridors(fixture.Ctx(), "tribe", tribeID.String(), owner.User.ID, "", "horse")
	if err != nil {
		t.Fatalf("the corridor read refused the honest mode: %v", err)
	}
	if len(modeRead.Corridors) != 0 {
		t.Fatalf("mode=horse returned %d corridors over a camel-only fixture", len(modeRead.Corridors))
	}
	camelRead, err := service.SubjectCorridors(fixture.Ctx(), "tribe", tribeID.String(), owner.User.ID, "summer", "camel")
	if err != nil {
		t.Fatalf("the corridor read refused the compound: %v", err)
	}
	if len(camelRead.Corridors) == 0 {
		t.Fatalf("the compound read (summer+camel) is empty — the filters multiplied to nothing")
	}
}
