-- Connections a project imports requirements from (see internal/tracker
-- source.go and docs/user-guide "Importing from Jira and Azure DevOps"):
-- JSON {"jira": {url, user, token, jql}, "azure_devops": {url, project,
-- token, area_path, work_item_types}}. The tokens are encrypted like the
-- defect tracker's token and never returned by the API. The defect
-- tracker settings (projects.defect_tracker) gain Request Tracker (type
-- "rt", with queue and labels_field) without a schema change.
ALTER TABLE projects ADD COLUMN import_sources TEXT NOT NULL DEFAULT '{}';
