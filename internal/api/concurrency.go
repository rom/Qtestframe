package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/store"
)

// Optimistic concurrency for requirements and tests: GET and PATCH answer
// with an ETag holding the item's version; a PATCH that sends If-Match with
// an older version (someone saved in between) is refused with 412 and the
// current item, so the client can show both versions instead of silently
// overwriting the other person's changes. Without If-Match a PATCH saves as
// before, but two saves that race are still caught by the store.

// setVersionTag sets the ETag of an item version.
func setVersionTag(w http.ResponseWriter, version int) {
	w.Header().Set("ETag", `"`+strconv.Itoa(version)+`"`)
}

// ifMatchVersion reads If-Match: `"5"`, `W/"5"` or `5`. ok is false
// when the header is absent or "*"; a malformed value is an error.
func ifMatchVersion(r *http.Request) (version int, ok bool, err error) {
	v := strings.TrimSpace(r.Header.Get("If-Match"))
	if v == "" || v == "*" {
		return 0, false, nil
	}
	v = strings.Trim(strings.TrimPrefix(v, "W/"), `"`)
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, false, badRequest("If-Match must be the item version from its ETag")
	}
	return n, true, nil
}

// staleResponse is the 412 body: the error plus what is saved now.
type staleResponse struct {
	apiError
	// Current is the item as it is saved now.
	Current any `json:"current"`
	// ChangedBy and ChangedAt describe the latest saved version.
	ChangedBy string `json:"changed_by,omitempty"`
	ChangedAt string `json:"changed_at,omitempty"`
}

// writeStale answers 412 with the current item and who saved it last.
func (s *Server) writeStale(ctx context.Context, w http.ResponseWriter, p *Principal, kind, id string, version int, current any) {
	out := staleResponse{apiError: apiError{Error: "it was changed by someone else since you opened it", Code: "stale"}, Current: current}
	if revs, err := s.Store.ListRevisions(ctx, p.TenantID(), kind, id); err == nil {
		for _, rv := range revs {
			if rv.Version == version {
				out.ChangedBy = rv.UserName
				if out.ChangedBy == "" {
					out.ChangedBy = rv.UserEmail
				}
				out.ChangedAt = rv.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			}
		}
	}
	setVersionTag(w, version)
	writeJSON(w, http.StatusPreconditionFailed, out)
}

// isStale reports a save that lost a race with another one.
func isStale(err error) bool { return errors.Is(err, store.ErrStale) }

// requireVersion checks If-Match against the saved version; on a mismatch
// it answers 412 and returns false.
func (s *Server) requireVersion(w http.ResponseWriter, r *http.Request, p *Principal, kind, id string, saved int) bool {
	want, ok, err := ifMatchVersion(r)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if ok && want != saved {
		s.staleItem(w, r, p, kind, id)
		return false
	}
	return true
}

// staleItem answers 412 with the item as it is saved now.
func (s *Server) staleItem(w http.ResponseWriter, r *http.Request, p *Principal, kind, id string) {
	if kind == "requirement" {
		cur, err := s.decoratedRequirement(r, p, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.writeStale(r.Context(), w, p, kind, id, cur.Version, cur)
		return
	}
	cur, err := s.decoratedTest(r, p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeStale(r.Context(), w, p, kind, id, cur.Version, cur)
}
