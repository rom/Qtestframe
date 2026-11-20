package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func TestAtomicRequests(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ATX")
	var hits atomic.Int32
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(204)
	}))
	defer hook.Close()
	a.m("POST", "/api/v1/projects/"+pid+"/webhooks", map[string]any{"name": "h", "url": hook.URL, "events": []string{"defect.updated"}, "enabled": true}, 201)

	p := &Principal{User: e.admin, Tenant: e.tenant, Role: rbac.RoleAdmin}
	n := 0
	// a handler that writes a project and fires a webhook, then answers status
	handler := func(status int, panics bool) handlerFunc {
		return e.srv.atomic(func(w http.ResponseWriter, r *http.Request, p *Principal) {
			n++
			key := "TX" + string(rune('A'+n))
			if err := e.st.CreateProject(r.Context(), e.tenant.ID, &model.Project{Key: key, Name: key}, false); err != nil {
				t.Fatal(err)
			}
			e.srv.notifyEvent(r, p, pid, "defect.updated", map[string]any{"n": n})
			if panics {
				panic("boom")
			}
			if status >= 400 {
				writeError(w, status, "failed", "boom")
				return
			}
			w.Header().Set("X-Test", "yes")
			writeJSON(w, status, map[string]int{"n": n})
		})
	}
	projects := func() int {
		ps, err := e.st.ListProjects(t.Context(), e.tenant.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(ps)
	}
	call := func(h handlerFunc) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("POST", "/x", nil), p)
		e.srv.WaitWebhooks()
		return rec
	}
	before := projects()

	// an error answer saves nothing and fires no webhook, but is sent
	if rec := call(handler(409, false)); rec.Code != 409 || projects() != before || hits.Load() != 0 {
		t.Fatal(rec.Code, projects(), hits.Load())
	}
	// a panic saves nothing either
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("no panic")
			}
		}()
		call(handler(200, true))
	}()
	if projects() != before || hits.Load() != 0 {
		t.Fatal(projects(), hits.Load())
	}
	// success commits, fires the webhook afterwards and sends the held answer
	rec := call(handler(200, false))
	if rec.Code != 200 || rec.Header().Get("X-Test") != "yes" || rec.Body.Len() == 0 || projects() != before+1 || hits.Load() != 1 {
		t.Fatal(rec.Code, rec.Header(), rec.Body.String(), projects(), hits.Load())
	}
	// a handler that writes nothing answers 200
	empty := e.srv.atomic(func(w http.ResponseWriter, r *http.Request, p *Principal) {})
	if rec := call(empty); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
}

// A bulk request that fails part-way saves none of its changes.
func TestBulkRequestIsAllOrNothing(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("AON")
	other := a.project("AOO")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	foreign := a.m("POST", "/api/v1/projects/"+other+"/requirements", map[string]any{"title": "F"}, 201)["id"].(string)
	k1 := a.m("POST", "/api/v1/projects/"+pid+"/risks", map[string]any{"title": "K1", "likelihood": 2, "impact": 2}, 201)["id"].(string)
	k2 := a.m("POST", "/api/v1/projects/"+pid+"/risks", map[string]any{"title": "K2", "likelihood": 2, "impact": 2}, 201)["id"].(string)
	// the first risk is changed, then linking a requirement of another
	// project fails: the change of the first risk is undone too
	a.m("POST", "/api/v1/projects/"+pid+"/risks/bulk", map[string]any{"ids": []string{k1, k2}, "set": map[string]any{"status": "mitigating"}, "add_requirement_ids": []string{r1, foreign}}, 400)
	for _, id := range []string{k1, k2} {
		k := a.m("GET", "/api/v1/risks/"+id, nil, 200)
		if k["status"] == "mitigating" {
			t.Fatal("partly saved", k)
		}
	}
}
