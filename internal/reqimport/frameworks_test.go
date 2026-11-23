package reqimport

import (
	"strings"
	"testing"
)

func TestFrameworkIDs(t *testing.T) {
	ids, warn := FrameworkIDs(FrameworkD3fend, "d3-mfa Multi-factor Authentication, D3-NOPE; D3-MFA")
	if strings.Join(ids, ",") != "D3-MFA" || !strings.Contains(warn, "unknown D3FEND technique D3-NOPE left out") {
		t.Fatal(ids, warn)
	}
	if ids, warn := FrameworkIDs(FrameworkEmb3d, "TID-201, MID-002"); len(ids) != 2 || warn != "" {
		t.Fatal(ids, warn)
	}
	// a risk takes threats only
	if ids, warn := FrameworkIDs(FrameworkEmb3dThreats, "TID-201, MID-002"); len(ids) != 1 || !strings.Contains(warn, "EMB3D threat MID-002") {
		t.Fatal(ids, warn)
	}
	if ids, warn := FrameworkIDs(FrameworkAttack, "t1110, T9999"); strings.Join(ids, ",") != "T1110" || !strings.Contains(warn, "T9999") {
		t.Fatal(ids, warn)
	}
	if ids, warn := FrameworkIDs(FrameworkD3fend, " "); len(ids) != 0 || warn != "" {
		t.Fatal(ids, warn)
	}
}

func TestCSVFrameworkColumns(t *testing.T) {
	// the columns of the requirement export, with D3FEND and EMB3D
	data := "Key,Title,Description,D3FEND,EMB3D\n" +
		"P-REQ-1,Login,The system MUST use MFA.,\"D3-MFA, D3-BOGUS\",TID-201\n" +
		"P-REQ-2,Boot,The device SHALL verify its firmware.,,\"MID-002, XYZ\"\n"
	res := mustParse(t, []byte(data), "export.csv", Options{})
	if fieldOf(res, "D3FEND") != FieldD3fend || fieldOf(res, "EMB3D") != FieldEmb3d {
		t.Fatalf("columns: %+v", res.Columns)
	}
	a, b := res.Candidates[0], res.Candidates[1]
	if strings.Join(a.D3fend, ",") != "D3-MFA" || strings.Join(a.Emb3d, ",") != "TID-201" || !strings.Contains(strings.Join(a.Warnings, "|"), "D3-BOGUS") {
		t.Errorf("first: %+v", a)
	}
	if len(b.D3fend) != 0 || strings.Join(b.Emb3d, ",") != "MID-002" || !strings.Contains(strings.Join(b.Warnings, "|"), "unknown EMB3D threat or mitigation XYZ") {
		t.Errorf("second: %+v", b)
	}
	// Swedish headers
	res = mustParse(t, []byte("Titel;Beskrivning;D3FEND-tekniker;EMB3D-hot\nA;Systemet SKA logga.;D3-MFA;TID-201\n"), "krav.csv", Options{})
	if c := res.Candidates[0]; len(c.D3fend) != 1 || len(c.Emb3d) != 1 {
		t.Errorf("swedish: %+v", c)
	}
}

func TestReadSheets(t *testing.T) {
	s, err := ReadSheets([]byte("sep=;\nTitle;Likelihood\nA;3\n"), "risks.csv")
	if err != nil || len(s) != 1 || len(s[0].Rows) != 2 || s[0].Lines[1] != 2 {
		t.Fatal(s, err)
	}
	if _, err := ReadSheets([]byte(" "), "x.csv"); err == nil {
		t.Fatal("empty file accepted")
	}
	if _, err := ReadSheets([]byte("PK\x03\x04broken"), "x.xlsx"); err == nil {
		t.Fatal("broken workbook accepted")
	}
	if _, err := ReadSheets([]byte("<x/>"), "x.docx"); err == nil {
		t.Fatal("docx accepted")
	}
	if NormHeader(" ATT&CK techniques (optional)") != "att&ck techniques" {
		t.Fatal(NormHeader(" ATT&CK techniques (optional)"))
	}
}
