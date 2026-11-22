package main

import (
	"strings"
	"testing"
)

const bundle = `{"objects": [
 {"type": "x-mitre-collection", "x_mitre_version": "20.1"},
 {"type": "x-mitre-matrix", "name": "Enterprise ATT&CK", "tactic_refs": ["tac-2", "tac-1", "tac-gone"],
  "external_references": [{"source_name": "mitre-attack", "external_id": "enterprise-attack", "url": "https://attack.mitre.org/matrices/enterprise"}]},
 {"type": "x-mitre-tactic", "id": "tac-1", "name": "Credential Access", "x_mitre_shortname": "credential-access",
  "external_references": [{"source_name": "mitre-attack", "external_id": "TA0006", "url": "https://attack.mitre.org/tactics/TA0006"}]},
 {"type": "x-mitre-tactic", "id": "tac-2", "name": "Initial Access", "x_mitre_shortname": "initial-access",
  "external_references": [{"source_name": "mitre-attack", "external_id": "TA0001", "url": "https://attack.mitre.org/tactics/TA0001"}]},
 {"type": "attack-pattern", "name": "Valid Accounts", "kill_chain_phases": [{"kill_chain_name": "mitre-attack", "phase_name": "credential-access"}, {"kill_chain_name": "mitre-attack", "phase_name": "initial-access"}],
  "external_references": [{"source_name": "mitre-attack", "external_id": "T1078", "url": "https://attack.mitre.org/techniques/T1078"}]},
 {"type": "attack-pattern", "name": "Old", "revoked": true,
  "external_references": [{"source_name": "mitre-attack", "external_id": "T1000", "url": "https://attack.mitre.org/techniques/T1000"}]},
 {"type": "attack-pattern", "name": "Gone", "x_mitre_deprecated": true,
  "external_references": [{"source_name": "mitre-attack", "external_id": "T1001", "url": "https://attack.mitre.org/techniques/T1001"}]},
 {"type": "course-of-action", "name": "Multi-factor \"Authentication\"",
  "external_references": [{"source_name": "mitre-attack", "external_id": "M1032", "url": "https://attack.mitre.org/mitigations/M1032"}]}
]}`

func TestParseAndWrite(t *testing.T) {
	m, err := parse([]byte(bundle))
	if err != nil {
		t.Fatal(err)
	}
	if m.id != "enterprise" || m.name != "Enterprise" || m.version != "20.1" || len(m.tactics) != 2 || m.tactics[0].id != "TA0001" {
		t.Fatalf("matrix %+v", m)
	}
	if len(m.techniques) != 1 || strings.Join(m.techniques[0].tactics, ",") != "TA0001,TA0006" {
		t.Fatalf("techniques %+v", m.techniques)
	}
	var b strings.Builder
	if err := write(&b, []matrix{m}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`  - {id: enterprise, name: "Enterprise", version: "20.1", url: https://attack.mitre.org/matrices/enterprise}`,
		`  - {id: T1078, name: "Valid Accounts", matrix: enterprise, tactics: [TA0001, TA0006], url: https://attack.mitre.org/techniques/T1078}`,
		`  - {id: M1032, name: "Multi-factor \"Authentication\"", matrix: enterprise`,
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in\n%s", want, b.String())
		}
	}
	if _, err := parse([]byte(`{"objects": []}`)); err == nil {
		t.Error("no matrix accepted")
	}
	if _, err := parse([]byte(`{`)); err == nil {
		t.Error("bad json accepted")
	}
	if matrixName("ATT&CK for ICS") != "ICS" {
		t.Error(matrixName("ATT&CK for ICS"))
	}
}
