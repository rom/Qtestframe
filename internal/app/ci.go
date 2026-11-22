package app

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/ci"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracker"
	"github.com/rom/qtestframe/internal/uploader"
)

// CI/CD commands: qtestframe gate (the quality gate of a pipeline) and
// qtestframe select (the tests to run for a change), and the commit
// statuses and merge request notes that report and gate post from the
// pipeline (see docs/user-guide/en/ci-cd.md).

// exitError makes Run exit with code (the message, if any, is printed).
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error { return e.err }

// gateError: the gate could not be evaluated (exit code 2).
func gateError(err error) error { return &exitError{code: 2, err: err} }

// getenv is os.Getenv (replaced in tests).
var getenv = os.Getenv

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// serverFlags are the connection flags of report, gate and select.
type serverFlags struct {
	url, token, project string
}

func (f *serverFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.url, "url", getenv("QTF_URL"), "server base URL, e.g. https://qtestframe.example.com (default $QTF_URL)")
	fs.StringVar(&f.token, "token", "", "API token (default $QTF_TOKEN)")
	fs.StringVar(&f.project, "project", getenv("QTF_PROJECT"), "project key, e.g. SHOP (default $QTF_PROJECT)")
}

func (f *serverFlags) client() (*uploader.Client, error) {
	if f.token == "" {
		f.token = getenv("QTF_TOKEN")
	}
	return uploader.New(f.url, f.token, nil)
}

// postFlags say what report and gate post to the code host.
type postFlags struct {
	status, comment bool
	host            string
}

func (f *postFlags) register(fs *flag.FlagSet) {
	f.status, f.comment = envBool("QTF_POST_STATUS"), envBool("QTF_POST_COMMENT")
	fs.BoolVar(&f.status, "post-status", f.status, "post a commit status to the code host (default $QTF_POST_STATUS)")
	fs.BoolVar(&f.comment, "post-comment", f.comment, "add or update a merge / pull request note with the summary (default $QTF_POST_COMMENT)")
	fs.BoolVar(&f.status, "gitlab-status", f.status, "same as --post-status")
	fs.BoolVar(&f.comment, "gitlab-comment", f.comment, "same as --post-comment")
	fs.StringVar(&f.host, "code-host", "auto", "where to post: auto (from the CI variables), gitlab or github")
}

// codeHost returns the code host to post to and how it authenticates
// (nil and a reason when it cannot).
func codeHost(e ci.Env, kind string, hc *http.Client) (tracker.CodeHost, string, string) {
	if kind == "auto" || kind == "" {
		kind = e.System
	}
	switch kind {
	case ci.SystemGitLab:
		if e.APIURL == "" || e.Project == "" {
			return nil, "", "CI_API_V4_URL and CI_PROJECT_ID are not set"
		}
		if tok := firstEnv("QTF_GITLAB_TOKEN", "GITLAB_TOKEN"); tok != "" {
			return tracker.NewGitLabHost(e.APIURL, e.Project, tok, tracker.GitLabPrivateToken, hc), "a GitLab access token", ""
		}
		if e.JobToken != "" {
			return tracker.NewGitLabHost(e.APIURL, e.Project, e.JobToken, tracker.GitLabJobToken, hc), "CI_JOB_TOKEN", ""
		}
		return nil, "", "no GitLab token: set QTF_GITLAB_TOKEN (a project access token with the api scope)"
	case ci.SystemGitHub:
		if e.Project == "" {
			return nil, "", "GITHUB_REPOSITORY is not set"
		}
		tok := firstEnv("QTF_GITHUB_TOKEN", "GITHUB_TOKEN")
		if tok == "" {
			return nil, "", "no GitHub token: set QTF_GITHUB_TOKEN (e.g. to ${{ secrets.GITHUB_TOKEN }})"
		}
		return tracker.NewGitHubHost(e.APIURL, e.Project, tok, hc), "a GitHub token", ""
	}
	return nil, "", "not running in GitLab CI or GitHub Actions (pass --code-host)"
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

// postHTTP is the HTTP client of posts to the code host (tests replace it).
var postHTTP = &http.Client{Timeout: 30 * time.Second}

// post sends the commit status (name) and the note of a report. Failures
// are warnings: they do not change the exit code.
func (f *postFlags) post(ctx context.Context, log io.Writer, e ci.Env, rep *ci.Report, name string) {
	if !f.status && !f.comment {
		return
	}
	host, how, why := codeHost(e, f.host, postHTTP)
	if host == nil {
		fmt.Fprintf(log, "warning: cannot post to the code host: %s\n", why)
		return
	}
	hint := func(err error) string {
		var te *tracker.Error
		if how == "CI_JOB_TOKEN" && errors.As(err, &te) && (te.Status == 401 || te.Status == 403 || te.Status == 404) {
			return " (GitLab does not accept the job token for this endpoint: set QTF_GITLAB_TOKEN to a project access token with the api scope)"
		}
		return ""
	}
	if f.status {
		if e.Commit == "" {
			fmt.Fprintln(log, "warning: no commit status: the commit is unknown (CI_COMMIT_SHA or QTF_COMMIT)")
		} else {
			state := tracker.StatusSuccess
			if !rep.Pass {
				state = tracker.StatusFailed
			}
			ref := ""
			if e.MergeRequest == "" {
				ref = e.Branch
			}
			err := host.SetStatus(ctx, tracker.CommitStatus{SHA: e.Commit, State: state, Name: name, TargetURL: rep.URL,
				Description: rep.StatusDescription(), Ref: ref, PipelineID: e.PipelineID})
			if err != nil {
				fmt.Fprintf(log, "warning: commit status %s: %v%s\n", name, err, hint(err))
			} else {
				fmt.Fprintf(log, "%s: commit status %s %s\n", host.Name(), name, state)
			}
		}
	}
	if f.comment {
		if e.MergeRequest == "" {
			fmt.Fprintln(log, "note: no merge request note: not a merge request pipeline")
			return
		}
		updated, err := host.UpsertNote(ctx, e.MergeRequest, ci.NoteMarker(rep.Project.Key), rep.Markdown())
		switch {
		case err != nil:
			fmt.Fprintf(log, "warning: merge request note: %v%s\n", err, hint(err))
		case updated:
			fmt.Fprintf(log, "%s: note of !%s updated\n", host.Name(), e.MergeRequest)
		default:
			fmt.Fprintf(log, "%s: note added to !%s\n", host.Name(), e.MergeRequest)
		}
	}
}

// parseArgs parses flags that may be interleaved with positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return rest, nil
		}
		rest = append(rest, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// writeOutput writes a rendering to a file ("" or "-": stdout).
func writeOutput(env Env, path string, render func(io.Writer) error) error {
	if path == "" || path == "-" {
		return render(env.Stdout)
	}
	var buf bytes.Buffer
	if err := render(&buf); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// ---------------------------------------------------------------- gate

// cmdGate checks the quality gate: exit code 0 when it passes, 1 when it
// fails, 2 when it cannot be evaluated.
func cmdGate(args []string, env Env) error {
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var sf serverFlags
	sf.register(fs)
	var pf postFlags
	pf.register(fs)
	var in uploader.GateRequest
	var require stringList
	fs.StringVar(&in.Release, "release", "", "release (name, version or id) whose exit criteria are checked")
	fs.StringVar(&in.Variant, "variant", "", "evaluate for this variant (name or id) only")
	fs.StringVar(&in.Run, "run", "", "run (name or id) whose results are checked")
	runID := fs.String("run-id", "", "id of the run whose results are checked")
	fs.StringVar(&in.Build, "build", "", "build (CI id or number) whose runs' results are checked")
	fs.Var(&require, "require", "condition, e.g. coverage>=80, must, no_failed, no_stale, no_blocking_defects, no_open_defects:major, cycle_executed:NAME>=90 (repeatable, or comma-separated); replaces the release's exit criteria")
	fs.StringVar(&in.Scope, "scope", "", "requirements of a gate without --release: all (default), affected (those the merge request / commit refers to) or a query")
	fs.StringVar(&in.Commit, "commit", "", "commit of the change (default from the CI variables)")
	fs.StringVar(&in.MergeRequest, "merge-request", "", "merge / pull request of the change (default from the CI variables)")
	format := fs.String("format", "text", "output: text, json, junit or markdown")
	output := fs.String("output", "", "write the output to this file (the text summary still goes to the log)")
	if _, err := parseArgs(fs, args); err != nil {
		return gateError(err)
	}
	if *runID != "" {
		in.Run = *runID
	}
	in.Require = require
	switch *format {
	case "text", "json", "junit", "markdown", "md":
	default:
		return gateError(fmt.Errorf("unknown --format %q (text, json, junit or markdown)", *format))
	}
	e := ci.Detect(getenv)
	if in.Commit == "" {
		in.Commit = e.Commit
	} else {
		e.Commit = in.Commit
	}
	if in.MergeRequest == "" {
		in.MergeRequest = e.MergeRequest
	} else {
		e.MergeRequest = in.MergeRequest
	}
	rep, err := evaluateGate(env.Ctx, sf, in)
	if err != nil {
		return gateError(err)
	}
	if *output != "" || *format == "text" {
		rep.WriteText(env.Stdout)
	}
	if *format != "text" || *output != "" {
		if err := writeOutput(env, *output, func(w io.Writer) error { return rep.Write(w, *format) }); err != nil {
			return gateError(err)
		}
	}
	pf.post(env.Ctx, env.Stderr, e, rep, "qtestframe/gate")
	if !rep.Pass {
		return &exitError{code: 1, err: fmt.Errorf("quality gate failed: %d of %d conditions not met", len(rep.Failing()), len(rep.Conditions))}
	}
	return nil
}

func evaluateGate(ctx context.Context, sf serverFlags, in uploader.GateRequest) (*ci.Report, error) {
	c, err := sf.client()
	if err != nil {
		return nil, err
	}
	if sf.project == "" {
		return nil, errors.New("--project KEY is required (or $QTF_PROJECT)")
	}
	p, err := c.ProjectByKey(ctx, sf.project)
	if err != nil {
		return nil, err
	}
	return c.Gate(ctx, p.ID, in)
}

// ---------------------------------------------------------------- select

// git runs git and returns its output (tests replace it).
var git = func(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// mapFlag is a repeatable NAME=VALUE flag.
type mapFlag map[string]string

func (m mapFlag) String() string { return "" }

func (m mapFlag) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(k) == "" {
		return fmt.Errorf("expected RUNNER=VALUE, got %q", v)
	}
	m[strings.ToLower(strings.TrimSpace(k))] = val
	return nil
}

func zeroSHA(s string) bool { return strings.Trim(s, "0") == "" }

// cmdSelect prints the tests to run for a change.
func cmdSelect(args []string, env Env) error {
	fs := flag.NewFlagSet("select", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var sf serverFlags
	sf.register(fs)
	var in uploader.SelectRequest
	var commits, files, tags, reqs, gateRequire stringList
	jobs, images := mapFlag{}, mapFlag{}
	since := fs.String("since", "", "compare with this git revision (default: the merge request's diff base, else the commit before the push)")
	head := fs.String("head", "", "the revision of the change (default $CI_COMMIT_SHA, else HEAD)")
	useGit := fs.Bool("git", true, "read the commits (messages) and changed files with git")
	fs.Var(&commits, "commits", "commit SHAs of the change (repeatable or comma-separated; instead of git)")
	fs.Var(&files, "files", "changed files (repeatable or comma-separated; instead of git)")
	mapFile := fs.String("map", "", "path map: changed files → tests, tags, requirements (default qtestframe-map.yml when it exists)")
	fs.StringVar(&in.MergeRequest, "merge-request", "", "merge / pull request number (default from the CI variables)")
	fs.Var(&reqs, "requirement", "changed requirement (key or id; repeatable)")
	fs.Var(&tags, "tag", "always run the tests with this tag (repeatable)")
	fs.StringVar(&in.SinceBuild, "since-build", "", "also the commits after this build (CI id or number)")
	fs.StringVar(&in.Variant, "variant", "", "only tests that apply to this variant")
	fs.BoolVar(&in.IncludeRelated, "include-related", false, "add requirements that depend on, refine or derive from a changed one")
	fs.BoolVar(&in.IncludeManual, "include-manual", false, "select manual tests too")
	fallback := fs.String("fallback", "none", "when the change is unknown (no commits, files or keys): none (select nothing) or all")
	all := fs.Bool("all", false, "select every test")
	format := fs.String("format", "keys", "output: "+strings.Join(ci.Formats, ", "))
	output := fs.String("output", "", "write the output to this file")
	fs.BoolVar(&in.CreateRun, "create-run", false, "create a run of the selected tests (results of report --run-id land in it)")
	fs.StringVar(&in.Run, "run", getenv("QTF_RUN"), "name of the run to create (default $QTF_RUN)")
	fs.StringVar(&in.Environment, "environment", "", "environment of the created run")
	release := fs.String("release", "", "release (name, version or id) of the created run")
	fs.Var(jobs, "job", "test command of a runner's job in the child pipeline, RUNNER=COMMAND (repeatable)")
	fs.Var(images, "image", "image of a runner's job in the child pipeline, RUNNER=IMAGE (repeatable)")
	cliImage := fs.String("cli-image", "", "image with the qtestframe binary for the child pipeline's report job (default $QTF_CLI_IMAGE)")
	gateRelease := fs.String("gate-release", "", "release whose exit criteria the child pipeline's gate checks")
	fs.Var(&gateRequire, "gate-require", "condition of the child pipeline's gate (repeatable)")
	noGate := fs.Bool("no-gate", false, "no gate in the child pipeline")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	if !contains(ci.Formats, *format) && *format != "junit" {
		return fmt.Errorf("unknown --format %q (use one of %s)", *format, strings.Join(ci.Formats, ", "))
	}
	if sf.project == "" {
		return errors.New("--project KEY is required (or $QTF_PROJECT)")
	}
	log := env.Stderr
	e := ci.Detect(getenv)
	if in.MergeRequest == "" {
		in.MergeRequest = e.MergeRequest
	}
	base := *since
	if base == "" {
		base = e.DiffBase
	}
	if base == "" {
		if b := getenv("CI_COMMIT_BEFORE_SHA"); b != "" && !zeroSHA(b) && e.MergeRequest == "" {
			base = b
		}
	}
	top := *head
	if top == "" {
		top = firstNonEmpty(e.Commit, "HEAD")
	}
	in.Commits = splitList(commits)
	changed := splitList(files)
	var messages []string
	if *useGit && base != "" && len(in.Commits) == 0 {
		out, err := git(env.Ctx, "log", "--format=%H%x1f%B%x1e", base+".."+top)
		if err != nil {
			return err
		}
		for _, rec := range strings.Split(out, "\x1e") {
			sha, msg, ok := strings.Cut(strings.TrimSpace(rec), "\x1f")
			if ok && sha != "" {
				in.Commits = append(in.Commits, sha)
				messages = append(messages, msg)
			}
		}
		if len(changed) == 0 {
			out, err := git(env.Ctx, "diff", "--name-only", base, top)
			if err != nil {
				return err
			}
			for _, l := range strings.Split(out, "\n") {
				if l = strings.TrimSpace(l); l != "" {
					changed = append(changed, l)
				}
			}
		}
	}
	for _, m := range messages {
		for _, k := range store.ExtractItemKeys(m, sf.project) {
			if !contains(in.Keys, k) {
				in.Keys = append(in.Keys, k)
			}
		}
	}
	mf := *mapFile
	if mf == "" {
		if _, err := os.Stat("qtestframe-map.yml"); err == nil {
			mf = "qtestframe-map.yml"
		}
	}
	tagList := splitList(tags)
	in.RequirementIDs = splitList(reqs)
	if mf != "" && len(changed) > 0 {
		data, err := os.ReadFile(mf)
		if err != nil {
			return err
		}
		pm, err := ci.ParsePathMap(data)
		if err != nil {
			return err
		}
		hit := pm.Match(changed)
		for _, k := range append(hit.Tests, hit.Requirements...) {
			if !contains(in.Keys, strings.ToUpper(k)) {
				in.Keys = append(in.Keys, strings.ToUpper(k))
			}
		}
		for _, t := range hit.Tags {
			if !contains(tagList, t) {
				tagList = append(tagList, t)
			}
		}
		for p, f := range hit.Why {
			fmt.Fprintf(log, "path map: %s matches %s\n", f, p)
		}
	}
	in.Tags = tagList
	known := len(in.Commits) > 0 || len(changed) > 0 || len(in.Keys) > 0 || in.MergeRequest != "" || len(in.RequirementIDs) > 0 || in.SinceBuild != ""
	in.All = *all || (!known && *fallback == "all")
	if !known && !in.All {
		fmt.Fprintln(log, "note: the change is unknown (no commits, changed files, keys or merge request); only tagged tests are selected (--fallback all selects every test)")
	}
	if *release != "" {
		in.ReleaseID = release
	}
	in.Build = e.BuildNumber
	if in.CreateRun && in.Run == "" {
		in.Run = "CI selection"
		if e.BuildNumber != "" {
			in.Run = "Pipeline " + e.BuildNumber
		}
	}
	c, err := sf.client()
	if err != nil {
		return err
	}
	p, err := c.ProjectByKey(env.Ctx, sf.project)
	if err != nil {
		return err
	}
	sel, err := c.Select(env.Ctx, p.ID, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(log, "selected %d automated test(s) of %s from %d commit(s), %d changed file(s), %d key(s)", len(sel.Tests), p.Key, len(in.Commits), len(changed), len(in.Keys))
	if len(sel.Manual) > 0 {
		fmt.Fprintf(log, "; %d manual test(s) not included", len(sel.Manual))
	}
	fmt.Fprintln(log)
	if len(sel.Unknown) > 0 {
		fmt.Fprintf(log, "unknown keys: %s\n", strings.Join(sel.Unknown, ", "))
	}
	if sel.Run != nil {
		fmt.Fprintf(log, "created run %q (%s) %s\n", sel.Run.Name, sel.Run.ID, sel.Run.URL)
	}
	return writeOutput(env, *output, func(w io.Writer) error {
		if *format != "gitlab-child-pipeline" {
			return sel.Write(w, *format)
		}
		o := ci.PipelineOptions{Commands: jobs, Images: images, CLIImage: *cliImage, NoGate: *noGate, RunName: in.Run}
		if sel.Run != nil {
			o.RunID = sel.Run.ID
		}
		if *gateRelease != "" {
			o.GateArgs = append(o.GateArgs, "--release", *gateRelease)
		}
		for _, r := range gateRequire {
			o.GateArgs = append(o.GateArgs, "--require", r)
		}
		y, err := sel.ChildPipeline(o)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, y)
		return err
	})
}

func splitList(l []string) []string {
	var out []string
	for _, v := range l {
		for _, x := range strings.Split(v, ",") {
			if x = strings.TrimSpace(x); x != "" && !contains(out, x) {
				out = append(out, x)
			}
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}
