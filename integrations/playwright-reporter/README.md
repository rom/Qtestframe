# @qtestframe/playwright-reporter

A [Playwright](https://playwright.dev) reporter that records the results of
your Playwright tests in a [Qtestframe](../../README.md) test run.

- finds the test run by name in your project, or creates it;
- maps every Playwright test to a Qtestframe test by its **test key**
  (`SHOP-TC-12`) and records `passed`, `failed` or `skipped`, the duration
  and an output text with the error and the `test.step()` titles;
- uploads screenshots, traces, videos and other attachments of **failed**
  tests as evidence of the execution;
- optionally creates draft automated tests for Playwright tests that have no
  key yet.

Problems while reporting (server down, wrong token, unknown project) are
logged with a `[qtestframe]` prefix and never fail your test run.

It has no dependencies besides `@playwright/test` (peer dependency) and
needs Node.js 18 or later (it uses the global `fetch`).

## Installation

```sh
npm install --save-dev @qtestframe/playwright-reporter
```

Create an API token in Qtestframe (*Administration → API tokens*) for a user
with the `run.execute` permission (the built-in *tester* role has it; also
`run.write` to create runs and `test.write` for `createMissing`).

## Configuration

`playwright.config.ts`:

```ts
import { defineConfig } from '@playwright/test';

export default defineConfig({
  reporter: [
    ['list'],
    ['@qtestframe/playwright-reporter', {
      url: process.env.QTF_URL,          // e.g. https://qtestframe.example.com
      token: process.env.QTF_TOKEN,      // API token (keep it in a CI secret)
      project: 'SHOP',                   // project key
      run: `Nightly ${process.env.BUILD_NUMBER ?? ''}`.trim(),
      createRun: true,
      build: process.env.BUILD_NUMBER,
      environment: 'staging',
    }],
  ],
});
```

| Option | Default | Meaning |
|---|---|---|
| `url` | `$QTF_URL` | Server base URL. Without `url` or `token` the reporter only logs that it is disabled. |
| `token` | `$QTF_TOKEN` | API token, sent as `Authorization: Bearer …`. |
| `project` | `$QTF_PROJECT` | Project key. Needed unless `runId` is given. |
| `run` | `$QTF_RUN`, else `Playwright <date time>` | Name of the test run. The newest run of that name is used. |
| `runId` | `$QTF_RUN_ID` | Id of an existing run (wins over `project`/`run`). |
| `createRun` | `true` when no `run` is given, else `false` | Create the run when the project has none of that name. |
| `build`, `environment` | `$QTF_BUILD`, `$QTF_ENVIRONMENT` | Stored on a created run. |
| `configuration` | `$QTF_CONFIGURATION` | Configuration (name or id) of the results, e.g. `Chrome / Windows`. The run gets the configuration when it does not target it yet; evidence is attached to the executions in that configuration. Without it, results go to the run's only configuration, or are recorded without configuration. |
| `createMissing` | `false` | Create a draft automated test (titled `describe › test`) for each Playwright test without a key and record its result. Later runs find the test again by its title. Needs `test.write`. |
| `keyPattern` | `/[A-Z][A-Z0-9]{1,15}-TC-\d+/` | Pattern of a test key in tags and titles (RegExp or string). |
| `attachEvidence` | `true` | Upload the attachments of failed tests (`screenshot`, `trace`, `video`, `error-context`, your own `testInfo.attach()`). |
| `maxAttachmentBytes` | 20 MB | Larger attachments are skipped. |
| `timeoutMs` | 60 000 | Timeout of one request. |
| `logger` | `console.log` | Receives the reporter's messages. |

To get screenshots and traces of failures, enable them in `use`, e.g.
`screenshot: 'only-on-failure', trace: 'retain-on-failure'`.

## Tagging tests with keys

The key of a test is taken from, in this order:

1. an annotation of type `qtestframe`:

   ```ts
   test('pays by card', {
     annotation: { type: 'qtestframe', description: 'SHOP-TC-12' },
   }, async ({ page }) => { /* … */ });
   ```

2. a tag:

   ```ts
   test('pays by card', { tag: '@SHOP-TC-12' }, async ({ page }) => { /* … */ });
   ```

3. the title or the title of an enclosing `describe`:

   ```ts
   test('SHOP-TC-12 pays by card', async ({ page }) => { /* … */ });
   ```

The same key on several tests (for example the same test in the
`chromium` and `firefox` projects) gives one result: failed wins over
passed, durations add up and the output lists each test. With retries the
last attempt counts; a flaky test (passed on retry) counts as passed and a
`test.fail()` test that fails as expected counts as passed.

Results are sent in one request when the test run ends. The server applies
its readiness rules: results of tests that may not be executed (for example
drafts, when the organisation does not allow them) are rejected, and the
reporter logs why. Keys that match no test are logged as unknown.

## CI examples

### GitHub Actions

```yaml
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: 22 }
      - run: npm ci
      - run: npx playwright install --with-deps
      - run: npx playwright test
        env:
          QTF_URL: https://qtestframe.example.com
          QTF_TOKEN: ${{ secrets.QTF_TOKEN }}
          QTF_PROJECT: SHOP
          QTF_RUN: "CI ${{ github.ref_name }} #${{ github.run_number }}"
          QTF_BUILD: ${{ github.sha }}
```

### GitLab CI

```yaml
e2e:
  image: mcr.microsoft.com/playwright:v1.64.1-noble
  variables:
    QTF_URL: https://qtestframe.example.com
    QTF_PROJECT: SHOP
    QTF_RUN: "CI $CI_COMMIT_REF_NAME #$CI_PIPELINE_IID"
    QTF_BUILD: $CI_COMMIT_SHORT_SHA
  script:
    - npm ci
    - npx playwright test
  # QTF_TOKEN is a masked CI/CD variable of the project
```

With `QTF_RUN` set, set `createRun: true` in the reporter options (or the
run must exist already).

## Without the reporter

Any tool that writes JUnit XML (Playwright's `junit` reporter, pytest,
go-junit-report, Maven Surefire, Jest, Vitest …) can be imported with the
`qtestframe report` command or `POST /api/v1/runs/{id}/junit`; see the
Qtestframe README and API documentation.

## Development

```sh
npm install
npm run typecheck
npm test
npm run build   # writes dist/
```
