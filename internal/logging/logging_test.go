package logging

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

// ---------------------------------------------------------------- helpers

type certFiles struct {
	cert, key string
	pair      tls.Certificate
	x509      *x509.Certificate
}

// genCert writes a self-signed certificate valid for 127.0.0.1.
func genCert(t *testing.T, dir, name string) certFiles {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	c := certFiles{cert: filepath.Join(dir, name+".crt"), key: filepath.Join(dir, name+".key")}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb})
	if err := os.WriteFile(c.cert, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.key, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if c.pair, err = tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	c.x509, _ = x509.ParseCertificate(der)
	return c
}

// readFramed reads one RFC 6587 octet-counted message.
func readFramed(r *bufio.Reader) (string, error) {
	n, err := r.ReadString(' ')
	if err != nil {
		return "", err
	}
	l, err := strconv.Atoi(strings.TrimSpace(n))
	if err != nil {
		return "", fmt.Errorf("bad frame length %q", n)
	}
	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

// streamServer accepts connections on ln and forwards each framed message
// to the returned channel. If closeAfterFirst is set the first connection
// is closed after one message.
func streamServer(t *testing.T, ln net.Listener, closeAfterFirst bool) <-chan string {
	t.Helper()
	ch := make(chan string, 16)
	t.Cleanup(func() { ln.Close() })
	go func() {
		first := true
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn, closeIt bool) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					m, err := readFramed(r)
					if err != nil {
						return
					}
					ch <- m
					if closeIt {
						return
					}
				}
			}(c, first && closeAfterFirst)
			first = false
		}
	}()
	return ch
}

func recv(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for syslog message")
		return ""
	}
}

var rfc5424 = regexp.MustCompile(`^<(\d+)>1 (\S+) (\S+) (\S+) (\d+) - - (.*)$`)

func shortTempDir(t *testing.T) string {
	t.Helper()
	// Unix socket paths are limited to ~104 bytes; t.TempDir can be long.
	d, err := os.MkdirTemp("", "qtflog")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

// ---------------------------------------------------------------- tests

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{"debug": slog.LevelDebug, " Info ": slog.LevelInfo, "WARN": slog.LevelWarn, "error": slog.LevelError, "info+2": slog.LevelInfo + 2} {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseLevel("loud"); err == nil {
		t.Error("expected error")
	}
}

func TestNewTextAndJSON(t *testing.T) {
	var buf bytes.Buffer
	log, closer, err := New(config.Logging{Level: "info", Format: "text"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	log.Debug("hidden")
	log.Info("hello", "k", "v")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if s := buf.String(); strings.Contains(s, "hidden") || !strings.Contains(s, "msg=hello k=v") {
		t.Errorf("text output: %q", s)
	}
	buf.Reset()
	log, _, err = New(config.Logging{Level: "debug", Format: "json"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	log.Debug("dbg", "n", 1)
	if s := buf.String(); !strings.Contains(s, `"msg":"dbg"`) || !strings.Contains(s, `"n":1`) {
		t.Errorf("json output: %q", s)
	}
}

func TestNewErrors(t *testing.T) {
	if _, _, err := New(config.Logging{Level: "nope"}, io.Discard); err == nil || !strings.Contains(err.Error(), "logging:") {
		t.Errorf("bad level: %v", err)
	}
	cfg := config.Logging{Level: "info", Syslog: config.Syslog{Enabled: true, Network: "udp", Address: "127.0.0.1:1", Facility: "bogus"}}
	if _, _, err := New(cfg, io.Discard); err == nil || !strings.Contains(err.Error(), "facility") {
		t.Errorf("bad facility: %v", err)
	}
}

func TestSeverity(t *testing.T) {
	cases := map[slog.Level]int{
		slog.LevelDebug - 4: 7, slog.LevelDebug: 7, slog.LevelInfo: 6, slog.LevelInfo + 2: 6,
		slog.LevelWarn: 4, slog.LevelError: 3, slog.LevelError + 3: 3, slog.LevelError + 4: 2, slog.LevelError + 8: 2,
	}
	for l, want := range cases {
		if got := Severity(l); got != want {
			t.Errorf("Severity(%v) = %d, want %d", l, got, want)
		}
	}
}

func TestNewWriterValidation(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		cfg  config.Syslog
		want string
	}{
		{config.Syslog{Network: "udp", Facility: "nope"}, "unknown syslog facility"},
		{config.Syslog{Network: "udp", Format: "rfc9999"}, "unknown syslog format"},
		{config.Syslog{Network: "pigeon"}, "unsupported syslog network"},
		{config.Syslog{Network: "tls", Address: "h:1", CAFile: filepath.Join(dir, "missing.pem")}, "syslog ca"},
		{config.Syslog{Network: "tls", Address: "h:1", CertFile: filepath.Join(dir, "c.pem")}, "client cert"},
	}
	empty := filepath.Join(dir, "empty.pem")
	os.WriteFile(empty, []byte("not a cert"), 0o600)
	cases = append(cases, struct {
		cfg  config.Syslog
		want string
	}{config.Syslog{Network: "tls", Address: "h:1", CAFile: empty}, "no certificates found"})
	for _, c := range cases {
		if _, err := NewWriter(c.cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: err = %v, want %q", c.cfg, err, c.want)
		}
	}
	w, err := NewWriter(config.Syslog{Network: "udp", Address: "127.0.0.1:1", Facility: "LOCAL7"})
	if err != nil {
		t.Fatal(err)
	}
	if w.cfg.Tag != "qtestframe" || w.cfg.Format != "rfc5424" || w.facility != 23 {
		t.Errorf("defaults: %+v fac=%d", w.cfg, w.facility)
	}
	w, err = NewWriter(config.Syslog{Network: "udp", Address: "127.0.0.1:1"})
	if err != nil || w.facility != 16 {
		t.Fatalf("default facility: %v %d", err, w.facility)
	}
	if err := w.Close(); err != nil {
		t.Errorf("close without connection: %v", err)
	}
}

func TestFormat(t *testing.T) {
	ts := time.Date(2026, 3, 4, 5, 6, 7, 800, time.FixedZone("X", 3600))
	w, err := NewWriter(config.Syslog{Network: "udp", Address: "x:1", Tag: "app", Facility: "local0"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(w.Format(6, ts, []byte("body")))
	m := rfc5424.FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("not RFC 5424: %q", got)
	}
	if m[1] != "134" || m[2] != "2026-03-04T04:06:07.0000008Z" || m[4] != "app" || m[5] != strconv.Itoa(os.Getpid()) || m[6] != "body" {
		t.Errorf("fields: %q", m)
	}

	w, err = NewWriter(config.Syslog{Network: "udp", Address: "x:1", Tag: "app", Facility: "auth", Format: "rfc3164"})
	if err != nil {
		t.Fatal(err)
	}
	got = string(w.Format(3, ts, []byte("body")))
	want := fmt.Sprintf("<35>%s %s app[%d]: body", ts.Format(time.Stamp), w.hostname, os.Getpid())
	if got != want {
		t.Errorf("rfc3164:\n got %q\nwant %q", got, want)
	}
}

func TestUDPViaNew(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	var out bytes.Buffer
	log, closer, err := New(config.Logging{Level: "info", Format: "text", Syslog: config.Syslog{
		Enabled: true, Network: "udp", Address: pc.LocalAddr().String(), Tag: "qtf", Facility: "local0",
	}}, &out)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	log.With("req", "r1").WithGroup("g").Warn("something", "a", 1)
	log.Debug("filtered")

	_ = pc.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 64<<10)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	msg := string(buf[:n])
	m := rfc5424.FindStringSubmatch(msg)
	if m == nil {
		t.Fatalf("not RFC 5424 (UDP must not be framed): %q", msg)
	}
	if m[1] != "132" || m[4] != "qtf" { // local0 (16*8) + warning (4)
		t.Errorf("header: %q", m)
	}
	if !strings.Contains(m[6], `"msg":"something"`) || !strings.Contains(m[6], `"req":"r1"`) || !strings.Contains(m[6], `"g":{"a":1}`) {
		t.Errorf("body: %q", m[6])
	}
	if s := out.String(); !strings.Contains(s, "req=r1") || !strings.Contains(s, "g.a=1") || strings.Contains(s, "filtered") {
		t.Errorf("local output: %q", s)
	}
	// Nothing else (the debug record) was sent.
	_ = pc.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if n, _, err := pc.ReadFrom(buf); err == nil {
		t.Errorf("unexpected datagram %q", buf[:n])
	}
}

func TestTCPFramingAndReconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ch := streamServer(t, ln, true)
	w, err := NewWriter(config.Syslog{Network: "tcp", Address: ln.Addr().String(), Facility: "local0"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	h := NewSyslogHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(h)
	log.Info("first")
	m := rfc5424.FindStringSubmatch(recv(t, ch))
	if m == nil || m[1] != "134" || !strings.Contains(m[6], `"msg":"first"`) {
		t.Fatalf("first message: %q", m)
	}
	// The server closed the first connection; keep sending until the
	// writer notices and reconnects.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := w.Send(6, time.Now(), []byte("again")); err != nil {
			t.Logf("send: %v", err)
		}
		select {
		case got := <-ch:
			if !strings.HasSuffix(got, "again") {
				t.Fatalf("after reconnect: %q", got)
			}
			return
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("writer did not reconnect")
		}
	}
}

func TestSendFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens any more
	w, err := NewWriter(config.Syslog{Network: "tcp", Address: addr})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Send(6, time.Now(), []byte("x")); err == nil || !strings.Contains(err.Error(), "syslog send") {
		t.Errorf("err = %v", err)
	}
	// The error surfaces through the fan-out handler too.
	var out bytes.Buffer
	h := Fanout(slog.NewTextHandler(&out, nil), NewSyslogHandler(w, nil))
	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	if err := h.Handle(context.Background(), r); err == nil {
		t.Error("fanout should report syslog error")
	}
	if !strings.Contains(out.String(), "boom") {
		t.Error("text handler did not receive record")
	}
}

func TestFanoutEnabled(t *testing.T) {
	var a, b bytes.Buffer
	h := Fanout(
		slog.NewTextHandler(&a, &slog.HandlerOptions{Level: slog.LevelWarn}),
		slog.NewTextHandler(&b, &slog.HandlerOptions{Level: slog.LevelInfo}),
	)
	ctx := context.Background()
	if h.Enabled(ctx, slog.LevelDebug) {
		t.Error("debug should be disabled")
	}
	if !h.Enabled(ctx, slog.LevelInfo) {
		t.Error("info should be enabled")
	}
	slog.New(h).WithGroup("grp").With("x", 1).Info("only-b")
	if a.Len() != 0 || !strings.Contains(b.String(), "grp.x=1") {
		t.Errorf("a=%q b=%q", a.String(), b.String())
	}
}

func TestTLS(t *testing.T) {
	dir := t.TempDir()
	srv := genCert(t, dir, "server")
	cli := genCert(t, dir, "client")
	clientPool := x509.NewCertPool()
	clientPool.AddCert(cli.x509)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{srv.pair},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientPool,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := streamServer(t, ln, false)
	w, err := NewWriter(config.Syslog{
		Network: "tls", Address: ln.Addr().String(), CAFile: srv.cert,
		CertFile: cli.cert, KeyFile: cli.key, Format: "rfc3164", Tag: "tlsapp",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.tlsConf.ServerName != "127.0.0.1" || w.tlsConf.RootCAs == nil || len(w.tlsConf.Certificates) != 1 {
		t.Fatalf("tls config: %+v", w.tlsConf)
	}
	if err := w.Send(6, time.Now(), []byte("secure")); err != nil {
		t.Fatal(err)
	}
	got := recv(t, ch)
	if !strings.HasPrefix(got, "<134>") || !strings.Contains(got, " tlsapp[") || !strings.HasSuffix(got, "]: secure") {
		t.Errorf("message: %q", got)
	}

	// Without the CA the server certificate is rejected.
	w2, err := NewWriter(config.Syslog{Network: "tls", Address: ln.Addr().String(), CertFile: cli.cert, KeyFile: cli.key})
	if err != nil {
		t.Fatal(err)
	}
	if err := w2.Send(6, time.Now(), []byte("x")); err == nil {
		t.Error("expected certificate verification failure")
	}
}

func TestUnixStream(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "s.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	ch := streamServer(t, ln, false)
	w, err := NewWriter(config.Syslog{Network: "unix", Address: path})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Send(3, time.Now(), []byte("over unix")); err != nil {
		t.Fatal(err)
	}
	if m := rfc5424.FindStringSubmatch(recv(t, ch)); m == nil || m[1] != "131" || m[6] != "over unix" {
		t.Errorf("message: %q", m)
	}
	if err := w.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestUnixgram(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "g.sock")
	pc, err := net.ListenPacket("unixgram", path)
	if err != nil {
		t.Skipf("unixgram unavailable: %v", err)
	}
	defer pc.Close()
	w, err := NewWriter(config.Syslog{Network: "unixgram", Address: path})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Send(6, time.Now(), []byte("dgram")); err != nil {
		t.Fatal(err)
	}
	_ = pc.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	// Datagram transports are not octet-counted.
	if m := rfc5424.FindStringSubmatch(string(buf[:n])); m == nil || m[6] != "dgram" {
		t.Errorf("message: %q", buf[:n])
	}
}

func TestNopCloser(t *testing.T) {
	var c io.Closer = nopCloser{}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}
