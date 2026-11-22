package ci

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func sampleReport() *Report {
	return &Report{Pass: false, Verdict: VerdictFail, Project: Ref{ID: "p1", Key: "SHOP", Name: "Shop"},
		Release: &Ref{ID: "r1", Name: "1.0", URL: "https://q.example/p/p1/releases?r=r1"},
		Run:     &Ref{ID: "run1", Name: "Pipeline 7", URL: "https://q.example/p/p1/runs/run1"},
		Build:   &Ref{ID: "b1", Name: "7"}, Variant: &Ref{Name: "EU"}, Scope: "release", ScopeSize: 3, URL: "https://q.example/p/p1/runs/run1",
		Conditions: []Condition{
			{Type: "coverage", Threshold: 80, Label: ConditionLabel("coverage", 80, "", ""), State: StatePass, Met: true, Actual: 92.5, Total: 40, Available: true},
			{Type: "no_failed", Label: "no_failed", State: StateFail, Actual: 2, Available: true,
				Items: []Item{{Kind: "test", Key: "SHOP-TC-3", Title: "Pay | card", Note: "failed", URL: "https://q.example/t3", Variants: []string{"EU"}}}},
			{Type: "mandatory_passed", Label: "mandatory_passed", State: StateWaived, Met: true, Total: 4, Available: true,
				Waived: []Item{{Kind: "requirement", Key: "SHOP-REQ-2", Title: "SSO", Note: "SHOP-WVR-1"}}},
			{Type: "executed", Threshold: 100, Label: ConditionLabel("executed", 0, "", ""), State: StateFail},
		},
		Variants: []VariantVerdict{{Name: "EU", Pass: false}},
		Results:  &Results{Total: 10, Passed: 7, Failed: 2, Blocked: 1},
		Failures: []Item{{Key: "SHOP-TC-3", Title: "Pay", Note: "failed"}},
		Affected: &Affected{Requirements: []AffectedRequirement{{Key: "SHOP-REQ-1", Title: "Cart", Result: "passed", Tests: 2}, {Key: "SHOP-REQ-5", Title: "New", Result: "none"}}, Covered: 1, Passed: 1, Total: 2},
		Waivers:  []Item{{Key: "SHOP-WVR-1", URL: "https://q.example/w1"}}}
}

func TestReportRenderings(t *testing.T) {
	r := sampleReport()
	var b bytes.Buffer
	r.WriteText(&b)
	text := b.String()
	for _, want := range []string{"Qtestframe quality gate: FAILED", `release "1.0"`, `run "Pipeline 7"`, "[PASS]   coverage >= 80%", "92.5% of 40",
		"[FAIL]   no_failed", "SHOP-TC-3  Pay | card  (failed)  [EU]  https://q.example/t3", "[WAIVED] mandatory_passed", "waived: SHOP-REQ-2",
		"executed >= 100%", "nothing to measure", "variant EU: FAIL", "results: 7 passed, 2 failed, 1 blocked (10)", "affected requirements: 2, 1 covered, 1 passed",
		"details: https://q.example/p/p1/runs/run1"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	md := r.Markdown()
	for _, want := range []string{NoteMarker("shop"), "quality gate failed", "[1.0](https://q.example/p/p1/releases?r=r1)", "Pay \\| card", "| ❌ | `no_failed` | 2 found |",
		"☑️ waived", "<details>", "**Requirements of this change:** 2", "⚠️ no tests", "Waivers in force: [SHOP-WVR-1](https://q.example/w1)", "[Open in Qtestframe]"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	if !strings.HasPrefix(md, "<!-- qtestframe:ci-summary:SHOP -->\n") {
		t.Error(md)
	}
	j, err := r.JUnit()
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Tests    int `xml:"tests,attr"`
		Failures int `xml:"failures,attr"`
		Cases    []struct {
			Name    string `xml:"name,attr"`
			Failure *struct {
				Message string `xml:"message,attr"`
				Text    string `xml:",chardata"`
			} `xml:"failure"`
		} `xml:"testcase"`
	}
	if err := xml.Unmarshal(j, &suite); err != nil {
		t.Fatal(err, string(j))
	}
	if suite.Tests != 4 || suite.Failures != 2 || suite.Cases[1].Failure == nil || !strings.Contains(suite.Cases[1].Failure.Text, "SHOP-TC-3") || suite.Cases[0].Failure != nil {
		t.Fatal(string(j))
	}
	for _, f := range []string{"text", "json", "junit", "markdown", "md", ""} {
		var out bytes.Buffer
		if err := r.Write(&out, f); err != nil || out.Len() == 0 {
			t.Errorf("%s: %v", f, err)
		}
	}
	var back Report
	var out bytes.Buffer
	_ = r.Write(&out, "json")
	if err := json.Unmarshal(out.Bytes(), &back); err != nil || back.Release.Name != "1.0" {
		t.Fatal(err)
	}
	if err := r.Write(&out, "html"); err == nil {
		t.Error("html accepted")
	}
	if d := r.StatusDescription(); d != "gate failed (2 of 4 conditions); 7 passed, 2 failed, 1 blocked (10)" {
		t.Error(d)
	}
	ok := &Report{Pass: true, SummaryOnly: true, Project: Ref{Key: "X"}, Results: &Results{}}
	b.Reset()
	ok.WriteText(&b)
	if !strings.Contains(b.String(), "Qtestframe results  (project X)") || !strings.Contains(b.String(), "no results") || ok.StatusDescription() != "no results" {
		t.Error(b.String(), ok.StatusDescription())
	}
	if !strings.Contains(ok.Markdown(), "Qtestframe test results") {
		t.Error(ok.Markdown())
	}
	pass := &Report{Pass: true, Project: Ref{Key: "X"}}
	if !strings.Contains(pass.Markdown(), "quality gate passed") || pass.StatusDescription() != "gate passed" {
		t.Error(pass.Markdown())
	}
}

func TestConditionLabels(t *testing.T) {
	for _, c := range []struct {
		typ, level, cycle string
		th                float64
		want              string
	}{
		{"coverage", "", "", 80, "coverage >= 80%"},
		{"passed", "", "", 95.5, "passed >= 95.5%"},
		{"cycle_executed", "", `"Sprint 3"`, 90, `cycle_executed "Sprint 3" >= 90%`},
		{"no_open_defects", "major", "", 0, "no_open_defects:major"},
		{"must", "", "", 0, "must"},
	} {
		if got := ConditionLabel(c.typ, c.th, c.level, c.cycle); got != c.want {
			t.Errorf("%s: %q, want %q", c.typ, got, c.want)
		}
	}
	m := (&Condition{Type: "mandatory_passed", Actual: 1, Total: 3}).measure()
	if m != "1 of 3 not passed" {
		t.Error(m)
	}
}

func TestDetect(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	gl := Detect(env(map[string]string{"GITLAB_CI": "true", "CI_PIPELINE_ID": "900", "CI_PIPELINE_IID": "12", "CI_PIPELINE_URL": "https://gl/p/-/pipelines/900",
		"CI_COMMIT_SHA": "abc1234", "CI_COMMIT_REF_NAME": "main", "CI_MERGE_REQUEST_IID": "5", "CI_MERGE_REQUEST_SOURCE_BRANCH_NAME": "feature",
		"CI_MERGE_REQUEST_DIFF_BASE_SHA": "base", "CI_API_V4_URL": "https://gl/api/v4", "CI_PROJECT_ID": "42", "CI_JOB_TOKEN": "job"}))
	if gl.System != SystemGitLab || gl.BuildID != "900" || gl.BuildNumber != "12" || gl.Branch != "feature" || gl.MergeRequest != "5" || gl.DiffBase != "base" ||
		gl.APIURL != "https://gl/api/v4" || gl.Project != "42" || gl.PipelineID != "900" || gl.JobToken != "job" || !gl.InCI() {
		t.Fatalf("%+v", gl)
	}
	gh := Detect(env(map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_RUN_ID": "77", "GITHUB_RUN_NUMBER": "3", "GITHUB_SHA": "def5678",
		"GITHUB_REF": "refs/pull/9/merge", "GITHUB_HEAD_REF": "topic", "GITHUB_REPOSITORY": "o/r", "GITHUB_SERVER_URL": "https://github.com", "GITHUB_API_URL": "https://api.github.com"}))
	if gh.System != SystemGitHub || gh.MergeRequest != "9" || gh.Branch != "topic" || gh.PipelineURL != "https://github.com/o/r/actions/runs/77" || gh.Project != "o/r" {
		t.Fatalf("%+v", gh)
	}
	j := Detect(env(map[string]string{"JENKINS_URL": "x", "BUILD_NUMBER": "4", "BUILD_TAG": "jenkins-app-4", "GIT_COMMIT": "aaa", "BRANCH_NAME": "dev", "CHANGE_ID": "11"}))
	if j.System != SystemJenkins || j.BuildID != "jenkins-app-4" || j.MergeRequest != "11" || j.Branch != "dev" {
		t.Fatalf("%+v", j)
	}
	g := Detect(env(map[string]string{"QTF_COMMIT": "fff", "QTF_BUILD_ID": "b"}))
	if g.System != SystemGeneric || g.Commit != "fff" || g.BuildID != "b" {
		t.Fatalf("%+v", g)
	}
	if Detect(env(map[string]string{"CI": "1"})).System != SystemGeneric || Detect(env(nil)).InCI() {
		t.Fatal("generic")
	}
}

func TestFilters(t *testing.T) {
	keys := []string{"SHOP-TC-3", "SHOP-TC-17"}
	for f, want := range map[string]string{
		"keys":       "SHOP-TC-3\nSHOP-TC-17",
		"pytest":     "SHOP_TC_3 or SHOP_TC_17",
		"go":         "(SHOP_TC_3|SHOP_TC_17)([^0-9]|$)",
		"playwright": "(SHOP[-_]TC[-_]3|SHOP[-_]TC[-_]17)(?![0-9])",
		"maven":      "*SHOP_TC_3*,*SHOP_TC_17*",
	} {
		got, err := Filter(f, keys)
		if err != nil || got != want {
			t.Errorf("%s: %q %v", f, got, err)
		}
	}
	if got, _ := Filter("pytest", nil); got != "" {
		t.Error(got)
	}
	if _, err := Filter("nope", keys); err == nil {
		t.Error("nope")
	}
	if RunnerOf([]string{"x", "Runner:Go"}, "") != "go" || RunnerOf(nil, "playwright") != "playwright" || RunnerOf(nil, "python") != "pytest" || RunnerOf(nil, "sh") != RunnerDefault {
		t.Error("runners")
	}
	sel := &Selection{Project: Ref{Key: "SHOP"}, Tests: []SelectedTest{{Key: "SHOP-TC-3", Runner: "pytest"}, {Key: "SHOP-TC-4"}}}
	var b bytes.Buffer
	if err := sel.Write(&b, "pytest"); err != nil || b.String() != "SHOP_TC_3 or SHOP_TC_4\n" {
		t.Error(b.String(), err)
	}
	b.Reset()
	if err := sel.Write(&b, "json"); err != nil || !strings.Contains(b.String(), `"key": "SHOP-TC-3"`) {
		t.Error(b.String())
	}
	b.Reset()
	if err := (&Selection{}).Write(&b, "go"); err != nil || b.Len() != 0 {
		t.Error(b.String())
	}
}

func TestChildPipeline(t *testing.T) {
	sel := &Selection{Project: Ref{Key: "SHOP"}, Run: &Ref{ID: "run-1", Name: "Pipeline 7"}, Tests: []SelectedTest{
		{Key: "SHOP-TC-3", Runner: "pytest"}, {Key: "SHOP-TC-4", Runner: "pytest"}, {Key: "SHOP-TC-9", Runner: "playwright"}, {Key: "SHOP-TC-10"}}}
	y, err := sel.ChildPipeline(PipelineOptions{RunID: "run-1", Images: map[string]string{"pytest": "python:3.13"}, Commands: map[string]string{"default": "make test TESTS=\"$QTF_TESTS\""},
		GateArgs: []string{"--release", "1.0 beta"}})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(y), &doc); err != nil {
		t.Fatal(err, y)
	}
	py := doc["qtestframe-tests:pytest"].(map[string]any)
	vars := py["variables"].(map[string]any)
	if py["image"] != "python:3.13" || vars["QTF_FILTER"] != "SHOP_TC_3 or SHOP_TC_4" || vars["QTF_TESTS"] != "SHOP-TC-3 SHOP-TC-4" || py["stage"] != "test" {
		t.Fatal(py)
	}
	def := doc["qtestframe-tests:default"].(map[string]any)
	if def["variables"].(map[string]any)["QTF_FILTER"] != "SHOP-TC-10" || !strings.Contains(def["script"].([]any)[1].(string), "make test") {
		t.Fatal(def)
	}
	pw := doc["qtestframe-tests:playwright"].(map[string]any)
	if !strings.Contains(pw["script"].([]any)[1].(string), "--grep") {
		t.Fatal(pw)
	}
	rep := doc["qtestframe-report"].(map[string]any)
	script := rep["script"].([]any)
	if rep["when"] != "always" || rep["image"] != "$QTF_CLI_IMAGE" || len(rep["needs"].([]any)) != 3 ||
		script[0] != "qtestframe report --run-id run-1 --junit 'qtf-results/*.xml'" || script[1] != "qtestframe gate --run-id run-1 --release '1.0 beta' --format junit --output qtf-gate.xml" {
		t.Fatal(rep)
	}
	// without a run: the run is created by report; no gate
	sel.Run = nil
	y, _ = sel.ChildPipeline(PipelineOptions{NoGate: true, CLIImage: "registry.example/qtf:1"})
	if !strings.Contains(y, `"qtestframe report --run \"$QTF_RUN\" --create-run --junit 'qtf-results/*.xml'"`) || strings.Contains(y, "qtestframe gate") || !strings.Contains(y, "registry.example/qtf:1") {
		t.Fatal(y)
	}
	empty, _ := (&Selection{Project: Ref{Key: "SHOP"}}).ChildPipeline(PipelineOptions{})
	if err := yaml.Unmarshal([]byte(empty), &doc); err != nil || !strings.Contains(empty, "qtestframe-no-tests") {
		t.Fatal(empty, err)
	}
	if shellQuote("") != "''" || shellQuote("it's") != `'it'"'"'s'` {
		t.Error(shellQuote("it's"))
	}
}

func TestPathMap(t *testing.T) {
	m, err := ParsePathMap([]byte(`
- paths: ["src/cart/**", "api/cart.go"]
  tests: [SHOP-TC-3]
  tags: [cart]
- paths: ["src/payment/"]
  requirements: [SHOP-REQ-4]
- paths: ["db/*.sql"]
  tags: [smoke, cart]
`))
	if err != nil {
		t.Fatal(err)
	}
	h := m.Match([]string{"src/cart/a/b.go", "db/1.sql", "README.md"})
	if strings.Join(h.Tests, ",") != "SHOP-TC-3" || strings.Join(h.Tags, ",") != "cart,smoke" || len(h.Requirements) != 0 || h.Why["src/cart/**"] != "src/cart/a/b.go" {
		t.Fatalf("%+v", h)
	}
	if h := m.Match([]string{"src/payment/x/y.go"}); strings.Join(h.Requirements, ",") != "SHOP-REQ-4" {
		t.Fatalf("%+v", h)
	}
	for pat, f := range map[string]string{"**/*.go": "a/b/c.go", "src/*.go": "src/a.go", "./a": "a", "**": "x/y"} {
		if !MatchPath(pat, f) {
			t.Errorf("%s !~ %s", pat, f)
		}
	}
	for pat, f := range map[string]string{"src/*.go": "src/a/b.go", "a/**/c": "a/b/d", "src/cart/**": "src/other"} {
		if MatchPath(pat, f) {
			t.Errorf("%s ~ %s", pat, f)
		}
	}
	for _, bad := range []string{"- tests: [X]", "- paths: ['[']", "- paths: [a]\n  unknown: 1", "{"} {
		if _, err := ParsePathMap([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// TestTemplates: the templates are valid YAML and examples/ci holds them
// rendered with the example values.
func TestTemplates(t *testing.T) {
	for _, tp := range Templates {
		s, ok := RenderTemplate(tp.Name, "https://q.example/", "ABC")
		if !ok || strings.Contains(s, "__QTF_") || !strings.Contains(s, "https://q.example") || !strings.Contains(s, "ABC") {
			t.Errorf("%s: %v", tp.Name, ok)
		}
		// a component has a spec document and the jobs
		dec := yaml.NewDecoder(strings.NewReader(s))
		for {
			var v any
			if err := dec.Decode(&v); err != nil {
				if err.Error() != "EOF" {
					t.Errorf("%s: %v", tp.Name, err)
				}
				break
			}
		}
		want, _ := RenderTemplate(tp.Name, "", "")
		got, err := os.ReadFile(filepath.Join("..", "..", "examples", "ci", tp.Name))
		if err != nil || string(got) != want {
			t.Errorf("examples/ci/%s differs from internal/ci/templates (copy it with the example values %s and %s)", tp.Name, ExampleServerURL, ExampleProject)
		}
	}
	if _, ok := RenderTemplate("../ci.go", "", ""); ok {
		t.Error("path accepted")
	}
	if _, ok := RenderTemplate("nope.yml", "", ""); ok {
		t.Error("unknown accepted")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "ci", "qtestframe-map.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePathMap(data); err != nil {
		t.Fatal(err)
	}
}
