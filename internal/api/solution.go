package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/prompt"
)

// The solution preamble of a software project (see docs/api.md "Solution
// preamble"): languages, stack, environments, constraints and conventions
// that the solution prompts (architecture, design, implementation plan,
// full solution; prompts.go) start with. It is stored with the project,
// versioned in the history (revisions of entity type solution_preamble)
// and part of project archives and backups. It holds descriptions only:
// fields that ask for secrets are refused, and what looks like a
// credential is reported as a warning (and masked in prompts).

func (s *Server) routesSolution(h routeFunc) {
	h("GET /api/v1/projects/{pid}/solution-preamble", s.handleGetPreamble, "project.read")
	h("PUT /api/v1/projects/{pid}/solution-preamble", s.handlePutPreamble, "project.update")
	h("POST /api/v1/projects/{pid}/solution-preamble/check", s.handleCheckPreamble, "project.read")
	h("GET /api/v1/projects/{pid}/solution-preamble/history", s.handlePreambleHistory, "project.read")
	h("GET /api/v1/projects/{pid}/solution-preamble/templates", s.handlePreambleTemplates, "project.read")
	// a built prompt sent to the optional AI assistance (promptsend.go);
	// read of the items' kind: see buildPrompt
	h("POST /api/v1/projects/{pid}/prompts/send", s.handlePromptSend, "project.read")
}

// preambleView is the answer of GET/PUT: the preamble, the warnings about
// credentials and the number of filled-in fields.
type preambleView struct {
	Preamble *model.SolutionPreamble `json:"preamble"`
	Warnings []prompt.Warning        `json:"warnings"`
	Filled   int                     `json:"filled"`
	// Software: the project is a software project (only those use it).
	Software bool `json:"software"`
}

func newPreambleView(pr *model.Project, p *model.SolutionPreamble) preambleView {
	return preambleView{Preamble: p, Warnings: prompt.CheckPreamble(p), Filled: p.Filled(), Software: pr.Type == model.ProjectTypeSoftware}
}

func (s *Server) handleGetPreamble(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pre, err := s.Store.SolutionPreamble(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPreambleView(pr, pre))
}

// decodePreamble reads a preamble from a request: only the known fields
// (and version), no field that asks for a secret, lengths and the style
// checked. version is the version the client edited (0: not given).
func decodePreamble(r *http.Request) (*model.SolutionPreamble, int, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, 0, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil || raw == nil {
		return nil, 0, badRequest("a JSON object is required")
	}
	known := map[string]bool{"template": true, "architecture_style": true, "version": true}
	for _, f := range model.PreambleFields {
		known[f] = true
	}
	var unknown []string
	for k := range raw {
		if prompt.ForbiddenPreambleKey(k) {
			return nil, 0, badRequest("%s: the solution preamble holds descriptions only; passwords, keys, tokens and connection strings are not stored (describe where they are kept instead)", k)
		}
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return nil, 0, badRequest("unknown fields: %s", strings.Join(unknown, ", "))
	}
	var in struct {
		model.SolutionPreamble
		Version int `json:"version"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, 0, badRequest("invalid JSON: %v", err)
	}
	pre := in.SolutionPreamble
	pre.Normalize()
	for _, f := range model.PreambleFields {
		limit := model.MaxPreambleField
		if f == "additional" {
			limit = model.MaxPreambleAdditional
		}
		if n := len([]rune(*pre.Field(f))); n > limit {
			return nil, 0, badRequest("%s may have at most %d characters (has %d)", f, limit, n)
		}
	}
	if pre.ArchitectureStyle != "" && !slices.Contains(model.ArchitectureStyles, pre.ArchitectureStyle) {
		return nil, 0, badRequest("architecture_style must be one of %s", strings.Join(model.ArchitectureStyles, ", "))
	}
	if pre.Template != "" && prompt.FindTemplate(pre.Template) == nil {
		return nil, 0, badRequest("unknown template %q", pre.Template)
	}
	return &pre, in.Version, nil
}

// handlePutPreamble: PUT /projects/{pid}/solution-preamble — replaces the
// preamble (fields left out are emptied). With "version" the save fails
// with 409 when someone else saved in between. Warnings do not stop the
// save; they are in the answer.
func (s *Server) handlePutPreamble(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	if pr.Type != model.ProjectTypeSoftware {
		s.fail(w, r, badRequest("the solution preamble is available in software projects; set the project type to software"))
		return
	}
	pre, version, err := decodePreamble(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	cur, err := s.Store.SolutionPreamble(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if version != 0 && version != cur.Version {
		writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("the solution preamble was changed by %s (version %d); reload it and apply your changes again", firstNonBlank(cur.UpdatedBy, "someone else"), cur.Version))
		return
	}
	if err := s.Store.SaveSolutionPreamble(ctx, p.TenantID(), pr.ID, pre, p.User.Email); err != nil {
		s.fail(w, r, err)
		return
	}
	v := newPreambleView(pr, pre)
	s.audit(ctx, p, "project.solution_preamble", "project", pr.ID, fmt.Sprintf("version %d, %d fields, %d warnings", pre.Version, v.Filled, len(v.Warnings)))
	writeJSON(w, http.StatusOK, v)
}

// handleCheckPreamble: POST …/solution-preamble/check — the warnings of an
// unsaved preamble (nothing is stored).
func (s *Server) handleCheckPreamble(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pre, _, err := decodePreamble(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPreambleView(pr, pre))
}

func (s *Server) handlePreambleHistory(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	revs, err := s.Store.ListRevisions(r.Context(), p.TenantID(), model.KindSolutionPreamble, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(revs))
}

func (s *Server) handlePreambleTemplates(w http.ResponseWriter, r *http.Request, p *Principal) {
	if _, ok := s.project(w, r, p); !ok {
		return
	}
	writeJSON(w, http.StatusOK, listOf(prompt.Templates))
}

// writePromptZip answers with the parts of a prompt as a ZIP archive: one
// Markdown file per part ("<KEY>-<kind>-<date>-part-01.md").
func writePromptZip(w http.ResponseWriter, projectKey string, res prompt.Result) {
	base := fmt.Sprintf("%s-%s-%s", firstNonBlank(projectKey, "prompt"), strings.ReplaceAll(res.Kind, "_", "-"), time.Now().UTC().Format("2006-01-02"))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := res.Parts
	if len(parts) == 0 {
		parts = []prompt.Part{{Index: 1, Title: res.Title, Text: res.Text}}
	}
	for _, pt := range parts {
		name := base + ".md"
		if len(parts) > 1 {
			name = fmt.Sprintf("%s-part-%02d.md", base, pt.Index)
		}
		f, _ := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now().UTC()})
		_, _ = f.Write([]byte(pt.Text))
	}
	_ = zw.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+base+`.zip"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
