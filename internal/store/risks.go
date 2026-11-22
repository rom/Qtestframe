package store

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// Risks: likelihood × impact, status, mitigation and the requirements that
// address them.

const riskCols = `id, project_id, key, title, description, category, likelihood, impact, status, mitigation, owner, created_by, created_at, updated_at,
	residual_likelihood, residual_impact, residual_manual, residual_justification, review_date, review_owner,
	accepted_by, accepted_name, accepted_at, acceptance_comment, acceptance_hash`

func scanRisk(r interface{ Scan(...any) error }) (*model.Risk, error) {
	var k model.Risk
	var manual int
	var acc sql.NullTime
	if err := r.Scan(&k.ID, &k.ProjectID, &k.Key, &k.Title, &k.Description, &k.Category, &k.Likelihood, &k.Impact, &k.Status, &k.Mitigation, &k.Owner, &k.CreatedBy, &k.CreatedAt, &k.UpdatedAt,
		&k.ResidualLikelihood, &k.ResidualImpact, &manual, &k.ResidualJustification, &k.ReviewDate, &k.ReviewOwner,
		&k.AcceptedBy, &k.AcceptedName, &acc, &k.AcceptanceComment, &k.AcceptanceHash); err != nil {
		return nil, err
	}
	k.ResidualManual = manual != 0
	k.AcceptedAt = timePtr(acc)
	k.Score = k.Likelihood * k.Impact
	k.Level = model.RiskLevel(k.Likelihood, k.Impact)
	// until decorated: the inherent score
	k.ResidualScore, k.ResidualLevel, k.ResidualSource = k.Score, k.Level, model.ResidualInherent
	return &k, nil
}

// RiskOpen reports whether a risk still needs action.
func RiskOpen(status string) bool { return slices.Contains(model.RiskOpenStatuses, status) }

// RiskLevelAtLeast reports whether level is at least min ("" = any).
func RiskLevelAtLeast(level, min string) bool {
	return min == "" || slices.Index(model.RiskLevels, level) >= slices.Index(model.RiskLevels, min)
}

func validateRisk(k *model.Risk) error {
	k.Title = strings.TrimSpace(k.Title)
	if k.Title == "" || len([]rune(k.Title)) > 300 {
		return Invalidf("risk title is required (at most 300 characters)")
	}
	if k.Likelihood == 0 {
		k.Likelihood = 3
	}
	if k.Impact == 0 {
		k.Impact = 3
	}
	if k.Likelihood < 1 || k.Likelihood > 5 || k.Impact < 1 || k.Impact > 5 {
		return Invalidf("likelihood and impact must be between 1 and 5")
	}
	if k.Status == "" {
		k.Status = "identified"
	}
	if !slices.Contains(model.RiskStatuses, k.Status) {
		return Invalidf("status must be one of %s", strings.Join(model.RiskStatuses, ", "))
	}
	k.Category = strings.TrimSpace(k.Category)
	return validateResidual(k)
}

// CreateRisk adds a risk with a new key (PROJ-RISK-n) and its requirements.
func (s *Store) CreateRisk(ctx context.Context, tenantID string, k *model.Risk) error {
	if err := validateRisk(k); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, k.ProjectID); err != nil {
		return err
	}
	k.ID = newID()
	k.CreatedAt = s.now()
	k.UpdatedAt = k.CreatedAt
	return s.tx(ctx, func(c conn) error {
		seq, key, err := nextKey(ctx, c, k.ProjectID, model.KindRisk)
		if err != nil {
			return err
		}
		k.Key = key
		if _, err := c.exec(ctx, `INSERT INTO risks (id, tenant_id, project_id, seq, key, title, description, category, likelihood, impact, status, mitigation, owner, created_by, created_at, updated_at,
			residual_likelihood, residual_impact, residual_manual, residual_justification, review_date, review_owner) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			k.ID, tenantID, k.ProjectID, seq, k.Key, k.Title, k.Description, k.Category, k.Likelihood, k.Impact, k.Status, k.Mitigation, k.Owner, k.CreatedBy, k.CreatedAt, k.UpdatedAt,
			k.ResidualLikelihood, k.ResidualImpact, boolInt(k.ResidualManual), k.ResidualJustification, k.ReviewDate, k.ReviewOwner); err != nil {
			return mapErr(err, "risk")
		}
		return setRiskRequirements(ctx, c, tenantID, k.ProjectID, k.ID, k.RequirementIDs, nil)
	})
}

// UpdateRisk saves the editable fields.
func (s *Store) UpdateRisk(ctx context.Context, tenantID string, k *model.Risk) error {
	if err := validateRisk(k); err != nil {
		return err
	}
	k.UpdatedAt = s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE risks SET title = ?, description = ?, category = ?, likelihood = ?, impact = ?, status = ?, mitigation = ?, owner = ?, updated_at = ?,
		residual_likelihood = ?, residual_impact = ?, residual_manual = ?, residual_justification = ?, review_date = ?, review_owner = ? WHERE id = ? AND tenant_id = ?`,
		k.Title, k.Description, k.Category, k.Likelihood, k.Impact, k.Status, k.Mitigation, k.Owner, k.UpdatedAt,
		k.ResidualLikelihood, k.ResidualImpact, boolInt(k.ResidualManual), k.ResidualJustification, k.ReviewDate, k.ReviewOwner, k.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "risk")
	}
	return nil
}

// setRiskRequirements adds and removes requirements of a risk.
func setRiskRequirements(ctx context.Context, c conn, tenantID, projectID, riskID string, add, remove []string) error {
	for _, id := range add {
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM requirements WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
			return mapErr(err, "requirement")
		}
		if pid != projectID {
			return Invalidf("requirement %s belongs to another project", id)
		}
		if _, err := c.exec(ctx, `INSERT INTO risk_requirements (risk_id, requirement_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, riskID, id); err != nil {
			return err
		}
	}
	for _, id := range remove {
		if _, err := c.exec(ctx, `DELETE FROM risk_requirements WHERE risk_id = ? AND requirement_id = ?`, riskID, id); err != nil {
			return err
		}
	}
	return nil
}

// SetRiskRequirements adds and removes requirements of a risk.
func (s *Store) SetRiskRequirements(ctx context.Context, tenantID, riskID string, add, remove []string) error {
	k, err := s.GetRisk(ctx, tenantID, riskID)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(c conn) error { return setRiskRequirements(ctx, c, tenantID, k.ProjectID, k.ID, add, remove) })
}

// GetRisk returns a risk with its requirements and their coverage.
func (s *Store) GetRisk(ctx context.Context, tenantID, id string) (*model.Risk, error) {
	k, err := scanRisk(s.c(ctx).row(ctx, `SELECT `+riskCols+` FROM risks WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "risk")
	}
	list := []model.Risk{*k}
	if err := s.decorateRisks(ctx, tenantID, k.ProjectID, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// ListRisks lists the risks of a project, highest score first.
func (s *Store) ListRisks(ctx context.Context, tenantID, projectID string) ([]model.Risk, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+riskCols+` FROM risks WHERE tenant_id = ? AND project_id = ? ORDER BY likelihood * impact DESC, seq`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []model.Risk{}
	for rows.Next() {
		k, err := scanRisk(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.decorateRisks(ctx, tenantID, projectID, out)
}

// decorateRisks fills requirement IDs and coverage counts.
func (s *Store) decorateRisks(ctx context.Context, tenantID, projectID string, risks []model.Risk) error {
	if len(risks) == 0 {
		return nil
	}
	rows, err := s.c(ctx).query(ctx, `SELECT rr.risk_id, rr.requirement_id FROM risk_requirements rr JOIN risks k ON k.id = rr.risk_id JOIN requirements q ON q.id = rr.requirement_id WHERE k.project_id = ? AND k.tenant_id = ? ORDER BY q.seq`, projectID, tenantID)
	if err != nil {
		return err
	}
	reqs := map[string][]string{}
	for rows.Next() {
		var rid, qid string
		if err := rows.Scan(&rid, &qid); err != nil {
			rows.Close()
			return err
		}
		reqs[rid] = append(reqs[rid], qid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	links, err := s.ListLinks(ctx, tenantID, projectID, "", "")
	if err != nil {
		return err
	}
	r2t, _ := LinkIndex(links)
	last, err := s.LastConfigResults(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	threshold, err := s.RiskThreshold(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	today := s.now().UTC().Format("2006-01-02")
	for i := range risks {
		k := &risks[i]
		k.RequirementIDs = reqs[k.ID]
		if k.RequirementIDs == nil {
			k.RequirementIDs = []string{}
		}
		for _, q := range k.RequirementIDs {
			if len(r2t[q]) > 0 {
				k.Covered++
			}
			if CoverageOf(r2t[q], last) == model.ResultPassed {
				k.Verified++
			}
		}
		decorateResidual(k, threshold, today)
	}
	return nil
}

// DeleteRisk removes a risk.
func (s *Store) DeleteRisk(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM risk_requirements WHERE risk_id = ?`, id); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM risks WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "risk")
		}
		return nil
	})
}

// RequirementRiskLevels returns, per requirement of a project, the highest
// level of the open risks it addresses.
func (s *Store) RequirementRiskLevels(ctx context.Context, tenantID, projectID string) (map[string]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT rr.requirement_id, k.likelihood, k.impact FROM risk_requirements rr JOIN risks k ON k.id = rr.risk_id
		WHERE k.tenant_id = ? AND k.project_id = ? AND k.status IN ('identified', 'mitigating')`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id string
		var l, i int
		if err := rows.Scan(&id, &l, &i); err != nil {
			return nil, err
		}
		if lv := model.RiskLevel(l, i); RiskLevelAtLeast(lv, out[id]) || out[id] == "" {
			out[id] = lv
		}
	}
	return out, rows.Err()
}

// ValidateRisk checks and normalises a risk's fields without saving it.
func ValidateRisk(k *model.Risk) error { return validateRisk(k) }
