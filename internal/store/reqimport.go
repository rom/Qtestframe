package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// ImportItem is a requirement to import with its folder path (names) below
// the import root.
type ImportItem struct {
	Requirement *model.Requirement
	Folder      []string
}

var errDryRun = errors.New("dry run")

// ImportRequirements validates and inserts requirements in one transaction:
// either all are stored or none. Folder paths are resolved below root (nil
// = top level); existing requirement folders with the same name
// (case-insensitive) are reused and missing ones created. With dryRun
// everything is rolled back at the end, so keys and folder counts show what
// an import would do. It returns the number of folders created.
func (s *Store) ImportRequirements(ctx context.Context, tenantID, projectID string, root *string, items []ImportItem, dryRun bool) (int, error) {
	// imported requirements get the defaults of the custom fields; a
	// required field that the file does not fill is left empty
	ctx = lenientCustomFields(ctx)
	for i, it := range items {
		if err := ValidateRequirement(it.Requirement); err != nil {
			return 0, fmt.Errorf("requirement %d (%s): %w", i+1, it.Requirement.Title, err)
		}
	}
	if _, err := s.GetProject(ctx, tenantID, projectID); err != nil {
		return 0, err
	}
	if root != nil && *root == "" {
		root = nil
	}
	now := s.now()
	created := 0
	err := s.tx(ctx, func(c conn) error {
		created = 0
		if err := s.checkFolder(ctx, c, tenantID, projectID, model.KindRequirement, root); err != nil {
			return err
		}
		rows, err := c.query(ctx, `SELECT `+folderCols+` FROM folders WHERE tenant_id = ? AND project_id = ? AND kind = ?`, tenantID, projectID, model.KindRequirement)
		if err != nil {
			return err
		}
		children := map[string]string{} // parent id + "\x00" + lower(name) → id
		for rows.Next() {
			f, err := scanFolder(rows)
			if err != nil {
				rows.Close()
				return err
			}
			parent := ""
			if f.ParentID != nil {
				parent = *f.ParentID
			}
			k := parent + "\x00" + strings.ToLower(f.Name)
			if _, dup := children[k]; !dup {
				children[k] = f.ID
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		resolve := func(path []string) (*string, error) {
			parent := root
			for _, name := range path {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				pid := ""
				if parent != nil {
					pid = *parent
				}
				k := pid + "\x00" + strings.ToLower(name)
				if id, ok := children[k]; ok {
					parent = &id
					continue
				}
				pos, err := nextFolderPos(ctx, c, projectID, model.KindRequirement, parent)
				if err != nil {
					return nil, err
				}
				id := newID()
				if _, err := c.exec(ctx, `INSERT INTO folders (id, tenant_id, project_id, parent_id, kind, name, description, position, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					id, tenantID, projectID, nullStr(parent), model.KindRequirement, name, "", pos, now); err != nil {
					return nil, err
				}
				created++
				children[k] = id
				parent = &id
			}
			return parent, nil
		}
		for _, it := range items {
			r := it.Requirement
			folder, err := resolve(it.Folder)
			if err != nil {
				return err
			}
			r.ProjectID = projectID
			r.FolderID = folder
			r.ID = newID()
			r.Version = 1
			r.CreatedAt, r.UpdatedAt = now, now
			if err := insertRequirement(ctx, c, tenantID, r); err != nil {
				return err
			}
		}
		if dryRun {
			return errDryRun
		}
		return nil
	})
	if errors.Is(err, errDryRun) {
		err = nil
	}
	return created, err
}
