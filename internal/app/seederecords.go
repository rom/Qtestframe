package app

import (
	"context"
	"time"

	"github.com/rom/qtestframe/internal/api"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedERecords sets up electronic-records mode as an organisation would
// before turning it on (left off, so the demo stays easy to explore) and
// records electronic signatures: an approved requirement, the approved
// waiver, a baseline sign-off and a requirement changed after it was
// approved (its signature no longer matches). The audit trail of the demo
// is hash-chained like any other.
func seedERecords(ctx context.Context, st *store.Store, tenantID string, p *model.Project, reqs []*model.Requirement) error {
	if err := st.SaveERecordsSettings(ctx, tenantID, &model.ERecordsSettings{
		Meanings:           []string{"authored", "reviewed", "approved", "verified", "released", "QA release"},
		SignatureActions:   []string{model.SignRequirementApprove, model.SignReleaseGo, model.SignBaselineCreate},
		ReasonEdits:        []string{"requirement.update", "requirement.delete", "test.delete", "execution.update", "verification.update"},
		IdleTimeoutMinutes: 30, SSOReauthMinutes: 5,
	}, "seed"); err != nil {
		return err
	}
	sign := func(kind, id, action, meaning, email, name, comment, method, zone string, at time.Time) error {
		hash, pid, key, err := api.RecordHash(ctx, st, tenantID, kind, id)
		if err != nil {
			return err
		}
		return st.AddESignature(ctx, tenantID, &model.ESignature{ProjectID: pid, EntityType: kind, EntityID: id, EntityKey: key, Action: action, Meaning: meaning,
			Comment: comment, UserEmail: email, UserName: name, Method: method, TimeZone: zone, ContentHash: hash, SignedAt: at})
	}
	now := time.Now().UTC().Truncate(time.Second)
	var approved []*model.Requirement
	for _, r := range reqs {
		if r.Status == "approved" || r.Status == "implemented" || r.Status == "verified" {
			approved = append(approved, r)
		}
	}
	if len(approved) > 0 {
		if err := sign(model.KindRequirement, approved[0].ID, model.SignRequirementApprove, "approved", "qa.lead@example.com", "QA lead (demo)",
			"Reviewed against the regulation; ready for implementation.", "password+mfa", "Europe/Stockholm", now.AddDate(0, 0, -20)); err != nil {
			return err
		}
	}
	if len(approved) > 1 {
		// signed, then the text changed: the signature shows that it no longer matches
		r := approved[1]
		if err := sign(model.KindRequirement, r.ID, model.SignRequirementApprove, "approved", "qa.lead@example.com", "QA lead (demo)", "", "password", "Europe/Stockholm", now.AddDate(0, 0, -15)); err != nil {
			return err
		}
		cur, err := st.GetRequirement(ctx, tenantID, r.ID)
		if err != nil {
			return err
		}
		cur.Description += "\n\nClarified after approval: the limit applies per calendar day."
		if err := st.UpdateRequirement(ctx, tenantID, cur, false); err != nil {
			return err
		}
	}
	ws, err := st.ListWaivers(ctx, tenantID, p.ID, store.WaiverFilter{})
	if err != nil {
		return err
	}
	for _, w := range ws {
		if w.Status == "approved" {
			if err := sign(model.KindWaiver, w.ID, model.SignWaiverDecision, "approved", "ciso@example.com", "CISO (demo)", "approve: Accepted for 1.4.", "sso+mfa", "America/New_York", now.AddDate(0, 0, -7)); err != nil {
				return err
			}
			break
		}
	}
	bls, err := st.ListBaselines(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	if len(bls) > 0 {
		if err := sign("baseline", bls[0].ID, model.SignBaselineSignoff, "QA release", "qa.lead@example.com", "QA lead (demo)", "Baseline for the supplier audit.", "password", "Europe/Stockholm", now.AddDate(0, 0, -3)); err != nil {
			return err
		}
	}
	return nil
}
