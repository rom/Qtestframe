-- Release gates per variant: the variants a release targets (JSON list of
-- variant ids; [] = the release is evaluated without a variant, as before)
-- and the variants a waiver is limited to (JSON list of variant ids, each
-- including its descendants; [] = every variant).
ALTER TABLE releases ADD COLUMN variant_ids TEXT NOT NULL DEFAULT '[]';
ALTER TABLE waivers ADD COLUMN variant_ids TEXT NOT NULL DEFAULT '[]';
