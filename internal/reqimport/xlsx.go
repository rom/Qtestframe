package reqimport

import (
	"bytes"
	"strings"

	"github.com/xuri/excelize/v2"
)

// preferredSheets are tried first when no sheet is named.
var preferredSheets = []string{"requirements", "krav", "kravlista", "kravspecifikation", "requirement"}

// criteriaSheets hold one criterion per row (the export's criteria sheet).
var criteriaSheets = []string{"criteria", "evaluation criteria", "kriterier", "acceptanskriterier", "acceptance criteria"}

func parseXLSX(data []byte, opt Options, res *Result) error {
	if !isZip(data) {
		return invalidf("not a valid .xlsx file (old .xls workbooks are not supported; save as .xlsx)")
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: maxUnzipped, UnzipXMLSizeLimit: maxUnzipped / 2})
	if err != nil {
		return invalidf("not a valid .xlsx file (%v)", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	res.Sheets = sheets
	if len(sheets) == 0 {
		return invalidf("the workbook has no sheets")
	}
	var order []string
	if opt.Sheet != "" {
		for _, s := range sheets {
			if strings.EqualFold(s, strings.TrimSpace(opt.Sheet)) {
				order = []string{s}
			}
		}
		if order == nil {
			return invalidf("sheet %q not found; the workbook has: %s", opt.Sheet, strings.Join(sheets, ", "))
		}
	} else {
		for _, want := range preferredSheets {
			for _, s := range sheets {
				if strings.EqualFold(s, want) {
					order = append(order, s)
				}
			}
		}
		for _, s := range sheets {
			if !containsFold(order, s) && !containsFold(criteriaSheets, s) {
				order = append(order, s)
			}
		}
	}
	for _, sheet := range order {
		rows, err := sheetRows(f, sheet)
		if err != nil {
			return invalidf("sheet %q: %v", sheet, err)
		}
		out, err := parseRows(rows, opt.Mapping, nil, "")
		if err != nil {
			continue // no header row: try the next sheet
		}
		res.Sheet = sheet
		res.Columns, res.Candidates, res.Errors = out.columns, out.candidates, out.errors
		if out.headerRow > 0 {
			res.warn("sheet %q: rows above the header row %d were ignored", sheet, out.headerRow+1)
		}
		attachCriteriaSheet(f, sheets, res)
		return nil
	}
	if opt.Sheet != "" {
		return noHeaderError("sheet \"" + order[0] + "\": ")
	}
	return noHeaderError("no sheet has a header row; ")
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// sheetRows reads all rows of a sheet and copies the value of merged cells
// into every cell of the merged range.
func sheetRows(f *excelize.File, sheet string) ([][]string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	merged, err := f.GetMergeCells(sheet)
	if err != nil {
		return rows, nil // merged cells are a convenience
	}
	for _, m := range merged {
		c1, r1, err1 := excelize.CellNameToCoordinates(m.GetStartAxis())
		c2, r2, err2 := excelize.CellNameToCoordinates(m.GetEndAxis())
		if err1 != nil || err2 != nil || (r2-r1+1)*(c2-c1+1) > 100000 {
			continue
		}
		v := m.GetCellValue()
		for r := r1; r <= r2; r++ {
			for len(rows) < r {
				rows = append(rows, nil)
			}
			for c := c1; c <= c2; c++ {
				for len(rows[r-1]) < c {
					rows[r-1] = append(rows[r-1], "")
				}
				rows[r-1][c-1] = v
			}
		}
	}
	return rows, nil
}

// attachCriteriaSheet adds the criteria of a criteria sheet (columns
// Requirement, Criterion, Method, Threshold as in the export) to the
// candidates with that external ID.
func attachCriteriaSheet(f *excelize.File, sheets []string, res *Result) {
	if res.Sheet == "" || containsFold(criteriaSheets, res.Sheet) {
		return
	}
	byID := map[string]*Candidate{}
	for i := range res.Candidates {
		if id := res.Candidates[i].ExternalID; id != "" {
			byID[strings.ToLower(id)] = &res.Candidates[i]
		}
	}
	if len(byID) == 0 {
		return
	}
	for _, s := range sheets {
		if !containsFold(criteriaSheets, s) {
			continue
		}
		rows, err := sheetRows(f, s)
		if err != nil || len(rows) < 2 {
			continue
		}
		col := map[string]int{}
		for i, h := range rows[0] {
			switch normHeader(h) {
			case "requirement", "krav", "key", "id", "requirement id":
				col["req"] = i
			case "criterion", "criteria", "kriterium", "description", "beskrivning":
				col["text"] = i
			case "method", "metod":
				col["method"] = i
			case "threshold", "tröskelvärde", "gränsvärde":
				col["threshold"] = i
			}
		}
		ri, okR := col["req"]
		ti, okT := col["text"]
		if !okR || !okT {
			continue
		}
		get := func(row []string, k string) string {
			if i, ok := col[k]; ok && i < len(row) {
				return cleanCell(row[i])
			}
			return ""
		}
		n := 0
		for _, row := range rows[1:] {
			if ri >= len(row) || ti >= len(row) {
				continue
			}
			c := byID[strings.ToLower(cleanCell(row[ri]))]
			text := cleanCell(row[ti])
			if c == nil || text == "" {
				continue
			}
			m, _ := ParseMethod(get(row, "method"))
			c.Criteria = append(c.Criteria, Criterion{Description: text, Method: m, Threshold: get(row, "threshold")})
			n++
		}
		if n > 0 {
			res.warn("%d evaluation criteria were read from sheet %q", n, s)
		}
	}
}
