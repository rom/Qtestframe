-- Optional AI assistance (internal/ai): a project can opt out of it even
-- when the organisation has it on. The configuration of the service is in
-- the system settings (section ai), the organisation switch in the tenant
-- settings; every request is written to the audit log (action ai.request).
ALTER TABLE projects ADD COLUMN ai_disabled {{BOOL}} NOT NULL DEFAULT {{FALSE}};
