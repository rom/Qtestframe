# Loggning och övervakning

## Serverloggen

Servern skriver strukturerade loggposter (Go `log/slog`) till **standard error**; under systemd hamnar de i journalen (`journalctl -u qtestframe -f`, `-o json` för fälten). Två format:

| `logging.format` | Exempel |
|---|---|
| `text` (standard) | `time=2026-11-22T09:14:03.120Z level=INFO msg="http request" method=GET path=/api/v1/projects status=200 bytes=5120 duration_ms=4 remote=10.0.0.7` |
| `json` | `{"time":"2026-11-22T09:14:03.120Z","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/projects","status":200,…}` |

`logging.level` är `debug`, `info` (standard), `warn` eller `error`. Nivå och format kan ändras under drift under **Administration → Systeminställningar → Loggning och syslog**, om inte `QTF_LOG_LEVEL`/`QTF_LOG_FORMAT` sätter dem (containeravbilden sätter formatet till `json`).

Det här loggas:

- **Varje begäran** utom `/healthz` och `/readyz`: metod, sökväg (utan frågesträng), status, byte, tid och klientadress (från `X-Forwarded-For` bara för `trusted_proxies`). Begäranden som besvaras med 5xx loggas på nivån ERROR, övriga på INFO.
- **Granskningsposter** (`msg=audit`, INFO) med organisation, användare, åtgärd, objekt och id, en för varje post i händelseloggen.
- **Säkerhetshändelser**: `login failed` och `second factor failed` (WARN, med e-post och klientadress), problem med konfiguration och nycklar vid start, raden `hardening`.
- **Bakgrundsarbete**: migreringar, rensade sessioner och papperskorgsposter, mätpunkter, fel vid e-post och webhooks, återställda jobb.

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
| `GET /readyz` | `200 {"status":"ready"}`, eller `503` när databasen inte svarar inom 2 sekunder | readiness-kontroll, lastbalanserare |

Båda kräver ingen inloggning och skrivs inte i begärandeloggen. `qtestframe healthcheck` kontrollerar `/healthz` på den lokala servern för hälsokontroller i containrar.

Det finns **ingen adress för mätvärden** (ingen Prometheus-exporter). *Trenderna* på översiktspanelen är projektdata (dagliga mätpunkter), inte servermätvärden.

## Vad som bör övervakas

| Signal | Hur |
|---|---|
| liveness och readiness | `/healthz`, `/readyz` (t.ex. Prometheus `blackbox_exporter`, modulen `http_2xx`) |
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
