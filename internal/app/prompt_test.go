package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakePrompts serves projects, folders, releases and /prompts (two parts
// when split).
func newFakePrompts(t *testing.T) (*[]map[string]any, *httptest.Server) {
	var mu sync.Mutex
	reqs := &[]map[string]any{}
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/projects":
			_, _ = w.Write([]byte(`{"items":[{"id":"p1","key":"SHOP","name":"Shop"}]}`))
		case "/api/v1/projects/p1/folders":
			_, _ = w.Write([]byte(`{"items":[{"id":"f1","name":"Checkout"},{"id":"f2","name":"Cards","parent_id":"f1"}]}`))
		case "/api/v1/projects/p1/releases":
			_, _ = w.Write([]byte(`{"items":[{"id":"r1","name":"Spring","version":"1.0"}]}`))
		case "/api/v1/projects/p1/prompts":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			*reqs = append(*reqs, in)
			if in["kind"] == "bogus" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"kind must be one of …"}`))
				return
			}
			parts := []any{map[string]any{"index": 1, "title": "A", "label": "Checkout", "prompt": "# A part 1\n", "items": 2, "tokens": 3}}
			text := "# A part 1\n"
			if in["split"] == "folder" {
				parts = append(parts, map[string]any{"index": 2, "title": "A", "label": "Cards", "prompt": "# A part 2\n", "items": 1, "tokens": 3})
				text += "\n---\n\n# A part 2\n"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"kind": in["kind"], "title": "A", "prompt": text, "items": 3, "tokens": 6, "parts": parts})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(hs.Close)
	return reqs, hs
}

func TestPromptCommand(t *testing.T) {
	reqs, hs := newFakePrompts(t)
	withEnv(t, func(k string) string {
		return map[string]string{"QTF_URL": hs.URL, "QTF_TOKEN": "qtf_tok", "QTF_PROJECT": "SHOP"}[k]
	})
	// to standard output
	r := run(t, "", "prompt", "--kind", "architecture", "--scope", "tag:checkout", "--lang", "sv", "--formats", "mermaid,c4")
	if r.code != 0 || r.stdout != "# A part 1\n" || !strings.Contains(r.stderr, "1 part(s), about 6 tokens") {
		t.Fatalf("%+v", r)
	}
	in := (*reqs)[0]
	if in["kind"] != "architecture" || in["q"] != "tag:checkout" || in["lang"] != "sv" || len(in["formats"].([]any)) != 2 || in["folder_id"] != nil {
		t.Fatal(in)
	}
	// into a directory, split, with a folder path and a release version
	dir := filepath.Join(t.TempDir(), "prompts")
	r = run(t, "", "prompt", "--kind", "full_solution", "--split", "folder", "--folder", "checkout / cards", "--release", "1.0", "--compact",
		"--max-tokens", "4000", "--formats", "none", "--out", dir)
	if r.code != 0 || !strings.Contains(r.stdout, "part-02.md (Cards)") {
		t.Fatalf("%+v", r)
	}
	in = (*reqs)[1]
	if in["folder_id"] != "f2" || in["release_id"] != "r1" || in["compact"] != true || in["max_tokens"].(float64) != 4000 || len(in["formats"].([]any)) != 0 {
		t.Fatal(in)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "SHOP-full-solution-*-part-0*.md"))
	if len(files) != 2 {
		t.Fatal(files)
	}
	if b, _ := os.ReadFile(files[1]); string(b) != "# A part 2\n" {
		t.Fatal(string(b))
	}
	// one part: a single file; the folder by name
	r = run(t, "", "prompt", "--folder", "Checkout", "--out", dir)
	if r.code != 0 || (*reqs)[2]["folder_id"] != "f1" || !strings.Contains(r.stdout, "SHOP-architecture-") || strings.Contains(r.stdout, "part-") {
		t.Fatalf("%+v", r)
	}
	// errors
	for _, args := range [][]string{
		{"prompt", "--folder", "Nowhere"},
		{"prompt", "--release", "9.9"},
		{"prompt", "--kind", "bogus"},
		{"prompt", "--project", "", "--kind", "design"},
	} {
		if r := run(t, "", args...); r.code == 0 {
			t.Errorf("%v: %+v", args, r)
		}
	}
}
