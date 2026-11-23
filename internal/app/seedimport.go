package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/api"
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

// SeedDemoReqSync sets up the two-way sync of the demo project (Tracker
// sync): Azure DevOps on, without a schedule (the demo has no token, so a
// "Sync now" shows the error), with new work items imported and comments
// synced; Jira off, pushing requirements tagged "jira". The two linked
// requirements get the state of an earlier sync; one of them has an open
// conflict (its title changed on both sides), and the log has two runs: a
// successful one and one that could not reach Azure DevOps. It runs after
// the administrator exists (the account the sync runs as).
func SeedDemoReqSync(ctx context.Context, st *store.Store, tenantID string, p *model.Project, adminEmail string) error {
	admin, err := st.GetUserByEmail(ctx, adminEmail)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	azure := &store.ReqSyncRow{TenantID: tenantID, ProjectID: p.ID, System: "azure-devops", Enabled: true, RunAs: admin.ID, UpdatedBy: admin.Email,
		Settings: api.DefaultSyncSettings("azure-devops")}
	azure.Settings.ImportNew, azure.Settings.Comments, azure.Settings.Links = true, true, true
	since := time.Now().UTC().Add(-14 * 24 * time.Hour)
	azure.Settings.CommentsSince = &since
	if err := st.SaveReqSync(ctx, azure, nil); err != nil {
		return err
	}
	jira := &store.ReqSyncRow{TenantID: tenantID, ProjectID: p.ID, System: "jira", RunAs: admin.ID, UpdatedBy: admin.Email, IntervalMinutes: 30,
		Settings: api.DefaultSyncSettings("jira")}
	jira.Settings.ConflictPolicy = model.SyncTrackerWins
	jira.Settings.Push = model.ReqSyncPush{Create: true, Tag: "jira", ProjectKey: "BANK", ItemType: "Story"}
	if err := st.SaveReqSync(ctx, jira, nil); err != nil {
		return err
	}
	refs, err := st.ListExternalRefs(ctx, tenantID, p.ID, "azure-devops")
	if err != nil {
		return err
	}
	synced := time.Now().UTC().Add(-26 * time.Hour)
	var entries []model.ReqSyncEntry
	for i := range refs {
		ref := &refs[i]
		rq, err := st.GetRequirement(ctx, tenantID, ref.EntityID)
		if err != nil {
			continue
		}
		vals := map[string]string{"title": strings.TrimSpace(rq.Title), "status": rq.Status, "priority": rq.Priority}
		remote := map[string]string{"title": vals["title"], "status": vals["status"], "priority": vals["priority"]}
		if i == 1 {
			// changed on both sides since: an open conflict
			vals["title"] = rq.Title + " (draft)"
			remote["title"] = vals["title"]
			c := &model.ReqSyncConflict{ProjectID: p.ID, System: "azure-devops", RequirementID: rq.ID, ExternalID: ref.ExternalID, LocalVersion: rq.Version, RemoteVersion: "7",
				Fields: []model.ReqSyncFieldDiff{{Field: "title", Base: vals["title"], Local: rq.Title, Remote: rq.Title + " (as agreed with the bank)"}}}
			if err := st.SaveReqSyncConflict(ctx, tenantID, c); err != nil {
				return err
			}
		}
		ref.Sync = &store.SyncState{Local: vals, Remote: remote}
		ref.RemoteVersion, ref.SyncedAt = "6", &synced
		if err := st.UpdateSyncRef(ctx, tenantID, ref); err != nil {
			return err
		}
		entries = append(entries, model.ReqSyncEntry{RequirementID: rq.ID, RequirementKey: rq.Key, Title: rq.Title, ExternalID: ref.ExternalID,
			Action: []string{"pull", "push"}[i%2], Fields: []string{"status"}, Changes: map[string]string{"status": "draft → " + rq.Status}})
	}
	fin := synced.Add(4 * time.Second)
	ok := &model.ReqSyncRun{ProjectID: p.ID, System: "azure-devops", Trigger: "manual", Status: "ok", StartedBy: admin.Email, StartedAt: synced, FinishedAt: &fin,
		Summary: model.ReqSyncSummary{Pulled: 1, Pushed: 1}, Entries: entries}
	if err := st.SaveReqSyncRun(ctx, tenantID, ok); err != nil {
		return err
	}
	at := synced.Add(20 * time.Hour)
	fin2 := at.Add(time.Second)
	failed := &model.ReqSyncRun{ProjectID: p.ID, System: "azure-devops", Trigger: "webhook", Status: "error", StartedBy: admin.Email, StartedAt: at, FinishedAt: &fin2,
		Error: "cannot reach the tracker: the demo has no Azure DevOps token", Entries: []model.ReqSyncEntry{}}
	if err := st.SaveReqSyncRun(ctx, tenantID, failed); err != nil {
		return err
	}
	return st.FinishReqSync(ctx, azure.ID, fin2, "error", failed.Error, 1, nil)
}
