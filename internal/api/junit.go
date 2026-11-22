package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/junit"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// MaxJUnitBytes limits a JUnit XML import (all files of one request).
const MaxJUnitBytes = 20 << 20

// routesJUnit registers the JUnit XML import (see docs/api.md
// "Operations").
func (s *Server) routesJUnit(h routeFunc) {
	h("POST /api/v1/runs/{id}/junit", s.handleImportJUnit, "run.execute")
}

// createdTest is a test created by an import with create_missing.
type createdTest struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

func (s *Server) handleImportJUnit(w http.ResponseWriter, r *http.Request, p *Principal) {
	createMissing := queryBool(r, "create_missing")
	if createMissing && !p.Can(rbac.TestWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.write required to create missing tests")
		return
	}
	docs, err := readJUnitBody(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var cases []junit.Case
	for _, d := range docs {
		cs, err := junit.ParseBytes(d.data)
		if err != nil {
			if d.name != "" {
				err = fmt.Errorf("%s: %w", d.name, err)
			}
			s.fail(w, r, badRequest("%v", err))
			return
		}
		cases = append(cases, cs...)
	}
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	mapped, unmapped := junit.Aggregate(cases)
	created := []createdTest{}
	if createMissing && len(unmapped) > 0 {
		var err error
		if mapped, created, err = s.createMissingTests(r, p, run, mapped, unmapped); err != nil {
			s.fail(w, r, err)
			return
		}
		unmapped = nil
	}
	results := make([]store.ImportResult, 0, len(mapped))
	for _, m := range mapped {
		results = append(results, store.ImportResult{TestKey: m.Key, Status: m.Status, DurationMS: m.DurationMS, Output: m.Output})
	}
	names := make([]string, 0, len(unmapped))
	for _, u := range unmapped {
		names = append(names, u.Name)
	}
	s.recordResults(w, r, p, run, results, "run.import_junit", map[string]any{"unmapped": names, "created": created})
}

// createMissingTests gives every unmapped result a test: an existing
// automated test of the run's project with exactly that title (created by
// an earlier import), else a new draft automated test.
func (s *Server) createMissingTests(r *http.Request, p *Principal, run *model.TestRun, mapped, unmapped []junit.Result) ([]junit.Result, []createdTest, error) {
	ctx := r.Context()
	tests, err := s.Store.ListTests(ctx, p.TenantID(), run.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	byTitle := map[string]string{}
	for _, t := range tests {
		if t.ExecutionType == model.ExecAutomated {
			if _, dup := byTitle[t.Title]; !dup {
				byTitle[t.Title] = t.Key
			}
		}
	}
	created := []createdTest{}
	for _, u := range unmapped {
		title := testTitle(u.Name)
		key, ok := byTitle[title]
		if !ok {
			t := &model.TestCase{ProjectID: run.ProjectID, Title: title, ExecutionType: model.ExecAutomated,
				Status: "draft", Description: "Created by a JUnit import into run " + run.Name + ".", CreatedBy: p.User.Email}
			if err := s.Store.CreateTest(store.LenientCustomFields(ctx), p.TenantID(), t); err != nil {
				return nil, nil, err
			}
			s.audit(ctx, p, "test.create", "test", t.ID, t.Key)
			s.recordTest(ctx, p, t.ID, "create")
			created = append(created, createdTest{ID: t.ID, Key: t.Key, Title: t.Title})
			key = t.Key
			byTitle[title] = key
		}
		u.Key = key
		mapped = append(mapped, u)
	}
	return mapped, created, nil
}

// testTitle makes a test title of a test case name (at most 500 bytes).
func testTitle(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = "Unnamed test case"
	}
	if len(name) > 500 {
		name = junit.Truncate(name, 500)
		name = strings.TrimSuffix(name, "\n… (truncated)") + "…"
	}
	return name
}

type junitDoc struct {
	name string
	data []byte
}

// readJUnitBody returns the XML documents of a request: the raw body, or
// every multipart part named "file".
func readJUnitBody(w http.ResponseWriter, r *http.Request) ([]junitDoc, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxJUnitBytes)
	tooLarge := func(err error) error {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return badRequest("JUnit report is too large (max %d MB)", MaxJUnitBytes>>20)
		}
		return badRequest("reading body: %v", err)
	}
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "multipart/form-data" {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, tooLarge(err)
		}
		if len(strings.TrimSpace(string(b))) == 0 {
			return nil, badRequest("request body required (JUnit XML)")
		}
		return []junitDoc{{data: b}}, nil
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, badRequest("invalid multipart body: %v", err)
	}
	var docs []junitDoc
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, tooLarge(err)
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		b, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			return nil, tooLarge(err)
		}
		docs = append(docs, junitDoc{name: store.CleanFileName(part.FileName()), data: b})
	}
	if len(docs) == 0 {
		return nil, badRequest(`missing form field "file"`)
	}
	return docs, nil
}

func queryBool(r *http.Request, name string) bool {
	switch strings.ToLower(r.URL.Query().Get(name)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
