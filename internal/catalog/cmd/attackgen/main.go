// Command attackgen writes the MITRE ATT&CK list of the catalogue
// (internal/catalog/data/attack.yaml) from the ATT&CK STIX 2.1 bundles:
// the tactics, techniques, sub-techniques and mitigations of the
// Enterprise, ICS and Mobile matrices, without revoked or deprecated
// objects. Download the bundles from the attack-stix-data repository
// (enterprise-attack.json, ics-attack.json, mobile-attack.json), then
//
//	go run ./internal/catalog/cmd/attackgen -out internal/catalog/data/attack.yaml \
//	    enterprise-attack.json ics-attack.json mobile-attack.json
//
// and run go test ./internal/catalog: risks and refs that cite an id that
// is no longer in the list fail the tests.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func main() {
	out := flag.String("out", "internal/catalog/data/attack.yaml", "the file to write")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "attackgen: name the STIX bundles (enterprise-attack.json …)")
		os.Exit(2)
	}
	var ms []matrix
	for _, path := range flag.Args() {
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "attackgen:", err)
			os.Exit(1)
		}
		m, err := parse(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "attackgen: %s: %v\n", path, err)
			os.Exit(1)
		}
		ms = append(ms, m)
	}
	f, err := os.Create(*out)
	if err == nil {
		err = write(f, ms)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "attackgen:", err)
		os.Exit(1)
	}
}

type extRef struct {
	Source string `json:"source_name"`
	ID     string `json:"external_id"`
	URL    string `json:"url"`
}

type object struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Revoked    bool     `json:"revoked"`
	Deprecated bool     `json:"x_mitre_deprecated"`
	Version    string   `json:"x_mitre_version"`
	Shortname  string   `json:"x_mitre_shortname"`
	TacticRefs []string `json:"tactic_refs"`
	Refs       []extRef `json:"external_references"`
	Phases     []struct {
		Chain string `json:"kill_chain_name"`
		Phase string `json:"phase_name"`
	} `json:"kill_chain_phases"`
}

func (o object) attackID() (string, string) {
	for _, r := range o.Refs {
		if r.Source == "mitre-attack" {
			return r.ID, r.URL
		}
	}
	return "", ""
}

type entry struct{ id, name, url string }

type technique struct {
	entry
	tactics []string
}

type matrix struct {
	id, name, version, url string
	tactics                []entry
	techniques             []technique
	mitigations            []entry
}

// matrixName is the short name of a matrix: "Enterprise", "ICS", "Mobile".
func matrixName(n string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(n, "ATT&CK for "), " ATT&CK"))
}

// matrixIDs maps the bundles' matrix ids to the catalogue's.
var matrixIDs = map[string]string{"enterprise-attack": "enterprise", "ics-attack": "ics", "mobile-attack": "mobile"}

func parse(b []byte) (matrix, error) {
	var bundle struct {
		Objects []object `json:"objects"`
	}
	if err := json.Unmarshal(b, &bundle); err != nil {
		return matrix{}, err
	}
	var m matrix
	tactics := map[string]object{}
	short := map[string]string{} // shortname → TA id
	var order []string
	for _, o := range bundle.Objects {
		if o.Revoked || o.Deprecated {
			continue
		}
		switch o.Type {
		case "x-mitre-collection":
			m.version = o.Version
		case "x-mitre-matrix":
			id, url := o.attackID()
			m.id, m.name, m.url, order = matrixIDs[id], matrixName(o.Name), url, o.TacticRefs
		case "x-mitre-tactic":
			tactics[o.ID] = o
			id, _ := o.attackID()
			short[o.Shortname] = id
		}
	}
	if m.id == "" {
		return m, fmt.Errorf("no ATT&CK matrix in the bundle")
	}
	for _, ref := range order {
		o, ok := tactics[ref]
		if !ok {
			continue
		}
		id, url := o.attackID()
		m.tactics = append(m.tactics, entry{id, o.Name, url})
	}
	for _, o := range bundle.Objects {
		if o.Revoked || o.Deprecated {
			continue
		}
		id, url := o.attackID()
		switch {
		case o.Type == "attack-pattern" && id != "":
			t := technique{entry: entry{id, o.Name, url}}
			for _, p := range o.Phases {
				if ta := short[p.Phase]; ta != "" && strings.HasPrefix(p.Chain, "mitre-") {
					t.tactics = append(t.tactics, ta)
				}
			}
			m.techniques = append(m.techniques, t)
		case o.Type == "course-of-action" && strings.HasPrefix(id, "M"):
			m.mitigations = append(m.mitigations, entry{id, o.Name, url})
		}
	}
	rank := map[string]int{}
	for i, t := range m.tactics {
		rank[t.id] = i
	}
	for i := range m.techniques {
		ts := m.techniques[i].tactics
		sort.SliceStable(ts, func(a, b int) bool { return rank[ts[a]] < rank[ts[b]] })
	}
	sort.Slice(m.techniques, func(i, j int) bool { return m.techniques[i].id < m.techniques[j].id })
	sort.Slice(m.mitigations, func(i, j int) bool { return m.mitigations[i].id < m.mitigations[j].id })
	return m, nil
}

// q quotes s as a YAML double-quoted scalar (a JSON string is one).
func q(s string) string {
	var b strings.Builder
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(s)
	return strings.TrimSpace(b.String())
}

func write(w io.Writer, ms []matrix) error {
	var sb strings.Builder
	sb.WriteString("# MITRE ATT&CK tactics, techniques, sub-techniques and mitigations\n")
	sb.WriteString("# (no revoked or deprecated ids). Generated by internal/catalog/cmd/attackgen\n")
	sb.WriteString("# from the ATT&CK STIX bundles: do not edit by hand. See FORMAT.md, \"MITRE ATT&CK\".\n")
	sb.WriteString("# MITRE ATT&CK® is a registered trademark of The MITRE Corporation; the list\n")
	sb.WriteString("# reproduces ids and names under the ATT&CK terms of use (https://attack.mitre.org/resources/legal-and-branding/terms-of-use/).\n")
	sb.WriteString("matrices:\n")
	for _, m := range ms {
		fmt.Fprintf(&sb, "  - {id: %s, name: %s, version: %s, url: %s}\n", m.id, q(m.name), q(m.version), m.url)
	}
	sb.WriteString("tactics:\n")
	for _, m := range ms {
		for _, t := range m.tactics {
			fmt.Fprintf(&sb, "  - {id: %s, name: %s, matrix: %s, url: %s}\n", t.id, q(t.name), m.id, t.url)
		}
	}
	sb.WriteString("techniques:\n")
	for _, m := range ms {
		for _, t := range m.techniques {
			fmt.Fprintf(&sb, "  - {id: %s, name: %s, matrix: %s, tactics: [%s], url: %s}\n", t.id, q(t.name), m.id, strings.Join(t.tactics, ", "), t.url)
		}
	}
	sb.WriteString("mitigations:\n")
	for _, m := range ms {
		for _, t := range m.mitigations {
			fmt.Fprintf(&sb, "  - {id: %s, name: %s, matrix: %s, url: %s}\n", t.id, q(t.name), m.id, t.url)
		}
	}
	_, err := io.WriteString(w, sb.String())
	return err
}
