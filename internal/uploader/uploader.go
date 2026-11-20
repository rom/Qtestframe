// Package uploader reports automated test results (JUnit XML or the JSON
// import format) to a Qtestframe server through its REST API. It backs the
// `qtestframe report` command.
package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Client talks to one Qtestframe server with an API token.
type Client struct {
	base  string
	token string
	hc    *http.Client
}

// New returns a client for baseURL (with or without a trailing /api/v1).
func New(baseURL, token string, hc *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/api/v1")
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid server URL %q (expected e.g. https://qtestframe.example.com)", baseURL)
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("an API token is required (--token or QTF_TOKEN)")
	}
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Client{base: baseURL + "/api/v1", token: strings.TrimSpace(token), hc: hc}, nil
}

// APIError is an error response of the server.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("server answered %d", e.Status)
	}
	return fmt.Sprintf("server answered %d: %s", e.Status, e.Message)
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, ctype string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "qtestframe-report")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		e := &APIError{Status: res.StatusCode}
		var m struct{ Error, Code string }
		if json.Unmarshal(data, &m) == nil && m.Error != "" {
			e.Message, e.Code = m.Error, m.Code
		} else {
			e.Message = strings.TrimSpace(string(data))
			if len(e.Message) > 200 {
				e.Message = e.Message[:200] + "…"
			}
		}
		return e
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("unexpected response from %s %s: %v", method, path, err)
		}
	}
	return nil
}

func (c *Client) sendJSON(ctx context.Context, method, path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return c.do(ctx, method, path, bytes.NewReader(b), "application/json", out)
}

// Project is the part of a project the uploader needs.
type Project struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Run is the part of a test run the uploader needs.
type Run struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ProjectByKey finds a project by its key (case-insensitive).
func (c *Client) ProjectByKey(ctx context.Context, key string) (*Project, error) {
	var l struct{ Items []Project }
	if err := c.do(ctx, http.MethodGet, "/projects", nil, "", &l); err != nil {
		return nil, err
	}
	for i := range l.Items {
		if strings.EqualFold(l.Items[i].Key, key) {
			return &l.Items[i], nil
		}
	}
	return nil, fmt.Errorf("project %q not found (or the token may not read it)", key)
}

// FindRun returns the newest run named name in the project, preferring runs
// that are not aborted; nil if there is none.
func (c *Client) FindRun(ctx context.Context, projectID, name string) (*Run, error) {
	var l struct{ Items []Run }
	if err := c.do(ctx, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/runs", nil, "", &l); err != nil {
		return nil, err
	}
	var found []Run
	for _, r := range l.Items {
		if r.Name == name {
			found = append(found, r)
		}
	}
	if len(found) == 0 {
		return nil, nil
	}
	sort.SliceStable(found, func(i, j int) bool {
		if a, b := found[i].Status == "aborted", found[j].Status == "aborted"; a != b {
			return b
		}
		return found[i].CreatedAt.After(found[j].CreatedAt)
	})
	return &found[0], nil
}

// CreateRun creates an empty run; imported results add their tests.
func (c *Client) CreateRun(ctx context.Context, projectID, name, build, environment string) (*Run, error) {
	var r Run
	err := c.sendJSON(ctx, http.MethodPost, "/projects/"+url.PathEscape(projectID)+"/runs",
		map[string]string{"name": name, "build": build, "environment": environment}, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Rejected is a result the server did not record because the test may not
// be executed.
type Rejected struct {
	TestKey string `json:"test_key"`
	Reason  string `json:"reason"`
}

// Created is a test created by an import with create_missing.
type Created struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

// Summary is the combined answer of the import requests.
type Summary struct {
	RunID    string     `json:"run_id"`
	Updated  int        `json:"updated"`
	Unknown  []string   `json:"unknown"`
	Unmapped []string   `json:"unmapped"`
	Rejected []Rejected `json:"rejected"`
	Created  []Created  `json:"created"`
}

func (s *Summary) add(o *Summary) {
	s.Updated += o.Updated
	s.Unknown = append(s.Unknown, o.Unknown...)
	s.Unmapped = append(s.Unmapped, o.Unmapped...)
	s.Rejected = append(s.Rejected, o.Rejected...)
	s.Created = append(s.Created, o.Created...)
}

// File is a report to upload.
type File struct {
	Name string
	Data []byte
}

// ImportJUnit uploads JUnit XML files in one request (so results of one
// test spread over several files combine on the server).
func (c *Client) ImportJUnit(ctx context.Context, runID string, files []File, createMissing bool, configuration string) (*Summary, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		w, err := mw.CreateFormFile("file", filepath.Base(f.Name))
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.Data); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	q := url.Values{}
	if createMissing {
		q.Set("create_missing", "true")
	}
	if configuration != "" {
		q.Set("configuration", configuration)
	}
	path := "/runs/" + url.PathEscape(runID) + "/junit"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var s Summary
	if err := c.do(ctx, http.MethodPost, path, &buf, mw.FormDataContentType(), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ImportJSON posts results in the JSON import format; configuration (may
// be empty) applies to results that name none.
func (c *Client) ImportJSON(ctx context.Context, runID string, results []json.RawMessage, configuration string) (*Summary, error) {
	var s Summary
	path := "/runs/" + url.PathEscape(runID) + "/results"
	if configuration != "" {
		path += "?" + url.Values{"configuration": {configuration}}.Encode()
	}
	err := c.sendJSON(ctx, http.MethodPost, path, map[string]any{"results": results}, &s)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Options configure Report.
type Options struct {
	URL, Token string
	// Project key; required unless RunID is given.
	Project string
	// Run name (found in the project, or created with CreateRun) or RunID.
	Run, RunID         string
	CreateRun          bool
	Build, Environment string
	// Configuration (name or id) of the results; empty: none, or the
	// run's only configuration.
	Configuration string
	// JUnit and JSON are file names or glob patterns.
	JUnit, JSON   []string
	CreateMissing bool
	FailOnUnknown bool
	HTTP          *http.Client
	// Log receives progress messages (may be nil).
	Log io.Writer
}

// ErrUnknown is returned (with the summary) when FailOnUnknown is set and
// some results could not be matched to a test.
var ErrUnknown = errors.New("some results did not match a test")

// Report resolves the run, uploads the files and returns the summary.
func Report(ctx context.Context, o Options) (*Summary, error) {
	logf := func(format string, a ...any) {
		if o.Log != nil {
			fmt.Fprintf(o.Log, format+"\n", a...)
		}
	}
	junitFiles, err := ExpandFiles(o.JUnit)
	if err != nil {
		return nil, err
	}
	jsonFiles, err := ExpandFiles(o.JSON)
	if err != nil {
		return nil, err
	}
	if len(junitFiles) == 0 && len(jsonFiles) == 0 {
		return nil, errors.New("nothing to report: give --junit FILE or --json FILE")
	}
	// read everything before touching the server
	var xmlDocs []File
	for _, f := range junitFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		xmlDocs = append(xmlDocs, File{Name: f, Data: b})
	}
	var jsonResults []json.RawMessage
	for _, f := range jsonFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Results []json.RawMessage `json:"results"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("%s: invalid JSON: %v", f, err)
		}
		jsonResults = append(jsonResults, doc.Results...)
	}
	c, err := New(o.URL, o.Token, o.HTTP)
	if err != nil {
		return nil, err
	}
	runID := o.RunID
	switch {
	case runID != "" && o.Run != "":
		return nil, errors.New("give either --run or --run-id, not both")
	case runID == "":
		if o.Run == "" {
			return nil, errors.New("--run NAME or --run-id ID is required")
		}
		if o.Project == "" {
			return nil, errors.New("--project KEY is required with --run")
		}
		p, err := c.ProjectByKey(ctx, o.Project)
		if err != nil {
			return nil, err
		}
		run, err := c.FindRun(ctx, p.ID, o.Run)
		if err != nil {
			return nil, err
		}
		if run == nil {
			if !o.CreateRun {
				return nil, fmt.Errorf("run %q not found in project %s (pass --create-run to create it)", o.Run, p.Key)
			}
			if run, err = c.CreateRun(ctx, p.ID, o.Run, o.Build, o.Environment); err != nil {
				return nil, err
			}
			logf("created run %q (%s)", run.Name, run.ID)
		}
		runID = run.ID
	}
	sum := &Summary{RunID: runID}
	if len(jsonResults) > 0 {
		s, err := c.ImportJSON(ctx, runID, jsonResults, o.Configuration)
		if err != nil {
			return nil, err
		}
		logf("uploaded %d JSON result(s) from %d file(s)", len(jsonResults), len(jsonFiles))
		sum.add(s)
	}
	if len(xmlDocs) > 0 {
		s, err := c.ImportJUnit(ctx, runID, xmlDocs, o.CreateMissing, o.Configuration)
		if err != nil {
			return sum, err
		}
		logf("uploaded %d JUnit file(s)", len(xmlDocs))
		sum.add(s)
	}
	if o.FailOnUnknown && len(sum.Unknown)+len(sum.Unmapped) > 0 {
		return sum, ErrUnknown
	}
	return sum, nil
}

// ExpandFiles resolves file names and glob patterns; a pattern that matches
// nothing is an error. Duplicates are removed.
func ExpandFiles(patterns []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %v", p, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no file matches %q", p)
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out, nil
}

// Print writes a short human-readable summary.
func (s *Summary) Print(w io.Writer) {
	fmt.Fprintf(w, "run %s: %d result(s) recorded\n", s.RunID, s.Updated)
	if len(s.Created) > 0 {
		fmt.Fprintf(w, "created %d test(s):\n", len(s.Created))
		for _, c := range s.Created {
			fmt.Fprintf(w, "  %s  %s\n", c.Key, c.Title)
		}
	}
	list := func(label string, v []string) {
		if len(v) > 0 {
			fmt.Fprintf(w, "%s (%d): %s\n", label, len(v), strings.Join(v, ", "))
		}
	}
	list("unknown test keys", s.Unknown)
	list("test cases without a test key", s.Unmapped)
	if len(s.Rejected) > 0 {
		fmt.Fprintf(w, "rejected (%d):\n", len(s.Rejected))
		for _, r := range s.Rejected {
			fmt.Fprintf(w, "  %s: %s\n", r.TestKey, r.Reason)
		}
	}
}
