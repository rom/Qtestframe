package app

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// With metrics.listen, /metrics is served on its own listener (with the
// probes) and the main listener answers 404.
func TestServeMetricsListener(t *testing.T) {
	maddr := freePort(t)
	cfg := writeConfig(t, fmt.Sprintf("metrics: {enabled: true, listen: %q}", maddr))
	s := startServe(t, cfg)
	c := &http.Client{Timeout: 5 * time.Second}
	if code := get(t, c, "http://"+s.addr+"/metrics"); code != http.StatusNotFound {
		t.Fatalf("main listener: %d", code)
	}
	res, err := c.Get("http://" + maddr + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "qtf_build_info{") {
		t.Fatalf("metrics listener: %d %s", res.StatusCode, body)
	}
	for _, p := range []string{"/healthz", "/readyz"} {
		if code := get(t, c, "http://"+maddr+p); code != http.StatusOK {
			t.Errorf("%s on the metrics listener: %d", p, code)
		}
	}
}

func TestServeMetricsListenerBusy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg := writeConfig(t, fmt.Sprintf("metrics: {enabled: true, listen: %q}", ln.Addr().String()))
	r := run(t, "", "serve", "--config", cfg)
	if r.code == 0 || !strings.Contains(r.stderr, "metrics listener") {
		t.Fatalf("%d %s", r.code, r.stderr)
	}
}
