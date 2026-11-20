package api

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSCIMFilterEvaluation(t *testing.T) {
	var res map[string]any
	_ = json.Unmarshal([]byte(`{
		"id": "u1", "externalId": "Ext-1", "userName": "Ada@Example.com", "active": true, "displayName": "",
		"name": {"givenName": "Ada", "familyName": "Lovelace"},
		"emails": [{"value": "ada@example.com", "type": "work", "primary": true}, {"value": "ada@home.org", "type": "home"}],
		"groups": [{"value": "g1", "display": "Leads"}],
		"meta": {"lastModified": "2026-01-02T03:04:05Z", "version": "W/\"1\""},
		"score": 42,
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": {"department": "QA"}
	}`), &res)
	cases := map[string]bool{
		`userName eq "ada@example.com"`: true,
		`USERNAME Eq "ADA@EXAMPLE.COM"`: true,
		`userName ne "ada@example.com"`: false,
		`userName ne "bob@example.com"`: true,
		`userName co "example"`:         true,
		`userName sw "ada@"`:            true,
		`userName ew ".com"`:            true,
		`userName ew ".org"`:            false,
		`externalId eq "Ext-1"`:         true,
		`externalId eq "ext-1"`:         false,
		`id eq "U1"`:                    false,
		`active eq true`:                true,
		`active eq false`:               false,
		`active eq "true"`:              false,
		`name.givenName eq "ada" and name.familyName eq "lovelace"`: true,
		`name.givenName eq "ada" and name.familyName eq "x"`:        false,
		`name.givenName eq "x" or name.familyName eq "lovelace"`:    true,
		`not (userName eq "ada@example.com")`:                       false,
		`(userName eq "x" or active eq true) and score gt 40`:       true,
		`score ge 42 and score le 42 and score lt 43`:               true,
		`score eq 41`:             false,
		`score gt "x"`:            false,
		`emails[type eq "work"]`:  true,
		`emails[type eq "other"]`: false,
		`emails[type eq "work"].value eq "ADA@example.com"`: true,
		`emails[type eq "home"].value eq "ada@example.com"`: false,
		`emails[type eq "work" and primary eq true]`:        true,
		`emails.value eq "ada@home.org"`:                    true,
		`emails pr`:                                         true,
		`displayName pr`:                                    false,
		`nickName pr`:                                       false,
		`title eq null`:                                     true,
		`userName eq null`:                                  false,
		`meta.lastModified gt "2026-01-01T00:00:00Z"`:       true,
		`meta.lastModified lt "2026-01-01T00:00:00Z"`:       false,
		`groups[value eq "g1"]`:                             true,
		`urn:ietf:params:scim:schemas:core:2.0:User:name.familyName eq "Lovelace"`:      true,
		`urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department eq "qa"`: true,
		`userName eq "a\"b"`:                             false,
		`active eq true and not (name.givenName sw "B")`: true,
	}
	for in, want := range cases {
		f, err := parseSCIMFilter(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got := f.match(res); got != want {
			t.Errorf("%s: got %v, want %v", in, got, want)
		}
	}
}

func TestSCIMFilterErrors(t *testing.T) {
	for _, in := range []string{
		``, `userName`, `userName eq`, `userName like "x"`, `userName eq "x`, `userName eq x`,
		`(userName eq "x"`, `not userName eq "x"`, `not (userName eq "x"`, `emails[type eq "work"`,
		`userName eq "x" and`, `userName eq "x" extra`, `"x" eq "y"`, `userName eq "\q"`,
		`emails[type eq "work"].value`, `emails[type eq "work"].value foo "x"`, `[`, `userName eq (`,
	} {
		if _, err := parseSCIMFilter(in); !errors.Is(err, errFilter) {
			t.Errorf("%q: want a filter error, got %v", in, err)
		}
	}
}

func TestSCIMPaths(t *testing.T) {
	p, err := parseSCIMPath(`emails[type eq "work"].value`)
	if err != nil || p.attr != "emails" || p.sub != "value" || p.filter == nil ||
		!p.filter.match(map[string]any{"type": "work"}) {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = parseSCIMPath("name.givenName")
	if err != nil || p.attr != "name" || p.sub != "givenname" || p.filter != nil {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = parseSCIMPath("urn:ietf:params:scim:schemas:core:2.0:User:active")
	if err != nil || p.attr != "active" {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = parseSCIMPath(`members[value eq "u1"]`)
	if err != nil || p.attr != "members" || !p.filter.match(map[string]any{"value": "u1"}) {
		t.Fatalf("%+v %v", p, err)
	}
	for _, bad := range []string{"", " ", `"x"`, `name.givenName[type eq "x"]`, `emails[type eq "x"`, `emails[type eq "x"] y`, `emails[type eq]`, `a b`, `x "y`} {
		if _, err := parseSCIMPath(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestSCIMProjectAndFlexBool(t *testing.T) {
	res := map[string]any{"schemas": 1, "id": 2, "meta": 3, "userName": 4, "emails": 5}
	if got := project(res, "", ""); len(got) != 5 {
		t.Fatal(got)
	}
	if got := project(res, "userName", ""); len(got) != 4 || got["emails"] != nil {
		t.Fatal(got)
	}
	if got := project(res, "", "emails, urn:ietf:params:scim:schemas:core:2.0:User:userName"); len(got) != 3 {
		t.Fatal(got)
	}
	for in, want := range map[string]bool{`true`: true, `"True"`: true, `"FALSE"`: false, `false`: false} {
		var b flexBool
		if err := json.Unmarshal([]byte(in), &b); err != nil || bool(b) != want {
			t.Errorf("%s: %v %v", in, b, err)
		}
	}
	for _, in := range []string{`"yes"`, `1`, `null`} {
		var b flexBool
		if err := json.Unmarshal([]byte(in), &b); err == nil {
			t.Errorf("%s: want an error", in)
		}
	}
	if pickEmail([]scimEmail{{Value: " "}, {Value: "h@x", Type: "home"}}) != "h@x" {
		t.Fatal("first e-mail")
	}
	if got, _ := rawString(json.RawMessage(`[{"value":"x@y"}]`)); got != "x@y" {
		t.Fatal(got)
	}
	if got, _ := rawString(json.RawMessage(`["a"]`)); got != "a" {
		t.Fatal(got)
	}
	if got, err := rawString(json.RawMessage(`null`)); got != "" || err != nil {
		t.Fatal(got, err)
	}
	for _, bad := range []string{`1`, `[1,2]`, `{`} {
		if _, err := rawString(json.RawMessage(bad)); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
	if added, removed := diffIDs([]string{"a", "b", "b"}, []string{"b", "c", "c"}); len(added) != 1 || added[0] != "c" || len(removed) != 1 || removed[0] != "a" {
		t.Fatal(added, removed)
	}
}
