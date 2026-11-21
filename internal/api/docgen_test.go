package api

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// fileContent downloads a file of the archive.
func (c *client) fileContent(id string) string {
	c.e.t.Helper()
	res := c.raw("GET", "/api/v1/files/"+id+"/content", nil)
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		c.e.t.Fatalf("download %s: %d %s", id, res.StatusCode, b)
	}
	return string(b)
}

func tagsOf(f map[string]any) []string {
	var out []string
	for _, t := range f["tags"].([]any) {
		out = append(out, t.(string))
	}
	return out
}

func TestGenerateRequirementsSpec(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("GEN")
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"description": "The shop of the future."}, 200)
	fold := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Checkout", "description": "Paying."}, 201)
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Pay by card", "description": "The system MUST accept cards.",
		"folder_id": fold["id"], "status": "approved", "rationale": "Customers want it", "criteria": []map[string]any{{"description": "Visa works", "method": "test"}}}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Fax orders", "status": "rejected"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Receipt", "status": "draft", "tags": []string{"mail"}}, 201)
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Card payment"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r1["id"], "test_case_id": tc["id"]}, 201)
	path := "/api/v1/projects/" + pid + "/documents/generate"

	f := a.m("POST", path, map[string]any{"kind": "requirements_spec", "format": "md", "options": map[string]any{"fields": []string{"description", "tests", "results", "criteria", "rationale"}}}, 201)
	if f["kind"] != "document" || f["name"] != "GEN-requirements-specification-v1.0.md" || !strings.HasPrefix(f["content_type"].(string), "text/markdown") {
		t.Fatal(f)
	}
	if tags := tagsOf(f); !slices.Equal(tags, []string{"generated", "requirements-specification", "v1.0"}) {
		t.Fatal(tags)
	}
	if f["description"] != "Requirements specification · Version 1.0" {
		t.Fatal(f["description"])
	}
	md := a.fileContent(f["id"].(string))
	for _, want := range []string{"# Requirements specification", "The shop of the future.", "### 2.1 Without folder", "### 2.2 Checkout", "Paying.",
		"#### GEN-REQ-1 Pay by card", "The system **MUST** accept cards.", "**Verified by:** " + tc["key"].(string) + " (Not run)", "Visa works",
		"Customers want it", "#### GEN-REQ-3 Receipt", "Appendix A. Traceability matrix", "](http://example.test/go/GEN-REQ-1)"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	if strings.Contains(md, "Fax orders") {
		t.Error("rejected requirements are left out by default")
	}

	// the second version: number and revision history; filter, inactive items, Swedish, PDF
	f2 := a.m("POST", path, map[string]any{"kind": "requirements_spec", "format": "docx", "options": map[string]any{"include_inactive": true, "lang": "sv", "query": "status:rejected"}}, 201)
	if f2["name"] != "GEN-requirements-specification-v2.0.docx" || f2["description"] != "Kravspecifikation · Version 2.0" {
		t.Fatal(f2)
	}
	docx := zipFiles(t, []byte(a.fileContent(f2["id"].(string))))
	body := docx["word/document.xml"]
	for _, want := range []string{"Kravspecifikation", "Fax orders", "GEN-requirements-specification-v1.0.md", "Revisionshistorik", "TOC \\o"} {
		if !strings.Contains(body, want) {
			t.Errorf("docx: missing %q", want)
		}
	}
	if strings.Contains(body, "Pay by card") {
		t.Error("the filter applies")
	}
	f3 := a.m("POST", path, map[string]any{"kind": "requirements_spec", "name": "My spec", "description": "For the review", "options": map[string]any{"version": "draft", "title": "Shop SRS", "author": "Ann"}}, 201)
	if f3["name"] != "My spec.pdf" || f3["description"] != "For the review" || slices.Contains(tagsOf(f3), "vdraft") {
		t.Fatal(f3)
	}
	if pdf := a.fileContent(f3["id"].(string)); !strings.HasPrefix(pdf, "%PDF") {
		t.Fatal("not a PDF")
	}
	// the archive lists them; the audit log records them
	docs := items(a.m("GET", "/api/v1/projects/"+pid+"/documents?q=tag:generated", nil, 200))
	if len(docs) != 3 {
		t.Fatalf("%d generated documents", len(docs))
	}
	found := false
	for _, x := range items(a.m("GET", "/api/v1/audit", nil, 200)) {
		if x["action"] == "document.generate" && strings.Contains(fmt.Sprint(x["details"]), "requirements_spec md") {
			found = true
		}
	}
	if !found {
		t.Error("no audit entry")
	}

	// errors
	a.m("POST", path, map[string]any{"kind": "nope"}, 400)
	a.m("POST", path, map[string]any{"kind": "requirements_spec", "format": "xlsx"}, 400)
	a.m("POST", path, map[string]any{"kind": "requirements_spec", "format": "rtf"}, 400)
	a.m("POST", path, map[string]any{"kind": "requirements_spec", "options": map[string]any{"query": "nosuchfield:x"}}, 400)
	a.m("POST", path, map[string]any{"kind": "requirements_spec", "options": map[string]any{"fields": []string{"steps"}}}, 400)
	a.m("POST", path, map[string]any{"kind": "requirements_spec", "options": map[string]any{"group_by": "colour"}}, 400)
	a.m("POST", path, map[string]any{"kind": "requirements_spec", "options": map[string]any{"baseline_id": "nope"}}, 400)

	// permissions: reading is not enough, the document is saved in the archive
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("POST", path, map[string]any{"kind": "requirements_spec"}, 403)
	e.member("tess@example.com", "tester")
	ts := e.login("tess@example.com")
	ts.m("POST", path, map[string]any{"kind": "test_plan", "format": "md"}, 201)
	// a role that may write requirements but not read tests gets no test plan
	a.m("POST", "/api/v1/roles", map[string]any{"name": "author", "permissions": []string{"project.read", "requirement.read", "requirement.write", "export"}}, 201)
	e.member("author@example.com", "author")
	au := e.login("author@example.com")
	au.m("POST", path, map[string]any{"kind": "requirements_spec", "format": "md"}, 201)
	au.m("POST", path, map[string]any{"kind": "test_plan", "format": "md"}, 403)
	// without export
	a.m("POST", "/api/v1/roles", map[string]any{"name": "noexport", "permissions": []string{"project.read", "requirement.read", "requirement.write"}}, 201)
	e.member("ne@example.com", "noexport")
	e.login("ne@example.com").m("POST", path, map[string]any{"kind": "requirements_spec"}, 403)
}

func TestGenerateTestPlanAndBaseline(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	pid := fx.pid
	path := "/api/v1/projects/" + pid + "/documents/generate"
	a.m("POST", "/api/v1/projects/"+pid+"/configurations", map[string]any{"name": "Chrome", "values": []map[string]any{{"dimension": "Browser", "value": "Chrome"}}}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "R1", "version": "1.0", "target_date": "2026-12-24"}, 201)

	f := a.m("POST", path, map[string]any{"kind": "test_plan", "format": "md", "options": map[string]any{"fields": []string{"steps", "requirements", "results", "execution_type"}}}, 201)
	if f["name"] != "RPT-test-plan-v1.0.md" || !slices.Contains(tagsOf(f), "test-plan") {
		t.Fatal(f)
	}
	md := a.fileContent(f["id"].(string))
	for _, want := range []string{"# Test plan", "### 3.2 Entry criteria", "### 3.3 Exit criteria", "**R1 1.0**", "| Chrome | Browser: Chrome |",
		"| staging | 1.0 | 1 |", "| R1 | 1.0 | Planned | 2026-12-24 |", "| Sprint 1 | Completed |", "## 7 Test specification", "### 7.1 Without folder", "### 7.2 Suite",
		"Valid login", "**Latest result:** Failed", "Call RPT-TC-1 Valid login", "| 2 | type wrong | | error |", "**Verifies:** RPT-REQ-1",
		"## Appendix A. Coverage", "### A.2 Requirements without tests", "[RPT-REQ-3](http://example.test/go/RPT-REQ-3) | Audit | Approved |"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	// filter on tests
	f = a.m("POST", path, map[string]any{"kind": "test_plan", "format": "md", "options": map[string]any{"query": "execution_type:automated"}}, 201)
	md = a.fileContent(f["id"].(string))
	if strings.Contains(md, "#### RPT-TC-1 Valid login") || !strings.Contains(md, "Wrong password") || !strings.Contains(md, "| 1.0 |") {
		t.Errorf("filtered plan:\n%s", md)
	}
	f = a.m("POST", path, map[string]any{"kind": "test_plan", "format": "pdf"}, 201)
	if pdf := a.fileContent(f["id"].(string)); !strings.HasPrefix(pdf, "%PDF") || f["content_type"] != "application/pdf" {
		t.Fatal("not a PDF")
	}

	// from a baseline: the frozen texts, its signatures
	bl := a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "BL-1"}, 201)
	a.m("POST", "/api/v1/baselines/"+bl["id"].(string)+"/signoffs", map[string]any{"meaning": "approved", "password": testPassword}, 201)
	reqs := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements", nil, 200))
	var login map[string]any
	for _, r := range reqs {
		if r["title"] == "Login" {
			login = r
		}
	}
	a.m("PATCH", "/api/v1/requirements/"+login["id"].(string), map[string]any{"title": "Sign in", "version": login["version"]}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Added later"}, 201)
	f = a.m("POST", path, map[string]any{"kind": "requirements_spec", "format": "md", "options": map[string]any{"baseline_id": bl["id"]}}, 201)
	md = a.fileContent(f["id"].(string))
	for _, want := range []string{"RPT-REQ-1 Login", "Baseline BL-1 of", "### Approvals", "| approved |", "RPT-TC-1"} {
		if !strings.Contains(md, want) {
			t.Errorf("baseline: missing %q in\n%s", want, md)
		}
	}
	if strings.Contains(md, "Sign in") || strings.Contains(md, "Added later") {
		t.Error("the baseline's frozen content is used")
	}
	if !strings.Contains(f["description"].(string), "Baseline BL-1") {
		t.Error(f["description"])
	}
	f = a.m("POST", path, map[string]any{"kind": "test_plan", "format": "md", "options": map[string]any{"baseline_id": bl["id"]}}, 201)
	if md = a.fileContent(f["id"].(string)); !strings.Contains(md, "Wrong password") {
		t.Error("test plan from a baseline")
	}
	// a baseline of another project
	other := a.m("POST", "/api/v1/projects/"+fx.otherPID+"/baselines", map[string]any{"name": "X"}, 201)
	a.m("POST", path, map[string]any{"kind": "requirements_spec", "options": map[string]any{"baseline_id": other["id"]}}, 400)
}

// TestGenerateLargeProject: a specification and a test plan of 2,000
// requirements and 2,000 tests are generated in reasonable time.
func TestGenerateLargeProject(t *testing.T) {
	if testing.Short() {
		t.Skip("large project")
	}
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BIG")
	ctx := context.Background()
	const n = 2000
	err := e.st.InTx(ctx, func(ctx context.Context) error {
		var folders []string
		for i := 0; i < 10; i++ {
			for _, kind := range []string{model.KindRequirement, model.KindTest} {
				f := &model.Folder{ProjectID: pid, Kind: kind, Name: fmt.Sprintf("%s area %d", kind, i)}
				if err := e.st.CreateFolder(ctx, e.tenant.ID, f); err != nil {
					return err
				}
				folders = append(folders, f.ID)
			}
		}
		for i := 0; i < n; i++ {
			rf, tf := folders[(i%10)*2], folders[(i%10)*2+1]
			r := &model.Requirement{ProjectID: pid, FolderID: &rf, Title: fmt.Sprintf("Requirement %d", i), Status: "approved", Obligation: "shall",
				Type: "functional", Priority: "medium", Description: "The system SHALL handle case " + fmt.Sprint(i) + " and SHOULD log it.", CreatedBy: "admin@example.com"}
			if err := e.st.CreateRequirement(ctx, e.tenant.ID, r); err != nil {
				return err
			}
			tc := &model.TestCase{ProjectID: pid, FolderID: &tf, Title: fmt.Sprintf("Test %d", i), ExecutionType: "manual", Status: "ready", Priority: "medium",
				Steps: []model.TestStep{{Action: "Do it", Expected: "Done"}, {Action: "Check", Data: "x", Expected: "Fine"}}, CreatedBy: "admin@example.com"}
			if err := e.st.CreateTest(ctx, e.tenant.ID, tc); err != nil {
				return err
			}
			if err := e.st.CreateLink(ctx, e.tenant.ID, &model.TraceLink{ProjectID: pid, RequirementID: r.ID, TestCaseID: tc.ID}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/projects/" + pid + "/documents/generate"
	for _, c := range []struct{ kind, format string }{{"requirements_spec", "pdf"}, {"test_plan", "docx"}, {"test_plan", "md"}} {
		start := time.Now()
		f := a.m("POST", path, map[string]any{"kind": c.kind, "format": c.format}, 201)
		took := time.Since(start)
		t.Logf("%s %s: %v, %v bytes", c.kind, c.format, took, f["size"])
		if took > 60*time.Second {
			t.Errorf("%s %s took %v", c.kind, c.format, took)
		}
		if size := f["size"].(float64); size < 100_000 {
			t.Errorf("%s %s: only %v bytes", c.kind, c.format, size)
		}
	}
}
