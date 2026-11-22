package store_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestCustomFieldsStore(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "CF", Name: "Custom"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	u := &model.User{Email: "eng@example.com"}
	must(t, st.CreateUser(ctx, u))
	must(t, st.SetMembership(ctx, ten.ID, u.ID, "tester"))

	who := &model.CustomField{ProjectID: p.ID, EntityType: model.KindTest, Label: "Engineer", Type: model.FieldUser}
	must(t, st.CreateCustomField(ctx, ten.ID, who))
	env := &model.CustomField{ProjectID: p.ID, EntityType: model.KindTest, Label: "Rig", Type: model.FieldSingleSelect, Options: []string{"A", "B"}, Default: "A"}
	must(t, st.CreateCustomField(ctx, ten.ID, env))
	if env.Position != 1 {
		t.Fatal(env.Position)
	}
	if err := st.CreateCustomField(ctx, ten.ID, &model.CustomField{ProjectID: "nope", EntityType: model.KindTest, Label: "X", Type: "text"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}

	tc := &model.TestCase{ProjectID: p.ID, Title: "T", CustomFields: model.CustomValues{"engineer": "ENG@example.com"}}
	must(t, st.CreateTest(ctx, ten.ID, tc))
	got, err := st.GetTest(ctx, ten.ID, tc.ID)
	must(t, err)
	if got.CustomFields["engineer"] != "eng@example.com" || got.CustomFields["rig"] != "A" {
		t.Fatal(got.CustomFields)
	}
	if v, err := st.CheckCustomValues(ctx, ten.ID, p.ID, model.KindTest, model.CustomValues{"rig": "b"}); err != nil || v["rig"] != "B" {
		t.Fatal(v, err)
	}
	if _, err := st.CheckCustomValues(ctx, ten.ID, p.ID, model.KindTest, model.CustomValues{"engineer": "x@example.com"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	// an option removed later: the stored value stays until changed
	env.Options = []string{"B", "C"}
	env.Default = nil
	must(t, st.UpdateCustomField(ctx, ten.ID, env))
	got.Title = "T2"
	must(t, st.UpdateTest(ctx, ten.ID, got, false))
	// fields by kind, usage
	by, err := st.CustomFieldsByKind(ctx, ten.ID, p.ID)
	must(t, err)
	if len(by[model.KindTest]) != 2 || by[model.KindTest][0].Key != "engineer" {
		t.Fatal(by)
	}
	if n, err := st.CustomFieldUsage(ctx, ten.ID, env); err != nil || n != 1 {
		t.Fatal(n, err)
	}

	// field set of a project type, applied on request
	set := &model.CustomField{ProjectType: "hardware", EntityType: model.KindRequirement, Label: "Tolerance", Type: model.FieldNumber, Unit: "mm"}
	must(t, st.CreateFieldSetField(ctx, ten.ID, set))
	if err := st.CreateFieldSetField(ctx, ten.ID, &model.CustomField{ProjectType: "x", EntityType: model.KindRequirement, Label: "X", Type: "text"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	set.Label = "Tolerance ±"
	must(t, st.UpdateFieldSetField(ctx, ten.ID, set))
	added, skipped, err := st.ApplyFieldSet(ctx, ten.ID, p.ID, "hardware")
	if err != nil || len(added) != 1 || len(skipped) != 0 {
		t.Fatal(added, skipped, err)
	}

	// templates and the default, then a project archive keeps all of it
	tpl := &model.ItemTemplate{ProjectID: p.ID, Kind: model.KindTest, Name: "Rig test", Content: model.TemplateContent{Title: "On rig", Steps: []model.TestStep{{Action: "Mount"}}, CustomFields: model.CustomValues{"rig": "B"}}}
	must(t, st.CreateTemplate(ctx, ten.ID, tpl))
	must(t, st.SetTemplateDefault(ctx, ten.ID, p.ID, model.KindTest, tpl.ID))
	if err := st.SetTemplateDefault(ctx, ten.ID, p.ID, model.KindTest, "builtin:software:api-test"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err) // a software template in a general project
	}
	must(t, st.SetTemplateDefault(ctx, ten.ID, p.ID, model.KindRequirement, "builtin:general:requirement"))
	org := &model.ItemTemplate{ProjectType: "general", Kind: model.KindTest, Name: "Org test"}
	must(t, st.CreateTemplate(ctx, ten.ID, org))
	org.Description = "changed"
	must(t, st.UpdateTemplate(ctx, ten.ID, true, org))
	if l, err := st.ListTemplates(ctx, ten.ID, true, "general", model.KindTest); err != nil || len(l) != 1 || l[0].Description != "changed" {
		t.Fatal(l, err)
	}

	// deleting a field archives its values
	a, err := st.DeleteCustomField(ctx, ten.ID, who.ID, "admin@example.com")
	must(t, err)
	if a.ItemCount != 1 || a.Values[0].Value != "eng@example.com" {
		t.Fatal(a)
	}
	arch, err := st.ListCustomFieldArchive(ctx, ten.ID, p.ID, true)
	if err != nil || len(arch) != 1 || len(arch[0].Values) != 1 || arch[0].Definition.Key != "engineer" {
		t.Fatal(arch, err)
	}

	var buf bytes.Buffer
	_, err = st.ExportProject(ctx, ten.ID, p.ID, &buf, "test")
	must(t, err)
	res, err := st.ImportProject(ctx, ten.ID, bytes.NewReader(buf.Bytes()), int64(buf.Len()), store.ProjectImport{Key: "CF2", Name: "Copy"})
	must(t, err)
	np := res.Project.ID
	defs, err := st.ListCustomFields(ctx, ten.ID, np, "")
	must(t, err)
	if len(defs) != 2 {
		t.Fatal(defs)
	}
	tests, err := st.ListTests(ctx, ten.ID, np)
	must(t, err)
	if len(tests) != 1 || tests[0].CustomFields["rig"] != "A" {
		t.Fatal(tests)
	}
	dflt, err := st.TemplateDefaults(ctx, ten.ID, np)
	must(t, err)
	tpls, err := st.ListTemplates(ctx, ten.ID, false, np, "")
	must(t, err)
	if len(tpls) != 1 || dflt[model.KindTest] != tpls[0].ID || dflt[model.KindRequirement] != "builtin:general:requirement" {
		t.Fatal(dflt, tpls)
	}
	if arch, _ := st.ListCustomFieldArchive(ctx, ten.ID, np, true); len(arch) != 1 || arch[0].Values[0].ID != tests[0].ID {
		t.Fatal(arch) // the archived values point at the loaded items
	}

	// deleting a template clears the default; a field set field goes
	must(t, st.DeleteTemplate(ctx, ten.ID, false, tpl.ID))
	if d, _ := st.TemplateDefaults(ctx, ten.ID, p.ID); d[model.KindTest] != "" {
		t.Fatal(d)
	}
	if err := st.DeleteTemplate(ctx, ten.ID, false, tpl.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.DeleteFieldSetField(ctx, ten.ID, set.ID))
	if _, err := st.GetFieldSetField(ctx, ten.ID, set.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetCustomField(ctx, ten.ID, who.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetTemplate(ctx, ten.ID, false, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestValidateTemplate(t *testing.T) {
	bad := []model.ItemTemplate{
		{Kind: model.KindRequirement},
		{Kind: "run", Name: "x"},
		{Kind: model.KindRequirement, Name: "x", Content: model.TemplateContent{Obligation: "never"}},
		{Kind: model.KindRequirement, Name: "x", Content: model.TemplateContent{Criteria: []model.EvaluationCriterion{{Description: "d", Method: "guess"}}}},
		{Kind: model.KindTest, Name: "x", Content: model.TemplateContent{Status: "approved"}},
		{Kind: model.KindTest, Name: "x", Content: model.TemplateContent{EstimatedMin: -1}},
		{Kind: model.KindTest, Name: "x", Content: model.TemplateContent{CustomFields: model.CustomValues{"k": map[string]any{}}}},
	}
	for i, b := range bad {
		if err := store.ValidateTemplate(&b); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%d: %v", i, err)
		}
	}
	ok := model.ItemTemplate{Kind: model.KindTest, Name: " T ", Content: model.TemplateContent{Obligation: "must", Criteria: []model.EvaluationCriterion{{Description: "x"}},
		Steps: []model.TestStep{{ID: "s", Action: "a"}}, CustomFields: model.CustomValues{"a": nil, "b": "x"}}}
	must(t, store.ValidateTemplate(&ok))
	if ok.Name != "T" || ok.Content.Obligation != "" || ok.Content.Criteria != nil || ok.Content.Steps[0].ID != "" || len(ok.Content.CustomFields) != 1 {
		t.Fatalf("%+v", ok)
	}
}
