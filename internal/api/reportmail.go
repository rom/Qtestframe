package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
)

// Reports by e-mail: send a report now, and scheduled subscriptions of
// saved reports. Recipients are members of the tenant who may read what
// the report shows; other addresses need project.update (so that the
// server is no open relay), and sending is rate limited per user.

const maxReportRecipients = 50

func (s *Server) routesReportMail(h routeFunc) {
	h("GET /api/v1/projects/{pid}/reports/recipients", s.handleReportRecipients, "export")
	h("POST /api/v1/projects/{pid}/reports/email", s.handleEmailReport, "export")
	h("GET /api/v1/projects/{pid}/reports/subscriptions", s.handleListSubscriptions, "export")
	h("POST /api/v1/projects/{pid}/reports/subscriptions", s.handleCreateSubscription, "export")
	h("PATCH /api/v1/reports/subscriptions/{id}", s.handleUpdateSubscription, "export")
	h("DELETE /api/v1/reports/subscriptions/{id}", s.handleDeleteSubscription, "export")
	h("POST /api/v1/reports/subscriptions/{id}/send", s.handleSendSubscription, "export")
}

// reportReadPerms are the permissions a recipient needs for a report.
func reportReadPerms(job reportJob) []rbac.Permission {
	perms := []rbac.Permission{rbac.ProjectRead}
	if job.custom != nil {
		switch job.custom.Entity {
		case report.EntityRequirement:
			return append(perms, rbac.RequirementRd)
		case report.EntityTest:
			return append(perms, rbac.TestRead)
		default:
			return append(perms, rbac.RunRead)
		}
	}
	switch job.template {
	case report.RequirementsCoverage, report.Traceability:
		return append(perms, rbac.RequirementRd, rbac.TestRead)
	case report.TestInventory:
		return append(perms, rbac.TestRead)
	case report.RunSummary:
		return append(perms, rbac.RunRead, rbac.TestRead)
	case report.Defects:
		return append(perms, rbac.RunRead)
	case report.Risks:
		return append(perms, rbac.RequirementRd, rbac.TestRead)
	}
	return append(perms, rbac.RequirementRd, rbac.TestRead, rbac.RunRead)
}

// reportRecipient is a member who can get reports.
type reportRecipient struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// readers returns the members of the tenant holding perms, by user ID.
func (s *Server) readers(ctx context.Context, tenantID, projectID string, perms []rbac.Permission) (map[string]reportRecipient, error) {
	// the members' permissions in the project (project sharing: access.go)
	access, ms, err := s.membersIn(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := map[string]reportRecipient{}
	for _, m := range ms {
		acc := access[m.UserID]
		ok := acc != nil
		for _, perm := range perms {
			ok = ok && slices.Contains(acc.Permissions, string(perm))
		}
		if ok {
			out[m.UserID] = reportRecipient{UserID: m.UserID, Email: m.Email, DisplayName: m.Name, Role: m.Role}
		}
	}
	return out, nil
}

// handleReportRecipients: GET /projects/{pid}/reports/recipients lists the
// members who may read the project, and whether the caller may also send
// to other addresses.
func (s *Server) handleReportRecipients(w http.ResponseWriter, r *http.Request, p *Principal) {
	if _, ok := s.project(w, r, p); !ok {
		return
	}
	rs, err := s.readers(r.Context(), p.TenantID(), r.PathValue("pid"), []rbac.Permission{rbac.ProjectRead})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list := make([]reportRecipient, 0, len(rs))
	for _, x := range rs {
		list = append(list, x)
	}
	slices.SortFunc(list, func(a, b reportRecipient) int { return strings.Compare(a.Email, b.Email) })
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "addresses_allowed": p.Can(rbac.ProjectUpdate), "email_enabled": s.emailOn()})
}

// cleanAddresses normalises and checks explicit addresses.
func cleanAddresses(list []string) ([]string, error) {
	var out []string
	for _, a := range list {
		a = store.NormalizeEmail(a)
		if a == "" {
			continue
		}
		if err := mail.CheckAddress(a); err != nil {
			return nil, badRequest("%q is not a valid e-mail address", a)
		}
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out, nil
}

type skippedRecipient struct {
	UserID string `json:"user_id"`
	Reason string `json:"reason"`
}

// resolveRecipients turns member IDs and addresses into mail addresses:
// members who may not read the report or are disabled are skipped.
func (s *Server) resolveRecipients(ctx context.Context, tenantID, projectID string, job reportJob, ids, addresses []string) ([]mail.Address, []skippedRecipient, error) {
	rs, err := s.readers(ctx, tenantID, projectID, reportReadPerms(job))
	if err != nil {
		return nil, nil, err
	}
	var to []mail.Address
	var skipped []skippedRecipient
	seen := map[string]bool{}
	for _, id := range uniqStrings(ids) {
		x, ok := rs[id]
		if !ok {
			skipped = append(skipped, skippedRecipient{UserID: id, Reason: "no_access"})
			continue
		}
		u, err := s.Store.GetUser(ctx, id)
		if err != nil || u.Disabled {
			skipped = append(skipped, skippedRecipient{UserID: id, Reason: "disabled"})
			continue
		}
		if !seen[x.Email] {
			seen[x.Email] = true
			to = append(to, userAddress(u))
		}
	}
	for _, a := range addresses {
		if !seen[a] {
			seen[a] = true
			to = append(to, mail.Address{Email: a})
		}
	}
	return to, skipped, nil
}

// reportMail builds the e-mail of a rendered report (in the report's
// language).
func (s *Server) reportMail(p *Principal, pr *model.Project, rep *renderedReport, format, message string, scheduled bool) *mail.Message {
	lang := rep.lang
	title := rep.title
	if title == "" {
		title = rep.name
	}
	project := pr.Key + " " + pr.Name
	intro := tr(lang, "report.intro", inviter(p), title, strings.ToUpper(format), project)
	c := mailContent{Lang: mailLang(lang), Title: title, Intro: []string{intro},
		Action: &mailLink{Text: tr(lang, "report.open"), URL: s.baseURL() + "/p/" + url.PathEscape(pr.ID) + "/reports"}}
	if scheduled {
		c.Intro = []string{tr(lang, "report.scheduled", title, strings.ToUpper(format), project)}
		c.Footer = []string{tr(lang, "report.schedFooter", inviter(p))}
	}
	if msg := strings.TrimSpace(message); msg != "" {
		c.Intro = append(c.Intro, msg)
	}
	m := s.message(nil, tr(lang, "report.subject", title), c)
	m.Attachments = []mail.Attachment{{Name: rep.name, ContentType: rep.contentType, Data: rep.data}}
	if mail.CheckAddress(p.User.Email) == nil {
		m.ReplyTo = &mail.Address{Name: oneLineText(inviter(p), 100), Email: p.User.Email}
	}
	if scheduled {
		m.Headers = map[string]string{"Auto-Submitted": "auto-generated"}
	}
	return m
}

type reportMailReq struct {
	Template     string            `json:"template"`
	Params       map[string]string `json:"params"`
	Custom       json.RawMessage   `json:"custom"`
	DefinitionID string            `json:"definition_id"`
	Format       string            `json:"format"`
	Recipients   []string          `json:"recipients"`
	Addresses    []string          `json:"addresses"`
	Message      string            `json:"message"`
	Lang         string            `json:"lang"`
}

// jobOf turns a request into a report job (exactly one source).
func (s *Server) jobOf(r *http.Request, p *Principal, pr *model.Project, req reportMailReq) (reportJob, error) {
	hasCustom := len(req.Custom) > 0 && string(req.Custom) != "null"
	req.Template = strings.TrimSpace(req.Template)
	n := 0
	for _, b := range []bool{req.Template != "", hasCustom, req.DefinitionID != ""} {
		if b {
			n++
		}
	}
	if n != 1 {
		return reportJob{}, badRequest("exactly one of template, custom or definition_id is required")
	}
	job := reportJob{template: req.Template, params: req.Params, format: req.Format, stem: req.Template, lang: reportLang(r, p, req.Lang)}
	if job.params == nil {
		job.params = map[string]string{}
	}
	switch {
	case hasCustom:
		c, err := report.ParseCustomConfig(req.Custom)
		if err != nil {
			return job, reportError(err)
		}
		job.custom, job.stem = c, "custom_report"
	case req.DefinitionID != "":
		d, err := s.Store.GetReportDefinition(r.Context(), p.TenantID(), req.DefinitionID)
		if err == nil && d.ProjectID != pr.ID {
			err = fmt.Errorf("report definition %s: %w", req.DefinitionID, store.ErrNotFound)
		}
		if err != nil {
			return job, err
		}
		c, err := report.ParseCustomConfig(d.Config)
		if err != nil {
			return job, reportError(err)
		}
		if c.Title == "" {
			c.Title = d.Name
		}
		job.custom, job.stem = c, d.Name
	}
	return job, nil
}

// handleEmailReport: POST /projects/{pid}/reports/email
// {"template"+"params" | "custom" | "definition_id", "format", "recipients": [user ids],
// "addresses": [...], "message"}. The report is rendered now and e-mailed
// in the background → 202 {"queued": n, "skipped": [{user_id, reason}]}. A
// failed delivery is reported to the sender as an in-app notification.
func (s *Server) handleEmailReport(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !s.emailOn() {
		s.fail(w, r, errEmailOff())
		return
	}
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req reportMailReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	addresses, err := cleanAddresses(req.Addresses)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(addresses) > 0 && !p.Can(rbac.ProjectUpdate) {
		writeError(w, http.StatusForbidden, "forbidden", "permission project.update required to send to addresses outside the organisation's members")
		return
	}
	if len(req.Recipients)+len(addresses) == 0 {
		s.fail(w, r, badRequest("choose at least one recipient"))
		return
	}
	if len(req.Recipients)+len(addresses) > maxReportRecipients {
		s.fail(w, r, badRequest("at most %d recipients", maxReportRecipients))
		return
	}
	if len(req.Message) > 4000 {
		s.fail(w, r, badRequest("message is too long (at most 4000 characters)"))
		return
	}
	job, err := s.jobOf(r, p, pr, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	to, skipped, err := s.resolveRecipients(r.Context(), p.TenantID(), pr.ID, job, req.Recipients, addresses)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(to) == 0 {
		s.fail(w, r, badRequest("none of the recipients may read this report"))
		return
	}
	if !s.mailLimiter.allow("report|" + p.User.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many report e-mails, try again later")
		return
	}
	rep, err := s.render(r, p, pr, job)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m := s.reportMail(p, pr, rep, req.Format, req.Message, false)
	m.To = to
	tenantID, user, title, lang := p.TenantID(), p.User.ID, rep.title, rep.lang
	s.sendMail(r.Context(), m, func(err error) {
		if err == nil {
			return
		}
		n := model.Notification{UserID: user, ProjectID: pr.ID, Event: "report.email_failed", Title: tr(lang, "report.failed") + ": " + title, Body: err.Error()}
		if err := s.Store.AddNotifications(context.Background(), tenantID, []model.Notification{n}); err != nil {
			s.Log.Warn("notification", "err", err)
		}
	})
	s.audit(r.Context(), p, "report.email", "project", pr.ID, fmt.Sprintf("%s to %d recipients (%s)", rep.details, len(to), strings.Join(m.Recipients(), ", ")))
	if skipped == nil {
		skipped = []skippedRecipient{}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": len(to), "skipped": skipped})
}

// ---------------------------------------------------------------- subscriptions

type subscriptionReq struct {
	DefinitionID *string   `json:"definition_id"`
	Frequency    *string   `json:"frequency"`
	Weekday      *int      `json:"weekday"`
	DayOfMonth   *int      `json:"day_of_month"`
	Time         *string   `json:"time"`
	Timezone     *string   `json:"timezone"`
	Format       *string   `json:"format"`
	Recipients   *[]string `json:"recipients"`
	Addresses    *[]string `json:"addresses"`
	Message      *string   `json:"message"`
	Enabled      *bool     `json:"enabled"`
	// Lang is the language the report is sent in (the requester's UI
	// language); "" (or an unsupported language) keeps the stored one.
	Lang *string `json:"lang"`
}

// apply sets the given fields on x. The language is the body's "lang", else
// the "lang" query parameter (unsupported or missing: unchanged).
func (req subscriptionReq) apply(r *http.Request, x *model.ReportSubscription) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	set(&x.Frequency, req.Frequency)
	set(&x.TimeOfDay, req.Time)
	set(&x.Timezone, req.Timezone)
	set(&x.Format, req.Format)
	set(&x.Message, req.Message)
	if req.Weekday != nil {
		x.Weekday = *req.Weekday
	}
	if req.DayOfMonth != nil {
		x.DayOfMonth = *req.DayOfMonth
	}
	if req.Recipients != nil {
		x.Recipients = uniqStrings(*req.Recipients)
	}
	if req.Addresses != nil {
		x.Addresses = *req.Addresses
	}
	if req.Enabled != nil {
		x.Enabled = *req.Enabled
	}
	lang := r.URL.Query().Get("lang")
	if req.Lang != nil && report.NormalizeLang(*req.Lang) != "" {
		lang = *req.Lang
	}
	if l := report.NormalizeLang(lang); l != "" {
		x.Language = l
	}
}

// checkSubscription validates format and recipients for the caller.
func (s *Server) checkSubscription(r *http.Request, p *Principal, x *model.ReportSubscription, addressesChanged bool) error {
	f, err := export.ParseFormat(x.Format)
	if err != nil {
		return badRequest("%v", err)
	}
	x.Format = string(f)
	if x.Addresses, err = cleanAddresses(x.Addresses); err != nil {
		return err
	}
	if x.Addresses == nil {
		x.Addresses = []string{}
	}
	if addressesChanged && len(x.Addresses) > 0 && !p.Can(rbac.ProjectUpdate) {
		return fmt.Errorf("%w: permission project.update required to send to addresses outside the organisation's members", errForbidden)
	}
	rs, err := s.readers(r.Context(), p.TenantID(), x.ProjectID, []rbac.Permission{rbac.ProjectRead})
	if err != nil {
		return err
	}
	for _, id := range x.Recipients {
		if _, ok := rs[id]; !ok {
			return badRequest("recipient %s is not a member who may read the project", id)
		}
	}
	if x.Recipients == nil {
		x.Recipients = []string{}
	}
	// the next run, from the server's clock
	x.NextRunAt = nil
	if next, err := x.NextRun(s.now()); err == nil && x.Enabled {
		x.NextRunAt = &next
	}
	return nil
}

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListReportSubscriptions(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": len(list), "email_enabled": s.emailOn()})
}

// handleCreateSubscription: POST /projects/{pid}/reports/subscriptions
// {"definition_id", "frequency", "weekday"?, "day_of_month"?, "time",
// "timezone", "format", "recipients", "addresses"?, "message"?, "enabled"?}.
// The caller becomes the owner: the report is generated with their
// permissions when it is sent.
func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !canWriteReports(w, p) {
		return
	}
	if p.Token != nil {
		writeError(w, http.StatusForbidden, "forbidden", "subscriptions cannot be created with an API token")
		return
	}
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req subscriptionReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.DefinitionID == nil {
		s.fail(w, r, badRequest("definition_id is required"))
		return
	}
	d, err := s.Store.GetReportDefinition(r.Context(), p.TenantID(), *req.DefinitionID)
	if err == nil && d.ProjectID != pr.ID {
		err = fmt.Errorf("report definition: %w", store.ErrNotFound)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	x := &model.ReportSubscription{DefinitionID: d.ID, ProjectID: d.ProjectID, OwnerID: p.User.ID, OwnerEmail: p.User.Email, Frequency: model.FrequencyWeekly,
		Weekday: 1, DayOfMonth: 1, TimeOfDay: "07:00", Timezone: "UTC", Format: "pdf", Enabled: true}
	req.apply(r, x)
	if err := s.checkSubscription(r, p, x, true); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateReportSubscription(r.Context(), p.TenantID(), x); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "report_subscription.create", "report_subscription", x.ID, d.Name+" "+x.Frequency)
	writeJSON(w, http.StatusCreated, x)
}

// subscription loads {id}; only its owner or a project manager
// (project.update) may change it.
func (s *Server) subscription(w http.ResponseWriter, r *http.Request, p *Principal, write bool) (*model.ReportSubscription, bool) {
	x, err := s.Store.GetReportSubscription(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	if write && x.OwnerID != p.User.ID && !p.Can(rbac.ProjectUpdate) {
		writeError(w, http.StatusForbidden, "forbidden", "only the owner of the subscription or a project manager can change it")
		return nil, false
	}
	return x, true
}

func (s *Server) handleUpdateSubscription(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !canWriteReports(w, p) {
		return
	}
	x, ok := s.subscription(w, r, p, true)
	if !ok {
		return
	}
	var req subscriptionReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.DefinitionID != nil && *req.DefinitionID != x.DefinitionID {
		s.fail(w, r, badRequest("the report of a subscription cannot be changed; create a new one"))
		return
	}
	before := slices.Clone(x.Addresses)
	req.apply(r, x)
	added := false
	for _, a := range x.Addresses {
		added = added || !slices.Contains(before, store.NormalizeEmail(a))
	}
	if err := s.checkSubscription(r, p, x, added); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateReportSubscription(r.Context(), p.TenantID(), x); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "report_subscription.update", "report_subscription", x.ID, x.DefinitionName)
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !canWriteReports(w, p) {
		return
	}
	x, ok := s.subscription(w, r, p, true)
	if !ok {
		return
	}
	if err := s.Store.DeleteReportSubscription(r.Context(), p.TenantID(), x.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "report_subscription.delete", "report_subscription", x.ID, x.DefinitionName)
	w.WriteHeader(http.StatusNoContent)
}

// handleSendSubscription: POST /reports/subscriptions/{id}/send sends the
// report now (waiting for the SMTP server) and returns the subscription
// with the outcome; the schedule is not changed.
func (s *Server) handleSendSubscription(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !s.emailOn() {
		s.fail(w, r, errEmailOff())
		return
	}
	x, ok := s.subscription(w, r, p, true)
	if !ok {
		return
	}
	if !s.mailLimiter.allow("report|" + p.User.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many report e-mails, try again later")
		return
	}
	err := s.sendSubscription(r.Context(), p.TenantID(), x)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if rerr := s.Store.RecordReportRun(r.Context(), x.ID, s.now(), msg, x.NextRunAt); rerr != nil {
		s.fail(w, r, rerr)
		return
	}
	s.audit(r.Context(), p, "report_subscription.send", "report_subscription", x.ID, x.DefinitionName+" "+orOK(msg))
	x, err = s.Store.GetReportSubscription(r.Context(), p.TenantID(), x.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, x)
}

func orOK(msg string) string {
	if msg == "" {
		return "sent"
	}
	return "failed: " + msg
}

// ownerPrincipal is the owner of a subscription as a principal of the
// tenant, with their current permissions in the project.
func (s *Server) ownerPrincipal(ctx context.Context, tenantID, userID, projectID string) (*Principal, error) {
	p, err := s.principalFor(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("the owner: %w", err)
	}
	if projectID != "" {
		acc, err := s.projectAccess(ctx, p, projectID)
		if err != nil {
			return nil, err
		}
		if acc == nil {
			return nil, errors.New("the owner may no longer see the project")
		}
		p.scope(projectID, acc)
	}
	if !p.Can(rbac.Export) {
		return nil, errors.New("the owner may no longer export reports")
	}
	return p, nil
}

// sendSubscription renders the saved report as its owner and e-mails it.
func (s *Server) sendSubscription(ctx context.Context, tenantID string, x *model.ReportSubscription) error {
	p, err := s.ownerPrincipal(ctx, tenantID, x.OwnerID, x.ProjectID)
	if err != nil {
		return err
	}
	pr, err := s.Store.GetProject(ctx, tenantID, x.ProjectID)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	if err != nil {
		return err
	}
	// the language stored with the subscription, else the owner's preference
	job, err := s.jobOf(r, p, pr, reportMailReq{DefinitionID: x.DefinitionID, Format: x.Format, Lang: x.Language})
	if err != nil {
		return err
	}
	addresses := x.Addresses
	if len(addresses) > 0 && !p.Can(rbac.ProjectUpdate) {
		addresses = nil // the owner lost the right to send outside the members
	}
	to, _, err := s.resolveRecipients(ctx, tenantID, x.ProjectID, job, x.Recipients, addresses)
	if err != nil {
		return err
	}
	if len(to) == 0 {
		return errors.New("none of the recipients may read this report any more")
	}
	rep, err := s.render(r, p, pr, job)
	if err != nil {
		return err
	}
	m := s.reportMail(p, pr, rep, x.Format, x.Message, true)
	m.To = to
	return s.deliverMail(ctx, m)
}

// RunReportSubscriptions sends the scheduled reports that are due and
// returns how many ran. A subscription that missed several runs (server
// down) is sent once; its next run is computed from now.
func (s *Server) RunReportSubscriptions(ctx context.Context) (int, error) {
	if !s.emailOn() {
		return 0, nil
	}
	now := s.now()
	due, err := s.Store.DueReportSubscriptions(ctx, now)
	if err != nil {
		return 0, err
	}
	for i := range due {
		x := &due[i]
		var next *time.Time
		if t, err := x.NextRun(now); err == nil {
			next = &t
		}
		// record the next run first, so that a crash does not send again
		if err := s.Store.StartReportRun(ctx, x.ID, now, next); err != nil {
			return i, err
		}
		msg := ""
		if err := s.sendSubscription(ctx, x.TenantID, &x.ReportSubscription); err != nil {
			msg = err.Error()
			s.Log.Warn("scheduled report", "subscription", x.ID, "err", err)
		}
		if err := s.Store.RecordReportRun(ctx, x.ID, now, msg, next); err != nil {
			return i, err
		}
	}
	return len(due), nil
}
