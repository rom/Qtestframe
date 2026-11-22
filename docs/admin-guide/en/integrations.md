# E-mail, webhooks and integrations

## E-mail

Outgoing e-mail is optional. With it, users can get their notifications by e-mail (each one at once or in a daily digest, chosen in their profile), admins can invite new members by e-mail, and reports can be sent by e-mail or on a schedule. Without it these features are hidden and the API answers that e-mail is not configured.

```yaml
server:
  base_url: https://qtf.example.com      # required with e-mail: links in the messages
email:
  host: smtp.example.com
  port: 587                  # 587 = STARTTLS, 465 = implicit TLS
  tls: starttls              # starttls | tls | none (local relay only)
  username: qtestframe       # leave out for a relay without authentication
  password_file: smtp-password
  from: qtestframe@example.com
  from_name: Qtestframe
  timeout: 30s
  digest_hour: 7             # UTC
```

E-mail is on while `email.host` is set (`enabled: false` keeps the settings but sends nothing). The same settings can be changed at runtime in **Administration → System settings → E-mail (SMTP)**, which also has **Send test e-mail**; organisation admins have **Send test e-mail** under **Members**. Both show the answer of the mail server when sending fails.

- Mail is sent in the background, never for a request that failed; failures are logged at WARN. On shutdown the server waits for messages in flight (each bounded by `email.timeout`).
- The daily digest and scheduled reports are checked every minute; the digest goes out at `digest_hour` (UTC).
- Invitation links are valid for 7 days and can be used once. Sending reports by e-mail is limited to 30 per user in 10 minutes.
- Notifications, @mentions and report e-mails go only to people who may see the project they are about.

## Notifications

In-app notifications (assignments, @mentions, review requests, defect changes, project sharing) need no configuration. Users choose in their profile whether they also get them by e-mail.

## Webhooks

Projects can send signed events to other systems. Users with `project.update` set them up in the project settings; every webhook has a URL, the events it wants (or all) and a secret that is shown once when created or rotated.

<!-- BEGIN GENERATED: webhook-events (make admin-guide-ref) -->

| Object | Events |
|---|---|
| requirement | `requirement.created`, `requirement.updated` |
| test | `test.created`, `test.updated` |
| run | `run.created`, `run.updated`, `run.completed` |
| execution | `execution.updated` |
| defect | `defect.created`, `defect.updated`, `defect.status`, `defect.assigned`, `defect.regression` |
| risk | `risk.created`, `risk.updated` |
| release | `release.created`, `release.updated`, `release.released` |
| comment | `comment.created` |
| review | `review.requested`, `review.completed` |
| baseline | `baseline.signed` |
| waiver | `waiver.requested`, `waiver.approved`, `waiver.rejected`, `waiver.revoked` |

<!-- END GENERATED: webhook-events -->

Each delivery is a `POST` with a JSON body `{"id", "event", "project_id", "actor", "time", "data"}` and the headers:

| Header | |
|---|---|
| `X-Qtestframe-Event` | the event, e.g. `run.completed` |
| `X-Qtestframe-Webhook` | the id of the webhook |
| `X-Qtestframe-Signature` | `sha256=` + hex HMAC-SHA256 of the raw body with the webhook's secret |
| `User-Agent` | `Qtestframe-Webhook/1` |

Receivers should verify the signature (compare in constant time) and ignore repeated `id`s. Deliveries are made after the change is committed, time out after 10 seconds, **never follow redirects** and are tried once; the status of the last delivery is shown with the webhook, and failures are logged (`webhook delivery`). The webhook secrets are stored encrypted. Loaded project archives contain no secrets and their webhooks are disabled.

## Chat notifications

Projects can post their events to chat channels: Slack, Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix and Discord. Users with `project.update` set the channels up in the project settings (see the user guide, *Reviews and notifications*). Nothing needs to be configured on the server, but:

- Set `server.base_url` (or the system setting): without it messages cannot link to the items.
- The server must reach the chat tool over HTTPS (outbound proxy and firewall). Posts time out after 10 seconds and never follow redirects.
- The webhook URL or the Matrix token of a channel is stored encrypted like the other secrets (`qtestframe secrets` re-encrypts it), is never returned by the API, written to the audit log or shown in an error, and is left out of project archives (a loaded channel is off until someone enters a new URL).

**Events.** A channel offers the events of the central event registry (the same as the webhooks above), grouped for the settings. The list is derived from the registry: an event that a later version adds appears in the channel settings and in **All events** without any change to the chat code; its group is taken from the part of its name before the dot (unknown parts go to *Other*), its label from the event name unless a translation exists.

<!-- BEGIN GENERATED: chat-events (make admin-guide-ref) -->

| Group | Events |
|---|---|
| Test runs | `run.created` (Test run created), `run.updated` (Test run updated), `run.completed` (Test run completed), `execution.updated` (Test result recorded) |
| Defects | `defect.created` (Defect reported), `defect.updated` (Defect updated), `defect.status` (Defect status changed), `defect.assigned` (Defect assigned), `defect.regression` (Regression detected) |
| Requirements and tests | `requirement.created` (Requirement created), `requirement.updated` (Requirement updated), `test.created` (Test created), `test.updated` (Test updated), `comment.created` (New comment), `baseline.signed` (Baseline signed) |
| Reviews | `review.requested` (Review requested), `review.completed` (Review completed) |
| Releases | `release.created` (Release created), `release.updated` (Release updated), `release.released` (Release released) |
| Waivers | `waiver.requested` (Waiver requested), `waiver.approved` (Waiver approved), `waiver.rejected` (Waiver rejected), `waiver.revoked` (Waiver revoked) |
| Risks | `risk.created` (Risk identified), `risk.updated` (Risk updated) |

<!-- END GENERATED: chat-events -->

**Delivery.** Every post is stored in a queue (table `chat_deliveries`) once the change is committed and sent at once in the background. A failed post is retried by the server's minute job with exponential backoff (1, 2, 4, 8 and 16 minutes, or the `Retry-After` of an HTTP 429, at most an hour), up to 6 attempts; an HTTP 4xx answer other than 408 and 429 (the chat tool refused the URL, token or message) is final. Several servers sharing one PostgreSQL database never post the same message twice (a post is claimed before it is sent). After 5 failed posts in a row the channel turns itself off; the project's administrators get an in-app notification (and an e-mail if they chose immediate e-mails), and the audit log has a `chat.disabled` entry. The last 50 posts of each channel are kept as its delivery log; the queue and the log are not part of a backup (the channels are). Digest channels collect their events and post them as one message every N minutes (1–1440).

**Content.** Messages contain the event, the item key and title, the status, the priority, the person and a summary of at most a few hundred characters. They never contain secrets. In a project with restricted access, messages contain only the event, key, status and link unless an organisation administrator (`tenant.manage`) allows the channel the full content. Broadcast mentions (`@channel`, `@here`, `@all`, `@everyone`) in texts are defused.

### Setting up each chat tool

| Tool | What to create | What to enter in Qtestframe |
|---|---|---|
| Slack | A Slack app with **Incoming Webhooks** turned on; **Add New Webhook to Workspace** and choose the channel. Messages use Block Kit. | The webhook URL `https://hooks.slack.com/services/…` |
| Microsoft Teams | In the channel: **Workflows** → template *Post to a channel when a webhook request is received* (Power Automate). Messages are Adaptive Cards 1.4. | The URL of the workflow's HTTP trigger |
| Microsoft Teams (Office 365 connector) | The older **Incoming Webhook** connector of a channel (MessageCard format). Microsoft is retiring these connectors: use it only where Workflows are not available. | The connector URL |
| Mattermost | **Integrations → Incoming Webhooks → Add Incoming Webhook**, choose the channel. Allow integrations to override the user name if the posts should show *Qtestframe*. | The URL `https://<server>/hooks/<id>` |
| Rocket.Chat | **Administration → Workspace → Integrations → New → Incoming**: turn it on, choose the channel (`#qa`) and the user to post as; no script is needed. | The webhook URL shown after saving |
| Google Chat | In the space: **Apps & integrations → Webhooks → Add webhook**. Messages are cards (cardsV2). | The webhook URL (it contains `key` and `token` parameters) |
| Matrix | A bot account on the homeserver; invite it to the room and let it join. Get its access token (for example by signing in with it once through the client API). Messages are `m.notice` events with HTML. | The homeserver URL, the room ID (`!…:server`) or alias (`#…:server`) and the access token |
| Discord | **Channel settings → Integrations → Webhooks → New Webhook**. Messages are embeds; mentions are turned off. | The webhook URL `https://discord.com/api/webhooks/…` |

Use **Send test message** to check a channel; the delivery log shows the HTTP status and the error of each post (for example `HTTP 404: no_service` from Slack when the webhook was removed, `HTTP 401: M_UNKNOWN_TOKEN` from Matrix). Personal chat notifications (direct messages to one user) are not offered; users get those as in-app notifications and e-mails.

## Defect and issue trackers

Projects can push defects and requirements to **Jira**, **GitHub** or **GitLab**, sync their status back and import issues. Users with `project.update` connect a project under its settings ([user guide](../../user-guide/en/defects.md#connect-jira-github-or-gitlab)); pushing and syncing needs `run.execute`, issues from requirements `requirement.write`.

| Tracker | URL | Credential | Smallest useful scope |
|---|---|---|---|
| Jira Cloud | `https://<site>.atlassian.net` | user e-mail + API token (HTTP Basic) | a user that may browse and create issues in the project |
| Jira Server / Data Center | your Jira URL | personal access token (Bearer) | as above |
| GitHub | default `https://api.github.com` (GitHub Enterprise: its API URL) | token (Bearer) | fine-grained token with *Issues: read and write* on the one repository |
| GitLab | default `https://gitlab.com`, or a self-managed instance (HTTPS) | token in `PRIVATE-TOKEN` | project access token with the `api` scope and the Reporter role |

The token is entered once, stored encrypted, and never returned by the API (only *has token*); it is not part of a project archive. Requests honour `HTTPS_PROXY`/`NO_PROXY` and the system's CA certificates, have a timeout, and wait for the tracker's `Retry-After` when it is short. Pushes and syncs are audited (`defect.push`, `defect.sync`, `requirement.issue.*`).

## Script and web test runner

The runner executes the scripts of automated tests (`sh`, `bash`, `python`, `javascript`) and web tests (Playwright) on the server. It is **off by default**; read [the sandbox section](security.md#the-script-and-web-test-sandbox) before turning it on. Its settings apply after a restart.

### Scripts

```yaml
runner:
  enabled: true
  timeout: 60s                       # keep below server.write_timeout
  work_dir: /var/lib/qtestframe/runner
  max_output_bytes: 1048576
  interpreters: {sh: /bin/sh, python: /usr/bin/python3}   # only what you need
  sandbox: true
  deny_network: true                 # when scripts need no network
```

*Run script* of a single script runs within the request, so `runner.timeout` must stay below `server.write_timeout`. *Run automated tests* on a run queues all automated tests of the run as background jobs. The work directory is created with mode 0700 and added to the Landlock paths; job directories are scratch space.

Under systemd, Node.js needs `MemoryDenyWriteExecute=no` in a drop-in (its JIT), or configure the interpreter as a wrapper that runs `node --jitless`; with SELinux enable `qtestframe_run_scripts` (and `qtestframe_runner_execmem` for JIT interpreters). AppArmor runs interpreters in the child profile `qtestframe//runner` without network.

### Web tests (Playwright)

Install Node.js 18 or later and Playwright with at least one browser:

```sh
sudo mkdir -p /opt/qtestframe-playwright && cd /opt/qtestframe-playwright
sudo npm init -y && sudo npm i @playwright/test
sudo PLAYWRIGHT_BROWSERS_PATH=/opt/qtestframe-playwright/browsers npx playwright install --with-deps chromium
```

```yaml
runner:
  enabled: true
  work_dir: /var/lib/qtestframe/runner       # keep it short
  web:
    enabled: true
    node: /usr/bin/node
    playwright_dir: /opt/qtestframe-playwright
    browsers_path: /opt/qtestframe-playwright/browsers
    browser: chromium
    timeout: 60s              # per test
    job_timeout: 10m          # whole Playwright process
    max_parallel: 2           # per server process
    screenshots: on_failure
    trace: on_failure
    video: "off"
```

And a systemd drop-in:

```ini
[Service]
MemoryDenyWriteExecute=no
ExecPaths=/opt/qtestframe-playwright
ReadWritePaths=/dev/shm
```

For each web test the server writes the spec and a generated `playwright.config.js` into a job directory and runs `<node> <playwright_dir>/node_modules/@playwright/test/cli.js test --config <file>` (`npx` is not used, nothing is downloaded); the result comes from Playwright's JSON report. Screenshots, the trace and the video become attachments of the execution (each at most `max_evidence_mb`). A trace contains the pages, the network requests (cookies included) and the console of the test.

Web tests always run as **background jobs**; `max_parallel` of them run at a time per server process. Jobs do not survive a restart: unfinished ones are marked interrupted and their executions get their earlier result back, at start-up (SQLite) or when the instance that queued them has not been seen for about a minute (PostgreSQL).

Projects set the base URL of the system under test and test variables under **Settings → Web tests**. Test variables reach every automated script as `QTF_VAR_<NAME>`; secret ones are stored encrypted and masked as `***` in stored output (not in screenshots or traces).

The container image needs `--build-arg WITH_PLAYWRIGHT=true` and `QTF_RUNNER_ENABLED=true`, `QTF_RUNNER_WEB_ENABLED=true` (see [Installation](installation.md#in-a-container-docker-or-podman)).
