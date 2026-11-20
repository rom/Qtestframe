// Package reqimport parses requirement specifications that customers send
// (CSV, XLSX, ReqIF and DOCX) into a common list of candidates. Nothing is
// stored here: the API previews the candidates and creates requirements.
package reqimport

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// Formats.
const (
	FormatCSV   = "csv"
	FormatXLSX  = "xlsx"
	FormatReqIF = "reqif"
	FormatDOCX  = "docx"
)

// Formats lists the supported formats.
var Formats = []string{FormatCSV, FormatXLSX, FormatReqIF, FormatDOCX}

// Limits.
const (
	// DefaultMaxCandidates is the largest number of candidates a file may yield.
	DefaultMaxCandidates = 5000
	// maxUnzipped bounds the decompressed size of one archive entry.
	maxUnzipped = 128 << 20
	// headerScanRows is how many leading rows are searched for a header row.
	headerScanRows = 20
)

// ErrInvalid marks problems with the uploaded file or the options; the
// message is meant for the user.
var ErrInvalid = errors.New("invalid import")

func invalidf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Criterion is an evaluation (acceptance) criterion of a candidate.
type Criterion struct {
	Description string `json:"description"`
	Method      string `json:"method"`
	Threshold   string `json:"threshold"`
}

// Candidate is a requirement found in a file. Enumerated fields hold model
// codes ("" when absent: the importer applies defaults).
type Candidate struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Obligation  string `json:"obligation"`
	// ObligationFrom is "column" (given in the file), "text" (detected from
	// title and description) or "" (none found).
	ObligationFrom string      `json:"obligation_from"`
	Type           string      `json:"type"`
	Priority       string      `json:"priority"`
	Status         string      `json:"status"`
	Source         string      `json:"source"`
	Rationale      string      `json:"rationale"`
	Owner          string      `json:"owner"`
	Tags           []string    `json:"tags"`
	Folder         string      `json:"folder"` // "A/B", relative to the import root
	Criteria       []Criterion `json:"criteria"`
	ExternalID     string      `json:"external_id"`
	// Row is the 1-based row, paragraph or object number in the file and
	// Location its human readable form ("row 5", "paragraph 12" …).
	Row      int      `json:"row"`
	Location string   `json:"location"`
	Warnings []string `json:"warnings"`
	// Duplicate is set by the importer: "external_id" or "title".
	Duplicate string `json:"duplicate,omitempty"`
}

// Column is a detected column (or ReqIF attribute) and the field it maps to
// ("" = ignored).
type Column struct {
	Index  int    `json:"index"`
	Header string `json:"header"`
	Field  string `json:"field"`
}

// Options control parsing.
type Options struct {
	// Format is csv, xlsx, reqif (also reqifz) or docx; "" = from the file
	// name, then from the content.
	Format string
	// Sheet selects an XLSX sheet by name ("" = the first sheet with a
	// recognisable header row).
	Sheet string
	// Mapping overrides the guessed field of columns by header name
	// (case-insensitive): header → field, "" or "ignore" to ignore it.
	Mapping map[string]string
	// MaxCandidates limits the result (0 = DefaultMaxCandidates).
	MaxCandidates int
}

// Result is a parsed file.
type Result struct {
	Format     string      `json:"format"`
	Sheet      string      `json:"sheet,omitempty"`
	Sheets     []string    `json:"sheets,omitempty"`
	Columns    []Column    `json:"columns"`
	Candidates []Candidate `json:"candidates"`
	// Warnings concern the whole file; Errors list parts of the file that
	// could not be turned into candidates (they are left out).
	Warnings []string `json:"warnings"`
	Errors   []string `json:"errors"`
}

func (r *Result) warn(format string, a ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, a...))
}
func (r *Result) errorf(format string, a ...any) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, a...))
}

// Parse detects the format and parses data. Errors wrapping ErrInvalid
// describe unusable files or options.
func Parse(data []byte, filename string, opt Options) (res *Result, err error) {
	defer func() {
		// Third-party parsers must never take the server down on odd input.
		if p := recover(); p != nil {
			res, err = nil, invalidf("the file could not be read (%v)", p)
		}
	}()
	if err := checkMapping(opt.Mapping); err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, invalidf("the file is empty")
	}
	format, err := DetectFormat(opt.Format, filename, data)
	if err != nil {
		return nil, err
	}
	res = &Result{Format: format, Columns: []Column{}, Candidates: []Candidate{}, Warnings: []string{}, Errors: []string{}}
	switch format {
	case FormatCSV:
		err = parseCSV(data, opt, res)
	case FormatXLSX:
		err = parseXLSX(data, opt, res)
	case FormatReqIF:
		err = parseReqIF(data, opt, res)
	case FormatDOCX:
		err = parseDOCX(data, opt, res)
	}
	if err != nil {
		return nil, err
	}
	// JSON clients expect arrays, never null.
	if res.Columns == nil {
		res.Columns = []Column{}
	}
	if res.Candidates == nil {
		res.Candidates = []Candidate{}
	}
	if res.Errors == nil {
		res.Errors = []string{}
	}
	max := opt.MaxCandidates
	if max <= 0 {
		max = DefaultMaxCandidates
	}
	if len(res.Candidates) > max {
		return nil, invalidf("the file contains %d requirements; at most %d can be imported at once", len(res.Candidates), max)
	}
	if len(res.Candidates) == 0 && len(res.Errors) == 0 {
		res.warn("no requirements were found in the file")
	}
	return res, nil
}

// DetectFormat returns the format named by explicit, else by the file
// extension, else by sniffing data.
func DetectFormat(explicit, filename string, data []byte) (string, error) {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(explicit), ".")) {
	case "":
	case "csv", "txt", "tsv":
		return FormatCSV, nil
	case "xlsx", "xlsm", "excel":
		return FormatXLSX, nil
	case "reqif", "reqifz", "xml":
		return FormatReqIF, nil
	case "docx", "word":
		return FormatDOCX, nil
	default:
		return "", invalidf("unsupported format %q (want csv, xlsx, reqif or docx)", explicit)
	}
	switch strings.ToLower(path.Ext(filename)) {
	case ".csv", ".txt", ".tsv":
		return FormatCSV, nil
	case ".xlsx", ".xlsm":
		return FormatXLSX, nil
	case ".reqif", ".reqifz":
		return FormatReqIF, nil
	case ".docx":
		return FormatDOCX, nil
	case ".xls", ".doc":
		return "", invalidf("old binary Office files (.xls, .doc) are not supported; save the file as .xlsx or .docx")
	}
	if isZip(data) {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", invalidf("the file is a damaged ZIP archive")
		}
		for _, f := range zr.File {
			switch {
			case f.Name == "word/document.xml":
				return FormatDOCX, nil
			case f.Name == "xl/workbook.xml":
				return FormatXLSX, nil
			case strings.HasSuffix(strings.ToLower(f.Name), ".reqif"):
				return FormatReqIF, nil
			}
		}
		return "", invalidf("unknown ZIP content; expected .xlsx, .docx or .reqifz")
	}
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}
	if bytes.Contains(head, []byte("REQ-IF")) {
		return FormatReqIF, nil
	}
	return FormatCSV, nil
}

func isZip(data []byte) bool { return bytes.HasPrefix(data, []byte("PK\x03\x04")) }

// readZipEntry reads f with the decompression limit.
func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, invalidf("%s: %v", f.Name, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, maxUnzipped+1))
	if err != nil {
		return nil, invalidf("%s: %v", f.Name, err)
	}
	if len(b) > maxUnzipped {
		return nil, invalidf("%s is larger than %d MB when unpacked", f.Name, maxUnzipped>>20)
	}
	return b, nil
}

func openZip(data []byte, what string) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, invalidf("not a valid %s file (%v)", what, err)
	}
	return zr, nil
}

func zipFile(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
