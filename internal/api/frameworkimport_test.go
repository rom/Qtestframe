package api

import (
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// The imports read the D3FEND and EMB3D columns and attributes the
// exports write: the requirement import (CSV, XLSX, ReqIF), the Excel
// exchange and the ReqIF exchange; unknown ids are row warnings.
func TestImportsReadFrameworkColumns(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	src := a.project("FWA")
	base := "/api/v1/projects/" + src
	rq := a.m("POST", base+"/requirements", map[string]any{"title": "MFA", "description": "The system MUST require MFA.", "d3fend": []string{"D3-MFA"}, "emb3d": []string{"TID-201"}}, 201)
	tc := a.m("POST", base+"/tests", map[string]any{"title": "MFA test", "d3fend": []string{"D3-MFA"}}, 201)
	a.m("POST", base+"/links", map[string]any{"requirement_id": rq["id"], "test_case_id": tc["id"]}, 201)

	// the CSV export → the requirement import
	res := a.raw("GET", base+"/export?format=csv&aspects=requirements", nil)
	csv, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(csv), "D3FEND") {
		t.Fatalf("export: %s", csv)
	}
	dst := a.project("FWB")
	pv := a.upload("POST", importPath(dst)+"/preview", "file", "export.csv", "text/csv", csv, nil, 200)
	c := pv["candidates"].([]any)[0].(map[string]any)
	if jsonText(c["d3fend"]) != `["D3-MFA"]` || jsonText(c["emb3d"]) != `["TID-201"]` {
		t.Fatalf("candidate: %v", c)
	}
	bad := "Title,Description,D3FEND,EMB3D\nX,The system SHALL do X.,\"D3-MFA, D3-NOPE\",MID-999\n"
	pv = a.upload("POST", importPath(dst)+"/preview", "file", "x.csv", "text/csv", []byte(bad), nil, 200)
	c = pv["candidates"].([]any)[0].(map[string]any)
	if w := jsonText(c["warnings"]); !strings.Contains(w, "D3-NOPE left out") || !strings.Contains(w, "MID-999 left out") {
		t.Fatal(w)
	}
	out := a.upload("POST", importPath(dst), "file", "x.csv", "text/csv", []byte(bad), nil, 201)
	created := a.m("GET", "/api/v1/requirements/"+out["items"].([]any)[0].(map[string]any)["id"].(string), nil, 200)
	if jsonText(created["d3fend"]) != `["D3-MFA"]` || jsonText(created["emb3d"]) != `[]` {
		t.Fatalf("created: %v %v", created["d3fend"], created["emb3d"])
	}

	// the ReqIF export writes the attributes; the ReqIF import reads them
	res = a.raw("GET", base+"/export?format=reqif&include=tests", nil)
	reqif, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(reqif), `LONG-NAME="D3FEND"`) || !strings.Contains(string(reqif), `THE-VALUE="TID-201"`) {
		t.Fatal("no D3FEND / EMB3D attributes in the ReqIF export")
	}
	pv = a.upload("POST", importPath(dst)+"/preview", "file", "x.reqif", "", reqif, nil, 200)
	c = pv["candidates"].([]any)[0].(map[string]any)
	if jsonText(c["d3fend"]) != `["D3-MFA"]` || jsonText(c["emb3d"]) != `["TID-201"]` {
		t.Fatalf("reqif candidate: %v", c)
	}
	third := a.project("FWC")
	ap := a.upload("POST", "/api/v1/projects/"+third+"/exchange/reqif/apply", "file", "x.reqif", "", reqif, nil, 200)
	if ap["created"].(float64) != 2 {
		t.Fatalf("reqif apply: %v", ap)
	}
	for _, kind := range []string{"requirements", "tests"} {
		it := items(a.m("GET", "/api/v1/projects/"+third+"/"+kind, nil, 200))[0]
		if jsonText(it["d3fend"]) != `["D3-MFA"]` {
			t.Fatalf("%s via ReqIF: %v", kind, it["d3fend"])
		}
	}

	// the Excel exchange: a column with an unknown id is a warning, the known ids are saved
	wb := a.get(base+"/exchange/xlsx?kind=requirement", 200)
	edited := sheetEdit(t, wb, func(f *excelize.File) {
		sh := "Requirements"
		_ = f.SetCellValue(sh, colOf(t, f, sh, "EMB3D")+rowOf(t, f, sh, rq["key"].(string)), "TID-201, MID-002, TID-000")
	})
	pv = a.upload("POST", base+"/exchange/xlsx/preview", "file", "rt.xlsx", "", edited, nil, 200)
	rows := rowsOf(pv)
	if len(rows) != 1 || rows[0]["status"] != "changed" || !strings.Contains(jsonText(rows[0]["warnings"]), "EMB3D: unknown EMB3D threat or mitigation TID-000 left out") {
		t.Fatalf("preview: %v", rows)
	}
	a.upload("POST", base+"/exchange/xlsx/apply", "file", "rt.xlsx", "", edited, nil, 200)
	if got := a.m("GET", "/api/v1/requirements/"+rq["id"].(string), nil, 200); jsonText(got["emb3d"]) != `["TID-201","MID-002"]` {
		t.Fatal(got["emb3d"])
	}
}

// The risk register import reads the register of the risks report (or a
// sheet of one's own), with ATT&CK techniques and EMB3D threats.
func TestRiskRegisterImport(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("RIM")
	base := "/api/v1/projects/" + pid
	k := a.m("POST", base+"/risks", map[string]any{"title": "Credential stuffing", "likelihood": 4, "impact": 4, "attack": []string{"T1110"}}, 201)

	// the risk report's register round-trips
	_, xlsx := a.download(base+"/reports/generate", map[string]any{"template": "risks", "format": "xlsx"}, 200)
	dry := a.upload("POST", base+"/risks/import", "file", "risks.xlsx", "", xlsx, map[string]string{"dry_run": "true"}, 200)
	if dry["unchanged"].(float64) != 1 || dry["created"].(float64) != 0 || dry["dry_run"] != true {
		t.Fatalf("round trip: %v", dry)
	}

	csv := "Key;Title;Likelihood;Impact;Status;Mitigation;ATT&CK techniques;EMB3D threats\n" +
		k["key"].(string) + ";Credential stuffing;5;4;Åtgärdas;Rate limits;T1110, T1078, T0000;TID-201, MID-002\n" +
		";Firmware tampering;9;3;bogus;Signed images;;TID-201\n" +
		";;;;;;;\n" +
		"RIM-RISK-99;Unknown key;2;2;;;;\n" +
		";;2;2;;;T1110;\n"
	dry = a.upload("POST", base+"/risks/import", "file", "risks.csv", "", []byte(csv), map[string]string{"dry_run": "true"}, 200)
	if dry["updated"].(float64) != 1 || dry["created"].(float64) != 2 || dry["errors"].(float64) != 1 {
		t.Fatalf("dry run: %v", dry)
	}
	if n := len(items(a.m("GET", base+"/risks", nil, 200))); n != 1 {
		t.Fatal("the dry run stored risks", n)
	}
	got := a.upload("POST", base+"/risks/import", "file", "risks.csv", "", []byte(csv), nil, 200)
	rows := got["rows"].([]any)
	w0, w1 := jsonText(rows[0].(map[string]any)["warnings"]), jsonText(rows[1].(map[string]any)["warnings"])
	if !strings.Contains(w0, "technique T0000 left out") || !strings.Contains(w0, "unknown EMB3D threat MID-002 left out") {
		t.Fatal(w0)
	}
	if !strings.Contains(w1, "likelihood") || !strings.Contains(w1, "unknown status") {
		t.Fatal(w1)
	}
	if !strings.Contains(jsonText(rows[2]), "a new risk is created") || rows[3].(map[string]any)["error"] != "a risk needs a title" {
		t.Fatal(jsonText(rows))
	}
	upd := a.m("GET", "/api/v1/risks/"+k["id"].(string), nil, 200)
	if upd["likelihood"] != float64(5) || upd["status"] != "mitigating" || upd["mitigation"] != "Rate limits" || jsonText(upd["attack"]) != `["T1110","T1078"]` || jsonText(upd["emb3d"]) != `["TID-201"]` {
		t.Fatal(jsonText(upd))
	}
	all := items(a.m("GET", base+"/risks", nil, 200))
	if len(all) != 3 {
		t.Fatal(len(all))
	}
	for _, r := range all {
		if r["title"] == "Firmware tampering" && (r["likelihood"] != float64(3) || r["status"] != "identified" || jsonText(r["emb3d"]) != `["TID-201"]`) {
			t.Fatal(jsonText(r))
		}
	}
	a.upload("POST", base+"/risks/import", "file", "x.csv", "", []byte("Foo;Bar\n1;2\n"), nil, 400)
	a.upload("POST", base+"/risks/import", "file", "x.docx", "", []byte("hello"), nil, 400)
	e.member("viewer@example.com", "viewer")
	e.login("viewer@example.com").upload("POST", base+"/risks/import", "file", "risks.csv", "", []byte(csv), nil, 403)
}
