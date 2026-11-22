-- MITRE ATT&CK and ATLAS techniques of a risk: a JSON list of technique
-- ids (T1110.004, T0886, AML.T0051), checked against the catalogue's
-- technique list (internal/catalog/data/attack.yaml, atlas.yaml) and copied
-- from the catalogue risk on import.
ALTER TABLE risks ADD COLUMN attack TEXT NOT NULL DEFAULT '[]';
