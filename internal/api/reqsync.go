package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracker"
)

// Two-way sync of requirements with Jira and Azure DevOps. The connections
// and their write-only tokens are the import sources (reqsources.go); the
// external references of the imports link requirements to issues and work
// items. The engine is in reqsync_engine.go, the field values in
// reqsync_fields.go.
//
//   - GET  /projects/{pid}/req-sync: the configuration of both sources
//     (defaults where none is saved), with the webhook URLs.
//   - PUT  /projects/{pid}/req-sync/{source}: save one (webhook_secret is
//     write-only; the saver becomes the account scheduled syncs run as).
//   - POST /projects/{pid}/req-sync/{source}/run: sync now, or preview
//     with {"dry_run": true}.
//   - GET  /projects/{pid}/req-sync/runs[/{run}]: the sync log.
//   - GET  /projects/{pid}/req-sync/conflicts, POST …/conflicts/{cid}/resolve.
//   - POST /hooks/req-sync/jira/{pid} and /hooks/req-sync/azure-devops/{pid}:
//     the webhook receivers (public; the shared secret is the credential).
//
// Scheduled syncs run from RunJobs (every minute) when their interval has
// passed; a failing sync backs off.

func (s *Server) routesReqSync(h routeFunc) {
	h("GET /api/v1/projects/{pid}/req-sync", s.handleListReqSync, "project.read")
	h("PUT /api/v1/projects/{pid}/req-sync/{source}", s.handlePutReqSync, "project.update")
	h("POST /api/v1/projects/{pid}/req-sync/{source}/run", s.handleRunReqSync, "requirement.write")
	h("GET /api/v1/projects/{pid}/req-sync/runs", s.handleListReqSyncRuns, "project.read")
	h("GET /api/v1/projects/{pid}/req-sync/runs/{run}", s.handleGetReqSyncRun, "project.read")
	h("GET /api/v1/projects/{pid}/req-sync/conflicts", s.handleListReqSyncConflicts, "requirement.read")
	h("POST /api/v1/projects/{pid}/req-sync/conflicts/{cid}/resolve", s.handleResolveReqSyncConflict, "requirement.write")
}

// routesReqSyncHooks registers the public webhook receivers.
func (s *Server) routesReqSyncHooks(pub func(string, http.HandlerFunc)) {
	pub("POST /api/v1/hooks/req-sync/jira/{pid}", s.handleReqSyncHook(tracker.SourceJira))
	pub("POST /api/v1/hooks/req-sync/azure-devops/{pid}", s.handleReqSyncHook(tracker.SourceAzure))
}

var syncSystems = []string{tracker.SourceJira, tracker.SourceAzure}

func syncSystem(v string) (string, error) {
	if !slices.Contains(syncSystems, v) {
		return "", badRequest("unknown source %q (want jira or azure-devops)", v)
	}
	return v, nil
}

func (s *Server) syncHookURL(system, projectID string) string {
	return s.baseURL() + "/api/v1/hooks/req-sync/" + system + "/" + projectID
}

// syncView is the API view of a configuration (a default one when x is nil).
func (s *Server) syncView(ctx context.Context, p *Principal, pr *model.Project, system string, x *store.ReqSyncRow, conflicts map[string]int, linked map[string]int) model.ReqSyncConfig {
	v := model.ReqSyncConfig{System: system, Settings: defaultSyncSettings(system), WebhookURL: s.syncHookURL(system, pr.ID),
		OpenConflicts: conflicts[system], Linked: linked[system]}
	if x != nil {
		v.ID, v.Enabled, v.Settings, v.IntervalMinutes, v.HasWebhookSecret = x.ID, x.Enabled, x.Settings, x.IntervalMinutes, x.WebhookSecret != ""
		v.NextRunAt, v.LastRunAt, v.LastStatus, v.LastError, v.Failures = x.NextRunAt, x.LastRunAt, x.LastStatus, x.LastError, x.Failures
		v.Version, v.UpdatedBy, v.UpdatedAt = x.Version, x.UpdatedBy, x.UpdatedAt
		if x.RunAs != "" {
			if u, err := s.Store.GetUser(ctx, x.RunAs); err == nil {
				v.RunAs = u.Email
			}
		}
		if v.Settings.Fields == nil {
			v.Settings.Fields = []model.ReqSyncField{}
		}
	}
	_, err := s.importSourceCtx(ctx, p, pr.ID, system)
	v.Connected = err == nil
	return v
}

func (s *Server) syncCounts(ctx context.Context, tenantID, projectID string) (map[string]int, map[string]int, error) {
	conflicts, err := s.Store.CountReqSyncConflicts(ctx, tenantID, projectID)
	if err != nil {
		return nil, nil, err
	}
	linked := map[string]int{}
	for _, sys := range syncSystems {
		refs, err := s.Store.ListExternalRefs(ctx, tenantID, projectID, sys)
		if err != nil {
			return nil, nil, err
		}
		for _, r := range refs {
			if r.EntityType == model.KindRequirement {
				linked[sys]++
			}
		}
	}
	return conflicts, linked, nil
}

func (s *Server) handleListReqSync(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	rows, err := s.Store.ListReqSync(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	conflicts, linked, err := s.syncCounts(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.ReqSyncConfig{}
	for _, sys := range syncSystems {
		var x *store.ReqSyncRow
		for i := range rows {
			if rows[i].System == sys {
				x = &rows[i]
			}
		}
		out = append(out, s.syncView(r.Context(), p, pr, sys, x, conflicts, linked))
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handlePutReqSync(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	sys, err := syncSystem(r.PathValue("source"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Enabled         bool                  `json:"enabled"`
		IntervalMinutes int                   `json:"interval_minutes"`
		Settings        model.ReqSyncSettings `json:"settings"`
		WebhookSecret   string                `json:"webhook_secret"`
		ClearSecret     bool                  `json:"clear_webhook_secret"`
		Version         int                   `json:"version"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if in.Enabled && !p.Can(rbac.RequirementWr) {
		s.fail(w, r, forbidden("permission requirement.write required: enabled syncs run with the permissions of the account that saves them"))
		return
	}
	ctx := r.Context()
	cur, err := s.Store.ReqSync(ctx, p.TenantID(), pr.ID, sys)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if in.Settings.Fields == nil {
		in.Settings.Fields = defaultSyncFields(sys)
	}
	if in.Settings.FolderID != "" || in.Settings.Push.FolderID != "" {
		for _, id := range []string{in.Settings.FolderID, in.Settings.Push.FolderID} {
			if id == "" {
				continue
			}
			f, err := s.Store.GetFolder(ctx, p.TenantID(), id)
			if err != nil || f.ProjectID != pr.ID || f.Kind != model.KindRequirement {
				s.fail(w, r, badRequest("folder %s is not a requirement folder of the project", id))
				return
			}
		}
	}
	// comments made before comment sync was turned on are not synced
	now := s.now()
	in.Settings.CommentsSince = nil
	if in.Settings.Comments {
		in.Settings.CommentsSince = &now
		if cur != nil && cur.Settings.Comments && cur.Settings.CommentsSince != nil {
			in.Settings.CommentsSince = cur.Settings.CommentsSince
		}
	}
	x := &store.ReqSyncRow{TenantID: p.TenantID(), ProjectID: pr.ID, System: sys, Enabled: in.Enabled, Settings: in.Settings,
		IntervalMinutes: in.IntervalMinutes, RunAs: p.User.ID, UpdatedBy: p.User.Email, Version: in.Version}
	if in.Enabled && in.IntervalMinutes > 0 {
		next := now.Add(time.Duration(in.IntervalMinutes) * time.Minute)
		if cur != nil && cur.Enabled && cur.IntervalMinutes == in.IntervalMinutes && cur.NextRunAt != nil {
			next = *cur.NextRunAt
		}
		x.NextRunAt = &next
	}
	var secret *string
	switch {
	case in.ClearSecret:
		empty := ""
		secret = &empty
	case strings.TrimSpace(in.WebhookSecret) != "":
		secret = &in.WebhookSecret
	}
	if err := s.Store.SaveReqSync(ctx, x, secret); err != nil {
		s.fail(w, r, err)
		return
	}
	details := fmt.Sprintf("%s: enabled %t, every %d min, policy %s, impact %s, %d fields", sourceNames[sys], x.Enabled, x.IntervalMinutes,
		x.Settings.ConflictPolicy, x.Settings.Impact, len(x.Settings.Fields))
	if secret != nil {
		details += map[bool]string{true: ", webhook secret removed", false: ", webhook secret set"}[*secret == ""]
	}
	s.audit(ctx, p, "req_sync.update", "project", pr.ID, details)
	conflicts, linked, err := s.syncCounts(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	saved, err := s.Store.ReqSync(ctx, p.TenantID(), pr.ID, sys)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.syncView(ctx, p, pr, sys, saved, conflicts, linked))
}

// syncConfigFor returns the saved configuration of a source, or the
// default one (not enabled) for a first manual sync or preview.
func (s *Server) syncConfigFor(ctx context.Context, p *Principal, pr *model.Project, sys string) (*store.ReqSyncRow, error) {
	x, err := s.Store.ReqSync(ctx, p.TenantID(), pr.ID, sys)
	if errors.Is(err, store.ErrNotFound) {
		return nil, badRequest("the sync with %s is not configured yet: save its settings first", sourceNames[sys])
	}
	return x, err
}

func (s *Server) handleRunReqSync(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	sys, err := syncSystem(r.PathValue("source"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		DryRun bool `json:"dry_run"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	cfg, err := s.syncConfigFor(r.Context(), p, pr, sys)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	run, err := s.runReqSync(r.Context(), p, pr, cfg, syncOptions{trigger: "manual", dry: in.DryRun})
	if run == nil {
		s.syncFail(w, r, err)
		return
	}
	if err != nil && run.Status == "error" && len(run.Entries) == 0 && run.Summary == (model.ReqSyncSummary{}) {
		s.syncFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// syncFail answers a failed sync: tracker errors as 502.
func (s *Server) syncFail(w http.ResponseWriter, r *http.Request, err error) {
	var rl *errRateLimited
	if errors.As(err, &rl) {
		err = rl.err
	}
	s.trackerFail(w, r, err)
}

func (s *Server) handleListReqSyncRuns(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	sys := r.URL.Query().Get("source")
	if sys != "" {
		if _, err := syncSystem(sys); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.Store.ListReqSyncRuns(r.Context(), p.TenantID(), pr.ID, sys, limit, boolParam(r, "entries", false))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(runs))
}

func (s *Server) handleGetReqSyncRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	run, err := s.Store.ReqSyncRun(r.Context(), p.TenantID(), pr.ID, r.PathValue("run"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// ---------------------------------------------------------------- conflicts

func (s *Server) handleListReqSyncConflicts(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = "open"
	}
	if status == "all" {
		status = ""
	} else if !slices.Contains([]string{"open", "resolved", "dismissed"}, status) {
		s.fail(w, r, badRequest("status must be open, resolved, dismissed or all"))
		return
	}
	sys := q.Get("source")
	if sys != "" {
		if _, err := syncSystem(sys); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	list, err := s.Store.ListReqSyncConflicts(r.Context(), p.TenantID(), pr.ID, sys, status)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	src, _ := s.Store.ImportSources(r.Context(), p.TenantID(), pr.ID)
	for i := range list {
		list[i].ExternalURL = s.syncItemURL(r.Context(), p, pr.ID, src, list[i].System, list[i].ExternalID)
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// syncItemURL is the web address of a tracker item ("" unknown).
func (s *Server) syncItemURL(ctx context.Context, p *Principal, projectID string, src *model.ImportSources, system, key string) string {
	if src == nil {
		return ""
	}
	if system == tracker.SourceAzure {
		if src.Azure.URL == "" {
			return ""
		}
		return strings.TrimRight(src.Azure.URL, "/") + "/" + strings.ReplaceAll(src.Azure.Project, " ", "%20") + "/_workitems/edit/" + key
	}
	base := src.Jira.URL
	if base == "" {
		if t, err := s.Store.DefectTracker(ctx, p.TenantID(), projectID); err == nil && t.Type == "jira" {
			base = t.URL
		}
	}
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/browse/" + key
}

// handleResolveReqSyncConflict decides a conflict: per field (or for all
// fields) the tracker's value, Qtestframe's value or keeping both as they
// are (dismiss). The decision is carried out by a sync of the one item.
func (s *Server) handleResolveReqSyncConflict(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in struct {
		Resolution string            `json:"resolution"` // tracker | qtestframe | dismiss
		Fields     map[string]string `json:"fields"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	c, err := s.Store.ReqSyncConflict(ctx, p.TenantID(), pr.ID, r.PathValue("cid"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if c.Status != "open" {
		s.fail(w, r, fmt.Errorf("%w: the conflict is %s already", store.ErrConflict, c.Status))
		return
	}
	decision := func(v string) (string, error) {
		switch v {
		case model.SyncTrackerWins, model.SyncLocalWins:
			return v, nil
		case "dismiss", "keep":
			return "keep", nil
		}
		return "", badRequest("resolution must be tracker, qtestframe or dismiss")
	}
	override := map[string]string{}
	var parts []string
	for _, f := range c.Fields {
		v := in.Fields[f.Field]
		if v == "" {
			v = in.Resolution
		}
		d, err := decision(v)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		override[f.Field] = d
		label := d
		if d == "keep" {
			label = "dismissed"
		}
		parts = append(parts, f.Field+": "+label)
	}
	cfg, err := s.syncConfigFor(ctx, p, pr, c.System)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	run, err := s.runReqSync(ctx, p, pr, cfg, syncOptions{trigger: "resolve", only: map[string]bool{c.ExternalID: true}, override: override})
	if err != nil {
		s.syncFail(w, r, err)
		return
	}
	resolution := strings.Join(parts, ", ")
	status := "resolved"
	if in.Resolution == "dismiss" && len(in.Fields) == 0 {
		status = "dismissed"
	}
	if run.Summary.Errors > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"code": "sync_failed", "error": "the decision could not be carried out: see the sync log", "run": run})
		return
	}
	if _, err := s.Store.CloseReqSyncConflicts(ctx, p.TenantID(), pr.ID, c.System, c.RequirementID, status, resolution, p.User.Email); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "req_sync.conflict.resolve", model.KindRequirement, c.RequirementID, c.RequirementKey+" ↔ "+sourceNames[c.System]+" "+c.ExternalID+": "+resolution)
	out, err := s.Store.ReqSyncConflict(ctx, p.TenantID(), pr.ID, c.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflict": out, "run": run})
}

// ---------------------------------------------------------------- background

// syncPrincipal is the account a configuration runs as, scoped to the project.
func (s *Server) syncPrincipal(ctx context.Context, x *store.ReqSyncRow) (*Principal, *model.Project, error) {
	if x.RunAs == "" {
		return nil, nil, errors.New("no account to run as: save the sync settings again")
	}
	p, err := s.principalFor(ctx, x.TenantID, x.RunAs)
	if err != nil {
		return nil, nil, fmt.Errorf("the account the sync runs as: %w", err)
	}
	acc, err := s.projectAccess(ctx, p, x.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	if acc == nil {
		return nil, nil, errors.New("the account the sync runs as may no longer see the project")
	}
	p.scope(x.ProjectID, acc)
	pr, err := s.Store.GetProject(ctx, x.TenantID, x.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	return p, pr, nil
}

// runBackground runs a scheduled or webhook sync; a configuration whose
// account cannot run it is recorded as failed.
func (s *Server) runBackground(ctx context.Context, x *store.ReqSyncRow, opt syncOptions) error {
	p, pr, err := s.syncPrincipal(ctx, x)
	if err != nil {
		now := s.now()
		var next *time.Time
		if x.IntervalMinutes > 0 {
			t := now.Add(min(time.Duration(x.IntervalMinutes)*time.Minute<<min(x.Failures+1, 8), 24*time.Hour))
			next = &t
		}
		_ = s.Store.FinishReqSync(ctx, x.ID, now, "error", err.Error(), x.Failures+1, next)
		_ = s.Store.Audit(ctx, model.AuditEntry{TenantID: x.TenantID, Action: "req_sync.run", EntityType: "project", EntityID: x.ProjectID,
			Details: sourceNames[x.System] + " (" + opt.trigger + "): " + err.Error()})
		return err
	}
	_, err = s.runReqSync(ctx, p, pr, x, opt)
	return err
}

// RunReqSyncJobs runs the scheduled syncs that are due; it returns how many ran.
func (s *Server) RunReqSyncJobs(ctx context.Context) (int, error) {
	due, err := s.Store.DueReqSyncs(ctx, s.now())
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range due {
		if ctx.Err() != nil {
			break
		}
		rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		if err := s.runBackground(rctx, &due[i], syncOptions{trigger: "schedule"}); err != nil {
			s.Log.Warn("requirement sync", "project", due[i].ProjectID, "system", due[i].System, "err", err)
		} else {
			n++
		}
		cancel()
	}
	return n, nil
}

// ---------------------------------------------------------------- webhooks

// syncHookQueue coalesces the webhook events of a configuration: one sync
// at a time per configuration takes the keys that arrived meanwhile.
type syncHookQueue struct {
	mu      sync.Mutex
	pending map[string]map[string]bool // configuration → keys
	running map[string]bool
}

var syncHooks = &syncHookQueue{pending: map[string]map[string]bool{}, running: map[string]bool{}}

// syncHookLimiter bounds the webhook calls per project.
var syncHookLimiter = newRateLimiter(600, time.Minute)

// verifySyncHook checks the shared secret of a webhook: Jira sends the
// HMAC-SHA256 signature of the body (X-Hub-Signature: sha256=…) when the
// webhook has a secret (Jira Cloud), or the secret as the query parameter
// secret (Jira Server and Data Center webhooks have no secret); Azure
// DevOps service hooks send it as the password of basic authentication.
// The header X-Qtestframe-Secret is accepted from both.
func verifySyncHook(system, secret string, r *http.Request, body []byte) bool {
	if secret == "" {
		return false
	}
	eq := func(v string) bool { return v != "" && subtle.ConstantTimeCompare([]byte(v), []byte(secret)) == 1 }
	if eq(r.Header.Get("X-Qtestframe-Secret")) {
		return true
	}
	if system == tracker.SourceAzure {
		_, pw, ok := r.BasicAuth()
		return ok && eq(pw)
	}
	if sig, ok := strings.CutPrefix(r.Header.Get("X-Hub-Signature"), "sha256="); ok {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write(body)
		want := hex.EncodeToString(m.Sum(nil))
		return subtle.ConstantTimeCompare([]byte(strings.ToLower(sig)), []byte(want)) == 1
	}
	return eq(r.URL.Query().Get("secret"))
}

func (s *Server) handleReqSyncHook(system string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		body, err := io.ReadAll(io.LimitReader(r.Body, MaxHookBytes+1))
		if err != nil || len(body) > MaxHookBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "webhook body too large")
			return
		}
		pid := r.PathValue("pid")
		if !syncHookLimiter.allow(system + "|" + pid) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many webhook calls")
			return
		}
		x, err := s.Store.ReqSyncForHook(ctx, pid, system)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, err)
			return
		}
		// an unknown project, no secret and a wrong secret look the same
		if err != nil || !verifySyncHook(system, x.WebhookSecret, r, body) {
			writeError(w, http.StatusUnauthorized, "invalid_signature", "the webhook secret or signature is not valid")
			return
		}
		var event string
		var keys []string
		if system == tracker.SourceAzure {
			event, keys = tracker.AzureHookKeys(body)
		} else {
			event, keys = tracker.JiraHookKeys(body)
		}
		if !x.Enabled || len(keys) == 0 {
			writeJSON(w, http.StatusAccepted, map[string]any{"ignored": true, "event": event})
			return
		}
		s.queueSyncHook(x, keys)
		writeJSON(w, http.StatusAccepted, map[string]any{"event": event, "items": keys})
	}
}

// queueSyncHook adds the keys of a webhook to the configuration's queue
// and starts its worker unless one is running.
func (s *Server) queueSyncHook(x *store.ReqSyncRow, keys []string) {
	q := syncHooks
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending[x.ID] == nil {
		q.pending[x.ID] = map[string]bool{}
	}
	for _, k := range keys {
		q.pending[x.ID][k] = true
	}
	if q.running[x.ID] {
		return
	}
	q.running[x.ID] = true
	s.hooks.Add(1)
	go func() {
		defer s.hooks.Done()
		for {
			q.mu.Lock()
			only := q.pending[x.ID]
			delete(q.pending, x.ID)
			if len(only) == 0 {
				delete(q.running, x.ID)
				q.mu.Unlock()
				return
			}
			q.mu.Unlock()
			s.runHookSync(x.ID, x.ProjectID, x.System, only)
		}
	}()
}

// syncHookWait is how long a webhook sync waits for a running sync.
var syncHookWait = 2 * time.Minute

func (s *Server) runHookSync(id, projectID, system string, only map[string]bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	deadline := time.Now().Add(syncHookWait)
	for {
		x, err := s.Store.ReqSyncForHook(ctx, projectID, system)
		if err != nil || x.ID != id || !x.Enabled {
			return
		}
		err = s.runBackground(ctx, x, syncOptions{trigger: "webhook", only: only})
		if err == nil || !errors.Is(err, store.ErrConflict) || !strings.Contains(err.Error(), "is running") || time.Now().After(deadline) {
			if err != nil {
				s.Log.Warn("requirement sync webhook", "project", projectID, "system", system, "err", err)
			}
			return
		}
		if syncSleep(ctx, 2*time.Second) != nil {
			return
		}
	}
}
