package store

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// The audit trail is a hash chain per tenant (tenant "" holds system
// entries). Audit writes an entry with seq NULL inside the caller's
// transaction; once that transaction commits, SealAudit chains the
// tenant's unchained entries in a short transaction of its own that holds
// the tenant's row in audit_heads: on PostgreSQL with SELECT … FOR UPDATE,
// so concurrent appends (also from other server processes) are serialised
// per tenant only while chaining, never while a request runs; SQLite has a
// single writer anyway. Each entry gets seq = previous + 1, prev_hash = the
// previous entry's hash and hash = SHA-256 of its canonical encoding
// (AuditEntryHash). Signed checkpoints (HMAC-SHA256 with a server key
// stored encrypted like the other secrets) are written every
// AuditCheckpointEvery entries, at least hourly and at every
// verification; they make a rewritten or truncated chain detectable.

// AuditChainVersion names the canonical encoding of an entry.
const AuditChainVersion = "qtf-audit-v1"

// AuditCheckpointEvery is the number of entries between signed checkpoints.
var AuditCheckpointEvery int64 = 1000

// auditCheckpointAge: a checkpoint is also written when the last one is
// older than this and entries were chained since.
const auditCheckpointAge = time.Hour

// auditSealBatch is the number of entries chained per transaction.
const auditSealBatch = 2000

// auditTime is the canonical form of an entry's time: UTC, microseconds
// (what both databases keep).
func auditTime(t time.Time) string {
	return t.UTC().Truncate(time.Microsecond).Format("2006-01-02T15:04:05.000000Z")
}

// AuditEntryHash returns the hash of an entry: hex SHA-256 over its
// canonical encoding, a version line followed by each field as
// "<length>:<value>\n" (tenant, seq, id, time, user id, user e-mail,
// action, entity type, entity id, details, reason, previous hash).
func AuditEntryHash(e *model.AuditEntry) string {
	h := sha256.New()
	h.Write([]byte(AuditChainVersion + "\n"))
	for _, f := range []string{e.TenantID, strconv.FormatInt(e.Seq, 10), e.ID, auditTime(e.CreatedAt), e.UserID, e.UserEmail,
		e.Action, e.EntityType, e.EntityID, e.Details, e.Reason, e.PrevHash} {
		h.Write([]byte(strconv.Itoa(len(f)) + ":" + f + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// checkpointMessage is what a checkpoint signs.
func checkpointMessage(tenantID string, seq int64, hash string, at time.Time) []byte {
	return []byte("qtf-audit-checkpoint-v1\n" + tenantID + "\n" + strconv.FormatInt(seq, 10) + "\n" + hash + "\n" + auditTime(at))
}

func signCheckpoint(key []byte, tenantID string, seq int64, hash string, at time.Time) string {
	m := hmac.New(sha256.New, key)
	m.Write(checkpointMessage(tenantID, seq, hash, at))
	return hex.EncodeToString(m.Sum(nil))
}

// ---------------------------------------------------------------- context

type reasonKey struct{}

// WithChangeReason returns a context whose audit entries record reason as
// the reason for change.
func WithChangeReason(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, reasonKey{}, reason)
}

// ChangeReason returns the reason for change of ctx ("" none).
func ChangeReason(ctx context.Context) string {
	r, _ := ctx.Value(reasonKey{}).(string)
	return r
}

// withoutTx returns ctx without its request transaction (for work after
// the commit).
func withoutTx(ctx context.Context) context.Context {
	if txOf(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, txKey{}, (*ctxTx)(nil))
}

// ---------------------------------------------------------------- sealing

// auditSealer coalesces the sealing of a tenant within this process: while
// one goroutine chains a tenant's entries, others only ask it to run once
// more. It also caches the checkpoint keys.
type auditSealer struct {
	mu      sync.Mutex
	running map[string]bool
	again   map[string]bool
	keys    map[string][]byte
}

func newAuditSealer() *auditSealer {
	return &auditSealer{running: map[string]bool{}, again: map[string]bool{}, keys: map[string][]byte{}}
}

func (a *auditSealer) begin(tenantID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running[tenantID] {
		a.again[tenantID] = true
		return false
	}
	a.running[tenantID] = true
	return true
}

func (a *auditSealer) end(tenantID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.again[tenantID] {
		delete(a.again, tenantID)
		return true
	}
	delete(a.running, tenantID)
	return false
}

// SealAudit chains the tenant's unchained audit entries. A call while
// another goroutine of this process seals the same tenant returns at once
// (that one runs again and chains the new entries).
func (s *Store) SealAudit(ctx context.Context, tenantID string) error {
	ctx = withoutTx(ctx)
	if !s.sealer.begin(tenantID) {
		return nil
	}
	var err error
	for {
		if err == nil {
			err = s.sealAll(ctx, tenantID, false)
		}
		if !s.sealer.end(tenantID) {
			return err
		}
	}
}

// sealAll chains every unchained entry of the tenant; checkpoint forces a
// signed checkpoint at the end of the chain.
func (s *Store) sealAll(ctx context.Context, tenantID string, checkpoint bool) error {
	for {
		n, err := s.sealOnce(ctx, tenantID, checkpoint)
		if err != nil {
			return err
		}
		if n < auditSealBatch {
			return nil
		}
	}
}

// SealAllAudit chains the unchained entries of every tenant (at start-up,
// which also completes the backfill after the upgrade).
func (s *Store) SealAllAudit(ctx context.Context) error {
	ctx = withoutTx(ctx)
	rows, err := s.c(ctx).query(ctx, `SELECT DISTINCT tenant_id FROM audit_log WHERE seq IS NULL`)
	if err != nil {
		return err
	}
	var tenants []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		tenants = append(tenants, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range tenants {
		if err := s.sealAll(ctx, t, false); err != nil {
			return fmt.Errorf("audit chain of tenant %q: %w", t, err)
		}
	}
	return nil
}

type auditHead struct {
	seq, checkpointSeq int64
	hash               string
	checkpointAt       *time.Time
}

// lockHead creates the tenant's head row if needed and locks it; created
// reports a new chain.
func (s *Store) lockHead(ctx context.Context, c conn, tenantID string) (*auditHead, bool, error) {
	res, err := c.exec(ctx, `INSERT INTO audit_heads (tenant_id, seq, hash, checkpoint_seq, updated_at) VALUES (?, 0, '', 0, ?) ON CONFLICT (tenant_id) DO NOTHING`, tenantID, s.now())
	if err != nil {
		return nil, false, err
	}
	n, _ := res.RowsAffected()
	q := `SELECT seq, hash, checkpoint_seq, checkpoint_at FROM audit_heads WHERE tenant_id = ?`
	if s.DB.Dialect == db.Postgres {
		q += ` FOR UPDATE`
	}
	h := &auditHead{}
	var at sql.NullTime
	if err := c.row(ctx, q, tenantID).Scan(&h.seq, &h.hash, &h.checkpointSeq, &at); err != nil {
		return nil, false, err
	}
	h.checkpointAt = timePtr(at)
	return h, n == 1, nil
}

// sealOnce chains up to auditSealBatch entries in one transaction and
// returns how many.
func (s *Store) sealOnce(ctx context.Context, tenantID string, checkpoint bool) (int, error) {
	count := 0
	err := s.tx(ctx, func(c conn) error {
		h, created, err := s.lockHead(ctx, c, tenantID)
		if err != nil {
			return err
		}
		now := s.now()
		if created {
			// the first chaining after the upgrade: a genesis entry first
			res, err := c.exec(ctx, `DELETE FROM audit_backfill WHERE tenant_id = ?`, tenantID)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				var pending int
				if err := c.row(ctx, `SELECT COUNT(*) FROM audit_log WHERE tenant_id = ? AND seq IS NULL`, tenantID).Scan(&pending); err != nil {
					return err
				}
				g := model.AuditEntry{ID: newID(), TenantID: tenantID, UserEmail: "system", Action: "audit.chain_start", EntityType: "audit",
					Details: fmt.Sprintf("audit trail hash chain started; the %d entries written before this upgrade follow as entries 2 to %d (backfill: their own times are kept, the chain only proves they were not changed after %s)",
						pending, pending+1, auditTime(now)),
					CreatedAt: now, Seq: 1}
				g.Hash = AuditEntryHash(&g)
				if _, err := c.exec(ctx, `INSERT INTO audit_log (id, tenant_id, user_id, user_email, action, entity_type, entity_id, details, reason, created_at, seq, prev_hash, hash) VALUES (?, ?, '', ?, ?, ?, '', ?, '', ?, ?, '', ?)`,
					g.ID, g.TenantID, g.UserEmail, g.Action, g.EntityType, g.Details, g.CreatedAt, g.Seq, g.Hash); err != nil {
					return err
				}
				h.seq, h.hash = 1, g.Hash
			}
		}
		rows, err := c.query(ctx, `SELECT id, user_id, user_email, action, entity_type, entity_id, details, reason, created_at FROM audit_log
			WHERE tenant_id = ? AND seq IS NULL ORDER BY created_at, id LIMIT ?`, tenantID, auditSealBatch)
		if err != nil {
			return err
		}
		var batch []model.AuditEntry
		for rows.Next() {
			e := model.AuditEntry{TenantID: tenantID}
			if err := rows.Scan(&e.ID, &e.UserID, &e.UserEmail, &e.Action, &e.EntityType, &e.EntityID, &e.Details, &e.Reason, &e.CreatedAt); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range batch {
			e := &batch[i]
			h.seq++
			e.Seq, e.PrevHash = h.seq, h.hash
			e.Hash = AuditEntryHash(e)
			h.hash = e.Hash
		}
		if err := updateChained(ctx, c, batch); err != nil {
			return err
		}
		count = len(batch)
		due := h.seq > h.checkpointSeq && (checkpoint || h.seq-h.checkpointSeq >= AuditCheckpointEvery ||
			(count > 0 && (h.checkpointAt == nil || now.Sub(*h.checkpointAt) >= auditCheckpointAge)))
		if due && (count < auditSealBatch || h.seq-h.checkpointSeq >= AuditCheckpointEvery) {
			if _, err := s.writeCheckpoint(ctx, c, tenantID, h.seq, h.hash, now); err != nil {
				return err
			}
			h.checkpointSeq, h.checkpointAt = h.seq, &now
		}
		_, err = c.exec(ctx, `UPDATE audit_heads SET seq = ?, hash = ?, checkpoint_seq = ?, checkpoint_at = ?, updated_at = ? WHERE tenant_id = ?`,
			h.seq, h.hash, h.checkpointSeq, h.checkpointAt, now, tenantID)
		return err
	})
	return count, err
}

// updateChained stores seq, prev_hash and hash of chained entries, many
// rows per statement.
func updateChained(ctx context.Context, c conn, batch []model.AuditEntry) error {
	const per = 500
	for start := 0; start < len(batch); start += per {
		part := batch[start:min(start+per, len(batch))]
		var b strings.Builder
		b.WriteString(`WITH v(id, seq, prev_hash, hash) AS (VALUES `)
		args := make([]any, 0, 4*len(part))
		for i, e := range part {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(`(?, CAST(? AS BIGINT), ?, ?)`)
			args = append(args, e.ID, e.Seq, e.PrevHash, e.Hash)
		}
		b.WriteString(`) UPDATE audit_log SET seq = v.seq, prev_hash = v.prev_hash, hash = v.hash FROM v WHERE audit_log.id = v.id AND audit_log.seq IS NULL`)
		res, err := c.exec(ctx, b.String(), args...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != int64(len(part)) {
			return fmt.Errorf("audit chain: %d of %d entries chained (concurrent sealing)", n, len(part))
		}
	}
	return nil
}

// ---------------------------------------------------------------- checkpoint keys

func adAuditKey(id string) string { return "audit_keys.secret|" + id }

// auditKey returns the active checkpoint key, creating one when there is
// none.
func (s *Store) auditKey(ctx context.Context, c conn) (string, []byte, error) {
	var id, stored string
	err := c.row(ctx, `SELECT id, secret FROM audit_keys ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&id, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return "", nil, err
		}
		id = newID()
		enc, err := s.seal(hex.EncodeToString(key), adAuditKey(id))
		if err != nil {
			return "", nil, err
		}
		if _, err := c.exec(ctx, `INSERT INTO audit_keys (id, secret, created_at) VALUES (?, ?, ?)`, id, enc, s.now()); err != nil {
			return "", nil, err
		}
		s.sealer.mu.Lock()
		s.sealer.keys[id] = key
		s.sealer.mu.Unlock()
		return id, key, nil
	}
	if err != nil {
		return "", nil, err
	}
	key, err := s.decodeAuditKey(ctx, id, stored)
	return id, key, err
}

func (s *Store) decodeAuditKey(ctx context.Context, id, stored string) ([]byte, error) {
	s.sealer.mu.Lock()
	k, ok := s.sealer.keys[id]
	s.sealer.mu.Unlock()
	if ok {
		return k, nil
	}
	v, err := s.unseal(ctx, stored, adAuditKey(id))
	if err != nil {
		return nil, fmt.Errorf("audit checkpoint key %s: %w", id, err)
	}
	k, err = hex.DecodeString(v)
	if err != nil || len(k) != 32 {
		return nil, fmt.Errorf("audit checkpoint key %s is damaged", id)
	}
	s.sealer.mu.Lock()
	s.sealer.keys[id] = k
	s.sealer.mu.Unlock()
	return k, nil
}

func (s *Store) writeCheckpoint(ctx context.Context, c conn, tenantID string, seq int64, hash string, at time.Time) (*model.AuditCheckpoint, error) {
	kid, key, err := s.auditKey(ctx, c)
	if err != nil {
		return nil, err
	}
	cp := &model.AuditCheckpoint{ID: newID(), TenantID: tenantID, Seq: seq, Hash: hash, KeyID: kid, CreatedAt: at,
		Signature: signCheckpoint(key, tenantID, seq, hash, at)}
	_, err = c.exec(ctx, `INSERT INTO audit_checkpoints (id, tenant_id, seq, hash, key_id, signature, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cp.ID, cp.TenantID, cp.Seq, cp.Hash, cp.KeyID, cp.Signature, cp.CreatedAt)
	return cp, err
}

// ListAuditCheckpoints returns a tenant's checkpoints, oldest first.
func (s *Store) ListAuditCheckpoints(ctx context.Context, tenantID string) ([]model.AuditCheckpoint, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, tenant_id, seq, hash, key_id, signature, created_at FROM audit_checkpoints WHERE tenant_id = ? ORDER BY seq, created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AuditCheckpoint{}
	for rows.Next() {
		var cp model.AuditCheckpoint
		if err := rows.Scan(&cp.ID, &cp.TenantID, &cp.Seq, &cp.Hash, &cp.KeyID, &cp.Signature, &cp.CreatedAt); err != nil {
			return nil, err
		}
		cp.CreatedAt = cp.CreatedAt.UTC()
		out = append(out, cp)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- reading the chain

const auditChainCols = `id, tenant_id, user_id, user_email, action, entity_type, entity_id, details, reason, created_at, COALESCE(seq, 0), prev_hash, hash`

func scanChained(rows *sql.Rows) (model.AuditEntry, error) {
	var e model.AuditEntry
	err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.UserEmail, &e.Action, &e.EntityType, &e.EntityID, &e.Details, &e.Reason, &e.CreatedAt, &e.Seq, &e.PrevHash, &e.Hash)
	e.CreatedAt = e.CreatedAt.UTC()
	return e, err
}

// WalkAuditChain calls fn for every chained entry of a tenant in chain
// order, reading a page at a time (the connection is free between pages).
func (s *Store) WalkAuditChain(ctx context.Context, tenantID string, fn func(e *model.AuditEntry) error) error {
	const page = 5000
	var after int64
	for {
		rows, err := s.c(ctx).query(ctx, `SELECT `+auditChainCols+` FROM audit_log WHERE tenant_id = ? AND seq > ? ORDER BY seq LIMIT ?`, tenantID, after, page)
		if err != nil {
			return err
		}
		var list []model.AuditEntry
		for rows.Next() {
			e, err := scanChained(rows)
			if err != nil {
				rows.Close()
				return err
			}
			list = append(list, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range list {
			if err := fn(&list[i]); err != nil {
				return err
			}
		}
		if len(list) < page {
			return nil
		}
		after = list[len(list)-1].Seq
	}
}

// AuditHead returns the recorded end of a tenant's chain (0, "" for none).
func (s *Store) AuditHead(ctx context.Context, tenantID string) (int64, string, error) {
	var seq int64
	var hash string
	err := s.c(ctx).row(ctx, `SELECT seq, hash FROM audit_heads WHERE tenant_id = ?`, tenantID).Scan(&seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return seq, hash, err
}

// ---------------------------------------------------------------- verification

// ChainChecker verifies entries of one chain in order (also used for an
// exported audit log, without the database).
type ChainChecker struct {
	Entries  int64
	LastSeq  int64
	LastHash string
	Broken   *model.AuditBreak
	// hashes of the entries with a checkpoint (seq → hash)
	want map[int64]string
	got  map[int64]string
}

// NewChainChecker returns a checker; checkpointSeqs are the sequence
// numbers whose hashes Hashes later returns.
func NewChainChecker(checkpointSeqs []int64) *ChainChecker {
	c := &ChainChecker{want: map[int64]string{}, got: map[int64]string{}}
	for _, s := range checkpointSeqs {
		c.want[s] = ""
	}
	return c
}

func breakAt(e *model.AuditEntry, problem, msg string) *model.AuditBreak {
	t := e.CreatedAt
	return &model.AuditBreak{Seq: e.Seq, EntryID: e.ID, CreatedAt: &t, Problem: problem, Message: msg}
}

// Add checks the next entry; it reports false once the chain is broken.
func (c *ChainChecker) Add(e *model.AuditEntry) bool {
	if c.Broken != nil {
		return false
	}
	switch {
	case e.Seq != c.LastSeq+1:
		c.Broken = breakAt(e, model.AuditSeqGap, fmt.Sprintf("entry %d follows entry %d: %d entries are missing", e.Seq, c.LastSeq, e.Seq-c.LastSeq-1))
	case e.PrevHash != c.LastHash:
		c.Broken = breakAt(e, model.AuditLinkBroken, fmt.Sprintf("entry %d does not point to the hash of entry %d: an entry was removed, replaced or reordered", e.Seq, c.LastSeq))
	case AuditEntryHash(e) != e.Hash:
		c.Broken = breakAt(e, model.AuditEntryChanged, fmt.Sprintf("entry %d was changed after it was written: its fields no longer give its hash", e.Seq))
	}
	if c.Broken != nil {
		return false
	}
	c.Entries++
	c.LastSeq, c.LastHash = e.Seq, e.Hash
	if _, ok := c.want[e.Seq]; ok {
		c.got[e.Seq] = e.Hash
	}
	return true
}

// CheckCheckpoint compares a checkpoint (whose signature was checked) with
// the chain; it records and returns the problem, if any.
func (c *ChainChecker) CheckCheckpoint(cp *model.AuditCheckpoint) *model.AuditBreak {
	if c.Broken != nil && c.Broken.Seq <= cp.Seq {
		return c.Broken
	}
	var b *model.AuditBreak
	if cp.Seq > c.LastSeq {
		at := cp.CreatedAt
		b = &model.AuditBreak{Seq: c.LastSeq + 1, CreatedAt: &at, Problem: model.AuditTruncated,
			Message: fmt.Sprintf("a checkpoint signed at %s covers entry %d, but the chain ends at entry %d: entries were removed", auditTime(cp.CreatedAt), cp.Seq, c.LastSeq)}
	} else if h := c.got[cp.Seq]; h != cp.Hash {
		at := cp.CreatedAt
		b = &model.AuditBreak{Seq: cp.Seq, CreatedAt: &at, Problem: model.AuditCheckpointMismatch,
			Message: fmt.Sprintf("the checkpoint signed at %s names another hash for entry %d: the chain was rewritten", auditTime(cp.CreatedAt), cp.Seq)}
	}
	if b != nil && (c.Broken == nil || b.Seq < c.Broken.Seq) {
		c.Broken = b
	}
	return b
}

// VerifyAudit chains pending entries, writes a checkpoint at the end of the
// chain and verifies the whole chain of a tenant: every entry's hash, the
// links, the sequence, the recorded head and every signed checkpoint. The
// result names the first broken link.
func (s *Store) VerifyAudit(ctx context.Context, tenantID string) (*model.AuditVerification, error) {
	ctx = withoutTx(ctx)
	start := time.Now()
	if err := s.sealAll(ctx, tenantID, true); err != nil {
		return nil, err
	}
	cps, err := s.ListAuditCheckpoints(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	seqs := make([]int64, len(cps))
	for i, cp := range cps {
		seqs[i] = cp.Seq
	}
	chk := NewChainChecker(seqs)
	headSeq, headHash, err := s.AuditHead(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	err = s.WalkAuditChain(ctx, tenantID, func(e *model.AuditEntry) error {
		if !chk.Add(e) {
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return nil, err
	}
	v := &model.AuditVerification{TenantID: tenantID, KeyProtected: s.keys != nil}
	if chk.Broken == nil && (headSeq != chk.LastSeq || headHash != chk.LastHash) {
		chk.Broken = &model.AuditBreak{Seq: chk.LastSeq + 1, Problem: model.AuditTruncated,
			Message: fmt.Sprintf("the chain ends at entry %d but its recorded end is entry %d: entries at the end were removed or the record was changed", chk.LastSeq, headSeq)}
	}
	// checkpoints: the signature, then the hash they name
	keys := map[string]string{}
	krows, err := s.c(ctx).query(ctx, `SELECT id, secret FROM audit_keys`)
	if err != nil {
		return nil, err
	}
	for krows.Next() {
		var id, sec string
		if err := krows.Scan(&id, &sec); err != nil {
			krows.Close()
			return nil, err
		}
		keys[id] = sec
	}
	krows.Close()
	for i := range cps {
		cp := &cps[i]
		stored, ok := keys[cp.KeyID]
		valid := false
		if ok {
			if key, err := s.decodeAuditKey(ctx, cp.KeyID, stored); err == nil {
				valid = hmac.Equal([]byte(signCheckpoint(key, tenantID, cp.Seq, cp.Hash, cp.CreatedAt)), []byte(cp.Signature))
			}
		}
		if !valid {
			at := cp.CreatedAt
			b := &model.AuditBreak{Seq: cp.Seq, CreatedAt: &at, Problem: model.AuditCheckpointForged,
				Message: fmt.Sprintf("the checkpoint of entry %d (%s) has no valid signature: it was not written by this server", cp.Seq, auditTime(cp.CreatedAt))}
			if chk.Broken == nil || b.Seq < chk.Broken.Seq {
				chk.Broken = b
			}
			continue
		}
		chk.CheckCheckpoint(cp)
	}
	var unchained int
	if err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM audit_log WHERE tenant_id = ? AND seq IS NULL`, tenantID).Scan(&unchained); err != nil {
		return nil, err
	}
	v.Entries, v.LastSeq, v.LastHash, v.Broken = chk.Entries, chk.LastSeq, chk.LastHash, chk.Broken
	v.OK = v.Broken == nil
	v.Checkpoints, v.Unchained = len(cps), unchained
	if len(cps) > 0 {
		v.LastCheckpoint = &cps[len(cps)-1]
	}
	v.VerifiedAt = s.now()
	v.DurationMS = time.Since(start).Milliseconds()
	return v, nil
}

var errStopWalk = errors.New("stop")

// CheckpointAudit chains pending entries of a tenant and writes a signed
// checkpoint at the end of its chain (before an export).
func (s *Store) CheckpointAudit(ctx context.Context, tenantID string) error {
	return s.sealAll(withoutTx(ctx), tenantID, true)
}

// AuditArchive is the JSON export of a tenant's audit chain for external
// archiving (see the administrator guide, "Audit trail").
type AuditArchive struct {
	Format      string                  `json:"format"`
	Chain       string                  `json:"chain"`
	TenantID    string                  `json:"tenant_id"`
	Tenant      string                  `json:"tenant"`
	ExportedAt  time.Time               `json:"exported_at"`
	HeadSeq     int64                   `json:"head_seq"`
	HeadHash    string                  `json:"head_hash"`
	Checkpoints []model.AuditCheckpoint `json:"checkpoints"`
	Entries     []model.AuditEntry      `json:"entries"`
}

// AuditArchiveFormat names the archive layout.
const AuditArchiveFormat = "qtestframe-audit-archive-v1"

// VerifyAuditArchive checks an exported chain without the database: the
// hashes, links and sequence of its entries, its end, and that its
// checkpoints name the hashes of their entries (their signatures need the
// server key; VerifyAudit checks them).
func VerifyAuditArchive(a *AuditArchive) *model.AuditVerification {
	seqs := make([]int64, len(a.Checkpoints))
	for i, cp := range a.Checkpoints {
		seqs[i] = cp.Seq
	}
	c := NewChainChecker(seqs)
	for i := range a.Entries {
		a.Entries[i].TenantID = a.TenantID
		if !c.Add(&a.Entries[i]) {
			break
		}
	}
	if c.Broken == nil && (a.HeadSeq != c.LastSeq || a.HeadHash != c.LastHash) {
		c.Broken = &model.AuditBreak{Seq: c.LastSeq + 1, Problem: model.AuditTruncated,
			Message: fmt.Sprintf("the export ends at entry %d but names entry %d as the end of the chain", c.LastSeq, a.HeadSeq)}
	}
	for i := range a.Checkpoints {
		c.CheckCheckpoint(&a.Checkpoints[i])
	}
	v := &model.AuditVerification{TenantID: a.TenantID, Entries: c.Entries, LastSeq: c.LastSeq, LastHash: c.LastHash, Broken: c.Broken,
		OK: c.Broken == nil, Checkpoints: len(a.Checkpoints)}
	if len(a.Checkpoints) > 0 {
		v.LastCheckpoint = &a.Checkpoints[len(a.Checkpoints)-1]
	}
	return v
}
