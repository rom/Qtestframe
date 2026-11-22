package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bundle = `{"type": "bundle", "objects": [
 {"type": "identity", "id": "identity--1"},
 {"type": "x-mitre-emb3d-property", "id": "x-mitre-emb3d-property--1", "name": "Device includes a microprocessor", "category": "Hardware", "x_mitre_emb3d_property_id": "PID-11"},
 {"type": "x-mitre-emb3d-property", "id": "x-mitre-emb3d-property--2", "name": "Device includes buses", "category": "Hardware", "x_mitre_emb3d_property_id": "PID-121"},
 {"type": "x-mitre-emb3d-property", "id": "x-mitre-emb3d-property--3", "name": "Memory", "category": "Hardware", "x_mitre_emb3d_property_id": "PID-12"},
 {"type": "vulnerability", "id": "vulnerability--1", "name": "Power \"Consumption\" Analysis", "x_mitre_emb3d_threat_id": "TID-101", "x_mitre_emb3d_threat_category": "hardware", "x_mitre_emb3d_threat_maturity": "Proof of Concept"},
 {"type": "vulnerability", "id": "vulnerability--2", "name": "Old", "revoked": true, "x_mitre_emb3d_threat_id": "TID-999"},
 {"type": "course-of-action", "id": "course-of-action--2", "name": "Software Patterns", "x_mitre_emb3d_mitigation_id": "MID-059", "x_mitre_emb3d_mitigation_maturity": "intermediate"},
 {"type": "course-of-action", "id": "course-of-action--1", "name": "Validated Cryptographic Libraries", "x_mitre_emb3d_mitigation_id": "MID-027", "x_mitre_emb3d_mitigation_maturity": "foundational"},
 {"type": "relationship", "relationship_type": "mitigates", "source_ref": "course-of-action--2", "target_ref": "vulnerability--1"},
 {"type": "relationship", "relationship_type": "mitigates", "source_ref": "course-of-action--1", "target_ref": "vulnerability--1"},
 {"type": "relationship", "relationship_type": "mitigates", "source_ref": "course-of-action--1", "target_ref": "vulnerability--1"},
 {"type": "relationship", "relationship_type": "relates-to", "source_ref": "x-mitre-emb3d-property--1", "target_ref": "vulnerability--1"},
 {"type": "relationship", "relationship_type": "subproperty-of", "source_ref": "x-mitre-emb3d-property--2", "target_ref": "x-mitre-emb3d-property--3"},
 {"type": "relationship", "relationship_type": "mitigates", "source_ref": "course-of-action--9", "target_ref": "vulnerability--1"}
]}`

func TestParseAndWrite(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "emb3d-stix-2.0.1.json")
	if err := os.WriteFile(in, []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "emb3d.yaml")
	if err := run(in, "", out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	for _, want := range []string{
		"version: \"2.0.1\"",
		`  - {id: PID-121, name: "Device includes buses", category: "hardware", parent: PID-12}`,
		`  - {id: TID-101, name: "Power \"Consumption\" Analysis", category: "hardware", maturity: "proof of concept", url: https://emb3d.mitre.org/threats/TID-101.html, properties: [PID-11], mitigations: [MID-027, MID-059]}`,
		`  - {id: MID-027, name: "Validated Cryptographic Libraries", maturity: "foundational", url: https://emb3d.mitre.org/mitigations/MID-027.html}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in\n%s", want, b)
		}
	}
	if strings.Contains(string(b), "TID-999") || strings.Index(string(b), "MID-027") > strings.Index(string(b), "id: MID-059") {
		t.Errorf("revoked threat or order:\n%s", b)
	}
	if err := run(in, "9.9", out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); !strings.Contains(string(b), `version: "9.9"`) {
		t.Error("version flag")
	}
	other := filepath.Join(dir, "bundle.json")
	_ = os.WriteFile(other, []byte(bundle), 0o600)
	if err := run(other, "", out); err == nil {
		t.Error("no version accepted")
	}
	for _, bad := range []string{`{`, `{"objects": []}`,
		`{"objects": [{"type": "vulnerability", "id": "v", "x_mitre_emb3d_threat_id": "X-1"}]}`,
		`{"objects": [{"type": "course-of-action", "id": "c", "x_mitre_emb3d_mitigation_id": "M1"}]}`,
		`{"objects": [{"type": "x-mitre-emb3d-property", "id": "p", "x_mitre_emb3d_property_id": "P"}]}`} {
		if _, err := parse([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if err := run(filepath.Join(dir, "missing.json"), "", out); err == nil {
		t.Error("missing file accepted")
	}
}
