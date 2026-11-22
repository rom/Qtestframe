package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/stubgen"
	"github.com/rom/qtestframe/internal/uploader"
)

// cmdStubs writes generated test stubs into a directory (qtestframe
// stubs, see docs/user-guide/en/ci-cd.md "Generate test code"). Files are
// merged with stubgen.Merge: new ones are created, unchanged stubs are
// regenerated, implemented ones only get their header block updated and
// files without qtestframe markers are left alone.
func cmdStubs(args []string, env Env) error {
	fs := flag.NewFlagSet("stubs", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var sf serverFlags
	sf.register(fs)
	var ids stringList
	framework := fs.String("framework", "", "test framework: "+strings.Join(stubgen.FrameworkIDs(), ", "))
	query := fs.String("query", "execution_type:automated", "the tests to generate stubs for (query language); \"\" for all")
	fs.Var(&ids, "ids", "only these tests (keys or ids; repeatable or comma-separated)")
	out := fs.String("out", ".", "directory to write the stubs to")
	pkg := fs.String("package", "", "Go package (default: the name of --out, else tests) or Java package of the stubs")
	dryRun := fs.Bool("dry-run", false, "only print what would be written")
	markScript := fs.Bool("mark-script", false, "set the script of tests without one to their stub path (needs test.write)")
	scriptDir := fs.String("script-dir", "", "directory of the stubs in the repository for --mark-script (default --out when it is relative)")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	fw, ok := stubgen.Lookup(*framework)
	if !ok {
		return fmt.Errorf("--framework is required: one of %s", strings.Join(stubgen.FrameworkIDs(), ", "))
	}
	if sf.project == "" {
		return errors.New("--project KEY is required (or $QTF_PROJECT)")
	}
	if *pkg == "" && fw.ID == "go" {
		if abs, err := filepath.Abs(*out); err == nil {
			*pkg = strings.ToLower(strings.ReplaceAll(filepath.Base(abs), "-", "_"))
		}
	}
	dir := *scriptDir
	if *markScript && dir == "" {
		if filepath.IsAbs(*out) {
			return errors.New("--mark-script with an absolute --out needs --script-dir (the directory in the repository)")
		}
		dir = filepath.ToSlash(filepath.Clean(*out))
	}
	c, err := sf.client()
	if err != nil {
		return err
	}
	p, err := c.ProjectByKey(env.Ctx, sf.project)
	if err != nil {
		return err
	}
	in := uploader.StubsRequest{Framework: fw.ID, Query: *query, IDs: splitList(ids), Package: *pkg}
	res, err := c.Stubs(env.Ctx, p.ID, in)
	if err != nil {
		return err
	}
	log := env.Stderr
	if len(res.Files) == 0 {
		fmt.Fprintf(log, "no tests of %s match %q\n", p.Key, *query)
		return nil
	}
	if !*dryRun {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			return err
		}
	}
	counts := map[stubgen.Action]int{}
	var written []string
	for _, f := range res.Files {
		name := path.Base(f.Path)
		if name != f.Path || name == "." || name == ".." || strings.ContainsAny(name, `\`) {
			return fmt.Errorf("unexpected stub path %q from the server", f.Path)
		}
		target := filepath.Join(*out, name)
		existing, err := os.ReadFile(target)
		if errors.Is(err, os.ErrNotExist) {
			existing, err = nil, nil
		}
		if err != nil {
			return err
		}
		content, action := stubgen.Merge(existing, stubgen.File{Path: f.Path, Content: f.Content, Key: f.Key, Framework: fw.ID})
		counts[action]++
		note := ""
		switch action {
		case stubgen.Kept:
			note = " (no qtestframe markers of " + f.Key + ": left alone)"
		case stubgen.HeaderUpdated:
			note = " (implemented: header block updated)"
		}
		fmt.Fprintf(env.Stdout, "%-11s %s%s\n", action, target, note)
		if action != stubgen.Kept {
			written = append(written, f.Key)
		}
		if *dryRun || action == stubgen.Kept || action == stubgen.Unchanged {
			continue
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return err
		}
	}
	var parts []string
	for _, a := range []stubgen.Action{stubgen.Created, stubgen.Regenerated, stubgen.HeaderUpdated, stubgen.Unchanged, stubgen.Kept} {
		if counts[a] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[a], a))
		}
	}
	verb := ""
	if *dryRun {
		verb = " (dry run: nothing written)"
	}
	fmt.Fprintf(log, "%s stubs of %d test(s) of %s in %s: %s%s\n", fw.Name, len(res.Files), p.Key, *out, strings.Join(parts, ", "), verb)
	if *markScript && !*dryRun && len(written) > 0 {
		sort.Strings(written)
		m, err := c.Stubs(env.Ctx, p.ID, uploader.StubsRequest{Framework: fw.ID, IDs: written, Query: "", Package: *pkg, Dir: dir, MarkScript: true})
		if err != nil {
			return fmt.Errorf("marking the scripts: %w", err)
		}
		fmt.Fprintf(log, "marked the script of %d test(s) with its stub path\n", m.Marked)
	}
	return nil
}
