package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
)

// Two-way sync of requirements (internal/api/reqsync.go): reading single
// issues or work items again, writing fields back, creating issues and
// work items, comments and links. The values the caller writes are
// Markdown or text; they are converted here to what the tracker stores
// (Jira wiki markup of the REST API v2, HTML in Azure DevOps).
//
// Optimistic concurrency on the tracker's side: Azure DevOps updates carry
// a "test" of the revision read (the update fails if the work item changed
// meanwhile); Jira has no such precondition, so the issue's "updated" time
// is read again right before the update.

// ErrRemoteChanged: the item changed in the tracker after it was read.
var ErrRemoteChanged = errors.New("the item was changed in the tracker meanwhile")

// ErrRemoteMissing: the item no longer exists (or the token cannot see it).
var ErrRemoteMissing = errors.New("the item no longer exists in the tracker")

// Write kinds of a field.
const (
	WriteText     = "text"     // plain text
	WriteMarkdown = "markdown" // rich text (Markdown in, the tracker's format out)
	WriteList     = "list"     // labels or tags
	WriteOption   = "option"   // a value chosen by name (Jira priority) or number (Azure priority)
	WriteStatus   = "status"   // the workflow status (Jira transition, Azure System.State)
)

// FieldWrite is the new value of a tracker field.
type FieldWrite struct {
	ID    string
	Kind  string
	Value string
}

// RemoteComment is a comment of an issue or work item.
type RemoteComment struct {
	ID     string
	Author string
	// Body as Markdown.
	Body    string
	Created time.Time
}

// SyncClient reads and writes the items of a Jira or Azure DevOps project.
type SyncClient interface {
	System() string
	// Search runs a JQL or WIQL query.
	Search(ctx context.Context, query string, max int) (*SourceResult, error)
	// Fetch reads items by key; missing items are left out.
	Fetch(ctx context.Context, keys []string) ([]SourceItem, error)
	// Update writes fields of an item last read at version.
	Update(ctx context.Context, key, version string, fields []FieldWrite) error
	// Create creates an item of the type typ ("" = the default) and returns its key.
	Create(ctx context.Context, typ string, fields []FieldWrite) (string, error)
	Comments(ctx context.Context, key string) ([]RemoteComment, error)
	// AddComment posts a Markdown comment and returns its ID.
	AddComment(ctx context.Context, key, body string) (string, error)
	// AddLink links two items as the relation l.Type (Source relation Target).
	AddLink(ctx context.Context, l SourceLink) error
}

// NewSyncJira returns the sync client of a Jira project (projectKey and
// issueType are those of new issues).
func NewSyncJira(t *model.DefectTracker, hc *http.Client, projectKey, issueType string) SyncClient {
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	if issueType == "" {
		issueType = "Story"
	}
	return &jiraSync{j: &jira{cfg: t, hc: hc}, base: strings.TrimRight(t.URL, "/"), project: projectKey, issueType: issueType}
}

// NewSyncAzure returns the sync client of an Azure DevOps project
// (workItemType: of new work items).
func NewSyncAzure(cfg *model.AzureDevOpsSource, hc *http.Client, workItemType string) SyncClient {
	if workItemType == "" {
		workItemType = "User Story"
	}
	return &azureSync{a: NewAzure(cfg, hc), typ: workItemType}
}

// ---------------------------------------------------------------- Jira

type jiraSync struct {
	j         *jira
	base      string
	project   string
	issueType string
}

func (s *jiraSync) System() string { return SourceJira }

func (s *jiraSync) Search(ctx context.Context, query string, max int) (*SourceResult, error) {
	return SearchJira(ctx, s.j.cfg, s.j.hc, query, max)
}

type jiraOne struct {
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
	Schema map[string]jiraSchema      `json:"schema"`
}

func (s *jiraSync) get(ctx context.Context, key string) (*SourceItem, error) {
	key, err := jiraKey(key)
	if err != nil {
		return nil, err
	}
	var one jiraOne
	err = do(ctx, s.j.hc, http.MethodGet, s.base+"/rest/api/2/issue/"+key+"?fields=*all&expand=names,schema", nil, s.j.auth, &one)
	var te *Error
	if errors.As(err, &te) && te.Status == http.StatusNotFound {
		return nil, ErrRemoteMissing
	}
	if err != nil {
		return nil, err
	}
	it := jiraItem(s.base, one.Key, one.Fields, one.Schema)
	return &it, nil
}

func (s *jiraSync) Fetch(ctx context.Context, keys []string) ([]SourceItem, error) {
	var out []SourceItem
	for _, k := range keys {
		it, err := s.get(ctx, k)
		if errors.Is(err, ErrRemoteMissing) || errors.Is(err, ErrBadKey) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, nil
}

// jiraValue is the JSON of a field write (status is written by a transition).
func jiraValue(f FieldWrite) any {
	switch f.Kind {
	case WriteMarkdown:
		return MarkdownJira(f.Value)
	case WriteList:
		return jiraLabels(splitLabels(f.Value))
	case WriteOption:
		if f.Value == "" {
			return nil
		}
		return map[string]string{"name": f.Value}
	}
	return f.Value
}

func (s *jiraSync) Update(ctx context.Context, key, version string, fields []FieldWrite) error {
	cur, err := s.get(ctx, key)
	if err != nil {
		return err
	}
	if version != "" && cur.Version != version {
		return ErrRemoteChanged
	}
	set := map[string]any{}
	status := ""
	for _, f := range fields {
		if f.Kind == WriteStatus {
			status = f.Value
			continue
		}
		set[f.ID] = jiraValue(f)
	}
	if len(set) > 0 {
		if err := do(ctx, s.j.hc, http.MethodPut, s.base+"/rest/api/2/issue/"+cur.Key, map[string]any{"fields": set}, s.j.auth, nil); err != nil {
			return err
		}
	}
	if status != "" {
		return s.transition(ctx, cur.Key, status)
	}
	return nil
}

// transition moves an issue to the status named to (a no-op when it is
// there already).
func (s *jiraSync) transition(ctx context.Context, key, to string) error {
	var tr struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := do(ctx, s.j.hc, http.MethodGet, s.base+"/rest/api/2/issue/"+key+"/transitions", nil, s.j.auth, &tr); err != nil {
		return err
	}
	var names []string
	for _, t := range tr.Transitions {
		if strings.EqualFold(t.To.Name, to) || strings.EqualFold(t.Name, to) {
			return do(ctx, s.j.hc, http.MethodPost, s.base+"/rest/api/2/issue/"+key+"/transitions", map[string]any{"transition": map[string]string{"id": t.ID}}, s.j.auth, nil)
		}
		names = append(names, t.To.Name)
	}
	return fmt.Errorf("%w: Jira offers no transition of %s to the status %q (available: %s)", ErrConfig, key, to, strings.Join(names, ", "))
}

func (s *jiraSync) Create(ctx context.Context, typ string, fields []FieldWrite) (string, error) {
	if s.project == "" {
		return "", fmt.Errorf("%w: no Jira project key for new issues", ErrConfig)
	}
	if typ == "" {
		typ = s.issueType
	}
	set := map[string]any{"project": map[string]string{"key": s.project}, "issuetype": map[string]string{"name": typ}}
	status := ""
	for _, f := range fields {
		if f.Kind == WriteStatus {
			status = f.Value
			continue
		}
		if v := jiraValue(f); v != nil {
			set[f.ID] = v
		}
	}
	var created struct {
		Key string `json:"key"`
	}
	if err := do(ctx, s.j.hc, http.MethodPost, s.base+"/rest/api/2/issue", map[string]any{"fields": set}, s.j.auth, &created); err != nil {
		return "", err
	}
	if status != "" {
		cur, err := s.get(ctx, created.Key)
		if err == nil && !strings.EqualFold(cur.Values["status"], status) {
			err = s.transition(ctx, created.Key, status)
		}
		if err != nil {
			return created.Key, fmt.Errorf("created %s, but its status: %w", created.Key, err)
		}
	}
	return created.Key, nil
}

func (s *jiraSync) Comments(ctx context.Context, key string) ([]RemoteComment, error) {
	key, err := jiraKey(key)
	if err != nil {
		return nil, err
	}
	var out []RemoteComment
	for start := 0; ; {
		var page struct {
			Comments []struct {
				ID     string `json:"id"`
				Body   string `json:"body"`
				Author struct {
					DisplayName string `json:"displayName"`
				} `json:"author"`
				Created string `json:"created"`
			} `json:"comments"`
			Total int `json:"total"`
		}
		if err := do(ctx, s.j.hc, http.MethodGet, s.base+"/rest/api/2/issue/"+key+"/comment?startAt="+strconv.Itoa(start)+"&maxResults=100", nil, s.j.auth, &page); err != nil {
			return nil, err
		}
		for _, c := range page.Comments {
			out = append(out, RemoteComment{ID: c.ID, Author: c.Author.DisplayName, Body: JiraMarkdown(c.Body), Created: jiraTime(c.Created)})
		}
		start += len(page.Comments)
		if len(page.Comments) == 0 || start >= page.Total || start >= 1000 {
			return out, nil
		}
	}
}

// jiraTime parses the times of the Jira REST API (2024-05-01T10:00:00.000+0200).
func jiraTime(s string) time.Time {
	for _, f := range []string{"2006-01-02T15:04:05.000-0700", time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func (s *jiraSync) AddComment(ctx context.Context, key, body string) (string, error) {
	key, err := jiraKey(key)
	if err != nil {
		return "", err
	}
	var c struct {
		ID string `json:"id"`
	}
	if err := do(ctx, s.j.hc, http.MethodPost, s.base+"/rest/api/2/issue/"+key+"/comment", map[string]string{"body": MarkdownJira(body)}, s.j.auth, &c); err != nil {
		return "", err
	}
	return c.ID, nil
}

// JiraLinkType is the Jira issue link type of a relation and whether the
// relation's source is the inward issue of the link (A blocks B: B
// depends on A). "" for relations Jira has no link type for.
func JiraLinkType(relation string) (name string, reverse bool) {
	switch relation {
	case "relates_to":
		return "Relates", false
	case "duplicates":
		return "Duplicate", false
	case "derived_from":
		return "Cloners", false
	case "depends_on":
		return "Blocks", true
	}
	return "", false
}

func (s *jiraSync) AddLink(ctx context.Context, l SourceLink) error {
	name, reverse := JiraLinkType(l.Type)
	if name == "" {
		return fmt.Errorf("%w: Jira has no link type for the relation %s", ErrConfig, l.Type)
	}
	from, to := l.Source, l.Target
	if reverse {
		from, to = to, from
	}
	// the issue named inwardIssue gets the outward description ("from blocks to")
	body := map[string]any{"type": map[string]string{"name": name}, "inwardIssue": map[string]string{"key": from}, "outwardIssue": map[string]string{"key": to}}
	return do(ctx, s.j.hc, http.MethodPost, s.base+"/rest/api/2/issueLink", body, s.j.auth, nil)
}

// ---------------------------------------------------------------- Azure DevOps

type azureSync struct {
	a   *Azure
	typ string
}

func (s *azureSync) System() string { return SourceAzure }

func (s *azureSync) Search(ctx context.Context, query string, max int) (*SourceResult, error) {
	return s.a.Search(ctx, query, max)
}

func (s *azureSync) Fetch(ctx context.Context, keys []string) ([]SourceItem, error) {
	var ids []int
	for _, k := range keys {
		if n, err := strconv.Atoi(strings.TrimSpace(k)); err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	res, err := s.a.read(ctx, ids, &SourceResult{System: SourceAzure, Base: s.a.org + "/" + s.a.project})
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

func (s *azureSync) itemURL(id string) string {
	return s.a.org + "/" + s.a.project + "/_apis/wit/workitems/" + id + "?" + azureAPI
}

// azureValue is the JSON value of a field write.
func azureValue(f FieldWrite) any {
	switch f.Kind {
	case WriteMarkdown:
		return markdownHTML(f.Value)
	case WriteList:
		return strings.Join(splitLabels(f.Value), "; ")
	case WriteOption:
		if n, err := strconv.Atoi(f.Value); err == nil {
			return n
		}
	}
	return f.Value
}

type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

func (s *azureSync) patch(ctx context.Context, method, u string, ops []patchOp, out any) error {
	return doContent(ctx, s.a.hc, method, u, "application/json-patch+json", ops, s.a.auth, out)
}

func (s *azureSync) Update(ctx context.Context, key, version string, fields []FieldWrite) error {
	if _, err := strconv.Atoi(key); err != nil {
		return fmt.Errorf("%w: an Azure DevOps work item is referred to by its number", ErrBadKey)
	}
	var ops []patchOp
	if version != "" {
		if n, err := strconv.Atoi(version); err == nil {
			ops = append(ops, patchOp{Op: "test", Path: "/rev", Value: n})
		}
	}
	for _, f := range fields {
		ops = append(ops, patchOp{Op: "add", Path: "/fields/" + f.ID, Value: azureValue(f)})
	}
	err := s.patch(ctx, http.MethodPatch, s.itemURL(key), ops, nil)
	return azureWriteErr(err)
}

// azureWriteErr maps a failed test of the revision and a missing work item.
func azureWriteErr(err error) error {
	var te *Error
	if errors.As(err, &te) {
		switch {
		case te.Status == http.StatusConflict || te.Status == http.StatusPreconditionFailed || strings.Contains(te.Body, "TF401289"):
			return ErrRemoteChanged
		case te.Status == http.StatusNotFound:
			return ErrRemoteMissing
		}
	}
	return err
}

func (s *azureSync) Create(ctx context.Context, typ string, fields []FieldWrite) (string, error) {
	if typ == "" {
		typ = s.typ
	}
	var ops []patchOp
	status := ""
	for _, f := range fields {
		if f.Kind == WriteStatus {
			// a new work item starts in the first state of its type
			status = f.Value
			continue
		}
		ops = append(ops, patchOp{Op: "add", Path: "/fields/" + f.ID, Value: azureValue(f)})
	}
	var created struct {
		ID  int `json:"id"`
		Rev int `json:"rev"`
	}
	u := s.a.org + "/" + s.a.project + "/_apis/wit/workitems/$" + url.PathEscape(typ) + "?" + azureAPI
	if err := s.patch(ctx, http.MethodPost, u, ops, &created); err != nil {
		return "", err
	}
	key := strconv.Itoa(created.ID)
	if status != "" {
		if err := s.Update(ctx, key, "", []FieldWrite{{ID: "System.State", Kind: WriteStatus, Value: status}}); err != nil {
			return key, fmt.Errorf("created %s, but its state: %w", key, err)
		}
	}
	return key, nil
}

const azureCommentsAPI = "api-version=7.1-preview.4"

func (s *azureSync) commentsURL(key string) string {
	return s.a.org + "/" + s.a.project + "/_apis/wit/workItems/" + key + "/comments?" + azureCommentsAPI
}

func (s *azureSync) Comments(ctx context.Context, key string) ([]RemoteComment, error) {
	if _, err := strconv.Atoi(key); err != nil {
		return nil, ErrBadKey
	}
	var page struct {
		Comments []struct {
			ID        int    `json:"id"`
			Text      string `json:"text"`
			CreatedBy struct {
				DisplayName string `json:"displayName"`
			} `json:"createdBy"`
			CreatedDate time.Time `json:"createdDate"`
		} `json:"comments"`
	}
	if err := do(ctx, s.a.hc, http.MethodGet, s.commentsURL(key)+"&$top=200", nil, s.a.auth, &page); err != nil {
		return nil, err
	}
	out := []RemoteComment{}
	for _, c := range page.Comments {
		out = append(out, RemoteComment{ID: strconv.Itoa(c.ID), Author: c.CreatedBy.DisplayName, Body: htmlMarkdown(c.Text), Created: c.CreatedDate.UTC()})
	}
	return out, nil
}

func (s *azureSync) AddComment(ctx context.Context, key, body string) (string, error) {
	if _, err := strconv.Atoi(key); err != nil {
		return "", ErrBadKey
	}
	var c struct {
		ID int `json:"id"`
	}
	if err := do(ctx, s.a.hc, http.MethodPost, s.commentsURL(key), map[string]string{"text": markdownHTML(body)}, s.a.auth, &c); err != nil {
		return "", err
	}
	return strconv.Itoa(c.ID), nil
}

// AzureLinkType is the relation type (rel) of a work item link that makes
// the relation from the source's side; "" when Azure DevOps has none.
func AzureLinkType(relation string) string {
	switch relation {
	case "relates_to":
		return "System.LinkTypes.Related"
	case "depends_on":
		return "System.LinkTypes.Dependency-Reverse"
	case "duplicates":
		return "System.LinkTypes.Duplicate-Reverse"
	case "refines":
		return "System.LinkTypes.Hierarchy-Reverse"
	}
	return ""
}

func (s *azureSync) AddLink(ctx context.Context, l SourceLink) error {
	rel := AzureLinkType(l.Type)
	if rel == "" {
		return fmt.Errorf("%w: Azure DevOps has no link type for the relation %s", ErrConfig, l.Type)
	}
	if _, err := strconv.Atoi(l.Target); err != nil {
		return ErrBadKey
	}
	ops := []patchOp{{Op: "add", Path: "/relations/-", Value: map[string]any{"rel": rel, "url": s.a.org + "/_apis/wit/workItems/" + l.Target}}}
	return azureWriteErr(s.patch(ctx, http.MethodPatch, s.itemURL(l.Source), ops, nil))
}

// ---------------------------------------------------------------- formats

var (
	mdHeadingRe = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	mdBulletRe  = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	mdNumberRe  = regexp.MustCompile(`^(\s*)\d+[.)]\s+(.*)$`)
	mdBoldRe    = regexp.MustCompile(`\*\*([^*\n]+)\*\*|__([^_\n]+)__`)
	mdItalicRe  = regexp.MustCompile(`(^|[\s(])[*_]([^*_\s][^*_\n]*)[*_]`)
	mdCodeRe    = regexp.MustCompile("`([^`\n]+)`")
	mdLinkRe    = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)
	mdStrikeRe  = regexp.MustCompile(`~~([^~\n]+)~~`)
)

// MarkdownJira converts Markdown to Jira wiki markup (the inverse of
// JiraMarkdown for what requirements use: headings, lists, quotes, code
// blocks, bold, italic, strikethrough, monospace and links).
func MarkdownJira(md string) string {
	var out []string
	inCode := false
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			if inCode {
				out = append(out, "{code}")
			} else if lang := strings.TrimSpace(strings.TrimPrefix(t, "```")); lang != "" {
				out = append(out, "{code:"+lang+"}")
			} else {
				out = append(out, "{code}")
			}
			inCode = !inCode
			continue
		}
		if inCode {
			out = append(out, line)
			continue
		}
		if m := mdHeadingRe.FindStringSubmatch(t); m != nil {
			out = append(out, "h"+strconv.Itoa(len(m[1]))+". "+jiraInlineOut(m[2]))
			continue
		}
		if m := mdBulletRe.FindStringSubmatch(line); m != nil {
			out = append(out, strings.Repeat("*", 1+len(m[1])/2)+" "+jiraInlineOut(m[2]))
			continue
		}
		if m := mdNumberRe.FindStringSubmatch(line); m != nil {
			out = append(out, strings.Repeat("#", 1+len(m[1])/2)+" "+jiraInlineOut(m[2]))
			continue
		}
		if q, ok := strings.CutPrefix(t, ">"); ok {
			out = append(out, "bq. "+jiraInlineOut(strings.TrimSpace(q)))
			continue
		}
		out = append(out, jiraInlineOut(line))
	}
	if inCode {
		out = append(out, "{code}")
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func jiraInlineOut(s string) string {
	// monospace first, so its content is not formatted
	var codes []string
	s = mdCodeRe.ReplaceAllStringFunc(s, func(m string) string {
		codes = append(codes, "{{"+mdCodeRe.FindStringSubmatch(m)[1]+"}}")
		return "\x00" + strconv.Itoa(len(codes)-1) + "\x00"
	})
	s = mdLinkRe.ReplaceAllString(s, "[$1|$2]")
	s = mdBoldRe.ReplaceAllStringFunc(s, func(m string) string {
		x := mdBoldRe.FindStringSubmatch(m)
		return "\x01" + x[1] + x[2] + "\x01"
	})
	s = mdItalicRe.ReplaceAllString(s, "${1}_${2}_")
	s = strings.ReplaceAll(s, "\x01", "*")
	s = mdStrikeRe.ReplaceAllString(s, "-$1-")
	for i, c := range codes {
		s = strings.Replace(s, "\x00"+strconv.Itoa(i)+"\x00", c, 1)
	}
	return s
}

// markdownHTML is the HTML of Markdown (the XHTML of the ReqIF export
// without its namespace prefix).
func markdownHTML(md string) string {
	h := export.MarkdownToXHTML(md, nil)
	return strings.ReplaceAll(strings.ReplaceAll(h, "<xhtml:", "<"), "</xhtml:", "</")
}

// htmlMarkdown is the Markdown of an HTML field or comment.
func htmlMarkdown(s string) string {
	return strings.TrimSpace(reqimport.HTMLMarkdown(s))
}

// ---------------------------------------------------------------- webhooks

// JiraHookKeys returns the issue keys a Jira webhook event is about
// (jira:issue_created, jira:issue_updated, comment_created, issuelink_created, …).
func JiraHookKeys(body []byte) (event string, keys []string) {
	var ev struct {
		Event string `json:"webhookEvent"`
		Issue struct {
			Key string `json:"key"`
		} `json:"issue"`
	}
	if json.Unmarshal(body, &ev) != nil {
		return "", nil
	}
	if ev.Issue.Key != "" {
		keys = append(keys, ev.Issue.Key)
	}
	return ev.Event, keys
}

// AzureHookKeys returns the work item IDs an Azure DevOps service hook
// event is about (workitem.created, workitem.updated, workitem.commented, …).
func AzureHookKeys(body []byte) (event string, keys []string) {
	var ev struct {
		Event    string `json:"eventType"`
		Resource struct {
			ID         json.Number `json:"id"`
			WorkItemID int         `json:"workItemId"`
			Revision   struct {
				ID int `json:"id"`
			} `json:"revision"`
		} `json:"resource"`
	}
	if json.Unmarshal(body, &ev) != nil {
		return "", nil
	}
	id := ev.Resource.WorkItemID
	if id == 0 {
		id = ev.Resource.Revision.ID
	}
	if id == 0 && strings.HasPrefix(ev.Event, "workitem.") && ev.Event != "workitem.updated" {
		n, _ := ev.Resource.ID.Int64()
		id = int(n)
	}
	if id > 0 {
		keys = append(keys, strconv.Itoa(id))
	}
	sort.Strings(keys)
	return ev.Event, keys
}
