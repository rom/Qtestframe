package config

import (
	"strings"
	"testing"
	"time"
)

func TestHostPatterns(t *testing.T) {
	for s, ok := range map[string]bool{"example.com": true, "*.example.com": true, "10.0.0.1": true, "::1": true, "jira": true, "a_b.example": true,
		"": false, "*.": false, "a b": false, "https://x": false, "x/y": false, "a..b": false, strings.Repeat("a", 64) + ".com": false} {
		if ValidHostPattern(s) != ok {
			t.Errorf("%q: %v", s, !ok)
		}
	}
	pats := []string{"Jira.Example.com", "*.corp.example"}
	for h, ok := range map[string]bool{"jira.example.com": true, "JIRA.example.com.": true, "a.corp.example": true, "corp.example": false, "x.example.com": false} {
		if MatchHost(h, pats) != ok {
			t.Errorf("%s: %v", h, !ok)
		}
	}
	p := Provisioning{AllowedDomains: []string{"example.com", "*.example.org"}}
	for e, ok := range map[string]bool{"a@example.com": true, "a@B.example.org": true, "a@example.org": false, "nobody": false} {
		if p.DomainAllowed(e) != ok {
			t.Errorf("%s: %v", e, !ok)
		}
	}
	if !(Provisioning{}).DomainAllowed("x") {
		t.Error("no domains = any")
	}
}

func TestValidateNewSettings(t *testing.T) {
	for name, mut := range map[string]func(*Config){
		"idle":    func(c *Config) { c.Auth.SessionIdleTimeout = 30 * time.Second },
		"classes": func(c *Config) { c.Auth.PasswordMinClasses = 5 },
		"history": func(c *Config) { c.Auth.PasswordHistory = 25 },
		"age":     func(c *Config) { c.Auth.PasswordMaxAgeDays = -1 },
		"tokens":  func(c *Config) { c.Auth.TokenMaxDays = 4000 },
		"proxy":   func(c *Config) { c.Outbound.Proxy = "socks5://x" },
		"hosts":   func(c *Config) { c.Outbound.DeniedHosts = []string{"a b"} },
		"reply": func(c *Config) {
			c.Email.Host, c.Email.From, c.Server.BaseURL, c.Email.ReplyTo = "h", "a@example.com", "https://q", "Name <a@example.com>"
		},
		"icon": func(c *Config) { c.Auth.OIDC = []OIDC{{Name: "a", Issuer: "https://i", ClientID: "c", Icon: "rocket"}} },
		"oidc domains": func(c *Config) {
			c.Auth.OIDC = []OIDC{{Name: "a", Issuer: "https://i", ClientID: "c", Provisioning: Provisioning{AllowedDomains: []string{"10.0.0.1"}}}}
		},
		"saml domains": func(c *Config) {
			c.Auth.SAML = []SAML{{Name: "s", IDPMetadataFile: "x", Provisioning: Provisioning{AllowedDomains: []string{"a@b"}}}}
		},
	} {
		c := Default()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c := Default()
	c.Auth.SessionIdleTimeout, c.Auth.PasswordMinClasses, c.Auth.PasswordHistory, c.Auth.TokenMaxDays = time.Hour, 3, 5, 90
	c.Outbound = Outbound{Proxy: "http://proxy:3128", NoProxy: []string{"*.local"}, AllowedHosts: []string{"jira.example.com"}, BlockPrivate: true}
	c.Auth.OIDC = []OIDC{{Name: "a", Issuer: "https://i", ClientID: "c", Icon: "github", Provisioning: Provisioning{AllowedDomains: []string{"example.com"}}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
