# Logging and monitoring

## The server log

The server writes structured log records (Go `log/slog`) to **standard error**; under systemd they go to the journal (`journalctl -u qtestframe -f`, `-o json` for the fields). Two formats:

| `logging.format` | Example |
|---|---|
| `text` (default) | `time=2026-11-22T09:14:03.120Z level=INFO msg="http request" method=GET path=/api/v1/projects status=200 bytes=5120 duration_ms=4 remote=10.0.0.7` |
| `json` | `{"time":"2026-11-22T09:14:03.120Z","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/projects","status":200,…}` |

`logging.level` is `debug`, `info` (default), `warn` or `error`. Level and format can be changed at runtime in **Administration → System settings → Logging and syslog**, unless `QTF_LOG_LEVEL`/`QTF_LOG_FORMAT` set them (the container image sets the format to `json`).

What is logged:

- **Every request** except `/healthz` and `/readyz`: method, path (without the query), status, bytes, duration and client address (from `X-Forwarded-For` only for `trusted_proxies`). Requests answered with 5xx are logged at ERROR, the others at INFO.
- **Audit records** (`msg=audit`, INFO) with organisation, user, action, entity and id, one for every entry of the audit log.
- **Security events**: `login failed` and `second factor failed` (WARN, with e-mail and client address), configuration and key problems at start-up, the `hardening` line.
- **Background work**: migrations, purged sessions and trash, metric snapshots, e-mail and webhook failures, job resets.

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
| `GET /readyz` | `200 {"status":"ready"}`, or `503` when the database does not answer within 2 seconds | readiness probe, load balancer |

Both need no sign-in and are not written to the request log. `qtestframe healthcheck` probes `/healthz` of the local server for container health checks.

There is **no metrics endpoint** (no Prometheus exporter). The dashboard *trends* are project data (daily snapshots), not server metrics.

## What to monitor

| Signal | How |
|---|---|
| liveness and readiness | `/healthz`, `/readyz` (e.g. Prometheus `blackbox_exporter`, module `http_2xx`) |
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
