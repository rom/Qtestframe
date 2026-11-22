package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// Code links: builds, commits and merge / pull requests of a project and
// the requirements and tests they refer to (see docs/api.md "Code and
// builds").

// Limits of code links.
const (
	MaxCodeText       = 500
	MaxCodeURL        = 2000
	MaxCodeListLimit  = 500
	MaxKeysPerChange  = 100
	MaxBuildHistory   = 100
	defaultCodeLimit  = 100
	MaxWebhookSecret  = 200
	MinWebhookSecret  = 16
	codeChangeColumns = `id, project_id, kind, provider, repository, ref, title, url, author, branch, target_branch, status, occurred_at, created_at, updated_at`
	buildColumns      = `id, project_id, external_id, number, commit_sha, branch, pipeline_url, created_by, created_at, updated_at`
)

func adCodeSecret(projectID string) string { return "code_settings.webhook_secret|" + projectID }

// ---------------------------------------------------------------- keys

var itemKeyRe = regexp.MustCompile(`(?i)\b(?:([A-Z][A-Z0-9]{1,15})-)?(REQ|TC)-(\d{1,9})\b`)

// ExtractItemKeys returns the requirement and test keys of a project that a
// text mentions, in order and without duplicates: full keys (DEMO-REQ-12)
// and short keys (REQ-12, TC-3) meaning the project's own. A key with
// another project's prefix (an upper-case project key) is ignored.
func ExtractItemKeys(text, projectKey string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range itemKeyRe.FindAllStringSubmatch(text, -1) {
		prefix := m[1]
		if prefix != "" && !strings.EqualFold(prefix, projectKey) && prefix == strings.ToUpper(prefix) {
			continue
		}
		k := strings.ToUpper(projectKey + "-" + m[2] + "-" + strings.TrimLeft(m[3], "0"))
		if strings.HasSuffix(k, "-") {
			continue
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
		if len(out) >= MaxKeysPerChange {
			break
		}
	}
	return out
}

// ResolveItemKeys returns the requirements and tests of a project with the
// given keys (unknown keys are left out).
func (s *Store) ResolveItemKeys(ctx context.Context, tenantID, projectID string, keys []string) ([]model.CodeChangeItem, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	var out []model.CodeChangeItem
	for _, t := range []struct{ table, kind string }{{"requirements", model.KindRequirement}, {"test_cases", model.KindTest}} {
		args := append([]any{tenantID, projectID}, anySlice(keys)...)
		rows, err := s.c(ctx).query(ctx, `SELECT id, key, title FROM `+t.table+` WHERE tenant_id = ? AND project_id = ? AND key IN (`+placeholders(len(keys))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			it := model.CodeChangeItem{ItemType: t.kind}
			if err := rows.Scan(&it.ItemID, &it.Key, &it.Title); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- builds

func scanBuild(r interface{ Scan(...any) error }) (*model.Build, error) {
	var b model.Build
	if err := r.Scan(&b.ID, &b.ProjectID, &b.ExternalID, &b.Number, &b.CommitSHA, &b.Branch, &b.PipelineURL, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

func cleanText(v string, n int) string {
	v = strings.Join(strings.Fields(v), " ")
	if len(v) > n {
		v = truncate(v, n)
	}
	return v
}

// validURL accepts "" or an http(s) URL.
func validURL(v string) bool {
	if v == "" {
		return true
	}
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && len(v) <= MaxCodeURL
}

var shaRe = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// ValidateBuild normalises and checks a build (without saving it): the CI
// build id defaults to the number.
func ValidateBuild(b *model.Build) error {
	b.ExternalID = cleanText(b.ExternalID, 200)
	b.Number = cleanText(b.Number, 200)
	b.CommitSHA = strings.ToLower(strings.TrimSpace(b.CommitSHA))
	b.Branch = cleanText(strings.TrimPrefix(strings.TrimSpace(b.Branch), "refs/heads/"), 250)
	b.PipelineURL = strings.TrimSpace(b.PipelineURL)
	if b.ExternalID == "" {
		b.ExternalID = b.Number
	}
	if b.ExternalID == "" {
		return Invalidf("a build needs its CI build id or number")
	}
	if b.CommitSHA != "" && !shaRe.MatchString(b.CommitSHA) {
		return Invalidf("commit must be a hexadecimal commit SHA")
	}
	if !validURL(b.PipelineURL) {
		return Invalidf("pipeline_url must be an http(s) URL")
	}
	return nil
}

// UpsertBuild creates the build with b's CI id in its project, or updates
// the fields b gives of the existing one. b is filled with the stored build.
func (s *Store) UpsertBuild(ctx context.Context, tenantID string, b *model.Build) (bool, error) {
	if err := ValidateBuild(b); err != nil {
		return false, err
	}
	if _, err := s.GetProject(ctx, tenantID, b.ProjectID); err != nil {
		return false, err
	}
	created := false
	now := s.now()
	err := s.tx(ctx, func(c conn) error {
		cur, err := scanBuild(c.row(ctx, `SELECT `+buildColumns+` FROM builds WHERE project_id = ? AND external_id = ?`, b.ProjectID, b.ExternalID))
		if db.IsNoRows(err) {
			created = true
			b.ID, b.CreatedAt, b.UpdatedAt = newID(), now, now
			_, err = c.exec(ctx, `INSERT INTO builds (id, tenant_id, project_id, external_id, number, commit_sha, branch, pipeline_url, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				b.ID, tenantID, b.ProjectID, b.ExternalID, b.Number, b.CommitSHA, b.Branch, b.PipelineURL, b.CreatedBy, now, now)
			return err
		}
		if err != nil {
			return err
		}
		for _, f := range []struct{ src, dst *string }{{&b.Number, &cur.Number}, {&b.CommitSHA, &cur.CommitSHA}, {&b.Branch, &cur.Branch}, {&b.PipelineURL, &cur.PipelineURL}} {
			if *f.src != "" {
				*f.dst = *f.src
			}
		}
		cur.UpdatedAt = now
		*b = *cur
		_, err = c.exec(ctx, `UPDATE builds SET number = ?, commit_sha = ?, branch = ?, pipeline_url = ?, updated_at = ? WHERE id = ?`, b.Number, b.CommitSHA, b.Branch, b.PipelineURL, now, b.ID)
		return err
	})
	return created, err
}

// GetBuild returns one build with its runs and result summary.
func (s *Store) GetBuild(ctx context.Context, tenantID, id string) (*model.Build, error) {
	b, err := scanBuild(s.c(ctx).row(ctx, `SELECT `+buildColumns+` FROM builds WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "build")
	}
	list := []model.Build{*b}
	if err := s.buildSummaries(ctx, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// ListBuilds returns the builds of a project, newest first (at most limit).
func (s *Store) ListBuilds(ctx context.Context, tenantID, projectID string, limit int) ([]model.Build, error) {
	if limit <= 0 || limit > MaxCodeListLimit {
		limit = defaultCodeLimit
	}
	rows, err := s.c(ctx).query(ctx, `SELECT `+buildColumns+` FROM builds WHERE tenant_id = ? AND project_id = ? ORDER BY created_at DESC, external_id DESC LIMIT ?`, tenantID, projectID, limit)
	if err != nil {
		return nil, err
	}
	out := []model.Build{}
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.buildSummaries(ctx, out)
}

// buildSummaries fills the run count and result summary of builds.
func (s *Store) buildSummaries(ctx context.Context, list []model.Build) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].ID
		list[i].Summary = map[string]int{}
	}
	idx := map[string]int{}
	for i, id := range ids {
		idx[id] = i
	}
	rows, err := s.c(ctx).query(ctx, `SELECT r.build_id, COUNT(DISTINCT r.id) FROM test_runs r WHERE r.build_id IN (`+placeholders(len(ids))+`) GROUP BY r.build_id`, anySlice(ids)...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			rows.Close()
			return err
		}
		list[idx[id]].Runs = n
	}
	rows.Close()
	rows, err = s.c(ctx).query(ctx, `SELECT r.build_id, e.status, COUNT(*) FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE r.build_id IN (`+placeholders(len(ids))+`) GROUP BY r.build_id, e.status`, anySlice(ids)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, st string
		var n int
		if err := rows.Scan(&id, &st, &n); err != nil {
			return err
		}
		list[idx[id]].Summary[st] = n
		list[idx[id]].Summary["total"] += n
	}
	return rows.Err()
}

// DeleteBuild removes a build (its runs stay, without build).
func (s *Store) DeleteBuild(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM builds WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "build")
	}
	return nil
}

// SetRunBuild associates a run with a build (and its variant when
// variantID is not nil; "" makes the run variant-neutral); the run's build
// text is set to the build's label when it is empty.
func (s *Store) SetRunBuild(ctx context.Context, tenantID, runID string, b *model.Build, variantID *string) error {
	run, err := s.GetRun(ctx, tenantID, runID)
	if err != nil {
		return err
	}
	if b != nil {
		run.BuildID = &b.ID
		if run.Build == "" {
			run.Build = b.Label()
		}
	}
	if variantID != nil {
		run.VariantID = variantID
		if *variantID == "" {
			run.VariantID = nil
		}
	}
	return s.UpdateRun(ctx, tenantID, run)
}

// BuildIndex holds the results of a project's runs that belong to builds.
type BuildIndex struct {
	// Builds newest first.
	Builds []model.Build
	units  map[string][]execUnit
	runs   map[string]map[string]bool
	cache  map[string]ConfigResults
}

// LoadBuildIndex loads the builds of a project and their results.
func (s *Store) LoadBuildIndex(ctx context.Context, tenantID, projectID string) (*BuildIndex, error) {
	builds, err := s.ListBuilds(ctx, tenantID, projectID, MaxCodeListLimit)
	if err != nil {
		return nil, err
	}
	all, err := s.projectExecutions(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	bi := &BuildIndex{Builds: builds, units: map[string][]execUnit{}, runs: map[string]map[string]bool{}, cache: map[string]ConfigResults{}}
	for _, u := range all {
		if u.build == "" {
			continue
		}
		bi.units[u.build] = append(bi.units[u.build], u)
		if bi.runs[u.build] == nil {
			bi.runs[u.build] = map[string]bool{}
		}
		bi.runs[u.build][u.run] = true
	}
	return bi, nil
}

// Results returns the latest results per test and configuration in a build.
func (bi *BuildIndex) Results(buildID string) ConfigResults {
	if r, ok := bi.cache[buildID]; ok {
		return r
	}
	r := aggregateUnits(bi.units[buildID], nil)
	bi.cache[buildID] = r
	return r
}

// BuildCoverage is the coverage of a requirement in one build.
type BuildCoverage struct {
	Build    model.Build `json:"build"`
	Coverage string      `json:"coverage"`
}

// RequirementBuilds returns, newest first, the builds in which a linked
// test was executed with the requirement's coverage in each (verdict nil:
// CoverageOf), and the latest build in which it was verified (passed).
func (bi *BuildIndex) RequirementBuilds(requirementID string, tests []string, verdict CoverageFunc) ([]BuildCoverage, *model.Build) {
	if verdict == nil {
		verdict = DefaultCoverage
	}
	out := []BuildCoverage{}
	var verified *model.Build
	for _, b := range bi.Builds {
		res := bi.Results(b.ID)
		hit := false
		for _, t := range tests {
			if _, ok := res[t]; ok {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		cov := verdict(requirementID, tests, res)
		out = append(out, BuildCoverage{Build: b, Coverage: cov})
		if verified == nil && cov == model.ResultPassed {
			v := b
			verified = &v
		}
	}
	return out, verified
}

// TestBuild is the result of a test in one build.
type TestBuild struct {
	Build  model.Build `json:"build"`
	Result string      `json:"result"`
	// Results per configuration (key "" for results without one).
	Results map[string]string `json:"results"`
}

// TestHistory returns the builds a test was executed in, newest first, with
// its combined result and the result per configuration.
func (bi *BuildIndex) TestHistory(testID string) []TestBuild {
	out := []TestBuild{}
	for _, b := range bi.Builds {
		res := bi.Results(b.ID)
		units, ok := res[testID]
		if !ok {
			continue
		}
		st := res.Combined(testID)
		out = append(out, TestBuild{Build: b, Result: st, Results: units})
		if len(out) >= MaxBuildHistory {
			break
		}
	}
	return out
}

// ---------------------------------------------------------------- code changes

func scanCodeChange(r interface{ Scan(...any) error }) (*model.CodeChange, error) {
	var c model.CodeChange
	if err := r.Scan(&c.ID, &c.ProjectID, &c.Kind, &c.Provider, &c.Repository, &c.Ref, &c.Title, &c.URL, &c.Author, &c.Branch, &c.TargetBranch, &c.Status, &c.OccurredAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Items = []model.CodeChangeItem{}
	return &c, nil
}

// ValidateCodeChange normalises and checks a code change (without saving it).
func ValidateCodeChange(c *model.CodeChange) error {
	c.Kind = strings.TrimSpace(c.Kind)
	c.Provider = strings.TrimSpace(c.Provider)
	if c.Provider == "" {
		c.Provider = model.ProviderManual
	}
	if c.Kind != model.CodeCommit && c.Kind != model.CodeMergeRequest {
		return Invalidf("kind must be commit or merge_request")
	}
	if !slices.Contains([]string{model.ProviderGitLab, model.ProviderGitHub, model.ProviderManual}, c.Provider) {
		return Invalidf("provider must be gitlab, github or manual")
	}
	c.Repository = cleanText(c.Repository, 250)
	c.Ref = strings.TrimPrefix(cleanText(c.Ref, 100), "!")
	c.Ref = strings.TrimPrefix(c.Ref, "#")
	c.URL = strings.TrimSpace(c.URL)
	if c.Ref == "" && c.URL == "" {
		return Invalidf("a code change needs its commit SHA, MR / PR number or URL")
	}
	if c.Ref == "" {
		c.Ref = c.URL
		if len(c.Ref) > 100 {
			c.Ref = truncate(c.Ref, 100)
		}
	}
	if c.Kind == model.CodeCommit && c.Provider != model.ProviderManual && !shaRe.MatchString(c.Ref) {
		return Invalidf("the commit must be a hexadecimal SHA")
	}
	if c.Kind == model.CodeCommit && shaRe.MatchString(c.Ref) {
		c.Ref = strings.ToLower(c.Ref)
	}
	if !validURL(c.URL) {
		return Invalidf("url must be an http(s) URL")
	}
	// the first line of a commit message is its title
	title, _, _ := strings.Cut(strings.TrimSpace(c.Title), "\n")
	c.Title = cleanText(title, MaxCodeText)
	c.Author = cleanText(c.Author, 200)
	c.Branch = cleanText(strings.TrimPrefix(strings.TrimSpace(c.Branch), "refs/heads/"), 250)
	c.TargetBranch = cleanText(c.TargetBranch, 250)
	c.Status = strings.TrimSpace(c.Status)
	if c.Kind == model.CodeCommit {
		c.Status = ""
	} else if c.Status == "" {
		c.Status = "open"
	} else if !slices.Contains(model.CodeChangeStatuses, c.Status) {
		return Invalidf("status must be open, merged or closed")
	}
	if utf8.RuneCountInString(c.Title) > MaxCodeText {
		return Invalidf("title is too long")
	}
	return nil
}

// UpsertCodeChange saves a code change: a new one, or the change with the
// same provider, repository, kind and ref (its title, URL, status and
// branches are updated; when it first happened is kept). c is filled with
// the stored change.
func (s *Store) UpsertCodeChange(ctx context.Context, tenantID string, c *model.CodeChange) (bool, error) {
	if err := ValidateCodeChange(c); err != nil {
		return false, err
	}
	now := s.now()
	if c.OccurredAt.IsZero() {
		c.OccurredAt = now
	}
	created := false
	err := s.tx(ctx, func(cn conn) error {
		cur, err := scanCodeChange(cn.row(ctx, `SELECT `+codeChangeColumns+` FROM code_changes WHERE project_id = ? AND provider = ? AND repository = ? AND kind = ? AND ref = ?`,
			c.ProjectID, c.Provider, c.Repository, c.Kind, c.Ref))
		if db.IsNoRows(err) {
			created = true
			c.ID, c.CreatedAt, c.UpdatedAt = newID(), now, now
			_, err = cn.exec(ctx, `INSERT INTO code_changes (id, tenant_id, project_id, kind, provider, repository, ref, title, url, author, branch, target_branch, status, occurred_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				c.ID, tenantID, c.ProjectID, c.Kind, c.Provider, c.Repository, c.Ref, c.Title, c.URL, c.Author, c.Branch, c.TargetBranch, c.Status, c.OccurredAt, now, now)
			return err
		}
		if err != nil {
			return err
		}
		for _, f := range []struct{ src, dst *string }{{&c.Title, &cur.Title}, {&c.URL, &cur.URL}, {&c.Author, &cur.Author}, {&c.Branch, &cur.Branch}, {&c.TargetBranch, &cur.TargetBranch}, {&c.Status, &cur.Status}} {
			if *f.src != "" {
				*f.dst = *f.src
			}
		}
		cur.UpdatedAt = now
		*c = *cur
		_, err = cn.exec(ctx, `UPDATE code_changes SET title = ?, url = ?, author = ?, branch = ?, target_branch = ?, status = ?, updated_at = ? WHERE id = ?`,
			c.Title, c.URL, c.Author, c.Branch, c.TargetBranch, c.Status, now, c.ID)
		return err
	})
	return created, err
}

// LinkCodeChange links items to a code change (pairs that exist are kept
// as they are) and returns the number added.
func (s *Store) LinkCodeChange(ctx context.Context, tenantID, changeID string, items []model.CodeChangeItem, source, user string) (int, error) {
	n := 0
	now := s.now()
	err := s.tx(ctx, func(c conn) error {
		for _, it := range items {
			var k int
			if err := c.row(ctx, `SELECT COUNT(*) FROM code_change_items WHERE change_id = ? AND item_type = ? AND item_id = ?`, changeID, it.ItemType, it.ItemID).Scan(&k); err != nil {
				return err
			}
			if k > 0 {
				if source == model.CodeLinkManual {
					// a link made by hand stays when the text no longer mentions the key
					if _, err := c.exec(ctx, `UPDATE code_change_items SET source = ? WHERE change_id = ? AND item_type = ? AND item_id = ?`, source, changeID, it.ItemType, it.ItemID); err != nil {
						return err
					}
				}
				continue
			}
			if _, err := c.exec(ctx, `INSERT INTO code_change_items (change_id, item_type, item_id, tenant_id, source, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				changeID, it.ItemType, it.ItemID, tenantID, source, user, now); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// UnlinkCodeChange removes the link between a code change and an item; a
// manual change linked to nothing else is removed with it.
func (s *Store) UnlinkCodeChange(ctx context.Context, tenantID, changeID, itemType, itemID string) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `DELETE FROM code_change_items WHERE change_id = ? AND item_type = ? AND item_id = ? AND tenant_id = ?`, changeID, itemType, itemID, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "code link")
		}
		_, err = c.exec(ctx, `DELETE FROM code_changes WHERE id = ? AND tenant_id = ? AND provider = ? AND NOT EXISTS (SELECT 1 FROM code_change_items i WHERE i.change_id = code_changes.id)`,
			changeID, tenantID, model.ProviderManual)
		return err
	})
}

// GetCodeChange returns one code change with its items.
func (s *Store) GetCodeChange(ctx context.Context, tenantID, id string) (*model.CodeChange, error) {
	c, err := scanCodeChange(s.c(ctx).row(ctx, `SELECT `+codeChangeColumns+` FROM code_changes WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "code change")
	}
	list := []model.CodeChange{*c}
	if err := s.codeChangeItems(ctx, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// CodeChangeFilter selects code changes of a project.
type CodeChangeFilter struct {
	// ItemType and ItemID: only changes linked to this requirement or test.
	ItemType, ItemID string
	// Kind: commit or merge_request ("" both).
	Kind string
	// Since: only changes that happened after this time.
	Since *time.Time
	// Branch: only changes on this branch ("" any).
	Branch string
	Limit  int
}

// ListCodeChanges returns code changes of a project, newest first, with
// their items and (commits) builds.
func (s *Store) ListCodeChanges(ctx context.Context, tenantID, projectID string, f CodeChangeFilter) ([]model.CodeChange, error) {
	where := `c.tenant_id = ? AND c.project_id = ?`
	args := []any{tenantID, projectID}
	if f.ItemID != "" {
		where += ` AND EXISTS (SELECT 1 FROM code_change_items i WHERE i.change_id = c.id AND i.item_type = ? AND i.item_id = ?)`
		args = append(args, f.ItemType, f.ItemID)
	}
	if f.Kind != "" {
		where += ` AND c.kind = ?`
		args = append(args, f.Kind)
	}
	if f.Since != nil {
		where += ` AND c.occurred_at > ?`
		args = append(args, *f.Since)
	}
	if f.Branch != "" {
		where += ` AND c.branch = ?`
		args = append(args, f.Branch)
	}
	limit := f.Limit
	if limit <= 0 || limit > MaxCodeListLimit {
		limit = defaultCodeLimit
	}
	args = append(args, limit)
	cols := "c." + strings.ReplaceAll(codeChangeColumns, ", ", ", c.")
	rows, err := s.c(ctx).query(ctx, `SELECT `+cols+` FROM code_changes c WHERE `+where+` ORDER BY c.occurred_at DESC, c.ref LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	out := []model.CodeChange{}
	for rows.Next() {
		c, err := scanCodeChange(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.codeChangeItems(ctx, out); err != nil {
		return nil, err
	}
	return out, s.commitBuilds(ctx, tenantID, projectID, out)
}

// codeChangeItems fills the items of code changes (with keys and titles).
func (s *Store) codeChangeItems(ctx context.Context, list []model.CodeChange) error {
	if len(list) == 0 {
		return nil
	}
	idx := map[string]int{}
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].ID
		idx[list[i].ID] = i
	}
	rows, err := s.c(ctx).query(ctx, `SELECT i.change_id, i.item_type, i.item_id, i.source, i.created_by, i.created_at,
		COALESCE(r.key, t.key, ''), COALESCE(r.title, t.title, '')
		FROM code_change_items i
		LEFT JOIN requirements r ON i.item_type = 'requirement' AND r.id = i.item_id
		LEFT JOIN test_cases t ON i.item_type = 'test' AND t.id = i.item_id
		WHERE i.change_id IN (`+placeholders(len(ids))+`)`, anySlice(ids)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid string
		var it model.CodeChangeItem
		if err := rows.Scan(&cid, &it.ItemType, &it.ItemID, &it.Source, &it.CreatedBy, &it.CreatedAt, &it.Key, &it.Title); err != nil {
			return err
		}
		list[idx[cid]].Items = append(list[idx[cid]].Items, it)
	}
	for i := range list {
		sort.SliceStable(list[i].Items, func(a, b int) bool { return naturalLess(list[i].Items[a].Key, list[i].Items[b].Key) })
	}
	return rows.Err()
}

// commitBuilds fills the builds of the commits among the changes.
func (s *Store) commitBuilds(ctx context.Context, tenantID, projectID string, list []model.CodeChange) error {
	var shas []string
	for _, c := range list {
		if c.Kind == model.CodeCommit && shaRe.MatchString(c.Ref) {
			shas = append(shas, c.Ref)
		}
	}
	if len(shas) == 0 {
		return nil
	}
	shas = uniq(shas)
	rows, err := s.c(ctx).query(ctx, `SELECT `+buildColumns+` FROM builds WHERE tenant_id = ? AND project_id = ? AND commit_sha IN (`+placeholders(len(shas))+`) ORDER BY created_at DESC`,
		append([]any{tenantID, projectID}, anySlice(shas)...)...)
	if err != nil {
		return err
	}
	by := map[string][]model.Build{}
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			rows.Close()
			return err
		}
		by[b.CommitSHA] = append(by[b.CommitSHA], *b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range list {
		if list[i].Kind == model.CodeCommit {
			list[i].Builds = by[list[i].Ref]
		}
	}
	return nil
}

// ChangesSinceBuild returns the commits of a project after a build: those
// that happened after the build's own commit (or after the build was
// recorded, when its commit is unknown), on the build's branch when it has
// one.
func (s *Store) ChangesSinceBuild(ctx context.Context, tenantID string, b *model.Build) ([]model.CodeChange, error) {
	since := b.CreatedAt
	if b.CommitSHA != "" {
		var at time.Time
		err := s.c(ctx).row(ctx, `SELECT occurred_at FROM code_changes WHERE tenant_id = ? AND project_id = ? AND kind = ? AND ref = ? ORDER BY occurred_at LIMIT 1`,
			tenantID, b.ProjectID, model.CodeCommit, b.CommitSHA).Scan(&at)
		if err == nil {
			since = at
		} else if !db.IsNoRows(err) {
			return nil, err
		}
	}
	return s.ListCodeChanges(ctx, tenantID, b.ProjectID, CodeChangeFilter{Kind: model.CodeCommit, Since: &since, Branch: b.Branch, Limit: MaxCodeListLimit})
}

// ---------------------------------------------------------------- settings

// CodeSettings returns a project's code integration including the secret
// (for the server's own use; the API masks it).
func (s *Store) CodeSettings(ctx context.Context, tenantID, projectID string) (*model.CodeSettings, error) {
	if _, err := s.GetProject(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	cs := &model.CodeSettings{}
	var secret string
	var at time.Time
	err := s.c(ctx).row(ctx, `SELECT webhook_secret, updated_by, updated_at FROM code_settings WHERE project_id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&secret, &cs.UpdatedBy, &at)
	if db.IsNoRows(err) {
		return cs, nil
	}
	if err != nil {
		return nil, err
	}
	cs.UpdatedAt = &at
	if cs.Secret, err = s.unseal(ctx, secret, adCodeSecret(projectID)); err != nil {
		return nil, fmt.Errorf("webhook secret: %w", err)
	}
	cs.HasSecret = cs.Secret != ""
	return cs, nil
}

// SaveCodeSettings sets ("" keeps it unless clear) the webhook secret of a project.
func (s *Store) SaveCodeSettings(ctx context.Context, tenantID, projectID, secret string, clear bool, user string) (*model.CodeSettings, error) {
	cur, err := s.CodeSettings(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	secret = strings.TrimSpace(secret)
	switch {
	case clear:
		secret = ""
	case secret == "":
		secret = cur.Secret
	case len(secret) < MinWebhookSecret || len(secret) > MaxWebhookSecret:
		return nil, Invalidf("the webhook secret must have %d to %d characters", MinWebhookSecret, MaxWebhookSecret)
	}
	sealed, err := s.seal(secret, adCodeSecret(projectID))
	if err != nil {
		return nil, err
	}
	now := s.now()
	err = s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM code_settings WHERE project_id = ?`, projectID); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO code_settings (project_id, tenant_id, webhook_secret, updated_by, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, tenantID, sealed, user, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &model.CodeSettings{Secret: secret, HasSecret: secret != "", UpdatedBy: user, UpdatedAt: &now}, nil
}

// WebhookProject returns the tenant and webhook secret of a project for an
// incoming webhook ("" secret: not configured). An unknown project is
// ErrNotFound.
func (s *Store) WebhookProject(ctx context.Context, projectID string) (*model.Project, string, error) {
	var tenantID string
	if err := s.c(ctx).row(ctx, `SELECT tenant_id FROM projects WHERE id = ?`, projectID).Scan(&tenantID); err != nil {
		return nil, "", mapErr(err, "project")
	}
	p, err := s.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, "", err
	}
	cs, err := s.CodeSettings(ctx, tenantID, projectID)
	if err != nil {
		return nil, "", err
	}
	return p, cs.Secret, nil
}
