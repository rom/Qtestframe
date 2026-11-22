package ci

import (
	"embed"
	"strings"
)

//go:embed templates/*.yml
var templateFS embed.FS

// Template is a ready-made pipeline definition.
type Template struct {
	// Name: the file name (gitlab-ci.yml, …).
	Name string `json:"name"`
	// Kind: gitlab | gitlab-selection | gitlab-component | github.
	Kind string `json:"kind"`
	// File: where it goes in the repository.
	File string `json:"file"`
}

// Templates lists the templates in the order the CI setup shows them.
var Templates = []Template{
	{Name: "gitlab-ci.yml", Kind: "gitlab", File: ".gitlab-ci.yml"},
	{Name: "gitlab-ci-selection.yml", Kind: "gitlab-selection", File: ".gitlab-ci.yml"},
	{Name: "gitlab-component.yml", Kind: "gitlab-component", File: "templates/qtestframe.yml"},
	{Name: "github-actions.yml", Kind: "github", File: ".github/workflows/qtestframe.yml"},
}

// Example values of the templates in examples/ci.
const (
	ExampleServerURL = "https://qtestframe.example.com"
	ExampleProject   = "SHOP"
)

// RenderTemplate returns a template with the server URL and project key
// filled in (ok false for an unknown name).
func RenderTemplate(name, serverURL, projectKey string) (string, bool) {
	b, err := templateFS.ReadFile("templates/" + name)
	if err != nil || strings.Contains(name, "/") {
		return "", false
	}
	if serverURL == "" {
		serverURL = ExampleServerURL
	}
	if projectKey == "" {
		projectKey = ExampleProject
	}
	return strings.NewReplacer("__QTF_URL__", strings.TrimRight(serverURL, "/"), "__QTF_PROJECT__", projectKey).Replace(string(b)), true
}
