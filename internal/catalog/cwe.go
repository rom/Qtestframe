package catalog

import (
	"regexp"
	"strings"
)

// cweRef matches a CWE reference of the catalogue ("CWE-434 (unrestricted
// upload of file with dangerous type)").
var cweRef = regexp.MustCompile(`\bCWE-([0-9]{1,5})\b(?:\s*\(([^)]*)\))?`)

// CWEs returns the CWE weaknesses the catalogue refers to (in the refs of
// its requirements, tests and risks): id ("CWE-434") → name as written
// there ("" when only the id is given). It is the embedded list the AI
// assistance validates suggested CWE ids against.
func (c *Catalog) CWEs() map[string]string {
	out := map[string]string{}
	add := func(refs []string) {
		for _, r := range refs {
			for _, m := range cweRef.FindAllStringSubmatch(r, -1) {
				id := "CWE-" + m[1]
				if name := strings.TrimSpace(m[2]); name != "" && out[id] == "" {
					out[id] = name
				} else if _, ok := out[id]; !ok {
					out[id] = ""
				}
			}
		}
	}
	for _, r := range c.Requirements {
		add(r.Refs)
	}
	for _, t := range c.Tests {
		add(t.Refs)
	}
	for _, k := range c.Risks {
		add(k.Refs)
	}
	return out
}
