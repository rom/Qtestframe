package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Batch updates of requirements and tests (the spreadsheet view): each item
// gets its own field values, and the whole batch is saved in one
// transaction (see atomic). Every item goes through the same PATCH handler
// as a single save, so validation, permissions (write or review), history,
// audit, suspect links and notifications are the same; a failing item rolls
// the whole batch back.
//
// Optimistic concurrency: an item sent with the version it was read at is
// checked before anything is written; when any is outdated nothing is saved
// and the answer is 409 "stale" with every outdated item as it is saved now.

// maxBatchItems caps one batch (a paste or fill-down over a large sheet).
const maxBatchItems = 5000

// batchKey marks a request context as a step of a batch: the PATCH handlers
// then answer with the stored item instead of loading the project context
// again to decorate it.
type batchKey struct{}

func inBatch(ctx context.Context) bool { return ctx.Value(batchKey{}) != nil }

// batchItem is one item of a batch update.
type batchItem struct {
	ID string `json:"id"`
	// Version, when set (> 0), is the version the change is based on.
	Version int `json:"version"`
	// Set holds the fields to change, as in a PATCH.
	Set json.RawMessage `json:"set"`
}

// batchConflict describes an item that was changed by someone else.
type batchConflict struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Version int    `json:"version"`
	Current any    `json:"current"`
}

// itemBatchFailure is the answer for an item that could not be saved.
type itemBatchFailure struct {
	apiError
	Index int    `json:"index"`
	ID    string `json:"id"`
}

func (s *Server) handleBatchUpdate(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var req struct {
			Items []batchItem `json:"items"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := checkBatch(req.Items); err != nil {
			s.fail(w, r, err)
			return
		}
		ctx := r.Context()
		// all versions first: an outdated item changes nothing
		var conflicts []batchConflict
		for _, it := range req.Items {
			project, version, key, err := s.itemVersion(ctx, p, kind, it.ID)
			if err == nil && project != pr.ID {
				err = notFound(kind, it.ID)
			}
			if err != nil {
				s.fail(w, r, err)
				return
			}
			if it.Version > 0 && it.Version != version {
				c := batchConflict{ID: it.ID, Key: key, Version: version}
				if kind == model.KindRequirement {
					c.Current, err = s.decoratedRequirement(r, p, it.ID)
				} else {
					c.Current, err = s.decoratedTest(r, p, it.ID)
				}
				if err != nil {
					s.fail(w, r, err)
					return
				}
				conflicts = append(conflicts, c)
			}
		}
		if len(conflicts) > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"error": fmt.Sprintf("%d of the items were changed by someone else since you loaded them", len(conflicts)), "code": "stale", "conflicts": conflicts})
			return
		}
		update := s.handleUpdateRequirement
		if kind == model.KindTest {
			update = s.handleUpdateTest
		}
		bctx := context.WithValue(ctx, batchKey{}, true)
		type saved struct {
			ID        string `json:"id"`
			Version   int    `json:"version"`
			UpdatedAt string `json:"updated_at,omitempty"`
		}
		out := make([]saved, 0, len(req.Items))
		for i, it := range req.Items {
			sub := r.Clone(bctx)
			sub.Method = http.MethodPatch
			sub.Body = io.NopCloser(bytes.NewReader(it.Set))
			sub.ContentLength = int64(len(it.Set))
			sub.SetPathValue("id", it.ID)
			sub.Header.Del("If-Match")
			if it.Version > 0 {
				sub.Header.Set("If-Match", `"`+strconv.Itoa(it.Version)+`"`)
			}
			rec := &heldResponse{header: http.Header{}}
			update(rec, sub, p)
			if rec.status() >= 400 {
				writeBatchFailure(w, rec, i, it.ID)
				return
			}
			var res struct {
				Version   int       `json:"version"`
				UpdatedAt time.Time `json:"updated_at"`
			}
			_ = json.Unmarshal(rec.body.Bytes(), &res)
			sv := saved{ID: it.ID, Version: res.Version}
			if !res.UpdatedAt.IsZero() {
				sv.UpdatedAt = res.UpdatedAt.UTC().Format(time.RFC3339Nano)
			}
			out = append(out, sv)
		}
		writeJSON(w, http.StatusOK, map[string]any{"updated": len(out), "items": out})
	}
}

// checkBatch validates the shape of a batch.
func checkBatch(items []batchItem) error {
	if len(items) == 0 {
		return badRequest("items required")
	}
	if len(items) > maxBatchItems {
		return badRequest("at most %d items per batch", maxBatchItems)
	}
	seen := map[string]bool{}
	for i, it := range items {
		if it.ID == "" {
			return badRequest("item %d: id required", i)
		}
		if seen[it.ID] {
			return badRequest("item %d: %s appears more than once", i, it.ID)
		}
		seen[it.ID] = true
		var set map[string]json.RawMessage
		if err := json.Unmarshal(it.Set, &set); err != nil || len(set) == 0 {
			return badRequest("item %d: set must be an object with the fields to change", i)
		}
	}
	return nil
}

// itemVersion returns an item's project, version and key.
func (s *Server) itemVersion(ctx context.Context, p *Principal, kind, id string) (project string, version int, key string, err error) {
	if kind == model.KindRequirement {
		rq, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
		if err != nil {
			return "", 0, "", err
		}
		return rq.ProjectID, rq.Version, rq.Key, nil
	}
	t, err := s.Store.GetTest(ctx, p.TenantID(), id)
	if err != nil {
		return "", 0, "", err
	}
	return t.ProjectID, t.Version, t.Key, nil
}

// writeBatchFailure answers with the failed item's error, its index and id.
func writeBatchFailure(w http.ResponseWriter, rec *heldResponse, index int, id string) {
	var e apiError
	_ = json.Unmarshal(rec.body.Bytes(), &e)
	if e.Code == "" {
		e.Code = "error"
	}
	if e.Error == "" {
		e.Error = http.StatusText(rec.status())
	}
	status := rec.status()
	if status == http.StatusPreconditionFailed {
		// a save that raced with this batch (versions were checked above)
		status = http.StatusConflict
	}
	writeJSON(w, status, itemBatchFailure{apiError: apiError{Error: fmt.Sprintf("item %d (%s): %s", index+1, id, e.Error), Code: e.Code}, Index: index, ID: id})
}

func notFound(kind, id string) error {
	return fmt.Errorf("%s %s is not in this project: %w", kind, id, store.ErrNotFound)
}

// ---------------------------------------------------------------- view export

// maxExportCells caps an exported view (rows × columns).
const maxExportCells = 2_000_000

// handleExportView renders a table the client shows (the spreadsheet view
// with its visible columns, order, sort and filters) with the export
// writers: POST /projects/{pid}/export/view
// {"format": "xlsx|csv|md|pdf|docx", "title", "name", "lang", "columns", "rows"}.
func (s *Server) handleExportView(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Format  string     `json:"format"`
		Title   string     `json:"title"`
		Name    string     `json:"name"`
		Lang    string     `json:"lang"`
		Filter  string     `json:"filter"`
		Columns []string   `json:"columns"`
		Rows    [][]string `json:"rows"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	f, err := export.ParseFormat(req.Format)
	if err != nil {
		s.fail(w, r, badRequest("%v", err))
		return
	}
	if len(req.Columns) == 0 || len(req.Columns) > 200 {
		s.fail(w, r, badRequest("between 1 and 200 columns required"))
		return
	}
	if len(req.Rows)*len(req.Columns) > maxExportCells {
		s.fail(w, r, badRequest("the view is too large to export (at most %d cells)", maxExportCells))
		return
	}
	for i, row := range req.Rows {
		if len(row) > len(req.Columns) {
			s.fail(w, r, badRequest("row %d has more cells than there are columns", i+1))
			return
		}
	}
	name := sectionName(req.Name)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = name
	}
	ds := export.Dataset{
		Title: pr.Key + " – " + pr.Name, GeneratedAt: time.Now().UTC(), Lang: req.Lang,
		Sections: []export.Section{{Name: name, Title: title, Columns: req.Columns, Rows: req.Rows}},
		Link:     s.itemLink(pr),
	}
	if req.Filter != "" {
		ds.Meta = []export.Item{{Label: "Filter", Value: req.Filter}}
	}
	file := fmt.Sprintf("%s-%s-%s.%s", pr.Key, name, time.Now().UTC().Format("20060102-150405"), f.Extension(false))
	var buf bytes.Buffer
	if err := export.Write(&buf, f, ds); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.export", "project", pr.ID, string(f)+" view "+name)
	w.Header().Set("Content-Type", f.ContentTypeFor(false))
	w.Header().Set("Content-Disposition", `attachment; filename="`+file+`"`)
	_, _ = w.Write(buf.Bytes())
}

// sectionName keeps a-z, 0-9 and dashes of a sheet / file name.
func sectionName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == ' ':
			b.WriteRune('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	if b.Len() == 0 {
		return "view"
	}
	return b.String()
}
