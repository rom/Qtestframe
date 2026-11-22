package api

import (
	"archive/zip"
	"bytes"
	"net/http"
	"path"
	"strings"

	"github.com/rom/qtestframe/internal/ci"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/stubgen"
)

// Generated automated-test stubs (see docs/api.md "Test stubs" and
// internal/stubgen): GET or POST /projects/{pid}/tests/stubs returns the
// stubs of the tests a query and/or ids select as a zip, as JSON (the
// preview of the web UI and what qtestframe stubs merges into a
// directory) or as a single file. POST with mark_script also sets the
// script of each test that has none to a reference to its stub
// (stubgen.ScriptRef), so the server does not run it and the next
// generation with -has:script skips it.

// MaxStubs limits the tests of one stub request.
const MaxStubs = 2000

func (s *Server) routesStubs(h routeFunc) {
	h("GET /api/v1/projects/{pid}/tests/stubs", s.handleStubs, "test.read")
	h("POST /api/v1/projects/{pid}/tests/stubs", s.impactAware(s.handleStubs, true), "test.read")
}

type stubRequest struct {
	Framework string   `json:"framework"`
	Query     string   `json:"query"`
	Q         string   `json:"q"`
	IDs       []string `json:"ids"`
	// Format: zip (default), json or file (exactly one test).
	Format string `json:"format"`
	// Package of the Go or Java stubs.
	Package string `json:"package"`
	// Dir is the directory of the stubs in the repository (a prefix of
	// the paths in the zip and of the script references).
	Dir string `json:"dir"`
	// MarkScript sets the script of tests without one to the stub path
	// (POST only; needs test.write).
	MarkScript bool `json:"mark_script"`
}

type stubFile struct {
	stubgen.File
	TestID string `json:"test_id"`
}

type stubResponse struct {
	Framework stubgen.Framework `json:"framework"`
	Files     []stubFile        `json:"files"`
	// Marked: tests whose script now refers to the stub.
	Marked int `json:"marked"`
}

func (s *Server) handleStubs(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in stubRequest
	if r.Method == http.MethodPost {
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
	} else {
		qv := r.URL.Query()
		in.Framework, in.Query, in.Q, in.Format, in.Package, in.Dir = qv.Get("framework"), qv.Get("query"), qv.Get("q"), qv.Get("format"), qv.Get("package"), qv.Get("dir")
		in.IDs = qv["ids"]
	}
	if in.Query == "" {
		in.Query = in.Q
	}
	var ids []string
	for _, v := range in.IDs {
		for _, id := range strings.Split(v, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	}
	fw, ok := stubgen.Lookup(in.Framework)
	if !ok {
		s.fail(w, r, badRequest("unknown framework %q (use one of %s)", in.Framework, strings.Join(stubgen.FrameworkIDs(), ", ")))
		return
	}
	switch in.Format {
	case "", "zip", "json", "file":
	default:
		s.fail(w, r, badRequest("unknown format %q (zip, json or file)", in.Format))
		return
	}
	dir, err := stubDir(in.Dir)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.MarkScript && !p.Can(rbac.TestWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.write required to mark the scripts")
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tests, err := s.filterTests(r, p, pr, pc, in.Query, "", true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(ids) > 0 {
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		var sel []model.TestCase
		for _, t := range tests {
			if want[t.ID] || want[t.Key] {
				sel = append(sel, t)
			}
		}
		tests = sel
	}
	if len(tests) > MaxStubs {
		s.fail(w, r, badRequest("%d tests selected; at most %d per request", len(tests), MaxStubs))
		return
	}
	out := stubResponse{Framework: fw, Files: []stubFile{}}
	for i := range tests {
		t := &tests[i]
		f, err := stubgen.Generate(fw.ID, stubgen.FromModel(t, pc.t2rKeys[t.ID], s.testURL(pr.ID, t.ID)), stubgen.Options{Package: in.Package})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		f.Path = dir + f.Path
		out.Files = append(out.Files, stubFile{File: f, TestID: t.ID})
	}
	if in.MarkScript && r.Method == http.MethodPost {
		n, err := s.markStubScripts(r, p, fw, out.Files)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out.Marked = n
	}
	switch in.Format {
	case "json":
		writeJSON(w, http.StatusOK, out)
	case "file":
		if len(out.Files) != 1 {
			s.fail(w, r, badRequest("format file needs exactly one test, %d selected", len(out.Files)))
			return
		}
		f := out.Files[0]
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", contentDisposition(path.Base(f.Path)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write([]byte(f.Content))
	default:
		if len(out.Files) == 0 {
			s.fail(w, r, badRequest("no tests selected"))
			return
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, f := range out.Files {
			fh := &zip.FileHeader{Name: f.Path, Method: zip.Deflate}
			fh.SetMode(0o644)
			fw, err := zw.CreateHeader(fh)
			if err == nil {
				_, err = fw.Write([]byte(f.Content))
			}
			if err != nil {
				s.fail(w, r, err)
				return
			}
		}
		if err := zw.Close(); err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", contentDisposition(strings.ToLower(pr.Key)+"-"+fw.ID+"-stubs.zip"))
		_, _ = w.Write(buf.Bytes())
	}
}

// stubDir normalises the directory of the stubs ("" or "dir/").
func stubDir(d string) (string, error) {
	d = strings.TrimSpace(strings.ReplaceAll(d, `\`, "/"))
	if d == "" || d == "." || d == "./" {
		return "", nil
	}
	c := path.Clean(strings.TrimPrefix(d, "./"))
	if path.IsAbs(c) || c == ".." || strings.HasPrefix(c, "../") || strings.ContainsAny(c, "\x00\n\r") {
		return "", badRequest("dir must be a relative path inside the repository")
	}
	return c + "/", nil
}

func (s *Server) testURL(projectID, testID string) string {
	if s.baseURL() == "" {
		return ""
	}
	return s.baseURL() + "/p/" + projectID + "/tests?item=" + testID
}

// markStubScripts sets the script of the tests without one (or with an
// earlier stub reference) to the stub reference, makes them automated and
// tags them with the framework's runner unless they have a runner tag.
// Must run in the request transaction (the POST route is impactAware).
func (s *Server) markStubScripts(r *http.Request, p *Principal, fw stubgen.Framework, files []stubFile) (int, error) {
	ctx := r.Context()
	n := 0
	for _, f := range files {
		t, err := s.Store.GetTest(ctx, p.TenantID(), f.TestID)
		if err != nil {
			return n, err
		}
		_, isRef := stubgen.ScriptRefPath(t.Script)
		if strings.TrimSpace(t.Script) != "" && !isRef {
			continue
		}
		ref := stubgen.ScriptRef(f.Path)
		hasRunner := false
		for _, tag := range t.Tags {
			if strings.HasPrefix(strings.ToLower(tag), ci.RunnerTagPrefix) {
				hasRunner = true
			}
		}
		if t.Script == ref && t.ExecutionType == model.ExecAutomated && hasRunner {
			continue
		}
		before := store.SnapshotTest(t)
		s.ensureInitialRevision(ctx, p, model.KindTest, t.ID)
		t.Script, t.ExecutionType = ref, model.ExecAutomated
		if fw.ScriptLanguage != "" {
			t.ScriptLanguage = fw.ScriptLanguage
		}
		if !hasRunner {
			t.Tags = append(t.Tags, ci.RunnerTagPrefix+fw.Runner)
		}
		if err := s.Store.UpdateTest(ctx, p.TenantID(), t, false); err != nil {
			return n, err
		}
		s.audit(ctx, p, "test.update", "test", t.ID, t.Key+" script → "+f.Path)
		s.recordTest(ctx, p, t.ID, "update")
		s.afterContentChange(ctx, p, model.KindTest, t.ID, t.Key, t.ProjectID, before)
		n++
	}
	return n, nil
}

// isStubRef: the script of a test refers to its code in the repository.
func isStubRef(script string) bool {
	_, ok := stubgen.ScriptRefPath(script)
	return ok
}

func stubPath(script string) string {
	p, _ := stubgen.ScriptRefPath(script)
	return p
}
