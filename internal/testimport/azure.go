package testimport

import (
	"strings"
)

// Azure Test Plans: the CSV export of test cases (Test Plans → a suite →
// Export test cases to CSV: one row per step, the rows of a case leave ID
// and Title empty) and the CSV of a work item query of test cases, whose
// Steps column holds the steps in Azure DevOps' XML. The area path
// becomes the folder (without the project's root area when every case
// shares it); work items in a Tests (or Requirements) column are the
// requirements the case verifies.

func parseAzureCSV(data []byte, res *Result) error {
	t, err := readCSV(data, "Title")
	if err != nil {
		return err
	}
	var (
		cID     = t.col("ID", "Work Item ID", "Id")
		cType   = t.col("Work Item Type")
		cTitle  = t.col("Title")
		cAction = t.col("Step Action", "Action")
		cExp    = t.col("Step Expected", "Expected Result", "Expected")
		cSteps  = t.col("Steps")
		cArea   = t.col("Area Path", "Area")
		cOwner  = t.col("Assigned To")
		cState  = t.col("State")
		cPrio   = t.col("Priority")
		cTags   = t.col("Tags")
		cDesc   = t.col("Description", "Summary")
		cAuto   = t.col("Automation status", "Automation Status")
		cTest   = t.col("Automated Test Name")
		cLinks  = t.col("Tests", "Tested Work Items", "Requirements", "Requirement", "Related Work Items", "Related", "Tested By")
	)
	t.col("Test Step")
	t.col("Iteration Path")
	t.col("Reason")
	t.col("Created Date")
	t.col("Created By")
	t.col("Changed Date")
	t.col("Changed By")
	t.col("Rev")
	t.col("Parameters")
	t.col("Local Data")
	if cTitle < 0 {
		return invalidf("no Title column: export the test cases from Azure Test Plans as CSV")
	}
	cur := -1
	for i, row := range t.rows {
		if blank(row) {
			continue
		}
		id, title := cell(row, cID), cell(row, cTitle)
		if id == "" && title == "" && cur >= 0 {
			c := &res.Cases[cur]
			c.Steps = append(c.Steps, Step{Action: htmlText(cell(row, cAction)), Expected: htmlText(cell(row, cExp))})
			mergeCustom(c, t.custom(row))
			continue
		}
		if typ := cell(row, cType); typ != "" && !strings.EqualFold(typ, "Test Case") {
			res.warn("%s: %s %s is a %s, not a test case; left out", t.location(i), id, title, typ)
			cur = -1
			continue
		}
		c := Case{ExternalID: id, Title: title, Description: htmlText(cell(row, cDesc)), Folder: strings.ReplaceAll(cell(row, cArea), "\\", "/"),
			Owner: azureUser(cell(row, cOwner)), RawStatus: cell(row, cState), RawPriority: cell(row, cPrio), Tags: splitTags(cell(row, cTags)),
			Refs: splitRefs(cell(row, cLinks)), Row: t.lines[i], Location: t.location(i), Custom: t.custom(row)}
		if a := cell(row, cAction); a != "" || cell(row, cExp) != "" {
			c.Steps = append(c.Steps, Step{Action: htmlText(a), Expected: htmlText(cell(row, cExp))})
		}
		if x := cell(row, cSteps); x != "" {
			steps, warns := AzureSteps(x)
			c.Steps = append(c.Steps, steps...)
			for _, w := range warns {
				c.warn("%s", w)
			}
		}
		if strings.EqualFold(cell(row, cAuto), "Automated") {
			c.Automated = true
			c.Definition = cell(row, cTest)
		}
		res.Cases = append(res.Cases, c)
		cur = len(res.Cases) - 1
	}
	trimCommonRoot(res.Cases)
	return nil
}

// azureUser turns "Ada Lovelace <ada@example.com>" into the address.
func azureUser(s string) string {
	if i, j := strings.LastIndexByte(s, '<'), strings.LastIndexByte(s, '>'); i >= 0 && j > i {
		return strings.TrimSpace(s[i+1 : j])
	}
	return s
}

// AzureSteps reads the steps XML of an Azure DevOps test case: each step
// has the action and the expected result as (HTML) parameterized strings;
// a shared step reference becomes a step naming the shared steps.
func AzureSteps(x string) ([]Step, []string) {
	root, err := parseXML([]byte(x))
	if err != nil {
		// not XML: the text is one step
		return []Step{{Action: htmlText(x)}}, nil
	}
	var out []Step
	var warns []string
	var walk func(n *node)
	walk = func(n *node) {
		for i := range n.Nodes {
			c := &n.Nodes[i]
			switch c.name() {
			case "step":
				ps := c.children("parameterizedString")
				s := Step{}
				if len(ps) > 0 {
					s.Action = htmlText(ps[0].Text)
				}
				if len(ps) > 1 {
					s.Expected = htmlText(ps[1].Text)
				}
				out = append(out, s)
			case "compref":
				ref := c.attr("ref")
				out = append(out, Step{Action: "Shared steps " + ref})
				warns = append(warns, "shared steps "+ref+" are imported as one step naming them")
				walk(c)
			}
		}
	}
	walk(root)
	return out, warns
}

// trimCommonRoot removes the first folder level when every case with a
// folder shares it (the project's root area or a single suite).
func trimCommonRoot(cases []Case) {
	root := ""
	for _, c := range cases {
		parts := FolderParts(c.Folder)
		if len(parts) == 0 {
			continue
		}
		if root == "" {
			root = parts[0]
		} else if !strings.EqualFold(root, parts[0]) {
			return
		}
	}
	if root == "" {
		return
	}
	for i := range cases {
		parts := FolderParts(cases[i].Folder)
		if len(parts) > 0 {
			cases[i].Folder = strings.Join(parts[1:], "/")
		}
	}
}
