// Package api implements the HTTP server: REST API, authentication
// flows, security middleware and serving of the embedded web client.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/live"
	"github.com/rom/qtestframe/internal/logging"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/runner"
	"github.com/rom/qtestframe/internal/store"
)

// Version is reported by /healthz; set by main.
var Version = "dev"

// MaxBodyBytes limits JSON request bodies.
const MaxBodyBytes = 8 << 20

// Server holds dependencies of the HTTP handlers.
type Server struct {
	Store  *store.Store
	Cfg    config.Config
	Log    *slog.Logger
	Runner *runner.Runner
	UI     fs.FS // embedded SPA (may be nil)
	// Catalog is the requirement and test catalogue served (nil = the
	// built-in one, catalog.Default).
	Catalog *catalog.Catalog
	// HTTPClient is used for outgoing calls (defect trackers, webhooks);
	// nil means a default client with a timeout.
	HTTPClient *http.Client

	OIDC   map[string]*auth.OIDCProvider
	SAML   map[string]*auth.SAMLProvider
	signer *auth.Signer

	limiter *rateLimiter
	// mfaLimiter bounds the second-factor attempts of one account.
	mfaLimiter *rateLimiter
	// passkeyLimiter bounds the passwordless sign-in challenges one client
	// address asks for.
	passkeyLimiter *rateLimiter
	// scimLimiter bounds the requests of one SCIM token, scimAuthLimiter
	// the failed SCIM authentications of one client address.
	scimLimiter     *rateLimiter
	scimAuthLimiter *rateLimiter
	mux             *http.ServeMux
	once            sync.Once
	// routeTable records every registered "METHOD /path" pattern (see
	// Routes); the OpenAPI test compares it with docs/openapi.yaml.
	routeTable []Route
	// hooks tracks webhook deliveries in flight (see WaitWebhooks).
	hooks sync.WaitGroup

	// Mail sends e-mail; nil when e-mail is not configured (email.host).
	Mail mail.Sender
	// Clock is the time of the e-mail jobs (tests); nil = time.Now.
	Clock func() time.Time
	// mails tracks e-mails in flight (see WaitMail).
	mails       sync.WaitGroup
	mailLimiter *rateLimiter
	// suggestLimiter bounds the autocomplete queries of one user (suggest.go).
	suggestLimiter *rateLimiter
	// aiLimit bounds the AI requests of one user per hour (ai.go).
	aiOnce  sync.Once
	aiLimit *rateLimiter

	// jobs runs automated tests in the background (scriptjobs.go);
	// JobHeartbeat is its heartbeat interval (tests; 0 = 15s).
	jobs         *jobManager
	JobHeartbeat time.Duration

	// Logs, when set, applies logging changes of the system settings at
	// runtime (nil: they are stored and apply at the next start).
	Logs *logging.Manager
	// live fans out change events and presence (live.go)
	live *live.Hub
	// liveReauth is how often a stream re-authenticates (tests; 0 = 1 min)
	liveReauth time.Duration

	// system settings (systemsettings.go): live is the effective
	// configuration (nil = Cfg, no overrides), boot the one at start-up.
	sys sysState
	// erec caches the electronic-records settings per tenant (erecords.go)
	erec erecCache
	// obs holds the metrics and the tracer (observability.go)
	obs *observability
	// bk is the state of the scheduled backups (backups.go)
	bk backupState
}

// New creates a server and its identity providers. cfg is the
// configuration of the file and the environment; the system settings
// stored in the database are applied over it (see systemsettings.go).
func New(st *store.Store, cfg config.Config, log *slog.Logger, r *runner.Runner, ui fs.FS) (*Server, error) {
	registerCustomFieldExchange()
	s := &Server{
		Store: st, Cfg: cfg, Log: log, Runner: r, UI: ui,
		OIDC:    map[string]*auth.OIDCProvider{},
		SAML:    map[string]*auth.SAMLProvider{},
		signer:  auth.NewSigner(nil),
		limiter: newRateLimiter(10, 5*time.Minute),
		// second-factor attempts per account, whatever the address
		mfaLimiter: newRateLimiter(10, 15*time.Minute),
		// passwordless challenges per address (the sign-in page asks for one
		// on every visit, for the passkey suggestions of the e-mail field)
		passkeyLimiter: newRateLimiter(60, 5*time.Minute),
		// identity providers sync in bursts (Entra ID: initial cycle)
		scimLimiter:     newRateLimiter(1200, time.Minute),
		scimAuthLimiter: newRateLimiter(20, 5*time.Minute),
		// report e-mails per user (no open relay)
		mailLimiter: newRateLimiter(30, 10*time.Minute),
		// autocomplete queries per user (typing is debounced in the client)
		suggestLimiter: newRateLimiter(300, time.Minute),
		jobs:           newJobManager(),
	}
	s.obs = newObservability(s)
	// outgoing calls follow the outbound settings (outbound.go)
	s.HTTPClient = s.outboundClient()
	// e-mail, OIDC providers and login limits follow the system settings
	s.initSettings(context.Background())
	// the limits of live updates are set up now, from the settings at
	// start-up (live.* restart settings)
	lc := s.sys.boot.Live
	s.live = live.NewHub(live.Options{MaxConnections: lc.MaxConnections, MaxPerUser: lc.MaxConnectionsPerUser, Replay: lc.ReplayBuffer, PresenceTTL: lc.PresenceTTL})
	st.OnNotifications(s.liveNotified)
	base := strings.TrimSuffix(cfg.Server.BaseURL, "/")
	for _, sc := range cfg.Auth.SAML {
		p, err := auth.NewSAMLProvider(sc, base+"/api/v1/auth/saml/"+url.PathEscape(sc.Name))
		if err != nil {
			return nil, err
		}
		s.SAML[sc.Name] = p
	}
	return s, nil
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	s.once.Do(s.routes)
	return s.recoverer(s.securityHeaders(s.observe(s.logRequests(s.mux))))
}

func (s *Server) routes() {
	m := http.NewServeMux()
	s.mux = m
	h := func(pattern string, fn handlerFunc, perm ...string) {
		s.routeTable = append(s.routeTable, Route{Pattern: pattern, Permissions: perm})
		m.Handle(pattern, s.authed(fn, perm...))
	}
	pub := func(pattern string, fn http.HandlerFunc) {
		s.routeTable = append(s.routeTable, Route{Pattern: pattern, Public: true})
		m.HandleFunc(pattern, fn)
	}
	pub("GET /healthz", s.handleHealth)
	pub("GET /readyz", s.handleReady)
	// Prometheus metrics (observability.go): off by default, its own protection
	pub("GET /metrics", s.handleMetrics)
	pub("GET /api/v1/openapi.json", s.handleOpenAPI)
	pub("GET /api/v1/openapi.yaml", s.handleOpenAPI)

	// auth
	pub("GET /api/v1/auth/providers", s.handleProviders)
	pub("POST /api/v1/auth/login", s.handleLogin)
	pub("POST /api/v1/auth/logout", s.handleLogout)
	// the second factor of a sign-in (mfa.go): the qtf_mfa cookie is the credential
	pub("GET /api/v1/auth/mfa/challenge", s.handleMFAChallenge)
	pub("POST /api/v1/auth/mfa/verify", s.handleMFAVerify)
	// passkeys (passkeys.go): passwordless sign-in and the second factor
	s.routesPasskeysPublic(pub)
	// e-mail invitations (invitations.go): the token is the credential
	pub("GET /api/v1/invitations/{token}", s.handleGetInvitation)
	pub("POST /api/v1/invitations/{token}/accept", s.handleAcceptInvitation)
	pub("GET /api/v1/auth/oidc/{name}/login", s.handleOIDCLogin)
	pub("GET /api/v1/auth/oidc/{name}/callback", s.handleOIDCCallback)
	pub("GET /api/v1/auth/saml/{name}/metadata", s.handleSAMLMetadata)
	// GitLab and GitHub webhooks (code.go): the signature or secret token is the credential
	s.routesCodeHooks(pub)
	// Jira and Azure DevOps webhooks of the requirement sync (reqsync.go): the shared secret is the credential
	s.routesReqSyncHooks(pub)
	// CI/CD pipeline templates (ci.go): no secrets in them
	s.routesCIPublic(pub)
	pub("GET /api/v1/auth/saml/{name}/login", s.handleSAMLLogin)
	pub("POST /api/v1/auth/saml/{name}/acs", s.handleSAMLACS)
	h("GET /api/v1/auth/me", s.handleMe)
	h("POST /api/v1/auth/switch-tenant", s.handleSwitchTenant)
	h("POST /api/v1/auth/password", s.handleChangePassword)
	h("PATCH /api/v1/auth/me", s.handleUpdateMe)
	h("GET /api/v1/auth/sessions", s.handleListSessions)
	h("DELETE /api/v1/auth/sessions/{id}", s.handleDeleteSession)
	h("POST /api/v1/auth/sessions/revoke-others", s.handleRevokeOtherSessions)

	// admin
	h("GET /api/v1/meta", s.handleMeta)
	// live updates and presence (live.go)
	h("GET /api/v1/events/stream", s.handleEventStream)
	h("POST /api/v1/presence", s.handlePresence)
	h("GET /api/v1/presence", s.handleGetPresence)
	h("GET /api/v1/rbac", s.handleRBAC)
	h("GET /api/v1/tenants", s.handleListTenants)
	h("POST /api/v1/tenants", s.handleCreateTenant)
	h("PATCH /api/v1/tenants/{id}", s.handleUpdateTenant)
	h("DELETE /api/v1/tenants/{id}", s.handleDeleteTenant)
	h("GET /api/v1/users", s.handleListUsers)
	h("POST /api/v1/users", s.handleCreateUser)
	h("PATCH /api/v1/users/{id}", s.handleUpdateUser)
	h("GET /api/v1/members", s.handleListMembers, "member.read")
	h("POST /api/v1/members", s.atomic(s.handleAddMember), "member.manage")
	h("POST /api/v1/members/bulk", s.atomic(s.handleBulkMembers), "member.manage")
	h("PATCH /api/v1/members/{uid}", s.handleUpdateMember, "member.manage")
	h("DELETE /api/v1/members/{uid}", s.handleRemoveMember, "member.manage")
	h("POST /api/v1/members/{uid}/reset-password", s.handleResetPassword, "member.manage")
	h("GET /api/v1/tokens", s.handleListTokens, "token.manage")
	h("POST /api/v1/tokens", s.handleCreateToken, "token.manage")
	h("DELETE /api/v1/tokens/{id}", s.handleDeleteToken, "token.manage")
	h("GET /api/v1/audit", s.handleAudit, "audit.read")

	// projects
	h("GET /api/v1/projects", s.handleListProjects) // the projects the caller may read: see access.go
	h("POST /api/v1/projects", s.handleCreateProject, "project.create")
	h("GET /api/v1/projects/{pid}", s.handleGetProject, "project.read")
	h("PATCH /api/v1/projects/{pid}", s.handleUpdateProject, "project.update")
	h("DELETE /api/v1/projects/{pid}", s.handleDeleteProject, "project.delete")
	h("GET /api/v1/projects/{pid}/dashboard", s.handleDashboard, "project.read")

	// folders & categories
	h("GET /api/v1/projects/{pid}/folders", s.handleListFolders, "project.read")
	h("POST /api/v1/projects/{pid}/folders", s.handleCreateFolder)
	h("POST /api/v1/projects/{pid}/folders/bulk", s.atomic(s.handleBulkFolders))
	h("PATCH /api/v1/folders/{id}", s.handleUpdateFolder)
	h("POST /api/v1/folders/{id}/move", s.handleMoveFolder)
	h("DELETE /api/v1/folders/{id}", s.handleDeleteFolder)
	h("GET /api/v1/projects/{pid}/categories", s.handleListCategories, "project.read")
	h("POST /api/v1/projects/{pid}/categories", s.handleCreateCategory)
	h("POST /api/v1/projects/{pid}/categories/move", s.handleMoveDimension)
	h("POST /api/v1/projects/{pid}/categories/bulk", s.atomic(s.handleBulkCategories))
	h("PATCH /api/v1/categories/{id}", s.handleUpdateCategory)
	h("DELETE /api/v1/categories/{id}", s.handleDeleteCategory)

	// requirements
	h("GET /api/v1/projects/{pid}/requirements", s.handleListRequirements, "requirement.read")
	h("GET /api/v1/projects/{pid}/people", s.handlePeople, "requirement.read")
	h("GET /api/v1/projects/{pid}/analytics", s.handleAnalytics, "requirement.read")
	h("GET /api/v1/projects/{pid}/analytics/compliance", s.handleCompliance, "requirement.read")
	h("GET /api/v1/projects/{pid}/analytics/quality", s.handleQuality, "project.read")
	h("POST /api/v1/projects/{pid}/prompts", s.handlePrompt, "project.read") // read of the items' kind: see prompts.go
	h("GET /api/v1/projects/{pid}/requirement-sources", s.handleRequirementSources, "requirement.read")
	h("POST /api/v1/projects/{pid}/tags/suggest", s.handleSuggestTags, "requirement.read")
	h("POST /api/v1/projects/{pid}/requirements", s.handleCreateRequirement, "requirement.write")
	h("POST /api/v1/projects/{pid}/requirements/move", s.handleMoveItems(model.KindRequirement), "requirement.write")
	h("POST /api/v1/projects/{pid}/requirements/bulk", s.impactAware(s.handleBulk(model.KindRequirement), true), "requirement.read")
	h("POST /api/v1/projects/{pid}/requirements/batch", s.impactAware(s.handleBatchUpdate(model.KindRequirement), true), "requirement.read") // write or review per item: see batch.go
	h("POST /api/v1/projects/{pid}/requirements/review", s.atomic(s.handleBulkReview(model.KindRequirement)), "requirement.read")
	h("DELETE /api/v1/projects/{pid}/requirements/bulk", s.atomic(s.handleBulkDelete(model.KindRequirement)), "requirement.write")
	h("GET /api/v1/requirements/{id}", s.handleGetRequirement, "requirement.read")
	h("PATCH /api/v1/requirements/{id}", s.impactAware(s.handleUpdateRequirement, false), "requirement.read") // write or review: see review.go
	h("POST /api/v1/requirements/{id}/copy", s.handleCopyRequirement, "requirement.write")
	h("DELETE /api/v1/requirements/{id}", s.handleDeleteRequirement, "requirement.write")
	h("GET /api/v1/requirements/{id}/links", s.handleItemLinks(model.KindRequirement), "requirement.read")
	h("GET /api/v1/requirements/{id}/history", s.handleHistory(model.KindRequirement), "requirement.read")
	h("POST /api/v1/requirements/{id}/restore", s.impactAware(s.handleRestoreRequirement, false), "requirement.write")
	h("GET /api/v1/requirements/{id}/relations", s.handleListRequirementRelations, "requirement.read")
	h("POST /api/v1/requirements/{id}/relations", s.handleCreateRelation, "requirement.write")
	h("DELETE /api/v1/relations/{id}", s.handleDeleteRelation, "requirement.write")
	h("GET /api/v1/projects/{pid}/relations", s.handleListProjectRelations, "requirement.read")
	h("POST /api/v1/projects/{pid}/relations/bulk", s.atomic(s.handleBulkCreateRelations), "requirement.write")
	h("DELETE /api/v1/projects/{pid}/relations/bulk", s.atomic(s.handleBulkDeleteRelations), "requirement.write")
	h("GET /api/v1/projects/{pid}/baselines", s.handleListBaselines, "requirement.read")
	h("POST /api/v1/projects/{pid}/baselines", s.handleCreateBaseline, "requirement.write")
	h("DELETE /api/v1/projects/{pid}/baselines/bulk", s.atomic(s.handleBulkDeleteBaselines), "requirement.write")
	h("POST /api/v1/projects/{pid}/baselines/signoffs", s.atomic(s.handleBulkSignBaselines), "requirement.read")
	h("GET /api/v1/baselines/{id}", s.handleGetBaseline, "requirement.read")
	h("GET /api/v1/baselines/{id}/compare", s.handleCompareBaseline, "requirement.read")
	h("POST /api/v1/baselines/{id}/restore", s.impactAware(s.handleRestoreFromBaseline, true), "requirement.read")
	h("DELETE /api/v1/baselines/{id}", s.handleDeleteBaseline, "requirement.write")
	h("POST /api/v1/reqlang/analyze", s.handleAnalyze)

	// tests
	h("GET /api/v1/projects/{pid}/tests", s.handleListTests, "test.read")
	h("POST /api/v1/projects/{pid}/tests", s.handleCreateTest, "test.write")
	h("POST /api/v1/projects/{pid}/tests/move", s.handleMoveItems(model.KindTest), "test.write")
	h("POST /api/v1/projects/{pid}/tests/bulk", s.impactAware(s.handleBulk(model.KindTest), true), "test.read")
	h("POST /api/v1/projects/{pid}/tests/batch", s.impactAware(s.handleBatchUpdate(model.KindTest), true), "test.read")
	h("POST /api/v1/projects/{pid}/tests/review", s.atomic(s.handleBulkReview(model.KindTest)), "test.read")
	h("DELETE /api/v1/projects/{pid}/tests/bulk", s.atomic(s.handleBulkDelete(model.KindTest)), "test.write")
	h("GET /api/v1/tests/{id}", s.handleGetTest, "test.read")
	h("PATCH /api/v1/tests/{id}", s.impactAware(s.handleUpdateTest, false), "test.read") // write or review: see review.go
	h("DELETE /api/v1/tests/{id}", s.handleDeleteTest, "test.write")
	h("GET /api/v1/requirements/{id}/item-links", s.handleListItemLinks(model.KindRequirement), "requirement.read")
	h("POST /api/v1/requirements/{id}/item-links", s.handleCreateItemLink(model.KindRequirement), "requirement.write")
	h("GET /api/v1/tests/{id}/item-links", s.handleListItemLinks(model.KindTest), "test.read")
	h("POST /api/v1/tests/{id}/item-links", s.handleCreateItemLink(model.KindTest), "test.write")
	h("GET /api/v1/runs/{id}/item-links", s.handleListItemLinks("run"), "run.read")
	h("POST /api/v1/runs/{id}/item-links", s.handleCreateItemLink("run"), "run.write")
	h("PATCH /api/v1/item-links/{id}", s.handleUpdateItemLink, "project.read") // write of the source: see itemlinks.go
	h("DELETE /api/v1/item-links/{id}", s.handleDeleteItemLink, "project.read")
	h("GET /api/v1/projects/{pid}/item-links", s.handleListProjectItemLinks, "project.read")
	h("POST /api/v1/projects/{pid}/item-links/bulk", s.atomic(s.handleBulkCreateItemLinks), "project.read") // write of each source: see bulkitemlinks.go
	h("DELETE /api/v1/projects/{pid}/item-links/bulk", s.atomic(s.handleBulkDeleteItemLinks), "project.read")
	h("GET /api/v1/projects/{pid}/trash", s.handleListTrash, "project.read")
	h("POST /api/v1/trash/{id}/restore", s.handleRestoreTrash, "project.read") // write of the kind: see trash.go
	h("POST /api/v1/tests/{id}/copy", s.handleCopyTest, "test.write")
	h("GET /api/v1/tests/{id}/links", s.handleItemLinks(model.KindTest), "test.read")
	h("GET /api/v1/tests/{id}/history", s.handleHistory(model.KindTest), "test.read")
	h("POST /api/v1/tests/{id}/restore", s.impactAware(s.handleRestoreTest, false), "test.write")
	h("GET /api/v1/tests/{id}/executions", s.handleTestExecutions, "run.read")

	// traceability
	h("GET /api/v1/projects/{pid}/links", s.handleListLinks, "project.read")
	h("POST /api/v1/projects/{pid}/links", s.handleCreateLink, "test.write")
	h("DELETE /api/v1/links/{id}", s.handleDeleteLink, "test.write")
	h("POST /api/v1/projects/{pid}/links/bulk", s.atomic(s.handleBulkCreateLinks), "test.write")
	h("DELETE /api/v1/projects/{pid}/links/bulk", s.atomic(s.handleBulkDeleteLinks), "test.write")
	h("POST /api/v1/projects/{pid}/links/confirm", s.atomic(s.handleBulkConfirmLinks), "test.write")
	h("POST /api/v1/links/{id}/confirm", s.handleConfirmLink, "test.write")
	h("POST /api/v1/requirements/{id}/links/confirm", s.handleConfirmItemLinks(model.KindRequirement), "test.write")
	h("POST /api/v1/tests/{id}/links/confirm", s.handleConfirmItemLinks(model.KindTest), "test.write")
	h("GET /api/v1/projects/{pid}/traceability", s.handleTraceability, "project.read")

	// operations
	h("GET /api/v1/projects/{pid}/runs", s.handleListRuns, "run.read")
	h("POST /api/v1/projects/{pid}/runs", s.handleCreateRun, "run.write")
	h("POST /api/v1/projects/{pid}/runs/bulk", s.atomic(s.handleBulkRuns), "run.write")
	h("DELETE /api/v1/projects/{pid}/runs/bulk", s.atomic(s.handleBulkDeleteRuns), "run.write")
	h("GET /api/v1/runs/{id}", s.handleGetRun, "run.read")
	h("PATCH /api/v1/runs/{id}", s.atomic(s.handleUpdateRun), "run.write")
	h("DELETE /api/v1/runs/{id}", s.handleDeleteRun, "run.write")
	h("GET /api/v1/runs/{id}/executions", s.handleListExecutions, "run.read")
	h("POST /api/v1/runs/{id}/executions", s.handleAddExecutions, "run.write")
	h("POST /api/v1/runs/{id}/executions/bulk", s.atomic(s.handleBulkExecutions), "run.read")
	h("POST /api/v1/runs/{id}/results", s.handleImportResults, "run.execute")
	h("PUT /api/v1/runs/{id}/configurations", s.atomic(s.handleSetRunConfigurations), "run.write")
	h("GET /api/v1/executions/{id}", s.handleGetExecution, "run.read")
	h("PATCH /api/v1/executions/{id}", s.handleUpdateExecution, "run.execute")
	h("POST /api/v1/executions/{id}/run-script", s.handleRunScript, "script.execute")

	// search & export
	h("GET /api/v1/search", s.handleSearch) // in the projects the caller may read: see access.go
	h("GET /api/v1/projects/{pid}/export", s.handleExport, "export")
	h("POST /api/v1/projects/{pid}/export/view", s.handleExportView, "export")

	// files, attachments, archive, avatars (files.go)
	s.routesFiles(h)
	// reports (reports.go) and backup/restore (backup.go)
	s.routesReports(h)
	s.routesBackup(h)
	s.routesRoles(h)
	s.routesDependencies(h)
	s.routesReleases(h)
	s.routesCycles(h)
	s.routesEnvironments(h)
	s.routesTestData(h)
	s.routesMetrics(h)
	s.routesDashboards(h)
	s.routesDefects(h)
	s.routesRequirementIssues(h)
	s.routesRisks(h)
	s.routesWaivers(h)
	s.routesChangeRequests(h)
	s.routesSoA(h)
	s.routesNotifications(h)
	s.routesWorkList(h)
	// discussions on items (discussions.go)
	s.routesDiscussions(h)
	s.routesChat(h)
	s.routesReviews(h)
	s.routesJUnit(h)
	s.routesReqImport(h)
	// Excel round-trip and ReqIF import (exchange.go), OSLC (oslc.go)
	s.routesExchange(h)
	s.routesRequirementSources(h)
	// two-way sync with Jira and Azure DevOps (reqsync.go)
	s.routesReqSync(h)
	s.routesOSLC(pub, h)
	// SCIM provisioning: its own token authentication (scim.go)
	s.routesSCIM(m)
	s.routesSCIMAdmin(h)
	s.routesTraceRules(h)
	s.routesEmail(h)
	s.routesConfigurations(h)
	s.routesVerification(h)
	// variants and applicability (variants.go), code and builds (code.go)
	s.routesVariants(h)
	s.routesCode(h)
	s.routesCI(h)
	s.routesStubs(h)
	s.routesGherkin(h)
	s.routesCatalog(h)
	s.routesWebTests(h)
	s.routesProjectArchive(h)
	s.routesMFA(h)
	s.routesPasskeys(h)
	s.routesSharing(h)
	// autocomplete suggestions and history (suggest.go)
	s.routesSuggest(h)
	// Administration → System settings (systemsettings.go)
	s.routesSystemSettings(h)
	s.routesSuperAdmin(h)
	// change and defect correlations (correlations.go)
	s.routesCorrelations(h)
	// custom fields, field sets and item templates (customfields.go)
	s.routesCustomFields(h)
	// exploratory testing sessions (exploratory.go)
	s.routesExploratory(h)
	// similar requirements and tests, duplicates and conflicts (similarity.go)
	s.routesSimilarity(h)
	// optional AI assistance (ai.go)
	s.routesAI(h)
	// tamper-evident audit trail and electronic-records mode (erecords.go)
	s.routesERecords(h)
	// the logotype of reports and generated documents (logo.go)
	s.routesLogo(h)
	// the solution preamble of software projects (solution.go)
	s.routesSolution(h)

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	m.HandleFunc("/", s.handleUI)
}

// ---------------------------------------------------------------- health

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": Version})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.DB.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "database unavailable")
		return
	}
	// the schema must be complete (e.g. another process is still migrating)
	if !s.obs.migrated.Load() {
		n, err := s.Store.DB.Pending(ctx)
		switch {
		case err != nil:
			writeError(w, http.StatusServiceUnavailable, "unavailable", "database unavailable")
			return
		case n > 0:
			writeError(w, http.StatusServiceUnavailable, "unavailable", fmt.Sprintf("%d database migrations are not applied", n))
			return
		}
		s.obs.migrated.Store(true)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "checks": map[string]string{"database": "ok", "migrations": "ok"}})
}

// ---------------------------------------------------------------- UI

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
		return
	}
	if s.UI == nil {
		http.Error(w, "web client not built (run `make web`)", http.StatusNotFound)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	if st, err := fs.Stat(s.UI, p); err != nil || st.IsDir() {
		p = "index.html" // SPA history fallback
	}
	f, err := s.UI.Open(p)
	if err != nil {
		http.Error(w, "web client not built (run `make web`)", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, p, st.ModTime(), rs)
}

// ---------------------------------------------------------------- middleware

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.Log.Error("panic", "err", fmt.Sprint(v), "path", r.URL.Path, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CSP allows only same-origin resources; PrimeVue injects styles at
// runtime, hence style-src 'unsafe-inline'.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self' https:; object-src 'none'"

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if s.conf().Server.HSTS && r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the connection (flushing
// server-sent events, a longer write deadline).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if quietPath(r.URL.Path) {
			return
		}
		lvl := slog.LevelInfo
		if sw.status >= 500 {
			lvl = slog.LevelError
		}
		s.Log.Log(r.Context(), lvl, "http request", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "bytes", sw.bytes, "duration_ms", time.Since(start).Milliseconds(),
			"remote", s.clientIP(r))
	})
}

// clientIP returns the client address, honouring X-Forwarded-For only from
// trusted proxies.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" && s.trustedProxy(host) {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return host
}

func (s *Server) trustedProxy(ip string) bool {
	addr := net.ParseIP(ip)
	for _, p := range s.conf().Server.TrustedProxies {
		if _, n, err := net.ParseCIDR(p); err == nil && addr != nil && n.Contains(addr) {
			return true
		}
		if p == ip {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Error    string `json:"error"`
	Code     string `json:"code"`
	Position *int   `json:"position,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Error: msg, Code: code})
}

// fail maps an error to an HTTP response.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var qe *query.Error
	var ae *apiErr
	var cb *changeBlocked
	switch {
	case errors.As(err, &cb):
		writeChangeBlocked(w, cb)
	case errors.As(err, &ae):
		writeError(w, ae.status, ae.code, ae.msg)
	case errors.As(err, &qe):
		pos := qe.Pos
		// err may name the query it belongs to ("selector: query: …")
		writeJSON(w, http.StatusBadRequest, apiError{Error: err.Error(), Code: "invalid_query", Position: &pos})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, store.ErrInvalid), errors.Is(err, errBadRequest), errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, runner.ErrDisabled), errors.Is(err, runner.ErrWebDisabled):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	default:
		s.Log.Error("request failed", "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

var (
	errBadRequest = errors.New("bad request")
	errForbidden  = errors.New("forbidden")
)

func badRequest(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errBadRequest, fmt.Sprintf(format, a...))
}

// decode reads a JSON body into v.
func decode(r *http.Request, v any) error {
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

func readBody(r *http.Request) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	if err != nil {
		return nil, badRequest("reading body: %v", err)
	}
	if len(b) > MaxBodyBytes {
		return nil, badRequest("request body too large")
	}
	return b, nil
}

// patchKeys returns the top-level keys present in a JSON object.
func patchKeys(body []byte) (map[string]bool, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, badRequest("invalid JSON: %v", err)
	}
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out, nil
}

type list[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

func listOf[T any](items []T) list[T] {
	if items == nil {
		items = []T{}
	}
	return list[T]{Items: items, Total: len(items)}
}

// ---------------------------------------------------------------- rate limiting

type rateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

// setLimits changes the limits (system settings); 0 keeps a value.
func (l *rateLimiter) setLimits(max int, window time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if max > 0 {
		l.max = max
	}
	if window > 0 {
		l.window = window
	}
}

// allow records an attempt for key and reports whether it is permitted.
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-l.window)
	h := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			h = append(h, t)
		}
	}
	if len(h) >= l.max {
		l.hits[key] = h
		return false
	}
	l.hits[key] = append(h, now)
	if len(l.hits) > 10000 { // bound memory
		for k, v := range l.hits {
			if len(v) == 0 || v[len(v)-1].Before(cut) {
				delete(l.hits, k)
			}
		}
	}
	return true
}

// blocked reports whether key has used up its attempts, without recording
// one.
func (l *rateLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := time.Now().Add(-l.window)
	n := 0
	for _, t := range l.hits[key] {
		if t.After(cut) {
			n++
		}
	}
	return n >= l.max
}

func (l *rateLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}
