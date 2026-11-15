// Package datahygiene answers one moderator question: which sources does the
// archive lean on, and which of them arrived without a recorded license.
//
// The archive's rule — inherited from plans 001, 002 and 007 and extended for
// external data — is that visibility and provenance never weaken: a fact drawn
// from an external source points back at that source, so a source that hides
// its license hides the terms of everything downstream of it. This report does
// not fix any row; it names the rows, with the counts of what depends on each
// one, so a moderator can see where the debt is heaviest.
//
// The gate follows the workspace precedent (research.workspaceHasModeratorRole):
// the role check is a single SELECT inside the service, because the archive has
// no admin middleware to call and inventing a second pattern would split the
// truth about who may read a moderator surface across two places.
package datahygiene

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("data hygiene database is unavailable")
	// ErrForbidden is the answer to a caller without the moderator role set,
	// anonymous included. The report is deliberately silent about what it
	// contains; 403 carries no rows.
	ErrForbidden = errors.New("data hygiene report is readable by moderators only")
)

// ReportLimit is how many source rows one report carries. The fetch asks for
// one row more than this, so the response can say it was cut instead of
// shipping an unbounded read of a table that grows with the dataset.
const ReportLimit = 100

// SourceLicense is one source's hygiene row: its license posture and the
// counts of archive artifacts that lean on it.
type SourceLicense struct {
	ID               string  `json:"id"`
	TitleAR          string  `json:"title_ar"`
	AuthorAR         *string `json:"author_ar"`
	SourceType       string  `json:"source_type"`
	DependencyStatus string  `json:"dependency_status"`
	HasLicense       bool    `json:"has_license"`
	License          *string `json:"license"`
	LicenseNote      *string `json:"license_note"`
	// The dependent counts are what turn a list of names into a list of debts:
	// a source without a license that carries passages and mapping associations
	// is a heavier problem than an empty row.
	FileCount        int64 `json:"file_count"`
	PassageCount     int64 `json:"passage_count"`
	AssociationCount int64 `json:"association_count"`
}

// Report is the moderator dashboard payload. The totals are computed over the
// whole sources table, never over the row window, so a truncated report still
// tells the truth at the top line.
type Report struct {
	TotalSources        int64           `json:"total_sources"`
	MissingLicenseCount int64           `json:"missing_license_count"`
	Sources             []SourceLicense `json:"sources"`
	Limit               int             `json:"limit"`
	Truncated           bool            `json:"truncated"`
}

// Service reads hygiene facts. It writes nothing: fixing a license is the
// source editor's job, not the dashboard's.
type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

// Report returns the license posture of every source whose row fits the
// bounded window. A caller without the moderator or admin role is refused;
// a caller with the session of a user that does not exist is refused the same
// way, because "no such user" is not information this surface needs to give.
func (s *Service) Report(ctx context.Context, actorID string) (Report, error) {
	if s.Pool == nil {
		return Report{}, ErrDatabaseUnavailable
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil || actorUUID == uuid.Nil {
		return Report{}, ErrForbidden
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return Report{}, err
	}
	if !allowed {
		return Report{}, ErrForbidden
	}
	return s.report(ctx)
}

func (s *Service) report(ctx context.Context) (Report, error) {
	report := Report{Limit: ReportLimit, Sources: []SourceLicense{}}
	err := s.Pool.QueryRow(ctx, `
		SELECT count(*)::bigint, count(*) FILTER (WHERE COALESCE(metadata->>'license', '') = '')::bigint
		FROM sources`).Scan(&report.TotalSources, &report.MissingLicenseCount)
	if err != nil {
		return Report{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id, s.title_ar, s.author_ar, s.source_type, s.dependency_status,
		       COALESCE(s.metadata->>'license', '') <> '' AS has_license,
		       s.metadata->>'license', s.metadata->>'license_note',
		       (SELECT count(*) FROM source_files f WHERE f.source_id = s.id)::bigint,
		       (SELECT count(*) FROM source_passages p WHERE p.source_id = s.id)::bigint,
		       (SELECT count(*) FROM geographic_associations ga WHERE ga.source_id = s.id)::bigint
		FROM sources s
		ORDER BY s.created_at, s.id
		LIMIT $1`, ReportLimit+1)
	if err != nil {
		return Report{}, err
	}
	defer rows.Close()
	for rows.Next() {
		row := SourceLicense{}
		if err := rows.Scan(&row.ID, &row.TitleAR, &row.AuthorAR, &row.SourceType, &row.DependencyStatus,
			&row.HasLicense, &row.License, &row.LicenseNote,
			&row.FileCount, &row.PassageCount, &row.AssociationCount); err != nil {
			return Report{}, err
		}
		report.Sources = append(report.Sources, row)
	}
	if err := rows.Err(); err != nil {
		return Report{}, err
	}
	// The extra row that told us the read was cut is withheld: a caller may see
	// that the window ended, not the row that did not fit.
	if len(report.Sources) > ReportLimit {
		report.Sources = report.Sources[:ReportLimit]
		report.Truncated = true
	}
	return report, nil
}
