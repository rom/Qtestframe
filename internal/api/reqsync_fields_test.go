package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/tracker"
)

func TestReqSyncFieldValues(t *testing.T) {
	status := &model.ReqSyncField{Field: "status", Remote: "status", Values: map[string]string{"To Do": "draft", "Done": "implemented"}}
	it := &tracker.SourceItem{Values: map[string]string{"status": "Weird", "priority": "P9", "crit": "* one\n- two", "summary": "  A   title "}}
	if v, ok := remoteSyncValue(it, status); ok || v != "?Weird" {
		t.Fatal(v, ok)
	}
	it.Category = "done"
	if v, ok := remoteSyncValue(it, status); !ok || v != "implemented" {
		t.Fatal(v, ok)
	}
	it.Values["status"] = "to do"
	if v, _ := remoteSyncValue(it, status); v != "draft" {
		t.Fatal(v)
	}
	prio := &model.ReqSyncField{Field: "priority", Remote: "priority"}
	if v, ok := remoteSyncValue(it, prio); ok || v != "?P9" {
		t.Fatal(v, ok)
	}
	it.Values["priority"] = ""
	if v, ok := remoteSyncValue(it, prio); !ok || v != "" {
		t.Fatal(v, ok)
	}
	if v, _ := remoteSyncValue(it, &model.ReqSyncField{Field: "criteria", Remote: "crit"}); v != "one\ntwo" {
		t.Fatalf("%q", v)
	}
	if v, _ := remoteSyncValue(it, &model.ReqSyncField{Field: "title", Remote: "summary"}); v != "A title" {
		t.Fatalf("%q", v)
	}
	if v, _ := remoteSyncValue(&tracker.SourceItem{Values: map[string]string{}}, &model.ReqSyncField{Field: "status", Remote: "status"}); v != "" {
		t.Fatal(v)
	}
	// pushes: the push map, else the first tracker value mapping to it
	if w := pushSyncValue(status, "implemented"); w.Kind != tracker.WriteStatus || w.Value != "Done" {
		t.Fatal(w)
	}
	status.PushValues = map[string]string{"implemented": "Closed"}
	if w := pushSyncValue(status, "implemented"); w.Value != "Closed" {
		t.Fatal(w)
	}
	if w := pushSyncValue(prio, "high"); w.Kind != tracker.WriteOption || w.Value != "high" {
		t.Fatal(w)
	}
	if w := pushSyncValue(&model.ReqSyncField{Field: "criteria", Remote: "c"}, "a\n\nb"); w.Kind != tracker.WriteMarkdown || w.Value != "- a\n- b" {
		t.Fatal(w)
	}
	if w := pushSyncValue(&model.ReqSyncField{Field: "tags", Remote: "labels"}, "a, b"); w.Kind != tracker.WriteList {
		t.Fatal(w)
	}
	rq := &model.Requirement{Title: " T ", Rationale: "r\r\n", Tags: []string{"b", "A", "a"}, Criteria: []model.EvaluationCriterion{{ID: "c1", Description: "x", Method: "test"}}}
	if localSyncValue(rq, "tags") != "A, b" || localSyncValue(rq, "rationale") != "r" || localSyncValue(rq, "title") != "T" || localSyncValue(rq, "nope") != "" {
		t.Fatal(localSyncValue(rq, "tags"))
	}
	b := localBody(rq, map[string]string{"criteria": "test: y\nz", "tags": "a, b", "title": "N"})
	if cs := b["criteria"].([]model.EvaluationCriterion); len(cs) != 2 || cs[0].ID != "c1" || b["title"] != "N" {
		t.Fatal(b)
	}
	if syncHash(map[string]string{"a": "1"}) == syncHash(map[string]string{"a": "2"}) || shorten("abcdef", 3) != "abc…" {
		t.Fatal("hash")
	}
	// errors that stop a run
	if fatalTrackerErr(&tracker.Error{Status: 429}) == nil || fatalTrackerErr(&tracker.Error{Status: 401}) == nil || fatalTrackerErr(&tracker.Error{Status: 400}) != nil ||
		fatalTrackerErr(errors.New("cannot reach the tracker: x")) == nil || fatalTrackerErr(errors.New("other")) != nil || fatalTrackerErr(nil) != nil {
		t.Fatal("fatal errors")
	}
	if (&errRateLimited{&tracker.Error{Status: 429}}).Error() == "" {
		t.Fatal("message")
	}
	rec := &heldResponse{header: http.Header{}}
	rec.WriteHeader(http.StatusPreconditionFailed)
	if !strings.Contains(syncSubFailure(rec), "changed meanwhile") {
		t.Fatal(syncSubFailure(rec))
	}
	if DefaultSyncSettings("azure-devops").Fields[0].Remote != tracker.AzureTitle {
		t.Fatal("defaults")
	}
	// the web address of tracker items
	s := &Server{}
	src := &model.ImportSources{Azure: model.AzureDevOpsSource{URL: "https://dev.azure.com/o/", Project: "My Shop"}, Jira: model.JiraImportSource{URL: "https://j.example"}}
	if u := s.syncItemURL(context.Background(), nil, "", src, tracker.SourceAzure, "5"); u != "https://dev.azure.com/o/My%20Shop/_workitems/edit/5" {
		t.Fatal(u)
	}
	if u := s.syncItemURL(context.Background(), nil, "", src, tracker.SourceJira, "A-1"); u != "https://j.example/browse/A-1" {
		t.Fatal(u)
	}
	if s.syncItemURL(context.Background(), nil, "", nil, tracker.SourceJira, "A-1") != "" || s.syncItemURL(context.Background(), nil, "", &model.ImportSources{}, tracker.SourceAzure, "1") != "" {
		t.Fatal("no source")
	}
}
