package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
)

// Generated documents (requirements specification, test plan): built from
// the project's requirements and tests (or a baseline of them) and saved in
// the document archive as kind "document". See docs/api.md "Generated
// documents".

// generatedTag marks generated documents in the archive (with the kind's
// tag and the version, e.g. "v2.0").
const generatedTag = "generated"

type generateDocumentReq struct {
	Kind        string            `json:"kind"`
	Format      string            `json:"format"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Options     report.DocOptions `json:"options"`
}

func (s *Server) handleGenerateDocument(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req generateDocumentReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if !slices.Contains(report.DocKinds, kind) {
		s.fail(w, r, badRequest("kind must be %s", strings.Join(report.DocKinds, " or ")))
		return
	}
	// a test plan contains the tests (the route requires requirement.read)
	if kind == report.DocTestPlan && !p.Can(rbac.TestRead) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.read required")
		return
	}
	// the result is a document of the archive: the upload permission
	if !p.Can(rbac.RequirementWr) && !p.Can(rbac.TestWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "permission requirement.write or test.write required")
		return
	}
	if strings.TrimSpace(req.Format) == "" {
		req.Format = string(export.PDF)
	}
	f, err := export.ParseFormat(req.Format)
	if err != nil || !slices.Contains(export.DocumentFormats, f) {
		s.fail(w, r, badRequest("format must be pdf, docx or md"))
		return
	}
	o := req.Options
	o.Lang = reportLang(r, p, o.Lang)
	if err := o.Normalize(kind); err != nil {
		s.fail(w, r, reportError(err))
		return
	}
	if o.Author == "" {
		o.Author = p.User.DisplayName
		if o.Author == "" {
			o.Author = p.User.Email
		}
	}
	start := time.Now()
	d, err := s.documentData(r, p, pr, kind, &o)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if o.Version == "" {
		o.Version = report.NextVersion(d.Revisions)
	}
	version := o.Version
	doc, err := report.BuildDocument(kind, d, o)
	if err != nil {
		s.fail(w, r, reportError(err))
		return
	}
	doc.Link = s.itemLink(pr)
	var buf bytes.Buffer
	if err := export.WriteDocument(&buf, f, doc); err != nil {
		s.fail(w, r, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("%s-%s-v%s", pr.Key, report.DocTag(kind), version)
	}
	if !strings.HasSuffix(strings.ToLower(name), "."+string(f)) {
		name += "." + string(f)
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = doc.Title + " · " + report.T(o.Lang, "Version") + " " + version
		if d.Baseline != nil {
			desc += " · " + report.T(o.Lang, "Baseline") + " " + d.Baseline.Name
		}
	}
	tags := []string{generatedTag, report.DocTag(kind)}
	if vt := versionTag(version); vt != "" {
		tags = append(tags, vt)
	}
	fi := &model.FileInfo{ProjectID: &pr.ID, Kind: model.FileDocument, Name: store.CleanFileName(name), ContentType: f.ContentType(),
		Description: desc, Tags: tags, UploadedBy: p.User.Email}
	if err := s.Store.CreateFile(r.Context(), p.TenantID(), fi, buf.Bytes()); err != nil {
		s.fail(w, r, err)
		return
	}
	details := fmt.Sprintf("%s %s %s (%d requirements, %d tests, %s)", kind, f, fi.Name, len(d.Requirements), len(d.Tests), time.Since(start).Round(time.Millisecond))
	if d.Baseline != nil {
		details += " baseline " + d.Baseline.Name
	}
	s.audit(r.Context(), p, "document.generate", "file", fi.ID, details)
	writeJSON(w, http.StatusCreated, fi)
}

// versionTag is the archive tag of a document version ("v2.0"); "" when the
// version is not a plain version number.
func versionTag(v string) string {
	v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	if !versionRe.MatchString(v) {
		return ""
	}
	return "v" + v
}

var versionRe = regexp.MustCompile(`^\d{1,6}(\.\d{1,6}){0,3}$`)

// inactive reports whether an item is left out of documents by default.
func inactiveRequirement(status string) bool { return status == "rejected" || status == "obsolete" }
func inactiveTest(status string) bool        { return status == "deprecated" }

// documentData loads what a generated document of kind needs: the
// requirements and tests (current or from a baseline, filtered), their
// links and, for a test plan, releases, runs, configurations, members and
// the execution settings.
func (s *Server) documentData(r *http.Request, p *Principal, pr *model.Project, kind string, o *report.DocOptions) (*report.DocData, error) {
	ctx := r.Context()
	tid := p.TenantID()
	plan := kind == report.DocTestPlan
	schema := requirementSchema
	if plan {
		schema = testSchema
	}
	q, err := query.Parse(o.Query, schema)
	if err != nil {
		return nil, err
	}
	pc, err := s.loadContext(ctx, tid, pr)
	if err != nil {
		return nil, err
	}
	d := &report.DocData{Project: *pr, Folders: pc.folders, Categories: pc.categories, LastResults: pc.last, Prerequisites: pc.prereqKeys,
		AllTests: map[string]model.TestRef{}, Now: time.Now().UTC()}

	var reqs []model.Requirement
	var tests []model.TestCase
	if o.BaselineID != "" {
		b, err := s.Store.GetBaseline(ctx, tid, o.BaselineID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && b.ProjectID != pr.ID) {
			return nil, badRequest("baseline %q does not belong to project %s", o.BaselineID, pr.Key)
		}
		if err != nil {
			return nil, err
		}
		for _, it := range b.Items {
			switch it.EntityType {
			case model.KindRequirement:
				var rq model.Requirement
				if err := json.Unmarshal(it.Snapshot, &rq); err != nil {
					return nil, err
				}
				reqs = append(reqs, rq)
			case model.KindTest:
				var t model.TestCase
				if err := json.Unmarshal(it.Snapshot, &t); err != nil {
					return nil, err
				}
				tests = append(tests, t)
			}
		}
		if d.Signoffs, err = s.Store.ListSignoffs(ctx, tid, b.ID); err != nil {
			return nil, err
		}
		b.Items = nil
		d.Baseline = b
	} else {
		if reqs, err = s.Store.ListRequirements(ctx, tid, pr.ID); err != nil {
			return nil, err
		}
		// a specification only needs the keys of the linked tests (in the links)
		if plan {
			if tests, err = s.Store.ListTests(ctx, tid, pr.ID); err != nil {
				return nil, err
			}
		}
	}

	// the items of the document (and the ends links may point to)
	inReq, inTest := map[string]bool{}, map[string]bool{}
	for i := range reqs {
		inReq[reqs[i].ID] = true
	}
	for i := range tests {
		inTest[tests[i].ID] = true
		d.AllTests[tests[i].ID] = model.TestRef{ID: tests[i].ID, Key: tests[i].Key, Title: tests[i].Title}
	}
	for _, l := range pc.links {
		// a baseline has its own items; current links between them count
		if d.Baseline != nil && (!inReq[l.RequirementID] || !inTest[l.TestCaseID]) {
			continue
		}
		d.Links = append(d.Links, l)
	}

	if d.Baseline == nil {
		// the verification of the current evidence (not for a baseline)
		d.Verification = pc.verification()
	}
	pc.indexCalls(tests)
	for i := range reqs {
		rq := &reqs[i]
		if !o.IncludeInactive && inactiveRequirement(rq.Status) {
			continue
		}
		pc.decorateRequirement(rq)
		if !plan && !q.Match(pc.requirementRecord(rq)) {
			continue
		}
		d.Requirements = append(d.Requirements, *rq)
	}
	if plan {
		for i := range tests {
			t := &tests[i]
			if !o.IncludeInactive && inactiveTest(t.Status) {
				continue
			}
			pc.decorateTest(t)
			if !q.Match(pc.testRecord(t)) {
				continue
			}
			d.Tests = append(d.Tests, *t)
		}
		st, err := s.Store.TenantSettings(ctx, tid)
		if err != nil {
			return nil, err
		}
		d.Settings = *st
		if d.Releases, err = s.Store.ListReleases(ctx, tid, pr.ID); err != nil {
			return nil, err
		}
		if d.Runs, err = s.Store.ListRuns(ctx, tid, pr.ID); err != nil {
			return nil, err
		}
		if d.Configurations, err = s.Store.ListConfigurations(ctx, tid, pr.ID); err != nil {
			return nil, err
		}
		if d.Members, err = s.Store.ListProjectMembers(ctx, tid, pr.ID); err != nil {
			return nil, err
		}
	}

	// earlier versions in the archive: the revision history
	files, err := s.Store.ListFiles(ctx, tid, pr.ID, model.FileDocument)
	if err != nil {
		return nil, err
	}
	tag := report.DocTag(kind)
	for _, f := range files {
		if !slices.Contains(f.Tags, generatedTag) || !slices.Contains(f.Tags, tag) {
			continue
		}
		rev := report.Revision{Date: f.CreatedAt, Author: f.UploadedBy, Name: f.Name}
		for _, t := range f.Tags {
			if versionTag(t) != "" && strings.HasPrefix(t, "v") {
				rev.Version = strings.TrimPrefix(t, "v")
			}
		}
		d.Revisions = append(d.Revisions, rev)
	}
	sort.SliceStable(d.Revisions, func(i, j int) bool { return d.Revisions[i].Date.Before(d.Revisions[j].Date) })
	return d, nil
}
