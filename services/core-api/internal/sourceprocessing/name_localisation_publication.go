package sourceprocessing

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// publishNameLocalisation replaces untouched machine geometry only after every
// page/tile has been detected. It never fabricates a human reject decision.
func (s *Service) publishNameLocalisation(ctx context.Context, work *nameReadingWork, run NameReadingRun) error {
	if run.SuggestionID != "" {
		return nil
	}
	return s.nameTransaction(ctx, work, func(tx pgx.Tx) error {
		// Match review/geometry lock order, so a concurrent human action wins safely.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(253025)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM source_files WHERE id=$1 FOR UPDATE`, run.FileID); err != nil {
			return err
		}
		var latest string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM source_name_reading_runs WHERE source_file_id=$1 AND suggestion_id IS NULL ORDER BY created_at DESC,id DESC LIMIT 1`, run.FileID).Scan(&latest); err != nil {
			return err
		}
		if latest != run.ID {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT sg.id::text,sg.page_number,sg.region,
    (sg.status<>'suggested' OR sg.review_revision>0 OR sg.geometry_revision>0 OR EXISTS(SELECT 1 FROM source_tree_suggestion_decisions d WHERE d.suggestion_id=sg.id)),
    EXISTS(SELECT 1 FROM source_name_reading_units u WHERE u.run_id=$2 AND u.stage='read' AND u.suggestion_id=sg.id),sg.localisation_pending,
    sg.reading_method LIKE 'local_%'
    FROM source_tree_suggestions sg WHERE sg.source_file_id=$1 AND sg.superseded_at IS NULL ORDER BY sg.id FOR UPDATE`, run.FileID, run.ID)
		if err != nil {
			return err
		}
		type candidate struct {
			id                                  string
			page                                int
			region                              Region
			protected, current, staged, machine bool
		}
		all := []candidate{}
		for rows.Next() {
			var item candidate
			var raw []byte
			if err := rows.Scan(&item.id, &item.page, &raw, &item.protected, &item.current, &item.staged, &item.machine); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal(raw, &item.region); err != nil {
				rows.Close()
				return err
			}
			all = append(all, item)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		retire := func(item candidate, reason string) error {
			var eventID string
			err := tx.QueryRow(ctx, `INSERT INTO source_name_review_events(suggestion_id,actor_id,event_type,revision,metadata)
     SELECT id,$2,'redetected',review_revision,$3 FROM source_tree_suggestions WHERE id=$1 RETURNING id::text`, item.id, run.RequestedBy, mustJSON(map[string]string{"runId": run.ID, "detectorVersion": run.Config.Localiser, "reason": reason})).Scan(&eventID)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE source_tree_suggestions SET superseded_at=clock_timestamp(),superseded_by_event_id=$2,localisation_pending=false,updated_at=now() WHERE id=$1`, item.id, eventID); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE source_name_reading_units SET status='succeeded',payload=jsonb_set(payload,'{outcome}','"human_geometry_preserved"'),updated_at=now() WHERE run_id=$1 AND stage='read' AND suggestion_id=$2`, run.ID, item.id)
			return err
		}
		for _, item := range all {
			if item.protected {
				continue
			}
			if item.current {
				duplicate := false
				for _, other := range all {
					if other.protected && other.page == item.page && sameNameLocalisation(item.region, other.region) {
						duplicate = true
						break
					}
				}
				if duplicate {
					if err := retire(item, "human_geometry_preserved"); err != nil {
						return err
					}
					continue
				}
				if _, err := tx.Exec(ctx, `UPDATE source_tree_suggestions SET localisation_pending=false,name_reading_run_id=$2,updated_at=now() WHERE id=$1`, item.id, run.ID); err != nil {
					return err
				}
			} else if item.machine && !item.staged {
				if err := retire(item, "replaced_machine_localisation"); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Compare overlap to the LARGER region. A rejected one-pixel branch fragment
// must not suppress a whole newly detected word that happens to contain it.
func sameNameLocalisation(a, b Region) bool {
	width := max(0, min(a.X+a.Width, b.X+b.Width)-max(a.X, b.X))
	height := max(0, min(a.Y+a.Height, b.Y+b.Height)-max(a.Y, b.Y))
	area := max(a.Width*a.Height, b.Width*b.Height)
	return area > 0 && width*height/area >= .5
}
