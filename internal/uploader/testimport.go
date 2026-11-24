package uploader

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
)

// TestImportRow is a row of a test import preview.
type TestImportRow struct {
	Index      int      `json:"index"`
	Location   string   `json:"location"`
	ExternalID string   `json:"external_id"`
	Title      string   `json:"title"`
	Folder     string   `json:"folder"`
	Action     string   `json:"action"` // new | update | unchanged | conflict | error
	Key        string   `json:"key"`
	Links      []string `json:"links"`
	Unresolved []string `json:"unresolved"`
	Executions int      `json:"executions"`
	Warnings   []string `json:"warnings"`
	Errors     []string `json:"errors"`
}

// TestImportPreview is what importing a file would do.
type TestImportPreview struct {
	SourceName string `json:"source_name"`
	Format     string `json:"format"`
	Total      int    `json:"total"`
	Summary    struct {
		New       int `json:"new"`
		Update    int `json:"update"`
		Unchanged int `json:"unchanged"`
		Conflict  int `json:"conflict"`
		Error     int `json:"error"`
	} `json:"summary"`
	Rows    []TestImportRow `json:"rows"`
	History struct {
		Runs       int `json:"runs"`
		Executions int `json:"executions"`
		Known      int `json:"known"`
	} `json:"history"`
	Unresolved []string `json:"unresolved_links"`
	Warnings   []string `json:"warnings"`
	Errors     []string `json:"errors"`
}

// TestImportResult is the outcome of an import.
type TestImportResult struct {
	Created        int      `json:"created"`
	Updated        int      `json:"updated"`
	Unchanged      int      `json:"unchanged"`
	Skipped        int      `json:"skipped"`
	FoldersCreated int      `json:"folders_created"`
	LinksAdded     int      `json:"links_added"`
	RunsCreated    int      `json:"runs_created"`
	Results        int      `json:"results_imported"`
	Unresolved     []string `json:"unresolved_links"`
	Warnings       []string `json:"warnings"`
	SkippedRows    []struct {
		Location string `json:"location"`
		Title    string `json:"title"`
		Reason   string `json:"reason"`
	} `json:"skipped_rows"`
}

// TestImport previews (apply false) or applies the import of a test
// export of another tool; fields are the form fields (source, folder_id,
// history …).
func (c *Client) TestImport(ctx context.Context, projectID string, apply bool, name string, data []byte, fields map[string]string, out any) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if v != "" {
			if err := mw.WriteField(k, v); err != nil {
				return err
			}
		}
	}
	w, err := mw.CreateFormFile("file", filepath.Base(name))
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	step := "preview"
	if apply {
		step = "apply"
	}
	return c.do(ctx, http.MethodPost, "/projects/"+url.PathEscape(projectID)+"/tests/import/"+step, &buf, mw.FormDataContentType(), out)
}
