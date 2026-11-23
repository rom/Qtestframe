package prompt

import (
	"regexp"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// The solution preamble (model.SolutionPreamble): the warnings about
// credentials, the redaction applied when a preamble is rendered into a
// prompt, and the templates for common stacks.

// Warning is a finding of CheckPreamble: a field that looks like it holds a
// credential.
type Warning struct {
	// Field is the JSON name of the field.
	Field string `json:"field"`
	// Kind: password_in_url, password_assignment, private_key, token.
	Kind string `json:"kind"`
	// Message describes the finding (English; the client translates Kind).
	Message string `json:"message"`
	// Excerpt shows where, with the secret part masked.
	Excerpt string `json:"excerpt"`
}

// Warning kinds.
const (
	WarnPasswordInURL      = "password_in_url"
	WarnPasswordAssignment = "password_assignment"
	WarnPrivateKey         = "private_key"
	WarnToken              = "token"
)

var warnMessages = map[string]string{
	WarnPasswordInURL:      "a connection string or URL with a password; describe the connection without the password",
	WarnPasswordAssignment: "a password, secret or key assignment; describe where the secret is kept instead (for example a vault or an environment variable)",
	WarnPrivateKey:         "a private key; never put keys in the preamble",
	WarnToken:              "something that looks like an access key or token; remove it",
}

// secretRule finds a credential; group 1 (when the rule has one) is the
// secret part that is masked, else the whole match.
type secretRule struct {
	kind string
	re   *regexp.Regexp
	// value checks the secret part (nil: always a finding).
	value func(string) bool
}

const masked = "[REDACTED]"

var secretRules = []secretRule{
	// scheme://user:password@host
	{WarnPasswordInURL, regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.\-]*://[^\s/:@]+:([^\s@/]+)@`), realSecret},
	// ADO.NET / JDBC / libpq key=value connection strings and assignments
	{WarnPasswordAssignment, regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|pass|secret|client[_-]?secret|api[_-]?key|access[_-]?key|secret[_-]?key|auth[_-]?token|access[_-]?token)\s*=\s*("[^"]*"|'[^']*'|<[^>]*>|\$\{[^}]*\}|[^\s;,&"']+)`), realSecret},
	// JSON / YAML style with a quoted value
	{WarnPasswordAssignment, regexp.MustCompile(`(?i)["']?\b(?:password|passwd|pwd|secret|client[_-]?secret|api[_-]?key|access[_-]?key|secret[_-]?key|auth[_-]?token|access[_-]?token)["']?\s*:\s*("[^"]+"|'[^']+')`), realSecret},
	// YAML style with an unquoted value that looks generated
	{WarnPasswordAssignment, regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|client[_-]?secret|api[_-]?key|secret[_-]?key)\s*:\s*([^\s"']{6,})`), generated},
	{WarnPrivateKey, regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`), nil},
	// well-known token shapes (cloud access key ids, code host and chat
	// tokens, JSON web tokens)
	{WarnToken, regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), nil},
	{WarnToken, regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,}|glpat-[A-Za-z0-9_\-]{20,}|xox[abprs]-[A-Za-z0-9\-]{10,}|sk-[A-Za-z0-9_\-]{20,})`), nil},
	{WarnToken, regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`), nil},
	// a long random-looking string (letters of both cases and digits)
	{WarnToken, regexp.MustCompile(`[A-Za-z0-9+/_\-]{40,}={0,2}`), randomLooking},
}

// placeholder reports whether a secret part is a placeholder, not a value:
// ${VAR}, $VAR, %VAR%, <password>, {{ secret }}, ***, "" or an obvious
// description.
func placeholder(v string) bool {
	v = strings.Trim(v, `"'`)
	if v == "" {
		return true
	}
	switch {
	case strings.HasPrefix(v, "$"), strings.HasPrefix(v, "%") && strings.HasSuffix(v, "%"),
		strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">"), strings.HasPrefix(v, "{{"), strings.HasPrefix(v, "{"),
		strings.Trim(v, "*x•.") == "", v == masked:
		return true
	}
	switch strings.ToLower(v) {
	case "none", "null", "empty", "redacted", "secret", "password", "changeme", "vault", "env", "environment",
		"from-vault", "from_vault", "from-env", "see-vault", "tbd", "n/a", "na", "required", "optional", "hidden", "masked":
		return true
	}
	return false
}

func realSecret(v string) bool { return !placeholder(v) }

// hashing and protocol words that follow "password:" in descriptions
var notSecrets = map[string]bool{"argon2": true, "argon2id": true, "argon2i": true, "bcrypt": true, "scrypt": true, "pbkdf2": true,
	"sha256": true, "sha-256": true, "sha512": true, "oauth2": true, "oidc": true, "saml2": true}

// generated reports whether an unquoted YAML value looks like a generated
// secret rather than a word: it mixes letters with digits or symbols.
func generated(v string) bool {
	if placeholder(v) || notSecrets[strings.ToLower(strings.TrimRight(v, ".,;"))] {
		return false
	}
	letters, digits, other := 0, 0, 0
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letters++
		case r >= '0' && r <= '9':
			digits++
		default:
			other++
		}
	}
	return letters > 0 && (digits > 0 || other > 0)
}

// randomLooking: upper and lower case letters and digits, no long words.
func randomLooking(v string) bool {
	var up, low, dig bool
	run, maxRun := 0, 0
	for _, r := range v {
		switch {
		case r >= 'A' && r <= 'Z':
			up = true
			run = 0
		case r >= 'a' && r <= 'z':
			low = true
			run++
		case r >= '0' && r <= '9':
			dig = true
			run = 0
		default:
			run = 0
		}
		maxRun = max(maxRun, run)
	}
	return up && low && dig && maxRun < 12
}

// findSecrets returns the findings in a text: kind, the masked excerpt and
// the byte range of the secret part.
func findSecrets(s string) (out []struct {
	kind, excerpt string
	from, to      int
}) {
	for _, rule := range secretRules {
		for _, m := range rule.re.FindAllStringSubmatchIndex(s, -1) {
			from, to := m[0], m[1]
			if len(m) >= 4 && m[2] >= 0 {
				from, to = m[2], m[3]
			}
			if rule.value != nil && !rule.value(s[from:to]) {
				continue
			}
			overlaps := false
			for _, o := range out {
				if from < o.to && to > o.from {
					overlaps = true
				}
			}
			if overlaps {
				continue
			}
			ex := s[m[0]:from] + masked + s[to:m[1]]
			if len(ex) > 120 {
				ex = ex[:117] + "…"
			}
			out = append(out, struct {
				kind, excerpt string
				from, to      int
			}{rule.kind, ex, from, to})
		}
	}
	return out
}

// CheckPreamble returns warnings for the fields that look like they hold a
// credential (a connection string with a password, a key, a token). The
// preamble is meant for descriptions only.
func CheckPreamble(p *model.SolutionPreamble) []Warning {
	out := []Warning{}
	for _, f := range model.PreambleFields {
		for _, x := range findSecrets(*p.Field(f)) {
			out = append(out, Warning{Field: f, Kind: x.kind, Message: warnMessages[x.kind], Excerpt: x.excerpt})
		}
	}
	return out
}

// Redact masks what looks like a credential in a text.
func Redact(s string) string {
	found := findSecrets(s)
	if len(found) == 0 {
		return s
	}
	// replace from the end so the ranges stay valid
	for i := 0; i < len(found); i++ {
		for j := i + 1; j < len(found); j++ {
			if found[j].from > found[i].from {
				found[i], found[j] = found[j], found[i]
			}
		}
	}
	for _, x := range found {
		s = s[:x.from] + masked + s[x.to:]
	}
	return s
}

// ForbiddenPreambleKey reports whether a field name of a preamble request
// asks to store a secret (password, secret, token, credential, key): such
// fields are refused.
func ForbiddenPreambleKey(k string) bool {
	k = strings.ToLower(k)
	for _, w := range []string{"password", "passwd", "secret", "token", "credential", "api_key", "apikey", "private_key", "connection_string"} {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- templates

// Template prefills a preamble for a common stack; every field stays
// editable.
type Template struct {
	ID string `json:"id"`
	// Name per language.
	Name     map[string]string      `json:"name"`
	Preamble model.SolutionPreamble `json:"preamble"`
}

// Templates are the preamble templates, in display order.
var Templates = []Template{
	{ID: "go-postgres-vue", Name: map[string]string{"en": "Go + PostgreSQL + Vue", "sv": "Go + PostgreSQL + Vue"}, Preamble: model.SolutionPreamble{
		Languages:         "Go 1.26 (backend), TypeScript 5 (frontend), SQL",
		Frameworks:        "Go standard library net/http (routing with method patterns), pgx (PostgreSQL driver), Vue 3 with the Composition API, Pinia, Vue Router, PrimeVue, vue-i18n",
		Environments:      "Linux containers (OCI images) on Kubernetes or a single VM; current evergreen browsers (Chrome, Firefox, Safari, Edge); desktop and tablet",
		ArchitectureStyle: "modular_monolith",
		ArchitectureNotes: "One Go binary serves the REST API (JSON over HTTPS, /api/v1) and the built single-page application; packages per domain (api, store, model); no shared mutable global state",
		Frontend:          "Vue 3 single-page application built with Vite; components with <script setup lang=\"ts\">; accessible (WCAG 2.2 AA)",
		Backend:           "Go HTTP server; handlers → store (SQL) → model; context-based cancellation; structured logging with log/slog",
		Messaging:         "None at first; server-sent events for live updates",
		Caching:           "In-process caches only; HTTP caching headers for static assets",
		Databases:         "PostgreSQL 16; the application owns its schema; versioned SQL migrations embedded in the binary and applied on start; connection settings come from the environment (no credentials in the repository)",
		Integrations:      "OIDC identity provider for single sign-on; SMTP for e-mail; outgoing webhooks",
		Deployment:        "Container image built in CI; pipeline runs gofmt, go vet, unit and integration tests (against PostgreSQL), frontend type checks and unit tests, then end-to-end tests with Playwright; deployment through the pipeline",
		Performance:       "95th percentile API response under 300 ms for lists of 10 000 items",
		Availability:      "99.5 % per month; zero-downtime deployments with rolling updates",
		Scalability:       "Up to 500 concurrent users per instance; horizontal scaling behind a load balancer",
		Security:          "OIDC/OAuth 2.0 sign-in, role-based access control, CSRF protection, secrets from the environment or a vault, TLS everywhere, OWASP ASVS level 2, GDPR (personal data minimised and exportable)",
		Conventions:       "gofmt and go vet clean; errors wrapped with context; table-driven tests; ESLint + Prettier for TypeScript; small focused commits referencing requirement keys",
		Testing:           "Unit tests per package (coverage ≥ 85 %), integration tests against real PostgreSQL, component tests with Vitest, end-to-end tests with Playwright, accessibility checks with axe",
		Repository:        "cmd/<binary>, internal/<package>, web/ (frontend), docs/, deploy/; one repository",
		OutOfScope:        "Native mobile apps; offline mode",
	}},
	{ID: "java-spring-oracle", Name: map[string]string{"en": "Java Spring Boot + Oracle", "sv": "Java Spring Boot + Oracle"}, Preamble: model.SolutionPreamble{
		Languages:         "Java 21 (LTS), SQL (Oracle dialect)",
		Frameworks:        "Spring Boot 3, Spring Web, Spring Data JPA (Hibernate), Spring Security, Bean Validation, MapStruct",
		Environments:      "Linux containers on an on-premises Kubernetes (OpenShift) cluster; internal users on current Chrome and Edge",
		ArchitectureStyle: "layered",
		ArchitectureNotes: "Controller → service → repository layers; domain model separate from the REST DTOs",
		Frontend:          "Server-provided REST API consumed by an existing web front end",
		Backend:           "Spring Boot services exposing REST/JSON, documented with OpenAPI",
		Messaging:         "IBM MQ or Kafka for asynchronous integration events",
		Caching:           "Spring Cache with Caffeine (local) for reference data",
		Databases:         "Oracle Database 19c; schema owned by the application; migrations with Flyway; access through a connection pool configured from the platform's secret store",
		Integrations:      "Corporate LDAP / Active Directory, internal REST services, batch file exchange over SFTP",
		Deployment:        "Maven build in Jenkins or GitLab CI; container image to the internal registry; Helm charts per environment (test, acceptance, production)",
		Performance:       "95th percentile response under 500 ms at 200 requests per second",
		Availability:      "99.9 % during business hours; planned maintenance windows",
		Scalability:       "Stateless services, two or more replicas",
		Security:          "Spring Security with OAuth 2.0 resource server (JWT from the corporate identity provider); audit logging; data classified per company policy",
		Conventions:       "Google Java Style; Checkstyle and SpotBugs in the build; constructor injection",
		Testing:           "JUnit 5 and Mockito unit tests, Spring Boot slice tests, Testcontainers integration tests, contract tests for the REST API",
		Repository:        "Maven multi-module project: api, domain, persistence, app",
		OutOfScope:        "Changes to the existing front end; data migration from the legacy system",
	}},
	{ID: "dotnet-sqlserver", Name: map[string]string{"en": ".NET + SQL Server", "sv": ".NET + SQL Server"}, Preamble: model.SolutionPreamble{
		Languages:         "C# 12 on .NET 8 (LTS), T-SQL",
		Frameworks:        "ASP.NET Core (minimal APIs or controllers), Entity Framework Core, FluentValidation, Serilog",
		Environments:      "Azure App Service or AKS (containers); Windows Server on-premises as an alternative; browsers: Edge and Chrome",
		ArchitectureStyle: "modular_monolith",
		ArchitectureNotes: "Clean architecture: Domain, Application, Infrastructure and Web projects",
		Frontend:          "Blazor or a separate SPA consuming the REST API",
		Backend:           "ASP.NET Core Web API with OpenAPI (Swashbuckle)",
		Messaging:         "Azure Service Bus for background work and integration events",
		Caching:           "IMemoryCache; Azure Cache for Redis for shared state",
		Databases:         "Microsoft SQL Server 2022 / Azure SQL; schema owned by the application; EF Core migrations; managed identity or integrated authentication for the connection (no passwords in configuration files)",
		Integrations:      "Microsoft Entra ID (single sign-on), Microsoft Graph, external REST APIs",
		Deployment:        "Azure DevOps or GitHub Actions: build, test, publish, deploy with infrastructure as code (Bicep or Terraform)",
		Performance:       "95th percentile response under 400 ms",
		Availability:      "99.9 % per month",
		Scalability:       "Scale out to several instances; stateless web tier",
		Security:          "Entra ID with OpenID Connect, policy-based authorisation, Key Vault for secrets, encryption at rest and in transit",
		Conventions:       ".NET coding conventions, nullable reference types enabled, analyzers as errors in CI",
		Testing:           "xUnit unit tests, integration tests with WebApplicationFactory and a test database, Playwright for end-to-end",
		Repository:        "src/<Project>.*, tests/<Project>.*.Tests, one solution file",
		OutOfScope:        "Desktop clients",
	}},
	{ID: "python-fastapi-postgres", Name: map[string]string{"en": "Python FastAPI + PostgreSQL", "sv": "Python FastAPI + PostgreSQL"}, Preamble: model.SolutionPreamble{
		Languages:         "Python 3.12, SQL",
		Frameworks:        "FastAPI, Pydantic v2, SQLAlchemy 2 (async), Alembic, Uvicorn",
		Environments:      "Linux containers on Kubernetes in a public cloud",
		ArchitectureStyle: "microservices",
		ArchitectureNotes: "A few services split by business capability; each owns its data",
		Frontend:          "Separate web client (not part of this solution) using the REST API",
		Backend:           "FastAPI services with OpenAPI generated from the code; async I/O",
		Messaging:         "RabbitMQ or Kafka for events between services",
		Caching:           "Redis for sessions and hot reads",
		Databases:         "PostgreSQL 16, one database per service; migrations with Alembic; connection settings injected by the platform",
		Integrations:      "Payment provider REST API, e-mail service, OAuth 2.0 identity provider",
		Deployment:        "GitHub Actions or GitLab CI: lint (ruff), type check (mypy), tests, build image, deploy with Helm or Argo CD",
		Performance:       "95th percentile under 200 ms for reads, 500 ms for writes",
		Availability:      "99.9 % per month",
		Scalability:       "Horizontal autoscaling on CPU and queue length",
		Security:          "OAuth 2.0 / OIDC bearer tokens, scopes per endpoint, input validation with Pydantic, secrets from the cloud secret manager, GDPR",
		Conventions:       "PEP 8 via ruff, type hints everywhere, dependency injection with FastAPI Depends",
		Testing:           "pytest with pytest-asyncio, httpx test client, Testcontainers for PostgreSQL, contract tests between services",
		Repository:        "One repository per service: app/, tests/, alembic/, Dockerfile",
		OutOfScope:        "Front-end development",
	}},
	{ID: "node-ts-mongodb", Name: map[string]string{"en": "Node.js/TypeScript + MongoDB", "sv": "Node.js/TypeScript + MongoDB"}, Preamble: model.SolutionPreamble{
		Languages:         "TypeScript 5 on Node.js 22 (LTS)",
		Frameworks:        "NestJS (or Express), Mongoose, Zod, React or Next.js for the client",
		Environments:      "Serverless functions or containers in a public cloud; evergreen browsers and mobile browsers",
		ArchitectureStyle: "serverless",
		ArchitectureNotes: "API gateway in front of functions or small services; static front end on a CDN",
		Frontend:          "React (Next.js) with TypeScript, server-side rendering for public pages",
		Backend:           "Node.js REST or GraphQL API",
		Messaging:         "Cloud queue or event bus for background jobs",
		Caching:           "CDN for static content; Redis for API caching",
		Databases:         "MongoDB 7 (Atlas or self-hosted replica set); collections owned by the application; schema validation and migrations with migrate-mongo; connection URI from the secret manager",
		Integrations:      "Third-party REST APIs, OAuth providers, e-mail and push notification services",
		Deployment:        "GitHub Actions: lint, type check, test, deploy with infrastructure as code",
		Performance:       "Largest Contentful Paint under 2.5 s on mobile; API p95 under 300 ms",
		Availability:      "99.9 % per month",
		Scalability:       "Scales to zero and out automatically",
		Security:          "OAuth 2.0 / OIDC, rate limiting, input validation, OWASP Top 10, secrets in the cloud secret manager",
		Conventions:       "ESLint + Prettier, strict TypeScript, conventional commits",
		Testing:           "Vitest or Jest unit tests, Supertest API tests, Playwright end-to-end tests, MongoDB memory server for tests",
		Repository:        "Monorepo with apps/ and packages/ (pnpm workspaces)",
		OutOfScope:        "Native apps",
	}},
	{ID: "embedded-c-rtos", Name: map[string]string{"en": "Embedded C + RTOS", "sv": "Inbyggt C + RTOS"}, Preamble: model.SolutionPreamble{
		Languages:         "C11 (MISRA C:2012 subset), some C++17 for host tools, Python 3 for build and test scripts",
		Frameworks:        "FreeRTOS (or Zephyr), vendor HAL, CMSIS, lwIP, mbedTLS",
		Environments:      "ARM Cortex-M microcontroller (e.g. 512 KB flash, 128 KB RAM); host tools on Linux and Windows",
		ArchitectureStyle: "event_driven",
		ArchitectureNotes: "Tasks communicating through queues; hardware abstraction layer; no dynamic memory allocation after start-up",
		Frontend:          "Device display and buttons; optional companion app over BLE",
		Backend:           "Firmware tasks: sensor acquisition, control loop, communication, diagnostics",
		Messaging:         "RTOS queues and event groups; CAN or UART protocol to other nodes; MQTT over TLS to the cloud",
		Caching:           "Not applicable",
		Databases:         "No database on the device; configuration in a versioned flash region with CRC; telemetry stored in the cloud back end",
		Integrations:      "Bootloader with secure firmware update (signed images), cloud IoT platform",
		Deployment:        "CMake build with arm-none-eabi-gcc in CI; static analysis; signed release images; over-the-air update with rollback",
		Performance:       "Control loop period 1 ms with jitter under 50 µs; boot under 500 ms",
		Availability:      "Watchdog recovery within 2 s; fail-safe state on fault",
		Scalability:       "Fleet of up to 100 000 devices",
		Security:          "Secure boot, signed updates, keys in a secure element, IEC 62443-4-2 / ETSI EN 303 645, functional safety per IEC 61508 where applicable",
		Conventions:       "MISRA C:2012 with documented deviations, clang-format, no recursion, fixed-width integer types",
		Testing:           "Unit tests on the host (Unity/CMock or GoogleTest), hardware-in-the-loop tests, static analysis (cppcheck, MISRA checker), coverage on the host",
		Repository:        "firmware/ (app, drivers, hal, rtos-config), tools/, tests/, docs/",
		OutOfScope:        "The cloud back end and the companion app",
	}},
}

// FindTemplate returns a template by id (nil: unknown).
func FindTemplate(id string) *Template {
	for i := range Templates {
		if Templates[i].ID == id {
			return &Templates[i]
		}
	}
	return nil
}
