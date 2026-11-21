package report

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/query"
)

// Entities of custom reports.
const (
	EntityRequirement = "requirement"
	EntityTest        = "test"
	EntityRun         = "run"
	EntityExecution   = "execution"
)

// Entities lists the entity types a custom report can list.
var Entities = []string{EntityRequirement, EntityTest, EntityRun, EntityExecution}

// MaxColumns is the maximum number of columns of a custom report.
const MaxColumns = 40

// CustomConfig defines a custom report (see docs/api.md "Reports").
type CustomConfig struct {
	Entity         string   `json:"entity"`
	Query          string   `json:"query"`
	Columns        []string `json:"columns"`
	GroupBy        string   `json:"group_by"`
	SortBy         string   `json:"sort_by"`
	SortDesc       bool     `json:"sort_desc"`
	IncludeSummary bool     `json:"include_summary"`
	Title          string   `json:"title"`
	RunID          string   `json:"run_id"`
}

// Field is a column of a custom report.
type Field struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// multi-line fields join their values with newlines
	lines bool
}

func f(id, label string) Field { return Field{ID: id, Label: label} }

// Field ids follow the query schemas of the list endpoints; run and
// execution add a few derived columns (counts, times, durations).
var fields = map[string][]Field{
	EntityRequirement: {
		f("key", "Key"), f("title", "Title"), f("description", "Description"), f("obligation", "Obligation"),
		f("type", "Type"), f("priority", "Priority"), f("status", "Status"), f("source", "Source"),
		f("rationale", "Rationale"), f("owner", "Owner"), f("tag", "Tags"), f("version", "Version"),
		f("folder", "Folder"), f("category", "Categories"), f("dimension", "Categories with dimension"),
		f("coverage", "Coverage"), f("linked", "Linked tests"), f("created_by", "Created by"),
		f("created_at", "Created"), f("updated_at", "Updated"), {ID: "criteria", Label: "Evaluation criteria", lines: true},
		f("id", "ID"), f("related", "Related requirements"), f("parent", "Parent requirements"),
		f("child", "Child requirements"), f("comments", "Comments"),
	},
	EntityTest: {
		f("key", "Key"), f("title", "Title"), f("description", "Description"), f("preconditions", "Preconditions"),
		f("execution_type", "Execution type"), f("script_language", "Script language"), {ID: "script", Label: "Script", lines: true},
		f("expected", "Expected result"), {ID: "step", Label: "Steps", lines: true}, f("priority", "Priority"),
		f("status", "Status"), f("estimate", "Estimate (minutes)"), f("tag", "Tags"), f("folder", "Folder"),
		f("category", "Categories"), f("dimension", "Categories with dimension"), f("linked", "Linked requirements"),
		f("last_result", "Last result"), f("created_by", "Created by"), f("created_at", "Created"),
		f("updated_at", "Updated"), f("id", "ID"), f("defect", "Defects"), f("calls", "Called tests"),
		f("comments", "Comments"), f("version", "Version"),
	},
	EntityRun: {
		f("name", "Name"), f("description", "Description"), f("status", "Status"), f("environment", "Environment"),
		f("build", "Build"), f("created_by", "Created by"), f("created_at", "Created"), f("id", "ID"),
		f("started_at", "Started"), f("completed_at", "Completed"), f("total", "Executions"),
		f("passed", "Passed"), f("failed", "Failed"), f("blocked", "Blocked"), f("skipped", "Skipped"),
		f("not_run", "Not run"), f("pass_rate", "Pass rate"),
	},
	EntityExecution: {
		f("key", "Test key"), f("title", "Test title"), f("status", "Result"), f("assignee", "Assignee"),
		f("executed_by", "Executed by"), f("notes", "Notes"), {ID: "output", Label: "Output", lines: true}, f("id", "ID"),
		f("defect", "Defects"), f("run", "Run"), f("started_at", "Started"), f("finished_at", "Finished"),
		f("duration_ms", "Duration (ms)"),
	},
}

// Fields returns the available columns of an entity type (nil if unknown).
func Fields(entity string) []Field {
	fs, ok := fields[entity]
	if !ok {
		return nil
	}
	return append([]Field{}, fs...)
}

func field(entity, id string) (Field, bool) {
	for _, fl := range fields[entity] {
		if fl.ID == id {
			return fl, true
		}
	}
	return Field{}, false
}

// ParseCustomConfig decodes and validates a CustomConfig.
func ParseCustomConfig(raw json.RawMessage) (*CustomConfig, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, invalidf("report config is required")
	}
	var c CustomConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, invalidf("invalid report config: %v", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate normalises and checks the configuration (the query syntax and
// the run are checked by the caller, which knows the schemas and data).
func (c *CustomConfig) Validate() error {
	c.Entity = strings.ToLower(strings.TrimSpace(c.Entity))
	c.Query = strings.TrimSpace(c.Query)
	c.GroupBy = strings.TrimSpace(c.GroupBy)
	c.SortBy = strings.TrimSpace(c.SortBy)
	c.Title = strings.TrimSpace(c.Title)
	c.RunID = strings.TrimSpace(c.RunID)
	if _, ok := fields[c.Entity]; !ok {
		return invalidf("entity must be one of %s", strings.Join(Entities, ", "))
	}
	if len(c.Columns) == 0 || len(c.Columns) > MaxColumns {
		return invalidf("a report needs 1 to %d columns", MaxColumns)
	}
	seen := map[string]bool{}
	for i, col := range c.Columns {
		col = strings.TrimSpace(col)
		c.Columns[i] = col
		if _, ok := field(c.Entity, col); !ok {
			return invalidf("unknown column %q for entity %s", col, c.Entity)
		}
		if seen[col] {
			return invalidf("column %q is listed twice", col)
		}
		seen[col] = true
	}
	if c.GroupBy != "" {
		if _, ok := field(c.Entity, c.GroupBy); !ok {
			return invalidf("unknown group_by field %q for entity %s", c.GroupBy, c.Entity)
		}
	}
	if c.SortBy != "" {
		if _, ok := field(c.Entity, c.SortBy); !ok {
			return invalidf("unknown sort_by field %q for entity %s", c.SortBy, c.Entity)
		}
	}
	if c.RunID != "" && c.Entity != EntityExecution {
		return invalidf("run_id is only valid for entity execution")
	}
	if len([]rune(c.Title)) > 200 {
		return invalidf("title is too long (max 200 characters)")
	}
	return nil
}

var entityTitles = map[string]string{EntityRequirement: "Requirements", EntityTest: "Test cases", EntityRun: "Test runs", EntityExecution: "Executions"}

// enumFields are the fields whose values are enumerations, per entity:
// translated reports show their labels instead of the stored values.
var enumFields = map[string]map[string]string{
	EntityRequirement: {"obligation": EnumObligation, "type": EnumReqType, "priority": EnumPriority, "status": EnumStatus, "coverage": EnumCoverage},
	EntityTest:        {"execution_type": EnumExecution, "priority": EnumPriority, "status": EnumStatus, "last_result": EnumResult},
	EntityRun:         {"status": EnumStatus},
	EntityExecution:   {"status": EnumResult},
}

// localize returns the records with enumeration values replaced by their
// labels (English reports keep the stored values).
func (l loc) localize(entity string, recs []query.MapRecord) []query.MapRecord {
	if l.lang == English {
		return append([]query.MapRecord{}, recs...)
	}
	out := make([]query.MapRecord, len(recs))
	for i, rec := range recs {
		cp := make(query.MapRecord, len(rec))
		for k, vs := range rec {
			kind, ok := enumFields[entity][k]
			if !ok {
				cp[k] = vs
				continue
			}
			lv := make([]string, len(vs))
			for j, v := range vs {
				lv[j] = l.enum(kind, v)
			}
			cp[k] = lv
		}
		out[i] = cp
	}
	return out
}

// noGroup labels records without a value in the group_by field.
const noGroup = "(none)"

func value(rec query.MapRecord, fl Field) string {
	sep := ", "
	if fl.lines {
		sep = "\n"
	}
	return strings.Join(rec[fl.ID], sep)
}

// BuildCustom renders records (already filtered by the configuration's
// query) as a dataset in language lang (see Languages; "" = English).
// subtitle is typically the project name.
func BuildCustom(c CustomConfig, recs []query.MapRecord, subtitle string, now time.Time, lang string) *export.Dataset {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	l := newLoc(lang)
	entity := l.t(entityTitles[c.Entity])
	title := c.Title
	if title == "" {
		title = l.t("Custom report – ") + entity
	}
	if c.Query != "" {
		subtitle += l.t(" · Filter: ") + c.Query
	}
	ds := &export.Dataset{Title: title, Subtitle: subtitle, GeneratedAt: now.UTC(), Lang: l.lang}
	cols := make([]Field, len(c.Columns))
	header := make([]string, len(c.Columns))
	for i, id := range c.Columns {
		cols[i], _ = field(c.Entity, id)
		header[i] = l.t(cols[i].Label)
	}
	recs = l.localize(c.Entity, recs)
	if c.SortBy != "" {
		sf, _ := field(c.Entity, c.SortBy)
		sort.SliceStable(recs, func(i, j int) bool {
			cmp := naturalCompare(value(recs[i], sf), value(recs[j], sf))
			if c.SortDesc {
				return cmp > 0
			}
			return cmp < 0
		})
	}
	row := func(rec query.MapRecord) []string {
		out := make([]string, len(cols))
		for i, fl := range cols {
			out[i] = value(rec, fl)
		}
		return out
	}
	if c.GroupBy == "" {
		sec := export.Section{Name: c.Entity, Title: entity, Columns: header}
		for _, rec := range recs {
			sec.Rows = append(sec.Rows, row(rec))
		}
		if c.IncludeSummary {
			ds.Sections = append(ds.Sections, l.kpiSection("summary", "Summary",
				kv{"Entity", entity}, kv{"Filter", c.Query}, kv{entityTitles[c.Entity], itoa(len(recs))}))
		}
		ds.Sections = append(ds.Sections, sec)
		return ds
	}
	gf, _ := field(c.Entity, c.GroupBy)
	groups := map[string][]query.MapRecord{}
	for _, rec := range recs {
		vals := uniqSorted(rec[gf.ID])
		if len(vals) == 0 {
			vals = []string{noGroup}
		}
		for _, v := range vals {
			groups[v] = append(groups[v], rec)
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if (keys[i] == noGroup) != (keys[j] == noGroup) {
			return keys[j] == noGroup
		}
		return naturalLess(keys[i], keys[j])
	})
	groupLabel := l.t(gf.Label)
	shown := func(k string) string {
		if k == noGroup {
			return l.t(noGroup)
		}
		return k
	}
	if c.IncludeSummary {
		sum := export.Section{Name: "summary", Title: l.t("Summary by ") + strings.ToLower(groupLabel), Columns: []string{groupLabel, l.t("Count"), l.t("Share")}}
		for _, k := range keys {
			sum.Rows = append(sum.Rows, []string{shown(k), itoa(len(groups[k])), l.pct(len(groups[k]), len(recs))})
		}
		sum.Rows = append(sum.Rows, []string{l.t("Total ") + strings.ToLower(entity), itoa(len(recs)), l.pct(len(recs), len(recs))})
		ds.Sections = append(ds.Sections, sum)
	}
	for _, k := range keys {
		sec := export.Section{Name: shown(k), Title: groupLabel + ": " + shown(k), Columns: header}
		for _, rec := range groups[k] {
			sec.Rows = append(sec.Rows, row(rec))
		}
		ds.Sections = append(ds.Sections, sec)
	}
	if len(keys) == 0 {
		ds.Sections = append(ds.Sections, export.Section{Name: c.Entity, Title: entity, Columns: header})
	}
	return ds
}
