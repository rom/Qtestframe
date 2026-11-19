# 12. Syslog shipping with RFC 5424 over UDP, TCP and TLS

Date: 2026-11-19

## Status

Accepted

## Context

The audit trail and operational logs must leave the host so that a
compromised server cannot erase them. Customer environments already run
rsyslog, syslog-ng or a SIEM that accepts syslog. Adding an agent or a
vendor-specific log shipper would widen the deployment and the confinement
policies.

## Decision

* Logging uses `log/slog` (text or JSON) to stdout/journald.
* With `logging.syslog.enabled`, `internal/logging` fans every record out to
  a built-in syslog writer:
  * transports `udp`, `tcp`, `tls` (TLS 1.2+, server verified against
    `ca_file` or the system roots, optional client certificate for mutual
    TLS), `unix` and `unixgram`;
  * framing RFC 5424 (default; `<PRI>1 TIMESTAMP HOST TAG PID - - MSG`) or
    RFC 3164; stream transports use RFC 6587 octet counting;
  * the message body is the slog record as JSON; severity is derived from
    the level (ERROR → 3, WARN → 4, INFO → 6, DEBUG → 7); facility is
    configurable (default `local0`);
  * the connection is lazy and re-established once per message on failure.
* Audit events are written both to `audit_log` and as `audit` log records,
  so they are shipped too.

## Consequences

* No extra agent; works with standard receivers (configuration examples in
  [operations.md](../operations.md#8-logging-and-central-syslog)).
* Delivery is best effort: there is no on-disk queue, a message is dropped
  if the receiver is unreachable after one reconnect, and UDP may lose
  messages. Use `tls` for integrity and confidentiality.
* Sending is synchronous under a mutex with a 5-second write deadline; a
  slow receiver slows down logging.
