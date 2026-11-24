package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/tracing"
)

func TestTraceIDs(t *testing.T) {
	var buf bytes.Buffer
	l, _, err := New(config.Logging{Level: "info", Format: "json"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	tr := tracing.New()
	tr.Configure(tracing.Config{Enabled: true, Endpoint: "http://127.0.0.1:1", SampleRatio: 1})
	defer tr.Shutdown(context.Background())
	ctx, span := tr.Start(context.Background(), "req", tracing.KindServer)
	l.With("a", 1).WithGroup("g").InfoContext(ctx, "inside", "k", "v")
	out := buf.String()
	if !strings.Contains(out, `"trace_id":"`+span.Context().TraceID.String()) || !strings.Contains(out, `"span_id":"`+span.Context().SpanID.String()) {
		t.Fatal(out)
	}
	buf.Reset()
	remote := tracing.ContextWithRemote(context.Background(), tracing.SpanContext{TraceID: tracing.TraceID{1}, SpanID: tracing.SpanID{2}})
	l.InfoContext(remote, "remote")
	if !strings.Contains(buf.String(), "trace_id") || strings.Contains(buf.String(), "span_id") {
		t.Fatal(buf.String())
	}
	buf.Reset()
	l.Info("plain")
	if strings.Contains(buf.String(), "trace_id") {
		t.Fatal(buf.String())
	}
}
