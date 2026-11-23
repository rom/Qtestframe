// Package trackerfake holds fake external trackers for tests: a Request
// Tracker REST2 API, the issue search of Jira and the work item API of
// Azure DevOps. They keep their data in memory and check the credentials
// (the tokens below are test values, not real ones).
package trackerfake

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Test credentials.
const (
	RTToken    = "rt-test"
	JiraUser   = "qa@example.com"
	JiraToken  = "jira-test"
	AzureToken = "ado-test"
)

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------- RT

// RT is Request Tracker below /rt with the queue "QA" (ID 3).
type RT struct {
	mu       sync.Mutex
	Tickets  map[int]map[string]any
	Comments map[int][]string
	// Last is the body of the last write.
	Last map[string]any
	next int
}

// NewRT returns an empty RT whose first ticket is #101.
func NewRT() *RT {
	return &RT{Tickets: map[int]map[string]any{}, Comments: map[int][]string{}, next: 100}
}

// SetStatus changes a ticket's status.
func (f *RT) SetStatus(id int, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Tickets[id]["Status"] = status
}

func (f *RT) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "token "+RTToken {
		write(w, 401, map[string]any{"message": "Unauthorized"})
		return
	}
	p, ok := strings.CutPrefix(r.URL.Path, "/rt/REST/2.0/")
	if !ok {
		write(w, 404, map[string]any{"message": "Not found"})
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body != nil {
		f.Last = body
	}
	parts := strings.Split(p, "/")
	switch {
	case parts[0] == "queue" && len(parts) == 2 && r.Method == http.MethodGet:
		if parts[1] == "QA" || parts[1] == "3" {
			write(w, 200, map[string]any{"id": 3, "Name": "QA"})
			return
		}
		write(w, 404, map[string]any{"message": "Resource does not exist"})
	case p == "ticket" && r.Method == http.MethodPost:
		if body["Queue"] != "QA" {
			write(w, 400, map[string]any{"message": "Invalid queue"})
			return
		}
		f.next++
		id := f.next
		f.Tickets[id] = map[string]any{"id": id, "Subject": body["Subject"], "Status": "new", "Content": body["Content"], "CustomFields": body["CustomFields"]}
		write(w, 201, map[string]any{"id": id, "type": "ticket", "_url": "/REST/2.0/ticket/" + strconv.Itoa(id)})
	case parts[0] == "ticket" && len(parts) >= 2:
		id, _ := strconv.Atoi(parts[1])
		t, ok := f.Tickets[id]
		if !ok {
			write(w, 404, map[string]any{"message": "Resource does not exist"})
			return
		}
		switch {
		case len(parts) == 2 && r.Method == http.MethodGet:
			write(w, 200, t)
		case len(parts) == 2 && r.Method == http.MethodPut:
			if s, ok := body["Subject"]; ok {
				t["Subject"] = s
			}
			write(w, 200, []string{"Ticket " + parts[1] + ": Subject changed"})
		case len(parts) == 3 && parts[2] == "comment" && r.Method == http.MethodPost:
			c, _ := body["Content"].(string)
			f.Comments[id] = append(f.Comments[id], c)
			write(w, 201, []string{"Comments added"})
		default:
			write(w, 405, map[string]any{"message": "Method not allowed"})
		}
	default:
		write(w, 404, map[string]any{"message": "Not found"})
	}
}

// ---------------------------------------------------------------- Jira

// Jira answers the issue search of Jira Cloud (or, with Classic, only the
// classic search of Jira Server) and the project check of project SHOP.
// The JQL is not evaluated: "project = X" selects the issues whose key
// starts with X-, anything else all issues. For the two-way sync it also
// reads and updates single issues (each write sets "updated"), moves them
// through the transitions of Workflow, creates issues, keeps comments and
// creates issue links.
type Jira struct {
	mu sync.Mutex
	// Comments by issue key.
	Comments map[string][]map[string]any
	// Workflow: the statuses (name → category) every status can move to.
	Workflow map[string]string
	// Writes counts the PUT and POST requests (other than searches).
	Writes int
	// FailUpdate answers every issue update with this status (0 none).
	FailUpdate int
	clock      int
	// Issues by key: the "fields" of each issue.
	Issues map[string]map[string]any
	// Names and Schema are the field names and schemas (expand).
	Names  map[string]string
	Schema map[string]map[string]string
	// Classic: no /search/jql (Jira Server and Data Center).
	Classic bool
	// PageSize caps the page (default 2, to exercise paging).
	PageSize int
	// Requests counts the search requests.
	Requests int
}

// NewJira returns a Jira with standard field names.
func NewJira() *Jira {
	return &Jira{Issues: map[string]map[string]any{}, PageSize: 2, Comments: map[string][]map[string]any{},
		Workflow: map[string]string{"To Do": "new", "In Progress": "indeterminate", "Done": "done"},
		Names:    map[string]string{"summary": "Summary", "description": "Description", "priority": "Priority", "labels": "Labels", "status": "Status", "issuetype": "Issue Type"},
		Schema:   map[string]map[string]string{"labels": {"type": "array", "items": "string"}, "description": {"type": "string"}}}
}

func (f *Jira) authorized(r *http.Request) bool {
	u, p, ok := r.BasicAuth()
	return ok && u == JiraUser && p == JiraToken || r.Header.Get("Authorization") == "Bearer "+JiraToken
}

func (f *Jira) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.authorized(r) {
		write(w, 401, map[string]any{"errorMessages": []string{"You are not authenticated."}})
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/rest/api/2/project/"):
		if strings.TrimPrefix(r.URL.Path, "/rest/api/2/project/") == "SHOP" {
			write(w, 200, map[string]any{"key": "SHOP"})
			return
		}
		write(w, 404, map[string]any{"errorMessages": []string{"No project could be found"}})
	case r.URL.Path == "/rest/api/2/search/jql" && !f.Classic, r.URL.Path == "/rest/api/2/search":
		f.search(w, r)
	case r.URL.Path == "/rest/api/2/issue" && r.Method == http.MethodPost:
		f.create(w, r)
	case r.URL.Path == "/rest/api/2/issueLink" && r.Method == http.MethodPost:
		f.link(w, r)
	case strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/"):
		f.issue(w, r, strings.Split(strings.TrimPrefix(r.URL.Path, "/rest/api/2/issue/"), "/"))
	default:
		write(w, 404, map[string]any{"errorMessages": []string{"Not found"}})
	}
}

// Touch marks an issue as changed (a new "updated" time).
func (f *Jira) Touch(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touch(key)
}

func (f *Jira) touch(key string) {
	f.clock++
	f.Issues[key]["updated"] = "2026-01-01T10:" + strconv.Itoa(10+f.clock/60) + ":" + strconv.Itoa(10+f.clock%50) + ".000+0000"
}

// Set changes a field of an issue (as an edit in Jira would).
func (f *Jira) Set(key, field string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Issues[key][field] = v
	f.touch(key)
}

// Field returns a field of an issue.
func (f *Jira) Field(key, field string) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Issues[key][field]
}

// AddComment adds a comment as a Jira user would.
func (f *Jira) AddComment(key, author, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addComment(key, author, body)
	f.touch(key)
}

func (f *Jira) addComment(key, author, body string) string {
	id := strconv.Itoa(10000 + len(f.Comments[key]) + 100*len(f.Comments))
	f.Comments[key] = append(f.Comments[key], map[string]any{"id": id, "body": body, "author": map[string]any{"displayName": author}, "created": "2030-01-01T10:00:00.000+0000"})
	return id
}

func (f *Jira) status(name string) map[string]any {
	return map[string]any{"name": name, "statusCategory": map[string]any{"key": f.Workflow[name]}}
}

func (f *Jira) create(w http.ResponseWriter, r *http.Request) {
	f.Writes++
	var body struct {
		Fields map[string]any `json:"fields"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	p, _ := body.Fields["project"].(map[string]any)
	if p == nil || p["key"] != "SHOP" {
		write(w, 400, map[string]any{"errors": map[string]any{"project": "valid project is required"}})
		return
	}
	if s, _ := body.Fields["summary"].(string); s == "" {
		write(w, 400, map[string]any{"errors": map[string]any{"summary": "You must specify a summary of the issue."}})
		return
	}
	n := 1
	for k := range f.Issues {
		if _, num, ok := strings.Cut(k, "-"); ok {
			if x, _ := strconv.Atoi(num); x >= n {
				n = x + 1
			}
		}
	}
	key := "SHOP-" + strconv.Itoa(n)
	fields := map[string]any{"status": f.status("To Do")}
	for k, v := range body.Fields {
		if k != "project" {
			fields[k] = v
		}
	}
	f.Issues[key] = fields
	f.touch(key)
	write(w, 201, map[string]any{"id": "1" + strconv.Itoa(n), "key": key})
}

func (f *Jira) link(w http.ResponseWriter, r *http.Request) {
	f.Writes++
	var body struct {
		Type struct {
			Name string `json:"name"`
		} `json:"type"`
		Inward struct {
			Key string `json:"key"`
		} `json:"inwardIssue"`
		Outward struct {
			Key string `json:"key"`
		} `json:"outwardIssue"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	from, to := f.Issues[body.Inward.Key], f.Issues[body.Outward.Key]
	if from == nil || to == nil {
		write(w, 404, map[string]any{"errorMessages": []string{"Issue does not exist"}})
		return
	}
	outward := strings.ToLower(body.Type.Name)
	switch body.Type.Name {
	case "Duplicate":
		outward = "duplicates"
	case "Cloners":
		outward = "clones"
	case "Relates":
		outward = "relates to"
	}
	id := strconv.Itoa(5000 + f.Writes)
	links, _ := from["issuelinks"].([]any)
	from["issuelinks"] = append(links, map[string]any{"id": id, "type": map[string]any{"name": body.Type.Name, "outward": outward}, "outwardIssue": map[string]any{"key": body.Outward.Key}})
	links, _ = to["issuelinks"].([]any)
	to["issuelinks"] = append(links, map[string]any{"id": id, "type": map[string]any{"name": body.Type.Name, "outward": outward}, "inwardIssue": map[string]any{"key": body.Inward.Key}})
	f.touch(body.Inward.Key)
	f.touch(body.Outward.Key)
	w.WriteHeader(201)
}

func (f *Jira) issue(w http.ResponseWriter, r *http.Request, parts []string) {
	key := parts[0]
	fields, ok := f.Issues[key]
	if !ok {
		write(w, 404, map[string]any{"errorMessages": []string{"Issue does not exist or you do not have permission to see it."}})
		return
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	switch {
	case sub == "" && r.Method == http.MethodGet:
		write(w, 200, map[string]any{"key": key, "fields": fields, "names": f.Names, "schema": f.Schema})
	case sub == "" && r.Method == http.MethodPut:
		f.Writes++
		if f.FailUpdate != 0 {
			write(w, f.FailUpdate, map[string]any{"errorMessages": []string{"update refused"}})
			return
		}
		var body struct {
			Fields map[string]any `json:"fields"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for k, v := range body.Fields {
			if k == "status" {
				write(w, 400, map[string]any{"errors": map[string]any{"status": "Field 'status' cannot be set. It is not on the appropriate screen, or unknown."}})
				return
			}
			fields[k] = v
		}
		f.touch(key)
		w.WriteHeader(204)
	case sub == "transitions" && r.Method == http.MethodGet:
		var list []map[string]any
		names := make([]string, 0, len(f.Workflow))
		for n := range f.Workflow {
			names = append(names, n)
		}
		sort.Strings(names)
		for i, n := range names {
			list = append(list, map[string]any{"id": strconv.Itoa(11 + i), "name": "Move to " + n, "to": map[string]any{"name": n}})
		}
		write(w, 200, map[string]any{"transitions": list})
	case sub == "transitions" && r.Method == http.MethodPost:
		f.Writes++
		var body struct {
			Transition struct {
				ID string `json:"id"`
			} `json:"transition"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		names := make([]string, 0, len(f.Workflow))
		for n := range f.Workflow {
			names = append(names, n)
		}
		sort.Strings(names)
		i, _ := strconv.Atoi(body.Transition.ID)
		if i-11 < 0 || i-11 >= len(names) {
			write(w, 400, map[string]any{"errorMessages": []string{"transition not valid"}})
			return
		}
		fields["status"] = f.status(names[i-11])
		f.touch(key)
		w.WriteHeader(204)
	case sub == "comment" && r.Method == http.MethodGet:
		list := f.Comments[key]
		if list == nil {
			list = []map[string]any{}
		}
		write(w, 200, map[string]any{"comments": list, "total": len(list), "startAt": 0})
	case sub == "comment" && r.Method == http.MethodPost:
		f.Writes++
		var body struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := f.addComment(key, "Sync Bot", body.Body)
		write(w, 201, map[string]any{"id": id})
	default:
		write(w, 405, map[string]any{"errorMessages": []string{"Method not allowed"}})
	}
}

func (f *Jira) search(w http.ResponseWriter, r *http.Request) {
	f.Requests++
	var body struct {
		JQL           string `json:"jql"`
		StartAt       int    `json:"startAt"`
		MaxResults    int    `json:"maxResults"`
		NextPageToken string `json:"nextPageToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if strings.Contains(body.JQL, "syntax error") {
		write(w, 400, map[string]any{"errorMessages": []string{"Error in the JQL Query"}})
		return
	}
	var keys []string
	prefix := ""
	if i := strings.Index(strings.ToLower(body.JQL), "project = "); i >= 0 {
		prefix = strings.Fields(body.JQL[i+len("project = "):])[0] + "-"
	}
	for k := range f.Issues {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keyLess(keys[i], keys[j]) })
	start := body.StartAt
	if r.URL.Path == "/rest/api/2/search/jql" {
		start, _ = strconv.Atoi(body.NextPageToken)
	}
	size := min(max(body.MaxResults, 1), f.PageSize)
	end := min(start+size, len(keys))
	var issues []map[string]any
	for _, k := range keys[min(start, len(keys)):end] {
		issues = append(issues, map[string]any{"key": k, "id": "1" + strings.TrimLeft(k, "ABCDEFGHIJKLMNOPQRSTUVWXYZ-"), "fields": f.Issues[k]})
	}
	out := map[string]any{"issues": issues, "names": f.Names, "schema": f.Schema}
	if r.URL.Path == "/rest/api/2/search/jql" {
		out["isLast"] = end >= len(keys)
		if end < len(keys) {
			out["nextPageToken"] = strconv.Itoa(end)
		}
	} else {
		out["startAt"], out["total"], out["maxResults"] = start, len(keys), size
	}
	write(w, 200, out)
}

func keyLess(a, b string) bool {
	pa, na, _ := strings.Cut(a, "-")
	pb, nb, _ := strings.Cut(b, "-")
	if pa != pb {
		return pa < pb
	}
	x, _ := strconv.Atoi(na)
	y, _ := strconv.Atoi(nb)
	return x < y
}

// ---------------------------------------------------------------- Azure DevOps

// Azure is the organization /org with the project "Shop". The WIQL is
// not evaluated beyond [System.WorkItemType] IN (…): every other query
// returns all work items in ID order.
type Azure struct {
	mu sync.Mutex
	// Items by ID: fields and relations ({"rel", "url", "attributes"}).
	Items     map[int]map[string]any
	Relations map[int][]map[string]any
	// Fields of the project (referenceName, name, type).
	Fields []map[string]string
	// Queries are the WIQL texts received.
	Queries []string
	// Revs by ID (default 1); every write adds one.
	Revs map[int]int
	// Comments by ID.
	Comments map[int][]map[string]any
	// Patches are the JSON patch documents received.
	Patches [][]map[string]any
	next    int
}

// Set changes a field of a work item (as an edit in Azure DevOps would).
func (f *Azure) Set(id int, field string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Items[id][field] = v
	f.Revs[id] = f.rev(id) + 1
}

// Field returns a field of a work item.
func (f *Azure) Field(id int, field string) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Items[id][field]
}

func (f *Azure) rev(id int) int {
	if r := f.Revs[id]; r > 0 {
		return r
	}
	return 1
}

// AddComment adds a comment as an Azure DevOps user would (the work
// item's revision does not change).
func (f *Azure) AddComment(id int, author, html string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addComment(id, author, html)
}

func (f *Azure) addComment(id int, author, html string) int {
	cid := 700 + len(f.Comments[id]) + 50*len(f.Comments)
	f.Comments[id] = append(f.Comments[id], map[string]any{"id": cid, "text": html, "createdBy": map[string]any{"displayName": author}, "createdDate": "2030-01-01T10:00:00Z"})
	return cid
}

// NewAzure returns an Azure DevOps project with the standard fields.
func NewAzure() *Azure {
	f := &Azure{Items: map[int]map[string]any{}, Relations: map[int][]map[string]any{}, Revs: map[int]int{}, Comments: map[int][]map[string]any{}}
	for _, x := range [][3]string{
		{"System.Title", "Title", "string"}, {"System.Description", "Description", "html"}, {"System.State", "State", "string"},
		{"System.Tags", "Tags", "plainText"}, {"System.WorkItemType", "Work Item Type", "string"}, {"System.AreaPath", "Area Path", "treePath"},
		{"Microsoft.VSTS.Common.Priority", "Priority", "integer"}, {"Microsoft.VSTS.Common.AcceptanceCriteria", "Acceptance Criteria", "html"},
		{"System.AssignedTo", "Assigned To", "identity"}, {"System.Rev", "Rev", "integer"},
	} {
		f.Fields = append(f.Fields, map[string]string{"referenceName": x[0], "name": x[1], "type": x[2]})
	}
	return f
}

// Link adds a relation of a work item to another.
func (f *Azure) Link(from int, rel string, to int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Relations[from] = append(f.Relations[from], map[string]any{"rel": rel, "url": "https://dev.azure.example/org/_apis/wit/workItems/" + strconv.Itoa(to), "attributes": map[string]any{"isLocked": false}})
}

func (f *Azure) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, p, ok := r.BasicAuth(); !ok || u != "" || p != AzureToken {
		w.WriteHeader(401) // Azure DevOps answers HTML sign-in pages here
		_, _ = w.Write([]byte("<html>Sign in</html>"))
		return
	}
	if v := r.URL.Query().Get("api-version"); v != "7.1" && !(v == "7.1-preview.4" && strings.Contains(r.URL.Path, "/comments")) {
		write(w, 400, map[string]any{"message": "api-version missing"})
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/org/Shop/_apis/wit/workItems/") && strings.HasSuffix(r.URL.Path, "/comments"):
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/org/Shop/_apis/wit/workItems/"), "/comments"))
		if f.Items[id] == nil {
			write(w, 404, map[string]any{"message": "TF401232: Work item does not exist."})
			return
		}
		if r.Method == http.MethodPost {
			var body struct {
				Text string `json:"text"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			write(w, 200, map[string]any{"id": f.addComment(id, "Sync Bot", body.Text)})
			return
		}
		list := f.Comments[id]
		if list == nil {
			list = []map[string]any{}
		}
		write(w, 200, map[string]any{"totalCount": len(list), "comments": list})
	case strings.HasPrefix(r.URL.Path, "/org/Shop/_apis/wit/workitems/$") && r.Method == http.MethodPost:
		f.createItem(w, r, strings.TrimPrefix(r.URL.Path, "/org/Shop/_apis/wit/workitems/$"))
	case strings.HasPrefix(r.URL.Path, "/org/Shop/_apis/wit/workitems/") && r.Method == http.MethodPatch:
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/org/Shop/_apis/wit/workitems/"))
		f.patchItem(w, r, id)
	case r.URL.Path == "/org/_apis/projects/Shop":
		write(w, 200, map[string]any{"id": "p1", "name": "Shop"})
	case strings.HasPrefix(r.URL.Path, "/org/_apis/projects/"):
		write(w, 404, map[string]any{"message": "The project does not exist."})
	case r.URL.Path == "/org/Shop/_apis/wit/wiql" && r.Method == http.MethodPost:
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.Queries = append(f.Queries, body.Query)
		if strings.Contains(body.Query, "syntax error") {
			write(w, 400, map[string]any{"message": "TF51005: The query references a field that does not exist."})
			return
		}
		var types []string
		if i := strings.Index(body.Query, "[System.WorkItemType] IN ("); i >= 0 {
			list := body.Query[i+len("[System.WorkItemType] IN ("):]
			list = list[:strings.Index(list, ")")]
			for _, t := range strings.Split(list, ",") {
				types = append(types, strings.Trim(strings.TrimSpace(t), "'"))
			}
		}
		ids := []int{}
		for id, it := range f.Items {
			if len(types) == 0 || contains(types, it["System.WorkItemType"]) {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
		if top, err := strconv.Atoi(r.URL.Query().Get("$top")); err == nil && top < len(ids) {
			ids = ids[:top]
		}
		var wi []map[string]any
		for _, id := range ids {
			wi = append(wi, map[string]any{"id": id, "url": "https://dev.azure.example/org/_apis/wit/workItems/" + strconv.Itoa(id)})
		}
		write(w, 200, map[string]any{"queryType": "flat", "workItems": wi})
	case r.URL.Path == "/org/Shop/_apis/wit/fields":
		write(w, 200, map[string]any{"count": len(f.Fields), "value": f.Fields})
	case r.URL.Path == "/org/Shop/_apis/wit/workitems":
		if r.URL.Query().Get("$expand") != "relations" {
			write(w, 400, map[string]any{"message": "expand"})
			return
		}
		var out []map[string]any
		for _, s := range strings.Split(r.URL.Query().Get("ids"), ",") {
			id, _ := strconv.Atoi(s)
			it, ok := f.Items[id]
			if !ok {
				continue
			}
			out = append(out, map[string]any{"id": id, "rev": f.rev(id), "fields": it, "relations": f.Relations[id],
				"_links": map[string]any{"html": map[string]any{"href": "https://dev.azure.example/org/Shop/_workitems/edit/" + s}}})
		}
		write(w, 200, map[string]any{"count": len(out), "value": out})
	default:
		write(w, 404, map[string]any{"message": "Not found " + r.URL.Path})
	}
}

func (f *Azure) readPatch(w http.ResponseWriter, r *http.Request) ([]map[string]any, bool) {
	if r.Header.Get("Content-Type") != "application/json-patch+json" {
		write(w, 415, map[string]any{"message": "content type must be application/json-patch+json"})
		return nil, false
	}
	var ops []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&ops); err != nil {
		write(w, 400, map[string]any{"message": "invalid patch"})
		return nil, false
	}
	f.Patches = append(f.Patches, ops)
	return ops, true
}

func (f *Azure) apply(id int, ops []map[string]any) {
	for _, op := range ops {
		path, _ := op["path"].(string)
		switch {
		case strings.HasPrefix(path, "/fields/"):
			f.Items[id][strings.TrimPrefix(path, "/fields/")] = op["value"]
		case path == "/relations/-":
			v, _ := op["value"].(map[string]any)
			v["attributes"] = map[string]any{"isLocked": false}
			f.Relations[id] = append(f.Relations[id], v)
		}
	}
}

func (f *Azure) createItem(w http.ResponseWriter, r *http.Request, typ string) {
	ops, ok := f.readPatch(w, r)
	if !ok {
		return
	}
	if f.next == 0 {
		f.next = 1000
		for id := range f.Items {
			f.next = max(f.next, id)
		}
	}
	f.next++
	id := f.next
	f.Items[id] = map[string]any{"System.WorkItemType": strings.ReplaceAll(typ, "%20", " "), "System.State": "New"}
	f.apply(id, ops)
	if t, _ := f.Items[id]["System.Title"].(string); t == "" {
		delete(f.Items, id)
		write(w, 400, map[string]any{"message": "TF401320: Rule Error for field Title. Error code: Required, InvalidEmpty."})
		return
	}
	f.Revs[id] = 1
	write(w, 200, map[string]any{"id": id, "rev": 1, "fields": f.Items[id]})
}

func (f *Azure) patchItem(w http.ResponseWriter, r *http.Request, id int) {
	if f.Items[id] == nil {
		write(w, 404, map[string]any{"message": "TF401232: Work item does not exist."})
		return
	}
	ops, ok := f.readPatch(w, r)
	if !ok {
		return
	}
	for _, op := range ops {
		if op["op"] == "test" && op["path"] == "/rev" {
			if n, _ := op["value"].(float64); int(n) != f.rev(id) {
				write(w, 412, map[string]any{"message": "TF401289: The current work item revision is " + strconv.Itoa(f.rev(id)) + "."})
				return
			}
		}
	}
	f.apply(id, ops)
	f.Revs[id] = f.rev(id) + 1
	write(w, 200, map[string]any{"id": id, "rev": f.rev(id), "fields": f.Items[id]})
}

func contains(list []string, v any) bool {
	s, _ := v.(string)
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
