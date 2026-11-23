-- The logotype of a project's reports and generated documents (JSON,
-- model.LogoSettings): its own uploaded image (a files row of kind logo),
-- placement and maximum size, or "off" for no logo. '{}' uses the
-- organisation's logo (tenants.settings → logo).
ALTER TABLE projects ADD COLUMN logo TEXT NOT NULL DEFAULT '{}';
