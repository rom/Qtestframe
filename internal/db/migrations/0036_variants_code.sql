-- Variants and applicability; links of requirements and tests to code
-- (commits, merge and pull requests) and to CI builds.

-- Variants of a project: product variants, markets or customer
-- configurations, optionally in a hierarchy (parent_id), with feature
-- attributes (JSON {"name": "value"}, inherited by child variants) and the
-- test configurations the variant is tested in (JSON list of
-- configuration ids; [] = every configuration).
CREATE TABLE variants (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    project_id         TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id          TEXT REFERENCES variants(id) ON DELETE SET NULL,
    name               TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    kind               TEXT NOT NULL DEFAULT '',
    attributes         TEXT NOT NULL DEFAULT '{}',
    configuration_ids  TEXT NOT NULL DEFAULT '[]',
    position           INTEGER NOT NULL DEFAULT 0,
    created_by         TEXT NOT NULL DEFAULT '',
    created_at         {{TS}} NOT NULL,
    updated_at         {{TS}} NOT NULL,
    UNIQUE (project_id, name)
);
CREATE INDEX variants_parent ON variants(parent_id);

-- Applicability of a requirement or test (item_type requirement | test):
-- the variants it applies to (JSON list of variant ids, each including its
-- descendants) and/or an applicability expression in the query language
-- evaluated against each variant. No row (or neither set): all variants.
CREATE TABLE item_applicability (
    item_type    TEXT NOT NULL,
    item_id      TEXT NOT NULL,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    variant_ids  TEXT NOT NULL DEFAULT '[]',
    expression   TEXT NOT NULL DEFAULT '',
    updated_by   TEXT NOT NULL DEFAULT '',
    updated_at   {{TS}} NOT NULL,
    PRIMARY KEY (item_type, item_id)
);
CREATE INDEX item_applicability_project ON item_applicability(project_id);

-- CI builds: the CI's build id (external_id, unique per project), its
-- number, the commit, branch and pipeline URL. Runs whose results were
-- uploaded for a build reference it.
CREATE TABLE builds (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    external_id   TEXT NOT NULL,
    number        TEXT NOT NULL DEFAULT '',
    commit_sha    TEXT NOT NULL DEFAULT '',
    branch        TEXT NOT NULL DEFAULT '',
    pipeline_url  TEXT NOT NULL DEFAULT '',
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    {{TS}} NOT NULL,
    updated_at    {{TS}} NOT NULL,
    UNIQUE (project_id, external_id)
);

-- The variant a run tests (NULL: variant-neutral) and the build its
-- results belong to.
ALTER TABLE test_runs ADD COLUMN variant_id TEXT REFERENCES variants(id) ON DELETE SET NULL;
ALTER TABLE test_runs ADD COLUMN build_id TEXT REFERENCES builds(id) ON DELETE SET NULL;
CREATE INDEX test_runs_build ON test_runs(build_id);

-- Commits and merge / pull requests (kind commit | merge_request) from a
-- GitLab or GitHub webhook, or added by hand (provider manual). ref is the
-- commit SHA or the MR / PR number; status open | merged | closed for
-- merge requests.
CREATE TABLE code_changes (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL,
    provider       TEXT NOT NULL,
    repository     TEXT NOT NULL DEFAULT '',
    ref            TEXT NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    url            TEXT NOT NULL DEFAULT '',
    author         TEXT NOT NULL DEFAULT '',
    branch         TEXT NOT NULL DEFAULT '',
    target_branch  TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT '',
    occurred_at    {{TS}} NOT NULL,
    created_at     {{TS}} NOT NULL,
    updated_at     {{TS}} NOT NULL,
    UNIQUE (project_id, provider, repository, kind, ref)
);
CREATE INDEX code_changes_time ON code_changes(project_id, occurred_at);

-- The requirements and tests a code change refers to (source auto: a key
-- in the commit or MR text; manual: linked by a user).
CREATE TABLE code_change_items (
    change_id   TEXT NOT NULL REFERENCES code_changes(id) ON DELETE CASCADE,
    item_type   TEXT NOT NULL,
    item_id     TEXT NOT NULL,
    tenant_id   TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'auto',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    PRIMARY KEY (change_id, item_type, item_id)
);
CREATE INDEX code_change_items_item ON code_change_items(item_type, item_id);

-- Code integration of a project: the shared secret of its GitLab and
-- GitHub webhooks (stored encrypted, write-only).
CREATE TABLE code_settings (
    project_id      TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id       TEXT NOT NULL,
    webhook_secret  TEXT NOT NULL DEFAULT '',
    updated_by      TEXT NOT NULL DEFAULT '',
    updated_at      {{TS}} NOT NULL
);
