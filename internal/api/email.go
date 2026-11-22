package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Outgoing e-mail: notification mails (immediately or as a daily digest),
// the test e-mail of administrators and the server jobs that send digests
// and scheduled reports. Invitations are in invitations.go, report mails in
// reportmail.go.

func (s *Server) routesEmail(h routeFunc) {
	h("POST /api/v1/admin/email/test", s.handleTestEmail, "tenant.manage")
	s.routesInvitations(h)
	s.routesReportMail(h)
}

// mailTimeout bounds one delivery.
func (s *Server) mailTimeout() time.Duration {
	if t := s.conf().Email.Timeout; t > 0 {
		return t
	}
	return 30 * time.Second
}

// emailOn reports whether the server can send e-mail.
func (s *Server) emailOn() bool { return s.mailer() != nil }

// errEmailOff is the answer when a feature needs e-mail that is not set up.
func errEmailOff() error {
	return badRequest("e-mail is not configured on this server (see the email section of the configuration)")
}

// now returns the time of the server's jobs (Clock in tests).
func (s *Server) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// baseURL is server.base_url without a trailing slash.
func (s *Server) baseURL() string { return strings.TrimSuffix(s.conf().Server.BaseURL, "/") }

func (s *Server) from() mail.Address {
	return mail.Address{Name: s.conf().Email.FromName, Email: s.conf().Email.From}
}

// message builds an e-mail from content.
func (s *Server) message(to []mail.Address, subject string, c mailContent) *mail.Message {
	c.Title = strings.TrimSpace(c.Title)
	return &mail.Message{From: s.from(), To: to, Subject: oneLineText(subject, 200), Text: c.text(), HTML: c.html()}
}

// oneLineText removes line breaks and shortens s (subjects).
func oneLineText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	return excerpt(s, n)
}

// sendMail sends m in the background once the request transaction of ctx
// is committed (at once outside a transaction); never when it is rolled
// back. done, when given, gets the outcome.
func (s *Server) sendMail(ctx context.Context, m *mail.Message, done func(error)) {
	if s.mailer() == nil {
		return
	}
	store.AfterCommit(ctx, func() {
		s.mails.Add(1)
		go func() {
			defer s.mails.Done()
			err := s.deliverMail(context.Background(), m)
			if done != nil {
				done(err)
			}
		}()
	})
}

// deliverMail sends m now, with the configured timeout, and logs failures.
func (s *Server) deliverMail(ctx context.Context, m *mail.Message) error {
	ctx, cancel := context.WithTimeout(ctx, s.mailTimeout())
	defer cancel()
	m2 := s.mailer()
	if m2 == nil {
		return errEmailOff()
	}
	err := m2.Send(ctx, m)
	if err != nil {
		s.Log.Warn("e-mail not sent", "to", strings.Join(m.Recipients(), ","), "subject", m.Subject, "err", err)
	} else {
		s.Log.Info("e-mail sent", "to", strings.Join(m.Recipients(), ","), "subject", m.Subject)
	}
	return err
}

// WaitMail waits for e-mails in flight (tests, shutdown).
func (s *Server) WaitMail() { s.mails.Wait() }

func userAddress(u *model.User) mail.Address {
	return mail.Address{Name: oneLineText(u.DisplayName, 100), Email: u.Email}
}

// ---------------------------------------------------------------- notifications

// notificationURL is the link of a notification (see web
// src/lib/notifications.ts notificationTarget).
func (s *Server) notificationURL(n *model.Notification) string {
	base := s.baseURL()
	if n.ProjectID == "" {
		return base + "/notifications"
	}
	p := base + "/p/" + url.PathEscape(n.ProjectID)
	q := url.QueryEscape(n.EntityID)
	switch n.EntityType {
	case model.KindRequirement:
		return p + "/requirements?item=" + q
	case model.KindTest:
		return p + "/tests?item=" + q
	case model.KindDefect:
		return p + "/defects?item=" + q
	case "risk":
		return p + "/risks?risk=" + q
	case "execution":
		return p + "/runs"
	case model.DiscussionRun:
		return p + "/runs/" + url.PathEscape(n.EntityID)
	case model.DiscussionReview:
		return p + "/reviews"
	case "chat_channel":
		return p + "/settings"
	}
	return p + "/dashboard"
}

// prefsLink leads to the notification settings in the profile.
func (s *Server) prefsLink(lang string) *mailLink {
	return &mailLink{Text: tr(lang, "footer.prefsLink"), URL: s.baseURL() + "/profile#email"}
}

func (s *Server) withPrefsFooter(m *mail.Message) *mail.Message {
	m.Headers = map[string]string{"List-Unsubscribe": "<" + s.baseURL() + "/profile#email>", "Auto-Submitted": "auto-generated"}
	return m
}

// mailNotifications e-mails stored notifications to the recipients who
// chose immediate e-mails. The actor never gets an e-mail about their own
// action.
func (s *Server) mailNotifications(ctx context.Context, p *Principal, list []model.Notification) {
	if !s.emailOn() {
		return
	}
	for i := range list {
		n := list[i]
		if n.UserID == p.User.ID {
			continue
		}
		u, err := s.Store.GetUser(ctx, n.UserID)
		if err != nil || u.Disabled || u.Preferences.EmailNotifications != model.EmailImmediate {
			continue
		}
		lang := u.Preferences.Language
		c := mailContent{Lang: mailLang(lang), Title: n.Title, Intro: []string{tr(lang, "by", n.Actor)},
			Items:  []mailItem{{Title: n.Title, Body: excerpt(n.Body, 1000)}},
			Action: &mailLink{Text: tr(lang, "notification.open"), URL: s.notificationURL(&n)},
			Footer: []string{tr(lang, "footer.prefs")}, FooterLink: s.prefsLink(lang)}
		c.Items[0].Title = eventLabel(lang, n.Event)
		m := s.withPrefsFooter(s.message([]mail.Address{userAddress(u)}, n.Title, c))
		id := n.ID
		s.sendMail(ctx, m, func(err error) {
			if err == nil {
				if err := s.Store.MarkNotificationsEmailed(context.Background(), []string{id}); err != nil {
					s.Log.Warn("notification e-mail record", "err", err)
				}
			}
		})
	}
}

// eventLabel names a notification event in an e-mail.
func eventLabel(lang, event string) string {
	sv := mailLang(lang) == "sv"
	labels := map[string][2]string{
		"mention":          {"You were mentioned", "Du har nämnts"},
		"comment.created":  {"New comment", "Ny kommentar"},
		"comment.resolved": {"Discussion resolved", "Diskussionen är löst"},
		"assigned":         {"Assigned to you", "Tilldelad dig"},
		"defect.created":   {"New defect", "Ny defekt"},
		"defect.assigned":  {"Defect assigned to you", "Defekt tilldelad dig"},
		"defect.status":    {"Defect status changed", "Defektens status ändrad"},
		"review.requested": {"Review requested", "Granskning begärd"},
		"review.completed": {"Review completed", "Granskning klar"},
		"review.reminder":  {"Review reminder", "Påminnelse om granskning"},
		"waiver.requested": {"Waiver to approve", "Undantag att godkänna"},
		"waiver.approved":  {"Waiver approved", "Undantag godkänt"},
		"waiver.rejected":  {"Waiver rejected", "Undantag avslaget"},
		"waiver.revoked":   {"Waiver revoked", "Undantag återkallat"},
		"waiver.expired":   {"Waiver expired", "Undantag har gått ut"},
		"project.shared":   {"A project was shared with you", "Ett projekt har delats med dig"},
		"impact.review":    {"A change affects your items: review recommended", "En ändring påverkar dina poster: granskning rekommenderas"},
		"chat.disabled":    {"A chat channel was turned off", "En chattkanal har stängts av"},
	}
	l, ok := labels[event]
	if !ok {
		return event
	}
	if sv {
		return l[1]
	}
	return l[0]
}

// ---------------------------------------------------------------- digest

const (
	digestJob   = "email.digest"
	digestLimit = 50
)

// digestDue returns the most recent scheduled digest time at or before now.
func (s *Server) digestDue(now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), s.conf().Email.DigestHour, 0, 0, 0, time.UTC)
	if t.After(now) {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

// RunDigest sends the daily digest when it is due (once per day; after
// downtime once, not for every missed day) and reports whether it ran.
func (s *Server) RunDigest(ctx context.Context) (bool, error) {
	if !s.emailOn() {
		return false, nil
	}
	now := s.now()
	last, err := s.Store.JobLastRun(ctx, digestJob)
	if err != nil {
		return false, err
	}
	if last != nil && !last.Before(s.digestDue(now)) {
		return false, nil
	}
	// record first, so that a crash while sending does not send twice
	if err := s.Store.SetJobLastRun(ctx, digestJob, now); err != nil {
		return false, err
	}
	users, err := s.Store.UsersWithEmailMode(ctx, model.EmailDigest)
	if err != nil {
		return true, err
	}
	for i := range users {
		s.sendDigest(ctx, &users[i])
	}
	return true, nil
}

func (s *Server) sendDigest(ctx context.Context, u *model.User) {
	list, err := s.Store.DigestNotifications(ctx, u.ID, digestLimit+1)
	if err != nil {
		s.Log.Warn("digest", "user", u.Email, "err", err)
		return
	}
	if len(list) == 0 {
		return
	}
	lang := u.Preferences.Language
	more := len(list) > digestLimit
	if more {
		list = list[:digestLimit]
	}
	c := mailContent{Lang: mailLang(lang), Intro: []string{tr(lang, "digest.intro")}, Footer: []string{tr(lang, "footer.prefs")}, FooterLink: s.prefsLink(lang)}
	ids := make([]string, 0, len(list))
	for i := range list {
		n := &list[i].Notification
		ids = append(ids, n.ID)
		c.Items = append(c.Items, mailItem{Title: n.Title, Meta: eventLabel(lang, n.Event) + " · " + tr(lang, "by", n.Actor) + " · " + mailDate(lang, n.CreatedAt),
			Body: excerpt(n.Body, 300), URL: s.notificationURL(n)})
	}
	if more {
		c.After = []string{tr(lang, "digest.more")}
	}
	c.Action = &mailLink{Text: tr(lang, "digest.all"), URL: s.baseURL() + "/notifications"}
	subject := tr(lang, "digest.subject", len(list))
	if len(list) == 1 {
		subject = tr(lang, "digest.subject1")
	}
	c.Title = subject
	m := s.withPrefsFooter(s.message([]mail.Address{userAddress(u)}, subject, c))
	if err := s.deliverMail(ctx, m); err != nil {
		return
	}
	if err := s.Store.MarkNotificationsEmailed(ctx, ids); err != nil {
		s.Log.Warn("digest record", "err", err)
	}
}

// ---------------------------------------------------------------- jobs

// RunJobs runs the e-mail jobs that are due: the daily digest and the
// scheduled reports.
func (s *Server) RunJobs(ctx context.Context) {
	// expired waivers no longer accept their requirements (also without e-mail)
	if _, err := s.RunWaiverExpiry(ctx); err != nil {
		s.Log.Warn("waiver expiry job", "err", err)
	}
	// chat digests and retries (also without e-mail)
	if _, err := s.RunChatJobs(ctx); err != nil {
		s.Log.Warn("chat job", "err", err)
	}
	// CI status posts to the code host that wait for a retry
	if _, err := s.RunCIJobs(ctx); err != nil {
		s.Log.Warn("ci job", "err", err)
	}
	// scheduled two-way syncs with Jira and Azure DevOps
	if _, err := s.RunReqSyncJobs(ctx); err != nil {
		s.Log.Warn("requirement sync job", "err", err)
	}
	// scheduled cycle starts and recurring cycles (also without e-mail)
	if _, err := s.RunCycleJobs(ctx); err != nil {
		s.Log.Warn("test cycle job", "err", err)
	}
	if !s.emailOn() {
		return
	}
	if _, err := s.RunDigest(ctx); err != nil {
		s.Log.Warn("digest job", "err", err)
	}
	if _, err := s.RunReportSubscriptions(ctx); err != nil {
		s.Log.Warn("report subscriptions job", "err", err)
	}
}

// StartJobs runs RunJobs every minute until ctx ends (RunJobs does
// nothing while e-mail is off; it can be turned on in the system settings).
func (s *Server) StartJobs(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		s.RunJobs(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.RunJobs(ctx)
			}
		}
	}()
}

// ---------------------------------------------------------------- test e-mail

// handleTestEmail: POST /admin/email/test sends a test e-mail to the
// caller's own address now and reports the SMTP error, if any.
func (s *Server) handleTestEmail(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !s.emailOn() {
		s.fail(w, r, errEmailOff())
		return
	}
	lang := p.User.Preferences.Language
	who := p.User.DisplayName
	if who == "" {
		who = p.User.Email
	}
	c := mailContent{Lang: mailLang(lang), Title: tr(lang, "test.subject"), Intro: []string{tr(lang, "test.intro", who)},
		Action: &mailLink{Text: tr(lang, "added.action"), URL: s.baseURL() + "/"}}
	m := s.message([]mail.Address{userAddress(p.User)}, tr(lang, "test.subject"), c)
	err := s.deliverMail(r.Context(), m)
	res := map[string]any{"ok": err == nil, "to": p.User.Email}
	details := p.User.Email
	if err != nil {
		res["error"] = err.Error()
		details += ": " + err.Error()
	}
	s.audit(r.Context(), p, "email.test", "user", p.User.ID, details)
	writeJSON(w, http.StatusOK, res)
}
