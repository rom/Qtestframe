-- BDD authoring (see docs/user-guide "Gherkin tests").
--
-- test_cases.format: "steps" (preconditions and steps, the default) or
-- "gherkin": the test is the Gherkin feature in test_cases.gherkin (a
-- Feature with an optional Background, Scenarios and Scenario Outlines with
-- Examples, tags and doc strings, in English or Swedish), parsed by
-- internal/gherkin. A run expands it into one iteration per scenario and
-- Examples row, whose step results keep the step's line (step_id
-- "gherkin:<line>").
ALTER TABLE test_cases ADD COLUMN format TEXT NOT NULL DEFAULT 'steps';
ALTER TABLE test_cases ADD COLUMN gherkin TEXT NOT NULL DEFAULT '';

-- The step library of a project: reusable step definitions that Gherkin
-- steps are matched against. pattern is a Cucumber expression ({int},
-- {float}, {word}, {string}, {}, optional (text) and a/b alternatives) or,
-- with kind "regex", a regular expression. keyword ("given", "when",
-- "then" or "" for any) only steers autocompletion.
CREATE TABLE step_definitions (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    keyword      TEXT NOT NULL DEFAULT '',
    pattern      TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'cucumber',
    description  TEXT NOT NULL DEFAULT '',
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL,
    UNIQUE (project_id, pattern)
);
CREATE INDEX step_definitions_project ON step_definitions(project_id);
