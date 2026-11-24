package store_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestPasskeys(t *testing.T) {
	st, ten, ctx := setup(t)
	u := &model.User{Email: "a@example.com"}
	must(t, st.CreateUser(ctx, u))
	must(t, st.SetMembership(ctx, ten.ID, u.ID, "tester"))
	// a session waiting for MFA enrolment: a passkey lifts it
	sess := &model.Session{ID: "s1", UserID: u.ID, CSRF: "c", ExpiresAt: time.Now().Add(time.Hour), MFAEnrolRequired: true}
	must(t, st.CreateSession(ctx, sess))
	pk := &model.Passkey{UserID: u.ID, CredentialID: "cred-1", PublicKey: "key", Alg: -7, SignCount: 3, AAGUID: "00", Attestation: "none",
		Name: "  Laptop ", UserVerified: true, BackupEligible: true}
	must(t, st.AddPasskey(ctx, pk))
	if pk.ID == "" || pk.Name != "Laptop" {
		t.Fatal(pk)
	}
	if s, _ := st.GetSession(ctx, "s1"); s.MFAEnrolRequired || s.ReauthAt != nil || !s.AuthenticatedAt().Equal(s.CreatedAt) {
		t.Fatalf("%+v", s)
	}
	// the same credential twice
	if err := st.AddPasskey(ctx, &model.Passkey{UserID: u.ID, CredentialID: "cred-1", PublicKey: "k", Name: "x"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	must(t, st.AddPasskey(ctx, &model.Passkey{UserID: u.ID, CredentialID: "cred-2", PublicKey: "k2", Alg: -8, Name: "Phone", Transports: []string{"hybrid"}}))
	list, err := st.ListPasskeys(ctx, u.ID)
	if err != nil || len(list) != 2 || list[0].Name != "Laptop" || list[1].Transports[0] != "hybrid" || list[0].SignCount != 3 || !list[0].UserVerified || list[0].LastUsedAt != nil {
		t.Fatalf("%+v %v", list, err)
	}
	got, err := st.GetPasskeyByCredentialID(ctx, "cred-2")
	if err != nil || got.UserID != u.ID || got.Alg != -8 {
		t.Fatal(got, err)
	}
	if _, err := st.GetPasskeyByCredentialID(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetPasskey(ctx, "other-user", pk.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	// use: the counter only grows
	must(t, st.UsePasskey(ctx, pk.ID, 10, true))
	must(t, st.UsePasskey(ctx, pk.ID, 5, true))
	got, _ = st.GetPasskey(ctx, u.ID, pk.ID)
	if got.SignCount != 10 || got.LastUsedAt == nil || !got.BackedUp {
		t.Fatalf("%+v", got)
	}
	// rename, delete
	must(t, st.RenamePasskey(ctx, u.ID, pk.ID, "Work"))
	if err := st.RenamePasskey(ctx, "other", pk.ID, "x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if n, _ := st.CountPasskeys(ctx, u.ID); n != 2 {
		t.Fatal(n)
	}
	counts, err := st.PasskeyCounts(ctx)
	if err != nil || counts[u.ID] != 2 {
		t.Fatal(counts, err)
	}
	ms, _ := st.ListTenantMembers(ctx, ten.ID)
	if len(ms) != 1 || ms[0].Passkeys != 2 {
		t.Fatalf("%+v", ms)
	}
	must(t, st.DeletePasskey(ctx, u.ID, pk.ID))
	if err := st.DeletePasskey(ctx, u.ID, pk.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if n, err := st.DeleteUserPasskeys(ctx, u.ID); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	// at most MaxPasskeysPerUser
	for i := 0; i < store.MaxPasskeysPerUser; i++ {
		must(t, st.AddPasskey(ctx, &model.Passkey{UserID: u.ID, CredentialID: fmt.Sprint("c", i), PublicKey: "k", Name: "n"}))
	}
	if err := st.AddPasskey(ctx, &model.Passkey{UserID: u.ID, CredentialID: "one-more", PublicKey: "k", Name: "n"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	// re-authentication of a session
	at := time.Now().Add(time.Minute).UTC()
	must(t, st.SetSessionReauth(ctx, "s1", at))
	if s, _ := st.GetSession(ctx, "s1"); s.ReauthAt == nil || !s.AuthenticatedAt().Equal(s.ReauthAt.UTC()) {
		t.Fatalf("%+v", s)
	}
}

func TestWebAuthnChallenges(t *testing.T) {
	st, _, ctx := setup(t)
	now := time.Now().UTC()
	st.SetClock(func() time.Time { return now })
	mk := func(id, purpose, binding string, ttl time.Duration) {
		must(t, st.CreateWebAuthnChallenge(ctx, &model.WebAuthnChallenge{ID: id, Purpose: purpose, UserID: "u", Binding: binding, UserVerification: "required", ExpiresAt: now.Add(ttl)}))
	}
	mk("c1", model.PasskeyLogin, "b", time.Minute)
	ch, err := st.TakeWebAuthnChallenge(ctx, "c1", model.PasskeyLogin, "b")
	if err != nil || ch.UserID != "u" || ch.UserVerification != "required" {
		t.Fatal(ch, err)
	}
	// single use
	if _, err := st.TakeWebAuthnChallenge(ctx, "c1", model.PasskeyLogin, "b"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	// another purpose or binding: refused, and gone
	mk("c2", model.PasskeyRegister, "b", time.Minute)
	if _, err := st.TakeWebAuthnChallenge(ctx, "c2", model.PasskeyLogin, "b"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	mk("c3", model.PasskeyRegister, "b", time.Minute)
	if _, err := st.TakeWebAuthnChallenge(ctx, "c3", model.PasskeyRegister, "other"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.TakeWebAuthnChallenge(ctx, "c3", model.PasskeyRegister, "b"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	// expired
	mk("c4", model.PasskeyMFA, "b", time.Minute)
	now = now.Add(2 * time.Minute)
	if _, err := st.TakeWebAuthnChallenge(ctx, "c4", model.PasskeyMFA, "b"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	// expired ones are purged
	mk("c5", model.PasskeyMFA, "b", time.Minute)
	now = now.Add(2 * time.Minute)
	if n, err := st.PurgeExpiredMFA(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	// concurrent answers: one wins
	mk("c6", model.PasskeyLogin, "b", time.Minute)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.TakeWebAuthnChallenge(ctx, "c6", model.PasskeyLogin, "b"); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatal(wins)
	}
}
