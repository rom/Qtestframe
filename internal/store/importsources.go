package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Requirement import sources of a project (projects.import_sources): the
// Jira instance and the Azure DevOps project requirements are imported
// from. The tokens are encrypted at rest; an empty token on save keeps
// the stored one unless it is cleared.

// ImportSources returns a project's import sources including the tokens
// (for the server's own use; the API masks them).
func (s *Store) ImportSources(ctx context.Context, tenantID, projectID string) (*model.ImportSources, error) {
	var raw string
	if err := s.c(ctx).row(ctx, `SELECT import_sources FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&raw); err != nil {
		return nil, mapErr(err, "project")
	}
	var src model.ImportSources
	_ = json.Unmarshal([]byte(raw), &src)
	var err error
	if src.Jira.Token, err = s.unseal(ctx, src.Jira.Token, adImportSource(projectID, "jira")); err != nil {
		return nil, fmt.Errorf("jira import token: %w", err)
	}
	if src.Azure.Token, err = s.unseal(ctx, src.Azure.Token, adImportSource(projectID, "azure_devops")); err != nil {
		return nil, fmt.Errorf("azure devops token: %w", err)
	}
	src.Jira.HasToken, src.Azure.HasToken = src.Jira.Token != "", src.Azure.Token != ""
	return &src, nil
}

// ClearTokens names the tokens to remove on save.
type ClearTokens struct {
	Jira  bool `json:"jira"`
	Azure bool `json:"azure_devops"`
}

func httpURL(u string, httpsOnly bool) bool {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" || p.User != nil || p.RawQuery != "" || p.Fragment != "" {
		return false
	}
	return p.Scheme == "https" || !httpsOnly && p.Scheme == "http"
}

// SaveImportSources validates and stores the import sources.
func (s *Store) SaveImportSources(ctx context.Context, tenantID, projectID string, src *model.ImportSources, clear ClearTokens) error {
	cur, err := s.ImportSources(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	j, a := &src.Jira, &src.Azure
	j.URL = strings.TrimRight(strings.TrimSpace(j.URL), "/")
	j.User, j.JQL = strings.TrimSpace(j.User), strings.TrimSpace(j.JQL)
	if j.URL != "" && !httpURL(j.URL, false) {
		return Invalidf("the Jira URL must be an http(s) URL, e.g. https://example.atlassian.net")
	}
	a.URL = strings.TrimRight(strings.TrimSpace(a.URL), "/")
	a.Project = strings.Trim(strings.TrimSpace(a.Project), "/")
	if a.URL != "" {
		if !httpURL(a.URL, false) {
			return Invalidf("the Azure DevOps URL must be the http(s) URL of the organization or collection, e.g. https://dev.azure.com/example")
		}
		if a.Project == "" || strings.ContainsAny(a.Project, "/\\?#") {
			return Invalidf("project must be the name of the Azure DevOps project")
		}
	} else if a.Project != "" {
		return Invalidf("the Azure DevOps URL is required with a project")
	}
	a.AreaPath, a.WorkItemTypes = strings.TrimSpace(a.AreaPath), strings.TrimSpace(a.WorkItemTypes)
	for name, v := range map[string]string{"jira.user": j.User, "jira.jql": j.JQL, "azure_devops.project": a.Project, "azure_devops.area_path": a.AreaPath, "azure_devops.work_item_types": a.WorkItemTypes, "jira.url": j.URL, "azure_devops.url": a.URL} {
		if utf8.RuneCountInString(v) > 2000 {
			return Invalidf("%s may have at most 2000 characters", name)
		}
	}
	if j.Token == "" && !clear.Jira && j.URL != "" {
		j.Token = cur.Jira.Token
	}
	if a.Token == "" && !clear.Azure && a.URL != "" {
		a.Token = cur.Azure.Token
	}
	if j.URL == "" {
		j.Token, j.User = "", ""
	}
	if a.URL == "" {
		a.Token = ""
	}
	stored := *src
	stored.Jira.HasToken, stored.Azure.HasToken = false, false
	if stored.Jira.Token, err = s.seal(j.Token, adImportSource(projectID, "jira")); err != nil {
		return err
	}
	if stored.Azure.Token, err = s.seal(a.Token, adImportSource(projectID, "azure_devops")); err != nil {
		return err
	}
	raw, _ := json.Marshal(stored)
	_, err = s.c(ctx).exec(ctx, `UPDATE projects SET import_sources = ? WHERE id = ? AND tenant_id = ?`, string(raw), projectID, tenantID)
	j.HasToken, a.HasToken = j.Token != "", a.Token != ""
	return err
}
