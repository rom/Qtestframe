package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"time"
)

// SMTPConfig configures an SMTP server.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	// TLS is "starttls" (upgrade, required), "tls" (implicit TLS, port
	// 465) or "none" (plain; only for local relays).
	TLS     string
	Timeout time.Duration
	// TLSConfig overrides the TLS settings (tests: a private CA).
	TLSConfig *tls.Config
}

// SMTP sends messages through an SMTP server.
type SMTP struct {
	Cfg SMTPConfig
}

// NewSMTP returns a sender for cfg.
func NewSMTP(cfg SMTPConfig) *SMTP { return &SMTP{Cfg: cfg} }

func (s *SMTP) tlsConfig() *tls.Config {
	if s.Cfg.TLSConfig != nil {
		c := s.Cfg.TLSConfig.Clone()
		if c.ServerName == "" {
			c.ServerName = s.Cfg.Host
		}
		return c
	}
	return &tls.Config{ServerName: s.Cfg.Host, MinVersion: tls.VersionTLS12}
}

// Send delivers m. The whole exchange is bounded by ctx and Cfg.Timeout.
func (s *SMTP) Send(ctx context.Context, m *Message) error {
	data, err := m.Bytes()
	if err != nil {
		return err
	}
	timeout := s.Cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	addr := net.JoinHostPort(s.Cfg.Host, strconv.Itoa(s.Cfg.Port))
	d := net.Dialer{}
	var conn net.Conn
	if s.Cfg.TLS == "tls" {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: s.tlsConfig()}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp: connect %s: %w", addr, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	// close the connection when ctx ends early (e.g. shutdown)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	c, err := smtp.NewClient(conn, s.Cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp: %w", err)
	}
	defer c.Close()
	if err := c.Hello("qtestframe.localhost"); err != nil {
		return fmt.Errorf("smtp: hello: %w", err)
	}
	if s.Cfg.TLS == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("smtp: the server does not offer STARTTLS (set email.tls to tls or none)")
		}
		if err := c.StartTLS(s.tlsConfig()); err != nil {
			return fmt.Errorf("smtp: starttls: %w", err)
		}
	}
	if s.Cfg.Username != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("smtp: the server does not support authentication")
		}
		if err := c.Auth(smtp.PlainAuth("", s.Cfg.Username, s.Cfg.Password, s.Cfg.Host)); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}
	if err := c.Mail(m.From.Email); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}
	for _, rcpt := range m.Recipients() {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp: rcpt %s: %w", rcpt, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("smtp: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	return c.Quit()
}
