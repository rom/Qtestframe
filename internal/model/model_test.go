package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestValid(t *testing.T) {
	sets := map[string][]string{
		"obligations": Obligations, "reqstatus": RequirementStatuses, "reqtypes": RequirementTypes,
		"priorities": Priorities, "teststatus": TestStatuses, "langs": ScriptLanguages,
		"methods": EvaluationMethods, "runstatus": RunStatuses, "results": ResultStatuses,
	}
	for name, set := range sets {
		if len(set) == 0 {
			t.Fatalf("%s empty", name)
		}
		for _, v := range set {
			if !Valid(set, v) {
				t.Errorf("%s: Valid(%q) = false", name, v)
			}
		}
		if Valid(set, "") || Valid(set, "nope") {
			t.Errorf("%s: invalid value accepted", name)
		}
	}
	if Valid(nil, "x") {
		t.Error("Valid(nil) = true")
	}
	if !Valid(Obligations, ObligationMustNot) || !Valid(ResultStatuses, ResultBlocked) {
		t.Error("constant missing from set")
	}
}

func TestJSONContract(t *testing.T) {
	u := User{ID: "u1", Email: "a@b.c", PasswordHash: "secret", CreatedAt: time.Unix(0, 0).UTC()}
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "external_id") {
		t.Errorf("user JSON leaks hash or includes empty external_id: %s", b)
	}
	tok := APIToken{ID: "t", Hash: "h4sh"}
	b, _ = json.Marshal(tok)
	if strings.Contains(string(b), "h4sh") || strings.Contains(string(b), "expires_at") {
		t.Errorf("token JSON: %s", b)
	}
	var r Requirement
	if err := json.Unmarshal([]byte(`{"key":"R-1","obligation":"must","tags":["a"],"linked_tests":2}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Key != "R-1" || r.Obligation != ObligationMust || len(r.Tags) != 1 || r.LinkedTests != 2 {
		t.Errorf("unexpected %+v", r)
	}
}
