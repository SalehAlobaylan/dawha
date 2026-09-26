package identity

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// The vocabulary of the identity write surface. Every list is closed: an
// unrecognised value is a validation error rather than a value handed to the
// database, so a typo cannot become a row the read paths cannot classify.
var (
	// personGenders is people.gender, which is nullable in the schema; the API
	// spells the unknown case explicitly so a stored row is never ambiguous
	// between "not recorded" and "not provided".
	personGenders = []string{"male", "female", "unknown"}
	// aliasTypes is person_aliases.alias_type. A source_spelling is the only type
	// that is expected to carry a source, and it is the only type whose visibility
	// plan 001 scopes by source.
	aliasTypes = []string{"alternative_name", "kunyah", "laqab", "nisbah", "source_spelling"}
	// placeTypes is places.place_type.
	placeTypes = []string{"region", "city", "village", "historical_settlement", "other"}
	// relationshipPredicates is the predicate vocabulary the generic entity
	// relationship table accepts. It matches the set the suggestions change set
	// already writes, so a relationship recorded through either path is
	// classified the same way.
	relationshipPredicates = []string{"parent_of", "father_of", "mother_of", "spouse_of", "sibling_of", "son_of", "daughter_of", "brother_of", "sister_of", "born_in", "died_in", "resided_in"}
	// relationshipEntityTypes is the subject_type/object_type vocabulary. Each
	// value names one identity table the endpoint set writes.
	relationshipEntityTypes = []string{"person", "family", "branch", "tribe", "place"}
	// relationshipStatuses is entity_relationships.status. A new relationship is
	// always created unresolved: a create cannot assert a settled reading.
	relationshipStatuses = []string{"documented", "supported", "contested", "disputed", "inferred", "unresolved"}
)

// Length caps. They bound a single request field so one write cannot store a
// document, and they are the same order of magnitude the tree service uses for
// the same columns.
const (
	maxNameRunes     = 200
	maxAliasRunes    = 300
	maxNotesRunes    = 5000
	maxReasonRunes   = 1000
	maxSearchRunes   = 200
	defaultListLimit = 100
)

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

// requiredName trims an Arabic display name, keeps the caller's original text
// and rejects an empty or oversized one. The normalized form is derived by the
// caller from the trimmed value, so the display value is never destroyed.
func requiredName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len([]rune(trimmed)) > maxNameRunes {
		return "", ErrValidation
	}
	return trimmed, nil
}

func optionalText(value string, maxRunes int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) > maxRunes {
		return "", ErrValidation
	}
	return trimmed, nil
}

func optionalEnum(value, fallback string, allowed []string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = fallback
	}
	if !contains(allowed, trimmed) {
		return "", ErrValidation
	}
	return trimmed, nil
}

func requiredEnum(value string, allowed []string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if !contains(allowed, trimmed) {
		return "", ErrValidation
	}
	return trimmed, nil
}

// parseActor resolves the acting user. An absent or malformed actor is a
// permission failure rather than a validation failure, so an unauthenticated
// caller cannot tell a well formed identifier from a malformed one.
func parseActor(actorID string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(actorID)
	if trimmed == "" {
		return uuid.Nil, ErrForbidden
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return uuid.Nil, ErrForbidden
	}
	return parsed, nil
}

// parseID resolves a row identifier from a path segment. A malformed
// identifier answers exactly like a missing row, so the endpoint cannot be used
// as an oracle for which identifiers are well formed.
func parseID(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	return parsed, nil
}

// optionalID resolves an optional reference. An empty value is no reference; a
// present but malformed value is a validation error, never a silent NULL.
func optionalID(value string) (any, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return nil, ErrValidation
	}
	return parsed, nil
}

func requiredID(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, ErrValidation
	}
	return parsed, nil
}

// dateRange is an optional pair of approximate dates. A missing bound is an
// open bound, which is why the columns are nullable: the product never assumes
// a date is exact, and it never invents one either.
type dateRange struct {
	From pgtype.Date
	To   pgtype.Date
}

// parseDateRange validates a pair of optional "2006-01-02" bounds and rejects a
// range that ends before it starts.
func parseDateRange(from, to string) (dateRange, error) {
	parsedFrom, err := parseOptionalDate(from)
	if err != nil {
		return dateRange{}, err
	}
	parsedTo, err := parseOptionalDate(to)
	if err != nil {
		return dateRange{}, err
	}
	if parsedFrom.Valid && parsedTo.Valid && parsedTo.Time.Before(parsedFrom.Time) {
		return dateRange{}, ErrValidation
	}
	return dateRange{From: parsedFrom, To: parsedTo}, nil
}

func parseOptionalDate(value string) (pgtype.Date, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.Date{}, nil
	}
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return pgtype.Date{}, ErrValidation
	}
	return pgtype.Date{Time: parsed, Valid: true}, nil
}

// validatePoint checks an optional WGS84 point. Both coordinates must be present
// together, and each must be inside its range: an out-of-range point is a
// validation error rather than a row PostGIS would reject with a driver error.
func validatePoint(longitude, latitude *float64) error {
	if longitude == nil && latitude == nil {
		return nil
	}
	if longitude == nil || latitude == nil {
		return ErrValidation
	}
	if *longitude < -180 || *longitude > 180 {
		return ErrValidation
	}
	if *latitude < -90 || *latitude > 90 {
		return ErrValidation
	}
	return nil
}

func dateValue(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func floatValue(value pgtype.Float8) (float64, bool) {
	if !value.Valid {
		return 0, false
	}
	return value.Float64, true
}
