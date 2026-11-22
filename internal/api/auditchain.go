package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// handleVerifyAudit: POST /audit/verify verifies the organisation's audit
// chain (and writes a signed checkpoint at its end): OK or the first broken
// link.
func (s *Server) handleVerifyAudit(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	v, err := s.Store.VerifyAudit(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	details := "ok: " + strconv.FormatInt(v.Entries, 10) + " entries"
	if !v.OK {
		details = "broken at entry " + strconv.FormatInt(v.Broken.Seq, 10) + ": " + v.Broken.Problem
	}
	s.audit(r.Context(), p, "audit.verify", "audit", "", details)
	writeJSON(w, http.StatusOK, v)
}

// handleAuditArchive: GET /audit/archive?format=json|csv exports the whole
// audit chain of the organisation with sequence numbers and hashes for
// external archiving (JSON also carries the signed checkpoints and the end
// of the chain; `qtestframe audit verify --file` checks it offline).
func (s *Server) handleAuditArchive(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		s.fail(w, r, badRequest("format must be json or csv"))
		return
	}
	ctx := r.Context()
	tid := p.TenantID()
	if err := s.Store.CheckpointAudit(ctx, tid); err != nil {
		s.fail(w, r, err)
		return
	}
	headSeq, headHash, err := s.Store.AuditHead(ctx, tid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cps, err := s.Store.ListAuditCheckpoints(ctx, tid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	name := "audit-trail-" + p.Tenant.Slug + "-" + time.Now().UTC().Format("20060102-150405") + "." + format
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	n := 0
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"seq", "id", "created_at", "user_id", "user_email", "action", "entity_type", "entity_id", "details", "reason", "prev_hash", "hash"})
		err = s.Store.WalkAuditChain(ctx, tid, func(e *model.AuditEntry) error {
			n++
			return cw.Write([]string{strconv.FormatInt(e.Seq, 10), e.ID, e.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z"), e.UserID, e.UserEmail, e.Action,
				e.EntityType, e.EntityID, e.Details, e.Reason, e.PrevHash, e.Hash})
		})
		cw.Flush()
	} else {
		w.Header().Set("Content-Type", "application/json")
		head := store.AuditArchive{Format: store.AuditArchiveFormat, Chain: store.AuditChainVersion, TenantID: tid, Tenant: p.Tenant.Slug,
			ExportedAt: s.Store.Now(), HeadSeq: headSeq, HeadHash: headHash, Checkpoints: cps}
		b, _ := json.Marshal(head)
		// the entries are streamed into the object: all but its last `"entries":null}`
		b = b[:len(b)-len(`"entries":null}`)]
		_, _ = w.Write(b)
		_, _ = w.Write([]byte(`"entries":[`))
		enc := json.NewEncoder(w)
		err = s.Store.WalkAuditChain(ctx, tid, func(e *model.AuditEntry) error {
			if n > 0 {
				if _, err := w.Write([]byte(",")); err != nil {
					return err
				}
			}
			n++
			return enc.Encode(e)
		})
		_, _ = w.Write([]byte("]}\n"))
	}
	if err != nil {
		s.Log.Error("audit archive failed", "err", err)
		return
	}
	s.audit(ctx, p, "audit.archive", "audit", "", format+": "+strconv.Itoa(n)+" entries up to "+strconv.FormatInt(headSeq, 10))
}
