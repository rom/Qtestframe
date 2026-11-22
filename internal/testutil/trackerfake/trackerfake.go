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
// starts with X-, anything else all issues.
type Jira struct {
	mu sync.Mutex
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
	return &Jira{Issues: map[string]map[string]any{}, PageSize: 2,
		Names:  map[string]string{"summary": "Summary", "description": "Description", "priority": "Priority", "labels": "Labels", "status": "Status", "issuetype": "Issue Type"},
		Schema: map[string]map[string]string{"labels": {"type": "array", "items": "string"}, "description": {"type": "string"}}}
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
	default:
		write(w, 404, map[string]any{"errorMessages": []string{"Not found"}})
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
}

// NewAzure returns an Azure DevOps project with the standard fields.
func NewAzure() *Azure {
	f := &Azure{Items: map[int]map[string]any{}, Relations: map[int][]map[string]any{}}
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
	if r.URL.Query().Get("api-version") != "7.1" {
		write(w, 400, map[string]any{"message": "api-version missing"})
		return
	}
	switch {
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
			out = append(out, map[string]any{"id": id, "rev": 1, "fields": it, "relations": f.Relations[id],
				"_links": map[string]any{"html": map[string]any{"href": "https://dev.azure.example/org/Shop/_workitems/edit/" + s}}})
		}
		write(w, 200, map[string]any{"count": len(out), "value": out})
	default:
		write(w, 404, map[string]any{"message": "Not found " + r.URL.Path})
	}
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
