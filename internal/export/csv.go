package export

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// sanitizeCSVCell protects against CSV / formula injection by prefixing cells
// that a spreadsheet could interpret as a formula with a single quote.
func sanitizeCSVCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func writeSectionCSV(w io.Writer, s Section) error {
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	n := width(s)
	rec := make([]string, n)
	put := func(row []string) error {
		for i := 0; i < n; i++ {
			rec[i] = sanitizeCSVCell(strings.ToValidUTF8(cell(row, i), "�"))
		}
		return cw.Write(rec)
	}
	if len(s.Columns) > 0 {
		if err := put(s.Columns); err != nil {
			return err
		}
	}
	for _, r := range s.Rows {
		if err := put(r); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteCSV writes a single-section dataset as plain RFC 4180 CSV (UTF-8,
// CRLF). Datasets with more than one section are written as a ZIP archive
// containing one <name>.csv per section.
func WriteCSV(w io.Writer, ds Dataset) error {
	switch len(ds.Sections) {
	case 0:
		return nil
	case 1:
		return writeSectionCSV(w, ds.Sections[0])
	}
	zw := zip.NewWriter(w)
	used := map[string]bool{}
	mod := generatedAt(ds)
	for i, s := range ds.Sections {
		base := safeFileName(s.Name)
		if base == "" {
			base = safeFileName(s.Title)
		}
		if base == "" {
			base = fmt.Sprintf("section-%d", i+1)
		}
		name := base
		for k := 2; used[strings.ToLower(name)]; k++ {
			name = fmt.Sprintf("%s-%d", base, k)
		}
		used[strings.ToLower(name)] = true
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name + ".csv", Method: zip.Deflate, Modified: mod})
		if err != nil {
			return err
		}
		if err := writeSectionCSV(f, s); err != nil {
			return err
		}
	}
	return zw.Close()
}
