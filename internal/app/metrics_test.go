package app

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil"
)

func TestRecordMetricsJob(t *testing.T) {
	st := store.New(testutil.OpenDB(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ten, err := st.CreateTenant(ctx, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	now := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	st.SetClock(func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	p := &model.Project{Key: "JOB", Name: "Job"}
	if err := st.CreateProject(ctx, ten.ID, p, false); err != nil {
		t.Fatal(err)
	}
	logs := newSyncBuffer()
	log := slog.New(slog.NewTextHandler(logs, nil))
	done := make(chan struct{})
	go func() { recordMetrics(ctx, st, log, 5*time.Millisecond); close(done) }()
	count := func() int {
		ss, err := st.ListSnapshots(ctx, ten.ID, p.ID, "2000-01-01", "2100-01-01")
		if err != nil {
			return -1
		}
		return len(ss)
	}
	wait := func(n int) {
		deadline := time.Now().Add(5 * time.Second)
		for count() < n {
			if time.Now().After(deadline) {
				t.Fatalf("%d snapshots, want %d; log %s", count(), n, logs.String())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait(1)
	// the next day (injected clock): one more snapshot
	mu.Lock()
	now = now.Add(24 * time.Hour)
	mu.Unlock()
	wait(2)
	cancel()
	<-done
	if !strings.Contains(logs.String(), "recorded metric snapshots") {
		t.Error(logs.String())
	}

	// failures are logged and the job keeps going until cancelled
	ctx2, cancel2 := context.WithCancel(context.Background())
	st.DB.Close()
	done = make(chan struct{})
	go func() { recordMetrics(ctx2, st, log, 5*time.Millisecond); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "metric snapshots failed") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel2()
	<-done
	if !strings.Contains(logs.String(), "metric snapshots failed") {
		t.Error(logs.String())
	}
}

func TestSeedDemoHistory(t *testing.T) {
	st := store.New(testutil.OpenDB(t))
	ctx := context.Background()
	ten, err := st.CreateTenant(ctx, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	p, err := SeedDemo(ctx, st, ten.ID)
	if err != nil {
		t.Fatal(err)
	}
	fl, err := st.FlakyTests(ctx, ten.ID, p.ID)
	if err != nil || len(fl) == 0 {
		t.Fatalf("flaky %v %v", fl, err)
	}
	// change and defect correlations: a regression, blocked requirements, a suspect relation, stale results
	set, _ := st.CorrelationSettings(ctx, ten.ID, p.ID)
	blocks, err := st.DefectBlocks(ctx, ten.ID, p.ID, set)
	if err != nil || len(blocks) == 0 {
		t.Fatalf("blocks %v %v", blocks, err)
	}
	// two suspect relations: the change travels transitively (two refinement steps)
	if n, err := st.SuspectRelationCounts(ctx, ten.ID, p.ID); err != nil || len(n) != 2 {
		t.Fatalf("suspect relations %v %v", n, err)
	}
	if x, err := st.LoadStaleIndex(ctx, ten.ID, p.ID); err != nil || len(x.StaleTests()) == 0 {
		t.Fatalf("stale %v", err)
	}
	if rc, err := st.RegressionCounts(ctx, ten.ID, p.ID); err != nil || len(rc) == 0 {
		t.Fatalf("regressions %v %v", rc, err)
	}
	if _, err := st.RecordSnapshot(ctx, ten.ID, p.ID, false); err != nil {
		t.Fatal(err)
	}
	ss, err := st.ListSnapshots(ctx, ten.ID, p.ID, "2000-01-01", "2100-01-01")
	if err != nil || len(ss) < SeedHistoryDays {
		t.Fatalf("%d snapshots %v", len(ss), err)
	}
	if first := ss[0].Data; first.Requirements == 0 || first.Requirements == ss[len(ss)-1].Data.Requirements {
		t.Errorf("requirements grow: %+v", first)
	}
	rels, err := st.ListReleases(ctx, ten.ID, p.ID)
	if err != nil || len(rels) != 1 {
		t.Fatal(rels, err)
	}
	b, err := st.ReleaseBurnup(ctx, ten.ID, rels[0].ID)
	if err != nil || len(b.Points) < 30 || b.Points[len(b.Points)-1].Passed == 0 {
		t.Errorf("burnup %+v %v", b, err)
	}
}
