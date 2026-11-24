# Runtime system settings

Super administrators can change a defined set of settings in the web client, under **Administration → System settings** (API `GET`/`PUT /api/v1/admin/settings`), without editing the file or restarting the server. The changes are stored in the database (table `system_settings`) and apply to the whole installation; no setting is per organisation. What the page looks like is described in the [user guide](../../user-guide/en/administration.md#system-settings); this chapter is about the rules.

Only super administrators **signed in with a session** can read or change the settings; API tokens (even a super administrator's) and organisation admins get `403`.

## Precedence and locking

**Precedence**, from strongest to weakest:

1. a `QTF_*` **environment variable** – it always wins and locks the setting on the page;
2. a value **changed in the web client** (stored in the database) – unless `system_settings.locked` locks the key, then the file wins;
3. the **configuration file**;
4. the built-in **default**.

The page shows the source of every value (*Default*, *Configuration file*, *Environment*, *Changed here*). A stored value that is shadowed – the key was later set by an environment variable or locked in the file – is kept but marked *ignored*, and the value in use is shown. **Reset to file value** deletes the stored value. The order is the same as before this page existed; operators who prefer the file to win lock the keys.

**The operator always wins**, through three kinds of lock:

| Lock | How | Effect |
|---|---|---|
| Environment | the setting is set by a `QTF_*` variable | read-only on the page, with the variable's name; a stored value is ignored |
| Configuration | `system_settings.locked` in the file lists it | read-only; a stored value is ignored |
| File only | the setting is part of the process set-up | shown for information, never changeable at runtime |

`system_settings.locked` takes keys, prefixes ending in `.*`, or `*` for everything:

```yaml
system_settings:
  locked: ["auth.oidc", "email.*", "runner.*"]
```

Use environment variables or locks for what must not change from the web client – in containers, the compose file is a natural place. Operators who do not want secrets in the database lock `email.*` and `auth.oidc` and keep using `*_file` options.

## When a change applies

| Section | Applies |
|---|---|
| General, Sign-in and passwords, API tokens, Single sign-on, Logging and syslog, E-mail (SMTP), Outgoing connections, Similarity, AI assistance | **at once** – the e-mail sender, the OIDC providers (unchanged providers are kept, so sign-ins in progress continue), the logger, the sign-in limits and the outgoing HTTP client follow the new values |
| Live updates | the switch and the heartbeat **at once** (new connections); the connection limits, the replay buffer and the presence timeout **after a restart** |
| Server and database | the HTTP read and write timeouts **after a restart**; everything else is set in the file and shown with its source (listen address, TLS files, database, first start, hardening, key provider) |
| Metrics and tracing | **at once** – the switches, the token, the allowlist and the trace export; the separate metrics address (`metrics.listen`) **after a restart**. The section also shows the state: where `/metrics` is served, how it is protected, and the spans exported, failed and dropped with the last export and the last error |
| Script and web test runner | **after a restart** – the runner and its sandbox are set up before the process is locked down (Landlock execute paths, work directory); the value is stored at once and the page shows the value in use until then |
| Retention and backups, Authorisation, Multi-factor authentication | nothing to change here: they explain and link to the pages that manage them (the trash keeps deleted items for 30 days, which is fixed) |

Settings marked **Applies after restart** keep their value in use until the next start. A banner at the top of the page lists every saved change that waits for a restart, with a link to the setting.

`auth.session_ttl` applies to sessions created after the change. New values are validated together with the rest of the configuration – the same checks as `check-config` – before they are stored. If the stored settings are invalid at start-up (for example after a change of the file), the server logs `stored system settings are invalid and were ignored`, uses the file, and the page shows a warning; fix or reset the values there.

## All settings

The settings the page offers, generated from `internal/settings`:

<!-- BEGIN GENERATED: system-settings (make admin-guide-ref) -->

| Section | Key | Values | Applies |
|---|---|---|---|
| General | `server.base_url` | text | at once |
| General | `files.max_upload_mb` | 1–1024 | at once |
| General | `requirements.auto_create_test` | on/off | at once |
| Server and database | `server.listen` | text | shown only (file) |
| Server and database | `server.tls_cert` | text | shown only (file) |
| Server and database | `server.tls_key` | text | shown only (file) |
| Server and database | `server.read_timeout` | 1s–2h | after a restart |
| Server and database | `server.write_timeout` | 1s–2h | after a restart |
| Server and database | `server.frame_ancestors` | list | at once |
| Server and database | `server.trusted_proxies` | list | shown only (file) |
| Server and database | `server.hsts` | on/off | shown only (file) |
| Server and database | `database.driver` | text | shown only (file) |
| Server and database | `database.dsn` | secret (write-only) | shown only (file) |
| Server and database | `database.max_open_conns` | number | shown only (file) |
| Server and database | `bootstrap.tenant_slug` | text | shown only (file) |
| Server and database | `bootstrap.tenant_name` | text | shown only (file) |
| Server and database | `bootstrap.admin_email` | text | shown only (file) |
| Server and database | `hardening.no_new_privs` | on/off | shown only (file) |
| Server and database | `hardening.landlock` | on/off | shown only (file) |
| Server and database | `hardening.seccomp` | on/off | shown only (file) |
| Server and database | `hardening.drop_capabilities` | on/off | shown only (file) |
| Server and database | `hardening.read_only_paths` | list | shown only (file) |
| Server and database | `hardening.read_write_paths` | list | shown only (file) |
| Server and database | `secrets.provider` | text | shown only (file) |
| Sign-in and passwords | `auth.local` | on/off | at once |
| Sign-in and passwords | `auth.session_ttl` | 5m–2160h | at once |
| Sign-in and passwords | `auth.session_idle_timeout` | 0s–720h | at once |
| Sign-in and passwords | `auth.password_min_length` | 8–72 | at once |
| Sign-in and passwords | `auth.password_min_classes` | 0–4 | at once |
| Sign-in and passwords | `auth.password_history` | 0–24 | at once |
| Sign-in and passwords | `auth.password_max_age_days` | 0–3650 | at once |
| Sign-in and passwords | `auth.login_max_attempts` | 1–1000 | at once |
| Sign-in and passwords | `auth.login_lockout` | 1m–24h | at once |
| Sign-in and passwords | `auth.cookie_secure` | on/off | shown only (file) |
| API tokens | `auth.token_max_days` | 0–3650 | at once |
| API tokens | `auth.token_require_expiry` | on/off | at once |
| Single sign-on | `auth.oidc` | list of providers | at once |
| Logging and syslog | `logging.level` | `debug`, `info`, `warn`, `error` | at once |
| Logging and syslog | `logging.format` | `text`, `json` | at once |
| Logging and syslog | `logging.syslog.enabled` | on/off | at once |
| Logging and syslog | `logging.syslog.network` | `udp`, `tcp`, `tls`, `unix`, `unixgram` | at once |
| Logging and syslog | `logging.syslog.address` | text | at once |
| Logging and syslog | `logging.syslog.format` | `rfc5424`, `rfc3164` | at once |
| Logging and syslog | `logging.syslog.facility` | `kern`, `user`, `mail`, `daemon`, `auth`, `syslog`, `lpr`, `news`, `uucp`, `cron`, `authpriv`, `ftp`, `local0`, `local1`, `local2`, `local3`, `local4`, `local5`, `local6`, `local7` | at once |
| Logging and syslog | `logging.syslog.tag` | text | at once |
| Logging and syslog | `logging.syslog.ca_file` | text | shown only (file) |
| Logging and syslog | `logging.syslog.cert_file` | text | shown only (file) |
| Logging and syslog | `logging.syslog.key_file` | text | shown only (file) |
| Metrics and tracing | `metrics.enabled` | on/off | at once |
| Metrics and tracing | `metrics.listen` | text | after a restart |
| Metrics and tracing | `metrics.token` | secret (write-only) | at once |
| Metrics and tracing | `metrics.allowed_ips` | list | at once |
| Metrics and tracing | `tracing.enabled` | on/off | at once |
| Metrics and tracing | `tracing.endpoint` | text | at once |
| Metrics and tracing | `tracing.headers` | secret (write-only) | at once |
| Metrics and tracing | `tracing.sample_ratio` | text | at once |
| Metrics and tracing | `tracing.service_name` | text | at once |
| E-mail (SMTP) | `email.enabled` | on/off | at once |
| E-mail (SMTP) | `email.host` | text | at once |
| E-mail (SMTP) | `email.port` | 1–65535 | at once |
| E-mail (SMTP) | `email.tls` | `starttls`, `tls`, `none` | at once |
| E-mail (SMTP) | `email.username` | text | at once |
| E-mail (SMTP) | `email.password` | secret (write-only) | at once |
| E-mail (SMTP) | `email.from` | text | at once |
| E-mail (SMTP) | `email.from_name` | text | at once |
| E-mail (SMTP) | `email.reply_to` | text | at once |
| E-mail (SMTP) | `email.timeout` | 1s–10m | at once |
| E-mail (SMTP) | `email.digest_hour` | 0–23 | at once |
| Outgoing connections | `outbound.proxy` | text | at once |
| Outgoing connections | `outbound.no_proxy` | list | at once |
| Outgoing connections | `outbound.allowed_hosts` | list | at once |
| Outgoing connections | `outbound.denied_hosts` | list | at once |
| Outgoing connections | `outbound.block_private` | on/off | at once |
| Script and web test runner | `runner.enabled` | on/off | after a restart |
| Script and web test runner | `runner.timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.max_output_bytes` | 1024–67108864 | after a restart |
| Script and web test runner | `runner.deny_network` | on/off | after a restart |
| Script and web test runner | `runner.sandbox` | on/off | shown only (file) |
| Script and web test runner | `runner.work_dir` | text | shown only (file) |
| Script and web test runner | `runner.interpreters` | list | shown only (file) |
| Script and web test runner | `runner.web.enabled` | on/off | after a restart |
| Script and web test runner | `runner.web.browser` | `chromium`, `firefox`, `webkit` | after a restart |
| Script and web test runner | `runner.web.channel` | text | after a restart |
| Script and web test runner | `runner.web.browser_channels` | on/off | after a restart |
| Script and web test runner | `runner.web.headless` | on/off | after a restart |
| Script and web test runner | `runner.web.timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.web.job_timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.web.max_parallel` | 1–32 | after a restart |
| Script and web test runner | `runner.web.max_evidence_mb` | 0–1024 | after a restart |
| Script and web test runner | `runner.web.screenshots` | `on_failure`, `always`, `off` | after a restart |
| Script and web test runner | `runner.web.trace` | `on_failure`, `always`, `off` | after a restart |
| Script and web test runner | `runner.web.video` | `off`, `on_failure`, `always` | after a restart |
| Script and web test runner | `runner.web.node` | text | shown only (file) |
| Script and web test runner | `runner.web.playwright_dir` | text | shown only (file) |
| Script and web test runner | `runner.web.command` | text | shown only (file) |
| Script and web test runner | `runner.web.browsers_path` | text | shown only (file) |
| Script and web test runner | `runner.web.executable_path` | text | shown only (file) |
| Live updates | `live.enabled` | on/off | at once |
| Live updates | `live.heartbeat` | 1s–5m | at once |
| Live updates | `live.presence_ttl` | 5s–1h | after a restart |
| Live updates | `live.max_connections` | 1–1000000 | after a restart |
| Live updates | `live.max_connections_per_user` | 1–1000 | after a restart |
| Live updates | `live.replay_buffer` | 0–100000 | after a restart |
| Similarity (duplicates) | `similarity.embeddings.enabled` | on/off | at once |
| Similarity (duplicates) | `similarity.embeddings.url` | text | at once |
| Similarity (duplicates) | `similarity.embeddings.model` | text | at once |
| Similarity (duplicates) | `similarity.embeddings.api_key` | secret (write-only) | at once |
| Similarity (duplicates) | `similarity.embeddings.timeout` | 1s–5m | at once |
| AI assistance | `ai.enabled` | on/off | at once |
| AI assistance | `ai.provider` | `anthropic`, `openai` | at once |
| AI assistance | `ai.url` | text | at once |
| AI assistance | `ai.model` | text | at once |
| AI assistance | `ai.api_key` | secret (write-only) | at once |
| AI assistance | `ai.timeout` | 5s–10m | at once |
| AI assistance | `ai.max_tokens` | 256–128000 | at once |
| AI assistance | `ai.temperature` | text | at once |
| AI assistance | `ai.rate_limit` | 1–10000 | at once |
| AI assistance | `ai.features.draft_tests` | on/off | at once |
| AI assistance | `ai.features.improve_requirement` | on/off | at once |
| AI assistance | `ai.features.summarize_run` | on/off | at once |
| AI assistance | `ai.features.suggest_mappings` | on/off | at once |
| AI assistance | `ai.features.suggest_criteria` | on/off | at once |
| AI assistance | `ai.price_input` | text | at once |
| AI assistance | `ai.price_output` | text | at once |

<!-- END GENERATED: system-settings -->

What each key means is in the [configuration reference](configuration.md#reference). Every key of the configuration file is either in this table – editable, or *shown only (file)* with its value and source – or file-only on purpose: the `*_file` options of secrets, the bootstrap password, the SAML providers (they need key and certificate files; the page lists them read-only with a *Test metadata* button), `system_settings.locked` itself and the details of the key provider of stored secrets (shown in the encryption panel of *Server and database*). A test (`TestSettingsInventory`) keeps it that way when a key is added.

## Using the page

- The side navigation groups the sections into *Server*, *Access* and *Services*. **Search** finds a setting in any section by its name, help text, key, value or environment variable, and jumps to it.
- Each section is saved on its own. **Save section** first shows the changes – old and new value, secrets only as *a new value is stored* – and whether they apply at once or after a restart.
- **Export settings (JSON)** downloads the values changed here (not the file's, never secrets, without the OIDC provider list) as `qtestframe-settings.json`; **Import settings** reads such a file on another server, skips unknown, locked and secret keys, and shows the changes before they are saved. **Export configuration** still downloads the whole effective configuration as YAML.

## Notes on single sign-on, syslog and e-mail

- **OIDC providers.** Changing the providers in the web client stores the whole list, which then **replaces** `auth.oidc` of the file. Client secrets of providers that come from the file are not copied into the database: they keep coming from `client_secret_file` or `QTF_OIDC_<NAME>_CLIENT_SECRET`, which always wins. **Reset to file value** returns to the file's list.
- **SAML** providers can only be configured in the file; the page lists them (entity ID, metadata, ACS URL, attributes, role mapping) and *Test metadata* reads the identity provider's metadata.
- **Outgoing connections.** `outbound.*` applies to every outgoing HTTP call: webhooks, chat channels, defect trackers and requirement sources, AI assistance, the embeddings service, OIDC discovery and environment health checks (not SMTP or syslog). `allowed_hosts` and `denied_hosts` take exact names, addresses or `*.example.com`; `block_private` refuses loopback, private, link-local and other non-public addresses, checked on the address actually connected to (so a DNS name that later resolves to an internal address is still refused), unless the host is allowed explicitly. *Check URL* tests an address against the rules, unsaved changes included. With a proxy, the addresses are checked on the server before the call. A proxy password can only come from the file or `HTTPS_PROXY`; the page and the export show it as `REDACTED`. There is no separate rate limit for webhooks or chat beyond their own retry rules.
- **Syslog over TLS** uses `ca_file`, `cert_file` and `key_file` from the file; they are readable after the lock-down because they are added to the Landlock read-only paths at start-up. A Unix socket that was not configured at start-up may be refused by Landlock; use `udp`, `tcp` or `tls`, or restart.
- **Secrets** entered on the page (the SMTP password, OIDC client secrets) are write-only, stored [encrypted](security.md#encryption-of-stored-secrets), never logged and never shown again.
- **Test buttons** (*Test connection* for OIDC discovery, *Test metadata* for SAML, *Send test message* for syslog, *Send test e-mail*, *Test the service* for embeddings and AI, *Check URL* for outgoing connections) make the server connect to an address the super administrator enters. They follow the outgoing-connection rules where they use HTTP; filter the server's egress if it can reach internal services.
- **Not on the page:** a backup schedule (backups are made on demand or from a systemd timer with `qtestframe backup`), metrics endpoints (there is none; `/healthz` and `/readyz` are public and carry no data), CI and tracker defaults (they are per project), and per-organisation settings, which are on the organisation's own pages (AI assistance, Electronic records, Multi-factor authentication) or in its policy (Administration → Organisations).

## Embeddings service for similar items

Suspected duplicate requirements and tests, the *Similar requirements* panel of the editors and the catalogue import warning compare items on the server by default: TF-IDF over the project with stemming, stop words and synonyms for English and Swedish (see the user guide, *Duplicates and conflicts*). Nothing is sent anywhere.

Optionally, an **OpenAI-compatible embeddings endpoint** computes the similarity instead: **Administration → System settings → Similarity (duplicates)**, or `similarity.embeddings` in the [configuration file](configuration.md#reference).

| Setting | Meaning |
|---|---|
| `similarity.embeddings.enabled` | off by default |
| `similarity.embeddings.url` | the endpoint, e.g. `https://llm.example.com/v1/embeddings`; it receives `POST {"model": …, "input": [texts]}` and answers `{"data": [{"index": i, "embedding": [numbers]}]}` |
| `similarity.embeddings.model` | sent as `model` (empty: the service's default) |
| `similarity.embeddings.api_key` | sent as `Authorization: Bearer …`; write-only, stored [encrypted](security.md#encryption-of-stored-secrets); in the file only as `api_key_file` |
| `similarity.embeddings.timeout` | longest wait (default 30 s) |

**Privacy.** When it is on, the **title, description and acceptance criteria of requirements and the title, description, preconditions and steps of tests are sent to that service**, of every project of every organisation on the server, whenever the similarity checks run or someone types in an editor (the text being written is sent too). Use a service run by your organisation or one you have an agreement with; leave it off otherwise.

The vectors are stored in the table `similarity_embeddings` per item, model and text (a changed item is sent again, an unchanged one is not). It is a cache: it is not part of backups or project archives and is filled again after a restore. Changing the model computes every vector again. **Test the service** on the page sends one short example sentence and reports the vector size. When the service fails or times out, the server logs a warning and uses the local similarity, so the checks never stop working. The conflict detection (opposite obligations, thresholds, exclusive values) always runs on the server. The thresholds of the checks apply to the service's cosine similarity as well; services differ, so check the suspected duplicates after turning it on and adjust the threshold of the duplicate checks if needed.

## AI assistance

Qtestframe can ask a language model service for **suggestions** that users review before anything is saved: drafts of tests for a requirement, an improved wording of a requirement, a summary of a failed run with defect drafts, framework mappings (ATT&CK, D3FEND, EMB3D, CWE) and acceptance criteria (see the user guide, *AI assistance*). It is **off by default** and needs three switches:

1. **Administration → System settings → AI assistance** (or `ai` in the [configuration file](configuration.md#reference)): the service and `ai.enabled`.
2. **Administration → AI assistance**: the switch of the organisation (`settings.manage`).
3. Each project can opt out: **Settings → General → AI assistance** (`project.update`).

| Setting | Meaning |
|---|---|
| `ai.provider` | `anthropic`: the Anthropic Messages API (`POST {url}/v1/messages`, key as `x-api-key`). `openai`: an OpenAI-compatible chat completions endpoint (`POST {url}/chat/completions`, key as `Authorization: Bearer`; an Azure OpenAI deployment URL with `?api-version=…` sends it as `api-key`): OpenAI, Azure OpenAI, or a local server such as Ollama or vLLM |
| `ai.url` | base URL; empty with `anthropic` means `https://api.anthropic.com`; required with `openai`, e.g. `https://llm.example.com/v1` |
| `ai.model` | the model id (required; there is no built-in default): the model of your Anthropic account, or your model or deployment for `openai` |
| `ai.api_key` | write-only, stored [encrypted](security.md#encryption-of-stored-secrets); in the file only as `api_key_file`; may stay empty for a local server |
| `ai.timeout`, `ai.max_tokens`, `ai.temperature` | the longest wait for an answer (default 2 min), the most tokens of an answer (default 8192), the temperature (empty: not sent) |
| `ai.rate_limit` | AI requests per user and hour (default 30) |
| `ai.features.*` | turn single features off |
| `ai.price_input`, `ai.price_output` | optional prices per million tokens: each answer then shows an estimated cost |

**Test the service** on the page sends a fixed short sentence (no item data) and reports the model and the tokens used.

**Data flow and privacy.** Nothing is sent until a user clicks an *Assist* action. The server then sends **only the items of that action** to the service, from the server's network:

| Action | Sent to the service |
|---|---|
| Draft tests, Suggest acceptance criteria | key, title, description, type, obligation, rationale and acceptance criteria of the requirement (the editor's current text) |
| Improve requirement | the same, plus the findings of the requirement checks |
| Summarise failures | name, status, environment and build of the run; key, title, notes, error output (the last 3000 characters) and failed steps of up to 40 failed or blocked executions; key, title, status and severity of the defects linked to the run |
| Suggest mappings | key, title, description (risks: category and mitigation) and the ids already mapped |

Users see a notice that the text is sent before they start. Use a service run by your organisation or one you have an agreement with that covers this data (retention, training, location). A local server (Ollama, vLLM) keeps everything on your network.

**Safety.** The service gets no tools and nothing it answers is executed. The item text is sent as JSON between `<item_data>` tags and the instructions tell the model to treat it as data (JSON escapes `<` and `>`, so an item cannot close the tag). Answers must be one JSON object of a fixed schema; the server checks it strictly, asks once more when it does not match, and drops what it cannot verify (framework ids that are not in the embedded lists, criteria that already exist, tests outside the run). The rewrite of a requirement is checked again with the requirement checks on the server, which reports the findings it really fixes. No service call runs inside a database transaction, and a user can cancel a request at any time.

**Audit.** Every request is written to the audit log as `ai.request` with the feature, the user, the item ids, the provider and model, the prompt version, the tokens, the duration and the outcome (`ok`, `cancelled`, `ai_refused`, `ai_invalid_answer`, `ai_service`, `ai_timeout`) — never the content. The prompts are embedded in the binary and versioned (`prompt_version` in each answer and audit entry). Changes of the organisation switch are audited as `settings.update`, the project opt-out as `project.ai_settings`, the test button as `settings.test_ai`.

## Audit and export

- Every change is written to the audit log as `settings.update` with old → new value (secrets only as *set* / *changed* / *removed*), and to the server log – an import too, as one change. The test buttons are audited as `settings.test_*`.
- **Export configuration** (`GET /api/v1/admin/settings/export`) downloads the effective configuration as YAML with secrets replaced by `REDACTED` – for example to move the values into the file and then reset them on the page.
- [Backups](backup-and-migration.md) contain the `system_settings` table with its secrets, still encrypted; a restore applies it at once. Project archives do not contain it.
