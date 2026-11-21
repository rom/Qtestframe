package catalog

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// The generated section of the guide's references page is current. After
// a change to the registry or the catalogue, run `make refs-docs` (go run
// ./internal/catalog/cmd/refsdoc) and commit the pages.
func TestReferencesDocIsCurrent(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range Languages {
		path := filepath.Join("..", "..", "docs", "user-guide", lang, RefsDocPage)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want, err := ReplaceRefsDoc(string(b), c.SourcesMarkdown(lang))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != want {
			t.Errorf("%s: the generated section %q is out of date: run make refs-docs", path, refsDocTexts[lang].heading)
		}
	}
}

// Every source of the built-in registry with a url, links or ref_links
// has well-formed https links (also checked by the loader), and most have one.
func TestSourcesHaveLinks(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	without := 0
	for _, s := range c.Sources {
		if s.URL == "" && len(s.Links) == 0 && len(s.RefLinks) == 0 {
			without++
		}
	}
	if without*10 > len(c.Sources) {
		t.Errorf("%d of %d sources have no link", without, len(c.Sources))
	}
}

func TestDocLinks(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[string]string{
		"RFC 2119":       "https://www.rfc-editor.org/rfc/rfc2119",
		"RFC 9110 §15.5": "https://www.rfc-editor.org/rfc/rfc9110",
		"RFC 6265bis":    "https://www.rfc-editor.org/",
		"IETF draft-ietf-httpapi-ratelimit-headers":        "https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/",
		"GDPR Art. 32(1)(a)":                               "https://eur-lex.europa.eu/eli/reg/2016/679/oj",
		"ISO/IEC 27033-2:2012":                             "https://www.iso.org/standard/51581.html",
		"EN 55032:2015":                                    "https://webstore.iec.ch/en/publication/22046",
		"CWE-79":                                           "https://cwe.mitre.org/data/definitions/79.html",
		"CWE-1464 (improper session invalidation)":         "https://cwe.mitre.org/data/definitions/1464.html",
		"CWE Top 25 Most Dangerous Software Weaknesses":    "https://cwe.mitre.org/top25/",
		"MITRE CWE":                                        "https://cwe.mitre.org/",
		"MITRE ATT&CK T1078 (valid accounts)":              "https://attack.mitre.org/techniques/T1078/",
		"MITRE ATT&CK T1110.001 (password guessing)":       "https://attack.mitre.org/techniques/T1110/001/",
		"MITRE ATT&CK M1032 (multi-factor authentication)": "https://attack.mitre.org/mitigations/M1032/",
		"MITRE ATT&CK TA0010 (exfiltration)":               "https://attack.mitre.org/tactics/TA0010/",
		"MITRE ATT&CK T0886 (remote services)":             "https://attack.mitre.org/techniques/T0886/",
		"MITRE ATT&CK T1693.001 (system firmware)":         "https://attack.mitre.org/techniques/T1693/001/",
		"MITRE ATT&CK M0930 (network segmentation)":        "https://attack.mitre.org/mitigations/M0930/",
		"MITRE ATT&CK T1638 (adversary-in-the-middle)":     "https://attack.mitre.org/techniques/T1638/",
		"MITRE ATT&CK Containers Matrix":                   "https://attack.mitre.org/matrices/enterprise/containers/",
		"MITRE ATT&CK":                                     "https://attack.mitre.org/",
		"3GPP TS 23.501":                                   "https://www.3gpp.org/DynaReport/23501.htm",
		"ITU-T G.8275.1":                                   "https://www.itu.int/rec/T-REC-G.8275.1",
	} {
		ts := c.RefTargets([]string{ref}, "sv")
		if len(ts) != 1 || ts[0].URL != want || ts[0].Source == "" || ts[0].Name == "" {
			t.Errorf("%s: %+v, want %s", ref, ts, want)
		}
	}
	if ts := c.RefTargets([]string{"Nothing cites this"}, "en"); ts[0].Source != "" || ts[0].URL != "" {
		t.Errorf("unknown ref: %+v", ts[0])
	}
	docs := c.CitedDocuments("ietf-rfc")
	if len(docs) < 20 || !slices.IsSortedFunc(docs, func(a, b DocLink) int {
		if naturalLess(a.Label, b.Label) {
			return -1
		}
		return 1
	}) {
		t.Errorf("cited RFCs %v", docs)
	}
	// one cited document per technique, sub-technique and mitigation, and
	// the analytics group sub-techniques under their technique
	attack := c.CitedDocuments("mitre-attack")
	labels := map[string]bool{}
	for _, d := range attack {
		labels[d.Label] = true
	}
	for _, l := range []string{"MITRE ATT&CK T1110", "MITRE ATT&CK T1110.001", "MITRE ATT&CK M1032", "MITRE ATT&CK T0886"} {
		if !labels[l] {
			t.Errorf("cited ATT&CK documents miss %s", l)
		}
	}
	if s := c.Source("mitre-attack"); s.ClauseOf("MITRE ATT&CK T1110.001 (password guessing)") != "T1110" || s.ClauseOf("MITRE ATT&CK M1032 (multi-factor authentication)") != "M1032" {
		t.Errorf("ATT&CK clauses %q %q", s.ClauseOf("MITRE ATT&CK T1110.001 (password guessing)"), s.ClauseOf("MITRE ATT&CK M1032 (multi-factor authentication)"))
	}
	if c.CitedDocuments("gdpr") != nil || c.CitedDocuments("nope") != nil {
		t.Error("documents of a source without ref_links")
	}
}

func TestNaturalLess(t *testing.T) {
	for _, x := range [][2]string{{"RFC 791", "RFC 2119"}, {"RFC 2119", "RFC 8174"}, {"a", "b"}, {"a", "ab"}, {"x 01", "x 2"}, {"x 1 a", "x 1 b"}} {
		if !naturalLess(x[0], x[1]) || naturalLess(x[1], x[0]) {
			t.Errorf("%q < %q", x[0], x[1])
		}
	}
}

func TestReplaceRefsDoc(t *testing.T) {
	sec := RefsDocBegin + "\nnew\n" + RefsDocEnd
	got, err := ReplaceRefsDoc("# R\n\ntext\n", sec)
	if err != nil || got != "# R\n\ntext\n\n"+sec+"\n" {
		t.Errorf("append: %q %v", got, err)
	}
	got, err = ReplaceRefsDoc("a\n"+RefsDocBegin+"\nold\n"+RefsDocEnd+"\nb\n", sec)
	if err != nil || got != "a\n"+sec+"\nb\n" {
		t.Errorf("replace: %q %v", got, err)
	}
	if _, err := ReplaceRefsDoc("a\n"+RefsDocBegin+"\n", sec); err == nil {
		t.Error("unbalanced markers accepted")
	}
}

// Sources without a url are listed without a link; links and cited
// documents are listed below their source; Markdown in names is escaped.
func TestSourcesMarkdown(t *testing.T) {
	reg := []byte(`kinds:
  - id: standard
    name: {en: Standards, sv: Standarder}
sources:
  - id: plain
    kind: standard
    jurisdiction: SE
    name: {en: "Plain *source*", sv: "Enkel [källa]"}
    match: ["Plain"]
  - id: family
    kind: standard
    jurisdiction: international
    short: FAM
    name: {en: "Family", sv: "Familj"}
    match: ["Fam "]
    url: https://example.org/fam
    links:
      - {label: "Fam 1", url: "https://example.org/fam/1"}
    ref_links:
      - {regex: '^Fam (\d+)', label: 'Fam $1', url: 'https://example.org/doc/$1'}
`)
	area := []byte(`area: x
prefix: XX
group: g
name: {en: X, sv: X}
description: {en: X, sv: X}
requirements:
  - id: XX-01
    type: security
    obligation: must
    priority: high
    refs: ["Plain §1", "Fam 12 §3", "Fam 2"]
    tests: [XX-T01]
    en: {title: T, description: The system MUST log every sign-in., rationale: R, criteria: [{method: test, text: C}]}
    sv: {title: T, description: Systemet SKA logga varje inloggning., rationale: R, criteria: [{method: test, text: C}]}
tests:
  - id: XX-T01
    priority: high
    execution: manual
    en: {title: T, description: D, preconditions: P, expected: E, steps: [{action: A, expected: E}]}
    sv: {title: T, description: D, preconditions: P, expected: E, steps: [{action: A, expected: E}]}
`)
	groups := []byte("groups:\n  - id: g\n    name: {en: G, sv: G}\n")
	c, err := Load(fstest.MapFS{"d/groups.yaml": {Data: groups}, "d/x.yaml": {Data: area}, "d/sources.yaml": {Data: reg}}, "d")
	if err != nil {
		t.Fatal(err)
	}
	en := c.SourcesMarkdown("en")
	for _, want := range []string{
		RefsDocBegin, RefsDocEnd, "## Sources cited in the catalogue", "cites 2 laws",
		"#### Standards: International\n\n- [Family](https://example.org/fam) (FAM): 1 requirement, 1 test\n" +
			"  - [Fam 1](https://example.org/fam/1) · [Fam 2](https://example.org/doc/2) · [Fam 12](https://example.org/doc/12)\n",
		"#### Standards: Sweden\n\n- Plain \\*source\\*: 1 requirement, 1 test\n",
	} {
		if !strings.Contains(en, want) {
			t.Errorf("missing %q in\n%s", want, en)
		}
	}
	if strings.Index(en, "International") > strings.Index(en, "Sweden") {
		t.Error("international after Sweden")
	}
	sv := c.SourcesMarkdown("sv")
	if !strings.Contains(sv, "- Enkel \\[källa\\]: 1 krav, 1 testfall") || !strings.Contains(sv, "#### Standarder: Sverige") {
		t.Errorf("sv:\n%s", sv)
	}
	if d, ok := c.Source("family").DocLink("Fam 1 §2"); !ok || d.URL != "https://example.org/doc/1" {
		t.Errorf("ref_links before links: %+v", d)
	}
	if _, ok := c.Source("plain").DocLink("Plain"); ok {
		t.Error("link for a source without url")
	}

	bad := []byte(`kinds:
  - id: standard
    name: {en: Standards, sv: Standarder}
sources:
  - id: b
    kind: standard
    name: {en: B, sv: B}
    match: ["B"]
    links:
      - {label: "", url: "https://example.org"}
      - {label: "L", url: "http://example.org"}
      - {label: "M", url: "https://example.org", match: [""]}
    ref_links:
      - {regex: '(', label: 'x', url: 'https://example.org'}
      - {regex: 'x', label: '', url: 'https://example.org'}
      - {regex: 'y', label: 'y', url: 'ftp://example.org'}
`)
	_, err = Load(fstest.MapFS{"d/groups.yaml": {Data: groups}, "d/sources.yaml": {Data: bad}}, "d")
	if err == nil {
		t.Fatal("bad links accepted")
	}
	for _, want := range []string{"empty or duplicate label", "link L: url must start with https://", "link M: empty match prefix", "ref_links regex", "label is needed", `ref_links "y": url`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%s", want, err)
		}
	}
}
