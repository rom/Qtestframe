package ci

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Reason says why a test or requirement is selected (see the regression
// selection): requirement, change, related, verifies, prerequisite, key
// (named by a commit message or explicitly), tag (always run), path (a
// changed file of the path map).
type Reason struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref,omitempty"`
	Type string `json:"type,omitempty"`
}

// SelectedTest is a test of a selection with what a test runner needs.
type SelectedTest struct {
	ID            string   `json:"id"`
	Key           string   `json:"key"`
	Title         string   `json:"title"`
	ExecutionType string   `json:"execution_type"`
	Tags          []string `json:"tags"`
	// Runner: the runner:<name> tag, else from the script language, else
	// "default"; the child pipeline has one job per runner.
	Runner string `json:"runner"`
	// Identifier: the key as automated tests name it (SHOP_TC_12).
	Identifier string   `json:"identifier"`
	LastResult string   `json:"last_result,omitempty"`
	Reasons    []Reason `json:"reasons"`
}

// SelectedRequirement is a changed (or related) requirement.
type SelectedRequirement struct {
	ID      string   `json:"id"`
	Key     string   `json:"key"`
	Title   string   `json:"title"`
	Reasons []Reason `json:"reasons"`
}

// Selection is the answer of POST /projects/{pid}/ci/select.
type Selection struct {
	Project Ref `json:"project"`
	// Tests: the automated tests to run; Manual: selected tests that are
	// not automated (for information; they are not run by the pipeline).
	Tests        []SelectedTest        `json:"tests"`
	Manual       []SelectedTest        `json:"manual"`
	Requirements []SelectedRequirement `json:"requirements"`
	// Changes: the code changes (commits, merge requests) that were used.
	Changes []Ref `json:"changes"`
	// Unknown: keys that matched no requirement or test of the project.
	Unknown []string `json:"unknown"`
	Variant *Ref     `json:"variant,omitempty"`
	// Run created with create_run (its tests are the selected ones).
	Run *Ref `json:"run,omitempty"`
}

// Identifier is a test key as an automated test can carry it in a
// function or class name: hyphens become underscores (SHOP-TC-12 →
// SHOP_TC_12). The JUnit import accepts both forms.
func Identifier(key string) string { return strings.ReplaceAll(key, "-", "_") }

var keyPartsRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*)-TC-(\d+)$`)

// Filter formats of a selection.
var Formats = []string{"keys", "pytest", "go", "playwright", "maven", "json", "gitlab-child-pipeline"}

// Filter renders the keys as a test runner filter:
//
//	pytest      SHOP_TC_3 or SHOP_TC_7            (pytest -k "…")
//	go          (SHOP_TC_3|SHOP_TC_7)([^0-9]|$)    (go test -run '…')
//	playwright  (SHOP[-_]TC[-_]3|…)(?![0-9])       (npx playwright test --grep '…')
//	maven       *SHOP_TC_3*,*SHOP_TC_7*            (mvn test -Dtest='…')
//	keys        one key per line
//
// An empty selection renders as "" (no filter: run nothing, not all).
func Filter(format string, keys []string) (string, error) {
	if len(keys) == 0 {
		return "", nil
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = Identifier(k)
	}
	switch format {
	case "keys":
		return strings.Join(keys, "\n"), nil
	case "pytest":
		return strings.Join(ids, " or "), nil
	case "go":
		return "(" + strings.Join(ids, "|") + ")([^0-9]|$)", nil
	case "playwright":
		alts := make([]string, len(keys))
		for i, k := range keys {
			alts[i] = regexp.QuoteMeta(k)
			if m := keyPartsRe.FindStringSubmatch(k); m != nil {
				alts[i] = regexp.QuoteMeta(m[1]) + "[-_]TC[-_]" + m[2]
			}
		}
		return "(" + strings.Join(alts, "|") + ")(?![0-9])", nil
	case "maven", "junit":
		pats := make([]string, len(ids))
		for i, id := range ids {
			pats[i] = "*" + id + "*"
		}
		return strings.Join(pats, ","), nil
	}
	return "", fmt.Errorf("unknown format %q (use one of %s)", format, strings.Join(Formats, ", "))
}

// Keys returns the keys of the selected automated tests.
func (s *Selection) Keys() []string {
	out := make([]string, 0, len(s.Tests))
	for _, t := range s.Tests {
		out = append(out, t.Key)
	}
	return out
}

// Write renders the selection in a format other than the child pipeline.
func (s *Selection) Write(w io.Writer, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	f, err := Filter(format, s.Keys())
	if err != nil {
		return err
	}
	if f != "" {
		f += "\n"
	}
	_, err = io.WriteString(w, f)
	return err
}

// Groups returns the selected automated tests by runner, sorted by runner.
func (s *Selection) Groups() []Group {
	by := map[string][]SelectedTest{}
	for _, t := range s.Tests {
		r := t.Runner
		if r == "" {
			r = RunnerDefault
		}
		by[r] = append(by[r], t)
	}
	out := make([]Group, 0, len(by))
	for r, list := range by {
		out = append(out, Group{Runner: r, Tests: list})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Runner < out[j].Runner })
	return out
}

// Group is the tests of one runner.
type Group struct {
	Runner string
	Tests  []SelectedTest
}

// Keys of the group's tests.
func (g Group) Keys() []string {
	out := make([]string, len(g.Tests))
	for i, t := range g.Tests {
		out[i] = t.Key
	}
	return out
}

// RunnerDefault is the runner of tests without a runner tag or a known
// script language.
const RunnerDefault = "default"

// RunnerTagPrefix: a test tagged runner:<name> runs in the job of <name>.
const RunnerTagPrefix = "runner:"

// RunnerOf returns the runner of a test: its runner:<name> tag, else one
// derived from the script language, else "default".
func RunnerOf(tags []string, scriptLanguage string) string {
	for _, t := range tags {
		if strings.HasPrefix(strings.ToLower(t), RunnerTagPrefix) {
			if r := strings.TrimSpace(t[len(RunnerTagPrefix):]); r != "" {
				return strings.ToLower(r)
			}
		}
	}
	switch scriptLanguage {
	case "playwright":
		return "playwright"
	case "python":
		return "pytest"
	}
	return RunnerDefault
}
