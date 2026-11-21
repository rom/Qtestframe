-- Runtime system settings (Administration → System settings, super
-- administrators only): database overrides of a whitelisted set of
-- configuration keys (see internal/settings). The effective value of a
-- setting is the override here, else the environment / configuration file
-- value, else the built-in default. Settings locked by the operator
-- (system_settings.locked in the file, or set by a QTF_* variable) ignore
-- their row. value is JSON: a string, number, boolean, a duration string
-- ("12h0m0s") or, for auth.oidc, the list of OIDC providers (client
-- secrets included; the API never returns them).
CREATE TABLE system_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_by  TEXT NOT NULL DEFAULT '',
    updated_at  {{TS}} NOT NULL
);
