package store

import (
	"context"
	"database/sql"
	"regexp"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

var projectKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,15}$`)

// ---------------------------------------------------------------- projects

const projectCols = `id, tenant_id, key, name, description, defect_url_template, icon, color, requirement_sources, created_at, updated_at`

func scanProject(r interface{ Scan(...any) error }) (*model.Project, error) {
	var p model.Project
	var sources string
	if err := r.Scan(&p.ID, &p.TenantID, &p.Key, &p.Name, &p.Description, &p.DefectURLTemplate, &p.Icon, &p.Color, &sources, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.RequirementSources = decodeTags(sources)
	p.CustomSources = len(p.RequirementSources) > 0
	if !p.CustomSources {
		p.RequirementSources = append([]string{}, model.DefaultRequirementSources...)
	}
	return &p, nil
}

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// MaxRequirementSources bounds a project's source list.
const MaxRequirementSources = 50

// normalizeProjectExtras validates the colour and cleans the source list
// (trimmed, de-duplicated case-insensitively, at most 100 characters each).
func normalizeProjectExtras(p *model.Project) error {
	p.Color = strings.ToLower(strings.TrimSpace(p.Color))
	if p.Color != "" && !colorRe.MatchString(p.Color) {
		return Invalidf("color must be empty or #rrggbb")
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range p.RequirementSources {
		v = strings.TrimSpace(v)
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		if len([]rune(v)) > 100 {
			return Invalidf("a requirement source may have at most 100 characters")
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	if len(out) > MaxRequirementSources {
		return Invalidf("at most %d requirement sources", MaxRequirementSources)
	}
	p.RequirementSources = out
	return nil
}

// storedSources is the JSON stored for a project's sources: [] when they
// equal the defaults (so later default changes reach the project).
func storedSources(p *model.Project) string {
	if !p.CustomSources || equalStrings(p.RequirementSources, model.DefaultRequirementSources) {
		return "[]"
	}
	return encodeTags(p.RequirementSources)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// CreateProject inserts a project; when seed is true default categories
// are created.
func (s *Store) CreateProject(ctx context.Context, tenantID string, p *model.Project, seed bool) error {
	p.Key = strings.ToUpper(strings.TrimSpace(p.Key))
	if !projectKeyRe.MatchString(p.Key) {
		return Invalidf("project key must be 2-16 chars, start with a letter and contain only A-Z and 0-9")
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Invalidf("project name is required")
	}
	if err := normalizeProjectExtras(p); err != nil {
		return err
	}
	defer effectiveSources(p)
	p.ID, p.TenantID = newID(), tenantID
	p.CreatedAt = s.now()
	p.UpdatedAt = p.CreatedAt
	return s.tx(ctx, func(c conn) error {
		_, err := c.exec(ctx, `INSERT INTO projects (id, tenant_id, key, name, description, color, requirement_sources, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.TenantID, p.Key, p.Name, p.Description, p.Color, storedSources(p), p.CreatedAt, p.UpdatedAt)
		if err != nil {
			return mapErr(err, "project key")
		}
		if !seed {
			return nil
		}
		for _, d := range DefaultCategories {
			for _, v := range d.Values {
				_, err := c.exec(ctx, `INSERT INTO categories (id, tenant_id, project_id, scope, dimension, name, color, description) VALUES (?, ?, ?, ?, ?, ?, ?, '')`,
					newID(), tenantID, p.ID, d.Scope, d.Dimension, v.Name, v.Color)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DefaultCategory describes a seeded category dimension.
type DefaultCategory struct {
	Scope     string
	Dimension string
	Values    []struct{ Name, Color string }
}

func dim(scope, d string, kv ...string) DefaultCategory {
	dc := DefaultCategory{Scope: scope, Dimension: d}
	for i := 0; i+1 < len(kv); i += 2 {
		dc.Values = append(dc.Values, struct{ Name, Color string }{kv[i], kv[i+1]})
	}
	return dc
}

// DefaultCategories are seeded into new projects.
var DefaultCategories = []DefaultCategory{
	dim(model.KindTest, "Polarity", "Positive", "#22c55e", "Negative", "#ef4444"),
	dim(model.KindTest, "Interface", "External", "#8b5cf6", "Internal", "#64748b"),
	dim(model.KindTest, "Medium", "File based", "#f59e0b", "Network based", "#0ea5e9"),
	dim(model.KindTest, "Execution", "Manual", "#a855f7", "Automated", "#14b8a6"),
	dim(model.KindTest, "Level", "Unit", "#94a3b8", "Integration", "#3b82f6", "System", "#6366f1", "Acceptance", "#10b981"),
	dim(model.KindRequirement, "Area", "Core", "#3b82f6", "UI", "#ec4899", "Security", "#ef4444", "Operations", "#f97316"),
}

// GetProject returns a project of the tenant.
func (s *Store) GetProject(ctx context.Context, tenantID, id string) (*model.Project, error) {
	p, err := scanProject(s.c().row(ctx, `SELECT `+projectCols+` FROM projects WHERE id = ? AND tenant_id = ?`, id, tenantID))
	return p, mapErr(err, "project")
}

// ListProjects returns all projects of a tenant.
func (s *Store) ListProjects(ctx context.Context, tenantID string) ([]model.Project, error) {
	rows, err := s.c().query(ctx, `SELECT `+projectCols+` FROM projects WHERE tenant_id = ? ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// UpdateProject updates name and description.
func (s *Store) UpdateProject(ctx context.Context, tenantID string, p *model.Project) error {
	if strings.TrimSpace(p.Name) == "" {
		return Invalidf("project name is required")
	}
	p.UpdatedAt = s.now()
	p.DefectURLTemplate = strings.TrimSpace(p.DefectURLTemplate)
	if t := p.DefectURLTemplate; t != "" && (!strings.Contains(t, "{id}") || !(strings.HasPrefix(t, "https://") || strings.HasPrefix(t, "http://"))) {
		return Invalidf("defect URL template must be an http(s) URL containing {id}")
	}
	if err := normalizeProjectExtras(p); err != nil {
		return err
	}
	res, err := s.c().exec(ctx, `UPDATE projects SET name = ?, description = ?, defect_url_template = ?, icon = ?, color = ?, requirement_sources = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		strings.TrimSpace(p.Name), p.Description, p.DefectURLTemplate, p.Icon, p.Color, storedSources(p), p.UpdatedAt, p.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "project")
	}
	effectiveSources(p)
	return nil
}

// effectiveSources sets the source list as the API shows it after a save.
func effectiveSources(p *model.Project) {
	p.CustomSources = storedSources(p) != "[]"
	if !p.CustomSources {
		p.RequirementSources = append([]string{}, model.DefaultRequirementSources...)
	}
}

// SourceCount is how many requirements of a project use a source.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// RequirementSourceUsage lists the sources used by a project's requirements, most used first.
func (s *Store) RequirementSourceUsage(ctx context.Context, tenantID, projectID string) ([]SourceCount, error) {
	rows, err := s.c().query(ctx, `SELECT source, COUNT(*) FROM requirements WHERE tenant_id = ? AND project_id = ? AND source <> '' GROUP BY source ORDER BY COUNT(*) DESC, source`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceCount{}
	for rows.Next() {
		var c SourceCount
		if err := rows.Scan(&c.Source, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteProject deletes a project and everything in it.
func (s *Store) DeleteProject(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		for kind, table := range map[string]string{model.KindRequirement: "requirements", model.KindTest: "test_cases"} {
			rows, err := c.query(ctx, `SELECT id FROM `+table+` WHERE project_id = ? AND tenant_id = ?`, id, tenantID)
			if err != nil {
				return err
			}
			ids, err := scanStrings(rows)
			if err != nil {
				return err
			}
			if err := deleteEntityData(ctx, c, kind, ids); err != nil {
				return err
			}
		}
		res, err := c.exec(ctx, `DELETE FROM projects WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "project")
		}
		return nil
	})
}

// nextKey allocates the next sequential key for kind in a project.
func nextKey(ctx context.Context, c conn, projectID, kind string) (int, string, error) {
	col, infix := "req_seq", "REQ"
	if kind == model.KindTest {
		col, infix = "test_seq", "TC"
	}
	var seq int
	var key string
	err := c.row(ctx, `UPDATE projects SET `+col+` = `+col+` + 1 WHERE id = ? RETURNING `+col+`, key`, projectID).Scan(&seq, &key)
	if err != nil {
		return 0, "", mapErr(err, "project")
	}
	return seq, key + "-" + infix + "-" + itoa(seq), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// ---------------------------------------------------------------- folders

const folderCols = `id, project_id, parent_id, kind, name, description, position, created_at`

func scanFolder(r interface{ Scan(...any) error }) (*model.Folder, error) {
	var f model.Folder
	var parent sql.NullString
	if err := r.Scan(&f.ID, &f.ProjectID, &parent, &f.Kind, &f.Name, &f.Description, &f.Position, &f.CreatedAt); err != nil {
		return nil, err
	}
	f.ParentID = strPtr(parent)
	return &f, nil
}

func validKind(k string) bool { return k == model.KindRequirement || k == model.KindTest }

// CreateFolder inserts a folder at the end of its siblings.
func (s *Store) CreateFolder(ctx context.Context, tenantID string, f *model.Folder) error {
	if !validKind(f.Kind) {
		return Invalidf("folder kind must be requirement or test")
	}
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return Invalidf("folder name is required")
	}
	if _, err := s.GetProject(ctx, tenantID, f.ProjectID); err != nil {
		return err
	}
	if f.ParentID != nil && *f.ParentID != "" {
		p, err := s.GetFolder(ctx, tenantID, *f.ParentID)
		if err != nil {
			return Invalidf("parent folder not found")
		}
		if p.ProjectID != f.ProjectID || p.Kind != f.Kind {
			return Invalidf("parent folder belongs to another project or kind")
		}
	} else {
		f.ParentID = nil
	}
	f.ID = newID()
	f.CreatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		pos, err := nextFolderPos(ctx, c, f.ProjectID, f.Kind, f.ParentID)
		if err != nil {
			return err
		}
		f.Position = pos
		_, err = c.exec(ctx, `INSERT INTO folders (id, tenant_id, project_id, parent_id, kind, name, description, position, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, tenantID, f.ProjectID, nullStr(f.ParentID), f.Kind, f.Name, f.Description, f.Position, f.CreatedAt)
		return err
	})
}

func nextFolderPos(ctx context.Context, c conn, projectID, kind string, parent *string) (int, error) {
	var n sql.NullInt64
	var err error
	if parent == nil {
		err = c.row(ctx, `SELECT MAX(position) FROM folders WHERE project_id = ? AND kind = ? AND parent_id IS NULL`, projectID, kind).Scan(&n)
	} else {
		err = c.row(ctx, `SELECT MAX(position) FROM folders WHERE parent_id = ?`, *parent).Scan(&n)
	}
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return int(n.Int64) + 1, nil
}

// GetFolder returns a folder of the tenant.
func (s *Store) GetFolder(ctx context.Context, tenantID, id string) (*model.Folder, error) {
	f, err := scanFolder(s.c().row(ctx, `SELECT `+folderCols+` FROM folders WHERE id = ? AND tenant_id = ?`, id, tenantID))
	return f, mapErr(err, "folder")
}

// ListFolders returns all folders of a kind in a project ordered by parent
// and position.
func (s *Store) ListFolders(ctx context.Context, tenantID, projectID, kind string) ([]model.Folder, error) {
	q := `SELECT ` + folderCols + ` FROM folders WHERE tenant_id = ? AND project_id = ?`
	args := []any{tenantID, projectID}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, kind)
	}
	rows, err := s.c().query(ctx, q+` ORDER BY position, name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Folder{}
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// UpdateFolder renames a folder.
func (s *Store) UpdateFolder(ctx context.Context, tenantID string, f *model.Folder) error {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return Invalidf("folder name is required")
	}
	res, err := s.c().exec(ctx, `UPDATE folders SET name = ?, description = ? WHERE id = ? AND tenant_id = ?`, f.Name, f.Description, f.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "folder")
	}
	return nil
}

// FolderPaths returns id → "A / B / C" for the given folders.
func FolderPaths(folders []model.Folder) map[string]string {
	byID := map[string]model.Folder{}
	for _, f := range folders {
		byID[f.ID] = f
	}
	out := map[string]string{}
	var path func(id string, depth int) string
	path = func(id string, depth int) string {
		if p, ok := out[id]; ok {
			return p
		}
		f, ok := byID[id]
		if !ok || depth > 64 {
			return ""
		}
		p := f.Name
		if f.ParentID != nil {
			if pp := path(*f.ParentID, depth+1); pp != "" {
				p = pp + " / " + f.Name
			}
		}
		out[id] = p
		return p
	}
	for _, f := range folders {
		path(f.ID, 0)
	}
	return out
}

// Descendants returns the ids of folder id and all of its descendants.
func Descendants(folders []model.Folder, id string) []string {
	children := map[string][]string{}
	for _, f := range folders {
		if f.ParentID != nil {
			children[*f.ParentID] = append(children[*f.ParentID], f.ID)
		}
	}
	out := []string{id}
	seen := map[string]bool{id: true}
	for i := 0; i < len(out); i++ {
		for _, c := range children[out[i]] {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// MoveFolder re-parents a folder (optionally into another project of the
// same tenant) and places it at position among its new siblings.
func (s *Store) MoveFolder(ctx context.Context, tenantID, id string, parentID *string, position int, targetProject string) error {
	f, err := s.GetFolder(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if targetProject == "" {
		targetProject = f.ProjectID
	}
	if _, err := s.GetProject(ctx, tenantID, targetProject); err != nil {
		return err
	}
	if parentID != nil && *parentID == "" {
		parentID = nil
	}
	all, err := s.ListFolders(ctx, tenantID, f.ProjectID, f.Kind)
	if err != nil {
		return err
	}
	subtree := Descendants(all, id)
	if parentID != nil {
		p, err := s.GetFolder(ctx, tenantID, *parentID)
		if err != nil {
			return Invalidf("target folder not found")
		}
		if p.Kind != f.Kind || p.ProjectID != targetProject {
			return Invalidf("target folder belongs to another project or kind")
		}
		for _, d := range subtree {
			if d == *parentID {
				return Conflictf("cannot move a folder into itself or its descendants")
			}
		}
	}
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `UPDATE folders SET parent_id = ? WHERE id = ?`, nullStr(parentID), id); err != nil {
			return err
		}
		if targetProject != f.ProjectID {
			if err := s.moveFolderTree(ctx, c, tenantID, f, subtree, targetProject); err != nil {
				return err
			}
		}
		return reorderFolders(ctx, c, targetProject, f.Kind, parentID, id, position)
	})
}

// moveFolderTree moves a folder subtree and the items in it to another
// project, assigning new keys and remapping categories.
func (s *Store) moveFolderTree(ctx context.Context, c conn, tenantID string, f *model.Folder, subtree []string, target string) error {
	in := placeholders(len(subtree))
	args := anySlice(subtree)
	if _, err := c.exec(ctx, `UPDATE folders SET project_id = ? WHERE id IN (`+in+`)`, append([]any{target}, args...)...); err != nil {
		return err
	}
	table := "requirements"
	if f.Kind == model.KindTest {
		table = "test_cases"
	}
	rows, err := c.query(ctx, `SELECT id FROM `+table+` WHERE folder_id IN (`+in+`)`, args...)
	if err != nil {
		return err
	}
	ids, err := scanStrings(rows)
	if err != nil {
		return err
	}
	return s.moveItemsToProject(ctx, c, tenantID, f.Kind, ids, f.ProjectID, target)
}

// reorderFolders places id at position among the children of parent.
func reorderFolders(ctx context.Context, c conn, projectID, kind string, parent *string, id string, position int) error {
	var rows *sql.Rows
	var err error
	if parent == nil {
		rows, err = c.query(ctx, `SELECT id FROM folders WHERE project_id = ? AND kind = ? AND parent_id IS NULL AND id <> ? ORDER BY position, name`, projectID, kind, id)
	} else {
		rows, err = c.query(ctx, `SELECT id FROM folders WHERE parent_id = ? AND id <> ? ORDER BY position, name`, *parent, id)
	}
	if err != nil {
		return err
	}
	ids, err := scanStrings(rows)
	if err != nil {
		return err
	}
	ids = insertAt(ids, id, position)
	for i, fid := range ids {
		if _, err := c.exec(ctx, `UPDATE folders SET position = ? WHERE id = ?`, i, fid); err != nil {
			return err
		}
	}
	return nil
}

func insertAt(list []string, id string, pos int) []string {
	if pos < 0 || pos > len(list) {
		pos = len(list)
	}
	out := make([]string, 0, len(list)+1)
	out = append(out, list[:pos]...)
	out = append(out, id)
	return append(out, list[pos:]...)
}

// DeleteFolder removes a folder. With cascade the subtree and contained
// items are deleted, otherwise children and items move to the parent.
func (s *Store) DeleteFolder(ctx context.Context, tenantID, id string, cascade bool) error {
	f, err := s.GetFolder(ctx, tenantID, id)
	if err != nil {
		return err
	}
	table := "requirements"
	if f.Kind == model.KindTest {
		table = "test_cases"
	}
	all, err := s.ListFolders(ctx, tenantID, f.ProjectID, f.Kind)
	if err != nil {
		return err
	}
	sub := Descendants(all, id)
	return s.tx(ctx, func(c conn) error {
		if cascade {
			rows, err := c.query(ctx, `SELECT id FROM `+table+` WHERE folder_id IN (`+placeholders(len(sub))+`)`, anySlice(sub)...)
			if err != nil {
				return err
			}
			ids, err := scanStrings(rows)
			if err != nil {
				return err
			}
			if err := deleteEntityData(ctx, c, f.Kind, ids); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `DELETE FROM `+table+` WHERE folder_id IN (`+placeholders(len(sub))+`)`, anySlice(sub)...); err != nil {
				return err
			}
		} else {
			if _, err := c.exec(ctx, `UPDATE `+table+` SET folder_id = ? WHERE folder_id = ?`, nullStr(f.ParentID), id); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `UPDATE folders SET parent_id = ? WHERE parent_id = ?`, nullStr(f.ParentID), id); err != nil {
				return err
			}
		}
		_, err := c.exec(ctx, `DELETE FROM folders WHERE id = ?`, id)
		return err
	})
}

// ---------------------------------------------------------------- categories

const categoryCols = `id, project_id, scope, dimension, name, color, description`

// CreateCategory inserts a category.
func (s *Store) CreateCategory(ctx context.Context, tenantID string, c *model.Category) error {
	if err := validateCategory(c); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, c.ProjectID); err != nil {
		return err
	}
	c.ID = newID()
	_, err := s.c().exec(ctx, `INSERT INTO categories (id, tenant_id, project_id, scope, dimension, name, color, description) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, tenantID, c.ProjectID, c.Scope, c.Dimension, c.Name, c.Color, c.Description)
	return mapErr(err, "category")
}

func validateCategory(c *model.Category) error {
	c.Dimension = strings.TrimSpace(c.Dimension)
	c.Name = strings.TrimSpace(c.Name)
	if !validKind(c.Scope) {
		return Invalidf("category scope must be requirement or test")
	}
	if c.Dimension == "" || c.Name == "" {
		return Invalidf("category dimension and name are required")
	}
	if strings.Contains(c.Dimension, "/") {
		return Invalidf("category dimension must not contain '/'")
	}
	return nil
}

// GetCategory returns a category.
func (s *Store) GetCategory(ctx context.Context, tenantID, id string) (*model.Category, error) {
	var c model.Category
	err := s.c().row(ctx, `SELECT `+categoryCols+` FROM categories WHERE id = ? AND tenant_id = ?`, id, tenantID).
		Scan(&c.ID, &c.ProjectID, &c.Scope, &c.Dimension, &c.Name, &c.Color, &c.Description)
	if err != nil {
		return nil, mapErr(err, "category")
	}
	return &c, nil
}

// ListCategories returns categories of a project, optionally by scope.
func (s *Store) ListCategories(ctx context.Context, tenantID, projectID, scope string) ([]model.Category, error) {
	q := `SELECT ` + categoryCols + ` FROM categories WHERE tenant_id = ? AND project_id = ?`
	args := []any{tenantID, projectID}
	if scope != "" {
		q += ` AND scope = ?`
		args = append(args, scope)
	}
	rows, err := s.c().query(ctx, q+` ORDER BY scope, dimension, name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Category{}
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Scope, &c.Dimension, &c.Name, &c.Color, &c.Description); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCategory stores category changes (scope is immutable).
func (s *Store) UpdateCategory(ctx context.Context, tenantID string, c *model.Category) error {
	old, err := s.GetCategory(ctx, tenantID, c.ID)
	if err != nil {
		return err
	}
	c.Scope = old.Scope
	c.ProjectID = old.ProjectID
	if err := validateCategory(c); err != nil {
		return err
	}
	_, err = s.c().exec(ctx, `UPDATE categories SET dimension = ?, name = ?, color = ?, description = ? WHERE id = ?`, c.Dimension, c.Name, c.Color, c.Description, c.ID)
	return mapErr(err, "category")
}

// DeleteCategory removes a category and its assignments.
func (s *Store) DeleteCategory(ctx context.Context, tenantID, id string) error {
	res, err := s.c().exec(ctx, `DELETE FROM categories WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "category")
	}
	return nil
}

// RenameDimension moves all categories of a dimension into another
// (possibly existing) dimension. Duplicate names are merged.
func (s *Store) RenameDimension(ctx context.Context, tenantID, projectID, scope, from, to string) (int, error) {
	to = strings.TrimSpace(to)
	if to == "" || strings.Contains(to, "/") {
		return 0, Invalidf("new dimension name is invalid")
	}
	cats, err := s.ListCategories(ctx, tenantID, projectID, scope)
	if err != nil {
		return 0, err
	}
	existing := map[string]string{}
	for _, c := range cats {
		if c.Dimension == to {
			existing[c.Name] = c.ID
		}
	}
	n := 0
	err = s.tx(ctx, func(c conn) error {
		for _, cat := range cats {
			if cat.Dimension != from || from == to {
				continue
			}
			n++
			if target, ok := existing[cat.Name]; ok {
				// merge assignments into the existing category
				for _, tbl := range []string{"requirement_categories", "test_case_categories"} {
					col := "requirement_id"
					if tbl == "test_case_categories" {
						col = "test_case_id"
					}
					if _, err := c.exec(ctx, `INSERT INTO `+tbl+` (`+col+`, category_id) SELECT `+col+`, ? FROM `+tbl+` WHERE category_id = ? AND `+col+` NOT IN (SELECT `+col+` FROM `+tbl+` WHERE category_id = ?)`, target, cat.ID, target); err != nil {
						return err
					}
				}
				if _, err := c.exec(ctx, `DELETE FROM categories WHERE id = ?`, cat.ID); err != nil {
					return err
				}
				continue
			}
			if _, err := c.exec(ctx, `UPDATE categories SET dimension = ? WHERE id = ?`, to, cat.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil && n == 0 {
		return 0, mapErr(sql.ErrNoRows, "dimension")
	}
	return n, err
}

// categoryAssignments loads item → category ids for a project.
func categoryAssignments(ctx context.Context, c conn, kind, projectID string) (map[string][]string, error) {
	q := `SELECT rc.requirement_id, rc.category_id FROM requirement_categories rc JOIN requirements r ON r.id = rc.requirement_id WHERE r.project_id = ?`
	if kind == model.KindTest {
		q = `SELECT tc.test_case_id, tc.category_id FROM test_case_categories tc JOIN test_cases t ON t.id = tc.test_case_id WHERE t.project_id = ?`
	}
	rows, err := c.query(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var item, cat string
		if err := rows.Scan(&item, &cat); err != nil {
			return nil, err
		}
		out[item] = append(out[item], cat)
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out, rows.Err()
}

// setCategories replaces category assignments of an item after
// validating that all categories belong to the project and scope.
func setCategories(ctx context.Context, c conn, kind, projectID, itemID string, ids []string) error {
	tbl, col := "requirement_categories", "requirement_id"
	if kind == model.KindTest {
		tbl, col = "test_case_categories", "test_case_id"
	}
	if _, err := c.exec(ctx, `DELETE FROM `+tbl+` WHERE `+col+` = ?`, itemID); err != nil {
		return err
	}
	return addCategories(ctx, c, kind, projectID, itemID, ids)
}

func addCategories(ctx context.Context, c conn, kind, projectID, itemID string, ids []string) error {
	tbl, col := "requirement_categories", "requirement_id"
	if kind == model.KindTest {
		tbl, col = "test_case_categories", "test_case_id"
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM categories WHERE id = ? AND project_id = ? AND scope = ?`, id, projectID, kind).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return Invalidf("category %s does not belong to this project/scope", id)
		}
		if _, err := c.exec(ctx, `DELETE FROM `+tbl+` WHERE `+col+` = ? AND category_id = ?`, itemID, id); err != nil {
			return err
		}
		if _, err := c.exec(ctx, `INSERT INTO `+tbl+` (`+col+`, category_id) VALUES (?, ?)`, itemID, id); err != nil {
			return err
		}
	}
	return nil
}

func removeCategories(ctx context.Context, c conn, kind, itemID string, ids []string) error {
	tbl, col := "requirement_categories", "requirement_id"
	if kind == model.KindTest {
		tbl, col = "test_case_categories", "test_case_id"
	}
	for _, id := range ids {
		if _, err := c.exec(ctx, `DELETE FROM `+tbl+` WHERE `+col+` = ? AND category_id = ?`, itemID, id); err != nil {
			return err
		}
	}
	return nil
}
