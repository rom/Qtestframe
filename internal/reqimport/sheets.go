package reqimport

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// Sheet is a table of a CSV file or a sheet of an XLSX workbook, for
// imports other than requirements (the risk register).
type Sheet struct {
	Name string
	Rows [][]string
	// Lines are the 1-based line (CSV) or row (XLSX) numbers of Rows.
	Lines []int
}

// ReadSheets reads the tables of a CSV or XLSX file (by its name, else by
// its content): one for a CSV file, every sheet of a workbook.
func ReadSheets(data []byte, filename string) ([]Sheet, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, invalidf("the file is empty")
	}
	format, err := DetectFormat("", filename, data)
	if err != nil {
		return nil, err
	}
	switch format {
	case FormatCSV:
		return readCSVSheet(data)
	case FormatXLSX:
		f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: maxUnzipped, UnzipXMLSizeLimit: maxUnzipped / 2})
		if err != nil {
			return nil, invalidf("not a valid .xlsx file (%v)", err)
		}
		defer f.Close()
		var out []Sheet
		for _, name := range f.GetSheetList() {
			rows, err := sheetRows(f, name)
			if err != nil {
				return nil, invalidf("sheet %q: %v", name, err)
			}
			s := Sheet{Name: name, Rows: rows}
			for i := range rows {
				s.Lines = append(s.Lines, i+1)
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, invalidf("the workbook has no sheets")
		}
		return out, nil
	}
	return nil, invalidf("expected a CSV or XLSX file")
}

func readCSVSheet(data []byte) ([]Sheet, error) {
	text, _ := decodeText(data)
	delim := rune(0)
	if strings.HasPrefix(strings.ToLower(text), "sep=") && len(text) > 4 {
		r, _ := utf8.DecodeRuneInString(text[4:])
		delim = r
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		} else {
			text = ""
		}
	}
	if delim == 0 {
		delim = sniffDelimiter(text)
	}
	cr := csv.NewReader(strings.NewReader(text))
	cr.Comma = delim
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	s := Sheet{Name: "csv"}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, invalidf("row %d: %v", pe.StartLine, pe.Err)
			}
			return nil, invalidf("%v", err)
		}
		line, _ := cr.FieldPos(0)
		s.Rows = append(s.Rows, rec)
		s.Lines = append(s.Lines, line)
	}
	return []Sheet{s}, nil
}

// NormHeader normalises a column header (lower case, decoration such as
// "(optional)" removed).
func NormHeader(h string) string { return normHeader(h) }
