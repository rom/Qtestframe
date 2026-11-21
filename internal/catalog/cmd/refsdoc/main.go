// Command refsdoc writes the "Sources cited in the catalogue" section of
// the user guide (docs/user-guide/<lang>/references.md) from the
// catalogue's source registry, between the markers of the section.
//
//	go run ./internal/catalog/cmd/refsdoc            # update the pages
//	go run ./internal/catalog/cmd/refsdoc -check     # fail when out of date
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rom/qtestframe/internal/catalog"
)

func main() {
	dir := flag.String("dir", "docs/user-guide", "the user guide folder (with one folder per language)")
	check := flag.Bool("check", false, "only report pages that are out of date (exit status 1)")
	flag.Parse()
	if err := run(*dir, *check); err != nil {
		fmt.Fprintln(os.Stderr, "refsdoc:", err)
		os.Exit(1)
	}
}

func run(dir string, check bool) error {
	c, err := catalog.Default()
	if err != nil {
		return err
	}
	stale := 0
	for _, lang := range catalog.Languages {
		path := filepath.Join(dir, lang, catalog.RefsDocPage)
		old, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		page, err := catalog.ReplaceRefsDoc(string(old), c.SourcesMarkdown(lang))
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if bytes.Equal(old, []byte(page)) {
			continue
		}
		if check {
			fmt.Fprintf(os.Stderr, "%s is out of date: run make refs-docs\n", path)
			stale++
			continue
		}
		if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}
	if stale > 0 {
		return fmt.Errorf("%d page(s) out of date", stale)
	}
	return nil
}
