package app

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rom/qtestframe/internal/ci"
	"github.com/rom/qtestframe/internal/uploader"
)

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// cmdReport uploads automated test results to a Qtestframe server (see
// internal/uploader). In a CI pipeline it records the build (pipeline,
// commit, branch, merge request) and, with --post-status / --post-comment,
// posts the commit status qtestframe/tests and a merge request note.
func cmdReport(args []string, env Env) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	o := uploader.Options{Log: env.Stderr}
	var sf serverFlags
	sf.register(fs)
	var pf postFlags
	pf.register(fs)
	fs.StringVar(&o.Run, "run", "", "name of the test run to report into")
	fs.StringVar(&o.RunID, "run-id", "", "id of the test run to report into")
	fs.BoolVar(&o.CreateRun, "create-run", false, "create the run named by --run when the project has none")
	fs.StringVar(&o.Build, "build", "", "build of a run created with --create-run (default the CI pipeline number)")
	fs.StringVar(&o.Environment, "environment", "", "environment of a run created with --create-run")
	fs.StringVar(&o.Configuration, "configuration", getenv("QTF_CONFIGURATION"), "configuration (name or id) the results were obtained in; added to the run when it does not target it yet (default $QTF_CONFIGURATION)")
	fs.Var((*stringList)(&o.JUnit), "junit", "JUnit XML report (file or glob; repeatable)")
	fs.Var((*stringList)(&o.JSON), "json", `results in the JSON import format {"results":[...]} (file or glob; repeatable)`)
	fs.BoolVar(&o.CreateMissing, "create-missing", false, "create draft automated tests for JUnit test cases without a test key (needs test.write)")
	fs.BoolVar(&o.FailOnUnknown, "fail-on-unknown", false, "exit with an error when results match no test (unknown keys or test cases without a key)")
	ciMode := fs.String("ci", "auto", "record the CI build (pipeline id and number, commit, branch, merge request) of the results: auto (from the CI variables) or none")
	fs.StringVar(&o.CI.BuildID, "build-id", "", "CI build id, unique in the project (default from the CI variables)")
	fs.StringVar(&o.CI.BuildNumber, "build-number", "", "CI build number (default from the CI variables)")
	fs.StringVar(&o.CI.Commit, "commit", "", "commit SHA of the build (default from the CI variables)")
	fs.StringVar(&o.CI.Branch, "branch", "", "branch of the build (default from the CI variables)")
	fs.StringVar(&o.CI.PipelineURL, "pipeline-url", "", "pipeline URL of the build (default from the CI variables)")
	fs.StringVar(&o.CI.MergeRequest, "merge-request", "", "merge / pull request of the build (default from the CI variables)")
	// Further arguments are report files, e.g. from a glob the shell
	// expanded after --junit: *.json are JSON results, all others JUnit XML.
	// Flags may follow them.
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	for _, a := range rest {
		if strings.EqualFold(filepath.Ext(a), ".json") {
			o.JSON = append(o.JSON, a)
		} else {
			o.JUnit = append(o.JUnit, a)
		}
	}
	if sf.token == "" {
		sf.token = getenv("QTF_TOKEN")
	}
	o.URL, o.Token, o.Project = sf.url, sf.token, sf.project
	e := ci.Detect(getenv)
	switch *ciMode {
	case "auto":
		fill := func(dst *string, v string) {
			if *dst == "" {
				*dst = v
			}
		}
		fill(&o.CI.BuildID, e.BuildID)
		fill(&o.CI.BuildNumber, e.BuildNumber)
		fill(&o.CI.Commit, e.Commit)
		fill(&o.CI.Branch, e.Branch)
		fill(&o.CI.PipelineURL, e.PipelineURL)
		fill(&o.CI.MergeRequest, e.MergeRequest)
	case "none":
	default:
		return errors.New("--ci must be auto or none")
	}
	e.Commit, e.MergeRequest, e.Branch = firstNonEmpty(o.CI.Commit, e.Commit), firstNonEmpty(o.CI.MergeRequest, e.MergeRequest), firstNonEmpty(o.CI.Branch, e.Branch)
	sum, err := uploader.Report(env.Ctx, o)
	if sum != nil {
		sum.Print(env.Stdout)
	}
	if err == nil || errors.Is(err, uploader.ErrUnknown) {
		postReport(env, sf, pf, e, sum)
	}
	if errors.Is(err, uploader.ErrUnknown) {
		return fmt.Errorf("%w (--fail-on-unknown)", err)
	}
	return err
}

// postReport posts the commit status qtestframe/tests (and the note) of
// the run's results.
func postReport(env Env, sf serverFlags, pf postFlags, e ci.Env, sum *uploader.Summary) {
	if (!pf.status && !pf.comment) || sum == nil {
		return
	}
	if sf.project == "" {
		fmt.Fprintln(env.Stderr, "warning: cannot post to the code host without --project")
		return
	}
	rep, err := evaluateGate(env.Ctx, sf, uploader.GateRequest{Run: sum.RunID, Commit: e.Commit, MergeRequest: e.MergeRequest, SummaryOnly: true})
	if err != nil {
		fmt.Fprintf(env.Stderr, "warning: cannot summarise the results: %v\n", err)
		return
	}
	pf.post(env.Ctx, env.Stderr, e, rep, "qtestframe/tests")
}
