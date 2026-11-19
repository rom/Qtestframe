package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/store"
)

// Query schemas per entity (see docs/api.md "Query language").
var (
	requirementSchema = &query.Schema{
		Fields: []string{"key", "title", "description", "obligation", "type", "priority", "status", "source", "rationale",
			"owner", "tag", "version", "folder", "category", "dimension", "coverage", "linked", "created_by",
			"created_at", "updated_at", "criteria", "id"},
		Aliases: map[string]string{"tags": "tag", "categories": "category", "desc": "description", "criterion": "criteria"},
	}
	testSchema = &query.Schema{
		Fields: []string{"key", "title", "description", "preconditions", "execution_type", "script_language", "script",
			"expected", "step", "priority", "status", "estimate", "tag", "folder", "category", "dimension", "linked",
			"last_result", "created_by", "created_at", "updated_at", "id"},
		Aliases: map[string]string{"type": "execution_type", "tags": "tag", "categories": "category", "steps": "step",
			"desc": "description", "language": "script_language", "result": "last_result"},
	}
	runSchema = &query.Schema{
		Fields: []string{"name", "description", "status", "environment", "build", "created_by", "created_at", "id"},
		Aliases: map[string]string{"env": "environment"},
	}
	executionSchema = &query.Schema{
		Fields: []string{"key", "title", "status", "assignee", "executed_by", "notes", "output", "id"},
	}
)

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// projectContext holds lookups needed to build records and derived fields.
type projectContext struct {
	project    *model.Project
	folders    []model.Folder
	paths      map[string]string
	categories map[string]model.Category
	links      []model.TraceLink
	r2t, t2r   map[string][]string
	last       map[string]string
}

func (s *Server) loadContext(ctx context.Context, tenantID string, p *model.Project) (*projectContext, error) {
	pc := &projectContext{project: p, categories: map[string]model.Category{}}
	var err error
	if pc.folders, err = s.Store.ListFolders(ctx, tenantID, p.ID, ""); err != nil {
		return nil, err
	}
	pc.paths = store.FolderPaths(pc.folders)
	cats, err := s.Store.ListCategories(ctx, tenantID, p.ID, "")
	if err != nil {
		return nil, err
	}
	for _, c := range cats {
		pc.categories[c.ID] = c
	}
	if pc.links, err = s.Store.ListLinks(ctx, tenantID, p.ID, "", ""); err != nil {
		return nil, err
	}
	pc.r2t, pc.t2r = store.LinkIndex(pc.links)
	if pc.last, err = s.Store.LastResults(ctx, tenantID, p.ID); err != nil {
		return nil, err
	}
	return pc, nil
}

func (pc *projectContext) catValues(ids []string) (names, dims []string) {
	for _, id := range ids {
		if c, ok := pc.categories[id]; ok {
			names = append(names, c.Name)
			dims = append(dims, c.Dimension+"/"+c.Name)
		}
	}
	return
}

func (pc *projectContext) folderPath(id *string) string {
	if id == nil {
		return ""
	}
	return pc.paths[*id]
}

// decorateRequirement fills derived fields.
func (pc *projectContext) decorateRequirement(r *model.Requirement) {
	r.FolderPath = pc.folderPath(r.FolderID)
	r.LinkedTests = len(pc.r2t[r.ID])
	r.Coverage = store.Coverage(pc.r2t[r.ID], pc.last)
}

func (pc *projectContext) decorateTest(t *model.TestCase) {
	t.FolderPath = pc.folderPath(t.FolderID)
	t.LinkedReqs = len(pc.t2r[t.ID])
	t.LastResult = pc.last[t.ID]
	if t.LastResult == "" {
		t.LastResult = model.ResultNotRun
	}
}

func (pc *projectContext) requirementRecord(r *model.Requirement) query.MapRecord {
	names, dims := pc.catValues(r.CategoryIDs)
	var crit []string
	for _, c := range r.Criteria {
		crit = append(crit, c.Description, c.Method, c.Threshold)
	}
	var linkedKeys []string
	for _, l := range pc.links {
		if l.RequirementID == r.ID {
			linkedKeys = append(linkedKeys, l.TestCaseKey)
		}
	}
	return query.MapRecord{
		"id": {r.ID}, "key": {r.Key}, "title": {r.Title}, "description": {r.Description}, "obligation": {r.Obligation},
		"type": {r.Type}, "priority": {r.Priority}, "status": {r.Status}, "source": {r.Source},
		"rationale": {r.Rationale}, "owner": {r.Owner}, "tag": r.Tags, "version": {strconv.Itoa(r.Version)},
		"folder": {r.FolderPath}, "category": names, "dimension": dims, "coverage": {r.Coverage},
		"linked": linkedKeys, "created_by": {r.CreatedBy}, "created_at": {ts(r.CreatedAt)},
		"updated_at": {ts(r.UpdatedAt)}, "criteria": crit,
	}
}

func (pc *projectContext) testRecord(t *model.TestCase) query.MapRecord {
	names, dims := pc.catValues(t.CategoryIDs)
	var steps []string
	for _, st := range t.Steps {
		steps = append(steps, st.Action, st.Expected, st.Data)
	}
	var linkedKeys []string
	for _, l := range pc.links {
		if l.TestCaseID == t.ID {
			linkedKeys = append(linkedKeys, l.RequirementKey)
		}
	}
	return query.MapRecord{
		"id": {t.ID}, "key": {t.Key}, "title": {t.Title}, "description": {t.Description},
		"preconditions": {t.Preconditions}, "execution_type": {t.ExecutionType},
		"script_language": {t.ScriptLanguage}, "script": {t.Script}, "expected": {t.ExpectedResult},
		"step": steps, "priority": {t.Priority}, "status": {t.Status}, "estimate": {strconv.Itoa(t.EstimatedMin)},
		"tag": t.Tags, "folder": {t.FolderPath}, "category": names, "dimension": dims, "linked": linkedKeys,
		"last_result": {t.LastResult}, "created_by": {t.CreatedBy}, "created_at": {ts(t.CreatedAt)},
		"updated_at": {ts(t.UpdatedAt)},
	}
}

func runRecord(r *model.TestRun) query.MapRecord {
	return query.MapRecord{"id": {r.ID}, "name": {r.Name}, "description": {r.Description}, "status": {r.Status},
		"environment": {r.Environment}, "build": {r.Build}, "created_by": {r.CreatedBy}, "created_at": {ts(r.CreatedAt)}}
}

func executionRecord(e *model.Execution) query.MapRecord {
	return query.MapRecord{"id": {e.ID}, "key": {e.TestKey}, "title": {e.TestTitle}, "status": {e.Status},
		"assignee": {e.Assignee}, "executed_by": {e.ExecutedBy}, "notes": {e.Notes}, "output": {e.Output}}
}

// folderFilter returns a predicate selecting items in folder (and
// descendants when recursive). folder "root" or "none" selects unfiled.
func (pc *projectContext) folderFilter(folderID string, recursive bool) func(*string) bool {
	if folderID == "" {
		return func(*string) bool { return true }
	}
	if folderID == "root" || folderID == "none" {
		return func(f *string) bool { return f == nil }
	}
	set := map[string]bool{folderID: true}
	if recursive {
		for _, id := range store.Descendants(pc.folders, folderID) {
			set[id] = true
		}
	}
	return func(f *string) bool { return f != nil && set[*f] }
}

// snippet returns a short excerpt of text around the first term.
func snippet(text string, terms []string) string {
	text = strings.Join(strings.Fields(text), " ")
	const width = 160
	idx := -1
	lower := strings.ToLower(text)
	for _, t := range terms {
		if i := strings.Index(lower, strings.ToLower(t)); i >= 0 && t != "" {
			idx = i
			break
		}
	}
	if idx < 0 || len(text) <= width {
		if len(text) > width {
			return truncateRunes(text, width) + "…"
		}
		return text
	}
	start := idx - width/3
	if start < 0 {
		start = 0
	}
	// align to rune boundary
	for start > 0 && start < len(text) && (text[start]&0xC0) == 0x80 {
		start--
	}
	out := truncateRunes(text[start:], width)
	if start > 0 {
		out = "…" + out
	}
	if start+len(out) < len(text) {
		out += "…"
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
