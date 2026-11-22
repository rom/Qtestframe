package store

import (
	"context"
	"regexp"
	"strings"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// Statement of Applicability decisions: per project, reference source and
// control, whether it applies, the justification and the owner.

var soaIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 .§()/_:-]{0,79}$`)

// ValidSoAControlID reports whether id can name a control.
func ValidSoAControlID(id string) bool { return soaIDRe.MatchString(id) }

func applicableText(b *bool) string {
	switch {
	case b == nil:
		return ""
	case *b:
		return "yes"
	}
	return "no"
}

func applicableOf(s string) *bool {
	switch s {
	case "yes":
		t := true
		return &t
	case "no":
		f := false
		return &f
	}
	return nil
}

// SoADecisions returns a project's decisions for a source by control id.
func (s *Store) SoADecisions(ctx context.Context, tenantID, projectID, sourceID string) (map[string]model.SoADecision, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT control_id, applicable, justification, owner, updated_by, updated_at FROM soa_decisions WHERE tenant_id = ? AND project_id = ? AND source_id = ?`,
		tenantID, projectID, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.SoADecision{}
	for rows.Next() {
		var d model.SoADecision
		var app string
		if err := rows.Scan(&d.ControlID, &app, &d.Justification, &d.Owner, &d.UpdatedBy, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Applicable = applicableOf(app)
		out[d.ControlID] = d
	}
	return out, rows.Err()
}

// SetSoADecisions saves decisions of a project for a source (a decision
// with nothing decided, justified or owned is removed). It returns the
// number of controls changed.
func (s *Store) SetSoADecisions(ctx context.Context, tenantID, projectID, sourceID, by string, list []model.SoADecision) (int, error) {
	if _, err := s.GetProject(ctx, tenantID, projectID); err != nil {
		return 0, err
	}
	if len(list) > 2000 {
		return 0, Invalidf("at most 2000 decisions at a time")
	}
	for i := range list {
		d := &list[i]
		d.ControlID = strings.TrimSpace(d.ControlID)
		d.Justification, d.Owner = strings.TrimSpace(d.Justification), strings.TrimSpace(d.Owner)
		if !ValidSoAControlID(d.ControlID) {
			return 0, Invalidf("invalid control id %q", d.ControlID)
		}
		if len([]rune(d.Justification)) > 4000 || len([]rune(d.Owner)) > 200 {
			return 0, Invalidf("%s: the justification is at most 4000 and the owner 200 characters", d.ControlID)
		}
	}
	now := s.now()
	changed := 0
	err := s.tx(ctx, func(c conn) error {
		for _, d := range list {
			var app, just, owner string
			err := c.row(ctx, `SELECT applicable, justification, owner FROM soa_decisions WHERE project_id = ? AND source_id = ? AND control_id = ?`, projectID, sourceID, d.ControlID).Scan(&app, &just, &owner)
			exists := err == nil
			if err != nil && !db.IsNoRows(err) {
				return err
			}
			want := applicableText(d.Applicable)
			if exists && app == want && just == d.Justification && owner == d.Owner {
				continue
			}
			changed++
			switch {
			case want == "" && d.Justification == "" && d.Owner == "":
				if _, err := c.exec(ctx, `DELETE FROM soa_decisions WHERE project_id = ? AND source_id = ? AND control_id = ?`, projectID, sourceID, d.ControlID); err != nil {
					return err
				}
				if !exists {
					changed--
				}
			case exists:
				if _, err := c.exec(ctx, `UPDATE soa_decisions SET applicable = ?, justification = ?, owner = ?, updated_by = ?, updated_at = ? WHERE project_id = ? AND source_id = ? AND control_id = ?`,
					want, d.Justification, d.Owner, by, now, projectID, sourceID, d.ControlID); err != nil {
					return err
				}
			default:
				if _, err := c.exec(ctx, `INSERT INTO soa_decisions (id, tenant_id, project_id, source_id, control_id, applicable, justification, owner, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					newID(), tenantID, projectID, sourceID, d.ControlID, want, d.Justification, d.Owner, by, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return changed, err
}
