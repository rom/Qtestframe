package interop

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

// The D3FEND and EMB3D columns of the exchange: known ids are written,
// unknown ones left out with a row warning.
func TestFrameworkFields(t *testing.T) {
	for _, kind := range []string{KindRequirement, KindTest} {
		fs := BuiltinFields(kind)
		if FieldByHeader(fs, "D3FEND") == nil || FieldByHeader(fs, "EMB3D threats and mitigations") == nil || FieldByHeader(fs, "d3fend-tekniker") == nil {
			t.Fatalf("%s: no framework fields", kind)
		}
	}
	fs := BuiltinFields(KindRequirement)
	it := RequirementItem(&model.Requirement{ID: "a", Key: "P-REQ-1", Title: "One", Version: 2, D3fend: []string{"D3-MFA"}}, "")
	rows := []Incoming{
		{Row: 2, ID: "a", Cells: map[string]Cell{"d3fend": {Raw: "D3-MFA, D3-NOPE", Ref: "M2"}, "emb3d": {Raw: "TID-201"}}},
		{Row: 3, Cells: map[string]Cell{"title": {Raw: "New"}, "emb3d": {Raw: "xyz"}}},
	}
	d := Compare(KindRequirement, fs, rows, ItemsByID([]*Item{it}), nil)
	r := d.Rows[0]
	if r.Status != StatusChanged || len(r.Changes) != 1 || r.Values["emb3d"] != "TID-201" || r.Values["d3fend"] != "" {
		t.Fatalf("row: %+v", r)
	}
	if strings.Join(r.Warnings, "|") != "M2 D3FEND: unknown D3FEND technique D3-NOPE left out" {
		t.Fatal(r.Warnings)
	}
	if n := d.Rows[1]; n.Status != StatusNew || n.Values["emb3d"] != "" || len(n.Warnings) != 1 {
		t.Fatalf("new: %+v", n)
	}
	// the values go into the create / PATCH body
	body := map[string]any{}
	if err := FieldByKey(fs, "emb3d").Put(body, "TID-201, MID-002", nil); err != nil || len(body["emb3d"].([]string)) != 2 {
		t.Fatal(body, err)
	}
	if got := FieldByKey(BuiltinFields(KindTest), "d3fend").Get(TestItem(&model.TestCase{ID: "t", D3fend: []string{"D3-MFA", "D3-AA"}}, "")); got != "D3-MFA, D3-AA" {
		t.Fatal(got)
	}
}
