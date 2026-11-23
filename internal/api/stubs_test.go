package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/uploader"
)

// stubsFixture: project ST with the automated test ST-TC-1 (steps, test
// data, verifying ST-REQ-1), the manual ST-TC-2 and the automated ST-TC-3
// with a shell script.
func stubsFixture(t *testing.T, a *client) (pid string, ids map[string]string) {
	t.Helper()
	pid = a.project("ST")
	pp := "/api/v1/projects/" + pid
	ids = map[string]string{}
	req := a.m("POST", pp+"/requirements", map[string]any{"title": "Pay by card"}, 201)
	for _, b := range []map[string]any{
		{"title": "Pay with a saved card", "execution_type": "automated", "preconditions": "Logged in", "expected_result": "Paid",
			"steps": []map[string]any{{"action": "Open the cart", "expected": "2 items"}, {"action": "Pay with {{card}}", "expected": "Receipt"}}},
		{"title": "Looks right"},
		{"title": "Scripted", "execution_type": "automated", "script": "echo ok"},
	} {
		m := a.m("POST", pp+"/tests", b, 201)
		ids[m["key"].(string)] = m["id"].(string)
	}
	a.m("PUT", "/api/v1/tests/"+ids["ST-TC-1"]+"/data", map[string]any{"parameters": []string{"card"},
		"data_rows": []map[string]any{{"name": "visa", "values": map[string]string{"card": "4242"}}, {"name": "amex", "values": map[string]string{"card": "3782"}}}}, 200)
	a.m("POST", pp+"/links", map[string]any{"requirement_id": req["id"], "test_case_id": ids["ST-TC-1"]}, 201)
	return pid, ids
}

func (c *client) body(method, path string, body any, want int) ([]byte, map[string][]string) {
	c.e.t.Helper()
	res := c.raw(method, path, body)
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, want, data)
	}
	return data, res.Header
}

func TestTestStubs(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid, ids := stubsFixture(t, a)
	base := "/api/v1/projects/" + pid + "/tests/stubs"

	// JSON: the automated tests without a script
	q := url.QueryEscape("execution_type:automated has:!script")
	res := a.m("GET", base+"?framework=pytest&format=json&q="+q, nil, 200)
	files := toMaps(res["files"])
	if len(files) != 1 || files[0]["path"] != "test_ST_TC_1.py" || files[0]["key"] != "ST-TC-1" || files[0]["test_id"] != ids["ST-TC-1"] ||
		res["framework"].(map[string]any)["id"] != "pytest" {
		t.Fatal(res)
	}
	c := files[0]["content"].(string)
	for _, want := range []string{"# qtestframe:begin ST-TC-1 pytest body=", "# Requirements: ST-REQ-1", "#   Logged in", "http://example.test/p/" + pid + "/tests?item=" + ids["ST-TC-1"],
		"def test_ST_TC_1_pay_with_a_saved_card(card):", `pytest.param("4242", id="visa")`, "# 2. TODO: Pay with {{card}}", "#    Expected: Receipt", "@pytest.mark.skip"} {
		if !strings.Contains(c, want) {
			t.Fatalf("missing %q in\n%s", want, c)
		}
	}
	// ids (keys or ids), query alias, every framework
	for _, fw := range []string{"pytest", "playwright", "go", "junit5", "jest", "vitest", "robot", "cucumber"} {
		res = a.m("POST", base, map[string]any{"framework": fw, "format": "json", "ids": []string{"ST-TC-1", ids["ST-TC-2"]}}, 200)
		if len(toMaps(res["files"])) != 2 {
			t.Fatal(fw, res)
		}
	}
	res = a.m("POST", base, map[string]any{"framework": "go", "format": "json", "query": "key:ST-TC-3", "package": "e2e"}, 200)
	if fs := toMaps(res["files"]); len(fs) != 1 || fs[0]["path"] != "st_tc_3_test.go" || !strings.Contains(fs[0]["content"].(string), "package e2e") {
		t.Fatal(res)
	}

	// zip (the default), with a directory
	data, hdr := a.body("GET", base+"?framework=junit5&dir=src/test/java/&ids="+ids["ST-TC-1"]+","+ids["ST-TC-3"], nil, 200)
	if hdr["Content-Type"][0] != "application/zip" || !strings.Contains(hdr["Content-Disposition"][0], "st-junit5-stubs.zip") {
		t.Fatal(hdr)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(zr.File) != 2 || zr.File[0].Name != "src/test/java/ST_TC_1Test.java" {
		t.Fatal(err, zr.File)
	}
	// a single file
	data, hdr = a.body("GET", base+"?framework=robot&format=file&ids="+ids["ST-TC-2"], nil, 200)
	if !strings.HasPrefix(hdr["Content-Type"][0], "text/plain") || !strings.Contains(hdr["Content-Disposition"][0], "ST-TC-2.robot") || !strings.Contains(string(data), "*** Test Cases ***") {
		t.Fatal(hdr, string(data))
	}
	// errors
	a.m("GET", base+"?framework=robot&format=file", nil, 400)
	a.m("GET", base+"?framework=cobol", nil, 400)
	a.m("GET", base+"?framework=go&format=tar", nil, 400)
	a.m("GET", base+"?framework=go&dir=../x", nil, 400)
	a.m("GET", base+"?framework=go&dir=/abs", nil, 400)
	a.m("GET", base+"?framework=go&q=key:NOPE", nil, 400)
	a.m("GET", base+"?framework=go&q=bogus:x", nil, 400)
	if r := a.m("GET", base+"?framework=go&format=json&q=key:NOPE", nil, 200); len(toMaps(r["files"])) != 0 {
		t.Fatal(r)
	}

	// a viewer reads stubs but does not mark scripts
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", base+"?framework=go&format=json", nil, 200)
	v.m("POST", base, map[string]any{"framework": "go", "format": "json", "mark_script": true}, 403)

	// mark the scripts: only tests without one
	res = a.m("POST", base, map[string]any{"framework": "pytest", "format": "json", "dir": "./tests", "mark_script": true, "ids": []string{"ST-TC-1", "ST-TC-2", "ST-TC-3"}}, 200)
	if res["marked"] != 2.0 || toMaps(res["files"])[0]["path"] != "tests/test_ST_TC_1.py" {
		t.Fatal(res)
	}
	t1 := a.m("GET", "/api/v1/tests/"+ids["ST-TC-1"], nil, 200)
	if t1["script"] != "qtestframe:stub tests/test_ST_TC_1.py" || t1["script_language"] != "python" || !strings.Contains(strings.Join(strs2(t1["tags"]), ","), "runner:pytest") {
		t.Fatal(t1)
	}
	if t2 := a.m("GET", "/api/v1/tests/"+ids["ST-TC-2"], nil, 200); t2["execution_type"] != "automated" {
		t.Fatal(t2)
	}
	if t3 := a.m("GET", "/api/v1/tests/"+ids["ST-TC-3"], nil, 200); t3["script"] != "echo ok" {
		t.Fatal(t3)
	}
	hist := items(a.m("GET", "/api/v1/tests/"+ids["ST-TC-1"]+"/history", nil, 200))
	if len(hist) < 2 {
		t.Fatal(hist)
	}
	// again: nothing to do; a GET never marks
	if r := a.m("POST", base, map[string]any{"framework": "pytest", "format": "json", "dir": "tests", "mark_script": true, "ids": []string{"ST-TC-1"}}, 200); r["marked"] != 0.0 {
		t.Fatal(r)
	}
	// now has:!script leaves them out
	if r := a.m("GET", base+"?framework=pytest&format=json&q="+q, nil, 200); len(toMaps(r["files"])) != 0 {
		t.Fatal(r)
	}
	// the server does not run a stub reference
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{ids["ST-TC-1"], ids["ST-TC-3"]}}, 201)["id"].(string)
	r := a.m("POST", "/api/v1/runs/"+run+"/run-automated", nil, 202)
	var codes []string
	for _, s := range toMaps(r["skipped"]) {
		codes = append(codes, s["test_key"].(string)+":"+s["code"].(string))
	}
	if strings.Join(codes, ",") != "ST-TC-1:external" {
		t.Fatal(r)
	}
	ex := executionsByKey(a, run)["ST-TC-1"]
	a.m("POST", "/api/v1/executions/"+ex["id"].(string)+"/run-script", nil, 400)
}

// TestStubResultsMapBack: a generated stub, run by its framework, reports
// skipped through qtestframe report; filled in, it reports passed.
func TestStubResultsMapBack(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid, ids := stubsFixture(t, a)
	e.member("tester@example.com", rbac.RoleTester)
	tok := e.login("tester@example.com").m("POST", "/api/v1/tokens", map[string]any{"name": "ci"}, 201)["token"].(string)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "Pipeline 1", "test_ids": []string{ids["ST-TC-1"], ids["ST-TC-2"]}}, 201)["id"].(string)

	report := func(t *testing.T, doc []byte) map[string]map[string]any {
		t.Helper()
		f := filepath.Join(t.TempDir(), "results.xml")
		if err := os.WriteFile(f, doc, 0o644); err != nil {
			t.Fatal(err)
		}
		sum, err := uploader.Report(context.Background(), uploader.Options{URL: e.hs.URL, Token: tok, RunID: run, JUnit: []string{f}})
		if err != nil {
			t.Fatal(err)
		}
		if len(sum.Unmapped) != 0 || len(sum.Unknown) != 0 || sum.Updated != 2 {
			t.Fatalf("%+v\n%s", sum, doc)
		}
		return executionsByKey(a, run)
	}
	stubs := func(fw string) []map[string]any {
		return toMaps(a.m("POST", "/api/v1/projects/"+pid+"/tests/stubs", map[string]any{"framework": fw, "format": "json", "ids": []string{"ST-TC-1", "ST-TC-2"}}, 200)["files"])
	}

	t.Run("go", func(t *testing.T) {
		gobin, err := exec.LookPath("go")
		if err != nil {
			t.Skip("go is not installed")
		}
		dir := t.TempDir()
		write := func(name, content string) {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write("go.mod", "module example.com/shop\n\ngo 1.21\n")
		files := stubs("go")
		goTest := func() []byte {
			cmd := exec.Command(gobin, "test", "-json", "-count=1", ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=-mod=mod")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("go test: %v\n%s", err, out)
			}
			return goTestJUnit(t, out)
		}
		for _, f := range files {
			write(f["path"].(string), f["content"].(string))
		}
		ex := report(t, goTest())
		if ex["ST-TC-1"]["status"] != "skipped" || ex["ST-TC-2"]["status"] != "skipped" {
			t.Fatal(ex)
		}
		// fill in one stub (the data-driven one: its rows run as subtests)
		for _, f := range files {
			c := f["content"].(string)
			if f["key"] == "ST-TC-1" {
				c = strings.Replace(c, "\tt.Skip(\"qtestframe stub: not implemented\")\n", "", 1)
			}
			write(f["path"].(string), c)
		}
		ex = report(t, goTest())
		if ex["ST-TC-1"]["status"] != "passed" || ex["ST-TC-2"]["status"] != "skipped" {
			t.Fatal(ex)
		}
	})

	t.Run("pytest", func(t *testing.T) {
		py, err := exec.LookPath("python3")
		if err != nil {
			t.Skip("python3 is not installed")
		}
		if err := exec.Command(py, "-I", "-c", "import pytest").Run(); err != nil {
			t.Skip("pytest is not installed")
		}
		dir := t.TempDir()
		files := stubs("pytest")
		pytest := func() []byte {
			x := filepath.Join(t.TempDir(), "junit.xml")
			cmd := exec.Command(py, "-m", "pytest", "-q", "-p", "no:cacheprovider", "--junitxml", x, ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("pytest: %v\n%s", err, out)
			}
			b, err := os.ReadFile(x)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		for _, f := range files {
			c := f["content"].(string)
			if f["key"] == "ST-TC-2" {
				// filled in: no skip marker, an assertion
				c = strings.Replace(c, "@pytest.mark.skip(reason=\"qtestframe stub: not implemented\")\n", "", 1) + "    assert 1 + 1 == 2\n"
			}
			if err := os.WriteFile(filepath.Join(dir, f["path"].(string)), []byte(c), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		ex := report(t, pytest())
		if ex["ST-TC-1"]["status"] != "skipped" || ex["ST-TC-2"]["status"] != "passed" {
			t.Fatal(ex)
		}
	})
}

// goTestJUnit converts `go test -json` output to JUnit XML, like
// go-junit-report or gotestsum.
func goTestJUnit(t *testing.T, out []byte) []byte {
	t.Helper()
	type tc struct {
		XMLName   xml.Name  `xml:"testcase"`
		Name      string    `xml:"name,attr"`
		ClassName string    `xml:"classname,attr"`
		Skipped   *struct{} `xml:"skipped"`
		Failure   *struct{} `xml:"failure"`
	}
	var cases []tc
	for _, l := range bytes.Split(out, []byte("\n")) {
		var ev struct{ Action, Test, Package string }
		if json.Unmarshal(l, &ev) != nil || ev.Test == "" {
			continue
		}
		c := tc{Name: ev.Test, ClassName: ev.Package}
		switch ev.Action {
		case "pass":
		case "skip":
			c.Skipped = &struct{}{}
		case "fail":
			c.Failure = &struct{}{}
		default:
			continue
		}
		cases = append(cases, c)
	}
	b, err := xml.Marshal(struct {
		XMLName xml.Name `xml:"testsuite"`
		Cases   []tc
	}{Cases: cases})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
