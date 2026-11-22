package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Electronic-records mode (21 CFR Part 11 / EU GMP Annex 11), an
// organisation setting that is off by default. When it is on:
//
//   - every electronic signature (baseline sign-offs, waiver decisions,
//     risk acceptance, sign-offs of verification records and the actions
//     of ERecordsSettings.SignatureActions) needs the signer's password and,
//     when enrolled, a second-factor code, and records a meaning from the
//     organisation's list, the signer's time zone and the hash of the record
//     version signed (table esignatures);
//   - the actions of SignatureActions are refused (428 signature_required)
//     unless the request carries a signing ticket (X-Signature-Ticket) from
//     POST /esign/tickets, on every route that performs them;
//   - the edits of ReasonEdits are refused (428 reason_required) without a
//     reason for change (X-Change-Reason, URL-encoded), which the audit
//     entries of the request record;
//   - browser sessions end after IdleTimeoutMinutes without activity.
//
// The web client answers both 428 codes with a dialog and repeats the
// request (web/src/components/ERecordsHost.vue).

const (
	headerSignatureTicket = "X-Signature-Ticket"
	headerChangeReason    = "X-Change-Reason"
	// headerBackground marks requests the client sends on its own (polling):
	// they do not count as activity of the session.
	headerBackground = "X-Qtf-Background"
	// signing tickets are valid this long
	ticketTTL = 5 * time.Minute
)

func (s *Server) routesERecords(h routeFunc) {
	h("GET /api/v1/erecords", s.handleGetERecords) // every member: the client adapts its dialogs
	h("PUT /api/v1/erecords", s.handlePutERecords, "tenant.manage")
	h("GET /api/v1/erecords/report", s.handleERecordsReport, "audit.read")
	h("POST /api/v1/esign/tickets", s.handleSigningTicket)
	h("GET /api/v1/esignatures", s.handleListESignatures) // read access to the records: see handleListESignatures
	h("POST /api/v1/audit/verify", s.handleVerifyAudit, "audit.read")
	h("GET /api/v1/audit/archive", s.handleAuditArchive, "audit.read")
}

// ---------------------------------------------------------------- settings cache

type erecCache struct {
	mu sync.Mutex
	m  map[string]erecCached
}

type erecCached struct {
	st *model.ERecordsSettings
	at time.Time
}

// erecords returns the electronic-records settings of a tenant (cached for
// a few seconds; nil tenant: off).
func (s *Server) erecords(ctx context.Context, tenantID string) *model.ERecordsSettings {
	off := &model.ERecordsSettings{}
	if tenantID == "" {
		return off
	}
	s.erec.mu.Lock()
	c, ok := s.erec.m[tenantID]
	s.erec.mu.Unlock()
	if ok && time.Since(c.at) < 5*time.Second {
		return c.st
	}
	st, err := s.Store.ERecordsSettings(ctx, tenantID)
	if err != nil {
		s.Log.Warn("electronic-records settings", "tenant", tenantID, "err", err)
		if ok {
			return c.st
		}
		return off
	}
	s.erec.mu.Lock()
	if s.erec.m == nil {
		s.erec.m = map[string]erecCached{}
	}
	s.erec.m[tenantID] = erecCached{st: st, at: time.Now()}
	s.erec.mu.Unlock()
	return st
}

func (s *Server) forgetERecords(tenantID string) {
	s.erec.mu.Lock()
	delete(s.erec.m, tenantID)
	s.erec.mu.Unlock()
}

func (s *Server) handleGetERecords(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	st, err := s.Store.ERecordsSettings(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": st, "signature_actions": model.ERecordsSignatureActions, "reason_edits": model.ERecordsReasonEdits})
}

func (s *Server) handlePutERecords(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	var st model.ERecordsSettings
	if err := decode(r, &st); err != nil {
		s.fail(w, r, err)
		return
	}
	before := s.erecords(r.Context(), p.TenantID())
	if err := s.Store.SaveERecordsSettings(r.Context(), p.TenantID(), &st, p.User.Email); err != nil {
		s.fail(w, r, err)
		return
	}
	s.forgetERecords(p.TenantID())
	action := "erecords.update"
	if st.Enabled != before.Enabled {
		action = map[bool]string{true: "erecords.enable", false: "erecords.disable"}[st.Enabled]
	}
	s.audit(r.Context(), p, action, "tenant", p.TenantID(), erecordsSummary(&st))
	writeJSON(w, http.StatusOK, st)
}

func erecordsSummary(st *model.ERecordsSettings) string {
	b, _ := json.Marshal(map[string]any{"enabled": st.Enabled, "meanings": st.Meanings, "signature_actions": st.SignatureActions,
		"reason_edits": st.ReasonEdits, "idle_timeout_minutes": st.IdleTimeoutMinutes, "sso_reauth_minutes": st.SSOReauthMinutes})
	return string(b)
}

// ---------------------------------------------------------------- re-authentication

// signerCreds are what a signer gives at signing time.
type signerCreds struct {
	Password string `json:"password"`
	MFACode  string `json:"mfa_code"`
	Meaning  string `json:"meaning"`
	// TimeZone is the signer's IANA time zone (the browser's).
	TimeZone string `json:"time_zone"`
}

// signerProof is the outcome of a re-authentication.
type signerProof struct {
	Method   string
	Meaning  string
	TimeZone string
}

func cleanTimeZone(tz string) string {
	tz = strings.TrimSpace(tz)
	if tz == "" || len(tz) > 64 {
		return "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return "UTC"
	}
	return tz
}

// confirmSigner re-authenticates the signer of an electronic signature. It
// always needs a browser session. Outside electronic-records mode: the
// password, or for single sign-on accounts a sign-in less than 15 minutes
// old (meanings is then the list the meaning must be in, nil: any). In
// electronic-records mode: the password and, when enrolled, a current
// second-factor code (single sign-on accounts: a sign-in within the
// configured window, and the code when enrolled here), and a meaning from
// the organisation's list. It writes the error and returns false otherwise.
//
// In electronic-records mode a request may instead carry a signing ticket
// for action (X-Signature-Ticket); a request without credentials answers
// 428 signature_required, so the client asks for them in its own dialog.
func (s *Server) confirmSigner(w http.ResponseWriter, r *http.Request, p *Principal, c signerCreds, meanings []string, action string) (*signerProof, bool) {
	if p.Token != nil || p.Session == nil {
		writeError(w, http.StatusForbidden, "forbidden", "electronic signatures are given in the browser, not with an API token")
		return nil, false
	}
	ctx := r.Context()
	st := s.erecords(ctx, p.TenantID())
	proof := &signerProof{Meaning: strings.TrimSpace(c.Meaning), TimeZone: cleanTimeZone(c.TimeZone)}
	if st.Enabled && action != "" {
		if tk := strings.TrimSpace(r.Header.Get(headerSignatureTicket)); tk != "" {
			t, ok := s.takeTicket(ctx, p, tk, action)
			if !ok {
				signatureRequired(w, action, st, "the signature is not valid for this action (expired, used or of someone else): sign again")
				return nil, false
			}
			return &signerProof{Method: t.Method, Meaning: t.Meaning, TimeZone: t.TimeZone}, true
		}
		if c.Password == "" && c.MFACode == "" && proof.Meaning == "" {
			signatureRequired(w, action, st, "this action needs your electronic signature (electronic-records mode)")
			return nil, false
		}
	}
	if st.Enabled {
		meanings = st.Meanings
		if proof.Meaning == "" {
			writeError(w, http.StatusBadRequest, "meaning_required", "choose the meaning of your signature")
			return nil, false
		}
	}
	if meanings != nil && proof.Meaning != "" && !slices.Contains(meanings, proof.Meaning) {
		writeError(w, http.StatusBadRequest, "bad_request", "meaning must be one of "+strings.Join(meanings, ", "))
		return nil, false
	}
	limiterKey := "sign:" + p.User.ID
	if !s.mfaLimiter.allow(limiterKey) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many signing attempts, try again later")
		return nil, false
	}
	window := 15 * time.Minute
	if st.Enabled {
		window = time.Duration(st.SSOReauthMinutes) * time.Minute
	}
	if p.User.PasswordHash != "" {
		if !auth.CheckPassword(p.User.PasswordHash, c.Password) {
			writeError(w, http.StatusForbidden, "reauth_failed", "the password is not correct")
			return nil, false
		}
		proof.Method = "password"
	} else {
		if time.Since(p.Session.CreatedAt) > window {
			writeError(w, http.StatusForbidden, "reauth_required", "sign in again (less than "+itoa(int(window/time.Minute))+" minutes ago) to sign")
			return nil, false
		}
		proof.Method = "sso"
	}
	if st.Enabled {
		m, err := s.userMFA(ctx, p.User.ID)
		if err != nil {
			s.fail(w, r, err)
			return nil, false
		}
		if m != nil && m.EnabledAt != nil {
			if strings.TrimSpace(c.MFACode) == "" {
				writeError(w, http.StatusForbidden, "mfa_required", "enter a code of your second factor to sign")
				return nil, false
			}
			ok, recovery, err := s.checkSecondFactor(ctx, p.User.ID, strings.TrimSpace(c.MFACode))
			if err != nil {
				s.fail(w, r, err)
				return nil, false
			}
			if !ok {
				writeError(w, http.StatusForbidden, "mfa_invalid", "the code is wrong or was already used")
				return nil, false
			}
			if recovery {
				s.audit(ctx, p, "mfa.recovery_code_used", "user", p.User.ID, p.User.Email)
			}
			proof.Method += "+mfa"
		}
	}
	s.mfaLimiter.reset(limiterKey)
	return proof, true
}

// recordSignature stores the manifestation of a signature (also outside
// electronic-records mode) and audits it.
func (s *Server) recordSignature(ctx context.Context, p *Principal, action, kind, id string, proof *signerProof, comment string) {
	h, err := s.recordHash(ctx, p.TenantID(), kind, id)
	if err != nil {
		s.Log.Warn("signature: record hash", "kind", kind, "id", id, "err", err)
		return
	}
	e := &model.ESignature{ProjectID: h.projectID, EntityType: kind, EntityID: id, EntityKey: h.key, Action: action, Meaning: proof.Meaning,
		Comment: strings.TrimSpace(comment), UserID: p.User.ID, UserEmail: p.User.Email, UserName: signerName(p), Method: proof.Method,
		TimeZone: proof.TimeZone, ContentHash: h.hash}
	if err := s.Store.AddESignature(ctx, p.TenantID(), e); err != nil {
		s.Log.Warn("signature: record", "kind", kind, "id", id, "err", err)
		return
	}
	s.audit(ctx, p, "esignature."+action, kind, id, strings.TrimSpace(h.key+" "+proof.Meaning+" "+proof.Method+" "+short(h.hash)))
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// handleSigningTicket: POST /esign/tickets {"action", "password",
// "mfa_code", "meaning", "comment", "time_zone"} re-authenticates the
// caller for one action of ERecordsSignatureActions and answers a
// single-use ticket for the request that performs it.
func (s *Server) handleSigningTicket(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	var req struct {
		signerCreds
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !slices.Contains(signableActions, req.Action) {
		s.fail(w, r, badRequest("action must be one of %s", strings.Join(signableActions, ", ")))
		return
	}
	if len([]rune(req.Comment)) > 2000 {
		s.fail(w, r, badRequest("the comment is at most 2000 characters"))
		return
	}
	st := s.erecords(r.Context(), p.TenantID())
	if !st.Enabled {
		s.fail(w, r, badRequest("electronic-records mode is off: this action needs no signature"))
		return
	}
	proof, ok := s.confirmSigner(w, r, p, req.signerCreds, nil, "")
	if !ok {
		return
	}
	tok := auth.RandomToken(32)
	t := &store.SigningTicket{TenantID: p.TenantID(), UserID: p.User.ID, SessionID: p.Session.ID, Action: req.Action, Meaning: proof.Meaning,
		Comment: strings.TrimSpace(req.Comment), Method: proof.Method, TimeZone: proof.TimeZone, ExpiresAt: s.Store.Now().Add(ticketTTL)}
	if err := s.Store.AddSigningTicket(r.Context(), auth.HashToken(tok), t); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ticket": tok, "action": t.Action, "meaning": t.Meaning, "expires_at": t.ExpiresAt})
}

// signatureRequired answers 428: the action needs a signature (ticket).
func signatureRequired(w http.ResponseWriter, action string, st *model.ERecordsSettings, msg string) {
	writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": msg, "code": "signature_required", "action": action, "meanings": st.Meanings})
}

// takeTicket uses up a signing ticket of the caller's session for action.
func (s *Server) takeTicket(ctx context.Context, p *Principal, ticket, action string) (*store.SigningTicket, bool) {
	t, err := s.Store.TakeSigningTicket(ctx, auth.HashToken(ticket))
	if err != nil || p.Session == nil || t.UserID != p.User.ID || t.TenantID != p.TenantID() || t.SessionID != p.Session.ID || t.Action != action {
		return nil, false
	}
	return t, true
}

// signableActions are the actions a signing ticket can be for.
var signableActions = append(append([]string{}, model.ERecordsSignatureActions...),
	model.SignBaselineSignoff, model.SignWaiverDecision, model.SignRiskAccept, model.SignVerificationSignoff)

// ---------------------------------------------------------------- record hashes

type recordHashInfo struct {
	hash, projectID, key, status string
}

func hashJSON(kind string, v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(append([]byte(kind+"|"), b...))
	return hex.EncodeToString(sum[:])
}

// recordHash returns the hash of the signed content of a record: its
// substance, not derived values or its status (a requirement that becomes
// verified keeps its approval signature; one whose text changes does not).
func (s *Server) recordHash(ctx context.Context, tenantID, kind, id string) (*recordHashInfo, error) {
	switch kind {
	case model.KindRequirement:
		q, err := s.Store.GetRequirement(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		crit := make([]string, 0, len(q.Criteria))
		for _, c := range q.Criteria {
			crit = append(crit, c.Description+"|"+c.Method+"|"+c.Threshold)
		}
		return &recordHashInfo{projectID: q.ProjectID, key: q.Key, status: q.Status, hash: hashJSON(kind, []any{q.Key, q.Title, q.Description, q.Obligation, q.Type,
			q.Priority, q.Source, q.Rationale, crit})}, nil
	case model.KindTest:
		t, err := s.Store.GetTest(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		steps := make([][]string, 0, len(t.Steps))
		for _, st := range t.Steps {
			steps = append(steps, []string{st.Action, st.Expected, st.Data})
		}
		return &recordHashInfo{projectID: t.ProjectID, key: t.Key, status: t.Status, hash: hashJSON(kind, []any{t.Key, t.Title, t.Description, t.Preconditions,
			t.ExecutionType, t.ScriptLanguage, t.Script, t.ExpectedResult, steps, t.Parameters, t.DataRows})}, nil
	case "release":
		rl, err := s.Store.GetRelease(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		return &recordHashInfo{projectID: rl.ProjectID, key: rl.Name, status: rl.Status, hash: hashJSON(kind, []any{rl.Name, rl.Version, rl.Description,
			rl.TargetDate, rl.ExitCriteria, rl.VariantIDs, rl.ReleaseNote})}, nil
	case "baseline":
		b, err := s.Store.GetBaseline(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		return &recordHashInfo{projectID: b.ProjectID, key: b.Name, hash: store.BaselineHash(b)}, nil
	case model.KindWaiver:
		wv, err := s.Store.GetWaiver(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		return &recordHashInfo{projectID: wv.ProjectID, key: wv.Key, status: wv.Status, hash: store.WaiverHash(wv)}, nil
	case "risk":
		k, err := s.Store.GetRisk(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		return &recordHashInfo{projectID: k.ProjectID, key: k.Key, hash: store.RiskAcceptanceHash(k)}, nil
	case "verification_record":
		v, err := s.Store.GetRecord(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		return &recordHashInfo{projectID: v.ProjectID, key: v.RequirementKey + " " + v.Method, hash: hashJSON(kind, []any{v.RequirementID, v.CriterionID,
			v.Method, v.Result, v.PerformedBy, v.PerformedOn, v.Note, v.FileIDs, v.ValidUntil, v.UntilChange})}, nil
	}
	return nil, store.Invalidf("unknown record type %q", kind)
}

// signedReadPerm is the permission that reads a kind of signed record.
func signedReadPerm(kind string) rbac.Permission {
	switch kind {
	case model.KindTest:
		return rbac.TestRead
	case "release":
		return rbac.ProjectRead
	}
	return rbac.RequirementRd
}

// handleListESignatures: GET /esignatures?entity_type=&entity_id= lists
// the signatures of a record (read permission on it), or with project_id
// or nothing those of a project or the organisation (audit.read). Each
// says whether the record still has the content that was signed.
func (s *Server) handleListESignatures(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	ctx := r.Context()
	qv := r.URL.Query()
	f := store.ESignatureFilter{EntityType: qv.Get("entity_type"), EntityID: qv.Get("entity_id"), ProjectID: qv.Get("project_id")}
	if f.EntityID != "" {
		h, err := s.recordHash(ctx, p.TenantID(), f.EntityType, f.EntityID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.canIn(ctx, p, h.projectID, signedReadPerm(f.EntityType)); err != nil {
			s.fail(w, r, err)
			return
		}
	} else if !p.Can(rbac.AuditRead) {
		writeError(w, http.StatusForbidden, "forbidden", "permission audit.read required")
		return
	}
	list, err := s.Store.ListESignatures(ctx, p.TenantID(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cur := map[string]string{}
	for i := range list {
		e := &list[i]
		k := e.EntityType + "|" + e.EntityID
		h, ok := cur[k]
		if !ok {
			if info, err := s.recordHash(ctx, p.TenantID(), e.EntityType, e.EntityID); err == nil {
				h = info.hash
			}
			cur[k] = h
		}
		e.CurrentHash, e.Valid = h, h != "" && h == e.ContentHash
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// ---------------------------------------------------------------- enforcement

// reasonRoutes maps the edit types of ERecordsReasonEdits to the routes
// that perform them.
var reasonRoutes = map[string]string{
	"PATCH /api/v1/requirements/{id}":                     "requirement.update",
	"POST /api/v1/projects/{pid}/requirements/bulk":       "requirement.update",
	"POST /api/v1/projects/{pid}/requirements/batch":      "requirement.update",
	"POST /api/v1/projects/{pid}/requirements/review":     "requirement.update",
	"POST /api/v1/requirements/{id}/restore":              "requirement.update",
	"DELETE /api/v1/requirements/{id}":                    "requirement.delete",
	"DELETE /api/v1/projects/{pid}/requirements/bulk":     "requirement.delete",
	"PATCH /api/v1/tests/{id}":                            "test.update",
	"POST /api/v1/projects/{pid}/tests/bulk":              "test.update",
	"POST /api/v1/projects/{pid}/tests/batch":             "test.update",
	"POST /api/v1/projects/{pid}/tests/review":            "test.update",
	"POST /api/v1/tests/{id}/restore":                     "test.update",
	"DELETE /api/v1/tests/{id}":                           "test.delete",
	"DELETE /api/v1/projects/{pid}/tests/bulk":            "test.delete",
	"PATCH /api/v1/runs/{id}":                             "run.update",
	"DELETE /api/v1/runs/{id}":                            "run.delete",
	"DELETE /api/v1/projects/{pid}/runs/bulk":             "run.delete",
	"PATCH /api/v1/executions/{id}":                       "execution.update",
	"POST /api/v1/runs/{id}/executions/bulk":              "execution.update",
	"PATCH /api/v1/releases/{id}":                         "release.update",
	"POST /api/v1/projects/{pid}/releases/bulk":           "release.update",
	"DELETE /api/v1/releases/{id}":                        "release.delete",
	"DELETE /api/v1/projects/{pid}/releases/bulk":         "release.delete",
	"DELETE /api/v1/baselines/{id}":                       "baseline.delete",
	"DELETE /api/v1/projects/{pid}/baselines/bulk":        "baseline.delete",
	"PATCH /api/v1/risks/{id}":                            "risk.update",
	"POST /api/v1/projects/{pid}/risks/bulk":              "risk.update",
	"DELETE /api/v1/risks/{id}":                           "risk.delete",
	"DELETE /api/v1/projects/{pid}/risks/bulk":            "risk.delete",
	"PATCH /api/v1/waivers/{id}":                          "waiver.update",
	"PATCH /api/v1/verification-records/{id}":             "verification.update",
	"DELETE /api/v1/verification-records/{id}":            "verification.delete",
	"POST /api/v1/requirements/{id}/verification-records": "verification.update",
}

// ReasonRoutes returns the routes of an edit type (documentation, tests).
func ReasonRoutes(editType string) []string {
	var out []string
	for pat, t := range reasonRoutes {
		if t == editType {
			out = append(out, pat)
		}
	}
	slices.Sort(out)
	return out
}

// signNeed is what a request needs signed: the action, the kind of record
// and the records known before the request (more may come from the
// answer), with the status they must have afterwards to be signed.
type signNeed struct {
	action, kind, target string
	ids                  []string
	// fromAnswer extracts records from the successful answer
	fromAnswer func(body []byte) []string
}

func idsOfAnswer(field string) func([]byte) []string {
	return func(b []byte) []string {
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) != nil {
			return nil
		}
		if field == "" {
			var id string
			_ = json.Unmarshal(m["id"], &id)
			if id == "" {
				return nil
			}
			return []string{id}
		}
		var items []struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(m[field], &items)
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, it.ID)
		}
		return out
	}
}

// signatureNeed tells whether a request performs an action that needs a
// signature under st (nil: none). body is the request body.
func (s *Server) signatureNeed(r *http.Request, p *Principal, st *model.ERecordsSettings, body []byte) *signNeed {
	on := func(a string) bool { return slices.Contains(st.SignatureActions, a) }
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(body, &obj)
	str := func(m map[string]json.RawMessage, k string) string {
		var v string
		_ = json.Unmarshal(m[k], &v)
		return v
	}
	// only records whose status is not the target yet
	notYet := func(kind, target string, ids []string) []string {
		var out []string
		for _, id := range uniqStrings(ids) {
			if h, err := s.recordHash(r.Context(), p.TenantID(), kind, id); err == nil && h.status != target {
				out = append(out, id)
			}
		}
		return out
	}
	itemRule := func(kind, action, target string) *signNeed {
		if !on(action) {
			return nil
		}
		need := &signNeed{action: action, kind: kind, target: target}
		switch {
		case r.Method == http.MethodPatch:
			if str(obj, "status") == target {
				need.ids = notYet(kind, target, []string{r.PathValue("id")})
			}
		case strings.HasSuffix(r.Pattern, "/bulk"):
			var set map[string]json.RawMessage
			_ = json.Unmarshal(obj["set"], &set)
			if str(set, "status") == target {
				var ids []string
				_ = json.Unmarshal(obj["ids"], &ids)
				need.ids = notYet(kind, target, ids)
			}
		case strings.HasSuffix(r.Pattern, "/batch"):
			var items []batchItem
			_ = json.Unmarshal(obj["items"], &items)
			var ids []string
			for _, it := range items {
				var set map[string]json.RawMessage
				_ = json.Unmarshal(it.Set, &set)
				if str(set, "status") == target {
					ids = append(ids, it.ID)
				}
			}
			need.ids = notYet(kind, target, ids)
		case strings.HasSuffix(r.Pattern, "/review"):
			var dry bool
			_ = json.Unmarshal(obj["dry_run"], &dry)
			if str(obj, "decision") == target && !dry {
				need.fromAnswer = idsOfAnswer("updated")
				return need
			}
		}
		if len(need.ids) == 0 {
			return nil
		}
		return need
	}
	switch r.Pattern {
	case "PATCH /api/v1/requirements/{id}", "POST /api/v1/projects/{pid}/requirements/bulk", "POST /api/v1/projects/{pid}/requirements/batch",
		"POST /api/v1/projects/{pid}/requirements/review":
		return itemRule(model.KindRequirement, model.SignRequirementApprove, "approved")
	case "PATCH /api/v1/tests/{id}", "POST /api/v1/projects/{pid}/tests/bulk", "POST /api/v1/projects/{pid}/tests/batch", "POST /api/v1/projects/{pid}/tests/review":
		return itemRule(model.KindTest, model.SignTestApprove, model.ReadyTestStatus)
	case "PATCH /api/v1/releases/{id}":
		if on(model.SignReleaseGo) && str(obj, "status") == "released" {
			if ids := notYet("release", "released", []string{r.PathValue("id")}); len(ids) > 0 {
				return &signNeed{action: model.SignReleaseGo, kind: "release", target: "released", ids: ids}
			}
		}
	case "POST /api/v1/projects/{pid}/releases/bulk":
		var set map[string]json.RawMessage
		_ = json.Unmarshal(obj["set"], &set)
		if on(model.SignReleaseGo) && str(set, "status") == "released" {
			// the releases (chosen by ids or a query) not released yet
			var ids []string
			if list, err := s.Store.ListReleases(r.Context(), p.TenantID(), r.PathValue("pid")); err == nil {
				for _, rl := range list {
					if rl.Status != "released" {
						ids = append(ids, rl.ID)
					}
				}
			}
			if len(ids) > 0 {
				return &signNeed{action: model.SignReleaseGo, kind: "release", target: "released", ids: ids}
			}
		}
	case "POST /api/v1/projects/{pid}/baselines":
		if on(model.SignBaselineCreate) {
			return &signNeed{action: model.SignBaselineCreate, kind: "baseline", fromAnswer: idsOfAnswer("")}
		}
	}
	return nil
}

// capture keeps the status and (the first MiB of) the body of an answer
// while passing it on.
type capture struct {
	http.ResponseWriter
	code int
	body bytes.Buffer
}

func (c *capture) WriteHeader(code int) {
	if c.code == 0 {
		c.code = code
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *capture) Write(b []byte) (int, error) {
	if c.code == 0 {
		c.code = http.StatusOK
	}
	if c.body.Len() < 1<<20 {
		c.body.Write(b)
	}
	return c.ResponseWriter.Write(b)
}

func (c *capture) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// serveERecords runs a handler under the electronic-records controls of
// the caller's organisation: the reason for change and the signatures the
// request needs.
func (s *Server) serveERecords(fn handlerFunc, w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Tenant == nil || r.Method == http.MethodGet || r.Method == http.MethodHead {
		fn(w, r, p)
		return
	}
	st := s.erecords(r.Context(), p.TenantID())
	if !st.Enabled {
		fn(w, r, p)
		return
	}
	if edit := reasonRoutes[r.Pattern]; edit != "" && slices.Contains(st.ReasonEdits, edit) {
		reason, _ := url.PathUnescape(r.Header.Get(headerChangeReason))
		reason = strings.TrimSpace(reason)
		if len([]rune(reason)) < 3 {
			writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "give a reason for this change (electronic-records mode)", "code": "reason_required", "edit_type": edit})
			return
		}
		if len([]rune(reason)) > 1000 {
			s.fail(w, r, badRequest("the reason for change is at most 1000 characters"))
			return
		}
		r = r.WithContext(store.WithChangeReason(r.Context(), reason))
	}
	if !s.signedRoute(r.Pattern) {
		fn(w, r, p)
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	need := s.signatureNeed(r, p, st, body)
	if need == nil {
		fn(w, r, p)
		return
	}
	ticket := strings.TrimSpace(r.Header.Get(headerSignatureTicket))
	if ticket == "" {
		signatureRequired(w, need.action, st, "this action needs your electronic signature (electronic-records mode)")
		return
	}
	t, ok := s.takeTicket(r.Context(), p, ticket, need.action)
	if !ok {
		signatureRequired(w, need.action, st, "the signature is not valid for this action (expired, used or of someone else): sign again")
		return
	}
	cw := &capture{ResponseWriter: w}
	fn(cw, r, p)
	if cw.code >= 400 {
		return
	}
	ids := need.ids
	if need.fromAnswer != nil {
		ids = append(ids, need.fromAnswer(cw.body.Bytes())...)
	}
	proof := &signerProof{Method: t.Method, Meaning: t.Meaning, TimeZone: t.TimeZone}
	ctx := store.WithChangeReason(context.WithoutCancel(r.Context()), store.ChangeReason(r.Context()))
	for _, id := range uniqStrings(ids) {
		if need.target != "" {
			if h, err := s.recordHash(ctx, p.TenantID(), need.kind, id); err != nil || h.status != need.target {
				continue // skipped by the request
			}
		}
		s.recordSignature(ctx, p, need.action, need.kind, id, proof, t.Comment)
	}
}

// signedRoute reports whether a route can perform an action of
// ERecordsSignatureActions.
func (s *Server) signedRoute(pattern string) bool {
	switch pattern {
	case "PATCH /api/v1/requirements/{id}", "POST /api/v1/projects/{pid}/requirements/bulk", "POST /api/v1/projects/{pid}/requirements/batch",
		"POST /api/v1/projects/{pid}/requirements/review",
		"PATCH /api/v1/tests/{id}", "POST /api/v1/projects/{pid}/tests/bulk", "POST /api/v1/projects/{pid}/tests/batch", "POST /api/v1/projects/{pid}/tests/review",
		"PATCH /api/v1/releases/{id}", "POST /api/v1/projects/{pid}/releases/bulk", "POST /api/v1/projects/{pid}/baselines":
		return true
	}
	return false
}

// idleExpired reports whether a session of a tenant in electronic-records
// mode has been inactive for longer than allowed.
func (s *Server) idleExpired(ctx context.Context, sess *model.Session) bool {
	if sess.TenantID == "" || sess.LastSeen == nil {
		return false
	}
	st := s.erecords(ctx, sess.TenantID)
	return st.Enabled && s.Store.Now().Sub(*sess.LastSeen) > time.Duration(st.IdleTimeoutMinutes)*time.Minute
}

// touchInterval is how often the activity of a session is recorded: every
// minute, more often under an inactivity timeout.
func (s *Server) touchInterval(ctx context.Context, sess *model.Session) time.Duration {
	if sess.TenantID != "" {
		if st := s.erecords(ctx, sess.TenantID); st.Enabled {
			return min(time.Minute, time.Duration(st.IdleTimeoutMinutes)*time.Minute/6)
		}
	}
	return time.Minute
}

var errIdle = errors.New("session ended after inactivity")
