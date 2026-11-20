package store

import (
	"context"
	"sort"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Flaky tests: tests whose recent results alternate between passed and
// failed. Only decisive results count (passed and failed, manual or
// automated, from every run of the project); blocked, skipped, in progress
// and not run say nothing about the test itself and are left out.
//
// The flakiness score of a test is the number of flips (a passed result
// followed by a failed one, or the reverse) among its last FlakyWindow
// decisive results, oldest to newest. A test with at least FlakyMinFlips
// flips is flaky: a test that broke once and was fixed has at most one flip
// per change, while 2 or more flips in 10 results means the outcome changed
// back and forth.
const (
	FlakyWindow   = 10
	FlakyMinFlips = 2
)

// FlakyResult is one decisive result in a test's recent history.
type FlakyResult struct {
	Status      string    `json:"status"`
	At          time.Time `json:"at"`
	RunID       string    `json:"run_id"`
	ExecutionID string    `json:"execution_id"`
	// Flip: the result differs from the previous one in the window.
	Flip bool `json:"flip"`
}

// FlakyTest is a test with its flakiness score.
type FlakyTest struct {
	TestID        string `json:"test_id"`
	Key           string `json:"key"`
	Title         string `json:"title"`
	ExecutionType string `json:"execution_type"`
	// Score: flips among the last results; Window: results considered.
	Score  int `json:"score"`
	Window int `json:"window"`
	// Results: the last decisive results, oldest first.
	Results  []FlakyResult `json:"results"`
	LastFlip *time.Time    `json:"last_flip"`
}

// Flakiness scores a sequence of results (oldest first): the number of
// flips between passed and failed among the last FlakyWindow decisive
// results, and those results with their flips marked.
func Flakiness(results []FlakyResult) (int, []FlakyResult) {
	var dec []FlakyResult
	for _, r := range results {
		if r.Status == model.ResultPassed || r.Status == model.ResultFailed {
			dec = append(dec, r)
		}
	}
	if len(dec) > FlakyWindow {
		dec = dec[len(dec)-FlakyWindow:]
	}
	flips := 0
	for i := range dec {
		dec[i].Flip = i > 0 && dec[i].Status != dec[i-1].Status
		if dec[i].Flip {
			flips++
		}
	}
	return flips, dec
}

// testHistories groups a project's results by test (oldest first).
func (s *Store) testHistories(ctx context.Context, tenantID, projectID string) (map[string][]FlakyResult, error) {
	evs, err := resultEvents(ctx, s.c(ctx), tenantID, projectID, "")
	if err != nil {
		return nil, err
	}
	out := map[string][]FlakyResult{}
	for _, ev := range evs {
		out[ev.test] = append(out[ev.test], FlakyResult{Status: ev.status, At: ev.at, RunID: ev.run, ExecutionID: ev.exec})
	}
	return out, nil
}

// FlakyScores returns test id → flakiness score for the tests of a project
// with at least one flip.
func (s *Store) FlakyScores(ctx context.Context, tenantID, projectID string) (map[string]int, error) {
	hs, err := s.testHistories(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for id, h := range hs {
		if n, _ := Flakiness(h); n > 0 {
			out[id] = n
		}
	}
	return out, nil
}

// FlakyTests lists the flaky tests of a project (score at least
// FlakyMinFlips), highest score first, then most recent flip.
func (s *Store) FlakyTests(ctx context.Context, tenantID, projectID string) ([]FlakyTest, error) {
	hs, err := s.testHistories(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []FlakyTest{}
	for id, h := range hs {
		n, win := Flakiness(h)
		if n < FlakyMinFlips {
			continue
		}
		ft := FlakyTest{TestID: id, Score: n, Window: len(win), Results: win}
		for i := len(win) - 1; i >= 0; i-- {
			if win[i].Flip {
				at := win[i].At
				ft.LastFlip = &at
				break
			}
		}
		out = append(out, ft)
	}
	if len(out) > 0 {
		rows, err := s.c(ctx).query(ctx, `SELECT id, key, title, execution_type FROM test_cases WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
		if err != nil {
			return nil, err
		}
		info := map[string][3]string{}
		for rows.Next() {
			var id, k, t, e string
			if err := rows.Scan(&id, &k, &t, &e); err != nil {
				rows.Close()
				return nil, err
			}
			info[id] = [3]string{k, t, e}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		for i := range out {
			v := info[out[i].TestID]
			out[i].Key, out[i].Title, out[i].ExecutionType = v[0], v[1], v[2]
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if !a.LastFlip.Equal(*b.LastFlip) {
			return a.LastFlip.After(*b.LastFlip)
		}
		return naturalLess(a.Key, b.Key)
	})
	return out, nil
}
