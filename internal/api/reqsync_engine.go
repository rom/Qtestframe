package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracker"
)

// The two-way sync engine. A run
//
//  1. reads the tracker items of the configured query and the linked items
//     the query no longer finds (no database transaction is open);
//  2. compares, per linked requirement and mapped field, the values of both
//     sides with the values both had after the last sync (store.SyncState):
//     a side whose value differs from its base changed. A field that
//     changed on one side goes to the other as its direction allows; one
//     that changed on both is decided by the conflict policy (manual:
//     a conflict waits for a decision in the conflict view);
//  3. writes to the tracker (paced, see syncPacer) and reads the written
//     items again, so the base is what the tracker stores;
//  4. writes to Qtestframe in one transaction through the regular
//     requirement handlers (PATCH /requirements/{id} with If-Match, POST
//     …/requirements), so permissions, change control, history, the audit
//     log and the impact choice of the configuration apply as for any
//     edit, and records the new base of every item;
//  5. syncs comments and links when they are turned on.
//
// A dry run does steps 1 and 2 and answers what it would do.

// syncOptions are the options of a run.
type syncOptions struct {
	trigger string // manual | schedule | webhook | resolve
	dry     bool
	// only: the tracker keys of a webhook or of a conflict being resolved
	// (nil: every item)
	only map[string]bool
	// override: the decision per field of a resolved conflict
	// (tracker | qtestframe | keep)
	override map[string]string
}

// syncSleep waits between tracker writes (replaced in tests).
var syncSleep = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// maxSyncWrites bounds the tracker writes of one run; the rest waits for
// the next run.
const maxSyncWrites = 500

// syncPacer spaces the writes to a tracker.
type syncPacer struct {
	gap    time.Duration
	last   time.Time
	writes int
}

var errWriteBudget = errors.New("the write limit of one sync is used up; the rest follows with the next sync")

func (p *syncPacer) wait(ctx context.Context) error {
	if p.writes >= maxSyncWrites {
		return errWriteBudget
	}
	if p.writes > 0 {
		if d := p.gap - time.Since(p.last); d > 0 {
			if err := syncSleep(ctx, d); err != nil {
				return err
			}
		}
	}
	p.writes++
	p.last = time.Now()
	return nil
}

// errRateLimited stops a run when the tracker limits the rate beyond the
// retries of internal/tracker.
type errRateLimited struct{ err *tracker.Error }

func (e *errRateLimited) Error() string { return e.err.Error() }

// fatalTrackerErr reports whether a tracker error must stop the run: the
// rate limit, rejected credentials, an unreachable tracker.
func fatalTrackerErr(err error) error {
	var te *tracker.Error
	if errors.As(err, &te) {
		switch te.Status {
		case http.StatusTooManyRequests:
			return &errRateLimited{te}
		case http.StatusUnauthorized:
			return err
		}
		return nil
	}
	if err != nil && (strings.Contains(err.Error(), "cannot reach") || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
		return err
	}
	return nil
}

// syncItem is the plan of a linked requirement.
type syncItem struct {
	ref  *store.ExternalRef
	rq   *model.Requirement
	it   *tracker.SourceItem
	L, R map[string]string // current values per field
	// pull and push: the fields to write on each side (requirement values)
	pull, push map[string]string
	// keep: fields whose difference is accepted as it is (conflict dismissed)
	keep      map[string]bool
	conflicts []model.ReqSyncFieldDiff
	warnings  []string
	failed    bool
	pushed    bool
	// touched: the tracker item changed since the last sync (its version)
	touched bool
}

type syncEngine struct {
	s      *Server
	p      *Principal
	pr     *model.Project
	cfg    *store.ReqSyncRow
	st     *model.ReqSyncSettings
	client tracker.SyncClient
	base   string // the source recorded with new references
	opt    syncOptions
	run    *model.ReqSyncRun
	pacer  *syncPacer
	// refs by tracker key and by requirement; comment refs by tracker id
	// and by local comment
	refs      map[string]*store.ExternalRef
	refByReq  map[string]*store.ExternalRef
	cmtRemote map[string]bool
	cmtLocal  map[string]bool
	items     map[string]*tracker.SourceItem
	// inScope: keys the query found
	inScope map[string]bool
	order   []string
	plans   []*syncItem
}

func (e *syncEngine) entry(x model.ReqSyncEntry) {
	if x.Action == "error" {
		e.run.Summary.Errors++
	}
	if len(e.run.Entries) < store.MaxSyncEntries {
		e.run.Entries = append(e.run.Entries, x)
	}
}

func (e *syncEngine) errEntry(si *syncItem, key, msg string) {
	x := model.ReqSyncEntry{ExternalID: key, Action: "error", Message: msg}
	if si != nil && si.rq != nil {
		x.RequirementID, x.RequirementKey, x.Title = si.rq.ID, si.rq.Key, si.rq.Title
		si.failed = true
	}
	e.entry(x)
}

// mapped returns the fields that take part (direction not none).
func (e *syncEngine) mapped() []*model.ReqSyncField {
	var out []*model.ReqSyncField
	for i := range e.st.Fields {
		if e.st.Fields[i].Direction != model.SyncNone {
			out = append(out, &e.st.Fields[i])
		}
	}
	return out
}

func (e *syncEngine) field(name string) *model.ReqSyncField {
	for _, f := range e.mapped() {
		if f.Field == name {
			return f
		}
	}
	return nil
}

func canPull(d string) bool { return d == model.SyncPull || d == model.SyncBoth }
func canPush(d string) bool { return d == model.SyncPush || d == model.SyncBoth }

func (e *syncEngine) values(rq *model.Requirement, it *tracker.SourceItem) (L, R map[string]string, pullable map[string]bool) {
	L, R, pullable = map[string]string{}, map[string]string{}, map[string]bool{}
	for _, f := range e.mapped() {
		if rq != nil {
			L[f.Field] = localSyncValue(rq, f.Field)
		}
		if it != nil {
			R[f.Field], pullable[f.Field] = remoteSyncValue(it, f)
		}
	}
	return
}

// newSyncEngine prepares a run (the tracker client of the import source).
func (s *Server) newSyncEngine(ctx context.Context, p *Principal, pr *model.Project, cfg *store.ReqSyncRow, opt syncOptions) (*syncEngine, error) {
	src, err := s.importSourceCtx(ctx, p, pr.ID, cfg.System)
	if err != nil {
		return nil, err
	}
	e := &syncEngine{s: s, p: p, pr: pr, cfg: cfg, st: &cfg.Settings, opt: opt,
		run:   &model.ReqSyncRun{ProjectID: pr.ID, System: cfg.System, Trigger: opt.trigger, DryRun: opt.dry, StartedBy: p.User.Email, StartedAt: s.now(), Entries: []model.ReqSyncEntry{}},
		pacer: &syncPacer{gap: time.Minute / time.Duration(max(cfg.Settings.MaxWritesPerMinute, 1))}}
	push := &cfg.Settings.Push
	if src.azure != nil {
		e.client = tracker.NewSyncAzure(src.azure, s.HTTPClient, push.ItemType)
		e.base = strings.TrimRight(src.azure.URL, "/") + "/" + src.azure.Project
		if e.st.Query == "" {
			e.st.Query = tracker.AzureWIQL(src.areaPath, strings.Split(src.types, ","))
		}
	} else {
		key := firstNonBlank(push.ProjectKey, src.jira.ProjectKey)
		e.client = tracker.NewSyncJira(src.jira, s.HTTPClient, key, push.ItemType)
		e.base = strings.TrimRight(src.jira.URL, "/")
		if e.st.Query == "" {
			e.st.Query = src.jql
		}
		if e.st.Query == "" && key != "" {
			e.st.Query = "project = " + key + " ORDER BY key"
		}
	}
	return e, nil
}

// load reads the references and the tracker items (step 1).
func (e *syncEngine) load(ctx context.Context) error {
	refs, err := e.s.Store.ListExternalRefs(ctx, e.p.TenantID(), e.pr.ID, e.cfg.System)
	if err != nil {
		return err
	}
	e.refs, e.refByReq, e.cmtRemote, e.cmtLocal = map[string]*store.ExternalRef{}, map[string]*store.ExternalRef{}, map[string]bool{}, map[string]bool{}
	for i := range refs {
		r := &refs[i]
		switch r.EntityType {
		case model.KindRequirement:
			e.refs[r.ExternalID] = r
			e.refByReq[r.EntityID] = r
		case "comment":
			e.cmtRemote[r.ExternalID] = true
			e.cmtLocal[r.EntityID] = true
		}
	}
	e.items, e.inScope = map[string]*tracker.SourceItem{}, map[string]bool{}
	add := func(list []tracker.SourceItem, scope bool) {
		for i := range list {
			it := &list[i]
			if e.opt.only != nil && !e.opt.only[it.Key] {
				continue
			}
			if _, ok := e.items[it.Key]; !ok {
				e.order = append(e.order, it.Key)
			}
			e.items[it.Key] = it
			if scope {
				e.inScope[it.Key] = true
			}
		}
	}
	// the query: every run, except one about linked items only
	needQuery := e.opt.only == nil
	for k := range e.opt.only {
		if e.refs[k] == nil && e.st.ImportNew {
			needQuery = true
		}
	}
	if needQuery {
		if e.st.Query == "" {
			return badRequest("no query: set the query of the sync (JQL or WIQL)")
		}
		res, err := e.client.Search(ctx, e.st.Query, tracker.MaxSourceItems)
		if err != nil {
			return err
		}
		add(res.Items, true)
		if res.Truncated {
			e.entry(model.ReqSyncEntry{Action: "skip", Message: fmt.Sprintf("the query finds more than %d items: only the first %d take part; narrow the query", len(res.Items), len(res.Items))})
		}
	}
	var missing []string
	for k, r := range e.refs {
		if r.EntityType != model.KindRequirement || e.items[k] != nil || (e.opt.only != nil && !e.opt.only[k]) {
			continue
		}
		missing = append(missing, k)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		list, err := e.client.Fetch(ctx, missing)
		if err != nil {
			return err
		}
		add(list, false)
	}
	return nil
}

// plan compares the sides of the linked items (step 2).
func (e *syncEngine) plan(ctx context.Context) error {
	keys := make([]string, 0, len(e.refs))
	for k := range e.refs {
		if e.opt.only == nil || e.opt.only[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		ref := e.refs[k]
		rq, err := e.s.Store.GetRequirement(ctx, e.p.TenantID(), ref.EntityID)
		if errors.Is(err, store.ErrNotFound) || err == nil && rq.ProjectID != e.pr.ID {
			e.entry(model.ReqSyncEntry{ExternalID: k, Action: "skip", Message: "the linked requirement no longer exists"})
			continue
		}
		if err != nil {
			return err
		}
		it := e.items[k]
		if it == nil {
			e.entry(model.ReqSyncEntry{RequirementID: rq.ID, RequirementKey: rq.Key, Title: rq.Title, ExternalID: k, Action: "skip", Message: "the tracker item no longer exists or the token cannot see it"})
			continue
		}
		e.plans = append(e.plans, e.compare(ref, rq, it))
	}
	return nil
}

// compare decides what to do with each field of a linked item.
func (e *syncEngine) compare(ref *store.ExternalRef, rq *model.Requirement, it *tracker.SourceItem) *syncItem {
	si := &syncItem{ref: ref, rq: rq, it: it, pull: map[string]string{}, push: map[string]string{}, keep: map[string]bool{},
		touched: ref.SyncedAt == nil || ref.RemoteVersion != it.Version}
	var pullable map[string]bool
	si.L, si.R, pullable = e.values(rq, it)
	base := ref.Sync
	for _, f := range e.mapped() {
		l, r := si.L[f.Field], si.R[f.Field]
		bl, hasL := "", false
		br, hasR := "", false
		if base != nil {
			bl, hasL = base.Local[f.Field]
			br, hasR = base.Remote[f.Field]
		}
		initial := !hasL || !hasR
		lChanged, rChanged := initial || l != bl, initial || r != br
		if l == r {
			continue
		}
		if !lChanged && !rChanged {
			continue // the difference was accepted before
		}
		decide := ""
		switch {
		case f.Direction == model.SyncPull:
			if rChanged {
				decide = model.SyncTrackerWins
			}
		case f.Direction == model.SyncPush:
			if lChanged {
				decide = model.SyncLocalWins
			}
		case rChanged && !lChanged:
			decide = model.SyncTrackerWins
		case lChanged && !rChanged:
			decide = model.SyncLocalWins
		default:
			decide = e.st.ConflictPolicy
		}
		if o, ok := e.opt.override[f.Field]; ok && decide != "" {
			decide = o
		}
		switch decide {
		case model.SyncTrackerWins:
			if !canPull(f.Direction) {
				continue
			}
			if !pullable[f.Field] {
				si.warnings = append(si.warnings, fmt.Sprintf("%s %q has no matching value in Qtestframe: map it in the sync settings", f.Field, strings.TrimPrefix(r, "?")))
				continue
			}
			si.pull[f.Field] = r
		case model.SyncLocalWins:
			if canPush(f.Direction) {
				si.push[f.Field] = l
			}
		case "keep":
			si.keep[f.Field] = true
		case model.SyncManual:
			b := bl
			if !hasL {
				b = ""
			}
			si.conflicts = append(si.conflicts, model.ReqSyncFieldDiff{Field: f.Field, Base: b, Local: l, Remote: strings.TrimPrefix(r, "?")})
		}
	}
	return si
}

func changeText(old, new string) string {
	return shorten(old, 80) + " → " + shorten(new, 80)
}

func fieldList(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pushes writes the changes of linked items to the tracker (step 3).
func (e *syncEngine) pushes(ctx context.Context) error {
	for _, si := range e.plans {
		if len(si.push) == 0 {
			continue
		}
		x := model.ReqSyncEntry{RequirementID: si.rq.ID, RequirementKey: si.rq.Key, Title: si.rq.Title, ExternalID: si.it.Key, Action: "push", Fields: fieldList(si.push), Changes: map[string]string{}}
		var writes []tracker.FieldWrite
		for _, field := range x.Fields {
			f := e.field(field)
			x.Changes[field] = changeText(strings.TrimPrefix(si.R[field], "?"), si.push[field])
			writes = append(writes, pushSyncValue(f, si.push[field]))
		}
		if e.opt.dry {
			e.entry(x)
			continue
		}
		if err := e.pacer.wait(ctx); err != nil {
			return err
		}
		err := e.client.Update(ctx, si.it.Key, si.it.Version, writes)
		if err != nil {
			if f := fatalTrackerErr(err); f != nil {
				return f
			}
			if errors.Is(err, tracker.ErrRemoteChanged) {
				err = errors.New("the item changed in the tracker meanwhile; it is compared again at the next sync")
			}
			e.errEntry(si, si.it.Key, "push: "+err.Error())
			continue
		}
		si.pushed = true
		e.run.Summary.Pushed++
		e.entry(x)
		e.s.audit(ctx, e.p, "requirement.sync.push", model.KindRequirement, si.rq.ID, si.rq.Key+" → "+sourceNames[e.cfg.System]+" "+si.it.Key+": "+strings.Join(x.Fields, ", "))
	}
	return nil
}

// createRemote creates tracker items for the requirements of the push
// scope that have none (step 3).
func (e *syncEngine) createRemote(ctx context.Context) ([]*syncItem, error) {
	if !e.st.Push.Create || e.opt.only != nil {
		return nil, nil
	}
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	pc, err := e.s.loadContext(ctx, e.p.TenantID(), e.pr)
	if err != nil {
		return nil, err
	}
	reqs, err := e.s.filterRequirements(r, e.p, e.pr, pc, e.st.Push.Query, e.st.Push.FolderID, true)
	if err != nil {
		return nil, badRequest("the push query: %v", err)
	}
	var out []*syncItem
	for i := range reqs {
		rq := &reqs[i]
		if e.refByReq[rq.ID] != nil {
			continue
		}
		if t := e.st.Push.Tag; t != "" && !slices.ContainsFunc(rq.Tags, func(x string) bool { return strings.EqualFold(x, t) }) {
			continue
		}
		full, err := e.s.Store.GetRequirement(ctx, e.p.TenantID(), rq.ID)
		if err != nil {
			return nil, err
		}
		L, _, _ := e.values(full, nil)
		var writes []tracker.FieldWrite
		x := model.ReqSyncEntry{RequirementID: full.ID, RequirementKey: full.Key, Title: full.Title, Action: "create_remote", Changes: map[string]string{}}
		hasTitle := false
		for _, f := range e.mapped() {
			if !canPush(f.Direction) || L[f.Field] == "" {
				continue
			}
			hasTitle = hasTitle || f.Field == "title"
			writes = append(writes, pushSyncValue(f, L[f.Field]))
			x.Fields = append(x.Fields, f.Field)
		}
		if !hasTitle {
			id := tracker.JiraSummary
			if e.cfg.System == tracker.SourceAzure {
				id = tracker.AzureTitle
			}
			writes = append(writes, tracker.FieldWrite{ID: id, Kind: tracker.WriteText, Value: full.Title})
		}
		if e.opt.dry {
			e.entry(x)
			continue
		}
		if err := e.pacer.wait(ctx); err != nil {
			return out, err
		}
		key, err := e.client.Create(ctx, "", writes)
		if key == "" && err != nil {
			if f := fatalTrackerErr(err); f != nil {
				return out, f
			}
			e.errEntry(&syncItem{rq: full}, "", "create: "+err.Error())
			continue
		}
		if err != nil {
			e.errEntry(&syncItem{rq: full}, key, err.Error())
		}
		x.ExternalID = key
		e.run.Summary.CreatedRemote++
		e.entry(x)
		e.s.audit(ctx, e.p, "requirement.sync.create", model.KindRequirement, full.ID, full.Key+" → "+sourceNames[e.cfg.System]+" "+key)
		out = append(out, &syncItem{rq: full, it: &tracker.SourceItem{Key: key}, L: L, pushed: true})
	}
	return out, nil
}

// reread reads the written items again: their base is what the tracker
// stores now.
func (e *syncEngine) reread(ctx context.Context, list []*syncItem) error {
	var keys []string
	for _, si := range list {
		if si.pushed && si.it != nil {
			keys = append(keys, si.it.Key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	items, err := e.client.Fetch(ctx, keys)
	if err != nil {
		return err
	}
	byKey := map[string]*tracker.SourceItem{}
	for i := range items {
		byKey[items[i].Key] = &items[i]
		e.items[items[i].Key] = &items[i]
	}
	for _, si := range list {
		if !si.pushed {
			continue
		}
		if it := byKey[si.it.Key]; it != nil {
			si.it = it
		}
	}
	return nil
}

// ---------------------------------------------------------------- Qtestframe side

// localRequest is the request the handlers of a sync run with.
func localRequest(ctx context.Context) *http.Request {
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/sync", nil)
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "127.0.0.1:0"
	return r
}

// syncSubFailure is the message of a failed handler call.
func syncSubFailure(rec *heldResponse) string {
	var e apiError
	_ = json.Unmarshal(rec.body.Bytes(), &e)
	if rec.status() == http.StatusPreconditionFailed || e.Code == "stale" {
		return "the requirement was changed meanwhile; it is compared again at the next sync"
	}
	return firstNonBlank(e.Error, http.StatusText(rec.status()))
}

// applyLocal writes the Qtestframe side in one transaction (step 4).
func (e *syncEngine) applyLocal(ctx context.Context, created []*syncItem) error {
	col := &impactCollector{choice: e.st.Impact, batch: true, roots: map[string][]store.ImpactRoot{}, reasons: map[string]string{}}
	ctx = context.WithValue(ctx, impactKey{}, col)
	return e.s.Store.InTx(ctx, func(ctx context.Context) error {
		r := localRequest(ctx)
		for _, si := range e.plans {
			if err := e.s.Store.Savepoint(ctx, func() error { return e.applyItem(ctx, r, si) }); err != nil {
				e.errEntry(si, si.it.Key, err.Error())
			}
		}
		if err := e.importNew(ctx, r); err != nil {
			return err
		}
		for _, si := range created {
			ref := &store.ExternalRef{ProjectID: e.pr.ID, System: e.cfg.System, EntityType: model.KindRequirement, EntityID: si.rq.ID, ExternalID: si.it.Key,
				Source: e.base, Version: si.rq.Version, Origin: "qtestframe"}
			e.setBase(ref, si.L, si.it)
			if err := e.s.Store.SaveExternalRef(ctx, e.p.TenantID(), ref); err != nil {
				return err
			}
			e.refs[si.it.Key], e.refByReq[si.rq.ID] = ref, ref
		}
		if e.st.Links {
			if err := e.pullLinks(ctx); err != nil {
				return err
			}
		}
		e.s.flushImpact(ctx, e.p, col)
		return nil
	})
}

// setBase records the values of both sides as the new base of a reference.
func (e *syncEngine) setBase(ref *store.ExternalRef, L map[string]string, it *tracker.SourceItem) {
	_, R, _ := e.values(nil, it)
	now := e.s.now()
	ref.Sync = &store.SyncState{Local: L, Remote: R}
	ref.LocalHash, ref.RemoteHash, ref.RemoteVersion, ref.SyncedAt = syncHash(L), syncHash(R), it.Version, &now
}

// applyItem pulls the changes of one linked item and records its state.
func (e *syncEngine) applyItem(ctx context.Context, r *http.Request, si *syncItem) error {
	tenant := e.p.TenantID()
	rq := si.rq
	if len(si.pull) > 0 {
		x := model.ReqSyncEntry{RequirementID: rq.ID, RequirementKey: rq.Key, Title: rq.Title, ExternalID: si.it.Key, Action: "pull", Fields: fieldList(si.pull), Changes: map[string]string{}}
		for _, f := range x.Fields {
			x.Changes[f] = changeText(si.L[f], si.pull[f])
		}
		if e.opt.dry {
			e.entry(x)
		} else {
			rec := e.s.subCall(r, e.p, e.s.handleUpdateRequirement, http.MethodPatch, map[string]string{"id": rq.ID}, localBody(rq, si.pull), rq.Version)
			if rec.status() >= 400 {
				return errors.New("pull: " + syncSubFailure(rec))
			}
			e.run.Summary.Pulled++
			e.entry(x)
			cur, err := e.s.Store.GetRequirement(ctx, tenant, rq.ID)
			if err != nil {
				return err
			}
			si.rq = cur
		}
	}
	for _, w := range si.warnings {
		e.entry(model.ReqSyncEntry{RequirementID: rq.ID, RequirementKey: rq.Key, Title: rq.Title, ExternalID: si.it.Key, Action: "skip", Message: w})
	}
	if len(si.conflicts) > 0 {
		x := model.ReqSyncEntry{RequirementID: rq.ID, RequirementKey: rq.Key, Title: rq.Title, ExternalID: si.it.Key, Action: "conflict", Changes: map[string]string{}}
		for _, c := range si.conflicts {
			x.Fields = append(x.Fields, c.Field)
			x.Changes[c.Field] = changeText(c.Local, c.Remote)
		}
		e.run.Summary.Conflicts++
		e.entry(x)
	}
	if len(si.pull) == 0 && len(si.push) == 0 && len(si.conflicts) == 0 {
		e.run.Summary.Unchanged++
	}
	if e.opt.dry {
		return nil
	}
	if len(si.conflicts) > 0 {
		c := &model.ReqSyncConflict{ProjectID: e.pr.ID, System: e.cfg.System, RequirementID: rq.ID, ExternalID: si.it.Key, Fields: si.conflicts,
			LocalVersion: si.rq.Version, RemoteVersion: si.it.Version}
		if err := e.s.Store.SaveReqSyncConflict(ctx, tenant, c); err != nil {
			return err
		}
	} else if n, err := e.s.Store.CloseReqSyncConflicts(ctx, tenant, e.pr.ID, e.cfg.System, rq.ID, "resolved", "the sides agree again", ""); err != nil {
		return err
	} else if n > 0 && e.opt.trigger != "resolve" {
		e.s.audit(ctx, e.p, "req_sync.conflict.closed", model.KindRequirement, rq.ID, rq.Key+": both sides agree again")
	}
	// the new base: the current values, except the fields in conflict and
	// pushes that failed, which keep the old base
	L, _, _ := e.values(si.rq, nil)
	_, R, _ := e.values(nil, si.it)
	old := si.ref.Sync
	keepOld := func(field string) bool {
		if old == nil {
			return false
		}
		if slices.ContainsFunc(si.conflicts, func(c model.ReqSyncFieldDiff) bool { return c.Field == field }) {
			return true
		}
		_, pushed := si.push[field]
		return pushed && !si.pushed
	}
	nl, nr := map[string]string{}, map[string]string{}
	for f := range L {
		if keepOld(f) {
			if v, ok := old.Local[f]; ok {
				nl[f] = v
			}
			if v, ok := old.Remote[f]; ok {
				nr[f] = v
			}
			continue
		}
		nl[f], nr[f] = L[f], R[f]
	}
	now := e.s.now()
	si.ref.Sync = &store.SyncState{Local: nl, Remote: nr}
	si.ref.Version, si.ref.RemoteVersion, si.ref.LocalHash, si.ref.RemoteHash, si.ref.SyncedAt = si.rq.Version, si.it.Version, syncHash(nl), syncHash(nr), &now
	return e.s.Store.UpdateSyncRef(ctx, tenant, si.ref)
}

// importNew creates requirements for the items of the query that are not
// linked yet.
func (e *syncEngine) importNew(ctx context.Context, r *http.Request) error {
	if !e.st.ImportNew {
		return nil
	}
	name := sourceNames[e.cfg.System]
	for _, k := range e.order {
		it := e.items[k]
		if !e.inScope[k] || e.refs[k] != nil {
			continue
		}
		_, R, pullable := e.values(nil, it)
		vals := map[string]string{}
		for _, f := range e.mapped() {
			if canPull(f.Direction) && pullable[f.Field] && R[f.Field] != "" {
				vals[f.Field] = R[f.Field]
			}
		}
		if vals["title"] == "" {
			vals["title"] = firstNonBlank(it.Values[tracker.JiraSummary], it.Values[tracker.AzureTitle], name+" "+k)
		}
		body := localBody(nil, vals)
		body["source"], body["create_test"] = name+" "+k, false
		if e.st.FolderID != "" {
			body["folder_id"] = e.st.FolderID
		}
		x := model.ReqSyncEntry{ExternalID: k, Title: vals["title"], Action: "create_local", Fields: fieldList(vals)}
		if e.opt.dry {
			e.entry(x)
			continue
		}
		var out model.Requirement
		err := e.s.Store.Savepoint(ctx, func() error {
			rec := e.s.subCall(r, e.p, e.s.handleCreateRequirement, http.MethodPost, map[string]string{"pid": e.pr.ID}, body, 0)
			if rec.status() >= 400 {
				return errors.New("create: " + syncSubFailure(rec))
			}
			_ = json.Unmarshal(rec.body.Bytes(), &out)
			cur, err := e.s.Store.GetRequirement(ctx, e.p.TenantID(), out.ID)
			if err != nil {
				return err
			}
			L, _, _ := e.values(cur, nil)
			ref := &store.ExternalRef{ProjectID: e.pr.ID, System: e.cfg.System, EntityType: model.KindRequirement, EntityID: cur.ID, ExternalID: k, Source: e.base, Version: cur.Version}
			e.setBase(ref, L, it)
			if err := e.s.Store.SaveExternalRef(ctx, e.p.TenantID(), ref); err != nil {
				return err
			}
			e.refs[k], e.refByReq[cur.ID] = ref, ref
			x.RequirementID, x.RequirementKey = cur.ID, cur.Key
			return nil
		})
		if err != nil {
			e.errEntry(nil, k, err.Error())
			continue
		}
		e.run.Summary.CreatedLocal++
		e.entry(x)
	}
	return nil
}

// ---------------------------------------------------------------- links

// pullLinks creates the relations of the tracker's links between linked items.
func (e *syncEngine) pullLinks(ctx context.Context) error {
	rels, err := e.s.Store.ListRelations(ctx, e.p.TenantID(), e.pr.ID, "")
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, rel := range rels {
		have[rel.Type+"|"+rel.SourceID+"|"+rel.TargetID] = true
	}
	for _, k := range e.order {
		it := e.items[k]
		for _, l := range it.Links {
			src, tgt := e.refs[l.Source], e.refs[l.Target]
			if l.Type == "" || src == nil || tgt == nil || src.EntityID == tgt.EntityID || have[l.Type+"|"+src.EntityID+"|"+tgt.EntityID] {
				continue
			}
			have[l.Type+"|"+src.EntityID+"|"+tgt.EntityID] = true
			x := model.ReqSyncEntry{ExternalID: l.Source + " " + l.Name + " " + l.Target, Action: "link_pull", Message: l.Type}
			if !e.opt.dry {
				rel := &model.Relation{SourceID: src.EntityID, TargetID: tgt.EntityID, Type: l.Type, CreatedBy: e.p.User.Email}
				err := e.s.Store.Savepoint(ctx, func() error { return e.s.Store.CreateRelation(ctx, e.p.TenantID(), rel) })
				if err != nil {
					e.errEntry(nil, x.ExternalID, "link: "+err.Error())
					continue
				}
				e.s.audit(ctx, e.p, "relation.create", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
				x.RequirementID, x.RequirementKey = rel.SourceID, rel.SourceKey
			}
			e.run.Summary.Links++
			e.entry(x)
		}
	}
	return nil
}

// remoteLinkKey identifies a link as a relation (source and target keys).
func remoteLinkKey(typ, src, tgt string) string { return typ + "|" + src + "|" + tgt }

// pushLinks links the tracker items of related requirements.
func (e *syncEngine) pushLinks(ctx context.Context) error {
	rels, err := e.s.Store.ListRelations(ctx, e.p.TenantID(), e.pr.ID, "")
	if err != nil {
		return err
	}
	remote := map[string]bool{}
	for _, it := range e.items {
		for _, l := range it.Links {
			remote[remoteLinkKey(l.Type, l.Source, l.Target)] = true
			if l.Type == "relates_to" {
				remote[remoteLinkKey(l.Type, l.Target, l.Source)] = true
			}
		}
	}
	for _, rel := range rels {
		src, tgt := e.refByReq[rel.SourceID], e.refByReq[rel.TargetID]
		if src == nil || tgt == nil || (e.opt.only != nil && !e.opt.only[src.ExternalID] && !e.opt.only[tgt.ExternalID]) {
			continue
		}
		// only items read in this run: their links are known
		if e.items[src.ExternalID] == nil || e.items[tgt.ExternalID] == nil || remote[remoteLinkKey(rel.Type, src.ExternalID, tgt.ExternalID)] {
			continue
		}
		if e.cfg.System == tracker.SourceAzure && tracker.AzureLinkType(rel.Type) == "" {
			continue
		}
		if name, _ := tracker.JiraLinkType(rel.Type); e.cfg.System == tracker.SourceJira && name == "" {
			continue
		}
		x := model.ReqSyncEntry{RequirementID: rel.SourceID, RequirementKey: rel.SourceKey, ExternalID: src.ExternalID + " " + rel.Type + " " + tgt.ExternalID, Action: "link_push", Message: rel.SourceKey + " " + rel.Type + " " + rel.TargetKey}
		remote[remoteLinkKey(rel.Type, src.ExternalID, tgt.ExternalID)] = true
		if !e.opt.dry {
			if err := e.pacer.wait(ctx); err != nil {
				return err
			}
			if err := e.client.AddLink(ctx, tracker.SourceLink{Type: rel.Type, Source: src.ExternalID, Target: tgt.ExternalID}); err != nil {
				if f := fatalTrackerErr(err); f != nil {
					return f
				}
				e.errEntry(nil, x.ExternalID, "link: "+err.Error())
				continue
			}
		}
		e.run.Summary.Links++
		e.entry(x)
	}
	return nil
}

// ---------------------------------------------------------------- comments

// commentKey identifies a tracker comment among the references.
func commentKey(item, id string) string { return item + "#" + id }

// syncComments pulls new tracker comments into the discussion and pushes
// new comments of the discussion (step 5).
func (e *syncEngine) syncComments(ctx context.Context) error {
	since := time.Time{}
	if e.st.CommentsSince != nil {
		since = *e.st.CommentsSince
	}
	name := sourceNames[e.cfg.System]
	var ids []string
	for _, si := range e.plans {
		if !si.failed {
			ids = append(ids, si.rq.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	local, err := e.s.Store.ProjectDiscussions(ctx, e.p.TenantID(), model.KindRequirement, ids)
	if err != nil {
		return err
	}
	for _, si := range e.plans {
		if si.failed {
			continue
		}
		key := si.it.Key
		// tracker comments: when the item changed (Jira's updated time
		// follows its comments) or a webhook named it
		if e.opt.only != nil || si.touched {
			remote, err := e.client.Comments(ctx, key)
			if err != nil {
				if f := fatalTrackerErr(err); f != nil {
					return f
				}
				e.errEntry(si, key, "comments: "+err.Error())
				continue
			}
			for _, c := range remote {
				ck := commentKey(key, c.ID)
				if e.cmtRemote[ck] || c.Created.Before(since) || strings.TrimSpace(c.Body) == "" {
					continue
				}
				x := model.ReqSyncEntry{RequirementID: si.rq.ID, RequirementKey: si.rq.Key, Title: si.rq.Title, ExternalID: ck, Action: "comment_pull", Message: c.Author}
				if !e.opt.dry {
					body := fmt.Sprintf("**%s** (%s %s):\n\n%s", firstNonBlank(c.Author, name), name, key, c.Body)
					if r := []rune(body); len(r) > store.MaxCommentLength {
						body = string(r[:store.MaxCommentLength-1]) + "…"
					}
					cm := &model.Comment{ProjectID: e.pr.ID, EntityType: model.KindRequirement, EntityID: si.rq.ID, UserID: e.p.User.ID, Body: body}
					if err := e.s.Store.AddComment(ctx, e.p.TenantID(), cm); err != nil {
						e.errEntry(si, ck, "comment: "+err.Error())
						continue
					}
					if err := e.s.Store.SaveExternalRef(ctx, e.p.TenantID(), &store.ExternalRef{ProjectID: e.pr.ID, System: e.cfg.System, EntityType: "comment", EntityID: cm.ID, ExternalID: ck, Source: e.base}); err != nil {
						return err
					}
					e.cmtRemote[ck], e.cmtLocal[cm.ID] = true, true
				}
				e.run.Summary.Comments++
				e.entry(x)
			}
		}
		for _, c := range local[si.rq.ID] {
			if e.cmtLocal[c.ID] || c.Deleted || c.CreatedAt.Before(since) || c.CreatedAt.Before(si.ref.ImportedAt) {
				continue
			}
			x := model.ReqSyncEntry{RequirementID: si.rq.ID, RequirementKey: si.rq.Key, Title: si.rq.Title, ExternalID: key, Action: "comment_push", Message: firstNonBlank(c.UserName, c.UserEmail)}
			if !e.opt.dry {
				if err := e.pacer.wait(ctx); err != nil {
					return err
				}
				body := fmt.Sprintf("**%s** (Qtestframe %s):\n\n%s", firstNonBlank(c.UserName, c.UserEmail), si.rq.Key, c.Body)
				id, err := e.client.AddComment(ctx, key, body)
				if err != nil {
					if f := fatalTrackerErr(err); f != nil {
						return f
					}
					e.errEntry(si, key, "comment: "+err.Error())
					continue
				}
				ck := commentKey(key, id)
				if err := e.s.Store.SaveExternalRef(ctx, e.p.TenantID(), &store.ExternalRef{ProjectID: e.pr.ID, System: e.cfg.System, EntityType: "comment", EntityID: c.ID, ExternalID: ck, Source: e.base, Origin: "qtestframe"}); err != nil {
					return err
				}
				e.cmtRemote[ck], e.cmtLocal[c.ID] = true, true
			}
			e.run.Summary.Comments++
			e.entry(x)
		}
	}
	return nil
}

// ---------------------------------------------------------------- run

// execute runs the steps and fills e.run (its status and error).
func (e *syncEngine) execute(ctx context.Context) error {
	if !e.p.Can(rbac.RequirementWr) {
		return forbidden("permission requirement.write required to sync requirements")
	}
	if err := e.load(ctx); err != nil {
		return err
	}
	if err := e.plan(ctx); err != nil {
		return err
	}
	if err := e.pushes(ctx); err != nil {
		return err
	}
	created, err := e.createRemote(ctx)
	if err != nil {
		return err
	}
	if !e.opt.dry {
		if err := e.reread(ctx, append(append([]*syncItem{}, e.plans...), created...)); err != nil {
			return err
		}
	}
	if err := e.applyLocal(ctx, created); err != nil {
		return err
	}
	if e.st.Links {
		if err := e.pushLinks(ctx); err != nil {
			return err
		}
	}
	if e.st.Comments {
		if err := e.syncComments(ctx); err != nil {
			return err
		}
	}
	return nil
}

// runReqSync runs a sync and records it (the log, the configuration's last
// run and next run, the audit log). A dry run records nothing.
func (s *Server) runReqSync(ctx context.Context, p *Principal, pr *model.Project, cfg *store.ReqSyncRow, opt syncOptions) (*model.ReqSyncRun, error) {
	now := s.now()
	if !opt.dry {
		ok, err := s.Store.ClaimReqSync(ctx, cfg.ID, now, now.Add(15*time.Minute))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: a sync of %s is running", store.ErrConflict, sourceNames[cfg.System])
		}
	}
	e, err := s.newSyncEngine(ctx, p, pr, cfg, opt)
	var runErr error
	if err == nil {
		runErr = e.execute(ctx)
	} else {
		runErr = err
		e = &syncEngine{run: &model.ReqSyncRun{ProjectID: pr.ID, System: cfg.System, Trigger: opt.trigger, DryRun: opt.dry, StartedBy: p.User.Email, StartedAt: now, Entries: []model.ReqSyncEntry{}}}
	}
	run := e.run
	fin := s.now()
	run.FinishedAt = &fin
	switch {
	case runErr != nil:
		run.Status, run.Error = "error", runErr.Error()
		if errors.Is(runErr, errWriteBudget) {
			run.Status = "partial"
		}
	case run.Summary.Errors > 0:
		run.Status = "partial"
	default:
		run.Status = "ok"
	}
	if opt.dry {
		return run, runErr
	}
	failures := 0
	if run.Status == "error" {
		failures = cfg.Failures + 1
	}
	var next *time.Time
	if cfg.Enabled && cfg.IntervalMinutes > 0 {
		wait := time.Duration(cfg.IntervalMinutes) * time.Minute
		if failures > 0 {
			// back off: twice the interval per failure, at most a day
			wait = min(wait<<min(failures, 8), 24*time.Hour)
		}
		var rl *errRateLimited
		if errors.As(runErr, &rl) && rl.err.RetryAfter > wait {
			wait = rl.err.RetryAfter
		}
		t := fin.Add(wait)
		next = &t
	} else {
		next = cfg.NextRunAt
	}
	bg := context.WithoutCancel(ctx)
	if err := s.Store.SaveReqSyncRun(bg, p.TenantID(), run); err != nil {
		s.Log.Warn("sync log", "err", err)
	}
	if err := s.Store.FinishReqSync(bg, cfg.ID, fin, run.Status, run.Error, failures, next); err != nil {
		s.Log.Warn("sync state", "err", err)
	}
	sm := run.Summary
	details := fmt.Sprintf("%s (%s): %d pulled, %d pushed, %d created here, %d created in %s, %d conflicts, %d comments, %d links, %d errors",
		sourceNames[cfg.System], opt.trigger, sm.Pulled, sm.Pushed, sm.CreatedLocal, sm.CreatedRemote, sourceNames[cfg.System], sm.Conflicts, sm.Comments, sm.Links, sm.Errors)
	if run.Error != "" {
		details += ": " + shorten(run.Error, 300)
	}
	s.audit(bg, p, "req_sync.run", "project", pr.ID, details)
	return run, runErr
}
