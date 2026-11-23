package reqimport

import (
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/catalog"
)

// MITRE D3FEND, EMB3D and ATT&CK ids in imported files: the columns and
// attributes the exports write (D3FEND, EMB3D, ATT&CK techniques). Ids
// are checked against the catalogue's lists; unknown ones are left out
// with a warning on the row, so that one typo does not stop an import.

// Framework id lists.
const (
	FrameworkD3fend       = "d3fend"        // D3FEND techniques (D3-MFA)
	FrameworkEmb3d        = "emb3d"         // EMB3D threats and mitigations (TID-201, MID-002)
	FrameworkEmb3dThreats = "emb3d-threats" // EMB3D threats only (risks)
	FrameworkAttack       = "attack"        // ATT&CK and ATLAS techniques (risks)
)

var frameworkNames = map[string]string{
	FrameworkD3fend: "D3FEND technique", FrameworkEmb3d: "EMB3D threat or mitigation",
	FrameworkEmb3dThreats: "EMB3D threat", FrameworkAttack: "ATT&CK or ATLAS technique",
}

// FrameworkIDs splits a cell into ids, normalises them and checks them
// against the catalogue: it returns the known ids (without duplicates)
// and a warning naming the unknown ones ("" when all are known). Without
// a catalogue every id is kept.
func FrameworkIDs(list, cell string) (ids []string, warning string) {
	ids = []string{}
	var bad []string
	c, _ := catalog.Default()
	for _, v := range SplitTags(cell) {
		var id string
		if list == FrameworkAttack {
			id = catalog.NormalizeTechnique(v)
		} else {
			id = catalog.NormalizeFrameworkID(v)
		}
		// "D3-MFA Multi-factor Authentication": the id is the first word
		if f := strings.Fields(id); len(f) > 1 {
			id = f[0]
		}
		if id == "" || slices.Contains(ids, id) || slices.Contains(bad, id) {
			continue
		}
		ok := c == nil
		if c != nil {
			switch list {
			case FrameworkD3fend:
				ok = c.ValidD3fend(id)
			case FrameworkEmb3d:
				ok = c.ValidEmb3d(id, false)
			case FrameworkEmb3dThreats:
				ok = c.ValidEmb3d(id, true)
			case FrameworkAttack:
				ok = c.ValidTechnique(id)
			}
		}
		if ok {
			ids = append(ids, id)
		} else {
			bad = append(bad, id)
		}
	}
	if len(bad) > 0 {
		warning = "unknown " + frameworkNames[list] + " " + strings.Join(bad, ", ") + " left out"
	}
	return ids, warning
}
