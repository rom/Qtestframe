-- Exploratory testing (session-based test management, SBTM): sessions
-- with a charter (mission and scope), a tester, a time box and a timer kept
-- by the server; timestamped notes taken during the session; a debrief
-- (summary, coverage estimate, time split, outcome). Keys are PROJ-EXP-n.
ALTER TABLE projects ADD COLUMN exploratory_seq INTEGER NOT NULL DEFAULT 0;

-- status: planned → in_progress ⇄ paused → completed → debriefed.
-- The timer: elapsed_seconds is the time recorded up to the last pause or
-- stop; while the session runs, running_since is when it was last started
-- (the elapsed time is then elapsed_seconds + now − running_since).
CREATE TABLE exploratory_sessions (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    seq              INTEGER NOT NULL,
    key              TEXT NOT NULL,
    title            TEXT NOT NULL,
    mission          TEXT NOT NULL DEFAULT '',
    tester           TEXT NOT NULL DEFAULT '',
    timebox_minutes  INTEGER NOT NULL DEFAULT 60,
    status           TEXT NOT NULL DEFAULT 'planned',
    started_at       {{TS}},
    running_since    {{TS}},
    elapsed_seconds  INTEGER NOT NULL DEFAULT 0,
    ended_at         {{TS}},
    -- debrief: coverage estimate and SBTM time split in percent (-1 = not
    -- given), outcome passed | failed | inconclusive ('' = none yet)
    summary          TEXT NOT NULL DEFAULT '',
    coverage_pct     INTEGER NOT NULL DEFAULT -1,
    setup_pct        INTEGER NOT NULL DEFAULT -1,
    testing_pct      INTEGER NOT NULL DEFAULT -1,
    bug_pct          INTEGER NOT NULL DEFAULT -1,
    outcome          TEXT NOT NULL DEFAULT '',
    debriefed_by     TEXT NOT NULL DEFAULT '',
    debriefed_name   TEXT NOT NULL DEFAULT '',
    debrief_comment  TEXT NOT NULL DEFAULT '',
    debriefed_at     {{TS}},
    version          INTEGER NOT NULL DEFAULT 1,
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    updated_at       {{TS}} NOT NULL,
    UNIQUE (project_id, key)
);
CREATE INDEX exploratory_sessions_project ON exploratory_sessions (project_id, status);
CREATE INDEX exploratory_sessions_tester ON exploratory_sessions (tenant_id, tester, status);

-- The charter's scope: areas (folders), requirements and tests
-- (entity_type folder | requirement | test).
CREATE TABLE exploratory_scope (
    session_id   TEXT NOT NULL REFERENCES exploratory_sessions(id) ON DELETE CASCADE,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    PRIMARY KEY (session_id, entity_type, entity_id)
);
CREATE INDEX exploratory_scope_entity ON exploratory_scope (entity_type, entity_id);

-- Notes taken during a session: type observation | question | issue |
-- idea | bug, at elapsed_seconds of session time; file_ids (JSON list)
-- are attachments of the session the note refers to; defect_id the defect
-- raised from the note.
CREATE TABLE exploratory_notes (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    session_id       TEXT NOT NULL REFERENCES exploratory_sessions(id) ON DELETE CASCADE,
    type             TEXT NOT NULL DEFAULT 'observation',
    body             TEXT NOT NULL,
    elapsed_seconds  INTEGER NOT NULL DEFAULT 0,
    file_ids         TEXT NOT NULL DEFAULT '[]',
    defect_id        TEXT REFERENCES defects(id) ON DELETE SET NULL,
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    updated_at       {{TS}} NOT NULL
);
CREATE INDEX exploratory_notes_session ON exploratory_notes (session_id, created_at);

-- Whether debriefed sessions count as verification evidence (kind
-- exploratory) for the requirements in their scope. Off by default.
ALTER TABLE verification_settings ADD COLUMN exploratory_evidence {{BOOL}} NOT NULL DEFAULT {{FALSE}};
