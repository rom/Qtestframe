// Command d3fendgen writes the MITRE D3FEND list of the catalogue
// (internal/catalog/data/d3fend.yaml) from the published D3FEND data: the
// defensive tactics and techniques (id D3-MFA, name, tactic, parent
// technique, link) of the ontology, and for each technique the ATT&CK
// techniques it counters as the D3FEND site infers them through the
// digital artifacts both act on. Download the ontology and the technique
// pages of the D3FEND API:
//
//	curl -o d3fend.json https://d3fend.mitre.org/ontologies/d3fend.json
//	curl -o all.json https://d3fend.mitre.org/api/technique/all.json
//	mkdir api; for t in $(jq -r '."@graph"[]."@id"' all.json); do
//	    curl -o "api/${t#d3f:}.json" "https://d3fend.mitre.org/api/technique/$t.json"; done
//
// then
//
//	go run ./internal/catalog/cmd/d3fendgen -ontology d3fend.json -api api \
//	    -attack internal/catalog/data/attack.yaml -out internal/catalog/data/d3fend.yaml
//
// and run go test ./internal/catalog: requirements that list an id no
// longer in the list fail the tests. Only ATT&CK ids of attack.yaml are
// kept (no revoked or deprecated ones, no other frameworks).
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
	ontology := flag.String("ontology", "d3fend.json", "the D3FEND ontology (JSON-LD)")
	apiDir := flag.String("api", "", "directory of the D3FEND API technique pages (<name>.json); empty: no ATT&CK relations")
	attack := flag.String("attack", "internal/catalog/data/attack.yaml", "the catalogue's ATT&CK list (ids kept)")
	out := flag.String("out", "internal/catalog/data/d3fend.yaml", "the file to write")
	flag.Parse()
	if err := run(*ontology, *apiDir, *attack, *out); err != nil {
		fmt.Fprintln(os.Stderr, "d3fendgen:", err)
		os.Exit(1)
	}
}

func run(ontology, apiDir, attack, out string) error {
	b, err := os.ReadFile(ontology)
	if err != nil {
		return err
	}
	l, err := parse(b)
	if err != nil {
		return fmt.Errorf("%s: %w", ontology, err)
	}
	known := map[string]bool{}
	if attack != "" {
		ab, err := os.ReadFile(attack)
		if err != nil {
			return err
		}
		for _, m := range attackIDRe.FindAllStringSubmatch(string(ab), -1) {
			known[m[1]] = true
		}
	}
	if apiDir != "" {
		for i := range l.techniques {
			t := &l.techniques[i]
			p := filepath.Join(apiDir, strings.TrimPrefix(t.iri, "d3f:")+".json")
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			ids, err := offensive(b)
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			for _, id := range ids {
				if known[id] {
					t.attack = append(t.attack, id)
				}
			}
		}
	}
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

// attackIDRe finds the technique ids of attack.yaml.
var attackIDRe = regexp.MustCompile(`(?m)^  - \{id: (T\d{4}(?:\.\d{3})?),`)

// tactics are the D3FEND tactics in display order.
var tactics = []string{"Model", "Harden", "Detect", "Isolate", "Deceive", "Evict", "Restore"}

type ref struct {
	ID string `json:"@id"`
}

// refs reads a JSON-LD value that is one {"@id"} or a list of them.
type refs []ref

func (r *refs) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		var l []ref
		err := json.Unmarshal(b, &l)
		*r = l
		return err
	}
	var x ref
	if err := json.Unmarshal(b, &x); err != nil {
		return nil // a literal: not a reference
	}
	*r = refs{x}
	return nil
}

// text reads a JSON-LD literal that is a string or {"@value"}.
type text string

func (t *text) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = text(s)
		return nil
	}
	var v struct {
		Value string `json:"@value"`
	}
	if json.Unmarshal(b, &v) == nil {
		*t = text(v.Value)
	}
	return nil
}

type node struct {
	ID       string `json:"@id"`
	D3ID     text   `json:"d3f:d3fend-id"`
	Label    text   `json:"rdfs:label"`
	SubClass refs   `json:"rdfs:subClassOf"`
	Enables  refs   `json:"d3f:enables"`
	OnProp   refs   `json:"owl:onProperty"`
	Some     refs   `json:"owl:someValuesFrom"`
	Version  text   `json:"owl:versionInfo"`
	Release  text   `json:"d3f:release-date"`
}

type technique struct {
	iri, id, name, tactic, parent string
	attack                        []string
}

type list struct {
	version, release string
	techniques       []technique
}

// parse reads the defensive techniques of the ontology: the classes with
// a D3- id below d3f:DefensiveTechnique, with the tactic they enable
// (inherited) and the nearest parent that is a technique.
func parse(b []byte) (list, error) {
	var g struct {
		Graph []node `json:"@graph"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		return list{}, err
	}
	byID := map[string]*node{}
	var l list
	for i := range g.Graph {
		n := &g.Graph[i]
		byID[n.ID] = n
		if n.Version != "" && strings.HasSuffix(n.ID, "d3fend.owl") {
			l.version, l.release = string(n.Version), string(n.Release)
			if len(l.release) >= 10 {
				l.release = l.release[:10]
			}
		}
	}
	if l.version == "" {
		return l, fmt.Errorf("no D3FEND ontology version")
	}
	// tactic enabled by the class or a restriction on it
	enables := func(n *node) string {
		for _, e := range n.Enables {
			if t := strings.TrimPrefix(e.ID, "d3f:"); contains(tactics, t) {
				return t
			}
		}
		for _, s := range n.SubClass {
			if r := byID[s.ID]; r != nil && strings.HasPrefix(r.ID, "_:") && len(r.OnProp) == 1 && r.OnProp[0].ID == "d3f:enables" {
				for _, v := range r.Some {
					if t := strings.TrimPrefix(v.ID, "d3f:"); contains(tactics, t) {
						return t
					}
				}
			}
		}
		return ""
	}
	var isDefensive func(id string, depth int) bool
	isDefensive = func(id string, depth int) bool {
		if id == "d3f:DefensiveTechnique" {
			return true
		}
		n := byID[id]
		if n == nil || depth > 30 {
			return false
		}
		for _, s := range n.SubClass {
			if !strings.HasPrefix(s.ID, "_:") && isDefensive(s.ID, depth+1) {
				return true
			}
		}
		return false
	}
	for i := range g.Graph {
		n := &g.Graph[i]
		if !strings.HasPrefix(string(n.D3ID), "D3-") || !isDefensive(n.ID, 0) {
			continue
		}
		t := technique{iri: n.ID, id: string(n.D3ID), name: strings.TrimSpace(string(n.Label))}
		// walk up: the first ancestor with a D3- id is the parent; the
		// first tactic found is the tactic
		cur := n
		for depth := 0; cur != nil && depth < 30; depth++ {
			if t.tactic == "" {
				t.tactic = enables(cur)
			}
			var next *node
			for _, s := range cur.SubClass {
				if p := byID[s.ID]; p != nil && !strings.HasPrefix(p.ID, "_:") && isDefensive(p.ID, 0) {
					next = p
					break
				}
			}
			if next == nil {
				break
			}
			if t.parent == "" && strings.HasPrefix(string(next.D3ID), "D3-") {
				t.parent = string(next.D3ID)
			}
			cur = next
		}
		if t.tactic == "" {
			return l, fmt.Errorf("%s (%s): no tactic", t.id, t.iri)
		}
		l.techniques = append(l.techniques, t)
	}
	if len(l.techniques) == 0 {
		return l, fmt.Errorf("no defensive technique")
	}
	rank := map[string]int{}
	for i, t := range tactics {
		rank[t] = i
	}
	sort.Slice(l.techniques, func(i, j int) bool {
		a, b := l.techniques[i], l.techniques[j]
		if rank[a.tactic] != rank[b.tactic] {
			return rank[a.tactic] < rank[b.tactic]
		}
		return a.id < b.id
	})
	return l, nil
}

// offensive returns the ATT&CK technique ids of a D3FEND API technique
// page (def_to_off: the inferred offensive techniques), sorted.
func offensive(b []byte) ([]string, error) {
	var p struct {
		DefToOff struct {
			Results struct {
				Bindings []struct {
					ID struct {
						Value string `json:"value"`
					} `json:"off_tech_id"`
				} `json:"bindings"`
			} `json:"results"`
		} `json:"def_to_off"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, x := range p.DefToOff.Results.Bindings {
		if id := x.ID.Value; id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

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
	sb.WriteString("# MITRE D3FEND defensive tactics and techniques, with the ATT&CK techniques\n")
	sb.WriteString("# each one counters (inferred by D3FEND through the digital artifacts, only\n")
	sb.WriteString("# ids of attack.yaml). Generated by internal/catalog/cmd/d3fendgen from the\n")
	sb.WriteString("# D3FEND ontology and API: do not edit by hand. See FORMAT.md, \"MITRE D3FEND\".\n")
	sb.WriteString("# D3FEND™ is a trademark of The MITRE Corporation; the list reproduces ids\n")
	sb.WriteString("# and names of the D3FEND knowledge graph (MIT licence).\n")
	fmt.Fprintf(&sb, "version: %s\nrelease: %s\nurl: https://d3fend.mitre.org/\n", q(l.version), q(l.release))
	sb.WriteString("tactics:\n")
	for _, t := range tactics {
		fmt.Fprintf(&sb, "  - {id: %s, name: %s, url: https://d3fend.mitre.org/tactic/d3f:%s/}\n", t, q(t), t)
	}
	sb.WriteString("techniques:\n")
	for _, t := range l.techniques {
		fmt.Fprintf(&sb, "  - {id: %s, name: %s, tactic: %s", t.id, q(t.name), t.tactic)
		if t.parent != "" {
			fmt.Fprintf(&sb, ", parent: %s", t.parent)
		}
		fmt.Fprintf(&sb, ", url: %s", q("https://d3fend.mitre.org/technique/"+t.iri+"/"))
		if len(t.attack) > 0 {
			fmt.Fprintf(&sb, ", attack: [%s]", strings.Join(t.attack, ", "))
		}
		sb.WriteString("}\n")
	}
	_, err := io.WriteString(w, sb.String())
	return err
}
