#!/usr/bin/env node
// Starts a throw-away Qtestframe server for the end-to-end tests
// (playwright.config.ts → webServer): copies examples/qtestframe.yaml into a
// fresh temporary directory (so the SQLite database starts empty), seeds the
// demo project and serves it on QTF_E2E_PORT. The directory is removed when
// the server stops.
//
//   QTF_BIN        server binary (default ../bin/qtestframe, relative to web/)
//   QTF_E2E_PORT   port to listen on (set by playwright.config.ts)
//   QTF_E2E_KEEP   1 keeps the temporary directory (for debugging)
import { spawn, execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const web = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const bin = resolve(web, process.env.QTF_BIN || '../bin/qtestframe')
const port = process.env.QTF_E2E_PORT || '8099'
if (!existsSync(bin)) {
  console.error(`Qtestframe binary not found at ${bin}: run \`make build\` or set QTF_BIN`)
  process.exit(1)
}
const dir = mkdtempSync(join(tmpdir(), 'qtf-e2e-'))
copyFileSync(resolve(web, '../examples/qtestframe.yaml'), join(dir, 'qtestframe.yaml'))

const env = {
  ...process.env,
  QTF_SERVER_BASE_URL: `http://127.0.0.1:${port}`,
  QTF_LOG_LEVEL: process.env.QTF_LOG_LEVEL || 'error',
}
let cleaned = false
function cleanup() {
  if (cleaned) return
  cleaned = true
  if (process.env.QTF_E2E_KEEP === '1') console.log(`e2e data kept in ${dir}`)
  else rmSync(dir, { recursive: true, force: true })
}

try {
  execFileSync(bin, ['seed-demo', '--config', 'qtestframe.yaml'], { cwd: dir, env, stdio: ['ignore', 'ignore', 'inherit'] })
} catch (e) {
  console.error(`seed-demo failed (${bin}): ${e.message}`)
  cleanup()
  process.exit(1)
}

const server = spawn(bin, ['serve', '--config', 'qtestframe.yaml', '--listen', `127.0.0.1:${port}`], { cwd: dir, env, stdio: 'inherit' })
server.on('exit', (code, signal) => {
  cleanup()
  process.exit(code ?? (signal ? 0 : 1))
})
for (const sig of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.on(sig, () => server.kill('SIGTERM'))
process.on('exit', () => {
  server.kill('SIGTERM')
  cleanup()
})
