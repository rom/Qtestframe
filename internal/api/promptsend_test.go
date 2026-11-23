package api

import (
	"strings"
	"testing"
)

func TestPromptSend(t *testing.T) {
	// without the AI assistance: 409
	e0 := newEnv(t)
	a0 := e0.login("admin@example.com")
	pid0 := a0.project("NOAI")
	a0.m("PATCH", "/api/v1/projects/"+pid0, map[string]any{"type": "software"}, 200)
	if r := a0.m("POST", "/api/v1/projects/"+pid0+"/prompts/send", map[string]any{"kind": "architecture"}, 409); r["code"] != "ai_unavailable" {
		t.Fatal(r)
	}

	f := &fakeLLM{}
	_, a, pid := aiEnv(t, f, nil)
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"type": "software"}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/prompts/send", map[string]any{"kind": "architecture"}, 409) // the organisation has not turned it on
	enableOrgAI(a, true)
	a.m("PUT", "/api/v1/projects/"+pid+"/solution-preamble", map[string]any{"languages": "Go 1.26", "databases": "postgres://app:" + "Hun" + "ter-22@db/app"}, 200)
	f1 := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"name": "Accounts", "kind": "requirement"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in.", "folder_id": f1["id"], "create_test": false}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Logout", "description": "The user MAY log out.", "create_test": false}, 201)

	// JSON
	res := a.m("POST", "/api/v1/projects/"+pid+"/prompts/send", map[string]any{"kind": "architecture"}, 200)
	if res["answer"] != "OK" || res["kind"] != "architecture" || res["meta"].(map[string]any)["feature"] != "prompt" {
		t.Fatal(res)
	}
	if len(f.users) != 1 || !strings.Contains(f.users[0], "## Solution preamble") || strings.Contains(f.users[0], "ter-22") || !strings.Contains(f.systems[0], "Markdown") {
		t.Fatal(f.users)
	}
	// streamed, the second part of a split prompt
	c := *a
	c.headers = map[string]string{"Accept": "text/event-stream"}
	ev := sse(t, c.raw("POST", "/api/v1/projects/"+pid+"/prompts/send", map[string]any{"kind": "design", "split": "folder", "part": 2}))
	if len(ev["start"]) != 1 || len(ev["delta"]) == 0 || len(ev["result"]) != 1 || ev["result"][0]["answer"] != "OK" {
		t.Fatal(ev)
	}
	if !strings.Contains(f.users[1], "part 2 of 2") || !strings.Contains(f.users[1], "Logout") || strings.Contains(f.users[1], "### AI-REQ-1 Login") {
		t.Fatal(f.users[1])
	}
	// validation before the call; bad prompts are refused too
	a.m("POST", "/api/v1/projects/"+pid+"/prompts/send", map[string]any{"kind": "design", "split": "folder", "part": 3}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/prompts/send", map[string]any{"kind": "nope"}, 400)
	if len(f.users) != 2 {
		t.Fatalf("%d calls", len(f.users))
	}
	// the audit entry names the kind, never the content
	audit := items(a.m("GET", "/api/v1/audit?action=ai.request", nil, 200))
	if len(audit) == 0 || !strings.Contains(audit[0]["details"].(string), "feature=prompt kind=design status=ok") || strings.Contains(audit[0]["details"].(string), "Logout") {
		t.Fatal(audit)
	}
	// a project that opted out
	_, a2, pid2 := aiEnv(t, &fakeLLM{}, nil)
	enableOrgAI(a2, true)
	a2.m("PATCH", "/api/v1/projects/"+pid2, map[string]any{"type": "software"}, 200)
	a2.m("PUT", "/api/v1/projects/"+pid2+"/ai", map[string]any{"disabled": true}, 200)
	a2.m("POST", "/api/v1/projects/"+pid2+"/prompts/send", map[string]any{"kind": "architecture"}, 409)
}
