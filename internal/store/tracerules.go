package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/similarity"
)

// Traceability rules of a project (see model.TraceRule). The selector and
// target queries are checked by the caller (the API knows the schemas).

const traceRuleCols = `id, project_id, name, description, severity, enabled, scope, selector, kind, min_count, options, position, created_by, created_at, updated_at`

// MaxTraceRules bounds the rules of one project.
const MaxTraceRules = 200

func scanTraceRule(r interface{ Scan(...any) error }) (*model.TraceRule, error) {
	var tr model.TraceRule
	var opts string
	if err := r.Scan(&tr.ID, &tr.ProjectID, &tr.Name, &tr.Description, &tr.Severity, &tr.Enabled, &tr.Scope, &tr.Selector, &tr.Kind, &tr.Min, &opts, &tr.Position, &tr.CreatedBy, &tr.CreatedAt, &tr.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(opts), &tr.TraceRuleOptions)
	return &tr, nil
}

// ValidateTraceRule checks and normalises the fields of a rule: options
// that do not apply to its kind are cleared.
func ValidateTraceRule(r *model.TraceRule) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || len([]rune(r.Name)) > 200 {
		return Invalidf("rule name is required (at most 200 characters)")
	}
	r.Description = strings.TrimSpace(r.Description)
	if len(r.Description) > 4000 {
		return Invalidf("description is too long (at most 4000 characters)")
	}
	if r.Severity == "" {
		r.Severity = "error"
	}
	if !slices.Contains(model.TraceRuleSeverities, r.Severity) {
		return Invalidf("severity must be one of %s", strings.Join(model.TraceRuleSeverities, ", "))
	}
	if !slices.Contains(model.TraceRuleKinds, r.Kind) {
		return Invalidf("kind must be one of %s", strings.Join(model.TraceRuleKinds, ", "))
	}
	if model.IsBuiltinCheck(r.Kind) {
		return validateBuiltinRule(r)
	}
	r.Threshold, r.Level = 0, ""
	switch r.Kind {
	case model.TraceVerifiedBy, model.TraceRelation:
		if r.Scope == "" {
			r.Scope = model.KindRequirement
		}
		if r.Scope != model.KindRequirement {
			return Invalidf("rules of kind %s apply to requirements", r.Kind)
		}
	case model.TraceVerifies:
		if r.Scope == "" {
			r.Scope = model.KindTest
		}
		if r.Scope != model.KindTest {
			return Invalidf("rules of kind %s apply to tests", r.Kind)
		}
	default:
		if r.Scope == "" {
			r.Scope = model.KindRequirement
		}
		if r.Scope != model.KindRequirement && r.Scope != model.KindTest {
			return Invalidf("scope must be requirement or test")
		}
	}
	if r.Min < 0 || r.Min > 1000 || (r.Min == 0 && !r.Every) {
		return Invalidf("min must be between 1 and 1000 (0 only with every)")
	}
	r.Selector = strings.TrimSpace(r.Selector)
	o := &r.TraceRuleOptions
	o.TargetQuery = strings.TrimSpace(o.TargetQuery)
	if len(r.Selector) > 4096 || len(o.TargetQuery) > 4096 {
		return Invalidf("queries are limited to 4096 characters")
	}
	if !slices.Contains(model.TraceRuleDirections, o.Direction) {
		return Invalidf("direction must be incoming, outgoing or empty (any)")
	}
	switch r.Kind {
	case model.TraceVerifiedBy:
		o.RelationType, o.Direction, o.TargetType = "", "", ""
	case model.TraceVerifies:
		o.Passed, o.RelationType, o.Direction, o.TargetType = false, "", "", ""
	case model.TraceRelation:
		o.NotSuspect, o.Passed, o.TargetType = false, false, ""
		if o.RelationType != "" && !slices.Contains(model.RelationTypes, o.RelationType) {
			return Invalidf("relation_type must be one of %s (or empty for any)", strings.Join(model.RelationTypes, ", "))
		}
	case model.TraceItemLink:
		o.NotSuspect, o.Passed, o.RelationType = false, false, ""
		if o.TargetType != "" && !slices.Contains(model.ItemLinkTargetTypes, o.TargetType) {
			return Invalidf("target_type must be one of %s (or empty for any)", strings.Join(model.ItemLinkTargetTypes, ", "))
		}
		if o.TargetQuery != "" && o.TargetType != model.KindRequirement && o.TargetType != model.KindTest {
			return Invalidf("target_query needs target_type requirement or test")
		}
	}
	return nil
}

// validateBuiltinRule normalises a built-in consistency check: its scope
// is fixed, it has no minimum and only its own option.
func validateBuiltinRule(r *model.TraceRule) error {
	scope := model.BuiltinScope(r.Kind)
	if r.Scope != "" && r.Scope != scope {
		return Invalidf("rules of kind %s apply to %s items", r.Kind, scope)
	}
	r.Scope = scope
	r.Selector = strings.TrimSpace(r.Selector)
	if len(r.Selector) > 4096 {
		return Invalidf("queries are limited to 4096 characters")
	}
	if scope == model.KindRisk && r.Selector != "" {
		return Invalidf("rules of kind %s take no selector", r.Kind)
	}
	r.Min = 1
	o := model.TraceRuleOptions{}
	switch r.Kind {
	case model.CheckDuplicateSuspect, model.CheckTestDuplicateSuspect, model.CheckPotentialConflict:
		o.Threshold = r.Threshold
		if o.Threshold == 0 {
			o.Threshold = DefaultDuplicateThreshold
			if r.Kind == model.CheckPotentialConflict {
				o.Threshold = similarity.DefaultConflictConfidence
			}
		}
		if o.Threshold < 0.5 || o.Threshold > 1 {
			return Invalidf("threshold must be between 0.5 and 1")
		}
	case model.CheckUnmitigatedRisk:
		o.Level = r.Level
		if o.Level == "" {
			o.Level = "high"
		}
		if !slices.Contains(model.RiskLevels, o.Level) {
			return Invalidf("level must be one of %s", strings.Join(model.RiskLevels, ", "))
		}
	}
	r.TraceRuleOptions = o
	return nil
}

// DefaultDuplicateThreshold is the similarity from which duplicate_suspect
// and test_duplicate_suspect report two items (internal/similarity).
const DefaultDuplicateThreshold = similarity.DefaultDuplicateThreshold

// CreateTraceRule adds a rule at the end of the project's rules.
func (s *Store) CreateTraceRule(ctx context.Context, tenantID string, r *model.TraceRule) error {
	if err := ValidateTraceRule(r); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, r.ProjectID); err != nil {
		return err
	}
	return s.tx(ctx, func(c conn) error {
		var n, pos int
		if err := c.row(ctx, `SELECT COUNT(*), COALESCE(MAX(position), 0) FROM trace_rules WHERE project_id = ?`, r.ProjectID).Scan(&n, &pos); err != nil {
			return err
		}
		if n >= MaxTraceRules {
			return Invalidf("a project can have at most %d traceability rules", MaxTraceRules)
		}
		r.ID = newID()
		r.Position = pos + 1
		r.CreatedAt = s.now()
		r.UpdatedAt = r.CreatedAt
		opts, _ := json.Marshal(r.TraceRuleOptions)
		_, err := c.exec(ctx, `INSERT INTO trace_rules (id, tenant_id, project_id, name, description, severity, enabled, scope, selector, kind, min_count, options, position, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ID, tenantID, r.ProjectID, r.Name, r.Description, r.Severity, r.Enabled, r.Scope, r.Selector, r.Kind, r.Min, string(opts), r.Position, r.CreatedBy, r.CreatedAt, r.UpdatedAt)
		return mapErr(err, "rule name")
	})
}

// GetTraceRule returns a rule.
func (s *Store) GetTraceRule(ctx context.Context, tenantID, id string) (*model.TraceRule, error) {
	r, err := scanTraceRule(s.c(ctx).row(ctx, `SELECT `+traceRuleCols+` FROM trace_rules WHERE id = ? AND tenant_id = ?`, id, tenantID))
	return r, mapErr(err, "rule")
}

// ListTraceRules lists the rules of a project in their order.
func (s *Store) ListTraceRules(ctx context.Context, tenantID, projectID string) ([]model.TraceRule, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+traceRuleCols+` FROM trace_rules WHERE tenant_id = ? AND project_id = ? ORDER BY position, created_at, id`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TraceRule{}
	for rows.Next() {
		r, err := scanTraceRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// UpdateTraceRule saves the editable fields of a rule.
func (s *Store) UpdateTraceRule(ctx context.Context, tenantID string, r *model.TraceRule) error {
	if err := ValidateTraceRule(r); err != nil {
		return err
	}
	r.UpdatedAt = s.now()
	opts, _ := json.Marshal(r.TraceRuleOptions)
	res, err := s.c(ctx).exec(ctx, `UPDATE trace_rules SET name = ?, description = ?, severity = ?, enabled = ?, scope = ?, selector = ?, kind = ?, min_count = ?, options = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		r.Name, r.Description, r.Severity, r.Enabled, r.Scope, r.Selector, r.Kind, r.Min, string(opts), r.UpdatedAt, r.ID, tenantID)
	if err != nil {
		return mapErr(err, "rule name")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "rule")
	}
	return nil
}

// DeleteTraceRule removes a rule.
func (s *Store) DeleteTraceRule(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM trace_rules WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "rule")
	}
	return nil
}
