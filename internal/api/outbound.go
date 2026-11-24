package api

// Outgoing HTTP calls (webhooks, chat, defect trackers, requirement
// sources, AI, embeddings, OIDC discovery, environment health checks) go
// through one client whose transport follows the system settings of the
// outbound section at every request: the proxy, the allowed and denied
// hosts and the block of non-public addresses (protection against
// server-side request forgery). Tests that set Server.HTTPClient replace it.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/tracing"
)

// errOutboundBlocked is returned for a call the outbound settings forbid.
var errOutboundBlocked = errors.New("outbound call blocked")

// outboundClient returns the client of outgoing calls of s.
func (s *Server) outboundClient() *http.Client {
	// inside a traced request the call gets a span and a traceparent header
	return &http.Client{Timeout: 60 * time.Second, Transport: &tracing.Transport{Base: newOutboundTransport(func() config.Outbound { return s.conf().Outbound })}}
}

func newOutboundTransport(conf func() config.Outbound) *outboundTransport {
	t := &outboundTransport{conf: conf}
	t.base = t.build()
	return t
}

// outboundTransport applies config.Outbound to each request.
type outboundTransport struct {
	conf func() config.Outbound
	base *http.Transport
}

func (t *outboundTransport) build() *http.Transport {
	b := http.DefaultTransport.(*http.Transport).Clone()
	b.Proxy = t.proxy
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	b.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		c := t.conf()
		if c.BlockPrivate && ctx.Value(viaProxy{}) == nil {
			// checked on the address actually dialled (no DNS rebinding)
			host, _, _ := net.SplitHostPort(addr)
			if !config.MatchHost(host, c.AllowedHosts) {
				dd := *d
				dd.Control = func(_, address string, _ syscall.RawConn) error {
					h, _, err := net.SplitHostPort(address)
					if err != nil {
						return err
					}
					if ip := net.ParseIP(h); ip != nil && !publicIP(ip) {
						return fmt.Errorf("%w: %s is not a public address (outbound.block_private)", errOutboundBlocked, h)
					}
					return nil
				}
				return dd.DialContext(ctx, network, addr)
			}
		}
		return d.DialContext(ctx, network, addr)
	}
	return b
}

// viaProxy marks requests sent through a proxy (the dial goes to the
// proxy, whose address may be private).
type viaProxy struct{}

func (t *outboundTransport) proxy(r *http.Request) (*url.URL, error) {
	c := t.conf()
	if c.Proxy == "" {
		return http.ProxyFromEnvironment(r)
	}
	if config.MatchHost(r.URL.Hostname(), c.NoProxy) {
		return nil, nil
	}
	return url.Parse(c.Proxy)
}

// RoundTrip checks the host of the request and sends it.
func (t *outboundTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := checkOutbound(r.Context(), t.conf(), r.URL); err != nil {
		return nil, err
	}
	tr := t.base
	if p, err := t.proxy(r); err == nil && p != nil {
		r = r.WithContext(context.WithValue(r.Context(), viaProxy{}, true))
	}
	return tr.RoundTrip(r)
}

// checkOutbound reports whether the outbound settings allow a call to u.
// With block_private, a call through a proxy is checked on the addresses
// the host resolves to here.
func checkOutbound(ctx context.Context, c config.Outbound, u *url.URL) error {
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: no host", errOutboundBlocked)
	}
	if config.MatchHost(host, c.DeniedHosts) {
		return fmt.Errorf("%w: %s is in outbound.denied_hosts", errOutboundBlocked, host)
	}
	allowed := config.MatchHost(host, c.AllowedHosts)
	if len(c.AllowedHosts) > 0 && !allowed {
		return fmt.Errorf("%w: %s is not in outbound.allowed_hosts", errOutboundBlocked, host)
	}
	if !c.BlockPrivate || allowed {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if !publicIP(ip) {
			return fmt.Errorf("%w: %s is not a public address (outbound.block_private)", errOutboundBlocked, host)
		}
		return nil
	}
	if c.Proxy != "" && !config.MatchHost(host, c.NoProxy) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return err
		}
		for _, a := range addrs {
			if !publicIP(a.IP) {
				return fmt.Errorf("%w: %s resolves to %s, not a public address (outbound.block_private)", errOutboundBlocked, host, a.IP)
			}
		}
	}
	return nil
}

// publicIP reports whether ip is a public unicast address.
func publicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, n := range nonPublic {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// nonPublic are further ranges that are not reachable on the internet
// (carrier-grade NAT, benchmarking, documentation, IPv4-mapped loopback
// and private ranges are covered by the net.IP methods).
var nonPublic = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "2001:db8::/32"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()
