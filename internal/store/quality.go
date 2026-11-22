package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Raw data of the quality metrics (internal/analytics quality.go):
// requirement revisions, execution results with durations and review
// requests of a project. Defects, releases and the items themselves come
// from the ordinary list functions.

// QualityRevision is a revision of a requirement. Title and Description
// are read from the snapshot (only when the caller asked for texts).
type QualityRevision struct {
	RequirementID      string
	Version            int
	Action             string
	ChangedFields      []string
	Title, Description string
	Status             string
	At                 time.Time
}

// QualityExecution is an execution with a result.
type QualityExecution struct {
	ID, TestID, RunID, Status string
	// At is the time of the result (finished_at, else started_at, else
	// the run's creation, as in LastResults).
	At time.Time
	// DurationMS as recorded, else finished_at − started_at (0 unknown).
	DurationMS int64
}

// QualityReview is a review request.
type QualityReview struct {
	EntityType, Status string
	CreatedAt          time.Time
	DecidedAt          *time.Time
}

// QualityRevisions returns the revisions of the project's requirements in
// time order. With textsFrom set, the texts are read for the revisions
// from that time on and for the latest revision of each requirement
// before it (the text at the start of the range).
func (s *Store) QualityRevisions(ctx context.Context, tenantID, projectID string, textsFrom *time.Time) ([]QualityRevision, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT v.id, v.entity_id, v.version, v.action, v.changed_fields, v.created_at FROM revisions v
		JOIN requirements q ON q.id = v.entity_id
		WHERE v.entity_type = 'requirement' AND v.tenant_id = ? AND q.project_id = ? ORDER BY v.created_at, v.version`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	var out []QualityRevision
	var ids []string
	for rows.Next() {
		var rv QualityRevision
		var id, cf string
		if err := rows.Scan(&id, &rv.RequirementID, &rv.Version, &rv.Action, &cf, &rv.At); err != nil {
			rows.Close()
			return nil, err
		}
		rv.At = rv.At.UTC()
		_ = json.Unmarshal([]byte(cf), &rv.ChangedFields)
		out = append(out, rv)
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil || textsFrom == nil {
		return out, err
	}
	// which revisions need their text: those in the range and the last
	// one of each requirement before it
	need := map[int]bool{}
	lastBefore := map[string]int{}
	for i, rv := range out {
		if rv.At.Before(*textsFrom) {
			lastBefore[rv.RequirementID] = i
		} else {
			need[i] = true
		}
	}
	for _, i := range lastBefore {
		need[i] = true
	}
	var want []string
	at := map[string]int{}
	for i := range need {
		want = append(want, ids[i])
		at[ids[i]] = i
	}
	for len(want) > 0 {
		n := min(len(want), 500)
		batch := want[:n]
		want = want[n:]
		rows, err := s.c(ctx).query(ctx, `SELECT id, snapshot FROM revisions WHERE id IN (`+placeholders(len(batch))+`)`, anySlice(batch)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, snap string
			if err := rows.Scan(&id, &snap); err != nil {
				rows.Close()
				return nil, err
			}
			var v struct{ Title, Description, Status string }
			_ = json.Unmarshal([]byte(snap), &v)
			i := at[id]
			out[i].Title, out[i].Description, out[i].Status = v.Title, v.Description, v.Status
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// QualityExecutions returns the executions of the project that have a
// result (not not_run).
func (s *Store) QualityExecutions(ctx context.Context, tenantID, projectID string) ([]QualityExecution, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT e.id, e.test_case_id, e.run_id, e.status, e.started_at, e.finished_at, e.duration_ms, r.created_at
		FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE r.project_id = ? AND e.tenant_id = ? AND e.status <> 'not_run' ORDER BY e.id`, projectID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QualityExecution
	for rows.Next() {
		var x QualityExecution
		var start, fin sql.NullTime
		var created time.Time
		if err := rows.Scan(&x.ID, &x.TestID, &x.RunID, &x.Status, &start, &fin, &x.DurationMS, &created); err != nil {
			return nil, err
		}
		switch {
		case fin.Valid:
			x.At = fin.Time
		case start.Valid:
			x.At = start.Time
		default:
			x.At = created
		}
		x.At = x.At.UTC()
		if x.DurationMS <= 0 && start.Valid && fin.Valid && fin.Time.After(start.Time) {
			x.DurationMS = fin.Time.Sub(start.Time).Milliseconds()
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// QualityReviews returns the review requests of a project.
func (s *Store) QualityReviews(ctx context.Context, tenantID, projectID string) ([]QualityReview, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT entity_type, status, created_at, decided_at FROM review_requests WHERE tenant_id = ? AND project_id = ? ORDER BY created_at, id`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QualityReview
	for rows.Next() {
		var v QualityReview
		var dec sql.NullTime
		if err := rows.Scan(&v.EntityType, &v.Status, &v.CreatedAt, &dec); err != nil {
			return nil, err
		}
		v.CreatedAt = v.CreatedAt.UTC()
		if dec.Valid {
			t := dec.Time.UTC()
			v.DecidedAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
