package mention

import (
	"reflect"
	"testing"
)

func TestSlugAndHandles(t *testing.T) {
	for in, want := range map[string]string{
		"Anna Berg":        "anna.berg",
		"  Åsa  Öberg ":    "åsa.öberg",
		"o'Neil, Pat":      "o.neil.pat",
		"-x-":              "x",
		"":                 "",
		"Team QA (nights)": "team.qa.nights",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	if EmailHandle("Anna.Berg@Example.com") != "anna.berg" || EmailHandle("@x.org") != "x.org" {
		t.Error("EmailHandle")
	}
}

func TestExtract(t *testing.T) {
	got := Extract("Ask @anna.berg and @Ben, cc @anna.berg. Mail me at x@example.com; @ alone; (@åsa-ö).")
	want := []string{"anna.berg", "ben", "åsa-ö"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if Extract("no mentions") != nil {
		t.Error("expected nil")
	}
}

func TestDirectory(t *testing.T) {
	users := []User{
		{ID: "2", Email: "anna@b.example", Name: ""},
		{ID: "1", Email: "anna@a.example", Name: "Anna Andersson", Avatar: "preset:fox"},
		{ID: "3", Email: "ben@a.example", Name: "Ben"},
	}
	names := []string{"@anna", "Anna Andersson", "ben@a.example", "Carl Carlsson", "carl carlsson", "@dora", "@dora", "", "@ann"}
	ps := Directory(users, names)
	byHandle := map[string]Person{}
	for _, p := range ps {
		byHandle[p.Handle] = p
	}
	if p := byHandle["anna"]; p.UserID != "1" || p.Uses != 2 || p.Avatar != "preset:fox" || p.Kind != KindUser {
		t.Errorf("anna: %+v", p)
	}
	if p := byHandle["anna-2"]; p.UserID != "2" || p.Name != "anna@b.example" {
		t.Errorf("anna-2: %+v", p)
	}
	if p := byHandle["ben"]; p.Uses != 1 {
		t.Errorf("ben: %+v", p)
	}
	if p := byHandle["carl.carlsson"]; p.Kind != KindPerson || p.Uses != 2 || p.Name != "Carl Carlsson" {
		t.Errorf("carl: %+v", p)
	}
	if p := byHandle["dora"]; p.Kind != KindPerson || p.Uses != 2 {
		t.Errorf("dora: %+v", p)
	}
	if _, ok := byHandle["ann"]; !ok {
		t.Error("unknown handle becomes a person")
	}
	// users first, then people, each by name
	kinds := ""
	for _, p := range ps {
		kinds += p.Kind[:1]
	}
	if kinds != "uuuppp" {
		t.Errorf("order %q: %+v", kinds, ps)
	}
	if ps[0].Name != "Anna Andersson" || ps[3].Name != "ann" {
		t.Errorf("sort: %+v", ps)
	}
	if len(Directory(nil, []string{"!!!"})) != 0 {
		t.Error("a name without letters is skipped")
	}
}
