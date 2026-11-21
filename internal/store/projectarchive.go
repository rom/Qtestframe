package store

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// Project archives ("Save project" / "Load project") hold one project:
// a ZIP archive with
//
//	manifest.json         ProjectArchiveManifest
//	tables/<table>.jsonl  the project's rows, one JSON object per line (the
//	                      same portable encoding as a backup)
//	files/<file id>       the content of each of the project's files
//
// Loading an archive creates a new project: every row gets a new ID and
// every reference to an old ID (in ID columns, JSON and text) is rewritten,
// so an archive can be loaded any number of times, into any organisation
// and any database. Secrets (webhook secrets, the tracker token), people's
// accounts, memberships, sessions, tokens, notifications, e-mail
// subscriptions, the audit log and the trash are not part of an archive.

// ProjectArchiveKind identifies a project archive (manifest "kind").
const ProjectArchiveKind = "qtestframe-project"

// ProjectArchiveFormatVersion is the version of the project archive layout.
const ProjectArchiveFormatVersion = 1

// ProjectArchiveManifest describes a project archive (manifest.json).
type ProjectArchiveManifest struct {
	Kind          string    `json:"kind"`
	FormatVersion int       `json:"format_version"`
	SchemaVersion int       `json:"schema_version"`
	AppVersion    string    `json:"app_version"`
	CreatedAt     time.Time `json:"created_at"`
	Driver        string    `json:"driver"`
	Project       struct {
		ID   string `json:"id"`
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"project"`
	// Tables counts the rows per table; Files and FileBytes the files.
	Tables    map[string]int `json:"tables"`
	Files     int            `json:"files"`
	FileBytes int64          `json:"file_bytes"`
}

// Rows returns the total number of rows in the archive.
func (m *ProjectArchiveManifest) Rows() int {
	n := 0
	for _, c := range m.Tables {
		n += c
	}
	return n
}

// archiveTable is a table of a project archive and the rows of the project
// in it (where: an SQL condition; every ? is the project id).
type archiveTable struct {
	name, where string
}

const (
	inReqs     = `requirement_id IN (SELECT id FROM requirements WHERE project_id = ?)`
	inTests    = `test_case_id IN (SELECT id FROM test_cases WHERE project_id = ?)`
	inRuns     = `run_id IN (SELECT id FROM test_runs WHERE project_id = ?)`
	inExecs    = `execution_id IN (SELECT e.id FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE r.project_id = ?)`
	inBaseline = `baseline_id IN (SELECT id FROM baselines WHERE project_id = ?)`
	inFiles    = `file_id IN (SELECT id FROM files WHERE project_id = ?)`
	ofProject  = `project_id = ?`
	// rows about any row of the archive (filtered by entity_id while saving)
	ofTenant = `tenant_id = ?`
)

// ProjectArchiveTables lists the tables of a project archive in
// foreign-key-safe order (parents first). File contents are stored as
// files/<id>, not as a file_blobs table.
var projectArchiveTables = []archiveTable{
	{"projects", `id = ?`},
	{"configurations", ofProject},
	{"releases", ofProject},
	{"folders", ofProject},
	{"categories", ofProject},
	{"requirements", ofProject},
	{"requirement_categories", inReqs},
	{"evaluation_criteria", inReqs},
	{"test_cases", ofProject},
	{"test_steps", inTests},
	{"test_case_categories", inTests},
	{"trace_links", ofProject},
	{"test_runs", ofProject},
	{"run_configurations", inRuns},
	{"test_executions", inRuns},
	{"execution_iterations", inExecs},
	{"step_results", inExecs},
	{"requirement_relations", ofProject},
	{"baselines", ofProject},
	{"baseline_items", inBaseline},
	{"files", ofProject},
	{"attachments", inFiles},
	{"report_definitions", ofProject},
	{"test_dependencies", ofProject},
	{"item_links", ofProject},
	{"release_requirements", `release_id IN (SELECT id FROM releases WHERE project_id = ?)`},
	{"defects", ofProject},
	{"defect_links", `defect_id IN (SELECT id FROM defects WHERE project_id = ?)`},
	{"risks", ofProject},
	{"risk_requirements", `risk_id IN (SELECT id FROM risks WHERE project_id = ?)`},
	{"webhooks", ofProject},
	{"review_requests", ofProject},
	{"baseline_signoffs", inBaseline},
	{"metric_snapshots", ofProject},
	{"trace_rules", ofProject},
	// last: they may be about any row above
	{"revisions", ofTenant},
	{"comments", ofTenant},
}

// ProjectArchiveExcluded names the tables that are deliberately not part
// of a project archive, with the reason.
var ProjectArchiveExcluded = map[string]string{
	"tenants":              "organisation",
	"users":                "accounts (references such as created_by are kept as values)",
	"memberships":          "access rights",
	"project_members":      "access rights (a loaded project gets its own sharing)",
	"api_tokens":           "secret",
	"audit_log":            "organisation audit trail",
	"login_events":         "account history",
	"roles":                "organisation roles",
	"trash":                "deleted items",
	"notifications":        "personal",
	"report_subscriptions": "personal e-mail schedules",
	"invitations":          "secret",
	"scim_tokens":          "secret",
	"scim_settings":        "organisation",
	"system_settings":      "server configuration",
	"secret_keys":          "server encryption keys",
	"scim_users":           "organisation",
	"scim_groups":          "organisation",
	"scim_group_members":   "organisation",
	"file_blobs":           "stored as files/<id>",
	"user_mfa":             "secret (second factors)",
	"mfa_recovery_codes":   "secret (second factors)",
	"mfa_policies":         "organisation",
}

// ProjectArchiveTableNames returns the names of the archived tables in order.
func ProjectArchiveTableNames() []string {
	out := make([]string, len(projectArchiveTables))
	for i, t := range projectArchiveTables {
		out[i] = t.name
	}
	return out
}

// entityTables are filtered to rows about an archived row.
var entityTables = map[string]bool{"revisions": true, "comments": true}

// mustMapColumns hold references to rows of the project; a row whose
// reference is not part of the archive is not loaded.
var mustMapColumns = map[string]string{
	"attachments":     "entity_id",
	"defect_links":    "entity_id",
	"comments":        "entity_id",
	"revisions":       "entity_id",
	"review_requests": "entity_id",
	"item_links":      "source_id",
}

// sanitizeArchiveRow removes secrets from a row (when saving and again
// when loading, so a hand-made archive cannot bring them in either).
func sanitizeArchiveRow(table string, row map[string]any) {
	switch table {
	case "webhooks":
		// the secret is write-only; a loaded webhook stays off until an
		// administrator sets a new secret and enables it
		row["secret"] = ""
		row["enabled"] = false
	case "projects":
		if s, ok := row["defect_tracker"].(string); ok && s != "" {
			var m map[string]any
			if json.Unmarshal([]byte(s), &m) == nil && m != nil {
				delete(m, "token")
				delete(m, "has_token")
				b, _ := json.Marshal(m)
				row["defect_tracker"] = string(b)
			} else {
				row["defect_tracker"] = "{}"
			}
		}
		// web test variables: secret values are write-only and stay behind
		if s, ok := row["web_tests"].(string); ok && s != "" {
			var w model.WebTestSettings
			if json.Unmarshal([]byte(s), &w) == nil {
				for i := range w.Variables {
					if w.Variables[i].Secret {
						w.Variables[i].Value, w.Variables[i].HasValue = "", false
					}
				}
				b, _ := json.Marshal(w)
				row["web_tests"] = string(b)
			} else {
				row["web_tests"] = "{}"
			}
		}
	}
}

// ---------------------------------------------------------------- save

// ExportProject writes a project archive of one project of a tenant to w,
// inside one read transaction (a consistent snapshot).
func (s *Store) ExportProject(ctx context.Context, tenantID, projectID string, w io.Writer, appVersion string) (*ProjectArchiveManifest, error) {
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
	m := &ProjectArchiveManifest{Kind: ProjectArchiveKind, FormatVersion: ProjectArchiveFormatVersion, CreatedAt: s.now(),
		AppVersion: appVersion, Driver: string(s.DB.Dialect), Tables: map[string]int{}}
	if err := c.row(ctx, `SELECT id, key, name FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).
		Scan(&m.Project.ID, &m.Project.Key, &m.Project.Name); err != nil {
		return nil, mapErr(err, "project")
	}
	if m.SchemaVersion, err = schemaVersion(ctx, c); err != nil {
		return nil, err
	}
	zw := zip.NewWriter(w)
	ids := map[string]bool{}
	var fileIDs []string
	for _, t := range projectArchiveTables {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: "tables/" + t.name + ".jsonl", Method: zip.Deflate, Modified: m.CreatedAt})
		if err != nil {
			return nil, err
		}
		args := repeatArg(projectID, strings.Count(t.where, "?"))
		if t.where == ofTenant {
			args = []any{tenantID}
		}
		n, err := exportRows(ctx, c, t, args, f, ids, func(row map[string]any) {
			if t.name == "files" {
				if id, ok := row["id"].(string); ok {
					fileIDs = append(fileIDs, id)
				}
			}
		})
		if err != nil {
			return nil, fmt.Errorf("save project %s: %w", t.name, err)
		}
		m.Tables[t.name] = n
	}
	// file contents, one query each (a query must not be open meanwhile)
	for _, id := range fileIDs {
		var data []byte
		if err := c.row(ctx, `SELECT data FROM file_blobs WHERE file_id = ?`, id).Scan(&data); err != nil {
			if db.IsNoRows(err) {
				continue
			}
			return nil, err
		}
		f, err := zw.CreateHeader(&zip.FileHeader{Name: "files/" + id, Method: zip.Deflate, Modified: m.CreatedAt})
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(data); err != nil {
			return nil, err
		}
		m.Files++
		m.FileBytes += int64(len(data))
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

func repeatArg(v any, n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// exportRows writes the rows of t matching its condition as JSON lines,
// adds their ids to ids and calls each for every written row.
func exportRows(ctx context.Context, c conn, t archiveTable, args []any, w io.Writer, ids map[string]bool, each func(map[string]any)) (int, error) {
	rows, err := c.query(ctx, `SELECT * FROM `+quoteIdent(t.name)+` WHERE `+t.where, args...)
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
	kinds := make([]colKind, len(cols))
	for i, col := range cols {
		kinds[i] = columnKind(t.name, col, types[i].DatabaseTypeName())
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
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = exportValue(kinds[i], vals[i])
		}
		if entityTables[t.name] {
			if id, _ := row["entity_id"].(string); !ids[id] {
				continue
			}
		}
		sanitizeArchiveRow(t.name, row)
		if id, ok := row["id"].(string); ok && id != "" {
			ids[id] = true
		}
		b, err := json.Marshal(row)
		if err != nil {
			return n, err
		}
		bw.Write(b)
		bw.WriteByte('\n')
		each(row)
		n++
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	return n, bw.Flush()
}

// ---------------------------------------------------------------- load

// ProjectImport are the options of ImportProject.
type ProjectImport struct {
	// Key and Name of the new project; empty: those of the archive.
	Key, Name string
	// DryRun checks the archive and counts the rows, but saves nothing; a
	// key that is taken is reported (KeyTaken) instead of failing.
	DryRun bool
}

// ProjectImportResult is the outcome of ImportProject.
type ProjectImportResult struct {
	Project  *model.Project          `json:"project,omitempty"`
	Manifest *ProjectArchiveManifest `json:"manifest"`
	// Imported counts the loaded rows per table; Skipped the rows that were
	// left out because they refer to something outside the archive (such as
	// a reviewer without an account here).
	Imported map[string]int `json:"imported"`
	Skipped  map[string]int `json:"skipped,omitempty"`
	Files    int            `json:"files"`
	DryRun   bool           `json:"dry_run,omitempty"`
	// KeyTaken (dry runs only): a project with the key exists already, so
	// loading it under that key would fail.
	KeyTaken bool `json:"key_taken,omitempty"`
}

var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// ImportProject creates a new project in a tenant from a project archive,
// in one transaction. Invalid archives return ErrInvalid; an archive of a
// newer schema or a key that is taken return ErrConflict.
func (s *Store) ImportProject(ctx context.Context, tenantID string, r io.ReaderAt, size int64, opt ProjectImport) (*ProjectImportResult, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, Invalidf("the project archive is not a valid ZIP file: %v", err)
	}
	tables, files, mf, err := projectArchiveEntries(zr)
	if err != nil {
		return nil, err
	}
	m, err := readProjectManifest(mf)
	if err != nil {
		return nil, err
	}
	cur, err := s.SchemaVersion(ctx)
	if err != nil {
		return nil, err
	}
	if m.SchemaVersion > cur {
		return nil, Conflictf("the project archive was saved by a newer version of qtestframe (%s, schema %d; this server has schema %d): upgrade before loading it",
			m.AppVersion, m.SchemaVersion, cur)
	}
	if tables["projects"] == nil {
		return nil, Invalidf("the project archive has no project")
	}
	// pass 1: every row id gets a new id
	idMap := map[string]string{}
	var src map[string]any
	budget := &budgetReader{left: MaxRestoreSize}
	for _, t := range projectArchiveTables {
		f := tables[t.name]
		if f == nil {
			continue
		}
		n := 0
		err := scanArchiveTable(f, budget, func(line int, row map[string]any) error {
			n++
			if id, ok := row["id"].(string); ok && id != "" {
				if _, dup := idMap[id]; dup {
					return Invalidf("%s line %d: id %s occurs twice", f.Name, line, id)
				}
				idMap[id] = newID()
			}
			if t.name == "projects" {
				src = row
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if c, ok := m.Tables[t.name]; ok && c != n {
			return nil, Invalidf("table %s has %d rows, the manifest says %d", t.name, n, c)
		}
		if t.name == "projects" && n != 1 {
			return nil, Invalidf("the project archive must hold exactly one project (it has %d)", n)
		}
	}
	oldPID, _ := src["id"].(string)
	oldKey, _ := src["key"].(string)
	newPID := idMap[oldPID]
	if newPID == "" {
		return nil, Invalidf("the project of the archive has no id")
	}
	// the new key and name
	key := strings.ToUpper(strings.TrimSpace(opt.Key))
	if key == "" {
		key = oldKey
	}
	if !projectKeyRe.MatchString(key) {
		return nil, Invalidf("project key must be 2-16 chars, start with a letter and contain only A-Z and 0-9")
	}
	name := strings.TrimSpace(opt.Name)
	if name == "" {
		name, _ = src["name"].(string)
		name = strings.TrimSpace(name)
	}
	if name == "" {
		return nil, Invalidf("project name is required")
	}
	if len([]rune(name)) > 200 {
		return nil, Invalidf("project name may have at most 200 characters")
	}
	var keyRe *regexp.Regexp
	if oldKey != "" && oldKey != key {
		keyRe = regexp.MustCompile(`\b` + regexp.QuoteMeta(oldKey) + `-(REQ|TC|BUG|RISK)-([0-9]+)\b`)
	}
	rewrite := func(v string) string {
		if len(v) >= 36 && strings.Count(v, "-") >= 4 {
			v = uuidRe.ReplaceAllStringFunc(v, func(id string) string {
				if n, ok := idMap[strings.ToLower(id)]; ok {
					return n
				}
				if n, ok := idMap[id]; ok {
					return n
				}
				return id
			})
		}
		if keyRe != nil && strings.Contains(v, oldKey) {
			v = keyRe.ReplaceAllString(v, key+"-$1-$2")
		}
		return v
	}

	res := &ProjectImportResult{Manifest: m, Imported: map[string]int{}, Skipped: map[string]int{}, DryRun: opt.DryRun}
	now := s.now()
	err = s.tx(ctx, func(c conn) error {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM projects WHERE tenant_id = ? AND key = ?`, tenantID, key).Scan(&n); err != nil {
			return err
		}
		if n > 0 && !opt.DryRun {
			return Conflictf("a project with key %s already exists: choose another key", key)
		}
		res.KeyTaken = n > 0
		// a dry run with a taken key loads under a stand-in key (and is
		// rolled back anyway)
		rowKey := key
		if res.KeyTaken {
			rowKey = "QTFDRYRUN" + strings.ToUpper(strings.ReplaceAll(newID(), "-", "")[:7])
		}
		budget := &budgetReader{left: MaxRestoreSize}
		users := map[string]bool{}
		for _, t := range projectArchiveTables {
			f := tables[t.name]
			if f == nil {
				continue
			}
			im, err := newArchiveImporter(ctx, c, t.name)
			if err != nil {
				return err
			}
			im.tenantID, im.idMap, im.users, im.rewrite = tenantID, idMap, users, rewrite
			im.files, im.budget = files, budget
			if t.name == "projects" {
				im.project = func(row map[string]any) {
					row["key"], row["name"], row["created_at"], row["updated_at"] = rowKey, name, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)
				}
			}
			var buffered []restoreRow
			selfCol, selfRef := selfReferences[t.name]
			err = scanArchiveTable(f, budget, func(line int, row map[string]any) error {
				if selfRef {
					buffered = append(buffered, restoreRow{line, row})
					return nil
				}
				return im.load(line, row)
			})
			if err == nil && selfRef {
				for _, r := range parentsFirst(buffered, selfCol) {
					if err = im.load(r.line, r.vals); err != nil {
						break
					}
				}
			}
			im.close()
			if err != nil {
				return err
			}
			if im.loaded > 0 {
				res.Imported[t.name] = im.loaded
			}
			if im.skipped > 0 {
				res.Skipped[t.name] = im.skipped
			}
			res.Files += im.blobs
			// every row must belong to the new project
			if t.name == "projects" && im.loaded != 1 {
				return Invalidf("the project of the archive could not be loaded")
			}
		}
		if opt.DryRun {
			return errDryRun
		}
		return nil
	})
	if errors.Is(err, errDryRun) {
		res.Project = &model.Project{Key: key, Name: name}
		return res, nil
	}
	if err != nil {
		return nil, err
	}
	if res.Project, err = s.GetProject(ctx, tenantID, newPID); err != nil {
		return nil, err
	}
	return res, nil
}

// projectArchiveEntries validates the entry names and sizes of an archive.
func projectArchiveEntries(zr *zip.Reader) (tables, files map[string]*zip.File, manifest *zip.File, err error) {
	known := map[string]bool{}
	for _, t := range projectArchiveTables {
		known[t.name] = true
	}
	tables, files = map[string]*zip.File{}, map[string]*zip.File{}
	var total uint64
	for _, f := range zr.File {
		name := f.Name
		var dup bool
		switch {
		case name == "manifest.json":
			dup = manifest != nil
			manifest = f
		case strings.HasPrefix(name, "tables/") && strings.HasSuffix(name, ".jsonl") && known[strings.TrimSuffix(strings.TrimPrefix(name, "tables/"), ".jsonl")]:
			t := strings.TrimSuffix(strings.TrimPrefix(name, "tables/"), ".jsonl")
			_, dup = tables[t]
			tables[t] = f
		case strings.HasPrefix(name, "files/") && uuidRe.MatchString(name[len("files/"):]) && len(name) == len("files/")+36:
			id := name[len("files/"):]
			_, dup = files[id]
			files[id] = f
		default:
			return nil, nil, nil, Invalidf("the project archive contains an unexpected file %q", name)
		}
		if dup {
			return nil, nil, nil, Invalidf("the project archive contains %q twice", name)
		}
		total += f.UncompressedSize64
		if total > uint64(MaxRestoreSize) {
			return nil, nil, nil, Invalidf("the project archive is too large (more than %d bytes uncompressed)", MaxRestoreSize)
		}
	}
	if manifest == nil {
		return nil, nil, nil, Invalidf("the file is not a project archive (it has no manifest.json)")
	}
	return tables, files, manifest, nil
}

func readProjectManifest(f *zip.File) (*ProjectArchiveManifest, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, Invalidf("manifest.json: %v", err)
	}
	defer rc.Close()
	var m ProjectArchiveManifest
	if err := json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&m); err != nil {
		return nil, Invalidf("manifest.json: %v", err)
	}
	if m.Kind != ProjectArchiveKind {
		return nil, Invalidf("the file is not a project archive (a full backup is restored under Administration → Backup)")
	}
	if m.FormatVersion != ProjectArchiveFormatVersion {
		return nil, Invalidf("unsupported project archive format version %d (want %d)", m.FormatVersion, ProjectArchiveFormatVersion)
	}
	if m.Tables == nil {
		m.Tables = map[string]int{}
	}
	return &m, nil
}

// scanArchiveTable decodes the JSON lines of a table entry.
func scanArchiveTable(f *zip.File, budget *budgetReader, fn func(line int, row map[string]any) error) error {
	rc, err := f.Open()
	if err != nil {
		return Invalidf("%s: %v", f.Name, err)
	}
	defer rc.Close()
	budget.r = rc
	sc := bufio.NewScanner(budget)
	sc.Buffer(make([]byte, 0, min(64<<10, MaxRestoreLine)), MaxRestoreLine)
	line := 0
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
			return Invalidf("%s line %d: invalid JSON object", f.Name, line)
		}
		if err := fn(line, row); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		switch {
		case errors.Is(err, bufio.ErrTooLong):
			return Invalidf("%s line %d: row exceeds %d bytes", f.Name, line+1, MaxRestoreLine)
		case errors.Is(err, errBudget):
			return Invalidf("the project archive is too large (more than %d bytes uncompressed)", MaxRestoreSize)
		}
		return Invalidf("%s: %v", f.Name, err)
	}
	return nil
}

// foreignKey is a column referencing another table.
type foreignKey struct {
	col, ref string
	nullable bool
}

// foreignKeys discovers the foreign keys of a table in the database.
func foreignKeys(ctx context.Context, c conn, table string) ([]foreignKey, error) {
	q := `SELECT f."from", f."table", i."notnull" = 0 FROM pragma_foreign_key_list(?) f JOIN pragma_table_info(?) i ON i.name = f."from"`
	args := []any{table, table}
	if c.d == db.Postgres {
		q = `SELECT a.attname, cf.relname, NOT a.attnotnull
			FROM pg_constraint con
			JOIN pg_class c ON c.oid = con.conrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			JOIN pg_class cf ON cf.oid = con.confrelid
			JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ANY (con.conkey)
			WHERE con.contype = 'f' AND n.nspname = current_schema() AND c.relname = ?`
		args = []any{table}
	}
	rows, err := c.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []foreignKey
	for rows.Next() {
		var fk foreignKey
		if err := rows.Scan(&fk.col, &fk.ref, &fk.nullable); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}

// archiveImporter loads the rows of one table of an archive.
type archiveImporter struct {
	*inserter
	fks      []foreignKey
	tenantID string
	idMap    map[string]string
	users    map[string]bool
	rewrite  func(string) string
	project  func(map[string]any)
	files    map[string]*zip.File
	budget   *budgetReader

	loaded, skipped, blobs int
}

func newArchiveImporter(ctx context.Context, c conn, table string) (*archiveImporter, error) {
	cols, order, err := targetColumns(ctx, c, table)
	if err != nil {
		return nil, fmt.Errorf("load project %s: %w", table, err)
	}
	fks, err := foreignKeys(ctx, c, table)
	if err != nil {
		return nil, fmt.Errorf("load project %s: %w", table, err)
	}
	return &archiveImporter{
		inserter: &inserter{ctx: ctx, c: c, table: table, cols: cols, order: order, stmts: map[string]*sql.Stmt{}},
		fks:      fks,
	}, nil
}

// userExists reports whether an account exists (cached).
func (im *archiveImporter) userExists(id string) (bool, error) {
	if ok, seen := im.users[id]; seen {
		return ok, nil
	}
	var n int
	if err := im.c.row(im.ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, id).Scan(&n); err != nil {
		return false, err
	}
	im.users[id] = n > 0
	return n > 0, nil
}

// load rewrites and inserts one row (or skips it).
func (im *archiveImporter) load(line int, row map[string]any) error {
	t := im.table
	// columns that this database does not have (an archive of an older
	// version may lack some; those get their defaults)
	for k := range row {
		if _, ok := im.cols[k]; !ok {
			delete(row, k)
		}
	}
	oldID, _ := row["id"].(string)
	// skip leaves the row out; rows referring to it are then left out too
	skip := func() error {
		im.skipped++
		if oldID != "" {
			delete(im.idMap, oldID)
		}
		return nil
	}
	if col, ok := mustMapColumns[t]; ok {
		if v, _ := row[col].(string); im.idMap[v] == "" {
			return skip()
		}
	}
	// foreign keys: to rows of the archive (else the row is left out or the
	// reference cleared), to accounts that exist here, to the organisation
	for _, fk := range im.fks {
		v, ok := row[fk.col].(string)
		if !ok || v == "" {
			continue
		}
		switch fk.ref {
		case "tenants":
			row[fk.col] = im.tenantID
		case "users":
			ok, err := im.userExists(v)
			if err != nil {
				return err
			}
			if !ok {
				return skip()
			}
		default:
			if _, mapped := im.idMap[v]; mapped {
				continue
			}
			if fk.nullable && fk.col != "project_id" {
				row[fk.col] = nil
				continue
			}
			return skip()
		}
	}
	for k, v := range row {
		if s, ok := v.(string); ok && !blobColumns[t+"."+k] {
			row[k] = im.rewrite(s)
		}
	}
	if _, ok := im.cols["tenant_id"]; ok {
		row["tenant_id"] = im.tenantID
	}
	sanitizeArchiveRow(t, row)
	if im.project != nil {
		im.project(row)
	}
	if err := im.insert(line, row); err != nil {
		return err
	}
	im.loaded++
	if t == "files" && oldID != "" {
		return im.loadBlob(oldID, row)
	}
	return nil
}

// loadBlob inserts the content of a file (files/<old id>) and checks its
// size and checksum.
func (im *archiveImporter) loadBlob(oldID string, row map[string]any) error {
	f := im.files[oldID]
	if f == nil {
		return Invalidf("the project archive lacks the content of file %s (files/%s)", row["name"], oldID)
	}
	rc, err := f.Open()
	if err != nil {
		return Invalidf("files/%s: %v", oldID, err)
	}
	defer rc.Close()
	im.budget.r = rc
	data, err := io.ReadAll(im.budget)
	if err != nil {
		if errors.Is(err, errBudget) {
			return Invalidf("the project archive is too large (more than %d bytes uncompressed)", MaxRestoreSize)
		}
		return Invalidf("files/%s: %v", oldID, err)
	}
	if want, ok := row["sha256"].(string); ok && want != "" {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), want) {
			return Invalidf("files/%s: content does not match its checksum", oldID)
		}
	}
	if _, err := im.c.exec(im.ctx, `INSERT INTO file_blobs (file_id, data) VALUES (?, ?)`, row["id"], data); err != nil {
		return err
	}
	im.blobs++
	return nil
}
