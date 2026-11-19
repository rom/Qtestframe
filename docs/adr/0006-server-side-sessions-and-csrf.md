# 6. Server-side sessions with hashed tokens, CSRF token and Origin check instead of JWT

Date: 2026-11-19

## Status

Accepted

## Context

Browsers need a login that survives page reloads, can be revoked
immediately (logout, disabled user, password reset) and is not readable by
JavaScript. CI systems need long-lived non-interactive credentials. Stateless
JWTs avoid a database lookup but cannot be revoked before expiry without a
deny-list, tend to end up in `localStorage`, and add key management. Every
request already touches the database.

## Decision

* **Sessions** are rows in `sessions`. The browser receives a random
  32-byte token in the cookie `qtf_session` (`HttpOnly`, `SameSite=Lax`,
  `Path=/`, `Secure` unless `auth.cookie_secure: false`); the database stores
  only its SHA-256 hash as the session ID. Sessions have an absolute
  lifetime (`auth.session_ttl`, default 12 h) and expired rows are deleted
  on access and by an hourly purge.
* Each session has a random **CSRF token** (24 bytes), returned as
  `csrf_token` by `/auth/login` and `/auth/me`. For session-authenticated
  requests with a method other than GET/HEAD, `authed` requires
  `X-CSRF-Token` to match (constant-time compare) **and** an `Origin`
  header, if present, to match the request host or `server.base_url`
  (`sameOrigin` in `internal/api/authn.go`).
* **API tokens** (`qtf_` + 32 random bytes, base64url) are stored as SHA-256
  hashes with a display prefix, bound to one tenant and a role, optionally
  expiring, and sent as `Authorization: Bearer`. They are exempt from CSRF
  checks because browsers never attach them automatically.
* Login state for OIDC/SAML is a short-lived HMAC-signed cookie, not a
  session.

## Consequences

* Logout deletes the session; disabling a user or resetting a password
  through `PATCH /users/{id}` deletes all of the user's sessions. A database
  leak does not reveal usable session or API tokens.
* One extra indexed lookup per request.
* Any node with database access can validate any session (HA-friendly);
  there is no signing key to distribute for sessions.
* `SameSite=Lax` plus the CSRF token plus the Origin check give three
  independent CSRF defences for the API; public endpoints (login, logout,
  SSO callbacks) are outside `authed` and not CSRF-checked.
* The session ID is not rotated on tenant switch (the session's tenant is
  updated in place); a new session is created at every login.
