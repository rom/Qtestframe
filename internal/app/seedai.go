package app

import (
	"context"

	"github.com/rom/qtestframe/internal/store"
)

// seedAI turns the optional AI assistance on for the demo organisation, so
// the Assist buttons of the editors, the run page and the risk dialog show
// up. The service itself stays off until a super administrator configures
// one (Administration → System settings → AI assistance): until then the
// buttons explain that. Nothing is sent anywhere by the seed.
func seedAI(ctx context.Context, st *store.Store, tenantID string) error {
	ts, err := st.TenantSettings(ctx, tenantID)
	if err != nil {
		return err
	}
	if ts.AIEnabled {
		return nil
	}
	ts.AIEnabled = true
	return st.SaveTenantSettings(ctx, tenantID, ts)
}
