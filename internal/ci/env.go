package ci

import (
	"strings"
)

// CI systems the commands recognise.
const (
	SystemGitLab  = "gitlab"
	SystemGitHub  = "github"
	SystemJenkins = "jenkins"
	SystemGeneric = "generic"
)

// Env is what a command learns from the CI system it runs in.
type Env struct {
	// System: gitlab, github, jenkins, generic ("" not in CI).
	System string
	// Build: the pipeline / workflow run (external id unique per project),
	// its number, URL, commit and branch.
	BuildID, BuildNumber, PipelineURL string
	Commit, Branch                    string
	// MergeRequest: the merge request IID (GitLab) or pull request number
	// (GitHub) of a pipeline for one.
	MergeRequest string
	// DiffBase: the commit the change is compared with (GitLab's
	// CI_MERGE_REQUEST_DIFF_BASE_SHA, the pull request's base).
	DiffBase string
	// APIURL and Project of the code host (GitLab: CI_API_V4_URL and
	// CI_PROJECT_ID; GitHub: GITHUB_API_URL and GITHUB_REPOSITORY).
	APIURL, Project string
	// PipelineID is GitLab's CI_PIPELINE_ID (statuses join the pipeline).
	PipelineID string
	// JobToken: CI_JOB_TOKEN (GitLab) — accepted only for some API
	// endpoints, see the user guide.
	JobToken string
}

// Detect reads the CI environment from getenv (os.Getenv). Variables set
// to override it: QTF_COMMIT, QTF_BRANCH, QTF_MERGE_REQUEST, QTF_BUILD_ID,
// QTF_BUILD_NUMBER, QTF_PIPELINE_URL, QTF_DIFF_BASE.
func Detect(getenv func(string) string) Env {
	g := func(k string) string { return strings.TrimSpace(getenv(k)) }
	var e Env
	switch {
	case g("GITLAB_CI") != "":
		e = Env{System: SystemGitLab, BuildID: g("CI_PIPELINE_ID"), BuildNumber: g("CI_PIPELINE_IID"), PipelineURL: g("CI_PIPELINE_URL"),
			Commit: g("CI_COMMIT_SHA"), Branch: first(g("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"), g("CI_COMMIT_BRANCH"), g("CI_COMMIT_REF_NAME")),
			MergeRequest: g("CI_MERGE_REQUEST_IID"), DiffBase: g("CI_MERGE_REQUEST_DIFF_BASE_SHA"),
			APIURL: g("CI_API_V4_URL"), Project: g("CI_PROJECT_ID"), PipelineID: g("CI_PIPELINE_ID"), JobToken: g("CI_JOB_TOKEN")}
	case g("GITHUB_ACTIONS") != "":
		e = Env{System: SystemGitHub, BuildID: g("GITHUB_RUN_ID"), BuildNumber: g("GITHUB_RUN_NUMBER"),
			Commit: g("GITHUB_SHA"), Branch: first(g("GITHUB_HEAD_REF"), g("GITHUB_REF_NAME")),
			APIURL: g("GITHUB_API_URL"), Project: g("GITHUB_REPOSITORY")}
		if s, r, id := g("GITHUB_SERVER_URL"), g("GITHUB_REPOSITORY"), g("GITHUB_RUN_ID"); s != "" && r != "" && id != "" {
			e.PipelineURL = s + "/" + r + "/actions/runs/" + id
		}
		// refs/pull/<n>/merge
		if ref := g("GITHUB_REF"); strings.HasPrefix(ref, "refs/pull/") {
			e.MergeRequest = strings.TrimSuffix(strings.TrimPrefix(ref, "refs/pull/"), "/merge")
		}
	case g("JENKINS_URL") != "":
		e = Env{System: SystemJenkins, BuildID: first(g("BUILD_TAG"), g("BUILD_NUMBER")), BuildNumber: g("BUILD_NUMBER"), PipelineURL: g("BUILD_URL"),
			Commit: g("GIT_COMMIT"), Branch: first(g("CHANGE_BRANCH"), g("BRANCH_NAME"), g("GIT_BRANCH")), MergeRequest: g("CHANGE_ID")}
	case g("CI") != "":
		e = Env{System: SystemGeneric}
	}
	for _, o := range []struct {
		dst *string
		key string
	}{{&e.Commit, "QTF_COMMIT"}, {&e.Branch, "QTF_BRANCH"}, {&e.MergeRequest, "QTF_MERGE_REQUEST"}, {&e.BuildID, "QTF_BUILD_ID"},
		{&e.BuildNumber, "QTF_BUILD_NUMBER"}, {&e.PipelineURL, "QTF_PIPELINE_URL"}, {&e.DiffBase, "QTF_DIFF_BASE"}} {
		if v := g(o.key); v != "" {
			*o.dst = v
			if e.System == "" {
				e.System = SystemGeneric
			}
		}
	}
	return e
}

func first(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// InCI reports whether a CI system was detected.
func (e Env) InCI() bool { return e.System != "" }
