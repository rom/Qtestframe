package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// S3 is the S3-compatible bucket backups are uploaded to (Amazon S3,
// MinIO, Ceph, Garage, …): one PUT per file (at most 5 GiB), path-style
// addressing, signed with AWS Signature Version 4.
type S3 struct {
	// Endpoint is the base URL, e.g. https://s3.eu-north-1.amazonaws.com
	// or https://minio.example.com:9000.
	Endpoint        string
	Region          string
	Bucket          string
	Prefix          string
	AccessKeyID     string
	SecretAccessKey string
}

// MaxUpload is the largest object one PUT may create.
const MaxUpload int64 = 5 << 30

// Key is the object key of a backup file.
func (c S3) Key(name string) string {
	p := strings.Trim(c.Prefix, "/")
	if p == "" {
		return name
	}
	return p + "/" + name
}

// Check reports what is missing in the configuration.
func (c S3) Check() error {
	u, err := url.Parse(c.Endpoint)
	switch {
	case c.Endpoint == "" || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "":
		return errors.New("backup.s3.endpoint must be the http(s) base URL of the storage, e.g. https://s3.eu-north-1.amazonaws.com")
	case c.Bucket == "":
		return errors.New("backup.s3.bucket is required")
	case c.Region == "":
		return errors.New("backup.s3.region is required (us-east-1 for most S3-compatible servers)")
	case c.AccessKeyID == "" || c.SecretAccessKey == "":
		return errors.New("backup.s3.access_key_id and backup.s3.secret_access_key are required")
	}
	return nil
}

// Upload stores the file at path as the object key; sum is its hex SHA-256
// (signed, so the storage rejects a corrupted upload).
func (c S3) Upload(ctx context.Context, client *http.Client, key, path, sum string, now time.Time) error {
	if err := c.Check(); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() > MaxUpload {
		return fmt.Errorf("upload: %d bytes is more than one S3 upload takes (5 GiB)", st.Size())
	}
	u := strings.TrimSuffix(c.Endpoint, "/") + "/" + uriEncode(c.Bucket, false) + "/" + uriEncode(key, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, f)
	if err != nil {
		return err
	}
	req.ContentLength = st.Size()
	req.GetBody = func() (io.ReadCloser, error) {
		g, err := os.Open(path)
		return g, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	signV4(req, sum, c.AccessKeyID, c.SecretAccessKey, c.Region, "s3", now)
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("upload: %s: %s", res.Status, s3Message(b))
	}
	return nil
}

// s3Message extracts <Code> and <Message> of an S3 error document.
func s3Message(b []byte) string {
	s := string(b)
	pick := func(tag string) string {
		i := strings.Index(s, "<"+tag+">")
		j := strings.Index(s, "</"+tag+">")
		if i < 0 || j < i {
			return ""
		}
		return s[i+len(tag)+2 : j]
	}
	if c, m := pick("Code"), pick("Message"); c != "" {
		return strings.TrimSpace(c + " " + m)
	}
	return strings.Join(strings.Fields(s), " ")
}

// uriEncode encodes s as SigV4 wants: everything but A–Z, a–z, 0–9, '-',
// '.', '_' and '~' (and '/' when keepSlash) as %XX.
func uriEncode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		case c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func hmacSHA256(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// signV4 signs req (AWS Signature Version 4, header authorization) with
// the payload hash payload ("UNSIGNED-PAYLOAD" for none). The host and
// every header already set on req are signed.
func signV4(req *http.Request, payload, keyID, secret, region, service string, now time.Time) {
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payload)
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	headers := map[string]string{"host": host}
	for k, v := range req.Header {
		headers[strings.ToLower(k)] = strings.Join(strings.Fields(strings.Join(v, ",")), " ")
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canon strings.Builder
	for _, k := range names {
		canon.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	creq := strings.Join([]string{req.Method, path, canonicalQuery(req.URL.Query()), canon.String(), signed, payload}, "\n")
	scope := day + "/" + region + "/" + service + "/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex(creq)
	k := hmacSHA256([]byte("AWS4"+secret), day)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+keyID+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := append([]string{}, q[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, uriEncode(k, false)+"="+uriEncode(v, false))
		}
	}
	return strings.Join(parts, "&")
}
