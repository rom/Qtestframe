package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
)

// SearchHit is one search result.
type SearchHit struct {
	Type          string   `json:"type"`
	ID            string   `json:"id"`
	ProjectID     string   `json:"project_id"`
	ProjectKey    string   `json:"project_key"`
	Key           string   `json:"key"`
	Title         string   `json:"title"`
	Snippet       string   `json:"snippet"`
	MatchedFields []string `json:"matched_fields"`
}

// union schema so a search may use fields of any entity type
var searchSchema = func() *query.Schema {
	seen := map[string]bool{}
	out := &query.Schema{Aliases: map[string]string{}}
	for _, s := range []*query.Schema{requirementSchema, testSchema, runSchema} {
		for _, f := range s.Fields {
			if !seen[f] {
				seen[f] = true
				out.Fields = append(out.Fields, f)
			}
		}
		for k, v := range s.Aliases {
			out.Aliases[k] = v
		}
	}
	out.Fields = append(out.Fields, "project", "kind")
	return out
}()

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, p *Principal) {
	expr := strings.TrimSpace(r.URL.Query().Get("q"))
	if expr == "" {
		writeJSON(w, http.StatusOK, listOf([]SearchHit{}))
		return
	}
	q, err := query.Parse(expr, searchSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	types := map[string]bool{"requirement": true, "test": true, "run": true}
	if t := r.URL.Query().Get("types"); t != "" {
		types = map[string]bool{}
		for _, x := range strings.Split(t, ",") {
			types[strings.TrimSpace(x)] = true
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	ctx := r.Context()
	projects, err := s.Store.ListProjects(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	terms := q.Terms()
	hits := []SearchHit{}
	add := func(rec query.MapRecord, h SearchHit) {
		if !q.Match(rec) {
			return
		}
		h.MatchedFields = q.MatchedFields(rec)
		text := ""
		for _, f := range h.MatchedFields {
			if vals, ok := rec.Values(f); ok && f != "title" && f != "key" {
				text = strings.Join(vals, " ")
				break
			}
		}
		if text == "" {
			if v, ok := rec.Values("description"); ok {
				text = strings.Join(v, " ")
			}
		}
		h.Snippet = snippet(text, terms)
		hits = append(hits, h)
	}
	only := r.URL.Query().Get("project_id")
	for i := range projects {
		pr := &projects[i]
		if only != "" && pr.ID != only {
			continue
		}
		pc, err := s.loadContext(ctx, p.TenantID(), pr)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		meta := func(rec query.MapRecord, kind string) query.MapRecord {
			rec["project"] = []string{pr.Key, pr.Name}
			rec["kind"] = []string{kind}
			return rec
		}
		if types["requirement"] {
			reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for j := range reqs {
				pc.decorateRequirement(&reqs[j])
				add(meta(pc.requirementRecord(&reqs[j]), "requirement"), SearchHit{Type: "requirement", ID: reqs[j].ID, ProjectID: pr.ID, ProjectKey: pr.Key, Key: reqs[j].Key, Title: reqs[j].Title})
			}
		}
		if types["test"] {
			tests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for j := range tests {
				pc.decorateTest(&tests[j])
				add(meta(pc.testRecord(&tests[j]), "test"), SearchHit{Type: "test", ID: tests[j].ID, ProjectID: pr.ID, ProjectKey: pr.Key, Key: tests[j].Key, Title: tests[j].Title})
			}
		}
		if types["run"] {
			runs, err := s.Store.ListRuns(ctx, p.TenantID(), pr.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for j := range runs {
				add(meta(runRecord(&runs[j]), "run"), SearchHit{Type: "run", ID: runs[j].ID, ProjectID: pr.ID, ProjectKey: pr.Key, Key: runs[j].Name, Title: runs[j].Name})
			}
		}
	}
	total := len(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	writeJSON(w, http.StatusOK, list[SearchHit]{Items: hits, Total: total})
}

// ---------------------------------------------------------------- export

var allAspects = []string{"requirements", "criteria", "tests", "traceability", "runs"}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	f, err := export.ParseFormat(r.URL.Query().Get("format"))
	if err != nil {
		s.fail(w, r, badRequest("%v", err))
		return
	}
	aspects := allAspects
	if a := r.URL.Query().Get("aspects"); a != "" {
		aspects = nil
		for _, x := range strings.Split(a, ",") {
			x = strings.TrimSpace(x)
			if !model.Valid(allAspects, x) {
				s.fail(w, r, badRequest("unknown aspect %q", x))
				return
			}
			aspects = append(aspects, x)
		}
	}
	ds, err := s.buildDataset(r, p, pr, aspects, r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	multi := len(ds.Sections) > 1
	name := fmt.Sprintf("%s-%s.%s", pr.Key, time.Now().UTC().Format("20060102-150405"), f.Extension(multi))
	w.Header().Set("Content-Type", f.ContentTypeFor(multi))
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if err := export.Write(w, f, *ds); err != nil {
		s.Log.Error("export failed", "err", err)
		return
	}
	s.audit(r.Context(), p, "project.export", "project", pr.ID, string(f)+" "+strings.Join(aspects, ","))
}

func (s *Server) buildDataset(r *http.Request, p *Principal, pr *model.Project, aspects []string, expr string) (*export.Dataset, error) {
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, a := range aspects {
		want[a] = true
	}
	ds := &export.Dataset{Title: pr.Key + " – " + pr.Name, Subtitle: pr.Description, GeneratedAt: time.Now().UTC()}
	var reqs []model.Requirement
	var tests []model.TestCase
	if want["requirements"] || want["criteria"] || want["traceability"] {
		if reqs, err = s.filterRequirements(r, p, pr, pc, reqExpr(expr), "", true); err != nil {
			return nil, err
		}
	}
	if want["tests"] || want["traceability"] {
		if tests, err = s.filterTests(r, p, pr, pc, testExpr(expr), "", true); err != nil {
			return nil, err
		}
	}
	cats := func(ids []string) string {
		_, dims := pc.catValues(ids)
		return strings.Join(dims, ", ")
	}
	if want["requirements"] {
		sec := export.Section{Name: "requirements", Title: "Requirements",
			Columns: []string{"Key", "Title", "Obligation", "Type", "Priority", "Status", "Folder", "Categories", "Tags", "Description", "Rationale", "Source", "Owner", "Version", "Linked tests", "Coverage"}}
		for _, rq := range reqs {
			sec.Rows = append(sec.Rows, []string{rq.Key, rq.Title, strings.ToUpper(strings.ReplaceAll(rq.Obligation, "_", " ")), rq.Type, rq.Priority, rq.Status,
				rq.FolderPath, cats(rq.CategoryIDs), strings.Join(rq.Tags, ", "), rq.Description, rq.Rationale, rq.Source, rq.Owner,
				strconv.Itoa(rq.Version), strconv.Itoa(rq.LinkedTests), rq.Coverage})
		}
		ds.Sections = append(ds.Sections, sec)
	}
	if want["criteria"] {
		sec := export.Section{Name: "criteria", Title: "Evaluation criteria", Columns: []string{"Requirement", "#", "Criterion", "Method", "Threshold"}}
		for _, rq := range reqs {
			for i, c := range rq.Criteria {
				sec.Rows = append(sec.Rows, []string{rq.Key, strconv.Itoa(i + 1), c.Description, c.Method, c.Threshold})
			}
		}
		ds.Sections = append(ds.Sections, sec)
	}
	if want["tests"] {
		sec := export.Section{Name: "tests", Title: "Test cases",
			Columns: []string{"Key", "Title", "Execution", "Status", "Priority", "Folder", "Categories", "Tags", "Preconditions", "Steps", "Expected result", "Script language", "Linked requirements", "Last result"}}
		for _, t := range tests {
			var steps []string
			for i, st := range t.Steps {
				line := fmt.Sprintf("%d. %s", i+1, st.Action)
				if st.Expected != "" {
					line += " => " + st.Expected
				}
				steps = append(steps, line)
			}
			sec.Rows = append(sec.Rows, []string{t.Key, t.Title, t.ExecutionType, t.Status, t.Priority, t.FolderPath, cats(t.CategoryIDs),
				strings.Join(t.Tags, ", "), t.Preconditions, strings.Join(steps, "\n"), t.ExpectedResult, t.ScriptLanguage,
				strconv.Itoa(t.LinkedReqs), t.LastResult})
		}
		ds.Sections = append(ds.Sections, sec)
	}
	if want["traceability"] {
		sec := export.Section{Name: "traceability", Title: "Traceability matrix", Columns: []string{"Requirement", "Requirement title", "Coverage", "Test", "Test title", "Last result"}}
		reqByID, testByID := map[string]model.Requirement{}, map[string]model.TestCase{}
		for _, rq := range reqs {
			reqByID[rq.ID] = rq
		}
		for _, t := range tests {
			testByID[t.ID] = t
		}
		for _, rq := range reqs {
			ids := pc.r2t[rq.ID]
			if len(ids) == 0 {
				sec.Rows = append(sec.Rows, []string{rq.Key, rq.Title, rq.Coverage, "", "", ""})
			}
			for _, tid := range ids {
				t, ok := testByID[tid]
				if !ok {
					continue
				}
				sec.Rows = append(sec.Rows, []string{rq.Key, rq.Title, rq.Coverage, t.Key, t.Title, t.LastResult})
			}
		}
		ds.Sections = append(ds.Sections, sec)
	}
	if want["runs"] {
		runs, err := s.Store.ListRuns(ctx, p.TenantID(), pr.ID)
		if err != nil {
			return nil, err
		}
		sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt.Before(runs[j].CreatedAt) })
		sec := export.Section{Name: "runs", Title: "Test runs", Columns: []string{"Run", "Run status", "Environment", "Build", "Created", "Test", "Title", "Result", "Executed by", "Duration (ms)", "Notes"}}
		for _, run := range runs {
			es, err := s.Store.ListExecutions(ctx, p.TenantID(), run.ID)
			if err != nil {
				return nil, err
			}
			for _, e := range es {
				sec.Rows = append(sec.Rows, []string{run.Name, run.Status, run.Environment, run.Build, ts(run.CreatedAt), e.TestKey, e.TestTitle,
					e.Status, e.ExecutedBy, strconv.FormatInt(e.DurationMS, 10), e.Notes})
			}
		}
		ds.Sections = append(ds.Sections, sec)
	}
	return ds, nil
}

// reqExpr/testExpr allow one export filter for both entity types: the
// expression is applied only to the type whose schema accepts it.
func reqExpr(e string) string {
	if _, err := query.Parse(e, requirementSchema); err != nil {
		if _, terr := query.Parse(e, testSchema); terr == nil {
			return ""
		}
	}
	return e
}

func testExpr(e string) string {
	if _, err := query.Parse(e, testSchema); err != nil {
		if _, rerr := query.Parse(e, requirementSchema); rerr == nil {
			return ""
		}
	}
	return e
}
