package store_test

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestPlaceholders(t *testing.T) {
	v := map[string]string{"user": "alice", "pw": "s3cret", "å": "ö"}
	for in, want := range map[string]string{
		"":                         "",
		"no placeholders":          "no placeholders",
		"Log in as {{user}}":       "Log in as alice",
		"{{ user }}/{{pw}}":        "alice/s3cret",
		"{{unknown}} {{user}}":     "{{unknown}} alice",
		"{{å}}":                    "ö",
		"{user} {{ 1x }} {{user}}": "{user} {{ 1x }} alice",
	} {
		if got := store.ResolvePlaceholders(in, v); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if got := store.ResolvePlaceholders("{{user}}", nil); got != "{{user}}" {
		t.Error(got)
	}
	if got := strings.Join(store.Placeholders("{{a}} {{ b }} {{a}} {{1}}"), ","); got != "a,b" {
		t.Error(got)
	}
	for name, ok := range map[string]bool{"user": true, "_x": true, "a.b-c": true, "åäö": true, "1a": false, "a b": false, "": false, strings.Repeat("a", 51): false} {
		if store.ValidParameterName(name) != ok {
			t.Error(name)
		}
	}
}

func TestDeriveResult(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{nil, model.ResultNotRun},
		{[]string{"passed", "failed"}, model.ResultFailed},
		{[]string{"passed", "blocked"}, model.ResultBlocked},
		{[]string{"passed", "skipped"}, model.ResultPassed},
		{[]string{"skipped"}, model.ResultSkipped},
		{[]string{"not_run", "not_run"}, model.ResultNotRun},
		{[]string{"passed", "not_run"}, model.ResultInProgress},
	} {
		if got := store.DeriveResult(c.in); got != c.want {
			t.Errorf("%v: %s, want %s", c.in, got, c.want)
		}
	}
}

func TestCoverageOfConfigurations(t *testing.T) {
	res := store.ConfigResults{
		"a": {"chrome": "passed", "firefox": "passed"},
		"b": {"chrome": "passed", "firefox": ""},
		"c": {"chrome": "blocked"},
		"d": {"": "passed"},
		"e": {"chrome": "skipped", "": "in_progress"},
	}
	for _, c := range []struct {
		tests []string
		want  string
	}{
		{nil, "none"},
		{[]string{"a"}, "passed"},
		{[]string{"a", "b"}, "partial"},
		{[]string{"a", "c"}, "failed"},
		{[]string{"d"}, "passed"},
		{[]string{"x"}, "not_run"},
		{[]string{"e"}, "not_run"},
	} {
		if got := store.CoverageOf(c.tests, res); got != c.want {
			t.Errorf("%v: %s, want %s", c.tests, got, c.want)
		}
	}
	// without configurations CoverageOf equals Coverage
	last := map[string]string{"a": "passed", "b": "failed"}
	plain := store.ConfigResults{"a": {"": "passed"}, "b": {"": "failed"}}
	for _, ts := range [][]string{{"a"}, {"a", "b"}, {"a", "z"}, {"z"}} {
		if store.CoverageOf(ts, plain) != store.Coverage(ts, last) {
			t.Error(ts)
		}
	}
	for _, c := range []struct {
		test, want string
	}{{"a", "passed"}, {"b", "in_progress"}, {"c", "blocked"}, {"e", "in_progress"}, {"x", "not_run"}} {
		if got := res.Combined(c.test); got != c.want {
			t.Errorf("combined %s: %s, want %s", c.test, got, c.want)
		}
	}
	if got := (store.ConfigResults{"f": {"x": "failed", "y": "blocked"}}).Combined("f"); got != "failed" {
		t.Error(got)
	}
	if got := (store.ConfigResults{"s": {"x": "skipped"}}).Combined("s"); got != "skipped" {
		t.Error(got)
	}
	for _, c := range []struct {
		tests     []string
		cfg, want string
	}{
		{nil, "chrome", "none"},
		{[]string{"a", "b"}, "chrome", "passed"},
		{[]string{"a", "b"}, "firefox", "partial"},
		{[]string{"a", "d"}, "firefox", "passed"},
		{[]string{"d"}, "chrome", "not_run"},
		{[]string{"d"}, "", "passed"},
	} {
		if got := store.CoverageInConfiguration(c.tests, res, c.cfg); got != c.want {
			t.Errorf("%v in %s: %s, want %s", c.tests, c.cfg, got, c.want)
		}
	}
	in := res.InConfiguration("chrome")
	if len(in) != 4 || in["a"] != "passed" || in["d"] != "" {
		t.Error(in)
	}
}

func TestValidateConfigurationAndTestData(t *testing.T) {
	c := &model.Configuration{Values: []model.DimensionValue{{Dimension: " OS ", Value: " Linux  Mint "}}}
	if err := store.ValidateConfiguration(c); err != nil || c.Name != "Linux Mint" || c.Values[0].Dimension != "OS" {
		t.Fatal(c, err)
	}
	if err := store.ValidateConfiguration(&model.Configuration{}); err == nil {
		t.Fatal("empty")
	}
	tc := &model.TestCase{Parameters: []string{" a "}, DataRows: []model.DataRow{{Name: " r ", Values: map[string]string{"a": "1", "b": "2"}}, {}}}
	if err := store.ValidateTestData(tc); err != nil || tc.Parameters[0] != "a" || tc.DataRows[0].Name != "r" || len(tc.DataRows[0].Values) != 1 || tc.DataRows[1].Values["a"] != "" {
		t.Fatal(tc, err)
	}
	empty := &model.TestCase{}
	if err := store.ValidateTestData(empty); err != nil || empty.Parameters == nil || empty.DataRows == nil {
		t.Fatal(empty)
	}
}

func TestConfigurationsStore(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "CS", Name: "CS"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	c := &model.Configuration{ProjectID: p.ID, Name: "Chrome"}
	must(t, st.CreateConfiguration(ctx, ten.ID, c))
	if _, err := st.FindConfiguration(ctx, ten.ID, p.ID, "CHROME"); err != nil {
		t.Fatal(err)
	}
	if f, err := st.FindConfiguration(ctx, ten.ID, p.ID, c.ID); err != nil || f.Name != "Chrome" {
		t.Fatal(f, err)
	}
	if _, err := st.FindConfiguration(ctx, ten.ID, p.ID, "Opera"); err == nil {
		t.Fatal("found Opera")
	}
	if err := st.CreateConfiguration(ctx, ten.ID, &model.Configuration{ProjectID: "missing", Name: "x"}); err == nil {
		t.Fatal("missing project")
	}
	tc := &model.TestCase{ProjectID: p.ID, Title: "T", Parameters: []string{"x"}, DataRows: []model.DataRow{{Values: map[string]string{"x": "1"}}}}
	must(t, st.CreateTest(ctx, ten.ID, tc))
	run := &model.TestRun{ProjectID: p.ID, Name: "R", Configurations: []model.ConfigurationRef{{ID: c.ID}}}
	must(t, st.CreateRun(ctx, ten.ID, run, []string{tc.ID}))
	es, err := st.ListExecutions(ctx, ten.ID, run.ID)
	if err != nil || len(es) != 1 || es[0].IterationCount != 1 || es[0].ConfigurationName != "Chrome" {
		t.Fatal(es, err)
	}
	res, err := st.LastConfigResults(ctx, ten.ID, p.ID)
	if err != nil || len(res[tc.ID]) != 1 {
		t.Fatal(res, err)
	}
	if _, err := st.SetRunConfigurations(ctx, ten.ID, "missing", nil); err == nil {
		t.Fatal("missing run")
	}
	// importing into a run with one configuration
	results := []store.ImportResult{{TestKey: tc.Key, Status: "passed"}}
	must(t, st.ResolveImportConfigurations(ctx, ten.ID, run.ID, "", results))
	if results[0].ConfigurationID == nil || *results[0].ConfigurationID != c.ID {
		t.Fatal(results)
	}
	if n, _, err := st.ImportResults(ctx, ten.ID, run.ID, "ci", results); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err := st.ResolveImportConfigurations(ctx, ten.ID, "missing", "", results); err == nil {
		t.Fatal("missing run")
	}
}
