package reqimport

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// tableOut is the result of parsing one table.
type tableOut struct {
	columns    []Column
	candidates []Candidate
	errors     []string
	headerRow  int // index into rows
}

var errNoHeader = errors.New("no header row")

// columnName returns the spreadsheet letter of a 0-based column index.
func columnName(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

// findHeader returns the index of the header row: the first of the leading
// non-empty rows with a column for the title or the description (or, with
// an explicit mapping, a column named in it).
func findHeader(rows [][]string, mapping map[string]string) int {
	seen := 0
	for i, row := range rows {
		if isBlankRow(row) {
			continue
		}
		if seen++; seen > headerScanRows {
			break
		}
		// A header row has an explicitly mapped column, a known Title or
		// Description column name, or at least two recognised columns one
		// of which is the title or description.
		fields := map[string]bool{}
		for _, cell := range row {
			h := cleanCell(cell)
			if h == "" {
				continue
			}
			if f, explicit := mappedField(mapping, h); explicit {
				if f != "" {
					return i
				}
				continue
			}
			f, ok, exact := guessField(h)
			if !ok || f == "" {
				continue
			}
			if exact && (f == FieldTitle || f == FieldDescription) {
				return i
			}
			fields[f] = true
		}
		if len(fields) >= 2 && (fields[FieldTitle] || fields[FieldDescription]) {
			return i
		}
	}
	return -1
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// headers names the columns of the header row: empty cells become
// "Column C" and repeated names get a number.
func headers(row []string, width int) []string {
	out := make([]string, width)
	seen := map[string]int{}
	for i := 0; i < width; i++ {
		h := ""
		if i < len(row) {
			h = strings.Join(strings.Fields(cleanCell(row[i])), " ")
		}
		if h == "" {
			h = "Column " + columnName(i)
		}
		key := strings.ToLower(h)
		if seen[key]++; seen[key] > 1 {
			h = fmt.Sprintf("%s (%d)", h, seen[key])
		}
		out[i] = h
	}
	return out
}

// parseRows turns tabular rows (header row first, possibly after a few
// leading rows) into candidates. num gives the user-visible number of a row
// index (nil: index+1) and prefix starts its location ("table 2, ").
func parseRows(rows [][]string, mapping map[string]string, num func(i int) int, prefix string) (*tableOut, error) {
	if num == nil {
		num = func(i int) int { return i + 1 }
	}
	loc := func(i int) string { return fmt.Sprintf("%srow %d", prefix, num(i)) }
	hi := findHeader(rows, mapping)
	if hi < 0 {
		return nil, errNoHeader
	}
	width := 0
	for _, r := range rows[hi:] {
		if len(r) > width {
			width = len(r)
		}
	}
	names := headers(rows[hi], width)
	out := &tableOut{headerRow: hi}
	for i, h := range names {
		f, _ := mappedField(mapping, h)
		out.columns = append(out.columns, Column{Index: i, Header: h, Field: f})
	}
	for i := hi + 1; i < len(rows); i++ {
		row := rows[i]
		if isBlankRow(row) {
			continue
		}
		c := Candidate{Row: num(i), Location: loc(i)}
		var rw raw
		var folders []string
		mappedContent := false
		for _, col := range out.columns {
			if col.Index >= len(row) || col.Field == "" {
				continue
			}
			v := cleanCell(row[col.Index])
			if v == "" {
				continue
			}
			mappedContent = true
			switch col.Field {
			case FieldTitle:
				c.Title = strings.TrimSpace(c.Title + " " + v)
			case FieldDescription:
				c.Description = joinText(c.Description, v)
			case FieldExternalID:
				c.ExternalID = firstNonEmpty(c.ExternalID, v)
			case FieldObligation:
				rw.obligation = firstNonEmpty(rw.obligation, v)
			case FieldType:
				rw.typ = firstNonEmpty(rw.typ, v)
			case FieldPriority:
				rw.priority = firstNonEmpty(rw.priority, v)
			case FieldStatus:
				rw.status = firstNonEmpty(rw.status, v)
			case FieldSource:
				c.Source = firstNonEmpty(c.Source, v)
			case FieldRationale:
				c.Rationale = joinText(c.Rationale, v)
			case FieldOwner:
				c.Owner = firstNonEmpty(c.Owner, v)
			case FieldTags:
				c.Tags = append(c.Tags, splitTags(v)...)
			case FieldFolder:
				folders = append(folders, v)
			case FieldCriteria:
				rw.criteria = joinText(rw.criteria, v)
			default:
				if k, ok := strings.CutPrefix(col.Field, CustomPrefix); ok {
					if c.CustomFields == nil {
						c.CustomFields = map[string]string{}
					}
					c.CustomFields[k] = joinText(c.CustomFields[k], v)
				}
			}
		}
		c.Folder = joinFolder(folders...)
		if !mappedContent {
			out.errors = append(out.errors, loc(i)+": no value in the imported columns; skipped")
			continue
		}
		if !finish(&c, rw) {
			out.errors = append(out.errors, loc(i)+": no title or description; skipped")
			continue
		}
		out.candidates = append(out.candidates, c)
	}
	return out, nil
}

func joinText(a, b string) string {
	if a == "" {
		return b
	}
	return a + "\n" + b
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func noHeaderError(where string) error {
	return invalidf("%sno header row found: one of the first %d rows must name the columns, with at least a "+
		"Title or Description column (Swedish: Titel, Beskrivning, Krav); or choose the columns yourself", where, headerScanRows)
}

// ---------------------------------------------------------------- CSV

// decodeText returns data as UTF-8: a UTF-8 or UTF-16 byte order mark is
// honoured, and text that is not valid UTF-8 is read as Windows-1252.
func decodeText(data []byte) (string, string) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return string(data[3:]), ""
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		le := data[0] == 0xFF
		data = data[2:]
		u := make([]uint16, 0, len(data)/2)
		for i := 0; i+1 < len(data); i += 2 {
			if le {
				u = append(u, uint16(data[i])|uint16(data[i+1])<<8)
			} else {
				u = append(u, uint16(data[i])<<8|uint16(data[i+1]))
			}
		}
		return string(utf16.Decode(u)), ""
	case utf8.Valid(data):
		return string(data), ""
	}
	var b strings.Builder
	for _, c := range data {
		if c >= 0x80 && c < 0xA0 {
			b.WriteRune(cp1252[c-0x80])
		} else {
			b.WriteRune(rune(c))
		}
	}
	return b.String(), "the file is not UTF-8; it was read as Windows-1252 (Western European)"
}

var cp1252 = [32]rune{'€', '�', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '�', 'Ž', '�',
	'�', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '�', 'ž', 'Ÿ'}

// sniffDelimiter picks ",", ";" or tab by counting them outside quotes in
// the first lines.
func sniffDelimiter(text string) rune {
	counts := map[rune]int{}
	inQuote := false
	nl := 0
	for _, r := range text {
		switch {
		case r == '"':
			inQuote = !inQuote
		case inQuote:
		case r == '\n':
			if nl++; nl >= 5 {
				goto done
			}
		case r == ',' || r == ';' || r == '\t':
			counts[r]++
		}
	}
done:
	best, n := ',', 0
	for _, r := range []rune{',', ';', '\t'} {
		if counts[r] > n {
			best, n = r, counts[r]
		}
	}
	return best
}

func parseCSV(data []byte, opt Options, res *Result) error {
	text, warn := decodeText(data)
	if warn != "" {
		res.warn("%s", warn)
	}
	// Excel's "sep=;" hint line.
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
	var rows [][]string
	var lineNo []int
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return invalidf("row %d: %v", pe.StartLine, pe.Err)
			}
			return invalidf("%v", err)
		}
		line, _ := cr.FieldPos(0)
		rows = append(rows, rec)
		lineNo = append(lineNo, line)
	}
	out, err := parseRows(rows, opt.Mapping, func(i int) int { return lineNo[i] }, "")
	if err != nil {
		return noHeaderError("")
	}
	res.Columns, res.Candidates, res.Errors = out.columns, out.candidates, out.errors
	if out.headerRow > 0 {
		res.warn("rows above the header row %d were ignored", lineNo[out.headerRow])
	}
	return nil
}
