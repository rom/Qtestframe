package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// Importing requirements from Jira (JQL search) and Azure DevOps (WIQL
// query, azure.go): the issues or work items are read with all their
// fields as text (rich text converted to Markdown) and their links to each
// other. Nothing is mapped to requirement fields here: the caller (the
// requirement import of internal/api) maps the fields.

// Import source systems.
const (
	SourceJira  = "jira"
	SourceAzure = "azure-devops"
)

// MaxSourceItems bounds the issues or work items of one import.
const MaxSourceItems = 1000

// Field types of a source.
const (
	FieldText     = "text"
	FieldMarkdown = "markdown" // rich text, converted to Markdown
	FieldList     = "list"     // values joined with ", "
)

// SourceField is a field of the issues or work items.
type SourceField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// SourceLink is a link between two items, oriented as the requirement
// relation it becomes (Source relation Target).
type SourceLink struct {
	// Ref identifies the link in the source (the same from both ends).
	Ref string
	// Type is the model relation type ("" = derived from Name by the caller).
	Type string
	// Name is the link type's name in the source.
	Name           string
	Source, Target string
}

// SourceItem is an issue or a work item.
type SourceItem struct {
	Key  string
	URL  string
	Type string // issue type or work item type
	// Values by field ID, as text.
	Values map[string]string
	// Category is the Jira status category (new, indeterminate, done).
	Category string
	Links    []SourceLink
}

// SourceResult is the outcome of a query.
type SourceResult struct {
	System string
	// Base identifies the instance and project (recorded with the
	// external references of the imported requirements).
	Base   string
	Fields []SourceField
	Items  []SourceItem
	// Truncated: the query found more than the items returned.
	Truncated bool
}

// Default field IDs.
const (
	JiraSummary     = "summary"
	JiraDescription = "description"
	AzureTitle      = "System.Title"
	AzureCriteria   = "Microsoft.VSTS.Common.AcceptanceCriteria"
)

// DefaultSourceMapping is the field mapping of a system: source field ID →
// requirement field.
func DefaultSourceMapping(system string) map[string]string {
	if system == SourceAzure {
		return map[string]string{AzureTitle: "title", "System.Description": "description", "Microsoft.VSTS.Common.Priority": "priority",
			"System.Tags": "tags", "System.State": "status", AzureCriteria: "criteria"}
	}
	return map[string]string{JiraSummary: "title", JiraDescription: "description", "priority": "priority", "labels": "tags", "status": "status"}
}

// sortFields orders the fields: those of the default mapping first, then by name.
func sortFields(system string, fs []SourceField) {
	def := DefaultSourceMapping(system)
	sort.SliceStable(fs, func(i, j int) bool {
		_, a := def[fs[i].ID]
		_, b := def[fs[j].ID]
		if a != b {
			return a
		}
		return strings.ToLower(fs[i].Name) < strings.ToLower(fs[j].Name)
	})
}

// ---------------------------------------------------------------- Jira

// jiraSkip are fields that are not content.
var jiraSkip = map[string]bool{
	"issuelinks": true, "attachment": true, "comment": true, "worklog": true, "watches": true, "votes": true, "timetracking": true,
	"progress": true, "aggregateprogress": true, "aggregatetimeestimate": true, "aggregatetimeoriginalestimate": true,
	"aggregatetimespent": true, "timeestimate": true, "timeoriginalestimate": true, "timespent": true, "subtasks": true,
	"lastViewed": true, "thumbnail": true, "project": true, "workratio": true, "statuscategorychangedate": true, "parent": true,
}

type jiraSchema struct {
	Type   string `json:"type"`
	Items  string `json:"items"`
	Custom string `json:"custom"`
}

type jiraSearchPage struct {
	Issues []struct {
		Key    string                     `json:"key"`
		Fields map[string]json.RawMessage `json:"fields"`
	} `json:"issues"`
	Names         map[string]string     `json:"names"`
	Schema        map[string]jiraSchema `json:"schema"`
	Total         int                   `json:"total"`
	IsLast        bool                  `json:"isLast"`
	NextPageToken string                `json:"nextPageToken"`
}

// SearchJira runs a JQL query (at most max issues, MaxSourceItems when 0).
// It uses the search of Jira Cloud (POST /rest/api/2/search/jql, paged by
// token) and falls back to the classic search of Jira Server and Data
// Center (POST /rest/api/2/search).
func SearchJira(ctx context.Context, t *model.DefectTracker, hc *http.Client, jql string, max int) (*SourceResult, error) {
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	if max <= 0 || max > MaxSourceItems {
		max = MaxSourceItems
	}
	j := &jira{cfg: t, hc: hc}
	res := &SourceResult{System: SourceJira, Base: strings.TrimRight(t.URL, "/")}
	names, schemas := map[string]string{}, map[string]jiraSchema{}
	classic, token, startAt := false, "", 0
	for {
		size := min(100, max-len(res.Items))
		var page jiraSearchPage
		var err error
		if !classic {
			body := map[string]any{"jql": jql, "maxResults": size, "fields": []string{"*all"}, "expand": "names,schema"}
			if token != "" {
				body["nextPageToken"] = token
			}
			err = do(ctx, hc, http.MethodPost, res.Base+"/rest/api/2/search/jql", body, j.auth, &page)
			var te *Error
			if errors.As(err, &te) && token == "" && (te.Status == http.StatusNotFound || te.Status == http.StatusMethodNotAllowed || te.Status == http.StatusGone) {
				classic = true
				continue
			}
		} else {
			body := map[string]any{"jql": jql, "startAt": startAt, "maxResults": size, "fields": []string{"*all"}, "expand": []string{"names", "schema"}}
			err = do(ctx, hc, http.MethodPost, res.Base+"/rest/api/2/search", body, j.auth, &page)
		}
		if err != nil {
			return nil, err
		}
		for k, v := range page.Names {
			names[k] = v
		}
		for k, v := range page.Schema {
			schemas[k] = v
		}
		for _, is := range page.Issues {
			if len(res.Items) >= max {
				res.Truncated = true
				break
			}
			res.Items = append(res.Items, jiraItem(res.Base, is.Key, is.Fields, schemas))
		}
		more := false
		if classic {
			startAt += len(page.Issues)
			more = len(page.Issues) > 0 && startAt < page.Total
		} else {
			token = page.NextPageToken
			more = len(page.Issues) > 0 && !page.IsLast && token != ""
		}
		if !more {
			break
		}
		if len(res.Items) >= max {
			res.Truncated = true
			break
		}
	}
	seen := map[string]bool{}
	for _, it := range res.Items {
		for id := range it.Values {
			if !seen[id] {
				seen[id] = true
				name := names[id]
				if name == "" {
					name = id
				}
				res.Fields = append(res.Fields, SourceField{ID: id, Name: name, Type: jiraFieldType(id, schemas[id])})
			}
		}
	}
	sortFields(SourceJira, res.Fields)
	return res, nil
}

func jiraRich(id string, s jiraSchema) bool {
	return id == "description" || id == "environment" || strings.HasSuffix(s.Custom, ":textarea")
}

func jiraFieldType(id string, s jiraSchema) string {
	switch {
	case jiraRich(id, s):
		return FieldMarkdown
	case s.Type == "array":
		return FieldList
	}
	return FieldText
}

func jiraItem(base, key string, fields map[string]json.RawMessage, schemas map[string]jiraSchema) SourceItem {
	it := SourceItem{Key: key, URL: base + "/browse/" + key, Values: map[string]string{}}
	for id, raw := range fields {
		if jiraSkip[id] {
			continue
		}
		var v any
		if json.Unmarshal(raw, &v) != nil || v == nil {
			continue
		}
		text := strings.TrimSpace(jsonText(v))
		if text == "" {
			continue
		}
		if s, ok := v.(string); ok && jiraRich(id, schemas[id]) {
			text = JiraMarkdown(s)
		}
		it.Values[id] = text
	}
	var f struct {
		IssueType struct {
			Name string `json:"name"`
		} `json:"issuetype"`
		Status struct {
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Parent struct {
			Key string `json:"key"`
		} `json:"parent"`
		Links []struct {
			ID   string `json:"id"`
			Type struct {
				Name    string `json:"name"`
				Outward string `json:"outward"`
			} `json:"type"`
			Outward *struct {
				Key string `json:"key"`
			} `json:"outwardIssue"`
		} `json:"issuelinks"`
	}
	all, _ := json.Marshal(fields)
	_ = json.Unmarshal(all, &f)
	it.Type, it.Category = f.IssueType.Name, f.Status.StatusCategory.Key
	if f.Parent.Key != "" {
		it.Links = append(it.Links, SourceLink{Ref: "parent:" + key, Type: "refines", Name: "Parent", Source: key, Target: f.Parent.Key})
	}
	for _, l := range f.Links {
		if l.Outward == nil || l.Outward.Key == "" {
			continue // the inward end: read from the other issue
		}
		sl := SourceLink{Ref: "link:" + l.ID, Name: l.Type.Name, Source: key, Target: l.Outward.Key}
		if sl.Ref == "link:" {
			sl.Ref = "link:" + key + ">" + l.Type.Name + ">" + l.Outward.Key
		}
		n := strings.ToLower(l.Type.Name + " " + l.Type.Outward)
		switch {
		case strings.Contains(n, "block"):
			// A blocks B: B depends on A
			sl.Type, sl.Source, sl.Target = "depends_on", l.Outward.Key, key
		case strings.Contains(n, "duplicate"):
			sl.Type = "duplicates"
		case strings.Contains(n, "clone"):
			sl.Type = "derived_from"
		case strings.Contains(n, "relate"):
			sl.Type = "relates_to"
		}
		it.Links = append(it.Links, sl)
	}
	return it
}

// jsonText renders a field value: strings as they are, numbers, options
// and users by their value or name, lists joined with ", " and Atlassian
// documents (API v3) as their text.
func jsonText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case []any:
		var out []string
		for _, e := range x {
			if s := strings.TrimSpace(jsonText(e)); s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, ", ")
	case map[string]any:
		if x["type"] == "doc" {
			return adfText(x)
		}
		for _, k := range []string{"value", "name", "displayName", "key", "uniqueName"} {
			if s, ok := x[k].(string); ok && s != "" {
				if c, ok := x["child"].(map[string]any); ok && k == "value" {
					if cs := jsonText(c); cs != "" {
						return s + " / " + cs
					}
				}
				return s
			}
		}
	}
	return ""
}

// adfText is the text of an Atlassian document, a paragraph per line.
func adfText(n map[string]any) string {
	var b strings.Builder
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		if s, ok := n["text"].(string); ok {
			b.WriteString(s)
		}
		kids, _ := n["content"].([]any)
		for _, k := range kids {
			if m, ok := k.(map[string]any); ok {
				walk(m)
			}
		}
		switch n["type"] {
		case "paragraph", "heading", "listItem", "hardBreak":
			b.WriteString("\n")
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}
