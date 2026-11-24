package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/testimport"
	"github.com/rom/qtestframe/internal/uploader"
)

// cmdImportTests imports the test case export of another test management
// tool (qtestframe import-tests, see docs/user-guide/en/interoperability.md
// "Import tests from other tools"): the file is previewed on the server and,
// unless --dry-run, applied. Importing the file again updates the tests.
func cmdImportTests(args []string, env Env) error {
	fs := flag.NewFlagSet("import-tests", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var sf serverFlags
	sf.register(fs)
	from := fs.String("from", "", "the tool the file comes from: "+strings.Join(testimport.Sources, ", "))
	format := fs.String("format", "", "file format: csv, xml, json or xlsx (default: from the file name)")
	folder := fs.String("folder", "", "test folder to import into (path or id; default: top level)")
	dryRun := fs.Bool("dry-run", false, "only show what would be created and updated")
	overwrite := fs.Bool("overwrite", false, "also update tests changed in Qtestframe since the last import (conflicts)")
	noHistory := fs.Bool("no-history", false, "do not import past results as runs")
	noLinks := fs.Bool("no-links", false, "do not link the tests to the requirements they reference")
	flat := fs.Bool("no-folders", false, "do not create folders for the sections, suites, modules or area paths")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	source, ok := testimport.ValidSource(*from)
	if !ok {
		return fmt.Errorf("--from is required: one of %s", strings.Join(testimport.Sources, ", "))
	}
	if len(rest) != 1 {
		return errors.New("name exactly one FILE to import")
	}
	if sf.project == "" {
		return errors.New("--project KEY is required (or $QTF_PROJECT)")
	}
	data, err := os.ReadFile(rest[0])
	if err != nil {
		return err
	}
	c, err := sf.client()
	if err != nil {
		return err
	}
	p, err := c.ProjectByKey(env.Ctx, sf.project)
	if err != nil {
		return err
	}
	fields := map[string]string{"source": source, "format": *format, "overwrite_conflicts": strconv.FormatBool(*overwrite),
		"history": strconv.FormatBool(!*noHistory), "link_requirements": strconv.FormatBool(!*noLinks), "create_folders": strconv.FormatBool(!*flat)}
	if *folder != "" {
		id, err := c.FindFolderOf(env.Ctx, p.ID, "test", *folder)
		if err != nil {
			return err
		}
		if id == "" {
			return fmt.Errorf("no test folder %q in %s", *folder, p.Key)
		}
		fields["folder_id"] = id
	}
	var pv uploader.TestImportPreview
	if err := c.TestImport(env.Ctx, p.ID, false, rest[0], data, fields, &pv); err != nil {
		return err
	}
	out := env.Stdout
	for _, r := range pv.Rows {
		label := r.Title
		if r.ExternalID != "" {
			label = r.ExternalID + " " + label
		}
		key := ""
		if r.Key != "" {
			key = " → " + r.Key
		}
		fmt.Fprintf(out, "%-9s %s%s\n", r.Action, label, key)
		for _, m := range r.Errors {
			fmt.Fprintf(out, "          error: %s\n", m)
		}
		for _, m := range r.Warnings {
			fmt.Fprintf(out, "          warning: %s\n", m)
		}
	}
	log := env.Stderr
	for _, m := range append(pv.Warnings, pv.Errors...) {
		fmt.Fprintf(log, "warning: %s\n", m)
	}
	s := pv.Summary
	fmt.Fprintf(log, "%s %s: %d test(s): %d new, %d to update, %d unchanged, %d conflict(s), %d error(s); %d past result(s) in %d run(s)",
		pv.SourceName, pv.Format, pv.Total, s.New, s.Update, s.Unchanged, s.Conflict, s.Error, pv.History.Executions-pv.History.Known, pv.History.Runs)
	if len(pv.Unresolved) > 0 {
		fmt.Fprintf(log, "; no requirement matches %s", strings.Join(pv.Unresolved, ", "))
	}
	fmt.Fprintln(log)
	if *dryRun {
		fmt.Fprintln(log, "dry run: nothing imported")
		return nil
	}
	var res uploader.TestImportResult
	if err := c.TestImport(env.Ctx, p.ID, true, rest[0], data, fields, &res); err != nil {
		return err
	}
	for _, sk := range res.SkippedRows {
		fmt.Fprintf(log, "skipped %s (%s): %s\n", sk.Location, sk.Title, sk.Reason)
	}
	for _, m := range res.Warnings {
		if !contains(pv.Warnings, m) {
			fmt.Fprintf(log, "warning: %s\n", m)
		}
	}
	fmt.Fprintf(log, "imported into %s: %d created, %d updated, %d unchanged, %d skipped, %d folder(s), %d link(s), %d run(s), %d past result(s)\n",
		p.Key, res.Created, res.Updated, res.Unchanged, res.Skipped, res.FoldersCreated, res.LinksAdded, res.RunsCreated, res.Results)
	return nil
}
