package reqimport

import (
	"strconv"
	"strings"
)

// Mapping presets for the CSV and Excel exports of other requirement tools,
// on top of the generic column guessing: the preset's mapping applies to
// the columns it names (an explicit mapping still wins), and with Headings
// the heading rows of the export become folders, nested by the object
// level or outline number, instead of requirements.

// Preset is a column mapping for the exports of one tool.
type Preset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Mapping maps column headers (case-insensitive) to fields; "" ignores
	// the column.
	Mapping map[string]string `json:"mapping"`
	// Headings turns heading rows into folders: rows with a title and no
	// text (DOORS "Object Heading" rows), or whose type is one of
	// HeadingTypes (Polarion "Heading" work items).
	Headings     bool     `json:"headings"`
	HeadingTypes []string `json:"heading_types,omitempty"`
}

// Presets lists the mapping presets.
var Presets = []Preset{
	{
		ID: "doors", Name: "IBM DOORS (CSV / Excel export)", Headings: true,
		Mapping: map[string]string{
			"Absolute Number":       FieldExternalID,
			"Object Identifier":     FieldExternalID,
			"ID":                    FieldExternalID,
			"Object Heading":        FieldTitle,
			"Object Short Text":     FieldTitle,
			"Object Text":           FieldDescription,
			"Object Level":          "",
			"Object Number":         "",
			"Created By":            "",
			"Created On":            "",
			"Created Thru":          "",
			"Last Modified By":      "",
			"Last Modified On":      "",
			"Object Type":           FieldType,
			"Priority":              FieldPriority,
			"Status":                FieldStatus,
			"Owner":                 FieldOwner,
			"Source":                FieldSource,
			"Rationale":             FieldRationale,
			"Acceptance Criteria":   FieldCriteria,
			"Verification Criteria": FieldCriteria,
		},
	},
	{
		ID: "polarion", Name: "Siemens Polarion (Excel / CSV export)", Headings: true, HeadingTypes: []string{"heading"},
		Mapping: map[string]string{
			"ID":                  FieldExternalID,
			"Title":               FieldTitle,
			"Description":         FieldDescription,
			"Status":              FieldStatus,
			"Type":                FieldType,
			"Priority":            FieldPriority,
			"Severity":            FieldObligation,
			"Assignee":            FieldOwner,
			"Assignees":           FieldOwner,
			"Author":              "",
			"Created":             "",
			"Updated":             "",
			"Resolution":          "",
			"Outline Number":      "",
			"Linked Work Items":   "",
			"Project":             "",
			"Space":               "",
			"Document":            FieldFolder,
			"Categories":          FieldTags,
			"Initial Estimate":    "",
			"Remaining Estimate":  "",
			"Time Spent":          "",
			"Acceptance Criteria": FieldCriteria,
		},
	},
}

// presetOf returns the preset with id ("" = none).
func presetOf(id string) (*Preset, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || id == "none" || id == "auto" {
		return nil, nil
	}
	for i := range Presets {
		if Presets[i].ID == id {
			return &Presets[i], nil
		}
	}
	ids := make([]string, len(Presets))
	for i, p := range Presets {
		ids[i] = p.ID
	}
	return nil, invalidf("unknown preset %q (want %s)", id, strings.Join(ids, ", "))
}

// merge returns the preset mapping overridden by explicit.
func (p *Preset) merge(explicit map[string]string) map[string]string {
	out := map[string]string{}
	for h, f := range p.Mapping {
		if f == "" {
			f = "ignore"
		}
		out[h] = f
	}
	for h, f := range explicit {
		for k := range out {
			if strings.EqualFold(k, h) {
				delete(out, k)
			}
		}
		out[h] = f
	}
	return out
}

// hierarchyCell records the hierarchy columns of tool exports.
func hierarchyCell(c *Candidate, header, v string) {
	if v == "" {
		return
	}
	switch normHeader(header) {
	case "object level", "level number", "outline level":
		if n, err := strconv.Atoi(strings.TrimSuffix(v, ".0")); err == nil && n > 0 {
			c.level = n
		}
	case "object number", "outline number", "section number", "paragraph number":
		c.outline = v
	}
}

// outlineLevel is the level of an outline number: "1.2.3" → 3; Polarion's
// "1.2-1" (the first item below heading 1.2) → 3.
func outlineLevel(s string) int {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if s == "" {
		return 0
	}
	n := 1 + strings.Count(s, ".")
	if strings.Contains(s, "-") {
		n++
	}
	return n
}

// applyHeadings turns heading candidates into folders of the candidates
// that follow them.
func applyHeadings(res *Result, p *Preset) {
	var stack []string // heading names by level (index 0 = level 1)
	out := res.Candidates[:0]
	headings := 0
	for _, c := range res.Candidates {
		level := c.level
		if level == 0 {
			level = outlineLevel(c.outline)
		}
		heading := c.Description == "" && len(c.Criteria) == 0
		if len(p.HeadingTypes) > 0 {
			heading = containsFold(p.HeadingTypes, strings.TrimSpace(c.rawType))
		}
		if heading {
			name := strings.ReplaceAll(strings.TrimSpace(c.Title), "/", "-")
			// "1.2 Scope" in the heading text: the number gives the level
			if level == 0 {
				if f := strings.Fields(name); len(f) > 1 && outlineLevel(f[0]) > 0 && strings.Trim(f[0], "0123456789.") == "" {
					level = outlineLevel(f[0])
				}
			}
			if level <= 0 {
				level = len(stack) + 1
			}
			if level-1 < len(stack) {
				stack = stack[:level-1]
			}
			for len(stack) < level-1 {
				stack = append(stack, "")
			}
			stack = append(stack, name)
			headings++
			continue
		}
		depth := len(stack)
		if level > 0 && level-1 < depth {
			depth = level - 1
		}
		c.Folder = joinFolder(append(append([]string{}, stack[:depth]...), c.Folder)...)
		out = append(out, c)
	}
	res.Candidates = out
	if headings > 0 {
		res.warn("%d heading rows became folders", headings)
	}
}
