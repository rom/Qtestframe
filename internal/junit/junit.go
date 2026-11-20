// Package junit parses JUnit XML reports (as written by Playwright, pytest,
// go-junit-report, Maven Surefire, Jest, Vitest and most other test tools)
// and maps their test cases to Qtestframe test keys.
package junit

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Statuses of a parsed test case (they equal the Qtestframe results).
const (
	Passed  = "passed"
	Failed  = "failed"
	Skipped = "skipped"
)

// MaxOutput limits the output text kept per test case and per result.
const MaxOutput = 64 << 10

// KeyProperties are the testcase property names that carry a test key, in
// order of preference ("qtestframe" is what Playwright writes for an
// annotation of type qtestframe with embedAnnotationsAsProperties).
var KeyProperties = []string{"qtestframe.key", "test_key", "qtestframe"}

// KeyPattern finds a test key such as SHOP-TC-12 in a test name. Underscores
// are accepted as separators (SHOP_TC_12) because many languages do not
// allow hyphens in function names; they are normalised to hyphens.
var KeyPattern = regexp.MustCompile(`[A-Z][A-Z0-9]{1,15}[-_]TC[-_][0-9]+`)

// ErrInvalid is wrapped by all parse errors.
var ErrInvalid = errors.New("invalid JUnit XML")

// Case is one <testcase>.
type Case struct {
	Name      string
	ClassName string
	// Suite is the name of the innermost enclosing <testsuite>.
	Suite string
	// Seconds is the duration reported in the time attribute.
	Seconds    float64
	Status     string
	Properties []Property
	// Output combines failure/error/skipped messages and system-out/err.
	Output string
}

// Property is a <property name value> (or with the value as text).
type Property struct {
	Name, Value string
}

// DurationMS returns the duration in milliseconds.
func (c *Case) DurationMS() int64 { return int64(math.Round(c.Seconds * 1000)) }

// Key returns the Qtestframe test key of the case: a key property, else the
// first key in the name, else in the class name; "" if there is none.
func (c *Case) Key() string {
	for _, want := range KeyProperties {
		for _, p := range c.Properties {
			if strings.EqualFold(p.Name, want) {
				if v := strings.ToUpper(strings.TrimSpace(p.Value)); v != "" {
					return v
				}
			}
		}
	}
	for _, s := range []string{c.Name, c.ClassName} {
		if k := KeyPattern.FindString(s); k != "" {
			return strings.ReplaceAll(k, "_", "-")
		}
	}
	return ""
}

// ---------------------------------------------------------------- parsing

type xmlSuite struct {
	XMLName    xml.Name
	Name       string        `xml:"name,attr"`
	Suites     []xmlSuite    `xml:"testsuite"`
	Cases      []xmlCase     `xml:"testcase"`
	Properties []xmlProperty `xml:"properties>property"`
}

type xmlCase struct {
	Name       string        `xml:"name,attr"`
	ClassName  string        `xml:"classname,attr"`
	Time       string        `xml:"time,attr"`
	Failures   []xmlProblem  `xml:"failure"`
	Errors     []xmlProblem  `xml:"error"`
	Skipped    []xmlProblem  `xml:"skipped"`
	SystemOut  []string      `xml:"system-out"`
	SystemErr  []string      `xml:"system-err"`
	Properties []xmlProperty `xml:"properties>property"`
}

type xmlProblem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type xmlProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
	Text  string `xml:",chardata"`
}

// Parse reads one JUnit XML document (root <testsuites> or <testsuite>).
func Parse(r io.Reader) ([]Case, error) {
	d := xml.NewDecoder(r)
	d.CharsetReader = charsetReader
	var root xmlSuite
	if err := d.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: empty document", ErrInvalid)
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if n := root.XMLName.Local; n != "testsuites" && n != "testsuite" {
		return nil, fmt.Errorf("%w: root element is <%s>, expected <testsuites> or <testsuite>", ErrInvalid, n)
	}
	// nothing but whitespace, comments or processing instructions may follow
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if _, ok := tok.(xml.StartElement); ok {
			return nil, fmt.Errorf("%w: more than one root element", ErrInvalid)
		}
	}
	var out []Case
	walk(&root, &out)
	return out, nil
}

// ParseBytes is Parse for an in-memory document.
func ParseBytes(b []byte) ([]Case, error) { return Parse(bytes.NewReader(b)) }

func walk(s *xmlSuite, out *[]Case) {
	for i := range s.Cases {
		*out = append(*out, convert(&s.Cases[i], s.Name))
	}
	for i := range s.Suites {
		walk(&s.Suites[i], out)
	}
}

func convert(x *xmlCase, suite string) Case {
	c := Case{Name: strings.TrimSpace(x.Name), ClassName: strings.TrimSpace(x.ClassName), Suite: suite,
		Seconds: parseSeconds(x.Time), Status: Passed}
	for _, p := range x.Properties {
		v := p.Value
		if v == "" {
			v = p.Text
		}
		c.Properties = append(c.Properties, Property{Name: strings.TrimSpace(p.Name), Value: strings.TrimSpace(v)})
	}
	var b strings.Builder
	problem := func(label string, ps []xmlProblem) {
		for _, p := range ps {
			head := strings.TrimSpace(strings.Join(nonEmpty(p.Type, p.Message), ": "))
			text := strings.TrimSpace(p.Text)
			if head == "" && text == "" {
				continue
			}
			section(&b, label)
			if head != "" {
				b.WriteString(head)
				b.WriteByte('\n')
			}
			if text != "" && text != head {
				b.WriteString(text)
				b.WriteByte('\n')
			}
		}
	}
	switch {
	case len(x.Failures) > 0 || len(x.Errors) > 0:
		c.Status = Failed
	case len(x.Skipped) > 0:
		c.Status = Skipped
	}
	problem("failure", x.Failures)
	problem("error", x.Errors)
	problem("skipped", x.Skipped)
	for _, label := range []string{"system-out", "system-err"} {
		texts := x.SystemOut
		if label == "system-err" {
			texts = x.SystemErr
		}
		for _, t := range texts {
			if t = strings.TrimSpace(t); t != "" {
				section(&b, label)
				b.WriteString(t)
				b.WriteByte('\n')
			}
		}
	}
	c.Output = Truncate(strings.TrimRight(b.String(), "\n"), MaxOutput)
	return c
}

func section(b *strings.Builder, label string) {
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString("[" + label + "]\n")
}

func nonEmpty(v ...string) []string {
	var out []string
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseSeconds accepts "1.5", "1,234.5" (Surefire with grouping) and "";
// invalid or negative values are 0.
func parseSeconds(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// charsetReader supports the single-byte encodings some older tools
// declare; everything else must be UTF-8.
func charsetReader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return in, nil
	case "iso-8859-1", "latin1", "latin-1", "iso8859-1", "windows-1252", "cp1252":
		b, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		return strings.NewReader(string(r)), nil
	}
	return nil, fmt.Errorf("unsupported encoding %q", label)
}

// Truncate shortens s to at most max bytes (on a rune boundary) and marks
// the cut.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	const mark = "\n… (truncated)"
	cut := max - len(mark)
	if cut < 0 {
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + mark
}

// ---------------------------------------------------------------- mapping

// Result is the combined outcome of all test cases with one key (or, for
// cases without a key, with one name).
type Result struct {
	// Key is the test key ("" for unmapped results).
	Key        string
	Name       string
	Status     string
	DurationMS int64
	Output     string
	// Cases counts the test cases that were combined.
	Cases int
}

// Aggregate groups cases by key. Cases without a key are grouped by name
// and returned as unmapped. Several cases with one key combine: failed wins
// over passed, passed over skipped; durations add up; outputs are joined.
// Order follows the first occurrence.
func Aggregate(cases []Case) (mapped, unmapped []Result) {
	idx := map[string]int{}
	uidx := map[string]int{}
	for i := range cases {
		c := &cases[i]
		key := c.Key()
		list, m, id := &mapped, idx, key
		if key == "" {
			list, m, id = &unmapped, uidx, strings.Join(strings.Fields(displayName(c)), " ")
		}
		j, ok := m[id]
		if !ok {
			m[id] = len(*list)
			*list = append(*list, Result{Key: key, Name: displayName(c), Status: c.Status, DurationMS: c.DurationMS(), Output: c.Output, Cases: 1})
			continue
		}
		r := &(*list)[j]
		if r.Cases == 1 && r.Output != "" {
			r.Output = "== " + r.Name + "\n" + r.Output
		}
		r.Cases++
		r.Status = combine(r.Status, c.Status)
		r.DurationMS += c.DurationMS()
		if c.Output != "" {
			if r.Output != "" {
				r.Output += "\n\n"
			}
			r.Output = Truncate(r.Output+"== "+displayName(c)+"\n"+c.Output, MaxOutput)
		}
	}
	return mapped, unmapped
}

func displayName(c *Case) string {
	if c.Name != "" {
		return c.Name
	}
	return c.ClassName
}

var rank = map[string]int{Skipped: 0, Passed: 1, Failed: 2}

func combine(a, b string) string {
	if rank[b] > rank[a] {
		return b
	}
	return a
}
