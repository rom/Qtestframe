package api

import (
	"encoding/json"
	"io"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/rom/qtestframe/docs"
)

var openAPIMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// loadOpenAPI parses docs/openapi.yaml.
func loadOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	var spec map[string]any
	if err := yaml.Unmarshal(docs.OpenAPI, &spec); err != nil {
		t.Fatalf("docs/openapi.yaml does not parse: %v", err)
	}
	return spec
}

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }

// TestOpenAPIRoutesInSync fails when a route registered with h(…) or
// pub(…) is missing from docs/openapi.yaml, when the description lists an
// operation that is not registered, or when x-permission differs from the
// permissions the router checks.
func TestOpenAPIRoutesInSync(t *testing.T) {
	e := newEnv(t)
	spec := loadOpenAPI(t)
	type op struct{ perm string }
	documented := map[string]op{}
	for path, item := range asMap(spec["paths"]) {
		for _, m := range openAPIMethods {
			o := asMap(asMap(item)[m])
			if o == nil {
				continue
			}
			perm, _ := o["x-permission"].(string)
			documented[strings.ToUpper(m)+" "+path] = op{perm}
		}
	}
	registered := map[string]Route{}
	for _, r := range e.srv.Routes() {
		registered[r.Pattern] = r
	}
	var missing, extra, perms []string
	for pattern, r := range registered {
		d, ok := documented[pattern]
		if !ok {
			missing = append(missing, pattern)
			continue
		}
		want := "authenticated"
		switch {
		case r.Public:
			want = "none"
		case len(r.Permissions) > 0:
			want = strings.Join(r.Permissions, ", ")
		}
		if d.perm != want {
			perms = append(perms, pattern+": x-permission "+`"`+d.perm+`", want "`+want+`"`)
		}
	}
	for pattern := range documented {
		if _, ok := registered[pattern]; !ok {
			extra = append(extra, pattern)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(perms)
	if len(missing) > 0 {
		t.Errorf("%d registered routes are missing from docs/openapi.yaml:\n%s", len(missing), strings.Join(missing, "\n"))
	}
	if len(extra) > 0 {
		t.Errorf("%d operations in docs/openapi.yaml are not registered routes:\n%s", len(extra), strings.Join(extra, "\n"))
	}
	if len(perms) > 0 {
		t.Errorf("%d operations have a wrong x-permission:\n%s", len(perms), strings.Join(perms, "\n"))
	}
}

// TestOpenAPIStructure checks the parts of the OpenAPI 3.1 structure that
// tools rely on: required fields, unique operation ids, declared path
// parameters, resolvable $refs and response objects.
func TestOpenAPIStructure(t *testing.T) {
	spec := loadOpenAPI(t)
	if v, _ := spec["openapi"].(string); !strings.HasPrefix(v, "3.1.") {
		t.Errorf("openapi: %v", spec["openapi"])
	}
	info := asMap(spec["info"])
	if info["title"] == nil || info["version"] == nil {
		t.Errorf("info needs title and version: %v", info)
	}
	comps := asMap(spec["components"])
	for _, s := range []string{"sessionCookie", "csrfToken", "bearerToken"} {
		if asMap(comps["securitySchemes"])[s] == nil {
			t.Errorf("security scheme %s missing", s)
		}
	}
	errSchema := asMap(asMap(comps["schemas"])["Error"])
	if props := asMap(errSchema["properties"]); props["error"] == nil || props["code"] == nil {
		t.Errorf("Error schema: %v", errSchema)
	}
	tags := map[string]bool{}
	for _, tg := range spec["tags"].([]any) {
		tags[asMap(tg)["name"].(string)] = true
	}
	// every $ref resolves
	var walk func(v any, at string)
	walk = func(v any, at string) {
		switch x := v.(type) {
		case map[string]any:
			if r, ok := x["$ref"].(string); ok {
				if !strings.HasPrefix(r, "#/") {
					t.Errorf("%s: external $ref %s", at, r)
				} else {
					var cur any = spec
					for _, part := range strings.Split(r[2:], "/") {
						cur = asMap(cur)[part]
					}
					if cur == nil {
						t.Errorf("%s: unresolved $ref %s", at, r)
					}
				}
			}
			for k, e := range x {
				walk(e, at+"/"+k)
			}
		case []any:
			for _, e := range x {
				walk(e, at+"[]")
			}
		}
	}
	walk(spec, "#")

	resolve := func(v any) map[string]any {
		m := asMap(v)
		if r, ok := m["$ref"].(string); ok {
			var cur any = spec
			for _, part := range strings.Split(strings.TrimPrefix(r, "#/"), "/") {
				cur = asMap(cur)[part]
			}
			return asMap(cur)
		}
		return m
	}
	ids := map[string]string{}
	paramRe := regexp.MustCompile(`\{([^}]+)\}`)
	statusRe := regexp.MustCompile(`^([1-5][0-9][0-9]|[1-5]XX|default)$`)
	for path, item := range asMap(spec["paths"]) {
		if !strings.HasPrefix(path, "/") {
			t.Errorf("path %q must start with /", path)
		}
		declared := map[string]bool{}
		for _, p := range asSlice(asMap(item)["parameters"]) {
			if pm := resolve(p); pm["in"] == "path" {
				declared[pm["name"].(string)] = true
			}
		}
		for _, m := range paramRe.FindAllStringSubmatch(path, -1) {
			if !declared[m[1]] {
				t.Errorf("%s: path parameter %s not declared", path, m[1])
			}
		}
		n := 0
		for _, m := range openAPIMethods {
			o := asMap(asMap(item)[m])
			if o == nil {
				continue
			}
			n++
			where := strings.ToUpper(m) + " " + path
			id, _ := o["operationId"].(string)
			if id == "" {
				t.Errorf("%s: operationId missing", where)
			} else if other, dup := ids[id]; dup {
				t.Errorf("%s: operationId %s also used by %s", where, id, other)
			}
			ids[id] = where
			if s, _ := o["summary"].(string); s == "" {
				t.Errorf("%s: summary missing", where)
			}
			if d, _ := o["description"].(string); !strings.Contains(d, "**Permission:**") {
				t.Errorf("%s: the description must name the permission", where)
			}
			for _, tg := range asSlice(o["tags"]) {
				if !tags[tg.(string)] {
					t.Errorf("%s: undeclared tag %v", where, tg)
				}
			}
			res := asMap(o["responses"])
			if len(res) == 0 {
				t.Errorf("%s: no responses", where)
			}
			for code, r := range res {
				if !statusRe.MatchString(code) {
					t.Errorf("%s: response key %q", where, code)
				}
				if d, _ := resolve(r)["description"].(string); d == "" {
					t.Errorf("%s %s: response needs a description", where, code)
				}
			}
			if rb := o["requestBody"]; rb != nil && asMap(resolve(rb)["content"]) == nil {
				t.Errorf("%s: requestBody without content", where)
			}
			for _, p := range asSlice(o["parameters"]) {
				pm := resolve(p)
				if pm["name"] == nil || (pm["in"] != "query" && pm["in"] != "header" && pm["in"] != "path" && pm["in"] != "cookie") {
					t.Errorf("%s: bad parameter %v", where, pm)
				}
			}
		}
		if n == 0 {
			t.Errorf("%s: no operations", path)
		}
	}
}

func asSlice(v any) []any { s, _ := v.([]any); return s }

// TestOpenAPIServed checks the unauthenticated JSON and YAML endpoints.
func TestOpenAPIServed(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	res := c.raw("GET", "/api/v1/openapi.json", nil)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var spec map[string]any
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") || json.Unmarshal(data, &spec) != nil {
		t.Fatalf("openapi.json: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if spec["openapi"] != "3.1.0" || asMap(asMap(spec["paths"])["/api/v1/projects/{pid}/export"])["get"] == nil {
		t.Errorf("served spec: %v", spec["openapi"])
	}
	res = c.raw("GET", "/api/v1/openapi.yaml", nil)
	data, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/yaml") || string(data) != string(docs.OpenAPI) {
		t.Errorf("openapi.yaml: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if _, err := openAPISpec([]byte("a: [")); err == nil {
		t.Error("bad YAML accepted")
	}
	if b, err := openAPISpec([]byte("a:\n  200: x\n  l: [{1: y}]\n")); err != nil || string(b) != `{"a":{"200":"x","l":[{"1":"y"}]}}` {
		t.Errorf("%s %v", b, err)
	}
}
