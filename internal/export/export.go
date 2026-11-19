// Package export renders a neutral tabular Dataset into CSV (or a ZIP of
// CSVs), XLSX, PDF and DOCX documents.
package export

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// Section is one table of a dataset.
type Section struct {
	Name    string // machine name (file / sheet name)
	Title   string // human readable heading
	Columns []string
	Rows    [][]string
}

// Dataset is a format-neutral export payload.
type Dataset struct {
	Title       string
	Subtitle    string
	GeneratedAt time.Time
	Sections    []Section
}

// Format is an export file format.
type Format string

// Supported formats.
const (
	CSV  Format = "csv"
	XLSX Format = "xlsx"
	PDF  Format = "pdf"
	DOCX Format = "docx"
)

// ParseFormat parses a format name (case-insensitive, optional leading dot;
// "excel" and "word" are accepted as aliases).
func ParseFormat(s string) (Format, error) {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), ".") {
	case "csv":
		return CSV, nil
	case "xlsx", "excel":
		return XLSX, nil
	case "pdf":
		return PDF, nil
	case "docx", "word":
		return DOCX, nil
	}
	return "", fmt.Errorf("export: unsupported format %q (want csv, xlsx, pdf or docx)", s)
}

// ContentType returns the MIME type of a single-file export in format f.
func (f Format) ContentType() string {
	switch f {
	case CSV:
		return "text/csv; charset=utf-8"
	case XLSX:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case PDF:
		return "application/pdf"
	case DOCX:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return "application/octet-stream"
}

// ContentTypeFor is like ContentType but accounts for multi-section CSV
// exports, which are delivered as a ZIP archive.
func (f Format) ContentTypeFor(multi bool) string {
	if f == CSV && multi {
		return "application/zip"
	}
	return f.ContentType()
}

// Extension returns the file extension (without dot). A CSV export with more
// than one section is a ZIP archive.
func (f Format) Extension(multi bool) string {
	if f == CSV && multi {
		return "zip"
	}
	return string(f)
}

// Write renders ds in format f.
func Write(w io.Writer, f Format, ds Dataset) error {
	switch f {
	case CSV:
		return WriteCSV(w, ds)
	case XLSX:
		return WriteXLSX(w, ds)
	case PDF:
		return WritePDF(w, ds)
	case DOCX:
		return WriteDOCX(w, ds)
	}
	return fmt.Errorf("export: unsupported format %q", string(f))
}

// ---------------------------------------------------------------------------
// helpers

func generatedAt(ds Dataset) time.Time {
	if ds.GeneratedAt.IsZero() {
		return time.Now().UTC()
	}
	return ds.GeneratedAt
}

func sectionTitle(s Section, i int) string {
	switch {
	case s.Title != "":
		return s.Title
	case s.Name != "":
		return s.Name
	}
	return fmt.Sprintf("Section %d", i+1)
}

// cell returns row[i] or "" when the row is short.
func cell(row []string, i int) string {
	if i < len(row) {
		return row[i]
	}
	return ""
}

// width returns the number of columns of a section (max of header and rows).
func width(s Section) int {
	n := len(s.Columns)
	for _, r := range s.Rows {
		if len(r) > n {
			n = len(r)
		}
	}
	return n
}

// cleanXML makes s valid UTF-8 and removes characters not allowed in XML 1.0.
func cleanXML(s string) string {
	s = strings.ToValidUTF8(s, "�")
	ok := true
	for _, r := range s {
		if !validXMLRune(r) {
			ok = false
			break
		}
	}
	if ok {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if validXMLRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validXMLRune(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}

// truncateRunes cuts s to at most n runes.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}

// safeFileName turns name into a portable file name stem.
func safeFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "._")
	return truncateRunes(s, 100)
}
