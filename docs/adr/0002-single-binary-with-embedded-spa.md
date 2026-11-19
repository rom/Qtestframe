# 2. Go backend with the SPA embedded into a single binary

Date: 2026-11-19

## Status

Accepted

## Context

Qtestframe is installed on-premises by organisations that run it on a
single hardened Linux host, in a container, or behind a load balancer. The
operational surface should be small: few files to install, no runtime
interpreter, no separate web server for static assets, and a clear unit to
confine with systemd, SELinux and AppArmor. The server also needs good
standard-library support for HTTP/2, TLS, OIDC/SAML clients and Linux
system calls (prctl, capset, Landlock, seccomp).

## Decision

* The backend is written in Go (see `go.mod` for the toolchain version)
  and built with `CGO_ENABLED=0` into one static binary,
  `cmd/qtestframe`.
* The Vue client is built by Vite into `internal/webui/dist` and embedded
  with `//go:embed all:dist` (`internal/webui/webui.go`). The API server
  serves it with an `index.html` fallback for client-side routes.
* The same binary contains the administrative CLI (`migrate`,
  `user create`, `tenant create`, `member add`, `seed-demo`,
  `check-config`, `healthcheck`, `version`) and the runner sandbox entry
  point (`sandbox-exec`).
* SQLite support uses the pure-Go `modernc.org/sqlite` driver so that
  cgo stays disabled.

## Consequences

* One artefact to install, sign, label (`qtestframe_exec_t`) and upgrade;
  the container image is distroless/static.
* API and UI are always the same version; no CORS configuration is
  needed, and the CSP can be `'self'` only.
* `CGO_ENABLED=0` is also a functional requirement: the hardening code uses
  `syscall.AllThreadsSyscall`, which is unavailable in cgo builds.
* Building requires Node (for the client) before `go build`; a binary built
  without the client still serves the API and answers `/` with 404 and a
  hint to run `make web`.
* Changing a single UI asset requires a new binary.
