package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

func TestWrapScript(t *testing.T) {
	body := "await page.goto('/login');\n\nawait expect(page.getByRole('heading')).toHaveText('Sign in')\n"
	got := WrapScript(`Login "page"`, body)
	for _, want := range []string{
		"import { test, expect } from '@playwright/test';",
		`test("Login \"page\"", async ({ page, context, request, baseURL }) => {`,
		"  await page.goto('/login');\n\n  await expect(",
		"});\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wrapped spec lacks %q:\n%s", want, got)
		}
	}
	if !strings.Contains(WrapScript("  ", "x"), `test("web test"`) {
		t.Error("empty title")
	}
	spec := "import { test } from '@playwright/test';\ntest('a', async ({ page }) => {});\n"
	if WrapScript("t", spec) != spec {
		t.Error("a complete spec must be used as is")
	}
	for s, want := range map[string]bool{
		"test('a', async () => {})":            true,
		"test.describe('x', () => {})":         true,
		"  test (\"b\", fn)":                   true,
		"await test.step('s', async () => {})": false,
		"latest('x')":                          false,
		"await page.goto('/')":                 false,
		"mytest('x')":                          false,
		"x.test('y')":                          false,
	} {
		if IsCompleteSpec(s) != want {
			t.Errorf("IsCompleteSpec(%q) = %v", s, !want)
		}
	}
}

func TestBrowserFor(t *testing.T) {
	for _, c := range []struct {
		v                string
		channels         bool
		browser, channel string
		ok               bool
	}{
		{"Chrome", false, "chromium", "", true},
		{"Google Chrome", true, "chromium", "chrome", true},
		{"Chromium", true, "chromium", "", true},
		{"Microsoft Edge", false, "chromium", "", true},
		{"Edge", true, "chromium", "msedge", true},
		{"Firefox", false, "firefox", "", true},
		{"Safari", false, "webkit", "", true},
		{" webkit ", false, "webkit", "", true},
		{"Opera", false, "", "", false},
	} {
		b, ch, ok := BrowserFor(c.v, c.channels)
		if b != c.browser || ch != c.channel || ok != c.ok {
			t.Errorf("BrowserFor(%q, %v) = %q %q %v", c.v, c.channels, b, ch, ok)
		}
	}
}

func TestMask(t *testing.T) {
	got := Mask("user alice pw s3cr3t and s3cr3t-long ab", []string{"s3cr3t", "s3cr3t-long", "ab", ""})
	if got != "user alice pw *** and *** ab" {
		t.Errorf("Mask = %q", got)
	}
	if StripANSI("\x1b[31mred\x1b[39m \x1b[2Kx") != "red x" {
		t.Error("StripANSI")
	}
}

func webRunner(t *testing.T, mutate func(*config.Runner)) *Runner {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}
	fake, err := filepath.Abs("testdata/fake-playwright.sh")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default().Runner
	cfg.Enabled, cfg.Sandbox = true, false
	cfg.WorkDir = filepath.Join(t.TempDir(), "w")
	cfg.Web.Enabled = true
	cfg.Web.Command = "/bin/sh " + fake
	cfg.Web.JobTimeout = 20 * time.Second
	if mutate != nil {
		mutate(&cfg)
	}
	return New(cfg)
}

func TestWebConfig(t *testing.T) {
	r := webRunner(t, func(c *config.Runner) {
		c.Web.Screenshots, c.Web.Trace, c.Web.Video, c.Web.Headless = "always", "off", "on_failure", false
		c.Web.ExecutablePath = "/opt/chromium/chrome"
	})
	parse := func(b []byte) map[string]any {
		t.Helper()
		s := strings.TrimSuffix(strings.TrimSpace(string(b)), ";")
		s = s[strings.Index(s, "module.exports = ")+len("module.exports = "):]
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("config is not JSON: %v\n%s", err, b)
		}
		return m
	}
	m := parse(r.WebConfig(WebJob{BaseURL: "https://app.example.com"}, "/jobs/web-1"))
	use := m["use"].(map[string]any)
	if m["testDir"] != "/jobs/web-1" || m["outputDir"] != "/jobs/web-1/results" || m["timeout"] != float64(60000) || m["workers"] != float64(1) {
		t.Errorf("config %v", m)
	}
	if use["baseURL"] != "https://app.example.com" || use["browserName"] != "chromium" || use["headless"] != false ||
		use["screenshot"] != "on" || use["trace"] != "off" || use["video"] != "retain-on-failure" || use["channel"] != nil {
		t.Errorf("use %v", use)
	}
	if lo, _ := use["launchOptions"].(map[string]any); lo["executablePath"] != "/opt/chromium/chrome" {
		t.Errorf("launchOptions %v", use["launchOptions"])
	}
	rep := m["reporter"].([]any)
	if len(rep) != 2 || rep[1].([]any)[1].(map[string]any)["outputFile"] != "/jobs/web-1/report.json" {
		t.Errorf("reporter %v", rep)
	}
	// a configuration's browser and channel; no executable path for them
	use = parse(r.WebConfig(WebJob{Browser: "chromium", Channel: "msedge"}, "/d"))["use"].(map[string]any)
	if use["channel"] != "msedge" || use["launchOptions"] != nil || use["baseURL"] != nil {
		t.Errorf("channel use %v", use)
	}
	use = parse(r.WebConfig(WebJob{Browser: "firefox", Channel: "chrome"}, "/d"))["use"].(map[string]any)
	if use["browserName"] != "firefox" || use["channel"] != nil || use["launchOptions"] != nil {
		t.Errorf("firefox use %v", use)
	}
	r.Cfg.Web.Screenshots, r.Cfg.Web.Trace = "off", "on_failure"
	use = parse(r.WebConfig(WebJob{}, "/d"))["use"].(map[string]any)
	if use["screenshot"] != "off" || use["trace"] != "retain-on-failure" {
		t.Errorf("modes %v", use)
	}
	r.Cfg.Web.Screenshots = "on_failure"
	if use = parse(r.WebConfig(WebJob{}, "/d"))["use"].(map[string]any); use["screenshot"] != "only-on-failure" {
		t.Errorf("on_failure %v", use)
	}
}

func TestWebArgvAndPaths(t *testing.T) {
	r := New(config.Runner{Web: config.WebRunner{Node: "/usr/bin/node", PlaywrightDir: "/opt/pw", BrowsersPath: "/opt/browsers", ExecutablePath: "/opt/chrome/chrome"}})
	if got := strings.Join(r.webArgv(), " "); got != "/usr/bin/node /opt/pw/node_modules/@playwright/test/cli.js" {
		t.Errorf("argv %q", got)
	}
	paths := strings.Join(r.webPaths(), " ")
	for _, want := range []string{"/opt/pw", "/opt/browsers", "/opt/chrome", "/usr/bin"} {
		if !strings.Contains(paths, want) {
			t.Errorf("paths %q lack %s", paths, want)
		}
	}
	r.Cfg.Web.Command = "npx playwright"
	if got := strings.Join(r.webArgv(), " "); got != "npx playwright" {
		t.Errorf("command argv %q", got)
	}
}

func TestParseReport(t *testing.T) {
	pass := `{"config":{},"suites":[{"title":"a.spec.ts","specs":[{"title":"one","tests":[{"status":"expected","results":[{"status":"passed","attachments":[{"name":"trace","path":"/x/trace.zip"}]}]}]}],
		"suites":[{"title":"group","specs":[{"title":"two","tests":[{"status":"flaky","results":[{"status":"failed","error":{"message":"first try"}},{"status":"passed"}]}]}]}]}],"errors":[]}`
	s, err := ParseReport([]byte(pass))
	if err != nil || s.Status != "passed" || s.Passed != 1 || s.Flaky != 1 || s.Message != "" || len(s.Attachments) != 1 {
		t.Errorf("pass: %+v %v", s, err)
	}
	fail := `{"stats":{},"suites":[{"title":"a.spec.ts","specs":[],"suites":[{"title":"g","specs":[{"title":"login","tests":[{"status":"unexpected","results":[{"status":"failed","errors":[{"message":"\u001b[31mboom\u001b[39m"}]}]}]},
		{"title":"slow","tests":[{"status":"unexpected","results":[{"status":"timedOut","error":{"message":"Test timeout of 30000ms exceeded."}}]}]}]}]}]}`
	s, err = ParseReport([]byte(fail))
	if err != nil || s.Status != "timedOut" || s.Failed != 2 || !strings.Contains(s.Message, "login: boom") || !strings.Contains(s.Message, "slow: Test timeout") {
		t.Errorf("fail: %+v %v", s, err)
	}
	s, err = ParseReport([]byte(`{"config":{},"suites":[],"errors":[{"message":"SyntaxError: Unexpected token"}]}`))
	if err != nil || s.Status != "failed" || s.Message != "SyntaxError: Unexpected token" {
		t.Errorf("errors: %+v %v", s, err)
	}
	s, err = ParseReport([]byte(`{"config":{},"suites":[{"title":"x","specs":[{"title":"s","tests":[{"status":"skipped","results":[]}]}]}]}`))
	if err != nil || s.Status != "skipped" || s.Skipped != 1 {
		t.Errorf("skipped: %+v %v", s, err)
	}
	s, err = ParseReport([]byte(`{"config":{},"suites":[]}`))
	if err != nil || s.Status != "failed" || !strings.Contains(s.Message, "no test") {
		t.Errorf("empty: %+v %v", s, err)
	}
	s, err = ParseReport([]byte(`{"config":{},"suites":[{"title":"x","specs":[{"title":"i","tests":[{"status":"unexpected","results":[{"status":"interrupted"}]}]}]}]}`))
	if err != nil || s.Status != "interrupted" {
		t.Errorf("interrupted: %+v %v", s, err)
	}
	if _, err := ParseReport([]byte(`{"a":1}`)); err == nil {
		t.Error("not a report")
	}
	if _, err := ParseReport([]byte(`nope`)); err == nil {
		t.Error("not JSON")
	}
}

func TestRunWebPass(t *testing.T) {
	r := webRunner(t, nil)
	if !r.WebEnabled() {
		t.Fatal("web not enabled")
	}
	res, err := r.RunWeb(context.Background(), WebJob{Title: "ok", Script: "await page.goto('/')", BaseURL: "https://app.example.com",
		Env: map[string]string{"var_username": "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed || res.Status != "passed" || res.Browser != "chromium" || len(res.Evidence) != 0 {
		t.Errorf("res %+v", res)
	}
	for _, want := range []string{"base=https://app.example.com browser=chromium", "user=alice", "config has baseURL", "1 passed", "[qtestframe] browser: chromium, base URL: https://app.example.com", "result: passed (1 passed, 0 failed"} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("output lacks %q:\n%s", want, res.Output)
		}
	}
	// the job directory is removed
	if entries, _ := os.ReadDir(r.Cfg.WorkDir); len(entries) != 0 {
		t.Errorf("job dir left: %v", entries)
	}
}

func TestRunWebFailWithEvidence(t *testing.T) {
	r := webRunner(t, func(c *config.Runner) { c.Web.MaxEvidenceMB = 1 })
	res, err := r.RunWeb(context.Background(), WebJob{Title: "fails", Script: "// FAIL", Browser: "chromium", Channel: "chrome",
		Env: map[string]string{"var_token": "s3cr3t-token"}, Secrets: []string{"s3cr3t-token"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed || res.Status != "failed" || res.ExitCode != 1 || res.Message != "login: Expected heading, got ***" {
		t.Errorf("res %+v", res)
	}
	if strings.Contains(res.Output, "s3cr3t-token") || !strings.Contains(res.Output, "token=***") || !strings.Contains(res.Output, "(channel chrome)") {
		t.Errorf("secret not masked or channel missing:\n%s", res.Output)
	}
	kinds := map[string]string{}
	for _, e := range res.Evidence {
		kinds[e.Kind] = e.Name + ":" + string(e.Data) + ":" + e.ContentType
	}
	if kinds["screenshot"] != "screenshot.png:PNG-DATA:image/png" || kinds["trace"] != "trace.zip:ZIP-DATA:application/zip" || kinds["video"] != "video.webm:VIDEO:video/webm" || len(res.Evidence) != 3 {
		t.Errorf("evidence %v", kinds)
	}
	if len(res.Omitted) != 1 || !strings.Contains(res.Omitted[0], "outside the job directory") || !strings.Contains(res.Output, "evidence left out: passwd") {
		t.Errorf("omitted %v", res.Omitted)
	}
}

func TestRunWebOutcomes(t *testing.T) {
	r := webRunner(t, nil)
	res, err := r.RunWeb(context.Background(), WebJob{Script: "// NOREPORT"})
	if err != nil || res.Status != "error" || res.ExitCode != 2 || !strings.Contains(res.Message, "no usable report") || !strings.Contains(res.Output, "crashed") {
		t.Errorf("no report: %+v %v", res, err)
	}
	res, err = r.RunWeb(context.Background(), WebJob{Script: "// SKIP"})
	if err != nil || res.Status != "skipped" || res.Passed {
		t.Errorf("skip: %+v %v", res, err)
	}
	r.Cfg.Web.JobTimeout = 300 * time.Millisecond
	res, err = r.RunWeb(context.Background(), WebJob{Script: "// SLEEP"})
	if err != nil || res.Status != "timedOut" || !res.TimedOut || res.ExitCode != -1 {
		t.Errorf("timeout: %+v %v", res, err)
	}
	r.Cfg.Web.JobTimeout = 20 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	res, err = r.RunWeb(ctx, WebJob{Script: "// SLEEP"})
	if err != nil || res.Status != "interrupted" || res.Passed {
		t.Errorf("cancel: %+v %v", res, err)
	}
}

func TestRunWebErrors(t *testing.T) {
	r := New(config.Runner{Enabled: true})
	if _, err := r.RunWeb(context.Background(), WebJob{}); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled: %v", err)
	}
	r = webRunner(t, func(c *config.Runner) { c.Web.Command = "/nonexistent/playwright" })
	if _, err := r.RunWeb(context.Background(), WebJob{Script: "x"}); err == nil || !strings.Contains(err.Error(), "start") {
		t.Errorf("missing command: %v", err)
	}
	r = webRunner(t, func(c *config.Runner) { c.WorkDir = "/proc/qtf-no-such-dir" })
	if _, err := r.RunWeb(context.Background(), WebJob{Script: "x"}); err == nil {
		t.Error("bad work dir")
	}
	// a long job directory gets a short TMPDIR elsewhere
	long := filepath.Join(t.TempDir(), strings.Repeat("d", 70))
	r = webRunner(t, func(c *config.Runner) { c.WorkDir = long })
	if res, err := r.RunWeb(context.Background(), WebJob{Script: "x"}); err != nil || !res.Passed {
		t.Errorf("long work dir: %+v %v", res, err)
	}
}

func TestWebEnv(t *testing.T) {
	r := New(config.Runner{Web: config.WebRunner{Node: "/opt/node/bin/node", PlaywrightDir: "/opt/pw", BrowsersPath: "/opt/b"}})
	env := strings.Join(r.webEnv("/job", "/job/tmp", WebJob{BaseURL: "http://x", Env: map[string]string{"var_a": "1"}}, "webkit", "chrome"), "\n")
	for _, want := range []string{"TMPDIR=/job/tmp", "HOME=/job", "PATH=/opt/node/bin:/usr/local/bin", "NODE_PATH=/opt/pw/node_modules",
		"PLAYWRIGHT_BROWSERS_PATH=/opt/b", "QTF_BASE_URL=http://x", "QTF_BROWSER=webkit", "QTF_BROWSER_CHANNEL=chrome", "QTF_VAR_A=1", "FORCE_COLOR=0"} {
		if !strings.Contains(env, want) {
			t.Errorf("env lacks %s:\n%s", want, env)
		}
	}
}

func TestSandboxWebOptions(t *testing.T) {
	c, err := ParseSandboxArgs([]string{"--workdir", "/w", "--web", "--ro", "/opt/pw", "--ro", "/opt/b", "--rw", "/tmp/x", "--", "node", "cli.js"}, os.Stderr)
	if err != nil || !c.Web || len(c.ReadOnly) != 2 || c.ReadWrite[0] != "/tmp/x" {
		t.Fatalf("%+v %v", c, err)
	}
	o := SandboxOptions(c)
	if o.Seccomp || o.DenyNetwork || !o.Landlock || !o.NoNewPrivs || !o.DropCapabilities {
		t.Errorf("web options %+v", o)
	}
	if !strings.Contains(strings.Join(o.ReadOnlyPaths, ","), "/opt/pw") || !strings.Contains(strings.Join(o.ReadWritePaths, ","), "/w,/dev/shm,/tmp/x") {
		t.Errorf("paths %+v", o)
	}
	if _, err := ParseSandboxArgs([]string{"--workdir", "/w", "--web", "--no-network", "--", "x"}, os.Stderr); err == nil {
		t.Error("--web with --no-network")
	}
	var l listFlag
	_ = l.Set("a")
	_ = l.Set("b")
	if l.String() != "a,b" {
		t.Error(l.String())
	}
}

// TestRunWebSandboxed runs the fake Playwright through sandbox-exec --web
// (the test binary is the re-exec target, see TestMain).
func TestRunWebSandboxed(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	r := webRunner(t, func(c *config.Runner) { c.Sandbox = true })
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r.Self = self
	res, err := r.RunWeb(context.Background(), WebJob{Script: "// FAIL", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "failed" || len(res.Evidence) != 3 {
		t.Errorf("sandboxed: %+v\n%s", res, res.Output)
	}
}

// TestRunWebPlaywright runs a real browser when QTF_TEST_PLAYWRIGHT_DIR
// names a directory with node_modules/@playwright/test (and browsers in
// PLAYWRIGHT_BROWSERS_PATH; QTF_TEST_CHROMIUM may name the executable).
func TestRunWebPlaywright(t *testing.T) {
	dir := os.Getenv("QTF_TEST_PLAYWRIGHT_DIR")
	if dir == "" {
		t.Skip("set QTF_TEST_PLAYWRIGHT_DIR to a Playwright installation")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	r := webRunner(t, func(c *config.Runner) {
		c.Web.Command, c.Web.Node, c.Web.PlaywrightDir = "", node, dir
		c.Web.BrowsersPath = os.Getenv("PLAYWRIGHT_BROWSERS_PATH")
		c.Web.ExecutablePath = os.Getenv("QTF_TEST_CHROMIUM")
		c.Web.Timeout, c.Web.JobTimeout = 20*time.Second, 2*time.Minute
		c.WorkDir = filepath.Join(os.TempDir(), "qtf-pw-test")
	})
	ok, err := r.RunWeb(context.Background(), WebJob{Title: "heading", Script: "await page.setContent('<h1>Sign in</h1>');\nawait expect(page.getByRole('heading')).toHaveText('Sign in');"})
	if err != nil || !ok.Passed {
		t.Fatalf("pass: %+v %v\n%s", ok, err, ok.Output)
	}
	bad, err := r.RunWeb(context.Background(), WebJob{Title: "heading", Script: "await page.setContent('<h1>Sign in</h1>');\nawait expect(page.getByRole('heading')).toHaveText('Sign out', { timeout: 500 });"})
	if err != nil || bad.Passed || bad.Status != "failed" || !strings.Contains(bad.Message, "Sign out") {
		t.Fatalf("fail: %+v %v", bad, err)
	}
	kinds := map[string]bool{}
	for _, e := range bad.Evidence {
		kinds[e.Kind] = len(e.Data) > 0
	}
	if !kinds["screenshot"] || !kinds["trace"] {
		t.Errorf("evidence %v (omitted %v)", kinds, bad.Omitted)
	}
	// the same through sandbox-exec --web (Landlock, rlimits, no seccomp)
	if runtime.GOOS == "linux" {
		r.Cfg.Sandbox = true
		if r.Self, err = os.Executable(); err != nil {
			t.Fatal(err)
		}
		sb, err := r.RunWeb(context.Background(), WebJob{Title: "heading", Script: "await page.setContent('<h1>Sign in</h1>');\nawait expect(page.getByRole('heading')).toHaveText('Sign in');"})
		if err != nil || !sb.Passed {
			t.Fatalf("sandboxed: %+v %v\n%s", sb, err, sb.Output)
		}
	}
}
