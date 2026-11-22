package app

import (
	"context"
	"strconv"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedImportSources gives the demo project the connections of the
// requirement import (Requirements → Import and exchange → From Jira / From
// Azure DevOps): an example Azure DevOps organization with an area path
// and work item types, and a default JQL query. No tokens: the demo cannot
// reach them, the settings and the import dialog show what is needed. Two
// requirements are recorded as imported from Azure DevOps work items, as
// an earlier import would have left them.
func seedImportSources(ctx context.Context, st *store.Store, tenantID string, p *model.Project, reqs []*model.Requirement) error {
	src := &model.ImportSources{
		Jira:  model.JiraImportSource{URL: "https://example-bank.atlassian.net", User: "qa@example.com", JQL: "project = BANK AND issuetype = Story ORDER BY key"},
		Azure: model.AzureDevOpsSource{URL: "https://dev.azure.com/example-bank", Project: "Online Banking", AreaPath: `Online Banking\Portal`, WorkItemTypes: "User Story, Requirement"},
	}
	if err := st.SaveImportSources(ctx, tenantID, p.ID, src, store.ClearTokens{}); err != nil {
		return err
	}
	for i, r := range reqs {
		if i >= 2 {
			break
		}
		cur, err := st.GetRequirement(ctx, tenantID, r.ID)
		if err != nil {
			return err
		}
		ref := &store.ExternalRef{ProjectID: p.ID, System: "azure-devops", EntityType: model.KindRequirement, EntityID: r.ID,
			ExternalID: strconv.Itoa(1201 + i), Source: "https://dev.azure.com/example-bank/Online%20Banking", Version: cur.Version}
		if err := st.SaveExternalRef(ctx, tenantID, ref); err != nil {
			return err
		}
	}
	return nil
}
