package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/secrets"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil"
)

// A backup restores the chain as it was (also into the other database);
// the restore is the next entry of the chain.
func TestAuditChainBackupRestore(t *testing.T) {
	ctx := context.Background()
	p := secrets.NewEphemeral()
	src := sqliteStore(t)
	must(t, src.SetupSecrets(ctx, p))
	ten, err := src.CreateTenant(ctx, "acme", "Acme")
	must(t, err)
	auditN(t, src, ctx, ten.ID, 20)
	verifyOK(t, src, ctx, ten.ID)
	b, _ := backup(t, src)
	dst := store.New(testutil.OpenDB(t))
	must(t, dst.SetupSecrets(ctx, p))
	if _, err := restore(dst, b); err != nil {
		t.Fatal(err)
	}
	must(t, dst.SetupSecrets(ctx, p))
	auditN(t, dst, ctx, ten.ID, 1)
	v := verifyOK(t, dst, ctx, ten.ID)
	if v.LastSeq != 21 || v.Checkpoints < 2 {
		t.Fatalf("%+v", v)
	}
}

func auditN(t *testing.T, st *store.Store, ctx context.Context, tenant string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		must(t, st.Audit(ctx, model.AuditEntry{TenantID: tenant, UserEmail: "a@example.com", Action: "x.update", EntityType: "x", EntityID: fmt.Sprint(i), Details: fmt.Sprintf("change %d", i)}))
	}
}

func verifyOK(t *testing.T, st *store.Store, ctx context.Context, tenant string) *model.AuditVerification {
	t.Helper()
	v, err := st.VerifyAudit(ctx, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK {
		t.Fatalf("chain broken: %+v", v.Broken)
	}
	return v
}

func verifyBroken(t *testing.T, st *store.Store, ctx context.Context, tenant, problem string, seq int64) {
	t.Helper()
	v, err := st.VerifyAudit(ctx, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if v.OK || v.Broken == nil || v.Broken.Problem != problem || v.Broken.Seq != seq {
		t.Fatalf("want %s at %d, got %+v", problem, seq, v.Broken)
	}
}

func TestAuditChain(t *testing.T) {
	st, ten, ctx := setup(t)
	must(t, st.SetupSecrets(ctx, secrets.NewEphemeral()))
	auditN(t, st, ctx, ten.ID, 10)
	list, err := st.ListAudit(ctx, ten.ID, 100)
	must(t, err)
	if len(list) != 10 {
		t.Fatal(len(list))
	}
	for _, e := range list {
		if e.Seq == 0 || len(e.Hash) != 64 {
			t.Fatalf("not chained: %+v", e)
		}
	}
	v := verifyOK(t, st, ctx, ten.ID)
	if v.Entries != 10 || v.LastSeq != 10 || v.Checkpoints < 1 || !v.KeyProtected || v.LastCheckpoint.Seq != 10 {
		t.Fatalf("%+v", v)
	}
	// a rolled back transaction leaves nothing in the chain
	_ = st.InTx(ctx, func(ctx context.Context) error {
		must(t, st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, Action: "x.rolled_back", EntityType: "x"}))
		return errors.New("no")
	})
	// one committed in a transaction is chained after the commit, with the reason of ctx
	must(t, st.InTx(store.WithChangeReason(ctx, "CAPA-12"), func(ctx context.Context) error {
		return st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, Action: "x.in_tx", EntityType: "x"})
	}))
	v = verifyOK(t, st, ctx, ten.ID)
	if v.LastSeq != 11 {
		t.Fatal(v.LastSeq)
	}
	list, _ = st.ListAudit(ctx, ten.ID, 1)
	if list[0].Action != "x.in_tx" || list[0].Reason != "CAPA-12" || list[0].Seq != 11 {
		t.Fatalf("%+v", list[0])
	}
	// other tenants have their own chains
	other, err := st.CreateTenant(ctx, "other", "Other")
	must(t, err)
	auditN(t, st, ctx, other.ID, 3)
	if v := verifyOK(t, st, ctx, other.ID); v.LastSeq != 3 {
		t.Fatal(v.LastSeq)
	}
	// system entries (tenant "") too
	auditN(t, st, ctx, "", 2)
	verifyOK(t, st, ctx, "")

	// an entry changed in the database
	_, err = st.DB.ExecContext(ctx, rebind(st, `UPDATE audit_log SET details = 'nothing to see' WHERE tenant_id = ? AND seq = 4`), ten.ID)
	must(t, err)
	verifyBroken(t, st, ctx, ten.ID, model.AuditEntryChanged, 4)
}

func rebind(st *store.Store, q string) string {
	if st.DB.Dialect != "postgres" {
		return q
	}
	n := 0
	var b strings.Builder
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestAuditChainTampering(t *testing.T) {
	exec := func(t *testing.T, st *store.Store, q string, args ...any) {
		t.Helper()
		if _, err := st.DB.ExecContext(context.Background(), rebind(st, q), args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("removed entry", func(t *testing.T) {
		st, ten, ctx := setup(t)
		auditN(t, st, ctx, ten.ID, 6)
		exec(t, st, `DELETE FROM audit_log WHERE tenant_id = ? AND seq = 3`, ten.ID)
		verifyBroken(t, st, ctx, ten.ID, model.AuditSeqGap, 4)
	})
	t.Run("renumbered after removal", func(t *testing.T) {
		st, ten, ctx := setup(t)
		auditN(t, st, ctx, ten.ID, 6)
		exec(t, st, `DELETE FROM audit_log WHERE tenant_id = ? AND seq = 3`, ten.ID)
		for s := 4; s <= 6; s++ {
			exec(t, st, `UPDATE audit_log SET seq = ? WHERE tenant_id = ? AND seq = ?`, s-1, ten.ID, s)
		}
		verifyBroken(t, st, ctx, ten.ID, model.AuditLinkBroken, 3)
	})
	t.Run("truncated end with head rewritten", func(t *testing.T) {
		st, ten, ctx := setup(t)
		auditN(t, st, ctx, ten.ID, 5)
		verifyOK(t, st, ctx, ten.ID) // checkpoint at 5
		var h4 string
		must(t, st.DB.QueryRowContext(ctx, rebind(st, `SELECT hash FROM audit_log WHERE tenant_id = ? AND seq = 4`), ten.ID).Scan(&h4))
		exec(t, st, `DELETE FROM audit_log WHERE tenant_id = ? AND seq = 5`, ten.ID)
		exec(t, st, `UPDATE audit_heads SET seq = 4, hash = ? WHERE tenant_id = ?`, h4, ten.ID)
		verifyBroken(t, st, ctx, ten.ID, model.AuditTruncated, 5)
	})
	t.Run("truncated end", func(t *testing.T) {
		st, ten, ctx := setup(t)
		auditN(t, st, ctx, ten.ID, 5)
		exec(t, st, `DELETE FROM audit_log WHERE tenant_id = ? AND seq = 5`, ten.ID)
		verifyBroken(t, st, ctx, ten.ID, model.AuditTruncated, 5)
	})
	t.Run("rewritten chain", func(t *testing.T) {
		// someone recomputes every hash after a change: the signed checkpoint tells
		st, ten, ctx := setup(t)
		auditN(t, st, ctx, ten.ID, 4)
		verifyOK(t, st, ctx, ten.ID)
		exec(t, st, `UPDATE audit_log SET details = 'rewritten' WHERE tenant_id = ? AND seq = 2`, ten.ID)
		prev := ""
		var last string
		for s := int64(1); s <= 4; s++ {
			rows, err := st.DB.QueryContext(ctx, rebind(st, `SELECT id, user_id, user_email, action, entity_type, entity_id, details, reason, created_at FROM audit_log WHERE tenant_id = ? AND seq = ?`), ten.ID, s)
			must(t, err)
			e := model.AuditEntry{TenantID: ten.ID, Seq: s, PrevHash: prev}
			rows.Next()
			must(t, rows.Scan(&e.ID, &e.UserID, &e.UserEmail, &e.Action, &e.EntityType, &e.EntityID, &e.Details, &e.Reason, &e.CreatedAt))
			rows.Close()
			h := store.AuditEntryHash(&e)
			exec(t, st, `UPDATE audit_log SET prev_hash = ?, hash = ? WHERE id = ?`, prev, h, e.ID)
			prev, last = h, h
		}
		exec(t, st, `UPDATE audit_heads SET hash = ? WHERE tenant_id = ?`, last, ten.ID)
		verifyBroken(t, st, ctx, ten.ID, model.AuditCheckpointMismatch, 4)
		// and a forged checkpoint is no help
		exec(t, st, `UPDATE audit_checkpoints SET hash = ? WHERE tenant_id = ? AND seq = 4`, last, ten.ID)
		verifyBroken(t, st, ctx, ten.ID, model.AuditCheckpointForged, 4)
	})
}

func TestAuditChainBackfill(t *testing.T) {
	st, ten, ctx := setup(t)
	// entries written before the upgrade: no chain, the tenant marked for backfill
	for i := 0; i < 3; i++ {
		_, err := st.DB.ExecContext(ctx, rebind(st, `INSERT INTO audit_log (id, tenant_id, user_id, user_email, action, entity_type, entity_id, details, created_at) VALUES (?, ?, '', 'old@example.com', 'old.action', 'x', '', '', ?)`),
			fmt.Sprintf("old-%d", i), ten.ID, time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC))
		must(t, err)
	}
	_, err := st.DB.ExecContext(ctx, rebind(st, `INSERT INTO audit_backfill (tenant_id) VALUES (?)`), ten.ID)
	must(t, err)
	must(t, st.SealAllAudit(ctx))
	v := verifyOK(t, st, ctx, ten.ID)
	if v.LastSeq != 4 {
		t.Fatal(v.LastSeq)
	}
	var first []model.AuditEntry
	must(t, st.WalkAuditChain(ctx, ten.ID, func(e *model.AuditEntry) error { first = append(first, *e); return nil }))
	if first[0].Action != "audit.chain_start" || !strings.Contains(first[0].Details, "3 entries") || first[1].ID != "old-0" || first[3].ID != "old-2" {
		t.Fatalf("%+v", first)
	}
	// a new tenant starts without a genesis entry
	n, err := st.CreateTenant(ctx, "new", "New")
	must(t, err)
	auditN(t, st, ctx, n.ID, 1)
	var got []model.AuditEntry
	must(t, st.WalkAuditChain(ctx, n.ID, func(e *model.AuditEntry) error { got = append(got, *e); return nil }))
	if len(got) != 1 || got[0].Action != "x.update" || got[0].PrevHash != "" {
		t.Fatalf("%+v", got)
	}
}

func TestAuditChainConcurrent(t *testing.T) {
	st, ten, ctx := setup(t)
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				err := st.InTx(ctx, func(ctx context.Context) error {
					return st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, Action: "x.parallel", EntityType: "x", Details: fmt.Sprint(g, i)})
				})
				if err != nil {
					errs <- err
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	// a second store over the same database (another server process)
	st2 := store.New(st.DB)
	auditN(t, st2, ctx, ten.ID, 5)
	v := verifyOK(t, st, ctx, ten.ID)
	if v.Entries != 205 {
		t.Fatal(v.Entries)
	}
}

func TestChainCheckerExport(t *testing.T) {
	st, ten, ctx := setup(t)
	auditN(t, st, ctx, ten.ID, 3)
	var es []model.AuditEntry
	must(t, st.WalkAuditChain(ctx, ten.ID, func(e *model.AuditEntry) error { es = append(es, *e); return nil }))
	c := store.NewChainChecker([]int64{3})
	for i := range es {
		if !c.Add(&es[i]) {
			t.Fatal(c.Broken)
		}
	}
	if b := c.CheckCheckpoint(&model.AuditCheckpoint{Seq: 3, Hash: es[2].Hash}); b != nil {
		t.Fatal(b)
	}
	if b := c.CheckCheckpoint(&model.AuditCheckpoint{Seq: 9, Hash: "x"}); b == nil || b.Problem != model.AuditTruncated {
		t.Fatal(b)
	}
	es[1].Details = "changed"
	c = store.NewChainChecker(nil)
	c.Add(&es[0])
	if c.Add(&es[1]) || c.Broken.Problem != model.AuditEntryChanged {
		t.Fatal(c.Broken)
	}
}

// TestAuditChainPerformance chains and verifies 100 000 entries (set
// QTF_AUDIT_PERF_N to change the number; -short uses 5 000).
func TestAuditChainPerformance(t *testing.T) {
	n := 100000
	if testing.Short() {
		n = 5000
	}
	if v := os.Getenv("QTF_AUDIT_PERF_N"); v != "" {
		fmt.Sscan(v, &n)
	}
	st, ten, ctx := setup(t)
	// written as before the upgrade (unchained), in bulk
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const per = 500
	for i := 0; i < n; i += per {
		var b strings.Builder
		b.WriteString(`INSERT INTO audit_log (id, tenant_id, user_id, user_email, action, entity_type, entity_id, details, created_at) VALUES `)
		var args []any
		for j := i; j < min(i+per, n); j++ {
			if j > i {
				b.WriteString(", ")
			}
			b.WriteString("(?, ?, 'u', 'perf@example.com', 'requirement.update', 'requirement', ?, 'title changed', ?)")
			args = append(args, fmt.Sprintf("perf-%07d", j), ten.ID, fmt.Sprint(j), base.Add(time.Duration(j)*time.Millisecond))
		}
		_, err := st.DB.ExecContext(ctx, rebind(st, b.String()), args...)
		must(t, err)
	}
	start := time.Now()
	must(t, st.SealAllAudit(ctx))
	sealed := time.Since(start)
	start = time.Now()
	v := verifyOK(t, st, ctx, ten.ID)
	verified := time.Since(start)
	if v.Entries != int64(n) {
		t.Fatal(v.Entries)
	}
	// appends after a long chain stay fast
	start = time.Now()
	auditN(t, st, ctx, ten.ID, 100)
	appended := time.Since(start)
	t.Logf("%s: %d entries chained in %s (%.0f/s), verified in %s (%.0f/s); 100 appends in %s; %d checkpoints",
		st.DB.Dialect, n, sealed, float64(n)/sealed.Seconds(), verified, float64(n)/verified.Seconds(), appended, v.Checkpoints)
}
