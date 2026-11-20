-- Suspect trace links: when the content of a requirement or test changes,
-- its links are marked suspect (since when and why) until someone confirms
-- that the test still verifies the requirement.
ALTER TABLE trace_links ADD COLUMN suspect_since {{TS}};
ALTER TABLE trace_links ADD COLUMN suspect_reason TEXT NOT NULL DEFAULT '';
