-- Project colour (used for the project icon and accents) and the list of
-- requirement sources offered by the requirement editor (JSON array; empty
-- means the defaults in model.DefaultRequirementSources).
ALTER TABLE projects ADD COLUMN color TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN requirement_sources TEXT NOT NULL DEFAULT '[]';
