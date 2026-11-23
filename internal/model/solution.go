package model

import (
	"strings"
	"time"
)

// KindSolutionPreamble names a project's solution preamble in the history
// (revisions with entity_id = the project id).
const KindSolutionPreamble = "solution_preamble"

// Architecture styles of a solution preamble ("" not chosen).
var ArchitectureStyles = []string{"monolith", "modular_monolith", "microservices", "serverless", "event_driven", "layered", "hexagonal", "other"}

// SolutionPreamble describes the intended solution of a software project:
// languages, stack, environments, constraints and conventions. Solution
// prompts (architecture, design, implementation plan, full solution) start
// with it. It holds descriptions only: no passwords, keys or tokens
// (connection details are described, never given); see
// prompt.CheckPreamble for the warnings. Stored in projects.solution_preamble.
type SolutionPreamble struct {
	// Template is the id of the template it was prefilled from ("" none).
	Template string `json:"template"`
	// Languages: implementation languages and versions.
	Languages string `json:"languages"`
	// Frameworks: frameworks and libraries.
	Frameworks string `json:"frameworks"`
	// Environments: target environments (cloud, on-premises, containers,
	// Kubernetes, operating systems, browsers, devices).
	Environments string `json:"environments"`
	// ArchitectureStyle is one of ArchitectureStyles or "".
	ArchitectureStyle string `json:"architecture_style"`
	// ArchitectureNotes add to the style.
	ArchitectureNotes string `json:"architecture_notes"`
	// The technology stack.
	Frontend  string `json:"frontend"`
	Backend   string `json:"backend"`
	Messaging string `json:"messaging"`
	Caching   string `json:"caching"`
	// Databases: engines, versions, schema ownership, migrations tool, how
	// the application connects — described, without credentials.
	Databases string `json:"databases"`
	// Integrations: external systems and APIs.
	Integrations string `json:"integrations"`
	// Deployment: deployment, CI/CD.
	Deployment string `json:"deployment"`
	// Non-functional targets.
	Performance  string `json:"performance"`
	Availability string `json:"availability"`
	Scalability  string `json:"scalability"`
	// Security: security and compliance constraints (authentication, data
	// protection, regulations).
	Security string `json:"security"`
	// Conventions: coding conventions.
	Conventions string `json:"conventions"`
	// Testing: the testing strategy.
	Testing string `json:"testing"`
	// Repository: the repository layout.
	Repository string `json:"repository"`
	// OutOfScope lists what the solution does not cover.
	OutOfScope string `json:"out_of_scope"`
	// Additional is free-text additional context.
	Additional string `json:"additional"`

	// Bookkeeping (set by the server).
	Version   int        `json:"version"`
	UpdatedAt *time.Time `json:"updated_at"`
	UpdatedBy string     `json:"updated_by"`
}

// PreambleFields lists the text fields of a solution preamble in display
// order (JSON names; architecture_style is a choice, not listed).
var PreambleFields = []string{"languages", "frameworks", "environments", "architecture_notes", "frontend", "backend", "messaging",
	"caching", "databases", "integrations", "deployment", "performance", "availability", "scalability", "security", "conventions",
	"testing", "repository", "out_of_scope", "additional"}

// MaxPreambleField is the longest text field of a preamble, MaxPreambleAdditional
// the longest additional context.
const (
	MaxPreambleField      = 4000
	MaxPreambleAdditional = 16000
)

// Field returns a pointer to a text field by its JSON name (nil: unknown).
func (p *SolutionPreamble) Field(name string) *string {
	switch name {
	case "languages":
		return &p.Languages
	case "frameworks":
		return &p.Frameworks
	case "environments":
		return &p.Environments
	case "architecture_notes":
		return &p.ArchitectureNotes
	case "frontend":
		return &p.Frontend
	case "backend":
		return &p.Backend
	case "messaging":
		return &p.Messaging
	case "caching":
		return &p.Caching
	case "databases":
		return &p.Databases
	case "integrations":
		return &p.Integrations
	case "deployment":
		return &p.Deployment
	case "performance":
		return &p.Performance
	case "availability":
		return &p.Availability
	case "scalability":
		return &p.Scalability
	case "security":
		return &p.Security
	case "conventions":
		return &p.Conventions
	case "testing":
		return &p.Testing
	case "repository":
		return &p.Repository
	case "out_of_scope":
		return &p.OutOfScope
	case "additional":
		return &p.Additional
	}
	return nil
}

// Normalize trims the text fields (keeping inner line breaks).
func (p *SolutionPreamble) Normalize() {
	for _, f := range PreambleFields {
		v := p.Field(f)
		*v = strings.TrimSpace(strings.ReplaceAll(*v, "\r\n", "\n"))
	}
	p.ArchitectureStyle = strings.TrimSpace(p.ArchitectureStyle)
	p.Template = strings.TrimSpace(p.Template)
}

// Empty reports whether nothing is filled in.
func (p *SolutionPreamble) Empty() bool {
	if p.ArchitectureStyle != "" {
		return false
	}
	for _, f := range PreambleFields {
		if strings.TrimSpace(*p.Field(f)) != "" {
			return false
		}
	}
	return true
}

// Filled counts the filled-in fields (the style included).
func (p *SolutionPreamble) Filled() int {
	n := 0
	if p.ArchitectureStyle != "" {
		n++
	}
	for _, f := range PreambleFields {
		if strings.TrimSpace(*p.Field(f)) != "" {
			n++
		}
	}
	return n
}
