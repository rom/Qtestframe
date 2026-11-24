package api

// Privacy and data retention (see docs/admin-guide "Privacy and data
// retention"): the retention policy with its dry-run preview and the
// nightly purge, legal holds on projects and releases, the export of a
// person's data (by super administrators and by the person) and the
// erasure (anonymisation) of an account, and IP address truncation.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

func (s *Server) routesPrivacy(h routeFunc) {
	h("GET /api/v1/admin/retention", s.handleRetentionPreview)
	h("POST /api/v1/admin/retention/run", s.handleRetentionRun)
	h("GET /api/v1/projects/{pid}/legal-hold", s.handleGetLegalHold, "project.read")
	h("PUT /api/v1/projects/{pid}/legal-hold", s.handleSetLegalHold(false), "project.read")
	h("PUT /api/v1/projects/{pid}/releases/{rid}/legal-hold", s.handleSetLegalHold(true), "project.read")
	h("GET /api/v1/users/{id}/export", s.handleExportUserData)
	h("POST /api/v1/users/{id}/erase", s.handleEraseUser)
	h("GET /api/v1/auth/me/export", s.handleExportMyData)
}

// ---------------------------------------------------------------- IP addresses

// TruncateIP shortens an address for storage and logs: IPv4 to its /24
// (the last byte 0), IPv6 to its /48. Anything else is returned unchanged.
func TruncateIP(ip string) string {
	a := net.ParseIP(ip)
	if a == nil {
		return ip
	}
	if v4 := a.To4(); v4 != nil {
		return net.IPv4(v4[0], v4[1], v4[2], 0).String()
	}
	return a.Mask(net.CIDRMask(48, 128)).String()
}

// loggedIP is the client address as it is stored and logged: truncated
// when privacy.truncate_ip is on (rate limits use the full address).
func (s *Server) loggedIP(r *http.Request) string {
	ip := s.clientIP(r)
	if s.conf().Privacy.TruncateIP {
		return TruncateIP(ip)
	}
	return ip
}

// ---------------------------------------------------------------- retention

const retentionJob = "retention.purge"

// systemRetention returns the system values of c by category.
func systemRetention(c *config.Config) (map[string]int, int) {
	r := c.Retention
	return map[string]int{model.RetainRuns: r.RunsDays, model.RetainLogs: r.LogsDays, model.RetainNotifications: r.NotificationsDays,
		model.RetainSessions: r.SessionsDays, model.RetainChat: r.ChatDays, model.RetainExploratory: r.ExploratoryDays}, r.RunsKeepLatest
}

// retentionReports previews (dryRun) or performs the purge of every
// organisation and of the system data.
func (s *Server) retentionReports(ctx context.Context, dryRun bool) ([]model.RetentionReport, error) {
	days, keep := systemRetention(s.conf())
	now := s.now()
	ts, err := s.Store.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	sys := store.NewRetentionPolicy(days, keep, nil)
	counts, err := s.Store.RetentionSystem(ctx, sys, dryRun)
	if err != nil {
		return nil, err
	}
	out := []model.RetentionReport{{TenantID: "", DryRun: dryRun, Policy: sys, Counts: counts, At: now}}
	for _, t := range ts {
		pol, err := s.Store.TenantPolicy(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		p := store.NewRetentionPolicy(days, keep, pol.Retention)
		counts, err := s.Store.Retention(ctx, t.ID, p, dryRun)
		if err != nil {
			return nil, err
		}
		out = append(out, model.RetentionReport{TenantID: t.ID, TenantName: t.Name, DryRun: dryRun, Policy: p, Counts: counts, At: now})
	}
	return out, nil
}

// retentionDetails is the audit text of a purge.
func retentionDetails(rep *model.RetentionReport) string {
	var parts []string
	for _, c := range rep.Counts {
		if c.Count > 0 || c.Held > 0 {
			p := fmt.Sprintf("%s: %d deleted (older than %d days)", c.Category, c.Count, c.Days)
			if c.Held > 0 {
				p += fmt.Sprintf(", %d kept by legal hold or signature", c.Held)
			}
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "nothing to delete"
	}
	return strings.Join(parts, "; ")
}

// purge runs the purge and records it in the audit trail of each
// organisation (and the system trail); who is the e-mail of the super
// administrator or "system" for the nightly job.
func (s *Server) purge(ctx context.Context, p *Principal) ([]model.RetentionReport, error) {
	reps, err := s.retentionReports(ctx, false)
	if err != nil {
		return nil, err
	}
	for i := range reps {
		rep := &reps[i]
		if rep.Total() == 0 && rep.TenantID != "" {
			continue
		}
		e := model.AuditEntry{TenantID: rep.TenantID, UserEmail: "system", Action: "retention.purge", EntityType: "retention", Details: retentionDetails(rep)}
		if p != nil {
			e.UserID, e.UserEmail = p.User.ID, p.User.Email
		}
		if err := s.Store.Audit(ctx, e); err != nil {
			s.Log.Warn("audit write failed", "err", err)
		}
		if n := rep.Total(); n > 0 {
			s.Log.Info("retention purge", "tenant", rep.TenantID, "deleted", n)
		}
	}
	return reps, nil
}

// retentionDue returns the most recent scheduled run at or before now.
func (s *Server) retentionDue(now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), s.conf().Retention.Hour, 0, 0, 0, time.UTC)
	if t.After(now) {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

// RunRetention runs the nightly purge when it is enabled and due (once
// per day) and reports whether it ran.
func (s *Server) RunRetention(ctx context.Context) (bool, error) {
	if !s.conf().Retention.Enabled {
		return false, nil
	}
	now := s.now()
	last, err := s.Store.JobLastRun(ctx, retentionJob)
	if err != nil {
		return false, err
	}
	if last != nil && !last.Before(s.retentionDue(now)) {
		return false, nil
	}
	if err := s.Store.SetJobLastRun(ctx, retentionJob, now); err != nil {
		return false, err
	}
	_, err = s.purge(ctx, nil)
	return true, err
}

// retentionView is the answer of GET /admin/retention.
type retentionView struct {
	Enabled bool                    `json:"enabled"`
	Hour    int                     `json:"hour"`
	LastRun *time.Time              `json:"last_run"`
	NextRun *time.Time              `json:"next_run"`
	Reports []model.RetentionReport `json:"reports"`
	Holds   []model.HeldItem        `json:"holds"`
}

func (s *Server) retentionView(ctx context.Context, reps []model.RetentionReport) (*retentionView, error) {
	holds, err := s.Store.LegalHolds(ctx, "")
	if err != nil {
		return nil, err
	}
	last, err := s.Store.JobLastRun(ctx, retentionJob)
	if err != nil {
		return nil, err
	}
	c := s.conf()
	v := &retentionView{Enabled: c.Retention.Enabled, Hour: c.Retention.Hour, LastRun: last, Reports: reps, Holds: holds}
	if v.Enabled {
		next := s.retentionDue(s.now()).AddDate(0, 0, 1)
		if last == nil || last.Before(s.retentionDue(s.now())) {
			next = s.now()
		}
		v.NextRun = &next
	}
	return v, nil
}

// handleRetentionPreview: GET /admin/retention — the dry run: what the
// purge would delete now, per organisation and category, and the holds.
func (s *Server) handleRetentionPreview(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	reps, err := s.retentionReports(r.Context(), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.retentionView(r.Context(), reps)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// handleRetentionRun: POST /admin/retention/run {"confirm": "PURGE"} —
// purge now (also while the nightly job is off).
func (s *Server) handleRetentionRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Confirm) != "PURGE" {
		s.fail(w, r, badRequest(`type PURGE to confirm the deletion`))
		return
	}
	reps, err := s.purge(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetJobLastRun(r.Context(), retentionJob, s.now()); err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.retentionView(r.Context(), reps)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ---------------------------------------------------------------- legal hold

// legalHoldView is the legal hold of a project and its releases.
type legalHoldView struct {
	Project  model.LegalHold     `json:"project"`
	Releases []model.ReleaseHold `json:"releases"`
	// CanEdit: the caller may set and lift holds (organisation and super
	// administrators).
	CanEdit bool `json:"can_edit"`
}

func (s *Server) respondLegalHold(w http.ResponseWriter, r *http.Request, p *Principal, projectID string) {
	h, rels, err := s.Store.ProjectLegalHold(r.Context(), p.TenantID(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, legalHoldView{Project: h, Releases: rels, CanEdit: canHold(p)})
}

func canHold(p *Principal) bool { return p.Token == nil && p.Can(rbac.TenantManage) }

func (s *Server) handleGetLegalHold(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	s.respondLegalHold(w, r, p, pr.ID)
}

// handleSetLegalHold: PUT /projects/{pid}/legal-hold and
// /projects/{pid}/releases/{rid}/legal-hold {"on": bool, "reason": "…"}.
func (s *Server) handleSetLegalHold(release bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		if !canHold(p) {
			s.fail(w, r, forbidden("only organisation and super administrators can set or lift a legal hold"))
			return
		}
		var req model.LegalHold
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		rid, entity, id := "", "project", pr.ID
		if release {
			rid, entity, id = r.PathValue("rid"), "release", r.PathValue("rid")
		}
		h, err := s.Store.SetLegalHold(r.Context(), p.TenantID(), pr.ID, rid, req, p.User.Email)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		action, details := "legal_hold.lift", pr.Key
		if h.On {
			action, details = "legal_hold.set", pr.Key+": "+h.Reason
		}
		s.audit(r.Context(), p, action, entity, id, details)
		s.respondLegalHold(w, r, p, pr.ID)
	}
}

// ---------------------------------------------------------------- data subject tools

// writeDataExport answers with the ZIP of a person's data: one JSON file
// per section and a short explanation.
func (s *Server) writeDataExport(w http.ResponseWriter, r *http.Request, userID string) (string, bool) {
	secs, err := s.Store.ExportUserData(r.Context(), userID)
	if err != nil {
		s.fail(w, r, err)
		return "", false
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	now := s.now()
	index := map[string]any{"format": "qtestframe-personal-data-v1", "user_id": userID, "exported_at": now, "files": []string{}}
	files := []string{}
	for _, sec := range secs {
		f, err := z.Create(sec.Name + ".json")
		if err != nil {
			s.fail(w, r, err)
			return "", false
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(sec.Rows); err != nil {
			s.fail(w, r, err)
			return "", false
		}
		files = append(files, sec.Name+".json")
	}
	index["files"] = files
	f, _ := z.Create("index.json")
	b, _ := json.MarshalIndent(index, "", "  ")
	_, _ = f.Write(b)
	readme, _ := z.Create("README.txt")
	_, _ = readme.Write([]byte(exportReadme))
	if err := z.Close(); err != nil {
		s.fail(w, r, err)
		return "", false
	}
	email := ""
	if len(secs) > 0 && len(secs[0].Rows) > 0 {
		email, _ = secs[0].Rows[0]["email"].(string)
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="personal-data-`+now.Format("20060102-150405")+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
	return email, true
}

const exportReadme = `Personal data export (Qtestframe)

Each JSON file holds the records about you of one kind: profile, memberships
and roles, project roles, comments you wrote, comments that mention you,
sessions, API tokens (without their secrets), the sign-in log, remembered
devices, notifications, electronic signatures, input history and the audit
trail entries by or about you. index.json lists the files.

Passwords, second-factor secrets and token secrets are never exported.
Content you created in projects (requirements, tests, runs) belongs to the
organisation and is exported by its administrators.
`

// handleExportUserData: GET /users/{id}/export (super administrators).
func (s *Server) handleExportUserData(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	id := r.PathValue("id")
	if _, ok := s.writeDataExport(w, r, id); ok {
		s.audit(r.Context(), p, "user.data_export", "user", id, "by a super administrator")
	}
}

// handleExportMyData: GET /auth/me/export — the caller's own data.
func (s *Server) handleExportMyData(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Token != nil {
		s.fail(w, r, forbidden("sign in to download your data (API tokens cannot)"))
		return
	}
	if _, ok := s.writeDataExport(w, r, p.User.ID); ok {
		s.audit(r.Context(), p, "user.data_export", "user", p.User.ID, "self-service")
	}
}

// handleEraseUser: POST /users/{id}/erase {"confirm": "<e-mail>"} —
// anonymise an account (super administrators; typed confirmation).
func (s *Server) handleEraseUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUser(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	switch {
	case u.ID == p.User.ID:
		s.fail(w, r, badRequest("you cannot erase your own account"))
		return
	case !strings.EqualFold(strings.TrimSpace(req.Confirm), u.Email):
		s.fail(w, r, badRequest("type the account's e-mail address to confirm the erasure"))
		return
	}
	if u.SuperAdmin && !u.Disabled {
		n, err := s.Store.CountSuperAdmins(ctx)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if n <= 1 {
			s.fail(w, r, badRequest("the last super administrator cannot be erased"))
			return
		}
	}
	res, err := s.Store.EraseUser(ctx, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var del []string
	for k, v := range res.Deleted {
		del = append(del, fmt.Sprintf("%s %d", k, v))
	}
	slices.Sort(del)
	// the audit entry names the pseudonym only (not the erased address)
	s.audit(ctx, p, "user.erase", "user", u.ID, fmt.Sprintf("%s; deleted: %s; %d fields re-attributed; kept: %d signatures, %d sign-offs, %d audit entries",
		res.Pseudonym, strings.Join(del, ", "), res.Rewritten, res.Kept["electronic_signatures"], res.Kept["sign_offs"], res.Kept["audit_entries"]))
	writeJSON(w, http.StatusOK, res)
}
