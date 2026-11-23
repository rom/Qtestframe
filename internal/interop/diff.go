package interop

import (
	"fmt"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// Row statuses of a diff.
const (
	StatusNew       = "new"
	StatusChanged   = "changed"
	StatusUnchanged = "unchanged"
	StatusConflict  = "conflict"
	StatusError     = "error"
)

// Cell is a value of a row as it is in the file.
type Cell struct {
	Raw string
	// Ref locates the cell for messages: "D7", or an attribute name.
	Ref string
}

// StepRow is a test step of a row (the Steps sheet).
type StepRow struct {
	Action, Data, Expected string
	// Call is the key of a called test ("" for a normal step).
	Call string
	Ref  string
}

// Incoming is an item as the file has it.
type Incoming struct {
	// Row is the sheet row or the object's number; Location its readable
	// form ("Requirements row 7", "SPEC-OBJECT REQ-12").
	Row      int
	Location string
	// ID of the stored item this row updates ("" = a new item).
	ID string
	// Key as written in the file; on a new row of a workbook it is a
	// temporary reference the Steps sheet can use.
	Key string
	// Ref is the identifier in the other tool (ReqIF IDENTIFIER).
	Ref string
	// BaseVersion is the version the file was made from (0 = unknown).
	BaseVersion int
	// Cells by field key: only the fields the file has.
	Cells map[string]Cell
	// Steps of a test, when the file has them (HasSteps).
	Steps    []StepRow
	HasSteps bool
	// Errors found while reading the row (an unknown ID …).
	Errors []CellError
	// Warnings about the row.
	Warnings []string
}

// Change is a changed value.
type Change struct {
	Field string `json:"field"`
	Label string `json:"label"`
	Cell  string `json:"cell"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// CellError is a value that cannot be imported.
type CellError struct {
	Field   string `json:"field"`
	Label   string `json:"label"`
	Cell    string `json:"cell"`
	Message string `json:"message"`
}

// RowDiff is the outcome for one row.
type RowDiff struct {
	// Index is the position of the row in the diff (also in apply's
	// exclude list).
	Index       int         `json:"index"`
	Row         int         `json:"row"`
	Location    string      `json:"location"`
	Status      string      `json:"status"`
	ID          string      `json:"id,omitempty"`
	Key         string      `json:"key,omitempty"`
	Ref         string      `json:"ref,omitempty"`
	Title       string      `json:"title"`
	BaseVersion int         `json:"base_version,omitempty"`
	Version     int         `json:"version,omitempty"`
	Changes     []Change    `json:"changes"`
	Errors      []CellError `json:"errors"`
	Warnings    []string    `json:"warnings"`

	// Values are the canonical values to write (the changed fields of an
	// existing item, every given field of a new one).
	Values map[string]string `json:"-"`
	// Steps to write (StepsChanged).
	Steps        []model.TestStep `json:"-"`
	StepsChanged bool             `json:"-"`
}

// Summary counts the rows by status.
type Summary struct {
	New       int `json:"new"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	Conflicts int `json:"conflicts"`
	Errors    int `json:"errors"`
}

// Diff compares a file with the stored items.
type Diff struct {
	Kind    string    `json:"kind"`
	Rows    []RowDiff `json:"rows"`
	Summary Summary   `json:"summary"`
	// Columns are the fields the file has (in file order).
	Columns []Field `json:"columns"`
	// Ignored lists the column headers that are not fields.
	Ignored  []string `json:"ignored"`
	Warnings []string `json:"warnings"`
}

// Lookup resolves an item key of kind in the project to its id.
type Lookup func(kind, key string) (id string, ok bool)

// Compare builds the diff of rows against the stored items (by id).
func Compare(kind string, fields []Field, rows []Incoming, current map[string]*Item, lookup Lookup) *Diff {
	d := &Diff{Kind: kind, Rows: make([]RowDiff, 0, len(rows)), Ignored: []string{}, Warnings: []string{}}
	seen := map[string]int{}
	for i, in := range rows {
		rd := compareRow(kind, fields, in, current, lookup)
		rd.Index = i
		if in.ID != "" {
			if j, dup := seen[in.ID]; dup {
				rd.Errors = append(rd.Errors, CellError{Field: "id", Label: "ID", Message: fmt.Sprintf("the item is also on %s", rows[j].Location)})
				rd.Status = StatusError
			}
			seen[in.ID] = i
		}
		switch rd.Status {
		case StatusNew:
			d.Summary.New++
		case StatusChanged:
			d.Summary.Changed++
		case StatusUnchanged:
			d.Summary.Unchanged++
		case StatusConflict:
			d.Summary.Conflicts++
		default:
			d.Summary.Errors++
		}
		d.Rows = append(d.Rows, rd)
	}
	return d
}

func compareRow(kind string, fields []Field, in Incoming, current map[string]*Item, lookup Lookup) RowDiff {
	rd := RowDiff{Row: in.Row, Location: in.Location, ID: in.ID, Key: in.Key, Ref: in.Ref, BaseVersion: in.BaseVersion,
		Changes: []Change{}, Errors: append([]CellError{}, in.Errors...), Warnings: append([]string{}, in.Warnings...), Values: map[string]string{}}
	var cur *Item
	if in.ID != "" {
		cur = current[in.ID]
		if cur == nil {
			rd.Errors = append(rd.Errors, CellError{Field: "id", Label: "ID", Message: "no such item in this project"})
		} else {
			rd.Key, rd.Version, rd.Title = cur.Key(), cur.Version(), cur.Title()
		}
	}
	for i := range fields {
		f := &fields[i]
		c, ok := in.Cells[f.Key]
		if !ok || f.ReadOnly {
			continue
		}
		v, err := parseValue(f, c.Raw)
		if err == nil && f.Type == Keys {
			err = checkKeys(kind, f, v, rd.Key, lookup)
		}
		if err == nil && f.Key == "title" && v == "" && (cur != nil || in.ID == "") {
			err = errEmpty
		}
		if err != nil {
			rd.Errors = append(rd.Errors, CellError{Field: f.Key, Label: f.Label, Cell: c.Ref, Message: err.Error()})
			continue
		}
		if f.Check != nil {
			var warn string
			if v, warn = f.Check(v); warn != "" {
				w := f.Label + ": " + warn
				if c.Ref != "" {
					w = c.Ref + " " + w
				}
				rd.Warnings = append(rd.Warnings, w)
			}
		}
		if cur == nil {
			if v != "" {
				rd.Values[f.Key] = v
				rd.Changes = append(rd.Changes, Change{Field: f.Key, Label: f.Label, Cell: c.Ref, New: v})
			}
			continue
		}
		old := ""
		if f.Get != nil {
			old = f.Get(cur)
		}
		if !equalValues(f, old, v) {
			rd.Values[f.Key] = v
			rd.Changes = append(rd.Changes, Change{Field: f.Key, Label: f.Label, Cell: c.Ref, Old: old, New: v})
		}
	}
	if in.ID == "" {
		if t := rd.Values["title"]; t != "" {
			rd.Title = t
		} else if _, has := in.Cells["title"]; !has {
			rd.Errors = append(rd.Errors, CellError{Field: "title", Label: "Title", Message: "a new item needs a title"})
		}
	}
	if kind == KindTest && in.HasSteps {
		steps, errs := buildSteps(in.Steps, lookup)
		rd.Errors = append(rd.Errors, errs...)
		var old []model.TestStep
		if cur != nil && cur.Test != nil {
			old = cur.Test.Steps
		}
		if len(errs) == 0 && !stepsEqual(old, steps) {
			// keep the ids of the steps at the same position
			for i := range steps {
				if i < len(old) {
					steps[i].ID = old[i].ID
				}
			}
			rd.Steps, rd.StepsChanged = steps, true
			rd.Changes = append(rd.Changes, Change{Field: "steps", Label: "Steps", Old: stepsText(old), New: stepsText(steps)})
		}
	}
	switch {
	case len(rd.Errors) > 0:
		rd.Status = StatusError
	case cur == nil:
		rd.Status = StatusNew
	case len(rd.Changes) == 0:
		rd.Status = StatusUnchanged
	case rd.BaseVersion > 0 && rd.BaseVersion != rd.Version:
		rd.Status = StatusConflict
	default:
		rd.Status = StatusChanged
	}
	return rd
}

// checkKeys checks the keys of a relation or link field.
func checkKeys(kind string, f *Field, v, self string, lookup Lookup) error {
	target := KindRequirement
	var bad []string
	for _, k := range SplitList(v) {
		if self != "" && strings.EqualFold(k, self) && f.Relation != "" {
			return fmt.Errorf("a requirement cannot be related to itself")
		}
		if lookup == nil {
			continue
		}
		if _, ok := lookup(target, k); !ok {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("unknown requirement %s", strings.Join(bad, ", "))
	}
	_ = kind
	return nil
}

func buildSteps(rows []StepRow, lookup Lookup) ([]model.TestStep, []CellError) {
	out := []model.TestStep{}
	var errs []CellError
	for _, r := range rows {
		st := model.TestStep{Action: normText(r.Action), Data: normText(r.Data), Expected: normText(r.Expected)}
		if call := strings.TrimSpace(r.Call); call != "" {
			id, ok := "", false
			if lookup != nil {
				id, ok = lookup(KindTest, call)
			}
			if !ok {
				errs = append(errs, CellError{Field: "steps", Label: "Call test", Cell: r.Ref, Message: fmt.Sprintf("unknown test %s", call)})
				continue
			}
			st.CallTestID, st.CallTestKey = &id, call
		}
		if st.Action == "" && st.CallTestID == nil {
			if st.Data != "" || st.Expected != "" {
				errs = append(errs, CellError{Field: "steps", Label: "Action", Cell: r.Ref, Message: "a step needs an action"})
			}
			continue
		}
		out = append(out, st)
	}
	return out, errs
}

func stepsEqual(a, b []model.TestStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		cx, cy := "", ""
		if x.CallTestID != nil {
			cx = *x.CallTestID
		}
		if y.CallTestID != nil {
			cy = *y.CallTestID
		}
		if normText(x.Action) != normText(y.Action) || normText(x.Data) != normText(y.Data) || normText(x.Expected) != normText(y.Expected) || cx != cy {
			return false
		}
	}
	return true
}

func stepsText(st []model.TestStep) string {
	var ls []string
	for i, s := range st {
		l := fmt.Sprintf("%d. %s", i+1, strings.Join(strings.Fields(s.Action), " "))
		if s.CallTestID != nil {
			l = fmt.Sprintf("%d. → %s", i+1, firstNonEmpty(s.CallTestKey, *s.CallTestID))
		}
		if s.Expected != "" {
			l += " ⇒ " + strings.Join(strings.Fields(s.Expected), " ")
		}
		ls = append(ls, l)
	}
	return strings.Join(ls, "\n")
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// Body builds the create or PATCH body of a row from its values.
func Body(fields []Field, rd *RowDiff, cur *Item) (map[string]any, error) {
	b := map[string]any{}
	for k, v := range rd.Values {
		f := FieldByKey(fields, k)
		if f == nil || f.Put == nil {
			continue
		}
		if err := f.Put(b, v, cur); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Label, err)
		}
	}
	if rd.StepsChanged {
		b["steps"] = rd.Steps
	}
	return b, nil
}
