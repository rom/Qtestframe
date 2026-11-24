package store_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/tracing"
)

func TestSystemCounts(t *testing.T) {
	st, ten, ctx := setup(t)
	if err := st.CreateProject(ctx, ten.ID, &model.Project{Key: "CNT", Name: "Counted"}, false); err != nil {
		t.Fatal(err)
	}
	c, err := st.SystemCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c["tenants"] < 1 || c["projects"] != 1 || c["requirements"] != 0 || len(c) != 6 {
		t.Fatal(c)
	}
}

// Queries inside a sampled span get a child span; without one nothing is
// recorded.
func TestQuerySpans(t *testing.T) {
	st, ten, _ := setup(t)
	col := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer col.Close()
	tr := tracing.New()
	tr.Configure(tracing.Config{Enabled: true, Endpoint: col.URL, SampleRatio: 1})
	ctx, root := tr.Start(context.Background(), "req", tracing.KindServer)
	if _, err := st.GetTenant(ctx, ten.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTenant(ctx, "missing"); err == nil {
		t.Fatal("missing tenant")
	}
	if _, err := st.SystemCounts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProject(ctx, ten.ID, &model.Project{Key: "SPN", Name: "Spans"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ListProjects(ctx, ten.ID); err != nil {
		t.Fatal(err)
	}
	root.End()
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := tr.Stats(); s.Exported < 5 {
		t.Fatalf("%+v", s)
	}
}
