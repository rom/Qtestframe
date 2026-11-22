-- Verification correlations: criterion-level verification, verification
-- records (non-test evidence), and allocation to system elements.

-- link_criteria: the evaluation criteria a trace link (test → requirement)
-- verifies, optionally through one step of the test (step_id, '' = the
-- whole test). trace_links.criterion_id stays as the first mapped
-- criterion for older clients.
CREATE TABLE link_criteria (
    link_id       TEXT NOT NULL REFERENCES trace_links(id) ON DELETE CASCADE,
    criterion_id  TEXT NOT NULL REFERENCES evaluation_criteria(id) ON DELETE CASCADE,
    step_id       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (link_id, criterion_id)
);
CREATE INDEX link_criteria_criterion ON link_criteria(criterion_id);
INSERT INTO link_criteria (link_id, criterion_id, step_id)
    SELECT id, criterion_id, '' FROM trace_links WHERE criterion_id IS NOT NULL;

-- verification_records: evidence other than test results (inspection,
-- analysis, demonstration, review, or test done outside the tool) about
-- a criterion of a requirement (criterion_id NULL: the requirement as a
-- whole). A record stops counting after valid_until (YYYY-MM-DD) or, with
-- until_change, when the requirement's content changes (invalidated_at).
CREATE TABLE verification_records (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    project_id         TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    requirement_id     TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    criterion_id       TEXT REFERENCES evaluation_criteria(id) ON DELETE SET NULL,
    method             TEXT NOT NULL,
    result             TEXT NOT NULL,
    performed_by       TEXT NOT NULL DEFAULT '',
    performed_on       TEXT NOT NULL DEFAULT '',
    note               TEXT NOT NULL DEFAULT '',
    valid_until        TEXT NOT NULL DEFAULT '',
    until_change       {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    invalidated_at     {{TS}},
    invalidated_reason TEXT NOT NULL DEFAULT '',
    signoff_by         TEXT NOT NULL DEFAULT '',
    signoff_name       TEXT NOT NULL DEFAULT '',
    signoff_comment    TEXT NOT NULL DEFAULT '',
    signoff_at         {{TS}},
    created_by         TEXT NOT NULL DEFAULT '',
    created_at         {{TS}} NOT NULL,
    updated_at         {{TS}} NOT NULL
);
CREATE INDEX verification_records_req ON verification_records(requirement_id);
CREATE INDEX verification_records_project ON verification_records(project_id);

-- documents of the project archive attached to a verification record
CREATE TABLE verification_record_files (
    record_id  TEXT NOT NULL REFERENCES verification_records(id) ON DELETE CASCADE,
    file_id    TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    PRIMARY KEY (record_id, file_id)
);

-- verification_settings: per project; strict_criteria makes every
-- criterion need its own evidence (a plain test link no longer covers
-- the criteria that have none).
CREATE TABLE verification_settings (
    project_id       TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id        TEXT NOT NULL,
    strict_criteria  {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    updated_by       TEXT NOT NULL DEFAULT '',
    updated_at       {{TS}} NOT NULL
);

-- system_elements: the product breakdown of a project (components,
-- subsystems, suppliers, products), a tree per project.
CREATE TABLE system_elements (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id    TEXT REFERENCES system_elements(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'component',
    description  TEXT NOT NULL DEFAULT '',
    position     INTEGER NOT NULL DEFAULT 0,
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL
);
CREATE INDEX system_elements_project ON system_elements(project_id);

-- allocation of requirements and tests to system elements
CREATE TABLE requirement_elements (
    requirement_id  TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    element_id      TEXT NOT NULL REFERENCES system_elements(id) ON DELETE CASCADE,
    PRIMARY KEY (requirement_id, element_id)
);
CREATE INDEX requirement_elements_element ON requirement_elements(element_id);
CREATE TABLE test_elements (
    test_case_id  TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    element_id    TEXT NOT NULL REFERENCES system_elements(id) ON DELETE CASCADE,
    PRIMARY KEY (test_case_id, element_id)
);
CREATE INDEX test_elements_element ON test_elements(element_id);
