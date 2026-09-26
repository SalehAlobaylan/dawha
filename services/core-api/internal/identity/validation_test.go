package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The validators are the only place a caller string becomes a stored value, so
// they are tested as a table rather than through the database: a rejection here is
// a 400 with a message, and a rejection that only appears against PostgreSQL is a
// rejection a caller cannot act on.
func TestValidatePersonInput(t *testing.T) {
	long := strings.Repeat("ش", maxNameRunes+1)
	cases := []struct {
		name    string
		gender  string
		birth   string
		birthTo string
		death   string
		deathTo string
		notes   string
		want    string
		got     string
		err     error
	}{
		{name: "minimal person", gender: "", want: "علي بن محمد", got: "unknown"},
		{name: "explicit gender", gender: "female", want: "علي بن محمد", got: "female"},
		{name: "approximate dates keep both bounds", gender: "", birth: "0600-01-01", birthTo: "0650-12-31", want: "علي بن محمد", got: "unknown"},
		{name: "an open date bound is left empty", gender: "", death: "0700-01-01", want: "علي بن محمد", got: "unknown"},
		{name: "gender outside the vocabulary", gender: "male-ish", err: ErrValidation},
		{name: "reversed birth range", gender: "", birth: "0650-01-01", birthTo: "0600-01-01", err: ErrValidation},
		{name: "reversed death range", gender: "", death: "0750-01-01", deathTo: "0700-01-01", err: ErrValidation},
		{name: "unparseable date", gender: "", birth: "0600", err: ErrValidation},
		{name: "blank name", gender: "", want: "   ", err: ErrValidation},
		{name: "oversized name", gender: "", want: long, err: ErrValidation},
		{name: "oversized notes", gender: "", notes: strings.Repeat("ب", maxNotesRunes+1), err: ErrValidation},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fields, err := validatePersonInput(testCase.want, testCase.gender, testCase.birth, testCase.birthTo, testCase.death, testCase.deathTo, testCase.notes)
			if testCase.err != nil {
				if !errors.Is(err, testCase.err) {
					t.Fatalf("error = %v, want %v", err, testCase.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fields.name != strings.TrimSpace(testCase.want) {
				t.Fatalf("name = %q, want the trimmed original %q", fields.name, strings.TrimSpace(testCase.want))
			}
			if fields.gender != testCase.got {
				t.Fatalf("gender = %q, want %q", fields.gender, testCase.got)
			}
		})
	}
}

// The Arabic normalizer is the index key for every name this package writes, so a
// caller-supplied variant has to land on the stored normalized form or the row
// becomes unfindable by the very spelling the dictionary lists.
func TestNormalizedNameIsDerivedFromTheTrimmedOriginal(t *testing.T) {
	fields, err := validatePersonInput("  أَبو بَكر ", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := NormalizeArabicName(fields.name); got != "ابو بكر" {
		t.Fatalf("normalized = %q, want %q", got, "ابو بكر")
	}
	if fields.name != "أَبو بَكر" {
		t.Fatalf("display name = %q, want the original with its diacritics", fields.name)
	}
}

func TestValidateAliasInput(t *testing.T) {
	sourceID := uuid.NewString()
	cases := []struct {
		name      string
		input     PersonAliasInput
		wantValue string
		wantType  string
		hasSource bool
		err       error
	}{
		{name: "kunyah", input: PersonAliasInput{ValueAR: "أبو بكر", AliasType: "kunyah"}, wantValue: "أبو بكر", wantType: "kunyah"},
		{name: "type defaults to an alternative name", input: PersonAliasInput{ValueAR: "عمر"}, wantValue: "عمر", wantType: "alternative_name"},
		{name: "source spelling carries its source", input: PersonAliasInput{ValueAR: "ابن الخطاب", AliasType: "source_spelling", SourceID: sourceID}, wantValue: "ابن الخطاب", wantType: "source_spelling", hasSource: true},
		{name: "unknown type", input: PersonAliasInput{ValueAR: "عمر", AliasType: "nickname"}, err: ErrValidation},
		{name: "malformed source", input: PersonAliasInput{ValueAR: "عمر", SourceID: "not-a-uuid"}, err: ErrValidation},
		{name: "blank value", input: PersonAliasInput{ValueAR: " "}, err: ErrValidation},
		{name: "oversized value", input: PersonAliasInput{ValueAR: strings.Repeat("ع", maxAliasRunes+1)}, err: ErrValidation},
		{name: "oversized reason", input: PersonAliasInput{ValueAR: "عمر", ReasonAR: strings.Repeat("س", maxReasonRunes+1)}, err: ErrValidation},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value, aliasType, source, _, err := validateAliasInput(testCase.input)
			if testCase.err != nil {
				if !errors.Is(err, testCase.err) {
					t.Fatalf("error = %v, want %v", err, testCase.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != testCase.wantValue || aliasType != testCase.wantType {
				t.Fatalf("got (%q, %q), want (%q, %q)", value, aliasType, testCase.wantValue, testCase.wantType)
			}
			if (source != nil) != testCase.hasSource {
				t.Fatalf("source = %v, want present = %t", source, testCase.hasSource)
			}
		})
	}
}

func TestValidatePlaceInput(t *testing.T) {
	// A coordinate pair has to travel as one value: a struct keeps "both present"
	// and "both absent" as the only two states a table row can express, which is
	// exactly what places.geometry allows.
	type coords struct {
		longitude *float64
		latitude  *float64
	}
	point := func(longitude, latitude float64) coords {
		return coords{longitude: &longitude, latitude: &latitude}
	}
	single := func(value float64) *float64 { return &value }
	parentID := uuid.NewString()
	cases := []struct {
		name      string
		placeType string
		parent    string
		at        coords
		err       error
	}{
		{name: "region without a point", placeType: "region"},
		{name: "city with a point", placeType: "city", at: point(39.2, 21.5)},
		{name: "nested place", placeType: "village", parent: parentID, at: point(39.2, 21.5)},
		{name: "place type outside the vocabulary", placeType: "hamlet", err: ErrValidation},
		{name: "missing place type", placeType: "", err: ErrValidation},
		{name: "malformed parent", placeType: "region", parent: "not-a-uuid", err: ErrValidation},
		{name: "longitude without a latitude", placeType: "region", at: coords{longitude: single(39.2)}, err: ErrValidation},
		{name: "latitude without a longitude", placeType: "region", at: coords{latitude: single(21.5)}, err: ErrValidation},
		{name: "longitude past the antimeridian", placeType: "region", at: point(181, 0), err: ErrValidation},
		{name: "latitude past the pole", placeType: "region", at: point(0, 91), err: ErrValidation},
		{name: "the antimeridian itself is inside the range", placeType: "region", at: point(180, -90)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, resolvedType, _, _, longitude, latitude, err := validatePlaceInput("موضع", testCase.placeType, testCase.parent, "", testCase.at.longitude, testCase.at.latitude)
			if testCase.err != nil {
				if !errors.Is(err, testCase.err) {
					t.Fatalf("error = %v, want %v", err, testCase.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolvedType != testCase.placeType {
				t.Fatalf("place type = %q, want %q", resolvedType, testCase.placeType)
			}
			// The caller's own pointers are passed through, so an absent point
			// stays absent instead of becoming the origin.
			if (longitude == nil) != (testCase.at.longitude == nil) || (latitude == nil) != (testCase.at.latitude == nil) {
				t.Fatalf("coordinates = (%v, %v), want the input's presence", longitude, latitude)
			}
		})
	}
}

func TestValidateRelationshipInput(t *testing.T) {
	subjectID := uuid.NewString()
	objectID := uuid.NewString()
	cases := []struct {
		name        string
		subjectType string
		subjectID   string
		predicate   string
		objectType  string
		objectID    string
		validFrom   string
		validTo     string
		err         error
	}{
		{name: "person to person", subjectType: "person", subjectID: subjectID, predicate: "father_of", objectType: "person", objectID: objectID},
		{name: "endpoint types default to person", subjectType: "", subjectID: subjectID, predicate: "resided_in", objectType: "", objectID: objectID},
		{name: "place endpoint", subjectType: "person", subjectID: subjectID, predicate: "born_in", objectType: "place", objectID: objectID, validFrom: "0700-01-01"},
		{name: "endpoint type outside the vocabulary", subjectType: "source", subjectID: subjectID, predicate: "born_in", objectType: "place", objectID: objectID, err: ErrValidation},
		{name: "predicate outside the vocabulary", subjectType: "person", subjectID: subjectID, predicate: "mentor_of", objectType: "person", objectID: objectID, err: ErrValidation},
		{name: "an entity cannot be related to itself", subjectType: "person", subjectID: subjectID, predicate: "father_of", objectType: "person", objectID: subjectID, err: ErrValidation},
		{name: "malformed subject", subjectType: "person", subjectID: "nope", predicate: "father_of", objectType: "person", objectID: objectID, err: ErrValidation},
		{name: "reversed validity range", subjectType: "person", subjectID: subjectID, predicate: "resided_in", objectType: "place", objectID: objectID, validFrom: "0750-01-01", validTo: "0700-01-01", err: ErrValidation},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := validateRelationshipInput(testCase.subjectType, testCase.subjectID, testCase.predicate, testCase.objectType, testCase.objectID, testCase.validFrom, testCase.validTo)
			if testCase.err != nil {
				if !errors.Is(err, testCase.err) {
					t.Fatalf("error = %v, want %v", err, testCase.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !contains(relationshipEntityTypes, plan.subjectType) || !contains(relationshipPredicates, plan.predicate) {
				t.Fatalf("plan escaped the closed vocabulary: %+v", plan)
			}
		})
	}
}

func TestEntityTableIsClosed(t *testing.T) {
	for _, entityType := range relationshipEntityTypes {
		if _, ok := entityTable(entityType); !ok {
			t.Fatalf("entity type %q is in the request vocabulary but has no table", entityType)
		}
	}
	for _, entityType := range []string{"source", "claim", "question", "tree", "user", "people"} {
		if _, ok := entityTable(entityType); ok {
			t.Fatalf("entity type %q resolved to a table but is not an identity family", entityType)
		}
	}
}

func TestParseActorAndID(t *testing.T) {
	if _, err := parseActor(""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an empty actor = %v, want ErrForbidden", err)
	}
	if _, err := parseActor("not-a-uuid"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a malformed actor = %v, want ErrForbidden", err)
	}
	if _, err := parseID("not-a-uuid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a malformed row id = %v, want ErrNotFound so it cannot be told from a missing row", err)
	}
	if _, err := optionalID("not-a-uuid"); !errors.Is(err, ErrValidation) {
		t.Fatalf("a malformed optional reference = %v, want ErrValidation rather than a silent NULL", err)
	}
	// An explicit nil is what a JSON null becomes, and it must stay a real NULL
	// rather than the nil uuid, which is a value no row can reference.
	reference, err := optionalID("  ")
	if err != nil || reference != nil {
		t.Fatalf("an absent reference = (%v, %v), want (nil, nil)", reference, err)
	}
}
