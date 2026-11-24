package tracing

import (
	"encoding/json"
	"math"
	"strconv"
)

// The OTLP/HTTP JSON encoding of ExportTraceServiceRequest
// (opentelemetry-proto, trace/v1): ids are hex strings, 64-bit integers
// decimal strings.

type otlpRequest struct {
	ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
}

type otlpResourceSpans struct {
	Resource   otlpResource     `json:"resource"`
	ScopeSpans []otlpScopeSpans `json:"scopeSpans"`
}

type otlpResource struct {
	Attributes []otlpKV `json:"attributes"`
}

type otlpScopeSpans struct {
	Scope otlpScope  `json:"scope"`
	Spans []otlpSpan `json:"spans"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type otlpSpan struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	ParentSpanID      string     `json:"parentSpanId,omitempty"`
	Name              string     `json:"name"`
	Kind              int        `json:"kind"`
	StartTimeUnixNano string     `json:"startTimeUnixNano"`
	EndTimeUnixNano   string     `json:"endTimeUnixNano"`
	Attributes        []otlpKV   `json:"attributes,omitempty"`
	Status            otlpStatus `json:"status"`
}

type otlpStatus struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type otlpKV struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
	IntValue    *string  `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
}

func kv(a Attr) otlpKV {
	var v otlpValue
	switch x := a.Value.(type) {
	case string:
		v.StringValue = &x
	case bool:
		v.BoolValue = &x
	case int:
		s := strconv.Itoa(x)
		v.IntValue = &s
	case int64:
		s := strconv.FormatInt(x, 10)
		v.IntValue = &s
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			s := strconv.FormatFloat(x, 'g', -1, 64)
			v.StringValue = &s
		} else {
			v.DoubleValue = &x
		}
	default:
		s := ""
		if x != nil {
			b, _ := json.Marshal(x)
			s = string(b)
		}
		v.StringValue = &s
	}
	return otlpKV{Key: a.Key, Value: v}
}

func encodeOTLP(cfg Config, spans []spanData) ([]byte, error) {
	res := []otlpKV{kv(Attr{"service.name", cfg.ServiceName}), kv(Attr{"telemetry.sdk.name", "qtestframe"}), kv(Attr{"telemetry.sdk.language", "go"})}
	if cfg.ServiceVersion != "" {
		res = append(res, kv(Attr{"service.version", cfg.ServiceVersion}))
	}
	out := make([]otlpSpan, 0, len(spans))
	for _, s := range spans {
		o := otlpSpan{TraceID: s.sc.TraceID.String(), SpanID: s.sc.SpanID.String(), Name: s.name, Kind: int(s.kind),
			StartTimeUnixNano: strconv.FormatInt(s.start.UnixNano(), 10), EndTimeUnixNano: strconv.FormatInt(s.end.UnixNano(), 10),
			Status: otlpStatus{Code: s.status, Message: s.statusMsg}}
		if s.parent.IsValid() {
			o.ParentSpanID = s.parent.String()
		}
		for _, a := range s.attrs {
			o.Attributes = append(o.Attributes, kv(a))
		}
		out = append(out, o)
	}
	return json.Marshal(otlpRequest{ResourceSpans: []otlpResourceSpans{{
		Resource:   otlpResource{Attributes: res},
		ScopeSpans: []otlpScopeSpans{{Scope: otlpScope{Name: "qtestframe", Version: cfg.ServiceVersion}, Spans: out}},
	}}})
}
