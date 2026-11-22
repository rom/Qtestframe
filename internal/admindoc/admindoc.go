// Package admindoc generates the parts of the administrator guide
// (docs/admin-guide/<lang>/*.md) that are taken from the code: the
// configuration reference and the environment variables (internal/config),
// the runtime system settings (internal/settings), the default permission
// matrix (internal/rbac), the hardening lists (internal/hardening), the
// webhook events (internal/model), the tables
// of a backup (internal/store), the migrations (internal/db) and the
// command line reference (internal/app).
//
// Each generated part sits between the markers
//
//	<!-- BEGIN GENERATED: name (make admin-guide-ref) -->
//	<!-- END GENERATED: name -->
//
// in a page of the guide; the rest of the page is written by hand. The
// tests of this package fail when a page is out of date (run
// `make admin-guide-ref`) or when a configuration key has no description.
package admindoc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Languages of the guide.
var Languages = []string{"en", "sv"}

// Dir is the guide folder, relative to the repository root.
const Dir = "docs/admin-guide"

// block is one generated part of a page.
type block struct {
	name, page string
	gen        func(lang string) (string, error)
}

func blocks() []block {
	return []block{
		{"config-reference", "configuration.md", configReference},
		{"environment", "configuration.md", envReference},
		{"system-settings", "system-settings.md", settingsReference},
		{"permissions", "authorisation.md", permissionMatrix},
		{"hardening", "security.md", hardeningLists},
		{"webhook-events", "integrations.md", webhookEvents},
		{"backup-tables", "backup-and-migration.md", backupTables},
		{"migrations", "backup-and-migration.md", migrationList},
		{"cli", "cli-reference.md", cliReference},
	}
}

func begin(name string) string {
	return "<!-- BEGIN GENERATED: " + name + " (make admin-guide-ref) -->"
}

func end(name string) string { return "<!-- END GENERATED: " + name + " -->" }

// Replace puts content between the markers of block name in page.
func Replace(page, name, content string) (string, error) {
	b, e := begin(name), end(name)
	i := strings.Index(page, b)
	j := strings.Index(page, e)
	if i < 0 || j < 0 || j < i || strings.Count(page, b) != 1 || strings.Count(page, e) != 1 {
		return "", fmt.Errorf("the markers of %q are missing or unbalanced", name)
	}
	return page[:i+len(b)] + "\n\n" + strings.TrimSpace(content) + "\n\n" + page[j:], nil
}

var markerRE = regexp.MustCompile(`<!-- BEGIN GENERATED: ([a-z-]+) `)

// Run regenerates the generated parts of the pages below root/Dir. With
// check, nothing is written and pages that are out of date are an error.
func Run(root string, check bool, log func(string)) error {
	stale := 0
	for _, lang := range Languages {
		pages := map[string]string{}
		order := []string{}
		for _, b := range blocks() {
			path := filepath.Join(root, Dir, lang, b.page)
			if _, ok := pages[path]; !ok {
				old, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				pages[path] = string(old)
				order = append(order, path)
			}
			content, err := b.gen(lang)
			if err != nil {
				return fmt.Errorf("%s: %w", b.name, err)
			}
			if pages[path], err = Replace(pages[path], b.name, content); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
		// every marker in the guide must belong to a known block
		known := map[string]bool{}
		for _, b := range blocks() {
			known[b.name] = true
		}
		files, _ := filepath.Glob(filepath.Join(root, Dir, lang, "*.md"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			for _, m := range markerRE.FindAllStringSubmatch(string(b), -1) {
				if !known[m[1]] {
					return fmt.Errorf("%s: unknown generated block %q", f, m[1])
				}
			}
		}
		for _, path := range order {
			old, _ := os.ReadFile(path)
			if bytes.Equal(old, []byte(pages[path])) {
				continue
			}
			if check {
				log(path + " is out of date: run make admin-guide-ref")
				stale++
				continue
			}
			if err := os.WriteFile(path, []byte(pages[path]), 0o644); err != nil {
				return err
			}
			log("wrote " + path)
		}
	}
	if stale > 0 {
		return fmt.Errorf("%d page(s) out of date", stale)
	}
	return nil
}

// cell escapes a table cell.
func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ")
}

func code(s string) string { return "`" + s + "`" }
