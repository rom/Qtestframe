// Package logging configures structured logging (log/slog) with optional
// forwarding to a central syslog server over UDP, TCP, TLS or a Unix socket
// using RFC 5424 or RFC 3164 framing.
package logging

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/tracing"
)

// ParseLevel converts a level name to slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	var l slog.Level
	err := l.UnmarshalText([]byte(strings.ToUpper(strings.TrimSpace(s))))
	return l, err
}

// New builds a logger writing to out and, if enabled, to syslog. The
// returned closer releases the syslog connection.
func New(cfg config.Logging, out io.Writer) (*slog.Logger, io.Closer, error) {
	lvl, err := ParseLevel(cfg.Level)
	if err != nil {
		return nil, nil, fmt.Errorf("logging: %w", err)
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if cfg.Format == "json" {
		h = slog.NewJSONHandler(out, opts)
	} else {
		h = slog.NewTextHandler(out, opts)
	}
	var closer io.Closer = nopCloser{}
	if cfg.Syslog.Enabled {
		w, err := NewWriter(cfg.Syslog)
		if err != nil {
			return nil, nil, err
		}
		closer = w
		h = Fanout(h, NewSyslogHandler(w, opts))
	}
	return slog.New(TraceIDs(h)), closer, nil
}

// TraceIDs returns a handler that adds trace_id (and span_id, for a span
// of this server) to the records logged with a context of a trace (see
// internal/tracing), so log lines can be matched with traces.
func TraceIDs(h slog.Handler) slog.Handler { return traceHandler{h} }

type traceHandler struct{ slog.Handler }

func (t traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if sc, ok := tracing.SpanContextFromContext(ctx); ok && sc.IsValid() {
			r = r.Clone()
			r.AddAttrs(slog.String("trace_id", sc.TraceID.String()))
			if !sc.Remote {
				r.AddAttrs(slog.String("span_id", sc.SpanID.String()))
			}
		}
	}
	return t.Handler.Handle(ctx, r)
}

func (t traceHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return traceHandler{t.Handler.WithAttrs(a)}
}
func (t traceHandler) WithGroup(n string) slog.Handler { return traceHandler{t.Handler.WithGroup(n)} }

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// ---------------------------------------------------------------- fanout

type fanout []slog.Handler

// Fanout returns a handler duplicating records to all handlers.
func Fanout(hs ...slog.Handler) slog.Handler { return fanout(hs) }

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanout) WithAttrs(a []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(a)
	}
	return out
}

func (f fanout) WithGroup(n string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(n)
	}
	return out
}

// ---------------------------------------------------------------- syslog handler

// SyslogHandler formats records as JSON and sends each as one syslog
// message with a severity derived from the slog level.
type SyslogHandler struct {
	h  slog.Handler
	st *syslogState
}

type syslogState struct {
	mu  sync.Mutex
	w   *Writer
	buf bytes.Buffer
}

// NewSyslogHandler returns a handler sending to w.
func NewSyslogHandler(w *Writer, opts *slog.HandlerOptions) *SyslogHandler {
	st := &syslogState{w: w}
	return &SyslogHandler{h: slog.NewJSONHandler(&st.buf, opts), st: st}
}

// Enabled implements slog.Handler.
func (s *SyslogHandler) Enabled(ctx context.Context, l slog.Level) bool { return s.h.Enabled(ctx, l) }

// Handle implements slog.Handler.
func (s *SyslogHandler) Handle(ctx context.Context, r slog.Record) error {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	s.st.buf.Reset()
	if err := s.h.Handle(ctx, r); err != nil {
		return err
	}
	return s.st.w.Send(Severity(r.Level), r.Time, bytes.TrimRight(s.st.buf.Bytes(), "\n"))
}

// WithAttrs implements slog.Handler.
func (s *SyslogHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &SyslogHandler{h: s.h.WithAttrs(a), st: s.st}
}

// WithGroup implements slog.Handler.
func (s *SyslogHandler) WithGroup(n string) slog.Handler {
	return &SyslogHandler{h: s.h.WithGroup(n), st: s.st}
}

// Severity maps slog levels to syslog severities.
func Severity(l slog.Level) int {
	switch {
	case l >= slog.LevelError+4:
		return 2 // critical
	case l >= slog.LevelError:
		return 3
	case l >= slog.LevelWarn:
		return 4
	case l >= slog.LevelInfo:
		return 6
	}
	return 7
}

// ---------------------------------------------------------------- writer

var facilities = map[string]int{
	"kern": 0, "user": 1, "mail": 2, "daemon": 3, "auth": 4, "syslog": 5, "lpr": 6,
	"news": 7, "uucp": 8, "cron": 9, "authpriv": 10, "ftp": 11,
	"local0": 16, "local1": 17, "local2": 18, "local3": 19,
	"local4": 20, "local5": 21, "local6": 22, "local7": 23,
}

// Writer sends syslog messages and reconnects on failure.
type Writer struct {
	cfg      config.Syslog
	facility int
	hostname string
	tlsConf  *tls.Config
	dial     func() (net.Conn, error)

	mu   sync.Mutex
	conn net.Conn
}

// NewWriter validates cfg and returns a writer (connection is lazy).
func NewWriter(cfg config.Syslog) (*Writer, error) {
	if cfg.Facility == "" {
		cfg.Facility = "local0"
	}
	fac, ok := facilities[strings.ToLower(cfg.Facility)]
	if !ok {
		return nil, fmt.Errorf("logging: unknown syslog facility %q", cfg.Facility)
	}
	if cfg.Tag == "" {
		cfg.Tag = "qtestframe"
	}
	if cfg.Format == "" {
		cfg.Format = "rfc5424"
	}
	if cfg.Format != "rfc5424" && cfg.Format != "rfc3164" {
		return nil, fmt.Errorf("logging: unknown syslog format %q", cfg.Format)
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "-"
	}
	w := &Writer{cfg: cfg, facility: fac, hostname: host}
	switch cfg.Network {
	case "udp", "tcp", "unix", "unixgram":
		network := cfg.Network
		w.dial = func() (net.Conn, error) { return net.DialTimeout(network, cfg.Address, 5*time.Second) }
	case "tls":
		tc, err := tlsConfig(cfg)
		if err != nil {
			return nil, err
		}
		w.tlsConf = tc
		w.dial = func() (net.Conn, error) {
			return tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", cfg.Address, tc)
		}
	default:
		return nil, fmt.Errorf("logging: unsupported syslog network %q", cfg.Network)
	}
	return w, nil
}

func tlsConfig(cfg config.Syslog) (*tls.Config, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12}
	if h, _, err := net.SplitHostPort(cfg.Address); err == nil {
		tc.ServerName = h
	}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("logging: syslog ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("logging: syslog ca: no certificates found")
		}
		tc.RootCAs = pool
	}
	if cfg.CertFile != "" || cfg.KeyFile != "" {
		kp, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("logging: syslog client cert: %w", err)
		}
		tc.Certificates = []tls.Certificate{kp}
	}
	return tc, nil
}

// Format renders one syslog message (without transport framing).
func (w *Writer) Format(sev int, t time.Time, msg []byte) []byte {
	pri := w.facility*8 + sev
	var b bytes.Buffer
	if w.cfg.Format == "rfc3164" {
		fmt.Fprintf(&b, "<%d>%s %s %s[%d]: ", pri, t.Format(time.Stamp), w.hostname, w.cfg.Tag, os.Getpid())
	} else {
		fmt.Fprintf(&b, "<%d>1 %s %s %s %d - - ", pri, t.UTC().Format(time.RFC3339Nano), w.hostname, w.cfg.Tag, os.Getpid())
	}
	b.Write(msg)
	return b.Bytes()
}

// Send delivers a message, reconnecting once on failure.
func (w *Writer) Send(sev int, t time.Time, msg []byte) error {
	data := w.Format(sev, t, msg)
	stream := w.cfg.Network == "tcp" || w.cfg.Network == "tls" || w.cfg.Network == "unix"
	if stream {
		// RFC 6587 octet counting
		data = append([]byte(fmt.Sprintf("%d ", len(data))), data...)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if w.conn == nil {
			if w.conn, err = w.dial(); err != nil {
				w.conn = nil
				continue
			}
		}
		_ = w.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err = w.conn.Write(data); err == nil {
			return nil
		}
		w.conn.Close()
		w.conn = nil
	}
	return fmt.Errorf("logging: syslog send: %w", err)
}

// Close closes the connection.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn == nil {
		return nil
	}
	err := w.conn.Close()
	w.conn = nil
	return err
}
