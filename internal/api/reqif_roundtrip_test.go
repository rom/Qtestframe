package api

import (
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/reqimport"
)

// reqifCanon is the semantic content of a ReqIF document: per item kind
// its objects (folder path and attribute values, ids, keys, versions,
// dates and the importer's source note left out) and its relations.
type reqifCanon struct {
	objects   []string
	relations []string
}

var keyPattern = regexp.MustCompile(`\b[A-Z][A-Z0-9]*-(REQ|TC)-[0-9]+\b|\b[A-Z][A-Z0-9]*-[0-9]+\b`)

func canonReqIF(t *testing.T, data []byte) reqifCanon {
	t.Helper()
	m, err := reqimport.ParseReqIFModel(data)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*reqimport.ReqIFObject{}
	for i := range m.Objects {
		byID[m.Objects[i].Identifier] = &m.Objects[i]
	}
	skip := map[string]bool{"ReqIF.ForeignID": true, "Version": true, "ReqIF.ForeignCreatedBy": true, "ReqIF.ForeignCreatedOn": true, "Source": true}
	title := map[string]string{}
	var c reqifCanon
	for _, o := range m.Objects {
		if o.TypeName == "Heading" {
			continue
		}
		var path []string
		for p := byID[o.Parent]; p != nil; p = byID[p.Parent] {
			path = append([]string{p.Values["ReqIF.ChapterName"]}, path...)
		}
		var vals []string
		for k, v := range o.Values {
			if !skip[k] {
				vals = append(vals, k+"="+keyPattern.ReplaceAllString(v, "KEY"))
			}
		}
		sort.Strings(vals)
		title[o.Identifier] = o.TypeName + ":" + o.Values["ReqIF.Name"]
		c.objects = append(c.objects, o.TypeName+" @"+strings.Join(path, "/")+" "+strings.Join(vals, " | "))
	}
	for _, r := range m.Relations {
		c.relations = append(c.relations, title[r.Source]+" "+r.Type+" "+title[r.Target])
	}
	sort.Strings(c.objects)
	sort.Strings(c.relations)
	return c
}

// TestReqIFRoundTripWithTests exports a project with requirements, tests,
// folders, custom fields, criteria, steps, relations, links and
// attachments as .reqifz, imports it into an empty project, exports that
// again and compares both documents semantically; importing the document
// once more changes nothing.
func TestReqIFRoundTripWithTests(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	src := a.project("RTA")
	dst := a.project("RTB")
	for _, pid := range []string{src, dst} {
		a.m("POST", "/api/v1/projects/"+pid+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Risk class", "type": "single_select", "options": []string{"Low", "High"}}, 201)
		a.m("POST", "/api/v1/projects/"+pid+"/custom-fields", map[string]any{"entity_type": "test", "label": "Rig", "type": "text"}, 201)
	}
	base := "/api/v1/projects/" + src
	sec := a.m("POST", base+"/folders", map[string]any{"kind": "requirement", "name": "Security"}, 201)["id"].(string)
	login := a.m("POST", base+"/folders", map[string]any{"kind": "requirement", "name": "Login", "parent_id": sec}, 201)["id"].(string)
	smoke := a.m("POST", base+"/folders", map[string]any{"kind": "test", "name": "Smoke"}, 201)["id"].(string)
	cfs := items(a.m("GET", base+"/custom-fields", nil, 200))
	cfKey := map[string]string{}
	for _, f := range cfs {
		cfKey[f["label"].(string)] = f["key"].(string)
	}
	r1 := a.m("POST", base+"/requirements", map[string]any{
		"title": "Lockout", "folder_id": login, "obligation": "shall", "type": "security", "priority": "high", "status": "approved",
		"description": "The system **SHALL** lock an account.\n\n- after 5 failed attempts\n- for 10 minutes",
		"rationale":   "Brute force", "owner": "anna", "tags": []string{"auth", "web"},
		"criteria":      []map[string]any{{"description": "Locked after 5 failures", "method": "test", "threshold": "5"}},
		"custom_fields": map[string]any{cfKey["Risk class"]: "High"},
	}, 201)
	r2 := a.m("POST", base+"/requirements", map[string]any{"title": "Unlock by e-mail", "folder_id": sec, "obligation": "should", "description": "A locked user MAY unlock by e-mail."}, 201)
	r3 := a.m("POST", base+"/requirements", map[string]any{"title": "Audit", "description": "Lockouts are logged.", "obligation": "must"}, 201)
	a.m("POST", "/api/v1/requirements/"+r2["id"].(string)+"/relations", map[string]any{"target_id": r1["id"], "type": "refines"}, 201)
	a.m("POST", "/api/v1/requirements/"+r3["id"].(string)+"/relations", map[string]any{"target_id": r1["id"], "type": "depends_on"}, 201)
	f := a.upload("POST", "/api/v1/requirements/"+r1["id"].(string)+"/attachments", "file", "flow.png", "image/png", pngBytes(t), nil, 201)
	a.m("PATCH", "/api/v1/requirements/"+r1["id"].(string), map[string]any{"description": r1["description"].(string) + "\n\n![flow](/api/v1/files/" + f["id"].(string) + "/content)"}, 200)
	t1 := a.m("POST", base+"/tests", map[string]any{"title": "Lockout after five attempts", "folder_id": smoke, "preconditions": "A user exists", "expected_result": "Locked",
		"priority": "high", "status": "ready", "estimated_minutes": 5, "tags": []string{"auth"}, "custom_fields": map[string]any{cfKey["Rig"]: "Bench 2"},
		"steps": []map[string]any{{"action": "Log in with a wrong password 5 times", "data": "user=anna", "expected": "Error each time"}, {"action": "Log in correctly", "expected": "Account locked"}}}, 201)
	t2 := a.m("POST", base+"/tests", map[string]any{"title": "Unlock mail", "description": "Check the mail"}, 201)
	a.m("POST", base+"/links", map[string]any{"requirement_id": r1["id"], "test_case_id": t1["id"]}, 201)
	a.m("POST", base+"/links", map[string]any{"requirement_id": r2["id"], "test_case_id": t2["id"]}, 201)
	a.upload("POST", "/api/v1/tests/"+t1["id"].(string)+"/attachments", "file", "log.txt", "text/plain", []byte("attempt log"), nil, 201)

	export := func(pid string) []byte {
		res := a.raw("GET", "/api/v1/projects/"+pid+"/export?format=reqifz&include=tests", nil)
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("export: %d %s", res.StatusCode, b)
		}
		return b
	}
	first := export(src)
	if s := string(first); !strings.Contains(s, "PK") {
		t.Fatal("not a zip")
	}

	dbase := "/api/v1/projects/" + dst
	pv := a.upload("POST", dbase+"/exchange/reqif/preview", "file", "rta.reqifz", "", first, nil, 200)
	if s := pv["summary"].(map[string]any); s["new"].(float64) != 3 {
		t.Fatalf("requirement diff: %v", s)
	}
	tests, _ := pv["tests"].(map[string]any)
	if tests == nil || tests["summary"].(map[string]any)["new"].(float64) != 2 {
		t.Fatalf("test diff: %v", pv["tests"])
	}
	rels := pv["relations"].(map[string]any)
	if rels["add"].(float64) != 4 || rels["links"].(float64) != 2 {
		t.Errorf("relations: %v", rels)
	}
	if files := pv["files"].([]any); len(files) != 2 {
		t.Errorf("files: %v", files)
	}
	kinds := map[string]string{}
	for _, x := range pv["object_types"].([]any) {
		m := x.(map[string]any)
		kinds[m["name"].(string)] = m["kind"].(string)
	}
	if kinds["Test case"] != "test" || kinds["Requirement"] != "requirement" {
		t.Errorf("object types: %v", kinds)
	}
	res := a.upload("POST", dbase+"/exchange/reqif/apply", "file", "rta.reqifz", "", first, nil, 200)
	if res["created"].(float64) != 5 || res["relations_added"].(float64) != 2 || res["links_added"].(float64) != 2 || res["files_attached"].(float64) != 2 {
		t.Fatalf("apply: %v", res)
	}
	// the image in the description points to the new attachment
	for _, rq := range items(a.m("GET", dbase+"/requirements", nil, 200)) {
		if rq["title"] == "Lockout" && !strings.Contains(rq["description"].(string), "](/api/v1/files/") {
			t.Errorf("description: %q", rq["description"])
		}
	}

	second := export(dst)
	c1, c2 := canonReqIF(t, first), canonReqIF(t, second)
	if strings.Join(c1.objects, "\n") != strings.Join(c2.objects, "\n") {
		t.Errorf("objects differ:\n%s\n---\n%s", strings.Join(c1.objects, "\n"), strings.Join(c2.objects, "\n"))
	}
	if strings.Join(c1.relations, "\n") != strings.Join(c2.relations, "\n") {
		t.Errorf("relations differ:\n%v\n%v", c1.relations, c2.relations)
	}
	if len(c1.relations) != 4 {
		t.Errorf("relations: %v", c1.relations)
	}

	// a re-import of the same document updates nothing
	pv = a.upload("POST", dbase+"/exchange/reqif/preview", "file", "rta.reqifz", "", first, nil, 200)
	if s := pv["summary"].(map[string]any); s["unchanged"].(float64) != 3 || pv["previous"].(float64) != 5 {
		t.Errorf("re-import: %v previous %v %v", s, pv["previous"], pv["rows"])
	}
	if s := pv["tests"].(map[string]any)["summary"].(map[string]any); s["unchanged"].(float64) != 2 {
		t.Errorf("re-import tests: %v %v", s, pv["tests"].(map[string]any)["rows"])
	}
	if r := pv["relations"].(map[string]any); r["keep"].(float64) != 4 || r["add"].(float64) != 0 {
		t.Errorf("re-import relations: %v", r)
	}
	for _, f := range pv["files"].([]any) {
		if !f.(map[string]any)["existing"].(bool) {
			t.Errorf("file not recognised: %v", f)
		}
	}

	// the type mapping: tests left out, the hierarchy as refines
	other := a.project("RTC")
	pv = a.upload("POST", "/api/v1/projects/"+other+"/exchange/reqif/preview", "file", "rta.reqifz", "", first,
		map[string]string{"object_types": `{"Test case":"ignore"}`, "relation_types": `{"Depends on":"relates_to"}`}, 200)
	if pv["tests"] != nil || pv["relations"].(map[string]any)["links"].(float64) != 0 {
		t.Errorf("tests ignored: %v %v", pv["tests"], pv["relations"])
	}
	for _, c := range pv["relations"].(map[string]any)["changes"].([]any) {
		if c := c.(map[string]any); c["file_type"] == "Depends on" && c["type"] != "relates_to" {
			t.Errorf("relation mapping: %v", c)
		}
	}
	a.upload("POST", "/api/v1/projects/"+other+"/exchange/reqif/preview", "file", "rta.reqifz", "", first, map[string]string{"object_types": `{"Test case":"nope"}`}, 400)
	a.upload("POST", "/api/v1/projects/"+other+"/exchange/reqif/preview", "file", "rta.reqifz", "", first, map[string]string{"relation_types": `{"x":"nope"}`}, 400)

	// a baseline export holds the frozen versions
	bl := a.m("POST", base+"/baselines", map[string]any{"name": "v1"}, 201)
	a.m("PATCH", "/api/v1/requirements/"+r3["id"].(string), map[string]any{"title": "Audit trail"}, 200)
	resp := a.raw("GET", base+"/export?format=reqif&include=tests,records&baseline="+url.QueryEscape(bl["id"].(string)), nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `THE-VALUE="Audit"`) || strings.Contains(string(body), "Audit trail") || !strings.Contains(string(body), "baseline: v1") {
		t.Errorf("baseline export: %d", resp.StatusCode)
	}
	a.m("GET", base+"/export?format=reqif&baseline=nope", nil, 400)
	a.m("GET", base+"/export?format=reqif&include=bogus", nil, 400)
}
