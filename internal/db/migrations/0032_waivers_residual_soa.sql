-- Waivers and deviations: a formal acceptance of requirements that are
-- failed, untested or not fulfilled (waiver) or met by a different
-- solution (deviation), for one release (release_id) or the whole project
-- (NULL). Keys are PROJ-WVR-n. Approvals are electronic signatures over a
-- hash of the waiver's content.
ALTER TABLE projects ADD COLUMN waiver_seq INTEGER NOT NULL DEFAULT 0;
CREATE TABLE waivers (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    seq              INTEGER NOT NULL,
    key              TEXT NOT NULL,
    type             TEXT NOT NULL DEFAULT 'waiver',
    title            TEXT NOT NULL,
    release_id       TEXT REFERENCES releases(id) ON DELETE SET NULL,
    justification    TEXT NOT NULL DEFAULT '',
    compensating     TEXT NOT NULL DEFAULT '',
    conditions       TEXT NOT NULL DEFAULT '',
    risk_id          TEXT REFERENCES risks(id) ON DELETE SET NULL,
    requested_by     TEXT NOT NULL DEFAULT '',
    approvers        TEXT NOT NULL DEFAULT '[]',
    status           TEXT NOT NULL DEFAULT 'draft',
    expires_on       TEXT NOT NULL DEFAULT '',
    decided_at       {{TS}},
    decided_by       TEXT NOT NULL DEFAULT '',
    decision_comment TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    updated_at       {{TS}} NOT NULL,
    UNIQUE (project_id, key)
);
CREATE INDEX waivers_project ON waivers (project_id, status);
CREATE TABLE waiver_requirements (
    waiver_id       TEXT NOT NULL REFERENCES waivers(id) ON DELETE CASCADE,
    requirement_id  TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    PRIMARY KEY (waiver_id, requirement_id)
);
CREATE INDEX waiver_requirements_req ON waiver_requirements (requirement_id);
CREATE TABLE waiver_signoffs (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    waiver_id    TEXT NOT NULL REFERENCES waivers(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL,
    user_email   TEXT NOT NULL,
    user_name    TEXT NOT NULL DEFAULT '',
    decision     TEXT NOT NULL,
    comment      TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    signed_at    {{TS}} NOT NULL,
    UNIQUE (waiver_id, user_id, content_hash)
);

-- Residual risk: likelihood and impact after mitigation (0 = not given),
-- entered by hand with a justification or taking effect when the
-- mitigation is verified; a review date and owner; and an "accept risk"
-- electronic signature over a hash of the residual assessment.
ALTER TABLE risks ADD COLUMN residual_likelihood INTEGER NOT NULL DEFAULT 0;
ALTER TABLE risks ADD COLUMN residual_impact INTEGER NOT NULL DEFAULT 0;
ALTER TABLE risks ADD COLUMN residual_manual INTEGER NOT NULL DEFAULT 0;
ALTER TABLE risks ADD COLUMN residual_justification TEXT NOT NULL DEFAULT '';
ALTER TABLE risks ADD COLUMN review_date TEXT NOT NULL DEFAULT '';
ALTER TABLE risks ADD COLUMN review_owner TEXT NOT NULL DEFAULT '';
ALTER TABLE risks ADD COLUMN accepted_by TEXT NOT NULL DEFAULT '';
ALTER TABLE risks ADD COLUMN accepted_name TEXT NOT NULL DEFAULT '';
ALTER TABLE risks ADD COLUMN accepted_at {{TS}};
ALTER TABLE risks ADD COLUMN acceptance_comment TEXT NOT NULL DEFAULT '';
ALTER TABLE risks ADD COLUMN acceptance_hash TEXT NOT NULL DEFAULT '';
-- the project's risk acceptance threshold: the highest residual level that
-- is acceptable without a sign-off ('' = medium); a release can set its own
-- with the level of its residual_risk exit criterion
ALTER TABLE projects ADD COLUMN risk_threshold TEXT NOT NULL DEFAULT '';

-- Statement of Applicability: per project, reference source and control,
-- whether it applies, why, and who owns it ('' = not decided).
CREATE TABLE soa_decisions (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_id     TEXT NOT NULL,
    control_id    TEXT NOT NULL,
    applicable    TEXT NOT NULL DEFAULT '',
    justification TEXT NOT NULL DEFAULT '',
    owner         TEXT NOT NULL DEFAULT '',
    updated_by    TEXT NOT NULL DEFAULT '',
    updated_at    {{TS}} NOT NULL,
    UNIQUE (project_id, source_id, control_id)
);
