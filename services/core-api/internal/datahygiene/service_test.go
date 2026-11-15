package datahygiene

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/google/uuid"
)

// The report is the first moderator surface that names which sources the
// archive leans on, so these tests pin what a moderator reads and who is
// refused. The fixture grows past one source deliberately: "one row" and
// "the rows the caller may see" agree on a single row's response and the test
// would prove nothing about the totals-over-window rule.
func seedHygiene(t *testing.T, fixture *testsupport.Fixture) {
	t.Helper()
	moderator := actor.Register(t, fixture, "مدير النظافة")
	fixture.GrantRole(moderator.User.ID, "moderator")

	documentedID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, metadata, created_by)
		VALUES ($1, 'مصدر مرخّص النظافة', 'book', $2, $3)`,
		documentedID, `{"synthetic":true,"license":"CC-BY-4.0","license_note":"بيانات تجريبية بترخيص موثق"}`, moderator.User.ID)
	missingID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, metadata)
		VALUES ($1, 'مصدر بلا رخصة النظافة', 'book', '{"synthetic":true}')`, missingID)

	// The documented source leans on the archive in three registered ways, so
	// the dependent counts answer with arithmetic, not with zeros.
	fixture.Exec(`INSERT INTO source_files (source_id, storage_key) VALUES ($1, 'hygiene/one.pdf')`, documentedID)
	fixture.Exec(`INSERT INTO source_passages (source_id, sequence_number, text_ar, normalized_text_ar)
		VALUES ($1, 1, 'نص النظافة التجريبي', 'نص النظافة التجريبي')`, documentedID)
	placeID := uuid.New()
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility)
		VALUES ($1, 'موضع النظافة', 'موضع النظافة', 'city', ST_SetSRID(ST_MakePoint(46.7, 24.6), 4326), 'public')`, placeID)
	fixture.Exec(`INSERT INTO geographic_associations (entity_type, entity_id, place_id, relation_type, status, source_id)
		VALUES ('person', $1, $2, 'lived_in', 'documented', $3)`, uuid.New(), placeID, documentedID)
}

func serviceOf(t *testing.T, fixture *testsupport.Fixture) *Service {
	t.Helper()
	return NewService(fixture.Pool())
}

func moderatorID(t *testing.T, fixture *testsupport.Fixture) string {
	t.Helper()
	var id string
	if err := fixture.QueryRow(`SELECT user_id FROM user_roles WHERE role = 'moderator' LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("seeded moderator role not found: %v", err)
	}
	return id
}

func TestReportTotalsAndDependentCounts(t *testing.T) {
	fixture := testsupport.New(t)
	seedHygiene(t, fixture)

	// The fixture schema carries the demo seed's sources too, so the totals'
	// comparator is a hand-written count over the same table rather than an
	// assumption that only this test's two sources exist.
	var derivedTotal, derivedMissing int64
	if err := fixture.QueryRow(`SELECT count(*)::bigint, count(*) FILTER (WHERE COALESCE(metadata->>'license', '') = '')::bigint FROM sources`).Scan(&derivedTotal, &derivedMissing); err != nil {
		t.Fatalf("hand-derive the totals: %v", err)
	}

	report, err := serviceOf(t, fixture).Report(fixture.Ctx(), moderatorID(t, fixture))
	if err != nil {
		t.Fatalf("moderator report: %v", err)
	}

	if report.TotalSources != derivedTotal {
		t.Fatalf("total_sources = %d, want the derived %d", report.TotalSources, derivedTotal)
	}
	if report.MissingLicenseCount != derivedMissing {
		t.Fatalf("missing_license_count = %d, want the derived %d", report.MissingLicenseCount, derivedMissing)
	}
	if report.Truncated {
		t.Fatalf("a two-source-plus-seed read must not claim truncation")
	}
	if len(report.Sources) == 0 {
		t.Fatalf("no sources in the window")
	}
	var licensedFound, licenselessFound bool
	for _, row := range report.Sources {
		if row.TitleAR == "مصدر مرخّص النظافة" {
			licensedFound = true
			if !row.HasLicense {
				t.Fatalf("documented source answered without a license")
			}
			if row.License == nil || *row.License != "CC-BY-4.0" {
				t.Fatalf("documented source license = %v, want CC-BY-4.0", row.License)
			}
			if row.FileCount != 1 || row.PassageCount != 1 || row.AssociationCount != 1 {
				t.Fatalf("dependent counts = (%d, %d, %d), want (1, 1, 1)",
					row.FileCount, row.PassageCount, row.AssociationCount)
			}
		}
		if row.TitleAR == "مصدر بلا رخصة النظافة" {
			licenselessFound = true
			if row.HasLicense || row.License != nil {
				t.Fatalf("licenseless source answered with a license value: %v", row.License)
			}
			if row.FileCount != 0 || row.PassageCount != 0 || row.AssociationCount != 0 {
				t.Fatalf("licenseless dependent counts = (%d, %d, %d), want zeros",
					row.FileCount, row.PassageCount, row.AssociationCount)
			}
		}
	}
	if !licensedFound || !licenselessFound {
		t.Fatalf("both seeded rows missing from the window (licensed=%v, licenseless=%v)", licensedFound, licenselessFound)
	}
}

func TestReportRefusesCallersWithoutModeratorRole(t *testing.T) {
	fixture := testsupport.New(t)
	seedHygiene(t, fixture)
	service := serviceOf(t, fixture)

	reader := actor.Register(t, fixture, "قارئ النظافة")
	if _, err := service.Report(fixture.Ctx(), reader.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("plain reader error = %v, want ErrForbidden", err)
	}
	// An anonymous caller passes no actor at all, which parses to nothing
	// rather than to some user.
	if _, err := service.Report(fixture.Ctx(), ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("anonymous error = %v, want ErrForbidden", err)
	}
	// A moderator role grant is what unblocks, not some other surface's role.
	stranger := actor.Register(t, fixture, "باحث النظافة")
	fixture.GrantRole(stranger.User.ID, "researcher")
	if _, err := service.Report(fixture.Ctx(), stranger.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("researcher error = %v, want ErrForbidden", err)
	}
}

func TestReportWindowStaysBoundedAndSaysWhenCut(t *testing.T) {
	fixture := testsupport.New(t)
	moderator := actor.Register(t, fixture, "مدير حدود النظافة")
	fixture.GrantRole(moderator.User.ID, "moderator")
	// Push the table past the report limit: the window has to stay at its bound
	// whatever the table has grown to.
	for index := 0; index < ReportLimit+3; index++ {
		fixture.Exec(`INSERT INTO sources (title_ar, source_type, metadata)
			VALUES ($1, 'book', '{}')`, uuid.New().String())
	}
	// The demo seed's sources share this fixture schema, so the "whole table"
	// expectation is derived from it, not from a literal: what the test pins is
	// that the window stayed bounded while the totals covered everything.
	var derivedTotal, derivedMissing int64
	if err := fixture.QueryRow(`SELECT count(*)::bigint, count(*) FILTER (WHERE COALESCE(metadata->>'license', '') = '')::bigint FROM sources`).Scan(&derivedTotal, &derivedMissing); err != nil {
		t.Fatalf("hand-derive the totals: %v", err)
	}

	report, err := serviceOf(t, fixture).Report(fixture.Ctx(), moderator.User.ID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if report.TotalSources != derivedTotal || report.MissingLicenseCount != derivedMissing {
		t.Fatalf("totals = (%d, %d), want the derived (%d, %d)",
			report.TotalSources, report.MissingLicenseCount, derivedTotal, derivedMissing)
	}
	if report.TotalSources < ReportLimit+1 {
		t.Fatalf("fixture pushed %d sources; the bound's cut must be reachable", report.TotalSources)
	}
	if len(report.Sources) != ReportLimit {
		t.Fatalf("window = %d rows, want the bounded %d", len(report.Sources), ReportLimit)
	}
	if !report.Truncated {
		t.Fatalf("a %d-row table must answer truncated with a %d-row window", derivedTotal, ReportLimit)
	}
	if report.Limit != ReportLimit {
		t.Fatalf("limit = %d, want %d", report.Limit, ReportLimit)
	}
}
