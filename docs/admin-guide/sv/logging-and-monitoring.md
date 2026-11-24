# Loggning och övervakning

## Serverloggen

Servern skriver strukturerade loggposter (Go `log/slog`) till **standard error**; under systemd hamnar de i journalen (`journalctl -u qtestframe -f`, `-o json` för fälten). Två format:

| `logging.format` | Exempel |
|---|---|
| `text` (standard) | `time=2026-11-22T09:14:03.120Z level=INFO msg="http request" method=GET path=/api/v1/projects status=200 bytes=5120 duration_ms=4 remote=10.0.0.7` |
| `json` | `{"time":"2026-11-22T09:14:03.120Z","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/projects","status":200,…}` |

`logging.level` är `debug`, `info` (standard), `warn` eller `error`. Nivå och format kan ändras under drift under **Administration → Systeminställningar → Loggning och syslog**, om inte `QTF_LOG_LEVEL`/`QTF_LOG_FORMAT` sätter dem (containeravbilden sätter formatet till `json`).

Det här loggas:

- **Varje begäran** utom `/healthz`, `/readyz` och `/metrics`: metod, sökväg (utan frågesträng), status, byte, tid och klientadress (från `X-Forwarded-For` bara för `trusted_proxies`). Begäranden som besvaras med 5xx loggas på nivån ERROR, övriga på INFO.
- **Granskningsposter** (`msg=audit`, INFO) med organisation, användare, åtgärd, objekt och id, en för varje post i händelseloggen.
- **Säkerhetshändelser**: `login failed` och `second factor failed` (WARN, med e-post och klientadress), problem med konfiguration och nycklar vid start, raden `hardening`.
- **Bakgrundsarbete**: migreringar, rensade sessioner och papperskorgsposter, mätpunkter, fel vid e-post och webhooks, återställda jobb.

När en begäran hör till ett spår (en inkommande `traceparent`-rubrik, eller spårning på) får dess poster `trace_id`, och `span_id` för den här serverns spann, så att en loggrad kan hittas från ett spår och tvärtom.

Lösenord, token, kakor och `Authorization`-huvuden loggas aldrig. Oväntade fel besvaras bara med "internal server error"; detaljerna finns i loggraden `request failed` (sökväg och fel).

## Central syslog

Varje post – även granskningsposterna – kan också skickas till en syslog-server, som ett JSON-meddelande per post:

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
    format: rfc5424              # eller rfc3164
    ca_file: /etc/qtestframe/tls/ca.crt
    cert_file: /etc/qtestframe/tls/syslog-client.crt   # valfritt, ömsesidig TLS
    key_file: /etc/qtestframe/tls/syslog-client.key
```

- Strömmande transporter (`tcp`, `tls`, `unix`) använder oktetträkning enligt RFC 6587; `tls` följer RFC 5425 och verifierar servern mot `ca_file` (eller systemets rotcertifikat) med TLS 1.2 eller senare. Ett klientcertifikat skapas med `scripts/certs/gen-client-cert.sh`.
- Allvarlighetsgrader: ERROR → 3, WARN → 4, INFO → 6, DEBUG → 7. Meddelandets APP-NAME är `tag`.
- Leveransen sker efter bästa förmåga: ett nytt anslutningsförsök per meddelande, en skrivgräns på 5 sekunder, ingen kö på disk. Journalen har den lokala kopian.
- Med SELinux: `setsebool -P qtestframe_remote_syslog on`.
- **Skicka testmeddelande** i systeminställningarna skickar ett meddelande med värdena i formuläret.

Exempel på mottagare för rsyslog (`imtcp` med `gtls`) och syslog-ng (`network(transport("tls") flags(syslog-protocol))`) finns i [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#8-logging-and-central-syslog) (på engelska). JSON-innehållet kan delas upp i fält med rsyslog `mmjsonparse` eller syslog-ng `json-parser()`.

## Hälsokontroller

| Adress | Svarar | Användning |
|---|---|---|
| `GET /healthz` | `200 {"status":"ok","version":"…"}` så länge processen besvarar begäranden | liveness-kontroll |
| `GET /readyz` | `200 {"status":"ready","checks":{"database":"ok","migrations":"ok"}}`, eller `503` när databasen inte svarar inom 2 sekunder eller migreringar inte är gjorda (en annan process migrerar fortfarande) | readiness-kontroll, lastbalanserare |

Båda kräver ingen inloggning och skrivs inte i begärandeloggen. `qtestframe healthcheck` kontrollerar `/healthz` på den lokala servern för hälsokontroller i containrar.

## Prometheus-mätvärden

`GET /metrics` visar mätvärden i Prometheus textformat. Den är **av som standard**; slå på den med `metrics.enabled` (eller `QTF_METRICS_ENABLED=true`, eller **Administration → Systeminställningar → Mätvärden och spårning**). *Trenderna* på översiktspanelen är projektdata (dagliga mätpunkter), inte servermätvärden.

| Mätvärde | Typ | Etiketter | Betydelse |
|---|---|---|---|
| `qtf_http_requests_total` | counter | `route`, `method`, `status` | förfrågningar per vägmall (`/api/v1/tests/{id}`, aldrig själva id:t), metod och statusklass (`2xx` … `5xx`) |
| `qtf_http_request_duration_seconds` | histogram | `route`, `method` | förfrågningarnas svarstid (utan händelseströmmar) |
| `qtf_http_requests_in_flight` | gauge | | förfrågningar som besvaras just nu (utan händelseströmmar) |
| `qtf_live_connections` | gauge | | öppna strömmar för liveuppdateringar (Server-Sent Events) |
| `qtf_db_connections` | gauge | `state` (`in_use`, `idle`) | anslutningar i databaspoolen |
| `qtf_db_max_open_connections` | gauge | | poolens gräns |
| `qtf_db_wait_total`, `qtf_db_wait_duration_seconds_total` | counter | | frågor som väntade på en ledig anslutning, och hur länge |
| `qtf_db_closed_connections_total` | counter | `reason` | anslutningar som poolen stängt |
| `qtf_jobs_total` | counter | `kind` (`script_job`, `req_sync`, `report_mail`), `result` | avslutade bakgrundsjobb: automatiserade tester (`passed`, `failed`, `skipped`, `cancelled`, `error`, `interrupted`), kravsynkningar (`ok`, `partial`, `error`) och schemalagda rapporter (`ok`, `error`) |
| `qtf_script_jobs_running` | gauge | | automatiserade tester som körs i den här processen |
| `qtf_logins_total` | counter | `method` (`local`, `oidc`, `saml`, `mfa`, `invitation`, `external`, `other`), `result` (`success`, `failure`) | inloggningsförsök |
| `qtf_tenants`, `qtf_users`, `qtf_projects`, `qtf_requirements`, `qtf_tests`, `qtf_runs` | gauge | | installationens storlek, räknas högst en gång i minuten |
| `qtf_tracing_spans_total` | counter | `outcome` (`exported`, `failed`, `dropped`) | spann i spårexporten |
| `qtf_build_info` | gauge | `version`, `goversion` | alltid 1 |
| `process_start_time_seconds` | gauge | | när processen startade (Unix-tid) |
| `go_goroutines`, `go_memstats_alloc_bytes`, `go_memstats_heap_inuse_bytes`, `go_memstats_heap_objects`, `go_memstats_sys_bytes`, `go_gc_next_bytes` | gauge | | Go-körtiden |
| `go_gc_cycles_total`, `go_gc_pause_seconds_total` | counter | | skräpsamling |

Slutpunkten skyddas av det som är konfigurerat, och en förfrågan måste klara **varje** konfigurerad kontroll:

- **`metrics.token`** – insamlaren skickar `Authorization: Bearer <token>`. Token är en hemlighet: kan bara skrivas i systeminställningarna (sparas krypterad), eller kommer från `metrics.token_file` eller `QTF_METRICS_TOKEN`. En saknad eller fel token ger `401`.
- **`metrics.allowed_ips`** – adresser eller CIDR-intervall (`10.0.0.0/8`). Klientens adress tas från `X-Forwarded-For` bara bakom `server.trusted_proxies`. Andra adresser får `403`.
- Utan **någon av dem** får bara loopback-adressen (`127.0.0.1`, `::1`) hämta: alla andra får `403`.

`metrics.listen` (t.ex. `127.0.0.1:9090`, gäller efter en omstart) visar `/metrics`, `/healthz` och `/readyz` på en egen lyssnare, med vanlig HTTP; huvudlyssnaren svarar då `404` på `/metrics`. Håll den adressen på ett internt nät.

```yaml
metrics:
  enabled: true
  listen: "10.0.0.5:9090"          # valfritt
  token_file: metrics-token        # relativ: i $CREDENTIALS_DIRECTORY
  allowed_ips: [10.0.0.0/24]
```

Motsvarande jobb i Prometheus:

```yaml
scrape_configs:
  - job_name: qtestframe
    scheme: http
    authorization:
      credentials_file: /etc/prometheus/qtestframe-token
    static_configs:
      - targets: ["10.0.0.5:9090"]
```

En exempelpanel för Grafana (`examples/monitoring/grafana-dashboard.json`) och larmregler (`examples/monitoring/prometheus-alerts.yml`) finns i förrådet: felandel, svarstid, mättad databaspool, misslyckade jobb och misslyckade inloggningar.

## OpenTelemetry-spårning

Med spårning på spelar servern in ett spann för varje förfrågan (namngivet efter vägen, t.ex. `GET /api/v1/tests/{id}`), ett underordnat spann för varje databasfråga i förfrågan (satsen utan värden) och ett klientspann för varje utgående anrop (webhooks, ärendesystem, AI, …). Spannen skickas i omgångar till en OpenTelemetry-insamlare över **OTLP/HTTP med JSON** (var 5:e sekund eller 512 spann; högst 4096 väntar, fler kastas och räknas).

| Inställning | Betydelse |
|---|---|
| `tracing.enabled` | på/av (av som standard) |
| `tracing.endpoint` | insamlaren, t.ex. `http://otel-collector:4318`; `/v1/traces` läggs till en adress utan sökväg |
| `tracing.headers` | rubriker för varje export som `nyckel=värde`-par åtskilda med komma, t.ex. `Authorization=Bearer <token>`; en hemlighet (kan bara skrivas, sparas krypterad, eller `tracing.headers_file`, `QTF_TRACING_HEADERS`) |
| `tracing.sample_ratio` | andel nya spår som spelas in, `0`–`1` (standard `1`) |
| `tracing.service_name` | spannens `service.name` (standard `qtestframe`) |

W3C Trace Context följs åt båda hållen: en förfrågan med en `traceparent`-rubrik fortsätter anroparens spår (och dess urvalsbeslut), och utgående anrop får en egen `traceparent`-rubrik. Hälsokontroller och insamlingar spåras inte. Exporten går inte genom serverns regler för utgående anslutningar (precis som syslog). Avsnittet **Mätvärden och spårning** i systeminställningarna visar antalet exporterade, misslyckade och kastade spann och insamlarens senaste fel.

```yaml
tracing:
  enabled: true
  endpoint: "http://otel-collector:4318"
  headers_file: otlp-headers       # t.ex. Authorization=Bearer <token>
  sample_ratio: "0.2"
  service_name: qtestframe
```

## Vad som bör övervakas

| Signal | Hur |
|---|---|
| liveness och readiness | `/healthz`, `/readyz` (t.ex. Prometheus `blackbox_exporter`, modulen `http_2xx`) |
| felandel och svarstid | `qtf_http_requests_total{status="5xx"}`, `qtf_http_request_duration_seconds` (se larmreglerna i `examples/monitoring/`) |
| bakgrundsarbete | `qtf_jobs_total` med `result` `error`, `failed` eller `partial` |
| tjänstens tillstånd | `systemctl is-active qtestframe`; enheten startar om servern vid fel |
| fel | loggposter med `level=ERROR`; `stored system settings are invalid`; `tls reload failed` |
| angrepp | många `login failed`, `second factor failed` eller misslyckade SCIM-autentiseringar från en adress |
| certifikatets giltighet | `scripts/certs/verify-cert.sh -q --connect qtf.example.com:8443 --warn-days 21` (avslutningskod 2 = går ut snart) |
| resurser | `systemctl status qtestframe` (minne och processer mot `MemoryMax`/`TasksMax`), ledigt diskutrymme för databasen och säkerhetskopiorna |
| säkerhetskopior | åldern på den senaste säkerhetskopian och på det senaste återställningstestet |
| nyckeln till de lagrade hemligheterna | `qtestframe secrets status` visar inga klartexthemligheter och den förväntade nyckeln |

Ett enkelt alternativ med cron till ett övervakningssystem:

```sh
curl -fsS --max-time 5 https://qtf.example.com/readyz >/dev/null || echo "qtestframe svarar inte" | mail -s LARM ops@example.com
```
