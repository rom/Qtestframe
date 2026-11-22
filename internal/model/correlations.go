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
	// ImpactDepth: how many hops a change travels (1 = only the items
	// related to the changed one directly; 1..MaxImpactDepth).
	ImpactDepth int `json:"impact_depth"`
	// RelationModes: relation type → ImpactModes value (what the change
	// does to the requirement it reaches along RelationImpact). Types not
	// listed are ModeSuspect.
	RelationModes map[string]string `json:"relation_modes"`
	// ImpactRules: the propagation rules of the other edges of the impact
	// graph (ImpactEdgeKinds: verifying tests, item links, risks, defects,
	// variants, code changes).
	ImpactRules map[string]ImpactRule `json:"impact_rules"`
	// ApplicabilityImpact: what a change of the variants a requirement or
	// test applies to does (ApplicabilityImpacts): ModeReview (default)
	// recommends a review of what the impact rules reach (its owners are
	// notified, nothing is marked), ModeSuspect is a change of substance
	// (the rules mark trace links and relations suspect),
	// ApplicabilityOff does nothing.
	ApplicabilityImpact string `json:"applicability_impact"`
}

// ApplicabilityOff: a change of applicability has no impact.
const ApplicabilityOff = "off"

// ApplicabilityImpacts lists the valid values of
// CorrelationSettings.ApplicabilityImpact.
var ApplicabilityImpacts = []string{ModeReview, ModeSuspect, ApplicabilityOff}

// ApplicabilityMode returns ApplicabilityImpact (ModeReview when unset).
func (c *CorrelationSettings) ApplicabilityMode() string {
	if slices.Contains(ApplicabilityImpacts, c.ApplicabilityImpact) {
		return c.ApplicabilityImpact
	}
	return ModeReview
}

// Change impact depth (CorrelationSettings.ImpactDepth).
const (
	DefaultImpactDepth = 3
	MaxImpactDepth     = 10
)

// Impact modes: what a change does to an item it reaches.
const (
	// ModeSuspect marks the relation or trace link suspect (and flags
	// tracker issues out of date).
	ModeSuspect = "suspect"
	// ModeReview only recommends a review: the item is listed in the
	// preview and its owner can be notified, nothing is marked.
	ModeReview = "review"
)

// ImpactModes lists the valid impact modes.
var ImpactModes = []string{ModeSuspect, ModeReview}

// Edge kinds of the impact graph other than the requirement relation types
// (their natural direction source → target in brackets).
const (
	EdgeVerifies = "verifies"        // test → requirement (trace link)
	EdgeItemLink = "item_link"       // item → item (item links between requirements and tests)
	EdgeRisk     = "risk"            // risk → requirement (the requirement addresses the risk)
	EdgeDefect   = "defect"          // defect → requirement or test
	EdgeVariant  = "variant"         // requirement or test → variant it applies to
	EdgeCode     = "code"            // code change → requirement or test
	EdgeCalls    = "calls"           // test → test it calls as shared steps
	EdgeTestDeps = "test_depends_on" // test → test it depends on
	// EdgeIssue, EdgeDocument and EdgeResult connect a changed item to
	// its tracker issues, the generated documents containing it and its
	// latest results (FlagIssues, StaleOnChange; no rule).
	EdgeIssue    = "issue"
	EdgeDocument = "document"
	EdgeResult   = "result"
)

// ImpactEdgeKinds lists the edge kinds with an ImpactRule.
var ImpactEdgeKinds = []string{EdgeVerifies, EdgeCalls, EdgeTestDeps, EdgeItemLink, EdgeRisk, EdgeDefect, EdgeVariant, EdgeCode}

// ImpactRule is how a change travels along one kind of edge: Direction is
// an ImpactDirections value seen from the natural source → target of the
// edge (incoming: the target changed, the source is affected), Mode an
// ImpactModes value. Edges without a suspect mark (test calls and
// dependencies, item links, risks, defects, variants, code changes) only
// recommend a review.
type ImpactRule struct {
	Direction string `json:"direction"`
	Mode      string `json:"mode"`
}

// Markable reports whether an edge kind has a suspect mark (relation
// types and trace links); the others are always ModeReview.
func Markable(edge string) bool {
	return edge == EdgeVerifies || slices.Contains(RelationTypes, edge)
}

// DefaultImpactRules returns the rules of a project that has not chosen
// its own.
func DefaultImpactRules() map[string]ImpactRule {
	return map[string]ImpactRule{
		EdgeVerifies: {Direction: ImpactBoth, Mode: ModeSuspect},
		EdgeCalls:    {Direction: ImpactIncoming, Mode: ModeReview},
		EdgeTestDeps: {Direction: ImpactIncoming, Mode: ModeReview},
		EdgeItemLink: {Direction: ImpactBoth, Mode: ModeReview},
		EdgeRisk:     {Direction: ImpactBoth, Mode: ModeReview},
		EdgeDefect:   {Direction: ImpactIncoming, Mode: ModeReview},
		EdgeVariant:  {Direction: ImpactOutgoing, Mode: ModeReview},
		EdgeCode:     {Direction: ImpactIncoming, Mode: ModeReview},
	}
}

// Depth returns the impact depth (the default when unset).
func (c *CorrelationSettings) Depth() int {
	switch {
	case c.ImpactDepth <= 0:
		return DefaultImpactDepth
	case c.ImpactDepth > MaxImpactDepth:
		return MaxImpactDepth
	}
	return c.ImpactDepth
}

// Rule returns the propagation rule of an edge kind or relation type.
func (c *CorrelationSettings) Rule(edge string) ImpactRule {
	if slices.Contains(RelationTypes, edge) {
		mode := c.RelationModes[edge]
		if mode != ModeReview {
			mode = ModeSuspect
		}
		return ImpactRule{Direction: c.ImpactDirection(edge), Mode: mode}
	}
	r, ok := c.ImpactRules[edge]
	if !ok {
		r = DefaultImpactRules()[edge]
	}
	if r.Direction == "" {
		r.Direction = ImpactOff
	}
	if r.Mode != ModeSuspect || !Markable(edge) {
		r.Mode = ModeReview
	}
	return r
}

// Impact choices: what saving a change does with its impact (the
// impact= query parameter of the changing endpoints).
const (
	// ImpactApplySuspect (default): marks what the rules mark suspect and
	// notifies the owners of the items only recommended for review.
	ImpactApplySuspect = "suspect"
	// ImpactApplyNotify marks nothing and notifies the owners of every
	// affected item.
	ImpactApplyNotify = "notify"
	// ImpactApplySkip marks nothing and notifies nobody (results recorded
	// before the change still go stale, documents still become outdated).
	ImpactApplySkip = "skip"
)

// ImpactChoices lists the valid impact choices.
var ImpactChoices = []string{ImpactApplySuspect, ImpactApplyNotify, ImpactApplySkip}

// DefaultCorrelationSettings returns the settings of a project that has
// not chosen its own.
func DefaultCorrelationSettings() CorrelationSettings {
	return CorrelationSettings{
		RelationImpact:      map[string]string{"refines": ImpactIncoming, "derived_from": ImpactIncoming, "depends_on": ImpactIncoming},
		FlagIssues:          true,
		StaleAfterDays:      DefaultStaleAfterDays,
		StaleOnChange:       true,
		BlockingSeverities:  []string{"critical", "major"},
		WarningSeverities:   []string{"minor"},
		RegressionAction:    RegressionReopen,
		ImpactDepth:         DefaultImpactDepth,
		RelationModes:       map[string]string{},
		ImpactRules:         DefaultImpactRules(),
		ApplicabilityImpact: ModeReview,
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

// ImpactStep is one step of the path along which a change reaches an
// item: the item, and the edge along which it was reached from the step
// before (empty on the first step, the changed item).
type ImpactStep struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Key  string `json:"key"`
	// Edge: relation type or edge kind (ImpactEdgeKinds, issue, document,
	// result); Direction: outgoing when the step before is the natural
	// source of the edge, incoming when it is the target.
	Edge      string `json:"edge,omitempty"`
	Direction string `json:"direction,omitempty"`
}

// ImpactItem is one thing a change makes suspect, stale or outdated.
type ImpactItem struct {
	// Kind: requirement | test | issue | document | result | risk |
	// defect | variant | code.
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
	// Depth: the number of steps from the changed item; Mode: ImpactModes
	// value; Path: the steps from the changed item to this one.
	Depth int          `json:"depth"`
	Mode  string       `json:"mode"`
	Path  []ImpactStep `json:"path"`
	// Owner: the owner (requirement, risk), assignee (defect) or author
	// (test) notified when owners are notified.
	Owner string `json:"owner,omitempty"`
	// Status of the item (requirements, tests, risks, defects, code changes).
	Status string `json:"status,omitempty"`
}

// ImpactRef names a changed item of an impact.
type ImpactRef struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	// ReviewOnly: the change is not one of substance (a status change):
	// it only recommends reviews.
	ReviewOnly bool `json:"review_only,omitempty"`
	// Fields: the changed fields, when known.
	Fields []string `json:"fields,omitempty"`
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
	Risks        []ImpactItem `json:"risks"`
	Defects      []ImpactItem `json:"defects"`
	Links        []ImpactItem `json:"links"`
	Variants     []ImpactItem `json:"variants"`
	Code         []ImpactItem `json:"code"`
	// Total counts the items that are not already marked.
	Total int `json:"total"`
	// Roots: the changed items (one for the impact of an item; several for
	// a bulk change, an import or a baseline diff).
	Roots []ImpactRef `json:"roots"`
	// Depth: the depth analysed; Suspect / Review: items (not already
	// marked) per mode; ByKind / ByDepth: items per kind and depth.
	Depth   int            `json:"depth"`
	Suspect int            `json:"suspect"`
	Review  int            `json:"review"`
	ByKind  map[string]int `json:"by_kind"`
	ByDepth map[string]int `json:"by_depth"`
}

// Lists returns the item lists of an impact by section name.
func (c *ChangeImpact) Lists() map[string]*[]ImpactItem {
	return map[string]*[]ImpactItem{"requirements": &c.Requirements, "tests": &c.Tests, "issues": &c.Issues, "documents": &c.Documents,
		"results": &c.Results, "risks": &c.Risks, "defects": &c.Defects, "links": &c.Links, "variants": &c.Variants, "code": &c.Code}
}

// ImpactSections lists the section names of an impact in display order.
var ImpactSections = []string{"requirements", "tests", "risks", "defects", "links", "issues", "documents", "results", "variants", "code"}

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
