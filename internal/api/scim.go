package api

// SCIM 2.0 service provider (RFC 7643 and RFC 7644) at /scim/v2, used by
// identity providers (Entra ID, Okta, OneLogin, JumpCloud …) to provision
// users and groups into one tenant.
//
// Model:
//   - A SCIM token (scim_tokens, prefix qtfscim_) is bound to one tenant;
//     it works only here, and API tokens and sessions do not work here.
//   - A provisioned user is a user row (matched by e-mail, created with
//     auth_provider "scim" and no password, so the user signs in through the
//     tenant's OIDC/SAML provider, which links the account by e-mail) plus a
//     scim_users row for the tenant. While active the user has a tenant
//     membership; active=false removes it and ends the user's sessions and
//     API tokens in the tenant; active=true restores it.
//   - Groups map to roles: tenant admins give each pushed group a role (or
//     none). An active user holds the roles of all their mapped groups
//     (several groups give several roles), else the tenant's default role
//     ("" = no access). The read-only "roles" attribute of a user lists
//     them; roles pushed in it are ignored (map groups instead).
//   - SCIM never changes super-admin status and never leaves a tenant
//     without an admin; the user's global profile (e-mail) is only changed
//     when the user belongs to this tenant alone.
//
// Every change is atomic and written to the audit log with the actor
// "SCIM: <token name>".

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// SCIM schema URNs.
const (
	scimUserSchema         = "urn:ietf:params:scim:schemas:core:2.0:User"
	scimGroupSchema        = "urn:ietf:params:scim:schemas:core:2.0:Group"
	scimListSchema         = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	scimErrorSchema        = "urn:ietf:params:scim:api:messages:2.0:Error"
	scimPatchSchema        = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	scimSPConfigSchema     = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	scimResourceTypeSchema = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	scimSchemaSchema       = "urn:ietf:params:scim:schemas:core:2.0:Schema"
	scimContentType        = "application/scim+json"
	// SCIMTokenPrefix starts every SCIM token (API tokens start with qtf_).
	SCIMTokenPrefix = "qtfscim_"
	// scimMaxResults bounds one page of a list.
	scimMaxResults = 1000
	// scimAuthProvider marks users created by SCIM until their first
	// single sign-on links them to the identity provider.
	scimAuthProvider = "scim"
)

// scimCaller is an authenticated identity provider.
type scimCaller struct {
	Token  *model.SCIMToken
	Tenant *model.Tenant
	base   string // .../scim/v2
}

// scimError is a SCIM error response (RFC 7644 §3.12).
type scimError struct {
	Status   int
	ScimType string
	Detail   string
}

func (e *scimError) Error() string { return e.Detail }

func scimErr(status int, scimType, format string, a ...any) *scimError {
	return &scimError{Status: status, ScimType: scimType, Detail: fmt.Sprintf(format, a...)}
}

var errLastAdmin = errors.New("the organisation needs at least one admin; give another member the admin role first")

func writeSCIM(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", scimContentType)
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeSCIMError(w http.ResponseWriter, e *scimError) {
	body := map[string]any{"schemas": []string{scimErrorSchema}, "status": strconv.Itoa(e.Status), "detail": e.Detail}
	if e.ScimType != "" {
		body["scimType"] = e.ScimType
	}
	writeSCIM(w, e.Status, body)
}

// toSCIMError maps an error to a SCIM error.
func (s *Server) toSCIMError(r *http.Request, err error) *scimError {
	var se *scimError
	switch {
	case errors.As(err, &se):
		return se
	case errors.Is(err, errLastAdmin):
		return scimErr(http.StatusConflict, "", "%s", err.Error())
	case errors.Is(err, errFilter):
		return scimErr(http.StatusBadRequest, "invalidFilter", "%s", err.Error())
	case errors.Is(err, store.ErrNotFound):
		return scimErr(http.StatusNotFound, "", "%s", err.Error())
	case errors.Is(err, store.ErrConflict):
		return scimErr(http.StatusConflict, "uniqueness", "%s", err.Error())
	case errors.Is(err, store.ErrInvalid):
		return scimErr(http.StatusBadRequest, "invalidValue", "%s", err.Error())
	case errors.Is(err, errBadRequest):
		return scimErr(http.StatusBadRequest, "invalidSyntax", "%s", err.Error())
	}
	s.Log.Error("scim request failed", "path", r.URL.Path, "err", err)
	return scimErr(http.StatusInternalServerError, "", "internal server error")
}

// scimResult is what a SCIM handler answers.
type scimResult struct {
	status   int
	body     any
	location string
	version  string
}

type scimHandler func(r *http.Request, c *scimCaller) (*scimResult, error)

func (s *Server) routesSCIM(m *http.ServeMux) {
	h := func(pattern string, fn scimHandler) { m.Handle(pattern, s.scimAuthed(fn)) }
	h("GET /scim/v2/ServiceProviderConfig", s.scimServiceProviderConfig)
	h("GET /scim/v2/ResourceTypes", s.scimResourceTypes)
	h("GET /scim/v2/ResourceTypes/{id}", s.scimResourceType)
	h("GET /scim/v2/Schemas", s.scimSchemas)
	h("GET /scim/v2/Schemas/{id}", s.scimSchema)
	h("GET /scim/v2/Users", s.scimListUsers)
	h("POST /scim/v2/Users", s.scimCreateUser)
	h("GET /scim/v2/Users/{id}", s.scimGetUser)
	h("PUT /scim/v2/Users/{id}", s.scimReplaceUser)
	h("PATCH /scim/v2/Users/{id}", s.scimPatchUser)
	h("DELETE /scim/v2/Users/{id}", s.scimDeleteUser)
	h("GET /scim/v2/Groups", s.scimListGroups)
	h("POST /scim/v2/Groups", s.scimCreateGroup)
	h("GET /scim/v2/Groups/{id}", s.scimGetGroup)
	h("PUT /scim/v2/Groups/{id}", s.scimReplaceGroup)
	h("PATCH /scim/v2/Groups/{id}", s.scimPatchGroup)
	h("DELETE /scim/v2/Groups/{id}", s.scimDeleteGroup)
	h("/scim/v2/", func(r *http.Request, c *scimCaller) (*scimResult, error) {
		switch strings.TrimPrefix(r.URL.Path, "/scim/v2/") {
		case "Me", "Bulk", ".search", "Users/.search", "Groups/.search":
			return nil, scimErr(http.StatusNotImplemented, "", "%s is not supported", r.URL.Path)
		}
		return nil, scimErr(http.StatusNotFound, "", "no such SCIM endpoint")
	})
}

// scimBase returns the SCIM base URL (from server.base_url, else the
// request).
func (s *Server) scimBase(r *http.Request) string {
	if b := strings.TrimSuffix(s.conf().Server.BaseURL, "/"); b != "" {
		return b + "/scim/v2"
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/scim/v2"
}

// scimAuthed authenticates the SCIM token, applies the rate limits and
// runs fn; writes run in one transaction that is committed only when fn
// succeeds.
func (s *Server) scimAuthed(fn scimHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ip := s.clientIP(r)
		if s.scimAuthLimiter.blocked(ip) {
			w.Header().Set("Retry-After", "300")
			writeSCIMError(w, scimErr(http.StatusTooManyRequests, "", "too many failed authentication attempts, try again later"))
			return
		}
		c, err := s.scimCaller(r)
		if err != nil {
			s.scimAuthLimiter.allow(ip)
			s.Log.Warn("scim authentication failed", "remote", s.loggedIP(r))
			w.Header().Set("WWW-Authenticate", `Bearer realm="scim"`)
			writeSCIMError(w, scimErr(http.StatusUnauthorized, "", "a valid SCIM bearer token is required"))
			return
		}
		if !s.scimLimiter.allow(c.Token.ID) {
			w.Header().Set("Retry-After", "60")
			writeSCIMError(w, scimErr(http.StatusTooManyRequests, "", "too many requests, slow down"))
			return
		}
		var res *scimResult
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			res, err = fn(r, c)
		} else {
			err = s.Store.InTx(r.Context(), func(ctx context.Context) error {
				var ferr error
				res, ferr = fn(r.WithContext(ctx), c)
				return ferr
			})
		}
		if err != nil {
			writeSCIMError(w, s.toSCIMError(r, err))
			return
		}
		if res.location != "" {
			w.Header().Set("Location", res.location)
		}
		if res.version != "" {
			w.Header().Set("ETag", res.version)
		}
		writeSCIM(w, res.status, res.body)
	})
}

func (s *Server) scimCaller(r *http.Request) (*scimCaller, error) {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "Bearer ") {
		return nil, errors.New("no bearer token")
	}
	tok := strings.TrimSpace(h[7:])
	if !strings.HasPrefix(tok, SCIMTokenPrefix) {
		return nil, errors.New("not a SCIM token")
	}
	t, err := s.Store.GetSCIMTokenByHash(r.Context(), auth.HashToken(tok))
	if err != nil {
		return nil, err
	}
	ten, err := s.Store.GetTenant(r.Context(), t.TenantID)
	if err != nil {
		return nil, err
	}
	return &scimCaller{Token: t, Tenant: ten, base: s.scimBase(r)}, nil
}

// scimAudit records a provisioning change.
func (s *Server) scimAudit(ctx context.Context, c *scimCaller, action, entity, id, details string) {
	e := model.AuditEntry{TenantID: c.Tenant.ID, UserEmail: "SCIM: " + c.Token.Name,
		Action: action, EntityType: entity, EntityID: id, Details: details}
	if err := s.Store.Audit(ctx, e); err != nil {
		s.Log.Warn("audit write failed", "err", err)
	}
	s.Log.Info("audit", "tenant", e.TenantID, "user", e.UserEmail, "action", action, "entity", entity, "id", id)
}

func scimDecode(r *http.Request, v any) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return badRequest("request body required")
	}
	if err := json.Unmarshal(body, v); err != nil {
		return badRequest("invalid JSON: %v", err)
	}
	return nil
}

func scimTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func scimVersion(t time.Time) string { return `W/"` + strconv.FormatInt(t.UnixNano(), 36) + `"` }

func scimMeta(kind, location string, created, modified time.Time) map[string]any {
	return map[string]any{"resourceType": kind, "created": scimTime(created), "lastModified": scimTime(modified),
		"location": location, "version": scimVersion(modified)}
}

// ---------------------------------------------------------------- lists

// scimPage reads startIndex and count (RFC 7644 §3.4.2.4).
func scimPage(r *http.Request) (start, count int, err error) {
	q := r.URL.Query()
	start, count = 1, 100
	if v := q.Get("startIndex"); v != "" {
		if start, err = strconv.Atoi(v); err != nil {
			return 0, 0, scimErr(http.StatusBadRequest, "invalidValue", "startIndex must be a number")
		}
		if start < 1 {
			start = 1
		}
	}
	if v := q.Get("count"); v != "" {
		if count, err = strconv.Atoi(v); err != nil {
			return 0, 0, scimErr(http.StatusBadRequest, "invalidValue", "count must be a number")
		}
		count = max(0, min(count, scimMaxResults))
	}
	return start, count, nil
}

// scimList filters, pages and projects resources into a ListResponse.
func scimList(r *http.Request, all []map[string]any) (*scimResult, error) {
	start, count, err := scimPage(r)
	if err != nil {
		return nil, err
	}
	q := r.URL.Query()
	if fs := strings.TrimSpace(q.Get("filter")); fs != "" {
		f, err := parseSCIMFilter(fs)
		if err != nil {
			return nil, err
		}
		var keep []map[string]any
		for _, res := range all {
			if f.match(res) {
				keep = append(keep, res)
			}
		}
		all = keep
	}
	total := len(all)
	page := []map[string]any{}
	for i := start - 1; i < total && len(page) < count; i++ {
		page = append(page, project(all[i], q.Get("attributes"), q.Get("excludedAttributes")))
	}
	return &scimResult{status: http.StatusOK, body: map[string]any{
		"schemas": []string{scimListSchema}, "totalResults": total, "startIndex": start,
		"itemsPerPage": len(page), "Resources": page,
	}}, nil
}

// project applies attributes / excludedAttributes (top-level names,
// RFC 7644 §3.9); schemas, id and meta are always returned.
func project(res map[string]any, attributes, excluded string) map[string]any {
	names := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, a := range strings.Split(s, ",") {
			if a = strings.TrimSpace(a); a != "" {
				p := splitAttrPath(a)
				out[strings.ToLower(p[0])] = true
			}
		}
		return out
	}
	if attributes == "" && excluded == "" {
		return res
	}
	keepOnly, drop := names(attributes), names(excluded)
	out := map[string]any{}
	for k, v := range res {
		lk := strings.ToLower(k)
		always := lk == "schemas" || lk == "id" || lk == "meta"
		if always || ((len(keepOnly) == 0 || keepOnly[lk]) && !drop[lk]) {
			out[k] = v
		}
	}
	return out
}

func scimGet(r *http.Request, res map[string]any) *scimResult {
	q := r.URL.Query()
	meta, _ := res["meta"].(map[string]any)
	out := &scimResult{status: http.StatusOK, body: project(res, q.Get("attributes"), q.Get("excludedAttributes"))}
	if meta != nil {
		out.version, _ = meta["version"].(string)
	}
	return out
}

// ---------------------------------------------------------------- users

func (s *Server) scimUserResource(c *scimCaller, u *model.SCIMUser, groups []model.SCIMGroup, roles []string) map[string]any {
	rl := []any{}
	for i, r := range roles {
		rl = append(rl, map[string]any{"value": r, "primary": i == 0})
	}
	gl := []any{}
	for _, g := range groups {
		gl = append(gl, map[string]any{"value": g.ID, "display": g.DisplayName, "$ref": c.base + "/Groups/" + g.ID, "type": "direct"})
	}
	name := map[string]any{}
	if u.GivenName != "" {
		name["givenName"] = u.GivenName
	}
	if u.FamilyName != "" {
		name["familyName"] = u.FamilyName
	}
	if f := strings.TrimSpace(u.GivenName + " " + u.FamilyName); f != "" {
		name["formatted"] = f
	}
	userName := u.UserName
	if userName == "" {
		userName = u.Email
	}
	res := map[string]any{
		"schemas":     []any{scimUserSchema},
		"id":          u.UserID,
		"userName":    userName,
		"displayName": u.DisplayName,
		"name":        name,
		"emails":      []any{map[string]any{"value": u.Email, "type": "work", "primary": true}},
		"active":      u.Active,
		"groups":      gl,
		"roles":       rl,
		"meta":        scimMeta("User", c.base+"/Users/"+u.UserID, u.CreatedAt, u.UpdatedAt),
	}
	if u.ExternalID != "" {
		res["externalId"] = u.ExternalID
	}
	return res
}

func (s *Server) scimLoadUser(ctx context.Context, c *scimCaller, id string) (*model.SCIMUser, map[string]any, error) {
	u, err := s.Store.GetSCIMUser(ctx, c.Tenant.ID, id)
	if err != nil {
		return nil, nil, scimErr(http.StatusNotFound, "", "user %s not found", id)
	}
	groups, err := s.Store.SCIMUserGroups(ctx, c.Tenant.ID, id)
	if err != nil {
		return nil, nil, err
	}
	var roles []string
	if m, err := s.Store.GetMembership(ctx, c.Tenant.ID, id); err == nil {
		roles = m.Roles
	}
	return u, s.scimUserResource(c, u, groups, roles), nil
}

func (s *Server) scimListUsers(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	users, err := s.Store.ListSCIMUsers(ctx, c.Tenant.ID)
	if err != nil {
		return nil, err
	}
	groups, err := s.Store.ListSCIMGroups(ctx, c.Tenant.ID)
	if err != nil {
		return nil, err
	}
	byUser := map[string][]model.SCIMGroup{}
	for _, g := range groups {
		for _, m := range g.Members {
			byUser[m] = append(byUser[m], g)
		}
	}
	ms, err := s.Store.ListTenantMembers(ctx, c.Tenant.ID)
	if err != nil {
		return nil, err
	}
	roles := map[string][]string{}
	for _, m := range ms {
		roles[m.UserID] = m.Roles
	}
	all := make([]map[string]any, 0, len(users))
	for i := range users {
		all = append(all, s.scimUserResource(c, &users[i], byUser[users[i].UserID], roles[users[i].UserID]))
	}
	return scimList(r, all)
}

func (s *Server) scimGetUser(r *http.Request, c *scimCaller) (*scimResult, error) {
	_, res, err := s.scimLoadUser(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	return scimGet(r, res), nil
}

// userState is the editable state of a provisioned user.
type userState struct {
	UserName    string
	Email       string // "" = the userName if it is an e-mail address, else prevEmail
	DisplayName string
	GivenName   string
	FamilyName  string
	ExternalID  string
	Active      bool
	prevEmail   string
}

func stateOf(u *model.SCIMUser) *userState {
	st := &userState{UserName: u.UserName, Email: u.Email, DisplayName: u.DisplayName, GivenName: u.GivenName,
		FamilyName: u.FamilyName, ExternalID: u.ExternalID, Active: u.Active, prevEmail: u.Email}
	if strings.EqualFold(u.UserName, u.Email) {
		st.Email = "" // the e-mail follows a change of the userName
	}
	return st
}

// scimUserInput is a User resource as sent by POST and PUT.
type scimUserInput struct {
	UserName    string `json:"userName"`
	ExternalID  string `json:"externalId"`
	DisplayName string `json:"displayName"`
	Name        *struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
		Formatted  string `json:"formatted"`
	} `json:"name"`
	Emails []scimEmail `json:"emails"`
	Active *flexBool   `json:"active"`
}

type scimEmail struct {
	Value   string    `json:"value"`
	Type    string    `json:"type"`
	Primary *flexBool `json:"primary"`
}

// flexBool accepts true, "True" and "false" (Entra ID sends strings).
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	v, err := parseFlexBool(json.RawMessage(data))
	if err != nil {
		return err
	}
	*b = flexBool(v)
	return nil
}

func parseFlexBool(raw json.RawMessage) (bool, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, err
	}
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(x))
		if err != nil {
			return false, scimErr(http.StatusBadRequest, "invalidValue", "%q is not a boolean", x)
		}
		return b, nil
	}
	return false, scimErr(http.StatusBadRequest, "invalidValue", "a boolean is required")
}

// pickEmail returns the primary e-mail, else the work e-mail, else the
// first one.
func pickEmail(es []scimEmail) string {
	best := ""
	for _, e := range es {
		v := strings.TrimSpace(e.Value)
		if v == "" {
			continue
		}
		if e.Primary != nil && bool(*e.Primary) {
			return v
		}
		if best == "" || strings.EqualFold(e.Type, "work") {
			best = v
		}
	}
	return best
}

func (in *scimUserInput) state(prev *userState) *userState {
	st := &userState{UserName: strings.TrimSpace(in.UserName), ExternalID: strings.TrimSpace(in.ExternalID),
		DisplayName: strings.TrimSpace(in.DisplayName), Email: pickEmail(in.Emails), Active: true}
	if in.Name != nil {
		st.GivenName, st.FamilyName = strings.TrimSpace(in.Name.GivenName), strings.TrimSpace(in.Name.FamilyName)
		if st.DisplayName == "" {
			st.DisplayName = strings.TrimSpace(in.Name.Formatted)
		}
	}
	switch {
	case in.Active != nil:
		st.Active = bool(*in.Active)
	case prev != nil:
		st.Active = prev.Active
	}
	if prev != nil {
		st.prevEmail = prev.prevEmail
	}
	return st
}

// email returns the sign-in e-mail of the state: the e-mail given, else
// the userName when it is an e-mail address, else the current e-mail.
func (st *userState) email() (string, error) {
	e := st.Email
	if e == "" {
		e = st.UserName
		if !strings.Contains(e, "@") && st.prevEmail != "" {
			e = st.prevEmail
		}
	}
	e = store.NormalizeEmail(e)
	if !strings.Contains(e, "@") || len(e) > 254 {
		return "", scimErr(http.StatusBadRequest, "invalidValue", "an e-mail address is required (as userName or in emails)")
	}
	return e, nil
}

func (st *userState) displayName() string {
	if st.DisplayName != "" {
		return st.DisplayName
	}
	return strings.TrimSpace(st.GivenName + " " + st.FamilyName)
}

// checkUserName fails when another provisioned user of the tenant has the
// userName (case-insensitive).
func (s *Server) checkUserName(ctx context.Context, c *scimCaller, userName, self string) error {
	if userName == "" {
		return scimErr(http.StatusBadRequest, "invalidValue", "userName is required")
	}
	users, err := s.Store.ListSCIMUsers(ctx, c.Tenant.ID)
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.UserID != self && (strings.EqualFold(u.UserName, userName) || (u.UserName == "" && strings.EqualFold(u.Email, userName))) {
			return scimErr(http.StatusConflict, "uniqueness", "a user with userName %s already exists", userName)
		}
	}
	return nil
}

func (s *Server) scimCreateUser(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	var in scimUserInput
	if err := scimDecode(r, &in); err != nil {
		return nil, err
	}
	st := in.state(nil)
	if err := s.checkUserName(ctx, c, st.UserName, ""); err != nil {
		return nil, err
	}
	email, err := st.email()
	if err != nil {
		return nil, err
	}
	u, err := s.Store.GetUserByEmail(ctx, email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		u = &model.User{Email: email, DisplayName: st.displayName(), AuthProvider: scimAuthProvider}
		if err := s.Store.CreateUser(ctx, u); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		if linked, err := s.Store.IsSCIMUser(ctx, c.Tenant.ID, u.ID); err != nil || linked {
			return nil, errOr(err, scimErr(http.StatusConflict, "uniqueness", "a user with the e-mail %s is already provisioned", email))
		}
		// an existing account (e.g. a member added by hand) is taken over
		if err := s.applyProfile(ctx, c, u, st); err != nil {
			return nil, err
		}
	}
	su := &model.SCIMUser{TenantID: c.Tenant.ID, UserID: u.ID, UserName: st.UserName, ExternalID: st.ExternalID,
		GivenName: st.GivenName, FamilyName: st.FamilyName, Active: st.Active}
	if err := s.Store.SaveSCIMUser(ctx, su); err != nil {
		return nil, err
	}
	role, err := s.scimSync(ctx, c.Tenant.ID, u.ID)
	if err != nil {
		return nil, err
	}
	s.scimAudit(ctx, c, "scim.user.create", "user", u.ID, describeUser(email, st.Active, role))
	_, res, err := s.scimLoadUser(ctx, c, u.ID)
	if err != nil {
		return nil, err
	}
	out := scimGet(r, res)
	out.status, out.location = http.StatusCreated, c.base+"/Users/"+u.ID
	return out, nil
}

func describeUser(email string, active bool, roles []string) string {
	switch {
	case !active:
		return email + " (inactive)"
	case len(roles) == 0:
		return email + " (no role)"
	}
	return email + " as " + strings.Join(roles, ", ")
}

// applyProfile stores the e-mail and display name of st on u. Users who
// also belong to other tenants, and super admins, keep their profile: one
// tenant's identity provider must not change them for the others.
func (s *Server) applyProfile(ctx context.Context, c *scimCaller, u *model.User, st *userState) error {
	email, err := st.email()
	if err != nil {
		return err
	}
	others, err := s.Store.OtherTenants(ctx, c.Tenant.ID, u.ID)
	if err != nil {
		return err
	}
	shared := others > 0 || u.SuperAdmin
	if email != u.Email {
		if shared {
			return scimErr(http.StatusBadRequest, "mutability", "the e-mail of %s cannot be changed here: the user also belongs to other organisations", u.Email)
		}
		if err := s.Store.SetUserEmail(ctx, u.ID, email); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return scimErr(http.StatusConflict, "uniqueness", "another user has the e-mail %s", email)
			}
			return err
		}
		u.Email = email
	}
	if dn := st.displayName(); !shared && dn != "" && dn != u.DisplayName {
		u.DisplayName = dn
		return s.Store.UpdateUser(ctx, u)
	}
	return nil
}

// saveUser stores the new state of a provisioned user and syncs the
// membership; it audits what changed.
func (s *Server) saveUser(ctx context.Context, c *scimCaller, su *model.SCIMUser, st *userState) error {
	if err := s.checkUserName(ctx, c, st.UserName, su.UserID); err != nil {
		return err
	}
	u, err := s.Store.GetUser(ctx, su.UserID)
	if err != nil {
		return err
	}
	if err := s.applyProfile(ctx, c, u, st); err != nil {
		return err
	}
	wasActive := su.Active
	su.UserName, su.ExternalID, su.GivenName, su.FamilyName, su.Active = st.UserName, st.ExternalID, st.GivenName, st.FamilyName, st.Active
	if err := s.Store.SaveSCIMUser(ctx, su); err != nil {
		return err
	}
	role, err := s.scimSync(ctx, c.Tenant.ID, su.UserID)
	if err != nil {
		return err
	}
	action := "scim.user.update"
	switch {
	case wasActive && !st.Active:
		action = "scim.user.deactivate"
	case !wasActive && st.Active:
		action = "scim.user.reactivate"
	}
	s.scimAudit(ctx, c, action, "user", su.UserID, describeUser(u.Email, st.Active, role))
	return nil
}

func (s *Server) scimReplaceUser(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	su, _, err := s.scimLoadUser(ctx, c, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	var in scimUserInput
	if err := scimDecode(r, &in); err != nil {
		return nil, err
	}
	if err := s.saveUser(ctx, c, su, in.state(stateOf(su))); err != nil {
		return nil, err
	}
	_, res, err := s.scimLoadUser(ctx, c, su.UserID)
	if err != nil {
		return nil, err
	}
	return scimGet(r, res), nil
}

// patchOp is one PATCH operation (RFC 7644 §3.5.2).
type patchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

type patchRequest struct {
	Schemas    []string  `json:"schemas"`
	Operations []patchOp `json:"Operations"`
}

func decodePatch(r *http.Request) (*patchRequest, error) {
	var req patchRequest
	if err := scimDecode(r, &req); err != nil {
		return nil, err
	}
	if len(req.Operations) == 0 {
		return nil, scimErr(http.StatusBadRequest, "invalidValue", "Operations are required")
	}
	for i := range req.Operations {
		op := strings.ToLower(strings.TrimSpace(req.Operations[i].Op))
		if op != "add" && op != "replace" && op != "remove" {
			return nil, scimErr(http.StatusBadRequest, "invalidSyntax", "unknown op %q", req.Operations[i].Op)
		}
		req.Operations[i].Op = op
	}
	return &req, nil
}

func (s *Server) scimPatchUser(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	su, _, err := s.scimLoadUser(ctx, c, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	req, err := decodePatch(r)
	if err != nil {
		return nil, err
	}
	st := stateOf(su)
	for _, op := range req.Operations {
		if err := patchUser(st, op); err != nil {
			return nil, err
		}
	}
	if err := s.saveUser(ctx, c, su, st); err != nil {
		return nil, err
	}
	_, res, err := s.scimLoadUser(ctx, c, su.UserID)
	if err != nil {
		return nil, err
	}
	return scimGet(r, res), nil
}

// patchUser applies one operation to st.
func patchUser(st *userState, op patchOp) error {
	if strings.TrimSpace(op.Path) == "" {
		if op.Op == "remove" {
			return scimErr(http.StatusBadRequest, "noTarget", "remove needs a path")
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(op.Value, &m); err != nil {
			return scimErr(http.StatusBadRequest, "invalidValue", "a value object is required without a path")
		}
		for k, v := range m {
			p, err := parseSCIMPath(k)
			if err != nil {
				return scimErr(http.StatusBadRequest, "invalidPath", "%s", err.Error())
			}
			if err := setUserAttr(st, op.Op, p, v); err != nil {
				return err
			}
		}
		return nil
	}
	p, err := parseSCIMPath(op.Path)
	if err != nil {
		return scimErr(http.StatusBadRequest, "invalidPath", "%s", err.Error())
	}
	return setUserAttr(st, op.Op, p, op.Value)
}

func rawString(raw json.RawMessage) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", scimErr(http.StatusBadRequest, "invalidValue", "a value is required")
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x), nil
	case nil:
		return "", nil
	case []any:
		// a one-element list, as some providers send for single values
		if len(x) == 1 {
			if s, ok := x[0].(string); ok {
				return strings.TrimSpace(s), nil
			}
			if m, ok := x[0].(map[string]any); ok {
				if s, ok := m["value"].(string); ok {
					return strings.TrimSpace(s), nil
				}
			}
		}
	}
	return "", scimErr(http.StatusBadRequest, "invalidValue", "a string value is required")
}

// setUserAttr applies add/replace/remove of one attribute. Attributes
// Qtestframe does not store (title, phone numbers, extensions …) are
// accepted and ignored, so providers do not retry them forever.
func setUserAttr(st *userState, op string, p *scimPath, raw json.RawMessage) error {
	remove := op == "remove"
	str := func(dst *string) error {
		if remove {
			*dst = ""
			return nil
		}
		v, err := rawString(raw)
		if err != nil {
			return err
		}
		*dst = v
		return nil
	}
	switch p.attr {
	case "username":
		if remove {
			return scimErr(http.StatusBadRequest, "mutability", "userName cannot be removed")
		}
		return str(&st.UserName)
	case "displayname":
		return str(&st.DisplayName)
	case "externalid":
		return str(&st.ExternalID)
	case "active":
		if remove {
			return nil
		}
		b, err := parseFlexBool(raw)
		if err != nil {
			return err
		}
		st.Active = b
		return nil
	case "name":
		switch p.sub {
		case "givenname":
			return str(&st.GivenName)
		case "familyname":
			return str(&st.FamilyName)
		case "formatted":
			if st.DisplayName == "" {
				return str(&st.DisplayName)
			}
			return nil
		case "":
			if remove {
				st.GivenName, st.FamilyName = "", ""
				return nil
			}
			var n map[string]json.RawMessage
			if err := json.Unmarshal(raw, &n); err != nil {
				return scimErr(http.StatusBadRequest, "invalidValue", "name must be an object")
			}
			for k, v := range n {
				if err := setUserAttr(st, op, &scimPath{attr: "name", sub: strings.ToLower(k)}, v); err != nil {
					return err
				}
			}
		}
		return nil
	case "emails":
		if remove {
			if p.filter == nil && p.sub == "" {
				st.Email = ""
			}
			return nil
		}
		if p.sub == "value" || p.filter != nil {
			if p.sub != "" && p.sub != "value" {
				return nil // emails[...].type, .primary
			}
			if p.sub == "" {
				var e scimEmail
				if err := json.Unmarshal(raw, &e); err != nil {
					return scimErr(http.StatusBadRequest, "invalidValue", "an e-mail object is required")
				}
				if e.Value != "" {
					st.Email = strings.TrimSpace(e.Value)
				}
				return nil
			}
			return str(&st.Email)
		}
		var es []scimEmail
		if err := json.Unmarshal(raw, &es); err != nil {
			var e scimEmail
			if err := json.Unmarshal(raw, &e); err != nil {
				return scimErr(http.StatusBadRequest, "invalidValue", "emails must be a list")
			}
			es = []scimEmail{e}
		}
		if e := pickEmail(es); e != "" {
			st.Email = e
		}
		return nil
	}
	return nil
}

func (s *Server) scimDeleteUser(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	su, _, err := s.scimLoadUser(ctx, c, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if err := s.guardLastAdmin(ctx, c.Tenant.ID, su.UserID, nil); err != nil {
		return nil, err
	}
	if err := s.Store.RemoveTenantAccess(ctx, c.Tenant.ID, su.UserID); err != nil {
		return nil, err
	}
	if err := s.Store.DeleteSCIMUser(ctx, c.Tenant.ID, su.UserID); err != nil {
		return nil, err
	}
	s.scimAudit(ctx, c, "scim.user.delete", "user", su.UserID, su.Email)
	return &scimResult{status: http.StatusNoContent}, nil
}

// ---------------------------------------------------------------- role sync

// scimRoles returns the roles a provisioned, active user should have: the
// roles of all the user's mapped groups (a user in several groups holds
// all their roles), else the default role (none = no access). Roles the
// tenant no longer has are ignored.
func (s *Server) scimRoles(ctx context.Context, tenantID, userID string) ([]string, error) {
	set, err := s.Store.RoleSet(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	groups, err := s.Store.SCIMUserGroups(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	var roles []string
	for _, g := range groups {
		if g.Role != "" && set.Valid(g.Role) {
			roles = append(roles, g.Role)
		}
	}
	if len(roles) > 0 {
		return rbac.NormalizeRoles(roles), nil
	}
	cfg, err := s.Store.SCIMSettings(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if set.Valid(cfg.DefaultRole) {
		return []string{cfg.DefaultRole}, nil
	}
	return nil, nil
}

// guardLastAdmin fails when giving the user the roles newRoles (none = no
// membership) would leave the tenant without an admin.
func (s *Server) guardLastAdmin(ctx context.Context, tenantID, userID string, newRoles []string) error {
	m, err := s.Store.GetMembership(ctx, tenantID, userID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !rbac.HasRole(m.Roles, rbac.RoleAdmin) || rbac.HasRole(newRoles, rbac.RoleAdmin) {
		return nil
	}
	n, err := s.Store.CountTenantAdmins(ctx, tenantID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return errLastAdmin
	}
	return nil
}

// scimSync brings the membership of a provisioned user in line with the
// user's state and groups and returns the roles (none = no membership).
// Inactive users lose the membership, their sessions in the tenant and
// their API tokens there.
func (s *Server) scimSync(ctx context.Context, tenantID, userID string) ([]string, error) {
	su, err := s.Store.GetSCIMUser(ctx, tenantID, userID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var roles []string
	if su.Active {
		if roles, err = s.scimRoles(ctx, tenantID, userID); err != nil {
			return nil, err
		}
	}
	cur, err := s.Store.GetMembership(ctx, tenantID, userID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if cur != nil && slices.Equal(cur.Roles, roles) {
		return roles, nil
	}
	if err := s.guardLastAdmin(ctx, tenantID, userID, roles); err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		if err := s.Store.RemoveTenantAccess(ctx, tenantID, userID); err != nil {
			return nil, err
		}
	} else if err := s.Store.SetMembershipRoles(ctx, tenantID, userID, roles); err != nil {
		return nil, err
	}
	return roles, s.Store.TouchSCIMUser(ctx, tenantID, userID)
}

// scimSyncAll syncs several users (deduplicated).
func (s *Server) scimSyncAll(ctx context.Context, tenantID string, ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := s.scimSync(ctx, tenantID, id); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- groups

func (s *Server) scimGroupResource(c *scimCaller, g *model.SCIMGroup, emails map[string]string) map[string]any {
	ms := []any{}
	for _, id := range g.Members {
		ms = append(ms, map[string]any{"value": id, "display": emails[id], "$ref": c.base + "/Users/" + id, "type": "User"})
	}
	res := map[string]any{
		"schemas":     []any{scimGroupSchema},
		"id":          g.ID,
		"displayName": g.DisplayName,
		"members":     ms,
		"meta":        scimMeta("Group", c.base+"/Groups/"+g.ID, g.CreatedAt, g.UpdatedAt),
	}
	if g.ExternalID != "" {
		res["externalId"] = g.ExternalID
	}
	return res
}

// scimUserEmails maps the provisioned users of the tenant to their e-mail.
func (s *Server) scimUserEmails(ctx context.Context, tenantID string) (map[string]string, error) {
	users, err := s.Store.ListSCIMUsers(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, u := range users {
		out[u.UserID] = u.Email
	}
	return out, nil
}

func (s *Server) scimLoadGroup(ctx context.Context, c *scimCaller, id string) (*model.SCIMGroup, map[string]any, error) {
	g, err := s.Store.GetSCIMGroup(ctx, c.Tenant.ID, id)
	if err != nil {
		return nil, nil, scimErr(http.StatusNotFound, "", "group %s not found", id)
	}
	emails, err := s.scimUserEmails(ctx, c.Tenant.ID)
	if err != nil {
		return nil, nil, err
	}
	return g, s.scimGroupResource(c, g, emails), nil
}

func (s *Server) scimListGroups(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	groups, err := s.Store.ListSCIMGroups(ctx, c.Tenant.ID)
	if err != nil {
		return nil, err
	}
	emails, err := s.scimUserEmails(ctx, c.Tenant.ID)
	if err != nil {
		return nil, err
	}
	all := make([]map[string]any, 0, len(groups))
	for i := range groups {
		all = append(all, s.scimGroupResource(c, &groups[i], emails))
	}
	return scimList(r, all)
}

func (s *Server) scimGetGroup(r *http.Request, c *scimCaller) (*scimResult, error) {
	_, res, err := s.scimLoadGroup(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	return scimGet(r, res), nil
}

type scimMember struct {
	Value string `json:"value"`
}

type scimGroupInput struct {
	DisplayName string       `json:"displayName"`
	ExternalID  string       `json:"externalId"`
	Members     []scimMember `json:"members"`
}

// memberIDs checks that every member is a provisioned user of the tenant.
func (s *Server) memberIDs(ctx context.Context, c *scimCaller, ms []scimMember) ([]string, error) {
	emails, err := s.scimUserEmails(ctx, c.Tenant.ID)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, m := range ms {
		id := strings.TrimSpace(m.Value)
		if _, ok := emails[id]; !ok {
			return nil, scimErr(http.StatusBadRequest, "invalidValue", "member %q is not a provisioned user", m.Value)
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *Server) scimCreateGroup(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	var in scimGroupInput
	if err := scimDecode(r, &in); err != nil {
		return nil, err
	}
	ids, err := s.memberIDs(ctx, c, in.Members)
	if err != nil {
		return nil, err
	}
	g := &model.SCIMGroup{TenantID: c.Tenant.ID, DisplayName: in.DisplayName, ExternalID: strings.TrimSpace(in.ExternalID), Members: ids}
	if err := s.Store.CreateSCIMGroup(ctx, g); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, scimErr(http.StatusConflict, "uniqueness", "a group named %s already exists", g.DisplayName)
		}
		return nil, err
	}
	if err := s.scimSyncAll(ctx, c.Tenant.ID, g.Members); err != nil {
		return nil, err
	}
	s.scimAudit(ctx, c, "scim.group.create", "group", g.ID, fmt.Sprintf("%s (%d members)", g.DisplayName, len(g.Members)))
	_, res, err := s.scimLoadGroup(ctx, c, g.ID)
	if err != nil {
		return nil, err
	}
	out := scimGet(r, res)
	out.status, out.location = http.StatusCreated, c.base+"/Groups/"+g.ID
	return out, nil
}

// saveGroup stores g with the new members, syncs the users whose groups
// changed and audits the change.
func (s *Server) saveGroup(ctx context.Context, c *scimCaller, g *model.SCIMGroup, before, after []string, renamed bool) error {
	if err := s.Store.UpdateSCIMGroup(ctx, g, after); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return scimErr(http.StatusConflict, "uniqueness", "a group named %s already exists", g.DisplayName)
		}
		return err
	}
	added, removed := diffIDs(before, after)
	if err := s.scimSyncAll(ctx, c.Tenant.ID, append(append([]string{}, added...), removed...)); err != nil {
		return err
	}
	var parts []string
	if renamed {
		parts = append(parts, "renamed")
	}
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("%d added", len(added)))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", len(removed)))
	}
	if len(parts) == 0 {
		parts = append(parts, "unchanged")
	}
	s.scimAudit(ctx, c, "scim.group.update", "group", g.ID, g.DisplayName+": "+strings.Join(parts, ", "))
	return nil
}

func diffIDs(before, after []string) (added, removed []string) {
	in := func(l []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range l {
			m[x] = true
		}
		return m
	}
	b, a := in(before), in(after)
	for _, x := range after {
		if !b[x] {
			added = append(added, x)
			b[x] = true
		}
	}
	for _, x := range before {
		if !a[x] {
			removed = append(removed, x)
			a[x] = true
		}
	}
	return added, removed
}

func (s *Server) scimReplaceGroup(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	g, _, err := s.scimLoadGroup(ctx, c, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	var in scimGroupInput
	if err := scimDecode(r, &in); err != nil {
		return nil, err
	}
	ids, err := s.memberIDs(ctx, c, in.Members)
	if err != nil {
		return nil, err
	}
	before := g.Members
	renamed := strings.TrimSpace(in.DisplayName) != g.DisplayName
	g.DisplayName, g.ExternalID = in.DisplayName, strings.TrimSpace(in.ExternalID)
	if err := s.saveGroup(ctx, c, g, before, ids, renamed); err != nil {
		return nil, err
	}
	_, res, err := s.scimLoadGroup(ctx, c, g.ID)
	if err != nil {
		return nil, err
	}
	return scimGet(r, res), nil
}

func (s *Server) scimPatchGroup(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	g, _, err := s.scimLoadGroup(ctx, c, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	req, err := decodePatch(r)
	if err != nil {
		return nil, err
	}
	before := append([]string{}, g.Members...)
	name := g.DisplayName
	members := append([]string{}, g.Members...)
	for _, op := range req.Operations {
		if members, err = s.patchGroup(ctx, c, g, members, op); err != nil {
			return nil, err
		}
	}
	if err := s.saveGroup(ctx, c, g, before, members, g.DisplayName != name); err != nil {
		return nil, err
	}
	// Entra ID and Okta accept 204 or 200; the resource is more useful
	_, res, err := s.scimLoadGroup(ctx, c, g.ID)
	if err != nil {
		return nil, err
	}
	return scimGet(r, res), nil
}

// patchGroup applies one operation to the group (name, external id) and
// returns the new member list.
func (s *Server) patchGroup(ctx context.Context, c *scimCaller, g *model.SCIMGroup, members []string, op patchOp) ([]string, error) {
	if strings.TrimSpace(op.Path) == "" {
		if op.Op == "remove" {
			return nil, scimErr(http.StatusBadRequest, "noTarget", "remove needs a path")
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(op.Value, &m); err != nil {
			return nil, scimErr(http.StatusBadRequest, "invalidValue", "a value object is required without a path")
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			var err error
			if members, err = s.patchGroup(ctx, c, g, members, patchOp{Op: op.Op, Path: k, Value: m[k]}); err != nil {
				return nil, err
			}
		}
		return members, nil
	}
	p, err := parseSCIMPath(op.Path)
	if err != nil {
		return nil, scimErr(http.StatusBadRequest, "invalidPath", "%s", err.Error())
	}
	switch p.attr {
	case "displayname":
		if op.Op == "remove" {
			return nil, scimErr(http.StatusBadRequest, "mutability", "displayName cannot be removed")
		}
		v, err := rawString(op.Value)
		if err != nil {
			return nil, err
		}
		g.DisplayName = v
	case "externalid":
		if op.Op == "remove" {
			g.ExternalID = ""
			break
		}
		v, err := rawString(op.Value)
		if err != nil {
			return nil, err
		}
		g.ExternalID = v
	case "members":
		return s.patchMembers(ctx, c, members, op, p)
	}
	// id, schemas, meta and unknown attributes are ignored
	return members, nil
}

func (s *Server) patchMembers(ctx context.Context, c *scimCaller, members []string, op patchOp, p *scimPath) ([]string, error) {
	var given []scimMember
	if len(op.Value) > 0 && string(op.Value) != "null" {
		if err := json.Unmarshal(op.Value, &given); err != nil {
			var one scimMember
			if err := json.Unmarshal(op.Value, &one); err != nil {
				return nil, scimErr(http.StatusBadRequest, "invalidValue", "members must be a list of {\"value\": id}")
			}
			given = []scimMember{one}
		}
	}
	switch op.Op {
	case "remove":
		drop := map[string]bool{}
		switch {
		case p.filter != nil:
			for _, id := range members {
				if p.filter.match(map[string]any{"value": id}) {
					drop[id] = true
				}
			}
		case len(given) > 0:
			for _, m := range given {
				drop[strings.TrimSpace(m.Value)] = true
			}
		default:
			return []string{}, nil
		}
		out := []string{}
		for _, id := range members {
			if !drop[id] {
				out = append(out, id)
			}
		}
		return out, nil
	case "replace":
		if p.filter != nil {
			return nil, scimErr(http.StatusBadRequest, "invalidPath", "replace of a filtered member is not supported")
		}
		return s.memberIDs(ctx, c, given)
	default: // add
		ids, err := s.memberIDs(ctx, c, given)
		if err != nil {
			return nil, err
		}
		return append(members, ids...), nil
	}
}

func (s *Server) scimDeleteGroup(r *http.Request, c *scimCaller) (*scimResult, error) {
	ctx := r.Context()
	g, _, err := s.scimLoadGroup(ctx, c, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if err := s.Store.DeleteSCIMGroup(ctx, c.Tenant.ID, g.ID); err != nil {
		return nil, err
	}
	if err := s.scimSyncAll(ctx, c.Tenant.ID, g.Members); err != nil {
		return nil, err
	}
	s.scimAudit(ctx, c, "scim.group.delete", "group", g.ID, g.DisplayName)
	return &scimResult{status: http.StatusNoContent}, nil
}

// ---------------------------------------------------------------- discovery

func (s *Server) scimServiceProviderConfig(r *http.Request, c *scimCaller) (*scimResult, error) {
	return &scimResult{status: http.StatusOK, body: map[string]any{
		"schemas":          []string{scimSPConfigSchema},
		"documentationUri": "https://github.com/rom/qtestframe/blob/main/docs/api.md#scim-20-provisioning",
		"patch":            map[string]any{"supported": true},
		"bulk":             map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":           map[string]any{"supported": true, "maxResults": scimMaxResults},
		"changePassword":   map[string]any{"supported": false},
		"sort":             map[string]any{"supported": false},
		"etag":             map[string]any{"supported": true},
		"authenticationSchemes": []any{map[string]any{
			"type": "oauthbearertoken", "name": "OAuth Bearer Token", "primary": true,
			"description": "A SCIM token created by an organisation admin under Administration → Provisioning (SCIM).",
		}},
		"meta": map[string]any{"resourceType": "ServiceProviderConfig", "location": c.base + "/ServiceProviderConfig"},
	}}, nil
}

func scimResourceTypeList(base string) []map[string]any {
	rt := func(id, endpoint, schema, desc string) map[string]any {
		return map[string]any{"schemas": []string{scimResourceTypeSchema}, "id": id, "name": id, "endpoint": endpoint,
			"description": desc, "schema": schema, "schemaExtensions": []any{},
			"meta": map[string]any{"resourceType": "ResourceType", "location": base + "/ResourceTypes/" + id}}
	}
	return []map[string]any{
		rt("User", "/Users", scimUserSchema, "User account (a member of the organisation while active)"),
		rt("Group", "/Groups", scimGroupSchema, "Group; organisation admins map groups to roles"),
	}
}

func (s *Server) scimResourceTypes(r *http.Request, c *scimCaller) (*scimResult, error) {
	return scimList(r, scimResourceTypeList(c.base))
}

func (s *Server) scimResourceType(r *http.Request, c *scimCaller) (*scimResult, error) {
	for _, rt := range scimResourceTypeList(c.base) {
		if rt["id"] == r.PathValue("id") {
			return &scimResult{status: http.StatusOK, body: rt}, nil
		}
	}
	return nil, scimErr(http.StatusNotFound, "", "resource type %s not found", r.PathValue("id"))
}

func scimAttr(name, typ string, multi, required bool, mutability, uniqueness string, sub ...map[string]any) map[string]any {
	a := map[string]any{"name": name, "type": typ, "multiValued": multi, "required": required, "caseExact": false,
		"mutability": mutability, "returned": "default", "uniqueness": uniqueness, "description": name}
	if len(sub) > 0 {
		a["subAttributes"] = sub
	}
	return a
}

func scimSchemaList(base string) []map[string]any {
	schema := func(id, name, desc string, attrs ...map[string]any) map[string]any {
		return map[string]any{"schemas": []string{scimSchemaSchema}, "id": id, "name": name, "description": desc, "attributes": attrs,
			"meta": map[string]any{"resourceType": "Schema", "location": base + "/Schemas/" + id}}
	}
	ref := func() []map[string]any {
		return []map[string]any{
			scimAttr("value", "string", false, false, "immutable", "none"),
			scimAttr("display", "string", false, false, "readOnly", "none"),
			scimAttr("$ref", "reference", false, false, "readOnly", "none"),
			scimAttr("type", "string", false, false, "readOnly", "none"),
		}
	}
	return []map[string]any{
		schema(scimUserSchema, "User", "User account",
			scimAttr("userName", "string", false, true, "readWrite", "server"),
			scimAttr("externalId", "string", false, false, "readWrite", "none"),
			scimAttr("displayName", "string", false, false, "readWrite", "none"),
			scimAttr("name", "complex", false, false, "readWrite", "none",
				scimAttr("givenName", "string", false, false, "readWrite", "none"),
				scimAttr("familyName", "string", false, false, "readWrite", "none"),
				scimAttr("formatted", "string", false, false, "readWrite", "none")),
			scimAttr("emails", "complex", true, false, "readWrite", "none",
				scimAttr("value", "string", false, false, "readWrite", "none"),
				scimAttr("type", "string", false, false, "readWrite", "none"),
				scimAttr("primary", "boolean", false, false, "readWrite", "none")),
			scimAttr("active", "boolean", false, false, "readWrite", "none"),
			scimAttr("groups", "complex", true, false, "readOnly", "none", ref()...),
			scimAttr("roles", "complex", true, false, "readOnly", "none",
				scimAttr("value", "string", false, false, "readOnly", "none"),
				scimAttr("primary", "boolean", false, false, "readOnly", "none"))),
		schema(scimGroupSchema, "Group", "Group",
			scimAttr("displayName", "string", false, true, "readWrite", "server"),
			scimAttr("externalId", "string", false, false, "readWrite", "none"),
			scimAttr("members", "complex", true, false, "readWrite", "none", ref()...)),
	}
}

func (s *Server) scimSchemas(r *http.Request, c *scimCaller) (*scimResult, error) {
	return scimList(r, scimSchemaList(c.base))
}

func (s *Server) scimSchema(r *http.Request, c *scimCaller) (*scimResult, error) {
	for _, sc := range scimSchemaList(c.base) {
		if sc["id"] == r.PathValue("id") {
			return &scimResult{status: http.StatusOK, body: sc}, nil
		}
	}
	return nil, scimErr(http.StatusNotFound, "", "schema %s not found", r.PathValue("id"))
}
