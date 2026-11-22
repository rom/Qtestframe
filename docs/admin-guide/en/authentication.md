# Authentication

Qtestframe knows these ways to prove who someone is:

| Method | For | Configured in |
|---|---|---|
| Local password | people | `auth.local`, password policy |
| Second factor (TOTP authenticator app, recovery codes) | people with a local password, optionally after OIDC | MFA policy in the web client; `local_mfa` per OIDC provider |
| OpenID Connect | people | `auth.oidc` (file or system settings) |
| SAML 2.0 | people | `auth.saml` (file only) |
| E-mail invitation | a person's first sign-in | needs e-mail |
| API token (`qtf_…`) | scripts and CI | **Administration → API tokens** |
| SCIM token (`qtfscim_…`) | an identity provider's provisioning | **Administration → Provisioning (SCIM)** |

## Local passwords

- Passwords are hashed with **bcrypt** (cost 12). The policy is: at least `auth.password_min_length` characters (default 10, at most 72 bytes, because bcrypt ignores the rest), with at least one letter and one character that is not a letter. `qtestframe user create` and the bootstrap administrator always check against 10.
- Unknown users, wrong passwords, disabled accounts and accounts without a password all get the same answer (`401 invalid credentials`), and take the same time.
- **Sign-in limit:** at most `auth.login_max_attempts` attempts (default 10) per *client address + e-mail address* within `auth.login_lockout` (default 5 minutes); more get `429 rate_limited`. A successful sign-in clears the counter. The counter is in memory, per server process. Behind a reverse proxy set `server.trusted_proxies`, or every user shares the proxy's address.
- Failed sign-ins are logged at WARN (`login failed`, with e-mail and client address) and recorded in the user's sign-in history, which the web client shows as "failed attempts since your last sign-in".
- **Disable local sign-in** with `auth.local: false` once single sign-on works. In the web client this is only possible while a single sign-on provider is enabled. Super administrators can still be managed on the command line.
- Organisation admins reset the passwords of members who belong only to their organisation (**Administration → Members → Reset password**: a temporary password, shown once, that must be changed at the next sign-in); super administrators reset anyone's under **Users**. Both sign the user out everywhere.

## Multi-factor authentication

The second factor is a **TOTP** code from an authenticator app (RFC 6238: HMAC-SHA-1, 6 digits, 30-second steps, ±1 step of clock drift), with **10 single-use recovery codes**. Users set it up under their profile. Each code works only once (the last used step is stored), so at most one sign-in per 30 seconds per user.

**Policies** decide who must use it (**Administration → Multi-factor authentication**, see the [user guide](../../user-guide/en/administration.md#multi-factor-authentication)):

| Policy | Set by | Applies to |
|---|---|---|
| Server policy | super administrators | every user; roles count in any organisation, plus the pseudo-role *super admin* |
| Organisation policy | organisation admins (`tenant.manage`) | the members of that organisation by their role there |

Each is *optional* (default), *required for everyone* or *required for some roles*. An organisation can require more than the server, never less. A **grace period** (days) starts when a policy begins to require MFA for more people, or at the user's creation if that is later; afterwards a user without a second factor must set one up before anything else works (`403 mfa_enrolment_required`). The policy is evaluated at sign-in, so a stricter policy reaches existing sessions at their next sign-in.

**Remember this device** (server policy, *days*, default 0 = off) lets a browser skip the code – never the password – for that many days. The token is revocable on the server; users can forget their devices, and an administrator reset or turning MFA off forgets them too.

Guessing is limited three ways: every code counts against the sign-in limit above (which the correct password alone does not reset), an account allows 10 code attempts per 15 minutes from any address, and a pending sign-in is dropped after 5 wrong codes.

**Lost phone and recovery codes:** an organisation admin uses **Reset MFA** in **Members** (for members of only that organisation), a super administrator under **Users**. It removes the authenticator, recovery codes and remembered devices, signs the user out and tells them by notification and e-mail. Verify the request out of band first.

The TOTP secrets are stored encrypted (see [Security](security.md#encryption-of-stored-secrets)). API tokens and SCIM tokens are not affected by MFA; protect them like passwords. WebAuthn and passkeys are not implemented.

## OpenID Connect

Register a **confidential web client** at the provider with:

- redirect URI `<server.base_url>/api/v1/auth/oidc/<name>/callback`,
- grant type *authorization code* (Qtestframe always uses PKCE with S256),
- scopes `openid email profile`, plus `groups` if the provider offers it.

Then add the provider:

```yaml
auth:
  oidc:
    - name: corp
      display_name: "Corporate login"
      issuer: "https://sso.example.com/realms/acme"
      client_id: qtestframe
      client_secret_file: oidc-corp            # or QTF_OIDC_CORP_CLIENT_SECRET
      scopes: [openid, email, profile]
      groups_claim: groups
      auto_provision: true
      default_tenant: acme
      default_role: viewer
      role_mapping: {"qtf-admins": admin, "qtf-testers": tester}
```

or add it in **Administration → System settings → Single sign-on (OIDC)**, which can test the discovery document. Every key is in the [reference](configuration.md#authoidc). Provider-specific steps for Keycloak, Microsoft Entra ID and Okta are in [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#6-single-sign-on-oidc).

What Qtestframe checks: state, nonce and the PKCE verifier are kept in a signed, single-use cookie that is valid for 10 minutes; the ID token signature is verified against the provider's keys, and issuer, audience and expiry are checked. The token must carry an e-mail address (`email_claim`); `email_verified: false` is refused. After the sign-in the browser always goes to `/`.

The server needs outgoing HTTPS to the issuer. With SELinux: `setsebool -P qtestframe_connect_http on`. Name providers with lower-case letters, digits and `_`: the name is part of the URL and, upper-cased, of `QTF_OIDC_<NAME>_CLIENT_SECRET` (a `-` would make that variable hard to set).

### Local MFA and amr/acr

By default the identity provider is responsible for MFA, and Qtestframe's own second factor is not asked for after an OIDC sign-in (`local_mfa: off`). Two stricter settings:

| `local_mfa` | Qtestframe asks for its own code |
|---|---|
| `off` (default) | never |
| `unless_idp_mfa` | unless the ID token shows MFA: its `amr` claim contains `mfa` or a value of `mfa_amr_values` (e.g. `[mfa, otp, hwk]`), or its `acr` is one of `mfa_acr_values` |
| `always` | on every sign-in through this provider |

When the code is asked for, users need an authenticator app in Qtestframe too (or must set one up if the MFA policy requires it). The reported `amr`/`acr` values are written to the audit entry of the sign-in. SAML sign-ins never use the local second factor; configure MFA at the identity provider.

## SAML 2.0

Create a key pair for Qtestframe as service provider and point the IdP at the SP metadata:

```sh
sudo scripts/certs/gen-saml-sp-cert.sh --cn qtf.example.com --name saml-adfs --out /etc/qtestframe/tls --group qtestframe
```

```yaml
auth:
  saml:
    - name: adfs
      display_name: "Corporate login (ADFS)"
      idp_metadata_url: "https://adfs.example.com/FederationMetadata/2007-06/FederationMetadata.xml"
      sp_cert_file: /etc/qtestframe/tls/saml-adfs.crt
      sp_key_file: /etc/qtestframe/tls/saml-adfs.key
      email_attr: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"
      groups_attr: "http://schemas.xmlsoap.org/claims/Group"
      auto_provision: true
      default_tenant: acme
      role_mapping: {"QTF-Admins": admin}
```

| URL | |
|---|---|
| `<base_url>/api/v1/auth/saml/<name>/metadata` | SP metadata (also the default entity ID) |
| `<base_url>/api/v1/auth/saml/<name>/acs` | assertion consumer service (HTTP-POST) |

Only **SP-initiated** sign-in is supported; IdP-initiated (unsolicited) responses are refused. The response or every assertion must be signed by the IdP certificate from the metadata; destination, audience, recipient and the validity window are checked. AuthnRequests are not signed, so turn off *client signature required* at the IdP. Encrypted assertions are decrypted with `sp_key_file`. SAML providers are configured only in the file and read `server.base_url` at start-up. The metadata URL is fetched at the first sign-in; `idp_metadata_file` needs no outgoing connection.

## Provisioning of single sign-on users

At every OIDC or SAML sign-in:

1. The user is found by provider and subject. Otherwise by e-mail: a local account (or one created by SCIM) with that address is **linked** to the provider and its password removed; an account bound to *another* provider is refused. Otherwise a new user is created only with `auto_provision: true`.
2. Disabled users are refused.
3. With `default_tenant`, the role is the most privileged role that `role_mapping` gives one of the user's groups, else `default_role` (`viewer` when empty or unknown). A missing membership is created with `auto_provision`; with a `role_mapping`, an existing membership is set to the mapped role at every sign-in – the identity provider is authoritative.

Because accounts are linked by e-mail address, connect only identity providers whose addresses are verified and belong to your organisation. Members that SCIM manages are left to SCIM.

## SCIM provisioning

With SCIM 2.0 an identity provider (Microsoft Entra ID, Okta and others) creates, updates and deactivates the members of **one organisation**. The organisation admin creates a SCIM token under **Administration → Provisioning (SCIM)** and gives the provider the base URL `<base_url>/scim/v2` and the token; the [user guide](../../user-guide/en/administration.md#provisioning-with-scim) describes the provider screens.

What the server administrator should know:

- A SCIM token (`qtfscim_` + 256 random bits) is stored as a hash, shown once, may expire, and authenticates only `/scim/v2` of its organisation. It controls that organisation's membership up to the admin role (through a group mapped to admin), so treat it like an admin password.
- It cannot change super administrator status, passwords, project data or other organisations; an organisation always keeps one admin.
- Provisioned accounts have no password: they sign in through OIDC or SAML, which links them by e-mail.
- Deactivating a user removes the membership and ends the user's sessions and API tokens in that organisation.
- Limits: 1200 requests per minute per token; 20 failed authentications per 5 minutes block the client address (`429`). Every change is audited with the actor `SCIM: <token name>`.

## API tokens

API tokens are for scripts and CI pipelines (`Authorization: Bearer qtf_…`). Members with `token.manage` (Tester and up by default) create them under **Administration → API tokens**.

- A token is `qtf_` + 32 random bytes; only its SHA-256 hash and a short prefix are stored, and it is shown once.
- It belongs to **one organisation** and has a role that cannot exceed its creator's. On every request its permissions are **capped by the creator's current role**: demoting the user demotes the token, removing the member ends it.
- Tokens cannot create tokens, never carry super administrator rights (not even a super administrator's), cannot use the system settings and are not subject to MFA.
- Optional expiry in days; the last use is recorded; revoking is immediate.
- Requests with a token skip the CSRF check (there is no cookie), so the token is the only credential: keep it in the CI system's secret store.

## Sessions

- A session is created at every sign-in. The browser holds only the random cookie `qtf_session` (`HttpOnly`, `SameSite=Lax`, `Secure` unless `auth.cookie_secure: false`); the database stores a SHA-256 hash of it.
- Sessions end after `auth.session_ttl` (default 12 h, absolute, no extension), at sign-out, and when an administrator disables the user, resets the password or resets MFA. Changing one's own password ends the user's other sessions. Expired sessions are purged hourly.
- Users see their sessions (browser, address, last activity) in their profile and can end each one.
- Requests from the browser that change something must send the per-session CSRF token and pass an origin check against the request host or `server.base_url`.
- Sessions are not part of a backup; a restore signs everyone out.
