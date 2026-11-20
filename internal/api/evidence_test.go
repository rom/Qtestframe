package api

import (
	"testing"
)

func TestStepEvidence(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("EVD")
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Login", "steps": []map[string]any{{"action": "Open", "expected": "Form"}, {"action": "Sign in", "expected": "Dashboard"}}}, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{tc["id"].(string)}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	d := a.m("GET", "/api/v1/executions/"+ex["id"].(string), nil, 200)
	steps := d["step_results"].([]any)
	s1 := steps[0].(map[string]any)["id"].(string)

	// evidence text is stored with the step result
	upd := a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"step_results": []map[string]any{
		{"id": s1, "status": "passed", "actual": "Form shown", "evidence": "Screenshot attached; response time 120 ms"},
	}}, 200)
	if sr := upd["step_results"].([]any)[0].(map[string]any); sr["evidence"] != "Screenshot attached; response time 120 ms" || sr["attachment_count"].(float64) != 0 {
		t.Fatal(sr)
	}
	long := make([]byte, 20001)
	for i := range long {
		long[i] = 'x'
	}
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"step_results": []map[string]any{{"id": s1, "evidence": string(long)}}}, 400)

	// files are attachments of the step result
	f := a.upload("POST", "/api/v1/step-results/"+s1+"/attachments", "file", "screen.png", "image/png", pngBytes(t), nil, 201)
	list := items(a.m("GET", "/api/v1/step-results/"+s1+"/attachments", nil, 200))
	if len(list) != 1 || list[0]["id"] != f["id"] {
		t.Fatal(list)
	}
	d = a.m("GET", "/api/v1/executions/"+ex["id"].(string), nil, 200)
	if sr := d["step_results"].([]any)[0].(map[string]any); sr["attachment_count"].(float64) != 1 {
		t.Fatal(sr)
	}
	// the document archive links the file to the execution's step
	docs := items(a.m("GET", "/api/v1/projects/"+pid+"/documents?kind=attachment", nil, 200))
	if len(docs) != 1 {
		t.Fatal(docs)
	}
	link := docs[0]["links"].([]any)[0].(map[string]any)
	if link["entity_type"] != "step" || link["execution_id"] != ex["id"] || link["run_id"] != run["id"] || link["key"] != "EVD-TC-1" {
		t.Fatal(link)
	}
	a.m("GET", "/api/v1/step-results/missing/attachments", nil, 404)
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/step-results/"+s1+"/attachments", nil, 200)
	v.upload("POST", "/api/v1/step-results/"+s1+"/attachments", "file", "x.txt", "text/plain", []byte("x"), nil, 403)

	// deleting the run removes the evidence files
	a.call("DELETE", "/api/v1/runs/"+run["id"].(string), nil, 204, nil)
	a.m("GET", "/api/v1/files/"+f["id"].(string), nil, 404)
}
