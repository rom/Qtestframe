// Package verification decides whether requirements are verified, and
// why, from their evaluation criteria and the evidence about them.
//
// Evidence is either a test (a trace link, optionally mapped to criteria
// and to one step of the test) or a verification record (an inspection,
// analysis, demonstration or review, or a test done outside the tool).
//
// The rules:
//
//   - A test counts as passed when its last result passed (with a step:
//     the step's result in that execution), failed when it failed or was
//     blocked, and pending otherwise; a suspect link is pending until it is
//     confirmed (a failure still counts as failed).
//   - A record counts with its result (pass and conditional pass, fail
//     fails) while it is valid: not after its valid-until date, and not
//     after it was invalidated by a change of the requirement. For each
//     criterion only the newest valid record counts.
//   - A criterion is verified when all the evidence that counts for it
//     passed, failed when any failed, pending when there is evidence but
//     not all passed, none without evidence. Plain evidence (a test link
//     without criterion mapping, a record without criterion) counts for
//     every criterion without evidence of its own, unless the project uses
//     strict criterion coverage.
//   - A requirement's own state: with criteria, verified when every
//     criterion is verified and all plain evidence passed; without
//     criteria, from its plain evidence. A plain failure always fails it.
//   - Roll-up along "refines" (the source refines, is a child of, the
//     target): a parent is verified only when its own state is verified
//     (or it has neither criteria nor evidence of its own) and every
//     active child is verified. Rejected and obsolete children are left
//     out.
package verification

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// TestInfo describes a test for the evidence lists.
type TestInfo struct {
	Key, Title string
	// StepPos: step id → 1-based position.
	StepPos map[string]int
}

// Input is a project's data.
type Input struct {
	// Requirements with their criteria.
	Requirements []model.Requirement
	// Links with their criterion mappings.
	Links []model.TraceLink
	Tests map[string]TestInfo
	// Last: test id → last result; LastAt: when it ran (optional).
	Last   map[string]string
	LastAt map[string]time.Time
	// Steps: test id → step id → result in the execution of Last.
	Steps map[string]map[string]string
	// Records: the project's verification records.
	Records []model.VerificationRecord
	// Relations of the project (only refines is used).
	Relations []model.Relation
	// Strict criterion coverage.
	Strict bool
	// Today (YYYY-MM-DD) for the validity of records.
	Today string
}

// Result maps requirement id → its verification.
type Result map[string]*model.RequirementVerification

// Validity of a record on a day: valid | expired | invalidated.
func Validity(r *model.VerificationRecord, today string) string {
	switch {
	case r.InvalidatedAt != nil:
		return "invalidated"
	case r.ValidUntil != "" && today != "" && r.ValidUntil < today:
		return "expired"
	}
	return "valid"
}

// Aggregate combines states: none when all are none (or there are none),
// failed when any failed, verified when all are verified, partial when
// some are verified or partial, pending otherwise.
func Aggregate(states []string) string {
	if len(states) == 0 {
		return model.VerNone
	}
	none, verified, some := 0, 0, 0
	for _, s := range states {
		switch s {
		case model.VerFailed:
			return model.VerFailed
		case model.VerVerified:
			verified++
			some++
		case model.VerPartial:
			some++
		case model.VerNone:
			none++
		}
	}
	switch {
	case none == len(states):
		return model.VerNone
	case verified == len(states):
		return model.VerVerified
	case some > 0:
		return model.VerPartial
	}
	return model.VerPending
}

// evidenceState turns evidence into a criterion-level state.
func evidenceStates(ev []model.Evidence) string {
	counted, passed := 0, 0
	for _, e := range ev {
		switch e.State {
		case model.EvidenceVoid:
			continue
		case model.EvidenceFailed:
			return model.VerFailed
		case model.EvidencePassed:
			passed++
		}
		counted++
	}
	switch {
	case counted == 0:
		return model.VerNone
	case passed == counted:
		return model.VerVerified
	case passed > 0:
		return model.VerPartial
	}
	return model.VerPending
}

func testEvidence(in *Input, l *model.TraceLink, stepID string) model.Evidence {
	ti := in.Tests[l.TestCaseID]
	e := model.Evidence{Kind: model.KindTest, ID: l.TestCaseID, LinkID: l.ID, Key: ti.Key, Title: ti.Title, StepID: stepID}
	if e.Key == "" {
		e.Key = l.TestCaseKey
	}
	if t, ok := in.LastAt[l.TestCaseID]; ok && !t.IsZero() {
		e.Date = t.UTC().Format("2006-01-02")
	}
	res := in.Last[l.TestCaseID]
	if stepID != "" {
		e.StepPosition = ti.StepPos[stepID]
		if res != "" {
			st, ok := in.Steps[l.TestCaseID][stepID]
			if !ok {
				st = model.ResultNotRun
			}
			res = st
		}
	}
	if res == "" {
		res = model.ResultNotRun
	}
	e.Result = res
	switch res {
	case model.ResultPassed:
		e.State = model.EvidencePassed
	case model.ResultFailed, model.ResultBlocked:
		e.State = model.EvidenceFailed
	default:
		e.State = model.EvidencePending
		e.Note = "not_run"
		if stepID != "" && in.Last[l.TestCaseID] != "" {
			e.Note = "step_not_run"
		}
	}
	if l.SuspectSince != nil && e.State == model.EvidencePassed {
		e.State, e.Note = model.EvidencePending, "suspect"
	}
	return e
}

func recordEvidence(r *model.VerificationRecord, today string) model.Evidence {
	e := model.Evidence{Kind: "record", ID: r.ID, Key: r.Method, Title: r.PerformedBy, Result: r.Result, Date: r.PerformedOn}
	switch v := Validity(r, today); v {
	case "valid":
		switch r.Result {
		case model.RecordPass:
			e.State = model.EvidencePassed
		case model.RecordConditional:
			e.State, e.Note = model.EvidencePassed, "conditional"
		default:
			e.State = model.EvidenceFailed
		}
	default:
		e.State, e.Note = model.EvidenceVoid, v
	}
	return e
}

// recordNewer orders records newest first (performed on, then created).
func recordNewer(a, b *model.VerificationRecord) bool {
	if a.PerformedOn != b.PerformedOn {
		return a.PerformedOn > b.PerformedOn
	}
	return a.CreatedAt.After(b.CreatedAt)
}

// records returns the evidence of records, newest first, with all but
// the newest valid one void (superseded).
func records(list []*model.VerificationRecord, today string) []model.Evidence {
	sort.SliceStable(list, func(i, j int) bool { return recordNewer(list[i], list[j]) })
	out := []model.Evidence{}
	counted := false
	for _, r := range list {
		e := recordEvidence(r, today)
		if e.State != model.EvidenceVoid {
			if counted {
				e.State, e.Note = model.EvidenceVoid, "superseded"
			}
			counted = true
		}
		out = append(out, e)
	}
	return out
}

func inactive(status string) bool { return status == "rejected" || status == "obsolete" }

// Compute returns the verification of every requirement of the input.
func Compute(in Input) Result {
	byID := map[string]*model.Requirement{}
	for i := range in.Requirements {
		byID[in.Requirements[i].ID] = &in.Requirements[i]
	}
	// evidence per requirement / criterion
	plain := map[string][]model.Evidence{}
	perCrit := map[string][]model.Evidence{}
	for i := range in.Links {
		l := &in.Links[i]
		r, ok := byID[l.RequirementID]
		if !ok {
			continue
		}
		mapped := false
		for _, lc := range l.Criteria {
			if !hasCriterion(r, lc.CriterionID) {
				continue
			}
			mapped = true
			perCrit[lc.CriterionID] = append(perCrit[lc.CriterionID], testEvidence(&in, l, lc.StepID))
		}
		if !mapped {
			plain[l.RequirementID] = append(plain[l.RequirementID], testEvidence(&in, l, ""))
		}
	}
	recByCrit := map[string][]*model.VerificationRecord{}
	recPlain := map[string][]*model.VerificationRecord{}
	for i := range in.Records {
		rec := &in.Records[i]
		r, ok := byID[rec.RequirementID]
		if !ok {
			continue
		}
		if rec.CriterionID != nil && hasCriterion(r, *rec.CriterionID) {
			recByCrit[*rec.CriterionID] = append(recByCrit[*rec.CriterionID], rec)
		} else {
			recPlain[rec.RequirementID] = append(recPlain[rec.RequirementID], rec)
		}
	}
	for id, list := range recPlain {
		plain[id] = append(plain[id], records(list, in.Today)...)
	}
	for id, list := range recByCrit {
		perCrit[id] = append(perCrit[id], records(list, in.Today)...)
	}

	// children / parents (refines: source is the child of target)
	children, parents := map[string][]string{}, map[string][]string{}
	for _, rel := range in.Relations {
		if rel.Type != "refines" || byID[rel.SourceID] == nil || byID[rel.TargetID] == nil || rel.SourceID == rel.TargetID {
			continue
		}
		children[rel.TargetID] = append(children[rel.TargetID], rel.SourceID)
		parents[rel.SourceID] = append(parents[rel.SourceID], rel.TargetID)
	}

	out := Result{}
	for i := range in.Requirements {
		r := &in.Requirements[i]
		v := &model.RequirementVerification{RequirementID: r.ID, Key: r.Key, Title: r.Title, Status: r.Status, Strict: in.Strict,
			Criteria: []model.CriterionVerification{}, Evidence: plain[r.ID], Children: []model.ChildVerification{},
			ParentIDs: parents[r.ID], Reasons: []model.VerificationReason{}, Inconsistencies: []model.VerificationReason{}}
		if v.Evidence == nil {
			v.Evidence = []model.Evidence{}
		}
		if v.ParentIDs == nil {
			v.ParentIDs = []string{}
		}
		plainState := evidenceStates(v.Evidence)
		states := []string{}
		for ci, c := range r.Criteria {
			cv := model.CriterionVerification{CriterionID: c.ID, Position: ci + 1, Description: c.Description, Method: c.Method, Evidence: perCrit[c.ID]}
			if cv.Evidence == nil {
				cv.Evidence = []model.Evidence{}
			}
			cv.State = evidenceStates(cv.Evidence)
			if cv.State == model.VerNone && !in.Strict && plainState != model.VerNone {
				for _, e := range v.Evidence {
					e.Plain = true
					cv.Evidence = append(cv.Evidence, e)
				}
				cv.State = evidenceStates(cv.Evidence)
			}
			if cv.State == model.VerVerified {
				v.CriteriaVerified++
			}
			v.Criteria = append(v.Criteria, cv)
			states = append(states, cv.State)
		}
		v.CriteriaTotal = len(r.Criteria)
		switch {
		case len(r.Criteria) == 0:
			v.Own = plainState
		case plainState == model.VerFailed:
			v.Own = model.VerFailed
		default:
			v.Own = Aggregate(states)
			if v.Own == model.VerVerified && plainState != model.VerNone && plainState != model.VerVerified {
				v.Own = model.VerPartial
			}
			if v.Own == model.VerNone && plainState != model.VerNone {
				v.Own = model.VerPending
			}
		}
		out[r.ID] = v
	}

	// roll-up (depth first, cycles cut)
	const visiting = "\x00"
	rolled := map[string]string{}
	var roll func(id string) string
	roll = func(id string) string {
		if s, ok := rolled[id]; ok {
			if s == visiting {
				return out[id].Own
			}
			return s
		}
		rolled[id] = visiting
		v := out[id]
		var cs []string
		for _, c := range children[id] {
			if inactive(byID[c].Status) {
				continue
			}
			cs = append(cs, roll(c))
		}
		s := v.Own
		if len(cs) > 0 {
			if v.Own == model.VerNone && v.CriteriaTotal == 0 {
				s = Aggregate(cs)
			} else {
				s = Aggregate(append(cs, v.Own))
			}
		}
		rolled[id] = s
		return s
	}
	for i := range in.Requirements {
		id := in.Requirements[i].ID
		v := out[id]
		v.State = roll(id)
		v.Verified = v.State == model.VerVerified
	}
	for i := range in.Requirements {
		r := &in.Requirements[i]
		v := out[r.ID]
		for _, c := range children[r.ID] {
			cr := byID[c]
			cv := out[c]
			v.Children = append(v.Children, model.ChildVerification{ID: c, Key: cr.Key, Title: cr.Title, Status: cr.Status, State: cv.State})
			if inactive(cr.Status) {
				continue
			}
			if cv.State == model.VerVerified {
				v.ChildrenVerified++
			} else {
				v.Reasons = append(v.Reasons, reason("child_not_verified", map[string]string{"key": cr.Key, "state": cv.State}))
			}
			if r.Status == "verified" && cr.Status != "verified" {
				v.Inconsistencies = append(v.Inconsistencies, reason("child_status_unverified", map[string]string{"key": cr.Key, "status": cr.Status}))
			}
		}
		v.Reasons = append(ownReasons(v), v.Reasons...)
		if r.Status == "verified" && !v.Verified && (v.State != model.VerNone || len(v.Children) > 0) {
			v.Inconsistencies = append([]model.VerificationReason{reason("status_inconsistent", map[string]string{"state": v.State})}, v.Inconsistencies...)
		}
		if v.Verified {
			v.Reasons = []model.VerificationReason{}
		}
	}
	return out
}

func hasCriterion(r *model.Requirement, id string) bool {
	for _, c := range r.Criteria {
		if c.ID == id {
			return true
		}
	}
	return false
}

func ownReasons(v *model.RequirementVerification) []model.VerificationReason {
	out := []model.VerificationReason{}
	if v.Own == model.VerNone && len(v.Children) == 0 {
		return append(out, reason("no_evidence", nil))
	}
	for _, c := range v.Criteria {
		p := map[string]string{"criterion": fmt.Sprint(c.Position), "description": c.Description, "method": c.Method}
		switch c.State {
		case model.VerNone:
			out = append(out, reason("criterion_missing", p))
		case model.VerFailed:
			out = append(out, reason("criterion_failed", p))
		case model.VerPending, model.VerPartial:
			out = append(out, reason("criterion_pending", p))
		}
	}
	for _, e := range v.Evidence {
		if v.CriteriaTotal > 0 && e.State != model.EvidenceFailed {
			continue
		}
		p := map[string]string{"kind": e.Kind, "key": e.Key}
		switch e.State {
		case model.EvidenceFailed:
			out = append(out, reason("evidence_failed", p))
		case model.EvidencePending:
			p["note"] = e.Note
			out = append(out, reason("evidence_pending", p))
		}
	}
	return out
}

var reasonText = map[string]string{
	"no_evidence":             "no test or verification record",
	"criterion_missing":       "criterion {criterion} has no evidence",
	"criterion_failed":        "criterion {criterion} failed",
	"criterion_pending":       "criterion {criterion} has not passed yet",
	"evidence_failed":         "{kind} {key} failed",
	"evidence_pending":        "{kind} {key} has not passed yet",
	"child_not_verified":      "child {key} is not verified",
	"status_inconsistent":     "the status is verified but the evidence is {state}",
	"child_status_unverified": "the status is verified but child {key} is {status}",
}

func reason(code string, params map[string]string) model.VerificationReason {
	t := reasonText[code]
	for k, v := range params {
		t = strings.ReplaceAll(t, "{"+k+"}", v)
	}
	return model.VerificationReason{Code: code, Params: params, Text: t}
}

// Why returns the reasons as one English sentence ("" when verified).
func Why(v *model.RequirementVerification) string {
	var parts []string
	for _, r := range v.Reasons {
		parts = append(parts, r.Text)
	}
	return strings.Join(parts, "; ")
}

// Ancestors returns the ids of every requirement that ids refine,
// directly or not (without ids themselves).
func Ancestors(res Result, ids []string) []string {
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	var out []string
	queue := append([]string{}, ids...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		v := res[id]
		if v == nil {
			continue
		}
		for _, p := range v.ParentIDs {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
				queue = append(queue, p)
			}
		}
	}
	return out
}

// Counts tallies the rolled-up states of the requirements ids.
func Counts(res Result, ids []string) map[string]int {
	out := map[string]int{}
	for _, id := range ids {
		if v := res[id]; v != nil {
			out[v.State]++
		}
	}
	return out
}

// ElementCoverage computes, for every element (in the given order, with
// RequirementIDs and TestIDs the direct allocations), the coverage and
// verification of the requirements and tests allocated to it or to any
// element below it. status: requirement id → status (rejected and
// obsolete requirements do not count); last: test id → last result.
func ElementCoverage(elements []model.SystemElement, ancestors map[string][]string, status map[string]string, res Result, last map[string]string) []model.ElementCoverage {
	reqs, tests := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, e := range elements {
		for _, a := range ancestors[e.ID] {
			if reqs[a] == nil {
				reqs[a], tests[a] = map[string]bool{}, map[string]bool{}
			}
			for _, id := range e.RequirementIDs {
				if st, ok := status[id]; ok && !inactive(st) {
					reqs[a][id] = true
				}
			}
			for _, id := range e.TestIDs {
				tests[a][id] = true
			}
		}
	}
	out := make([]model.ElementCoverage, 0, len(elements))
	for _, e := range elements {
		c := model.ElementCoverage{ElementID: e.ID, ParentID: e.ParentID, Name: e.Name, Kind: e.Kind, Path: e.Path, Depth: len(ancestors[e.ID]) - 1}
		if c.Depth < 0 {
			c.Depth = 0
		}
		for id := range reqs[e.ID] {
			c.Requirements++
			v := res[id]
			if v == nil {
				continue
			}
			if v.State != model.VerNone {
				c.Covered++
			}
			switch v.State {
			case model.VerVerified:
				c.Verified++
			case model.VerFailed:
				c.Failed++
			}
		}
		for id := range tests[e.ID] {
			c.Tests++
			if last[id] == model.ResultPassed {
				c.TestsPassed++
			}
		}
		if c.Requirements > 0 {
			c.Percent = float64(int(float64(c.Verified)*1000/float64(c.Requirements)+0.5)) / 10
		}
		out = append(out, c)
	}
	return out
}
