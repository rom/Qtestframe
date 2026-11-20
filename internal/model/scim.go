package model

import "time"

// SCIMToken authenticates an identity provider at /scim/v2 for one tenant.
// It is not an API token: neither works in place of the other.
type SCIMToken struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenant_id"`
	Name      string     `json:"name"`
	Hash      string     `json:"-"`
	Prefix    string     `json:"prefix"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
}

// SCIMSettings configure the provisioning of a tenant.
type SCIMSettings struct {
	// DefaultRole is given to provisioned users without a mapped group;
	// "" means such users get no access.
	DefaultRole string `json:"default_role"`
}

// SCIMUser is a user provisioned into a tenant, with the user's fields.
type SCIMUser struct {
	TenantID    string    `json:"tenant_id"`
	UserID      string    `json:"user_id"`
	UserName    string    `json:"user_name"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	ExternalID  string    `json:"external_id"`
	GivenName   string    `json:"given_name"`
	FamilyName  string    `json:"family_name"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SCIMGroup is a group pushed by the identity provider. Its members get
// Role in the tenant ("" = not mapped).
type SCIMGroup struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	DisplayName string    `json:"display_name"`
	ExternalID  string    `json:"external_id"`
	Role        string    `json:"role"`
	Members     []string  `json:"members"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
