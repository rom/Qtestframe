package api

import (
	"archive/zip"
	"bytes"
	"io"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// TestReqIFExportRoundTrip exports requirements as ReqIF and imports the
// file into a new project: keys (as external IDs), titles, descriptions,
// enumerations, texts, tags, criteria and folders come back.
func TestReqIFExportRoundTrip(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("RIF")
	sec := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Security"}, 201)["id"].(string)
	login := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Login", "parent_id": sec}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Unused"}, 201)
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{
		"title": "Lockout", "folder_id": login, "obligation": "shall", "type": "security", "priority": "high", "status": "approved",
		"description": "The system **SHALL** lock an account.\n\n- after 5 failed attempts\n- for 10 minutes",
		"source":      "Security policy", "rationale": "Brute force protection", "owner": "anna", "tags": []string{"auth", "web"},
		"criteria": []map[string]any{{"description": "Locked after 5 failures", "method": "test", "threshold": "5"}},
	}, 201)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{
		"title": "Export data", "folder_id": sec, "obligation": "may", "type": "functional", "priority": "low", "status": "draft",
		"description": "The user MAY export data.",
	}, 201)
	r3 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Top level", "description": "Line one\nLine two", "obligation": "must"}, 201)
	a.m("POST", "/api/v1/requirements/"+r2["id"].(string)+"/relations", map[string]any{"target_id": r1["id"], "type": "refines"}, 201)
	a.upload("POST", "/api/v1/requirements/"+r1["id"].(string)+"/attachments", "file", "flow.png", "image/png", pngBytes(t), nil, 201)

	get := func(q string) (string, string, []byte) {
		res := a.raw("GET", "/api/v1/projects/"+pid+"/export?"+q, nil)
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("%s: %d %s", q, res.StatusCode, data)
		}
		return res.Header.Get("Content-Type"), res.Header.Get("Content-Disposition"), data
	}
	ct, cd, doc := get("format=reqif")
	if !strings.HasPrefix(ct, "application/xml") || !strings.Contains(cd, ".reqif\"") || !strings.Contains(cd, "RIF-") {
		t.Fatalf("headers %s %s", ct, cd)
	}
	s := string(doc)
	for _, want := range []string{"qtf-req-" + r1["id"].(string), "qtf-folder-" + sec, "<SPEC-RELATION ", "qtf-reltype-refines</SPEC-RELATION-TYPE-REF>",
		`LONG-NAME="Unused"`, "<xhtml:strong>SHALL</xhtml:strong>"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(s, "flow.png") {
		t.Error("attachments in a plain .reqif")
	}

	// Filtered: only REQ-1 and its folders.
	_, _, filtered := get("format=reqif&q=" + url.QueryEscape("obligation:shall"))
	if f := string(filtered); !strings.Contains(f, r1["key"].(string)) || strings.Contains(f, r2["key"].(string)+`"`) ||
		strings.Contains(f, `LONG-NAME="Unused"`) || !strings.Contains(f, `LONG-NAME="Security"`) || strings.Contains(f, "<SPEC-RELATION ") {
		t.Errorf("filtered export:\n%s", f)
	}

	// .reqifz with the attachment.
	ct, cd, zdata := get("format=REQIFZ")
	zr, err := zip.NewReader(bytes.NewReader(zdata), int64(len(zdata)))
	if err != nil || ct != "application/zip" || !strings.Contains(cd, ".reqifz") {
		t.Fatalf("%v %s %s", err, ct, cd)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "RIF.reqif,files/"+r1["key"].(string)+"/flow.png" {
		t.Errorf("archive: %v", names)
	}

	// Import both into new projects.
	for _, file := range []struct {
		name string
		data []byte
	}{{"RIF.reqif", doc}, {"RIF.reqifz", zdata}} {
		npid := a.project("NEW" + strings.ToUpper(strings.TrimPrefix(file.name[strings.LastIndex(file.name, "."):], ".")))
		res := a.upload("POST", importPath(npid), "file", file.name, "", file.data, nil, 201)
		if res["created"].(float64) != 3 {
			t.Fatalf("%s import: %v", file.name, res)
		}
		got := map[string]map[string]any{}
		for _, rq := range items(a.m("GET", "/api/v1/projects/"+npid+"/requirements", nil, 200)) {
			got[rq["title"].(string)] = rq
		}
		for _, orig := range []map[string]any{r1, r2, r3} {
			orig = a.m("GET", "/api/v1/requirements/"+orig["id"].(string), nil, 200)
			n := got[orig["title"].(string)]
			if n == nil {
				t.Fatalf("%s: %s missing", file.name, orig["title"])
			}
			for _, f := range []string{"description", "obligation", "type", "priority", "status", "rationale", "owner", "folder_path"} {
				if n[f] != orig[f] {
					t.Errorf("%s %s: %s = %q, want %q", file.name, orig["key"], f, n[f], orig[f])
				}
			}
			if want := sourceWithID(orig["source"].(string), orig["key"].(string)); n["source"] != want {
				t.Errorf("%s source %q, want %q", file.name, n["source"], want)
			}
			if a, b := n["tags"], orig["tags"]; strings.Join(toStrings(a), ",") != strings.Join(toStrings(b), ",") {
				t.Errorf("tags %v %v", a, b)
			}
		}
		full := a.m("GET", "/api/v1/requirements/"+got["Lockout"]["id"].(string), nil, 200)
		cr, _ := full["criteria"].([]any)
		if len(cr) != 1 || cr[0].(map[string]any)["threshold"] != "5" || cr[0].(map[string]any)["method"] != "test" {
			t.Errorf("criteria: %v", full["criteria"])
		}
	}

	// Permission and errors.
	a.m("POST", "/api/v1/roles", map[string]any{"name": "reader", "permissions": []string{"project.read", "requirement.read"}}, 201)
	e.member("reader@example.com", "reader")
	e.login("reader@example.com").m("GET", "/api/v1/projects/"+pid+"/export?format=reqif", nil, 403)
	a.m("GET", "/api/v1/projects/nope/export?format=reqif", nil, 404)
	a.m("GET", "/api/v1/projects/"+pid+"/export?format=reqif&q=bad%3A%3A", nil, 400)
}

func toStrings(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		out = append(out, x.(string))
	}
	return out
}
