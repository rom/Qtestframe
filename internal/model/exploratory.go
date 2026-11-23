package model

import "time"

// Exploratory testing, session based (SBTM): a session has a charter (a
// mission and a scope of areas, requirements and tests), a tester and a
// time box. The tester starts, pauses and stops the session's timer (the
// time is kept by the server), takes timestamped notes, attaches files and
// raises defects from notes. A debrief closes the session: a summary, an
// estimate of the coverage of the charter, the split of the session time
// into set-up, testing and bug investigation, and an outcome. When the
// project allows it (VerificationSettings.ExploratoryEvidence), a debriefed
// session is verification evidence of kind exploratory for the
// requirements in its scope.

// KindExploratory names exploratory sessions (keys PROJ-EXP-n; the entity
// type of attachments, defect links, revisions and the audit log).
const KindExploratory = "exploratory_session"

// Session states and the rest of the vocabulary.
var (
	// ExploratoryStatuses: planned → in_progress ⇄ paused → completed →
	// debriefed.
	ExploratoryStatuses = []string{"planned", "in_progress", "paused", "completed", "debriefed"}
	// ExploratoryOpenStatuses are the states of a session still to be done
	// or under way (a tester's work list).
	ExploratoryOpenStatuses = []string{"planned", "in_progress", "paused"}
	// ExploratoryNoteTypes are the types of a note.
	ExploratoryNoteTypes = []string{"observation", "question", "issue", "idea", "bug"}
	// ExploratoryOutcomes of a debrief: passed (the scope works as
	// required), failed (problems found that fail the scope), inconclusive
	// (it could not be judged).
	ExploratoryOutcomes = []string{"passed", "failed", "inconclusive"}
	// ExploratoryScopeTypes are the kinds of items in a charter's scope;
	// folders are the areas.
	ExploratoryScopeTypes = []string{"folder", KindRequirement, KindTest}
	// ExploratoryTimeboxes are the usual time boxes (minutes); any value
	// from 5 to 480 is accepted.
	ExploratoryTimeboxes = []int{30, 60, 90}
)

// ExploratorySession is one exploratory test session.
type ExploratorySession struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Key       string `json:"key"`
	// Title is the short name of the charter; Mission what to explore,
	// with what and to find what.
	Title   string `json:"title"`
	Mission string `json:"mission"`
	// Tester: the e-mail of the person doing the session ("" = not
	// assigned yet).
	Tester         string `json:"tester"`
	TimeboxMinutes int    `json:"timebox_minutes"`
	Status         string `json:"status"`
	// EnvironmentID / EnvironmentName: the environment explored (nil none).
	EnvironmentID   *string `json:"environment_id"`
	EnvironmentName string  `json:"environment_name,omitempty"`
	// The timer: StartedAt the first start, EndedAt the stop; while it runs
	// RunningSince is set. ElapsedSeconds is the time recorded at the moment
	// of reading (including the running part).
	StartedAt      *time.Time `json:"started_at"`
	RunningSince   *time.Time `json:"running_since"`
	ElapsedSeconds int        `json:"elapsed_seconds"`
	EndedAt        *time.Time `json:"ended_at"`
	// Scope of the charter.
	Scope []ExploratoryScopeItem `json:"scope"`
	// Debrief.
	Debrief *ExploratoryDebrief `json:"debrief"`
	// Version counts the changes (history).
	Version   int       `json:"version"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Derived: the number of notes, of notes per type, of bugs (bug notes
	// and notes a defect was raised from) and of defects linked to the
	// session.
	NoteCount  int            `json:"note_count"`
	NoteTypes  map[string]int `json:"note_types"`
	BugCount   int            `json:"bug_count"`
	Defects    []DefectLink   `json:"defects,omitempty"`
	ProjectKey string         `json:"project_key,omitempty"`
}

// RequirementIDs returns the requirements in the session's scope.
func (s *ExploratorySession) RequirementIDs() []string { return s.scopeIDs(KindRequirement) }

// TestIDs returns the tests in the session's scope.
func (s *ExploratorySession) TestIDs() []string { return s.scopeIDs(KindTest) }

// FolderIDs returns the areas (folders) in the session's scope.
func (s *ExploratorySession) FolderIDs() []string { return s.scopeIDs("folder") }

func (s *ExploratorySession) scopeIDs(kind string) []string {
	out := []string{}
	for _, it := range s.Scope {
		if it.Type == kind {
			out = append(out, it.ID)
		}
	}
	return out
}

// ExploratoryScopeItem is an area (folder), requirement or test in a
// charter's scope.
type ExploratoryScopeItem struct {
	// Type: folder | requirement | test.
	Type string `json:"type"`
	ID   string `json:"id"`
	// Derived: key (requirements, tests) or path (folders) and title.
	Key   string `json:"key,omitempty"`
	Title string `json:"title,omitempty"`
}

// ExploratoryDebrief closes a session.
type ExploratoryDebrief struct {
	Summary string `json:"summary"`
	// CoveragePct: the estimated coverage of the charter (0–100).
	CoveragePct int `json:"coverage_pct"`
	// The SBTM time split of the session in percent (sums to 100): set-up
	// (preparing the session and the environment), testing (test design
	// and execution), bug investigation and reporting.
	SetupPct   int `json:"setup_pct"`
	TestingPct int `json:"testing_pct"`
	BugPct     int `json:"bug_pct"`
	// Outcome: passed | failed | inconclusive.
	Outcome string `json:"outcome"`
	// By: who debriefed (e-mail; a reviewer when not the tester), Name the
	// display name, Comment the reviewer's remarks.
	By      string    `json:"by"`
	Name    string    `json:"name"`
	Comment string    `json:"comment"`
	At      time.Time `json:"at"`
	// Reviewed: debriefed by someone other than the tester.
	Reviewed bool `json:"reviewed"`
}

// ExploratoryNote is a note taken during a session.
type ExploratoryNote struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	// Type: observation | question | issue | idea | bug.
	Type string `json:"type"`
	Body string `json:"body"`
	// ElapsedSeconds: the session time when the note was taken.
	ElapsedSeconds int `json:"elapsed_seconds"`
	// FileIDs: attachments of the session the note refers to.
	FileIDs []string  `json:"file_ids"`
	Files   []FileRef `json:"files"`
	// DefectID: the defect raised from the note (DefectKey derived).
	DefectID  *string   `json:"defect_id"`
	DefectKey string    `json:"defect_key,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExploratoryMetrics are the figures of a project's sessions.
type ExploratoryMetrics struct {
	Sessions  int            `json:"sessions"`
	ByStatus  map[string]int `json:"by_status"`
	ByOutcome map[string]int `json:"by_outcome"`
	// Hours of session time (all sessions), Bugs found (bug notes and notes
	// a defect was raised from) and BugsPerHour (0 without session time).
	Hours       float64 `json:"hours"`
	Bugs        int     `json:"bugs"`
	BugsPerHour float64 `json:"bugs_per_hour"`
	// Average time split and coverage of the debriefed sessions (-1 when
	// none was given).
	AvgSetupPct    float64 `json:"avg_setup_pct"`
	AvgTestingPct  float64 `json:"avg_testing_pct"`
	AvgBugPct      float64 `json:"avg_bug_pct"`
	AvgCoveragePct float64 `json:"avg_coverage_pct"`
	// ByArea: per area (folder in a session's scope; "" = sessions without
	// an area), ByTester per tester.
	ByArea   []ExploratoryGroup `json:"by_area"`
	ByTester []ExploratoryGroup `json:"by_tester"`
}

// ExploratoryGroup is a row of the metrics: sessions, hours and bugs of an
// area or a tester.
type ExploratoryGroup struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Sessions    int     `json:"sessions"`
	Hours       float64 `json:"hours"`
	Bugs        int     `json:"bugs"`
	BugsPerHour float64 `json:"bugs_per_hour"`
}
