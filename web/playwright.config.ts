// End-to-end tests of Qtestframe in a real browser (see docs/testing.md).
//
//   QTF_BIN        server binary (default ../bin/qtestframe; build it with `make build`)
//   QTF_E2E_PORT   port of the test server (default: a free port)
//   QTF_E2E_URL    test an already running server instead (seeded with
//                  `qtestframe seed-demo`, admin@example.com / Change-me-1234)
//   PW_CHROMIUM    Chromium executable (set without a path: /opt/pw-browsers/chromium)
import { defineConfig, devices } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { chromium } from 'playwright-core'
import { STORAGE_STATE } from './e2e/helpers'

/** A free TCP port, chosen once: workers inherit it through the environment. */
function freePort(): string {
  const script = "const s=require('net').createServer();s.listen(0,'127.0.0.1',()=>{process.stdout.write(String(s.address().port));s.close()})"
  return execFileSync(process.execPath, ['-e', script], { encoding: 'utf8' }).trim()
}
if (!process.env.QTF_E2E_URL && !process.env.QTF_E2E_PORT) process.env.QTF_E2E_PORT = freePort()
const baseURL = (process.env.QTF_E2E_URL || `http://127.0.0.1:${process.env.QTF_E2E_PORT}`).replace(/\/+$/, '')

/** Playwright's own Chromium, else a pre-installed one (PW_CHROMIUM). */
function executablePath(): string | undefined {
  const env = process.env.PW_CHROMIUM
  if (env) return existsSync(env) ? env : '/opt/pw-browsers/chromium'
  try {
    if (existsSync(chromium.executablePath())) return undefined
  } catch {
    /* no bundled browser */
  }
  return existsSync('/opt/pw-browsers/chromium') ? '/opt/pw-browsers/chromium' : undefined
}

const CI = !!process.env.CI

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup.ts',
  fullyParallel: true,
  forbidOnly: CI,
  retries: CI ? 1 : 0,
  workers: CI ? 2 : undefined,
  timeout: 30_000,
  expect: { timeout: 7_000 },
  reporter: [['list'], ['html', { outputFolder: 'playwright-report', open: 'never' }]],
  use: {
    baseURL,
    headless: true,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    locale: 'en-GB',
    reducedMotion: 'reduce',
    storageState: STORAGE_STATE,
    launchOptions: { executablePath: executablePath() },
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } }],
  webServer: process.env.QTF_E2E_URL
    ? undefined
    : {
        command: 'node e2e/server.mjs',
        url: `${baseURL}/healthz`,
        reuseExistingServer: false,
        timeout: 60_000,
        // SIGTERM (not the default SIGKILL) lets server.mjs remove its temporary directory
        gracefulShutdown: { signal: 'SIGTERM', timeout: 10_000 },
        stdout: 'ignore',
        stderr: 'pipe',
      },
})
