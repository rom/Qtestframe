package logging

import (
	"bytes"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestManagerReconfigure(t *testing.T) {
	var out lockedBuf
	m, err := NewManager(config.Logging{Level: "info", Format: "text"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	log := m.Logger()
	derived := log.With("component", "x").WithGroup("g")
	log.Debug("hidden")
	derived.Info("one", "k", 1)
	if strings.Contains(out.String(), "hidden") || !strings.Contains(out.String(), "component=x") || !strings.Contains(out.String(), "g.k=1") {
		t.Fatalf("before: %s", out.String())
	}
	if err := m.Reconfigure(config.Logging{Level: "debug", Format: "json"}); err != nil {
		t.Fatal(err)
	}
	log.Debug("visible")
	derived.Info("two", "k", 2)
	s := out.String()
	if !strings.Contains(s, `"msg":"visible"`) || !strings.Contains(s, `"component":"x","g":{"k":2}`) {
		t.Errorf("after: %s", s)
	}
	if m.Config().Format != "json" {
		t.Error("config")
	}
	// an invalid configuration keeps the previous one
	if err := m.Reconfigure(config.Logging{Level: "nope"}); err == nil {
		t.Error("invalid level accepted")
	}
	if m.Config().Level != "debug" {
		t.Error("config replaced on error")
	}
	if _, err := NewManager(config.Logging{Level: "nope"}, &out); err == nil {
		t.Error("NewManager accepted a bad level")
	}
}

func TestManagerSyslogAndTestMessage(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	sc := config.Syslog{Enabled: true, Network: "udp", Address: pc.LocalAddr().String(), Tag: "t", Facility: "local1", Format: "rfc3164"}
	if err := SendTestMessage(sc, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	_ = pc.SetReadDeadline(time.Now().Add(3 * time.Second))
	b := make([]byte, 1024)
	n, _, err := pc.ReadFrom(b)
	if err != nil || !strings.HasPrefix(string(b[:n]), "<142>") || !strings.HasSuffix(string(b[:n]), "hello") {
		t.Fatalf("got %q %v", b[:n], err)
	}
	if err := SendTestMessage(config.Syslog{Network: "carrier-pigeon"}, nil); err == nil {
		t.Error("bad network accepted")
	}
	var out lockedBuf
	m, err := NewManager(config.Logging{Level: "info"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Reconfigure(config.Logging{Level: "info", Syslog: sc}); err != nil {
		t.Fatal(err)
	}
	m.Logger().Info("via syslog")
	n, _, err = pc.ReadFrom(b)
	if err != nil || !strings.Contains(string(b[:n]), "via syslog") {
		t.Errorf("syslog: %q %v", b[:n], err)
	}
	if err := m.Close(); err != nil {
		t.Error(err)
	}
}
