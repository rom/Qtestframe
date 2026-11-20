package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// ReqIF export of a project's requirements: GET /projects/{pid}/export
// with format=reqif or reqifz (see handleExport and docs/api.md "Export").

// reqifFormat returns "reqif" or "reqifz" when the format parameter names
// a ReqIF export, else "".
func reqifFormat(f string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(f)), ".") {
	case "reqif":
		return "reqif"
	case "reqifz":
		return "reqifz"
	}
	return ""
}

func (s *Server) exportReqIF(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project, zipped bool) {
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	expr := r.URL.Query().Get("q")
	reqs, err := s.filterRequirements(r, p, pr, pc, reqExpr(expr), "", true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	doc := export.ReqIFDocument{ProjectID: pr.ID, ProjectKey: pr.Key, Title: pr.Key + " – " + pr.Name, ToolVersion: Version, GeneratedAt: now,
		Comment: "Requirements of project " + pr.Key + " exported by Qtestframe"}
	if strings.TrimSpace(expr) != "" {
		doc.Comment += "; filter: " + strings.TrimSpace(expr)
	}

	// Folders: all requirement folders, or with a filter only those that
	// contain an exported requirement (directly or below).
	parent := map[string]string{}
	for _, f := range pc.folders {
		if f.Kind == model.KindRequirement && f.ParentID != nil {
			parent[f.ID] = *f.ParentID
		}
	}
	keep := map[string]bool{}
	for _, rq := range reqs {
		for id := rq.FolderID; id != nil && !keep[*id]; {
			keep[*id] = true
			if pid, ok := parent[*id]; ok {
				id = &pid
			} else {
				id = nil
			}
		}
	}
	filtered := strings.TrimSpace(reqExpr(expr)) != ""
	for _, f := range pc.folders {
		if f.Kind != model.KindRequirement || (filtered && !keep[f.ID]) {
			continue
		}
		rf := export.ReqIFFolder{ID: f.ID, Name: f.Name, CreatedAt: f.CreatedAt}
		if f.ParentID != nil {
			rf.ParentID = *f.ParentID
		}
		doc.Folders = append(doc.Folders, rf)
	}

	for i := range reqs {
		rq := &reqs[i]
		_, cats := pc.catValues(rq.CategoryIDs)
		x := export.ReqIFRequirement{ID: rq.ID, Key: rq.Key, Title: rq.Title, Description: rq.Description, Obligation: rq.Obligation,
			Type: rq.Type, Priority: rq.Priority, Status: rq.Status, Source: rq.Source, Rationale: rq.Rationale, Owner: rq.Owner,
			CreatedBy: rq.CreatedBy, Tags: rq.Tags, Categories: cats, Version: rq.Version, CreatedAt: rq.CreatedAt, UpdatedAt: rq.UpdatedAt}
		if rq.FolderID != nil {
			x.FolderID = *rq.FolderID
		}
		for _, c := range rq.Criteria {
			x.Criteria = append(x.Criteria, export.ReqIFCriterion{Description: c.Description, Method: c.Method, Threshold: c.Threshold})
		}
		if zipped {
			files, err := s.Store.ListAttachments(ctx, p.TenantID(), model.KindRequirement, rq.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for _, f := range files {
				id := f.ID
				x.Attachments = append(x.Attachments, export.ReqIFFile{ID: f.ID, Name: f.Name, ContentType: f.ContentType,
					Load: func() ([]byte, error) { return s.Store.FileData(ctx, id) }})
			}
		}
		doc.Requirements = append(doc.Requirements, x)
	}
	for _, rel := range rels {
		doc.Relations = append(doc.Relations, export.ReqIFRelation{ID: rel.ID, Type: rel.Type, SourceID: rel.SourceID, TargetID: rel.TargetID, CreatedAt: rel.CreatedAt})
	}

	name := export.ReqIFFileName(pr.Key, now, zipped)
	format := "reqif"
	if zipped {
		format = "reqifz"
		w.Header().Set("Content-Type", export.ReqIFZContentType)
	} else {
		w.Header().Set("Content-Type", export.ReqIFContentType)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if zipped {
		err = export.WriteReqIFZ(w, doc)
	} else {
		err = export.WriteReqIF(w, doc)
	}
	if err != nil {
		s.Log.Error("reqif export failed", "err", err)
		return
	}
	s.audit(ctx, p, "project.export", "project", pr.ID, format+" requirements")
}
