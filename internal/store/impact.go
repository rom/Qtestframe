package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// Transitive change impact.
//
// The impact graph of a project is loaded once (LoadImpactIndex): its
// requirements, tests, risks, open defects, variants and code changes are
// the nodes; requirement relations, trace links (verifies), item links,
// risk ↔ requirement, defect ↔ item, applicability and code links are the
// edges. Analyze walks it breadth first from the changed items (the roots)
// along the edges whose rule (CorrelationSettings.Rule) lets the change
// travel in that direction, up to the project's impact depth, visiting
// every item once (cycle safe) and keeping the shortest path to it.
//
// Depth counts the steps between requirements, tests and risks. Trace
// links attach evidence without counting as a step and are not followed
// further (the tests verifying a reached requirement, the requirements a
// reached test verifies: their other end did not change) except to their
// own defects, variants and code changes; the defects, variants, code
// changes and runs of a reached item are attached the same way. So depth 1 is the impact of the earlier one-level analysis: the
// requirements related directly, the tests of the changed requirement and
// of those requirements. Tracker issues, generated documents and
// results belong to the changed items only (they become out of date,
// outdated or stale because those changed).
//
// The mode of an item is the weakest mode along its path: suspect only when
// every edge on the way marks suspect. A change that is not one of
// substance (a status change: ImpactRoot.ReviewOnly) only recommends
// reviews. Analyze also returns the marks to set (ImpactMark): the
// relations, trace links and issues that become suspect or out of date.

// impactNode is a node of the impact graph.
type impactNode struct {
	kind, id, key, title, owner, status, url string
}

// impactEdge is an edge of the impact graph seen from one of its ends.
type impactEdge struct {
	// edge: relation type or model.Edge* kind; id: the relation, trace
	// link or item link (empty for the others).
	edge, id string
	to       string // node key of the other end
	// forward: this end is the natural source of the edge.
	forward bool
	// already: the relation or trace link is already suspect.
	already bool
}

// ImpactIndex holds what the change impact of a project's items needs,
// loaded once (one item, every changed item of a bulk change, an import or
// a baseline diff).
type ImpactIndex struct {
	Settings *model.CorrelationSettings
	Stale    *StaleIndex
	nodes    map[string]*impactNode // kind:id → node
	adj      map[string][]impactEdge
	issues   map[string][]model.ItemLink // requirement id → issues
	docs     map[string][]documentEntry  // kind:id → documents
	last     map[string]string           // test id → latest result
}

func nodeKey(kind, id string) string { return kind + ":" + id }

// ImpactRoot is a changed item.
type ImpactRoot struct {
	Kind, ID string
	// ReviewOnly: not a change of substance (a status change).
	ReviewOnly bool
	Fields     []string
}

// ImpactMark is a mark a change sets: a relation or trace link that
// becomes suspect, a tracker issue that becomes out of date.
type ImpactMark struct {
	// Edge: relation type, model.EdgeVerifies or model.EdgeIssue; ID: the
	// relation, trace link or item link.
	Edge, ID string
	// End (relations): the suspect end, source | target.
	End string
	// Kind, ItemID, Key: the affected item; RequirementID: the requirement
	// of a trace link.
	Kind, ItemID, Key, RequirementID string
	// Root: key of the changed item; From: key of the item the change
	// came from (the step before); Depth and Path as in the impact.
	Root, From string
	Depth      int
	Path       []model.ImpactStep
	Already    bool
}

// LoadImpactIndex loads the impact graph of a project.
func (s *Store) LoadImpactIndex(ctx context.Context, tenantID, projectID string) (*ImpactIndex, error) {
	x := &ImpactIndex{nodes: map[string]*impactNode{}, adj: map[string][]impactEdge{}, issues: map[string][]model.ItemLink{}, docs: map[string][]documentEntry{}}
	var err error
	if x.Settings, err = s.CorrelationSettings(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	if x.Stale, err = s.LoadStaleIndex(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	c := s.c(ctx)
	scan := func(q string, args []any, fn func(*sql.Rows) error) error {
		rows, err := c.query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := fn(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	pa := []any{tenantID, projectID}
	addNode := func(n *impactNode) { x.nodes[nodeKey(n.kind, n.id)] = n }
	link := func(edge, id, from, to string, already bool) {
		x.adj[from] = append(x.adj[from], impactEdge{edge: edge, id: id, to: to, forward: true, already: already})
		x.adj[to] = append(x.adj[to], impactEdge{edge: edge, id: id, to: from, forward: false, already: already})
	}
	// nodes: requirements, tests
	if err := scan(`SELECT id, key, title, owner, status FROM requirements WHERE tenant_id = ? AND project_id = ?`, pa, func(r *sql.Rows) error {
		n := &impactNode{kind: model.KindRequirement}
		if err := r.Scan(&n.id, &n.key, &n.title, &n.owner, &n.status); err != nil {
			return err
		}
		addNode(n)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := scan(`SELECT id, key, title, created_by, status FROM test_cases WHERE tenant_id = ? AND project_id = ?`, pa, func(r *sql.Rows) error {
		n := &impactNode{kind: model.KindTest}
		if err := r.Scan(&n.id, &n.key, &n.title, &n.owner, &n.status); err != nil {
			return err
		}
		addNode(n)
		return nil
	}); err != nil {
		return nil, err
	}
	has := func(kind, id string) bool { _, ok := x.nodes[nodeKey(kind, id)]; return ok }
	// relations and trace links
	if err := scan(`SELECT id, source_id, target_id, type, CASE WHEN suspect_since IS NULL THEN 0 ELSE 1 END FROM requirement_relations WHERE tenant_id = ? AND project_id = ?`, pa, func(r *sql.Rows) error {
		var id, src, dst, typ string
		var sus int
		if err := r.Scan(&id, &src, &dst, &typ, &sus); err != nil {
			return err
		}
		if src != dst && has(model.KindRequirement, src) && has(model.KindRequirement, dst) {
			link(typ, id, nodeKey(model.KindRequirement, src), nodeKey(model.KindRequirement, dst), sus == 1)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := scan(`SELECT id, test_case_id, requirement_id, CASE WHEN suspect_since IS NULL THEN 0 ELSE 1 END FROM trace_links WHERE tenant_id = ? AND project_id = ? ORDER BY created_at, id`, pa, func(r *sql.Rows) error {
		var id, tid, rid string
		var sus int
		if err := r.Scan(&id, &tid, &rid, &sus); err != nil {
			return err
		}
		if has(model.KindTest, tid) && has(model.KindRequirement, rid) {
			link(model.EdgeVerifies, id, nodeKey(model.KindTest, tid), nodeKey(model.KindRequirement, rid), sus == 1)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// shared-step calls and dependencies between tests
	if err := scan(`SELECT DISTINCT s.test_case_id, s.call_test_id FROM test_steps s JOIN test_cases t ON t.id = s.test_case_id
		WHERE t.tenant_id = ? AND t.project_id = ? AND s.call_test_id IS NOT NULL`, pa, func(r *sql.Rows) error {
		var caller, called string
		if err := r.Scan(&caller, &called); err != nil {
			return err
		}
		if caller != called && has(model.KindTest, called) {
			link(model.EdgeCalls, "", nodeKey(model.KindTest, caller), nodeKey(model.KindTest, called), false)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := scan(`SELECT id, test_id, depends_on_id FROM test_dependencies WHERE tenant_id = ? AND project_id = ?`, pa, func(r *sql.Rows) error {
		var id, a, b string
		if err := r.Scan(&id, &a, &b); err != nil {
			return err
		}
		if a != b && has(model.KindTest, a) && has(model.KindTest, b) {
			link(model.EdgeTestDeps, id, nodeKey(model.KindTest, a), nodeKey(model.KindTest, b), false)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// item links between requirements and tests, and to runs
	if err := scan(`SELECT l.id, l.source_type, l.source_id, l.target_type, COALESCE(l.target_id, ''), l.label, COALESCE(r.name, '')
		FROM item_links l LEFT JOIN test_runs r ON l.target_type = 'run' AND r.id = l.target_id
		WHERE l.tenant_id = ? AND l.project_id = ? AND l.source_type IN ('requirement', 'test') AND l.target_type IN ('requirement', 'test', 'run')`, pa, func(r *sql.Rows) error {
		var id, st, sid, tt, tid, label, runName string
		if err := r.Scan(&id, &st, &sid, &tt, &tid, &label, &runName); err != nil {
			return err
		}
		if !has(st, sid) {
			return nil
		}
		if tt == "run" {
			if runName == "" {
				return nil
			}
			if !has("run", tid) {
				addNode(&impactNode{kind: "run", id: tid, key: runName, title: label})
			}
		} else if !has(tt, tid) || (st == tt && sid == tid) {
			return nil
		}
		link(model.EdgeItemLink, id, nodeKey(st, sid), nodeKey(tt, tid), false)
		return nil
	}); err != nil {
		return nil, err
	}
	// risks
	if err := scan(`SELECT k.id, k.key, k.title, k.owner, k.status, rr.requirement_id FROM risks k JOIN risk_requirements rr ON rr.risk_id = k.id
		WHERE k.tenant_id = ? AND k.project_id = ?`, pa, func(r *sql.Rows) error {
		n := &impactNode{kind: model.KindRisk}
		var rid string
		if err := r.Scan(&n.id, &n.key, &n.title, &n.owner, &n.status, &rid); err != nil {
			return err
		}
		if !has(model.KindRisk, n.id) {
			addNode(n)
		}
		if has(model.KindRequirement, rid) {
			link(model.EdgeRisk, "", nodeKey(model.KindRisk, n.id), nodeKey(model.KindRequirement, rid), false)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// open defects of requirements and tests
	if err := scan(`SELECT d.id, d.key, d.title, d.assignee, d.created_by, d.status, l.entity_type, l.entity_id FROM defects d JOIN defect_links l ON l.defect_id = d.id
		WHERE d.tenant_id = ? AND d.project_id = ? AND d.status IN ('open', 'in_progress') AND l.entity_type IN ('requirement', 'test')`, pa, func(r *sql.Rows) error {
		n := &impactNode{kind: model.KindDefect}
		var by, et, eid string
		if err := r.Scan(&n.id, &n.key, &n.title, &n.owner, &by, &n.status, &et, &eid); err != nil {
			return err
		}
		if n.owner == "" {
			n.owner = by
		}
		if !has(model.KindDefect, n.id) {
			addNode(n)
		}
		if has(et, eid) {
			link(model.EdgeDefect, "", nodeKey(model.KindDefect, n.id), nodeKey(et, eid), false)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// variants the items apply to (explicitly chosen ones)
	variants := map[string]string{}
	if err := scan(`SELECT id, name FROM variants WHERE tenant_id = ? AND project_id = ?`, pa, func(r *sql.Rows) error {
		var id, name string
		if err := r.Scan(&id, &name); err != nil {
			return err
		}
		variants[id] = name
		return nil
	}); err != nil {
		return nil, err
	}
	if len(variants) > 0 {
		if err := scan(`SELECT item_type, item_id, variant_ids FROM item_applicability WHERE tenant_id = ? AND project_id = ?`, pa, func(r *sql.Rows) error {
			var it, iid, raw string
			if err := r.Scan(&it, &iid, &raw); err != nil {
				return err
			}
			var ids []string
			_ = json.Unmarshal([]byte(raw), &ids)
			for _, v := range ids {
				name, ok := variants[v]
				if !ok || !has(it, iid) {
					continue
				}
				if !has("variant", v) {
					addNode(&impactNode{kind: "variant", id: v, key: name})
				}
				link(model.EdgeVariant, "", nodeKey(it, iid), nodeKey("variant", v), false)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	// code changes referring to the items
	if err := scan(`SELECT c.id, c.kind, c.ref, c.title, c.status, c.url, i.item_type, i.item_id FROM code_changes c JOIN code_change_items i ON i.change_id = c.id
		WHERE c.tenant_id = ? AND c.project_id = ?`, pa, func(r *sql.Rows) error {
		n := &impactNode{kind: "code"}
		var kind, it, iid string
		if err := r.Scan(&n.id, &kind, &n.key, &n.title, &n.status, &n.url, &it, &iid); err != nil {
			return err
		}
		if kind == "merge_request" {
			n.key = "!" + n.key
		} else if len(n.key) > 10 {
			n.key = n.key[:10]
		}
		if !has("code", n.id) {
			addNode(n)
		}
		if has(it, iid) {
			link(model.EdgeCode, "", nodeKey("code", n.id), nodeKey(it, iid), false)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	issues, err := s.IssueLinks(ctx, tenantID, projectID, nil)
	if err != nil {
		return nil, err
	}
	for _, l := range issues {
		x.issues[l.SourceID] = append(x.issues[l.SourceID], l)
	}
	entries, err := s.documentEntries(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		x.docs[e.kind+":"+e.id] = append(x.docs[e.kind+":"+e.id], e)
	}
	if x.last, err = s.LastResults(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	return x, nil
}

// Has reports whether an item is in the index.
func (x *ImpactIndex) Has(kind, id string) bool {
	_, ok := x.nodes[nodeKey(kind, id)]
	return ok
}

// Of returns what a change of the substance of a requirement or test
// would make suspect, out of date, outdated or stale, or only recommend
// for review (nil when the item is not in the project).
func (x *ImpactIndex) Of(kind, id string) *model.ChangeImpact {
	if !x.Has(kind, id) {
		return nil
	}
	imp, _ := x.Analyze([]ImpactRoot{{Kind: kind, ID: id}}, 0)
	return imp
}

// allowed reports whether a change travels along an edge seen from the
// changed end (forward: it is the natural source) under a direction.
func allowed(direction string, forward bool) bool {
	switch direction {
	case model.ImpactBoth:
		return true
	case model.ImpactOutgoing:
		return forward
	case model.ImpactIncoming:
		return !forward
	}
	return false
}

// leafKinds are attached to a reached item without counting as a step
// and are not followed further.
var leafKinds = map[string]bool{model.KindDefect: true, "variant": true, "code": true, "run": true}

// sectionOf returns the section of an impact item kind.
func sectionOf(kind string) string {
	switch kind {
	case model.KindRequirement:
		return "requirements"
	case model.KindTest:
		return "tests"
	case model.KindRisk:
		return "risks"
	case model.KindDefect:
		return "defects"
	case "variant":
		return "variants"
	case "code":
		return "code"
	case "issue":
		return "issues"
	case "document":
		return "documents"
	case "result":
		return "results"
	}
	return "links"
}

func dirText(forward bool) string {
	if forward {
		return "outgoing"
	}
	return "incoming"
}

// Analyze returns the impact of changes of the roots up to depth (0: the
// project's impact depth) and the marks the change sets. Roots not in the
// project are left out.
func (x *ImpactIndex) Analyze(roots []ImpactRoot, depth int) (*model.ChangeImpact, []ImpactMark) {
	if depth <= 0 {
		depth = x.Settings.Depth()
	}
	depth = min(depth, model.MaxImpactDepth)
	imp := &model.ChangeImpact{Requirements: []model.ImpactItem{}, Tests: []model.ImpactItem{}, Issues: []model.ImpactItem{}, Documents: []model.ImpactItem{},
		Results: []model.ImpactItem{}, Risks: []model.ImpactItem{}, Defects: []model.ImpactItem{}, Links: []model.ImpactItem{}, Variants: []model.ImpactItem{},
		Code: []model.ImpactItem{}, Roots: []model.ImpactRef{}, Depth: depth, ByKind: map[string]int{}, ByDepth: map[string]int{}}
	type visit struct {
		key   string
		depth int
		mode  string
		path  []model.ImpactStep
		// leaves: only attach the leaves (defects, variants, code) of
		// an item reached as evidence
		leaves bool
	}
	isRoot := map[string]bool{}
	var queue []visit
	for _, r := range roots {
		k := nodeKey(r.Kind, r.ID)
		n := x.nodes[k]
		if n == nil || isRoot[k] {
			continue
		}
		isRoot[k] = true
		imp.Roots = append(imp.Roots, model.ImpactRef{Kind: n.kind, ID: n.id, Key: n.key, Title: n.title, ReviewOnly: r.ReviewOnly, Fields: r.Fields})
		mode := model.ModeSuspect
		if r.ReviewOnly {
			mode = model.ModeReview
		}
		queue = append(queue, visit{key: k, mode: mode, path: []model.ImpactStep{{Kind: n.kind, ID: n.id, Key: n.key}}})
	}
	if len(imp.Roots) == 1 {
		imp.Kind, imp.ID, imp.Key, imp.Title = imp.Roots[0].Kind, imp.Roots[0].ID, imp.Roots[0].Key, imp.Roots[0].Title
	}
	items := map[string]*model.ImpactItem{}
	var order []string
	var marks []ImpactMark
	weaker := func(a, b string) string {
		if a == model.ModeReview || b == model.ModeReview {
			return model.ModeReview
		}
		return model.ModeSuspect
	}
	// the changed items' own issues, documents and results
	for _, v := range queue {
		n := x.nodes[v.key]
		root := v.path[0]
		leaf := func(it model.ImpactItem) {
			k := it.Kind + ":" + it.ID
			if _, ok := items[k]; ok {
				return
			}
			it.Depth = 1
			it.Path = []model.ImpactStep{root, {Kind: it.Kind, ID: it.ID, Key: it.Key, Edge: it.Kind, Direction: "outgoing"}}
			items[k] = &it
			order = append(order, k)
		}
		if n.kind == model.KindRequirement && x.Settings.FlagIssues {
			for _, l := range x.issues[n.id] {
				mode := v.mode
				it := model.ImpactItem{Kind: "issue", ID: l.ID, Key: l.Issue.Key, Title: l.Label, URL: l.URL, RequirementID: n.id, Mode: mode, Status: l.Issue.State}
				it.Already = mode == model.ModeSuspect && l.Issue.OutdatedSince != nil
				leaf(it)
				if mode == model.ModeSuspect {
					marks = append(marks, ImpactMark{Edge: model.EdgeIssue, ID: l.ID, Kind: "issue", ItemID: l.ID, Key: l.Issue.Key, RequirementID: n.id,
						Root: n.key, From: n.key, Depth: 1, Already: l.Issue.OutdatedSince != nil})
				}
			}
		}
		for _, e := range x.docs[v.key] {
			leaf(model.ImpactItem{Kind: "document", ID: e.fileID, Key: e.fileName, Mode: v.mode, Already: v.mode == model.ModeSuspect && e.outdated()})
		}
		if x.Settings.StaleOnChange && v.mode == model.ModeSuspect {
			var tests []string
			if n.kind == model.KindTest {
				tests = []string{n.id}
			} else {
				for _, e := range x.adj[v.key] {
					if e.edge == model.EdgeVerifies {
						tests = append(tests, strings.TrimPrefix(e.to, model.KindTest+":"))
					}
				}
			}
			for _, tid := range tests {
				st, ok := x.last[tid]
				t := x.nodes[nodeKey(model.KindTest, tid)]
				if !ok || st == model.ResultNotRun || t == nil {
					continue
				}
				leaf(model.ImpactItem{Kind: "result", ID: tid, TestID: tid, Key: t.key, Title: t.title, Result: st, Mode: model.ModeSuspect, Already: x.Stale.Test(tid).Stale})
			}
		}
	}
	// breadth first along the edges
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		n := x.nodes[v.key]
		for _, e := range x.adj[v.key] {
			if isRoot[e.to] {
				continue
			}
			rule := x.Settings.Rule(e.edge)
			if !allowed(rule.Direction, e.forward) {
				continue
			}
			to := x.nodes[e.to]
			if v.leaves && !leafKinds[to.kind] {
				continue
			}
			// trace links attach the evidence: the tests of a reached
			// requirement, the requirements of a reached test; they are
			// not followed further (the other end did not change)
			evidence := e.edge == model.EdgeVerifies
			step := !evidence && !leafKinds[to.kind]
			if step && v.depth+1 > depth {
				continue
			}
			mode := weaker(v.mode, rule.Mode)
			path := make([]model.ImpactStep, len(v.path), len(v.path)+1)
			copy(path, v.path)
			path = append(path, model.ImpactStep{Kind: to.kind, ID: to.id, Key: to.key, Edge: e.edge, Direction: dirText(e.forward)})
			if mode == model.ModeSuspect && model.Markable(e.edge) {
				m := ImpactMark{Edge: e.edge, ID: e.id, Kind: to.kind, ItemID: to.id, Key: to.key, Root: v.path[0].Key, From: n.key, Depth: len(path) - 1, Path: path, Already: e.already}
				if e.edge == model.EdgeVerifies {
					m.RequirementID = n.id
					if to.kind == model.KindRequirement {
						m.RequirementID = to.id
					}
				} else if e.forward {
					m.End = "target"
				} else {
					m.End = "source"
				}
				marks = append(marks, m)
			}
			it, seen := items[e.to]
			if seen {
				if mode == model.ModeSuspect && it.Mode == model.ModeReview {
					// a stronger path at the same depth
					if len(path)-1 == it.Depth {
						it.Mode, it.Already, it.Path = mode, e.already, path
						if step || evidence {
							queue = append(queue, visit{key: e.to, depth: v.depth + 1, mode: mode, path: path, leaves: evidence})
						}
					}
				} else if mode == model.ModeSuspect && model.Markable(e.edge) {
					it.Already = it.Already && e.already
				}
				continue
			}
			it = &model.ImpactItem{Kind: to.kind, ID: to.id, Key: to.key, Title: to.title, Depth: len(path) - 1, Mode: mode, Path: path, Owner: to.owner, Status: to.status, URL: to.url}
			it.Already = mode == model.ModeSuspect && model.Markable(e.edge) && e.already
			switch {
			case to.kind == model.KindRequirement:
				it.Relation, it.Direction = e.edge, dirText(e.forward)
			case to.kind == model.KindTest && e.edge == model.EdgeVerifies:
				it.RequirementID = n.id
				if v.depth > 0 {
					it.Via = n.key
				}
			}
			items[e.to] = it
			order = append(order, e.to)
			if step || evidence {
				queue = append(queue, visit{key: e.to, depth: v.depth + 1, mode: mode, path: path, leaves: evidence})
			}
		}
	}
	lists := imp.Lists()
	for _, k := range order {
		it := items[k]
		sec := sectionOf(it.Kind)
		*lists[sec] = append(*lists[sec], *it)
		imp.ByKind[sec]++
		imp.ByDepth[strconv.Itoa(it.Depth)]++
		if it.Already {
			continue
		}
		imp.Total++
		if it.Mode == model.ModeSuspect {
			imp.Suspect++
		} else {
			imp.Review++
		}
	}
	for _, sec := range model.ImpactSections {
		l := *lists[sec]
		sort.SliceStable(l, func(i, j int) bool {
			if l[i].Depth != l[j].Depth {
				return l[i].Depth < l[j].Depth
			}
			return naturalLess(l[i].Key, l[j].Key)
		})
	}
	return imp, marks
}

// Recount recomputes the counts of an impact after items were left out
// (e.g. those a caller may not see).
func Recount(imp *model.ChangeImpact) {
	imp.Total, imp.Suspect, imp.Review = 0, 0, 0
	imp.ByKind, imp.ByDepth = map[string]int{}, map[string]int{}
	for sec, l := range imp.Lists() {
		for _, it := range *l {
			imp.ByKind[sec]++
			imp.ByDepth[strconv.Itoa(it.Depth)]++
			if it.Already {
				continue
			}
			imp.Total++
			if it.Mode == model.ModeSuspect {
				imp.Suspect++
			} else {
				imp.Review++
			}
		}
	}
}

// FilterImpact leaves out the items for which keep returns false, and the
// items whose path passes through one of them.
func FilterImpact(imp *model.ChangeImpact, keep func(it model.ImpactItem) bool) {
	dropped := map[string]bool{}
	for _, l := range imp.Lists() {
		for _, it := range *l {
			if !keep(it) {
				dropped[it.Kind+":"+it.ID] = true
			}
		}
	}
	for _, l := range imp.Lists() {
		out := (*l)[:0]
		for _, it := range *l {
			ok := !dropped[it.Kind+":"+it.ID]
			for _, st := range it.Path {
				if dropped[st.Kind+":"+st.ID] {
					ok = false
				}
			}
			if ok {
				out = append(out, it)
			}
		}
		*l = slices.Clip(out)
	}
	Recount(imp)
}

// PathText renders the path of an impact item: A → refines → B → verifies → T-12.
func PathText(path []model.ImpactStep) string {
	var b strings.Builder
	for i, st := range path {
		if i > 0 {
			b.WriteString(" → " + st.Edge + " → ")
		}
		b.WriteString(st.Key)
	}
	return b.String()
}

// ---------------------------------------------------------------- marks

// MarkRelationSuspect marks a relation suspect at an end (relations
// already suspect keep their date and reason). Returns whether it was
// newly marked.
func (s *Store) MarkRelationSuspect(ctx context.Context, tenantID, id, end, reason string) (bool, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE requirement_relations SET suspect_since = ?, suspect_reason = ?, suspect_end = ? WHERE id = ? AND tenant_id = ? AND suspect_since IS NULL`,
		s.now(), clip(reason, 500), end, id, tenantID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkLinkSuspect marks a trace link suspect (links already suspect keep
// their date and reason). Returns whether it was newly marked.
func (s *Store) MarkLinkSuspect(ctx context.Context, tenantID, id, reason string) (bool, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE trace_links SET suspect_since = ?, suspect_reason = ? WHERE id = ? AND tenant_id = ? AND suspect_since IS NULL`,
		s.now(), clip(reason, 500), id, tenantID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkIssueOutdated flags one tracker issue link out of date. Returns
// whether it was newly flagged.
func (s *Store) MarkIssueOutdated(ctx context.Context, tenantID, id, reason string) (bool, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE item_links SET issue_outdated_since = ?, issue_outdated_reason = ? WHERE tenant_id = ? AND id = ? AND issue_outdated_since IS NULL`,
		s.now(), clip(reason, 500), tenantID, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
