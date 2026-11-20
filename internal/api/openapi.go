package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/rom/qtestframe/docs"
)

// Route is a registered "METHOD /path" pattern with the permissions the
// router checks before the handler runs (Public: no authentication).
type Route struct {
	Pattern     string
	Permissions []string
	Public      bool
}

// Routes returns the registered routes in registration order.
func (s *Server) Routes() []Route {
	s.once.Do(s.routes)
	return append([]Route(nil), s.routeTable...)
}

var (
	openAPIOnce sync.Once
	openAPIJSON []byte
	openAPIErr  error
)

// openAPISpec returns the embedded description (docs/openapi.yaml) as JSON.
func openAPISpec(src []byte) ([]byte, error) {
	var doc any
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	return json.Marshal(jsonable(doc))
}

// jsonable turns the maps decoded from YAML into JSON-encodable maps.
func jsonable(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = jsonable(e)
		}
	case map[any]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[fmt.Sprint(k)] = jsonable(e)
		}
		return m
	case []any:
		for i, e := range x {
			x[i] = jsonable(e)
		}
	}
	return v
}

// handleOpenAPI serves the OpenAPI description without authentication:
// openapi.json as JSON, openapi.yaml as written.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	if strings.HasSuffix(r.URL.Path, ".yaml") {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(docs.OpenAPI)
		return
	}
	openAPIOnce.Do(func() { openAPIJSON, openAPIErr = openAPISpec(docs.OpenAPI) })
	if openAPIErr != nil {
		s.fail(w, r, openAPIErr)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(openAPIJSON)
}
