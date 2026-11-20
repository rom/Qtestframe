package api

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/rom/qtestframe/internal/model"
)

const importCSV = "ID;Titel;Beskrivning;Mapp;Prioritet;Acceptanskriterier\n" +
	"K-1;Inloggning;Användaren SKA kunna logga in.;Säkerhet/Inloggning;Hög;Inloggning fungerar\n" +
	"K-2;Utloggning;Användaren BÖR kunna logga ut.;Säkerhet;;\n" +
	"K-3;Export;The system MAY export data.;;low;\n"

func importPath(pid string) string { return "/api/v1/projects/" + pid + "/requirements/import" }

func TestRequirementImportPreview(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("IMP")
	// An existing requirement makes K-3's title a duplicate.
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Export", "source": "Customer"}, 201)

	res := a.upload("POST", importPath(pid)+"/preview", "file", "krav.csv", "text/csv", []byte(importCSV), nil, 200)
	if res["format"] != "csv" || res["total"].(float64) != 3 || res["duplicates"].(float64) != 1 {
		t.Fatalf("preview: %v", res)
	}
	cands := res["candidates"].([]any)
	first := cands[0].(map[string]any)
	if first["external_id"] != "K-1" || first["obligation"] != "shall" || first["folder"] != "Säkerhet/Inloggning" || first["priority"] != "high" {
		t.Errorf("first: %v", first)
	}
	if d := cands[2].(map[string]any)["duplicate"]; d != "title" {
		t.Errorf("duplicate: %v", d)
	}
	cols := res["columns"].([]any)
	if len(cols) != 6 || cols[0].(map[string]any)["field"] != "external_id" {
		t.Errorf("columns: %v", cols)
	}
	if _, ok := res["errors"].([]any); !ok {
		t.Errorf("errors must be an array: %v", res["errors"])
	}
	if len(res["fields"].([]any)) < 10 {
		t.Errorf("fields: %v", res["fields"])
	}
	// Nothing stored.
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/requirements", nil, 200))); n != 1 {
		t.Errorf("preview stored requirements: %d", n)
	}
	// Mapping override: the "Mapp" column is ignored, "Prioritet" read as tags.
	res = a.upload("POST", importPath(pid)+"/preview", "file", "krav.csv", "", []byte(importCSV),
		map[string]string{"mapping": `{"Mapp":"ignore","prioritet":"tags"}`}, 200)
	first = res["candidates"].([]any)[0].(map[string]any)
	if first["folder"] != "" || first["priority"] != "" || len(first["tags"].([]any)) != 1 {
		t.Errorf("mapping: %v", first)
	}
	// Target folder path in the preview.
	f := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Kund"}, 201)
	res = a.upload("POST", importPath(pid)+"/preview", "file", "krav.csv", "", []byte(importCSV), map[string]string{"folder_id": f["id"].(string)}, 200)
	if res["target_folder"] != "Kund" {
		t.Errorf("target folder: %v", res["target_folder"])
	}
}

func TestRequirementImportErrors(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("IMP")
	p := importPath(pid) + "/preview"
	a.upload("POST", p, "file", "x.csv", "", []byte("foo,bar\n1,2\n"), nil, 400)                                    // no header
	a.upload("POST", p, "file", "x.docx", "", []byte("not a zip"), nil, 400)                                        // broken docx
	a.upload("POST", p, "file", "x.xls", "", []byte("binary"), nil, 400)                                            // old Excel
	a.upload("POST", p, "file", "x.csv", "", []byte(importCSV), map[string]string{"format": "pdf"}, 400)            // unknown format
	a.upload("POST", p, "file", "x.csv", "", []byte(importCSV), map[string]string{"mapping": "{bad"}, 400)          // bad JSON
	a.upload("POST", p, "file", "x.csv", "", []byte(importCSV), map[string]string{"mapping": `{"ID":"nope"}`}, 400) // unknown field
	a.upload("POST", p, "file", "x.csv", "", []byte(importCSV), map[string]string{"folder_id": "missing"}, 400)
	a.upload("POST", p, "", "", "", nil, nil, 400) // no file
	a.upload("POST", "/api/v1/projects/nope/requirements/import/preview", "file", "x.csv", "", []byte(importCSV), nil, 404)
	big := bytes.Repeat([]byte("x"), maxImportBytes+10)
	a.upload("POST", p, "file", "x.csv", "", big, nil, 400)
	var many strings.Builder
	many.WriteString("Title\n")
	for i := 0; i <= maxImportCandidates; i++ {
		many.WriteString("R\n")
	}
	a.upload("POST", p, "file", "x.csv", "", []byte(many.String()), nil, 400)
	imp := importPath(pid)
	a.upload("POST", imp, "file", "x.csv", "", []byte(importCSV), map[string]string{"status": "weird"}, 400)
	a.upload("POST", imp, "file", "x.csv", "", []byte(importCSV), map[string]string{"exclude": "[x"}, 400)
	a.upload("POST", imp, "file", "x.csv", "", []byte("Title,Type\nA,functional\n"), map[string]string{"mapping": `{"Type":"title"}`}, 201)

	// Permissions: a tester has no requirement.write.
	e.member("tester@example.com", "tester")
	tc := e.login("tester@example.com")
	tc.upload("POST", p, "file", "x.csv", "", []byte(importCSV), nil, 403)
	tc.upload("POST", imp, "file", "x.csv", "", []byte(importCSV), nil, 403)

	// A role that may write but not review requirements cannot import approved ones.
	a.m("POST", "/api/v1/roles", map[string]any{"name": "writer", "permissions": []string{"project.read", "requirement.read", "requirement.write"}}, 201)
	e.member("writer@example.com", "writer")
	wc := e.login("writer@example.com")
	approved := "Title,Status\nA,approved\n"
	wc.upload("POST", imp, "file", "x.csv", "", []byte(approved), nil, 403)
	wc.upload("POST", imp, "file", "x.csv", "", []byte("Title\nA\n"), map[string]string{"status": "approved"}, 403)
	wc.upload("POST", imp, "file", "x.csv", "", []byte("Title\nA\n"), map[string]string{"create_tests": "true"}, 403)
	wc.upload("POST", imp, "file", "x.csv", "", []byte("Title\nB\n"), nil, 201)
}

func TestRequirementImportWithFolders(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("IMP")
	root := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Kund"}, 201)
	rootID := root["id"].(string)
	// An existing folder below the root is reused (case-insensitive).
	a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "säkerhet", "parent_id": rootID}, 201)

	// Dry run: nothing stored, but the counts are reported.
	dry := a.upload("POST", importPath(pid), "file", "krav.csv", "", []byte(importCSV),
		map[string]string{"folder_id": rootID, "dry_run": "true"}, 200)
	if dry["created"].(float64) != 3 || dry["folders_created"].(float64) != 1 || dry["dry_run"] != true {
		t.Fatalf("dry run: %v", dry)
	}
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/requirements", nil, 200))); n != 0 {
		t.Fatalf("dry run stored %d", n)
	}

	res := a.upload("POST", importPath(pid), "file", "krav.csv", "", []byte(importCSV),
		map[string]string{"folder_id": rootID, "create_tests": "true", "status": "review", "exclude": "[2]"}, 201)
	if res["created"].(float64) != 2 || res["skipped"].(float64) != 1 || res["folders_created"].(float64) != 1 || res["tests_created"].(float64) != 2 {
		t.Fatalf("import: %v", res)
	}
	got := res["items"].([]any)
	first := got[0].(map[string]any)
	rq := a.m("GET", "/api/v1/requirements/"+first["id"].(string), nil, 200)
	if rq["source"] != "ID: K-1" || rq["status"] != "review" || rq["obligation"] != "shall" || rq["priority"] != "high" ||
		rq["folder_path"] != "Kund / säkerhet / Inloggning" || len(rq["criteria"].([]any)) != 1 || rq["linked_tests"].(float64) != 1 {
		t.Errorf("requirement: %v", rq)
	}
	if skipped := res["skipped_items"].([]any)[0].(map[string]any); skipped["reason"] != "excluded" || skipped["row"].(float64) != 4 {
		t.Errorf("skipped: %v", skipped)
	}
	folders := items(a.m("GET", "/api/v1/projects/"+pid+"/folders", nil, 200))
	reqFolders := 0
	for _, f := range folders {
		if f["kind"] == "requirement" {
			reqFolders++
		}
	}
	if reqFolders != 3 {
		t.Errorf("requirement folders: %d (%v)", reqFolders, folders)
	}

	// Importing again: duplicates by external ID (K-1, K-2) are skipped,
	// K-3 is new now.
	res = a.upload("POST", importPath(pid), "file", "krav.csv", "", []byte(importCSV), map[string]string{"create_folders": "false"}, 201)
	if res["created"].(float64) != 1 || res["skipped"].(float64) != 2 {
		t.Fatalf("re-import: %v", res)
	}
	reasons := res["skipped_items"].([]any)
	if reasons[0].(map[string]any)["reason"] != "duplicate_external_id" {
		t.Errorf("reason: %v", reasons)
	}
	k3 := a.m("GET", "/api/v1/requirements/"+res["items"].([]any)[0].(map[string]any)["id"].(string), nil, 200)
	if k3["folder_id"] != nil || k3["status"] != "draft" || k3["obligation"] != "may" {
		t.Errorf("k3: %v", k3)
	}
	// Same titles, no IDs: skipped by title; with skip_duplicates=false imported.
	titles := "Title\nInloggning\nUtloggning\n"
	res = a.upload("POST", importPath(pid), "file", "t.csv", "", []byte(titles), nil, 201)
	if res["created"].(float64) != 0 || res["skipped_items"].([]any)[0].(map[string]any)["reason"] != "duplicate_title" {
		t.Errorf("title duplicates: %v", res)
	}
	res = a.upload("POST", importPath(pid), "file", "t.csv", "", []byte(titles), map[string]string{"skip_duplicates": "false"}, 201)
	if res["created"].(float64) != 2 {
		t.Errorf("no skipping: %v", res)
	}
	// The import is audited.
	entries, err := e.st.ListAudit(context.Background(), e.tenant.ID, 100)
	audited := 0
	for _, en := range entries {
		if en.Action == "requirement.import" {
			audited++
		}
	}
	if err != nil || audited != 4 {
		t.Errorf("audit entries: %d %v", audited, err)
	}
}

func TestRequirementImportFormats(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("IMP")

	// XLSX
	f := excelize.NewFile()
	_ = f.SetSheetRow("Sheet1", "A1", &[]any{"Key", "Title", "Description", "Folder"})
	_ = f.SetSheetRow("Sheet1", "A2", &[]any{"X-1", "From Excel", "It MUST work", "Excel / Sub"})
	var xb bytes.Buffer
	if err := f.Write(&xb); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	res := a.upload("POST", importPath(pid), "file", "spec.xlsx", "", xb.Bytes(), map[string]string{"sheet": "Sheet1"}, 201)
	if res["format"] != "xlsx" || res["folders_created"].(float64) != 2 {
		t.Errorf("xlsx: %v", res)
	}

	// DOCX
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Word chapter</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>REQ-9: The tool SHALL read Word files.</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Plain text.</w:t></w:r></w:p></w:body></w:document>`
	res = a.upload("POST", importPath(pid)+"/preview", "file", "spec.docx", "", zipBytes(t, map[string]string{"word/document.xml": doc}), nil, 200)
	c := res["candidates"].([]any)
	if res["format"] != "docx" || len(c) != 1 || c[0].(map[string]any)["external_id"] != "REQ-9" || c[0].(map[string]any)["folder"] != "Word chapter" {
		t.Errorf("docx: %v", res)
	}

	// ReqIF (zipped)
	reqif := `<?xml version="1.0"?><REQ-IF><CORE-CONTENT><REQ-IF-CONTENT>
	<SPEC-TYPES><SPEC-OBJECT-TYPE IDENTIFIER="t"><SPEC-ATTRIBUTES>
	<ATTRIBUTE-DEFINITION-STRING IDENTIFIER="n" LONG-NAME="ReqIF.Name"/></SPEC-ATTRIBUTES></SPEC-OBJECT-TYPE></SPEC-TYPES>
	<SPEC-OBJECTS><SPEC-OBJECT IDENTIFIER="o"><VALUES><ATTRIBUTE-VALUE-STRING THE-VALUE="ReqIF SHALL load">
	<DEFINITION><ATTRIBUTE-DEFINITION-STRING-REF>n</ATTRIBUTE-DEFINITION-STRING-REF></DEFINITION></ATTRIBUTE-VALUE-STRING></VALUES></SPEC-OBJECT></SPEC-OBJECTS>
	</REQ-IF-CONTENT></CORE-CONTENT></REQ-IF>`
	res = a.upload("POST", importPath(pid), "file", "spec.reqifz", "", zipBytes(t, map[string]string{"spec.reqif": reqif}), nil, 201)
	if res["format"] != "reqif" || res["created"].(float64) != 1 {
		t.Errorf("reqif: %v", res)
	}
	// An empty import creates nothing.
	res = a.upload("POST", importPath(pid), "file", "e.csv", "", []byte("Title\n\n"), nil, 201)
	if res["created"].(float64) != 0 {
		t.Errorf("empty: %v", res)
	}
}

func TestImportHelpers(t *testing.T) {
	if sourceWithID("", "") != "" || sourceWithID("Kund", "") != "Kund" || sourceWithID("", "R1") != "ID: R1" ||
		sourceWithID("Kund", "R1") != "Kund, ID: R1" || sourceWithID("Kund, ID: R1", "r1") != "Kund, ID: R1" {
		t.Error("sourceWithID")
	}
	if n := markDuplicates([]model.Requirement{{Title: "A", Source: "Contract, ID: X-1"}}, nil); n != 0 {
		t.Error("markDuplicates nil")
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
