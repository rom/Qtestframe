-- Issues created from requirements in the project's tracker (GitLab, GitHub
-- or Jira) are external item links (target_type 'url') that also remember
-- the issue: the tracker type, its key (group/project#12, owner/repo#12,
-- SHOP-12), the tracker's own status, the state mapped to open or closed,
-- when it was last read, and when the "all tests passed" comment was posted
-- (NULL: not posted, or the tests stopped passing since).
ALTER TABLE item_links ADD COLUMN issue_tracker TEXT NOT NULL DEFAULT '';
ALTER TABLE item_links ADD COLUMN issue_key TEXT NOT NULL DEFAULT '';
ALTER TABLE item_links ADD COLUMN issue_status TEXT NOT NULL DEFAULT '';
ALTER TABLE item_links ADD COLUMN issue_state TEXT NOT NULL DEFAULT '';
ALTER TABLE item_links ADD COLUMN issue_synced_at {{TS}};
ALTER TABLE item_links ADD COLUMN issue_passed_at {{TS}};
