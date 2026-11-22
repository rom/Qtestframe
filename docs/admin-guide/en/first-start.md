# First start

## What happens at start-up

`qtestframe serve` does, in this order:

1. load and validate the configuration (an invalid one stops it, with every error listed);
2. open the database and apply the pending [migrations](backup-and-migration.md#migrations);
3. open the key provider of the stored secrets, and encrypt secrets that an earlier version stored as plaintext (a missing or wrong key stops it here);
4. create the **bootstrap administrator** if the database has no users;
5. apply the system settings stored in the database;
6. open the listener and the TLS certificate, then **lock the process down** (no_new_privs, capabilities, Landlock, seccomp) and log one `hardening` line;
7. start the background jobs and log `qtestframe listening` with the address, TLS, version and database driver.

## The first administrator

There are two ways to get the first account.

**Bootstrap in the configuration.** While the database has **no users at all**, the server creates a user from the `bootstrap` section, makes it a **super administrator** and **admin** of the organisation `bootstrap.tenant_slug` (created if missing, default `default` / *Default*):

```yaml
bootstrap:
  tenant_slug: acme
  tenant_name: "Acme Corp"
  admin_email: admin@acme.example
  admin_password_file: /etc/qtestframe/secrets/admin-password
```

The password must have at least 10 characters with at least one letter and one non-letter; a weaker one stops the start. Once any user exists, the section is ignored, so it can stay in the file, but remove the password file once the administrator has signed in and changed the password. Without bootstrap settings the server logs `no users exist; configure bootstrap.admin_email/admin_password or run qtestframe user create`.

**On the command line**, as the service user:

```sh
alias qtf='sudo -u qtestframe env QTF_CONFIG=/etc/qtestframe/config.yaml /usr/bin/qtestframe'
read -rs PW && printf '%s\n' "$PW" | qtf user create --email admin@acme.example --name "Admin" --password-stdin --super-admin
qtf tenant create --slug acme --name "Acme Corp"
qtf member add --tenant acme --email admin@acme.example --role admin
```

The password is read from standard input, never from the command line (where `ps` would show it). A user created without `--password-stdin` has no password and can only sign in through single sign-on.

## Organisations

An **organisation** (tenant) is an isolated workspace: projects, members, roles, API tokens, SCIM tokens, MFA policy and audit log belong to one organisation and are invisible to the others. A user account belongs to the installation and can be a member of several organisations, with one role in each; the user switches between them in the web client.

- Create organisations under **Administration → Organisations** (super administrators) or with `qtestframe tenant create --slug S --name N`. The slug is 2–63 characters of `a-z`, `0-9` and `-`; users can type it at sign-in.
- Give people roles under **Administration → Members** (organisation admins) or with `qtestframe member add`.
- Deleting an organisation deletes all its projects and data and cannot be undone (only a [restore](backup-and-migration.md#restore) brings it back).

Single sign-on can put new users straight into an organisation (`default_tenant`, `role_mapping`, see [Authentication](authentication.md#provisioning-of-single-sign-on-users)), and SCIM provisions the members of one organisation per token.

## Demo data

```sh
qtf seed-demo                 # into bootstrap.tenant_slug
qtf seed-demo --tenant acme
```

`seed-demo` adds the sample project *Demo – Online Banking* (key `DEMO`, then `DEMO2`, … when it already exists) with requirements, tests, links, runs with history, a release, a demo web test, and a restricted project shared with one of two demo users, `dana@example.com` and `erik@example.com`, whose password is `Demo-user-1234`. It runs the bootstrap first, so the demo users never take the place of the first administrator. The demo web test opens the server's own sign-in page at `server.base_url`.

**Never run `seed-demo` on a production database**: it creates accounts with a published password. Use a separate database for trials and training.

## Checklist after the first start

- Sign in as the bootstrap administrator and **change the password** (user menu → **Profile & password**); set up an authenticator app.
- Set `server.base_url` to the address users type; it is needed for single sign-on and e-mail.
- Back up the **key file** of the stored secrets (`/var/lib/qtestframe/qtestframe-secrets.key` on a SQLite installation) to a place apart from the database backups. See [Security](security.md#encryption-of-stored-secrets).
- Decide on single sign-on, an [MFA policy](authentication.md#multi-factor-authentication), e-mail and syslog.
- Set up regular [backups](backup-and-migration.md) and try a restore.
