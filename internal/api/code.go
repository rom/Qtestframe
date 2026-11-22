package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Code and builds: commits and merge / pull requests linked to
// requirements and tests (automatically from GitLab and GitHub webhooks,
// or by hand), CI builds of uploaded results and the regression selection
// (see docs/api.md "Code and builds").

// MaxHookBytes limits the body of an incoming webhook.
const MaxHookBytes = 5 << 20

// maxHookCommits bounds the commits of one push that are processed.
const maxHookCommits = 1000

func (s *Server) routesCode(h routeFunc) {
	h("GET /api/v1/projects/{pid}/code-settings", s.handleGetCodeSettings, "project.read")
	h("PUT /api/v1/projects/{pid}/code-settings", s.handlePutCodeSettings, "project.update")
	h("GET /api/v1/projects/{pid}/code-changes", s.handleListCodeChanges, "project.read")
	h("GET /api/v1/projects/{pid}/builds", s.handleListBuilds, "run.read")
	h("POST /api/v1/projects/{pid}/builds", s.handleCreateBuild, "run.execute")
	h("GET /api/v1/builds/{id}", s.handleGetBuild, "run.read")
	h("DELETE /api/v1/builds/{id}", s.handleDeleteBuild, "run.write")
	h("GET /api/v1/requirements/{id}/code", s.handleItemCode(model.KindRequirement), "requirement.read")
	h("POST /api/v1/requirements/{id}/code", s.handleAddCodeLink(model.KindRequirement), "requirement.write")
	h("DELETE /api/v1/requirements/{id}/code/{cid}", s.handleRemoveCodeLink(model.KindRequirement), "requirement.write")
	h("GET /api/v1/tests/{id}/code", s.handleItemCode(model.KindTest), "test.read")
	h("POST /api/v1/tests/{id}/code", s.handleAddCodeLink(model.KindTest), "test.write")
	h("DELETE /api/v1/tests/{id}/code/{cid}", s.handleRemoveCodeLink(model.KindTest), "test.write")
	h("GET /api/v1/tests/{id}/builds", s.handleTestBuilds, "test.read")
	h("POST /api/v1/projects/{pid}/regression", s.handleRegression, "test.read")
	h("POST /api/v1/projects/{pid}/regression/run", s.handleRegressionRun, "run.write")
}

// routesCodeHooks registers the public webhook endpoints: the signature
// (GitHub) or secret token (GitLab) is the credential.
func (s *Server) routesCodeHooks(pub func(string, http.HandlerFunc)) {
	pub("POST /api/v1/hooks/gitlab/{pid}", s.handleCodeHook(model.ProviderGitLab))
	pub("POST /api/v1/hooks/github/{pid}", s.handleCodeHook(model.ProviderGitHub))
}

// ---------------------------------------------------------------- settings

func (s *Server) hookURL(provider, projectID string) string {
	return s.baseURL() + "/api/v1/hooks/" + provider + "/" + projectID
}

func (s *Server) codeSettingsView(projectID string, cs *model.CodeSettings) *model.CodeSettings {
	out := *cs
	out.Secret, out.CIToken = "", ""
	out.GitLabURL, out.GitHubURL = s.hookURL(model.ProviderGitLab, projectID), s.hookURL(model.ProviderGitHub, projectID)
	return &out
}

func (s *Server) handleGetCodeSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	cs, err := s.Store.CodeSettings(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.codeSettingsView(pr.ID, cs))
}

func (s *Server) handlePutCodeSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in struct {
		Secret      string `json:"secret"`
		ClearSecret bool   `json:"clear_secret"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	cs, err := s.Store.SaveCodeSettings(r.Context(), p.TenantID(), pr.ID, in.Secret, in.ClearSecret, p.User.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	details := "webhook secret kept"
	switch {
	case in.ClearSecret:
		details = "webhook secret removed"
	case in.Secret != "":
		details = "webhook secret set"
	}
	s.audit(r.Context(), p, "project.code_settings", "project", pr.ID, details)
	writeJSON(w, http.StatusOK, s.codeSettingsView(pr.ID, cs))
}

// ---------------------------------------------------------------- builds

// buildFromQuery reads a build from the query parameters of a result
// upload (nil when none is given).
func buildFromQuery(r *http.Request) *model.Build {
	q := r.URL.Query()
	b := &model.Build{ExternalID: q.Get("build_id"), Number: q.Get("build_number"), CommitSHA: q.Get("commit"),
		Branch: q.Get("branch"), PipelineURL: q.Get("pipeline_url")}
	if b.ExternalID == "" && b.Number == "" && b.CommitSHA == "" && b.Branch == "" && b.PipelineURL == "" {
		return nil
	}
	return b
}

// applyRunBuild associates a run with the build (and variant) given in the
// query of a result upload, creating the build when it is new.
func (s *Server) applyRunBuild(r *http.Request, p *Principal, run *model.TestRun) (*model.Build, error) {
	b := buildFromQuery(r)
	variant := strings.TrimSpace(r.URL.Query().Get("variant"))
	if b == nil && variant == "" {
		return nil, nil
	}
	var variantID *string
	if variant != "" {
		v, err := s.Store.FindVariant(r.Context(), p.TenantID(), run.ProjectID, variant)
		if err != nil {
			return nil, err
		}
		variantID = &v.ID
	}
	if b != nil {
		if b.ExternalID == "" && b.Number == "" {
			return nil, badRequest("build_id or build_number is required with commit, branch or pipeline_url")
		}
		b.ProjectID, b.CreatedBy = run.ProjectID, p.User.Email
		created, err := s.Store.UpsertBuild(r.Context(), p.TenantID(), b)
		if err != nil {
			return nil, err
		}
		if created {
			s.audit(r.Context(), p, "build.create", "build", b.ID, b.Label())
		}
	}
	if err := s.Store.SetRunBuild(r.Context(), p.TenantID(), run.ID, b, variantID); err != nil {
		return nil, err
	}
	if b != nil {
		run.BuildID = &b.ID
	}
	if variantID != nil {
		run.VariantID = variantID
	}
	return b, nil
}

func (s *Server) handleListBuilds(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Store.ListBuilds(r.Context(), p.TenantID(), pr.ID, limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleCreateBuild(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var b model.Build
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	b.ID, b.ProjectID, b.CreatedBy = "", pr.ID, p.User.Email
	created, err := s.Store.UpsertBuild(r.Context(), p.TenantID(), &b)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		s.audit(r.Context(), p, "build.create", "build", b.ID, b.Label())
	}
	out, err := s.Store.GetBuild(r.Context(), p.TenantID(), b.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out)
}

func (s *Server) handleGetBuild(w http.ResponseWriter, r *http.Request, p *Principal) {
	b, err := s.Store.GetBuild(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	runs, err := s.Store.ListRuns(r.Context(), p.TenantID(), b.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in := []model.TestRun{}
	for _, run := range runs {
		if run.BuildID != nil && *run.BuildID == b.ID {
			in = append(in, run)
		}
	}
	changes := []model.CodeChange{}
	if b.CommitSHA != "" {
		all, err := s.Store.ListCodeChanges(r.Context(), p.TenantID(), b.ProjectID, store.CodeChangeFilter{Kind: model.CodeCommit, Limit: store.MaxCodeListLimit})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, c := range all {
			if c.Ref == b.CommitSHA {
				changes = append(changes, c)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"build": b, "runs": in, "changes": changes})
}

func (s *Server) handleDeleteBuild(w http.ResponseWriter, r *http.Request, p *Principal) {
	b, err := s.Store.GetBuild(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteBuild(r.Context(), p.TenantID(), b.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "build.delete", "build", b.ID, b.Label())
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- code changes

func (s *Server) handleListCodeChanges(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	kind := q.Get("kind")
	if kind != "" && kind != model.CodeCommit && kind != model.CodeMergeRequest {
		s.fail(w, r, badRequest("kind must be commit or merge_request"))
		return
	}
	var list []model.CodeChange
	var err error
	if ref := q.Get("since_build"); ref != "" {
		b, ferr := s.findBuild(r, p, pr.ID, ref)
		if ferr != nil {
			s.fail(w, r, ferr)
			return
		}
		list, err = s.Store.ChangesSinceBuild(r.Context(), p.TenantID(), b)
	} else {
		list, err = s.Store.ListCodeChanges(r.Context(), p.TenantID(), pr.ID, store.CodeChangeFilter{Kind: kind, Branch: q.Get("branch"), Limit: limit})
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// findBuild finds a build of a project by id, CI id or number.
func (s *Server) findBuild(r *http.Request, p *Principal, projectID, ref string) (*model.Build, error) {
	ref = strings.TrimSpace(ref)
	if b, err := s.Store.GetBuild(r.Context(), p.TenantID(), ref); err == nil && b.ProjectID == projectID {
		return b, nil
	}
	list, err := s.Store.ListBuilds(r.Context(), p.TenantID(), projectID, store.MaxCodeListLimit)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ExternalID == ref || list[i].Number == ref {
			return &list[i], nil
		}
	}
	return nil, badRequest("build %q not found in project", ref)
}

// itemCode is the "Code" panel of a requirement or test.
type itemCode struct {
	Changes []model.CodeChange `json:"changes"`
	// Builds: requirement — the builds a linked test ran in, with the
	// requirement's coverage there; test — its build history.
	Builds any `json:"builds"`
	// VerifiedBuild: the latest build in which the requirement was
	// verified (every linked test passed); requirements only.
	VerifiedBuild *model.Build `json:"verified_build,omitempty"`
}

func (s *Server) handleItemCode(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		ctx := r.Context()
		id := r.PathValue("id")
		pid, _, err := s.itemProject(r.Context(), p, kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out := itemCode{Builds: []any{}}
		if out.Changes, err = s.Store.ListCodeChanges(ctx, p.TenantID(), pid, store.CodeChangeFilter{ItemType: kind, ItemID: id, Limit: store.MaxCodeListLimit}); err != nil {
			s.fail(w, r, err)
			return
		}
		if p.Can(rbac.RunRead) {
			bi, err := s.Store.LoadBuildIndex(ctx, p.TenantID(), pid)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			if kind == model.KindTest {
				out.Builds = bi.TestHistory(id)
			} else {
				links, err := s.Store.ListLinks(ctx, p.TenantID(), pid, id, "")
				if err != nil {
					s.fail(w, r, err)
					return
				}
				var tests []string
				for _, l := range links {
					tests = append(tests, l.TestCaseID)
				}
				out.Builds, out.VerifiedBuild = bi.RequirementBuilds(id, tests, nil)
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Server) handleTestBuilds(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.Store.GetTest(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !p.Can(rbac.RunRead) {
		writeError(w, http.StatusForbidden, "forbidden", "permission run.read required")
		return
	}
	bi, err := s.Store.LoadBuildIndex(r.Context(), p.TenantID(), t.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(bi.TestHistory(t.ID)))
}

// providerOf guesses the provider of a pasted URL.
func providerOf(u string) string {
	switch {
	case strings.Contains(u, "github"):
		return model.ProviderGitHub
	case strings.Contains(u, "gitlab"):
		return model.ProviderGitLab
	}
	return model.ProviderManual
}

func (s *Server) handleAddCodeLink(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		pid, key, err := s.itemProject(r.Context(), p, kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var in struct {
			// ChangeID links an existing change; otherwise the fields
			// describe a commit or merge request to add.
			ChangeID string `json:"change_id"`
			model.CodeChange
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		var c *model.CodeChange
		if in.ChangeID != "" {
			if c, err = s.Store.GetCodeChange(r.Context(), p.TenantID(), in.ChangeID); err != nil || c.ProjectID != pid {
				s.fail(w, r, badRequest("code change %q not found in project", in.ChangeID))
				return
			}
		} else {
			c = &in.CodeChange
			c.ID, c.ProjectID = "", pid
			if c.Kind == "" {
				c.Kind = model.CodeCommit
			}
			// a hand-made link is "manual" unless it names its provider
			c.Provider = strings.TrimSpace(c.Provider)
			if c.Provider == "" {
				c.Provider = model.ProviderManual
			}
			if _, err := s.Store.UpsertCodeChange(r.Context(), p.TenantID(), c); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		if _, err := s.Store.LinkCodeChange(r.Context(), p.TenantID(), c.ID, []model.CodeChangeItem{{ItemType: kind, ItemID: id}}, model.CodeLinkManual, p.User.Email); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, kind+".code_link", kind, id, key+" → "+c.Kind+" "+c.Ref)
		out, err := s.Store.GetCodeChange(r.Context(), p.TenantID(), c.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

func (s *Server) handleRemoveCodeLink(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		_, key, err := s.itemProject(r.Context(), p, kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		c, err := s.Store.GetCodeChange(r.Context(), p.TenantID(), r.PathValue("cid"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.Store.UnlinkCodeChange(r.Context(), p.TenantID(), c.ID, kind, id); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, kind+".code_unlink", kind, id, key+" ↛ "+c.Kind+" "+c.Ref)
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---------------------------------------------------------------- webhooks

// hookChange is a code change of a webhook with the text its keys are
// read from.
type hookChange struct {
	change model.CodeChange
	text   string
}

func parseHookTime(v string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 MST", "2006-01-02 15:04:05 -0700"} {
		if t, err := time.Parse(layout, strings.TrimSpace(v)); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// verifyHook checks the credential of a webhook request.
func verifyHook(provider, secret string, r *http.Request, body []byte) bool {
	if secret == "" {
		return false
	}
	if provider == model.ProviderGitLab {
		tok := r.Header.Get("X-Gitlab-Token")
		return tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(secret)) == 1
	}
	sig, ok := strings.CutPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

type hookCommit struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
	Author    struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"author"`
}

func pushChanges(provider, repo, ref string, commits []hookCommit) []hookChange {
	branch := strings.TrimPrefix(ref, "refs/heads/")
	if strings.HasPrefix(ref, "refs/tags/") {
		branch = ""
	}
	var out []hookChange
	for i, c := range commits {
		if i >= maxHookCommits {
			break
		}
		author := c.Author.Name
		if author == "" {
			author = c.Author.Username
		}
		out = append(out, hookChange{change: model.CodeChange{Kind: model.CodeCommit, Provider: provider, Repository: repo, Ref: c.ID,
			Title: c.Message, URL: c.URL, Author: author, Branch: branch, OccurredAt: parseHookTime(c.Timestamp)}, text: c.Message})
	}
	return out
}

// parseHook turns a GitLab or GitHub event into code changes; ok is false
// for events that are not handled.
func parseHook(provider string, r *http.Request, body []byte) ([]hookChange, string, error) {
	if provider == model.ProviderGitHub {
		event := r.Header.Get("X-GitHub-Event")
		switch event {
		case "ping":
			return nil, event, nil
		case "push":
			var ev struct {
				Ref        string       `json:"ref"`
				Commits    []hookCommit `json:"commits"`
				Repository struct {
					FullName string `json:"full_name"`
				} `json:"repository"`
			}
			if err := json.Unmarshal(body, &ev); err != nil {
				return nil, event, badRequest("invalid push event: %v", err)
			}
			return pushChanges(provider, ev.Repository.FullName, ev.Ref, ev.Commits), event, nil
		case "pull_request":
			var ev struct {
				PR struct {
					Number    int    `json:"number"`
					Title     string `json:"title"`
					Body      string `json:"body"`
					HTMLURL   string `json:"html_url"`
					State     string `json:"state"`
					Merged    bool   `json:"merged"`
					CreatedAt string `json:"created_at"`
					User      struct {
						Login string `json:"login"`
					} `json:"user"`
					Head struct {
						Ref string `json:"ref"`
					} `json:"head"`
					Base struct {
						Ref string `json:"ref"`
					} `json:"base"`
				} `json:"pull_request"`
				Repository struct {
					FullName string `json:"full_name"`
				} `json:"repository"`
			}
			if err := json.Unmarshal(body, &ev); err != nil || ev.PR.Number == 0 {
				return nil, event, badRequest("invalid pull_request event")
			}
			st := "open"
			if ev.PR.State == "closed" {
				st = "closed"
				if ev.PR.Merged {
					st = "merged"
				}
			}
			c := model.CodeChange{Kind: model.CodeMergeRequest, Provider: provider, Repository: ev.Repository.FullName, Ref: strconv.Itoa(ev.PR.Number),
				Title: ev.PR.Title, URL: ev.PR.HTMLURL, Author: ev.PR.User.Login, Branch: ev.PR.Head.Ref, TargetBranch: ev.PR.Base.Ref, Status: st,
				OccurredAt: parseHookTime(ev.PR.CreatedAt)}
			return []hookChange{{change: c, text: ev.PR.Title + "\n" + ev.PR.Body + "\n" + ev.PR.Head.Ref}}, event, nil
		}
		return nil, "", nil
	}
	var kind struct {
		ObjectKind string `json:"object_kind"`
	}
	if err := json.Unmarshal(body, &kind); err != nil {
		return nil, "", badRequest("invalid event: %v", err)
	}
	switch kind.ObjectKind {
	case "push":
		var ev struct {
			Ref     string       `json:"ref"`
			Commits []hookCommit `json:"commits"`
			Project struct {
				Path string `json:"path_with_namespace"`
			} `json:"project"`
		}
		if err := json.Unmarshal(body, &ev); err != nil {
			return nil, kind.ObjectKind, badRequest("invalid push event: %v", err)
		}
		return pushChanges(provider, ev.Project.Path, ev.Ref, ev.Commits), kind.ObjectKind, nil
	case "merge_request":
		var ev struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
			Project struct {
				Path string `json:"path_with_namespace"`
			} `json:"project"`
			Attrs struct {
				IID          int    `json:"iid"`
				Title        string `json:"title"`
				Description  string `json:"description"`
				URL          string `json:"url"`
				State        string `json:"state"`
				SourceBranch string `json:"source_branch"`
				TargetBranch string `json:"target_branch"`
				CreatedAt    string `json:"created_at"`
			} `json:"object_attributes"`
		}
		if err := json.Unmarshal(body, &ev); err != nil || ev.Attrs.IID == 0 {
			return nil, kind.ObjectKind, badRequest("invalid merge_request event")
		}
		st := "open"
		switch ev.Attrs.State {
		case "merged":
			st = "merged"
		case "closed":
			st = "closed"
		}
		c := model.CodeChange{Kind: model.CodeMergeRequest, Provider: provider, Repository: ev.Project.Path, Ref: strconv.Itoa(ev.Attrs.IID),
			Title: ev.Attrs.Title, URL: ev.Attrs.URL, Author: ev.User.Username, Branch: ev.Attrs.SourceBranch, TargetBranch: ev.Attrs.TargetBranch,
			Status: st, OccurredAt: parseHookTime(ev.Attrs.CreatedAt)}
		return []hookChange{{change: c, text: ev.Attrs.Title + "\n" + ev.Attrs.Description + "\n" + ev.Attrs.SourceBranch}}, kind.ObjectKind, nil
	}
	return nil, "", nil
}

// handleCodeHook receives GitLab (push, merge request) and GitHub (push,
// pull_request, ping) webhooks of a project: commits and MRs / PRs are
// recorded and linked to the requirements and tests whose keys their text
// mentions.
func (s *Server) handleCodeHook(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		body, err := io.ReadAll(io.LimitReader(r.Body, MaxHookBytes+1))
		if err != nil || len(body) > MaxHookBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "webhook body too large")
			return
		}
		pr, secret, err := s.Store.WebhookProject(ctx, r.PathValue("pid"))
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, err)
			return
		}
		// unknown project, no secret and a wrong signature look the same
		if err != nil || !verifyHook(provider, secret, r, body) {
			writeError(w, http.StatusUnauthorized, "invalid_signature", "the webhook signature or token is not valid")
			return
		}
		changes, event, err := parseHook(provider, r, body)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if event == "" {
			writeJSON(w, http.StatusAccepted, map[string]any{"ignored": true})
			return
		}
		stored, linked := 0, 0
		for _, hc := range changes {
			c := hc.change
			c.ProjectID = pr.ID
			if _, err := s.Store.UpsertCodeChange(ctx, pr.TenantID, &c); err != nil {
				if errors.Is(err, store.ErrInvalid) {
					continue // e.g. a commit without a valid SHA
				}
				s.fail(w, r, err)
				return
			}
			stored++
			items, err := s.Store.ResolveItemKeys(ctx, pr.TenantID, pr.ID, store.ExtractItemKeys(hc.text, pr.Key))
			if err != nil {
				s.fail(w, r, err)
				return
			}
			n, err := s.Store.LinkCodeChange(ctx, pr.TenantID, c.ID, items, model.CodeLinkAuto, "")
			if err != nil {
				s.fail(w, r, err)
				return
			}
			linked += n
		}
		e := model.AuditEntry{TenantID: pr.TenantID, UserEmail: provider + " webhook", Action: "code.webhook", EntityType: "project", EntityID: pr.ID,
			Details: fmt.Sprintf("%s: %d change(s), %d new link(s)", event, stored, linked)}
		if err := s.Store.Audit(ctx, e); err != nil {
			s.Log.Warn("audit write failed", "err", err)
		}
		writeJSON(w, http.StatusOK, map[string]any{"event": event, "changes": stored, "links": linked})
	}
}

// ---------------------------------------------------------------- regression selection

type regressionReq struct {
	// RequirementIDs: changed requirements (ids or keys).
	RequirementIDs []string `json:"requirement_ids"`
	// ChangeIDs: code changes whose requirements and tests changed.
	ChangeIDs []string `json:"change_ids"`
	// SinceBuild: the commits after this build (id, CI id or number).
	SinceBuild string `json:"since_build"`
	// Variant: only requirements and tests that apply to it.
	Variant string `json:"variant"`
	// IncludeRelated adds the requirements that depend on, refine or are
	// derived from a changed requirement.
	IncludeRelated bool `json:"include_related"`
	// IncludePrerequisites adds the prerequisites of the selected tests
	// (default true).
	IncludePrerequisites *bool `json:"include_prerequisites"`
	// Commits: SHAs (or prefixes of at least 7 characters) of stored
	// commits; MergeRequest: the number of a stored merge / pull request.
	// Their requirements and tests changed.
	Commits      []string `json:"commits"`
	MergeRequest string   `json:"merge_request"`
	// Keys: requirement and test keys (full or short) that changed, e.g.
	// found in commit messages by qtestframe select; unknown keys are
	// answered in unknown.
	Keys []string `json:"keys"`
	// Tags: tests with one of these tags are always selected.
	Tags []string `json:"tags"`
	// All selects every test (e.g. a pipeline of the default branch).
	All bool `json:"all"`
	// run (POST /regression/run)
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Environment      string   `json:"environment"`
	Build            string   `json:"build"`
	ReleaseID        *string  `json:"release_id"`
	ConfigurationIDs []string `json:"configuration_ids"`
}

// regressionReason says why an item is selected: kind requirement (a
// changed requirement), change (named in a code change: ref), related
// (relation type: ref), verifies (verifies requirement ref) or
// prerequisite (of test ref).
type regressionReason struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref,omitempty"`
	Type string `json:"type,omitempty"`
}

type regressionItem struct {
	ID         string             `json:"id"`
	Key        string             `json:"key"`
	Title      string             `json:"title"`
	LastResult string             `json:"last_result,omitempty"`
	Reasons    []regressionReason `json:"reasons"`
}

type regressionSelection struct {
	Requirements []regressionItem   `json:"requirements"`
	Tests        []regressionItem   `json:"tests"`
	Changes      []model.CodeChange `json:"changes"`
	Variant      *model.VariantRef  `json:"variant,omitempty"`
	SinceBuild   *model.Build       `json:"since_build,omitempty"`
	// Unknown: keys that match no requirement or test of the project.
	Unknown []string `json:"unknown"`
}

func shortRef(c *model.CodeChange) string {
	if c.Kind == model.CodeCommit && len(c.Ref) > 8 {
		return c.Ref[:8]
	}
	if c.Kind == model.CodeMergeRequest {
		if c.Provider == model.ProviderGitHub {
			return "#" + c.Ref
		}
		return "!" + c.Ref
	}
	return c.Ref
}

// regression selects the tests to run after changes: the tests that
// verify the changed requirements (and, with include_related, the
// requirements that depend on, refine or derive from them), the tests
// named by the code changes and the prerequisites of all of them, without
// deprecated tests and, with a variant, only what applies to it.
func (s *Server) regression(r *http.Request, p *Principal, pr *model.Project, in *regressionReq) (*regressionSelection, error) {
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	sel := &regressionSelection{Requirements: []regressionItem{}, Tests: []regressionItem{}, Changes: []model.CodeChange{}, Unknown: []string{}}
	variant := ""
	if strings.TrimSpace(in.Variant) != "" {
		v := pc.variants.Find(in.Variant)
		if v == nil {
			return nil, badRequest("unknown variant %q", in.Variant)
		}
		variant, sel.Variant = v.ID, &model.VariantRef{ID: v.ID, Name: v.Name}
	}
	vc, err := s.variantCoverage(ctx, p.TenantID(), pc)
	if err != nil {
		return nil, err
	}
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	tests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	reqByID, reqByKey := map[string]*model.Requirement{}, map[string]*model.Requirement{}
	for i := range reqs {
		reqByID[reqs[i].ID], reqByKey[strings.ToUpper(reqs[i].Key)] = &reqs[i], &reqs[i]
	}
	testByID := map[string]*model.TestCase{}
	for i := range tests {
		testByID[tests[i].ID] = &tests[i]
	}
	reqReasons, testReasons := map[string][]regressionReason{}, map[string][]regressionReason{}
	var reqOrder, testOrder []string
	addReq := func(id string, why regressionReason) {
		if _, ok := reqByID[id]; !ok || !vc.AppliesTo(id, variant) {
			return
		}
		if _, ok := reqReasons[id]; !ok {
			reqOrder = append(reqOrder, id)
		}
		if !slices.Contains(reqReasons[id], why) {
			reqReasons[id] = append(reqReasons[id], why)
		}
	}
	addTest := func(id string, why regressionReason) bool {
		t, ok := testByID[id]
		if !ok || t.Status == "deprecated" || !vc.AppliesTo(id, variant) {
			return false
		}
		if _, ok := testReasons[id]; !ok {
			testOrder = append(testOrder, id)
		}
		if !slices.Contains(testReasons[id], why) {
			testReasons[id] = append(testReasons[id], why)
		}
		return true
	}
	for _, ref := range in.RequirementIDs {
		rq := reqByID[ref]
		if rq == nil {
			rq = reqByKey[strings.ToUpper(strings.TrimSpace(ref))]
		}
		if rq == nil {
			return nil, badRequest("requirement %q not found in project", ref)
		}
		addReq(rq.ID, regressionReason{Kind: "requirement"})
	}
	// code changes: explicit and since a build
	if in.SinceBuild != "" {
		b, err := s.findBuild(r, p, pr.ID, in.SinceBuild)
		if err != nil {
			return nil, err
		}
		sel.SinceBuild = b
		if sel.Changes, err = s.Store.ChangesSinceBuild(ctx, p.TenantID(), b); err != nil {
			return nil, err
		}
	}
	for _, id := range in.ChangeIDs {
		c, err := s.Store.GetCodeChange(ctx, p.TenantID(), id)
		if err != nil || c.ProjectID != pr.ID {
			return nil, badRequest("code change %q not found in project", id)
		}
		if !slices.ContainsFunc(sel.Changes, func(x model.CodeChange) bool { return x.ID == c.ID }) {
			sel.Changes = append(sel.Changes, *c)
		}
	}
	addChanges := func(list []model.CodeChange) {
		for _, c := range list {
			if !slices.ContainsFunc(sel.Changes, func(x model.CodeChange) bool { return x.ID == c.ID }) {
				sel.Changes = append(sel.Changes, c)
			}
		}
	}
	if len(in.Commits) > 0 {
		list, err := s.Store.CodeChangesByRef(ctx, p.TenantID(), pr.ID, model.CodeCommit, in.Commits)
		if err != nil {
			return nil, err
		}
		addChanges(list)
	}
	if mr := strings.TrimSpace(in.MergeRequest); mr != "" {
		list, err := s.Store.CodeChangesByRef(ctx, p.TenantID(), pr.ID, model.CodeMergeRequest, []string{mr})
		if err != nil {
			return nil, err
		}
		addChanges(list)
	}
	for i := range sel.Changes {
		c := &sel.Changes[i]
		why := regressionReason{Kind: "change", Ref: shortRef(c), Type: c.Kind}
		for _, it := range c.Items {
			if it.ItemType == model.KindRequirement {
				addReq(it.ItemID, why)
			} else {
				addTest(it.ItemID, why)
			}
		}
	}
	// keys (e.g. from commit messages) and always-run tags
	if len(in.Keys) > 0 {
		testByKey := map[string]*model.TestCase{}
		for i := range tests {
			testByKey[strings.ToUpper(tests[i].Key)] = &tests[i]
		}
		prefix := strings.ToUpper(pr.Key) + "-"
		sel.Unknown = []string{}
		for _, k := range in.Keys {
			k = strings.ToUpper(strings.TrimSpace(k))
			if k == "" {
				continue
			}
			full := k
			if !strings.HasPrefix(full, prefix) {
				full = prefix + k
			}
			why := regressionReason{Kind: "key", Ref: k}
			switch {
			case reqByKey[k] != nil:
				addReq(reqByKey[k].ID, why)
			case reqByKey[full] != nil:
				addReq(reqByKey[full].ID, why)
			case testByKey[k] != nil:
				addTest(testByKey[k].ID, why)
			case testByKey[full] != nil:
				addTest(testByKey[full].ID, why)
			default:
				if !slices.Contains(sel.Unknown, k) {
					sel.Unknown = append(sel.Unknown, k)
				}
			}
		}
	}
	if in.All {
		for i := range tests {
			addTest(tests[i].ID, regressionReason{Kind: "all"})
		}
	}
	for _, tag := range in.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		for i := range tests {
			if slices.ContainsFunc(tests[i].Tags, func(x string) bool { return strings.EqualFold(x, tag) }) {
				addTest(tests[i].ID, regressionReason{Kind: "tag", Ref: tag})
			}
		}
	}
	if in.IncludeRelated {
		seeds := append([]string{}, reqOrder...)
		for _, id := range seeds {
			for _, ref := range pc.refs[id] {
				if ref.Kind == model.KindRequirement && ref.Direction == "incoming" && slices.Contains([]string{"depends_on", "refines", "derived_from"}, ref.Type) {
					addReq(ref.ID, regressionReason{Kind: "related", Ref: reqByID[id].Key, Type: ref.Type})
				}
			}
		}
	}
	for _, id := range reqOrder {
		for _, tid := range pc.r2t[id] {
			addTest(tid, regressionReason{Kind: "verifies", Ref: reqByID[id].Key})
		}
	}
	if in.IncludePrerequisites == nil || *in.IncludePrerequisites {
		pre := map[string][]string{}
		for _, d := range pc.deps {
			pre[d.TestID] = append(pre[d.TestID], d.DependsOnID)
		}
		queue := append([]string{}, testOrder...)
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			for _, dep := range pre[id] {
				_, had := testReasons[dep]
				if addTest(dep, regressionReason{Kind: "prerequisite", Ref: testByID[id].Key}) && !had {
					queue = append(queue, dep)
				}
			}
		}
	}
	for _, id := range reqOrder {
		rq := reqByID[id]
		sel.Requirements = append(sel.Requirements, regressionItem{ID: id, Key: rq.Key, Title: rq.Title, Reasons: reqReasons[id]})
	}
	for _, id := range testOrder {
		t := testByID[id]
		last := vc.TestResult(id, variant)
		sel.Tests = append(sel.Tests, regressionItem{ID: id, Key: t.Key, Title: t.Title, LastResult: last, Reasons: testReasons[id]})
	}
	slices.SortStableFunc(sel.Requirements, func(a, b regressionItem) int { return keyCmp(a.Key, b.Key) })
	slices.SortStableFunc(sel.Tests, func(a, b regressionItem) int { return keyCmp(a.Key, b.Key) })
	return sel, nil
}

func keyCmp(a, b string) int {
	switch {
	case naturalKeyLess(a, b):
		return -1
	case naturalKeyLess(b, a):
		return 1
	}
	return 0
}

func (s *Server) handleRegression(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in regressionReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	sel, err := s.regression(r, p, pr, &in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sel)
}

// handleRegressionRun: POST /projects/{pid}/regression/run — creates a run
// of the regression selection (for the variant, when given).
func (s *Server) handleRegressionRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in regressionReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if !p.Can(rbac.TestRead) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.read required")
		return
	}
	sel, err := s.regression(r, p, pr, &in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(sel.Tests) == 0 {
		s.fail(w, r, badRequest("the regression selection has no tests"))
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Regression " + time.Now().UTC().Format("2006-01-02")
		if sel.SinceBuild != nil {
			name = "Regression since build " + sel.SinceBuild.Label()
		}
	}
	run := &model.TestRun{ProjectID: pr.ID, Name: name, Description: in.Description, Environment: in.Environment, Build: in.Build,
		CreatedBy: p.User.Email, ReleaseID: in.ReleaseID}
	if sel.Variant != nil {
		run.VariantID = &sel.Variant.ID
	}
	for _, id := range in.ConfigurationIDs {
		run.Configurations = append(run.Configurations, model.ConfigurationRef{ID: id})
	}
	ids := make([]string, 0, len(sel.Tests))
	for _, t := range sel.Tests {
		ids = append(ids, t.ID)
	}
	if err := s.Store.CreateRun(r.Context(), p.TenantID(), run, ids); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "run.create", "run", run.ID, fmt.Sprintf("%s (regression selection, %d tests)", run.Name, len(ids)))
	s.notifyEvent(r, p, run.ProjectID, "run.created", map[string]any{"run": run})
	out, err := s.Store.GetRun(r.Context(), p.TenantID(), run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"run": out, "selection": sel})
}
