package settings

import (
	"encoding/json"
	"testing"
)

func TestMonitoringSettings(t *testing.T) {
	ok := map[string]string{
		"metrics.enabled":      `true`,
		"metrics.listen":       `"127.0.0.1:9090"`,
		"metrics.token":        `"<token>"`,
		"metrics.allowed_ips":  `["10.0.0.0/8", "::1"]`,
		"tracing.enabled":      `true`,
		"tracing.endpoint":     `"http://otel:4318"`,
		"tracing.headers":      `"Authorization=Bearer <token>, x-scope=a"`,
		"tracing.sample_ratio": `"0.1"`,
		"tracing.service_name": `"qtf-prod"`,
	}
	for k, raw := range ok {
		if _, err := def(t, k).Decode(json.RawMessage(raw)); err != nil {
			t.Errorf("%s %s: %v", k, raw, err)
		}
		if def(t, k).Section != "monitoring" {
			t.Errorf("%s: section %s", k, def(t, k).Section)
		}
	}
	bad := map[string]string{
		"metrics.listen":       `"9090"`,
		"metrics.token":        `"a\nb"`,
		"metrics.allowed_ips":  `["example.com"]`,
		"tracing.endpoint":     `"otel:4318"`,
		"tracing.headers":      `"no value"`,
		"tracing.sample_ratio": `"1.5"`,
		"tracing.service_name": `"has space"`,
	}
	for k, raw := range bad {
		if _, err := def(t, k).Decode(json.RawMessage(raw)); err == nil {
			t.Errorf("%s accepted %s", k, raw)
		}
	}
	if def(t, "metrics.listen").Apply != Restart || def(t, "metrics.token").Kind != Secret || def(t, "tracing.headers").Kind != Secret {
		t.Fatal("apply and kinds")
	}
}
