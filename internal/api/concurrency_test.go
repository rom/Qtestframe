package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// patchIf sends a PATCH with If-Match and returns the status, ETag and body.
func patchIf(t *testing.T, c *client, path, ifMatch string, body any) (int, string, map[string]any) {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest("PATCH", c.e.hs.URL+path, bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, res.Header.Get("ETag"), out
}

func TestOptimisticConcurrency(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("OCC")
	for _, kind := range []string{"requirements", "tests"} {
		id := a.m("POST", "/api/v1/projects/"+pid+"/"+kind, map[string]any{"title": "Original"}, 201)["id"].(string)
		path := "/api/v1/" + kind + "/" + id

		res := a.raw("GET", path, nil)
		res.Body.Close()
		if tag := res.Header.Get("ETag"); tag != `"1"` {
			t.Fatal(kind, "etag", tag)
		}
		// Anna saves version 1
		code, tag, _ := patchIf(t, a, path, `"1"`, map[string]any{"title": "Anna's title"})
		if code != 200 || tag != `"2"` {
			t.Fatal(kind, code, tag)
		}
		// Bo also started from version 1: refused with what is saved now
		code, tag, body := patchIf(t, a, path, `"1"`, map[string]any{"title": "Bo's title"})
		if code != 412 || tag != `"2"` || body["code"] != "stale" {
			t.Fatal(kind, code, tag, body)
		}
		cur := body["current"].(map[string]any)
		if cur["title"] != "Anna's title" || cur["version"].(float64) != 2 || body["changed_by"] == "" || body["changed_at"] == "" {
			t.Fatal(kind, body)
		}
		// weak tags and the current version work; "*" and no header do not check
		if code, _, _ := patchIf(t, a, path, `W/"2"`, map[string]any{"title": "Bo's title"}); code != 200 {
			t.Fatal(kind, code)
		}
		if code, _, _ := patchIf(t, a, path, `*`, map[string]any{"priority": "high"}); code != 200 {
			t.Fatal(kind, code)
		}
		if code, tag, _ := patchIf(t, a, path, "", map[string]any{"priority": "low"}); code != 200 || tag != `"5"` {
			t.Fatal(kind, code, tag)
		}
		if code, _, _ := patchIf(t, a, path, `"x"`, map[string]any{"priority": "low"}); code != 400 {
			t.Fatal(kind, code)
		}
		if got := a.m("GET", path, nil, 200)["title"]; got != "Bo's title" {
			t.Fatal(kind, got)
		}
	}
}
