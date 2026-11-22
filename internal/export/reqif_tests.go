package export

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Test cases, their "Verifies" links and verification records in the ReqIF
// export (see reqif.go for requirements):
//
//   - a test is a SPEC-OBJECT of the type "Test case" (qtf-type-test) with
//     ReqIF.ForeignID, ReqIF.Name, ReqIF.Text, Preconditions, Steps (an
//     XHTML table Action | Data | Expected result | Calls), Expected result,
//     Execution type, Priority, Status (ENUMERATIONs), Estimated minutes,
//     Tags, Version and the test custom fields;
//   - the tests and their folders (heading objects) are the SPEC-HIERARCHY
//     of a second SPECIFICATION;
//   - a trace link is a SPEC-RELATION of the type "Verifies" from the test
//     to the requirement;
//   - a verification record is a SPEC-OBJECT of the type "Verification
//     record" in a third SPECIFICATION, related to its requirement by a
//     "Verification record of" SPEC-RELATION.
//
// The importer (internal/interop) reads the "Test case" type back as tests
// and the "Verifies" relations as trace links.

// ReqIF type and relation type long names of tests, links and records.
const (
	ReqIFTestType        = "Test case"
	ReqIFRecordType      = "Verification record"
	ReqIFVerifiesType    = "Verifies"
	ReqIFRecordRelType   = "Verification record of"
	reqifTestSpecType    = "Test specification"
	reqifRecordsSpecType = "Verification records"
)

var (
	reqifTestStatuses = []string{"draft", "ready", "review", "deprecated"}
	reqifExecTypes    = []string{"manual", "automated"}
)

// ReqIFTest is one test case. Enumerated fields hold model codes.
type ReqIFTest struct {
	ID, Key, Title, Description string
	Preconditions, Expected     string
	ExecutionType, Priority     string
	Status, CreatedBy, FolderID string
	EstimatedMinutes, Version   int
	Tags                        []string
	Steps                       []ReqIFStep
	CreatedAt, UpdatedAt        time.Time
	Attachments                 []ReqIFFile // written by WriteReqIFZ only
	attachmentPaths             []string
	Custom                      map[string][]string
}

// ReqIFStep is a test step; Call is the key of a called test.
type ReqIFStep struct{ Action, Data, Expected, Call string }

// ReqIFLink is a trace link: the test verifies the requirement.
type ReqIFLink struct {
	ID, RequirementID, TestID string
	CreatedAt                 time.Time
}

// ReqIFRecord is a verification record of a requirement.
type ReqIFRecord struct {
	ID, RequirementID, RequirementKey string
	Method, Result, Criterion         string
	PerformedBy, PerformedOn, Note    string
	ValidUntil, Validity, SignedOffBy string
	CreatedAt, UpdatedAt              time.Time
}

var reqifTestAttrs = []reqifAttr{
	{"qtf-at-key", "ReqIF.ForeignID", "string", ""},
	{"qtf-at-title", "ReqIF.Name", "string", ""},
	{"qtf-at-text", "ReqIF.Text", "xhtml", ""},
	{"qtf-at-preconditions", "Preconditions", "xhtml", ""},
	{"qtf-at-steps", "Steps", "xhtml", ""},
	{"qtf-at-expected", "Expected result", "xhtml", ""},
	{"qtf-at-exec", "Execution type", "enum", "exec"},
	{"qtf-at-priority", "Priority", "enum", "priority"},
	{"qtf-at-status", "Status", "enum", "teststatus"},
	{"qtf-at-estimate", "Estimated minutes", "integer", ""},
	{"qtf-at-tags", "Tags", "string", ""},
	{"qtf-at-version", "Version", "integer", ""},
	{"qtf-at-created-by", "ReqIF.ForeignCreatedBy", "string", ""},
	{"qtf-at-created-on", "ReqIF.ForeignCreatedOn", "date", ""},
	{"qtf-at-attachments", "Attachments", "xhtml", ""},
}

var reqifRecordAttrs = []reqifAttr{
	{"qtf-ar-title", "ReqIF.Name", "string", ""},
	{"qtf-ar-requirement", "Requirement", "string", ""},
	{"qtf-ar-criterion", "Criterion", "string", ""},
	{"qtf-ar-method", "Method", "string", ""},
	{"qtf-ar-result", "Result", "string", ""},
	{"qtf-ar-by", "Performed by", "string", ""},
	{"qtf-ar-on", "Performed on", "string", ""},
	{"qtf-ar-note", "Note", "string", ""},
	{"qtf-ar-valid-until", "Valid until", "string", ""},
	{"qtf-ar-validity", "Validity", "string", ""},
	{"qtf-ar-signoff", "Signed off by", "string", ""},
}

func reqifTestCustomID(f ReqIFCustomField) string {
	return "qtf-at-cf-" + strings.ReplaceAll(f.Key, "_", "-")
}

func reqifTestOptionID(f ReqIFCustomField, i int) string {
	return "qtf-ev-tcf-" + strings.ReplaceAll(f.Key, "_", "-") + "-" + strconv.Itoa(i)
}

func reqifTestTypes(w *reqifWriter, doc ReqIFDocument, now time.Time) {
	if len(doc.Tests) > 0 {
		w.open(`<SPEC-OBJECT-TYPE` + reqifAttrs("qtf-type-test", ReqIFTestType, now) + `>`)
		w.open("<SPEC-ATTRIBUTES>")
		for _, a := range reqifTestAttrs {
			reqifAttrDef(w, a, now)
		}
		reqifCustomAttrDefs(w, doc.TestCustomFields, reqifTestCustomID, "qtf-dt-tcf-", now)
		w.close("</SPEC-ATTRIBUTES>")
		w.close("</SPEC-OBJECT-TYPE>")
		w.line(`<SPECIFICATION-TYPE` + reqifAttrs("qtf-type-test-specification", reqifTestSpecType, now) + `/>`)
	}
	if len(doc.Records) > 0 {
		w.open(`<SPEC-OBJECT-TYPE` + reqifAttrs("qtf-type-record", ReqIFRecordType, now) + `>`)
		w.open("<SPEC-ATTRIBUTES>")
		for _, a := range reqifRecordAttrs {
			reqifAttrDef(w, a, now)
		}
		w.close("</SPEC-ATTRIBUTES>")
		w.close("</SPEC-OBJECT-TYPE>")
		w.line(`<SPECIFICATION-TYPE` + reqifAttrs("qtf-type-records-specification", reqifRecordsSpecType, now) + `/>`)
	}
}

func reqifTestRelationTypes(w *reqifWriter, doc ReqIFDocument, now time.Time) {
	if len(doc.Tests) > 0 {
		w.line(`<SPEC-RELATION-TYPE` + reqifAttrs("qtf-reltype-verifies", ReqIFVerifiesType, now) + `/>`)
	}
	if len(doc.Records) > 0 {
		w.line(`<SPEC-RELATION-TYPE` + reqifAttrs("qtf-reltype-record", ReqIFRecordRelType, now) + `/>`)
	}
}

// attachmentPathsByID maps attachment ids to their archive paths.
func attachmentPathsByID(files []ReqIFFile, paths []string) map[string]string {
	out := map[string]string{}
	for i, f := range files {
		if i < len(paths) {
			out[f.ID] = paths[i]
		}
	}
	return out
}

// imageResolver points images that are attachments in the archive to the
// archive file.
func imageResolver(files []ReqIFFile, paths []string) func(string) (string, string) {
	byID := attachmentPathsByID(files, paths)
	return func(url string) (string, string) {
		for _, f := range files {
			if p, ok := byID[f.ID]; ok && strings.Contains(url, "/files/"+f.ID+"/") {
				return p, f.ContentType
			}
		}
		return url, ""
	}
}

// attachmentsXHTML lists archive files as XHTML objects.
func attachmentsXHTML(paths []string, files []ReqIFFile) string {
	var b strings.Builder
	b.WriteString("<xhtml:div><xhtml:ul>")
	for i, p := range paths {
		f := files[i]
		ct := f.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		b.WriteString(`<xhtml:li><xhtml:object data="` + esc(p) + `" type="` + esc(strings.SplitN(ct, ";", 2)[0]) + `">` + esc(f.Name) + `</xhtml:object></xhtml:li>`)
	}
	b.WriteString("</xhtml:ul></xhtml:div>")
	return b.String()
}

// stepsXHTML renders test steps as a table with a header row.
func stepsXHTML(steps []ReqIFStep) string {
	cell := func(tag, s string) string {
		var lines []string
		for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
			lines = append(lines, esc(strings.TrimSpace(l)))
		}
		return "<xhtml:" + tag + ">" + strings.Join(lines, "<xhtml:br/>") + "</xhtml:" + tag + ">"
	}
	var b strings.Builder
	b.WriteString("<xhtml:div><xhtml:table><xhtml:tr>")
	for _, h := range []string{"Action", "Data", "Expected result", "Calls"} {
		b.WriteString(cell("th", h))
	}
	b.WriteString("</xhtml:tr>")
	for _, st := range steps {
		b.WriteString("<xhtml:tr>" + cell("td", st.Action) + cell("td", st.Data) + cell("td", st.Expected) + cell("td", st.Call) + "</xhtml:tr>")
	}
	b.WriteString("</xhtml:table></xhtml:div>")
	return b.String()
}

func reqifTestObjects(w *reqifWriter, doc ReqIFDocument) {
	for _, f := range doc.TestFolders {
		w.open(`<SPEC-OBJECT` + reqifAttrs("qtf-tfolder-"+f.ID, f.Name, f.CreatedAt) + `>`)
		w.open("<VALUES>")
		reqifString(w, "qtf-ad-chapter", f.Name)
		w.close("</VALUES>")
		w.line("<TYPE><SPEC-OBJECT-TYPE-REF>qtf-type-heading</SPEC-OBJECT-TYPE-REF></TYPE>")
		w.close("</SPEC-OBJECT>")
	}
	for _, t := range doc.Tests {
		w.open(`<SPEC-OBJECT` + reqifAttrs("qtf-test-"+t.ID, t.Key, t.UpdatedAt) + `>`)
		w.open("<VALUES>")
		reqifString(w, "qtf-at-key", t.Key)
		reqifString(w, "qtf-at-title", t.Title)
		md := func(def, text string) {
			if strings.TrimSpace(text) != "" {
				reqifXHTML(w, def, MarkdownToXHTML(text, imageResolver(t.Attachments, t.attachmentPaths)))
			}
		}
		md("qtf-at-text", t.Description)
		md("qtf-at-preconditions", t.Preconditions)
		if len(t.Steps) > 0 {
			reqifXHTML(w, "qtf-at-steps", stepsXHTML(t.Steps))
		}
		md("qtf-at-expected", t.Expected)
		reqifEnum(w, "qtf-at-exec", "exec", t.ExecutionType)
		reqifEnum(w, "qtf-at-priority", "priority", t.Priority)
		reqifEnum(w, "qtf-at-status", "teststatus", t.Status)
		if t.EstimatedMinutes > 0 {
			w.line(`<ATTRIBUTE-VALUE-INTEGER THE-VALUE="` + strconv.Itoa(t.EstimatedMinutes) + `"><DEFINITION><ATTRIBUTE-DEFINITION-INTEGER-REF>qtf-at-estimate</ATTRIBUTE-DEFINITION-INTEGER-REF></DEFINITION></ATTRIBUTE-VALUE-INTEGER>`)
		}
		reqifString(w, "qtf-at-tags", strings.Join(t.Tags, ", "))
		w.line(`<ATTRIBUTE-VALUE-INTEGER THE-VALUE="` + strconv.Itoa(t.Version) + `"><DEFINITION><ATTRIBUTE-DEFINITION-INTEGER-REF>qtf-at-version</ATTRIBUTE-DEFINITION-INTEGER-REF></DEFINITION></ATTRIBUTE-VALUE-INTEGER>`)
		reqifString(w, "qtf-at-created-by", t.CreatedBy)
		if !t.CreatedAt.IsZero() {
			w.line(`<ATTRIBUTE-VALUE-DATE THE-VALUE="` + reqifTime(t.CreatedAt) + `"><DEFINITION><ATTRIBUTE-DEFINITION-DATE-REF>qtf-at-created-on</ATTRIBUTE-DEFINITION-DATE-REF></DEFINITION></ATTRIBUTE-VALUE-DATE>`)
		}
		if len(t.attachmentPaths) > 0 {
			reqifXHTML(w, "qtf-at-attachments", attachmentsXHTML(t.attachmentPaths, t.Attachments))
		}
		reqifCustomValues(w, doc.TestCustomFields, t.Custom, reqifTestCustomID, reqifTestOptionID)
		w.close("</VALUES>")
		w.line("<TYPE><SPEC-OBJECT-TYPE-REF>qtf-type-test</SPEC-OBJECT-TYPE-REF></TYPE>")
		w.close("</SPEC-OBJECT>")
	}
	for _, rc := range doc.Records {
		w.open(`<SPEC-OBJECT` + reqifAttrs("qtf-rec-"+rc.ID, "", rc.UpdatedAt) + `>`)
		w.open("<VALUES>")
		title := strings.TrimSpace(rc.Method + " " + rc.RequirementKey)
		if rc.PerformedOn != "" {
			title += " " + rc.PerformedOn
		}
		for _, v := range [][2]string{{"qtf-ar-title", title}, {"qtf-ar-requirement", rc.RequirementKey}, {"qtf-ar-criterion", rc.Criterion},
			{"qtf-ar-method", rc.Method}, {"qtf-ar-result", rc.Result}, {"qtf-ar-by", rc.PerformedBy}, {"qtf-ar-on", rc.PerformedOn},
			{"qtf-ar-note", rc.Note}, {"qtf-ar-valid-until", rc.ValidUntil}, {"qtf-ar-validity", rc.Validity}, {"qtf-ar-signoff", rc.SignedOffBy}} {
			reqifString(w, v[0], v[1])
		}
		w.close("</VALUES>")
		w.line("<TYPE><SPEC-OBJECT-TYPE-REF>qtf-type-record</SPEC-OBJECT-TYPE-REF></TYPE>")
		w.close("</SPEC-OBJECT>")
	}
}

// reqifTestRelations returns writers of the "Verifies" and record
// relations between exported objects.
func reqifTestRelations(doc ReqIFDocument, reqs map[string]bool) []func(*reqifWriter) {
	tests := map[string]bool{}
	for _, t := range doc.Tests {
		tests[t.ID] = true
	}
	var out []func(*reqifWriter)
	for _, l := range doc.Links {
		if !tests[l.TestID] || !reqs[l.RequirementID] {
			continue
		}
		l := l
		out = append(out, func(w *reqifWriter) {
			w.open(`<SPEC-RELATION` + reqifAttrs("qtf-link-"+l.ID, ReqIFVerifiesType, l.CreatedAt) + `>`)
			w.line("<SOURCE><SPEC-OBJECT-REF>qtf-test-" + esc(l.TestID) + "</SPEC-OBJECT-REF></SOURCE>")
			w.line("<TARGET><SPEC-OBJECT-REF>qtf-req-" + esc(l.RequirementID) + "</SPEC-OBJECT-REF></TARGET>")
			w.line("<TYPE><SPEC-RELATION-TYPE-REF>qtf-reltype-verifies</SPEC-RELATION-TYPE-REF></TYPE>")
			w.close("</SPEC-RELATION>")
		})
	}
	for _, rc := range doc.Records {
		if !reqs[rc.RequirementID] {
			continue
		}
		rc := rc
		out = append(out, func(w *reqifWriter) {
			w.open(`<SPEC-RELATION` + reqifAttrs("qtf-recrel-"+rc.ID, ReqIFRecordRelType, rc.CreatedAt) + `>`)
			w.line("<SOURCE><SPEC-OBJECT-REF>qtf-rec-" + esc(rc.ID) + "</SPEC-OBJECT-REF></SOURCE>")
			w.line("<TARGET><SPEC-OBJECT-REF>qtf-req-" + esc(rc.RequirementID) + "</SPEC-OBJECT-REF></TARGET>")
			w.line("<TYPE><SPEC-RELATION-TYPE-REF>qtf-reltype-record</SPEC-RELATION-TYPE-REF></TYPE>")
			w.close("</SPEC-RELATION>")
		})
	}
	return out
}

// reqifTestSpecifications writes the test specification (the test folder
// tree) and the verification records' specification.
func reqifTestSpecifications(w *reqifWriter, doc ReqIFDocument, now time.Time) {
	if len(doc.Tests) > 0 {
		folders := map[string]*ReqIFFolder{}
		for i := range doc.TestFolders {
			folders[doc.TestFolders[i].ID] = &doc.TestFolders[i]
		}
		testsIn := map[string][]ReqIFTest{}
		for _, t := range doc.Tests {
			fid := t.FolderID
			if folders[fid] == nil {
				fid = ""
			}
			testsIn[fid] = append(testsIn[fid], t)
		}
		subs := map[string][]string{}
		for _, f := range doc.TestFolders {
			parent := f.ParentID
			if folders[parent] == nil || parent == f.ID {
				parent = ""
			}
			subs[parent] = append(subs[parent], f.ID)
		}
		w.open(`<SPECIFICATION` + reqifAttrs("qtf-spec-tests-"+doc.ProjectID, doc.Title+" – Tests", now) + `>`)
		w.line("<TYPE><SPECIFICATION-TYPE-REF>qtf-type-test-specification</SPECIFICATION-TYPE-REF></TYPE>")
		seen := map[string]bool{}
		var hier func(fid string, depth int)
		hier = func(fid string, depth int) {
			if len(testsIn[fid]) == 0 && len(subs[fid]) == 0 {
				return
			}
			w.open("<CHILDREN>")
			for _, t := range testsIn[fid] {
				w.line(`<SPEC-HIERARCHY` + reqifAttrs("qtf-sh-"+t.ID, "", t.UpdatedAt) + `><OBJECT><SPEC-OBJECT-REF>qtf-test-` + esc(t.ID) + `</SPEC-OBJECT-REF></OBJECT></SPEC-HIERARCHY>`)
			}
			for _, sub := range subs[fid] {
				if seen[sub] || depth > 64 {
					continue
				}
				seen[sub] = true
				f := folders[sub]
				w.open(`<SPEC-HIERARCHY` + reqifAttrs("qtf-sh-"+f.ID, "", f.CreatedAt) + `>`)
				w.line("<OBJECT><SPEC-OBJECT-REF>qtf-tfolder-" + esc(f.ID) + "</SPEC-OBJECT-REF></OBJECT>")
				hier(sub, depth+1)
				w.close("</SPEC-HIERARCHY>")
			}
			w.close("</CHILDREN>")
		}
		hier("", 0)
		w.close("</SPECIFICATION>")
	}
	if len(doc.Records) > 0 {
		recs := append([]ReqIFRecord{}, doc.Records...)
		sort.SliceStable(recs, func(i, j int) bool { return recs[i].RequirementKey < recs[j].RequirementKey })
		w.open(`<SPECIFICATION` + reqifAttrs("qtf-spec-records-"+doc.ProjectID, doc.Title+" – Verification records", now) + `>`)
		w.line("<TYPE><SPECIFICATION-TYPE-REF>qtf-type-records-specification</SPECIFICATION-TYPE-REF></TYPE>")
		w.open("<CHILDREN>")
		for _, rc := range recs {
			w.line(`<SPEC-HIERARCHY` + reqifAttrs("qtf-sh-"+rc.ID, "", rc.UpdatedAt) + `><OBJECT><SPEC-OBJECT-REF>qtf-rec-` + esc(rc.ID) + `</SPEC-OBJECT-REF></OBJECT></SPEC-HIERARCHY>`)
		}
		w.close("</CHILDREN>")
		w.close("</SPECIFICATION>")
	}
}
