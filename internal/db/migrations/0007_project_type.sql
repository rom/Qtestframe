-- Kind of project (general, software, hardware, system, business); software
-- projects get extra features such as prompt generation. tech_context
-- describes the technology (languages, frameworks, architecture) and is
-- included in generated prompts.
ALTER TABLE projects ADD COLUMN type TEXT NOT NULL DEFAULT 'general';
ALTER TABLE projects ADD COLUMN tech_context TEXT NOT NULL DEFAULT '';
