//go:build unix

package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCertReloader(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := genCert(t, dir)

	if _, err := newCertReloader(filepath.Join(dir, "missing.crt"), keyFile); err == nil || !strings.HasPrefix(err.Error(), "tls:") {
		t.Errorf("missing cert: %v", err)
	}
	c, err := newCertReloader(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.get(&tls.ClientHelloInfo{})
	if err != nil || first == nil || len(first.Certificate) == 0 {
		t.Fatalf("get: %v %v", first, err)
	}

	// Guard: make sure a SIGHUP can never terminate the test binary, even
	// before watch has registered its handler.
	guard := make(chan os.Signal, 16)
	signal.Notify(guard, syscall.SIGHUP)
	defer signal.Stop(guard)

	var logs syncBufferLog
	log := slog.New(slog.NewTextHandler(&logs, nil))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.watch(ctx, log); close(done) }()

	// Broken files: reload fails, the previous certificate is kept.
	os.WriteFile(certFile, []byte("garbage"), 0o600)
	waitFor(t, func() bool {
		_ = syscall.Kill(os.Getpid(), syscall.SIGHUP)
		return strings.Contains(logs.String(), "tls reload failed")
	})
	if cur, _ := c.get(nil); cur != first {
		t.Error("certificate replaced although reload failed")
	}

	// Replace with a new valid pair: the pointer changes.
	newCert, newKey := genCert(t, t.TempDir())
	for src, dst := range map[string]string{newCert: certFile, newKey: keyFile} {
		b, _ := os.ReadFile(src)
		os.WriteFile(dst, b, 0o600)
	}
	waitFor(t, func() bool {
		_ = syscall.Kill(os.Getpid(), syscall.SIGHUP)
		cur, _ := c.get(nil)
		return cur != first
	})
	if !strings.Contains(logs.String(), "tls certificate reloaded") {
		t.Errorf("logs: %s", logs.String())
	}
	cur, _ := c.get(nil)
	if bytes.Equal(cur.Certificate[0], first.Certificate[0]) {
		t.Error("certificate bytes unchanged")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not stop")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type syncBufferLog struct{ b syncBuffer }

func (s *syncBufferLog) Write(p []byte) (int, error) { s.init(); return s.b.Write(p) }
func (s *syncBufferLog) String() string              { s.init(); return s.b.String() }
func (s *syncBufferLog) init() {
	if s.b.mu == nil {
		s.b.mu = make(chan struct{}, 1)
	}
}
