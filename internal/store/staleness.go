package store

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Result staleness.
//
// A test result goes stale when it is older than the project's
// stale_after_days (0: never by age), or, with stale_on_change, when the
// test itself or a requirement it verifies changed in substance after the
// result was recorded (content_changed_at, set by MarkContentChanged). A
// stale result does not count as passing evidence: status follows evidence,
// coverage and release readiness treat a stale pass as not run. Lists,
// coverage, the traceability matrix and reports show it with a stale badge.
//
// Reuse: StalePolicy.Check is the pure rule; StaleIndex applies it to the
// latest results of a project (LoadStaleIndex) and its Check method to any
// other result (e.g. evidence of a single criterion) with the same changes.

// StalePolicy is the staleness rule of a project at a point in time.
type StalePolicy struct {
	// MaxAge: results older than this are stale (0: no age limit).
	MaxAge time.Duration
	// OnChange: a change of the test or a verified requirement after the
	// result makes it stale.
	OnChange bool
	// Now is the time the age is measured at.
	Now time.Time
}

// PolicyOf returns the staleness policy of correlation settings at now.
func PolicyOf(set *model.CorrelationSettings, now time.Time) StalePolicy {
	p := StalePolicy{OnChange: set.StaleOnChange, Now: now}
	if set.StaleAfterDays > 0 {
		p.MaxAge = time.Duration(set.StaleAfterDays) * 24 * time.Hour
	}
	return p
}

// ItemChange is a change of the substance of a requirement or test.
type ItemChange struct {
	// Kind: requirement | test.
	Kind string
	Key  string
	At   time.Time
}

// Check returns the staleness of a result recorded at recordedAt, given
// the changes of its test and the requirements it verifies. Age wins over
// a change; of several changes the latest is reported.
func (p StalePolicy) Check(recordedAt time.Time, changes ...ItemChange) model.Staleness {
	at := recordedAt
	out := model.Staleness{RecordedAt: &at}
	if p.MaxAge > 0 && !p.Now.IsZero() && p.Now.Sub(recordedAt) > p.MaxAge {
		out.Stale, out.Reason = true, model.StaleAge
		return out
	}
	if !p.OnChange {
		return out
	}
	var latest *ItemChange
	for i := range changes {
		c := &changes[i]
		if !c.At.IsZero() && c.At.After(recordedAt) && (latest == nil || c.At.After(latest.At)) {
			latest = c
		}
	}
	if latest != nil {
		out.Stale, out.Key = true, latest.Key
		out.Reason = model.StaleRequirementChanged
		if latest.Kind == model.KindTest {
			out.Reason = model.StaleTestChanged
		}
	}
	return out
}

// timedResult is a result with the time it was recorded.
type timedResult struct {
	status string
	at     time.Time
}

// StaleIndex knows the latest results of a project's tests (per
// configuration), the changes of its tests and requirements, and which
// results are stale. A nil *StaleIndex finds nothing stale.
type StaleIndex struct {
	Policy StalePolicy
	// settings the policy was made of
	settings *model.CorrelationSettings
	// results: test → configuration → latest result other than not_run
	results map[string]map[string]timedResult
	// substance changes and keys
	testChanged, reqChanged map[string]time.Time
	testKeys, reqKeys       map[string]string
	// test → the requirements it verifies
	t2r map[string][]string
	// test → configuration → staleness of that latest result (stale only)
	stale map[string]map[string]model.Staleness
}

// LoadStaleIndex loads the stale index of a project with its settings.
func (s *Store) LoadStaleIndex(ctx context.Context, tenantID, projectID string) (*StaleIndex, error) {
	set, err := s.CorrelationSettings(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	x := &StaleIndex{Policy: PolicyOf(set, s.now()), settings: set, results: map[string]map[string]timedResult{}, t2r: map[string][]string{},
		testChanged: map[string]time.Time{}, reqChanged: map[string]time.Time{}, testKeys: map[string]string{}, reqKeys: map[string]string{}}
	rows, err := s.c(ctx).query(ctx, `SELECT e.test_case_id, e.configuration_id, e.status, e.finished_at, e.started_at, r.created_at
		FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE r.project_id = ? AND e.tenant_id = ? AND e.status <> 'not_run'`, projectID, tenantID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var tid, st string
		var cfg sql.NullString
		var fin, start sql.NullTime
		var at time.Time
		if err := rows.Scan(&tid, &cfg, &st, &fin, &start, &at); err != nil {
			rows.Close()
			return nil, err
		}
		if start.Valid {
			at = start.Time
		}
		if fin.Valid {
			at = fin.Time
		}
		if x.results[tid] == nil {
			x.results[tid] = map[string]timedResult{}
		}
		if cur, ok := x.results[tid][cfg.String]; !ok || at.After(cur.at) {
			x.results[tid][cfg.String] = timedResult{st, at.UTC()}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.loadChanges(ctx, tenantID, projectID, x); err != nil {
		return nil, err
	}
	rows, err = s.c(ctx).query(ctx, `SELECT test_case_id, requirement_id FROM trace_links WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t, r string
		if err := rows.Scan(&t, &r); err != nil {
			rows.Close()
			return nil, err
		}
		x.t2r[t] = append(x.t2r[t], r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	x.compute()
	return x, nil
}

// loadChanges reads the keys and substance changes of a project's items.
func (s *Store) loadChanges(ctx context.Context, tenantID, projectID string, x *StaleIndex) error {
	for _, q := range []struct {
		table   string
		keys    map[string]string
		changed map[string]time.Time
	}{{"requirements", x.reqKeys, x.reqChanged}, {"test_cases", x.testKeys, x.testChanged}} {
		rows, err := s.c(ctx).query(ctx, `SELECT id, key, content_changed_at FROM `+q.table+` WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, key string
			var at sql.NullTime
			if err := rows.Scan(&id, &key, &at); err != nil {
				rows.Close()
				return err
			}
			q.keys[id] = key
			if at.Valid {
				q.changed[id] = at.Time.UTC()
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (x *StaleIndex) compute() {
	x.stale = map[string]map[string]model.Staleness{}
	for tid, units := range x.results {
		for cfg, r := range units {
			if st := x.Check(r.at, tid); st.Stale {
				if x.stale[tid] == nil {
					x.stale[tid] = map[string]model.Staleness{}
				}
				x.stale[tid][cfg] = st
			}
		}
	}
}

// changesOf returns the substance changes that matter to a result of a
// test: the test's and those of the requirements (all it verifies when
// requirementIDs is empty).
func (x *StaleIndex) changesOf(testID string, requirementIDs []string) []ItemChange {
	var out []ItemChange
	if at, ok := x.testChanged[testID]; ok {
		out = append(out, ItemChange{Kind: model.KindTest, Key: x.testKeys[testID], At: at})
	}
	if len(requirementIDs) == 0 {
		requirementIDs = x.t2r[testID]
	}
	for _, r := range requirementIDs {
		if at, ok := x.reqChanged[r]; ok {
			out = append(out, ItemChange{Kind: model.KindRequirement, Key: x.reqKeys[r], At: at})
		}
	}
	return out
}

// Check returns the staleness of a result of a test recorded at
// recordedAt (any result, e.g. the evidence of one criterion): its age and
// the changes of the test and of the requirements given (all requirements
// the test verifies when none are given).
func (x *StaleIndex) Check(recordedAt time.Time, testID string, requirementIDs ...string) model.Staleness {
	if x == nil {
		at := recordedAt
		return model.Staleness{RecordedAt: &at}
	}
	return x.Policy.Check(recordedAt, x.changesOf(testID, requirementIDs)...)
}

// Test returns the staleness of a test's latest result (over all its
// configurations): stale when the latest result in any configuration is.
func (x *StaleIndex) Test(testID string) model.Staleness {
	if x == nil {
		return model.Staleness{}
	}
	units := x.stale[testID]
	if len(units) == 0 {
		return model.Staleness{}
	}
	cfgs := make([]string, 0, len(units))
	for c := range units {
		cfgs = append(cfgs, c)
	}
	sort.Strings(cfgs)
	return units[cfgs[0]]
}

// Settings returns the correlation settings the index was loaded with.
func (x *StaleIndex) Settings() *model.CorrelationSettings {
	if x == nil || x.settings == nil {
		d := model.DefaultCorrelationSettings()
		return &d
	}
	return x.settings
}

// Unit returns the staleness of the latest result of a test in one
// configuration (NoConfiguration: without one).
func (x *StaleIndex) Unit(testID, cfg string) model.Staleness {
	if x == nil {
		return model.Staleness{}
	}
	return x.stale[testID][cfg]
}

// StaleTests returns the tests whose latest result is stale in at least
// one configuration, with the staleness.
func (x *StaleIndex) StaleTests() map[string]model.Staleness {
	out := map[string]model.Staleness{}
	if x == nil {
		return out
	}
	for tid := range x.stale {
		out[tid] = x.Test(tid)
	}
	return out
}

// FilterConfig returns a copy of res in which stale passes are no
// results ("": planned, not run), so they do not count as evidence.
func (x *StaleIndex) FilterConfig(res ConfigResults) ConfigResults {
	out := make(ConfigResults, len(res))
	for tid, units := range res {
		m := make(map[string]string, len(units))
		for cfg, st := range units {
			if st == model.ResultPassed && x.Unit(tid, cfg).Stale {
				st = ""
			}
			m[cfg] = st
		}
		out[tid] = m
	}
	return out
}

// FilterLast returns a copy of last (test → latest result, as
// LastResults) without the stale passes.
func (x *StaleIndex) FilterLast(last map[string]string) map[string]string {
	out := make(map[string]string, len(last))
	for tid, st := range last {
		if st == model.ResultPassed && x.Test(tid).Stale {
			continue
		}
		out[tid] = st
	}
	return out
}

// StaleCount returns how many of the tests have a stale latest result.
func (x *StaleIndex) StaleCount(tests []string) int {
	n := 0
	for _, t := range tests {
		if x.Test(t).Stale {
			n++
		}
	}
	return n
}
