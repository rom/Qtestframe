package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

func msg() *Message {
	return &Message{
		From:    Address{Name: "Qtestframe", Email: "noreply@example.com"},
		To:      []Address{{Name: "Åsa Öberg", Email: "asa@example.com"}, {Email: "bo@example.com"}},
		Subject: "Rapport: täckning ✓",
		Text:    "Hej!\nRad två är lång " + strings.Repeat("x", 100),
	}
}

func TestBytesPlain(t *testing.T) {
	m := msg()
	b, err := m.render(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	pm, err := mail.ReadMessage(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	dec := new(mime.WordDecoder)
	subj, _ := dec.DecodeHeader(pm.Header.Get("Subject"))
	if subj != m.Subject {
		t.Fatalf("subject %q", subj)
	}
	to, err := pm.Header.AddressList("To")
	if err != nil || len(to) != 2 || to[0].Name != "Åsa Öberg" || to[1].Address != "bo@example.com" {
		t.Fatal(to, err)
	}
	if !strings.HasPrefix(pm.Header.Get("Content-Type"), "text/plain; charset=utf-8") || pm.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
		t.Fatal(pm.Header)
	}
	if !strings.Contains(pm.Header.Get("Message-ID"), "@example.com>") || pm.Header.Get("Date") == "" {
		t.Fatal(pm.Header)
	}
	if !strings.Contains(string(b), "\r\n") || strings.Contains(strings.ReplaceAll(string(b), "\r\n", ""), "\n") {
		t.Fatal("line endings must be CRLF")
	}
	if got := m.Recipients(); len(got) != 2 || got[0] != "asa@example.com" {
		t.Fatal(got)
	}
}

func TestBytesAlternativeAndAttachments(t *testing.T) {
	m := msg()
	m.HTML = "<p>Hej</p>"
	m.ReplyTo = &Address{Name: "Ann", Email: "ann@example.com"}
	m.Headers = map[string]string{"List-Unsubscribe": "<https://x/profile>", "Auto-Submitted": "auto-generated"}
	m.Attachments = []Attachment{{Name: "täckning.pdf", ContentType: "application/pdf", Data: []byte(strings.Repeat("%PDF", 50))}, {Name: "x.bin", Data: []byte{0, 1}}}
	b, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	pm, err := mail.ReadMessage(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	if pm.Header.Get("Reply-To") == "" || pm.Header.Get("List-Unsubscribe") != "<https://x/profile>" {
		t.Fatal(pm.Header)
	}
	mt, params, err := mime.ParseMediaType(pm.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/mixed" {
		t.Fatal(mt, err)
	}
	mr := multipart.NewReader(pm.Body, params["boundary"])
	var parts []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ct := p.Header.Get("Content-Type")
		parts = append(parts, ct)
		if strings.HasPrefix(ct, "multipart/alternative") {
			_, ap, _ := mime.ParseMediaType(ct)
			ar := multipart.NewReader(p, ap["boundary"])
			n := 0
			for {
				q, err := ar.NextPart()
				if err != nil {
					break
				}
				body, _ := io.ReadAll(q) // quoted-printable is decoded by the reader
				if n == 1 && string(body) != "<p>Hej</p>" {
					t.Fatalf("html %q", body)
				}
				n++
			}
			if n != 2 {
				t.Fatal("alternative parts", n)
			}
		}
		if ct == "application/pdf" {
			if p.FileName() != "täckning.pdf" {
				t.Fatal(p.FileName())
			}
			data, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
			if string(data) != strings.Repeat("%PDF", 50) {
				t.Fatal("attachment data")
			}
		}
	}
	if len(parts) != 3 || parts[2] != "application/octet-stream" {
		t.Fatal(parts)
	}
}

func TestValidate(t *testing.T) {
	for name, mut := range map[string]func(*Message){
		"to crlf":      func(m *Message) { m.To[0].Email = "a@example.com\r\nBcc: x@y.z" },
		"name crlf":    func(m *Message) { m.To[0].Name = "A\nBcc: x" },
		"subject crlf": func(m *Message) { m.Subject = "Hi\r\nBcc: x@y.z" },
		"from":         func(m *Message) { m.From.Email = "nope" },
		"reply-to":     func(m *Message) { m.ReplyTo = &Address{Email: "bad"} },
		"no to":        func(m *Message) { m.To = nil },
		"named addr":   func(m *Message) { m.To[0].Email = "A <a@example.com>" },
		"header name":  func(m *Message) { m.Headers = map[string]string{"Bcc": "x@y.z"} },
		"header value": func(m *Message) { m.Headers = map[string]string{"X-A": "a\nb"} },
		"header chars": func(m *Message) { m.Headers = map[string]string{"X A": "a"} },
		"header empty": func(m *Message) { m.Headers = map[string]string{"": "a"} },
		"attachment":   func(m *Message) { m.Attachments = []Attachment{{Name: "a\nb"}} },
		"att type":     func(m *Message) { m.Attachments = []Attachment{{Name: "a", ContentType: "bad type/"}} },
	} {
		m := msg()
		mut(m)
		if _, err := m.Bytes(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if CheckAddress("a@example.com") != nil || CheckAddress("x") == nil || CheckAddress(strings.Repeat("a", 250)+"@b.cd") == nil {
		t.Fatal("CheckAddress")
	}
}

func TestFake(t *testing.T) {
	f := &Fake{}
	if err := f.Send(context.Background(), msg()); err != nil {
		t.Fatal(err)
	}
	bad := msg()
	bad.Subject = "a\nb"
	if f.Send(context.Background(), bad) == nil {
		t.Fatal("invalid message accepted")
	}
	f.Err = errors.New("down")
	if f.Send(context.Background(), msg()) == nil || len(f.Messages()) != 1 {
		t.Fatal("fake error")
	}
	f.Reset()
	if len(f.Messages()) != 0 {
		t.Fatal("reset")
	}
}

// ---------------------------------------------------------------- SMTP

type received struct {
	from string
	rcpt []string
	data string
	auth string
	tls  bool
}

// smtpServer is a tiny SMTP server for the tests (optionally STARTTLS,
// implicit TLS and AUTH PLAIN).
type smtpServer struct {
	ln       net.Listener
	cert     *tls.Config
	starttls bool
	auth     bool
	rejectTo string
	mu       sync.Mutex
	got      []received
}

func selfSigned(t *testing.T) (*tls.Config, *x509.CertPool) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(c)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, pool
}

func startSMTP(t *testing.T, srv *smtpServer, implicit bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if implicit {
		ln = tls.NewListener(ln, srv.cert)
	}
	srv.ln = ln
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.serve(c, implicit)
		}
	}()
	return ln.Addr().String()
}

func (s *smtpServer) serve(c net.Conn, secure bool) {
	defer c.Close()
	r, w := bufio.NewReader(c), bufio.NewWriter(c)
	say := func(l string) { w.WriteString(l + "\r\n"); w.Flush() }
	say("220 test ESMTP")
	var cur received
	cur.tls = secure
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			w.WriteString("250-test\r\n")
			if s.starttls && !cur.tls {
				w.WriteString("250-STARTTLS\r\n")
			}
			if s.auth {
				w.WriteString("250-AUTH PLAIN\r\n")
			}
			say("250 8BITMIME")
		case cmd == "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(c, s.cert)
			if err := tc.Handshake(); err != nil {
				return
			}
			c, r, w = tc, bufio.NewReader(tc), bufio.NewWriter(tc)
			say = func(l string) { w.WriteString(l + "\r\n"); w.Flush() }
			cur.tls = true
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN"):]))
			cur.auth = string(raw)
			if strings.HasSuffix(cur.auth, "\x00wrong") {
				say("535 bad credentials")
				continue
			}
			say("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			cur.from, _, _ = strings.Cut(strings.TrimLeft(line[len("MAIL FROM:"):], "< "), ">")
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			to := strings.Trim(line[len("RCPT TO:"):], "<> ")
			if to == s.rejectTo {
				say("550 no such user")
				continue
			}
			cur.rcpt = append(cur.rcpt, to)
			say("250 ok")
		case cmd == "DATA":
			say("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			cur.data = b.String()
			s.mu.Lock()
			s.got = append(s.got, cur)
			s.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func split(t *testing.T, addr string) (string, int) {
	host, port, _ := net.SplitHostPort(addr)
	var n int
	for _, c := range port {
		n = n*10 + int(c-'0')
	}
	return host, n
}

func TestSMTPModes(t *testing.T) {
	cert, pool := selfSigned(t)
	for _, mode := range []string{"none", "starttls", "tls"} {
		srv := &smtpServer{cert: cert, starttls: mode == "starttls", auth: mode != "none"}
		host, port := split(t, startSMTP(t, srv, mode == "tls"))
		cfg := SMTPConfig{Host: host, Port: port, TLS: mode, Timeout: 5 * time.Second, TLSConfig: &tls.Config{RootCAs: pool}}
		if mode != "none" {
			cfg.Username, cfg.Password = "qtf", "secret"
		}
		if err := NewSMTP(cfg).Send(context.Background(), msg()); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if len(srv.got) != 1 {
			t.Fatalf("%s: got %d", mode, len(srv.got))
		}
		g := srv.got[0]
		if g.from != "noreply@example.com" || len(g.rcpt) != 2 || !strings.Contains(g.data, "Subject: =?utf-8?") || g.tls != (mode != "none") {
			t.Fatalf("%s: %+v", mode, g)
		}
		if mode != "none" && g.auth != "\x00qtf\x00secret" {
			t.Fatalf("%s: auth %q", mode, g.auth)
		}
	}
}

func TestSMTPErrors(t *testing.T) {
	cert, pool := selfSigned(t)
	ctx := context.Background()
	// no STARTTLS offered
	srv := &smtpServer{cert: cert}
	host, port := split(t, startSMTP(t, srv, false))
	err := NewSMTP(SMTPConfig{Host: host, Port: port, TLS: "starttls"}).Send(ctx, msg())
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatal(err)
	}
	// authentication wanted, not offered
	if err := NewSMTP(SMTPConfig{Host: host, Port: port, TLS: "none", Username: "u", Password: "p"}).Send(ctx, msg()); err == nil {
		t.Fatal("auth without AUTH")
	}
	// wrong password
	srv2 := &smtpServer{cert: cert, starttls: true, auth: true}
	host2, port2 := split(t, startSMTP(t, srv2, false))
	err = NewSMTP(SMTPConfig{Host: host2, Port: port2, TLS: "starttls", Username: "u", Password: "wrong", TLSConfig: &tls.Config{RootCAs: pool}}).Send(ctx, msg())
	if err == nil || !strings.Contains(err.Error(), "auth") {
		t.Fatal(err)
	}
	// untrusted certificate
	if err := NewSMTP(SMTPConfig{Host: host2, Port: port2, TLS: "starttls"}).Send(ctx, msg()); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	// rejected recipient
	srv3 := &smtpServer{cert: cert, rejectTo: "bo@example.com"}
	host3, port3 := split(t, startSMTP(t, srv3, false))
	err = NewSMTP(SMTPConfig{Host: host3, Port: port3, TLS: "none"}).Send(ctx, msg())
	if err == nil || !strings.Contains(err.Error(), "bo@example.com") {
		t.Fatal(err)
	}
	// nothing listening
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	_, p4 := split(t, ln.Addr().String())
	ln.Close()
	if err := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: p4, TLS: "none", Timeout: time.Second}).Send(ctx, msg()); err == nil || !strings.Contains(err.Error(), "connect") {
		t.Fatal(err)
	}
	// invalid message: nothing is sent
	bad := msg()
	bad.To = nil
	if err := NewSMTP(SMTPConfig{Host: host3, Port: port3, TLS: "none"}).Send(ctx, bad); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	// a server that never answers: the timeout ends the attempt
	quiet, _ := net.Listen("tcp", "127.0.0.1:0")
	defer quiet.Close()
	go func() {
		for {
			c, err := quiet.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	_, p5 := split(t, quiet.Addr().String())
	start := time.Now()
	if err := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: p5, TLS: "none", Timeout: 300 * time.Millisecond}).Send(ctx, msg()); err == nil || time.Since(start) > 5*time.Second {
		t.Fatal("timeout", err)
	}
}
