package export

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestSwedishLabels(t *testing.T) {
	ds := Dataset{Title: "Projektstatus", GeneratedAt: ts, Lang: "sv", Sections: []Section{
		{Name: "kpis", Title: "Nyckeltal", Columns: []string{"Mått", "Värde"}, Rows: [][]string{{"Krav", "3"}}},
		{Name: "empty"}, {Name: "norows", Title: "Krav per status", Columns: []string{"Status"}},
	}}
	var buf bytes.Buffer
	if err := Write(&buf, DOCX, ds); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
	}
	for _, s := range []string{"Skapad 2026-03-04", "(inga data)", "(inga rader)", "Nyckeltal"} {
		if !strings.Contains(parts["word/document.xml"], s) {
			t.Errorf("document.xml missing %q", s)
		}
	}
	if !strings.Contains(parts["word/styles.xml"], `<w:lang w:val="sv-SE"/>`) {
		t.Fatal("docx language")
	}
	buf.Reset()
	if err := Write(&buf, XLSX, ds); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.GetSheetList(), "|"); got != "Sammanfattning|Nyckeltal|empty|Krav per status" {
		t.Fatal(got)
	}
	for ref, want := range map[string]string{"A3": "Skapad", "A5": "Avsnitt", "B5": "Rader"} {
		if v, _ := f.GetCellValue("Sammanfattning", ref); v != want {
			t.Errorf("%s = %q", ref, v)
		}
	}
	buf.Reset()
	if err := Write(&buf, PDF, ds); err != nil || !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Fatal(err)
	}
	// unknown languages give English
	ds.Lang = "fr"
	if ds.label("generated") != "Generated" || !ds.english() || (Dataset{}).label("page") != "Page %d/%s" {
		t.Fatal("fallback")
	}
	if (Dataset{Lang: "sv"}).english() {
		t.Fatal("sv is not English")
	}
}
