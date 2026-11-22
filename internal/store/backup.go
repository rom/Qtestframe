package store

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/secrets"
)

// Logical backups are database agnostic: a ZIP archive with manifest.json
// and one <table>.jsonl per table (one JSON object per row, column →
// value). Times are RFC 3339 strings, booleans true/false and BLOBs base64,
// so a backup taken from SQLite restores into PostgreSQL and vice versa.

// BackupFormatVersion is the version of the backup archive layout.
const BackupFormatVersion = 1

// BackupTables lists the backed up tables in foreign-key-safe order
// (parents first). sessions and schema_migrations are not backed up.
var BackupTables = []string{
	"tenants", "users", "memberships", "api_tokens", "projects", "configurations", "releases", "folders", "categories",
	"requirements", "requirement_categories", "evaluation_criteria", "test_cases", "test_steps",
	"test_case_categories", "trace_links", "variants", "builds", "test_cycles", "test_cycle_tests", "test_runs", "run_configurations", "test_executions", "execution_iterations", "step_results",
	"audit_log", "revisions", "requirement_relations", "baselines", "baseline_items", "comments",
	"files", "file_blobs", "attachments", "report_definitions", "login_events", "roles", "test_dependencies",
	"trash", "item_links", "release_requirements", "defects", "defect_links", "risks", "risk_requirements", "notifications", "webhooks", "review_requests", "baseline_signoffs",
	"scim_tokens", "scim_settings", "scim_users", "scim_groups", "scim_group_members",
	"metric_snapshots",
	"trace_rules",
	// change and defect correlations: generated document contents, regressions
	"document_items", "defect_regressions",
	"consistency_dismissals",
	"invitations", "report_subscriptions",
	"user_mfa", "mfa_recovery_codes", "mfa_policies",
	"project_members",
	// waivers and deviations with their approvals, the Statement of
	// Applicability decisions
	"waivers", "waiver_requirements", "waiver_signoffs", "soa_decisions",
	"system_settings",
	// where items came from in other tools (the ReqIF import)
	"external_refs",
	// verification correlations (0030)
	"link_criteria", "system_elements", "requirement_elements", "test_elements",
	"verification_settings", "verification_records", "verification_record_files",
	// variants and applicability, code links (0036)
	"item_applicability", "code_changes", "code_change_items", "code_settings",
	// custom fields, their archived values and item templates
	"custom_fields", "field_set_fields", "custom_field_archive", "item_templates", "org_item_templates", "project_template_defaults",
	// exploratory testing sessions, their scope and notes (0040)
	"exploratory_sessions", "exploratory_scope", "exploratory_notes",
	// the data-encryption keys of the stored secrets, wrapped by the key
	// provider's key: a backup restores only where that key is available
	// test cycles (0041); test_cycles and test_cycle_tests come before
	// test_runs above
	"test_cycle_schedules",
	"secret_keys",
	// chat notification channels (0045)
	"chat_channels",
}

// tables that exist but are deliberately not part of a backup.
// (pending MFA sign-ins and remembered devices are as transient as sessions)
var notBackedUp = map[string]bool{"sessions": true, "schema_migrations": true, "job_runs": true, "script_jobs": true,
	"mfa_challenges": true, "mfa_trusted_devices": true,
	// personal autocomplete history: a convenience, not project data
	"suggest_history": true,
	// queue and log of chat posts (the channels are backed up)
	"chat_deliveries": true,
	// queue and log of CI status posts (0053; the settings are backed up)
	"ci_deliveries": true,
	// vectors of the embeddings service: a cache, computed again
	"similarity_embeddings": true}

// NotBackedUpTables returns the tables that exist but are deliberately not
// part of a backup, sorted (for the administrator guide, see
// internal/admindoc).
func NotBackedUpTables() []string {
	out := make([]string, 0, len(notBackedUp))
	for t := range notBackedUp {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// boolColumns are stored as INTEGER 0/1 on SQLite and BOOLEAN on PostgreSQL.
var boolColumns = map[string]bool{
	"users.super_admin": true, "users.disabled": true, "users.must_change_password": true,
	"login_events.success": true, "scim_users.active": true, "trace_rules.enabled": true,
	"invitations.sso": true, "report_subscriptions.enabled": true, "secret_keys.active": true,
	"verification_records.until_change": true, "verification_settings.strict_criteria": true,
	"verification_settings.exploratory_evidence": true, "custom_fields.required": true, "custom_fields.show_in_list": true,
	"field_set_fields.required": true, "field_set_fields.show_in_list": true,
	"chat_channels.enabled": true, "chat_channels.restricted_content": true,
	"test_cycle_schedules.auto_start": true, "test_cycle_schedules.enabled": true,
}

// blobColumns hold binary data (base64 in the backup).
var blobColumns = map[string]bool{"file_blobs.data": true}

// selfReferences names tables whose rows reference rows of the same table;
// restore inserts parents before children.
var selfReferences = map[string]string{"folders": "parent_id", "system_elements": "parent_id", "variants": "parent_id"}

// Restore limits (variables so that tests can lower them).
var (
	// MaxRestoreSize bounds the total uncompressed size of a backup.
	MaxRestoreSize int64 = 4 << 30
	// MaxRestoreLine bounds one JSON line (one row) of a table file.
	MaxRestoreLine = 512 << 20
)

// BackupManifest describes a backup archive (manifest.json).
type BackupManifest struct {
	FormatVersion int            `json:"format_version"`
	SchemaVersion int            `json:"schema_version"`
	CreatedAt     time.Time      `json:"created_at"`
	AppVersion    string         `json:"app_version"`
	Driver        string         `json:"driver"`
	Tables        map[string]int `json:"tables"`
}

// Rows returns the total number of rows in the backup.
func (m *BackupManifest) Rows() int {
	n := 0
	for _, c := range m.Tables {
		n += c
	}
	return n
}

// SchemaVersion returns the highest applied migration.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	return schemaVersion(ctx, s.c(ctx))
}

func schemaVersion(ctx context.Context, c conn) (int, error) {
	var v sql.NullInt64
	if err := c.row(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, err
	}
	return int(v.Int64), nil
}

// listTables returns the names of all tables of the database.
func listTables(ctx context.Context, c conn) ([]string, error) {
	q := `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`
	if c.d == db.Postgres {
		q = `SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`
	}
	rows, err := c.query(ctx, q)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// checkTables fails when the database has tables the backup does not know
// (a migration added a table without updating BackupTables).
func checkTables(ctx context.Context, c conn) error {
	names, err := listTables(ctx, c)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, t := range BackupTables {
		known[t] = true
	}
	for _, n := range names {
		if !known[n] && !notBackedUp[n] {
			return fmt.Errorf("backup: table %q is not covered by store.BackupTables", n)
		}
	}
	return nil
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Backup writes a logical backup of all tables (all tenants) as a ZIP
// archive to w. Rows are streamed table by table inside one read
// transaction, so the backup is a consistent snapshot.
func (s *Store) Backup(ctx context.Context, w io.Writer, appVersion string) (*BackupManifest, error) {
	var opts *sql.TxOptions
	if s.DB.Dialect == db.Postgres {
		opts = &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	}
	tx, err := s.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // read-only
	c := conn{q: tx, d: s.DB.Dialect}
	if err := checkTables(ctx, c); err != nil {
		return nil, err
	}
	m := &BackupManifest{FormatVersion: BackupFormatVersion, CreatedAt: s.now(), AppVersion: appVersion,
		Driver: string(s.DB.Dialect), Tables: map[string]int{}}
	if m.SchemaVersion, err = schemaVersion(ctx, c); err != nil {
		return nil, err
	}
	zw := zip.NewWriter(w)
	for _, t := range BackupTables {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: t + ".jsonl", Method: zip.Deflate, Modified: m.CreatedAt})
		if err != nil {
			return nil, err
		}
		n, err := dumpTable(ctx, c, t, f)
		if err != nil {
			return nil, fmt.Errorf("backup %s: %w", t, err)
		}
		m.Tables[t] = n
	}
	f, err := zw.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate, Modified: m.CreatedAt})
	if err != nil {
		return nil, err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return m, nil
}

func dumpTable(ctx context.Context, c conn, table string, w io.Writer) (int, error) {
	rows, err := c.query(ctx, `SELECT * FROM `+quoteIdent(table))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return 0, err
	}
	keys := make([][]byte, len(cols))
	kinds := make([]colKind, len(cols))
	for i, col := range cols {
		keys[i], _ = json.Marshal(col)
		kinds[i] = columnKind(table, col, types[i].DatabaseTypeName())
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	bw := bufio.NewWriterSize(w, 64<<10)
	n := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return n, err
		}
		bw.WriteByte('{')
		for i := range cols {
			if i > 0 {
				bw.WriteByte(',')
			}
			bw.Write(keys[i])
			bw.WriteByte(':')
			b, err := json.Marshal(exportValue(kinds[i], vals[i]))
			if err != nil {
				return n, err
			}
			bw.Write(b)
		}
		bw.WriteString("}\n")
		n++
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	return n, bw.Flush()
}

// colKind is the portable type of a column.
type colKind int

const (
	kindString colKind = iota
	kindInt
	kindFloat
	kindBool
	kindTime
	kindBlob
)

// columnKind classifies a column by its declared/database type name;
// booleans and blobs are also recognised by name, because SQLite declares
// booleans as INTEGER.
func columnKind(table, col, dbType string) colKind {
	if boolColumns[table+"."+col] {
		return kindBool
	}
	if blobColumns[table+"."+col] {
		return kindBlob
	}
	t := strings.ToUpper(dbType)
	switch {
	case strings.Contains(t, "BOOL"):
		return kindBool
	case strings.Contains(t, "TIMESTAMP"), t == "DATE", t == "DATETIME", t == "TIME", t == "TIMETZ":
		return kindTime
	case t == "BLOB", t == "BYTEA":
		return kindBlob
	case strings.Contains(t, "INT"):
		return kindInt
	case strings.Contains(t, "REAL"), strings.Contains(t, "FLOA"), strings.Contains(t, "DOUB"), strings.Contains(t, "NUMERIC"), strings.Contains(t, "DECIMAL"):
		return kindFloat
	}
	return kindString
}

// exportValue normalises a scanned value for the backup.
func exportValue(k colKind, v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case []byte:
		if k == kindBlob {
			return base64.StdEncoding.EncodeToString(x)
		}
		return string(x)
	case int64:
		if k == kindBool {
			return x != 0
		}
		return x
	case string:
		if k == kindBlob {
			return base64.StdEncoding.EncodeToString([]byte(x))
		}
		if k == kindBool {
			return x == "1" || strings.EqualFold(x, "true") || strings.EqualFold(x, "t")
		}
		if k == kindTime {
			if t, err := parseTime(x); err == nil {
				return t.UTC().Format(time.RFC3339Nano)
			}
		}
		return x
	}
	return v
}

var timeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02",
}

func parseTime(s string) (time.Time, error) {
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", s)
}

// importValue converts a JSON value of the backup for a column of kind k.
func importValue(k colKind, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch k {
	case kindBool:
		switch x := v.(type) {
		case bool:
			return x, nil
		case json.Number:
			return x.String() != "0", nil
		case string:
			return strconv.ParseBool(x)
		}
	case kindTime:
		if x, ok := v.(string); ok {
			return parseTime(x)
		}
	case kindBlob:
		if x, ok := v.(string); ok {
			return base64.StdEncoding.DecodeString(x)
		}
	case kindInt:
		switch x := v.(type) {
		case json.Number:
			return x.Int64()
		case bool:
			if x {
				return int64(1), nil
			}
			return int64(0), nil
		case string:
			return strconv.ParseInt(x, 10, 64)
		}
	case kindFloat:
		switch x := v.(type) {
		case json.Number:
			return x.Float64()
		case string:
			return strconv.ParseFloat(x, 64)
		}
	default:
		switch x := v.(type) {
		case string:
			return x, nil
		case json.Number:
			return x.String(), nil
		case bool:
			return strconv.FormatBool(x), nil
		}
	}
	return nil, fmt.Errorf("unexpected value %v (%T)", v, v)
}

// ---------------------------------------------------------------- restore

// Restore replaces all data with the content of a backup archive in one
// transaction: every table (sessions included, so everyone is signed out)
// is emptied in reverse foreign-key order, then the rows of the backup are
// inserted. It returns the number of restored rows per table. Invalid
// archives return ErrInvalid, a schema version mismatch ErrConflict.
func (s *Store) Restore(ctx context.Context, r io.ReaderAt, size int64) (map[string]int, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, Invalidf("backup is not a valid ZIP archive: %v", err)
	}
	files, err := backupEntries(zr)
	if err != nil {
		return nil, err
	}
	mf, ok := files["manifest.json"]
	if !ok {
		return nil, Invalidf("backup has no manifest.json")
	}
	m, err := readManifest(mf)
	if err != nil {
		return nil, err
	}
	cur, err := s.SchemaVersion(ctx)
	if err != nil {
		return nil, err
	}
	if m.SchemaVersion != cur {
		return nil, Conflictf("backup schema version %d does not match the database schema version %d; restore it with the qtestframe version that created it (%s)",
			m.SchemaVersion, cur, m.AppVersion)
	}
	for _, t := range BackupTables {
		_, inManifest := m.Tables[t]
		_, inZip := files[t+".jsonl"]
		if !inManifest || !inZip {
			return nil, Invalidf("backup is incomplete: table %s is missing", t)
		}
	}
	for t := range m.Tables {
		if _, ok := files[t+".jsonl"]; !ok {
			return nil, Invalidf("backup manifest names unknown table %q", t)
		}
	}
	budget := &budgetReader{left: MaxRestoreSize}
	out := map[string]int{}
	var kr *secrets.Keyring
	err = s.tx(ctx, func(c conn) error {
		del := deleteOrder()
		for _, t := range del {
			if _, err := c.exec(ctx, `DELETE FROM `+quoteIdent(t)); err != nil {
				return fmt.Errorf("restore: clearing %s: %w", t, err)
			}
		}
		for _, t := range BackupTables {
			n, err := restoreTable(ctx, c, t, files[t+".jsonl"], budget)
			if err != nil {
				return err
			}
			if n != m.Tables[t] {
				return Invalidf("table %s has %d rows, the manifest says %d", t, n, m.Tables[t])
			}
			out[t] = n
		}
		var kerr error
		kr, kerr = s.restoreKeys(ctx, c)
		return kerr
	})
	if err != nil {
		return nil, err
	}
	if kr != nil {
		s.keys.Replace(kr)
		// secrets of a backup taken before they were encrypted
		if _, err := s.MigrateSecrets(ctx); err != nil {
			return out, fmt.Errorf("restore: encrypt the restored secrets: %w", err)
		}
	}
	return out, nil
}

// deleteOrder is the reverse foreign-key order of all data tables.
func deleteOrder() []string {
	all := make([]string, 0, len(BackupTables)+1)
	for _, t := range BackupTables {
		all = append(all, t)
		if t == "users" {
			all = append(all, "sessions", "mfa_challenges", "mfa_trusted_devices", "suggest_history")
		}
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all
}

// backupEntries validates the entry names and sizes of an archive.
func backupEntries(zr *zip.Reader) (map[string]*zip.File, error) {
	known := map[string]bool{"manifest.json": true}
	for _, t := range BackupTables {
		known[t+".jsonl"] = true
	}
	files := map[string]*zip.File{}
	var total uint64
	for _, f := range zr.File {
		name := f.Name
		if name == "" || strings.ContainsAny(name, `/\:`) || strings.Contains(name, "..") || strings.HasPrefix(name, ".") {
			return nil, Invalidf("backup contains an unsafe file name %q", name)
		}
		if !known[name] {
			return nil, Invalidf("backup contains an unexpected file %q", name)
		}
		if _, dup := files[name]; dup {
			return nil, Invalidf("backup contains %q twice", name)
		}
		total += f.UncompressedSize64
		if total > uint64(MaxRestoreSize) {
			return nil, Invalidf("backup is too large (more than %d bytes uncompressed)", MaxRestoreSize)
		}
		files[name] = f
	}
	return files, nil
}

func readManifest(f *zip.File) (*BackupManifest, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, Invalidf("manifest.json: %v", err)
	}
	defer rc.Close()
	var m BackupManifest
	dec := json.NewDecoder(io.LimitReader(rc, 1<<20))
	if err := dec.Decode(&m); err != nil {
		return nil, Invalidf("manifest.json: %v", err)
	}
	if m.FormatVersion != BackupFormatVersion {
		return nil, Invalidf("unsupported backup format version %d (want %d)", m.FormatVersion, BackupFormatVersion)
	}
	if m.Tables == nil {
		return nil, Invalidf("manifest.json lists no tables")
	}
	return &m, nil
}

// budgetReader limits the total number of bytes read across all entries
// (the sizes in the ZIP directory are not trusted).
type budgetReader struct {
	r    io.Reader
	left int64
}

var errBudget = errors.New("budget exceeded")

func (b *budgetReader) Read(p []byte) (int, error) {
	if b.left <= 0 {
		return 0, errBudget
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.r.Read(p)
	b.left -= int64(n)
	return n, err
}

type targetColumn struct {
	name string
	kind colKind
}

// targetColumns discovers the columns of a table in the target database.
func targetColumns(ctx context.Context, c conn, table string) (map[string]targetColumn, []string, error) {
	rows, err := c.query(ctx, `SELECT * FROM `+quoteIdent(table)+` WHERE 1 = 0`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, nil, err
	}
	cols := map[string]targetColumn{}
	order := make([]string, 0, len(types))
	for _, ct := range types {
		cols[ct.Name()] = targetColumn{name: ct.Name(), kind: columnKind(table, ct.Name(), ct.DatabaseTypeName())}
		order = append(order, ct.Name())
	}
	return cols, order, rows.Err()
}

type restoreRow struct {
	line int
	vals map[string]any
}

func restoreTable(ctx context.Context, c conn, table string, f *zip.File, budget *budgetReader) (int, error) {
	cols, order, err := targetColumns(ctx, c, table)
	if err != nil {
		return 0, fmt.Errorf("restore %s: %w", table, err)
	}
	rc, err := f.Open()
	if err != nil {
		return 0, Invalidf("%s: %v", f.Name, err)
	}
	defer rc.Close()
	budget.r = rc
	sc := bufio.NewScanner(budget)
	// the initial buffer counts as a limit too (bufio.Scanner.Buffer)
	sc.Buffer(make([]byte, 0, min(64<<10, MaxRestoreLine)), MaxRestoreLine)

	ins := &inserter{ctx: ctx, c: c, table: table, cols: cols, order: order, stmts: map[string]*sql.Stmt{}}
	defer ins.close()
	selfCol, selfRef := selfReferences[table]
	var buffered []restoreRow
	n, line := 0, 0
	for sc.Scan() {
		line++
		b := sc.Bytes()
		if len(bytes.TrimSpace(b)) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		var row map[string]any
		if err := dec.Decode(&row); err != nil || row == nil {
			return n, Invalidf("%s line %d: invalid JSON object", f.Name, line)
		}
		n++
		if selfRef {
			buffered = append(buffered, restoreRow{line, row})
			continue
		}
		if err := ins.insert(line, row); err != nil {
			return n, err
		}
	}
	if err := sc.Err(); err != nil {
		switch {
		case errors.Is(err, bufio.ErrTooLong):
			return n, Invalidf("%s line %d: row exceeds %d bytes", f.Name, line+1, MaxRestoreLine)
		case errors.Is(err, errBudget):
			return n, Invalidf("backup is too large (more than %d bytes uncompressed)", MaxRestoreSize)
		}
		return n, Invalidf("%s: %v", f.Name, err)
	}
	if selfRef {
		for _, r := range parentsFirst(buffered, selfCol) {
			if err := ins.insert(r.line, r.vals); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// parentsFirst orders rows so that a row's parent (column col referencing
// "id" of the same table) precedes it. Rows in cycles or with unknown
// parents keep their relative order at the end.
func parentsFirst(rows []restoreRow, col string) []restoreRow {
	byID := map[string]int{}
	for i, r := range rows {
		if id, ok := r.vals["id"].(string); ok {
			byID[id] = i
		}
	}
	done := make([]bool, len(rows))
	visiting := make([]bool, len(rows))
	out := make([]restoreRow, 0, len(rows))
	var visit func(i int)
	visit = func(i int) {
		if done[i] || visiting[i] {
			return
		}
		visiting[i] = true
		if p, ok := rows[i].vals[col].(string); ok {
			if j, ok := byID[p]; ok {
				visit(j)
			}
		}
		visiting[i] = false
		done[i] = true
		out = append(out, rows[i])
	}
	for i := range rows {
		visit(i)
	}
	return out
}

// inserter inserts rows with prepared statements per column set.
type inserter struct {
	ctx   context.Context
	c     conn
	table string
	cols  map[string]targetColumn
	order []string
	stmts map[string]*sql.Stmt
}

func (in *inserter) close() {
	for _, st := range in.stmts {
		st.Close()
	}
}

func (in *inserter) insert(line int, row map[string]any) error {
	for k := range row {
		if _, ok := in.cols[k]; !ok {
			return Invalidf("%s line %d: unknown column %q", in.table, line, k)
		}
	}
	names := make([]string, 0, len(row))
	args := make([]any, 0, len(row))
	for _, col := range in.order {
		v, ok := row[col]
		if !ok {
			continue
		}
		val, err := importValue(in.cols[col].kind, v)
		if err != nil {
			return Invalidf("%s line %d column %s: %v", in.table, line, col, err)
		}
		names = append(names, col)
		args = append(args, val)
	}
	if len(names) == 0 {
		return Invalidf("%s line %d: empty row", in.table, line)
	}
	sig := strings.Join(names, ",")
	st, ok := in.stmts[sig]
	if !ok {
		quoted := make([]string, len(names))
		for i, n := range names {
			quoted[i] = quoteIdent(n)
		}
		q := `INSERT INTO ` + quoteIdent(in.table) + ` (` + strings.Join(quoted, ", ") + `) VALUES (` + placeholders(len(names)) + `)`
		p, ok := in.c.q.(interface {
			PrepareContext(context.Context, string) (*sql.Stmt, error)
		})
		if !ok {
			return errors.New("restore: connection cannot prepare statements")
		}
		var err error
		if st, err = p.PrepareContext(in.ctx, db.Rebind(in.c.d, q)); err != nil {
			return fmt.Errorf("restore %s: %w", in.table, err)
		}
		in.stmts[sig] = st
	}
	if _, err := st.ExecContext(in.ctx, args...); err != nil {
		return Invalidf("%s line %d: %v", in.table, line, err)
	}
	return nil
}

// SortedTables returns the table names of a count map in backup order
// (helper for command line output).
func SortedTables(counts map[string]int) []string {
	pos := map[string]int{}
	for i, t := range BackupTables {
		pos[t] = i
	}
	out := make([]string, 0, len(counts))
	for t := range counts {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return pos[out[i]] < pos[out[j]] })
	return out
}
