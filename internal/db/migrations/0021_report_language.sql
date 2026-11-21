-- Scheduled reports (report_subscriptions): the language the report and
-- its e-mail are produced in, the UI language of whoever created or last
-- changed the subscription ('' = the owner's preferred language).
ALTER TABLE report_subscriptions ADD COLUMN language TEXT NOT NULL DEFAULT '';
