package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/mention"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
)

// Owners of requirements, tests and risks.
//
// An owner is a project member, stored as the member's e-mail address like
// the assignees of defects and executions. Writes accept any way of naming
// a member (e-mail address, display name, @handle or user id) and store the
// e-mail address; a member who may not see the project is refused. A name
// that is no member stays as free text (owners imported from other tools,
// people outside the organisation). Changing a member's e-mail address
// moves the references along (store.SetUserEmail).

// people is the tenant's people directory for resolving owners.
type people struct {
	// index: lower-case name, e-mail, handle, @handle or user id → user id
	index map[string]string
	// email: user id → e-mail address
	email map[string]string
}

// loadPeople reads the tenant's members; nil when they cannot be read.
func (s *Server) loadPeople(ctx context.Context, p *Principal) *people {
	ms, err := s.Store.ListTenantMembers(ctx, p.TenantID())
	if err != nil {
		return nil
	}
	users := make([]mention.User, 0, len(ms))
	out := &people{index: map[string]string{}, email: map[string]string{}}
	for _, m := range ms {
		users = append(users, mention.User{ID: m.UserID, Email: m.Email, Name: m.Name})
		out.email[m.UserID] = m.Email
	}
	for _, d := range mention.Directory(users, nil) {
		if d.UserID == "" {
			continue
		}
		for _, k := range []string{d.UserID, d.Email, d.Name, d.Handle, "@" + d.Handle} {
			if k != "" {
				out.index[strings.ToLower(k)] = d.UserID
			}
		}
	}
	return out
}

// resolve returns the member a name stands for ("" none).
func (pp *people) resolve(name string) string {
	if pp == nil {
		return ""
	}
	return pp.index[strings.ToLower(strings.TrimSpace(name))]
}

// namesOf returns every way a member can be named (lower case), the
// e-mail address first.
func (pp *people) namesOf(userID, email string) []string {
	out := []string{strings.ToLower(email)}
	if pp != nil {
		for k, id := range pp.index {
			if id == userID && k != out[0] {
				out = append(out, k)
			}
		}
	}
	return out
}

// myNames returns the ways the caller can be named in owner and assignee
// fields.
func (s *Server) myNames(ctx context.Context, p *Principal) []string {
	return s.loadPeople(ctx, p).namesOf(p.User.ID, p.User.Email)
}

// bindMe makes "owner:me" and "assignee:me" (in the given fields) match
// the caller.
func (s *Server) bindMe(ctx context.Context, p *Principal, q *query.Query, fields ...string) {
	if q.IsEmpty() || p == nil || p.User == nil {
		return
	}
	if len(fields) == 0 {
		fields = []string{"owner", "assignee", "review_owner"}
	}
	q.Bind(fields, "me", s.myNames(ctx, p))
}

// ownerOf normalises an owner for a project: a member becomes their e-mail
// address (refused when they may not see the project), any other name is
// kept as text, "" stays "".
func (s *Server) ownerOf(ctx context.Context, p *Principal, pp *people, projectID, owner string) (string, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return "", nil
	}
	if len([]rune(owner)) > 200 {
		return "", badRequest("owner is too long")
	}
	id := pp.resolve(owner)
	if id == "" {
		return owner, nil
	}
	if !s.userCanSee(ctx, p.TenantID(), projectID, id, "") {
		return "", badRequest("%s is not a member of the project", owner)
	}
	return pp.email[id], nil
}

// normalizeOwner is ownerOf with the people read on demand.
func (s *Server) normalizeOwner(ctx context.Context, p *Principal, projectID string, owner *string) error {
	if owner == nil || strings.TrimSpace(*owner) == "" {
		if owner != nil {
			*owner = ""
		}
		return nil
	}
	v, err := s.ownerOf(ctx, p, s.loadPeople(ctx, p), projectID, *owner)
	if err != nil {
		return err
	}
	*owner = v
	return nil
}

// ownerNow returns the stored owner of a requirement or test ("" when it
// cannot be read).
func (s *Server) ownerNow(ctx context.Context, p *Principal, kind, id string) string {
	if kind == model.KindTest {
		if t, err := s.Store.GetTest(ctx, p.TenantID(), id); err == nil {
			return t.Owner
		}
		return ""
	}
	if rq, err := s.Store.GetRequirement(ctx, p.TenantID(), id); err == nil {
		return rq.Owner
	}
	return ""
}

// ownerAssigned tells the new owner of a requirement or test changed in
// bulk that it is theirs now.
func (s *Server) ownerAssigned(r *http.Request, p *Principal, kind, projectID, id string) {
	ctx := r.Context()
	var key, title, owner string
	if kind == model.KindTest {
		t, err := s.Store.GetTest(ctx, p.TenantID(), id)
		if err != nil {
			return
		}
		key, title, owner = t.Key, t.Title, t.Owner
	} else {
		rq, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
		if err != nil {
			return
		}
		key, title, owner = rq.Key, rq.Title, rq.Owner
	}
	s.notifyUsers(ctx, p, s.recipients(ctx, p, owner), model.Notification{ProjectID: projectID, Event: "assigned",
		Title: key + " " + title, EntityType: kind, EntityID: id})
}
