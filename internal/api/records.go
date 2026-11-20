package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/mention"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/store"
)

// Query schemas per entity (see docs/api.md "Query language").
var (
	requirementSchema = &query.Schema{
		Fields: []string{"key", "title", "description", "obligation", "type", "priority", "status", "source", "rationale",
			"owner", "tag", "version", "folder", "category", "dimension", "coverage", "linked", "created_by",
			"created_at", "updated_at", "criteria", "id", "related", "parent", "child", "comments", "mention", "suspect", "defects", "risk"},
		Aliases: map[string]string{"tags": "tag", "categories": "category", "desc": "description", "criterion": "criteria",
			"relation": "related", "parents": "parent", "children": "child", "mentions": "mention"},
	}
	testSchema = &query.Schema{
		Fields: []string{"key", "title", "description", "preconditions", "execution_type", "script_language", "script",
			"expected", "step", "priority", "status", "estimate", "tag", "folder", "category", "dimension", "linked",
			"last_result", "created_by", "created_at", "updated_at", "id", "defect", "calls", "comments", "version", "mention",
			"depends_on", "suspect"},
		Aliases: map[string]string{"type": "execution_type", "tags": "tag", "categories": "category", "steps": "step",
			"desc": "description", "language": "script_language", "result": "last_result", "defects": "defect", "mentions": "mention",
			"depends": "depends_on", "dependency": "depends_on", "prerequisite": "depends_on"},
	}
	runSchema = &query.Schema{
		Fields:  []string{"name", "description", "status", "environment", "build", "created_by", "created_at", "id", "configuration"},
		Aliases: map[string]string{"env": "environment", "config": "configuration", "configurations": "configuration"},
	}
	executionSchema = &query.Schema{
		Fields:  []string{"key", "title", "status", "assignee", "executed_by", "notes", "output", "id", "defect", "mention", "configuration"},
		Aliases: map[string]string{"defects": "defect", "mentions": "mention", "config": "configuration"},
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
	// suspect: requirement or test id → number of its suspect links
	suspect map[string]int
	// requirement id → open defects / highest open risk level
	openDefects map[string]int
	riskLevel   map[string]string
	last        map[string]string
	// cfgResults: latest result of every test per configuration
	cfgResults store.ConfigResults
	comments   map[string]int
	relCount   map[string]int
	defects    map[string][]string
	// requirement id → related keys; parent/child keys of refines relations
	related, parents, children map[string][]string
	// item id → references to related items (both directions)
	refs map[string][]model.ItemRef
	// test id → tests whose steps call it (set by filterTests)
	calledBy map[string][]model.ItemRef
	// test dependencies of the project; test id → keys of its prerequisites
	deps       []model.TestDependency
	prereqKeys map[string][]string
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
	pc.suspect = map[string]int{}
	for _, l := range pc.links {
		if l.SuspectSince != nil {
			pc.suspect[l.RequirementID]++
			pc.suspect[l.TestCaseID]++
		}
	}
	if pc.last, err = s.Store.LastResults(ctx, tenantID, p.ID); err != nil {
		return nil, err
	}
	if pc.cfgResults, err = s.Store.LastConfigResults(ctx, tenantID, p.ID); err != nil {
		return nil, err
	}
	if pc.openDefects, err = s.Store.OpenDefectCounts(ctx, tenantID, p.ID); err != nil {
		return nil, err
	}
	if pc.riskLevel, err = s.Store.RequirementRiskLevels(ctx, tenantID, p.ID); err != nil {
		return nil, err
	}
	if pc.comments, pc.relCount, err = s.Store.ProjectCounts(ctx, tenantID, p.ID); err != nil {
		return nil, err
	}
	if pc.defects, err = s.Store.TestDefects(ctx, tenantID, p.ID); err != nil {
		return nil, err
	}
	rels, err := s.Store.ListRelations(ctx, tenantID, p.ID, "")
	if err != nil {
		return nil, err
	}
	pc.related, pc.parents, pc.children = map[string][]string{}, map[string][]string{}, map[string][]string{}
	pc.refs = map[string][]model.ItemRef{}
	for _, r := range rels {
		pc.refs[r.SourceID] = append(pc.refs[r.SourceID], model.ItemRef{Type: r.Type, Direction: "outgoing", Kind: model.KindRequirement, ID: r.TargetID, Key: r.TargetKey, Title: r.TargetTitle})
		pc.refs[r.TargetID] = append(pc.refs[r.TargetID], model.ItemRef{Type: r.Type, Direction: "incoming", Kind: model.KindRequirement, ID: r.SourceID, Key: r.SourceKey, Title: r.SourceTitle})
		pc.related[r.SourceID] = append(pc.related[r.SourceID], r.TargetKey)
		pc.related[r.TargetID] = append(pc.related[r.TargetID], r.SourceKey)
		if r.Type == "refines" {
			pc.parents[r.SourceID] = append(pc.parents[r.SourceID], r.TargetKey)
			pc.children[r.TargetID] = append(pc.children[r.TargetID], r.SourceKey)
		}
	}
	for _, l := range pc.links {
		pc.refs[l.TestCaseID] = append(pc.refs[l.TestCaseID], model.ItemRef{Type: "verifies", Direction: "outgoing", Kind: model.KindRequirement, ID: l.RequirementID, Key: l.RequirementKey})
		pc.refs[l.RequirementID] = append(pc.refs[l.RequirementID], model.ItemRef{Type: "verifies", Direction: "incoming", Kind: model.KindTest, ID: l.TestCaseID, Key: l.TestCaseKey})
	}
	if pc.deps, err = s.Store.ListTestDependencies(ctx, tenantID, p.ID, ""); err != nil {
		return nil, err
	}
	pc.prereqKeys = map[string][]string{}
	for _, d := range pc.deps {
		pc.refs[d.TestID] = append(pc.refs[d.TestID], model.ItemRef{Type: "depends_on", Direction: "outgoing", Kind: model.KindTest, ID: d.DependsOnID, Key: d.DependsOnKey, Title: d.DependsOnTitle})
		pc.refs[d.DependsOnID] = append(pc.refs[d.DependsOnID], model.ItemRef{Type: "depends_on", Direction: "incoming", Kind: model.KindTest, ID: d.TestID, Key: d.TestKey, Title: d.TestTitle})
		pc.prereqKeys[d.TestID] = append(pc.prereqKeys[d.TestID], d.DependsOnKey)
	}
	return pc, nil
}

// indexCalls records, for every test, the tests whose steps call it.
func (pc *projectContext) indexCalls(tests []model.TestCase) {
	pc.calledBy = map[string][]model.ItemRef{}
	for _, t := range tests {
		seen := map[string]bool{}
		for _, st := range t.Steps {
			if st.CallTestID == nil || seen[*st.CallTestID] {
				continue
			}
			seen[*st.CallTestID] = true
			pc.calledBy[*st.CallTestID] = append(pc.calledBy[*st.CallTestID], model.ItemRef{Type: "calls", Direction: "incoming", Kind: model.KindTest, ID: t.ID, Key: t.Key, Title: t.Title})
		}
	}
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
	r.Coverage = store.CoverageOf(pc.r2t[r.ID], pc.cfgResults)
	r.CommentCount = pc.comments[r.ID]
	r.Relations = pc.relCount[r.ID]
	r.RelationRefs = pc.refs[r.ID]
	r.CriteriaCount = len(r.Criteria)
	r.SuspectLinks = pc.suspect[r.ID]
	r.OpenDefects = pc.openDefects[r.ID]
	r.RiskLevel = pc.riskLevel[r.ID]
}

func (pc *projectContext) decorateTest(t *model.TestCase) {
	t.FolderPath = pc.folderPath(t.FolderID)
	t.LinkedReqs = len(pc.t2r[t.ID])
	t.LastResult = pc.last[t.ID]
	if t.LastResult == "" {
		t.LastResult = model.ResultNotRun
	}
	t.CommentCount = pc.comments[t.ID]
	t.StepCount = len(t.Steps)
	t.Defects = pc.defects[t.ID]
	t.SuspectLinks = pc.suspect[t.ID]
	refs := append([]model.ItemRef{}, pc.refs[t.ID]...)
	seen := map[string]bool{}
	for _, st := range t.Steps {
		if st.CallTestID != nil && !seen[*st.CallTestID] {
			seen[*st.CallTestID] = true
			refs = append(refs, model.ItemRef{Type: "calls", Direction: "outgoing", Kind: model.KindTest, ID: *st.CallTestID, Key: st.CallTestKey, Title: st.CallTestTitle})
		}
	}
	refs = append(refs, pc.calledBy[t.ID]...)
	for _, c := range t.CalledBy {
		if pc.calledBy == nil {
			refs = append(refs, model.ItemRef{Type: "calls", Direction: "incoming", Kind: model.KindTest, ID: c.ID, Key: c.Key, Title: c.Title})
		}
	}
	if len(refs) > 0 {
		t.RelationRefs = refs
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
		"related": pc.related[r.ID], "parent": pc.parents[r.ID], "child": pc.children[r.ID],
		"comments": {strconv.Itoa(r.CommentCount)},
		"suspect":  {strconv.Itoa(r.SuspectLinks)},
		"defects":  {strconv.Itoa(r.OpenDefects)},
		"risk":     {r.RiskLevel},
		"mention":  mentions(r.Owner, r.Title, r.Description, r.Rationale, r.Source),
	}
}

// mentions returns the @handles in the given texts, each also as "@handle"
// so that both mention:anna and mention:@anna match.
func mentions(texts ...string) []string {
	var out []string
	for _, h := range mention.Extract(strings.Join(texts, "\n")) {
		out = append(out, h, "@"+h)
	}
	return out
}

func (pc *projectContext) testRecord(t *model.TestCase) query.MapRecord {
	names, dims := pc.catValues(t.CategoryIDs)
	var steps, calls []string
	for _, st := range t.Steps {
		steps = append(steps, st.Action, st.Expected, st.Data)
		if st.CallTestKey != "" {
			calls = append(calls, st.CallTestKey)
		}
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
		"updated_at": {ts(t.UpdatedAt)}, "defect": pc.defects[t.ID], "calls": calls,
		"comments": {strconv.Itoa(t.CommentCount)}, "version": {strconv.Itoa(t.Version)}, "suspect": {strconv.Itoa(t.SuspectLinks)},
		"mention":    mentions(append([]string{t.Title, t.Description, t.Preconditions, t.ExpectedResult}, steps...)...),
		"depends_on": pc.prereqKeys[t.ID],
	}
}

func runRecord(r *model.TestRun) query.MapRecord {
	cfgs := make([]string, 0, len(r.Configurations))
	for _, c := range r.Configurations {
		cfgs = append(cfgs, c.Name)
	}
	return query.MapRecord{"id": {r.ID}, "name": {r.Name}, "description": {r.Description}, "status": {r.Status},
		"environment": {r.Environment}, "build": {r.Build}, "created_by": {r.CreatedBy}, "created_at": {ts(r.CreatedAt)}, "configuration": cfgs}
}

func executionRecord(e *model.Execution) query.MapRecord {
	return query.MapRecord{"id": {e.ID}, "key": {e.TestKey}, "title": {e.TestTitle}, "status": {e.Status},
		"assignee": {e.Assignee}, "executed_by": {e.ExecutedBy}, "notes": {e.Notes}, "output": {e.Output}, "defect": e.Defects,
		"mention": mentions(e.Assignee, e.Notes), "configuration": {e.ConfigurationName}}
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
