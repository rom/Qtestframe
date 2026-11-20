package store_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/app"
	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil"
)

// sqliteStore returns a migrated in-memory SQLite store (regardless of
// QTF_TEST_POSTGRES_DSN).
func sqliteStore(t *testing.T) *store.Store {
	t.Helper()
	d, err := db.Open(context.Background(), "sqlite", ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store.New(d)
}

type seeded struct {
	super, disabled *model.User
	file            *model.FileInfo
	blob            []byte
	child, parent   string // folder ids (child created before parent)
}

// seed fills st with a representative data set across all tables.
func seed(t *testing.T, st *store.Store) seeded {
	t.Helper()
	ctx := context.Background()
	ten, err := st.CreateTenant(ctx, "acme", "Acme")
	must(t, err)
	ten2, err := st.CreateTenant(ctx, "other", "Other")
	must(t, err)
	super := &model.User{Email: "root@example.com", SuperAdmin: true, PasswordHash: "h"}
	must(t, st.CreateUser(ctx, super))
	disabled := &model.User{Email: "gone@example.com"}
	must(t, st.CreateUser(ctx, disabled))
	disabled.Disabled, disabled.MustChangePassword = true, true
	disabled.Preferences.Theme = "dark"
	must(t, st.UpdateUser(ctx, disabled))
	must(t, st.SetMembership(ctx, ten.ID, super.ID, "admin"))
	must(t, st.SetMembership(ctx, ten2.ID, disabled.ID, "viewer"))
	exp := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)
	must(t, st.CreateToken(ctx, &model.APIToken{TenantID: ten.ID, UserID: super.ID, Name: "ci", Role: "tester", Hash: "hash1", Prefix: "qtf_ab", ExpiresAt: &exp}))
	must(t, st.CreateSession(ctx, &model.Session{ID: "sess", UserID: super.ID, TenantID: ten.ID, CSRF: "c", ExpiresAt: exp}))
	p, err := app.SeedDemo(ctx, st, ten.ID)
	must(t, err)
	if _, err := app.SeedDemo(ctx, st, ten2.ID); err != nil {
		t.Fatal(err)
	}
	// a folder whose parent is created after it
	child := &model.Folder{ProjectID: p.ID, Kind: model.KindRequirement, Name: "Child"}
	must(t, st.CreateFolder(ctx, ten.ID, child))
	parent := &model.Folder{ProjectID: p.ID, Kind: model.KindRequirement, Name: "Parent"}
	must(t, st.CreateFolder(ctx, ten.ID, parent))
	must(t, st.MoveFolder(ctx, ten.ID, child.ID, &parent.ID, 0, ""))

	reqs, err := st.ListRequirements(ctx, ten.ID, p.ID)
	must(t, err)
	must(t, st.CreateRelation(ctx, ten.ID, &model.Relation{SourceID: reqs[1].ID, TargetID: reqs[0].ID, Type: "refines", CreatedBy: "x"}))
	must(t, st.AddComment(ctx, ten.ID, &model.Comment{EntityType: model.KindRequirement, EntityID: reqs[0].ID, UserID: super.ID, Body: "Looks good ✓"}))
	must(t, st.CreateBaseline(ctx, ten.ID, &model.Baseline{ProjectID: p.ID, Name: "v1"},
		[]model.BaselineItem{{EntityType: model.KindRequirement, EntityID: reqs[0].ID, Key: reqs[0].Key, Title: reqs[0].Title, Version: 1, Snapshot: json.RawMessage(`{"a":1}`)}}))
	blob := make([]byte, 600)
	for i := range blob {
		blob[i] = byte(i * 7)
	}
	f := &model.FileInfo{ProjectID: &p.ID, Kind: model.FileAttachment, Name: "bin.dat", Tags: []string{"x"}, UploadedBy: super.Email}
	must(t, st.CreateFile(ctx, ten.ID, f, blob))
	must(t, st.Attach(ctx, f.ID, model.KindRequirement, reqs[0].ID))
	must(t, st.CreateFile(ctx, "", &model.FileInfo{Kind: model.FileAvatar, Name: "a.png"}, []byte{0x89, 'P', 'N', 'G'}))
	must(t, st.CreateReportDefinition(ctx, ten.ID, &model.ReportDefinition{ProjectID: p.ID, Name: "r", Config: json.RawMessage(`{"entity":"test"}`)}))
	must(t, st.RecordLogin(ctx, &model.LoginEvent{UserID: super.ID, Success: true, Method: "local"}))
	must(t, st.RecordLogin(ctx, &model.LoginEvent{UserID: super.ID, Success: false, Method: "local"}))
	must(t, st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, UserEmail: super.Email, Action: "x", EntityType: "y"}))
	return seeded{super: super, disabled: disabled, file: f, blob: blob, child: child.ID, parent: parent.ID}
}

func backup(t *testing.T, st *store.Store) ([]byte, *store.BackupManifest) {
	t.Helper()
	var buf bytes.Buffer
	m, err := st.Backup(context.Background(), &buf, "test-1.0")
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), m
}

func restore(st *store.Store, b []byte) (map[string]int, error) {
	return st.Restore(context.Background(), bytes.NewReader(b), int64(len(b)))
}

// dump returns every backed up table as sorted, normalised JSON lines.
func dump(t *testing.T, b []byte) map[string][]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".jsonl") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		var lines []string
		for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if l == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
			nb, _ := json.Marshal(m) // sorted keys
			lines = append(lines, string(nb))
		}
		sort.Strings(lines)
		out[strings.TrimSuffix(f.Name, ".jsonl")] = lines
	}
	return out
}

func entry(t *testing.T, b []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, _ := f.Open()
			defer rc.Close()
			data, _ := io.ReadAll(rc)
			return data
		}
	}
	t.Fatalf("no entry %s", name)
	return nil
}

// rewrite copies an archive, letting fn replace (or drop, by returning
// nil) entries; extra entries are appended.
func rewrite(t *testing.T, b []byte, fn func(name string, data []byte) []byte, extra ...[2]string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if fn != nil {
			data = fn(f.Name, data)
		}
		if data == nil {
			continue
		}
		w, _ := zw.Create(f.Name)
		w.Write(data)
	}
	for _, e := range extra {
		w, err := zw.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e[1]))
	}
	must(t, zw.Close())
	return out.Bytes()
}

func manifestEdit(fn func(m map[string]any)) func(string, []byte) []byte {
	return func(name string, data []byte) []byte {
		if name != "manifest.json" {
			return data
		}
		var m map[string]any
		json.Unmarshal(data, &m)
		fn(m)
		nb, _ := json.Marshal(m)
		return nb
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := sqliteStore(t)
	sd := seed(t, src)
	b, m := backup(t, src)

	if m.FormatVersion != 1 || m.AppVersion != "test-1.0" || m.Driver != "sqlite" || m.CreatedAt.IsZero() {
		t.Fatalf("%+v", m)
	}
	if v, _ := src.SchemaVersion(ctx); m.SchemaVersion != v || v < 3 {
		t.Fatal(m.SchemaVersion, v)
	}
	if len(m.Tables) != len(store.BackupTables) || m.Tables["sessions"] != 0 {
		t.Fatal(m.Tables)
	}
	for _, tb := range store.BackupTables {
		if tb == "file_blobs" || tb == "users" || tb == "folders" || tb == "login_events" || tb == "test_executions" {
			if m.Tables[tb] == 0 {
				t.Fatalf("no rows in %s", tb)
			}
		}
	}
	var mf map[string]any
	must(t, json.Unmarshal(entry(t, b, "manifest.json"), &mf))
	for _, k := range []string{"format_version", "schema_version", "created_at", "app_version", "driver", "tables"} {
		if _, ok := mf[k]; !ok {
			t.Fatalf("manifest lacks %s", k)
		}
	}
	// value normalisation in the JSON lines
	users := string(entry(t, b, "users.jsonl"))
	if !strings.Contains(users, `"super_admin":true`) || !strings.Contains(users, `"disabled":true`) || !strings.Contains(users, `"must_change_password":true`) || strings.Contains(users, `"super_admin":1`) {
		t.Fatal(users)
	}
	logins := string(entry(t, b, "login_events.jsonl"))
	if !strings.Contains(logins, `"success":true`) || !strings.Contains(logins, `"success":false`) {
		t.Fatal(logins)
	}
	var u map[string]any
	must(t, json.Unmarshal([]byte(strings.SplitN(users, "\n", 2)[0]), &u))
	if _, err := time.Parse(time.RFC3339, u["created_at"].(string)); err != nil {
		t.Fatal(u["created_at"])
	}
	if _, ok := dump(t, b)["sessions"]; ok {
		t.Fatal("sessions must not be backed up")
	}

	// restore into a fresh database (PostgreSQL when configured)
	dst := store.New(testutil.OpenDB(t))
	// pre-existing data is replaced
	if _, err := dst.CreateTenant(ctx, "old", "Old"); err != nil {
		t.Fatal(err)
	}
	counts, err := restore(dst, b)
	if err != nil {
		t.Fatal(err)
	}
	for tb, n := range m.Tables {
		if counts[tb] != n {
			t.Fatalf("%s: restored %d, want %d", tb, counts[tb], n)
		}
	}
	if _, err := dst.GetTenantBySlug(ctx, "old"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("old data kept", err)
	}
	if _, err := dst.GetSession(ctx, "sess"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("sessions must not be restored", err)
	}
	// byte-for-byte blob, booleans, timestamps
	data, err := dst.FileData(ctx, sd.file.ID)
	if err != nil || !bytes.Equal(data, sd.blob) {
		t.Fatal("blob differs", err)
	}
	gotSuper, err := dst.GetUser(ctx, sd.super.ID)
	must(t, err)
	if !gotSuper.SuperAdmin || gotSuper.Disabled || !gotSuper.CreatedAt.Equal(sd.super.CreatedAt) {
		t.Fatalf("%+v vs %+v", gotSuper, sd.super)
	}
	gotDis, err := dst.GetUser(ctx, sd.disabled.ID)
	must(t, err)
	if !gotDis.Disabled || !gotDis.MustChangePassword || gotDis.SuperAdmin || gotDis.Preferences.Theme != "dark" {
		t.Fatalf("%+v", gotDis)
	}
	logs, err := dst.ListLogins(ctx, sd.super.ID, 10)
	must(t, err)
	succ := map[bool]int{}
	for _, l := range logs {
		succ[l.Success]++
	}
	if succ[true] != 1 || succ[false] != 1 {
		t.Fatal(logs)
	}
	fi, err := dst.GetFile(ctx, fiTenant(t, src, sd.file.ID), sd.file.ID)
	if err != nil || !fi.CreatedAt.Equal(sd.file.CreatedAt) || len(fi.Links) != 1 {
		t.Fatal(fi, err)
	}
	if f, err := dst.GetFolder(ctx, fiTenant(t, src, sd.file.ID), sd.child); err != nil || f.ParentID == nil || *f.ParentID != sd.parent {
		t.Fatal(f, err)
	}
	// a backup of the restored database equals the original
	b2, m2 := backup(t, dst)
	if m2.Driver != testutil.Driver() {
		t.Fatal(m2.Driver)
	}
	d1, d2 := dump(t, b), dump(t, b2)
	for _, tb := range store.BackupTables {
		if strings.Join(d1[tb], "\n") != strings.Join(d2[tb], "\n") {
			t.Fatalf("table %s differs after round trip:\n%v\n%v", tb, d1[tb], d2[tb])
		}
	}
	// and restores back into SQLite (PostgreSQL → SQLite when configured)
	back := sqliteStore(t)
	if _, err := restore(back, b2); err != nil {
		t.Fatal(err)
	}
	b3, _ := backup(t, back)
	d3 := dump(t, b3)
	for _, tb := range store.BackupTables {
		if strings.Join(d1[tb], "\n") != strings.Join(d3[tb], "\n") {
			t.Fatalf("table %s differs after second round trip", tb)
		}
	}
	got := store.SortedTables(map[string]int{"users": 1, "tenants": 2, "files": 3})
	if strings.Join(got, ",") != "tenants,users,files" {
		t.Fatal(got)
	}
	if m.Rows() == 0 {
		t.Fatal("rows")
	}
}

// fiTenant returns the tenant of a file in st.
func fiTenant(t *testing.T, st *store.Store, fileID string) string {
	t.Helper()
	var tid string
	if err := st.DB.QueryRowContext(context.Background(), st.DB.Rebind(`SELECT tenant_id FROM files WHERE id = ?`), fileID).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	return tid
}

func TestBackupRejectsUnknownTables(t *testing.T) {
	st := sqliteStore(t)
	if _, err := st.DB.Exec(`CREATE TABLE extra_stuff (id TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Backup(context.Background(), io.Discard, "x"); err == nil || !strings.Contains(err.Error(), "extra_stuff") {
		t.Fatal(err)
	}
}

func TestRestoreValidation(t *testing.T) {
	ctx := context.Background()
	src := sqliteStore(t)
	seed(t, src)
	good, _ := backup(t, src)
	dst := sqliteStore(t)
	keep, err := dst.CreateTenant(ctx, "keep", "Keep")
	must(t, err)

	tableEdit := func(table string, fn func(string) string) func(string, []byte) []byte {
		return func(name string, data []byte) []byte {
			if name == table+".jsonl" {
				return []byte(fn(string(data)))
			}
			return data
		}
	}
	firstLine := func(s string) string { return strings.SplitN(s, "\n", 2)[0] }
	cases := []struct {
		name string
		data []byte
		want error
		msg  string
	}{
		{"not a zip", []byte("hello"), store.ErrInvalid, "ZIP"},
		{"no manifest", rewrite(t, good, func(n string, d []byte) []byte {
			if n == "manifest.json" {
				return nil
			}
			return d
		}), store.ErrInvalid, "manifest"},
		{"bad manifest", rewrite(t, good, func(n string, d []byte) []byte {
			if n == "manifest.json" {
				return []byte("{")
			}
			return d
		}), store.ErrInvalid, "manifest"},
		{"format version", rewrite(t, good, manifestEdit(func(m map[string]any) { m["format_version"] = 2 })), store.ErrInvalid, "format version"},
		{"no tables", rewrite(t, good, manifestEdit(func(m map[string]any) { delete(m, "tables") })), store.ErrInvalid, "no tables"},
		{"schema version", rewrite(t, good, manifestEdit(func(m map[string]any) { m["schema_version"] = 999 })), store.ErrConflict, "schema version 999"},
		{"missing table", rewrite(t, good, func(n string, d []byte) []byte {
			if n == "users.jsonl" {
				return nil
			}
			return d
		}), store.ErrInvalid, "users is missing"},
		{"unknown manifest table", rewrite(t, good, manifestEdit(func(m map[string]any) { m["tables"].(map[string]any)["nope"] = 1 })), store.ErrInvalid, "unknown table"},
		{"zip slip", rewrite(t, good, nil, [2]string{"../evil.jsonl", "{}"}), store.ErrInvalid, "unsafe"},
		{"zip slip abs", rewrite(t, good, nil, [2]string{"/etc/passwd", "x"}), store.ErrInvalid, "unsafe"},
		{"zip slip dir", rewrite(t, good, nil, [2]string{"sub/users.jsonl", "x"}), store.ErrInvalid, "unsafe"},
		{"zip slip windows", rewrite(t, good, nil, [2]string{`..\users.jsonl`, "x"}), store.ErrInvalid, "unsafe"},
		{"unexpected file", rewrite(t, good, nil, [2]string{"readme.txt", "x"}), store.ErrInvalid, "unexpected"},
		{"duplicate", rewrite(t, good, nil, [2]string{"users.jsonl", "x"}), store.ErrInvalid, "twice"},
		{"count mismatch", rewrite(t, good, manifestEdit(func(m map[string]any) { m["tables"].(map[string]any)["users"] = 99 })), store.ErrInvalid, "manifest says 99"},
		{"bad json", rewrite(t, good, tableEdit("tenants", func(s string) string { return s + "{not json\n" })), store.ErrInvalid, "invalid JSON"},
		{"unknown column", rewrite(t, good, tableEdit("tenants", func(s string) string { return strings.Replace(s, `"slug"`, `"slugx"`, 1) })), store.ErrInvalid, "unknown column"},
		{"empty row", rewrite(t, good, tableEdit("comments", func(s string) string { return "{}\n" + s })), store.ErrInvalid, "empty row"},
		{"bad time", rewrite(t, good, tableEdit("tenants", func(s string) string {
			return strings.Replace(s, firstLine(s), strings.Replace(firstLine(s), `"created_at":"`, `"created_at":"x`, 1), 1)
		})), store.ErrInvalid, "invalid time"},
		{"bad bool", rewrite(t, good, tableEdit("users", func(s string) string { return strings.Replace(s, `"disabled":true`, `"disabled":"maybe"`, 1) })), store.ErrInvalid, "disabled"},
		{"bad int", rewrite(t, good, tableEdit("projects", func(s string) string { return strings.Replace(s, `"req_seq":`, `"req_seq":[1],"x":`, 1) })), store.ErrInvalid, ""},
		{"bad blob", rewrite(t, good, tableEdit("file_blobs", func(s string) string { return strings.Replace(s, `"data":"`, `"data":"!!!`, 1) })), store.ErrInvalid, "data"},
		{"constraint", rewrite(t, good, tableEdit("memberships", func(s string) string {
			return strings.Replace(s, firstLine(s), strings.Replace(firstLine(s), `"tenant_id":"`, `"tenant_id":"missing-`, 1), 1)
		})), store.ErrInvalid, "memberships line 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := restore(dst, c.data)
			if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.msg) {
				t.Fatalf("got %v, want %v containing %q", err, c.want, c.msg)
			}
			// nothing changed
			if _, err := dst.GetTenant(ctx, keep.ID); err != nil {
				t.Fatal("data lost after a failed restore", err)
			}
		})
	}
}

func TestRestoreLimits(t *testing.T) {
	src := sqliteStore(t)
	seed(t, src)
	good, _ := backup(t, src)
	dst := sqliteStore(t)

	oldLine, oldSize := store.MaxRestoreLine, store.MaxRestoreSize
	t.Cleanup(func() { store.MaxRestoreLine, store.MaxRestoreSize = oldLine, oldSize })
	store.MaxRestoreLine = 200
	if _, err := restore(dst, good); !errors.Is(err, store.ErrInvalid) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatal(err)
	}
	store.MaxRestoreLine = oldLine
	store.MaxRestoreSize = 1000
	if _, err := restore(dst, good); !errors.Is(err, store.ErrInvalid) || !strings.Contains(err.Error(), "too large") {
		t.Fatal(err)
	}
	store.MaxRestoreSize = oldSize
	// blank lines are tolerated, everything else restores fine
	b := rewrite(t, good, func(n string, d []byte) []byte {
		if n == "tenants.jsonl" {
			return append([]byte("\n  \n"), d...)
		}
		return d
	})
	if _, err := restore(dst, b); err != nil {
		t.Fatal(err)
	}
}

func mustReplace(t *testing.T, s, old, repl string, n int) string {
	t.Helper()
	if !strings.Contains(s, old) {
		t.Fatalf("%q not found", old)
	}
	return strings.Replace(s, old, repl, n)
}

func TestRestoreLenientValues(t *testing.T) {
	// values as other producers might write them: numeric or string
	// booleans, numbers and booleans for strings and integers, other
	// time layouts
	ctx := context.Background()
	src := sqliteStore(t)
	sd := seed(t, src)
	good, _ := backup(t, src)
	b := rewrite(t, good, func(n string, d []byte) []byte {
		s := string(d)
		switch n {
		case "users.jsonl":
			s = mustReplace(t, s, `"super_admin":true`, `"super_admin":1`, -1)
			s = mustReplace(t, s, `"disabled":true`, `"disabled":"true"`, -1)
			s = mustReplace(t, s, `"auth_provider":"local"`, `"auth_provider":true`, -1)
		case "tenants.jsonl":
			s = mustReplace(t, s, `"name":"Acme"`, `"name":42`, 1)
		case "test_executions.jsonl":
			s = mustReplace(t, s, `"duration_ms":0`, `"duration_ms":false`, 1)
			s = mustReplace(t, s, `"duration_ms":0`, `"duration_ms":true`, 1)
			s = mustReplace(t, s, `"duration_ms":0`, `"duration_ms":"0"`, 1)
		case "audit_log.jsonl":
			var m map[string]any
			json.Unmarshal([]byte(strings.SplitN(s, "\n", 2)[0]), &m)
			tm, _ := time.Parse(time.RFC3339Nano, m["created_at"].(string))
			s = mustReplace(t, s, m["created_at"].(string), tm.Format("2006-01-02 15:04:05.999999999"), 1)
		}
		return []byte(s)
	})
	dst := sqliteStore(t)
	if _, err := restore(dst, b); err != nil {
		t.Fatal(err)
	}
	u, err := dst.GetUser(ctx, sd.super.ID)
	must(t, err)
	if !u.SuperAdmin || u.AuthProvider != "true" {
		t.Fatalf("%+v", u)
	}
	ten, err := dst.GetTenantBySlug(ctx, "acme")
	if err != nil || ten.Name != "42" {
		t.Fatal(ten, err)
	}
	var durations int64
	must(t, dst.DB.QueryRow(`SELECT SUM(duration_ms) FROM test_executions`).Scan(&durations))
	if durations != 1 {
		t.Fatal(durations)
	}
	a, err := dst.ListAudit(ctx, ten.ID, 10)
	if err != nil || len(a) != 1 || a[0].CreatedAt.IsZero() {
		t.Fatal(a, err)
	}
}
