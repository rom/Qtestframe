package export

import (
	"bytes"
	"strings"
	"testing"
)

// mdCells splits a GFM table row into its cells (unescaped pipes only).
func mdCells(row string) []string {
	row = strings.TrimSpace(row)
	var cells []string
	var cur strings.Builder
	esc := false
	for _, r := range row {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\':
			cur.WriteRune(r)
			esc = true
		case r == '|':
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	cells = append(cells, cur.String())
	// a row starts and ends with a pipe: drop the empty outer parts
	return cells[1 : len(cells)-1]
}

// checkTables checks that every table row of md has as many cells as its
// header and returns the number of tables.
func checkTables(t *testing.T, md string) int {
	t.Helper()
	tables, want := 0, -1
	for i, l := range strings.Split(md, "\n") {
		if !strings.HasPrefix(l, "|") {
			want = -1
			continue
		}
		n := len(mdCells(l))
		if want < 0 {
			want = n
			tables++
			continue
		}
		if n != want {
			t.Errorf("line %d has %d cells, want %d: %q", i+1, n, want, l)
		}
	}
	return tables
}

func TestParseFormatMarkdown(t *testing.T) {
	for _, in := range []string{"md", "MD", ".md", "markdown", " Markdown "} {
		if f, err := ParseFormat(in); err != nil || f != MD {
			t.Errorf("%q: %v %v", in, f, err)
		}
	}
	if MD.ContentType() != "text/markdown; charset=utf-8" || MD.ContentTypeFor(true) != MD.ContentType() ||
		MD.Extension(true) != "md" || MD.Extension(false) != "md" {
		t.Fatal("md type / extension")
	}
	if _, err := ParseFormat("html"); err == nil || !strings.Contains(err.Error(), "md") {
		t.Fatal(err)
	}
}

func TestWriteMarkdown(t *testing.T) {
	ds := sample()
	ds.Meta = []Item{{Label: "Filter", Value: "status:approved | obligation:must"}, {Label: "Empty"}}
	ds.Link = func(v string) string {
		if strings.HasPrefix(v, "REQ-") {
			return "https://qtf.example.com/go/" + v
		}
		return ""
	}
	ds.Sections[1].Rows = append(ds.Sections[1].Rows, []string{"TC-3", "https://ci.example.com/job/1"})
	ds.Sections = append(ds.Sections, Section{Name: "empty"}, Section{Name: "norows", Title: "No rows", Columns: []string{"A"}},
		Section{Title: "Single", Columns: []string{"A"}, Rows: [][]string{{"x"}}})
	var buf bytes.Buffer
	if err := Write(&buf, MD, ds); err != nil {
		t.Fatal(err)
	}
	md := buf.String()
	for _, want := range []string{
		"# Requirements – Überblick\n",
		"Project \\<ACME\\> \\& \"friends\"\n",
		"- **Generated:** 2026-03-04 05:06:07 UTC\n- **Filter:** status:approved \\| obligation:must\n",
		"## Contents\n\n- Requirements (4 rows)\n- Tests (3 rows)\n- empty (0 rows)\n- No rows (0 rows)\n- Single (1 row)\n",
		"## Requirements\n\n| Key | Title | Note |\n| --- | --- | --- |\n",
		"| [REQ-1](https://qtf.example.com/go/REQ-1) | =HYPERLINK(\"http://evil\") | multi<br>line tab |\n",
		"| [REQ-3](https://qtf.example.com/go/REQ-3) | ctrlchar | +cmd |\n",
		"| [REQ-4](https://qtf.example.com/go/REQ-4) | | |\n",
		"| Key | Status | |\n",
		"| TC-2 | failed | extra |\n",
		"| TC-3 | [https://ci.example.com/job/1](https://ci.example.com/job/1) | |\n",
		"## empty\n\n*(no data)*\n",
		"## No rows\n\n| A |\n| --- |\n\n*(no rows)*\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	if strings.Contains(md, "Empty") || !strings.HasSuffix(md, "| x |\n") {
		t.Fatalf("meta without value / trailing newline:\n%s", md)
	}
	if n := checkTables(t, md); n != 4 {
		t.Fatalf("%d tables", n)
	}

	// Swedish labels, a single section has no contents list
	buf.Reset()
	sv := Dataset{Title: "Projektstatus", GeneratedAt: ts, Lang: "sv", Sections: []Section{{Title: "Krav", Columns: []string{"A"}}}}
	if err := WriteMarkdown(&buf, sv); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "**Skapad:**") || !strings.Contains(got, "*(inga rader)*") || strings.Contains(got, "Innehåll") {
		t.Fatal(got)
	}
	if (Dataset{Lang: "sv"}).rowCount(2) != "2 rader" || (Dataset{Lang: "sv"}).rowCount(1) != "1 rad" {
		t.Fatal("sv row count")
	}
	if err := WriteMarkdown(&failWriter{}, sv); err == nil {
		t.Fatal("expected write error")
	}
}

func TestMarkdownEscaping(t *testing.T) {
	for in, want := range map[string]string{
		"plain text":             "plain text",
		"# not a heading":        `\# not a heading`,
		"- not a list":           `\- not a list`,
		"+ x":                    `\+ x`,
		"> quote":                `\> quote`,
		"=== rule":               `\=== rule`,
		"1. first":               `1\. first`,
		"12) twelve":             `12\) twelve`,
		"*em* _em_ **b** `code`": `\*em\* \_em\_ \*\*b\*\* \` + "`code\\`",
		"[link](x) ![img](y)":    `\[link\](x) \!\[img\](y)`,
		"<b>html</b> &amp;":      `\<b\>html\</b\> \&amp;`,
		"a|b ~~s~~ c\\d":         `a\|b \~\~s\~\~ c\\d`,
		"tab\there\x00\x1b":      "tab here",
		"not_run _x_ a__b":       `not_run \_x\_ a\_\_b`,
		"bad \xff utf8":          "bad � utf8",
	} {
		if got := MarkdownText(in); got != want {
			t.Errorf("MarkdownText(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"a\nb":          "a<br>b",
		"\r\na\r\n\r\n": "a",
		"x\r\ny\rz":     "x<br>y<br>z",
		"a | b":         `a \| b`,
		"# x":           "# x", // no block starters inside a cell
	} {
		if got := MarkdownCell(in); got != want {
			t.Errorf("MarkdownCell(%q) = %q, want %q", in, got, want)
		}
	}
	for _, c := range []struct{ text, url, want string }{
		{"REQ-1", "https://x/go/REQ-1", "[REQ-1](https://x/go/REQ-1)"},
		{"", "https://x", "[https://x](https://x)"},
		{"a b", "https://x/a b(c)", "[a b](<https://x/a b(c)>)"},
		{"pipe", "https://x/?a|b", "[pipe](https://x/?a%7Cb)"},
		{"rel", "/p/1/reports", "[rel](/p/1/reports)"},
		{"mail", "mailto:a@b.c", "[mail](mailto:a@b.c)"},
		{"js", "javascript:alert(1)", "js"},
		{"none", "", "none"},
		{"angle", "https://x/<y>", "angle"},
	} {
		if got := Link(c.text, c.url); got != c.want {
			t.Errorf("Link(%q, %q) = %q, want %q", c.text, c.url, got, c.want)
		}
	}
}

func TestMarkdownWriterBlocks(t *testing.T) {
	var m MarkdownWriter
	m.Heading(0, "Title #")
	m.Heading(9, "")
	m.Heading(2, "#")
	m.Paragraph("one\ntwo\n\n\nthree  \n")
	m.Paragraph("   ")
	m.Emphasis("")
	m.List()
	m.List(Item{Value: "- dash"}, Item{Label: "L", Value: "v\nw"})
	m.Table(nil, nil, nil)
	m.Table(nil, [][]string{{"a", "b"}}, func(string) string { return "" })
	want := "# Title \\#\n\n###### \u00a0\n\n## \\#\n\none\\\ntwo\n\nthree\n\n- \\- dash\n- **L:** v w\n\n| | |\n| --- | --- |\n| a | b |\n"
	if got := m.String(); got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
	if (&MarkdownWriter{}).String() != "\n" {
		t.Fatal("empty document")
	}
}
