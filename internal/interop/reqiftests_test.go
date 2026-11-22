package interop

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/reqimport"
)

// TestReqIFPlanTests maps a DOORS Next document with test cases: the types,
// the test attributes (steps from a table), headings as folders of each
// kind, "Validated By" (requirement → test) as a trace link, nested
// requirements as refines.
func TestReqIFPlanTests(t *testing.T) {
	m, err := reqimport.ParseReqIFModel(mustRead(t, "testdata/dng-tests.reqif"))
	if err != nil {
		t.Fatal(err)
	}
	opt := ReqIFOptions{Hierarchy: true, Refines: true, TestFields: BuiltinFields(KindTest), Root: "DNG", TestRoot: "DNG tests"}
	p := PlanReqIF(m, BuiltinFields(KindRequirement), opt, nil)
	kinds := map[string]string{}
	for _, x := range p.Types {
		kinds[x.Name] = x.Kind
	}
	if kinds["Test Case"] != ObjTest || kinds["Stakeholder Requirement"] != ObjRequirement {
		t.Errorf("types: %v", kinds)
	}
	if len(p.Rows) != 2 || len(p.TestRows) != 1 || p.Folders != 1 || p.TestFolders != 1 {
		t.Fatalf("rows %d tests %d folders %d/%d", len(p.Rows), len(p.TestRows), p.Folders, p.TestFolders)
	}
	tr := p.TestRows[0]
	if !tr.HasSteps || len(tr.Steps) != 2 || tr.Steps[1].Action != "Brake at full force" || tr.Steps[1].Expected != "Stops within 12 m" {
		t.Errorf("steps: %+v", tr.Steps)
	}
	if tr.Cells["preconditions"].Raw != "Dry track, driver trained" || tr.Cells["folder"].Raw != "DNG tests/Acceptance tests" ||
		!strings.Contains(tr.Cells["description"].Raw, "![Track layout](files/track.png)") || tr.Key != "3101" {
		t.Errorf("test cells: %+v key %s", tr.Cells, tr.Key)
	}
	if r := p.Rows[0]; r.Cells["folder"].Raw != "DNG/Braking" || !strings.Contains(r.Cells["description"].Raw, "| 50 km/h | 12 m |") || r.Cells["priority"].Raw != "High" {
		t.Errorf("requirement cells: %+v", r.Cells)
	}
	var verifies, refines int
	for _, r := range p.Relations {
		switch r.Type {
		case RelVerifies:
			verifies++
			if r.Source != "_t1" || r.Target != "_r1" {
				t.Errorf("verifies not test → requirement: %+v", r)
			}
		case "refines":
			refines++
		}
	}
	if verifies != 1 || refines != 1 {
		t.Errorf("relations: %+v", p.Relations)
	}
	// the mappings: tests ignored, Satisfies as relates_to, no refines from the hierarchy
	opt.ObjectTypes = map[string]string{"Test Case": ObjIgnore}
	opt.RelationTypes = map[string]string{"Satisfies": "relates_to"}
	opt.Refines = false
	p = PlanReqIF(m, BuiltinFields(KindRequirement), opt, nil)
	if len(p.TestRows) != 0 || len(p.Relations) != 1 || p.Relations[0].Type != "relates_to" {
		t.Errorf("mapped: %d tests %+v", len(p.TestRows), p.Relations)
	}
	// with refines from the hierarchy as well
	opt.Refines = true
	p = PlanReqIF(m, BuiltinFields(KindRequirement), opt, nil)
	if len(p.Relations) != 2 || p.Relations[1].Ref != "hierarchy:_r2>_r1" {
		t.Errorf("hierarchy: %+v", p.Relations)
	}
	if err := CheckTypeMappings(map[string]string{"x": "bogus"}, nil); err == nil {
		t.Error("bad object kind accepted")
	}
	if err := CheckTypeMappings(nil, map[string]string{"x": "bogus"}); err == nil {
		t.Error("bad relation type accepted")
	}
	if err := CheckMapping(BuiltinFields(KindTest), map[string]string{"Steps": RoleSteps}); err != nil {
		t.Error(err)
	}
	for name, want := range map[string]string{"Validated By": RelVerifies, "Tested by": RelVerifies, "Verification record of": RelIgnore, "Satisfies": "refines"} {
		if got := GuessRelationMapping(name); got != want {
			t.Errorf("GuessRelationMapping(%q) = %s", name, got)
		}
	}
	if steps := reqimport.ParseStepsXHTML("<ul><li>Open → Opened</li><li>Close</li></ul>"); len(steps) != 2 || steps[0].Expected != "Opened" {
		t.Errorf("list steps: %+v", steps)
	}
	if steps := reqimport.ParseStepsXHTML("<table><tr><td>a</td><td>b</td><td>c</td></tr></table>"); len(steps) != 1 || steps[0].Data != "b" || steps[0].Expected != "c" {
		t.Errorf("headerless steps: %+v", steps)
	}
	refs := reqimport.EmbeddedRefs(`<p><object data="files/a.ole"><object data="files/a.png" type="image/png"/></object><img src="b.png"/><a href="https://x">x</a></p>`)
	if strings.Join(refs, ",") != "files/a.ole,files/a.png,b.png" {
		t.Errorf("refs: %v", refs)
	}
}
