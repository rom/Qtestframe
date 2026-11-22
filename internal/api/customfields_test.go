package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func cfKeys(list []map[string]any) []string {
	var out []string
	for _, x := range list {
		out = append(out, x["key"].(string))
	}
	return out
}

func TestCustomFieldsEndToEnd(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("CF")
	pp := "/api/v1/projects/" + pid

	// definitions
	sup := a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Supplier", "type": "text", "show_in_list": true}, 201)
	if sup["key"] != "supplier" || sup["position"].(float64) != 0 {
		t.Fatal(sup)
	}
	mass := a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Mass", "type": "number", "unit": "kg", "required": true}, 201)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Coating", "type": "multi_select", "options": []string{"zinc", "paint", "none"}, "default": []string{"none"}}, 201)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Engineer", "type": "user"}, 201)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "test", "label": "Rig", "type": "single_select", "options": []string{"A", "B"}}, 201)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Supplier", "type": "text"}, 409)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Bad", "type": "single_select"}, 400)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "nothing", "label": "X", "type": "text"}, 400)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "X", "type": "number", "default": "abc"}, 400)
	if keys := cfKeys(items(a.m("GET", pp+"/custom-fields?entity_type=requirement", nil, 200))); strings.Join(keys, ",") != "supplier,mass,coating,engineer" {
		t.Fatal(keys)
	}
	a.m("GET", pp+"/custom-fields?entity_type=bogus", nil, 400)

	// values: validated on the server
	a.m("POST", pp+"/requirements", map[string]any{"title": "No mass"}, 400)
	a.m("POST", pp+"/requirements", map[string]any{"title": "Bad mass", "custom_fields": map[string]any{"mass": "heavy"}}, 400)
	a.m("POST", pp+"/requirements", map[string]any{"title": "Unknown", "custom_fields": map[string]any{"mass": 1, "colour": "red"}}, 400)
	a.m("POST", pp+"/requirements", map[string]any{"title": "Not a member", "custom_fields": map[string]any{"mass": 1, "engineer": "eve@example.com"}}, 400)
	r1 := a.m("POST", pp+"/requirements", map[string]any{"title": "Bracket", "custom_fields": map[string]any{"supplier": "Acme Ltd", "mass": "2,5", "engineer": "ADMIN@example.com"}}, 201)
	cf := r1["custom_fields"].(map[string]any)
	if cf["mass"] != 2.5 || cf["engineer"] != "admin@example.com" || cf["coating"].([]any)[0] != "none" {
		t.Fatal(cf)
	}
	r2 := a.m("POST", pp+"/requirements", map[string]any{"title": "Housing", "custom_fields": map[string]any{"supplier": "Other", "mass": 10, "coating": []string{"PAINT", "zinc"}}}, 201)
	if got := r2["custom_fields"].(map[string]any)["coating"].([]any); got[0] != "zinc" || got[1] != "paint" {
		t.Fatal(got) // option spelling and order
	}
	a.m("POST", pp+"/requirements", map[string]any{"title": "Light", "custom_fields": map[string]any{"mass": 0.5}}, 201)

	// query: filter and sort
	list := items(a.m("GET", pp+`/requirements?q=cf.supplier:"Acme"`, nil, 200))
	if len(list) != 1 || list[0]["title"] != "Bracket" {
		t.Fatal(list)
	}
	list = items(a.m("GET", pp+"/requirements?q=cf.mass>2", nil, 200))
	if len(list) != 2 {
		t.Fatal(list)
	}
	list = items(a.m("GET", pp+"/requirements?q=sort:-cf.mass", nil, 200))
	if list[0]["title"] != "Housing" || list[2]["title"] != "Light" {
		t.Fatal(list)
	}
	a.m("GET", pp+"/requirements?q=NOT+sort:title", nil, 400)

	// PATCH merges; null removes; the required field cannot be cleared
	id1 := r1["id"].(string)
	u := a.m("PATCH", "/api/v1/requirements/"+id1, map[string]any{"custom_fields": map[string]any{"supplier": nil}}, 200)
	if _, ok := u["custom_fields"].(map[string]any)["supplier"]; ok || u["custom_fields"].(map[string]any)["mass"] != 2.5 {
		t.Fatal(u["custom_fields"])
	}
	a.m("PATCH", "/api/v1/requirements/"+id1, map[string]any{"custom_fields": map[string]any{"mass": nil}}, 400)
	a.m("PATCH", "/api/v1/requirements/"+id1, map[string]any{"title": "Bracket A"}, 200)
	hist := items(a.m("GET", "/api/v1/requirements/"+id1+"/history", nil, 200))
	found := false
	for _, h := range hist {
		for _, f := range h["changed_fields"].([]any) {
			if f == "custom_fields.supplier" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("history: %v", hist)
	}

	// batch (spreadsheet) uses the same rules
	a.m("POST", pp+"/requirements/batch", map[string]any{"items": []map[string]any{{"id": id1, "set": map[string]any{"custom_fields": map[string]any{"mass": "x"}}}}}, 400)
	a.m("POST", pp+"/requirements/batch", map[string]any{"items": []map[string]any{{"id": id1, "set": map[string]any{"custom_fields": map[string]any{"supplier": "Batch Co"}}}}}, 200)

	// export: one column per field
	res, body := a.getBytes(pp+"/export?format=csv&aspects=requirements", 200)
	res.Body.Close()
	csv := string(body)
	if !strings.Contains(csv, "Supplier,Mass (kg),Coating,Engineer") || !strings.Contains(csv, "Batch Co,2.5,none,admin@example.com") {
		t.Fatal(csv)
	}
	_, body = a.getBytes(pp+"/export?format=reqif", 200)
	if x := string(body); !strings.Contains(x, `LONG-NAME="Mass (kg)"`) || !strings.Contains(x, `ATTRIBUTE-VALUE-REAL THE-VALUE="2.5"`) || !strings.Contains(x, `MULTI-VALUED="true"`) {
		t.Fatal(x)
	}

	// report fields and custom report columns
	var fields []map[string]any
	a.call("GET", pp+"/reports/fields?entity=requirement", nil, 200, &fields)
	if last := fields[len(fields)-1]; last["id"] != "cf.engineer" {
		t.Fatal(last)
	}
	_, rep := a.download(pp+"/reports/generate", map[string]any{"format": "csv", "custom": map[string]any{"entity": "requirement", "columns": []string{"key", "cf.mass"}, "sort_by": "cf.mass"}}, 200)
	if !strings.Contains(string(rep), "Mass (kg)") || !strings.Contains(string(rep), "0.5") {
		t.Fatal(string(rep))
	}

	// usage, confirmation, archive
	mid := mass["id"].(string)
	if u := a.m("GET", "/api/v1/custom-fields/"+mid+"/usage", nil, 200); u["items"].(float64) != 3 {
		t.Fatal(u)
	}
	if c := a.m("DELETE", "/api/v1/custom-fields/"+mid, nil, 409); c["code"] != "confirmation_required" || c["usage"].(float64) != 3 {
		t.Fatal(c)
	}
	arch := a.m("DELETE", "/api/v1/custom-fields/"+mid+"?confirm=true", nil, 200)
	if arch["item_count"].(float64) != 3 || arch["field_key"] != "mass" {
		t.Fatal(arch)
	}
	g := a.m("GET", "/api/v1/requirements/"+id1, nil, 200)
	if _, ok := g["custom_fields"].(map[string]any)["mass"]; ok {
		t.Fatal(g)
	}
	al := items(a.m("GET", pp+"/custom-fields/archive?values=true", nil, 200))
	if len(al) != 1 || len(al[0]["values"].([]any)) != 3 {
		t.Fatal(al)
	}
	// update: label, options; key and kind stay
	sid := sup["id"].(string)
	up := a.m("PATCH", "/api/v1/custom-fields/"+sid, map[string]any{"label": "Vendor", "key": "vendor", "entity_type": "test", "help": "Who makes it"}, 200)
	if up["key"] != "supplier" || up["entity_type"] != "requirement" || up["label"] != "Vendor" {
		t.Fatal(up)
	}
	// options are kept unless given, and replaced when given
	cid := items(a.m("GET", pp+"/custom-fields?entity_type=requirement", nil, 200))[1]["id"].(string)
	if c := a.m("PATCH", "/api/v1/custom-fields/"+cid, map[string]any{"position": 9}, 200); len(c["options"].([]any)) != 3 {
		t.Fatal(c)
	}
	if c := a.m("PATCH", "/api/v1/custom-fields/"+cid, map[string]any{"options": []string{"zinc", "none"}}, 200); len(c["options"].([]any)) != 2 {
		t.Fatal(c)
	}
	// type change only while no item has a value
	a.m("PATCH", "/api/v1/custom-fields/"+sid, map[string]any{"type": "number"}, 409)

	// a viewer reads but does not change definitions
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", pp+"/custom-fields", nil, 200)
	v.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Nope", "type": "text"}, 403)
	v.m("DELETE", "/api/v1/custom-fields/"+sid+"?confirm=true", nil, 403)

	// tests, defects, risks and runs
	tc := a.m("POST", pp+"/tests", map[string]any{"title": "Rig test", "custom_fields": map[string]any{"rig": "b"}}, 201)
	if tc["custom_fields"].(map[string]any)["rig"] != "B" {
		t.Fatal(tc)
	}
	if l := items(a.m("GET", pp+"/tests?q=cf.rig=B", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "defect", "label": "Found in", "type": "text"}, 201)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "risk", "label": "Reviewed", "type": "checkbox"}, 201)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "run", "label": "Lab", "type": "url"}, 201)
	d := a.m("POST", pp+"/defects", map[string]any{"title": "Crack", "custom_fields": map[string]any{"found_in": "v2"}}, 201)
	a.m("PATCH", "/api/v1/defects/"+d["id"].(string), map[string]any{"custom_fields": map[string]any{"found_in": "v3"}}, 200)
	if l := items(a.m("GET", pp+"/defects?q=cf.found_in:v3", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("POST", pp+"/risks", map[string]any{"title": "Fatigue", "likelihood": 2, "impact": 3, "custom_fields": map[string]any{"reviewed": true}}, 201)
	if l := items(a.m("GET", pp+"/risks?q=cf.reviewed=true", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("POST", pp+"/runs", map[string]any{"name": "R1", "custom_fields": map[string]any{"lab": "ftp://x"}}, 400)
	a.m("POST", pp+"/runs", map[string]any{"name": "R1", "custom_fields": map[string]any{"lab": "https://lab.example.org"}}, 201)
	if l := items(a.m("GET", pp+"/runs?q=cf.lab:lab.example", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	// a required test field: users must fill it, items the system creates need not
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "test", "label": "Bench", "type": "text", "required": true}, 201)
	a.m("POST", pp+"/tests", map[string]any{"title": "No bench"}, 400)
	lr := a.m("POST", pp+"/requirements", map[string]any{"title": "With a linked test", "create_test": true}, 201)
	if lr["linked_test"] == nil {
		t.Fatal(lr)
	}
	a.m("POST", "/api/v1/tests/"+tc["id"].(string)+"/copy", map[string]any{}, 201)
}

func TestFieldSetsAndTemplates(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	a.m("POST", "/api/v1/field-sets/hardware/fields", map[string]any{"entity_type": "requirement", "label": "Tolerance", "type": "number", "unit": "mm"}, 201)
	a.m("POST", "/api/v1/field-sets/hardware/fields", map[string]any{"entity_type": "test", "label": "Chamber", "type": "text"}, 201)
	a.m("POST", "/api/v1/field-sets/spaceship/fields", map[string]any{"entity_type": "test", "label": "X", "type": "text"}, 400)
	if l := items(a.m("GET", "/api/v1/field-sets?project_type=hardware", nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	// a new hardware project gets the set
	p := a.m("POST", "/api/v1/projects", map[string]any{"key": "HW", "name": "Hardware", "type": "hardware"}, 201)
	pp := "/api/v1/projects/" + p["id"].(string)
	if l := items(a.m("GET", pp+"/custom-fields", nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	// an existing project applies it; known keys are skipped
	gid := a.project("GEN")
	res := a.m("POST", "/api/v1/projects/"+gid+"/custom-fields/apply-set", map[string]any{"project_type": "hardware"}, 200)
	if len(res["added"].([]any)) != 2 {
		t.Fatal(res)
	}
	res = a.m("POST", "/api/v1/projects/"+gid+"/custom-fields/apply-set", map[string]any{"project_type": "hardware"}, 200)
	if len(res["added"].([]any)) != 0 || len(res["skipped"].([]any)) != 2 {
		t.Fatal(res)
	}
	fs := items(a.m("GET", "/api/v1/field-sets", nil, 200))
	a.m("PATCH", "/api/v1/field-sets/fields/"+fs[0]["id"].(string), map[string]any{"help": "± value"}, 200)
	a.m("DELETE", "/api/v1/field-sets/fields/"+fs[0]["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/field-sets/fields/"+fs[0]["id"].(string), nil, 404)

	// a tester can read but not change field sets
	e.member("tess@example.com", rbac.RoleTester)
	tst := e.login("tess@example.com")
	tst.m("GET", "/api/v1/field-sets", nil, 200)
	tst.m("POST", "/api/v1/field-sets/hardware/fields", map[string]any{"entity_type": "test", "label": "Y", "type": "text"}, 403)

	// templates: built-in per type and language
	tl := a.m("GET", pp+"/templates?kind=test&lang=sv", nil, 200)
	var names []string
	for _, x := range tl["items"].([]any) {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	if !strings.Contains(strings.Join(names, "|"), "Miljötest (enligt IEC 60068)") || !strings.Contains(strings.Join(names, "|"), "EMC-test") {
		t.Fatal(names)
	}
	// project and organisation templates
	own := a.m("POST", pp+"/templates", map[string]any{"kind": "requirement", "name": "Mechanical", "content": map[string]any{
		"title": "Part shall", "priority": "high", "tags": []string{"mech"}, "criteria": []map[string]any{{"description": "Measured"}},
		"custom_fields": map[string]any{"tolerance": 0.1}}}, 201)
	if own["source"] != "project" || own["content"].(map[string]any)["criteria"].([]any)[0].(map[string]any)["method"] != "test" {
		t.Fatal(own)
	}
	a.m("POST", pp+"/templates", map[string]any{"kind": "requirement", "name": "Bad", "content": map[string]any{"priority": "urgent"}}, 400)
	a.m("POST", pp+"/templates", map[string]any{"kind": "run", "name": "Bad"}, 400)
	org := a.m("POST", "/api/v1/template-sets/hardware/templates", map[string]any{"kind": "test", "name": "Vibration", "content": map[string]any{"steps": []map[string]any{{"action": "Mount", "expected": "Fixed"}}}}, 201)
	if org["source"] != "organisation" {
		t.Fatal(org)
	}
	tl = a.m("GET", pp+"/templates", nil, 200)
	srcs := map[string]int{}
	for _, x := range tl["items"].([]any) {
		srcs[x.(map[string]any)["source"].(string)]++
	}
	if srcs["project"] != 1 || srcs["organisation"] != 1 || srcs["builtin"] != 3 {
		t.Fatal(srcs)
	}
	// defaults
	a.m("PUT", pp+"/templates/default", map[string]any{"kind": "requirement", "template_id": "builtin:software:user-story"}, 400)
	a.m("PUT", pp+"/templates/default", map[string]any{"kind": "test", "template_id": own["id"]}, 400)
	d := a.m("PUT", pp+"/templates/default", map[string]any{"kind": "test", "template_id": "builtin:hardware:emc"}, 200)
	if d["defaults"].(map[string]any)["test"] != "builtin:hardware:emc" {
		t.Fatal(d)
	}
	a.m("PUT", pp+"/templates/default", map[string]any{"kind": "requirement", "template_id": own["id"]}, 200)
	tl = a.m("GET", pp+"/templates?kind=requirement", nil, 200)
	for _, x := range tl["items"].([]any) {
		m := x.(map[string]any)
		if (m["id"] == own["id"]) != (m["default"] == true) {
			t.Fatal(m)
		}
	}
	upd := a.m("PATCH", "/api/v1/templates/"+own["id"].(string), map[string]any{"name": "Mechanical part", "content": map[string]any{"title": "New"}}, 200)
	if upd["content"].(map[string]any)["priority"] != nil || upd["name"] != "Mechanical part" {
		t.Fatal(upd) // content is replaced as a whole
	}
	a.m("DELETE", "/api/v1/templates/"+own["id"].(string), nil, 204)
	if d := a.m("GET", pp+"/templates", nil, 200)["defaults"].(map[string]any); d["requirement"] != nil {
		t.Fatal(d) // the default went with the template
	}
	a.m("PATCH", "/api/v1/template-sets/templates/"+org["id"].(string), map[string]any{"description": "IEC 60068-2-6"}, 200)
	sets := items(a.m("GET", "/api/v1/template-sets?project_type=hardware&kind=test", nil, 200))
	if len(sets) != 3 {
		t.Fatal(sets)
	}
	a.m("GET", "/api/v1/template-sets?project_type=nope", nil, 400)
	a.m("DELETE", "/api/v1/template-sets/templates/"+org["id"].(string), nil, 204)
	tst.m("POST", pp+"/templates", map[string]any{"kind": "test", "name": "T"}, 403)
}

func TestCustomFieldsImport(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("IMP")
	pp := "/api/v1/projects/" + pid
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Mass", "type": "number", "unit": "kg", "required": true}, 201)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Supplier", "type": "single_select", "options": []string{"Acme", "Other"}}, 201)
	csv := "Title,Mass (kg),Supplier\nFirst,1.5 kg,acme\nSecond,,Nobody\n"
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "reqs.csv")
	_, _ = fw.Write([]byte(csv))
	mw.Close()
	req, _ := http.NewRequest("POST", e.hs.URL+pp+"/requirements/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", a.csrf)
	res, err := a.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 201 {
		t.Fatal(res.StatusCode)
	}
	list := items(a.m("GET", pp+"/requirements?q=sort:title", nil, 200))
	if len(list) != 2 {
		t.Fatal(list)
	}
	first := list[0]["custom_fields"].(map[string]any)
	if first["mass"] != 1.5 || first["supplier"] != "Acme" {
		t.Fatal(first)
	}
	if _, ok := list[1]["custom_fields"]; ok {
		t.Fatal(list[1]) // the invalid option is left out; the required mass may stay empty on import
	}
}
