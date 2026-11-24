# Logging and monitoring

## The server log

The server writes structured log records (Go `log/slog`) to **standard error**; under systemd they go to the journal (`journalctl -u qtestframe -f`, `-o json` for the fields). Two formats:

| `logging.format` | Example |
|---|---|
| `text` (default) | `time=2026-11-22T09:14:03.120Z level=INFO msg="http request" method=GET path=/api/v1/projects status=200 bytes=5120 duration_ms=4 remote=10.0.0.7` |
| `json` | `{"time":"2026-11-22T09:14:03.120Z","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/projects","status":200,…}` |

`logging.level` is `debug`, `info` (default), `warn` or `error`. Level and format can be changed at runtime in **Administration → System settings → Logging and syslog**, unless `QTF_LOG_LEVEL`/`QTF_LOG_FORMAT` set them (the container image sets the format to `json`).

What is logged:

- **Every request** except `/healthz`, `/readyz` and `/metrics`: method, path (without the query), status, bytes, duration and client address (from `X-Forwarded-For` only for `trusted_proxies`). Requests answered with 5xx are logged at ERROR, the others at INFO.
- **Audit records** (`msg=audit`, INFO) with organisation, user, action, entity and id, one for every entry of the audit log.
- **Security events**: `login failed` and `second factor failed` (WARN, with e-mail and client address), configuration and key problems at start-up, the `hardening` line.
- **Background work**: migrations, purged sessions and trash, metric snapshots, e-mail and webhook failures, job resets.

When a request belongs to a trace (an incoming `traceparent` header, or tracing on), its records carry `trace_id`, and `span_id` for the span of this server, so a log line can be found from a trace and the other way round.

Passwords, tokens, cookies and `Authorization` headers are never logged. Unexpected errors are answered with only "internal server error"; the details are in the log line `request failed` (path and error).

## Central syslog

Every record – the audit records included – can also go to a syslog server, as one JSON message per record:

```yaml
logging:
  level: info
  format: json
  syslog:
    enabled: true
    network: tls                 # udp | tcp | tls | unix | unixgram
    address: "logs.example.com:6514"
    tag: qtestframe
    facility: local0
    format: rfc5424              # or rfc3164
    ca_file: /etc/qtestframe/tls/ca.crt
    cert_file: /etc/qtestframe/tls/syslog-client.crt   # optional, mutual TLS
    key_file: /etc/qtestframe/tls/syslog-client.key
```

- Stream transports (`tcp`, `tls`, `unix`) use RFC 6587 octet counting; `tls` follows RFC 5425 and verifies the server against `ca_file` (or the system roots) with TLS 1.2 or later. A client certificate is made with `scripts/certs/gen-client-cert.sh`.
- Severities: ERROR → 3, WARN → 4, INFO → 6, DEBUG → 7. The message's APP-NAME is `tag`.
- Delivery is best effort: one reconnect per message, a 5-second write deadline, no disk queue. The journal keeps the local copy.
- With SELinux: `setsebool -P qtestframe_remote_syslog on`.
- **Send test message** in the system settings sends one message with the values of the form.

Receiver examples for rsyslog (`imtcp` with `gtls`) and syslog-ng (`network(transport("tls") flags(syslog-protocol))`) are in [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#8-logging-and-central-syslog). JSON payloads can be parsed into fields with rsyslog `mmjsonparse` or syslog-ng `json-parser()`.

## Health endpoints

| Endpoint | Answers | Use |
|---|---|---|
| `GET /healthz` | `200 {"status":"ok","version":"…"}` while the process serves requests | liveness probe |
| `GET /readyz` | `200 {"status":"ready","checks":{"database":"ok","migrations":"ok"}}`, or `503` when the database does not answer within 2 seconds or migrations are not applied (another process is still migrating) | readiness probe, load balancer |

Both need no sign-in and are not written to the request log. `qtestframe healthcheck` probes `/healthz` of the local server for container health checks.

## Prometheus metrics

`GET /metrics` serves metrics in the Prometheus text format. It is **off by default**; turn it on with `metrics.enabled` (or `QTF_METRICS_ENABLED=true`, or **Administration → System settings → Metrics and tracing**). The dashboard *trends* are project data (daily snapshots), not server metrics.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `qtf_http_requests_total` | counter | `route`, `method`, `status` | requests by route template (`/api/v1/tests/{id}`, never the id itself), method and status class (`2xx` … `5xx`) |
| `qtf_http_request_duration_seconds` | histogram | `route`, `method` | duration of requests (event streams excluded) |
| `qtf_http_requests_in_flight` | gauge | | requests being served (event streams excluded) |
| `qtf_live_connections` | gauge | | open live-update streams (Server-Sent Events) |
| `qtf_db_connections` | gauge | `state` (`in_use`, `idle`) | connections of the database pool |
| `qtf_db_max_open_connections` | gauge | | the pool limit |
| `qtf_db_wait_total`, `qtf_db_wait_duration_seconds_total` | counter | | queries that waited for a free connection, and how long |
| `qtf_db_closed_connections_total` | counter | `reason` | connections closed by the pool |
| `qtf_jobs_total` | counter | `kind` (`script_job`, `req_sync`, `report_mail`), `result` | finished background jobs: automated tests (`passed`, `failed`, `skipped`, `cancelled`, `error`, `interrupted`), requirement syncs (`ok`, `partial`, `error`) and scheduled reports (`ok`, `error`) |
| `qtf_script_jobs_running` | gauge | | automated tests running on this process |
| `qtf_logins_total` | counter | `method` (`local`, `oidc`, `saml`, `mfa`, `invitation`, `external`, `other`), `result` (`success`, `failure`) | sign-in attempts |
| `qtf_tenants`, `qtf_users`, `qtf_projects`, `qtf_requirements`, `qtf_tests`, `qtf_runs` | gauge | | the size of the installation, counted at most once a minute |
| `qtf_tracing_spans_total` | counter | `outcome` (`exported`, `failed`, `dropped`) | spans of the trace export |
| `qtf_build_info` | gauge | `version`, `goversion` | always 1 |
| `process_start_time_seconds` | gauge | | start of the process (Unix time) |
| `go_goroutines`, `go_memstats_alloc_bytes`, `go_memstats_heap_inuse_bytes`, `go_memstats_heap_objects`, `go_memstats_sys_bytes`, `go_gc_next_bytes` | gauge | | Go runtime |
| `go_gc_cycles_total`, `go_gc_pause_seconds_total` | counter | | garbage collection |

The endpoint is protected by what is configured, and a request must pass **every** configured check:

- **`metrics.token`** – the scraper sends `Authorization: Bearer <token>`. The token is a secret: write-only in the system settings (stored encrypted), or from `metrics.token_file` or `QTF_METRICS_TOKEN`. A missing or wrong token gets `401`.
- **`metrics.allowed_ips`** – addresses or CIDR ranges (`10.0.0.0/8`). The client address is taken from `X-Forwarded-For` only behind `server.trusted_proxies`. Other addresses get `403`.
- With **neither**, only the loopback address (`127.0.0.1`, `::1`) may scrape: everyone else gets `403`.

`metrics.listen` (e.g. `127.0.0.1:9090`, applies after a restart) serves `/metrics`, `/healthz` and `/readyz` on a listener of its own, in plain HTTP; the main listener then answers `/metrics` with `404`. Keep that address on an internal network.

```yaml
metrics:
  enabled: true
  listen: "10.0.0.5:9090"          # optional
  token_file: metrics-token        # relative: in $CREDENTIALS_DIRECTORY
  allowed_ips: [10.0.0.0/24]
```

The matching Prometheus job:

```yaml
scrape_configs:
  - job_name: qtestframe
    scheme: http
    authorization:
      credentials_file: /etc/prometheus/qtestframe-token
    static_configs:
      - targets: ["10.0.0.5:9090"]
```

An example Grafana dashboard (`examples/monitoring/grafana-dashboard.json`) and alert rules (`examples/monitoring/prometheus-alerts.yml`) are in the repository: error rate, latency, pool saturation, failing jobs and sign-in failures.

## OpenTelemetry tracing

With tracing on, the server records a span for every request (named after its route, e.g. `GET /api/v1/tests/{id}`), a child span for every database query of the request (the statement without its values) and a client span for every outgoing call (webhooks, trackers, AI, …). Spans are sent in batches to an OpenTelemetry collector over **OTLP/HTTP with JSON** (every 5 seconds or 512 spans; at most 4096 wait, more are dropped and counted).

| Setting | Meaning |
|---|---|
| `tracing.enabled` | on/off (off by default) |
| `tracing.endpoint` | the collector, e.g. `http://otel-collector:4318`; `/v1/traces` is added to an address without a path |
| `tracing.headers` | headers of every export as `key=value` pairs separated by commas, e.g. `Authorization=Bearer <token>`; a secret (write-only, stored encrypted, or `tracing.headers_file`, `QTF_TRACING_HEADERS`) |
| `tracing.sample_ratio` | share of new traces that are recorded, `0`–`1` (default `1`) |
| `tracing.service_name` | `service.name` of the spans (default `qtestframe`) |

W3C Trace Context is followed both ways: a request with a `traceparent` header continues the caller's trace (and its sampling decision), and outgoing calls carry a `traceparent` header of their own. Health checks and scrapes are not traced. The export does not go through the outgoing-connection rules of the server (like syslog). The section **Metrics and tracing** of the system settings shows the spans exported, failed and dropped and the last error of the collector.

```yaml
tracing:
  enabled: true
  endpoint: "http://otel-collector:4318"
  headers_file: otlp-headers       # e.g. Authorization=Bearer <token>
  sample_ratio: "0.2"
  service_name: qtestframe
```

## What to monitor

| Signal | How |
|---|---|
| liveness and readiness | `/healthz`, `/readyz` (e.g. Prometheus `blackbox_exporter`, module `http_2xx`) |
| error rate and latency | `qtf_http_requests_total{status="5xx"}`, `qtf_http_request_duration_seconds` (see the alert rules in `examples/monitoring/`) |
| background work | `qtf_jobs_total` with a `result` of `error`, `failed` or `partial` |
| service state | `systemctl is-active qtestframe`; the unit restarts the server on failure |
| errors | log records with `level=ERROR`; `stored system settings are invalid`; `tls reload failed` |
| attacks | many `login failed`, `second factor failed` or SCIM authentication failures from one address |
| certificate expiry | `scripts/certs/verify-cert.sh -q --connect qtf.example.com:8443 --warn-days 21` (exit 2 = expiring) |
| resources | `systemctl status qtestframe` (memory and tasks against `MemoryMax`/`TasksMax`), free disk for the database and backups |
| backups | the age of the newest backup and of the last restore test |
| key of the stored secrets | `qtestframe secrets status` shows no plaintext secrets and the expected key |

A simple cron alternative to a monitoring system:

```sh
curl -fsS --max-time 5 https://qtf.example.com/readyz >/dev/null || echo "qtestframe not ready" | mail -s ALERT ops@example.com
```
