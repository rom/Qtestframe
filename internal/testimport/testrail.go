package testimport

import (
	"fmt"
	"strings"
)

// TestRail: the CSV export of a suite (Test Cases → Export → CSV, steps
// "separated" on rows of their own or in one cell), the CSV export of a
// test run (its results become executions) and the XML export of a suite
// (Test Cases → Export → XML), whose nested sections become folders.

func parseTestRailCSV(data []byte, res *Result) error {
	t, err := readCSV(data, "Title", "Case ID")
	if err != nil {
		return err
	}
	var (
		cID      = t.col("ID")
		cCase    = t.col("Case ID")
		cTitle   = t.col("Title")
		cSection = t.col("Section")
		cHier    = t.col("Section Hierarchy")
		cSuite   = t.col("Suite")
		cType    = t.col("Type")
		cPrio    = t.col("Priority")
		cEst     = t.col("Estimate")
		cRefs    = t.col("References", "Refs")
		cPre     = t.col("Preconditions", "Precondition")
		cSteps   = t.col("Steps")
		cExp     = t.col("Expected Result", "Expected")
		cStep    = t.col("Steps (Step)", "Step")
		cStepExp = t.col("Steps (Expected Result)", "Steps (Expected)")
		cStepAdd = t.col("Steps (Additional Info)", "Steps (Data)")
		cStatus  = t.col("Status")
		cRun     = t.col("Run")
		cRunID   = t.col("Run ID")
		cBy      = t.col("Tested By")
		cOn      = t.col("Tested On")
		cComment = t.col("Comment", "Comments")
		cAssign  = t.col("Assigned To")
		cMission = t.col("Mission")
		cGoals   = t.col("Goals")
	)
	// ignored: TestRail's own bookkeeping
	t.col("Section Description")
	t.col("Section Depth")
	t.col("Suite ID")
	t.col("Template")
	t.col("Created By")
	t.col("Created On")
	t.col("Updated By")
	t.col("Updated On")
	t.col("Forecast")
	t.col("Elapsed")
	t.col("Defects")
	t.col("Version")
	t.col("Steps (Shared step ID)")
	if cCase >= 0 {
		// a run export: "ID" is the test of the run (T…), "Case ID" the case
		cID, cCase = cCase, cID
	}
	if cTitle < 0 {
		return invalidf("no Title column: export the test cases from TestRail as CSV with the Title field")
	}
	suites := map[string]bool{}
	byID := map[string]int{}
	var cur *Case
	var suiteOf []string
	for i, row := range t.rows {
		if blank(row) {
			continue
		}
		id, title := cell(row, cID), cell(row, cTitle)
		if id == "" && title == "" && cur != nil {
			// a continuation row with the next separated step
			cur.Steps = append(cur.Steps, Step{Action: cell(row, cStep), Expected: cell(row, cStepExp), Data: cell(row, cStepAdd)})
			mergeCustom(cur, t.custom(row))
			continue
		}
		exec := testRailExecution(row, cStatus, cRun, cRunID, cBy, cOn, cComment)
		if j, ok := byID[strings.ToLower(id)]; ok && id != "" {
			// the same case again (a run export spanning runs)
			if exec != nil {
				res.Cases[j].Executions = append(res.Cases[j].Executions, *exec)
			}
			cur = &res.Cases[j]
			continue
		}
		c := Case{ExternalID: id, Title: title, Row: t.lines[i], Location: t.location(i),
			Type: cell(row, cType), RawPriority: cell(row, cPrio), Preconditions: cell(row, cPre),
			Refs: splitRefs(cell(row, cRefs)), EstimatedMin: estimateMinutes(cell(row, cEst)), Owner: cell(row, cAssign)}
		folder := strings.ReplaceAll(cell(row, cHier), " > ", "/")
		if folder == "" {
			folder = cell(row, cSection)
		}
		c.Folder = folder
		suite := cell(row, cSuite)
		suites[suite] = true
		suiteOf = append(suiteOf, suite)
		if c.Type != "" {
			c.Automated = strings.Contains(strings.ToLower(c.Type), "automat")
			c.Custom = map[string]string{"Type": c.Type}
		}
		mergeCustom(&c, t.custom(row))
		if s := cell(row, cStep); s != "" || cell(row, cStepExp) != "" {
			steps, exp := textSteps(s, cell(row, cStepExp))
			if len(steps) == 1 {
				steps[0].Data = cell(row, cStepAdd)
			}
			c.Steps, c.ExpectedResult = steps, exp
		} else if s := cell(row, cSteps); s != "" {
			c.Steps, c.ExpectedResult = textSteps(s, cell(row, cExp))
		} else {
			c.ExpectedResult = cell(row, cExp)
		}
		if m := cell(row, cMission); m != "" {
			c.Description = m
		}
		if g := cell(row, cGoals); g != "" {
			c.Description = strings.TrimSpace(c.Description + "\n\n" + g)
		}
		if exec != nil {
			c.Executions = append(c.Executions, *exec)
		}
		if id != "" {
			byID[strings.ToLower(id)] = len(res.Cases)
		}
		res.Cases = append(res.Cases, c)
		cur = &res.Cases[len(res.Cases)-1]
	}
	if len(suites) > 1 {
		for i := range res.Cases {
			if i < len(suiteOf) && suiteOf[i] != "" {
				res.Cases[i].Folder = suiteOf[i] + "/" + res.Cases[i].Folder
			}
		}
	}
	return nil
}

func testRailExecution(row []string, cStatus, cRun, cRunID, cBy, cOn, cComment int) *Execution {
	raw := cell(row, cStatus)
	st := GuessResult(raw)
	if st == "" {
		return nil
	}
	run := cell(row, cRun)
	if run == "" {
		run = "TestRail"
	}
	return &Execution{Run: run, RunID: cell(row, cRunID), Status: st, RawStatus: raw, ExecutedBy: cell(row, cBy),
		ExecutedAt: parseTime(cell(row, cOn)), Comment: cell(row, cComment)}
}

func parseTestRailXML(data []byte, res *Result) error {
	root, err := parseXML(data)
	if err != nil {
		return err
	}
	if root.name() != "suite" && root.name() != "sections" {
		return invalidf("not a TestRail XML export (the root element is <%s>, expected <suite>)", root.XMLName.Local)
	}
	sections := root
	if root.name() == "suite" {
		sections = root.child("sections")
	}
	var walk func(n *node, path []string, depth int)
	walk = func(n *node, path []string, depth int) {
		if n == nil || depth > 64 {
			return
		}
		for _, sec := range n.children("section") {
			p := append(append([]string(nil), path...), strings.ReplaceAll(sec.text("name"), "/", "-"))
			for _, cn := range sec.path("cases").children("case") {
				res.Cases = append(res.Cases, testRailCase(cn, strings.Join(p, "/"), len(res.Cases)+1))
			}
			walk(sec.child("sections"), p, depth+1)
		}
	}
	walk(sections, nil, 0)
	return nil
}

// testRailStandard are the custom elements with a built-in target.
var testRailStandard = map[string]bool{"preconds": true, "steps": true, "expected": true, "steps_separated": true, "mission": true, "goals": true}

func testRailCase(n *node, folder string, i int) Case {
	c := Case{ExternalID: n.text("id"), Title: n.text("title"), Folder: folder, Type: n.text("type"), RawPriority: n.text("priority"),
		Refs: splitRefs(n.text("references")), EstimatedMin: estimateMinutes(n.text("estimate")), Row: i, Location: fmt.Sprintf("case %d", i)}
	if c.ExternalID != "" {
		c.Location += " (" + c.ExternalID + ")"
	}
	if c.Type != "" {
		c.Automated = strings.Contains(strings.ToLower(c.Type), "automat")
		c.Custom = map[string]string{"Type": c.Type}
	}
	cust := n.child("custom")
	c.Preconditions = cust.text("preconds")
	if sep := cust.child("steps_separated"); sep != nil {
		for _, s := range sep.children("step") {
			c.Steps = append(c.Steps, Step{Action: s.text("content"), Expected: s.text("expected"), Data: s.text("additional_info")})
		}
		c.ExpectedResult = ""
	} else {
		c.Steps, c.ExpectedResult = textSteps(cust.text("steps"), cust.text("expected"))
	}
	c.Description = strings.TrimSpace(cust.text("mission") + "\n\n" + cust.text("goals"))
	if cust != nil {
		for _, f := range cust.Nodes {
			name := f.XMLName.Local
			if testRailStandard[strings.ToLower(name)] {
				continue
			}
			v := f.value()
			if v == "" {
				// a multi-select: <item><value>…</value></item>…
				var vals []string
				for _, it := range f.Nodes {
					if x := it.value(); x != "" {
						vals = append(vals, x)
					}
				}
				v = strings.Join(vals, ", ")
			}
			if v != "" {
				if c.Custom == nil {
					c.Custom = map[string]string{}
				}
				c.Custom[name] = v
			}
		}
	}
	return c
}
