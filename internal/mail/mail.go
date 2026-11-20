// Package mail builds MIME e-mail messages and sends them over SMTP.
//
// Messages are plain text with an optional HTML alternative and optional
// attachments. Header values are checked for line breaks (no header
// injection) and non-ASCII subjects and names are encoded (RFC 2047).
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrInvalid marks a message that cannot be sent as it is (bad address,
// line break in a header, no recipients).
var ErrInvalid = errors.New("invalid message")

// Address is a mailbox with an optional display name.
type Address struct {
	Name  string
	Email string
}

// String formats the address for a header, encoding the name if needed.
func (a Address) String() string {
	return (&mail.Address{Name: a.Name, Address: a.Email}).String()
}

// Attachment is a file sent with a message.
type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

// Message is an e-mail.
type Message struct {
	From    Address
	ReplyTo *Address
	To      []Address
	Subject string
	// Text is the plain text body (required); HTML an optional alternative.
	Text        string
	HTML        string
	Attachments []Attachment
	// Headers are extra headers (e.g. List-Unsubscribe).
	Headers map[string]string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m *Message) error
}

func invalidf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func oneLine(s string) bool { return !strings.ContainsAny(s, "\r\n\x00") }

// CheckAddress validates a bare e-mail address (no name, no line breaks).
func CheckAddress(email string) error {
	if !oneLine(email) {
		return invalidf("address must not contain line breaks")
	}
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || !strings.Contains(email, "@") || len(email) > 254 {
		return invalidf("%q is not a valid e-mail address", email)
	}
	return nil
}

func checkAddr(a Address) error {
	if !oneLine(a.Name) {
		return invalidf("name must not contain line breaks")
	}
	return CheckAddress(a.Email)
}

// Validate checks the message before it is built or sent.
func (m *Message) Validate() error {
	if err := checkAddr(m.From); err != nil {
		return fmt.Errorf("from: %w", err)
	}
	if m.ReplyTo != nil {
		if err := checkAddr(*m.ReplyTo); err != nil {
			return fmt.Errorf("reply-to: %w", err)
		}
	}
	if len(m.To) == 0 {
		return invalidf("no recipients")
	}
	for _, a := range m.To {
		if err := checkAddr(a); err != nil {
			return err
		}
	}
	if !oneLine(m.Subject) {
		return invalidf("subject must not contain line breaks")
	}
	for k, v := range m.Headers {
		if !validHeaderName(k) || !oneLine(v) {
			return invalidf("bad header %q", k)
		}
	}
	for _, at := range m.Attachments {
		if !oneLine(at.Name) || !oneLine(at.ContentType) || at.Name == "" {
			return invalidf("bad attachment name or type")
		}
	}
	return nil
}

func validHeaderName(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	switch strings.ToLower(k) {
	case "from", "to", "cc", "bcc", "subject", "content-type", "mime-version", "content-transfer-encoding", "date", "message-id", "reply-to":
		return false
	}
	return true
}

// Recipients returns the bare addresses of To.
func (m *Message) Recipients() []string {
	out := make([]string, len(m.To))
	for i, a := range m.To {
		out[i] = a.Email
	}
	return out
}

func boundary() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "qtf-" + hex.EncodeToString(b)
}

// Bytes renders the message as RFC 5322 text with CRLF line endings.
func (m *Message) Bytes() ([]byte, error) {
	return m.render(time.Now())
}

func (m *Message) render(now time.Time) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	hdr := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	hdr("From", m.From.String())
	to := make([]string, len(m.To))
	for i, a := range m.To {
		to[i] = a.String()
	}
	hdr("To", strings.Join(to, ", "))
	if m.ReplyTo != nil {
		hdr("Reply-To", m.ReplyTo.String())
	}
	hdr("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	hdr("Date", now.Format(time.RFC1123Z))
	domain := m.From.Email[strings.LastIndex(m.From.Email, "@")+1:]
	hdr("Message-ID", "<"+boundary()[4:]+"@"+domain+">")
	hdr("MIME-Version", "1.0")
	keys := make([]string, 0, len(m.Headers))
	for k := range m.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		hdr(k, m.Headers[k])
	}
	body := func(w *bytes.Buffer) {
		if m.HTML == "" {
			writeText(w, "text/plain", m.Text)
			return
		}
		alt := boundary()
		w.WriteString("Content-Type: multipart/alternative; boundary=\"" + alt + "\"\r\n\r\n")
		w.WriteString("--" + alt + "\r\n")
		writeText(w, "text/plain", m.Text)
		w.WriteString("\r\n--" + alt + "\r\n")
		writeText(w, "text/html", m.HTML)
		w.WriteString("\r\n--" + alt + "--\r\n")
	}
	if len(m.Attachments) == 0 {
		body(&b)
		return b.Bytes(), nil
	}
	mixed := boundary()
	b.WriteString("Content-Type: multipart/mixed; boundary=\"" + mixed + "\"\r\n\r\n")
	b.WriteString("--" + mixed + "\r\n")
	body(&b)
	for _, at := range m.Attachments {
		b.WriteString("\r\n--" + mixed + "\r\n")
		ct := at.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		if _, _, err := mime.ParseMediaType(ct); err != nil {
			return nil, invalidf("bad attachment type %q", ct)
		}
		b.WriteString("Content-Type: " + ct + "\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		b.WriteString("Content-Disposition: " + mime.FormatMediaType("attachment", map[string]string{"filename": at.Name}) + "\r\n\r\n")
		enc := base64.StdEncoding.EncodeToString(at.Data)
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	b.WriteString("\r\n--" + mixed + "--\r\n")
	return b.Bytes(), nil
}

func writeText(w *bytes.Buffer, ct, text string) {
	w.WriteString("Content-Type: " + ct + "; charset=utf-8\r\n")
	w.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(w)
	_, _ = qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")))
	_ = qp.Close()
}

// ---------------------------------------------------------------- fake

// Fake records messages instead of sending them (tests). Err, when set, is
// returned by Send.
type Fake struct {
	mu   sync.Mutex
	msgs []*Message
	Err  error
}

// Send records m (after validating and rendering it).
func (f *Fake) Send(ctx context.Context, m *Message) error {
	if _, err := m.Bytes(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.msgs = append(f.msgs, m)
	return nil
}

// Messages returns the messages sent so far.
func (f *Fake) Messages() []*Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*Message(nil), f.msgs...)
}

// Reset forgets the messages sent so far.
func (f *Fake) Reset() {
	f.mu.Lock()
	f.msgs = nil
	f.mu.Unlock()
}
