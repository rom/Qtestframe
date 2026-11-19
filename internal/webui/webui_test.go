package webui

import (
	"io/fs"
	"testing"
)

func TestFS(t *testing.T) {
	_, statErr := fs.Stat(dist, "dist/index.html")
	got := FS()
	if statErr != nil {
		if got != nil {
			t.Fatal("FS() must be nil when index.html is not embedded")
		}
		return
	}
	if got == nil {
		t.Fatal("FS() is nil although index.html is embedded")
	}
	b, err := fs.ReadFile(got, "index.html")
	if err != nil || len(b) == 0 {
		t.Fatalf("read index.html: %v", err)
	}
}
