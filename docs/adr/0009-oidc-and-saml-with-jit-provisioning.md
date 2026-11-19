# 9. OIDC (PKCE + nonce) and SAML via crewjam/saml with JIT provisioning and group→role mapping

Date: 2026-11-19

## Status

Accepted

## Context

Enterprise customers authenticate against Keycloak, Microsoft Entra ID,
Okta or ADFS, using OpenID Connect or SAML 2.0. Accounts and roles should
follow the IdP's groups, so that joiners and leavers are handled centrally.
Local accounts remain useful for small installations and break-glass
access.

## Decision

* **OIDC** (`internal/auth/oidc.go`) uses `coreos/go-oidc/v3` and
  `golang.org/x/oauth2`: discovery from the issuer, authorization code flow
  with **PKCE (S256)**, random `state` and **nonce**, ID token verification
  (signature via JWKS, issuer, audience = client ID, expiry), nonce
  comparison, rejection of `email_verified: false`.
* **SAML** (`internal/auth/saml.go`) uses `crewjam/saml` as service
  provider: SP-initiated, AuthnRequest via HTTP-Redirect, response via
  HTTP-POST to `/acs`; IdP metadata from URL or file; optional SP key pair
  for metadata and encrypted assertions. `ParseResponse` is called with
  the AuthnRequest ID from the state cookie as the only allowed
  `InResponseTo`, so IdP-initiated responses are rejected.
* Between redirect and callback the flow state (OIDC state, nonce, PKCE
  verifier; SAML request ID) is kept in an **HMAC-signed cookie** with a
  10-minute expiry, which is cleared when the callback consumes it. The
  SAML state cookie is `SameSite=None; Secure` because the ACS POST is
  cross-site.
* **Just-in-time provisioning** (`provision` in `internal/api/authn.go`):
  users are found by (provider, subject), then by e-mail (linking a local
  account to the IdP and clearing its password), and created only with
  `auto_provision: true`. With `default_tenant` set, a membership is
  created with the highest-ranked role from `role_mapping` matching the
  user's groups, else `default_role` (falling back to `viewer`). When
  `role_mapping` is configured, it is authoritative and overwrites the
  existing role at every login.
* Local login (`auth.local`) can be disabled.

## Consequences

* Standards-based SSO with the major IdPs (examples in
  [operations.md](../operations.md)).
* There is **no SAML assertion replay cache**; replay protection relies on
  the single-use signed state cookie binding the response to this browser's
  AuthnRequest, and on crewjam/saml's validity window checks. A captured
  response could be replayed only together with the matching state cookie,
  within the assertion's validity.
* The state-cookie signing key is generated per process: in multi-node
  deployments the login redirect and callback must reach the same node
  (session affinity for `/api/v1/auth/`), and a restart aborts in-flight
  logins.
* Linking by e-mail trusts the IdP's e-mail claim; an account already bound
  to a different IdP is rejected.
* Group names in `role_mapping` are matched exactly (e.g. Entra group
  object IDs).
