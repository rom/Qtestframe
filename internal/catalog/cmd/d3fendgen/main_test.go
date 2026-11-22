package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ontology = `{"@graph": [
 {"@id": "http://d3fend.mitre.org/ontologies/d3fend.owl", "owl:versionInfo": "1.6.0", "d3f:release-date": {"@type": "xsd:dateTime", "@value": "2026-08-31T00:12:00+00:00"}},
 {"@id": "d3f:DefensiveTechnique", "rdfs:label": "Defensive Technique"},
 {"@id": "d3f:Harden", "rdfs:label": "Harden"},
 {"@id": "d3f:AgentAuthentication", "d3f:d3fend-id": "D3-AA", "rdfs:label": "Agent Authentication", "d3f:enables": {"@id": "d3f:Harden"},
  "rdfs:subClassOf": [{"@id": "_:b1"}, {"@id": "d3f:DefensiveTechnique"}]},
 {"@id": "_:b1", "@type": "owl:Restriction", "owl:onProperty": {"@id": "d3f:enables"}, "owl:someValuesFrom": {"@id": "d3f:Harden"}},
 {"@id": "d3f:Multi-factorAuthentication", "d3f:d3fend-id": "D3-MFA", "rdfs:label": "Multi-factor \"Authentication\"",
  "rdfs:subClassOf": [{"@id": "_:b2"}, {"@id": "d3f:AgentAuthentication"}]},
 {"@id": "_:b2", "@type": "owl:Restriction", "owl:onProperty": {"@id": "d3f:uses"}, "owl:someValuesFrom": {"@id": "d3f:Credential"}},
 {"@id": "d3f:NetworkTrafficAnalysis", "d3f:d3fend-id": "D3-NTA", "rdfs:label": "Network Traffic Analysis",
  "rdfs:subClassOf": [{"@id": "_:b3"}, {"@id": "d3f:DefensiveTechnique"}]},
 {"@id": "_:b3", "@type": "owl:Restriction", "owl:onProperty": {"@id": "d3f:enables"}, "owl:someValuesFrom": {"@id": "d3f:Detect"}},
 {"@id": "d3f:Clustering", "d3f:d3fend-id": "D3A-C", "rdfs:label": "Clustering", "rdfs:subClassOf": {"@id": "d3f:AnalyticTechnique"}},
 {"@id": "d3f:ARMA", "d3f:d3fend-id": "D3-ARMA", "rdfs:label": "ARMA", "rdfs:subClassOf": {"@id": "d3f:AnalyticTechnique"}}
]}`

const page = `{"def_to_off": {"results": {"bindings": [
 {"off_tech_id": {"value": "T1110.004"}}, {"off_tech_id": {"value": "T1110"}}, {"off_tech_id": {"value": "T1110"}},
 {"off_tech_id": {"value": "T1142"}}, {"off_tech_id": {"value": "SPARTA-1"}}, {"other": {"value": "x"}}]}}}`

func TestParseAndWrite(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ont := write("d3fend.json", ontology)
	att := write("attack.yaml", "techniques:\n  - {id: T1110, name: x}\n  - {id: T1110.004, name: y}\n")
	api := filepath.Join(dir, "api")
	if err := os.Mkdir(api, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"AgentAuthentication", "Multi-factorAuthentication", "NetworkTrafficAnalysis"} {
		write("api/"+n+".json", page)
	}
	out := filepath.Join(dir, "out.yaml")
	if err := run(ont, api, att, out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	for _, want := range []string{
		"version: \"1.6.0\"\nrelease: \"2026-08-31\"",
		`  - {id: D3-AA, name: "Agent Authentication", tactic: Harden, url: "https://d3fend.mitre.org/technique/d3f:AgentAuthentication/", attack: [T1110, T1110.004]}`,
		`  - {id: D3-MFA, name: "Multi-factor \"Authentication\"", tactic: Harden, parent: D3-AA,`,
		`  - {id: D3-NTA, name: "Network Traffic Analysis", tactic: Detect,`,
		"  - {id: Restore, name: \"Restore\", url: https://d3fend.mitre.org/tactic/d3f:Restore/}",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in\n%s", want, b)
		}
	}
	if strings.Contains(string(b), "D3A-C") || strings.Contains(string(b), "D3-ARMA") || strings.Contains(string(b), "T1142") {
		t.Errorf("analytic technique or unknown ATT&CK id written:\n%s", b)
	}
	// Harden before Detect
	if strings.Index(string(b), "D3-NTA") < strings.Index(string(b), "D3-MFA") {
		t.Error("order")
	}
	// without API pages: no relations
	if err := run(ont, "", "", out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); strings.Contains(string(b), "attack:") {
		t.Error("relations without api")
	}
	if err := run(ont, filepath.Join(dir, "missing"), att, out); err == nil {
		t.Error("missing api page accepted")
	}
	if err := run(write("bad.json", `{"@graph": []}`), "", "", out); err == nil {
		t.Error("ontology without version accepted")
	}
	if err := run(write("bad2.json", `{`), "", "", out); err == nil {
		t.Error("bad json accepted")
	}
	noTactic := strings.Replace(ontology, `"owl:someValuesFrom": {"@id": "d3f:Detect"}`, `"owl:someValuesFrom": {"@id": "d3f:Nothing"}`, 1)
	if err := run(write("bad3.json", noTactic), "", "", out); err == nil || !strings.Contains(err.Error(), "no tactic") {
		t.Errorf("technique without tactic: %v", err)
	}
	if _, err := offensive([]byte(`{`)); err == nil {
		t.Error("bad page accepted")
	}
}
