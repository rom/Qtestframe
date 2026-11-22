package interop

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"github.com/rom/qtestframe/internal/model"
)

// The round-trip workbook:
//
//	Requirements | Tests   one row per item: ID, Key, Version, then one
//	                        column per field (relations and links as keys)
//	Steps                   (tests) one row per step: Test, Step, Action,
//	                        Data, Expected result, Call test
//	Instructions            how to edit the workbook
//	_qtf (hidden)           format marker, kind, project and the field key
//	                        of every column
//
// Columns may be reordered or deleted (a missing column changes nothing);
// rows without an ID are new items.

// Workbook format marker and version.
const (
	WorkbookMarker  = "qtestframe-roundtrip"
	WorkbookVersion = 1
	metaSheet       = "_qtf"
	stepsSheet      = "Steps"
	maxCell         = 32767
)

// Fixed columns.
const (
	colID      = "ID"
	colKey     = "Key"
	colVersion = "Version"
)

// Workbook is the input of WriteWorkbook.
type Workbook struct {
	Kind        string
	ProjectID   string
	ProjectKey  string
	ProjectName string
	Fields      []Field
	Items       []*Item
	GeneratedAt time.Time
}

// SheetName returns the main sheet of kind.
func SheetName(kind string) string {
	if kind == KindTest {
		return "Tests"
	}
	return "Requirements"
}

func cellText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	var b strings.Builder
	for _, r := range s {
		if r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || r >= 0x10000 {
			b.WriteRune(r)
		}
	}
	s = b.String()
	if utf8.RuneCountInString(s) > maxCell {
		s = string([]rune(s)[:maxCell])
	}
	// cells are written as strings, which Excel never evaluates as formulas
	return s
}

// WriteWorkbook writes the round-trip workbook.
func WriteWorkbook(w io.Writer, wb Workbook) error {
	f := excelize.NewFile()
	defer f.Close()
	main := SheetName(wb.Kind)
	if err := f.SetSheetName("Sheet1", main); err != nil {
		return err
	}
	header, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"305496"}},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	if err != nil {
		return err
	}
	fixedHeader, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"595959"}},
	})
	if err != nil {
		return err
	}
	locked, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "595959"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EDEDED"}},
		Alignment: &excelize.Alignment{Vertical: "top"},
	})
	if err != nil {
		return err
	}
	wrap, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	if err != nil {
		return err
	}

	cols := []string{colID, colKey, colVersion}
	for _, fl := range wb.Fields {
		cols = append(cols, fl.Label)
	}
	row := make([]any, len(cols))
	for i, c := range cols {
		row[i] = c
	}
	if err := f.SetSheetRow(main, "A1", &row); err != nil {
		return err
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = utf8.RuneCountInString(c) + 2
	}
	for r, it := range wb.Items {
		vals := make([]any, len(cols))
		vals[0], vals[1], vals[2] = it.ID(), it.Key(), it.Version()
		for i := range wb.Fields {
			v := ""
			if wb.Fields[i].Get != nil {
				v = wb.Fields[i].Get(it)
			}
			vals[i+3] = cellText(v)
			for _, l := range strings.Split(v, "\n") {
				if n := utf8.RuneCountInString(l) + 2; n > widths[i+3] {
					widths[i+3] = n
				}
			}
		}
		if n := utf8.RuneCountInString(it.Key()) + 2; n > widths[1] {
			widths[1] = n
		}
		ref, _ := excelize.CoordinatesToCellName(1, r+2)
		if err := f.SetSheetRow(main, ref, &vals); err != nil {
			return err
		}
	}
	last, _ := excelize.CoordinatesToCellName(len(cols), 1)
	_ = f.SetCellStyle(main, "A1", last, header)
	_ = f.SetCellStyle(main, "A1", "C1", fixedHeader)
	n := len(wb.Items) + 1
	if len(wb.Items) > 0 {
		lastData, _ := excelize.CoordinatesToCellName(len(cols), n)
		_ = f.SetCellStyle(main, "D2", lastData, wrap)
		_ = f.SetCellStyle(main, "A2", fmt.Sprintf("C%d", n), locked)
	}
	for i, fl := range wb.Fields {
		col, _ := excelize.ColumnNumberToName(i + 4)
		if fl.ReadOnly {
			_ = f.SetCellStyle(main, col+"1", col+"1", fixedHeader)
			if len(wb.Items) > 0 {
				_ = f.SetCellStyle(main, col+"2", fmt.Sprintf("%s%d", col, n), locked)
			}
		}
		if fl.Type == Enum && len(fl.Values) > 0 && len(strings.Join(fl.Values, ",")) < 250 {
			dv := excelize.NewDataValidation(true)
			dv.SetSqref(fmt.Sprintf("%s2:%s%d", col, col, n+1000))
			if err := dv.SetDropList(fl.Values); err == nil {
				dv.SetError(excelize.DataValidationErrorStyleWarning, fl.Label, "Use one of: "+strings.Join(fl.Values, ", "))
				_ = f.AddDataValidation(main, dv)
			}
		}
	}
	for i, wd := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		w := float64(wd)
		switch {
		case i == 0:
			w = 12 // the id is for the importer
		case w < 10:
			w = 10
		case w > 60:
			w = 60
		}
		_ = f.SetColWidth(main, col, col, w)
	}
	_ = f.SetPanes(main, &excelize.Panes{Freeze: true, XSplit: 2, YSplit: 1, TopLeftCell: "C2", ActivePane: "bottomRight",
		Selection: []excelize.Selection{{SQRef: "C2", ActiveCell: "C2", Pane: "bottomRight"}}})
	lastRef, _ := excelize.CoordinatesToCellName(len(cols), n)
	_ = f.AutoFilter(main, "A1:"+lastRef, nil)

	if wb.Kind == KindTest {
		if err := writeSteps(f, wb, header, locked, wrap); err != nil {
			return err
		}
	}
	if err := writeInstructions(f, wb); err != nil {
		return err
	}
	if err := writeMeta(f, wb); err != nil {
		return err
	}
	f.SetActiveSheet(0)
	return f.Write(w)
}

var stepColumns = []string{"Test ID", "Test", "Step", "Action", "Data", "Expected result", "Call test"}

func writeSteps(f *excelize.File, wb Workbook, header, locked, wrap int) error {
	if _, err := f.NewSheet(stepsSheet); err != nil {
		return err
	}
	row := make([]any, len(stepColumns))
	for i, c := range stepColumns {
		row[i] = c
	}
	if err := f.SetSheetRow(stepsSheet, "A1", &row); err != nil {
		return err
	}
	r := 2
	for _, it := range wb.Items {
		if it.Test == nil {
			continue
		}
		for i, st := range it.Test.Steps {
			call := ""
			if st.CallTestID != nil {
				call = st.CallTestKey
			}
			vals := []any{it.ID(), it.Key(), i + 1, cellText(st.Action), cellText(st.Data), cellText(st.Expected), call}
			ref, _ := excelize.CoordinatesToCellName(1, r)
			if err := f.SetSheetRow(stepsSheet, ref, &vals); err != nil {
				return err
			}
			r++
		}
	}
	_ = f.SetCellStyle(stepsSheet, "A1", "G1", header)
	if r > 2 {
		_ = f.SetCellStyle(stepsSheet, "A2", fmt.Sprintf("A%d", r-1), locked)
		_ = f.SetCellStyle(stepsSheet, "D2", fmt.Sprintf("F%d", r-1), wrap)
	}
	for col, w := range map[string]float64{"A": 12, "B": 14, "C": 6, "D": 50, "E": 30, "F": 50, "G": 14} {
		_ = f.SetColWidth(stepsSheet, col, col, w)
	}
	_ = f.SetPanes(stepsSheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
		Selection: []excelize.Selection{{SQRef: "A2", ActiveCell: "A2", Pane: "bottomLeft"}}})
	return nil
}

func writeInstructions(f *excelize.File, wb Workbook) error {
	const sheet = "Instructions"
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}
	what := "requirements"
	if wb.Kind == KindTest {
		what = "tests"
	}
	lines := []string{
		"Qtestframe round-trip workbook: " + wb.ProjectKey + " – " + wb.ProjectName,
		"Edit the " + what + " on the sheet " + SheetName(wb.Kind) + " and import the workbook again (Import → Edited Excel workbook). You see every change before it is saved.",
		"Do not change the columns ID and Version: they identify the item and the version you started from. If someone saved the item after the export, the row is shown as a conflict.",
		"New " + what + ": add a row and leave ID empty. A key you type (e.g. NEW-1) is only a reference within this workbook.",
		"Folder: a path such as Security/Login; missing folders are created. Relations and links: keys separated by commas.",
		"You may delete or reorder columns: a missing column changes nothing. Grey columns are for information and are not imported.",
	}
	if wb.Kind == KindTest {
		lines = append(lines, "Steps: the sheet Steps has one row per step. The steps of a test listed on the Tests sheet are replaced by its rows there (by Test ID, or by the key of a new row).")
	}
	lines = append(lines, "", "Svenska: redigera bladet, låt ID och Version vara, lämna ID tomt för nya rader och importera arbetsboken igen. Du ser alla ändringar innan de sparas.")
	for i, l := range lines {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", i+1), l)
	}
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	_ = f.SetCellStyle(sheet, "A1", "A1", bold)
	return f.SetColWidth(sheet, "A", "A", 140)
}

func writeMeta(f *excelize.File, wb Workbook) error {
	if _, err := f.NewSheet(metaSheet); err != nil {
		return err
	}
	at := wb.GeneratedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	rows := [][]any{
		{"marker", WorkbookMarker},
		{"format", WorkbookVersion},
		{"kind", wb.Kind},
		{"project_id", wb.ProjectID},
		{"project_key", wb.ProjectKey},
		{"exported_at", at.UTC().Format(time.RFC3339)},
		{"column", "key"},
		{colID, "id"}, {colKey, "key"}, {colVersion, "version"},
	}
	for _, fl := range wb.Fields {
		rows = append(rows, []any{fl.Label, fl.Key})
	}
	for i, r := range rows {
		if err := f.SetSheetRow(metaSheet, fmt.Sprintf("A%d", i+1), &r); err != nil {
			return err
		}
	}
	return f.SetSheetVisible(metaSheet, false)
}

// ---------------------------------------------------------------- reading

// Sheet is a read round-trip workbook (or any workbook with an ID or Key
// column and field columns).
type Sheet struct {
	Kind      string
	ProjectID string
	// Rows of the main sheet; IDs and versions as given.
	Rows    []SheetRow
	Columns []string // the field keys of the columns, in order
	Ignored []string // headers that are not fields
	// Steps by test reference (ID, or Key of a new row); HasSteps reports
	// a Steps sheet.
	Steps    map[string][]StepRow
	HasSteps bool
	Warnings []string
}

// SheetRow is a row of the main sheet.
type SheetRow struct {
	Row     int
	ID      string
	Key     string
	Version string
	Cells   map[string]Cell
}

// ErrNotWorkbook marks a file that is not a round-trip workbook.
type ErrNotWorkbook struct{ Msg string }

func (e *ErrNotWorkbook) Error() string { return e.Msg }

func notWorkbook(format string, a ...any) error {
	return &ErrNotWorkbook{Msg: fmt.Sprintf(format, a...)}
}

// ReadWorkbook reads a round-trip workbook. kind ("" = from the workbook)
// selects the main sheet; fields name the columns.
func ReadWorkbook(data []byte, kind string, fieldsOf func(kind string) []Field) (sh *Sheet, err error) {
	defer func() {
		if p := recover(); p != nil {
			sh, err = nil, notWorkbook("the workbook could not be read (%v)", p)
		}
	}()
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return nil, notWorkbook("not an .xlsx workbook")
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 128 << 20, UnzipXMLSizeLimit: 64 << 20})
	if err != nil {
		return nil, notWorkbook("not a valid .xlsx workbook (%v)", err)
	}
	defer f.Close()
	sh = &Sheet{Steps: map[string][]StepRow{}, Ignored: []string{}, Warnings: []string{}}
	metaKeys := map[string]string{} // header (lower) → key
	if idx, _ := f.GetSheetIndex(metaSheet); idx >= 0 {
		rows, _ := f.GetRows(metaSheet)
		inCols := false
		for _, r := range rows {
			if len(r) < 2 {
				continue
			}
			switch {
			case inCols:
				metaKeys[strings.ToLower(strings.TrimSpace(r[0]))] = strings.TrimSpace(r[1])
			case r[0] == "kind":
				sh.Kind = strings.TrimSpace(r[1])
			case r[0] == "project_id":
				sh.ProjectID = strings.TrimSpace(r[1])
			case r[0] == "column":
				inCols = true
			}
		}
	}
	if kind != "" {
		if sh.Kind != "" && sh.Kind != kind {
			return nil, notWorkbook("the workbook holds %ss, not %ss", sh.Kind, kind)
		}
		sh.Kind = kind
	}
	main := ""
	for _, name := range f.GetSheetList() {
		switch {
		case sh.Kind != "" && strings.EqualFold(name, SheetName(sh.Kind)):
			main = name
		case sh.Kind == "" && strings.EqualFold(name, SheetName(KindRequirement)):
			main, sh.Kind = name, KindRequirement
		case sh.Kind == "" && strings.EqualFold(name, SheetName(KindTest)):
			main, sh.Kind = name, KindTest
		}
		if main != "" {
			break
		}
	}
	if main == "" {
		if sh.Kind == "" {
			return nil, notWorkbook("no sheet named Requirements or Tests; export a round-trip workbook first")
		}
		main = f.GetSheetList()[0]
	}
	fields := fieldsOf(sh.Kind)
	rows, err := f.GetRows(main)
	if err != nil {
		return nil, notWorkbook("sheet %s: %v", main, err)
	}
	if len(rows) == 0 {
		return nil, notWorkbook("sheet %s is empty", main)
	}
	idCol, keyCol, verCol := -1, -1, -1
	keysOf := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		k := metaKeys[strings.ToLower(h)]
		switch {
		case k == "id" || (k == "" && strings.EqualFold(h, colID)):
			idCol = i
			continue
		case k == "key" || (k == "" && (strings.EqualFold(h, colKey) || strings.EqualFold(h, "nyckel"))):
			keyCol = i
			continue
		case k == "version" || (k == "" && strings.EqualFold(h, colVersion)):
			verCol = i
			continue
		}
		var fl *Field
		if k != "" {
			fl = FieldByKey(fields, k)
		}
		if fl == nil {
			fl = FieldByHeader(fields, h)
		}
		if fl == nil {
			sh.Ignored = append(sh.Ignored, h)
			continue
		}
		if containsStr(sh.Columns, fl.Key) {
			sh.Warnings = append(sh.Warnings, fmt.Sprintf("column %q repeats the field %s; it was ignored", h, fl.Label))
			continue
		}
		keysOf[i] = fl.Key
		sh.Columns = append(sh.Columns, fl.Key)
	}
	if idCol < 0 && keyCol < 0 {
		return nil, notWorkbook("sheet %s has no ID or Key column", main)
	}
	get := func(r []string, i int) string {
		if i >= 0 && i < len(r) {
			return strings.TrimSpace(r[i])
		}
		return ""
	}
	for ri, r := range rows[1:] {
		blank := true
		for _, c := range r {
			if strings.TrimSpace(c) != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		sr := SheetRow{Row: ri + 2, ID: get(r, idCol), Key: get(r, keyCol), Version: get(r, verCol), Cells: map[string]Cell{}}
		for i, k := range keysOf {
			if k == "" {
				continue
			}
			ref, _ := excelize.CoordinatesToCellName(i+1, ri+2)
			sr.Cells[k] = Cell{Raw: get(r, i), Ref: ref}
		}
		sh.Rows = append(sh.Rows, sr)
	}
	if sh.Kind == KindTest {
		if idx, _ := f.GetSheetIndex(stepsSheet); idx >= 0 {
			sh.HasSteps = true
			readSteps(f, sh)
		}
	}
	return sh, nil
}

func readSteps(f *excelize.File, sh *Sheet) {
	rows, err := f.GetRows(stepsSheet)
	if err != nil || len(rows) == 0 {
		return
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "test id", "id":
			col["id"] = i
		case "test", "test key", "key", "nyckel":
			col["key"] = i
		case "action", "åtgärd", "steg":
			col["action"] = i
		case "data", "testdata":
			col["data"] = i
		case "expected result", "expected", "förväntat resultat":
			col["expected"] = i
		case "call test", "anropa test":
			col["call"] = i
		}
	}
	get := func(r []string, k string) string {
		if i, ok := col[k]; ok && i < len(r) {
			return r[i]
		}
		return ""
	}
	for ri, r := range rows[1:] {
		ref := strings.TrimSpace(get(r, "id"))
		if ref == "" {
			ref = strings.TrimSpace(get(r, "key"))
		}
		if ref == "" {
			continue
		}
		sh.Steps[strings.ToLower(ref)] = append(sh.Steps[strings.ToLower(ref)], StepRow{
			Action: get(r, "action"), Data: get(r, "data"), Expected: get(r, "expected"),
			Call: strings.TrimSpace(get(r, "call")), Ref: stepsSheet + "!" + strconv.Itoa(ri+2),
		})
	}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Incoming turns the sheet rows into incoming rows: a row is matched by
// its ID, else by its Key (resolve returns the id of an item key in the
// project, "" when there is none).
func (sh *Sheet) Incoming(resolve func(key string) string, known func(id string) bool) []Incoming {
	out := make([]Incoming, 0, len(sh.Rows))
	main := SheetName(sh.Kind)
	for _, r := range sh.Rows {
		in := Incoming{Row: r.Row, Location: fmt.Sprintf("%s row %d", main, r.Row), Key: r.Key, Cells: r.Cells}
		switch {
		case r.ID != "":
			in.ID = r.ID
			if known != nil && !known(r.ID) {
				in.Errors = append(in.Errors, CellError{Field: "id", Label: colID, Cell: fmt.Sprintf("A%d", r.Row), Message: "no such item in this project; clear the ID to create a new item"})
			}
		case r.Key != "" && resolve != nil:
			in.ID = resolve(r.Key)
		}
		if r.Version != "" {
			v, err := strconv.Atoi(strings.TrimSuffix(r.Version, ".0"))
			if err != nil || v < 0 {
				in.Errors = append(in.Errors, CellError{Field: "version", Label: colVersion, Message: fmt.Sprintf("%q is not a version number", r.Version)})
			}
			in.BaseVersion = v
		}
		if sh.Kind == KindTest && sh.HasSteps {
			in.HasSteps = true
			ref := strings.ToLower(firstNonEmpty(in.ID, r.Key))
			in.Steps = sh.Steps[ref]
			if in.ID != "" && r.Key != "" && len(in.Steps) == 0 {
				in.Steps = sh.Steps[strings.ToLower(r.Key)]
			}
		}
		out = append(out, in)
	}
	return out
}

// ItemsByID indexes items.
func ItemsByID(items []*Item) map[string]*Item {
	m := make(map[string]*Item, len(items))
	for _, it := range items {
		m[it.ID()] = it
	}
	return m
}

// RequirementItem and TestItem wrap stored items.
func RequirementItem(r *model.Requirement, folder string) *Item {
	return &Item{Req: r, Folder: folder, Relations: map[string][]string{}, Extra: map[string]string{}}
}

// TestItem wraps a stored test.
func TestItem(t *model.TestCase, folder string) *Item {
	return &Item{Test: t, Folder: folder, Relations: map[string][]string{}, Extra: map[string]string{}}
}
