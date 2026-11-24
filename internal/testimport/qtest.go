package testimport

import (
	"regexp"
	"strings"
)

// qTest Manager: the Excel (XLSX) export of test cases (Test Design →
// Export → Test Cases), also saved as CSV: one row per step, the rows of
// a case leave Id and Name empty; modules become folders.

var qtestHeader = []string{"Name", "Test Case Name"}

var moduleIDRe = regexp.MustCompile(`^MD-\d+\s+`)

func parseQTest(t *table, res *Result) error {
	var (
		cID     = t.col("Id", "Test Case Id", "PID", "Test Case ID")
		cName   = t.col("Name", "Test Case Name")
		cDesc   = t.col("Description")
		cPre    = t.col("Precondition", "Preconditions", "Pre-condition")
		cStep   = t.col("Test Step Description", "Step Description", "Test Step", "Step")
		cExp    = t.col("Test Step Expected Result", "Step Expected Result", "Expected Result")
		cData   = t.col("Test Step Test Data", "Test Data", "Step Test Data")
		cModule = t.col("Module", "Module Path", "Parent Module", "Module Hierarchy")
		cPrio   = t.col("Priority")
		cStatus = t.col("Status")
		cType   = t.col("Type", "Test Case Type")
		cOwner  = t.col("Assigned To", "Assignee", "Owner")
		cReq    = t.col("Linked Requirements", "Requirements", "Requirement Ids", "Requirement", "Linked Requirement Ids")
		cAuto   = t.col("Automation", "Automated")
	)
	t.col("Test Step #")
	t.col("Step #")
	t.col("Version")
	t.col("Created Date")
	t.col("Last Modified Date")
	t.col("Created By")
	t.col("Last Modified By")
	if cName < 0 {
		return invalidf("no Name column: export the test cases from qTest with the Name field")
	}
	cur := -1
	for i, row := range t.rows {
		if blank(row) {
			continue
		}
		id, name := cell(row, cID), cell(row, cName)
		step := Step{Action: htmlText(cell(row, cStep)), Data: htmlText(cell(row, cData)), Expected: htmlText(cell(row, cExp))}
		if id == "" && name == "" && cur >= 0 {
			res.Cases[cur].Steps = append(res.Cases[cur].Steps, step)
			mergeCustom(&res.Cases[cur], t.custom(row))
			continue
		}
		var folder []string
		for _, p := range strings.Split(strings.ReplaceAll(cell(row, cModule), "\\", "/"), "/") {
			folder = append(folder, moduleIDRe.ReplaceAllString(strings.TrimSpace(p), ""))
		}
		c := Case{ExternalID: id, Title: name, Description: htmlText(cell(row, cDesc)), Preconditions: htmlText(cell(row, cPre)),
			Folder: strings.Join(folder, "/"), RawPriority: cell(row, cPrio), RawStatus: cell(row, cStatus), Type: cell(row, cType),
			Owner: cell(row, cOwner), Refs: splitRefs(cell(row, cReq)), Row: t.lines[i], Location: t.location(i), Custom: t.custom(row), Steps: []Step{step}}
		if c.Type != "" {
			c.Custom["Type"] = c.Type
		}
		switch strings.ToLower(cell(row, cAuto)) {
		case "yes", "true", "1", "automated":
			c.Automated = true
		}
		if strings.Contains(strings.ToLower(c.Type), "automat") {
			c.Automated = true
		}
		res.Cases = append(res.Cases, c)
		cur = len(res.Cases) - 1
	}
	return nil
}
