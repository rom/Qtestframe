package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Web tests (script language "playwright") run a Playwright spec in a real
// browser. The runner writes the spec and a generated playwright.config.js
// into a fresh job directory, runs `playwright test` with the configured
// Node/Playwright installation and derives the result from Playwright's
// JSON report. Screenshots, traces and videos named in the report are
// returned as evidence.

// LangPlaywright is the script language of web tests.
const LangPlaywright = "playwright"

// WebJob is one web test.
type WebJob struct {
	// Title names the test in the generated spec (the test case title).
	Title  string
	Script string
	// BaseURL is Playwright's use.baseURL (page.goto('/login') is relative
	// to it); empty = none.
	BaseURL string
	// Browser is chromium, firefox or webkit; empty = the configured default.
	Browser string
	// Channel is a Chromium channel (chrome, msedge); empty = bundled.
	Channel string
	// Env entries are passed as QTF_<KEY>=value (see Run).
	Env map[string]string
	// Secrets are masked ("***") in the output and failure message.
	Secrets []string
}

// Evidence is a file produced by a web test.
type Evidence struct {
	// Kind is screenshot, trace or video.
	Kind        string
	Name        string
	ContentType string
	Data        []byte
}

// WebResult is the outcome of a web test.
type WebResult struct {
	Result
	// Status is the overall Playwright result: passed, failed, timedOut,
	// interrupted, skipped, or error (no usable report).
	Status string `json:"status"`
	// Message is the first failure message (without colour codes).
	Message  string     `json:"message"`
	Browser  string     `json:"browser"`
	Channel  string     `json:"channel,omitempty"`
	BaseURL  string     `json:"base_url"`
	Evidence []Evidence `json:"-"`
	// Omitted lists evidence files that were left out (too large, unreadable).
	Omitted []string `json:"omitted,omitempty"`
}

// WebEnabled reports whether web tests may run.
func (r *Runner) WebEnabled() bool { return r.Enabled() && r.Cfg.Web.Enabled }

var completeSpecRe = regexp.MustCompile(`(^|[^.\w$])test(\.describe|\.only)?\s*\(`)

// IsCompleteSpec reports whether script is a complete spec file (it calls
// test(...) itself) rather than the body of one test.
func IsCompleteSpec(script string) bool { return completeSpecRe.MatchString(script) }

// WrapScript returns the spec file for script: a complete spec as is, else
// the script as the body of one test titled title.
func WrapScript(title, script string) string {
	if IsCompleteSpec(script) {
		return script
	}
	if strings.TrimSpace(title) == "" {
		title = "web test"
	}
	t, _ := json.Marshal(title)
	var b strings.Builder
	b.WriteString("import { test, expect } from '@playwright/test';\n\n")
	b.WriteString("test(" + string(t) + ", async ({ page, context, request, baseURL }) => {\n")
	for _, line := range strings.Split(strings.TrimRight(script, "\n"), "\n") {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("});\n")
	return b.String()
}

// BrowserFor maps the value of a Browser configuration dimension to a
// Playwright browser and channel: Chrome, Chromium and Edge run in
// chromium (with channels, Chrome and Edge use the installed chrome or
// msedge), Firefox in firefox, Safari and WebKit in webkit. ok is false for
// an unknown value.
func BrowserFor(value string, channels bool) (browser, channel string, ok bool) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(v, "edge"):
		if channels {
			return "chromium", "msedge", true
		}
		return "chromium", "", true
	case strings.Contains(v, "chromium"):
		return "chromium", "", true
	case strings.Contains(v, "chrome"):
		if channels {
			return "chromium", "chrome", true
		}
		return "chromium", "", true
	case strings.Contains(v, "firefox"):
		return "firefox", "", true
	case strings.Contains(v, "safari"), strings.Contains(v, "webkit"):
		return "webkit", "", true
	}
	return "", "", false
}

// Mask replaces every secret (of at least 3 characters) in s by ***.
func Mask(s string, secrets []string) string {
	list := append([]string{}, secrets...)
	// longest first, so a secret containing another is masked whole
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
	for _, v := range list {
		if len(v) >= 3 {
			s = strings.ReplaceAll(s, v, "***")
		}
	}
	return s
}

func pwMode(v string) string {
	switch v {
	case "always":
		return "on"
	case "on_failure":
		return "retain-on-failure"
	}
	return "off"
}

// Generated file names inside the job directory.
const (
	specFile   = "test.spec.ts"
	configFile = "playwright.config.js"
	reportFile = "report.json"
	resultsDir = "results"
)

// WebConfig returns the playwright.config.js of a job in dir.
func (r *Runner) WebConfig(job WebJob, dir string) []byte {
	w := r.Cfg.Web
	browser, channel := job.Browser, job.Channel
	if browser == "" {
		browser, channel = w.Browser, w.Channel
	}
	screenshot := "only-on-failure"
	switch w.Screenshots {
	case "always":
		screenshot = "on"
	case "off":
		screenshot = "off"
	}
	use := map[string]any{
		"browserName": browser,
		"headless":    w.Headless,
		"screenshot":  screenshot,
		"trace":       pwMode(w.Trace),
		"video":       pwMode(w.Video),
	}
	if job.BaseURL != "" {
		use["baseURL"] = job.BaseURL
	}
	if channel != "" && browser == "chromium" {
		use["channel"] = channel
	}
	if w.ExecutablePath != "" && browser == "chromium" && channel == "" {
		use["launchOptions"] = map[string]any{"executablePath": w.ExecutablePath}
	}
	cfg := map[string]any{
		"testDir":       dir,
		"testMatch":     []string{specFile},
		"outputDir":     filepath.Join(dir, resultsDir),
		"timeout":       w.Timeout.Milliseconds(),
		"workers":       1,
		"retries":       0,
		"fullyParallel": false,
		"reporter":      []any{[]any{"list"}, []any{"json", map[string]string{"outputFile": filepath.Join(dir, reportFile)}}},
		"use":           use,
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return []byte("// generated by qtestframe for one web test\nmodule.exports = " + string(b) + ";\n")
}

// webArgv is the Playwright command without "test --config <file>".
func (r *Runner) webArgv() []string {
	w := r.Cfg.Web
	if w.Command != "" {
		return strings.Fields(w.Command)
	}
	return []string{w.Node, filepath.Join(w.PlaywrightDir, "node_modules", "@playwright", "test", "cli.js")}
}

// webTempDir returns a TMPDIR for the browser. Chromium keeps a unix
// socket in TMPDIR, whose path must stay short (108 bytes), so a long job
// directory gets a short temporary directory instead (removed by cleanup).
func webTempDir(dir string) (string, func(), error) {
	if t := filepath.Join(dir, "tmp"); len(t) <= 60 {
		return t, func() {}, os.Mkdir(t, 0o700)
	}
	t, err := os.MkdirTemp("", "qtf-")
	if err != nil {
		return "", nil, err
	}
	return t, func() { os.RemoveAll(t) }, nil
}

func (r *Runner) webEnv(dir, tmp string, job WebJob, browser, channel string) []string {
	w := r.Cfg.Web
	extra := map[string]string{}
	for k, v := range job.Env {
		extra[k] = v
	}
	extra["base_url"] = job.BaseURL
	extra["browser"] = browser
	env := buildEnv(dir, extra)
	for i, e := range env {
		switch {
		case strings.HasPrefix(e, "TMPDIR="):
			env[i] = "TMPDIR=" + tmp
		case strings.HasPrefix(e, "PATH=") && w.Node != "" && filepath.IsAbs(w.Node):
			env[i] = "PATH=" + filepath.Dir(w.Node) + ":" + strings.TrimPrefix(e, "PATH=")
		}
	}
	if w.PlaywrightDir != "" {
		env = append(env, "NODE_PATH="+filepath.Join(w.PlaywrightDir, "node_modules"))
	}
	if w.BrowsersPath != "" {
		env = append(env, "PLAYWRIGHT_BROWSERS_PATH="+w.BrowsersPath)
	}
	if channel != "" {
		env = append(env, "QTF_BROWSER_CHANNEL="+channel)
	}
	return append(env, "FORCE_COLOR=0", "PW_TEST_HTML_REPORT_OPEN=never", "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1")
}

// webPaths are the directories a sandboxed web test reads or executes
// from besides the system directories.
func (r *Runner) webPaths() []string {
	w := r.Cfg.Web
	var out []string
	for _, p := range []string{w.PlaywrightDir, w.BrowsersPath, w.ExecutablePath} {
		if p != "" {
			if abs, err := filepath.Abs(p); err == nil {
				out = append(out, abs)
			}
		}
	}
	if w.ExecutablePath != "" {
		if real, err := filepath.EvalSymlinks(w.ExecutablePath); err == nil {
			out = append(out, filepath.Dir(real))
		}
	}
	if w.Node != "" && filepath.IsAbs(w.Node) {
		out = append(out, filepath.Dir(w.Node))
	}
	for _, a := range r.webArgv() {
		if filepath.IsAbs(a) {
			out = append(out, filepath.Dir(a))
		}
	}
	return out
}

// RunWeb runs a web test.
func (r *Runner) RunWeb(ctx context.Context, job WebJob) (*WebResult, error) {
	if !r.WebEnabled() {
		return nil, ErrDisabled
	}
	w := r.Cfg.Web
	workDir, err := filepath.Abs(r.Cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("runner: work dir: %w", err)
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("runner: work dir: %w", err)
	}
	dir, err := os.MkdirTemp(workDir, "web-")
	if err != nil {
		return nil, fmt.Errorf("runner: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	tmp, cleanup, err := webTempDir(dir)
	if err != nil {
		return nil, fmt.Errorf("runner: temp dir: %w", err)
	}
	defer cleanup()
	browser, channel := job.Browser, job.Channel
	if browser == "" {
		browser, channel = w.Browser, w.Channel
	}
	job.Browser, job.Channel = browser, channel
	if err := os.WriteFile(filepath.Join(dir, specFile), []byte(WrapScript(job.Title, job.Script)), 0o600); err != nil {
		return nil, fmt.Errorf("runner: write spec: %w", err)
	}
	cfgPath := filepath.Join(dir, configFile)
	if err := os.WriteFile(cfgPath, r.WebConfig(job, dir), 0o600); err != nil {
		return nil, fmt.Errorf("runner: write config: %w", err)
	}
	timeout := w.JobTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	argv := append(r.webArgv(), "test", "--config", cfgPath)
	var cmd *exec.Cmd
	if r.Cfg.Sandbox {
		args := []string{"sandbox-exec", "--workdir", dir, "--cpu", strconv.Itoa(int(timeout.Seconds()) + 1), "--web", "--rw", tmp}
		for _, p := range r.webPaths() {
			args = append(args, "--ro", p)
		}
		args = append(append(args, "--"), argv...)
		cmd = exec.CommandContext(cctx, r.Self, args...)
	} else {
		cmd = exec.CommandContext(cctx, argv[0], argv[1:]...)
	}
	cmd.Dir = dir
	cmd.Env = r.webEnv(dir, tmp, job, browser, channel)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 5 * time.Second
	max := r.Cfg.MaxOutputBytes
	if max <= 0 {
		max = 1 << 20
	}
	out := &limitedBuffer{max: max}
	cmd.Stdout, cmd.Stderr = out, out
	start := time.Now()
	err = cmd.Run()
	res := &WebResult{Result: Result{Duration: time.Since(start)}, Browser: browser, Channel: channel, BaseURL: job.BaseURL}
	var ee *exec.ExitError
	switch {
	case cctx.Err() == context.DeadlineExceeded:
		res.TimedOut, res.ExitCode = true, -1
	case ctx.Err() != nil:
		res.ExitCode = -1
	case err == nil:
	case errors.As(err, &ee):
		res.ExitCode = ee.ExitCode()
	default:
		return nil, fmt.Errorf("runner: start %s: %w", argv[0], err)
	}
	report, rerr := os.ReadFile(filepath.Join(dir, reportFile))
	var sum *ReportSummary
	if rerr == nil {
		sum, rerr = ParseReport(report)
	}
	switch {
	case res.TimedOut:
		res.Status, res.Message = "timedOut", fmt.Sprintf("the web test did not finish within %s", timeout)
	case ctx.Err() != nil:
		res.Status, res.Message = "interrupted", "the web test was stopped"
	case rerr != nil:
		res.Status = "error"
		res.Message = fmt.Sprintf("Playwright wrote no usable report (exit status %d); see the output", res.ExitCode)
	default:
		res.Status, res.Message = sum.Status, sum.Message
	}
	res.Passed = res.Status == "passed"
	if sum != nil {
		limit := int64(w.MaxEvidenceMB) << 20
		if limit <= 0 {
			limit = 25 << 20
		}
		res.Evidence, res.Omitted = collectEvidence(dir, sum.Attachments, limit)
	}
	var b strings.Builder
	b.WriteString(out.String())
	fmt.Fprintf(&b, "\n[qtestframe] browser: %s", browser)
	if channel != "" {
		fmt.Fprintf(&b, " (channel %s)", channel)
	}
	if job.BaseURL != "" {
		fmt.Fprintf(&b, ", base URL: %s", job.BaseURL)
	}
	fmt.Fprintf(&b, "\n[qtestframe] result: %s", res.Status)
	if sum != nil {
		fmt.Fprintf(&b, " (%d passed, %d failed, %d flaky, %d skipped)", sum.Passed, sum.Failed, sum.Flaky, sum.Skipped)
	}
	b.WriteString("\n")
	if res.Message != "" && !res.Passed {
		b.WriteString("[qtestframe] " + firstLines(res.Message, 20) + "\n")
	}
	for _, o := range res.Omitted {
		b.WriteString("[qtestframe] evidence left out: " + o + "\n")
	}
	res.Output = Mask(b.String(), job.Secrets)
	res.Message = Mask(res.Message, job.Secrets)
	res.Truncated = out.truncated
	return res, nil
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- report

// ReportAttachment is a file named in Playwright's JSON report.
type ReportAttachment struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Path        string `json:"path"`
}

// ReportSummary is what qtestframe takes from a JSON report.
type ReportSummary struct {
	// Status: passed, failed, timedOut, interrupted or skipped.
	Status                         string
	Message                        string
	Passed, Failed, Flaky, Skipped int
	Attachments                    []ReportAttachment
}

type pwError struct {
	Message string `json:"message"`
}

type pwResult struct {
	Status      string             `json:"status"`
	Error       *pwError           `json:"error"`
	Errors      []pwError          `json:"errors"`
	Attachments []ReportAttachment `json:"attachments"`
}

type pwSuite struct {
	Title string `json:"title"`
	Specs []struct {
		Title string `json:"title"`
		Tests []struct {
			Status  string     `json:"status"` // expected, unexpected, flaky, skipped
			Results []pwResult `json:"results"`
		} `json:"tests"`
	} `json:"specs"`
	Suites []pwSuite `json:"suites"`
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

// StripANSI removes terminal colour codes.
func StripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// ParseReport reads Playwright's JSON report: the result is passed when
// at least one test passed and none failed, and there are no errors
// outside tests (e.g. a syntax error in the spec).
func ParseReport(data []byte) (*ReportSummary, error) {
	var rep struct {
		Suites []pwSuite `json:"suites"`
		Errors []pwError `json:"errors"`
		Config *struct{} `json:"config"`
		Stats  *struct{} `json:"stats"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("runner: playwright report: %w", err)
	}
	if rep.Config == nil && rep.Stats == nil && rep.Suites == nil {
		return nil, errors.New("runner: playwright report: not a JSON report")
	}
	sum := &ReportSummary{}
	var msgs []string
	worst := ""
	var walk func(prefix string, s pwSuite)
	walk = func(prefix string, s pwSuite) {
		title := s.Title
		if prefix != "" && title != "" {
			title = prefix + " › " + title
		} else if prefix != "" {
			title = prefix
		}
		for _, sp := range s.Specs {
			for _, t := range sp.Tests {
				switch t.Status {
				case "expected":
					sum.Passed++
				case "flaky":
					sum.Flaky++
				case "skipped":
					sum.Skipped++
				default:
					sum.Failed++
				}
				for _, r := range t.Results {
					sum.Attachments = append(sum.Attachments, r.Attachments...)
					if r.Status == "passed" || r.Status == "skipped" {
						continue
					}
					if worst == "" || worst == "failed" && r.Status != "failed" {
						worst = r.Status
					}
					m := ""
					if r.Error != nil {
						m = r.Error.Message
					} else if len(r.Errors) > 0 {
						m = r.Errors[0].Message
					}
					if m != "" && t.Status != "flaky" {
						msgs = append(msgs, sp.Title+": "+StripANSI(m))
					}
				}
			}
		}
		for _, c := range s.Suites {
			walk(title, c)
		}
	}
	for _, s := range rep.Suites {
		walk("", s)
	}
	for _, e := range rep.Errors {
		msgs = append(msgs, StripANSI(e.Message))
	}
	switch {
	case len(rep.Errors) > 0 || sum.Failed > 0:
		sum.Status = "failed"
		if worst == "timedOut" || worst == "interrupted" {
			sum.Status = worst
		}
	case sum.Passed+sum.Flaky > 0:
		sum.Status = "passed"
	case sum.Skipped > 0:
		sum.Status = "skipped"
		msgs = append(msgs, "every test was skipped")
	default:
		sum.Status = "failed"
		msgs = append(msgs, "the spec contains no test")
	}
	if len(msgs) > 0 {
		sum.Message = strings.TrimSpace(strings.Join(msgs, "\n\n"))
	}
	return sum, nil
}

var evidenceKinds = map[string]string{"screenshot": "screenshot", "trace": "trace", "video": "video"}

// collectEvidence reads the screenshots, traces and videos of a report.
// Only regular files inside dir are read; files larger than limit are
// left out.
func collectEvidence(dir string, atts []ReportAttachment, limit int64) ([]Evidence, []string) {
	var out []Evidence
	var omitted []string
	n := map[string]int{}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, nil
	}
	for _, a := range atts {
		kind := evidenceKinds[a.Name]
		if kind == "" || a.Path == "" {
			continue
		}
		p, err := filepath.EvalSymlinks(a.Path)
		if err != nil {
			omitted = append(omitted, filepath.Base(a.Path)+" (missing)")
			continue
		}
		if rel, err := filepath.Rel(root, p); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			omitted = append(omitted, filepath.Base(a.Path)+" (outside the job directory)")
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			omitted = append(omitted, filepath.Base(a.Path)+" (unreadable)")
			continue
		}
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() {
			f.Close()
			omitted = append(omitted, filepath.Base(a.Path)+" (not a file)")
			continue
		}
		if st.Size() > limit {
			f.Close()
			omitted = append(omitted, fmt.Sprintf("%s (%d bytes, limit %d)", filepath.Base(a.Path), st.Size(), limit))
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, limit+1))
		f.Close()
		if err != nil {
			continue
		}
		n[kind]++
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		out = append(out, Evidence{Kind: kind, Name: filepath.Ext(p), ContentType: ct, Data: data})
	}
	// screenshot.png, or screenshot-1.png, screenshot-2.png, … for several
	seen := map[string]int{}
	for i, e := range out {
		seen[e.Kind]++
		if n[e.Kind] > 1 {
			out[i].Name = fmt.Sprintf("%s-%d%s", e.Kind, seen[e.Kind], e.Name)
		} else {
			out[i].Name = e.Kind + e.Name
		}
	}
	return out, omitted
}
