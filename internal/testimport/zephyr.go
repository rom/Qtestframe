package testimport

import (
	"fmt"
	"strconv"
	"strings"
)

// Zephyr: the CSV export of Zephyr Scale (Tests → Export → CSV, one row per
// step) and of Zephyr Squad (the Jira issue export with steps), and the
// XML export of Zephyr Scale (Tests → Export → XML).

func parseZephyrCSV(data []byte, res *Result) error {
	t, err := readCSV(data, "Name", "Summary")
	if err != nil {
		return err
	}
	var (
		cKey    = t.col("Key", "Test Case Key", "Issue Key", "Issue key")
		cName   = t.col("Name", "Summary", "Test Case Name")
		cStatus = t.col("Status")
		cPre    = t.col("Precondition", "Preconditions")
		cObj    = t.col("Objective", "Description")
		cFolder = t.col("Folder", "Test Repository Path", "Folder Path")
		cPrio   = t.col("Priority")
		cLabels = t.col("Labels", "Label")
		cOwner  = t.col("Owner", "Assignee")
		cEst    = t.col("Estimated Time", "Estimate")
		cCov    = t.col("Coverage (Issues)", "Coverage", "Issue Links", "Linked Issues", "Requirements", "Requirement")
		cStep   = t.col("Test Script (Step-by-Step) - Step", "Step", "Test Step", "Step Description")
		cData   = t.col("Test Script (Step-by-Step) - Test Data", "Test Data", "Data")
		cExp    = t.col("Test Script (Step-by-Step) - Expected Result", "Expected Result", "Result")
		cPlain  = t.col("Test Script (Plain Text)", "Plain Text")
		cBDD    = t.col("Test Script (BDD)", "BDD")
	)
	t.col("Coverage (Pages)")
	t.col("Order ID")
	t.col("Step Order")
	t.col("Created On")
	t.col("Created By")
	t.col("Updated On")
	t.col("Project")
	t.col("Issue Type")
	if cName < 0 {
		return invalidf("no Name column: export the test cases from Zephyr as CSV with the Name field")
	}
	byKey := map[string]int{}
	cur := -1
	for i, row := range t.rows {
		if blank(row) {
			continue
		}
		key, name := cell(row, cKey), cell(row, cName)
		step := Step{Action: cell(row, cStep), Data: cell(row, cData), Expected: cell(row, cExp)}
		j, seen := byKey[strings.ToLower(key)]
		if (key == "" && name == "" && cur >= 0) || (key != "" && seen) {
			if key == "" {
				j = cur
			}
			res.Cases[j].Steps = append(res.Cases[j].Steps, step)
			mergeCustom(&res.Cases[j], t.custom(row))
			cur = j
			continue
		}
		c := Case{ExternalID: key, Title: name, RawStatus: cell(row, cStatus), Preconditions: cell(row, cPre), Description: cell(row, cObj),
			Folder: cell(row, cFolder), RawPriority: cell(row, cPrio), Tags: splitTags(cell(row, cLabels)), Owner: cell(row, cOwner),
			EstimatedMin: zephyrEstimate(cell(row, cEst)), Refs: splitRefs(cell(row, cCov)), Row: t.lines[i], Location: t.location(i),
			Custom: t.custom(row), Steps: []Step{step}}
		zephyrScript(&c, cell(row, cPlain), cell(row, cBDD))
		if key != "" {
			byKey[strings.ToLower(key)] = len(res.Cases)
		}
		res.Cases = append(res.Cases, c)
		cur = len(res.Cases) - 1
	}
	return nil
}

// zephyrScript applies a plain text or BDD test script.
func zephyrScript(c *Case, plain, bdd string) {
	if plain = strings.TrimSpace(plain); plain != "" {
		steps, exp := textSteps(htmlText(plain), "")
		c.Steps, c.ExpectedResult = append(c.Steps, steps...), exp
	}
	if bdd = strings.TrimSpace(bdd); bdd != "" {
		c.Type, c.Gherkin = "BDD", htmlText(bdd)
		c.Steps = GherkinSteps(c.Gherkin)
		c.Tags = append(c.Tags, "bdd")
	}
}

// zephyrEstimate reads an estimate: Zephyr Scale exports milliseconds.
func zephyrEstimate(s string) int {
	if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && n >= 60000 {
		return int(n / 60000)
	}
	return estimateMinutes(s)
}

func parseZephyrXML(data []byte, res *Result) error {
	root, err := parseXML(data)
	if err != nil {
		return err
	}
	tcs := root.find("testCases")
	if tcs == nil {
		return invalidf("not a Zephyr Scale XML export (no <testCases> element)")
	}
	for i, n := range tcs.children("testCase") {
		res.Cases = append(res.Cases, zephyrCase(n, i+1))
	}
	return nil
}

func zephyrCase(n *node, i int) Case {
	c := Case{ExternalID: n.attr("key"), Title: n.text("name"), Description: htmlText(n.text("objective")), Preconditions: htmlText(n.text("precondition")),
		Folder: n.text("folder"), RawPriority: n.text("priority"), RawStatus: n.text("status"), Owner: n.text("owner"),
		EstimatedMin: zephyrEstimate(n.text("estimatedTime")), Row: i, Location: fmt.Sprintf("test case %d", i)}
	if c.ExternalID == "" {
		c.ExternalID = n.text("key")
	}
	if c.ExternalID != "" {
		c.Location += " (" + c.ExternalID + ")"
	}
	for _, l := range n.path("labels").children("label") {
		c.Tags = append(c.Tags, l.value())
	}
	for _, is := range n.path("issues").children("issue") {
		if k := is.text("key"); k != "" {
			c.Refs = append(c.Refs, k)
		} else if v := is.value(); v != "" {
			c.Refs = append(c.Refs, splitRefs(v)...)
		}
	}
	for _, cf := range n.path("customFields").children("customField") {
		name := cf.attr("name")
		if name == "" {
			continue
		}
		v := cf.value()
		if v == "" {
			var vals []string
			for _, x := range cf.Nodes {
				if s := x.value(); s != "" {
					vals = append(vals, s)
				}
			}
			v = strings.Join(vals, ", ")
		}
		if v != "" {
			if c.Custom == nil {
				c.Custom = map[string]string{}
			}
			c.Custom[name] = htmlText(v)
		}
	}
	script := n.child("testScript")
	switch typ := strings.ToLower(strings.ReplaceAll(script.attr("type"), "_", "-")); {
	case strings.Contains(typ, "step"):
		for _, s := range script.path("steps").children("step") {
			c.Steps = append(c.Steps, Step{Action: htmlText(s.text("description")), Data: htmlText(s.text("testData")), Expected: htmlText(s.text("expectedResult"))})
		}
	case strings.Contains(typ, "plain"):
		zephyrScript(&c, script.text("text"), "")
	case strings.Contains(typ, "bdd"):
		zephyrScript(&c, "", script.text("text"))
	}
	return c
}
