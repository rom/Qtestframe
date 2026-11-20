-- SCIM 2.0 user provisioning (RFC 7643/7644). Identity providers
-- authenticate with a SCIM token bound to one tenant; the users and groups
-- they push are tracked here, next to the users and memberships tables,
-- which keep their meaning (see internal/api/scim.go).

-- Bearer tokens for /scim/v2 (SHA-256 hashes; not API tokens).
CREATE TABLE scim_tokens (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    hash        TEXT NOT NULL UNIQUE,
    prefix      TEXT NOT NULL,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    expires_at  {{TS}},
    last_used   {{TS}}
);
CREATE INDEX scim_tokens_tenant ON scim_tokens (tenant_id);

-- Role of provisioned users without a mapped group ('' = no access).
CREATE TABLE scim_settings (
    tenant_id    TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    default_role TEXT NOT NULL DEFAULT 'viewer',
    updated_at   {{TS}} NOT NULL
);

-- A user provisioned into a tenant. While active the user has a
-- membership whose role follows the groups; inactive users have none.
CREATE TABLE scim_users (
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- userName as the provider sent it (the e-mail is users.email)
    user_name    TEXT NOT NULL DEFAULT '',
    external_id  TEXT NOT NULL DEFAULT '',
    given_name   TEXT NOT NULL DEFAULT '',
    family_name  TEXT NOT NULL DEFAULT '',
    active       {{BOOL}} NOT NULL DEFAULT {{TRUE}},
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL,
    PRIMARY KEY (tenant_id, user_id)
);

-- Groups pushed by the identity provider; role is the tenant role the
-- group grants ('' = not mapped), chosen by tenant admins.
CREATE TABLE scim_groups (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    external_id  TEXT NOT NULL DEFAULT '',
    role         TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL
);
CREATE UNIQUE INDEX scim_groups_name ON scim_groups (tenant_id, display_name);

CREATE TABLE scim_group_members (
    group_id   TEXT NOT NULL REFERENCES scim_groups(id) ON DELETE CASCADE,
    tenant_id  TEXT NOT NULL,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at {{TS}} NOT NULL,
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX scim_group_members_user ON scim_group_members (tenant_id, user_id);
