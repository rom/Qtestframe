package customfield

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

func def(t, key string, opts ...string) model.CustomField {
	return model.CustomField{Key: key, Label: strings.ToUpper(key[:1]) + key[1:], Type: t, EntityType: model.KindRequirement, Options: opts}
}

func TestKeyFromLabel(t *testing.T) {
	for in, want := range map[string]string{
		"Mass (kg)": "mass_kg", "Leverantör": "leverantor", "  Supplier  ": "supplier", "1st run": "f_1st_run",
		"Ärende-nr": "arende_nr", "!!!": "f_", strings.Repeat("abc ", 20): "abc_abc_abc_abc_abc_abc_abc_abc_abc_abc",
	} {
		if got := KeyFromLabel(in); got != want {
			t.Errorf("KeyFromLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateDefinition(t *testing.T) {
	f := model.CustomField{Label: " Mass ", Type: model.FieldNumber, Unit: " kg ", EntityType: model.KindRequirement, Options: []string{"x"}, Default: "2,5"}
	if err := ValidateDefinition(&f); err != nil {
		t.Fatal(err)
	}
	if f.Key != "mass" || f.Unit != "kg" || len(f.Options) != 0 || f.Default != 2.5 {
		t.Fatalf("%+v", f)
	}
	s := model.CustomField{Label: "Coating", Type: model.FieldMultiSelect, EntityType: "run", Unit: "m", Options: []string{" zinc", "Zinc", "", "paint"}, Default: []any{"PAINT"}}
	if err := ValidateDefinition(&s); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Options, []string{"zinc", "paint"}) || s.Unit != "" || !reflect.DeepEqual(s.Default, []string{"paint"}) {
		t.Fatalf("%+v", s)
	}
	bad := []model.CustomField{
		{Label: "", Type: "text", EntityType: "test"},
		{Label: strings.Repeat("x", 101), Type: "text", EntityType: "test"},
		{Label: "x", Key: "Bad Key", Type: "text", EntityType: "test"},
		{Label: "x", Type: "text", EntityType: "nope"},
		{Label: "x", Type: "colour", EntityType: "test"},
		{Label: "x", Type: "single_select", EntityType: "test"},
		{Label: "x", Type: "single_select", EntityType: "test", Options: []string{strings.Repeat("o", 201)}},
		{Label: "x", Type: "number", EntityType: "test", Unit: strings.Repeat("u", 21)},
		{Label: "x", Type: "text", EntityType: "test", Help: strings.Repeat("h", 1001)},
		{Label: "x", Type: "date", EntityType: "test", Default: "tomorrow"},
		{Label: "x", Type: "user", EntityType: "test", Default: "a@example.com"},
	}
	many := model.CustomField{Label: "x", Type: "single_select", EntityType: "test"}
	for i := 0; i <= MaxOptions; i++ {
		many.Options = append(many.Options, strings.Repeat("o", i+1))
	}
	bad = append(bad, many)
	for i, b := range bad {
		if err := ValidateDefinition(&b); !errors.Is(err, ErrInvalid) {
			t.Errorf("%d: %+v: %v", i, b, err)
		}
	}
}

func TestNormalize(t *testing.T) {
	sel := def(model.FieldSingleSelect, "rig", "A", "B")
	multi := def(model.FieldMultiSelect, "coat", "zinc", "paint", "none")
	cases := []struct {
		f    model.CustomField
		in   any
		want any
		err  bool
	}{
		{def("text", "t"), "  hi ", "hi", false},
		{def("text", "t"), "  ", nil, false},
		{def("text", "t"), 3.0, nil, true},
		{def("text", "t"), strings.Repeat("x", MaxTextLen+1), nil, true},
		{def("long_text", "l"), "line\n  two ", "line\n  two ", false},
		{def("number", "n"), 2.0, 2.0, false},
		{def("number", "n"), 3, 3.0, false},
		{def("number", "n"), "1 200,5", 1200.5, false},
		{def("number", "n"), "", nil, false},
		{def("number", "n"), "x", nil, true},
		{def("number", "n"), true, nil, true},
		{def("number", "n"), "NaN", nil, true},
		{def("date", "d"), "2026-02-28", "2026-02-28", false},
		{def("date", "d"), "2026-02-28T22:00:00Z", "2026-02-28", false},
		{def("date", "d"), "2026-02-30", nil, true},
		{def("date", "d"), 5.0, nil, true},
		{def("date", "d"), "", nil, false},
		{sel, "b", "B", false},
		{sel, "C", nil, true},
		{sel, "", nil, false},
		{sel, 1.0, nil, true},
		{multi, []any{"PAINT", "zinc", "zinc"}, []string{"zinc", "paint"}, false},
		{multi, []string{"none"}, []string{"none"}, false},
		{multi, "paint; zinc", []string{"zinc", "paint"}, false},
		{multi, []any{}, nil, false},
		{multi, []any{1.0}, nil, true},
		{multi, []any{"gold"}, nil, true},
		{multi, 2.0, nil, true},
		{def("checkbox", "c"), true, true, false},
		{def("checkbox", "c"), "ja", true, false},
		{def("checkbox", "c"), "no", false, false},
		{def("checkbox", "c"), "maybe", nil, true},
		{def("checkbox", "c"), 1.0, nil, true},
		{def("user", "u"), " Anna@Example.com ", "anna@example.com", false},
		{def("user", "u"), "anna", nil, true},
		{def("user", "u"), "", nil, false},
		{def("user", "u"), 1.0, nil, true},
		{def("url", "w"), "https://example.org/x", "https://example.org/x", false},
		{def("url", "w"), "javascript:alert(1)", nil, true},
		{def("url", "w"), "/relative", nil, true},
		{def("url", "w"), "", nil, false},
		{def("url", "w"), 1.0, nil, true},
		{def("other", "o"), "x", nil, true},
		{def("text", "t"), nil, nil, false},
	}
	for i, c := range cases {
		got, err := Normalize(&c.f, c.in)
		if (err != nil) != c.err || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%d %s %v: got %#v, %v; want %#v (error %v)", i, c.f.Type, c.in, got, err, c.want, c.err)
		}
	}
}

func TestValidate(t *testing.T) {
	mass := def("number", "mass")
	mass.Required = true
	coat := def(model.FieldMultiSelect, "coat", "zinc", "none")
	coat.Default = []string{"none"}
	ok := def("checkbox", "ok")
	who := def("user", "who")
	defs := []model.CustomField{mass, coat, ok, who}
	isUser := func(e string) bool { return e == "anna@example.com" }

	// create: defaults, required
	got, err := Validate(defs, model.CustomValues{"mass": "2"}, Options{Create: true, IsUser: isUser})
	if err != nil || got["mass"] != 2.0 || !reflect.DeepEqual(got["coat"], []string{"none"}) {
		t.Fatal(got, err)
	}
	if _, err := Validate(defs, model.CustomValues{}, Options{Create: true}); err == nil {
		t.Fatal("required")
	}
	if got, err := Validate(defs, nil, Options{Create: true, Lenient: true}); err != nil || got["mass"] != nil {
		t.Fatal(got, err)
	}
	// an explicit empty value wins over the default
	if got, _ := Validate(defs, model.CustomValues{"mass": 1.0, "coat": nil}, Options{Create: true}); got["coat"] != nil {
		t.Fatal(got)
	}
	// an unticked optional checkbox is not stored
	if got, _ := Validate(defs, model.CustomValues{"mass": 1.0, "ok": false}, Options{}); got["ok"] != nil {
		t.Fatal(got)
	}
	// users
	if _, err := Validate(defs, model.CustomValues{"mass": 1.0, "who": "eve@example.com"}, Options{Create: true, IsUser: isUser}); err == nil {
		t.Fatal("not a member")
	}
	old := model.CustomValues{"mass": 1.0, "who": "eve@example.com", "legacy": "kept", "coat": []string{"gold"}}
	// update: unchanged values that no longer fit are kept
	got, err = Validate(defs, model.CustomValues{"mass": 1.0, "who": "eve@example.com", "legacy": "kept", "coat": []any{"gold"}}, Options{Old: old, IsUser: isUser})
	if err != nil || got["legacy"] != "kept" || got["who"] != "eve@example.com" {
		t.Fatal(got, err)
	}
	if _, err := Validate(defs, model.CustomValues{"mass": 1.0, "legacy": "changed"}, Options{Old: old}); err == nil {
		t.Fatal("unknown changed key")
	}
	if _, err := Validate(defs, model.CustomValues{"mass": nil}, Options{Old: old}); err == nil {
		t.Fatal("cannot clear a required value")
	}
	// a legacy item without the required value can still be saved
	if got, err := Validate(defs, model.CustomValues{"x": nil}, Options{Old: model.CustomValues{}}); err != nil || got != nil {
		t.Fatal(got, err)
	}
	if _, err := Validate(defs, model.CustomValues{"mass": "heavy"}, Options{}); err == nil {
		t.Fatal("bad number")
	}
}

func TestTextAndParse(t *testing.T) {
	mass := def("number", "mass")
	mass.Unit = "kg"
	ok := def("checkbox", "ok")
	multi := def(model.FieldMultiSelect, "m", "a", "b")
	if Text(&mass, 2.5, "en") != "2.5 kg" || Column(&mass) != "Mass (kg)" || Column(&ok) != "Ok" {
		t.Fatal(Text(&mass, 2.5, "en"))
	}
	if Text(&ok, true, "sv") != "Ja" || Text(&ok, false, "sv") != "Nej" || Text(&ok, true, "en") != "Yes" || Text(&ok, false, "en") != "No" {
		t.Fatal("checkbox")
	}
	if Text(&multi, []string{"a", "b"}, "en") != "a, b" || Text(&multi, nil, "en") != "" || Plain([]any{"a", "b"}) != "a, b" {
		t.Fatal("multi")
	}
	if v, err := Parse(&mass, "3 kg"); err != nil || v != 3.0 {
		t.Fatal(v, err)
	}
	if v, err := Parse(&ok, "Ja"); err != nil || v != true {
		t.Fatal(v, err)
	}
	if v, err := Parse(&multi, "a, b"); err != nil || !reflect.DeepEqual(v, []string{"a", "b"}) {
		t.Fatal(v, err)
	}
	if v, err := Parse(&mass, " "); err != nil || v != nil {
		t.Fatal(v, err)
	}
	if !reflect.DeepEqual(Strings(3), []string{"3"}) || Strings(struct{}{})[0] != "{}" || Strings(nil) != nil {
		t.Fatal("strings")
	}
	if !Equal([]any{"a"}, []string{"a"}) || Equal("1", 1.0) || !Equal(2, 2.0) || Equal(true, "true") || !Equal(nil, nil) {
		t.Fatal("equal")
	}
	defs := []model.CustomField{{Key: "b", EntityType: "test", Position: 2}, {Key: "a", EntityType: "test", Position: 1}, {Key: "c", EntityType: "run"}}
	if got := ForEntity(defs, "test"); len(got) != 2 || got[0].Key != "a" {
		t.Fatal(got)
	}
}

func TestBuiltins(t *testing.T) {
	ids := map[string]bool{}
	for _, pt := range model.ProjectTypes {
		en, sv := Builtins(pt, "", "en"), Builtins(pt, "", "sv")
		if len(en) < 2 || len(en) != len(sv) {
			t.Fatalf("%s: %d/%d", pt, len(en), len(sv))
		}
		for i := range en {
			if ids[en[i].ID] {
				t.Fatalf("duplicate id %s", en[i].ID)
			}
			ids[en[i].ID] = true
			if en[i].Name == sv[i].Name && en[i].ID != "builtin:hardware:emc" {
				t.Errorf("%s: no Swedish name", en[i].ID)
			}
			if !IsBuiltin(en[i].ID) || en[i].Source != model.TemplateBuiltin || en[i].ProjectType != pt {
				t.Errorf("%+v", en[i])
			}
			c := en[i].Content
			if en[i].Kind == model.KindRequirement && (len(c.Criteria) == 0 || c.Obligation == "" || len(c.Steps) != 0) {
				t.Errorf("%s: %+v", en[i].ID, c)
			}
			if en[i].Kind == model.KindTest && (len(c.Steps) == 0 || len(c.Criteria) != 0) {
				t.Errorf("%s: %+v", en[i].ID, c)
			}
		}
	}
	for _, want := range []string{"builtin:software:user-story", "builtin:software:non-functional", "builtin:software:api-test",
		"builtin:hardware:environmental", "builtin:hardware:emc", "builtin:hardware:design-tolerance",
		"builtin:system:interface", "builtin:system:integration", "builtin:business:business-rule", "builtin:business:acceptance"} {
		if !ids[want] {
			t.Errorf("missing %s", want)
		}
	}
	if b, ok := Builtin("builtin:hardware:environmental", "sv"); !ok || !strings.Contains(b.Name, "Miljötest") || len(b.Content.Steps) != 6 {
		t.Fatal(b)
	}
	if _, ok := Builtin("builtin:nope", "en"); ok {
		t.Fatal("unknown")
	}
	if len(Builtins("software", model.KindTest, "en")) != 1 {
		t.Fatal("kind filter")
	}
}
