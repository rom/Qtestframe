package store

// Administration of organisations by super administrators: suspension,
// the policy (quotas, AI, sign-in methods, e-mail domains) and the usage
// counts (see model.TenantPolicy and migration 0063).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
)

// SetTenantSuspended suspends (on) or reactivates an organisation. The
// sessions of a suspended organisation are moved out of it.
func (s *Store) SetTenantSuspended(ctx context.Context, id string, on bool) (*model.Tenant, error) {
	err := s.tx(ctx, func(c conn) error {
		var at any
		if on {
			at = s.now()
		}
		res, err := c.exec(ctx, `UPDATE tenants SET suspended_at = ? WHERE id = ?`, at, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "tenant")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetTenant(ctx, id)
}

// TenantPolicy returns the policy of an organisation.
func (s *Store) TenantPolicy(ctx context.Context, id string) (model.TenantPolicy, error) {
	return tenantPolicy(ctx, s.c(ctx), id)
}

func tenantPolicy(ctx context.Context, c conn, id string) (model.TenantPolicy, error) {
	var raw string
	var p model.TenantPolicy
	if err := c.row(ctx, `SELECT policy FROM tenants WHERE id = ?`, id).Scan(&raw); err != nil {
		return p, mapErr(err, "tenant")
	}
	_ = json.Unmarshal([]byte(raw), &p) // unknown keys are ignored
	normalizePolicyLists(&p)
	return p, nil
}

func normalizePolicyLists(p *model.TenantPolicy) {
	if p.AllowedProviders == nil {
		p.AllowedProviders = []string{}
	}
	if p.EmailDomains == nil {
		p.EmailDomains = []string{}
	}
}

// NormalizeTenantPolicy validates p and cleans its lists (trimmed, lower
// case domains, without duplicates).
func NormalizeTenantPolicy(p *model.TenantPolicy) error {
	if p.MaxMembers < 0 || p.MaxProjects < 0 || p.MaxStorageMB < 0 || p.MaxMembers > 1000000 || p.MaxProjects > 1000000 || p.MaxStorageMB > 100000000 {
		return Invalidf("quotas must be 0 (no limit) or a positive number")
	}
	var errs []error
	providers := []string{}
	for _, v := range p.AllowedProviders {
		v = strings.TrimSpace(v)
		if v == "" || slices.Contains(providers, v) {
			continue
		}
		kind, name, _ := strings.Cut(v, ":")
		if !(v == "local" || ((kind == "oidc" || kind == "saml") && name != "" && len(name) <= 64 && !strings.ContainsAny(name, " \t\r\n"))) {
			errs = append(errs, Invalidf("allowed sign-in method %q: use local, oidc:<name> or saml:<name>", v))
			continue
		}
		providers = append(providers, v)
	}
	domains := []string{}
	for _, d := range p.EmailDomains {
		d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(d), "@")))
		if d == "" || slices.Contains(domains, d) {
			continue
		}
		if strings.HasPrefix(d, "*.") || !config.ValidHostPattern(d) || !strings.Contains(d, ".") {
			errs = append(errs, Invalidf("e-mail domain %q is not a domain such as example.com", d))
			continue
		}
		domains = append(domains, d)
	}
	if len(providers) > 50 || len(domains) > 200 {
		errs = append(errs, Invalidf("too many sign-in methods or e-mail domains"))
	}
	p.Note = strings.TrimSpace(p.Note)
	if len(p.Note) > 2000 {
		errs = append(errs, Invalidf("the note is too long (at most 2000 characters)"))
	}
	p.AllowedProviders, p.EmailDomains = providers, domains
	return errors.Join(errs...)
}

// SetTenantPolicy stores the policy of an organisation. An e-mail domain
// can belong to one organisation only.
func (s *Store) SetTenantPolicy(ctx context.Context, id string, p model.TenantPolicy) (model.TenantPolicy, error) {
	if err := NormalizeTenantPolicy(&p); err != nil {
		return p, err
	}
	err := s.tx(ctx, func(c conn) error {
		for _, d := range p.EmailDomains {
			other, err := tenantByEmailDomain(ctx, c, d)
			if err != nil {
				return err
			}
			if other != "" && other != id {
				return Conflictf("the e-mail domain %s already belongs to another organisation", d)
			}
		}
		b, _ := json.Marshal(p)
		res, err := c.exec(ctx, `UPDATE tenants SET policy = ? WHERE id = ?`, string(b), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "tenant")
		}
		return nil
	})
	return p, err
}

// TenantByEmailDomain returns the id of the organisation whose policy lists
// the e-mail domain ("" = none).
func (s *Store) TenantByEmailDomain(ctx context.Context, domain string) (string, error) {
	return tenantByEmailDomain(ctx, s.c(ctx), strings.ToLower(strings.TrimSpace(domain)))
}

func tenantByEmailDomain(ctx context.Context, c conn, domain string) (string, error) {
	if domain == "" {
		return "", nil
	}
	// a cheap filter in SQL, the exact match on the decoded list
	rows, err := c.query(ctx, `SELECT id, policy FROM tenants WHERE policy LIKE ? ORDER BY created_at`, `%"`+escapeLike(domain)+`"%`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return "", err
		}
		var p model.TenantPolicy
		_ = json.Unmarshal([]byte(raw), &p)
		if slices.Contains(p.EmailDomains, domain) {
			return id, nil
		}
	}
	return "", rows.Err()
}

// escapeLike drops the LIKE wildcards of s (domains have none).
func escapeLike(s string) string {
	return strings.NewReplacer("%", "", "_", "").Replace(s)
}

// TenantUsage counts what an organisation holds.
func (s *Store) TenantUsage(ctx context.Context, id string) (model.TenantUsage, error) {
	var u model.TenantUsage
	c := s.c(ctx)
	if _, err := s.GetTenant(ctx, id); err != nil {
		return u, err
	}
	for _, q := range []struct {
		sql string
		dst *int
	}{
		{`SELECT COUNT(*) FROM memberships WHERE tenant_id = ?`, &u.Members},
		{`SELECT COUNT(*) FROM projects WHERE tenant_id = ?`, &u.Projects},
		{`SELECT COUNT(*) FROM requirements WHERE tenant_id = ?`, &u.Requirements},
		{`SELECT COUNT(*) FROM test_cases WHERE tenant_id = ?`, &u.Tests},
		{`SELECT COUNT(*) FROM test_runs WHERE tenant_id = ?`, &u.Runs},
		{`SELECT COUNT(*) FROM files WHERE tenant_id = ?`, &u.Files},
	} {
		if err := c.row(ctx, q.sql, id).Scan(q.dst); err != nil {
			return u, err
		}
	}
	if err := c.row(ctx, `SELECT COALESCE(SUM(size), 0) FROM files WHERE tenant_id = ?`, id).Scan(&u.StorageBytes); err != nil {
		return u, err
	}
	var last sql.NullTime
	err := c.row(ctx, `SELECT created_at FROM audit_log WHERE tenant_id = ? ORDER BY created_at DESC LIMIT 1`, id).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return u, err
	}
	u.LastActivity = timePtr(last)
	return u, nil
}

// quotas checked by checkQuota.
const (
	quotaMembers = iota
	quotaProjects
	quotaStorage
)

// checkQuota refuses one more member or project, or extra more bytes of
// files, beyond the organisation's policy.
func (s *Store) checkQuota(ctx context.Context, c conn, tenantID string, kind int, extra int64) error {
	p, err := tenantPolicy(ctx, c, tenantID)
	if errors.Is(err, ErrNotFound) {
		return nil // the insert reports the missing tenant
	}
	if err != nil {
		return err
	}
	switch kind {
	case quotaMembers:
		if p.MaxMembers > 0 {
			var n int
			if err := c.row(ctx, `SELECT COUNT(*) FROM memberships WHERE tenant_id = ?`, tenantID).Scan(&n); err != nil {
				return err
			}
			if n >= p.MaxMembers {
				return Conflictf("the organisation has reached its limit of %d members", p.MaxMembers)
			}
		}
	case quotaProjects:
		if p.MaxProjects > 0 {
			var n int
			if err := c.row(ctx, `SELECT COUNT(*) FROM projects WHERE tenant_id = ?`, tenantID).Scan(&n); err != nil {
				return err
			}
			if n >= p.MaxProjects {
				return Conflictf("the organisation has reached its limit of %d projects", p.MaxProjects)
			}
		}
	case quotaStorage:
		if p.MaxStorageMB > 0 {
			var used int64
			if err := c.row(ctx, `SELECT COALESCE(SUM(size), 0) FROM files WHERE tenant_id = ?`, tenantID).Scan(&used); err != nil {
				return err
			}
			if used+extra > int64(p.MaxStorageMB)<<20 {
				return Conflictf("the organisation has reached its storage limit of %d MB", p.MaxStorageMB)
			}
		}
	}
	return nil
}
