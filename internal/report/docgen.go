package report

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqlang"
)

// Document generators: complete documents built from a project's
// requirements and tests (see export.Document), as opposed to the tabular
// reports.
const (
	// DocRequirementsSpec is a requirements specification modelled on
	// ISO/IEC/IEEE 29148 (software requirements specification).
	DocRequirementsSpec = "requirements_spec"
	// DocTestPlan is a test plan with the test specification, modelled
	// on ISO/IEC/IEEE 29119-3.
	DocTestPlan = "test_plan"
)

// DocKinds lists the document generators.
var DocKinds = []string{DocRequirementsSpec, DocTestPlan}

// Groupings of the requirements in a specification.
const (
	GroupByFolder   = "folder"
	GroupByCategory = "category"
	GroupByType     = "type"
)

// DocGroupings lists the groupings of a requirements specification (test
// plans are grouped by folder).
var DocGroupings = []string{GroupByFolder, GroupByCategory, GroupByType}

// Document fields: what is shown of each requirement and test.
var (
	RequirementDocFields = []string{"description", "rationale", "criteria", "type", "priority", "obligation", "status", "source",
		"owner", "tags", "categories", "relations", "tests", "results", "version"}
	TestDocFields = []string{"description", "preconditions", "steps", "expected", "priority", "execution_type", "status",
		"estimate", "tags", "categories", "dependencies", "requirements", "data", "results", "version"}
)

// DefaultDocFields are the fields shown when none are chosen: all but the
// latest results (a specification describes, it does not report).
func DefaultDocFields(kind string) []string {
	fs := RequirementDocFields
	if kind == DocTestPlan {
		fs = TestDocFields
	}
	var out []string
	for _, f := range fs {
		if f != "results" {
			out = append(out, f)
		}
	}
	return out
}

// DocOptions are the choices of a generated document.
type DocOptions struct {
	// Title of the document (default: the generator's name).
	Title string `json:"title"`
	// Version shown on the title page (default: one more than the
	// latest generated version of this kind, "1.0" for the first).
	Version string `json:"version"`
	// Author (default: the user generating the document).
	Author string `json:"author"`
	// Query filters the requirements (specification) or tests (plan)
	// with the query language of the lists.
	Query string `json:"query"`
	// IncludeInactive includes rejected and obsolete requirements and
	// deprecated tests (left out by default).
	IncludeInactive bool `json:"include_inactive"`
	// BaselineID generates the document from a baseline's frozen items
	// instead of the current ones.
	BaselineID string `json:"baseline_id"`
	// GroupBy groups the requirements of a specification: folder
	// (default), category or type.
	GroupBy string `json:"group_by"`
	// Dimension is the category dimension of GroupBy category (default:
	// the first dimension of the requirement categories).
	Dimension string `json:"dimension"`
	// Fields shown of each item (DefaultDocFields when empty).
	Fields []string `json:"fields"`
	// Traceability adds the traceability (specification) or coverage
	// (plan) appendix; default true.
	Traceability *bool `json:"traceability,omitempty"`
	// Glossary adds the glossary and references appendix; default true.
	Glossary *bool `json:"glossary,omitempty"`
	// Lang of the document's own texts (en, sv; default: the user's).
	Lang string `json:"lang"`
	// Variant generates the document for one variant (id or name): only
	// the requirements and tests that apply to it, with its coverage and
	// results.
	Variant string `json:"variant,omitempty"`
}

// Normalize validates o for a document of kind and fills in defaults.
func (o *DocOptions) Normalize(kind string) error {
	if !slices.Contains(DocKinds, kind) {
		return invalidf("unknown document kind %q (want %s)", kind, strings.Join(DocKinds, " or "))
	}
	o.Title, o.Version, o.Author = strings.TrimSpace(o.Title), strings.TrimSpace(o.Version), strings.TrimSpace(o.Author)
	o.Query, o.BaselineID, o.Dimension = strings.TrimSpace(o.Query), strings.TrimSpace(o.BaselineID), strings.TrimSpace(o.Dimension)
	o.Variant = strings.TrimSpace(o.Variant)
	if len([]rune(o.Title)) > 200 || len([]rune(o.Version)) > 40 || len([]rune(o.Author)) > 200 {
		return invalidf("title (200), version (40) or author (200) is too long")
	}
	o.GroupBy = strings.TrimSpace(o.GroupBy)
	if o.GroupBy == "" || kind == DocTestPlan {
		o.GroupBy = GroupByFolder
	}
	if !slices.Contains(DocGroupings, o.GroupBy) {
		return invalidf("group_by must be one of %s", strings.Join(DocGroupings, ", "))
	}
	known := RequirementDocFields
	if kind == DocTestPlan {
		known = TestDocFields
	}
	if len(o.Fields) == 0 {
		o.Fields = DefaultDocFields(kind)
	}
	var fs []string
	for _, f := range o.Fields {
		f = strings.TrimSpace(f)
		if !slices.Contains(known, f) {
			return invalidf("unknown field %q (want %s)", f, strings.Join(known, ", "))
		}
		if !slices.Contains(fs, f) {
			fs = append(fs, f)
		}
	}
	o.Fields = fs
	t := true
	if o.Traceability == nil {
		o.Traceability = &t
	}
	if o.Glossary == nil {
		o.Glossary = &t
	}
	return nil
}

func (o *DocOptions) has(field string) bool { return slices.Contains(o.Fields, field) }

// Revision is an earlier generated version of a document (from the
// archive), listed in the revision history.
type Revision struct {
	Version string
	Date    time.Time
	Author  string
	Name    string
}

// DocData is the project content of a generated document. Requirements and
// tests are already filtered (status, query) and decorated (FolderPath);
// for a test plan, Requirements are the requirements in scope (not
// filtered by the query, which applies to the tests).
type DocData struct {
	Project      model.Project
	Requirements []model.Requirement
	Tests        []model.TestCase
	// AllTests maps the id of every test of the project (or baseline) to
	// its key and title (called tests, linked tests outside the filter).
	AllTests map[string]model.TestRef
	// Links are the trace links between the items (with keys).
	Links      []model.TraceLink
	Folders    []model.Folder
	Categories map[string]model.Category
	// LastResults: test id → latest result.
	LastResults map[string]string
	// Prerequisites: test id → keys of the tests it depends on.
	Prerequisites map[string][]string
	// test plan: planning context
	Releases       []model.Release
	Runs           []model.TestRun
	Configurations []model.Configuration
	Members        []model.ProjectMember
	Settings       model.TenantSettings
	// Baseline the items come from (nil: current data) and its signatures.
	Baseline *model.Baseline
	Signoffs []model.Signoff
	// Variant the document is for (nil: all variants).
	Variant   *model.VariantRef
	Revisions []Revision
	Now       time.Time
	// Verification: requirement id → its verification (current data only;
	// nil leaves the verification out of the document).
	Verification map[string]*model.RequirementVerification
	// Waivers of the project (requirements specification: marked in the
	// requirements and listed in an appendix).
	Waivers []model.Waiver
}

// DocTitle is the default title of a document of kind in lang.
func DocTitle(kind, lang string) string {
	l := newLoc(lang)
	if kind == DocTestPlan {
		return l.t("Test plan")
	}
	return l.t("Requirements specification")
}

// DocTag is the archive tag of generated documents of kind.
func DocTag(kind string) string {
	if kind == DocTestPlan {
		return "test-plan"
	}
	return "requirements-specification"
}

// NextVersion is the version after the latest of revs ("1.0" first): one
// more than the highest major number.
func NextVersion(revs []Revision) string {
	hi := 0
	for _, r := range revs {
		major, _, _ := strings.Cut(strings.TrimPrefix(strings.ToLower(r.Version), "v"), ".")
		if n, err := strconv.Atoi(major); err == nil && n > hi {
			hi = n
		}
	}
	return strconv.Itoa(hi+1) + ".0"
}

// BuildDocument builds a document of kind. o must be normalized.
func BuildDocument(kind string, d *DocData, o DocOptions) (*export.Document, error) {
	if err := o.Normalize(kind); err != nil {
		return nil, err
	}
	g := newDocGen(kind, d, o)
	if kind == DocTestPlan {
		g.testPlan()
	} else {
		g.requirementsSpec()
	}
	return g.doc, nil
}

// ---------------------------------------------------------------- builder

type docGen struct {
	l    loc
	kind string
	d    *DocData
	o    DocOptions
	doc  *export.Document
	num  [6]int
	app  bool // numbering appendices (A, B, …)
	// trace links: requirement id → test ids, test id → requirement ids
	r2t, t2r map[string][]string
	// keys of the link ends (items outside the document)
	reqKeys, testKeys map[string]string
	reqByID           map[string]*model.Requirement
	testByID          map[string]*model.TestCase
}

func newDocGen(kind string, d *DocData, o DocOptions) *docGen {
	g := &docGen{l: newLoc(o.Lang), kind: kind, d: d, o: o, r2t: map[string][]string{}, t2r: map[string][]string{},
		reqByID: map[string]*model.Requirement{}, testByID: map[string]*model.TestCase{}, reqKeys: map[string]string{}, testKeys: map[string]string{}}
	for i := range d.Requirements {
		g.reqByID[d.Requirements[i].ID] = &d.Requirements[i]
	}
	for i := range d.Tests {
		g.testByID[d.Tests[i].ID] = &d.Tests[i]
	}
	seen := map[string]bool{}
	for _, l := range d.Links {
		k := l.RequirementID + "|" + l.TestCaseID
		if seen[k] {
			continue
		}
		seen[k] = true
		g.reqKeys[l.RequirementID], g.testKeys[l.TestCaseID] = l.RequirementKey, l.TestCaseKey
		g.r2t[l.RequirementID] = append(g.r2t[l.RequirementID], l.TestCaseID)
		g.t2r[l.TestCaseID] = append(g.t2r[l.TestCaseID], l.RequirementID)
	}
	title := o.Title
	if title == "" {
		title = DocTitle(kind, g.l.lang)
	}
	version := o.Version
	if version == "" {
		version = NextVersion(d.Revisions)
	}
	g.o.Title, g.o.Version = title, version
	g.doc = &export.Document{
		Title:       title,
		Subtitle:    d.Project.Key + " – " + d.Project.Name,
		Header:      title + " · " + d.Project.Key + " · " + g.l.t("Version") + " " + version,
		Author:      o.Author,
		Lang:        g.l.lang,
		GeneratedAt: g.now(),
		TOCLevels:   3,
	}
	g.doc.TitleFields = []export.Item{
		{Label: g.l.t("Project"), Value: d.Project.Key + " – " + d.Project.Name},
		{Label: g.l.t("Document ID"), Value: g.docID()},
		{Label: g.l.t("Version"), Value: version},
		{Label: g.l.t("Date"), Value: g.now().Format("2006-01-02")},
		{Label: g.l.t("Author"), Value: o.Author},
	}
	if b := d.Baseline; b != nil {
		g.doc.TitleFields = append(g.doc.TitleFields, export.Item{Label: g.l.t("Baseline"), Value: b.Name + " (" + b.CreatedAt.UTC().Format("2006-01-02") + ")"})
	}
	if v := d.Variant; v != nil {
		g.doc.TitleFields = append(g.doc.TitleFields, export.Item{Label: g.l.t("Variant"), Value: v.Name})
		g.doc.Subtitle += " · " + v.Name
	}
	return g
}

func (g *docGen) now() time.Time {
	if g.d.Now.IsZero() {
		return time.Now().UTC()
	}
	return g.d.Now.UTC()
}

func (g *docGen) docID() string {
	if g.kind == DocTestPlan {
		return g.d.Project.Key + "-TP"
	}
	return g.d.Project.Key + "-SRS"
}

// f translates en and replaces its {name} placeholders (pairs of name,
// value).
func (g *docGen) f(en string, kv ...string) string {
	s := g.l.t(en)
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}

func (g *docGen) add(bs ...export.Block) { g.doc.Blocks = append(g.doc.Blocks, bs...) }

// h adds a numbered heading ("2.1 Title"; appendices "Appendix A. Title",
// "A.1 Title").
func (g *docGen) h(level int, text string) {
	level = min(max(level, 1), 6)
	g.num[level-1]++
	for i := level; i < len(g.num); i++ {
		g.num[i] = 0
	}
	parts := make([]string, level)
	for i := 0; i < level; i++ {
		parts[i] = strconv.Itoa(g.num[i])
	}
	if g.app {
		parts[0] = string(rune('A' + g.num[0] - 1))
		if level == 1 {
			g.add(export.Heading(1, g.f("Appendix {n}. {title}", "n", parts[0], "title", text)))
			return
		}
	}
	g.add(export.Heading(level, strings.Join(parts, ".")+" "+text))
}

// plainHeading adds an unnumbered heading.
func (g *docGen) plainHeading(level int, text string) { g.add(export.Heading(level, text)) }

// itemHeading adds the heading of one requirement or test (not numbered,
// not in the table of contents).
func (g *docGen) itemHeading(level int, key, title string) {
	b := export.Heading(min(max(level, 1), 6), strings.TrimSpace(key+" "+title))
	b.NoTOC = true
	g.add(b)
}

func (g *docGen) appendices() {
	g.app = true
	g.num = [6]int{}
}

func (g *docGen) label(text string) export.Block { return caption(g.l.t(text)) }

// caption is a bold paragraph (above a table or a block of text).
func caption(text string) export.Block {
	return export.RichParagraph(export.Span{Text: text, Bold: true})
}

func (g *docGen) enum(kind, v string) string { return g.l.enum(kind, v) }

func (g *docGen) yes(b bool) string {
	if b {
		return g.l.t("Yes")
	}
	return g.l.t("No")
}

func joinSorted(in []string) string { return strings.Join(uniqSorted(in), ", ") }

// keywordSpans emphasises the obligation keywords of a requirement text
// (as the requirement analysis detects them; lowercase RFC keywords that
// are not normative are left as they are).
func keywordSpans(text string) []export.Span {
	a := reqlang.Analyze(text)
	lower := map[int]bool{}
	for _, is := range a.Issues {
		if is.Rule == reqlang.RuleLowercaseKeyword {
			lower[is.Offset] = true
		}
	}
	kws := append([]reqlang.Keyword{}, a.Keywords...)
	sort.Slice(kws, func(i, j int) bool { return kws[i].Offset < kws[j].Offset })
	var out []export.Span
	pos := 0
	for _, k := range kws {
		if k.Offset < pos || lower[k.Offset] || k.Offset+k.Length > len(text) {
			continue
		}
		if k.Offset > pos {
			out = append(out, export.Span{Text: text[pos:k.Offset]})
		}
		out = append(out, export.Span{Text: text[k.Offset : k.Offset+k.Length], Bold: true})
		pos = k.Offset + k.Length
	}
	if pos < len(text) {
		out = append(out, export.Span{Text: text[pos:]})
	}
	return out
}

// ---------------------------------------------------------------- shared sections

// variantLabel names the variant of the document ("All variants" without one).
func (g *docGen) variantLabel() string {
	if g.d.Variant != nil {
		return g.d.Variant.Name
	}
	return g.l.t("All variants")
}

func (g *docGen) documentControl() {
	l := g.l
	d := g.d
	g.plainHeading(1, l.t("Document control"))
	basis := l.t("Current data of the project")
	if d.Baseline != nil {
		basis = g.f("Baseline {name} of {date}", "name", d.Baseline.Name, "date", d.Baseline.CreatedAt.UTC().Format("2006-01-02"))
	}
	filter := g.o.Query
	if filter == "" {
		filter = l.t("None")
	}
	inactive := l.t("Rejected and obsolete requirements are left out.")
	if g.kind == DocTestPlan {
		inactive = l.t("Rejected and obsolete requirements and deprecated tests are left out.")
	}
	if g.o.IncludeInactive {
		inactive = l.t("Included")
	}
	fields := []export.Item{
		{Label: l.t("Document title"), Value: g.o.Title},
		{Label: l.t("Document ID"), Value: g.docID()},
		{Label: l.t("Version"), Value: g.o.Version},
		{Label: l.t("Date"), Value: g.now().Format("2006-01-02")},
		{Label: l.t("Author"), Value: g.o.Author},
		{Label: l.t("Project"), Value: d.Project.Key + " – " + d.Project.Name},
		{Label: l.t("Based on"), Value: basis},
		{Label: l.t("Filter"), Value: filter},
		{Label: l.t("Variant"), Value: g.variantLabel()},
		{Label: l.t("Inactive items"), Value: inactive},
	}
	if g.kind == DocRequirementsSpec {
		fields = append(fields, export.Item{Label: l.t("Grouped by"), Value: g.groupingLabel()})
	}
	fields = append(fields, export.Item{Label: l.t("Generated by"), Value: "Qtestframe"})
	g.add(export.Fields(fields...))

	g.plainHeading(2, l.t("Revision history"))
	rows := [][]string{}
	for _, r := range d.Revisions {
		rows = append(rows, []string{r.Version, r.Date.UTC().Format("2006-01-02"), r.Author, r.Name})
	}
	rows = append(rows, []string{g.o.Version, g.now().Format("2006-01-02"), g.o.Author, l.t("This version")})
	g.add(export.Table([]string{l.t("Version"), l.t("Date"), l.t("Author"), l.t("Description")}, rows, 1, 1.4, 2.4, 4))

	if d.Baseline != nil {
		g.plainHeading(2, l.t("Approvals"))
		if len(d.Signoffs) == 0 {
			g.add(export.Note(l.t("The baseline has not been signed.")))
		} else {
			var rows [][]string
			for _, s := range d.Signoffs {
				name := s.UserName
				if name == "" {
					name = s.UserEmail
				}
				rows = append(rows, []string{name, s.Meaning, s.SignedAt.UTC().Format("2006-01-02 15:04"), g.yes(s.Valid)})
			}
			g.add(export.Table([]string{l.t("Name"), l.t("Meaning"), l.t("Signed"), l.t("Valid")}, rows, 2.4, 2.4, 1.6, 1))
		}
	}
	g.add(export.TOC())
}

func (g *docGen) groupingLabel() string {
	switch g.o.GroupBy {
	case GroupByCategory:
		if dim := g.dimension(); dim != "" {
			return g.l.t("Category") + " (" + dim + ")"
		}
		return g.l.t("Category")
	case GroupByType:
		return g.l.t("Type")
	}
	return g.l.t("Folder")
}

// scopeText is the project description, or a placeholder.
func (g *docGen) scope() {
	if desc := strings.TrimSpace(g.d.Project.Description); desc != "" {
		g.add(export.Paragraph(desc))
	} else {
		g.add(export.Note(g.l.t("The project has no description.")))
	}
}

func (g *docGen) keyWords() {
	l := g.l
	g.add(export.Paragraph(l.t(bcp14)))
	rows := [][]string{}
	for _, o := range []string{model.ObligationMust, model.ObligationMustNot, model.ObligationShould, model.ObligationShouldNot, model.ObligationMay} {
		rows = append(rows, []string{g.enum(EnumObligation, o), l.t(obligationMeaning[o])})
	}
	g.add(export.Table([]string{l.t("Key word"), l.t("Meaning")}, rows, 1.2, 4))
}

const bcp14 = `The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in BCP 14 [RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown here.`

var obligationMeaning = map[string]string{
	model.ObligationMust:      "An absolute requirement of the specification.",
	model.ObligationMustNot:   "An absolute prohibition of the specification.",
	model.ObligationShould:    "Recommended: there may be valid reasons to ignore it, but the full implications must be understood and weighed first.",
	model.ObligationShouldNot: "Not recommended: there may be valid reasons to do it, but the full implications must be understood and weighed first.",
	model.ObligationMay:       "Truly optional.",
}

func (g *docGen) glossary(plan bool) {
	l := g.l
	g.h(1, l.t("Glossary and references"))
	g.h(2, l.t("Glossary"))
	terms := [][]string{
		{l.t("Requirement"), l.t("A statement of a need or a condition the product must satisfy.")},
		{l.t("Acceptance criterion"), l.t("A condition that shows that a requirement is fulfilled, with the method used to evaluate it.")},
		{l.t("Test case"), l.t("A set of preconditions, steps and expected results that verifies one or more requirements.")},
		{l.t("Traceability"), l.t("The links between requirements and the tests that verify them.")},
		{l.t("Coverage"), l.t("The share of requirements that are verified by at least one test, and the results of those tests.")},
		{l.t("Baseline"), l.t("A frozen, named version of the requirements and tests.")},
	}
	if plan {
		terms = append(terms,
			[]string{l.t("Test run"), l.t("One execution of a set of tests, in one or more configurations.")},
			[]string{l.t("Configuration"), l.t("A combination of environment values (for example browser, operating system or hardware) that tests are executed in.")},
			[]string{l.t("Entry criteria"), l.t("The conditions under which a test may be executed.")},
			[]string{l.t("Exit criteria"), l.t("The conditions under which testing of a release is complete.")})
	}
	g.add(export.Table([]string{l.t("Term"), l.t("Definition")}, terms, 1.2, 4))

	g.h(2, l.t("References"))
	refs := [][]string{
		{"RFC 2119", "Key words for use in RFCs to Indicate Requirement Levels (BCP 14)"},
		{"RFC 8174", "Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words (BCP 14)"},
		{"ISO/IEC/IEEE 29148:2018", "Systems and software engineering – Life cycle processes – Requirements engineering"},
	}
	if plan {
		refs = append(refs, []string{"ISO/IEC/IEEE 29119-3:2021", "Software and systems engineering – Software testing – Part 3: Test documentation"})
	}
	sources := map[string]int{}
	for _, rq := range g.d.Requirements {
		if s := strings.TrimSpace(rq.Source); s != "" {
			sources[s]++
		}
	}
	var names []string
	for s := range sources {
		names = append(names, s)
	}
	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
	for _, s := range names {
		refs = append(refs, []string{s, g.l.n("Source of {n} requirement", "Source of {n} requirements", sources[s])})
	}
	g.add(export.Table([]string{l.t("Reference"), l.t("Title")}, refs, 1.6, 4))
}

// n is a counted text in the document language.
func (l loc) n(one, many string, n int) string {
	s := one
	if n != 1 {
		s = many
	}
	return strings.ReplaceAll(l.t(s), "{n}", strconv.Itoa(n))
}

// ---------------------------------------------------------------- grouping

// folderNode is a folder with its subfolders and the items in it.
type folderNode struct {
	f        model.Folder
	children []*folderNode
	items    []int
	total    int // items in the subtree
}

// folderTree arranges items (indexes, by folder id) into the folders of
// kind; it returns the root nodes and the items without (a known) folder.
func folderTree(folders []model.Folder, kind string, folderOf func(i int) *string, n int) (roots []*folderNode, loose []int) {
	nodes := map[string]*folderNode{}
	for _, f := range folders {
		if f.Kind == kind {
			nodes[f.ID] = &folderNode{f: f}
		}
	}
	for _, nd := range nodes {
		if nd.f.ParentID != nil {
			if p, ok := nodes[*nd.f.ParentID]; ok {
				p.children = append(p.children, nd)
				continue
			}
		}
		roots = append(roots, nd)
	}
	for i := 0; i < n; i++ {
		if id := folderOf(i); id != nil {
			if nd, ok := nodes[*id]; ok {
				nd.items = append(nd.items, i)
				continue
			}
		}
		loose = append(loose, i)
	}
	var sortNodes func(ns []*folderNode) int
	sortNodes = func(ns []*folderNode) int {
		sort.Slice(ns, func(i, j int) bool {
			if ns[i].f.Position != ns[j].f.Position {
				return ns[i].f.Position < ns[j].f.Position
			}
			return naturalLess(ns[i].f.Name, ns[j].f.Name)
		})
		sum := 0
		for _, nd := range ns {
			nd.total = len(nd.items) + sortNodes(nd.children)
			sum += nd.total
		}
		return sum
	}
	sortNodes(roots)
	return roots, loose
}

// grouped writes the items of a chapter (level 1) grouped by folder:
// items without a folder first, then every folder with items in it (with
// its description), recursively.
func (g *docGen) byFolder(kind string, n int, folderOf func(int) *string, order func([]int), emit func(i, level int)) {
	roots, loose := folderTree(g.d.Folders, kind, folderOf, n)
	order(loose)
	hasFolders := false
	for _, r := range roots {
		if r.total > 0 {
			hasFolders = true
		}
	}
	if len(loose) > 0 {
		lvl := 2
		if hasFolders {
			g.h(2, g.l.t("Without folder"))
			lvl = 3
		}
		for _, i := range loose {
			emit(i, lvl)
		}
	}
	var visit func(nd *folderNode, level int)
	visit = func(nd *folderNode, level int) {
		if nd.total == 0 {
			return
		}
		g.h(min(level, 5), nd.f.Name)
		if desc := strings.TrimSpace(nd.f.Description); desc != "" {
			g.add(export.Paragraph(desc))
		}
		order(nd.items)
		for _, i := range nd.items {
			emit(i, min(level+1, 6))
		}
		for _, c := range nd.children {
			visit(c, level+1)
		}
	}
	for _, r := range roots {
		visit(r, 2)
	}
}

// dimension is the category dimension requirements are grouped by.
func (g *docGen) dimension() string {
	if g.o.Dimension != "" {
		return g.o.Dimension
	}
	var dims []string
	for _, c := range g.d.Categories {
		if c.Scope == "" || c.Scope == model.KindRequirement {
			dims = append(dims, c.Dimension)
		}
	}
	if len(dims) == 0 {
		return ""
	}
	sort.Slice(dims, func(i, j int) bool { return naturalLess(dims[i], dims[j]) })
	return dims[0]
}

// ---------------------------------------------------------------- requirements specification

func (g *docGen) reqOrder(idx []int) {
	rs := g.d.Requirements
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := rs[idx[a]], rs[idx[b]]
		if x.Position != y.Position {
			return x.Position < y.Position
		}
		return naturalLess(x.Key, y.Key)
	})
}

func (g *docGen) requirementsSpec() {
	l := g.l
	d := g.d
	g.documentControl()

	g.h(1, l.t("Introduction"))
	g.h(2, l.t("Purpose"))
	g.add(export.Paragraph(g.f("This document specifies the requirements of {project}. Its structure follows ISO/IEC/IEEE 29148 (software requirements specification).", "project", d.Project.Name)))
	g.h(2, l.t("Scope"))
	g.scope()
	g.h(2, l.t("Requirement key words"))
	g.keyWords()
	g.h(2, l.t("Overview"))
	g.add(export.Paragraph(l.n("The specification contains {n} requirement.", "The specification contains {n} requirements.", len(d.Requirements))))
	counts := map[string]int{}
	for _, rq := range d.Requirements {
		o := rq.Obligation
		if o == "" {
			o = obligationUnset
		}
		counts[o]++
	}
	if len(d.Requirements) > 0 {
		s := l.countSection("", "Requirements by obligation", "Obligation", counts, append(append([]string{}, model.Obligations...), obligationUnset), l.enumLabels(EnumObligation, keysOf(counts)))
		g.add(caption(s.Title), export.Table(s.Columns, dropZero(s.Rows), 3, 1, 1))
	}

	g.add(export.PageBreak())
	g.h(1, l.t("Requirements"))
	if len(d.Requirements) == 0 {
		g.add(export.Note(l.t("No requirements match the selection.")))
	}
	n := len(d.Requirements)
	emit := func(i, level int) { g.requirement(&d.Requirements[i], level) }
	switch g.o.GroupBy {
	case GroupByType:
		byType := map[string][]int{}
		for i, rq := range d.Requirements {
			byType[rq.Type] = append(byType[rq.Type], i)
		}
		for _, typ := range orderedKeys(byType, model.RequirementTypes) {
			name := g.enum(EnumReqType, typ)
			if typ == "" {
				name = l.t("Without type")
			}
			g.h(2, name)
			g.reqOrder(byType[typ])
			for _, i := range byType[typ] {
				emit(i, 3)
			}
		}
	case GroupByCategory:
		dim := g.dimension()
		byCat := map[string][]int{}
		for i, rq := range d.Requirements {
			name := ""
			var vals []string
			for _, id := range rq.CategoryIDs {
				if c, ok := d.Categories[id]; ok && c.Dimension == dim {
					vals = append(vals, c.Name)
				}
			}
			if len(vals) > 0 {
				sort.Slice(vals, func(a, b int) bool { return naturalLess(vals[a], vals[b]) })
				name = vals[0]
			}
			byCat[name] = append(byCat[name], i)
		}
		var names []string
		for k := range byCat {
			if k != "" {
				names = append(names, k)
			}
		}
		sort.Slice(names, func(a, b int) bool { return naturalLess(names[a], names[b]) })
		if _, ok := byCat[""]; ok {
			names = append(names, "")
		}
		for _, name := range names {
			title := name
			if name == "" {
				title = l.t("Without category")
			} else if dim != "" {
				title = dim + ": " + name
			}
			g.h(2, title)
			g.reqOrder(byCat[name])
			for _, i := range byCat[name] {
				emit(i, 3)
			}
		}
	default:
		g.byFolder(model.KindRequirement, n, func(i int) *string { return d.Requirements[i].FolderID }, g.reqOrder, emit)
	}

	waivers := g.specWaivers()
	if *g.o.Traceability || *g.o.Glossary || waivers {
		g.appendices()
	}
	if *g.o.Traceability {
		g.add(export.PageBreak())
		g.traceabilityMatrix()
	}
	if waivers {
		g.waiverAppendix()
	}
	if *g.o.Glossary {
		g.add(export.PageBreak())
		g.glossary(false)
	}
}

func dropZero(rows [][]string) [][]string {
	var out [][]string
	for _, r := range rows {
		if len(r) > 1 && r[1] == "0" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// keysOf returns the keys of counts.
func keysOf(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for k := range counts {
		out = append(out, k)
	}
	return out
}

// orderedKeys returns the keys of m in the given order, then the others
// sorted, "" last.
func orderedKeys[V any](m map[string]V, order []string) []string {
	var out []string
	for _, k := range order {
		if _, ok := m[k]; ok {
			out = append(out, k)
		}
	}
	var extra []string
	for k := range m {
		if k != "" && !slices.Contains(order, k) {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	out = append(out, extra...)
	if _, ok := m[""]; ok {
		out = append(out, "")
	}
	return out
}

// testRefs returns the keys of the tests linked to a requirement, with
// their latest results when results are shown ("KEY-TC-1 (Passed)").
func (g *docGen) testRefs(reqID string, inPlan bool) []string {
	var out []string
	for _, tid := range g.r2t[reqID] {
		if inPlan && g.testByID[tid] == nil {
			continue
		}
		key := g.testKey(tid)
		if key == "" {
			continue
		}
		if g.o.has("results") {
			res := g.d.LastResults[tid]
			if res == "" {
				res = model.ResultNotRun
			}
			key += " (" + g.enum(EnumResult, res) + ")"
		}
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return naturalLess(out[i], out[j]) })
	return out
}

func (g *docGen) testKey(id string) string {
	if t := g.testByID[id]; t != nil {
		return t.Key
	}
	if t, ok := g.d.AllTests[id]; ok {
		return t.Key
	}
	return g.testKeys[id]
}

func (g *docGen) requirement(rq *model.Requirement, level int) {
	l := g.l
	o := g.o
	g.itemHeading(level, rq.Key, rq.Title)
	if o.has("description") && strings.TrimSpace(rq.Description) != "" {
		g.add(export.RichParagraph(keywordSpans(strings.TrimSpace(rq.Description))...))
	}
	var fs []export.Item
	add := func(field, label, value string) {
		if o.has(field) {
			fs = append(fs, export.Item{Label: l.t(label), Value: value})
		}
	}
	add("type", "Type", g.enum(EnumReqType, rq.Type))
	add("priority", "Priority", g.enum(EnumPriority, rq.Priority))
	obl := rq.Obligation
	if obl == "" {
		obl = obligationUnset
	}
	add("obligation", "Obligation", g.enum(EnumObligation, obl))
	add("status", "Status", g.enum(EnumStatus, rq.Status))
	src := rq.Source
	if rq.CatalogRef != "" {
		src = strings.TrimSpace(src + "\n" + l.t("Catalogue") + ": " + rq.CatalogRef)
	}
	add("source", "Source", src)
	add("owner", "Owner", rq.Owner)
	add("tags", "Tags", strings.Join(rq.Tags, ", "))
	if o.has("categories") {
		fs = append(fs, export.Item{Label: l.t("Categories"), Value: g.categoryNames(rq.CategoryIDs)})
	}
	if o.has("relations") {
		var rel []string
		for _, r := range rq.RelationRefs {
			if r.Kind == model.KindRequirement {
				rel = append(rel, r.Key)
			}
		}
		fs = append(fs, export.Item{Label: l.t("Related requirements"), Value: joinSorted(rel)})
	}
	add("rationale", "Rationale", strings.TrimSpace(rq.Rationale))
	if o.has("tests") {
		tests := strings.Join(g.testRefs(rq.ID, false), ", ")
		if tests == "" {
			tests = l.t("None")
		}
		fs = append(fs, export.Item{Label: l.t("Verified by"), Value: tests})
	}
	if v := g.waiverField(rq.ID); v != "" {
		fs = append(fs, export.Item{Label: l.t("Waived by"), Value: v})
	}
	if o.has("version") && rq.Version > 0 {
		fs = append(fs, export.Item{Label: l.t("Version"), Value: strconv.Itoa(rq.Version)})
	}
	v := g.d.Verification[rq.ID]
	if v != nil && (o.has("tests") || o.has("criteria")) {
		val := g.enum(EnumVerification, v.State)
		if v.CriteriaTotal > 0 {
			val += " · " + g.f("{n} of {total} criteria verified", "n", strconv.Itoa(v.CriteriaVerified), "total", strconv.Itoa(v.CriteriaTotal))
		}
		fs = append(fs, export.Item{Label: l.t("Verification"), Value: val})
	}
	if len(fs) > 0 {
		g.add(export.Fields(fs...))
	}
	if o.has("criteria") && len(rq.Criteria) > 0 {
		g.add(g.label("Acceptance criteria"))
		rows := make([][]string, len(rq.Criteria))
		if v != nil {
			states := map[string]model.CriterionVerification{}
			for _, c := range v.Criteria {
				states[c.CriterionID] = c
			}
			for i, c := range rq.Criteria {
				cv := states[c.ID]
				state := g.enum(EnumVerification, cv.State)
				if ev := g.l.evidenceList(cv.Evidence); ev != "" {
					state += "\n" + ev
				}
				rows[i] = []string{strconv.Itoa(i + 1), c.Description, g.enum(EnumEvalMethod, c.Method), c.Threshold, state}
			}
			g.add(export.Table([]string{"#", l.t("Criterion"), l.t("Method"), l.t("Threshold"), l.t("Verification")}, rows, 0.4, 3.6, 1.2, 1.2, 2.4))
		} else {
			for i, c := range rq.Criteria {
				rows[i] = []string{strconv.Itoa(i + 1), c.Description, g.enum(EnumEvalMethod, c.Method), c.Threshold}
			}
			g.add(export.Table([]string{"#", l.t("Criterion"), l.t("Method"), l.t("Threshold")}, rows, 0.4, 5, 1.4, 1.6))
		}
	}
}

func (g *docGen) categoryNames(ids []string) string {
	var out []string
	for _, id := range ids {
		if c, ok := g.d.Categories[id]; ok {
			out = append(out, c.Dimension+": "+c.Name)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func (g *docGen) traceabilityMatrix() {
	l := g.l
	g.h(1, l.t("Traceability matrix"))
	g.add(export.Paragraph(l.t("Every requirement of this specification with the tests that verify it.")))
	cols := []string{l.t("Key"), l.t("Title"), l.t("Obligation"), l.t("Tests")}
	var rows [][]string
	without := 0
	idx := make([]int, len(g.d.Requirements))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return naturalLess(g.d.Requirements[idx[a]].Key, g.d.Requirements[idx[b]].Key) })
	for _, i := range idx {
		rq := &g.d.Requirements[i]
		tests := g.testRefs(rq.ID, false)
		if len(tests) == 0 {
			without++
		}
		obl := rq.Obligation
		if obl == "" {
			obl = obligationUnset
		}
		row := []string{rq.Key, rq.Title, g.enum(EnumObligation, obl), strings.Join(tests, ", ")}
		if g.d.Verification != nil {
			state := ""
			if v := g.d.Verification[rq.ID]; v != nil {
				state = g.enum(EnumVerification, v.State)
				if v.CriteriaTotal > 0 {
					state += " (" + strconv.Itoa(v.CriteriaVerified) + "/" + strconv.Itoa(v.CriteriaTotal) + ")"
				}
			}
			row = append(row, state)
		}
		rows = append(rows, row)
	}
	if g.d.Verification != nil {
		g.add(export.Table(append(cols, l.t("Verification")), rows, 1.2, 2.6, 1.2, 2, 1.6))
	} else {
		g.add(export.Table(cols, rows, 1.3, 3, 1.3, 2.4))
	}
	g.add(export.Paragraph(l.n("{n} requirement has no test.", "{n} requirements have no test.", without)))
}

// ---------------------------------------------------------------- test plan

func (g *docGen) testOrder(idx []int) {
	ts := g.d.Tests
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := ts[idx[a]], ts[idx[b]]
		if x.Position != y.Position {
			return x.Position < y.Position
		}
		return naturalLess(x.Key, y.Key)
	})
}

// covered returns the requirements in scope that tests of the plan verify,
// and those no test of the plan verifies (both sorted by key).
func (g *docGen) covered() (in, out []*model.Requirement) {
	for i := range g.d.Requirements {
		rq := &g.d.Requirements[i]
		if len(g.testRefs(rq.ID, true)) > 0 {
			in = append(in, rq)
		} else {
			out = append(out, rq)
		}
	}
	less := func(s []*model.Requirement) {
		sort.SliceStable(s, func(a, b int) bool { return naturalLess(s[a].Key, s[b].Key) })
	}
	less(in)
	less(out)
	return in, out
}

func (g *docGen) testPlan() {
	l := g.l
	d := g.d
	g.documentControl()
	in, out := g.covered()

	g.h(1, l.t("Introduction"))
	g.h(2, l.t("Purpose"))
	g.add(export.Paragraph(g.f("This test plan describes the scope, approach, environment, roles and schedule of the testing of {project}, and specifies its test cases. Its structure follows ISO/IEC/IEEE 29119-3 (test plan and test specification).", "project", d.Project.Name)))
	g.h(2, l.t("Scope"))
	g.scope()
	g.add(export.Paragraph(l.n("The plan contains {n} test case.", "The plan contains {n} test cases.", len(d.Tests)) + " " +
		l.n("They verify {n} requirement.", "They verify {n} requirements.", len(in))))
	g.h(2, l.t("Requirement key words"))
	g.keyWords()

	g.h(1, l.t("Test items and features to be tested"))
	g.h(2, l.t("Features to be tested"))
	g.add(export.Paragraph(l.t("The features to be tested are the requirements that the test cases of this plan verify.")))
	var rows [][]string
	for _, rq := range in {
		obl := rq.Obligation
		if obl == "" {
			obl = obligationUnset
		}
		rows = append(rows, []string{rq.Key, rq.Title, rq.FolderPath, g.enum(EnumObligation, obl), g.enum(EnumPriority, rq.Priority), strconv.Itoa(len(g.testRefs(rq.ID, true)))})
	}
	g.add(export.Table([]string{l.t("Key"), l.t("Title"), l.t("Folder"), l.t("Obligation"), l.t("Priority"), l.t("Tests")}, rows, 1.3, 3, 1.8, 1.3, 1, 0.7))
	g.h(2, l.t("Features not to be tested"))
	if len(out) == 0 {
		g.add(export.Paragraph(l.t("Every requirement in scope is verified by at least one test case of this plan.")))
	} else {
		g.add(export.Paragraph(l.n("{n} requirement in scope is not verified by a test case of this plan; see the coverage appendix.",
			"{n} requirements in scope are not verified by a test case of this plan; see the coverage appendix.", len(out))))
	}

	g.h(1, l.t("Test approach"))
	g.h(2, l.t("Approach"))
	g.add(export.Paragraph(l.t("The test cases are derived from the requirements; every test case is traced to the requirements it verifies. Manual test cases are executed step by step in test runs, automated ones by the test runner; every execution records its result and evidence.")))
	byExec, byPrio := map[string]int{}, map[string]int{}
	for _, t := range d.Tests {
		byExec[t.ExecutionType]++
		byPrio[t.Priority]++
	}
	if len(d.Tests) > 0 {
		s := l.countSection("", "Test cases by execution type", "Execution type", byExec, []string{model.ExecManual, model.ExecAutomated}, l.enumLabels(EnumExecution, keysOf(byExec)))
		g.add(caption(s.Title), export.Table(s.Columns, dropZero(s.Rows), 3, 1, 1))
		s = l.countSection("", "Test cases by priority", "Priority", byPrio, model.Priorities, l.enumLabels(EnumPriority, keysOf(byPrio)))
		g.add(caption(s.Title), export.Table(s.Columns, dropZero(s.Rows), 3, 1, 1))
	}
	g.h(2, l.t("Entry criteria"))
	g.entryCriteria()
	g.h(2, l.t("Exit criteria"))
	g.exitCriteria()
	g.h(2, l.t("Suspension and resumption"))
	g.add(export.Paragraph(l.t("A test case is suspended when it is blocked, for example by a defect or a missing prerequisite, and is resumed in a later test run when the cause has been removed. Failed test cases are executed again after the defect has been fixed.")))

	g.h(1, l.t("Test environment"))
	g.h(2, l.t("Configurations"))
	if len(d.Configurations) == 0 {
		g.add(export.Paragraph(l.t("No test configurations are defined: the test cases are executed in one environment.")))
	} else {
		var rows [][]string
		for _, c := range d.Configurations {
			var vals []string
			for _, v := range c.Values {
				vals = append(vals, v.Dimension+": "+v.Value)
			}
			rows = append(rows, []string{c.Name, strings.Join(vals, "\n"), c.Description})
		}
		g.add(export.Table([]string{l.t("Name"), l.t("Values"), l.t("Description")}, rows, 1.6, 2.4, 3))
	}
	envs := map[string]map[string]bool{}
	envRuns := map[string]int{}
	for _, r := range d.Runs {
		e := strings.TrimSpace(r.Environment)
		if e == "" {
			continue
		}
		if envs[e] == nil {
			envs[e] = map[string]bool{}
		}
		if b := strings.TrimSpace(r.Build); b != "" {
			envs[e][r.Build] = true
		}
		envRuns[e]++
	}
	if len(envs) > 0 {
		g.h(2, l.t("Environments of the test runs"))
		var rows [][]string
		for _, e := range orderedKeys(envs, nil) {
			var builds []string
			for b := range envs[e] {
				builds = append(builds, b)
			}
			rows = append(rows, []string{e, joinSorted(builds), strconv.Itoa(envRuns[e])})
		}
		g.add(export.Table([]string{l.t("Environment"), l.t("Builds"), l.t("Test runs")}, rows, 2, 3, 1))
	}

	g.h(1, l.t("Roles and responsibilities"))
	g.add(export.Table([]string{l.t("Role"), l.t("Responsibility")}, [][]string{
		{l.t("Manager"), l.t("Owns the requirements, plans releases and decides whether the exit criteria are met.")},
		{l.t("Test lead"), l.t("Plans the testing, creates test runs, sets the execution rules and follows up the results.")},
		{l.t("Tester"), l.t("Designs and maintains test cases, executes them in test runs and records results, evidence and defects.")},
		{l.t("Reviewer"), l.t("Reviews and approves requirements and test cases, and signs baselines.")},
		{l.t("Viewer"), l.t("Follows the progress and reads requirements, test cases and results.")},
		{l.t("Admin"), l.t("Manages the project, its members and settings.")},
	}, 1.2, 4))
	if len(d.Members) > 0 {
		var rows [][]string
		for _, m := range d.Members {
			name := m.Name
			if name == "" {
				name = m.Email
			}
			rows = append(rows, []string{name, m.Email, g.roleLabel(m.Role)})
		}
		g.add(g.label("Project members"))
		g.add(export.Table([]string{l.t("Name"), l.t("E-mail"), l.t("Role")}, rows, 2, 2.4, 1.2))
	}

	g.h(1, l.t("Schedule"))
	g.schedule()

	g.add(export.PageBreak())
	g.h(1, l.t("Test specification"))
	if len(d.Tests) == 0 {
		g.add(export.Note(l.t("No test cases match the selection.")))
	}
	g.byFolder(model.KindTest, len(d.Tests), func(i int) *string { return d.Tests[i].FolderID }, g.testOrder,
		func(i, level int) { g.test(&d.Tests[i], level) })

	if *g.o.Traceability || *g.o.Glossary {
		g.appendices()
	}
	if *g.o.Traceability {
		g.add(export.PageBreak())
		g.coverage(in, out)
	}
	if *g.o.Glossary {
		g.add(export.PageBreak())
		g.glossary(true)
	}
}

func (g *docGen) roleLabel(role string) string {
	switch role {
	case "admin":
		return g.l.t("Admin")
	case "manager":
		return g.l.t("Manager")
	case "test_lead":
		return g.l.t("Test lead")
	case "tester":
		return g.l.t("Tester")
	case "reviewer":
		return g.l.t("Reviewer")
	case "viewer":
		return g.l.t("Viewer")
	}
	return humanize(role)
}

// enforced adds the note for a readiness rule the organisation does not
// enforce.
func (g *docGen) enforced(text string, allowed bool) string {
	if allowed {
		return text + " " + g.l.t("(Not enforced: the organisation allows executing the test anyway, with a warning.)")
	}
	return text
}

func (g *docGen) entryCriteria() {
	l := g.l
	s := g.d.Settings
	ready := s.ReadyRequirementStatuses
	if len(ready) == 0 {
		ready = model.DefaultReadyRequirementStatuses
	}
	var labels []string
	for _, st := range ready {
		labels = append(labels, g.enum(EnumStatus, st))
	}
	g.add(export.Paragraph(l.t("A test case may be executed when:")))
	g.add(export.List(
		g.enforced(g.f("it has the status {status};", "status", g.enum(EnumStatus, model.ReadyTestStatus)), s.AllowUnreadyTests),
		g.enforced(g.f("the requirements it verifies have one of the statuses {statuses};", "statuses", strings.Join(labels, ", ")), s.AllowUnreadyRequirements),
		g.enforced(l.t("the tests it depends on have passed."), s.AllowUnmetDependencies),
	))
	readyN := 0
	for _, t := range g.d.Tests {
		if t.Status == model.ReadyTestStatus {
			readyN++
		}
	}
	g.add(export.Paragraph(g.f("{ready} of {total} test cases have the status {status}.", "ready", strconv.Itoa(readyN), "total", strconv.Itoa(len(g.d.Tests)), "status", g.enum(EnumStatus, model.ReadyTestStatus))))
}

// criterion describes a release exit criterion.
func (g *docGen) criterion(c model.ExitCriterion) string {
	th := c.Threshold
	if th <= 0 {
		th = 100
	}
	pct := strconv.FormatFloat(th, 'f', -1, 64)
	switch c.Type {
	case "coverage":
		return g.f("At least {pct}% of the requirements in scope have a test.", "pct", pct)
	case "passed":
		return g.f("At least {pct}% of the requirements in scope have passed all their tests in the release's test runs.", "pct", pct)
	case "mandatory_passed":
		return g.l.t("Every MUST and SHALL requirement in scope has passed.")
	case "executed":
		return g.f("At least {pct}% of the executions of the release's test runs have a result.", "pct", pct)
	case "no_failed":
		return g.l.t("No test has failed or is blocked in the release's test runs.")
	case "approved":
		return g.f("At least {pct}% of the requirements in scope are approved, implemented or verified.", "pct", pct)
	case "no_suspect":
		return g.l.t("No trace link in scope is suspect.")
	case "no_open_defects":
		if c.Level != "" {
			return g.f("No open defect of severity {level} or higher on the scope.", "level", g.enum(EnumSeverity, c.Level))
		}
		return g.l.t("No open defect on the scope.")
	case "risks_mitigated":
		lvl := c.Level
		if lvl == "" {
			lvl = "low"
		}
		return g.f("Every risk of level {level} or higher linked to the scope is mitigated or accepted.", "level", g.enum(EnumRiskLevel, lvl))
	case "residual_risk":
		if c.Level != "" {
			return g.f("No risk linked to the scope has a residual level above {level} unless its acceptance is signed.", "level", g.enum(EnumRiskLevel, c.Level))
		}
		return g.l.t("No risk linked to the scope has a residual level above the project's risk acceptance threshold unless its acceptance is signed.")
	case "trace_rules":
		if c.Level == "warning" {
			return g.l.t("The traceability rules pass on the scope, without errors or warnings.")
		}
		return g.l.t("The traceability rules pass on the scope, without errors.")
	}
	return humanize(c.Type)
}

func (g *docGen) exitCriteria() {
	l := g.l
	var rels []model.Release
	for _, r := range g.d.Releases {
		if r.Status != "cancelled" {
			rels = append(rels, r)
		}
	}
	if len(rels) == 0 {
		g.add(export.Paragraph(l.t("No release is planned; the default exit criteria of a release apply:")))
		var items []string
		for _, c := range model.DefaultExitCriteria {
			items = append(items, g.criterion(c))
		}
		g.add(export.List(items...))
		return
	}
	g.add(export.Paragraph(l.t("Testing of a release is complete when its exit criteria are met:")))
	for _, r := range rels {
		name := r.Name
		if r.Version != "" {
			name += " " + r.Version
		}
		g.add(export.RichParagraph(export.Span{Text: name, Bold: true}, export.Span{Text: " (" + g.releaseStatus(r.Status) + ")"}))
		var items []string
		for _, c := range r.ExitCriteria {
			items = append(items, g.criterion(c))
		}
		if len(items) == 0 {
			g.add(export.Note(l.t("The release has no exit criteria.")))
			continue
		}
		g.add(export.List(items...))
	}
}

func (g *docGen) releaseStatus(s string) string {
	switch s {
	case "planned":
		return g.l.t("Planned")
	case "active":
		return g.l.t("Active")
	case "released":
		return g.l.t("Released")
	case "cancelled":
		return g.l.t("Cancelled")
	}
	return humanize(s)
}

// maxScheduleRuns is the number of test runs listed in the schedule.
const maxScheduleRuns = 50

func (g *docGen) schedule() {
	l := g.l
	d := g.d
	if len(d.Releases) == 0 && len(d.Runs) == 0 {
		g.add(export.Paragraph(l.t("No releases or test runs are planned yet.")))
		return
	}
	if len(d.Releases) > 0 {
		rels := append([]model.Release{}, d.Releases...)
		sort.SliceStable(rels, func(i, j int) bool {
			a, b := rels[i].TargetDate, rels[j].TargetDate
			if (a == "") != (b == "") {
				return b == ""
			}
			if a != b {
				return a < b
			}
			return rels[i].CreatedAt.Before(rels[j].CreatedAt)
		})
		var rows [][]string
		for _, r := range rels {
			rows = append(rows, []string{r.Name, r.Version, g.releaseStatus(r.Status), r.TargetDate, strconv.Itoa(r.Requirements), strconv.Itoa(r.Runs)})
		}
		g.h(2, l.t("Releases"))
		g.add(export.Table([]string{l.t("Release"), l.t("Version"), l.t("Status"), l.t("Target date"), l.t("Requirements"), l.t("Test runs")}, rows, 2, 1, 1, 1.2, 1, 1))
	}
	if len(d.Runs) > 0 {
		runs := append([]model.TestRun{}, d.Runs...)
		sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
		total := len(runs)
		if len(runs) > maxScheduleRuns {
			runs = runs[:maxScheduleRuns]
		}
		sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt.Before(runs[j].CreatedAt) })
		var rows [][]string
		for _, r := range runs {
			rows = append(rows, []string{r.Name, g.enum(EnumStatus, r.Status), r.ReleaseName, r.CreatedAt.UTC().Format("2006-01-02"),
				dateOf(r.StartedAt), dateOf(r.CompletedAt), strconv.Itoa(r.Summary["total"])})
		}
		g.h(2, l.t("Test runs"))
		if total > maxScheduleRuns {
			g.add(export.Paragraph(g.f("The {n} most recent of {total} test runs.", "n", strconv.Itoa(maxScheduleRuns), "total", strconv.Itoa(total))))
		}
		g.add(export.Table([]string{l.t("Test run"), l.t("Status"), l.t("Release"), l.t("Created"), l.t("Started"), l.t("Completed"), l.t("Executions")}, rows, 2.2, 1, 1.4, 1, 1, 1, 0.9))
	}
}

func dateOf(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func (g *docGen) test(t *model.TestCase, level int) {
	l := g.l
	o := g.o
	g.itemHeading(level, t.Key, t.Title)
	if o.has("description") && strings.TrimSpace(t.Description) != "" {
		g.add(export.Paragraph(strings.TrimSpace(t.Description)))
	}
	var fs []export.Item
	add := func(field, label, value string) {
		if o.has(field) {
			fs = append(fs, export.Item{Label: l.t(label), Value: value})
		}
	}
	add("priority", "Priority", g.enum(EnumPriority, t.Priority))
	exec := g.enum(EnumExecution, t.ExecutionType)
	if t.ExecutionType == model.ExecAutomated && t.ScriptLanguage != "" {
		exec += " (" + t.ScriptLanguage + ")"
	}
	add("execution_type", "Execution type", exec)
	add("status", "Status", g.enum(EnumStatus, t.Status))
	if t.EstimatedMin > 0 {
		add("estimate", "Estimated time", g.f("{n} min", "n", strconv.Itoa(t.EstimatedMin)))
	}
	add("tags", "Tags", strings.Join(t.Tags, ", "))
	if o.has("categories") {
		fs = append(fs, export.Item{Label: l.t("Categories"), Value: g.categoryNames(t.CategoryIDs)})
	}
	if o.has("requirements") {
		var keys []string
		for _, rid := range g.t2r[t.ID] {
			if rq := g.reqByID[rid]; rq != nil {
				keys = append(keys, rq.Key)
			} else {
				keys = append(keys, g.reqKeys[rid])
			}
		}
		v := joinSorted(keys)
		if v == "" {
			v = l.t("None")
		}
		fs = append(fs, export.Item{Label: l.t("Verifies"), Value: v})
	}
	if o.has("dependencies") {
		fs = append(fs, export.Item{Label: l.t("Prerequisites"), Value: joinSorted(g.d.Prerequisites[t.ID])})
	}
	if o.has("results") {
		res := g.d.LastResults[t.ID]
		if res == "" {
			res = model.ResultNotRun
		}
		fs = append(fs, export.Item{Label: l.t("Latest result"), Value: g.enum(EnumResult, res)})
	}
	if o.has("version") && t.Version > 0 {
		fs = append(fs, export.Item{Label: l.t("Version"), Value: strconv.Itoa(t.Version)})
	}
	if len(fs) > 0 {
		g.add(export.Fields(fs...))
	}
	if o.has("preconditions") && strings.TrimSpace(t.Preconditions) != "" {
		g.add(g.label("Preconditions"), export.Paragraph(strings.TrimSpace(t.Preconditions)))
	}
	if o.has("steps") && len(t.Steps) > 0 {
		g.add(g.label("Steps"))
		rows := make([][]string, len(t.Steps))
		for i, st := range t.Steps {
			action := st.Action
			if st.CallTestID != nil {
				ref := g.d.AllTests[*st.CallTestID]
				key, title := ref.Key, ref.Title
				if key == "" {
					key, title = st.CallTestKey, st.CallTestTitle
				}
				action = strings.TrimSpace(g.f("Call {key} {title}", "key", key, "title", title))
			}
			rows[i] = []string{strconv.Itoa(i + 1), action, st.Data, st.Expected}
		}
		g.add(export.Table([]string{"#", l.t("Action"), l.t("Test data"), l.t("Expected result")}, rows, 0.4, 3.6, 2, 3.6))
	}
	if o.has("expected") && strings.TrimSpace(t.ExpectedResult) != "" {
		g.add(g.label("Expected result"), export.Paragraph(strings.TrimSpace(t.ExpectedResult)))
	}
	if o.has("data") && len(t.Parameters) > 0 {
		g.add(g.label("Test data"))
		cols := append([]string{l.t("Name")}, t.Parameters...)
		var rows [][]string
		for i, r := range t.DataRows {
			name := r.Name
			if name == "" {
				name = strconv.Itoa(i + 1)
			}
			row := []string{name}
			for _, p := range t.Parameters {
				row = append(row, r.Values[p])
			}
			rows = append(rows, row)
		}
		g.add(export.Table(cols, rows))
	}
}

func (g *docGen) coverage(in, out []*model.Requirement) {
	l := g.l
	g.h(1, l.t("Coverage"))
	g.h(2, l.t("Requirements and their tests"))
	var rows [][]string
	all := append(append([]*model.Requirement{}, in...), out...)
	sort.SliceStable(all, func(a, b int) bool { return naturalLess(all[a].Key, all[b].Key) })
	for _, rq := range all {
		obl := rq.Obligation
		if obl == "" {
			obl = obligationUnset
		}
		rows = append(rows, []string{rq.Key, rq.Title, g.enum(EnumObligation, obl), strings.Join(g.testRefs(rq.ID, true), ", ")})
	}
	g.add(export.Table([]string{l.t("Key"), l.t("Title"), l.t("Obligation"), l.t("Tests")}, rows, 1.3, 3, 1.3, 2.4))
	g.h(2, l.t("Requirements without tests"))
	if len(out) == 0 {
		g.add(export.Paragraph(l.t("Every requirement in scope is verified by at least one test case of this plan.")))
		return
	}
	rows = nil
	for _, rq := range out {
		obl := rq.Obligation
		if obl == "" {
			obl = obligationUnset
		}
		rows = append(rows, []string{rq.Key, rq.Title, g.enum(EnumStatus, rq.Status), g.enum(EnumObligation, obl)})
	}
	g.add(export.Table([]string{l.t("Key"), l.t("Title"), l.t("Status"), l.t("Obligation")}, rows, 1.3, 3.6, 1.2, 1.4))
}
