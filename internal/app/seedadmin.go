package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedAdmin shows the super-administrator pages with content: the demo
// organisation gets a policy (quotas and an e-mail domain), a second
// organisation is suspended, and a few system settings are changed in the
// web client (an inactivity timeout, a token policy, outbound hosts that
// are never called).
func seedAdmin(ctx context.Context, st *store.Store, tenantID string) error {
	pol := model.TenantPolicy{MaxMembers: 1000, MaxProjects: 500, MaxStorageMB: 20000, Note: "Demo organisation: quotas of the standard plan."}
	// the domain of the first demo organisation (seeded again, another
	// organisation keeps it)
	if owner, err := st.TenantByEmailDomain(ctx, "demo.example"); err != nil {
		return err
	} else if owner == "" || owner == tenantID {
		pol.EmailDomains = []string{"demo.example"}
	}
	if _, err := st.SetTenantPolicy(ctx, tenantID, pol); err != nil {
		return err
	}
	partner, err := st.GetTenantBySlug(ctx, "partner")
	if errors.Is(err, store.ErrNotFound) {
		partner, err = st.CreateTenant(ctx, "partner", "Partner Labs (suspended)")
	}
	if err != nil {
		return err
	}
	if _, err := st.SetTenantPolicy(ctx, partner.ID, model.TenantPolicy{MaxMembers: 10, MaxProjects: 3, MaxStorageMB: 500, AIDisabled: true,
		AllowedProviders: []string{"local"}, Note: "Trial ended; suspended until the contract is renewed."}); err != nil {
		return err
	}
	if _, err := st.SetTenantSuspended(ctx, partner.ID, true); err != nil {
		return err
	}
	stored, err := st.SystemSettings(ctx)
	if err != nil {
		return err
	}
	values := map[string]json.RawMessage{}
	for k, v := range map[string]string{
		"auth.session_idle_timeout": `"8h0m0s"`,
		"auth.token_max_days":       `365`,
		"outbound.denied_hosts":     `["169.254.169.254","metadata.google.internal"]`,
	} {
		if _, ok := stored[k]; !ok {
			values[k] = json.RawMessage(v)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return st.SaveSystemSettings(ctx, values, "seed")
}
