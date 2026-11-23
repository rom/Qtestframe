package model

import (
	"errors"
	"time"
)

// Test cycles and their planning (migration 0041).

// Cycle statuses.
const (
	CyclePlanned = "planned"
	CycleActive  = "active"
	CycleClosed  = "closed"
)

// CycleStatuses are the states of a test cycle.
var CycleStatuses = []string{CyclePlanned, CycleActive, CycleClosed}

// TestCycle groups the test executions planned for a goal ("Sprint 14
// regression"): the tests to run (its plan, TestCycleTest), who runs them
// and how long they should take. The cycle creates its runs and tracks
// their results.
type TestCycle struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	Goal        string `json:"goal"`
	Description string `json:"description"`
	Status      string `json:"status"`
	// OwnerID is the user responsible for the cycle (usually the test lead).
	OwnerID string `json:"owner_id"`
	// ReleaseID, VariantID and BuildID: what the cycle tests (optional);
	// its runs get them.
	ReleaseID   *string `json:"release_id"`
	VariantID   *string `json:"variant_id"`
	BuildID     *string `json:"build_id"`
	Environment string  `json:"environment"`
	// EnvironmentID: the environment item (its runs get it too).
	EnvironmentID *string `json:"environment_id"`
	// ConfigurationIDs: the configurations tests are planned in (each test
	// once per configuration); empty: without configurations.
	ConfigurationIDs []string `json:"configuration_ids"`
	// StartDate and EndDate (YYYY-MM-DD) bound the cycle (and its burndown).
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	// ScheduledStartAt: when the cycle starts by itself (planned → active,
	// its run created); nil: started by hand.
	ScheduledStartAt *time.Time `json:"scheduled_start_at"`
	// ScheduleID: the recurring schedule that created the cycle.
	ScheduleID *string    `json:"schedule_id"`
	Version    int        `json:"version"`
	StartedAt  *time.Time `json:"started_at"`
	ClosedAt   *time.Time `json:"closed_at"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	// Derived.
	ReleaseName string        `json:"release_name,omitempty"`
	VariantName string        `json:"variant_name,omitempty"`
	BuildLabel  string        `json:"build_label,omitempty"`
	RunIDs      []string      `json:"run_ids"`
	Progress    CycleProgress `json:"progress"`
}

// CycleProgress counts the plan of a cycle: planned tests (rows of the
// plan), those with a final result in the cycle's runs and the results,
// the estimated minutes (all, and of the tests not yet executed) and the
// actual time of the executed ones.
type CycleProgress struct {
	Planned    int `json:"planned"`
	Executed   int `json:"executed"`
	Passed     int `json:"passed"`
	Failed     int `json:"failed"`
	Blocked    int `json:"blocked"`
	Skipped    int `json:"skipped"`
	InProgress int `json:"in_progress"`
	NotRun     int `json:"not_run"`
	// NotInRun: planned tests that no run of the cycle contains yet.
	NotInRun      int     `json:"not_in_run"`
	Unassigned    int     `json:"unassigned"`
	EstimatedMin  int     `json:"estimated_minutes"`
	RemainingMin  int     `json:"remaining_minutes"`
	ActualMS      int64   `json:"actual_ms"`
	EstimatedDone int     `json:"estimated_minutes_executed"`
	PercentDone   float64 `json:"percent_executed"`
	PercentPassed float64 `json:"percent_passed"`
}

// TestCycleTest is one planned execution: a test (in a configuration)
// with its tester and estimate, and how it went in the cycle's runs.
type TestCycleTest struct {
	ID                string  `json:"id"`
	CycleID           string  `json:"cycle_id"`
	TestCaseID        string  `json:"test_case_id"`
	TestKey           string  `json:"test_key"`
	TestTitle         string  `json:"test_title"`
	ConfigurationID   *string `json:"configuration_id"`
	ConfigurationName string  `json:"configuration_name,omitempty"`
	// AssigneeID is the user id of the tester ("" nobody).
	AssigneeID      string    `json:"assignee_id"`
	EstimateMinutes int       `json:"estimate_minutes"`
	Position        int       `json:"position"`
	AddedBy         string    `json:"added_by"`
	AddedAt         time.Time `json:"added_at"`
	// Derived from the cycle's runs: the latest execution of the test (and
	// configuration), its status (not_run when there is none), who
	// executed it, when it finished and how long it took.
	ExecutionID *string    `json:"execution_id"`
	RunID       *string    `json:"run_id"`
	Status      string     `json:"status"`
	ExecutedBy  string     `json:"executed_by,omitempty"`
	FinishedAt  *time.Time `json:"finished_at"`
	DurationMS  int64      `json:"duration_ms"`
}

// Executed reports whether the planned test has a final result.
func (t *TestCycleTest) Executed() bool {
	switch t.Status {
	case ResultPassed, ResultFailed, ResultBlocked, ResultSkipped:
		return true
	}
	return false
}

// CycleTesterLoad is the workload of one tester in a cycle ("" = not
// assigned).
type CycleTesterLoad struct {
	AssigneeID   string `json:"assignee_id"`
	Planned      int    `json:"planned"`
	Executed     int    `json:"executed"`
	Passed       int    `json:"passed"`
	Failed       int    `json:"failed"`
	Blocked      int    `json:"blocked"`
	EstimatedMin int    `json:"estimated_minutes"`
	RemainingMin int    `json:"remaining_minutes"`
	ActualMS     int64  `json:"actual_ms"`
}

// CycleBurndownPoint is one day of a cycle's burndown: the planned tests
// and estimated minutes not yet executed at the end of the day, and the
// ideal line (linear from the plan at the start to 0 at the end date).
type CycleBurndownPoint struct {
	Day string `json:"day"`
	// Remaining and RemainingMin are nil for days after today.
	Remaining    *int    `json:"remaining"`
	RemainingMin *int    `json:"remaining_minutes"`
	Ideal        float64 `json:"ideal"`
	IdealMin     float64 `json:"ideal_minutes"`
}

// CycleReport is the progress of a cycle: the totals, the workload per
// tester and the burndown.
type CycleReport struct {
	Cycle    *TestCycle           `json:"cycle"`
	Testers  []CycleTesterLoad    `json:"testers"`
	Burndown []CycleBurndownPoint `json:"burndown"`
}

// CycleSchedule creates a cycle on a schedule from a template cycle.
type CycleSchedule struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	// NamePattern names the new cycles: {date} is replaced by the start
	// date (YYYY-MM-DD), {n} by the number of the cycle.
	NamePattern string `json:"name_pattern"`
	// TemplateCycleID: the cycle whose plan, testers, estimates, release,
	// variant, build and configurations are copied (nil: none).
	TemplateCycleID *string `json:"template_cycle_id"`
	// Query adds the tests matching it (query language).
	Query   string `json:"query"`
	OwnerID string `json:"owner_id"`
	Schedule
	DurationDays int        `json:"duration_days"`
	AutoStart    bool       `json:"auto_start"`
	Enabled      bool       `json:"enabled"`
	Runs         int        `json:"runs"`
	NextRunAt    *time.Time `json:"next_run_at"`
	LastRunAt    *time.Time `json:"last_run_at"`
	LastCycleID  *string    `json:"last_cycle_id"`
	LastError    string     `json:"last_error"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// Derived.
	TemplateName string `json:"template_name,omitempty"`
}

// Schedule is a daily, weekly or monthly time (as the e-mailed reports).
type Schedule struct {
	Frequency string `json:"frequency"`
	// Weekday 0 (Sunday) – 6 for weekly; DayOfMonth 1–28 for monthly.
	Weekday    int    `json:"weekday"`
	DayOfMonth int    `json:"day_of_month"`
	TimeOfDay  string `json:"time"` // HH:MM
	Timezone   string `json:"timezone"`
}

// NextRun returns the first scheduled time strictly after after, or an
// error when the schedule is invalid.
func (s Schedule) NextRun(after time.Time) (time.Time, error) {
	if s.Timezone == "" || s.Timezone == "Local" {
		return time.Time{}, errors.New("unknown time zone")
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Time{}, errors.New("unknown time zone")
	}
	h, m, ok := ParseTimeOfDay(s.TimeOfDay)
	if !ok {
		return time.Time{}, errors.New("time must be HH:MM")
	}
	t := after.In(loc)
	y, mo, d := t.Date()
	var c time.Time
	switch s.Frequency {
	case FrequencyDaily:
		c = time.Date(y, mo, d, h, m, 0, 0, loc)
		if !c.After(after) {
			c = time.Date(y, mo, d+1, h, m, 0, 0, loc)
		}
	case FrequencyWeekly:
		if s.Weekday < 0 || s.Weekday > 6 {
			return time.Time{}, errors.New("weekday must be 0 (Sunday) to 6")
		}
		ahead := (s.Weekday - int(t.Weekday()) + 7) % 7
		c = time.Date(y, mo, d+ahead, h, m, 0, 0, loc)
		if !c.After(after) {
			c = time.Date(y, mo, d+ahead+7, h, m, 0, 0, loc)
		}
	case FrequencyMonthly:
		if s.DayOfMonth < 1 || s.DayOfMonth > 28 {
			return time.Time{}, errors.New("day of month must be 1 to 28")
		}
		c = time.Date(y, mo, s.DayOfMonth, h, m, 0, 0, loc)
		if !c.After(after) {
			c = time.Date(y, mo+1, s.DayOfMonth, h, m, 0, 0, loc)
		}
	default:
		return time.Time{}, errors.New("frequency must be daily, weekly or monthly")
	}
	return c.UTC(), nil
}

// AssignedCycleTest is a planned, not yet executed test assigned to a
// user in an open (planned or active) cycle: an entry of the user's work.
type AssignedCycleTest struct {
	TestCycleTest
	ProjectID  string `json:"project_id"`
	ProjectKey string `json:"project_key"`
	CycleName  string `json:"cycle_name"`
	CycleState string `json:"cycle_status"`
	EndDate    string `json:"end_date"`
}
