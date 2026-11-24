package testimport

import (
	"fmt"
	"strings"

	"github.com/rom/qtestframe/internal/reqimport"
)

// table is the header and rows of a CSV file or a sheet.
type table struct {
	sheet   string
	headers []string
	rows    [][]string
	lines   []int
	used    map[int]bool
}

const headerScanRows = 20

// readCSV reads a CSV file (delimiter, encoding and "sep=" line as the
// requirement import does) whose header row has a column of names.
func readCSV(data []byte, names ...string) (*table, error) {
	sheets, err := reqimport.ReadSheets(data, "import.csv")
	if err != nil {
		return nil, unwrapReq(err)
	}
	return headerTable(sheets[0], names)
}

// readXLSX reads the first sheet of a workbook with a header row that has
// a column of names.
func readXLSX(data []byte, names []string) (*table, error) {
	sheets, err := reqimport.ReadSheets(data, "import.xlsx")
	if err != nil {
		return nil, unwrapReq(err)
	}
	var first error
	for _, s := range sheets {
		t, err := headerTable(s, names)
		if err == nil {
			t.sheet = s.Name
			return t, nil
		}
		if first == nil {
			first = err
		}
	}
	return nil, first
}

func unwrapReq(err error) error {
	return invalidf("%s", strings.TrimPrefix(err.Error(), reqimport.ErrInvalid.Error()+": "))
}

// headerTable finds the header row: the first of the leading rows with a
// cell named like one of names (any row when names is empty).
func headerTable(s reqimport.Sheet, names []string) (*table, error) {
	want := map[string]bool{}
	for _, n := range names {
		want[norm(n)] = true
	}
	seen := 0
	for i, row := range s.Rows {
		blank := true
		for _, c := range row {
			if strings.TrimSpace(c) != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		if seen++; seen > headerScanRows {
			break
		}
		ok := len(want) == 0
		for _, c := range row {
			if want[norm(c)] {
				ok = true
			}
		}
		if !ok {
			continue
		}
		t := &table{used: map[int]bool{}}
		for _, c := range row {
			t.headers = append(t.headers, strings.Join(strings.Fields(strings.TrimPrefix(c, "\ufeff")), " "))
		}
		for j := i + 1; j < len(s.Rows); j++ {
			t.rows = append(t.rows, s.Rows[j])
			t.lines = append(t.lines, s.Lines[j])
		}
		return t, nil
	}
	return nil, invalidf("no header row found; expected a column named %s", strings.Join(quoteAll(names), " or "))
}

func quoteAll(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// col returns the index of the first column named like one of names (-1
// none) and marks it as used.
func (t *table) col(names ...string) int {
	for _, n := range names {
		nn := norm(n)
		for i, h := range t.headers {
			if norm(h) == nn {
				t.used[i] = true
				return i
			}
		}
	}
	return -1
}

// prefixCol returns the first column whose normalised name starts with
// prefix.
func (t *table) prefixCol(prefixes ...string) int {
	for _, p := range prefixes {
		np := norm(p)
		for i, h := range t.headers {
			if strings.HasPrefix(norm(h), np) {
				t.used[i] = true
				return i
			}
		}
	}
	return -1
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(row[i], "\r\n", "\n"))
}

func blank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// custom returns the values of the columns no parser used.
func (t *table) custom(row []string) map[string]string {
	out := map[string]string{}
	for i, h := range t.headers {
		if t.used[i] || h == "" {
			continue
		}
		if v := cell(row, i); v != "" {
			out[h] = v
		}
	}
	return out
}

// mergeCustom adds the values of a continuation row that the case does not
// have yet.
func mergeCustom(c *Case, more map[string]string) {
	for k, v := range more {
		if c.Custom == nil {
			c.Custom = map[string]string{}
		}
		if _, ok := c.Custom[k]; !ok {
			c.Custom[k] = v
		}
	}
}

func (t *table) location(i int) string {
	if t.sheet != "" {
		return fmt.Sprintf("sheet %q, row %d", t.sheet, t.lines[i])
	}
	return fmt.Sprintf("row %d", t.lines[i])
}
