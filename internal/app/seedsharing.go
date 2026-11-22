package app

import (
	"context"
	"errors"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// DemoUserPassword is the password of the demo users of seed-demo.
const DemoUserPassword = "Demo-user-1234"

// Demo users of seed-demo: Dana is a viewer in the restricted project,
// Erik is not a member of it (both are testers in the organisation).
const (
	DemoMemberEmail   = "dana@example.com"
	DemoOutsiderEmail = "erik@example.com"
	// DemoSMEEmail is Sara, a subject matter expert who also tests: she
	// holds the roles sme and tester in the organisation, and sme and
	// reviewer in the restricted project.
	DemoSMEEmail = "sara@example.com"
)

// demoUser returns the user with email, creating it (with
// DemoUserPassword) when missing, and makes it a member of the tenant
// with roles (keeping an existing membership's roles).
func demoUser(ctx context.Context, st *store.Store, tenantID, email, name string, roles ...string) (*model.User, error) {
	u, err := st.GetUserByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		hash, herr := auth.HashPassword(DemoUserPassword)
		if herr != nil {
			return nil, herr
		}
		u = &model.User{Email: email, DisplayName: name, PasswordHash: hash}
		err = st.CreateUser(ctx, u)
	}
	if err != nil {
		return nil, err
	}
	if _, err := st.GetMembership(ctx, tenantID, u.ID); errors.Is(err, store.ErrNotFound) {
		err = st.SetMembershipRoles(ctx, tenantID, u.ID, roles)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return u, nil
}

// SeedDemoSharing adds a small restricted project ("VAULT") shared with
// DemoMemberEmail as a viewer, so project sharing can be tried out: Dana
// sees it read-only, Erik does not see it at all, organisation admins see
// it as always. It does nothing (nil) when the tenant already has a
// project VAULT.
func SeedDemoSharing(ctx context.Context, st *store.Store, tenantID string) (*model.Project, error) {
	dana, err := demoUser(ctx, st, tenantID, DemoMemberEmail, "Dana Lind", rbac.RoleTester)
	if err != nil {
		return nil, err
	}
	if _, err := demoUser(ctx, st, tenantID, DemoOutsiderEmail, "Erik Berg", rbac.RoleTester); err != nil {
		return nil, err
	}
	sara, err := demoUser(ctx, st, tenantID, DemoSMEEmail, "Sara Ek", rbac.RoleSME, rbac.RoleTester)
	if err != nil {
		return nil, err
	}
	ps, err := st.ListProjects(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	const key = "VAULT"
	for _, p := range ps {
		if p.Key == key {
			return nil, nil
		}
	}
	p := &model.Project{Key: key, Name: "Card vault (restricted)", Color: "#7c3aed",
		Description: "A small confidential project: only its members and the organisation's administrators see it (Project settings → Sharing)."}
	if err := st.CreateProject(ctx, tenantID, p, false); err != nil {
		return nil, err
	}
	var reqs []*model.Requirement
	for _, x := range []struct{ title, desc string }{
		{"Card numbers are encrypted at rest", "The vault MUST store card numbers encrypted with AES-256."},
		{"Key rotation", "Encryption keys SHALL be rotated every 90 days."},
	} {
		r := &model.Requirement{ProjectID: p.ID, Title: x.title, Description: x.desc, Obligation: model.ObligationMust,
			Type: "security", Priority: "high", Status: "approved", CreatedBy: "seed"}
		if err := st.CreateRequirement(ctx, tenantID, r); err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	t := &model.TestCase{ProjectID: p.ID, Title: "Stored card numbers are not readable", Status: "ready", CreatedBy: "seed",
		Steps: []model.TestStep{{Action: "Read the card table in the database", Expected: "Only ciphertext is stored"}}}
	if err := st.CreateTest(ctx, tenantID, t); err != nil {
		return nil, err
	}
	if err := st.CreateLink(ctx, tenantID, &model.TraceLink{RequirementID: reqs[0].ID, TestCaseID: t.ID}); err != nil {
		return nil, err
	}
	if err := st.SetProjectAccess(ctx, tenantID, p.ID, model.ProjectAccessRestricted); err != nil {
		return nil, err
	}
	if _, err := st.SetProjectMember(ctx, tenantID, p.ID, store.MemberUser, dana.ID, rbac.RoleViewer, "seed"); err != nil {
		return nil, err
	}
	// Sara writes and reviews the vault's requirements but does not test
	// there: her project roles replace her organisation roles
	if _, err := st.SetProjectMemberRoles(ctx, tenantID, p.ID, store.MemberUser, sara.ID, []string{rbac.RoleSME, rbac.RoleReviewer}, "seed"); err != nil {
		return nil, err
	}
	p.Access = model.ProjectAccessRestricted
	return p, nil
}
