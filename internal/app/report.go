package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
// internal/uploader).
func cmdReport(args []string, env Env) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	o := uploader.Options{Log: env.Stderr}
	fs.StringVar(&o.URL, "url", os.Getenv("QTF_URL"), "server base URL, e.g. https://qtestframe.example.com (default $QTF_URL)")
	fs.StringVar(&o.Token, "token", "", "API token (default $QTF_TOKEN)")
	fs.StringVar(&o.Project, "project", "", "project key, e.g. SHOP (needed with --run)")
	fs.StringVar(&o.Run, "run", "", "name of the test run to report into")
	fs.StringVar(&o.RunID, "run-id", "", "id of the test run to report into")
	fs.BoolVar(&o.CreateRun, "create-run", false, "create the run named by --run when the project has none")
	fs.StringVar(&o.Build, "build", "", "build of a run created with --create-run")
	fs.StringVar(&o.Environment, "environment", "", "environment of a run created with --create-run")
	fs.Var((*stringList)(&o.JUnit), "junit", "JUnit XML report (file or glob; repeatable)")
	fs.Var((*stringList)(&o.JSON), "json", `results in the JSON import format {"results":[...]} (file or glob; repeatable)`)
	fs.BoolVar(&o.CreateMissing, "create-missing", false, "create draft automated tests for JUnit test cases without a test key (needs test.write)")
	fs.BoolVar(&o.FailOnUnknown, "fail-on-unknown", false, "exit with an error when results match no test (unknown keys or test cases without a key)")
	// Further arguments are report files, e.g. from a glob the shell
	// expanded after --junit: *.json are JSON results, all others JUnit XML.
	// Flags may follow them.
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		rest = append(rest, fs.Arg(0))
		args = fs.Args()[1:]
	}
	for _, a := range rest {
		if strings.EqualFold(filepath.Ext(a), ".json") {
			o.JSON = append(o.JSON, a)
		} else {
			o.JUnit = append(o.JUnit, a)
		}
	}
	if o.Token == "" {
		o.Token = os.Getenv("QTF_TOKEN")
	}
	sum, err := uploader.Report(env.Ctx, o)
	if sum != nil {
		sum.Print(env.Stdout)
	}
	if errors.Is(err, uploader.ErrUnknown) {
		return fmt.Errorf("%w (--fail-on-unknown)", err)
	}
	return err
}
