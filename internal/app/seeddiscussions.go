package app

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// SeedDemoDiscussions gives the demo project discussions: an open thread
// with a reply and a mention on a requirement, a resolved thread, a thread
// on a defect with a removed comment, and one on the regression run with a
// reaction. The authors are the demo users (and the first administrator);
// it does nothing when the project already has comments.
func SeedDemoDiscussions(ctx context.Context, st *store.Store, tenantID string, p *model.Project) error {
	dana, err := demoUser(ctx, st, tenantID, DemoMemberEmail, "Dana Lind", rbac.RoleTester)
	if err != nil {
		return err
	}
	erik, err := demoUser(ctx, st, tenantID, DemoOutsiderEmail, "Erik Berg", rbac.RoleTester)
	if err != nil {
		return err
	}
	lead := dana
	ms, err := st.ListTenantMembers(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.Role == rbac.RoleAdmin {
			if u, err := st.GetUser(ctx, m.UserID); err == nil {
				lead = u
				break
			}
		}
	}
	if idx, err := st.DiscussionIndex(ctx, tenantID, p.ID); err != nil || len(idx) > 0 {
		return err
	}
	reqs, err := st.ListRequirements(ctx, tenantID, p.ID)
	if err != nil || len(reqs) < 2 {
		return err
	}
	add := func(kind, id, parent string, u *model.User, body string, mentions ...string) (*model.Comment, error) {
		c := &model.Comment{ProjectID: p.ID, EntityType: kind, EntityID: id, ParentID: parent, UserID: u.ID, Body: body, Mentions: mentions}
		return c, st.AddComment(ctx, tenantID, c)
	}
	q := reqs[0]
	root, err := add(model.KindRequirement, q.ID, "", dana, "Is a minimum length of **12** characters enough here, or do we follow the pen test report?\n\n@erik can you check what it recommends?", erik.ID)
	if err != nil {
		return err
	}
	if _, err := add(model.KindRequirement, q.ID, root.ID, erik, "The report recommends at least *14* characters and a check against known breached passwords. I would add both as evaluation criteria."); err != nil {
		return err
	}
	if _, err := st.ToggleReaction(ctx, tenantID, root.ID, lead.ID, "eyes"); err != nil {
		return err
	}
	done, err := add(model.KindRequirement, reqs[1].ID, "", lead, "Wording agreed in the review meeting: keep MUST, the lockout time goes to the rationale.")
	if err != nil {
		return err
	}
	if _, err := add(model.KindRequirement, reqs[1].ID, done.ID, dana, "Updated the rationale accordingly."); err != nil {
		return err
	}
	if err := st.ResolveThread(ctx, tenantID, done.ID, lead.ID, true); err != nil {
		return err
	}
	if ds, err := st.ListDefects(ctx, tenantID, p.ID); err == nil && len(ds) > 0 {
		d := ds[0]
		t, err := add(model.KindDefect, d.ID, "", erik, "Reproduced on staging with build 1.4.0-rc1. Steps:\n\n1. Enter an unknown e-mail address\n2. Press **Sign in**\n\nThe message says the account does not exist.")
		if err != nil {
			return err
		}
		gone, err := add(model.KindDefect, d.ID, t.ID, dana, "Duplicate of an older ticket?")
		if err != nil {
			return err
		}
		if err := st.RemoveComment(ctx, tenantID, gone.ID, dana.ID); err != nil {
			return err
		}
		if _, err := add(model.KindDefect, d.ID, t.ID, lead, "@dana please retest once the fix is deployed.", dana.ID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	runs, err := st.ListRuns(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	for _, r := range runs {
		if r.Name != "Sprint 12 regression" {
			continue
		}
		c, err := add(model.DiscussionRun, r.ID, "", dana, "Staging was down from 10:00 to 10:20, so the failures before then can be ignored.")
		if err != nil {
			return err
		}
		if _, err := st.ToggleReaction(ctx, tenantID, c.ID, erik.ID, "+1"); err != nil {
			return err
		}
		break
	}
	return nil
}
