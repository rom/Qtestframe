package export

import (
	"archive/zip"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// ReqIF 1.2 export of requirements.
//
// Mapping (see docs/user-guide/en/reports-and-export.md):
//   - one SPEC-OBJECT per requirement (type "Requirement") with the
//     attributes ReqIF.ForeignID (the key), ReqIF.Name (title), ReqIF.Text
//     (the description, Markdown converted to XHTML) and the other fields;
//     obligation, type, priority and status are ENUMERATION attributes;
//   - folders are heading SPEC-OBJECTs (type "Heading", attribute
//     ReqIF.ChapterName), the convention of DOORS and most ReqIF tools: the
//     folder tree is the SPEC-HIERARCHY of one SPECIFICATION, a folder's
//     requirements and then its subfolders are the children of its heading;
//   - requirement relations are SPEC-RELATIONs, one SPEC-RELATION-TYPE per
//     relation type;
//   - IDENTIFIERs are derived from the database ids ("qtf-req-<id>" …), so
//     repeated exports of the same project keep them.

// ReqIF tool and format identification.
const (
	ReqIFToolID   = "Qtestframe"
	reqifNS       = "http://www.omg.org/spec/ReqIF/20110401/reqif.xsd"
	xhtmlNS       = "http://www.w3.org/1999/xhtml"
	reqifMaxLen   = 1000000
	reqifDateTime = "2006-01-02T15:04:05.000Z07:00"
)

// ReqIF content types.
const (
	ReqIFContentType  = "application/xml; charset=utf-8"
	ReqIFZContentType = "application/zip"
)

// ReqIFDocument is the input of WriteReqIF / WriteReqIFZ.
type ReqIFDocument struct {
	ProjectID   string
	ProjectKey  string
	Title       string // e.g. "SHOP – Webshop"
	Comment     string
	ToolVersion string
	GeneratedAt time.Time
	// Folders in display order (parents before or after children: the
	// hierarchy is built from ParentID).
	Folders []ReqIFFolder
	// Requirements in display order.
	Requirements []ReqIFRequirement
	Relations    []ReqIFRelation
	// CustomFields are the project's requirement custom fields, exported
	// as further attributes of the Requirement type.
	CustomFields []ReqIFCustomField
}

// ReqIFCustomField is a custom field exported as an attribute: Type is the
// custom field type (number → REAL, checkbox → BOOLEAN, date → DATE,
// single_select and multi_select → ENUMERATION with the options, others →
// STRING); the attribute's LONG-NAME is Name.
type ReqIFCustomField struct {
	Key, Name, Type string
	Options         []string
}

// ReqIFFolder is a requirement folder.
type ReqIFFolder struct {
	ID, ParentID, Name string
	CreatedAt          time.Time
}

// ReqIFRequirement is one requirement. Enumerated fields hold model codes.
type ReqIFRequirement struct {
	ID, Key, Title, Description         string
	Obligation, Type, Priority, Status  string
	Source, Rationale, Owner, CreatedBy string
	FolderID                            string
	Tags, Categories                    []string
	Version                             int
	Criteria                            []ReqIFCriterion
	CreatedAt, UpdatedAt                time.Time
	Attachments                         []ReqIFFile // written by WriteReqIFZ only
	attachmentPaths                     []string
	// Custom are the custom field values by key in their text form (one
	// string per selected option).
	Custom map[string][]string
}

// ReqIFCriterion is an evaluation criterion.
type ReqIFCriterion struct{ Description, Method, Threshold string }

// ReqIFRelation is a relation between two requirements (source → target).
type ReqIFRelation struct {
	ID, Type, SourceID, TargetID string
	CreatedAt                    time.Time
}

// ReqIFFile is an attachment of a requirement.
type ReqIFFile struct {
	ID, Name, ContentType string
	Data                  []byte
	// Load reads the content when Data is nil (so that large archives are
	// not held in memory at once).
	Load func() ([]byte, error)
}

// ReqIF enumeration values: code → long name. The long names are what
// ReqIF tools display; the importer maps them back to the codes.
var (
	reqifObligations = []string{"must", "must_not", "shall", "shall_not", "should", "should_not", "may", "optional", "will", "describe"}
	reqifTypes       = []string{"functional", "non_functional", "security", "performance", "usability", "interface", "constraint", "business", "regulatory"}
	reqifPriorities  = []string{"critical", "high", "medium", "low"}
	reqifStatuses    = []string{"draft", "review", "approved", "implemented", "verified", "rejected", "obsolete"}
	reqifRelTypes    = []string{"refines", "depends_on", "provides", "derived_from", "conflicts_with", "duplicates", "relates_to"}
)

// reqifLabel turns a code into its long name: obligations upper case
// ("MUST NOT"), others capitalised ("Non-functional", "Depends on").
func reqifLabel(enum, code string) string {
	switch enum {
	case "obligation":
		return strings.ToUpper(strings.ReplaceAll(code, "_", " "))
	case "type":
		code = strings.ReplaceAll(code, "_", "-")
	default:
		code = strings.ReplaceAll(code, "_", " ")
	}
	if code == "" {
		return ""
	}
	return strings.ToUpper(code[:1]) + code[1:]
}

type reqifAttr struct {
	id, name, kind, enum string // kind: string | xhtml | integer | date | enum
}

// Attribute definitions of the requirement type, in output order.
var reqifReqAttrs = []reqifAttr{
	{"qtf-ad-key", "ReqIF.ForeignID", "string", ""},
	{"qtf-ad-title", "ReqIF.Name", "string", ""},
	{"qtf-ad-text", "ReqIF.Text", "xhtml", ""},
	{"qtf-ad-obligation", "Obligation", "enum", "obligation"},
	{"qtf-ad-type", "Type", "enum", "type"},
	{"qtf-ad-priority", "Priority", "enum", "priority"},
	{"qtf-ad-status", "Status", "enum", "status"},
	{"qtf-ad-source", "Source", "string", ""},
	{"qtf-ad-rationale", "Rationale", "string", ""},
	{"qtf-ad-owner", "Owner", "string", ""},
	{"qtf-ad-tags", "Tags", "string", ""},
	{"qtf-ad-categories", "Categories", "string", ""},
	{"qtf-ad-criteria", "Acceptance criteria", "xhtml", ""},
	{"qtf-ad-version", "Version", "integer", ""},
	{"qtf-ad-created-by", "ReqIF.ForeignCreatedBy", "string", ""},
	{"qtf-ad-created-on", "ReqIF.ForeignCreatedOn", "date", ""},
	{"qtf-ad-attachments", "Attachments", "xhtml", ""},
}

func reqifEnumValues(enum string) []string {
	switch enum {
	case "obligation":
		return reqifObligations
	case "type":
		return reqifTypes
	case "priority":
		return reqifPriorities
	}
	return reqifStatuses
}

func reqifTime(t time.Time) string {
	if t.IsZero() {
		t = time.Unix(0, 0)
	}
	return t.UTC().Format(reqifDateTime)
}

// reqifWriter writes indented XML.
type reqifWriter struct {
	b     strings.Builder
	depth int
}

func (w *reqifWriter) line(s string) {
	w.b.WriteString(strings.Repeat("  ", w.depth))
	w.b.WriteString(s)
	w.b.WriteByte('\n')
}
func (w *reqifWriter) open(s string)  { w.line(s); w.depth++ }
func (w *reqifWriter) close(s string) { w.depth--; w.line(s) }

func reqifAttrs(id, name string, changed time.Time) string {
	s := ` IDENTIFIER="` + esc(id) + `" LAST-CHANGE="` + reqifTime(changed) + `"`
	if name != "" {
		s += ` LONG-NAME="` + esc(name) + `"`
	}
	return s
}

// WriteReqIF writes doc as a ReqIF 1.2 XML document.
func WriteReqIF(out io.Writer, doc ReqIFDocument) error {
	_, err := io.WriteString(out, buildReqIF(doc))
	return err
}

// WriteReqIFZ writes doc as a .reqifz archive: the .reqif document and the
// requirements' attachments below files/<key>/, referenced from the
// "Attachments" attribute.
func WriteReqIFZ(out io.Writer, doc ReqIFDocument) error {
	zw := zip.NewWriter(out)
	used := map[string]bool{}
	type entry struct {
		name string
		file ReqIFFile
	}
	var files []entry
	for i := range doc.Requirements {
		rq := &doc.Requirements[i]
		rq.attachmentPaths = nil
		for _, f := range rq.Attachments {
			stem := safeFileName(rq.Key)
			if stem == "" {
				stem = "requirement"
			}
			name := safeFileName(f.Name)
			if name == "" {
				name = "file"
			}
			p := path.Join("files", stem, name)
			for n := 2; used[p]; n++ {
				ext := path.Ext(name)
				p = path.Join("files", stem, strings.TrimSuffix(name, ext)+"-"+strconv.Itoa(n)+ext)
			}
			used[p] = true
			rq.attachmentPaths = append(rq.attachmentPaths, p)
			files = append(files, entry{p, f})
		}
	}
	stem := safeFileName(doc.ProjectKey)
	if stem == "" {
		stem = "requirements"
	}
	mod := doc.GeneratedAt
	if mod.IsZero() {
		mod = time.Now().UTC()
	}
	write := func(name string, data []byte) error {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: mod})
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	}
	if err := write(stem+".reqif", []byte(buildReqIF(doc))); err != nil {
		return err
	}
	for _, f := range files {
		data := f.file.Data
		if data == nil && f.file.Load != nil {
			var err error
			if data, err = f.file.Load(); err != nil {
				return err
			}
		}
		if err := write(f.name, data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func buildReqIF(doc ReqIFDocument) string {
	now := doc.GeneratedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	w := &reqifWriter{}
	w.b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	w.open(`<REQ-IF xmlns="` + reqifNS + `" xmlns:xhtml="` + xhtmlNS + `">`)

	// header
	w.open("<THE-HEADER>")
	w.open(`<REQ-IF-HEADER IDENTIFIER="qtf-header-` + esc(doc.ProjectID) + `">`)
	if doc.Comment != "" {
		w.line("<COMMENT>" + esc(doc.Comment) + "</COMMENT>")
	}
	w.line("<CREATION-TIME>" + reqifTime(now) + "</CREATION-TIME>")
	w.line("<REQ-IF-TOOL-ID>" + ReqIFToolID + "</REQ-IF-TOOL-ID>")
	w.line("<REQ-IF-VERSION>1.0</REQ-IF-VERSION>")
	src := ReqIFToolID
	if doc.ToolVersion != "" {
		src += " " + doc.ToolVersion
	}
	w.line("<SOURCE-TOOL-ID>" + esc(src) + "</SOURCE-TOOL-ID>")
	w.line("<TITLE>" + esc(doc.Title) + "</TITLE>")
	w.close("</REQ-IF-HEADER>")
	w.close("</THE-HEADER>")

	w.open("<CORE-CONTENT>")
	w.open("<REQ-IF-CONTENT>")

	// datatypes
	w.open("<DATATYPES>")
	w.line(`<DATATYPE-DEFINITION-STRING` + reqifAttrs("qtf-dt-string", "String", now) + ` MAX-LENGTH="` + strconv.Itoa(reqifMaxLen) + `"/>`)
	w.line(`<DATATYPE-DEFINITION-XHTML` + reqifAttrs("qtf-dt-xhtml", "Formatted text", now) + `/>`)
	w.line(`<DATATYPE-DEFINITION-INTEGER` + reqifAttrs("qtf-dt-integer", "Integer", now) + ` MAX="2147483647" MIN="0"/>`)
	w.line(`<DATATYPE-DEFINITION-DATE` + reqifAttrs("qtf-dt-date", "Date", now) + `/>`)
	for _, enum := range []string{"obligation", "type", "priority", "status"} {
		w.open(`<DATATYPE-DEFINITION-ENUMERATION` + reqifAttrs("qtf-dt-"+enum, reqifLabel("", enum), now) + `>`)
		w.open("<SPECIFIED-VALUES>")
		for i, code := range reqifEnumValues(enum) {
			w.open(`<ENUM-VALUE` + reqifAttrs("qtf-ev-"+enum+"-"+strings.ReplaceAll(code, "_", "-"), reqifLabel(enum, code), now) + `>`)
			w.line(`<PROPERTIES><EMBEDDED-VALUE KEY="` + strconv.Itoa(i) + `" OTHER-CONTENT="` + code + `"/></PROPERTIES>`)
			w.close("</ENUM-VALUE>")
		}
		w.close("</SPECIFIED-VALUES>")
		w.close("</DATATYPE-DEFINITION-ENUMERATION>")
	}
	reqifCustomDatatypes(w, doc.CustomFields, now)
	w.close("</DATATYPES>")

	// spec types
	w.open("<SPEC-TYPES>")
	w.open(`<SPEC-OBJECT-TYPE` + reqifAttrs("qtf-type-requirement", "Requirement", now) + `>`)
	w.open("<SPEC-ATTRIBUTES>")
	for _, a := range reqifReqAttrs {
		reqifAttrDef(w, a, now)
	}
	reqifCustomAttrDefs(w, doc.CustomFields, now)
	w.close("</SPEC-ATTRIBUTES>")
	w.close("</SPEC-OBJECT-TYPE>")
	w.open(`<SPEC-OBJECT-TYPE` + reqifAttrs("qtf-type-heading", "Heading", now) + `>`)
	w.open("<SPEC-ATTRIBUTES>")
	reqifAttrDef(w, reqifAttr{"qtf-ad-chapter", "ReqIF.ChapterName", "string", ""}, now)
	w.close("</SPEC-ATTRIBUTES>")
	w.close("</SPEC-OBJECT-TYPE>")
	w.line(`<SPECIFICATION-TYPE` + reqifAttrs("qtf-type-specification", "Requirements specification", now) + `/>`)
	for _, rt := range reqifRelTypes {
		w.line(`<SPEC-RELATION-TYPE` + reqifAttrs("qtf-reltype-"+strings.ReplaceAll(rt, "_", "-"), reqifLabel("", rt), now) + `/>`)
	}
	w.close("</SPEC-TYPES>")

	// folders: keep those that are the ancestors of an exported requirement
	// (the caller decides which folders to pass).
	folders := map[string]*ReqIFFolder{}
	for i := range doc.Folders {
		folders[doc.Folders[i].ID] = &doc.Folders[i]
	}
	inExport := map[string]bool{}
	for _, rq := range doc.Requirements {
		inExport[rq.ID] = true
	}

	// spec objects
	w.open("<SPEC-OBJECTS>")
	for _, f := range doc.Folders {
		w.open(`<SPEC-OBJECT` + reqifAttrs("qtf-folder-"+f.ID, f.Name, f.CreatedAt) + `>`)
		w.open("<VALUES>")
		reqifString(w, "qtf-ad-chapter", f.Name)
		w.close("</VALUES>")
		w.line("<TYPE><SPEC-OBJECT-TYPE-REF>qtf-type-heading</SPEC-OBJECT-TYPE-REF></TYPE>")
		w.close("</SPEC-OBJECT>")
	}
	for _, rq := range doc.Requirements {
		reqifObject(w, rq, doc.CustomFields)
	}
	w.close("</SPEC-OBJECTS>")

	// relations between exported requirements
	var rels []ReqIFRelation
	for _, rel := range doc.Relations {
		if inExport[rel.SourceID] && inExport[rel.TargetID] && contains(reqifRelTypes, rel.Type) {
			rels = append(rels, rel)
		}
	}
	if len(rels) > 0 {
		w.open("<SPEC-RELATIONS>")
		for _, rel := range rels {
			w.open(`<SPEC-RELATION` + reqifAttrs("qtf-rel-"+rel.ID, reqifLabel("", rel.Type), rel.CreatedAt) + `>`)
			w.line("<SOURCE><SPEC-OBJECT-REF>qtf-req-" + esc(rel.SourceID) + "</SPEC-OBJECT-REF></SOURCE>")
			w.line("<TARGET><SPEC-OBJECT-REF>qtf-req-" + esc(rel.TargetID) + "</SPEC-OBJECT-REF></TARGET>")
			w.line("<TYPE><SPEC-RELATION-TYPE-REF>qtf-reltype-" + strings.ReplaceAll(rel.Type, "_", "-") + "</SPEC-RELATION-TYPE-REF></TYPE>")
			w.close("</SPEC-RELATION>")
		}
		w.close("</SPEC-RELATIONS>")
	}

	// specification: the folder tree
	reqsIn := map[string][]ReqIFRequirement{}
	for _, rq := range doc.Requirements {
		fid := rq.FolderID
		if folders[fid] == nil {
			fid = ""
		}
		reqsIn[fid] = append(reqsIn[fid], rq)
	}
	subs := map[string][]string{}
	for _, f := range doc.Folders {
		parent := f.ParentID
		if folders[parent] == nil || parent == f.ID {
			parent = ""
		}
		subs[parent] = append(subs[parent], f.ID)
	}
	w.open(`<SPECIFICATIONS>`)
	w.open(`<SPECIFICATION` + reqifAttrs("qtf-spec-"+doc.ProjectID, doc.Title, now) + `>`)
	w.line("<TYPE><SPECIFICATION-TYPE-REF>qtf-type-specification</SPECIFICATION-TYPE-REF></TYPE>")
	seen := map[string]bool{}
	var hier func(fid string, depth int)
	hier = func(fid string, depth int) {
		if len(reqsIn[fid]) == 0 && len(subs[fid]) == 0 {
			return
		}
		w.open("<CHILDREN>")
		for _, rq := range reqsIn[fid] {
			w.line(`<SPEC-HIERARCHY` + reqifAttrs("qtf-sh-"+rq.ID, "", rq.UpdatedAt) + `><OBJECT><SPEC-OBJECT-REF>qtf-req-` + esc(rq.ID) + `</SPEC-OBJECT-REF></OBJECT></SPEC-HIERARCHY>`)
		}
		for _, sub := range subs[fid] {
			if seen[sub] || depth > 64 {
				continue
			}
			seen[sub] = true
			f := folders[sub]
			w.open(`<SPEC-HIERARCHY` + reqifAttrs("qtf-sh-"+f.ID, "", f.CreatedAt) + `>`)
			w.line("<OBJECT><SPEC-OBJECT-REF>qtf-folder-" + esc(f.ID) + "</SPEC-OBJECT-REF></OBJECT>")
			hier(sub, depth+1)
			w.close("</SPEC-HIERARCHY>")
		}
		w.close("</CHILDREN>")
	}
	hier("", 0)
	w.close("</SPECIFICATION>")
	w.close("</SPECIFICATIONS>")

	w.close("</REQ-IF-CONTENT>")
	w.close("</CORE-CONTENT>")
	w.close("</REQ-IF>")
	return w.b.String()
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func reqifAttrDef(w *reqifWriter, a reqifAttr, now time.Time) {
	switch a.kind {
	case "enum":
		w.open(`<ATTRIBUTE-DEFINITION-ENUMERATION` + reqifAttrs(a.id, a.name, now) + ` MULTI-VALUED="false">`)
		w.line("<TYPE><DATATYPE-DEFINITION-ENUMERATION-REF>qtf-dt-" + a.enum + "</DATATYPE-DEFINITION-ENUMERATION-REF></TYPE>")
		w.close("</ATTRIBUTE-DEFINITION-ENUMERATION>")
	default:
		kind := strings.ToUpper(a.kind)
		w.open(`<ATTRIBUTE-DEFINITION-` + kind + reqifAttrs(a.id, a.name, now) + `>`)
		w.line("<TYPE><DATATYPE-DEFINITION-" + kind + "-REF>qtf-dt-" + a.kind + "</DATATYPE-DEFINITION-" + kind + "-REF></TYPE>")
		w.close("</ATTRIBUTE-DEFINITION-" + kind + ">")
	}
}

func reqifString(w *reqifWriter, def, v string) {
	if v == "" {
		return
	}
	w.line(`<ATTRIBUTE-VALUE-STRING THE-VALUE="` + esc(v) + `"><DEFINITION><ATTRIBUTE-DEFINITION-STRING-REF>` + def +
		`</ATTRIBUTE-DEFINITION-STRING-REF></DEFINITION></ATTRIBUTE-VALUE-STRING>`)
}

func reqifXHTML(w *reqifWriter, def, xhtml string) {
	w.line(`<ATTRIBUTE-VALUE-XHTML><DEFINITION><ATTRIBUTE-DEFINITION-XHTML-REF>` + def +
		`</ATTRIBUTE-DEFINITION-XHTML-REF></DEFINITION><THE-VALUE>` + xhtml + `</THE-VALUE></ATTRIBUTE-VALUE-XHTML>`)
}

func reqifEnum(w *reqifWriter, def, enum, code string) {
	if !contains(reqifEnumValues(enum), code) {
		return
	}
	w.line(`<ATTRIBUTE-VALUE-ENUMERATION><DEFINITION><ATTRIBUTE-DEFINITION-ENUMERATION-REF>` + def +
		`</ATTRIBUTE-DEFINITION-ENUMERATION-REF></DEFINITION><VALUES><ENUM-VALUE-REF>qtf-ev-` + enum + "-" +
		strings.ReplaceAll(code, "_", "-") + `</ENUM-VALUE-REF></VALUES></ATTRIBUTE-VALUE-ENUMERATION>`)
}

// criteriaXHTML renders the criteria as a list of "method: description
// (threshold: …)" items, the form the importer reads back.
func criteriaXHTML(cs []ReqIFCriterion) string {
	var b strings.Builder
	b.WriteString("<xhtml:div><xhtml:ul>")
	for _, c := range cs {
		item := strings.Join(strings.Fields(c.Description), " ")
		if c.Method != "" {
			item = c.Method + ": " + item
		}
		if t := strings.TrimSpace(c.Threshold); t != "" {
			item += " (threshold: " + t + ")"
		}
		b.WriteString("<xhtml:li>" + esc(item) + "</xhtml:li>")
	}
	b.WriteString("</xhtml:ul></xhtml:div>")
	return b.String()
}

func reqifObject(w *reqifWriter, rq ReqIFRequirement, custom []ReqIFCustomField) {
	w.open(`<SPEC-OBJECT` + reqifAttrs("qtf-req-"+rq.ID, rq.Key, rq.UpdatedAt) + `>`)
	w.open("<VALUES>")
	reqifString(w, "qtf-ad-key", rq.Key)
	reqifString(w, "qtf-ad-title", rq.Title)
	if strings.TrimSpace(rq.Description) != "" {
		files := map[string]string{}
		for i, f := range rq.Attachments {
			if i < len(rq.attachmentPaths) {
				files[f.ID] = rq.attachmentPaths[i]
			}
		}
		reqifXHTML(w, "qtf-ad-text", MarkdownToXHTML(rq.Description, func(url string) (string, string) {
			// images that are attachments in the archive: point to the file
			for i, f := range rq.Attachments {
				if p, ok := files[f.ID]; ok && strings.Contains(url, "/files/"+f.ID+"/") {
					return p, rq.Attachments[i].ContentType
				}
			}
			return url, ""
		}))
	}
	reqifEnum(w, "qtf-ad-obligation", "obligation", rq.Obligation)
	reqifEnum(w, "qtf-ad-type", "type", rq.Type)
	reqifEnum(w, "qtf-ad-priority", "priority", rq.Priority)
	reqifEnum(w, "qtf-ad-status", "status", rq.Status)
	reqifString(w, "qtf-ad-source", rq.Source)
	reqifString(w, "qtf-ad-rationale", rq.Rationale)
	reqifString(w, "qtf-ad-owner", rq.Owner)
	reqifString(w, "qtf-ad-tags", strings.Join(rq.Tags, ", "))
	reqifString(w, "qtf-ad-categories", strings.Join(rq.Categories, ", "))
	if len(rq.Criteria) > 0 {
		reqifXHTML(w, "qtf-ad-criteria", criteriaXHTML(rq.Criteria))
	}
	w.line(`<ATTRIBUTE-VALUE-INTEGER THE-VALUE="` + strconv.Itoa(rq.Version) + `"><DEFINITION><ATTRIBUTE-DEFINITION-INTEGER-REF>qtf-ad-version</ATTRIBUTE-DEFINITION-INTEGER-REF></DEFINITION></ATTRIBUTE-VALUE-INTEGER>`)
	reqifString(w, "qtf-ad-created-by", rq.CreatedBy)
	if !rq.CreatedAt.IsZero() {
		w.line(`<ATTRIBUTE-VALUE-DATE THE-VALUE="` + reqifTime(rq.CreatedAt) + `"><DEFINITION><ATTRIBUTE-DEFINITION-DATE-REF>qtf-ad-created-on</ATTRIBUTE-DEFINITION-DATE-REF></DEFINITION></ATTRIBUTE-VALUE-DATE>`)
	}
	if len(rq.attachmentPaths) > 0 {
		var b strings.Builder
		b.WriteString("<xhtml:div><xhtml:ul>")
		for i, p := range rq.attachmentPaths {
			f := rq.Attachments[i]
			ct := f.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			b.WriteString(`<xhtml:li><xhtml:object data="` + esc(p) + `" type="` + esc(strings.SplitN(ct, ";", 2)[0]) + `">` + esc(f.Name) + `</xhtml:object></xhtml:li>`)
		}
		b.WriteString("</xhtml:ul></xhtml:div>")
		reqifXHTML(w, "qtf-ad-attachments", b.String())
	}
	reqifCustomValues(w, custom, rq.Custom)
	w.close("</VALUES>")
	w.line("<TYPE><SPEC-OBJECT-TYPE-REF>qtf-type-requirement</SPEC-OBJECT-TYPE-REF></TYPE>")
	w.close("</SPEC-OBJECT>")
}

// ReqIFFileName returns the download name of a ReqIF export.
func ReqIFFileName(projectKey string, at time.Time, zipped bool) string {
	ext := "reqif"
	if zipped {
		ext = "reqifz"
	}
	return fmt.Sprintf("%s-%s.%s", projectKey, at.UTC().Format("20060102-150405"), ext)
}

// ---------------------------------------------------------------- custom fields

func reqifCustomKind(t string) string {
	switch t {
	case "number":
		return "REAL"
	case "checkbox":
		return "BOOLEAN"
	case "date":
		return "DATE"
	case "single_select", "multi_select":
		return "ENUMERATION"
	}
	return "STRING"
}

func reqifCustomID(f ReqIFCustomField) string {
	return "qtf-ad-cf-" + strings.ReplaceAll(f.Key, "_", "-")
}

func reqifOptionID(f ReqIFCustomField, i int) string {
	return "qtf-ev-cf-" + strings.ReplaceAll(f.Key, "_", "-") + "-" + strconv.Itoa(i)
}

// reqifCustomDatatypes writes the REAL and BOOLEAN datatypes (when used)
// and one enumeration per select field.
func reqifCustomDatatypes(w *reqifWriter, fields []ReqIFCustomField, now time.Time) {
	real, boolean := false, false
	for _, f := range fields {
		switch reqifCustomKind(f.Type) {
		case "REAL":
			real = true
		case "BOOLEAN":
			boolean = true
		case "ENUMERATION":
			w.open(`<DATATYPE-DEFINITION-ENUMERATION` + reqifAttrs("qtf-dt-cf-"+strings.ReplaceAll(f.Key, "_", "-"), f.Name, now) + `>`)
			w.open("<SPECIFIED-VALUES>")
			for i, o := range f.Options {
				w.open(`<ENUM-VALUE` + reqifAttrs(reqifOptionID(f, i), o, now) + `>`)
				w.line(`<PROPERTIES><EMBEDDED-VALUE KEY="` + strconv.Itoa(i) + `" OTHER-CONTENT="` + esc(o) + `"/></PROPERTIES>`)
				w.close("</ENUM-VALUE>")
			}
			w.close("</SPECIFIED-VALUES>")
			w.close("</DATATYPE-DEFINITION-ENUMERATION>")
		}
	}
	if real {
		w.line(`<DATATYPE-DEFINITION-REAL` + reqifAttrs("qtf-dt-real", "Real", now) + ` ACCURACY="10" MAX="1.0E308" MIN="-1.0E308"/>`)
	}
	if boolean {
		w.line(`<DATATYPE-DEFINITION-BOOLEAN` + reqifAttrs("qtf-dt-boolean", "Boolean", now) + `/>`)
	}
}

func reqifCustomAttrDefs(w *reqifWriter, fields []ReqIFCustomField, now time.Time) {
	for _, f := range fields {
		kind := reqifCustomKind(f.Type)
		dt := "qtf-dt-" + strings.ToLower(kind)
		multi := ""
		if kind == "ENUMERATION" {
			dt = "qtf-dt-cf-" + strings.ReplaceAll(f.Key, "_", "-")
			multi = ` MULTI-VALUED="` + strconv.FormatBool(f.Type == "multi_select") + `"`
		}
		w.open(`<ATTRIBUTE-DEFINITION-` + kind + reqifAttrs(reqifCustomID(f), f.Name, now) + multi + `>`)
		w.line("<TYPE><DATATYPE-DEFINITION-" + kind + "-REF>" + dt + "</DATATYPE-DEFINITION-" + kind + "-REF></TYPE>")
		w.close("</ATTRIBUTE-DEFINITION-" + kind + ">")
	}
}

func reqifCustomValues(w *reqifWriter, fields []ReqIFCustomField, values map[string][]string) {
	for _, f := range fields {
		vals := values[f.Key]
		if len(vals) == 0 || (len(vals) == 1 && vals[0] == "") {
			continue
		}
		kind := reqifCustomKind(f.Type)
		def := `<DEFINITION><ATTRIBUTE-DEFINITION-` + kind + `-REF>` + reqifCustomID(f) + `</ATTRIBUTE-DEFINITION-` + kind + `-REF></DEFINITION>`
		switch kind {
		case "ENUMERATION":
			var refs strings.Builder
			for _, v := range vals {
				for i, o := range f.Options {
					if o == v {
						refs.WriteString("<ENUM-VALUE-REF>" + reqifOptionID(f, i) + "</ENUM-VALUE-REF>")
					}
				}
			}
			if refs.Len() > 0 {
				w.line(`<ATTRIBUTE-VALUE-ENUMERATION>` + def + `<VALUES>` + refs.String() + `</VALUES></ATTRIBUTE-VALUE-ENUMERATION>`)
			}
		case "DATE":
			if t, err := time.Parse(time.DateOnly, vals[0]); err == nil {
				w.line(`<ATTRIBUTE-VALUE-DATE THE-VALUE="` + reqifTime(t) + `">` + def + `</ATTRIBUTE-VALUE-DATE>`)
			}
		default:
			w.line(`<ATTRIBUTE-VALUE-` + kind + ` THE-VALUE="` + esc(strings.Join(vals, ", ")) + `">` + def + `</ATTRIBUTE-VALUE-` + kind + `>`)
		}
	}
}
