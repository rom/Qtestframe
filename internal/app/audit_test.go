package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestAuditVerifyCommand(t *testing.T) {
	ctx := context.Background()
	cfg := writeConfig(t, "")
	st := openTestStore(t, cfg)
	ten, err := st.CreateTenant(ctx, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, Action: "x.update", EntityType: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Audit(ctx, model.AuditEntry{UserEmail: "cli", Action: "admin.backup", EntityType: "system"})
	// an export for the offline check
	var arc store.AuditArchive
	arc.Format, arc.Chain, arc.TenantID = store.AuditArchiveFormat, store.AuditChainVersion, ten.ID
	if err := st.CheckpointAudit(ctx, ten.ID); err != nil {
		t.Fatal(err)
	}
	arc.HeadSeq, arc.HeadHash, _ = st.AuditHead(ctx, ten.ID)
	arc.Checkpoints, _ = st.ListAuditCheckpoints(ctx, ten.ID)
	_ = st.WalkAuditChain(ctx, ten.ID, func(e *model.AuditEntry) error { arc.Entries = append(arc.Entries, *e); return nil })
	st.DB.Close()

	r := run(t, "", "audit", "verify", "--config", cfg)
	if r.code != 0 || !strings.Contains(r.stdout, "acme: OK, 5 entries") || !strings.Contains(r.stdout, "(system): OK, 1 entries") {
		t.Fatal(r)
	}
	r = run(t, "", "audit", "verify", "--config", cfg, "--tenant", "acme", "--json")
	var out []map[string]any
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &out) != nil || len(out) != 1 || out[0]["ok"] != true {
		t.Fatal(r)
	}
	if r := run(t, "", "audit", "verify", "--config", cfg, "--tenant", "nope"); r.code != 1 {
		t.Fatal(r)
	}
	if r := run(t, "", "audit"); r.code != 1 || !strings.Contains(r.stderr, "usage: qtestframe audit verify") {
		t.Fatal(r)
	}

	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	b, _ := json.Marshal(arc)
	os.WriteFile(good, b, 0o600)
	if r := run(t, "", "audit", "verify", "--file", good); r.code != 0 || !strings.Contains(r.stdout, "OK, 5 entries") {
		t.Fatal(r)
	}
	arc.Entries = arc.Entries[:4] // truncated
	b, _ = json.Marshal(arc)
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, b, 0o600)
	if r := run(t, "", "audit", "verify", "--file", bad); r.code != 1 || !strings.Contains(r.stdout, "BROKEN at entry 5 (truncated)") {
		t.Fatal(r)
	}
	os.WriteFile(bad, []byte(`{"format":"other"}`), 0o600)
	if r := run(t, "", "audit", "verify", "--file", bad); r.code != 1 || !strings.Contains(r.stderr, "not an audit trail export") {
		t.Fatal(r)
	}
	if r := run(t, "", "audit", "verify", "--file", filepath.Join(dir, "none.json")); r.code != 1 {
		t.Fatal(r)
	}
}
