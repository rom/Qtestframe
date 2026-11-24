package testimport

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Xray (Jira): the CSV of the Test Case Importer and of the Test
// Repository export (one row per step, the rows of a test share its key or
// leave key and summary empty), and the JSON of the test case import and
// of the REST export (with past test runs). Manual tests keep their steps,
// Cucumber tests their Gherkin scenario (as text and as steps), Generic
// tests their definition.

func parseXrayCSV(data []byte, res *Result) error {
	t, err := readCSV(data, "Summary", "Test Summary", "Name")
	if err != nil {
		return err
	}
	var (
		cKey    = t.col("Issue Key", "Key", "Test Key", "Issue key", "TCID", "Test ID", "Issue Id")
		cTitle  = t.col("Summary", "Test Summary", "Name")
		cDesc   = t.col("Description")
		cType   = t.col("Test Type", "TestType", "Type")
		cPrio   = t.col("Priority")
		cLabels = t.col("Labels", "Label")
		cAction = t.col("Action", "Step", "Step Action", "Manual Test Steps", "Test Step")
		cData   = t.col("Data", "Test Data", "Step Data")
		cResult = t.col("Result", "Expected Result", "Expected", "Step Expected Result")
		cGherk  = t.col("Gherkin", "Gherkin Definition", "Cucumber Scenario", "Scenario", "Cucumber Test Scenario")
		cGen    = t.col("Generic Definition", "Generic Test Definition", "Definition", "Unstructured Definition")
		cPre    = t.col("Precondition", "Preconditions", "Pre-Condition", "Pre-Conditions", "Precondition Definition")
		cPath   = t.col("Test Repository Path", "Repository Path", "Folder", "Test Repository Folder")
		cSet    = t.col("Test Set", "Test Sets")
		cReq    = t.col("Requirement", "Requirements", "Tests", "Covers", "Requirement Keys", "Linked Issues", "Issue Links")
		cStatus = t.col("Status")
		cOwner  = t.col("Assignee", "Owner")
	)
	t.col("Reporter")
	t.col("Created")
	t.col("Updated")
	t.col("Project")
	t.col("Issue Type")
	if cTitle < 0 {
		return invalidf("no Summary column: export the tests from Xray as CSV with the Summary field")
	}
	byKey := map[string]int{}
	cur := -1
	for i, row := range t.rows {
		if blank(row) {
			continue
		}
		key, title := cell(row, cKey), cell(row, cTitle)
		step := Step{Action: cell(row, cAction), Data: cell(row, cData), Expected: cell(row, cResult)}
		j, seen := byKey[strings.ToLower(key)]
		if (key == "" && title == "" && cur >= 0) || (key != "" && seen) {
			if key == "" {
				j = cur
			}
			c := &res.Cases[j]
			c.Steps = append(c.Steps, step)
			c.Refs = append(c.Refs, splitRefs(cell(row, cReq))...)
			c.Tags = append(c.Tags, splitTags(cell(row, cLabels))...)
			mergeCustom(c, t.custom(row))
			cur = j
			continue
		}
		c := Case{ExternalID: key, Title: title, Description: cell(row, cDesc), Type: cell(row, cType), RawPriority: cell(row, cPrio),
			RawStatus: cell(row, cStatus), Tags: splitTags(cell(row, cLabels)), Refs: splitRefs(cell(row, cReq)), Owner: cell(row, cOwner),
			Preconditions: cell(row, cPre), Folder: cell(row, cPath), Row: t.lines[i], Location: t.location(i)}
		if sets := splitRefs(cell(row, cSet)); len(sets) > 0 {
			c.Tags = append(c.Tags, sets...)
			if c.Folder == "" {
				c.Folder = sets[0]
			}
		}
		c.Custom = t.custom(row)
		c.Steps = []Step{step}
		xrayType(&c, cell(row, cGherk), cell(row, cGen))
		if key != "" {
			byKey[strings.ToLower(key)] = len(res.Cases)
		}
		res.Cases = append(res.Cases, c)
		cur = len(res.Cases) - 1
	}
	return nil
}

// xrayType applies the test type: Cucumber tests get their scenario,
// Generic ones their definition.
func xrayType(c *Case, gherkin, generic string) {
	typ := strings.ToLower(c.Type)
	switch {
	case strings.Contains(typ, "cucumber") || (typ == "" && gherkin != ""):
		if c.Type == "" {
			c.Type = "Cucumber"
		}
		c.Gherkin = strings.TrimSpace(gherkin)
		if steps := GherkinSteps(c.Gherkin); len(steps) > 0 {
			c.Steps = steps
		}
		c.Tags = append(c.Tags, "bdd")
	case strings.Contains(typ, "generic") || (typ == "" && generic != ""):
		if c.Type == "" {
			c.Type = "Generic"
		}
		c.Definition = strings.TrimSpace(generic)
		c.Automated = true
	}
}

// ---------------------------------------------------------------- JSON

func parseXrayJSON(data []byte, res *Result) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return invalidf("not a valid JSON file (%v)", err)
	}
	var list []any
	switch v := raw.(type) {
	case []any:
		list = v
	case map[string]any:
		for _, k := range []string{"tests", "testCases", "issues", "results", "data"} {
			if l, ok := v[k].([]any); ok {
				list = l
				break
			}
		}
		if list == nil {
			list = []any{v}
		}
	}
	if list == nil {
		return invalidf("expected a JSON array of tests")
	}
	for i, x := range list {
		m, ok := x.(map[string]any)
		if !ok {
			res.errorf("item %d is not an object; left out", i+1)
			continue
		}
		res.Cases = append(res.Cases, xrayJSONCase(m, i+1))
	}
	return nil
}

// xrayStandard are the keys with a built-in target (at the top level of a
// test or in its fields).
var xrayStandard = map[string]bool{
	"key": true, "issuekey": true, "testkey": true, "update_key": true, "id": true, "self": true, "fields": true, "summary": true,
	"description": true, "priority": true, "labels": true, "status": true, "testtype": true, "type": true, "steps": true, "gherkin_def": true,
	"gherkin": true, "scenario": true, "unstructured_def": true, "unstructured": true, "definition": true, "generic": true,
	"preconditions": true, "precondition": true, "xray_test_repository_folder": true, "folder": true, "repositorypath": true,
	"testrepositorypath": true, "xray_test_sets": true, "testsets": true, "requirementkeys": true, "requirements": true, "coverage": true,
	"tests": true, "links": true, "testruns": true, "executions": true, "assignee": true, "project": true, "issuetype": true,
	"reporter": true, "created": true, "updated": true, "xray_precondition": true, "components": true,
}

func xrayJSONCase(m map[string]any, i int) Case {
	f, _ := m["fields"].(map[string]any)
	get := func(keys ...string) any {
		for _, k := range keys {
			for _, src := range []map[string]any{m, f} {
				for mk, v := range src {
					if strings.EqualFold(mk, k) && v != nil {
						return v
					}
				}
			}
		}
		return nil
	}
	c := Case{ExternalID: jsonText(get("key", "issueKey", "testKey", "update_key")), Title: jsonText(get("summary", "name", "title")),
		Description: jsonText(get("description")), RawPriority: jsonText(get("priority")), RawStatus: jsonText(get("status")),
		Type: jsonText(get("testtype", "testType", "type")), Owner: jsonText(get("assignee")),
		Folder: jsonText(get("xray_test_repository_folder", "folder", "repositoryPath", "testRepositoryPath")), Row: i, Location: fmt.Sprintf("test %d", i)}
	if c.ExternalID != "" {
		c.Location += " (" + c.ExternalID + ")"
	}
	c.Tags = jsonList(get("labels"))
	c.Tags = append(c.Tags, jsonList(get("components"))...)
	for _, s := range jsonArray(get("steps")) {
		sm, _ := s.(map[string]any)
		if sm == nil {
			continue
		}
		fields, _ := sm["fields"].(map[string]any) // Xray Cloud: {"fields": {"Action": …}}
		pick := func(keys ...string) string {
			for _, src := range []map[string]any{sm, fields} {
				for _, k := range keys {
					for mk, v := range src {
						if strings.EqualFold(mk, k) {
							if t := jsonText(v); t != "" {
								return t
							}
						}
					}
				}
			}
			return ""
		}
		c.Steps = append(c.Steps, Step{Action: pick("action", "step"), Data: pick("data", "testData"), Expected: pick("result", "expectedResult", "expected")})
	}
	var pre []string
	for _, p := range jsonArray(get("preconditions", "precondition", "xray_precondition")) {
		if pm, ok := p.(map[string]any); ok {
			if d := jsonText(firstOf(pm, "definition", "condition", "summary")); d != "" {
				pre = append(pre, d)
			} else if k := jsonText(firstOf(pm, "key")); k != "" {
				pre = append(pre, k)
			}
		} else if s := jsonText(p); s != "" {
			pre = append(pre, s)
		}
	}
	c.Preconditions = strings.Join(pre, "\n\n")
	for _, s := range jsonArray(get("xray_test_sets", "testSets")) {
		if k := jsonKey(s); k != "" {
			c.Tags = append(c.Tags, k)
			if c.Folder == "" {
				c.Folder = k
			}
		}
	}
	for _, r := range jsonArray(get("requirementKeys", "requirements", "coverage", "tests", "links")) {
		if k := jsonKey(r); k != "" {
			c.Refs = append(c.Refs, splitRefs(k)...)
		}
	}
	gherkin := jsonText(get("gherkin_def", "gherkin", "scenario"))
	generic := jsonText(get("unstructured_def", "unstructured", "generic"))
	if def := jsonText(get("definition")); def != "" {
		if strings.Contains(strings.ToLower(c.Type), "cucumber") {
			gherkin = def
		} else {
			generic = def
		}
	}
	xrayType(&c, gherkin, generic)
	for _, x := range jsonArray(get("testRuns", "testruns", "executions")) {
		xm, _ := x.(map[string]any)
		if xm == nil {
			continue
		}
		raw := jsonText(firstOf(xm, "status"))
		st := GuessResult(raw)
		if st == "" {
			continue
		}
		run := jsonText(firstOf(xm, "testExecKey", "testExecutionKey", "testExecution", "execution", "key"))
		if run == "" {
			run = "Xray"
		}
		c.Executions = append(c.Executions, Execution{Run: run, Status: st, RawStatus: raw, ExecutedBy: jsonText(firstOf(xm, "executedBy", "executor", "assignee")),
			ExecutedAt: parseTime(jsonText(firstOf(xm, "finishedOn", "finishedDate", "executedOn", "finished", "startedOn"))), Comment: jsonText(firstOf(xm, "comment"))})
	}
	for _, src := range []map[string]any{m, f} {
		for _, k := range sortedKeys(src) {
			if xrayStandard[strings.ToLower(k)] {
				continue
			}
			if v := jsonText(src[k]); v != "" {
				if c.Custom == nil {
					c.Custom = map[string]string{}
				}
				c.Custom[k] = v
			}
		}
	}
	return c
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		for mk, v := range m {
			if strings.EqualFold(mk, k) && v != nil {
				return v
			}
		}
	}
	return nil
}

// jsonText returns the text of a scalar, of {"name"}/{"value"}/{"key"}
// objects (Jira fields) or of a list of them.
func jsonText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case map[string]any:
		for _, k := range []string{"name", "value", "displayName", "key", "text", "emailAddress"} {
			if s, ok := x[k]; ok {
				return jsonText(s)
			}
		}
		return ""
	case []any:
		return strings.Join(jsonList(x), ", ")
	}
	return ""
}

func jsonArray(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case nil:
		return nil
	}
	return []any{v}
}

func jsonList(v any) []string {
	if s, ok := v.(string); ok {
		return splitTags(s)
	}
	var out []string
	for _, x := range jsonArray(v) {
		if s := jsonText(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// jsonKey returns the key of an issue reference ("CALC-1" or {"key":…}).
func jsonKey(v any) string {
	if m, ok := v.(map[string]any); ok {
		return jsonText(firstOf(m, "key", "issueKey", "id"))
	}
	return jsonText(v)
}
