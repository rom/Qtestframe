# Runtime system settings

Super administrators can change a defined set of settings in the web client, under **Administration → System settings** (API `GET`/`PUT /api/v1/admin/settings`), without editing the file or restarting the server. The changes are stored in the database (table `system_settings`) and apply to the whole installation; no setting is per organisation. What the page looks like is described in the [user guide](../../user-guide/en/administration.md#system-settings); this chapter is about the rules.

Only super administrators **signed in with a session** can read or change the settings; API tokens (even a super administrator's) and organisation admins get `403`.

## Precedence and locking

**Precedence:** a value changed in the web client wins over the `QTF_*` environment variable and the configuration file, which win over the built-in default. The page shows the source of every value (*Default*, *Configuration file*, *Environment*, *Changed here*), and **Reset to file value** deletes the stored value.

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
| General, Security, Single sign-on (OIDC), Logging and syslog, E-mail (SMTP) | **at once** – the e-mail sender, the OIDC providers (unchanged providers are kept, so sign-ins in progress continue), the logger and the sign-in limits are rebuilt |
| Script and web test runner | **after a restart** – the runner and its sandbox are set up before the process is locked down (Landlock execute paths, work directory); the value is stored at once and the page shows the value in use until then |
| Retention and backups | nothing to change: the trash keeps deleted items for 30 days, which is fixed |

`auth.session_ttl` applies to sessions created after the change. New values are validated together with the rest of the configuration – the same checks as `check-config` – before they are stored. If the stored settings are invalid at start-up (for example after a change of the file), the server logs `stored system settings are invalid and were ignored`, uses the file, and the page shows a warning; fix or reset the values there.

## All settings

The settings the page offers, generated from `internal/settings`:

<!-- BEGIN GENERATED: system-settings (make admin-guide-ref) -->

| Section | Key | Values | Applies |
|---|---|---|---|
| General | `server.base_url` | text | at once |
| General | `files.max_upload_mb` | 1–1024 | at once |
| General | `requirements.auto_create_test` | on/off | at once |
| General | `server.listen` | text | shown only (file) |
| General | `database.driver` | text | shown only (file) |
| Security | `auth.local` | on/off | at once |
| Security | `auth.session_ttl` | 5m–2160h | at once |
| Security | `auth.password_min_length` | 8–72 | at once |
| Security | `auth.login_max_attempts` | 1–1000 | at once |
| Security | `auth.login_lockout` | 1m–24h | at once |
| Security | `auth.cookie_secure` | on/off | shown only (file) |
| Security | `server.hsts` | on/off | shown only (file) |
| Security | `server.trusted_proxies` | list | shown only (file) |
| Single sign-on (OIDC) | `auth.oidc` | list of providers | at once |
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
| E-mail (SMTP) | `email.enabled` | on/off | at once |
| E-mail (SMTP) | `email.host` | text | at once |
| E-mail (SMTP) | `email.port` | 1–65535 | at once |
| E-mail (SMTP) | `email.tls` | `starttls`, `tls`, `none` | at once |
| E-mail (SMTP) | `email.username` | text | at once |
| E-mail (SMTP) | `email.password` | secret (write-only) | at once |
| E-mail (SMTP) | `email.from` | text | at once |
| E-mail (SMTP) | `email.from_name` | text | at once |
| E-mail (SMTP) | `email.timeout` | 1s–10m | at once |
| E-mail (SMTP) | `email.digest_hour` | 0–23 | at once |
| Script and web test runner | `runner.enabled` | on/off | after a restart |
| Script and web test runner | `runner.timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.max_output_bytes` | 1024–67108864 | after a restart |
| Script and web test runner | `runner.deny_network` | on/off | after a restart |
| Script and web test runner | `runner.sandbox` | on/off | shown only (file) |
| Script and web test runner | `runner.web.enabled` | on/off | after a restart |
| Script and web test runner | `runner.web.browser` | `chromium`, `firefox`, `webkit` | after a restart |
| Script and web test runner | `runner.web.headless` | on/off | after a restart |
| Script and web test runner | `runner.web.timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.web.job_timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.web.max_parallel` | 1–32 | after a restart |
| Script and web test runner | `runner.web.max_evidence_mb` | 0–1024 | after a restart |
| Script and web test runner | `runner.web.screenshots` | `on_failure`, `always`, `off` | after a restart |
| Script and web test runner | `runner.web.trace` | `on_failure`, `always`, `off` | after a restart |
| Script and web test runner | `runner.web.video` | `off`, `on_failure`, `always` | after a restart |
| Script and web test runner | `runner.web.playwright_dir` | text | shown only (file) |
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

What each key means is in the [configuration reference](configuration.md#reference). Everything else – TLS, the database, hardening, the key provider, SAML, `trusted_proxies`, `cookie_secure`, `hsts`, the paths of the runner – is set only in the file.

## Notes on single sign-on, syslog and e-mail

- **OIDC providers.** Changing the providers in the web client stores the whole list, which then **replaces** `auth.oidc` of the file. Client secrets of providers that come from the file are not copied into the database: they keep coming from `client_secret_file` or `QTF_OIDC_<NAME>_CLIENT_SECRET`, which always wins. **Reset to file value** returns to the file's list.
- **SAML** providers can only be configured in the file.
- **Syslog over TLS** uses `ca_file`, `cert_file` and `key_file` from the file; they are readable after the lock-down because they are added to the Landlock read-only paths at start-up. A Unix socket that was not configured at start-up may be refused by Landlock; use `udp`, `tcp` or `tls`, or restart.
- **Secrets** entered on the page (the SMTP password, OIDC client secrets) are write-only, stored [encrypted](security.md#encryption-of-stored-secrets), never logged and never shown again.
- **Test buttons** (*Test connection* for OIDC discovery, *Send test message* for syslog, *Send test e-mail*) make the server connect to an address the super administrator enters. Filter the server's egress if it can reach internal services.

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

- Every change is written to the audit log as `settings.update` with old → new value (secrets only as *set* / *changed* / *removed*), and to the server log. The test buttons are audited as `settings.test_*`.
- **Export configuration** (`GET /api/v1/admin/settings/export`) downloads the effective configuration as YAML with secrets replaced by `REDACTED` – for example to move the values into the file and then reset them on the page.
- [Backups](backup-and-migration.md) contain the `system_settings` table with its secrets, still encrypted; a restore applies it at once. Project archives do not contain it.
