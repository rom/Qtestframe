// Command admindoc writes the parts of the administrator guide
// (docs/admin-guide/<lang>/*.md) that are generated from the code: the
// configuration reference, the environment variables, the system
// settings, the permission matrix, the hardening lists, the webhook events,
// the backup tables, the migrations and the command line reference (see
// internal/admindoc).
//
//	go run ./internal/admindoc/cmd/admindoc            # update the pages
//	go run ./internal/admindoc/cmd/admindoc -check     # fail when out of date
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rom/qtestframe/internal/admindoc"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "admindoc:", err)
		os.Exit(1)
	}
}

func run(args []string, log io.Writer) error {
	fs := flag.NewFlagSet("admindoc", flag.ContinueOnError)
	fs.SetOutput(log)
	root := fs.String("root", ".", "the repository root")
	check := fs.Bool("check", false, "only report pages that are out of date (exit status 1)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return admindoc.Run(*root, *check, func(s string) { fmt.Fprintln(log, s) })
}
