-- Tenant and account administration by super administrators (see
-- docs/admin-guide "Organisations" and "Authentication").
--
-- tenants.suspended_at: set while an organisation is suspended; its
-- members cannot open it and its API tokens are refused until it is
-- reactivated (nothing is deleted).
-- tenants.policy (JSON, model.TenantPolicy): limits and rules only super
-- administrators change: quotas (members, projects, storage), AI assistance
-- forbidden, the sign-in methods its members may use and the e-mail
-- domains whose new single sign-on users join it.
ALTER TABLE tenants ADD COLUMN suspended_at {{TS}};
ALTER TABLE tenants ADD COLUMN policy TEXT NOT NULL DEFAULT '{}';

-- users.password_changed_at: when the local password was last set (for
-- auth.password_max_age_days; NULL = unknown, counted from now on).
-- users.password_history (JSON list): hashes of earlier passwords, newest
-- first, for auth.password_history.
ALTER TABLE users ADD COLUMN password_changed_at {{TS}};
ALTER TABLE users ADD COLUMN password_history TEXT NOT NULL DEFAULT '[]';
