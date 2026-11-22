package model

import (
	"slices"
	"time"
)

// Change and defect correlations: change impact across relations, result
// staleness, defects that block requirements and regressions. The settings
// are per project (projects.correlation_settings).

// Impact directions of a relation type: which requirement becomes suspect
// when the requirement at the other end of a relation changes.
const (
	// ImpactIncoming: when the target changes, the source becomes suspect
	// (A refines B: B changed, A is suspect).
	ImpactIncoming = "incoming"
	// ImpactOutgoing: when the source changes, the target becomes suspect.
	ImpactOutgoing = "outgoing"
	// ImpactBoth: either end changing makes the other suspect.
	ImpactBoth = "both"
	// ImpactOff: changes do not travel along the relation type.
	ImpactOff = "off"
)

// ImpactDirections lists the valid directions of CorrelationSettings.RelationImpact.
var ImpactDirections = []string{ImpactIncoming, ImpactOutgoing, ImpactBoth, ImpactOff}

// Regression actions: what happens when a test that verified a resolved or
// closed defect fails again.
const (
	RegressionReopen = "reopen"     // reopen the defect
	RegressionNew    = "new_defect" // create a linked regression defect
	RegressionOff    = "off"        // nothing
)

// RegressionActions lists the valid values of CorrelationSettings.RegressionAction.
var RegressionActions = []string{RegressionReopen, RegressionNew, RegressionOff}

// Defect block levels (see DefectBlockLevel).
const (
	DefectBlocks = "block"
	DefectWarns  = "warn"
)

// DefaultStaleAfterDays is the age after which a result is stale unless a
// project chooses another (0 there turns the age rule off).
const DefaultStaleAfterDays = 90

// CorrelationSettings are a project's rules for change impact, result
// staleness, blocking defects and regressions. Every project starts with
// DefaultCorrelationSettings.
type CorrelationSettings struct {
	// RelationImpact: relation type → ImpactDirections value. Types not
	// listed are ImpactOff.
	RelationImpact map[string]string `json:"relation_impact"`
	// FlagIssues marks the tracker issues of a changed requirement out of
	// date (until they are updated from it).
	FlagIssues bool `json:"flag_issues"`
	// StaleAfterDays: a result older than this is stale (0 = never by age).
	StaleAfterDays int `json:"stale_after_days"`
	// StaleOnChange: a result is stale when its test or a requirement the
	// test verifies changed after the result was recorded.
	StaleOnChange bool `json:"stale_on_change"`
	// BlockingSeverities: open defects of these severities, linked to a
	// requirement, a test verifying it or an execution of such a test,
	// block the requirement; WarningSeverities only warn.
	BlockingSeverities []string `json:"blocking_severities"`
	WarningSeverities  []string `json:"warning_severities"`
	// RegressionAction: RegressionActions value.
	RegressionAction string `json:"regression_action"`
}

// DefaultCorrelationSettings returns the settings of a project that has
// not chosen its own.
func DefaultCorrelationSettings() CorrelationSettings {
	return CorrelationSettings{
		RelationImpact:     map[string]string{"refines": ImpactIncoming, "derived_from": ImpactIncoming, "depends_on": ImpactIncoming},
		FlagIssues:         true,
		StaleAfterDays:     DefaultStaleAfterDays,
		StaleOnChange:      true,
		BlockingSeverities: []string{"critical", "major"},
		WarningSeverities:  []string{"minor"},
		RegressionAction:   RegressionReopen,
	}
}

// ImpactDirection returns the impact direction of a relation type.
func (c *CorrelationSettings) ImpactDirection(relType string) string {
	if d, ok := c.RelationImpact[relType]; ok && d != "" {
		return d
	}
	return ImpactOff
}

// DefectBlockLevel returns DefectBlocks, DefectWarns or "" for an open
// defect of a severity (closed defects never block).
func (c *CorrelationSettings) DefectBlockLevel(severity string) string {
	switch {
	case slices.Contains(c.BlockingSeverities, severity):
		return DefectBlocks
	case slices.Contains(c.WarningSeverities, severity):
		return DefectWarns
	}
	return ""
}

// ImpactItem is one thing a change makes suspect, stale or outdated.
type ImpactItem struct {
	// Kind: requirement | test | issue | document | result.
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title,omitempty"`
	// Relation type and direction (requirements reached through a relation).
	Relation  string `json:"relation,omitempty"`
	Direction string `json:"direction,omitempty"`
	// Via: key of the requirement through which it is reached (the tests
	// of a related requirement), or the test of a result.
	Via string `json:"via,omitempty"`
	// Already: it is already suspect, stale or outdated.
	Already bool `json:"already,omitempty"`
	// RequirementID / TestID locate an issue's requirement or a result's test.
	RequirementID string `json:"requirement_id,omitempty"`
	TestID        string `json:"test_id,omitempty"`
	// Result (results): the result that becomes stale; URL (issues).
	Result string `json:"result,omitempty"`
	URL    string `json:"url,omitempty"`
}

// ChangeImpact lists what a change of the substance of a requirement or
// test makes suspect (related requirements, verifying tests), out of date
// (tracker issues), outdated (generated documents) and stale (results).
type ChangeImpact struct {
	Kind         string       `json:"kind"`
	ID           string       `json:"id"`
	Key          string       `json:"key"`
	Title        string       `json:"title"`
	Requirements []ImpactItem `json:"requirements"`
	Tests        []ImpactItem `json:"tests"`
	Issues       []ImpactItem `json:"issues"`
	Documents    []ImpactItem `json:"documents"`
	Results      []ImpactItem `json:"results"`
	// Total counts the items that are not already marked.
	Total int `json:"total"`
}

// DefectBlock is an open defect that blocks (or warns about) a requirement.
type DefectBlock struct {
	DefectID string `json:"defect_id"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	// Level: DefectBlocks | DefectWarns.
	Level string `json:"level"`
	// Via: requirement (linked directly), or the key of the test it is
	// linked to (directly or through an execution).
	Via string `json:"via"`
}

// DefectRegression records that a test that verified a resolved or closed
// defect failed again.
type DefectRegression struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	DefectID  string `json:"defect_id"`
	DefectKey string `json:"defect_key"`
	// RegressionDefectID: the regression defect created (action created).
	RegressionDefectID  *string `json:"regression_defect_id"`
	RegressionDefectKey string  `json:"regression_defect_key,omitempty"`
	TestCaseID          string  `json:"test_case_id"`
	TestKey             string  `json:"test_key"`
	ExecutionID         string  `json:"execution_id"`
	RunID               string  `json:"run_id,omitempty"`
	RunName             string  `json:"run_name,omitempty"`
	// Action: reopened | created.
	Action         string    `json:"action"`
	PreviousStatus string    `json:"previous_status"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

// Staleness of a result (see store.ResultStaleness).
type Staleness struct {
	Stale bool `json:"stale"`
	// Reason: age | test_changed | requirement_changed.
	Reason string `json:"reason,omitempty"`
	// Key of the changed item (test_changed, requirement_changed).
	Key string `json:"key,omitempty"`
	// RecordedAt: when the result was recorded.
	RecordedAt *time.Time `json:"recorded_at,omitempty"`
}

// Staleness reasons.
const (
	StaleAge                = "age"
	StaleTestChanged        = "test_changed"
	StaleRequirementChanged = "requirement_changed"
)
