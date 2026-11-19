package export

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

const (
	maxSheetName = 31
	maxCellChars = 32767
	summarySheet = "Summary"
)

// sheetNamer produces valid, unique (case-insensitive) sheet names.
type sheetNamer struct{ used map[string]bool }

func (n *sheetNamer) name(want string, idx int) string {
	var b strings.Builder
	for _, r := range cleanXML(want) {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']', '\n', '\r', '\t':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	base := strings.Trim(strings.TrimSpace(b.String()), "'")
	if base == "" {
		base = fmt.Sprintf("Sheet %d", idx+1)
	}
	base = truncateRunes(base, maxSheetName)
	name := base
	for k := 2; n.used[strings.ToLower(name)]; k++ {
		suffix := fmt.Sprintf(" (%d)", k)
		name = truncateRunes(base, maxSheetName-len(suffix)) + suffix
	}
	n.used[strings.ToLower(name)] = true
	return name
}

func xlsxText(s string) string {
	return truncateRunes(cleanXML(s), maxCellChars)
}

// WriteXLSX writes ds as an Excel workbook: a "Summary" sheet followed by
// one sheet per section with a bold, filled, frozen header row and an
// autofilter.
func WriteXLSX(w io.Writer, ds Dataset) error {
	f := excelize.NewFile()
	defer f.Close()

	if err := f.SetSheetName("Sheet1", summarySheet); err != nil {
		return err
	}
	namer := &sheetNamer{used: map[string]bool{strings.ToLower(summarySheet): true}}

	header, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"305496"}},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "bottom", Color: "1F3864", Style: 1},
		},
	})
	if err != nil {
		return err
	}
	titleStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 16}})
	if err != nil {
		return err
	}
	wrap, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	if err != nil {
		return err
	}

	// Summary sheet.
	set := func(sheet, ref string, v any) {
		_ = f.SetCellValue(sheet, ref, v)
	}
	set(summarySheet, "A1", xlsxText(ds.Title))
	_ = f.SetCellStyle(summarySheet, "A1", "A1", titleStyle)
	set(summarySheet, "A2", xlsxText(ds.Subtitle))
	set(summarySheet, "A3", "Generated")
	set(summarySheet, "B3", generatedAt(ds).Format("2006-01-02 15:04:05 MST"))
	set(summarySheet, "A5", "Section")
	set(summarySheet, "B5", "Rows")
	_ = f.SetCellStyle(summarySheet, "A5", "B5", header)
	_ = f.SetColWidth(summarySheet, "A", "A", 40)
	_ = f.SetColWidth(summarySheet, "B", "B", 12)

	for i, s := range ds.Sections {
		want := s.Name
		if want == "" {
			want = s.Title
		}
		sheet := namer.name(want, i)
		if _, err := f.NewSheet(sheet); err != nil {
			return err
		}
		set(summarySheet, fmt.Sprintf("A%d", 6+i), xlsxText(sectionTitle(s, i)))
		set(summarySheet, fmt.Sprintf("B%d", 6+i), len(s.Rows))
		if err := writeSheet(f, sheet, s, header, wrap); err != nil {
			return err
		}
	}
	f.SetActiveSheet(0)
	return f.Write(w)
}

func writeSheet(f *excelize.File, sheet string, s Section, header, wrap int) error {
	n := width(s)
	if n == 0 {
		return nil
	}
	widths := make([]int, n)
	put := func(rowNum int, row []string) error {
		vals := make([]any, n)
		for i := 0; i < n; i++ {
			v := xlsxText(cell(row, i))
			vals[i] = v
			for _, line := range strings.Split(v, "\n") {
				if l := utf8.RuneCountInString(line); l > widths[i] {
					widths[i] = l
				}
			}
		}
		ref, _ := excelize.CoordinatesToCellName(1, rowNum)
		return f.SetSheetRow(sheet, ref, &vals)
	}
	if err := put(1, s.Columns); err != nil {
		return err
	}
	for i, r := range s.Rows {
		if err := put(i+2, r); err != nil {
			return err
		}
	}
	last, _ := excelize.CoordinatesToCellName(n, 1)
	if err := f.SetCellStyle(sheet, "A1", last, header); err != nil {
		return err
	}
	if len(s.Rows) > 0 {
		lastData, _ := excelize.CoordinatesToCellName(n, len(s.Rows)+1)
		if err := f.SetCellStyle(sheet, "A2", lastData, wrap); err != nil {
			return err
		}
	}
	for i, wd := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		w := float64(wd) + 2
		if w < 10 {
			w = 10
		}
		if w > 60 {
			w = 60
		}
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return err
		}
	}
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
		Selection: []excelize.Selection{{SQRef: "A2", ActiveCell: "A2", Pane: "bottomLeft"}},
	}); err != nil {
		return err
	}
	lastRef, _ := excelize.CoordinatesToCellName(n, len(s.Rows)+1)
	return f.AutoFilter(sheet, "A1:"+lastRef, nil)
}
