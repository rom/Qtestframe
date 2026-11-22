package tracker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
)

// ---------------------------------------------------------------- Azure DevOps
//
// Azure DevOps Services (https://dev.azure.com/<org>) or Server (a
// collection URL) through the Work Item Tracking REST API 7.1. The personal
// access token (scope Work Items: Read) is sent with Basic authentication
// and an empty user name. Work items are found with a WIQL query and read
// in batches of 200 with their relations; HTML fields become Markdown.

const azureAPI = "api-version=7.1"

// Azure is a connection to one Azure DevOps project.
type Azure struct {
	cfg *model.AzureDevOpsSource
	hc  *http.Client
	org string // organization or collection URL
	// project: the URL path segment of the project
	project string
}

// NewAzure returns the client of an Azure DevOps source.
func NewAzure(cfg *model.AzureDevOpsSource, hc *http.Client) *Azure {
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	return &Azure{cfg: cfg, hc: hc, org: strings.TrimRight(strings.TrimSpace(cfg.URL), "/"), project: url.PathEscape(strings.TrimSpace(cfg.Project))}
}

func (a *Azure) auth(r *http.Request) {
	if a.cfg.Token != "" {
		r.SetBasicAuth("", a.cfg.Token)
	}
}

// Check reads the project, which needs a valid token that can see it.
func (a *Azure) Check(ctx context.Context) error {
	return do(ctx, a.hc, http.MethodGet, a.org+"/_apis/projects/"+a.project+"?"+azureAPI, nil, a.auth, nil)
}

// wiqlQuote quotes a WIQL string literal.
func wiqlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// AzureWIQL builds the query of the simple form: the work items of the
// project under an area path ("" = all) of the given types (none = all),
// in ID order.
func AzureWIQL(areaPath string, types []string) string {
	q := "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project"
	if a := strings.TrimSpace(areaPath); a != "" {
		q += " AND [System.AreaPath] UNDER " + wiqlQuote(a)
	}
	var ts []string
	for _, t := range types {
		if t = strings.TrimSpace(t); t != "" {
			ts = append(ts, wiqlQuote(t))
		}
	}
	if len(ts) > 0 {
		q += " AND [System.WorkItemType] IN (" + strings.Join(ts, ", ") + ")"
	}
	return q + " ORDER BY [System.Id]"
}

// azureSkip are bookkeeping fields that are not content.
var azureSkip = map[string]bool{
	"System.Id": true, "System.Rev": true, "System.Watermark": true, "System.AuthorizedDate": true, "System.RevisedDate": true,
	"System.AuthorizedAs": true, "System.PersonId": true, "System.CommentCount": true, "System.TeamProject": true,
	"System.NodeName": true, "System.AreaId": true, "System.IterationId": true, "System.AreaLevel1": true, "System.IterationLevel1": true,
}

type azureField struct {
	ReferenceName string `json:"referenceName"`
	Name          string `json:"name"`
	Type          string `json:"type"`
}

type azureWorkItem struct {
	ID        int            `json:"id"`
	Rev       int            `json:"rev"`
	Fields    map[string]any `json:"fields"`
	Relations []struct {
		Rel        string         `json:"rel"`
		URL        string         `json:"url"`
		Attributes map[string]any `json:"attributes"`
	} `json:"relations"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"_links"`
}

// Search runs a WIQL query (at most max work items, MaxSourceItems when 0).
func (a *Azure) Search(ctx context.Context, wiql string, max int) (*SourceResult, error) {
	if max <= 0 || max > MaxSourceItems {
		max = MaxSourceItems
	}
	res := &SourceResult{System: SourceAzure, Base: a.org + "/" + a.project}
	var q struct {
		WorkItems []struct {
			ID int `json:"id"`
		} `json:"workItems"`
		Relations []struct {
			Target *struct {
				ID int `json:"id"`
			} `json:"target"`
		} `json:"workItemRelations"`
	}
	if err := do(ctx, a.hc, http.MethodPost, a.org+"/"+a.project+"/_apis/wit/wiql?$top="+strconv.Itoa(max+1)+"&"+azureAPI, map[string]string{"query": wiql}, a.auth, &q); err != nil {
		return nil, err
	}
	var ids []int
	seen := map[int]bool{}
	add := func(id int) {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, w := range q.WorkItems {
		add(w.ID)
	}
	for _, r := range q.Relations { // a query of links between work items
		if r.Target != nil {
			add(r.Target.ID)
		}
	}
	if len(ids) > max {
		ids, res.Truncated = ids[:max], true
	}
	return a.read(ctx, ids, res)
}

// read reads work items by ID (with the project's fields) into res.
func (a *Azure) read(ctx context.Context, ids []int, res *SourceResult) (*SourceResult, error) {
	var fl struct {
		Value []azureField `json:"value"`
	}
	if err := do(ctx, a.hc, http.MethodGet, a.org+"/"+a.project+"/_apis/wit/fields?"+azureAPI, nil, a.auth, &fl); err != nil {
		return nil, err
	}
	fields := map[string]azureField{}
	for _, f := range fl.Value {
		fields[f.ReferenceName] = f
	}
	for i := 0; i < len(ids); i += 200 {
		batch := ids[i:min(i+200, len(ids))]
		parts := make([]string, len(batch))
		for j, id := range batch {
			parts[j] = strconv.Itoa(id)
		}
		var out struct {
			Value []azureWorkItem `json:"value"`
		}
		if err := do(ctx, a.hc, http.MethodGet, a.org+"/"+a.project+"/_apis/wit/workitems?ids="+strings.Join(parts, ",")+"&$expand=relations&errorPolicy=omit&"+azureAPI, nil, a.auth, &out); err != nil {
			return nil, err
		}
		for _, w := range out.Value {
			if w.ID > 0 {
				res.Items = append(res.Items, a.item(&w, fields))
			}
		}
	}
	used := map[string]bool{}
	for _, it := range res.Items {
		for id := range it.Values {
			if !used[id] {
				used[id] = true
				f := fields[id]
				name := f.Name
				if name == "" {
					name = id
				}
				res.Fields = append(res.Fields, SourceField{ID: id, Name: name, Type: azureFieldType(id, f.Type)})
			}
		}
	}
	sortFields(SourceAzure, res.Fields)
	return res, nil
}

func azureFieldType(id, typ string) string {
	switch {
	case strings.EqualFold(typ, "html"):
		return FieldMarkdown
	case id == "System.Tags":
		return FieldList
	}
	return FieldText
}

func (a *Azure) item(w *azureWorkItem, fields map[string]azureField) SourceItem {
	key := strconv.Itoa(w.ID)
	it := SourceItem{Key: key, URL: w.Links.HTML.Href, Values: map[string]string{}}
	if it.URL == "" {
		it.URL = a.org + "/" + a.project + "/_workitems/edit/" + key
	}
	names := make([]string, 0, len(w.Fields))
	for id := range w.Fields {
		names = append(names, id)
	}
	sort.Strings(names)
	for _, id := range names {
		if azureSkip[id] {
			continue
		}
		v := w.Fields[id]
		text := ""
		if s, ok := v.(string); ok && strings.EqualFold(fields[id].Type, "html") {
			text = reqimport.HTMLMarkdown(s)
		} else {
			text = jsonText(v)
		}
		if id == "System.Tags" {
			text = strings.Join(splitLabels(strings.ReplaceAll(text, ";", ",")), ", ")
		}
		if text = strings.TrimSpace(text); text != "" {
			it.Values[id] = text
		}
	}
	it.Type = it.Values["System.WorkItemType"]
	if w.Rev > 0 {
		it.Version = strconv.Itoa(w.Rev)
	}
	for _, r := range w.Relations {
		i := strings.LastIndex(strings.ToLower(r.URL), "/workitems/")
		if i < 0 {
			continue // a hyperlink or an artifact link
		}
		other := r.URL[i+len("/workitems/"):]
		if n, err := strconv.Atoi(other); err != nil || n <= 0 {
			continue
		}
		name, _ := r.Attributes["name"].(string)
		l := SourceLink{Name: firstText(name, r.Rel)}
		switch r.Rel {
		case "System.LinkTypes.Hierarchy-Reverse": // the parent
			l.Type, l.Source, l.Target = "refines", key, other
		case "System.LinkTypes.Dependency-Reverse": // a predecessor
			l.Type, l.Source, l.Target = "depends_on", key, other
		case "System.LinkTypes.Duplicate-Reverse": // duplicate of
			l.Type, l.Source, l.Target = "duplicates", key, other
		case "System.LinkTypes.Related":
			a, b := key, other
			if atoi(a) > atoi(b) {
				a, b = b, a
			}
			l.Type, l.Source, l.Target = "relates_to", a, b
		case "System.LinkTypes.Hierarchy-Forward", "System.LinkTypes.Dependency-Forward", "System.LinkTypes.Duplicate-Forward":
			continue // read from the other end
		default:
			if strings.HasSuffix(r.Rel, "-Reverse") || strings.HasPrefix(r.Rel, "Microsoft.VSTS.Common.Tested") {
				continue
			}
			l.Source, l.Target = key, other
		}
		l.Ref = fmt.Sprintf("%s:%s:%s", r.Rel, l.Source, l.Target)
		if r.Rel == "System.LinkTypes.Related" {
			l.Ref = "related:" + l.Source + ":" + l.Target
		}
		it.Links = append(it.Links, l)
	}
	return it
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func firstText(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
