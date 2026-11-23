package model

import (
	"encoding/json"
	"time"
)

// Dashboard is a layout of widgets on a grid of DashboardColumns columns.
// A project dashboard is shared (OwnerID "") or personal; a home dashboard
// (ProjectID "") is the personal cross-project one of its owner.
type Dashboard struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id,omitempty"`
	OwnerID   string `json:"owner_id,omitempty"`
	Name      string `json:"name"`
	// Builtin is the key of the built-in layout a shared dashboard replaces
	// (or a personal one was copied from); SourceID the dashboard it was
	// copied from. Reset returns to that layout.
	Builtin   string            `json:"builtin,omitempty"`
	SourceID  string            `json:"source_id,omitempty"`
	Widgets   []DashboardWidget `json:"widgets"`
	Position  int               `json:"position"`
	CreatedBy string            `json:"created_by,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`

	// computed for the caller
	Shared bool `json:"shared"`
	// Stored is false for a built-in layout nobody has changed yet.
	Stored   bool `json:"stored"`
	Default  bool `json:"default"`
	Editable bool `json:"editable"`
}

// DashboardWidget is one widget of a dashboard: its type (see the widget
// registry of the API), title (empty: the type's name), place and size in
// grid units and settings (per type, e.g. the release or the query).
type DashboardWidget struct {
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
	Title    string                     `json:"title,omitempty"`
	X        int                        `json:"x"`
	Y        int                        `json:"y"`
	W        int                        `json:"w"`
	H        int                        `json:"h"`
	Settings map[string]json.RawMessage `json:"settings,omitempty"`
}

// Dashboard grid limits.
const (
	DashboardColumns    = 12
	DashboardMaxHeight  = 24
	DashboardMaxRows    = 400
	DashboardMaxWidgets = 60
	// DashboardMaxPerUser bounds the personal dashboards of a user in a
	// project and the shared ones of a project.
	DashboardMaxPerUser = 30
)

// DashboardExportFormat names the JSON export of a dashboard.
const DashboardExportFormat = "qtestframe-dashboard"
