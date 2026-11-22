package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// Residual risk: the likelihood and impact of a risk after its mitigation,
// the project's risk acceptance threshold, the review date and the "accept
// risk" sign-off.

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// validateResidual checks the residual assessment, review and acceptance
// fields of a risk.
func validateResidual(k *model.Risk) error {
	if k.ResidualLikelihood < 0 || k.ResidualLikelihood > 5 || k.ResidualImpact < 0 || k.ResidualImpact > 5 {
		return Invalidf("residual likelihood and impact must be between 1 and 5 (0 = not assessed)")
	}
	if (k.ResidualLikelihood == 0) != (k.ResidualImpact == 0) {
		return Invalidf("give both residual likelihood and residual impact, or neither")
	}
	k.ResidualJustification = strings.TrimSpace(k.ResidualJustification)
	if k.ResidualManual {
		if k.ResidualLikelihood == 0 {
			return Invalidf("a residual score entered by hand needs residual likelihood and impact")
		}
		if len([]rune(k.ResidualJustification)) < 3 {
			return Invalidf("a residual score entered by hand needs a justification")
		}
	}
	if len([]rune(k.ResidualJustification)) > 4000 {
		return Invalidf("the residual justification is at most 4000 characters")
	}
	k.ReviewDate, k.ReviewOwner = strings.TrimSpace(k.ReviewDate), strings.TrimSpace(k.ReviewOwner)
	if k.ReviewDate != "" && !dateRe.MatchString(k.ReviewDate) {
		return Invalidf("review_date must be YYYY-MM-DD")
	}
	return nil
}

// ValidRiskThreshold reports whether t is a risk level ("" = the default).
func ValidRiskThreshold(t string) bool { return t == "" || slices.Contains(model.RiskLevels, t) }

// EffectiveThreshold returns the first set threshold of the candidates,
// else model.RiskThresholdDefault.
func EffectiveThreshold(candidates ...string) string {
	for _, c := range candidates {
		if slices.Contains(model.RiskLevels, c) {
			return c
		}
	}
	return model.RiskThresholdDefault
}

// LevelAbove reports whether level is above threshold.
func LevelAbove(level, threshold string) bool {
	return slices.Index(model.RiskLevels, level) > slices.Index(model.RiskLevels, EffectiveThreshold(threshold))
}

// RiskAcceptanceHash is what an "accept risk" signature signs: the risk
// and its residual assessment as accepted.
func RiskAcceptanceHash(k *model.Risk) string {
	l, i, src := k.ResidualOf(k.ResidualSource == model.ResidualVerified)
	h := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%d\x00%d\x00%d\x00%d\x00%s\x00%s", k.ID, k.Title, k.Likelihood, k.Impact, l, i, src, k.ResidualJustification))
	return hex.EncodeToString(h[:])
}

// decorateResidual fills the derived residual fields of a risk whose
// Verified count is known.
func decorateResidual(k *model.Risk, threshold, today string) {
	verified := len(k.RequirementIDs) > 0 && k.Verified == len(k.RequirementIDs)
	l, i, src := k.ResidualOf(verified)
	k.ResidualScore, k.ResidualLevel, k.ResidualSource = l*i, model.RiskLevel(l, i), src
	k.Threshold = EffectiveThreshold(threshold)
	k.AboveThreshold = k.Status != "closed" && LevelAbove(k.ResidualLevel, k.Threshold)
	k.AcceptanceValid = k.AcceptedBy != "" && k.AcceptanceHash == RiskAcceptanceHash(k)
	k.ReviewDue = k.ReviewDate != "" && k.ReviewDate <= today && k.Status != "closed"
}

// RiskThreshold returns the project's risk acceptance threshold as stored
// ("" = model.RiskThresholdDefault).
func (s *Store) RiskThreshold(ctx context.Context, tenantID, projectID string) (string, error) {
	var t string
	err := s.c(ctx).row(ctx, `SELECT risk_threshold FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&t)
	if err != nil {
		return "", mapErr(err, "project")
	}
	return t, nil
}

// SetRiskThreshold changes the project's risk acceptance threshold.
func (s *Store) SetRiskThreshold(ctx context.Context, tenantID, projectID, threshold string) error {
	if !ValidRiskThreshold(threshold) {
		return Invalidf("threshold must be one of %s (or empty for the default)", strings.Join(model.RiskLevels, ", "))
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE projects SET risk_threshold = ? WHERE id = ? AND tenant_id = ?`, threshold, projectID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "project")
	}
	return nil
}

// AcceptRisk records the "accept risk" sign-off on a risk's residual
// assessment and sets its status to accepted.
func (s *Store) AcceptRisk(ctx context.Context, tenantID, id, email, name, comment string) (*model.Risk, error) {
	if len([]rune(comment)) > 2000 {
		return nil, Invalidf("the comment is at most 2000 characters")
	}
	k, err := s.GetRisk(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if k.Status == "closed" {
		return nil, Invalidf("a closed risk cannot be accepted")
	}
	now := s.now()
	hash := RiskAcceptanceHash(k)
	if _, err := s.c(ctx).exec(ctx, `UPDATE risks SET status = 'accepted', accepted_by = ?, accepted_name = ?, accepted_at = ?, acceptance_comment = ?, acceptance_hash = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		email, name, now, strings.TrimSpace(comment), hash, now, id, tenantID); err != nil {
		return nil, err
	}
	return s.GetRisk(ctx, tenantID, id)
}

// WithdrawRiskAcceptance removes the sign-off; an accepted risk goes back
// to mitigating.
func (s *Store) WithdrawRiskAcceptance(ctx context.Context, tenantID, id string) (*model.Risk, error) {
	k, err := s.GetRisk(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	status := k.Status
	if status == "accepted" {
		status = "mitigating"
	}
	if _, err := s.c(ctx).exec(ctx, `UPDATE risks SET status = ?, accepted_by = '', accepted_name = '', accepted_at = NULL, acceptance_comment = '', acceptance_hash = '', updated_at = ? WHERE id = ? AND tenant_id = ?`,
		status, s.now(), id, tenantID); err != nil {
		return nil, err
	}
	return s.GetRisk(ctx, tenantID, id)
}
