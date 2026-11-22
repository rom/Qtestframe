package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Transitive change impact (store/impact.go) at every place a change is
// made.
//
// The endpoints that change requirements or tests (PATCH, bulk edit and
// status, the spreadsheet batch, the Excel and ReqIF exchange, imports,
// restoring a version or a baseline) are wrapped by impactAware:
//
//   - ?dry_run=impact runs the change in a transaction, collects the items
//     whose substance (or status) changed, answers with their combined
//     impact and rolls everything back: nothing is saved.
//   - ?impact=suspect|notify|skip chooses what saving does with the impact
//     (model.ImpactChoices; suspect is the default and the earlier
//     behaviour).
//
// A single change applies its impact at once (afterContentChange); the
// changes of a bulk endpoint are collected and their impact is applied once,
// on the graph loaded once, before the transaction commits.

// impactKey holds the impactCollector of a request.
type impactKey struct{}

// impactCollector collects the changed items of a request.
type impactCollector struct {
	dry    bool
	choice string
	// batch: collect and apply at the end (else apply each at once).
	batch    bool
	projects []string
	roots    map[string][]store.ImpactRoot // project → roots
	reasons  map[string]string             // kind:id → reason
}

func collectorOf(ctx context.Context) *impactCollector {
	c, _ := ctx.Value(impactKey{}).(*impactCollector)
	return c
}

func (c *impactCollector) add(projectID string, root store.ImpactRoot, reason string) {
	k := root.Kind + ":" + root.ID
	if c.roots[projectID] == nil {
		c.projects = append(c.projects, projectID)
	}
	for i, r := range c.roots[projectID] {
		if r.Kind == root.Kind && r.ID == root.ID {
			// changed twice in one request: a change of substance wins
			if r.ReviewOnly && !root.ReviewOnly {
				c.roots[projectID][i] = root
				c.reasons[k] = reason
			}
			return
		}
	}
	c.roots[projectID] = append(c.roots[projectID], root)
	c.reasons[k] = reason
}

// ImpactDryRun is the answer of a change sent with ?dry_run=impact.
type ImpactDryRun struct {
	DryRun string `json:"dry_run"`
	// Status: the HTTP status the change would have answered with.
	Status int                 `json:"status"`
	Impact *model.ChangeImpact `json:"impact"`
}

// impactParams reads ?dry_run= and ?impact=.
func impactParams(r *http.Request) (dry bool, choice string, err error) {
	q := r.URL.Query()
	switch v := q.Get("dry_run"); v {
	case "impact":
		dry = true
	case "", "false", "0", "true", "1":
		// the boolean dry_run of the imports is theirs
	default:
		return false, "", badRequest("dry_run must be impact")
	}
	choice = q.Get("impact")
	if choice == "" {
		choice = model.ImpactApplySuspect
	}
	if !slices.Contains(model.ImpactChoices, choice) {
		return false, "", badRequest("impact must be one of %s", strings.Join(model.ImpactChoices, ", "))
	}
	return dry, choice, nil
}

// impactAware wraps an endpoint that changes requirements or tests (see
// above). batch: the endpoint changes several items (it is then atomic, as
// s.atomic).
func (s *Server) impactAware(fn handlerFunc, batch bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		dry, choice, err := impactParams(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		col := &impactCollector{dry: dry, choice: choice, batch: batch, roots: map[string][]store.ImpactRoot{}, reasons: map[string]string{}}
		r = withOverride(r.WithContext(context.WithValue(r.Context(), impactKey{}, col)))
		if !dry && !batch {
			fn(w, r, p)
			return
		}
		rec := &heldResponse{header: http.Header{}}
		var out *ImpactDryRun
		err = s.Store.InTx(r.Context(), func(ctx context.Context) error {
			fn(rec, r.WithContext(ctx), p)
			if rec.status() >= 400 {
				return errRolledBack
			}
			if dry {
				out, err = s.dryRunImpact(ctx, p, col)
				if err != nil {
					return err
				}
				out.Status = rec.status()
				return errRolledBack
			}
			s.flushImpact(ctx, p, col)
			return nil
		})
		if err != nil && !errors.Is(err, errRolledBack) {
			s.fail(w, r, err)
			return
		}
		if out != nil {
			writeJSON(w, http.StatusOK, out)
			return
		}
		rec.send(w)
	}
}

// dryRunImpact returns the combined impact of the collected changes.
func (s *Server) dryRunImpact(ctx context.Context, p *Principal, col *impactCollector) (*ImpactDryRun, error) {
	out := &ImpactDryRun{DryRun: "impact"}
	for _, pid := range col.projects {
		x, err := s.Store.LoadImpactIndex(ctx, p.TenantID(), pid)
		if err != nil {
			return nil, err
		}
		imp, _ := x.Analyze(col.roots[pid], 0)
		s.redactImpact(p, imp)
		out.Impact = mergeImpact(out.Impact, imp)
	}
	if out.Impact == nil {
		out.Impact = emptyImpact()
	}
	return out, nil
}

func emptyImpact() *model.ChangeImpact {
	imp := &model.ChangeImpact{Roots: []model.ImpactRef{}, ByKind: map[string]int{}, ByDepth: map[string]int{}}
	for _, l := range imp.Lists() {
		*l = []model.ImpactItem{}
	}
	return imp
}

// mergeImpact adds the impact b (another project) to a.
func mergeImpact(a, b *model.ChangeImpact) *model.ChangeImpact {
	if a == nil {
		return b
	}
	la, lb := a.Lists(), b.Lists()
	for k, l := range lb {
		*la[k] = append(*la[k], *l...)
	}
	a.Roots = append(a.Roots, b.Roots...)
	a.Depth = max(a.Depth, b.Depth)
	a.Kind, a.ID, a.Key, a.Title = "", "", "", ""
	store.Recount(a)
	return a
}

// flushImpact applies the impact of the collected changes.
func (s *Server) flushImpact(ctx context.Context, p *Principal, col *impactCollector) {
	for _, pid := range col.projects {
		s.applyImpact(ctx, p, pid, col.roots[pid], col.reasons, col.choice)
	}
}

// recordImpact handles a changed item: collected (dry run, bulk change) or
// applied at once.
func (s *Server) recordImpact(ctx context.Context, p *Principal, projectID string, root store.ImpactRoot, reason string) {
	col := collectorOf(ctx)
	if col != nil && (col.dry || col.batch) {
		col.add(projectID, root, reason)
		return
	}
	choice := model.ImpactApplySuspect
	if col != nil {
		choice = col.choice
	}
	s.applyImpact(ctx, p, projectID, []store.ImpactRoot{root}, map[string]string{root.Kind + ":" + root.ID: reason}, choice)
}

// applyImpact carries out the impact of changes of a project's items under
// a choice: marks the relations and trace links the rules mark suspect,
// flags the tracker issues of changed requirements out of date, notifies
// owners, and lets the statuses of the affected requirements follow their
// evidence.
func (s *Server) applyImpact(ctx context.Context, p *Principal, projectID string, roots []store.ImpactRoot, reasons map[string]string, choice string) {
	var reqs []string
	var tests []string
	for _, r := range roots {
		if r.ReviewOnly {
			continue
		}
		if r.Kind == model.KindRequirement {
			// stale results and invalidated records may change its status even without links
			reqs = append(reqs, r.ID)
		} else {
			tests = append(tests, r.ID)
		}
	}
	defer func() {
		if len(tests) > 0 {
			reqs = append(reqs, s.requirementsOfTests(ctx, p, projectID, tests)...)
		}
		if len(reqs) > 0 {
			s.syncEvidence(ctx, p, projectID, uniqStrings(reqs))
		}
	}()
	if choice == model.ImpactApplySkip {
		s.auditImpactChoice(ctx, p, projectID, roots, choice, 0)
		return
	}
	// a status change only recommends reviews: its owners hear of it only
	// when they are notified on purpose
	use := roots
	if choice == model.ImpactApplySuspect {
		use = nil
		for _, r := range roots {
			if !r.ReviewOnly {
				use = append(use, r)
			}
		}
	}
	if len(use) == 0 {
		return
	}
	x, err := s.Store.LoadImpactIndex(ctx, p.TenantID(), projectID)
	if err != nil {
		s.Log.Warn("load impact", "err", err)
		return
	}
	imp, marks := x.Analyze(use, 0)
	reasonOf := func(rootKey string) string {
		for _, r := range imp.Roots {
			if r.Key == rootKey {
				return reasons[r.Kind+":"+r.ID]
			}
		}
		return ""
	}
	if choice == model.ImpactApplySuspect {
		linked := map[string]bool{}
		for _, m := range marks {
			if m.Already {
				continue
			}
			reason := reasonOf(m.Root)
			if m.Depth > 1 && m.From != m.Root {
				reason = store.PathText(m.Path) + ": " + reason
			}
			switch {
			case m.Edge == model.EdgeIssue:
				if ok, err := s.Store.MarkIssueOutdated(ctx, p.TenantID(), m.ID, reason); err != nil {
					s.Log.Warn("mark issue outdated", "err", err)
				} else if ok && !linked["issue:"+m.RequirementID] {
					linked["issue:"+m.RequirementID] = true
					s.audit(ctx, p, "requirement.issue.outdated", model.KindRequirement, m.RequirementID, m.Root+": "+reason)
				}
			case m.Edge == model.EdgeVerifies:
				ok, err := s.Store.MarkLinkSuspect(ctx, p.TenantID(), m.ID, reason)
				if err != nil {
					s.Log.Warn("mark link suspect", "err", err)
					continue
				}
				if ok {
					reqs = append(reqs, m.RequirementID)
					// one audit entry per changed item for its own links
					if m.From == m.Root && !linked[m.Root] {
						linked[m.Root] = true
						for _, r := range imp.Roots {
							if r.Key == m.Root {
								s.audit(ctx, p, "link.suspect", r.Kind, r.ID, reason)
							}
						}
					}
				}
			default:
				ok, err := s.Store.MarkRelationSuspect(ctx, p.TenantID(), m.ID, m.End, reason)
				if err != nil {
					s.Log.Warn("mark relation suspect", "err", err)
					continue
				}
				if ok {
					text := m.From + " " + m.Edge + " " + m.Key
					if m.End == "source" {
						text = m.Key + " " + m.Edge + " " + m.From
					}
					s.audit(ctx, p, "relation.suspect", "relation", m.ID, text+": "+reason)
				}
			}
		}
	}
	n := s.notifyImpactOwners(ctx, p, projectID, imp, choice == model.ImpactApplyNotify)
	if choice != model.ImpactApplySuspect {
		s.auditImpactChoice(ctx, p, projectID, roots, choice, n)
	}
}

func (s *Server) auditImpactChoice(ctx context.Context, p *Principal, projectID string, roots []store.ImpactRoot, choice string, notified int) {
	keys := make([]string, 0, len(roots))
	for _, r := range roots {
		keys = append(keys, r.Kind+":"+r.ID)
	}
	s.audit(ctx, p, "impact."+choice, "project", projectID, fmt.Sprintf("%s (%d notified)", strings.Join(keys, ","), notified))
}

// notifyImpactOwners tells the owners of the affected items (all of them,
// or only those recommended for review) that a change affects them; one
// notification per owner. Returns the number of notifications.
func (s *Server) notifyImpactOwners(ctx context.Context, p *Principal, projectID string, imp *model.ChangeImpact, all bool) int {
	byOwner := map[string][]string{}
	var owners []string
	for _, sec := range model.ImpactSections {
		for _, it := range *imp.Lists()[sec] {
			if it.Owner == "" || (!all && it.Mode != model.ModeReview) {
				continue
			}
			if byOwner[it.Owner] == nil {
				owners = append(owners, it.Owner)
			}
			if !slices.Contains(byOwner[it.Owner], it.Key) {
				byOwner[it.Owner] = append(byOwner[it.Owner], it.Key)
			}
		}
	}
	if len(owners) == 0 || len(imp.Roots) == 0 {
		return 0
	}
	var rootKeys []string
	for _, r := range imp.Roots {
		rootKeys = append(rootKeys, r.Key)
	}
	title := excerpt(strings.Join(rootKeys, ", "), 120) + " changed: review recommended"
	n := 0
	for _, o := range owners {
		users := s.recipients(ctx, p, o)
		if len(users) == 0 {
			continue
		}
		keys := byOwner[o]
		body := "Affected: " + strings.Join(keys[:min(len(keys), 25)], ", ")
		if len(keys) > 25 {
			body += fmt.Sprintf(" and %d more", len(keys)-25)
		}
		s.notifyUsers(ctx, p, users, model.Notification{ProjectID: projectID, Event: "impact.review", Title: title, Body: body,
			EntityType: imp.Roots[0].Kind, EntityID: imp.Roots[0].ID})
		n += len(users)
	}
	return n
}

// ---------------------------------------------------------------- analysis

// impactRequest is the body of POST /projects/{pid}/impact: the items to
// analyse (kind requirement | test), or the changes since a baseline.
type impactRequest struct {
	Items []struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"items"`
	BaselineID string `json:"baseline_id"`
	Depth      int    `json:"depth"`
}

// impactOf analyses a request in a project.
func (s *Server) impactOf(r *http.Request, p *Principal, pr *model.Project, req impactRequest) (*model.ChangeImpact, error) {
	ctx := r.Context()
	if req.Depth < 0 || req.Depth > model.MaxImpactDepth {
		return nil, badRequest("depth must be between 1 and %d", model.MaxImpactDepth)
	}
	x, err := s.Store.LoadImpactIndex(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	var roots []store.ImpactRoot
	for _, it := range req.Items {
		if it.Kind != model.KindRequirement && it.Kind != model.KindTest {
			return nil, badRequest("kind must be requirement or test")
		}
		if it.Kind == model.KindTest && !p.Can(rbac.TestRead) {
			continue
		}
		if !x.Has(it.Kind, it.ID) {
			return nil, store.ErrNotFound
		}
		roots = append(roots, store.ImpactRoot{Kind: it.Kind, ID: it.ID})
	}
	if req.BaselineID != "" {
		b, err := s.Store.GetBaseline(ctx, p.TenantID(), req.BaselineID)
		if err != nil {
			return nil, err
		}
		if b.ProjectID != pr.ID {
			return nil, store.ErrNotFound
		}
		items, err := s.currentItems(r, p, pr, "")
		if err != nil {
			return nil, err
		}
		diff := store.CompareItems(b.Name, "current", b.Items, items)
		for _, c := range diff.Changed {
			if c.EntityType == model.KindTest && !p.Can(rbac.TestRead) {
				continue
			}
			content := store.ContentChanged(c.EntityType, c.ChangedFields)
			if len(content) == 0 && !slices.Contains(c.ChangedFields, "status") {
				continue
			}
			roots = append(roots, store.ImpactRoot{Kind: c.EntityType, ID: c.EntityID, ReviewOnly: len(content) == 0, Fields: c.ChangedFields})
		}
	}
	if len(req.Items) == 0 && req.BaselineID == "" {
		return nil, badRequest("choose at least one item or a baseline")
	}
	imp, _ := x.Analyze(roots, req.Depth)
	s.redactImpact(p, imp)
	return imp, nil
}

// handleProjectImpact: POST /projects/{pid}/impact — the impact analysis
// of items or of a baseline diff.
func (s *Server) handleProjectImpact(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req impactRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	imp, err := s.impactOf(r, p, pr, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, imp)
}

// handleProjectImpactExport: GET /projects/{pid}/impact/export — the
// impact analysis as CSV or PDF (?items=kind:id,…, ?baseline=, ?depth=,
// ?format=csv|pdf, ?lang=en|sv).
func (s *Server) handleProjectImpactExport(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := export.Format(q.Get("format"))
	if f == "" {
		f = export.CSV
	}
	if f != export.CSV && f != export.PDF {
		s.fail(w, r, badRequest("format must be csv or pdf"))
		return
	}
	var req impactRequest
	for _, v := range strings.Split(q.Get("items"), ",") {
		kind, id, ok := strings.Cut(strings.TrimSpace(v), ":")
		if ok {
			req.Items = append(req.Items, struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			}{kind, id})
		}
	}
	req.BaselineID = q.Get("baseline")
	if v := q.Get("depth"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil {
			s.fail(w, r, badRequest("depth must be a number"))
			return
		}
		req.Depth = d
	}
	imp, err := s.impactOf(r, p, pr, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	lang := reportLang(r, p, q.Get("lang"))
	ds := impactDataset(pr, imp, lang)
	if f == export.CSV {
		// one file: the rows only
		ds.Sections = ds.Sections[1:]
	}
	name := fmt.Sprintf("%s-impact-%s.%s", pr.Key, time.Now().UTC().Format("20060102-150405"), f.Extension(false))
	w.Header().Set("Content-Type", f.ContentTypeFor(false))
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if err := export.Write(w, f, ds); err != nil {
		s.Log.Error("impact export failed", "err", err)
		return
	}
	s.audit(r.Context(), p, "project.export", "project", pr.ID, string(f)+" impact")
}

// impactText holds the texts of the impact export per language.
var impactText = map[string]map[string]string{
	"en": {"title": "Change impact", "changed": "Changed", "depth": "Depth", "section": "Impact", "kind": "Kind", "key": "Key", "itemTitle": "Title",
		"mode": "Effect", "state": "State", "path": "Path", "owner": "Owner", "suspect": "suspect", "review": "review recommended", "already": "already marked",
		"requirement": "Requirement", "test": "Test", "risk": "Risk", "defect": "Defect", "variant": "Variant", "code": "Code change", "run": "Run",
		"issue": "Tracker issue", "document": "Document", "result": "Result", "summary": "Summary", "count": "Items", "baseline": "Baseline"},
	"sv": {"title": "Ändringspåverkan", "changed": "Ändrat", "depth": "Djup", "section": "Påverkan", "kind": "Typ", "key": "Nyckel", "itemTitle": "Titel",
		"mode": "Effekt", "state": "Läge", "path": "Väg", "owner": "Ägare", "suspect": "misstänkt", "review": "granskning rekommenderas", "already": "redan markerad",
		"requirement": "Krav", "test": "Test", "risk": "Risk", "defect": "Defekt", "variant": "Variant", "code": "Kodändring", "run": "Körning",
		"issue": "Ärende", "document": "Dokument", "result": "Resultat", "summary": "Sammanfattning", "count": "Poster", "baseline": "Baslinje"},
}

func impactDataset(pr *model.Project, imp *model.ChangeImpact, lang string) export.Dataset {
	tx := impactText["en"]
	if lang == "sv" {
		tx = impactText["sv"]
	}
	label := func(k string) string {
		if v, ok := tx[k]; ok {
			return v
		}
		return k
	}
	var roots []string
	for _, r := range imp.Roots {
		roots = append(roots, r.Key)
	}
	ds := export.Dataset{Title: tx["title"] + " — " + pr.Name, Subtitle: tx["changed"] + ": " + strings.Join(roots, ", "), GeneratedAt: time.Now().UTC(), Lang: lang,
		Meta: []export.Item{{Label: tx["changed"], Value: strings.Join(roots, ", ")}, {Label: tx["depth"], Value: strconv.Itoa(imp.Depth)}}}
	sum := export.Section{Name: "summary", Title: tx["summary"], Columns: []string{tx["kind"], tx["count"]}}
	main := export.Section{Name: "impact", Title: tx["section"], Columns: []string{tx["kind"], tx["key"], tx["itemTitle"], tx["depth"], tx["mode"], tx["state"], tx["owner"], tx["path"]}}
	lists := imp.Lists()
	for _, sec := range model.ImpactSections {
		l := *lists[sec]
		if len(l) == 0 {
			continue
		}
		sum.Rows = append(sum.Rows, []string{label(l[0].Kind), strconv.Itoa(len(l))})
		for _, it := range l {
			state := ""
			if it.Already {
				state = tx["already"]
			}
			main.Rows = append(main.Rows, []string{label(it.Kind), it.Key, it.Title, strconv.Itoa(it.Depth), label(it.Mode), state, it.Owner, store.PathText(it.Path)})
		}
	}
	ds.Sections = []export.Section{sum, main}
	return ds
}

// itemImpactDepth reads ?depth= of the impact of one item (0: the
// project's depth).
func itemImpactDepth(r *http.Request) (int, error) {
	v := r.URL.Query().Get("depth")
	if v == "" {
		return 0, nil
	}
	d, err := strconv.Atoi(v)
	if err != nil || d < 1 || d > model.MaxImpactDepth {
		return 0, badRequest("depth must be between 1 and %d", model.MaxImpactDepth)
	}
	return d, nil
}
