package api

import (
	"context"
	"slices"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func workRow(t *testing.T, list []map[string]any, kind, id string) map[string]any {
	t.Helper()
	for _, it := range list {
		if it["kind"] == kind && it["id"] == id {
			return it
		}
	}
	return nil
}

func reasonsOf(it map[string]any) []string {
	var out []string
	for _, r := range it["reasons"].([]any) {
		out = append(out, r.(string))
	}
	return out
}

func workKeys(list []map[string]any) []string {
	var out []string
	for _, it := range list {
		out = append(out, it["key"].(string))
	}
	return out
}

func TestOwners(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	anna := e.member("anna@example.com", rbac.RoleTestLead)
	anna.DisplayName = "Anna Svensson"
	if err := e.st.UpdateUser(context.Background(), anna); err != nil {
		t.Fatal(err)
	}
	pid := a.project("OWN")
	base := "/api/v1/projects/" + pid

	// a member named by display name, handle or e-mail becomes the e-mail address
	rq := a.m("POST", base+"/requirements", map[string]any{"title": "Login", "owner": "Anna Svensson"}, 201)
	if rq["owner"] != "anna@example.com" {
		t.Fatal(rq["owner"])
	}
	rid := rq["id"].(string)
	if got := a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"owner": "External Consultant"}, 200); got["owner"] != "External Consultant" {
		t.Fatal(got["owner"])
	}
	if got := a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"owner": "@anna"}, 200); got["owner"] != "anna@example.com" {
		t.Fatal(got["owner"])
	}
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"owner": longText(201)}, 400)

	// tests have owners: create, patch, bulk; history records them
	tc := a.m("POST", base+"/tests", map[string]any{"title": "Login works", "owner": "ANNA@example.com"}, 201)
	tid := tc["id"].(string)
	if tc["owner"] != "anna@example.com" {
		t.Fatal(tc["owner"])
	}
	t2 := a.m("POST", base+"/tests", map[string]any{"title": "Logout works"}, 201)["id"].(string)
	a.m("POST", base+"/tests/bulk", map[string]any{"ids": []string{t2}, "set": map[string]any{"owner": "Anna Svensson"}}, 200)
	if got := a.m("GET", "/api/v1/tests/"+t2, nil, 200); got["owner"] != "anna@example.com" {
		t.Fatal(got["owner"])
	}
	a.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"owner": ""}, 200)
	revs := items(a.m("GET", "/api/v1/tests/"+tid+"/history", nil, 200))
	if len(revs) < 2 {
		t.Fatal(revs)
	}

	// owner:me and owner:<email> in the query language
	annaC := e.login("anna@example.com")
	got := items(annaC.m("GET", base+"/tests?q=owner:me", nil, 200))
	if len(got) != 1 || got[0]["id"] != t2 {
		t.Fatal(workKeys(got))
	}
	if got := items(a.m("GET", base+"/tests?q=owner:me", nil, 200)); len(got) != 0 {
		t.Fatal(workKeys(got))
	}
	if got := items(a.m("GET", base+"/requirements?q=owner:anna@example.com", nil, 200)); len(got) != 1 {
		t.Fatal(workKeys(got))
	}
	if got := items(annaC.m("GET", base+"/requirements?q=-owner:me", nil, 200)); len(got) != 0 {
		t.Fatal(workKeys(got))
	}

	// the new owner is told
	n := annaC.m("GET", "/api/v1/notifications", nil, 200)
	assigned := 0
	for _, x := range items(n) {
		if x["event"] == "assigned" {
			assigned++
		}
	}
	if assigned < 3 {
		t.Fatal(items(n))
	}

	// risks: owner and review owner normalised; bulk owner
	k := a.m("POST", base+"/risks", map[string]any{"title": "Data loss", "likelihood": 3, "impact": 4, "owner": "Anna Svensson", "review_owner": "@anna"}, 201)
	if k["owner"] != "anna@example.com" || k["review_owner"] != "anna@example.com" {
		t.Fatal(k)
	}
	k2 := a.m("POST", base+"/risks", map[string]any{"title": "Outage", "likelihood": 2, "impact": 2, "owner": "Ops team"}, 201)
	if k2["owner"] != "Ops team" {
		t.Fatal(k2["owner"])
	}
	a.m("POST", base+"/risks/bulk", map[string]any{"ids": []string{k2["id"].(string)}, "set": map[string]any{"owner": "anna@example.com"}}, 200)
	if got := items(annaC.m("GET", base+"/risks?q=owner:me", nil, 200)); len(got) != 2 {
		t.Fatal(workKeys(got))
	}

	// a member who may not see the project cannot own its items
	a.m("PUT", base+"/access", map[string]any{"access": "restricted"}, 200)
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"owner": "Anna Svensson"}, 400)

	// a changed e-mail address moves the references along
	if err := e.st.SetUserEmail(context.Background(), anna.ID, "anna.s@example.com"); err != nil {
		t.Fatal(err)
	}
	if got := a.m("GET", "/api/v1/tests/"+t2, nil, 200); got["owner"] != "anna.s@example.com" {
		t.Fatal(got["owner"])
	}
}

func longText(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

func TestMyWork(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("lead@example.com", rbac.RoleManager)
	lead := e.login("lead@example.com")
	pid := a.project("MW")
	other := a.project("OTH")
	base := "/api/v1/projects/" + pid

	// owned requirement in draft, owned verified requirement (final: not listed
	// unless flagged), owned test linked to it
	r1 := a.m("POST", base+"/requirements", map[string]any{"title": "Draft one", "owner": "lead@example.com"}, 201)["id"].(string)
	r2 := a.m("POST", base+"/requirements", map[string]any{"title": "Done one", "owner": "lead@example.com", "status": "rejected"}, 201)["id"].(string)
	t1 := a.m("POST", base+"/tests", map[string]any{"title": "Check", "owner": "lead@example.com", "status": "ready"}, 201)["id"].(string)
	a.m("POST", base+"/links", map[string]any{"requirement_id": r2, "test_case_id": t1}, 201)

	// a defect and an execution assigned, in another project too
	d := a.m("POST", base+"/defects", map[string]any{"title": "Crash", "assignee": "lead@example.com"}, 201)["id"].(string)
	a.m("POST", base+"/defects", map[string]any{"title": "Closed one", "assignee": "lead@example.com", "status": "closed"}, 201)
	ot := a.m("POST", "/api/v1/projects/"+other+"/tests", map[string]any{"title": "Other test"}, 201)["id"].(string)
	run := a.m("POST", "/api/v1/projects/"+other+"/runs", map[string]any{"name": "Sprint", "test_ids": []string{ot}}, 201)["id"].(string)
	ex := items(a.m("GET", "/api/v1/runs/"+run+"/executions", nil, 200))[0]["id"].(string)
	a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"assignee": "@lead"}, 200)

	// a review request with a due date in the past
	a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": r1, "reviewers": []string{"lead@example.com"}, "due_date": "2000-01-01"}, 201)

	// a waiver waiting for the lead's approval
	w := a.m("POST", base+"/waivers", map[string]any{"title": "Ship without", "requirement_ids": []string{r1}, "justification": "later",
		"approvers": []string{"lead@example.com"}, "submit": true}, 201)

	// a risk above the threshold, mitigated without an acceptance; a risk
	// review owned and overdue
	k := a.m("POST", base+"/risks", map[string]any{"title": "Breach", "likelihood": 5, "impact": 5, "status": "mitigated"}, 201)["id"].(string)
	k2 := a.m("POST", base+"/risks", map[string]any{"title": "Review me", "likelihood": 1, "impact": 1, "status": "mitigated",
		"review_owner": "lead@example.com", "review_date": "2000-01-01"}, 201)["id"].(string)

	res := lead.m("GET", "/api/v1/my-work", nil, 200)
	list := items(res)
	check := func(kind, id string, want ...string) map[string]any {
		t.Helper()
		it := workRow(t, list, kind, id)
		if it == nil {
			t.Fatalf("no %s %s in %v", kind, id, workKeys(list))
		}
		for _, r := range want {
			if !slices.Contains(reasonsOf(it), r) {
				t.Fatalf("%s: reasons %v, want %s", it["key"], it["reasons"], r)
			}
		}
		return it
	}
	check("requirement", r1, "owner", "review_requested", "overdue")
	check("defect", d, "assigned")
	if it := check("execution", ex, "assigned"); it["run_id"] != run || it["project_key"] != "OTH" || it["detail"] != "Sprint" {
		t.Fatal(it)
	}
	check("waiver", w["id"].(string), "waiver_approval")
	check("risk", k, "risk_acceptance")
	if it := check("risk", k2, "risk_review", "overdue"); it["overdue"] != true || it["due"] != "2000-01-01" {
		t.Fatal(it)
	}
	if it := workRow(t, list, "requirement", r2); it != nil {
		t.Fatal(it)
	}
	if workRow(t, list, "requirement", r2) != nil || workRow(t, list, "test", t1) != nil {
		t.Fatal("final items listed without a flag", workKeys(list))
	}
	if list[0]["overdue"] != true {
		t.Fatal("overdue rows come first", workKeys(list))
	}
	sum := res["summary"].(map[string]any)
	if sum["count"] != float64(len(list)) || sum["overdue"] != 2.0 || len(list) != 6 {
		t.Fatal(sum, workKeys(list))
	}

	// the requirement changes: its link to the test becomes suspect
	a.m("PATCH", "/api/v1/requirements/"+r2, map[string]any{"description": "changed text"}, 200)
	list = items(lead.m("GET", "/api/v1/my-work", nil, 200))
	check("test", t1, "suspect")

	// filters: project, source, reason; count
	if got := items(lead.m("GET", "/api/v1/my-work?project=OTH", nil, 200)); len(got) != 1 || got[0]["id"] != ex {
		t.Fatal(workKeys(got))
	}
	if got := items(lead.m("GET", "/api/v1/my-work?source=defects", nil, 200)); len(got) != 1 || got[0]["id"] != d {
		t.Fatal(workKeys(got))
	}
	if got := items(lead.m("GET", "/api/v1/my-work?reason=overdue", nil, 200)); len(got) != 2 {
		t.Fatal(workKeys(got))
	}
	if c := lead.m("GET", "/api/v1/my-work/count?project="+other, nil, 200); c["count"] != 1.0 {
		t.Fatal(c)
	}
	lead.m("GET", "/api/v1/my-work?project=NOPE", nil, 404)

	// the requester does not approve their own waiver; nothing assigned to the admin
	list = items(a.m("GET", "/api/v1/my-work", nil, 200))
	if workRow(t, list, "waiver", w["id"].(string)) != nil || workRow(t, list, "defect", d) != nil {
		t.Fatal(workKeys(list))
	}

	// a project the lead may no longer see drops out
	a.m("PUT", "/api/v1/projects/"+other+"/access", map[string]any{"access": "restricted"}, 200)
	if got := items(lead.m("GET", "/api/v1/my-work?source=executions", nil, 200)); len(got) != 0 {
		t.Fatal(workKeys(got))
	}
}

func TestMyWorkReviews(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("lead@example.com", rbac.RoleManager)
	e.member("rev@example.com", rbac.RoleReviewer)
	lead, rev := e.login("lead@example.com"), e.login("rev@example.com")
	pid := a.project("MR")
	base := "/api/v1/projects/" + pid

	// the lead writes a requirement, puts it in review and asks two reviewers
	rq := lead.m("POST", base+"/requirements", map[string]any{"title": "Needs eyes"}, 201)["id"].(string)
	lead.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"status": "review"}, 200)
	created := lead.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": rq, "reviewers": []string{"rev@example.com", "admin@example.com"}}, 201)
	var revID string
	for _, v := range items(created) {
		if v["reviewer_email"] == "rev@example.com" {
			revID = v["id"].(string)
		}
	}
	if revID == "" {
		t.Fatal(created)
	}
	ids := func(it map[string]any, field string) []string {
		var out []string
		if l, ok := it[field].([]any); ok {
			for _, x := range l {
				out = append(out, x.(string))
			}
		}
		return out
	}
	row := func(c *client, path string) (map[string]any, map[string]any) {
		t.Helper()
		res := c.m("GET", path, nil, 200)
		return workRow(t, items(res), "requirement", rq), res["summary"].(map[string]any)
	}

	// the reviewer: one row, to review and in review, with the request id
	it, sum := row(rev, "/api/v1/my-work")
	if it == nil || !slices.Contains(reasonsOf(it), "review_requested") || !slices.Contains(reasonsOf(it), "in_review") || !slices.Equal(ids(it, "review_ids"), []string{revID}) || sum["count"] != 1.0 {
		t.Fatal(it, sum)
	}

	// the requester: waiting for two reviews; listed, not counted
	it, sum = row(lead, "/api/v1/my-work")
	if it == nil || !slices.Contains(reasonsOf(it), "awaiting_review") || !slices.Contains(reasonsOf(it), "in_review") || len(ids(it, "requested_review_ids")) != 2 || it["status"] != "review" {
		t.Fatal(it)
	}
	if sum["count"] != 0.0 || sum["waiting"] != 1.0 {
		t.Fatal(sum)
	}

	// changes requested: back with the requester, counted
	rev.m("PATCH", "/api/v1/reviews/"+revID, map[string]any{"decision": "changes_requested", "comment": "Too vague"}, 200)
	it, sum = row(lead, "/api/v1/my-work?source=requested_reviews")
	if it == nil || !slices.Contains(reasonsOf(it), "review_returned") || !slices.Contains(reasonsOf(it), "awaiting_review") || sum["count"] != 1.0 || sum["waiting"] != 0.0 {
		t.Fatal(it, sum)
	}
	if it, _ := row(rev, "/api/v1/my-work?source=reviews"); it != nil {
		t.Fatal("decided review still listed", it)
	}

	// editing the item answers the requested changes
	lead.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"description": "Clearer now"}, 200)
	if it, _ := row(lead, "/api/v1/my-work?reason=review_returned"); it != nil {
		t.Fatal(it)
	}

	// an item in review only because the caller may review its kind is not
	// personal work (GET /reviews/in-review still lists it)
	other := a.m("POST", base+"/requirements", map[string]any{"title": "Someone else's"}, 201)["id"].(string)
	a.m("PATCH", "/api/v1/requirements/"+other, map[string]any{"status": "review"}, 200)
	if workRow(t, items(rev.m("GET", "/api/v1/my-work?source=in_review", nil, 200)), "requirement", other) != nil {
		t.Fatal("reviewer-only item listed")
	}
	found := false
	for _, x := range items(rev.m("GET", "/api/v1/reviews/in-review", nil, 200)) {
		found = found || x["entity_id"] == other
	}
	if !found {
		t.Fatal("in-review list lost the item")
	}

	// a project the reviewer may no longer see drops out
	a.m("PUT", base+"/access", map[string]any{"access": "restricted"}, 200)
	if got := items(rev.m("GET", "/api/v1/my-work", nil, 200)); len(got) != 0 {
		t.Fatal(workKeys(got))
	}
}
