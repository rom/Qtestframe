package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMetricsAndTracingConfig(t *testing.T) {
	d := Default()
	if d.Metrics.Enabled || d.Tracing.Enabled || d.Tracing.SampleRatio != "1" || d.Tracing.ServiceName != "qtestframe" {
		t.Fatalf("defaults: %+v %+v", d.Metrics, d.Tracing)
	}
	dir := t.TempDir()
	tok := writeFile(t, dir, "tok", "file-token\n")
	hdr := writeFile(t, dir, "hdr", "Authorization=Bearer x\n")
	p := writeFile(t, dir, "c.yaml", `
metrics:
  enabled: true
  listen: "127.0.0.1:9090"
  token_file: `+tok+`
  allowed_ips: [10.0.0.0/8, 192.0.2.7]
tracing:
  enabled: true
  endpoint: http://otel:4318
  headers_file: `+hdr+`
  sample_ratio: 0.25
  service_name: qtf-prod
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Metrics.Token != "file-token" || c.Tracing.Headers != "Authorization=Bearer x" || c.Tracing.SampleRatio != "0.25" ||
		!c.Meta.FileKeys["metrics.token"] || !c.Meta.FileKeys["tracing.headers"] || len(c.Metrics.AllowedIPs) != 2 {
		t.Fatalf("%+v %+v", c.Metrics, c.Tracing)
	}

	t.Setenv("QTF_METRICS_TOKEN", "env-token")
	t.Setenv("QTF_TRACING_HEADERS", "x-key=1")
	t.Setenv("QTF_METRICS_ENABLED", "false")
	t.Setenv("QTF_TRACING_ENABLED", "1")
	t.Setenv("QTF_TRACING_ENDPOINT", "https://otel.example.com")
	c, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Metrics.Token != "env-token" || c.Tracing.Headers != "x-key=1" || c.Metrics.Enabled || c.Tracing.Endpoint != "https://otel.example.com" ||
		c.Meta.EnvKeys["metrics.token"] != "QTF_METRICS_TOKEN" || c.Meta.EnvKeys["tracing.headers"] != "QTF_TRACING_HEADERS" || c.Meta.EnvKeys["tracing.enabled"] != "QTF_TRACING_ENABLED" {
		t.Fatalf("env: %+v %+v %v", c.Metrics, c.Tracing, c.Meta.EnvKeys)
	}

	if _, err := Load(writeFile(t, dir, "m.yaml", "metrics: {token_file: "+filepath.Join(dir, "missing")+"}")); err == nil {
		t.Fatal("missing token file")
	}
	if _, err := Load(writeFile(t, dir, "h.yaml", "tracing: {headers_file: "+filepath.Join(dir, "missing")+"}")); err == nil {
		t.Fatal("missing headers file")
	}
}

func TestMetricsAndTracingValidate(t *testing.T) {
	for name, c := range map[string]struct {
		mut  func(*Config)
		want string
	}{
		"listen":       {func(c *Config) { c.Metrics.Listen = "9090" }, "metrics.listen"},
		"allowed":      {func(c *Config) { c.Metrics.AllowedIPs = []string{"10.0.0.0/8", "nope"} }, "metrics.allowed_ips"},
		"endpoint":     {func(c *Config) { c.Tracing.Endpoint = "otel:4318" }, "tracing.endpoint must"},
		"no endpoint":  {func(c *Config) { c.Tracing.Enabled = true }, "tracing.endpoint is required"},
		"ratio":        {func(c *Config) { c.Tracing.SampleRatio = "1.5" }, "tracing.sample_ratio"},
		"ratio text":   {func(c *Config) { c.Tracing.SampleRatio = "x" }, "tracing.sample_ratio"},
		"service name": {func(c *Config) { c.Tracing.ServiceName = " " }, "tracing.service_name"},
		"headers":      {func(c *Config) { c.Tracing.Headers = "no value" }, "tracing.headers"},
	} {
		cfg := Default()
		c.mut(&cfg)
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	cfg := Default()
	cfg.Metrics.Listen = "[::1]:9090"
	cfg.Metrics.AllowedIPs = []string{"::1", "fd00::/8"}
	cfg.Tracing.SampleRatio = " 0 "
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if f, err := ParseRatio("0.5"); err != nil || f != 0.5 {
		t.Fatal(f, err)
	}
	if _, err := ParseRatio("NaN"); err == nil {
		t.Fatal("NaN accepted")
	}
}
