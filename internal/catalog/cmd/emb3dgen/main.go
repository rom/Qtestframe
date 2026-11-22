// Command emb3dgen writes the MITRE EMB3D list of the catalogue
// (internal/catalog/data/emb3d.yaml) from the EMB3D STIX 2.1 bundle: the
// device properties (PID-11), threats (TID-101) with the properties that
// expose a device to them and the mitigations that address them, and the
// mitigations (MID-001) with their maturity. Download the bundle from the
// EMB3D repository (assets/emb3d-stix-<version>.json), then
//
//	go run ./internal/catalog/cmd/emb3dgen -out internal/catalog/data/emb3d.yaml \
//	    emb3d-stix-2.0.1.json
//
// and run go test ./internal/catalog: items that list an id no longer in
// the list fail the tests.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func main() {
	out := flag.String("out", "internal/catalog/data/emb3d.yaml", "the file to write")
	version := flag.String("version", "", "the EMB3D version (default: from the file name emb3d-stix-<version>.json)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "emb3dgen: name the STIX bundle (emb3d-stix-2.0.1.json)")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *version, *out); err != nil {
		fmt.Fprintln(os.Stderr, "emb3dgen:", err)
		os.Exit(1)
	}
}

var fileVersionRe = regexp.MustCompile(`emb3d-stix-([0-9][0-9.]*[0-9])\.json$`)

func run(path, version, out string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if version == "" {
		if m := fileVersionRe.FindStringSubmatch(filepath.Base(path)); m != nil {
			version = m[1]
		} else {
			return fmt.Errorf("give -version: the file name has none")
		}
	}
	l, err := parse(b)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	l.version = version
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	err = write(f, l)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

type object struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Revoked    bool   `json:"revoked"`
	Deprecated bool   `json:"x_mitre_deprecated"`
	RelType    string `json:"relationship_type"`
	Source     string `json:"source_ref"`
	Target     string `json:"target_ref"`
	// properties
	PropertyID string `json:"x_mitre_emb3d_property_id"`
	Category   string `json:"category"`
	// threats
	ThreatID       string `json:"x_mitre_emb3d_threat_id"`
	ThreatCategory string `json:"x_mitre_emb3d_threat_category"`
	ThreatMaturity string `json:"x_mitre_emb3d_threat_maturity"`
	// mitigations
	MitigationID string `json:"x_mitre_emb3d_mitigation_id"`
	Maturity     string `json:"x_mitre_emb3d_mitigation_maturity"`
}

type property struct{ id, name, category, parent string }

type threat struct {
	id, name, category, maturity string
	properties, mitigations      []string
}

type mitigation struct{ id, name, maturity string }

type list struct {
	version     string
	properties  []property
	threats     []threat
	mitigations []mitigation
}

var (
	pidRe = regexp.MustCompile(`^PID-\d{2,6}$`)
	tidRe = regexp.MustCompile(`^TID-\d{3}$`)
	midRe = regexp.MustCompile(`^MID-\d{3}$`)
)

func parse(b []byte) (list, error) {
	var bundle struct {
		Objects []object `json:"objects"`
	}
	if err := json.Unmarshal(b, &bundle); err != nil {
		return list{}, err
	}
	var l list
	ids := map[string]string{} // STIX id → EMB3D id
	for _, o := range bundle.Objects {
		if o.Revoked || o.Deprecated {
			continue
		}
		switch o.Type {
		case "x-mitre-emb3d-property":
			if !pidRe.MatchString(o.PropertyID) {
				return l, fmt.Errorf("property %s: id %q", o.ID, o.PropertyID)
			}
			ids[o.ID] = o.PropertyID
			l.properties = append(l.properties, property{id: o.PropertyID, name: strings.TrimSpace(o.Name), category: strings.ToLower(o.Category)})
		case "vulnerability":
			if !tidRe.MatchString(o.ThreatID) {
				return l, fmt.Errorf("threat %s: id %q", o.ID, o.ThreatID)
			}
			ids[o.ID] = o.ThreatID
			l.threats = append(l.threats, threat{id: o.ThreatID, name: strings.TrimSpace(o.Name), category: strings.ToLower(o.ThreatCategory), maturity: strings.ToLower(o.ThreatMaturity)})
		case "course-of-action":
			if !midRe.MatchString(o.MitigationID) {
				return l, fmt.Errorf("mitigation %s: id %q", o.ID, o.MitigationID)
			}
			ids[o.ID] = o.MitigationID
			l.mitigations = append(l.mitigations, mitigation{id: o.MitigationID, name: strings.TrimSpace(o.Name), maturity: strings.ToLower(o.Maturity)})
		}
	}
	if len(l.threats) == 0 {
		return l, fmt.Errorf("no EMB3D threat in the bundle")
	}
	threatByID := map[string]*threat{}
	for i := range l.threats {
		threatByID[l.threats[i].id] = &l.threats[i]
	}
	propByID := map[string]*property{}
	for i := range l.properties {
		propByID[l.properties[i].id] = &l.properties[i]
	}
	add := func(s []string, v string) []string {
		for _, x := range s {
			if x == v {
				return s
			}
		}
		return append(s, v)
	}
	for _, o := range bundle.Objects {
		if o.Type != "relationship" || o.Revoked || o.Deprecated {
			continue
		}
		src, dst := ids[o.Source], ids[o.Target]
		if src == "" || dst == "" {
			continue
		}
		switch o.RelType {
		case "mitigates":
			if t := threatByID[dst]; t != nil && midRe.MatchString(src) {
				t.mitigations = add(t.mitigations, src)
			}
		case "relates-to":
			if t := threatByID[dst]; t != nil && pidRe.MatchString(src) {
				t.properties = add(t.properties, src)
			}
		case "subproperty-of":
			if p := propByID[src]; p != nil && propByID[dst] != nil {
				p.parent = dst
			}
		}
	}
	for i := range l.threats {
		sort.Slice(l.threats[i].properties, func(a, b int) bool { return pidLess(l.threats[i].properties[a], l.threats[i].properties[b]) })
		sort.Strings(l.threats[i].mitigations)
	}
	sort.Slice(l.properties, func(i, j int) bool { return pidLess(l.properties[i].id, l.properties[j].id) })
	sort.Slice(l.threats, func(i, j int) bool { return l.threats[i].id < l.threats[j].id })
	sort.Slice(l.mitigations, func(i, j int) bool { return l.mitigations[i].id < l.mitigations[j].id })
	return l, nil
}

// pidLess orders property ids as the EMB3D site does: by digit string
// (PID-11, PID-12, PID-121, PID-13).
func pidLess(a, b string) bool { return a < b }

// q quotes s as a YAML double-quoted scalar (a JSON string is one).
func q(s string) string {
	var b strings.Builder
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(s)
	return strings.TrimSpace(b.String())
}

func write(w io.Writer, l list) error {
	var sb strings.Builder
	sb.WriteString("# MITRE EMB3D device properties, threats and mitigations. Generated by\n")
	sb.WriteString("# internal/catalog/cmd/emb3dgen from the EMB3D STIX bundle: do not edit by\n")
	sb.WriteString("# hand. See FORMAT.md, \"MITRE EMB3D\". EMB3D™ is a trademark of The MITRE\n")
	sb.WriteString("# Corporation; the list reproduces ids and names under the EMB3D terms of use\n")
	sb.WriteString("# (https://emb3d.mitre.org/subtabs/terms-of-use.html).\n")
	fmt.Fprintf(&sb, "version: %s\nurl: https://emb3d.mitre.org/\n", q(l.version))
	sb.WriteString("properties:\n")
	for _, p := range l.properties {
		fmt.Fprintf(&sb, "  - {id: %s, name: %s, category: %s", p.id, q(p.name), q(p.category))
		if p.parent != "" {
			fmt.Fprintf(&sb, ", parent: %s", p.parent)
		}
		sb.WriteString("}\n")
	}
	sb.WriteString("threats:\n")
	for _, t := range l.threats {
		fmt.Fprintf(&sb, "  - {id: %s, name: %s, category: %s, maturity: %s, url: https://emb3d.mitre.org/threats/%s.html, properties: [%s], mitigations: [%s]}\n",
			t.id, q(t.name), q(t.category), q(t.maturity), t.id, strings.Join(t.properties, ", "), strings.Join(t.mitigations, ", "))
	}
	sb.WriteString("mitigations:\n")
	for _, m := range l.mitigations {
		fmt.Fprintf(&sb, "  - {id: %s, name: %s, maturity: %s, url: https://emb3d.mitre.org/mitigations/%s.html}\n", m.id, q(m.name), q(m.maturity), m.id)
	}
	_, err := io.WriteString(w, sb.String())
	return err
}
