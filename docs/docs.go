// Package docs embeds documentation files that the server serves, such as
// the OpenAPI description of the REST API.
package docs

import _ "embed"

// OpenAPI is the OpenAPI 3.1 description of the REST API (openapi.yaml).
//
//go:embed openapi.yaml
var OpenAPI []byte
