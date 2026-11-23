-- The solution preamble of a software project (JSON, model.SolutionPreamble):
-- languages, stack, environments, constraints and conventions that solution
-- prompts (architecture, design, implementation plan, full solution) start
-- with. Descriptions only: never passwords or keys. Its versions are
-- revisions of entity_type solution_preamble (entity_id = the project).
ALTER TABLE projects ADD COLUMN solution_preamble TEXT NOT NULL DEFAULT '{}';
