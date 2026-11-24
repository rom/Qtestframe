package tracing

import (
	"net/http"
)

// Transport traces outgoing HTTP requests: inside a span it starts a
// client span and sends its context in the traceparent header.
type Transport struct {
	Base http.RoundTripper
}

// RoundTrip sends r with the trace context of its context.
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	p := SpanFromContext(r.Context())
	if p == nil || p.tracer == nil {
		return base.RoundTrip(r)
	}
	ctx, s := p.tracer.Start(r.Context(), r.Method, KindClient,
		Attr{"http.request.method", r.Method}, Attr{"server.address", r.URL.Hostname()}, Attr{"url.full", redactURL(r)})
	if s == nil { // tracing was turned off meanwhile
		return base.RoundTrip(r)
	}
	r2 := r.Clone(ctx)
	r2.Header.Set(Header, Traceparent(s.Context()))
	res, err := base.RoundTrip(r2)
	if err != nil {
		s.SetError(err)
	} else {
		s.SetAttr(Attr{"http.response.status_code", res.StatusCode})
		if res.StatusCode >= 400 {
			s.SetStatus(StatusError, res.Status)
		}
	}
	s.End()
	return res, err
}

// redactURL is the URL of r without user information and query (tokens
// are often sent in either).
func redactURL(r *http.Request) string {
	u := *r.URL
	u.User = nil
	u.RawQuery, u.ForceQuery = "", false
	u.Fragment, u.RawFragment = "", ""
	return u.String()
}
