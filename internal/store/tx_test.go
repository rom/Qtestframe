package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestInTx(t *testing.T) {
	st, ten, ctx := setup(t)
	count := func() int {
		ps, err := st.ListProjects(ctx, ten.ID)
		must(t, err)
		return len(ps)
	}
	mk := func(ctx context.Context, key string) error {
		// CreateProject opens its own transaction: inside InTx it joins
		return st.CreateProject(ctx, ten.ID, &model.Project{Key: key, Name: key}, true)
	}

	// an error rolls everything back and skips AfterCommit
	boom := errors.New("boom")
	ran := false
	err := st.InTx(ctx, func(ctx context.Context) error {
		if !store.InTransaction(ctx) {
			t.Fatal("no transaction")
		}
		must(t, mk(ctx, "AAA"))
		must(t, mk(ctx, "BBB"))
		store.AfterCommit(ctx, func() { ran = true })
		return boom
	})
	if !errors.Is(err, boom) || ran || count() != 0 {
		t.Fatal(err, ran, count())
	}

	// a panic rolls back too
	func() {
		defer func() { _ = recover() }()
		_ = st.InTx(ctx, func(ctx context.Context) error {
			must(t, mk(ctx, "PAN"))
			panic("x")
		})
	}()
	if count() != 0 {
		t.Fatal(count())
	}

	// success commits, runs AfterCommit after the commit, and nests
	var seen int
	must(t, st.InTx(ctx, func(ctx context.Context) error {
		must(t, mk(ctx, "CCC"))
		store.AfterCommit(ctx, func() { seen = count() })
		return st.InTx(ctx, func(ctx context.Context) error { return mk(ctx, "DDD") })
	}))
	if seen != 2 || count() != 2 {
		t.Fatal(seen, count())
	}

	// outside a transaction AfterCommit runs at once
	now := false
	store.AfterCommit(ctx, func() { now = true })
	if !now || store.InTransaction(ctx) {
		t.Fatal("after commit outside a transaction")
	}
}

func TestSavepoint(t *testing.T) {
	st, ten, ctx := setup(t)
	mk := func(ctx context.Context, key string) error {
		return st.CreateProject(ctx, ten.ID, &model.Project{Key: key, Name: key}, true)
	}
	must(t, st.InTx(ctx, func(ctx context.Context) error {
		must(t, mk(ctx, "AAA"))
		// a duplicate key fails inside a savepoint; the transaction goes on
		if err := st.Savepoint(ctx, func() error { return mk(ctx, "AAA") }); err == nil {
			t.Fatal("duplicate accepted")
		}
		// a failure after writes undoes only the savepoint's writes
		boom := errors.New("boom")
		if err := st.Savepoint(ctx, func() error {
			must(t, mk(ctx, "BBB"))
			return boom
		}); !errors.Is(err, boom) {
			t.Fatal(err)
		}
		return mk(ctx, "CCC")
	}))
	ps, err := st.ListProjects(ctx, ten.ID)
	must(t, err)
	keys := map[string]bool{}
	for _, p := range ps {
		keys[p.Key] = true
	}
	if len(ps) != 2 || !keys["AAA"] || !keys["CCC"] {
		t.Fatal(keys)
	}
	// outside a transaction it just runs fn
	must(t, st.Savepoint(ctx, func() error { return nil }))
}
