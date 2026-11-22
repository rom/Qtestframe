package model

import "time"

// Custom fields: project-defined fields of requirements, tests, defects,
// risks and runs (see internal/customfield for the value rules).

// CustomFieldEntities are the item kinds that can have custom fields.
var CustomFieldEntities = []string{KindRequirement, KindTest, KindDefect, KindRisk, "run"}

// Custom field types.
const (
	FieldText         = "text"
	FieldLongText     = "long_text"
	FieldNumber       = "number"
	FieldDate         = "date"
	FieldSingleSelect = "single_select"
	FieldMultiSelect  = "multi_select"
	FieldCheckbox     = "checkbox"
	FieldUser         = "user"
	FieldURL          = "url"
)

// CustomFieldTypes lists every custom field type.
var CustomFieldTypes = []string{FieldText, FieldLongText, FieldNumber, FieldDate, FieldSingleSelect,
	FieldMultiSelect, FieldCheckbox, FieldUser, FieldURL}

// CustomValues are the custom field values of an item: field key → value
// (string, float64, bool or []string after validation; []any when decoded
// from JSON).
type CustomValues map[string]any

// CustomField is the definition of a custom field: of a project
// (ProjectID) or of an organisation's field set for a project type
// (ProjectType).
type CustomField struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id,omitempty"`
	ProjectType string `json:"project_type,omitempty"`
	// EntityType is one of CustomFieldEntities.
	EntityType string `json:"entity_type"`
	// Key is the stable name ([a-z][a-z0-9_]*, at most 40 characters) used
	// in queries (cf.<key>), exports and the stored values; it cannot be
	// changed.
	Key   string `json:"key"`
	Label string `json:"label"`
	// Type is one of CustomFieldTypes.
	Type string `json:"type"`
	// Unit of a number field (e.g. "kg"), shown after the value.
	Unit string `json:"unit"`
	// Options are the choices of a select field.
	Options  []string `json:"options"`
	Required bool     `json:"required"`
	// Default is the value a new item gets (nil for none).
	Default  any    `json:"default"`
	Help     string `json:"help"`
	Position int    `json:"position"`
	// ShowInList shows the field as a column in the lists by default.
	ShowInList bool      `json:"show_in_list"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CustomFieldArchive holds the values of a deleted custom field.
type CustomFieldArchive struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	EntityType string            `json:"entity_type"`
	FieldKey   string            `json:"field_key"`
	Label      string            `json:"label"`
	Definition CustomField       `json:"definition"`
	Values     []ArchivedCFValue `json:"values,omitempty"`
	ItemCount  int               `json:"item_count"`
	DeletedBy  string            `json:"deleted_by"`
	DeletedAt  time.Time         `json:"deleted_at"`
}

// ArchivedCFValue is the value one item had in a deleted custom field.
type ArchivedCFValue struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// Template sources.
const (
	TemplateProject      = "project"
	TemplateOrganisation = "organisation"
	TemplateBuiltin      = "builtin"
)

// ItemTemplate pre-fills a new requirement or test.
type ItemTemplate struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id,omitempty"`
	ProjectType string `json:"project_type,omitempty"`
	// Kind is requirement or test.
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Source: project, organisation or builtin (derived).
	Source    string          `json:"source"`
	Content   TemplateContent `json:"content"`
	Position  int             `json:"position"`
	CreatedBy string          `json:"created_by,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	// Default: the project's default template for its kind (derived).
	Default bool `json:"default,omitempty"`
}

// TemplateContent are the fields a template fills in.
type TemplateContent struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Status      string   `json:"status,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// requirements
	Obligation string                `json:"obligation,omitempty"`
	Type       string                `json:"type,omitempty"`
	Source     string                `json:"source,omitempty"`
	Rationale  string                `json:"rationale,omitempty"`
	Criteria   []EvaluationCriterion `json:"criteria,omitempty"`
	// tests
	Preconditions  string     `json:"preconditions,omitempty"`
	ExpectedResult string     `json:"expected_result,omitempty"`
	ExecutionType  string     `json:"execution_type,omitempty"`
	EstimatedMin   int        `json:"estimated_minutes,omitempty"`
	Steps          []TestStep `json:"steps,omitempty"`
	// CustomFields are values by field key (fields the project does not
	// have are ignored when the template is used).
	CustomFields CustomValues `json:"custom_fields,omitempty"`
}
