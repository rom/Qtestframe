package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/similarity"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracerules"
)

// Similar requirements and tests (internal/similarity): the suggestions
// while editing, the review of suspected duplicates and potential
// conflicts, merging a duplicate into the item that survives, and the
// optional embeddings service.

func (s *Server) routesSimilarity(h routeFunc) {
	h("POST /api/v1/projects/{pid}/similar", s.handleSimilar, "project.read") // read of the kind: see handleSimilar
	h("GET /api/v1/projects/{pid}/consistency/review", s.handleSimilarityReview, "project.read")
	h("POST /api/v1/projects/{pid}/consistency/merge", s.atomic(s.handleMerge), "project.read") // write of the kind: see handleMerge
	h("POST /api/v1/projects/{pid}/catalog/similar", s.handleCatalogSimilar, "requirement.read")
	h("POST /api/v1/admin/settings/test/embeddings", s.handleTestEmbeddings)
}

// Engines of the similarity.
const (
	EngineTFIDF      = "tfidf"
	EngineEmbeddings = "embeddings"
)

// reqDoc is the text of a requirement for the similarity.
func reqDoc(rq *model.Requirement) similarity.Doc {
	d := similarity.Doc{ID: rq.ID, Title: rq.Title, Description: rq.Description}
	for _, c := range rq.Criteria {
		if t := strings.TrimSpace(c.Description + " " + c.Threshold); t != "" {
			d.Details = append(d.Details, t)
		}
	}
	return d
}

// testDoc is the text of a test: title, description, and its
// preconditions, steps and expected results as details.
func testDoc(t *model.TestCase) similarity.Doc {
	d := similarity.Doc{ID: t.ID, Title: t.Title, Description: t.Description}
	for _, x := range []string{t.Preconditions, t.ExpectedResult} {
		if x = strings.TrimSpace(x); x != "" {
			d.Details = append(d.Details, x)
		}
	}
	for _, st := range t.Steps {
		if x := strings.TrimSpace(st.Action + " " + st.Expected); x != "" {
			d.Details = append(d.Details, x)
		}
	}
	return d
}

// ---------------------------------------------------------------- embeddings

// embedder returns the configured embeddings service (nil: off).
func (s *Server) embedder() similarity.Embedder {
	e := s.conf().Similarity.Embeddings
	if !e.On() {
		return nil
	}
	return s.embeddingsClient(e)
}

func (s *Server) embeddingsClient(e config.Embeddings) *similarity.Client {
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	hc := &http.Client{Timeout: timeout}
	if s.HTTPClient != nil {
		c := *s.HTTPClient
		c.Timeout = timeout
		hc = &c
	}
	return &similarity.Client{URL: e.URL, Key: e.APIKey, ModelName: e.Model, HTTP: hc}
}

// embeddedIndex returns the embeddings of docs (cached per item version
// and text) as an index. versions: item id → version.
func (s *Server) embeddedIndex(ctx context.Context, emb similarity.Embedder, tenantID, projectID, kind string, docs []similarity.Doc, versions map[string]int) (*similarity.DenseIndex, error) {
	cached, err := s.Store.CachedEmbeddings(ctx, tenantID, projectID, kind, emb.Model())
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(docs))
	vecs := make([][]float32, len(docs))
	var missing []int
	var texts []string
	for i, d := range docs {
		ids[i] = d.ID
		text := d.Text()
		if c, ok := cached[d.ID]; ok && c.TextHash == similarity.TextHash(text) {
			if v, err := similarity.DecodeVector(c.Vector); err == nil {
				vecs[i] = v
				continue
			}
		}
		missing = append(missing, i)
		texts = append(texts, text)
	}
	if len(missing) > 0 {
		got, err := emb.Embed(ctx, texts)
		if err != nil {
			return nil, err
		}
		save := make([]store.CachedEmbedding, len(missing))
		for k, i := range missing {
			vecs[i] = got[k]
			save[k] = store.CachedEmbedding{ItemID: ids[i], Version: versions[ids[i]], TextHash: similarity.TextHash(texts[k]), Vector: similarity.EncodeVector(got[k])}
		}
		if err := s.Store.SaveEmbeddings(ctx, tenantID, projectID, kind, emb.Model(), save); err != nil {
			s.Log.Warn("similarity: storing embeddings", "err", err)
		}
	}
	return similarity.NewDenseIndex(ids, vecs), nil
}

// similarityFacts fills the texts of the items into g and, with an
// embeddings service, the scorer that uses it.
func (s *Server) similarityFacts(ctx context.Context, tenantID, projectID string, g *tracerules.Graph, reqs []model.Requirement, tests []model.TestCase) {
	f := &g.Facts
	f.Docs = make(map[string]similarity.Doc, len(reqs)+len(tests))
	f.TestCreated = make(map[string]time.Time, len(tests))
	versions := map[string]int{}
	for i := range reqs {
		f.Docs[reqs[i].ID] = reqDoc(&reqs[i])
		versions[reqs[i].ID] = reqs[i].Version
	}
	for i := range tests {
		f.Docs[tests[i].ID] = testDoc(&tests[i])
		f.TestCreated[tests[i].ID] = tests[i].CreatedAt
		versions[tests[i].ID] = tests[i].Version
	}
	emb := s.embedder()
	if emb == nil || store.InTransaction(ctx) {
		// (no outside call while a transaction holds the database)
		return
	}
	g.Hooks.Scorer = func(_ *tracerules.Graph, kind string, m *tracerules.SimModel) similarity.Scorer {
		docs := make([]similarity.Doc, len(m.Docs))
		for i, d := range m.Docs {
			docs[i] = d.Doc
		}
		x, err := s.embeddedIndex(ctx, emb, tenantID, projectID, kind, docs, versions)
		if err != nil {
			s.Log.Warn("similarity: embeddings service failed, using the local similarity", "err", err)
			return nil
		}
		return x
	}
}

func engineOf(m *tracerules.SimModel) string {
	if _, ok := m.Scorer.(*similarity.DenseIndex); ok {
		return EngineEmbeddings
	}
	return EngineTFIDF
}

// ---------------------------------------------------------------- corpus cache

// simCorpus is the model of the active items of a kind, for the
// suggestions while editing (cached until the items change).
type simCorpus struct {
	stamp  string
	items  map[string]simItem
	index  *similarity.Index
	dense  *similarity.DenseIndex
	engine string
}

// simItem is an item in an answer.
type simItem struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	Key     string    `json:"key"`
	Title   string    `json:"title"`
	Status  string    `json:"status"`
	Links   int       `json:"links"`
	Created time.Time `json:"created_at"`
}

var simCorpora sync.Map // "<server>|tenant|project|kind|engine" → *simCorpus

func (s *Server) corpus(ctx context.Context, tenantID, projectID, kind string) (*simCorpus, error) {
	emb := s.embedder()
	engine := EngineTFIDF
	if emb != nil {
		engine = EngineEmbeddings + ":" + emb.Model()
	}
	stamp, err := s.Store.ItemsStamp(ctx, tenantID, projectID, kind)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%p|%s|%s|%s|%s", s, tenantID, projectID, kind, engine)
	if v, ok := simCorpora.Load(key); ok && v.(*simCorpus).stamp == stamp {
		return v.(*simCorpus), nil
	}
	c := &simCorpus{stamp: stamp, items: map[string]simItem{}, engine: EngineTFIDF}
	var docs []similarity.Doc
	versions := map[string]int{}
	if kind == model.KindTest {
		tests, err := s.Store.ListTests(ctx, tenantID, projectID)
		if err != nil {
			return nil, err
		}
		for i := range tests {
			t := &tests[i]
			if t.Status == "deprecated" {
				continue
			}
			docs = append(docs, testDoc(t))
			versions[t.ID] = t.Version
			c.items[t.ID] = simItem{ID: t.ID, Kind: kind, Key: t.Key, Title: t.Title, Status: t.Status, Links: t.LinkedReqs, Created: t.CreatedAt}
		}
	} else {
		reqs, err := s.Store.ListRequirements(ctx, tenantID, projectID)
		if err != nil {
			return nil, err
		}
		for i := range reqs {
			rq := &reqs[i]
			if rq.Status == "rejected" || rq.Status == "obsolete" {
				continue
			}
			docs = append(docs, reqDoc(rq))
			versions[rq.ID] = rq.Version
			c.items[rq.ID] = simItem{ID: rq.ID, Kind: kind, Key: rq.Key, Title: rq.Title, Status: rq.Status, Links: rq.LinkedTests, Created: rq.CreatedAt}
		}
	}
	c.index = similarity.NewIndex(docs)
	if emb != nil && !store.InTransaction(ctx) {
		if x, err := s.embeddedIndex(ctx, emb, tenantID, projectID, kind, docs, versions); err != nil {
			s.Log.Warn("similarity: embeddings service failed, using the local similarity", "err", err)
		} else {
			c.dense, c.engine = x, EngineEmbeddings
		}
	}
	if c.dense != nil || emb == nil {
		// (a corpus without the vectors it should have is not kept)
		simCorpora.Store(key, c)
	}
	return c, nil
}

// similar returns the items of the corpus most similar to d.
func (s *Server) similar(ctx context.Context, c *simCorpus, d similarity.Doc, limit int, min float64, skip func(string) bool) ([]similarity.Match, string) {
	if c.dense != nil {
		if emb := s.embedder(); emb != nil {
			if v, err := emb.Embed(ctx, []string{d.Text()}); err == nil && len(v) == 1 {
				return c.dense.Nearest(v[0], limit, min, skip), EngineEmbeddings
			} else if err != nil {
				s.Log.Warn("similarity: embeddings service failed, using the local similarity", "err", err)
			}
		}
	}
	return c.index.Similar(d, limit, min, skip), EngineTFIDF
}

// ---------------------------------------------------------------- similar

type similarRequest struct {
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Details     []string `json:"details"`
	// ExcludeID is the item being edited.
	ExcludeID string  `json:"exclude_id"`
	Limit     int     `json:"limit"`
	Min       float64 `json:"min"`
}

type similarHit struct {
	simItem
	Score float64 `json:"score"`
}

type similarResponse struct {
	Engine string       `json:"engine"`
	Items  []similarHit `json:"items"`
	// Threshold of a suspected duplicate (scores at or above it).
	Threshold float64 `json:"threshold"`
}

func kindReadPerm(kind string) rbac.Permission {
	if kind == model.KindTest {
		return rbac.TestRead
	}
	return rbac.RequirementRd
}

func kindWritePerm(kind string) rbac.Permission {
	if kind == model.KindTest {
		return rbac.TestWrite
	}
	return rbac.RequirementWr
}

func validKind(kind string) (string, error) {
	switch kind {
	case "", model.KindRequirement:
		return model.KindRequirement, nil
	case model.KindTest:
		return kind, nil
	}
	return "", badRequest("kind must be requirement or test")
}

// handleSimilar: POST /projects/{pid}/similar {kind, title, description,
// details, exclude_id, limit, min} — the active items of the project most
// similar to a text being written (top 5 by default).
func (s *Server) handleSimilar(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req similarRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	kind, err := validKind(req.Kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if err := s.canIn(ctx, p, pr.ID, kindReadPerm(kind)); err != nil {
		s.fail(w, r, err)
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	limit = min(limit, 20)
	floor := req.Min
	if floor <= 0 {
		floor = similarity.DefaultSuggestThreshold
	}
	threshold, _ := s.thresholds(ctx, p.TenantID(), pr.ID, kind)
	out := similarResponse{Engine: EngineTFIDF, Items: []similarHit{}, Threshold: threshold}
	d := similarity.Doc{ID: "\x00query", Title: req.Title, Description: req.Description, Details: req.Details}
	if strings.TrimSpace(d.Text()) == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	c, err := s.corpus(ctx, p.TenantID(), pr.ID, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	matches, engine := s.similar(ctx, c, d, limit, floor, func(id string) bool { return id == req.ExcludeID })
	out.Engine = engine
	for _, m := range matches {
		if it, ok := c.items[m.ID]; ok {
			out.Items = append(out.Items, similarHit{simItem: it, Score: round2(m.Score)})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func round2(x float64) float64 { return float64(int(x*100+0.5)) / 100 }

// thresholds returns the duplicate threshold of kind and the conflict
// confidence of the project: those of its installed rules, else the
// defaults.
func (s *Server) thresholds(ctx context.Context, tenantID, projectID, kind string) (dup, conflict float64) {
	dup, conflict = similarity.DefaultDuplicateThreshold, similarity.DefaultConflictConfidence
	rules, err := s.Store.ListTraceRules(ctx, tenantID, projectID)
	if err != nil {
		return dup, conflict
	}
	dupKind := model.CheckDuplicateSuspect
	if kind == model.KindTest {
		dupKind = model.CheckTestDuplicateSuspect
	}
	for _, r := range rules {
		switch {
		case r.Kind == dupKind && r.Threshold > 0:
			dup = r.Threshold
		case r.Kind == model.CheckPotentialConflict && r.Threshold > 0:
			conflict = r.Threshold
		}
	}
	return dup, conflict
}

// ---------------------------------------------------------------- review

type duplicatePair struct {
	A         simItem `json:"a"`
	B         simItem `json:"b"`
	Score     float64 `json:"score"`
	Dismissed bool    `json:"dismissed"`
}

type conflictPair struct {
	A          simItem             `json:"a"`
	B          simItem             `json:"b"`
	Kind       string              `json:"kind"`
	Reason     string              `json:"reason"`
	Confidence float64             `json:"confidence"`
	Similarity float64             `json:"similarity"`
	Note       string              `json:"note"`
	EvidenceA  similarity.Evidence `json:"evidence_a"`
	EvidenceB  similarity.Evidence `json:"evidence_b"`
	Dismissed  bool                `json:"dismissed"`
}

type reviewResponse struct {
	Kind               string          `json:"kind"`
	Engine             string          `json:"engine"`
	Threshold          float64         `json:"threshold"`
	ConflictConfidence float64         `json:"conflict_confidence"`
	Duplicates         []duplicatePair `json:"duplicates"`
	Conflicts          []conflictPair  `json:"conflicts"`
	// Dismissed is the number of dismissed pairs (shown with dismissed=true).
	Dismissed int `json:"dismissed"`
	// Truncated: more pairs than listed (limit).
	Truncated bool `json:"truncated,omitempty"`
}

const maxReviewPairs = 200

func floatParam(r *http.Request, name string, def, lo, hi float64) (float64, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < lo || f > hi {
		return 0, badRequest("%s must be a number between %g and %g", name, lo, hi)
	}
	return f, nil
}

// handleSimilarityReview: GET /projects/{pid}/consistency/review?kind=
// requirement|test&threshold=&confidence=&dismissed= — the suspected
// duplicates (pairs at least threshold similar that are not marked as
// duplicates) and, for requirements, the potential conflicts (at least
// confidence, not related by conflicts_with) with their evidence.
// Dismissed pairs are left out unless dismissed=true.
func (s *Server) handleSimilarityReview(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	kind, err := validKind(r.URL.Query().Get("kind"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if err := s.canIn(ctx, p, pr.ID, kindReadPerm(kind)); err != nil {
		s.fail(w, r, err)
		return
	}
	dupDef, confDef := s.thresholds(ctx, p.TenantID(), pr.ID, kind)
	threshold, err := floatParam(r, "threshold", dupDef, 0.3, 1)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	confidence, err := floatParam(r, "confidence", confDef, tracerules.MinConflictConfidence, 1)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	withDismissed := boolParam(r, "dismissed", false)
	g, err := s.traceGraph(ctx, p.TenantID(), pr, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m := g.SimilarityModel(kind)
	links := map[string]int{}
	for _, l := range g.Links {
		links[l.RequirementID]++
		links[l.TestCaseID]++
	}
	items := map[string]simItem{}
	for _, it := range m.Items {
		st, created := g.Reqs[it.ID].Status, g.Reqs[it.ID].Created
		if kind == model.KindTest {
			st, created = g.TestStatus[it.ID], g.TestCreated[it.ID]
		}
		items[it.ID] = simItem{ID: it.ID, Kind: kind, Key: it.Key, Title: it.Title, Status: st, Links: links[it.ID], Created: created}
	}
	out := reviewResponse{Kind: kind, Engine: engineOf(m), Threshold: threshold, ConflictConfidence: confidence, Duplicates: []duplicatePair{}, Conflicts: []conflictPair{}}
	dupCheck := model.CheckDuplicateSuspect
	if kind == model.KindTest {
		dupCheck = model.CheckTestDuplicateSuspect
	}
	conflicting := map[string]bool{}
	if kind == model.KindRequirement {
		marked := g.MarkedConflicts()
		for _, c := range m.Conflicts() {
			key := tracerules.DismissKey("", c.A, c.B)
			if c.Confidence >= similarity.DefaultConflictConfidence {
				conflicting[key] = true
			}
			if c.Confidence < confidence || marked[key] {
				continue
			}
			dismissed := g.Dismissed[tracerules.DismissKey(model.CheckPotentialConflict, c.A, c.B)]
			if dismissed {
				out.Dismissed++
				if !withDismissed {
					continue
				}
			}
			if len(out.Conflicts) >= maxReviewPairs {
				out.Truncated = true
				continue
			}
			out.Conflicts = append(out.Conflicts, conflictPair{A: items[c.A], B: items[c.B], Kind: c.Kind, Reason: tracerules.ConflictReason(c.Kind),
				Confidence: c.Confidence, Similarity: round2(c.Similarity), Note: c.Note, EvidenceA: c.EvidenceA, EvidenceB: c.EvidenceB, Dismissed: dismissed})
		}
	}
	marked := g.MarkedDuplicates(kind)
	for _, pr := range m.Scorer.Pairs(threshold) {
		key := tracerules.DismissKey("", pr.A, pr.B)
		if marked[key] || conflicting[key] {
			continue
		}
		a, okA := items[pr.A]
		b, okB := items[pr.B]
		if !okA || !okB {
			continue
		}
		// the older item first: the newer one duplicates it
		if b.Created.Before(a.Created) || (b.Created.Equal(a.Created) && b.Key < a.Key) {
			a, b = b, a
		}
		dismissed := g.Dismissed[tracerules.DismissKey(dupCheck, pr.A, pr.B)]
		if dismissed {
			out.Dismissed++
			if !withDismissed {
				continue
			}
		}
		if len(out.Duplicates) >= maxReviewPairs {
			out.Truncated = true
			continue
		}
		out.Duplicates = append(out.Duplicates, duplicatePair{A: a, B: b, Score: round2(pr.Score), Dismissed: dismissed})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- merge

type mergeRequest struct {
	Kind string `json:"kind"`
	// SurvivorID is the item that stays; DuplicateID the one merged into
	// it (obsoleted, or deprecated for a test).
	SurvivorID  string `json:"survivor_id"`
	DuplicateID string `json:"duplicate_id"`
}

type mergeResponse struct {
	Survivor  simItem           `json:"survivor"`
	Duplicate simItem           `json:"duplicate"`
	Moved     store.MergeCounts `json:"moved"`
}

// mergeTexts are the comments a merge leaves on both items.
var mergeTexts = map[string][2]string{
	"en": {"Merged %s “%s” into this item (%s). The duplicate is now %s.", "Merged into %s “%s” (%s); this item is a duplicate of it."},
	"sv": {"%s ”%s” har slagits ihop med det här objektet (%s). Dubbletten har nu status %s.", "Sammanslagen med %s ”%s” (%s); det här objektet är en dubblett av det."},
}

func mergeSummary(lang string, c *store.MergeCounts) string {
	if lang == "sv" {
		return fmt.Sprintf("%d spårlänkar, %d relationer, %d förutsättningar och %d objektlänkar flyttade", c.Links, c.Relations, c.Dependencies, c.ItemLinks)
	}
	return fmt.Sprintf("%d trace links, %d relations, %d prerequisites and %d item links moved", c.Links, c.Relations, c.Dependencies, c.ItemLinks)
}

// handleMerge: POST /projects/{pid}/consistency/merge {kind, survivor_id,
// duplicate_id} — merges a duplicate into the item that survives: its
// trace links, relations or prerequisites and item links move to the
// survivor, the duplicate becomes obsolete (a test: deprecated) and is
// marked as its duplicate (relation, or item link for tests), and both
// get a comment for the history. Needs write permission on the kind; the
// status change goes through the ordinary update (history, audit,
// review rules). Atomic.
func (s *Server) handleMerge(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req mergeRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	kind, err := validKind(req.Kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if err := s.canIn(ctx, p, pr.ID, kindWritePerm(kind)); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.SurvivorID == "" || req.DuplicateID == "" || req.SurvivorID == req.DuplicateID {
		s.fail(w, r, badRequest("survivor_id and duplicate_id must name two different items"))
		return
	}
	var surv, dup simItem
	for _, x := range []struct {
		id  string
		out *simItem
	}{{req.SurvivorID, &surv}, {req.DuplicateID, &dup}} {
		project, _, key, err := s.itemVersion(ctx, p, kind, x.id)
		if err == nil && project != pr.ID {
			err = notFound(kind, x.id)
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		*x.out = simItem{ID: x.id, Kind: kind, Key: key}
	}
	moved, err := s.Store.MergeItems(ctx, p.TenantID(), pr.ID, kind, dup.ID, surv.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// mark the duplicate
	newStatus := "obsolete"
	if kind == model.KindTest {
		newStatus = "deprecated"
		target := surv.ID
		l := &model.ItemLink{ProjectID: pr.ID, SourceType: model.KindTest, SourceID: dup.ID, TargetType: model.KindTest, TargetID: &target, Label: model.DuplicateLinkLabel, CreatedBy: p.User.Email}
		if err := s.Store.CreateItemLink(ctx, p.TenantID(), l); err != nil {
			s.fail(w, r, err)
			return
		}
	} else {
		rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, dup.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !slices.ContainsFunc(rels, func(x model.Relation) bool {
			return x.Type == "duplicates" && x.SourceID == dup.ID && x.TargetID == surv.ID
		}) {
			rel := &model.Relation{SourceID: dup.ID, TargetID: surv.ID, Type: "duplicates", CreatedBy: p.User.Email}
			if err := s.Store.CreateRelation(ctx, p.TenantID(), rel); err != nil {
				s.fail(w, r, err)
				return
			}
		}
	}
	// the status through the ordinary update (history, audit, review)
	body := []byte(`{"status":"` + newStatus + `"}`)
	sub := r.Clone(context.WithValue(ctx, batchKey{}, true))
	sub.Method = http.MethodPatch
	sub.Body = io.NopCloser(bytes.NewReader(body))
	sub.ContentLength = int64(len(body))
	sub.SetPathValue("id", dup.ID)
	sub.Header.Del("If-Match")
	rec := &heldResponse{header: http.Header{}}
	if kind == model.KindTest {
		s.handleUpdateTest(rec, sub, p)
	} else {
		s.handleUpdateRequirement(rec, sub, p)
	}
	if rec.status() >= 400 {
		for k, v := range rec.header {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.status())
		_, _ = w.Write(rec.body.Bytes())
		return
	}
	// comments for the history of both items
	lang := p.User.Preferences.Language
	if lang != "sv" {
		lang = "en"
	}
	texts := mergeTexts[lang]
	sTitle, dTitle := s.itemTitle(ctx, p, kind, surv.ID), s.itemTitle(ctx, p, kind, dup.ID)
	surv.Title, dup.Title, dup.Status = sTitle, dTitle, newStatus
	summary := mergeSummary(lang, moved)
	for _, c := range []*model.Comment{
		{EntityType: kind, EntityID: surv.ID, UserID: p.User.ID, Body: fmt.Sprintf(texts[0], dup.Key, dTitle, summary, newStatus)},
		{EntityType: kind, EntityID: dup.ID, UserID: p.User.ID, Body: fmt.Sprintf(texts[1], surv.Key, sTitle, summary)},
	} {
		if err := s.Store.AddComment(ctx, p.TenantID(), c); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.audit(ctx, p, "consistency.merge", kind, surv.ID, dup.Key+" → "+surv.Key+": "+mergeSummary("en", moved))
	writeJSON(w, http.StatusOK, mergeResponse{Survivor: surv, Duplicate: dup, Moved: *moved})
}

func (s *Server) itemTitle(ctx context.Context, p *Principal, kind, id string) string {
	if kind == model.KindTest {
		if t, err := s.Store.GetTest(ctx, p.TenantID(), id); err == nil {
			return t.Title
		}
		return ""
	}
	if rq, err := s.Store.GetRequirement(ctx, p.TenantID(), id); err == nil {
		return rq.Title
	}
	return ""
}

// ---------------------------------------------------------------- catalogue

type catalogSimilarItem struct {
	CatalogRef string       `json:"catalog_ref"`
	Title      string       `json:"title"`
	Similar    []similarHit `json:"similar"`
}

// handleCatalogSimilar: POST /projects/{pid}/catalog/similar
// {requirement_ids, lang} — the catalogue requirements that are similar
// to requirements the project already has (suspected duplicates), with
// those requirements; for the warning before an import.
func (s *Server) handleCatalogSimilar(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	cat, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		RequirementIDs []string `json:"requirement_ids"`
		Lang           string   `json:"lang"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.RequirementIDs) > maxCatalogImport {
		s.fail(w, r, badRequest("at most %d requirements", maxCatalogImport))
		return
	}
	ctx := r.Context()
	hits, err := s.catalogSimilar(ctx, p.TenantID(), pr.ID, cat.Requirements, uniqStrings(req.RequirementIDs), req.Lang)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(hits))
}

// catalogSimilar compares catalogue requirements with the project's.
func (s *Server) catalogSimilar(ctx context.Context, tenantID, projectID string, reqs map[string]*catalog.Requirement, ids []string, lang string) ([]catalogSimilarItem, error) {
	out := []catalogSimilarItem{}
	if len(ids) == 0 {
		return out, nil
	}
	c, err := s.corpus(ctx, tenantID, projectID, model.KindRequirement)
	if err != nil {
		return nil, err
	}
	threshold, _ := s.thresholds(ctx, tenantID, projectID, model.KindRequirement)
	for _, id := range ids {
		rq := reqs[id]
		if rq == nil {
			continue
		}
		m := catalogRequirement(rq, lang, "draft", "")
		d := reqDoc(m)
		d.ID = "\x00catalog"
		matches, _ := s.similar(ctx, c, d, 3, threshold, nil)
		if len(matches) == 0 {
			continue
		}
		it := catalogSimilarItem{CatalogRef: id, Title: m.Title}
		for _, mt := range matches {
			if x, ok := c.items[mt.ID]; ok {
				it.Similar = append(it.Similar, similarHit{simItem: x, Score: round2(mt.Score)})
			}
		}
		if len(it.Similar) > 0 {
			out = append(out, it)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- settings test

// handleTestEmbeddings: POST /admin/settings/test/embeddings {values} —
// sends a short text to the embeddings service of the form (unsaved
// values; the stored key when none is typed) and reports the vector size.
func (s *Server) handleTestEmbeddings(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := decodeOptional(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	cfg, err := s.candidate(r.Context(), req.Values, "similarity.")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	e := cfg.Similarity.Embeddings
	if e.URL == "" {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: "no endpoint URL"})
		return
	}
	c := s.embeddingsClient(e)
	v, err := c.Embed(r.Context(), []string{"The user MUST be able to sign in."})
	if err != nil {
		s.audit(r.Context(), p, "settings.test_embeddings", "system_settings", "similarity.embeddings", e.URL+": "+err.Error())
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	s.audit(r.Context(), p, "settings.test_embeddings", "system_settings", "similarity.embeddings", e.URL)
	writeJSON(w, http.StatusOK, testResult{OK: true, Message: fmt.Sprintf("%d dimensions", len(v[0]))})
}
