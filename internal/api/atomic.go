package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
)

// Bulk requests are atomic: atomic runs the whole handler in one database
// transaction and holds the response back until the end. When the handler
// answers with an error status (or panics) nothing it wrote is saved;
// otherwise the transaction is committed and the response sent. Webhooks
// fire only after the commit (see notifyEvent). Items a handler skips with
// a reason are part of a successful answer, so the others are still saved.
//
// Only for handlers that work with the database: an outside call (a defect
// tracker, a webhook test) or a streamed download would hold the
// transaction, and with it SQLite's single connection, open.
func (s *Server) atomic(fn handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		rec := &heldResponse{header: http.Header{}}
		err := s.Store.InTx(r.Context(), func(ctx context.Context) error {
			fn(rec, r.WithContext(ctx), p)
			if rec.status() >= 400 {
				return errRolledBack
			}
			return nil
		})
		if err != nil && !errors.Is(err, errRolledBack) {
			// the commit failed: nothing was saved
			s.fail(w, r, err)
			return
		}
		rec.send(w)
	}
}

var errRolledBack = errors.New("rolled back")

// heldResponse records a response so it can be sent after the commit.
type heldResponse struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (h *heldResponse) Header() http.Header { return h.header }

func (h *heldResponse) WriteHeader(code int) {
	if h.code == 0 {
		h.code = code
	}
}

func (h *heldResponse) Write(b []byte) (int, error) {
	if h.code == 0 {
		h.code = http.StatusOK
	}
	return h.body.Write(b)
}

func (h *heldResponse) status() int {
	if h.code == 0 {
		return http.StatusOK
	}
	return h.code
}

func (h *heldResponse) send(w http.ResponseWriter) {
	for k, v := range h.header {
		w.Header()[k] = v
	}
	w.WriteHeader(h.status())
	_, _ = w.Write(h.body.Bytes())
}
