package store_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil"
)

func saveProject(t *testing.T, st *store.Store, tenantID, pid string) ([]byte, *store.ProjectArchiveManifest) {
	t.Helper()
	var buf bytes.Buffer
	m, err := st.ExportProject(context.Background(), tenantID, pid, &buf, "test-1.0")
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), m
}

func loadProject(st *store.Store, tenantID string, b []byte, opt store.ProjectImport) (*store.ProjectImportResult, error) {
	return st.ImportProject(context.Background(), tenantID, bytes.NewReader(b), int64(len(b)), opt)
}

// archiveIDs returns every "id" value of the rows of an archive.
func archiveIDs(t *testing.T, b []byte) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	for _, lines := range dump(t, b) {
		for _, l := range lines {
			if i := strings.Index(l, `"id":"`); i >= 0 {
				ids[l[i+6:i+6+36]] = true
			}
		}
	}
	return ids
}

func count(t *testing.T, st *store.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := st.DB.QueryRowContext(context.Background(), st.DB.Rebind(q), args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

// archiveFixture is a seeded project with secrets, a review request, an
// item link and a risk link on top of the demo data.
type archiveFixture struct {
	st                *store.Store
	ten, ten2         *model.Tenant
	project           *model.Project
	reviewer          *model.User
	secret, token     string
	attachmentsBefore int
}

func newArchiveFixture(t *testing.T, st *store.Store) archiveFixture {
	t.Helper()
	ctx := context.Background()
	sd := seed(t, st)
	ten, err := st.GetTenantBySlug(ctx, "acme")
	must(t, err)
	ten2, err := st.GetTenantBySlug(ctx, "other")
	must(t, err)
	ps, err := st.ListProjects(ctx, ten.ID)
	must(t, err)
	p := &ps[0]
	reqs, err := st.ListRequirements(ctx, ten.ID, p.ID)
	must(t, err)
	tests, err := st.ListTests(ctx, ten.ID, p.ID)
	must(t, err)
	f := archiveFixture{st: st, ten: ten, ten2: ten2, project: p, reviewer: sd.super, secret: "whsec-very-secret-value", token: "tracker-token-xyz"}
	must(t, st.CreateWebhook(ctx, ten.ID, &model.Webhook{ProjectID: p.ID, Name: "hook", URL: "https://example.com/hook", Secret: f.secret, Events: []string{"requirement.created"}, Enabled: true}))
	must(t, st.SaveDefectTracker(ctx, ten.ID, p.ID, &model.DefectTracker{Type: "github", Repo: "acme/app", Token: f.token}, false))
	must(t, st.SaveImportSources(ctx, ten.ID, p.ID, &model.ImportSources{Azure: model.AzureDevOpsSource{URL: "https://dev.azure.com/acme", Project: "Shop", Token: f.token}}, store.ClearTokens{}))
	_, err = st.CreateReviewRequests(ctx, ten.ID, model.ReviewRequest{EntityType: model.KindRequirement, EntityID: reqs[0].ID, RequestedBy: "x"}, []string{sd.super.ID})
	must(t, err)
	tid := tests[0].ID
	must(t, st.CreateItemLink(ctx, ten.ID, &model.ItemLink{ProjectID: p.ID, SourceType: model.KindRequirement, SourceID: reqs[1].ID, TargetType: model.KindTest, TargetID: &tid, Label: "see"}))
	// a description that names another item by id and key
	reqs[2].Description = "Depends on " + p.Key + "-REQ-1 (/requirements/" + reqs[0].ID + ")"
	must(t, st.UpdateRequirement(ctx, ten.ID, &reqs[2], false))
	must(t, st.AddRevision(ctx, ten.ID, model.KindRequirement, reqs[2].ID, 2, "update", "x", []byte(`{"key":"`+reqs[2].Key+`","id":"`+reqs[2].ID+`"}`)))
	must(t, st.CreateRisk(ctx, ten.ID, &model.Risk{ProjectID: p.ID, Title: "Outage", Likelihood: 3, Impact: 4, RequirementIDs: []string{reqs[0].ID}}))
	// an exploratory session with its scope and a note
	x := &model.ExploratorySession{ProjectID: p.ID, Title: "Explore login", Tester: "t@example.com",
		Scope: []model.ExploratoryScopeItem{{Type: model.KindRequirement, ID: reqs[0].ID}, {Type: model.KindTest, ID: tid}}}
	must(t, st.CreateExploratorySession(ctx, ten.ID, x))
	_, err = st.ExploratoryTimer(ctx, ten.ID, x.ID, store.TimerStart)
	must(t, err)
	must(t, st.AddExploratoryNote(ctx, ten.ID, &model.ExploratoryNote{SessionID: x.ID, Type: "idea", Body: "Try a locked account"}))
	f.attachmentsBefore = count(t, st, `SELECT COUNT(*) FROM attachments`)
	return f
}

func TestProjectArchiveTablesCovered(t *testing.T) {
	archived := store.ProjectArchiveTableNames()
	for _, tbl := range store.BackupTables {
		_, excluded := store.ProjectArchiveExcluded[tbl]
		in := slices.Contains(archived, tbl)
		if in == excluded {
			t.Errorf("table %s: archived %v, excluded %v (exactly one must hold)", tbl, in, excluded)
		}
	}
	for _, tbl := range archived {
		if !slices.Contains(store.BackupTables, tbl) {
			t.Errorf("archived table %s is not a backup table", tbl)
		}
	}
}

func TestProjectArchiveRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := store.New(testutil.OpenDB(t))
	fx := newArchiveFixture(t, st)
	p := fx.project
	orig, m := saveProject(t, st, fx.ten.ID, p.ID)
	if m.Kind != store.ProjectArchiveKind || m.FormatVersion != 1 || m.Project.Key != p.Key || m.Project.ID != p.ID || m.SchemaVersion == 0 {
		t.Fatalf("%+v", m)
	}
	for _, tbl := range []string{"requirements", "test_cases", "test_steps", "trace_links", "test_runs", "test_executions", "step_results",
		"revisions", "requirement_relations", "baselines", "comments", "files", "attachments", "defects", "risks", "releases",
		"configurations", "trace_rules", "webhooks", "review_requests", "item_links", "folders", "categories",
		"exploratory_sessions", "exploratory_scope", "exploratory_notes"} {
		if m.Tables[tbl] == 0 {
			t.Errorf("archive has no %s", tbl)
		}
	}
	if m.Files == 0 || m.Files != m.Tables["files"] {
		t.Fatalf("files %d, rows %d", m.Files, m.Tables["files"])
	}
	// no secrets, no other project's rows
	for name, data := range archiveEntries(t, orig) {
		// the sign-in session's id, as a JSON value ("exploratory_sessions"
		// and its session_id columns contain the letters)
		for _, s := range []string{fx.secret, fx.token, "hash1", `"sess"`} {
			if bytes.Contains(data, []byte(s)) && !(s == `"sess"` && strings.HasPrefix(name, "files/")) {
				t.Errorf("%s contains %q", name, s)
			}
		}
	}
	if n := len(dump(t, orig)["tables/projects"]); n != 1 {
		t.Fatal("projects", n)
	}

	// dry run: nothing is saved
	dry, err := loadProject(st, fx.ten.ID, orig, store.ProjectImport{Key: "COPY", DryRun: true})
	must(t, err)
	if !dry.DryRun || dry.Imported["requirements"] != m.Tables["requirements"] || dry.Project.Key != "COPY" || dry.Project.Name != p.Name {
		t.Fatalf("%+v", dry)
	}
	if ps, _ := st.ListProjects(ctx, fx.ten.ID); len(ps) != 1 {
		t.Fatal("dry run saved a project")
	}

	res, err := loadProject(st, fx.ten.ID, orig, store.ProjectImport{Key: "copy", Name: "Copy of demo"})
	must(t, err)
	np := res.Project
	if np.Key != "COPY" || np.Name != "Copy of demo" || np.ID == p.ID || np.TenantID != fx.ten.ID {
		t.Fatalf("%+v", np)
	}
	if len(res.Skipped) != 0 || res.Files != m.Files {
		t.Fatalf("skipped %v, files %d", res.Skipped, res.Files)
	}
	for tbl, n := range m.Tables {
		if res.Imported[tbl] != n {
			t.Errorf("%s: imported %d of %d", tbl, res.Imported[tbl], n)
		}
	}
	// the copy saves to an archive with the same counts and no shared ids
	cp, cm := saveProject(t, st, fx.ten.ID, np.ID)
	for tbl, n := range m.Tables {
		if cm.Tables[tbl] != n {
			t.Errorf("copy %s: %d rows, original %d", tbl, cm.Tables[tbl], n)
		}
	}
	if cm.Files != m.Files || cm.FileBytes != m.FileBytes {
		t.Fatalf("files %d/%d bytes %d/%d", cm.Files, m.Files, cm.FileBytes, m.FileBytes)
	}
	origIDs := archiveIDs(t, orig)
	for id := range archiveIDs(t, cp) {
		if origIDs[id] {
			t.Fatalf("id %s is shared by original and copy", id)
		}
	}
	// references point into the copy
	for _, q := range []string{
		`SELECT COUNT(*) FROM trace_links l JOIN requirements r ON r.id = l.requirement_id JOIN test_cases c ON c.id = l.test_case_id WHERE l.project_id = ? AND (r.project_id <> l.project_id OR c.project_id <> l.project_id)`,
		`SELECT COUNT(*) FROM requirement_relations x JOIN requirements a ON a.id = x.source_id JOIN requirements b ON b.id = x.target_id WHERE x.project_id = ? AND (a.project_id <> x.project_id OR b.project_id <> x.project_id)`,
		`SELECT COUNT(*) FROM test_executions e JOIN test_runs r ON r.id = e.run_id JOIN test_cases c ON c.id = e.test_case_id WHERE r.project_id = ? AND c.project_id <> r.project_id`,
		`SELECT COUNT(*) FROM requirements r JOIN folders f ON f.id = r.folder_id WHERE r.project_id = ? AND f.project_id <> r.project_id`,
		`SELECT COUNT(*) FROM test_runs r JOIN releases x ON x.id = r.release_id WHERE r.project_id = ? AND x.project_id <> r.project_id`,
		`SELECT COUNT(*) FROM item_links l JOIN test_cases c ON c.id = l.target_id WHERE l.project_id = ? AND c.project_id <> l.project_id`,
		`SELECT COUNT(*) FROM review_requests x JOIN requirements r ON r.id = x.entity_id WHERE x.project_id = ? AND r.project_id <> x.project_id`,
		`SELECT COUNT(*) FROM requirements WHERE project_id = ? AND key NOT LIKE 'COPY-REQ-%'`,
		`SELECT COUNT(*) FROM test_cases WHERE project_id = ? AND key NOT LIKE 'COPY-TC-%'`,
	} {
		if n := count(t, st, q, np.ID); n != 0 {
			t.Errorf("%d bad rows: %s", n, q)
		}
	}
	// attachments of the copy are on items of the copy and have content
	if n := count(t, st, `SELECT COUNT(*) FROM attachments a JOIN files f ON f.id = a.file_id JOIN requirements r ON r.id = a.entity_id WHERE f.project_id = ? AND r.project_id = ?`, np.ID, np.ID); n == 0 {
		t.Fatal("no attachment on a requirement of the copy")
	}
	if n := count(t, st, `SELECT COUNT(*) FROM files f JOIN file_blobs b ON b.file_id = f.id WHERE f.project_id = ?`, np.ID); n != m.Files {
		t.Fatal("blobs", n)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM attachments`); n != 2*fx.attachmentsBefore {
		t.Fatalf("attachments %d, before %d", n, fx.attachmentsBefore)
	}
	// texts: ids and keys follow
	reqs, err := st.ListRequirements(ctx, fx.ten.ID, np.ID)
	must(t, err)
	var described string
	for _, r := range reqs {
		if strings.HasPrefix(r.Description, "Depends on") {
			described = r.Description
		}
	}
	if !strings.Contains(described, "COPY-REQ-1 (/requirements/"+reqs[0].ID+")") && !strings.Contains(described, "COPY-REQ-1") {
		t.Fatalf("description %q", described)
	}
	if strings.Contains(described, p.Key+"-REQ-1") {
		t.Fatalf("description keeps the old key: %q", described)
	}
	// secrets stay out; the webhook is off
	hooks, err := st.ListWebhooks(ctx, fx.ten.ID, np.ID)
	must(t, err)
	if len(hooks) != 1 || hooks[0].Enabled || hooks[0].HasSecret {
		t.Fatalf("%+v", hooks)
	}
	tr, err := st.DefectTracker(ctx, fx.ten.ID, np.ID)
	must(t, err)
	is, err := st.ImportSources(ctx, fx.ten.ID, np.ID)
	must(t, err)
	if is.Azure.Token != "" || is.Azure.Project != "Shop" {
		t.Fatalf("%+v", is)
	}
	if tr.Token != "" || tr.Repo != "acme/app" {
		t.Fatalf("%+v", tr)
	}
	// new items get the next free key
	r := &model.Requirement{ProjectID: np.ID, Title: "After the load"}
	must(t, st.CreateRequirement(ctx, fx.ten.ID, r))
	if !strings.HasPrefix(r.Key, "COPY-REQ-") {
		t.Fatal(r.Key)
	}
	for _, x := range reqs {
		if x.Key == r.Key {
			t.Fatalf("key %s after %s", r.Key, x.Key)
		}
	}

	// the original is untouched
	again, _ := saveProject(t, st, fx.ten.ID, p.ID)
	a, b := dump(t, orig), dump(t, again)
	for tbl := range a {
		if !slices.Equal(a[tbl], b[tbl]) {
			t.Errorf("original %s changed", tbl)
		}
	}

	// the key is taken now: a dry run says so, a load fails
	if dry, err := loadProject(st, fx.ten.ID, orig, store.ProjectImport{Key: "COPY", DryRun: true}); err != nil || !dry.KeyTaken || dry.Project.Key != "COPY" {
		t.Fatal(dry, err)
	}
	if _, err := loadProject(st, fx.ten.ID, orig, store.ProjectImport{Key: "COPY"}); !errors.Is(err, store.ErrConflict) || !strings.Contains(err.Error(), "COPY already exists") {
		t.Fatal(err)
	}
	if _, err := loadProject(st, fx.ten.ID, orig, store.ProjectImport{}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	// another organisation may keep the key… unless it is taken there too
	if _, err := loadProject(st, fx.ten2.ID, orig, store.ProjectImport{}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	other, err := loadProject(st, fx.ten2.ID, orig, store.ProjectImport{Key: "ACME"})
	must(t, err)
	if other.Project.TenantID != fx.ten2.ID {
		t.Fatal(other.Project)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM requirements WHERE project_id = ? AND tenant_id <> ?`, other.Project.ID, fx.ten2.ID); n != 0 {
		t.Fatal("rows of another tenant", n)
	}
}

func archiveEntries(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	rewrite(t, b, func(name string, data []byte) []byte {
		out[name] = data
		return data
	})
	return out
}

func TestProjectArchiveIntoAnotherDatabase(t *testing.T) {
	src := store.New(testutil.OpenDB(t))
	fx := newArchiveFixture(t, src)
	b, m := saveProject(t, src, fx.ten.ID, fx.project.ID)
	// a fresh SQLite database without the reviewer's account
	dst := sqliteStore(t)
	ten, err := dst.CreateTenant(context.Background(), "new", "New")
	must(t, err)
	res, err := loadProject(dst, ten.ID, b, store.ProjectImport{})
	must(t, err)
	if res.Project.Key != fx.project.Key {
		t.Fatal(res.Project.Key)
	}
	if res.Skipped["review_requests"] != m.Tables["review_requests"] || res.Imported["review_requests"] != 0 {
		t.Fatalf("skipped %v", res.Skipped)
	}
	if res.Imported["requirements"] != m.Tables["requirements"] || res.Imported["test_executions"] != m.Tables["test_executions"] {
		t.Fatalf("%v", res.Imported)
	}
}

func TestProjectArchiveRejects(t *testing.T) {
	ctx := context.Background()
	st := sqliteStore(t)
	fx := newArchiveFixture(t, st)
	b, _ := saveProject(t, st, fx.ten.ID, fx.project.ID)
	bad := func(name string, data []byte, want error, msg string) {
		t.Helper()
		_, err := loadProject(st, fx.ten.ID, data, store.ProjectImport{Key: "NEW"})
		if !errors.Is(err, want) || !strings.Contains(err.Error(), msg) {
			t.Errorf("%s: %v (want %v %q)", name, err, want, msg)
		}
	}
	bad("not a zip", []byte("nope"), store.ErrInvalid, "not a valid ZIP")
	bk, _ := backup(t, st)
	bad("full backup", bk, store.ErrInvalid, "")
	bad("newer schema", rewrite(t, b, manifestEdit(func(m map[string]any) { m["schema_version"] = 9999 })), store.ErrConflict, "newer version")
	bad("wrong kind", rewrite(t, b, manifestEdit(func(m map[string]any) { m["kind"] = "x" })), store.ErrInvalid, "not a project archive")
	bad("format", rewrite(t, b, manifestEdit(func(m map[string]any) { m["format_version"] = 7 })), store.ErrInvalid, "format version 7")
	bad("no manifest", rewrite(t, b, func(n string, d []byte) []byte {
		if n == "manifest.json" {
			return nil
		}
		return d
	}), store.ErrInvalid, "no manifest")
	bad("extra file", rewrite(t, b, nil, [2]string{"../evil", "x"}), store.ErrInvalid, "unexpected file")
	bad("duplicate", rewrite(t, b, nil, [2]string{"manifest.json", "{}"}), store.ErrInvalid, "twice")
	bad("counts", rewrite(t, b, manifestEdit(func(m map[string]any) { m["tables"].(map[string]any)["requirements"] = 1 })), store.ErrInvalid, "the manifest says 1")
	if _, err := loadProject(st, fx.ten.ID, b, store.ProjectImport{Key: "1x"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := loadProject(st, fx.ten.ID, b, store.ProjectImport{Key: "OK", Name: strings.Repeat("n", 201)}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	bad("no project", rewrite(t, b, func(n string, d []byte) []byte {
		if n == "tables/projects.jsonl" {
			return nil
		}
		return d
	}), store.ErrInvalid, "no project")
	bad("lost file", rewrite(t, b, func(n string, d []byte) []byte {
		if strings.HasPrefix(n, "files/") {
			return nil
		}
		return d
	}), store.ErrInvalid, "lacks the content")
	bad("changed file", rewrite(t, b, func(n string, d []byte) []byte {
		if strings.HasPrefix(n, "files/") {
			return append(d, 'x')
		}
		return d
	}), store.ErrInvalid, "checksum")
	bad("broken line", rewrite(t, b, func(n string, d []byte) []byte {
		if n == "tables/requirements.jsonl" {
			return append(d, []byte("{oops\n")...)
		}
		return d
	}), store.ErrInvalid, "invalid JSON")
	// a hand-made row pointing at a project outside the archive is left out
	ps, _ := st.ListProjects(ctx, fx.ten2.ID)
	foreign := rewrite(t, b, func(n string, d []byte) []byte {
		if n == "tables/requirements.jsonl" {
			first := bytes.SplitN(d, []byte("\n"), 2)[0]
			return bytes.Replace(d, first, bytes.Replace(first, []byte(fx.project.ID), []byte(ps[0].ID), 1), 1)
		}
		return d
	})
	res, err := loadProject(st, fx.ten.ID, foreign, store.ProjectImport{Key: "FOREIGN"})
	must(t, err)
	if res.Skipped["requirements"] != 1 {
		t.Fatalf("%v", res.Skipped)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM requirements WHERE project_id = ?`, ps[0].ID); n != count(t, st, `SELECT COUNT(*) FROM requirements WHERE project_id = ?`, fx.project.ID) {
		t.Fatal("rows were added to another project", n)
	}
	bad("manifest JSON", rewrite(t, b, func(n string, d []byte) []byte {
		if n == "manifest.json" {
			return []byte("{")
		}
		return d
	}), store.ErrInvalid, "manifest.json")
	bad("two projects", rewrite(t, b, func(n string, d []byte) []byte {
		if n == "tables/projects.jsonl" {
			return append(d, bytes.Replace(d, []byte(fx.project.ID), []byte("11111111-2222-3333-4444-555555555555"), 1)...)
		}
		return d
	}), store.ErrInvalid, "")
	// a tracker setting that is not JSON is reset; the project still loads
	broken := rewrite(t, b, func(n string, d []byte) []byte {
		if n == "tables/projects.jsonl" {
			return []byte(strings.Replace(string(d), `"defect_tracker":"{`, `"defect_tracker":"not json{`, 1))
		}
		return d
	})
	res, err = loadProject(st, fx.ten.ID, broken, store.ProjectImport{Key: "BROKEN"})
	must(t, err)
	if tr, err := st.DefectTracker(ctx, fx.ten.ID, res.Project.ID); err != nil || tr.Type != "" {
		t.Fatal(tr, err)
	}
	oldLine := store.MaxRestoreLine
	store.MaxRestoreLine = 64
	bad("long line", b, store.ErrInvalid, "exceeds")
	store.MaxRestoreLine = oldLine
	// size limits
	old := store.MaxRestoreSize
	store.MaxRestoreSize = 1000
	defer func() { store.MaxRestoreSize = old }()
	bad("too large", b, store.ErrInvalid, "too large")
	if _, err := st.ExportProject(ctx, fx.ten2.ID, fx.project.ID, &bytes.Buffer{}, "x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("other tenant's project", err)
	}
}
