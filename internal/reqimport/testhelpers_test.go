package reqimport

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range sortedKeys(files) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// xlsxOf builds a workbook; sheets maps a name to rows, in order of names.
func xlsxOf(t *testing.T, names []string, sheets map[string][][]any, merge map[string][2]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for i, name := range names {
		if i == 0 {
			if err := f.SetSheetName("Sheet1", name); err != nil {
				t.Fatal(err)
			}
		} else if _, err := f.NewSheet(name); err != nil {
			t.Fatal(err)
		}
		for r, row := range sheets[name] {
			cell, _ := excelize.CoordinatesToCellName(1, r+1)
			if err := f.SetSheetRow(name, cell, &row); err != nil {
				t.Fatal(err)
			}
		}
		if m, ok := merge[name]; ok {
			if err := f.MergeCell(name, m[0], m[1]); err != nil {
				t.Fatal(err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Word document builders.

func wPara(style, text string, numbered bool) string {
	var ppr string
	if style != "" || numbered {
		ppr = "<w:pPr>"
		if style != "" {
			ppr += `<w:pStyle w:val="` + style + `"/>`
		}
		if numbered {
			ppr += `<w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr>`
		}
		ppr += "</w:pPr>"
	}
	var runs strings.Builder
	for i, part := range strings.Split(text, "\n") {
		if i > 0 {
			runs.WriteString("<w:r><w:br/></w:r>")
		}
		runs.WriteString(`<w:r><w:t xml:space="preserve">` + xmlEscape(part) + `</w:t></w:r>`)
	}
	return "<w:p>" + ppr + runs.String() + "</w:p>"
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// wTable builds a table; a cell "<<" continues a vertical merge and a cell
// starting with "span2:" spans two columns.
func wTable(rows [][]string) string {
	var b strings.Builder
	b.WriteString("<w:tbl><w:tblPr/>")
	for _, row := range rows {
		b.WriteString("<w:tr>")
		for _, c := range row {
			b.WriteString("<w:tc>")
			switch {
			case c == "<<":
				b.WriteString(`<w:tcPr><w:vMerge/></w:tcPr><w:p/>`)
			case strings.HasPrefix(c, "span2:"):
				b.WriteString(`<w:tcPr><w:gridSpan w:val="2"/></w:tcPr>` + wPara("", strings.TrimPrefix(c, "span2:"), false))
			case strings.HasPrefix(c, "restart:"):
				b.WriteString(`<w:tcPr><w:vMerge w:val="restart"/></w:tcPr>` + wPara("", strings.TrimPrefix(c, "restart:"), false))
			default:
				b.WriteString(wPara("", c, false))
			}
			b.WriteString("</w:tc>")
		}
		b.WriteString("</w:tr>")
	}
	b.WriteString("</w:tbl>")
	return b.String()
}

const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"`

func docxOf(t *testing.T, body string, styles string) []byte {
	t.Helper()
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"word/document.xml":   `<?xml version="1.0" encoding="UTF-8"?><w:document ` + wNS + `><w:body>` + body + `<w:sectPr/></w:body></w:document>`,
	}
	if styles != "" {
		files["word/styles.xml"] = `<?xml version="1.0"?><w:styles ` + wNS + `>` + styles + `</w:styles>`
	}
	return zipOf(t, files)
}

func mustParse(t *testing.T, data []byte, name string, opt Options) *Result {
	t.Helper()
	res, err := Parse(data, name, opt)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return res
}

func fieldOf(res *Result, header string) string {
	for _, c := range res.Columns {
		if c.Header == header {
			return c.Field
		}
	}
	return "?"
}
