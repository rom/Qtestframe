// Package stubgen generates automated-test stubs from Qtestframe test
// cases: one source file per test for pytest, Playwright, Go testing,
// JUnit 5, Jest, Vitest, Robot Framework and Cucumber, named so that the
// JUnit XML of a run maps back to the test key (see internal/junit) and
// marked so that an unimplemented stub reports as skipped, not passed.
//
// Every file starts with a header block between the marker lines
// "qtestframe:begin KEY FRAMEWORK body=HASH" and "qtestframe:end" (in the
// language's line comments). The header documents the test (title,
// requirements, preconditions, expected result) and records the hash of
// the generated body; Merge uses it to regenerate a file idempotently:
// an unchanged stub is replaced by the new stub, a file whose body was
// edited (implemented) only gets its header block rewritten, and a file
// without markers is left alone.
package stubgen

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/gherkin"
	"github.com/rom/qtestframe/internal/model"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Framework is a test framework stubs can be generated for.
type Framework struct {
	// ID is the value of --framework / ?framework=.
	ID string `json:"id"`
	// Name is the display name.
	Name string `json:"name"`
	// Language is the source language (python, typescript, go, java,
	// robot, gherkin).
	Language string `json:"language"`
	// Runner is the runner:<name> tag of qtestframe select's child
	// pipeline jobs that run these tests.
	Runner string `json:"runner"`
	// ScriptLanguage is the test's script language that matches the
	// framework ("" when none of model.ScriptLanguages does).
	ScriptLanguage string `json:"script_language,omitempty"`
	// Parametrised says whether data rows become a parametrised test.
	Parametrised bool   `json:"parametrised"`
	comment      string // line comment prefix
	tmpl         string
}

// Frameworks lists the supported frameworks in display order.
var Frameworks = []Framework{
	{ID: "pytest", Name: "pytest", Language: "python", Runner: "pytest", ScriptLanguage: "python", Parametrised: true, comment: "#", tmpl: "pytest.tmpl"},
	{ID: "playwright", Name: "Playwright (TypeScript)", Language: "typescript", Runner: "playwright", ScriptLanguage: "playwright", Parametrised: true, comment: "//", tmpl: "playwright.tmpl"},
	{ID: "go", Name: "Go testing", Language: "go", Runner: "go", Parametrised: true, comment: "//", tmpl: "go.tmpl"},
	{ID: "junit5", Name: "JUnit 5 (Java)", Language: "java", Runner: "maven", Parametrised: true, comment: "//", tmpl: "junit5.tmpl"},
	{ID: "jest", Name: "Jest (TypeScript)", Language: "typescript", Runner: "jest", ScriptLanguage: "javascript", Parametrised: true, comment: "//", tmpl: "jest.tmpl"},
	{ID: "vitest", Name: "Vitest (TypeScript)", Language: "typescript", Runner: "vitest", ScriptLanguage: "javascript", Parametrised: true, comment: "//", tmpl: "jest.tmpl"},
	{ID: "robot", Name: "Robot Framework", Language: "robot", Runner: "robot", Parametrised: true, comment: "#", tmpl: "robot.tmpl"},
	{ID: "cucumber", Name: "Cucumber (Gherkin)", Language: "gherkin", Runner: "cucumber", Parametrised: true, comment: "#", tmpl: "cucumber.tmpl"},
}

// FrameworkIDs returns the ids of Frameworks.
func FrameworkIDs() []string {
	out := make([]string, len(Frameworks))
	for i, f := range Frameworks {
		out[i] = f.ID
	}
	return out
}

// Lookup returns the framework with an id (or a common alias).
func Lookup(id string) (Framework, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	switch id {
	case "python", "py":
		id = "pytest"
	case "golang", "go-testing":
		id = "go"
	case "junit", "java", "junit-5":
		id = "junit5"
	case "robotframework", "robot-framework":
		id = "robot"
	case "gherkin", "feature":
		id = "cucumber"
	case "playwright-ts", "pw":
		id = "playwright"
	}
	for _, f := range Frameworks {
		if f.ID == id {
			return f, true
		}
	}
	return Framework{}, false
}

// ErrUnknownFramework is returned for an unsupported framework.
var ErrUnknownFramework = errors.New("unknown framework")

// Test is what a stub is generated from.
type Test struct {
	Key           string
	Title         string
	Description   string
	Preconditions string
	Expected      string
	Priority      string
	// URL links to the test in Qtestframe ("" none).
	URL string
	// Requirements are the keys of the requirements it verifies.
	Requirements []string
	Tags         []string
	Steps        []Step
	// Parameters and Rows: the test data; each row becomes one case of a
	// parametrised test.
	Parameters []string
	Rows       []Row
	// Gherkin is the feature of a test in the Gherkin format ("" for a
	// test in the step format): the Cucumber stub is that feature, tagged
	// with the key.
	Gherkin string
}

// Step is one step of a test.
type Step struct {
	Action, Expected, Data string
	// Call is "KEY title" of a called test ("" for an ordinary step).
	Call string
}

// Row is one row of test data.
type Row struct {
	Name   string
	Values map[string]string
}

// FromModel converts a test case (with its steps) and the keys of the
// requirements it verifies.
func FromModel(t *model.TestCase, requirements []string, url string) Test {
	out := Test{Key: t.Key, Title: t.Title, Description: t.Description, Preconditions: t.Preconditions,
		Expected: t.ExpectedResult, Priority: t.Priority, URL: url, Tags: append([]string(nil), t.Tags...),
		Parameters: append([]string(nil), t.Parameters...)}
	reqs := append([]string(nil), requirements...)
	sort.Strings(reqs)
	out.Requirements = reqs
	for _, s := range t.Steps {
		st := Step{Action: s.Action, Expected: s.Expected, Data: s.Data}
		if s.CallTestID != nil {
			st.Call = strings.TrimSpace(s.CallTestKey + " " + s.CallTestTitle)
			if st.Call == "" {
				st.Call = "another test"
			}
		}
		out.Steps = append(out.Steps, st)
	}
	for _, r := range t.DataRows {
		out.Rows = append(out.Rows, Row{Name: r.Name, Values: r.Values})
	}
	if t.Format == model.FormatGherkin && strings.TrimSpace(t.Gherkin) != "" {
		out.Gherkin = t.Gherkin
		// the other frameworks document the scenarios as steps
		if doc, err := gherkin.Parse(t.Gherkin); err == nil {
			pt, _ := gherkin.ToPlain(doc)
			if strings.TrimSpace(out.Preconditions) == "" {
				out.Preconditions = pt.Preconditions
			}
			out.Steps = nil
			for _, st := range pt.Steps {
				out.Steps = append(out.Steps, Step{Action: st.Action, Expected: st.Expected, Data: st.Data})
			}
			out.Parameters, out.Rows = pt.Parameters, nil
			for _, r := range pt.Rows {
				out.Rows = append(out.Rows, Row{Name: r.Name, Values: r.Values})
			}
		}
	}
	return out
}

// gherkinBody is the Cucumber stub of a Gherkin test: its feature with
// the key and the stub tag ("" when it does not parse).
func gherkinBody(t Test) string {
	if strings.TrimSpace(t.Gherkin) == "" {
		return ""
	}
	src, err := gherkin.AddTags(t.Gherkin, []string{"@" + oneLine(t.Key), "@qtestframe-stub"})
	if err != nil {
		return ""
	}
	return src
}

// Options tune the generated code.
type Options struct {
	// Package is the Go package (default "tests") or the Java package
	// (default none) of the stubs.
	Package string
}

// File is a generated stub.
type File struct {
	// Path is the file name (relative, no directory).
	Path    string `json:"path"`
	Content string `json:"content"`
	Key     string `json:"key"`
	// Framework is the framework id.
	Framework string `json:"framework"`
}

// Ident returns the key as an identifier: SHOP-TC-12 → SHOP_TC_12 (what
// the JUnit key pattern accepts in function and class names).
func Ident(key string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(key)) {
		if r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// FileName returns the file name of a test's stub.
func FileName(fw, key string) string {
	id := Ident(key)
	k := strings.ReplaceAll(id, "_", "-")
	switch fw {
	case "pytest":
		return "test_" + id + ".py"
	case "go":
		return strings.ToLower(id) + "_test.go"
	case "junit5":
		return id + "Test.java"
	case "playwright":
		return k + ".spec.ts"
	case "jest", "vitest":
		return k + ".test.ts"
	case "robot":
		return k + ".robot"
	case "cucumber":
		return k + ".feature"
	}
	return id
}

// Generate renders the stub of a test.
func Generate(fwID string, t Test, o Options) (File, error) {
	fw, ok := Lookup(fwID)
	if !ok {
		return File{}, fmt.Errorf("%w %q (use one of %s)", ErrUnknownFramework, fwID, strings.Join(FrameworkIDs(), ", "))
	}
	if strings.TrimSpace(t.Key) == "" {
		return File{}, errors.New("the test has no key")
	}
	d := newData(fw, t, o)
	var body bytes.Buffer
	if g := gherkinBody(t); fw.ID == "cucumber" && g != "" {
		body.WriteString(g)
	} else if err := templates().ExecuteTemplate(&body, fw.tmpl, d); err != nil {
		return File{}, err
	}
	src := tidy(body.String())
	// a "# language:" header stays the first line of a feature file
	lang := ""
	if fw.ID == "cucumber" && strings.HasPrefix(src, "# language:") {
		first, rest, _ := strings.Cut(src, "\n")
		lang, src = first+"\n", strings.TrimLeft(rest, "\n")
	}
	if fw.ID == "go" {
		b, err := format.Source([]byte(src))
		if err != nil {
			return File{}, fmt.Errorf("generated Go code does not parse: %w", err)
		}
		src = string(b)
	}
	head := header(fw, t, bodyHash(src))
	return File{Path: FileName(fw.ID, t.Key), Content: lang + head + "\n" + src, Key: t.Key, Framework: fw.ID}, nil
}

// tidy removes trailing spaces and runs of blank lines, and ends the text
// with exactly one newline.
func tidy(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	blanks := 2 // drop leading blank lines
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			// at most two blank lines in a row (PEP 8 between definitions)
			if blanks >= 2 {
				continue
			}
			blanks++
		} else {
			blanks = 0
		}
		out = append(out, l)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n") + "\n"
}

// ---------------------------------------------------------------- header

const (
	beginMarker = "qtestframe:begin"
	endMarker   = "qtestframe:end"
)

func header(fw Framework, t Test, hash string) string {
	c := fw.comment
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	add(fmt.Sprintf("%s %s %s %s body=%s", c, beginMarker, t.Key, fw.ID, hash))
	add(c + ` Generated by "qtestframe stubs". Keep the marker lines: generating again`)
	add(c + " rewrites this block, and the rest of the file only while it is unchanged.")
	add(c)
	if strings.TrimSpace(t.Title) == "" {
		add(c + " " + oneLine(t.Key))
	} else {
		add(c + " " + oneLine(t.Key+": "+t.Title))
	}
	if t.URL != "" {
		add(c + " " + oneLine(t.URL))
	}
	if len(t.Requirements) > 0 {
		add(c + " Requirements: " + oneLine(strings.Join(t.Requirements, ", ")))
	}
	if t.Priority != "" {
		add(c + " Priority: " + oneLine(t.Priority))
	}
	if len(t.Tags) > 0 {
		add(c + " Tags: " + oneLine(strings.Join(t.Tags, ", ")))
	}
	section := func(title, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		add(c)
		add(c + " " + title + ":")
		for _, l := range textLines(text) {
			add(strings.TrimRight(c+"   "+l, " "))
		}
	}
	section("Description", t.Description)
	section("Preconditions", t.Preconditions)
	section("Expected result", t.Expected)
	add(c + " " + endMarker)
	out := strings.Join(lines, "\n") + "\n"
	if fw.Language == "java" {
		out = javaComment(out)
	}
	return out
}

// textLines splits a text into lines without control characters.
func textLines(s string) []string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	var out []string
	for _, l := range strings.Split(s, "\n") {
		out = append(out, strings.TrimRight(clean(l), " "))
	}
	return out
}

// clean replaces control characters (and the Unicode line separators that
// some languages treat as line ends) with spaces.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || r == '\u0085' || r == '\ufeff' {
			return ' '
		}
		return r
	}, s)
}

// oneLine joins a text into one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(clean(s)), " ")
}

// javaComment doubles backslashes: javac reads \uXXXX escapes even in
// comments, so \u000a in a title would end the comment line.
func javaComment(s string) string { return strings.ReplaceAll(s, `\`, `\\`) }

// ---------------------------------------------------------------- template data

type param struct {
	Name  string // as entered
	Ident string // identifier in the target language
}

type row struct {
	Name   string
	Values []string // in the order of the parameters
}

type data struct {
	Test
	FW      Framework
	Ident   string // SHOP_TC_12
	Display string // "SHOP-TC-12 Title"
	Snake   string // words of the title: checkout_with_a_saved_card
	Camel   string // CheckoutWithASavedCard
	Package string
	Params  []param
	DRows   []row
}

func newData(fw Framework, t Test, o Options) *data {
	d := &data{Test: t, FW: fw, Ident: Ident(t.Key), Display: oneLine(t.Key + " " + t.Title), Package: o.Package}
	words := titleWords(t.Title)
	d.Snake = strings.Join(words, "_")
	for _, w := range words {
		d.Camel += strings.ToUpper(w[:1]) + w[1:]
	}
	if fw.ID == "go" && (d.Package == "" || !validIdent(d.Package)) {
		d.Package = "tests"
	}
	if fw.ID == "junit5" && d.Package != "" && !validJavaPackage(d.Package) {
		d.Package = ""
	}
	if len(t.Rows) > 0 && len(t.Parameters) > 0 {
		used := map[string]bool{"name": true, "row": true, "t": true, "page": true}
		for _, p := range t.Parameters {
			id := paramIdent(fw, p)
			for used[strings.ToLower(id)] {
				id += "_"
			}
			used[strings.ToLower(id)] = true
			d.Params = append(d.Params, param{Name: p, Ident: id})
		}
		names := map[string]int{}
		for i, r := range t.Rows {
			n := oneLine(r.Name)
			if n == "" {
				n = "row " + strconv.Itoa(i+1)
			}
			if names[n]++; names[n] > 1 {
				n += " (" + strconv.Itoa(names[n]) + ")"
			}
			dr := row{Name: n}
			for _, p := range t.Parameters {
				dr.Values = append(dr.Values, r.Values[p])
			}
			d.DRows = append(d.DRows, dr)
		}
	}
	return d
}

// Rows says whether the stub is parametrised.
func (d *data) Parametrised() bool { return len(d.DRows) > 0 }

// translit maps common accented Latin letters to ASCII.
var translit = map[rune]string{
	'å': "a", 'ä': "a", 'á': "a", 'à': "a", 'â': "a", 'ã': "a", 'æ': "ae",
	'ö': "o", 'ø': "o", 'ó': "o", 'ò': "o", 'ô': "o", 'õ': "o", 'œ': "oe",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e", 'í': "i", 'ì': "i", 'î': "i", 'ï': "i",
	'ú': "u", 'ù': "u", 'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y", 'ç': "c", 'ñ': "n", 'ß': "ss",
}

// asciiWords splits a text into lower-case ASCII words.
func asciiWords(s string) []string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		case translit[r] != "":
			b.WriteString(translit[r])
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String())
}

// titleWords: at most 8 words / 48 characters of the title.
func titleWords(title string) []string {
	var out []string
	n := 0
	for _, w := range asciiWords(title) {
		if len(out) == 8 || n+len(w) > 48 {
			break
		}
		out = append(out, w)
		n += len(w) + 1
	}
	return out
}

var keywords = map[string]map[string]bool{
	"python":     set("False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield pytest"),
	"go":         set("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var testing"),
	"java":       set("abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for goto if implements import instanceof int interface long native new package private protected public return short static strictfp super switch synchronized this throw throws transient try void volatile while true false null var record yield"),
	"typescript": set("break case catch class const continue debugger default delete do else enum export extends false finally for function if import in instanceof new null return super switch this throw true try typeof var void while with let static yield await implements interface package private protected public test expect"),
}

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// paramIdent makes a parameter name an identifier: snake_case for Python
// and Robot, lowerCamelCase for the others.
func paramIdent(fw Framework, name string) string {
	words := asciiWords(name)
	if len(words) == 0 {
		words = []string{"value"}
	}
	var id string
	switch fw.Language {
	case "python", "robot", "gherkin":
		id = strings.Join(words, "_")
	default:
		id = words[0]
		for _, w := range words[1:] {
			id += strings.ToUpper(w[:1]) + w[1:]
		}
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "p" + id
	}
	if keywords[fw.Language][id] {
		id += "_"
	}
	return id
}

func validIdent(s string) bool {
	if s == "" || keywords["go"][s] {
		return false
	}
	for i, r := range s {
		if !(r == '_' || r < utf8.RuneSelf && unicode.IsLetter(r) || i > 0 && r < utf8.RuneSelf && unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func validJavaPackage(s string) bool {
	for _, p := range strings.Split(s, ".") {
		if !validIdent(p) || keywords["java"][p] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- template functions

var tmpls *template.Template

func templates() *template.Template {
	if tmpls == nil {
		tmpls = template.Must(template.New("").Funcs(template.FuncMap{
			"py":      pyString,
			"pydoc":   pyDoc,
			"gostr":   strconv.Quote,
			"js":      jsString,
			"java":    javaString,
			"csv":     csvLine,
			"robot":   robotCell,
			"gherkin": gherkinText,
			"cell":    gherkinCell,
			"todo":    todo,
			"comment": commentLine,
			"join":    strings.Join,
			"oneline": oneLine,
			"outline": outline,
			"table":   gherkinTable,
			"lines":   textLines,
			"idents": func(ps []param) []string {
				out := make([]string, len(ps))
				for i, p := range ps {
					out[i] = p.Ident
				}
				return out
			},
			"lowerFirst": func(s string) string {
				if s == "" {
					return s
				}
				return strings.ToLower(s[:1]) + s[1:]
			},
			"nonEmptyLines": func(s string) []string {
				var out []string
				for _, l := range textLines(s) {
					if l = strings.TrimSpace(l); l != "" {
						out = append(out, l)
					}
				}
				return out
			},
		}).ParseFS(templateFS, "templates/*.tmpl"))
	}
	return tmpls
}

// pyString is a double-quoted Python string literal.
func pyString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || r == '\ufeff':
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// pyDoc is the text of a one-line docstring.
func pyDoc(s string) string {
	s = strings.ReplaceAll(oneLine(s), `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// jsString is a single-quoted JavaScript string literal.
func jsString(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch {
		case r == '\\' || r == '\'':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || r == '\ufeff':
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// javaString is a Java string literal (backslashes doubled, so no \u
// escape survives).
func javaString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || r == '\ufeff':
			fmt.Fprintf(&b, `\\u%04x`, r) // never a real escape in Java source
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// csvLine is one line of a JUnit @CsvSource: every value in single quotes
// (a quote doubled), as a Java string literal.
func csvLine(r row) string {
	vals := append([]string{r.Name}, r.Values...)
	for i, v := range vals {
		vals[i] = "'" + strings.ReplaceAll(oneLine(v), "'", "''") + "'"
	}
	return javaString(strings.Join(vals, ", "))
}

// robotCell escapes a Robot Framework data cell (one line).
func robotCell(s string) string {
	s = oneLine(s)
	if s == "" {
		return "${EMPTY}"
	}
	var b strings.Builder
	for i, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '#':
			b.WriteString(`\#`)
		case '$', '@', '&', '%':
			if i+1 < len(s) && s[i+1] == '{' {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		case '=':
			if i == 0 {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		case ' ':
			// two spaces separate cells
			if i > 0 && s[i-1] == ' ' {
				continue
			}
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	if strings.HasPrefix(out, "...") || strings.HasPrefix(out, "[") {
		out = `\` + out
	}
	return out
}

// gherkinText is step or name text on one line.
func gherkinText(s string) string { return oneLine(s) }

// outline turns {{param}} placeholders into <param> ones for a Scenario
// Outline.
func outline(s string, params []param) string {
	s = oneLine(s)
	for _, p := range params {
		s = strings.ReplaceAll(s, "{{"+p.Name+"}}", "<"+gherkinHeader(p.Name)+">")
		s = strings.ReplaceAll(s, "{{ "+p.Name+" }}", "<"+gherkinHeader(p.Name)+">")
	}
	return s
}

func gherkinHeader(s string) string {
	return strings.NewReplacer("|", " ", "<", " ", ">", " ").Replace(oneLine(s))
}

// gherkinCell escapes a table cell.
func gherkinCell(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return oneLine(s)
}

// gherkinTable renders the Examples table (columns aligned).
func gherkinTable(indent string, d *data) string {
	head := []string{"row"}
	for _, p := range d.Params {
		head = append(head, gherkinHeader(p.Name))
	}
	rows := [][]string{head}
	for _, r := range d.DRows {
		cells := []string{gherkinCell(r.Name)}
		for _, v := range r.Values {
			cells = append(cells, gherkinCell(v))
		}
		rows = append(rows, cells)
	}
	width := make([]int, len(head))
	for _, r := range rows {
		for i, c := range r {
			width[i] = max(width[i], utf8.RuneCountInString(c))
		}
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(indent + "|")
		for i, c := range r {
			b.WriteString(" " + c + strings.Repeat(" ", width[i]-utf8.RuneCountInString(c)) + " |")
		}
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// commentLine is a one-line comment text, safe in the language.
func commentLine(lang, s string) string {
	s = oneLine(s)
	if lang == "java" {
		s = javaComment(s)
	}
	return s
}

// todo renders the steps as numbered TODO comments, each line prefixed by
// prefix (indentation and the comment marker).
func todo(lang, prefix string, t Test) string {
	var lines []string
	add := func(first, cont, text string) {
		for i, l := range textLines(text) {
			if i == 0 {
				lines = append(lines, first+l)
			} else {
				lines = append(lines, cont+l)
			}
		}
	}
	if len(t.Steps) == 0 {
		lines = append(lines, "TODO: implement "+oneLine(t.Key+" "+t.Title))
		if strings.TrimSpace(t.Expected) != "" {
			add("Expected: ", "          ", t.Expected)
		}
	}
	for i, s := range t.Steps {
		n := strconv.Itoa(i+1) + ". "
		pad := strings.Repeat(" ", len(n))
		action := s.Action
		if s.Call != "" {
			action = "Call " + s.Call
		}
		if strings.TrimSpace(action) == "" {
			action = "(no action)"
		}
		add(n+"TODO: ", pad+"      ", action)
		if strings.TrimSpace(s.Data) != "" {
			add(pad+"Data: ", pad+"      ", s.Data)
		}
		if strings.TrimSpace(s.Expected) != "" {
			add(pad+"Expected: ", pad+"          ", s.Expected)
		}
	}
	for i, l := range lines {
		l = strings.TrimRight(prefix+l, " ")
		if lang == "java" {
			l = javaComment(l)
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}
