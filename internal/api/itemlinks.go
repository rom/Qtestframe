package api

import (
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// Item links: links from a requirement, test or run to other items of the
// project (requirements, tests, runs, archive files, report definitions) or
// to external URLs. Reading needs the read permission of the item, adding
// and removing the write permission of the item that carries the link.

func linkReadPerm(typ string) rbac.Permission {
	switch typ {
	case model.KindRequirement:
		return rbac.RequirementRd
	case model.KindTest:
		return rbac.TestRead
	case "run":
		return rbac.RunRead
	}
	return rbac.ProjectRead
}

func linkWritePerm(typ string) rbac.Permission {
	switch typ {
	case model.KindRequirement:
		return rbac.RequirementWr
	case model.KindTest:
		return rbac.TestWrite
	}
	return rbac.RunWrite
}

func (s *Server) handleListItemLinks(typ string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		if _, err := s.linkSourceKey(r, p, typ, id); err != nil {
			s.fail(w, r, err)
			return
		}
		links, err := s.Store.ListItemLinks(r.Context(), p.TenantID(), typ, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out := make([]model.ItemLink, 0, len(links))
		for _, l := range links {
			other := l.TargetType
			if l.Direction == "incoming" {
				other = l.SourceType
			}
			if other == "url" || p.Can(linkReadPerm(other)) {
				out = append(out, l)
			}
		}
		writeJSON(w, http.StatusOK, listOf(out))
	}
}

// linkSourceKey checks that a link source belongs to the caller's tenant.
func (s *Server) linkSourceKey(r *http.Request, p *Principal, typ, id string) (string, error) {
	ctx := r.Context()
	switch typ {
	case model.KindRequirement:
		x, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
		if err != nil {
			return "", err
		}
		return x.Key, nil
	case model.KindTest:
		x, err := s.Store.GetTest(ctx, p.TenantID(), id)
		if err != nil {
			return "", err
		}
		return x.Key, nil
	default:
		x, err := s.Store.GetRun(ctx, p.TenantID(), id)
		if err != nil {
			return "", err
		}
		return x.Name, nil
	}
}

func (s *Server) handleCreateItemLink(typ string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		var req struct {
			TargetType string  `json:"target_type"`
			TargetID   *string `json:"target_id"`
			URL        string  `json:"url"`
			Label      string  `json:"label"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		if req.TargetType != "url" && req.TargetType != "" && !p.Can(linkReadPerm(req.TargetType)) {
			writeError(w, http.StatusForbidden, "forbidden", "permission "+string(linkReadPerm(req.TargetType))+" required to link to it")
			return
		}
		l := &model.ItemLink{SourceType: typ, SourceID: r.PathValue("id"), TargetType: req.TargetType, TargetID: req.TargetID, URL: req.URL, Label: req.Label, CreatedBy: p.User.Email}
		if err := s.Store.CreateItemLink(r.Context(), p.TenantID(), l); err != nil {
			s.fail(w, r, err)
			return
		}
		target := l.URL
		if l.TargetID != nil {
			target = l.TargetType + " " + l.TargetKey
		}
		s.audit(r.Context(), p, typ+".link.add", typ, l.SourceID, l.SourceKey+" → "+target)
		l.Direction = "outgoing"
		writeJSON(w, http.StatusCreated, l)
	}
}

// itemLinkForWrite loads a link and checks the write permission of its source.
func (s *Server) itemLinkForWrite(w http.ResponseWriter, r *http.Request, p *Principal) (*model.ItemLink, bool) {
	l, err := s.Store.GetItemLink(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	if perm := linkWritePerm(l.SourceType); !p.Can(perm) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(perm)+" required")
		return nil, false
	}
	return l, true
}

func (s *Server) handleUpdateItemLink(w http.ResponseWriter, r *http.Request, p *Principal) {
	l, ok := s.itemLinkForWrite(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Label string `json:"label"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateItemLinkLabel(r.Context(), p.TenantID(), l.ID, req.Label); err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.Store.GetItemLink(r.Context(), p.TenantID(), l.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleDeleteItemLink(w http.ResponseWriter, r *http.Request, p *Principal) {
	l, ok := s.itemLinkForWrite(w, r, p)
	if !ok {
		return
	}
	if err := s.Store.DeleteItemLink(r.Context(), p.TenantID(), l.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	target := l.URL
	if l.TargetID != nil {
		target = l.TargetType + " " + l.TargetKey
	}
	s.audit(r.Context(), p, l.SourceType+".link.remove", l.SourceType, l.SourceID, l.SourceKey+" → "+target)
	w.WriteHeader(http.StatusNoContent)
}
