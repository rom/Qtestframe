package stubgen

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/junit"
)

// The tests below compile or parse the generated stubs with the real tools
// where they are installed (go always; python, pytest, javac, node and the
// web/ TypeScript compiler when present) and check that the JUnit XML of a
// run maps back to the test keys: skipped while a stub is unimplemented,
// passed once it is filled in.

func writeStubs(t *testing.T, dir, fw string, o Options) []File {
	t.Helper()
	var out []File
	for _, tc := range fixtures() {
		f, err := Generate(fw, tc, o)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f.Path), []byte(f.Content), 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

func run(t *testing.T, dir string, env []string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	err := cmd.Run()
	return b.String(), err
}

// goJUnit converts `go test -json` output to JUnit XML (as go-junit-report
// or gotestsum would).
func goJUnit(t *testing.T, out string) []byte {
	t.Helper()
	type tc struct {
		XMLName   xml.Name  `xml:"testcase"`
		Name      string    `xml:"name,attr"`
		ClassName string    `xml:"classname,attr"`
		Skipped   *struct{} `xml:"skipped"`
		Failure   *struct{} `xml:"failure"`
	}
	var cases []tc
	for _, l := range strings.Split(out, "\n") {
		var ev struct{ Action, Test, Package string }
		if json.Unmarshal([]byte(l), &ev) != nil || ev.Test == "" {
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

// statuses maps a JUnit XML document to key → status.
func statuses(t *testing.T, doc []byte) map[string]string {
	t.Helper()
	cases, err := junit.ParseBytes(doc)
	if err != nil {
		t.Fatalf("%v\n%s", err, doc)
	}
	mapped, unmapped := junit.Aggregate(cases)
	if len(unmapped) > 0 {
		t.Fatalf("unmapped results: %+v", unmapped)
	}
	out := map[string]string{}
	for _, m := range mapped {
		out[m.Key] = m.Status
	}
	return out
}

func wantAll(t *testing.T, got map[string]string, status string) {
	t.Helper()
	if len(got) != len(fixtures()) {
		t.Fatalf("%d results, want %d: %v", len(got), len(fixtures()), got)
	}
	for k, s := range got {
		if s != status {
			t.Fatalf("%s: %s, want %s (%v)", k, s, status, got)
		}
	}
}

func TestGoStubsVetAndMap(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/stubs\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := writeStubs(t, dir, "go", Options{})
	env := []string{"GOTOOLCHAIN=local", "GOFLAGS=-mod=mod", "GOWORK=off"}
	if out, err := run(t, dir, env, gobin, "vet", "./..."); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
	if out, err := run(t, dir, env, "gofmt", "-l", "."); err == nil && strings.TrimSpace(out) != "" {
		t.Fatalf("not gofmt-clean: %s", out)
	}
	out, err := run(t, dir, env, gobin, "test", "-json", "-count=1", "./...")
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
	wantAll(t, statuses(t, goJUnit(t, out)), junit.Skipped)
	// fill in the stubs: without the skip they pass
	for _, f := range files {
		impl := strings.Replace(f.Content, "\tt.Skip(\"qtestframe stub: not implemented\")\n", "", 1)
		if err := os.WriteFile(filepath.Join(dir, f.Path), []byte(impl), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err = run(t, dir, env, gobin, "test", "-json", "-count=1", "./...")
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
	wantAll(t, statuses(t, goJUnit(t, out)), junit.Passed)
}

func TestPythonStubsCompileAndMap(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	dir := t.TempDir()
	files := writeStubs(t, dir, "pytest", Options{})
	args := []string{"-I", "-m", "py_compile"}
	for _, f := range files {
		args = append(args, f.Path)
	}
	if out, err := run(t, dir, nil, py, args...); err != nil {
		t.Fatalf("py_compile: %v\n%s", err, out)
	}
	if _, err := run(t, dir, nil, py, "-I", "-c", "import pytest"); err != nil {
		t.Log("pytest is not installed: only compiled")
		return
	}
	junitRun := func() map[string]string {
		x := filepath.Join(t.TempDir(), "junit.xml")
		out, err := run(t, dir, []string{"PYTHONDONTWRITEBYTECODE=1"}, py, "-m", "pytest", "-q", "-p", "no:cacheprovider", "--junitxml", x, ".")
		if err != nil {
			t.Fatalf("pytest: %v\n%s", err, out)
		}
		doc, err := os.ReadFile(x)
		if err != nil {
			t.Fatal(err)
		}
		return statuses(t, doc)
	}
	wantAll(t, junitRun(), junit.Skipped)
	for _, f := range files {
		impl := strings.Replace(f.Content, "@pytest.mark.skip(reason=\"qtestframe stub: not implemented\")\n", "", 1)
		if err := os.WriteFile(filepath.Join(dir, f.Path), []byte(impl), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantAll(t, junitRun(), junit.Passed)
}

// fakeJUnit are minimal JUnit 5 annotations, enough for javac to check the
// stubs without the JUnit jars.
var fakeJUnit = map[string]string{
	"org/junit/jupiter/api/Test.java":                  "package org.junit.jupiter.api; public @interface Test {}",
	"org/junit/jupiter/api/Disabled.java":              "package org.junit.jupiter.api; public @interface Disabled { String value() default \"\"; }",
	"org/junit/jupiter/api/DisplayName.java":           "package org.junit.jupiter.api; public @interface DisplayName { String value(); }",
	"org/junit/jupiter/api/Tag.java":                   "package org.junit.jupiter.api; public @interface Tag { String value(); }",
	"org/junit/jupiter/params/ParameterizedTest.java":  "package org.junit.jupiter.params; public @interface ParameterizedTest { String name() default \"\"; }",
	"org/junit/jupiter/params/provider/CsvSource.java": "package org.junit.jupiter.params.provider; public @interface CsvSource { String[] value() default {}; }",
}

func TestJavaStubsCompile(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac is not installed")
	}
	dir := t.TempDir()
	var args []string
	for p, src := range fakeJUnit {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		args = append(args, filepath.FromSlash(p))
	}
	for _, f := range writeStubs(t, dir, "junit5", Options{}) {
		args = append(args, f.Path)
	}
	if out, err := run(t, dir, nil, javac, append([]string{"-encoding", "UTF-8", "-d", filepath.Join(dir, "out")}, args...)...); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
}

// tsCheck: syntax of every file, types against the packages of web/ for the
// Playwright and Vitest stubs.
const tsCheck = `
const [tsPath, nodeModules, ...files] = process.argv.slice(2);
const ts = require(tsPath);
const opts = { strict: true, noEmit: true, target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext,
  moduleResolution: ts.ModuleResolutionKind.Bundler, skipLibCheck: true, types: [], baseUrl: nodeModules,
  paths: { '*': [nodeModules + '/*'] } };
let bad = 0;
const typed = [];
for (const f of files) {
  const src = require('fs').readFileSync(f, 'utf8');
  const r = ts.transpileModule(src, { reportDiagnostics: true, fileName: f, compilerOptions: { target: ts.ScriptTarget.ES2022 } });
  for (const d of r.diagnostics) { bad++; console.log(f + ': ' + ts.flattenDiagnosticMessageText(d.messageText, '\n')); }
  if (!src.includes('@jest/globals')) typed.push(f);
}
const prog = ts.createProgram(typed, opts);
for (const d of ts.getPreEmitDiagnostics(prog)) {
  bad++;
  console.log((d.file ? d.file.fileName : '') + ': ' + ts.flattenDiagnosticMessageText(d.messageText, '\n'));
}
process.exit(bad ? 1 : 0);
`

func TestTypeScriptStubsCompile(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	nm, _ := filepath.Abs(filepath.Join("..", "..", "web", "node_modules"))
	tsPath := filepath.Join(nm, "typescript")
	for _, p := range []string{tsPath, filepath.Join(nm, "@playwright", "test"), filepath.Join(nm, "vitest")} {
		if _, err := os.Stat(p); err != nil {
			t.Skip("web/node_modules is not installed (npm ci in web/)")
		}
	}
	dir := t.TempDir()
	script := filepath.Join(t.TempDir(), "check.js")
	if err := os.WriteFile(script, []byte(tsCheck), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{script, tsPath, nm}
	for _, fw := range []string{"playwright", "jest", "vitest"} {
		sub := filepath.Join(dir, fw)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range writeStubs(t, sub, fw, Options{}) {
			args = append(args, filepath.Join(sub, f.Path))
		}
	}
	if out, err := run(t, dir, nil, node, args...); err != nil {
		t.Fatalf("TypeScript: %v\n%s", err, out)
	}
}
