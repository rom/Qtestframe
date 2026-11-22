-- MITRE D3FEND and EMB3D mappings of project items: JSON lists of ids,
-- checked against the catalogue's lists (internal/catalog/data/d3fend.yaml,
-- emb3d.yaml) and copied from the catalogue item on import.
--   d3fend: the defensive techniques a requirement implements or a test
--           verifies (D3-MFA)
--   emb3d:  the EMB3D mitigations and threats a requirement or test
--           addresses (MID-026, TID-201); the threats that realise a risk
ALTER TABLE requirements ADD COLUMN d3fend TEXT NOT NULL DEFAULT '[]';
ALTER TABLE requirements ADD COLUMN emb3d TEXT NOT NULL DEFAULT '[]';
ALTER TABLE test_cases ADD COLUMN d3fend TEXT NOT NULL DEFAULT '[]';
ALTER TABLE test_cases ADD COLUMN emb3d TEXT NOT NULL DEFAULT '[]';
ALTER TABLE risks ADD COLUMN emb3d TEXT NOT NULL DEFAULT '[]';
