-- Similarity checks (internal/similarity): the cache of the optional
-- embeddings service. One vector per item and model, for the version and
-- text it was computed from (text_hash); a changed item gets a new vector
-- on the next check. vector is base64 of little-endian float32 values.
-- A cache: not part of backups or project archives (computed again).
CREATE TABLE similarity_embeddings (
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    item_type   TEXT NOT NULL,
    item_id     TEXT NOT NULL,
    model       TEXT NOT NULL,
    version     INTEGER NOT NULL DEFAULT 0,
    text_hash   TEXT NOT NULL,
    vector      TEXT NOT NULL,
    created_at  {{TS}} NOT NULL,
    PRIMARY KEY (item_type, item_id, model)
);
CREATE INDEX similarity_embeddings_project ON similarity_embeddings (project_id, item_type, model);
