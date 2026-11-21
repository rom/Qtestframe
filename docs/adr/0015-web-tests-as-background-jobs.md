# 15. Web tests with Playwright as background jobs

Date: 2026-11-21

## Status

Accepted. Extends [ADR 0011](0011-script-runner-sandbox-via-re-exec.md).

## Context

Automated tests could carry a shell, Python or JavaScript script that the
server ran synchronously inside the "Run script" request. Teams asked to
run **web tests in a real browser** from the tool. A browser test takes
seconds to minutes, needs the network and a browser (many system calls,
threads, shared memory), and produces evidence (screenshots, traces,
videos). Running it inside an HTTP request would hold the request (and,
for bulk routes, a database transaction) open for minutes, and the strict
script sandbox (seccomp deny-list, optional socket deny) would break the
browser.

## Decision

* New script language **`playwright`**. The script is the body of one
  Playwright test (wrapped into `test(<title>, async ({ page, context,
  request, baseURL }) => { … })`) or, when it calls `test(` itself, a
  complete spec file. Per job the runner writes the spec and a generated
  `playwright.config.js` (testDir = job directory, baseURL, browser,
  screenshot/trace/video modes, JSON reporter + list, outputDir in the job
  directory, per-test timeout, one worker, no retries) and runs
  `node <playwright_dir>/node_modules/@playwright/test/cli.js test` with the
  configured installation (`runner.web`). The result comes from the JSON
  report, not the exit code; screenshots, traces and videos named in it are
  attached to the execution.
* **Background jobs** (table `script_jobs`, `internal/api/scriptjobs.go`):
  "Run script" of a web test and "Run automated tests" of a run enqueue
  jobs and answer at once; the execution is `in_progress` until its job
  ends and keeps its previous result in the job, to be restored when the
  job is cancelled, skipped or interrupted. A worker pool per process
  (`runner.web.max_parallel`, default 2) runs jobs in queue order; a job
  waits for the jobs of its prerequisites in the same batch and re-checks
  readiness before it starts.
* Jobs **do not survive a restart**. Each process has an instance id and
  refreshes the heartbeat of its jobs; at start-up unfinished jobs are
  reset (SQLite: all of them; PostgreSQL: those whose heartbeat is stale,
  because several instances may share the database). A job's principal is
  kept in memory, so only the process that enqueued a job runs it.
* Scripts of the other languages keep running synchronously on "Run
  script" (they are bounded by `runner.timeout` and quick); "Run automated
  tests" runs them as jobs too.
* **Sandbox for web tests**: the same re-exec (`sandbox-exec --web`) with
  no_new_privs, dropped capabilities, rlimits suited to a browser and
  Landlock (writes only to the job directory, `/dev/shm`, a short TMPDIR),
  but **no seccomp filter** and **always network access**. The server adds
  the Playwright, browser and Node.js directories to its own Landlock rules
  when web tests are enabled.
* **Where tests run**: project settings (`projects.web_tests`) hold a base
  URL, a base URL per run environment and test variables (secret values
  write-only, masked in output); a configuration dimension "Base URL"
  overrides the URL and "Browser"/"Webbläsare" picks chromium, firefox or
  webkit.

## Consequences

* Users with `script.execute` can make the server's browser reach any host
  the server can reach; operators restrict egress with the firewall.
* Results of web tests appear asynchronously; the runner and the run page
  poll the execution and `GET /runs/{id}/jobs`.
* A restart loses queued and running jobs (their executions are reset with
  a note); there is no durable queue to operate.
* The Playwright installation (Node.js, browsers) is outside the static
  binary; the container image gets it only with `--build-arg
  WITH_PLAYWRIGHT=true`.
