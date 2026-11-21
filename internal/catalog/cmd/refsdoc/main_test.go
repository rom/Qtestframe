package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/catalog"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	for _, lang := range catalog.Languages {
		if err := os.MkdirAll(filepath.Join(dir, lang), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, lang, catalog.RefsDocPage), []byte("# References\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(dir, true); err == nil || !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("check of stale pages: %v", err)
	}
	if err := run(dir, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "sv", catalog.RefsDocPage))
	if !strings.HasPrefix(string(b), "# References\n\n"+catalog.RefsDocBegin) || !strings.Contains(string(b), "Källor som katalogen hänvisar till") {
		t.Errorf("page:\n%.300s", b)
	}
	if err := run(dir, true); err != nil {
		t.Fatalf("check of current pages: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "en", catalog.RefsDocPage), []byte(catalog.RefsDocBegin), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(dir, false); err == nil {
		t.Error("unbalanced markers accepted")
	}
	if err := run(filepath.Join(dir, "missing"), false); err == nil {
		t.Error("missing folder accepted")
	}
}
