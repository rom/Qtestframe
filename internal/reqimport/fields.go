package reqimport

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqlang"
)

// Candidate fields a column can map to.
const (
	FieldTitle       = "title"
	FieldDescription = "description"
	FieldExternalID  = "external_id"
	FieldObligation  = "obligation"
	FieldType        = "type"
	FieldPriority    = "priority"
	FieldStatus      = "status"
	FieldSource      = "source"
	FieldRationale   = "rationale"
	FieldOwner       = "owner"
	FieldTags        = "tags"
	FieldFolder      = "folder"
	FieldCriteria    = "criteria"
)

// Fields lists the mappable fields.
var Fields = []string{FieldTitle, FieldDescription, FieldExternalID, FieldObligation, FieldType, FieldPriority,
	FieldStatus, FieldSource, FieldRationale, FieldOwner, FieldTags, FieldFolder, FieldCriteria}

const maxTitle = 500

// headerNames maps normalised header names (English and Swedish, export
// column names included) to fields; "" marks known columns that are ignored.
var headerNames = map[string]string{
	"title": FieldTitle, "titel": FieldTitle, "name": FieldTitle, "namn": FieldTitle, "rubrik": FieldTitle,
	"heading": FieldTitle, "summary": FieldTitle, "sammanfattning": FieldTitle, "subject": FieldTitle,
	"short text": FieldTitle, "kortnamn": FieldTitle, "requirement title": FieldTitle, "kravrubrik": FieldTitle,
	"kravtitel": FieldTitle, "object heading": FieldTitle, "reqif.name": FieldTitle,

	"description": FieldDescription, "beskrivning": FieldDescription, "text": FieldDescription,
	"requirement": FieldDescription, "requirements": FieldDescription, "krav": FieldDescription,
	"kravtext": FieldDescription, "requirement text": FieldDescription, "statement": FieldDescription,
	"details": FieldDescription, "detaljer": FieldDescription, "object text": FieldDescription,
	"reqif.text": FieldDescription, "body": FieldDescription, "innehåll": FieldDescription,
	"kravbeskrivning": FieldDescription,

	"id": FieldExternalID, "key": FieldExternalID, "nyckel": FieldExternalID, "ref": FieldExternalID,
	"reference": FieldExternalID, "referens": FieldExternalID, "identifier": FieldExternalID,
	"external id": FieldExternalID, "req id": FieldExternalID, "requirement id": FieldExternalID,
	"krav-id": FieldExternalID, "krav id": FieldExternalID, "kravid": FieldExternalID, "kravnummer": FieldExternalID,
	"nr": FieldExternalID, "no": FieldExternalID, "number": FieldExternalID, "nummer": FieldExternalID,
	"reqif.foreignid": FieldExternalID, "foreign id": FieldExternalID, "absolute number": FieldExternalID,

	"obligation": FieldObligation, "kravnivå": FieldObligation, "obligation level": FieldObligation,
	"level": FieldObligation, "nivå": FieldObligation, "modality": FieldObligation, "verb": FieldObligation,

	"type": FieldType, "typ": FieldType, "requirement type": FieldType, "kravtyp": FieldType, "kind": FieldType,

	"priority": FieldPriority, "prioritet": FieldPriority, "prio": FieldPriority, "importance": FieldPriority,
	"vikt": FieldPriority,

	"status": FieldStatus, "state": FieldStatus, "tillstånd": FieldStatus, "läge": FieldStatus,

	"source": FieldSource, "källa": FieldSource, "origin": FieldSource, "ursprung": FieldSource,
	"kravställare": FieldSource, "stakeholder": FieldSource,

	"rationale": FieldRationale, "motivering": FieldRationale, "justification": FieldRationale,
	"motiv": FieldRationale, "reason": FieldRationale, "skäl": FieldRationale, "comment": FieldRationale,
	"comments": FieldRationale, "kommentar": FieldRationale, "notes": FieldRationale, "anmärkning": FieldRationale,

	"owner": FieldOwner, "ägare": FieldOwner, "kravägare": FieldOwner, "responsible": FieldOwner,
	"ansvarig": FieldOwner, "assignee": FieldOwner,

	"tags": FieldTags, "tag": FieldTags, "taggar": FieldTags, "tagg": FieldTags, "labels": FieldTags,
	"etiketter": FieldTags, "keywords": FieldTags, "nyckelord": FieldTags,

	"folder": FieldFolder, "mapp": FieldFolder, "section": FieldFolder, "avsnitt": FieldFolder,
	"chapter": FieldFolder, "kapitel": FieldFolder, "group": FieldFolder, "grupp": FieldFolder,
	"module": FieldFolder, "modul": FieldFolder, "path": FieldFolder, "sökväg": FieldFolder,
	"folder path": FieldFolder, "area": FieldFolder, "område": FieldFolder,

	"criteria": FieldCriteria, "criterion": FieldCriteria, "acceptance criteria": FieldCriteria,
	"acceptance criterion": FieldCriteria, "acceptanskriterier": FieldCriteria, "acceptanskriterium": FieldCriteria,
	"kriterier": FieldCriteria, "evaluation criteria": FieldCriteria, "verification criteria": FieldCriteria,
	"utvärderingskriterier": FieldCriteria, "verifieringskriterier": FieldCriteria,

	// Columns of the requirement export that have no import meaning.
	"version": "", "linked tests": "", "coverage": "", "categories": "", "kategorier": "",
	"länkade tester": "", "täckning": "",
}

var headerJunk = regexp.MustCompile(`\s*\([^)]*\)\s*|[*:#?]+`)

// normHeader lowercases a header and drops decoration such as "(optional)"
// or a trailing "*".
func normHeader(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if n := headerJunk.ReplaceAllString(h, " "); strings.TrimSpace(n) != "" {
		h = n
	}
	h = strings.Join(strings.FieldsFunc(h, func(r rune) bool { return unicode.IsSpace(r) || r == '_' }), " ")
	return h
}

// GuessField returns the field a header most likely holds and whether it
// was recognised.
func GuessField(header string) (string, bool) {
	f, ok, _ := guessField(header)
	return f, ok
}

// guessField also reports whether the header is a known column name (not
// only a word match).
func guessField(header string) (field string, ok, exact bool) {
	f, ok := fuzzyField(header)
	if !ok {
		return "", false, false
	}
	h := normHeader(header)
	_, ex := headerNames[h]
	if !ex {
		_, ex = headerNames[strings.TrimPrefix(h, "reqif.")]
	}
	return f, true, ex
}

func fuzzyField(header string) (string, bool) {
	h := normHeader(header)
	if h == "" || utf8.RuneCountInString(h) > 40 || len(strings.Fields(h)) > 5 {
		return "", false // headers are short; longer cells are content
	}
	if f, ok := headerNames[h]; ok {
		return f, true
	}
	if f, ok := headerNames[strings.TrimPrefix(h, "reqif.")]; ok {
		return f, true
	}
	words := strings.FieldsFunc(h, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	has := func(ws ...string) bool {
		for _, w := range words {
			for _, x := range ws {
				if w == x {
					return true
				}
			}
		}
		return false
	}
	contains := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(h, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("id", "key", "ref", "nr", "nyckel", "identifier", "foreignid"):
		return FieldExternalID, true
	case contains("criteri", "kriteri"):
		return FieldCriteria, true
	case contains("obligation", "kravnivå"):
		return FieldObligation, true
	case has("title", "titel", "name", "namn", "rubrik", "heading", "summary"):
		return FieldTitle, true
	case contains("priorit"):
		return FieldPriority, true
	case has("status", "state"):
		return FieldStatus, true
	case has("type", "typ") || contains("kravtyp"):
		return FieldType, true
	case contains("rationale", "motivering", "justification"):
		return FieldRationale, true
	case contains("owner", "ägare", "ansvarig"):
		return FieldOwner, true
	case contains("källa", "source"):
		return FieldSource, true
	case has("tags", "taggar", "labels"):
		return FieldTags, true
	case contains("description", "beskrivning", "kravtext") || has("text", "requirement", "krav", "statement"):
		return FieldDescription, true
	case contains("folder", "mapp", "section", "avsnitt", "chapter", "kapitel"):
		return FieldFolder, true
	}
	return "", false
}

// checkMapping validates the fields of an explicit mapping.
func checkMapping(m map[string]string) error {
	for h, f := range m {
		f = strings.TrimSpace(strings.ToLower(f))
		if f == "" || f == "ignore" {
			continue
		}
		if !model.Valid(Fields, f) {
			return invalidf("mapping for column %q: unknown field %q", h, f)
		}
	}
	return nil
}

// mappedField returns the field of header: the explicit mapping wins over
// the guess. ok reports an explicit mapping.
func mappedField(m map[string]string, header string) (field string, explicit bool) {
	for h, f := range m {
		if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(header)) {
			f = strings.TrimSpace(strings.ToLower(f))
			if f == "ignore" {
				f = ""
			}
			return f, true
		}
	}
	f, _ := GuessField(header)
	return f, false
}

// ---------------------------------------------------------------- values

func normValue(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("_", " ", "-", " ", "–", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

var obligationValues = map[string]string{
	"must": model.ObligationMust, "must not": model.ObligationMustNot, "mustnot": model.ObligationMustNot,
	"required": model.ObligationMust, "mandatory": model.ObligationMust,
	"shall": model.ObligationShall, "shall not": model.ObligationShallNot, "shallnot": model.ObligationShallNot,
	"should": model.ObligationShould, "should not": model.ObligationShouldNot, "shouldnot": model.ObligationShouldNot,
	"recommended": model.ObligationShould, "not recommended": model.ObligationShouldNot,
	"may": model.ObligationMay, "optional": model.ObligationOptional, "will": model.ObligationWill,
	"describe": model.ObligationDescribe, "describes": model.ObligationDescribe, "informative": model.ObligationDescribe,
	"info": model.ObligationDescribe,
	"ska":  model.ObligationShall, "skall": model.ObligationShall, "ska inte": model.ObligationShallNot,
	"skall inte": model.ObligationShallNot, "ska ej": model.ObligationShallNot, "skall ej": model.ObligationShallNot,
	"måste": model.ObligationMust, "obligatorisk": model.ObligationMust, "obligatoriskt": model.ObligationMust,
	"bör": model.ObligationShould, "bör inte": model.ObligationShouldNot, "bör ej": model.ObligationShouldNot,
	"rekommenderas": model.ObligationShould, "får": model.ObligationMay, "får inte": model.ObligationMustNot,
	"får ej": model.ObligationMustNot, "valfri": model.ObligationOptional, "valfritt": model.ObligationOptional,
	"kommer att": model.ObligationWill, "beskrivande": model.ObligationDescribe, "informativ": model.ObligationDescribe,
	"informativt": model.ObligationDescribe,
}

var typeValues = map[string]string{
	"functional": "functional", "function": "functional", "fr": "functional", "funktionell": "functional",
	"funktion": "functional", "funktionellt": "functional",
	"non functional": "non_functional", "nonfunctional": "non_functional", "nfr": "non_functional",
	"quality": "non_functional", "icke funktionell": "non_functional", "icke funktionellt": "non_functional",
	"kvalitet": "non_functional", "kvalitetskrav": "non_functional",
	"security": "security", "säkerhet": "security", "informationssäkerhet": "security", "safety": "security",
	"performance": "performance", "prestanda": "performance",
	"usability": "usability", "användbarhet": "usability",
	"interface": "interface", "gränssnitt": "interface", "integration": "interface",
	"constraint": "constraint", "begränsning": "constraint", "restriktion": "constraint", "villkor": "constraint",
	"business": "business", "affär": "business", "verksamhet": "business", "verksamhetskrav": "business",
	"affärskrav": "business",
	"regulatory": "regulatory", "legal": "regulatory", "compliance": "regulatory", "regulatorisk": "regulatory",
	"lagkrav": "regulatory", "juridisk": "regulatory", "författningskrav": "regulatory",
}

var priorityValues = map[string]string{
	"critical": "critical", "blocker": "critical", "urgent": "critical", "highest": "critical", "kritisk": "critical",
	"akut": "critical", "p1": "critical", "1": "critical",
	"high": "high", "major": "high", "hög": "high", "högt": "high", "p2": "high", "2": "high",
	"medium": "medium", "normal": "medium", "moderate": "medium", "medel": "medium", "mellan": "medium",
	"p3": "medium", "3": "medium",
	"low": "low", "minor": "low", "lowest": "low", "trivial": "low", "låg": "low", "lågt": "low", "p4": "low", "4": "low",
}

var statusValues = map[string]string{
	"draft": "draft", "new": "draft", "proposed": "draft", "open": "draft", "utkast": "draft", "ny": "draft",
	"föreslagen": "draft",
	"review":     "review", "in review": "review", "under review": "review", "granskning": "review",
	"under granskning": "review", "för granskning": "review",
	"approved": "approved", "accepted": "approved", "agreed": "approved", "godkänd": "approved",
	"godkänt": "approved", "fastställd": "approved", "beslutad": "approved",
	"implemented": "implemented", "done": "implemented", "implementerad": "implemented",
	"genomförd": "implemented", "klar": "implemented", "införd": "implemented",
	"verified": "verified", "tested": "verified", "verifierad": "verified", "testad": "verified",
	"rejected": "rejected", "declined": "rejected", "avvisad": "rejected", "avslagen": "rejected",
	"obsolete": "obsolete", "deprecated": "obsolete", "deleted": "obsolete", "föråldrad": "obsolete",
	"utgått": "obsolete", "utgången": "obsolete", "struken": "obsolete",
}

var methodValues = map[string]string{
	"test": "test", "testing": "test", "prov": "test", "provning": "test",
	"inspection": "inspection", "inspektion": "inspection", "besiktning": "inspection",
	"analysis": "analysis", "analys": "analysis",
	"demonstration": "demonstration", "demo": "demonstration",
	"review": "review", "granskning": "review",
}

func lookup(table map[string]string, codes []string, raw string) (string, bool) {
	v := normValue(raw)
	if v == "" {
		return "", true
	}
	if code := strings.ReplaceAll(v, " ", "_"); model.Valid(codes, code) {
		return code, true
	}
	if c, ok := table[v]; ok {
		return c, true
	}
	return "", false
}

// ParseObligation maps a column value (MUST, SHALL NOT, SKA, BÖR, the model
// codes …) to an obligation code.
func ParseObligation(raw string) (string, bool) {
	if c, ok := lookup(obligationValues, model.Obligations, raw); ok {
		return c, true
	}
	// "The system SHALL …" in an obligation column: detect the keyword.
	if o := reqlang.DetectObligation(raw); o != "" {
		return o, true
	}
	return "", false
}

// ParseType, ParsePriority, ParseStatus and ParseMethod map column values
// in English or Swedish to model codes.
func ParseType(raw string) (string, bool) { return lookup(typeValues, model.RequirementTypes, raw) }

// ParsePriority maps a priority value.
func ParsePriority(raw string) (string, bool) { return lookup(priorityValues, model.Priorities, raw) }

// ParseStatus maps a status value.
func ParseStatus(raw string) (string, bool) {
	return lookup(statusValues, model.RequirementStatuses, raw)
}

// ParseMethod maps an evaluation method.
func ParseMethod(raw string) (string, bool) {
	return lookup(methodValues, model.EvaluationMethods, raw)
}

// ---------------------------------------------------------------- text helpers

// cleanCell trims a cell and undoes the formula-injection guard of the CSV
// export (a leading "'" before = + - @).
func cleanCell(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
	if len(s) > 1 && s[0] == '\'' && strings.ContainsRune("=+-@", rune(s[1])) {
		s = s[1:]
	}
	return strings.TrimSpace(strings.ToValidUTF8(s, "�"))
}

var bulletRe = regexp.MustCompile(`^\s*(?:[-*•·◦▪–—]|\(?\d{1,3}[.)]|\(?[a-zA-Z][.)])\s+`)

// lines splits s into trimmed non-empty lines without bullets.
func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(bulletRe.ReplaceAllString(l, ""))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '|' }) {
		if t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#")); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// folderParts splits a folder path on "/", "\" or ">".
func folderParts(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '\\' || r == '>' || r == '\n' }) {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			out = append(out, truncRunes(p, 200))
		}
	}
	return out
}

// joinFolder joins path parts in the canonical "A/B" form.
func joinFolder(parts ...string) string {
	var all []string
	for _, p := range parts {
		all = append(all, folderParts(p)...)
	}
	return strings.Join(all, "/")
}

// FolderParts splits a candidate's folder path.
func FolderParts(folder string) []string { return folderParts(folder) }

func truncRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

var criterionMethodRe = regexp.MustCompile(`(?i)^\[?([\p{L}]+)\]?\s*[:\]–-]\s*(.+)$`)

// parseCriteria turns one criterion per line into criteria. A line may
// start with a method: "inspection: …" or "[analys] …".
func parseCriteria(s string) []Criterion {
	var out []Criterion
	for _, l := range lines(s) {
		c := Criterion{Description: l}
		if m := criterionMethodRe.FindStringSubmatch(l); m != nil {
			if meth, ok := ParseMethod(m[1]); ok && meth != "" {
				c.Method, c.Description = meth, strings.TrimSpace(m[2])
			}
		} else if strings.HasPrefix(l, "[") {
			if i := strings.Index(l, "]"); i > 1 {
				if meth, ok := ParseMethod(l[1:i]); ok && meth != "" {
					c.Method, c.Description = meth, strings.TrimSpace(l[i+1:])
				}
			}
		}
		if c.Description != "" {
			out = append(out, c)
		}
	}
	return out
}

// idPrefixRe matches a leading identifier such as "REQ-12:", "[SRS-001]",
// "FR 1.2 –" or "K12)".
var idPrefixRe = regexp.MustCompile(`^\s*(?:\[([\p{L}][\p{L}\d_.\-]*\d[\p{L}\d_.\-]*)\]\s*[:.)–—-]?|([\p{L}][\p{L}\d_.\-]*\d[\p{L}\d_.\-]*)\s*(?:[:)]|\s[–—-]))\s*`)

// splitID removes a leading identifier from text.
func splitID(text string) (id, rest string) {
	m := idPrefixRe.FindStringSubmatchIndex(text)
	if m == nil {
		return "", text
	}
	rest = strings.TrimSpace(text[m[1]:])
	if rest == "" {
		return "", text
	}
	if m[2] >= 0 {
		id = text[m[2]:m[3]]
	} else {
		id = text[m[4]:m[5]]
	}
	return strings.TrimRight(id, ".-_"), rest
}

// deriveTitle builds a title from the first words of a description.
func deriveTitle(desc string) string {
	first := desc
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	words := strings.Fields(first)
	const n = 8
	t := strings.Join(words, " ")
	if len(words) > n {
		t = strings.Join(words[:n], " ") + "…"
	}
	return truncRunes(strings.TrimRight(t, ".,;:"), maxTitle)
}

// raw holds unparsed values of a candidate before normalisation.
type raw struct {
	obligation, typ, priority, status, criteria string
}

// finish normalises a candidate: enumerations, derived title, obligation
// detection. It reports false when the candidate has no content.
func finish(c *Candidate, r raw) bool {
	c.Title = strings.Join(strings.Fields(c.Title), " ")
	c.Description = strings.TrimSpace(c.Description)
	if c.Title == "" && c.Description == "" {
		return false
	}
	if c.Tags == nil {
		c.Tags = []string{}
	}
	if c.Warnings == nil {
		c.Warnings = []string{}
	}
	c.ExternalID = strings.Join(strings.Fields(c.ExternalID), " ")
	c.Folder = joinFolder(c.Folder)
	if c.Title == "" {
		c.Title = deriveTitle(c.Description)
		c.Warnings = append(c.Warnings, "title derived from the description")
	}
	if utf8.RuneCountInString(c.Title) > maxTitle {
		c.Title = truncRunes(c.Title, maxTitle)
		c.Warnings = append(c.Warnings, "title shortened to 500 characters")
	}
	enum := func(dst *string, rawv, what string, parse func(string) (string, bool)) {
		if strings.TrimSpace(rawv) == "" {
			return
		}
		v, ok := parse(rawv)
		if !ok {
			c.Warnings = append(c.Warnings, "unknown "+what+" \""+strings.TrimSpace(rawv)+"\"; the default is used")
			return
		}
		*dst = v
	}
	if strings.TrimSpace(r.obligation) != "" {
		enum(&c.Obligation, r.obligation, "obligation", ParseObligation)
		if c.Obligation != "" {
			c.ObligationFrom = "column"
		}
	}
	if c.Obligation == "" {
		if o := reqlang.DetectObligation(c.Title + "\n" + c.Description); o != "" {
			c.Obligation, c.ObligationFrom = o, "text"
		} else {
			c.Warnings = append(c.Warnings, "no obligation keyword (MUST, SHALL, SKA, BÖR …) found")
		}
	}
	enum(&c.Type, r.typ, "type", ParseType)
	enum(&c.Priority, r.priority, "priority", ParsePriority)
	enum(&c.Status, r.status, "status", ParseStatus)
	if r.criteria != "" {
		c.Criteria = append(c.Criteria, parseCriteria(r.criteria)...)
	}
	if c.Criteria == nil {
		c.Criteria = []Criterion{}
	}
	return true
}
