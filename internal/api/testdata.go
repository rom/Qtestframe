package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Test data sets (see docs/api.md "Test data sets"): named, versioned data
// linked to tests, configurations and runs. Reading needs test.read,
// changing needs test.write; anyone who writes tests or executes runs can
// ask the owner of a set to refresh it.

// dataSetSchema filters data sets (GET /projects/{pid}/test-data-sets?q=).
var dataSetSchema = &query.Schema{
	Fields: []string{"id", "name", "description", "kind", "classification", "owner", "status", "tag", "expires", "expired", "expiring",
		"parameter", "linked", "version", "usage", "refresh", "created_by", "created_at", "updated_at"},
	Aliases: map[string]string{"tags": "tag", "class": "classification", "expires_on": "expires", "parameters": "parameter", "param": "parameter",
		"links": "linked", "used": "usage", "refreshes": "refresh"},
}

func (s *Server) routesTestData(h routeFunc) {
	h("GET /api/v1/projects/{pid}/test-data-sets", s.handleListDataSets, "test.read")
	h("POST /api/v1/projects/{pid}/test-data-sets", s.handleCreateDataSet, "test.write")
	h("GET /api/v1/test-data-sets/{id}", s.handleGetDataSet, "test.read")
	h("PATCH /api/v1/test-data-sets/{id}", s.handleUpdateDataSet, "test.write")
	h("DELETE /api/v1/test-data-sets/{id}", s.handleDeleteDataSet, "test.write")
	h("GET /api/v1/test-data-sets/{id}/history", s.handleDataSetHistory, "test.read")
	h("GET /api/v1/test-data-sets/{id}/usage", s.handleDataSetUsage, "test.read")
	h("POST /api/v1/test-data-sets/{id}/links", s.handleLinkDataSet, "test.write")
	h("DELETE /api/v1/test-data-sets/{id}/links/{link}", s.handleUnlinkDataSet, "test.write")
	h("GET /api/v1/test-data-sets/{id}/refresh-requests", s.handleListRefreshRequests, "test.read")
	h("POST /api/v1/test-data-sets/{id}/refresh-requests", s.handleCreateRefreshRequest, "test.read")
	h("POST /api/v1/test-data-refresh-requests/{id}/resolve", s.handleResolveRefreshRequest, "test.read")
	h("GET /api/v1/tests/{id}/data-sets", s.handleItemDataSets(model.KindTest), "test.read")
	h("GET /api/v1/configurations/{id}/data-sets", s.handleItemDataSets("configuration"), "test.read")
	h("GET /api/v1/runs/{id}/data-sets", s.handleItemDataSets("run"), "test.read")
}

// dataSetReq is the body of POST /projects/{pid}/test-data-sets and PATCH
// /test-data-sets/{id}.
type dataSetReq struct {
	Name           *string          `json:"name"`
	Description    *string          `json:"description"`
	Kind           *string          `json:"kind"`
	Parameters     *[]string        `json:"parameters"`
	DataRows       *[]model.DataRow `json:"data_rows"`
	Classification *string          `json:"classification"`
	OwnerID        *string          `json:"owner_id"`
	ExpiresOn      *string          `json:"expires_on"`
	Tags           *[]string        `json:"tags"`
	Status         *string          `json:"status"`
	// Version (PATCH): the version the change is based on (0: any).
	Version int `json:"version"`
}

func (in *dataSetReq) apply(d *model.DataSet) {
	for _, f := range []struct{ src, dst *string }{{in.Name, &d.Name}, {in.Description, &d.Description}, {in.Kind, &d.Kind},
		{in.Classification, &d.Classification}, {in.OwnerID, &d.OwnerID}, {in.ExpiresOn, &d.ExpiresOn}, {in.Status, &d.Status}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if in.Parameters != nil {
		d.Parameters = *in.Parameters
	}
	if in.DataRows != nil {
		d.DataRows = *in.DataRows
	}
	if in.Tags != nil {
		d.Tags = *in.Tags
	}
}

func dataSetRecord(d *model.DataSet, people func(string) []string) query.MapRecord {
	var linked []string
	for _, l := range d.Links {
		linked = append(linked, l.ItemKey)
	}
	return query.MapRecord{"id": {d.ID}, "name": {d.Name}, "description": {d.Description}, "kind": {d.Kind}, "classification": {d.Classification},
		"owner": people(d.OwnerID), "status": {d.Status}, "tag": d.Tags, "expires": nonEmpty(d.ExpiresOn), "expired": {strconv.FormatBool(d.Expired)},
		"expiring": {strconv.FormatBool(d.Expiring || d.Expired)}, "parameter": d.Parameters, "linked": linked, "version": {strconv.Itoa(d.Version)},
		"usage": {strconv.Itoa(d.UsageCount)}, "refresh": {strconv.Itoa(d.OpenRefreshes)}, "created_by": {d.CreatedBy}, "created_at": {ts(d.CreatedAt)},
		"updated_at": {ts(d.UpdatedAt)}}
}

func (s *Server) handleListDataSets(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q, err := query.Parse(r.URL.Query().Get("q"), dataSetSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListDataSets(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	people := s.peopleLookup(r.Context())
	out := []model.DataSet{}
	var recs []query.MapRecord
	for i := range list {
		if rec := dataSetRecord(&list[i], people); q.Match(rec) {
			out = append(out, list[i])
			recs = append(recs, rec)
		}
	}
	sortByQuery(q, out, recs)
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) recordDataSet(ctx context.Context, tenantID, user, id, action string) {
	d, err := s.Store.GetDataSet(ctx, tenantID, id)
	if err == nil {
		err = s.Store.AddRevision(ctx, tenantID, model.KindDataSet, id, d.Version, action, user, store.SnapshotDataSet(d))
	}
	if err != nil {
		s.Log.Warn("revision not recorded", "entity", model.KindDataSet, "id", id, "err", err)
	}
}

func (s *Server) handleCreateDataSet(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	var in dataSetReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	d := &model.DataSet{ProjectID: pr.ID, OwnerID: p.User.ID, CreatedBy: p.User.Email}
	in.apply(d)
	if err := s.checkTester(ctx, p.TenantID(), pr.ID, d.OwnerID, "owner"); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateDataSet(ctx, p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "testdata.create", model.KindDataSet, d.ID, d.Name+" ("+d.Classification+")")
	s.recordDataSet(ctx, p.TenantID(), p.User.Email, d.ID, "create")
	out, err := s.Store.GetDataSet(ctx, p.TenantID(), d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleGetDataSet(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetDataSet(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUpdateDataSet(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	d, err := s.Store.GetDataSet(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in dataSetReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	owner := d.OwnerID
	in.apply(d)
	if d.OwnerID != owner {
		if err := s.checkTester(ctx, p.TenantID(), d.ProjectID, d.OwnerID, "owner"); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if err := s.Store.UpdateDataSet(ctx, p.TenantID(), d, in.Version); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "testdata.update", model.KindDataSet, d.ID, d.Name+" v"+strconv.Itoa(d.Version))
	s.recordDataSet(ctx, p.TenantID(), p.User.Email, d.ID, "update")
	out, err := s.Store.GetDataSet(ctx, p.TenantID(), d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteDataSet(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetDataSet(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteDataSet(r.Context(), p.TenantID(), d.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "testdata.delete", model.KindDataSet, d.ID, d.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDataSetHistory(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetDataSet(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	revs, err := s.Store.ListRevisions(r.Context(), p.TenantID(), model.KindDataSet, d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(revs))
}

func (s *Server) handleDataSetUsage(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetDataSet(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Store.DataSetUsage(r.Context(), p.TenantID(), d.ID, limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleLinkDataSet(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	d, err := s.Store.GetDataSet(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		ItemType string `json:"item_type"`
		ItemID   string `json:"item_id"`
		Usage    string `json:"usage"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	l := &model.DataLink{DataSetID: d.ID, ItemType: in.ItemType, ItemID: strings.TrimSpace(in.ItemID), Usage: in.Usage, CreatedBy: p.User.Email}
	if err := s.Store.SetDataLink(ctx, p.TenantID(), l); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "testdata.link", model.KindDataSet, d.ID, d.Name+" → "+l.ItemType+" "+l.ItemID+" ("+l.Usage+")")
	s.recordDataSet(ctx, p.TenantID(), p.User.Email, d.ID, "update")
	out, err := s.Store.GetDataSet(ctx, p.TenantID(), d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUnlinkDataSet(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	d, err := s.Store.GetDataSet(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteDataLink(ctx, p.TenantID(), d.ID, r.PathValue("link")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "testdata.unlink", model.KindDataSet, d.ID, d.Name)
	s.recordDataSet(ctx, p.TenantID(), p.User.Email, d.ID, "update")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleItemDataSets(itemType string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		list, err := s.Store.ItemDataLinks(r.Context(), p.TenantID(), itemType, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(list))
	}
}

func (s *Server) handleListRefreshRequests(w http.ResponseWriter, r *http.Request, p *Principal) {
	list, err := s.Store.ListRefreshRequests(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleCreateRefreshRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	if !p.Can(rbac.TestWrite) && !p.Can(rbac.RunExecute) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.write or run.execute required")
		return
	}
	d, err := s.Store.GetDataSet(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Reason   string `json:"reason"`
		NeededBy string `json:"needed_by"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	q := &model.RefreshRequest{DataSetID: d.ID, RequesterID: p.User.ID, RequestedBy: p.User.Email, Reason: in.Reason, NeededBy: in.NeededBy}
	if err := s.Store.CreateRefreshRequest(ctx, p.TenantID(), q); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "testdata.refresh_request", model.KindDataSet, d.ID, d.Name+": "+excerpt(q.Reason, 200))
	if d.OwnerID != "" {
		s.notifyUsers(ctx, p, []string{d.OwnerID}, model.Notification{ProjectID: d.ProjectID, Event: "testdata.refresh_requested", Title: d.Name,
			Body: excerpt(q.Reason, 300), EntityType: model.KindDataSet, EntityID: d.ID})
	}
	s.notifyEvent(r, p, d.ProjectID, "testdata.refresh_requested", map[string]any{"data_set": map[string]any{"id": d.ID, "name": d.Name, "status": d.Status}, "request": q})
	out, err := s.Store.GetRefreshRequest(ctx, p.TenantID(), q.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleResolveRefreshRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	q, err := s.Store.GetRefreshRequest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.Store.GetDataSet(ctx, p.TenantID(), q.DataSetID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !p.Can(rbac.TestWrite) && d.OwnerID != p.User.ID {
		writeError(w, http.StatusForbidden, "forbidden", "only the owner of the data set or someone with test.write can close a refresh request")
		return
	}
	var in struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.Store.ResolveRefreshRequest(ctx, p.TenantID(), q.ID, in.Status, p.User.Email, in.Note)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "testdata.refresh_"+out.Status, model.KindDataSet, d.ID, d.Name+nonBlank(": ", in.Note))
	if out.RequesterID != "" {
		s.notifyUsers(ctx, p, []string{out.RequesterID}, model.Notification{ProjectID: d.ProjectID, Event: "testdata.refresh_resolved", Title: d.Name,
			Body: out.Status + nonBlank(": ", excerpt(in.Note, 300)), EntityType: model.KindDataSet, EntityID: d.ID})
	}
	s.notifyEvent(r, p, d.ProjectID, "testdata.refresh_resolved", map[string]any{"data_set": map[string]any{"id": d.ID, "name": d.Name, "status": d.Status}, "request": out})
	writeJSON(w, http.StatusOK, out)
}
