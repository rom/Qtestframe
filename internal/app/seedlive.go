package app

import (
	"context"
	"errors"

	"github.com/rom/qtestframe/internal/store"
)

// SeedDemoLive prepares the demo of live updates: they need nothing stored
// (events and presence are in memory), so to try them out sign in as the
// administrator and as DemoMemberEmail in two browsers and open the same
// requirement or run. DemoOutsiderEmail has turned live updates off in the
// profile, to show the per-user setting (the client polls instead).
func SeedDemoLive(ctx context.Context, st *store.Store) error {
	u, err := st.GetUserByEmail(ctx, DemoOutsiderEmail)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil || u.Preferences.LiveUpdatesOff {
		return err
	}
	u.Preferences.LiveUpdatesOff = true
	return st.UpdateUser(ctx, u)
}
