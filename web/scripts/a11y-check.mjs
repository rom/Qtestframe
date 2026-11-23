#!/usr/bin/env node
// Automated accessibility (axe-core, WCAG 2.1 A/AA) and responsive
// (no horizontal page overflow) checks against a running Qtestframe server
// with seed data (`qtestframe seed-demo`). See docs/testing.md.
//
//   QTF_URL            server URL (default http://127.0.0.1:8080)
//   QTF_EMAIL / QTF_PASSWORD   login (default admin@example.com / Change-me-1234)
//   QTF_CHROME         Chrome/Chromium executable (default: Playwright's, else a system Chrome)
//   QTF_A11Y_ONLY      comma-separated route names to check (e.g. "dashboard,traceability")
//   QTF_A11Y_CONFIGS   comma-separated configurations (e.g. "light/en,navy/en"; default all)
//   QTF_A11Y_VIEWPORTS comma-separated viewports (phone, tablet, desktop; default all)
//   QTF_A11Y_KEYBOARD  0 skips the keyboard/focus checks (skip link, route focus, drawers, runner)
//   QTF_A11Y_SHOTS     directory: also save a screenshot per route, viewport and configuration
//
// Exits 1 on any serious or critical axe violation, any horizontal overflow
// or a failed keyboard/focus check, 2 when the server cannot be reached or
// the login fails.
import { createHmac } from 'node:crypto'
import { existsSync, mkdirSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { AxeBuilder } from '@axe-core/playwright'

/** the repository (test fixtures for uploads) */
const REPO = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const BASE = (process.env.QTF_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '')
const EMAIL = process.env.QTF_EMAIL || 'admin@example.com'
const PASSWORD = process.env.QTF_PASSWORD || 'Change-me-1234'
const ONLY = (process.env.QTF_A11Y_ONLY || '').split(',').map((s) => s.trim()).filter(Boolean)
const SHOTS = process.env.QTF_A11Y_SHOTS || ''

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']
const FAIL_IMPACTS = new Set(['serious', 'critical'])

/**
 * Rules disabled for documented false positives. Keep this list empty unless
 * a rule is wrong for this app, and say why next to it.
 */
const DISABLED_RULES = []

const pick = (list, env) => {
  const want = (process.env[env] || '').split(',').map((x) => x.trim()).filter(Boolean)
  return want.length ? list.filter((x) => want.includes(x.name)) : list
}

const ALL_VIEWPORTS = [
  { name: 'phone', width: 390, height: 844, mobile: true },
  { name: 'tablet', width: 820, height: 1180, mobile: true },
  { name: 'tablet-landscape', width: 1024, height: 768, mobile: true },
  { name: 'desktop', width: 1440, height: 900, mobile: false },
]

/** theme + language combinations; axe runs at every viewport in each. */
const ALL_CONFIGS = [
  { name: 'light/en', theme: 'light', lang: 'en' },
  { name: 'dark/sv', theme: 'dark', lang: 'sv' },
  { name: 'navy/en', theme: 'navy', lang: 'en' },
  { name: 'crazy/sv', theme: 'crazy', lang: 'sv' },
  { name: 'green_console/en', theme: 'green_console', lang: 'en' },
  { name: 'ocean/sv', theme: 'ocean', lang: 'sv' },
  { name: 'forest/en', theme: 'forest', lang: 'en' },
  { name: 'sunset/sv', theme: 'sunset', lang: 'sv' },
  { name: 'nordic/en', theme: 'nordic', lang: 'en' },
  { name: 'lavender/sv', theme: 'lavender', lang: 'sv' },
  { name: 'high_contrast/en', theme: 'high_contrast', lang: 'en' },
  { name: 'high_contrast/sv', theme: 'high_contrast', lang: 'sv' },
  { name: 'high_contrast_light/en', theme: 'high_contrast_light', lang: 'en' },
  { name: 'camouflage/en', theme: 'camouflage', lang: 'en' },
  { name: 'city_camouflage/sv', theme: 'city_camouflage', lang: 'sv' },
  { name: 'arctic_camouflage/sv', theme: 'arctic_camouflage', lang: 'sv' },
  { name: 'midnight/en', theme: 'midnight', lang: 'en' },
  { name: 'amber_terminal/en', theme: 'amber_terminal', lang: 'en' },
  { name: 'rose/sv', theme: 'rose', lang: 'sv' },
]

const VIEWPORTS = pick(ALL_VIEWPORTS, 'QTF_A11Y_VIEWPORTS')
const CONFIGS = pick(ALL_CONFIGS, 'QTF_A11Y_CONFIGS')

function chromePath() {
  if (process.env.QTF_CHROME) return process.env.QTF_CHROME
  try {
    const p = chromium.executablePath()
    if (p && existsSync(p)) return undefined
  } catch {
    /* no bundled browser */
  }
  for (const p of ['/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome']) {
    if (existsSync(p)) return p
  }
  return undefined
}

async function api(request, method, path, body, csrf) {
  const res = await request.fetch(`${BASE}/api/v1${path}`, {
    method,
    data: body === undefined ? undefined : body,
    headers: { ...(csrf ? { 'X-CSRF-Token': csrf } : {}), Origin: BASE, 'Content-Type': 'application/json' },
  })
  if (!res.ok()) throw new Error(`${method} ${path}: HTTP ${res.status()} ${(await res.text()).slice(0, 200)}`)
  const text = await res.text()
  return text ? JSON.parse(text) : null
}
const items = (x) => (Array.isArray(x) ? x : (x?.items ?? []))

// ---- live updates: a second user on the item (presence) who saves a newer
// version while the page is open (the notice in the editor; route
// requirement-live) ----
const LIVE_EMAIL = 'a11y.live@example.com'
const LIVE_PASSWORD = 'A11y-live-check-2468'
let liveUser = null

/** Creates the second user (once) and signs it in, in a context of its own. */
async function setupLiveUser(browser, request, csrf) {
  if (!items(await api(request, 'GET', '/members')).some((x) => x.email === LIVE_EMAIL)) {
    await api(request, 'POST', '/members', { email: LIVE_EMAIL, role: 'manager', display_name: 'Lena Live', password: LIVE_PASSWORD }, csrf)
  }
  const ctx = await browser.newContext()
  const me = await api(ctx.request, 'POST', '/auth/login', { email: LIVE_EMAIL, password: LIVE_PASSWORD })
  liveUser = { ctx, csrf: me.csrf_token }
}

/** The second user views and edits the item (presence lasts 45 s). */
async function liveBefore(id) {
  await api(liveUser.ctx.request, 'POST', '/presence', { entity_type: 'requirement', entity_id: id, state: 'editing' }, liveUser.csrf)
}

/** The second user saves a newer version (the rationale gains or loses a full stop). */
async function liveAfter(id) {
  const rq = await api(liveUser.ctx.request, 'GET', `/requirements/${id}`)
  const r = rq.rationale ?? ''
  await api(liveUser.ctx.request, 'PATCH', `/requirements/${id}`, { rationale: r.endsWith('.') ? r.slice(0, -1) : r + '.' }, liveUser.csrf)
}

// ---- multi-factor authentication: a user with an authenticator app, for the
// second step of the sign-in page (route login-mfa) ----
const MFA_EMAIL = 'a11y.mfa@example.com'
const MFA_PASSWORD = 'A11y-mfa-check-2468'

/** RFC 6238 code (SHA-1, 6 digits, 30 s) of a base32 secret. */
function totp(secret, t = Date.now()) {
  const A = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret.replace(/[\s=]/g, '').toUpperCase()) bits += A.indexOf(c).toString(2).padStart(5, '0')
  const key = Buffer.from(bits.match(/.{8}/g).map((b) => parseInt(b, 2)))
  const msg = Buffer.alloc(8)
  msg.writeBigUInt64BE(BigInt(Math.floor(t / 30000)))
  const h = createHmac('sha1', key).update(msg).digest()
  const o = h[h.length - 1] & 15
  return String((h.readUInt32BE(o) & 0x7fffffff) % 1000000).padStart(6, '0')
}

/** Creates (or resets) the MFA test user and sets up its authenticator. */
async function setupMfaUser(browser, request, csrf) {
  let m = items(await api(request, 'GET', '/members')).find((x) => x.email === MFA_EMAIL)
  if (!m) {
    await api(request, 'POST', '/members', { email: MFA_EMAIL, role: 'viewer', display_name: 'A11y MFA', password: MFA_PASSWORD }, csrf)
    m = items(await api(request, 'GET', '/members')).find((x) => x.email === MFA_EMAIL)
  } else if (m.mfa_enabled) {
    await api(request, 'POST', `/members/${m.user_id}/mfa/reset`, undefined, csrf)
  }
  const ctx = await browser.newContext()
  try {
    const me = await api(ctx.request, 'POST', '/auth/login', { email: MFA_EMAIL, password: MFA_PASSWORD })
    await api(ctx.request, 'PATCH', '/auth/me', { preferences: { intro_done: true } }, me.csrf_token)
    const setup = await api(ctx.request, 'POST', '/auth/mfa/totp', undefined, me.csrf_token)
    await api(ctx.request, 'POST', '/auth/mfa/totp/confirm', { code: totp(setup.secret) }, me.csrf_token)
  } finally {
    await ctx.close()
  }
}

/** Storage state of a sign-in waiting for its second factor (the qtf_mfa
 * cookie lives 5 minutes; renewed after 4). */
let mfaPending = null
async function mfaPendingState(browser) {
  if (mfaPending && Date.now() - mfaPending.at < 4 * 60 * 1000) return mfaPending.state
  const ctx = await browser.newContext()
  try {
    const res = await api(ctx.request, 'POST', '/auth/login', { email: MFA_EMAIL, password: MFA_PASSWORD })
    if (!res?.mfa_required) throw new Error('the MFA test user signed in without a second factor')
    mfaPending = { at: Date.now(), state: await ctx.storageState() }
    return mfaPending.state
  } finally {
    await ctx.close()
  }
}

/** Routes to visit, with ids from the seed data. */
async function discover(request) {
  const projects = items(await api(request, 'GET', '/projects'))
  const project = projects.find((p) => p.key === 'DEMO') ?? projects[0]
  if (!project) throw new Error('no project found – run `qtestframe seed-demo` first')
  const pid = project.id
  // the restricted project of seed-demo (project sharing), else the main one
  const vault = projects.find((x) => x.key === 'VAULT') ?? project
  const reqs = items(await api(request, 'GET', `/projects/${pid}/requirements`))
  // a requirement with at least two relations, for the relations tab's bulk removal
  const relCount = new Map()
  for (const r of items(await api(request, 'GET', `/projects/${pid}/relations`))) for (const id of [r.source_id, r.target_id]) relCount.set(id, (relCount.get(id) ?? 0) + 1)
  const relReq = reqs.find((r) => (relCount.get(r.id) ?? 0) > 1)
  // a requirement with at least two links, for the links tab's bulk removal
  const linkCount = new Map()
  for (const l of items(await api(request, 'GET', `/projects/${pid}/item-links?source_type=requirement`))) linkCount.set(l.source_id, (linkCount.get(l.source_id) ?? 0) + 1)
  const linkReq = reqs.find((r) => (linkCount.get(r.id) ?? 0) > 1)
  const runs = items(await api(request, 'GET', `/projects/${pid}/runs`))
  const run = runs[0]
  const execs = run ? items(await api(request, 'GET', `/runs/${run.id}/executions`)) : []
  // prefer a manual execution with steps for the runner
  const tests = items(await api(request, 'GET', `/projects/${pid}/tests`))
  // a requirement with a similar one (seed: "Password sign-in" and "Password login")
  const simReq = reqs.find((r) => r.title === 'Password sign-in')
  // a requirement with MITRE D3FEND and EMB3D mappings (seed: catalogue EMBS-26)
  const fwReq = reqs.find((r) => (r.d3fend ?? []).length || (r.emb3d ?? []).length)
  const manual = new Set(tests.filter((t) => t.execution_type === 'manual').map((t) => t.id))
  const ex = execs.find((e) => manual.has(e.test_case_id)) ?? execs[0]
  // a run with an execution that is not ready (its readiness cell opens a popover)
  let readinessRun = null
  for (const r of runs) {
    const list = r.id === run?.id ? execs : items(await api(request, 'GET', `/runs/${r.id}/executions`))
    if (list.some((e) => e.readiness && !e.readiness.ready)) {
      readinessRun = r
      break
    }
  }
  const hasTags = items(await api(request, 'GET', `/projects/${pid}/tags`).catch(() => [])).length > 0
  // configurations and a data-driven test (seed-demo: "Cross-browser smoke")
  const crossRun = runs.find((r) => (r.configurations ?? []).length > 0)
  const crossExecs = crossRun ? items(await api(request, 'GET', `/runs/${crossRun.id}/executions`)) : []
  const ddEx = crossExecs.find((e) => e.iteration_count > 0)
  const ddLinks = ddEx ? items(await api(request, 'GET', `/tests/${ddEx.test_case_id}/links`)) : []
  const hasDefects = items(await api(request, 'GET', `/projects/${pid}/defects`)).length > 0
  const hasRisks = items(await api(request, 'GET', `/projects/${pid}/risks`)).length > 0
  const hasReleases = items(await api(request, 'GET', `/projects/${pid}/releases`)).length > 0
  const hasWaivers = items(await api(request, 'GET', `/projects/${pid}/waivers`)).length > 0
  // change requests (seed-demo: one in review by Sara for the administrator, closed, deferred and draft ones)
  const crs = items(await api(request, 'GET', `/projects/${pid}/change-requests`).catch(() => []))
  const crReview = crs.find((c) => c.status === 'in_review')
  // a release gated per variant (seed: 1.5 for both products) and a requirement whose applicability changed
  const gatedRelease = items(await api(request, 'GET', `/projects/${pid}/releases`)).find((r) => (r.variant_ids ?? []).length > 0)
  const applicReq = items(await api(request, 'GET', `/projects/${pid}/requirements?q=applicability:selected`).catch(() => []))[0]
  // exploratory sessions (seed-demo: debriefed, paused and planned ones)
  const xSessions = items(await api(request, 'GET', `/projects/${pid}/exploratory-sessions`).catch(() => []))
  const xDebriefed = xSessions.find((x) => x.status === 'debriefed')
  const xOpen = xSessions.find((x) => x.status === 'paused' || x.status === 'in_progress')
  const hasBaselines = items(await api(request, 'GET', `/projects/${pid}/baselines`)).length > 0
  const firstBaseline = hasBaselines ? items(await api(request, 'GET', `/projects/${pid}/baselines`))[0] : null
  const documents = items(await api(request, 'GET', `/projects/${pid}/documents`))
  const hasDocuments = documents.length > 0
  // a generated document (requirements specification, test plan), shown highlighted with its actions
  const generatedDoc = documents.find((d) => (d.tags ?? []).includes('generated'))
  const hasWebhooks = items(await api(request, 'GET', `/projects/${pid}/webhooks`).catch(() => [])).length > 0
  const hasRequested = items(await api(request, 'GET', '/reviews?requested_by=me').catch(() => [])).length > 0
  const hasToReview = items(await api(request, 'GET', '/reviews?reviewer=me&status=pending').catch(() => [])).length > 0
  const hasNotifications = items(await api(request, 'GET', '/notifications').catch(() => [])).length > 0
  // a requirement that breaks a traceability rule (seed-demo has rules)
  const ruleReport = await api(request, 'GET', `/projects/${pid}/trace-rules/evaluate`).catch(() => null)
  const ruleViolator = (ruleReport?.results ?? []).flatMap((r) => r.violations ?? []).find((v) => v.kind === 'requirement')
  const emailOn = (await api(request, 'GET', '/meta').catch(() => ({}))).email_enabled === true
  const hasDefinitions = items(await api(request, 'GET', `/projects/${pid}/reports/definitions`).catch(() => [])).length > 0
  // the first catalogue requirement, for the preview
  const catalogReq = items(await api(request, 'GET', '/catalog/requirements').catch(() => []))[0]
  // AI assistance: the Assist buttons are shown in every project of an organisation that turned it on
  // (seed-demo); its dialog only opens when a service is configured (ai in the configuration file)
  const aiStatus = await api(request, 'GET', `/projects/${pid}/ai`).catch(() => null)
  const aiOrg = aiStatus?.organisation_enabled === true
  const aiOn = aiStatus?.available === true
  const failedRun = runs.find((r) => (r.summary?.failed ?? 0) + (r.summary?.blocked ?? 0) > 0)
  // web tests (Playwright): a run with background jobs and the execution of a web test
  let jobsRun = null
  for (const r of runs) {
    const j = await api(request, 'GET', `/runs/${r.id}/jobs`).catch(() => null)
    if (j?.counts?.total) {
      jobsRun = r
      break
    }
  }
  const webTest = tests.find((t) => t.script_language === 'playwright')
  const webRun = runs.find((r) => r.name === 'Web smoke')
  const webEx = webRun ? items(await api(request, 'GET', `/runs/${webRun.id}/executions`)).find((e) => e.test_case_id === webTest?.id) : null
  // change and defect correlations: a defect with a regression (seed-demo reopens one), a requirement with linked tests
  let regressedDefect = null
  for (const d of items(await api(request, 'GET', `/projects/${pid}/defects`))) {
    if (items(await api(request, 'GET', `/defects/${d.id}/regressions`).catch(() => [])).length) {
      regressedDefect = d
      break
    }
  }
  const linkedReq = reqs.find((r) => r.linked_tests > 0) ?? reqs[0]
  // the active test cycle (seed-demo: "Sprint 14 regression")
  const cycle = items(await api(request, 'GET', `/projects/${pid}/cycles`).catch(() => [])).find((c) => c.status === 'active')
  // test environments and test data (seed-demo: staging with bookings, a set with a refresh request)
  const envs = items(await api(request, 'GET', `/projects/${pid}/environments`).catch(() => []))
  const stagingEnv = envs.find((e) => e.name === 'staging') ?? envs[0]
  const dataSets = items(await api(request, 'GET', `/projects/${pid}/test-data-sets`).catch(() => []))
  const tableSet = dataSets.find((d) => d.kind === 'table' && d.open_refreshes > 0) ?? dataSets[0]
  const p = `/p/${pid}`
  const routes = [
    { name: 'projects', path: '/projects', wait: '.project-cards .pc-meta' },
    // the table layout: "Started by" column
    { name: 'projects-table', path: '/projects', open: '.page-header .p-selectbutton .p-togglebutton:nth-child(2)', wait: '.p-datatable-tbody tr' },
    // who started the project and when (project settings)
    { name: 'settings-project', path: `${p}/settings?tab=project`, wait: '.project-started-line time' },
    // save / load / duplicate a project (project archives)
    { name: 'project-load-dialog', path: '/projects', open: '.page-header .actions button:has(.pi-upload)', wait: '.p-dialog #project-load-form' },
    { name: 'project-duplicate-dialog', path: '/projects', open: '.project-cards li:first-child button:has(.pi-copy)', wait: '.p-dialog #project-duplicate-form' },
    { name: 'dashboard', path: `${p}/dashboard`, wait: 'canvas' },
    { name: 'dashboard-trends', path: `${p}/dashboard?tab=trends`, wait: 'canvas, .empty' },
    { name: 'dashboard-quality', path: `${p}/dashboard?tab=quality`, wait: 'canvas, .empty' },
    { name: 'dashboard-quality-metrics', path: `${p}/dashboard?tab=quality`, wait: 'section.quality .qkpis' },
    // customisable dashboards: the seeded shared dashboard, edit mode, the add and settings dialogs, the home dashboard
    { name: 'dashboard-shared', path: `${p}/dashboard`, open: '[role="tab"]:has-text("Release health")', wait: '.widget-saved_query table, .widget-saved_query .empty' },
    { name: 'dashboard-edit', path: `${p}/dashboard`, open: 'button[data-test="customise"]', wait: '.dash-grid.editing .handle' },
    { name: 'dashboard-add-widget', path: `${p}/dashboard`, open: ['button[data-test="customise"]', 'button[data-test="add-widget"]'], wait: '.p-dialog .widget-types' },
    { name: 'dashboard-widget-settings', path: `${p}/dashboard`, open: ['button[data-test="customise"]', '.widget-coverage .widget-tools button:has(.pi-cog)'], wait: '.p-dialog form' },
    { name: 'home-dashboard', path: '/home', wait: '.dash-grid .widget' },
    { name: 'requirements', path: `${p}/requirements` },
    { name: 'requirement-editor', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, wait: '.p-drawer' },
    // live updates: someone else is editing, then saves a newer version (presence and the notice)
    reqs[0] && { name: 'requirement-live', path: `${p}/requirements?item=${reqs[0].id}`, live: reqs[0].id, wait: '[data-testid="live-banner"]' },
    { name: 'tests', path: `${p}/tests` },
    // the spreadsheet view: grid, an open cell editor, a refused value, row selection, keyboard help
    { name: 'requirements-sheet', path: `${p}/requirements?view=sheet`, wait: '[role="grid"] [role="gridcell"]' },
    { name: 'tests-sheet', path: `${p}/tests?view=sheet`, wait: '[role="grid"] [role="gridcell"]' },
    { name: 'requirements-sheet-edit', path: `${p}/requirements?view=sheet`, open: '.sheet-body [role="gridcell"][aria-colindex="5"]', keys: ['F2'], wait: '.cell-editor' },
    { name: 'tests-sheet-edit-text', path: `${p}/tests?view=sheet`, open: '.sheet-body [role="gridcell"][aria-colindex="4"]', keys: ['F2'], wait: 'textarea.cell-editor' },
    { name: 'requirements-sheet-error', path: `${p}/requirements?view=sheet`, open: '.sheet-body [role="gridcell"][aria-colindex="3"]', keys: ['F2', 'Control+a', 'Backspace', 'Enter'], wait: '.cell-error' },
    // a row ticked with Space (the pointer then moves away, so nothing is hovered when the toolbar pushes the grid down)
    { name: 'requirements-sheet-bulk', path: `${p}/requirements?view=sheet`, open: '.sheet-body [role="gridcell"][aria-colindex="3"]', keys: [' '], wait: '.bulkbar' },
    { name: 'tests-sheet-keys', path: `${p}/tests?view=sheet`, open: '.sheet-keys summary', wait: '.sheet-keys[open] dl' },
    { name: 'runs', path: `${p}/runs` },
    { name: 'runs-bulk', path: `${p}/runs`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    // open states: dialogs and the off-canvas navigation
    { name: 'run-new-dialog', path: `${p}/runs`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog' },
    { name: 'nav-drawer', path: `${p}/dashboard`, open: 'button[aria-controls="app-sidebar"]', wait: '.sidebar.open', narrow: true },
    run && { name: 'run', path: `${p}/runs/${run.id}` },
    run && { name: 'run-bulk', path: `${p}/runs/${run.id}`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    run && ex && { name: 'execute', path: `${p}/runs/${run.id}/exec/${ex.id}` },
    { name: 'traceability', path: `${p}/traceability` },
    { name: 'trace-rules', path: `${p}/traceability?view=rules`, wait: '.trace-rules .rule-list' },
    { name: 'trace-rules-violations', path: `${p}/traceability?view=rules`, open: '.trace-rules .violations-toggle', wait: '.trace-rules .violations' },
    { name: 'trace-rule-builtin-dialog', path: `${p}/traceability?view=rules`, open: '.trace-rules .rule:has(.pi-shield) button:has(.pi-pencil)', wait: '.p-dialog .trace-rule-form' },
    { name: 'trace-rule-dialog', path: `${p}/traceability?view=rules`, open: '.trace-rules .filterbar button:has(.pi-plus)', wait: '.p-dialog .trace-rule-form' },
    ruleViolator && { name: 'requirement-trace-rules', path: `${p}/requirements?item=${ruleViolator.id}`, wait: '.p-drawer .trace-violations' },
    { name: 'traceability-selected', path: `${p}/traceability`, open: '.trace-bulk .p-checkbox', wait: '.trace-bulk' },
    { name: 'graph', path: `${p}/graph`, wait: '.graph-node, .empty' },
    // test cycles (CyclesView, CycleDetailView and their dialogs)
    { name: 'cycles', path: `${p}/cycles`, wait: '.p-datatable, .empty-state' },
    { name: 'cycles-timeline', path: `${p}/cycles?tab=timeline`, wait: '.timeline, .empty-state' },
    { name: 'cycles-schedules', path: `${p}/cycles?tab=schedules`, wait: '.sched-row, .empty-state' },
    { name: 'cycle-new-dialog', path: `${p}/cycles`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog' },
    { name: 'cycle-schedule-dialog', path: `${p}/cycles?tab=schedules`, open: '.sched-new', wait: '.p-dialog' },
    cycle && { name: 'cycle', path: `${p}/cycles/${cycle.id}`, wait: '.p-datatable' },
    cycle && { name: 'cycle-board', path: `${p}/cycles/${cycle.id}?tab=board`, wait: '.board, .empty-state' },
    cycle && { name: 'cycle-progress', path: `${p}/cycles/${cycle.id}?tab=progress`, wait: 'canvas' },
    cycle && { name: 'cycle-history', path: `${p}/cycles/${cycle.id}?tab=history`, wait: '.card' },
    cycle && { name: 'cycle-add-dialog', path: `${p}/cycles/${cycle.id}`, open: '.filterbar button:has(.pi-plus)', wait: '.p-dialog' },
    cycle && { name: 'cycle-edit-dialog', path: `${p}/cycles/${cycle.id}`, open: '.page-header .actions button:has(.pi-pencil)', wait: '.p-dialog' },
    // variants and code (VariantsView, CodeView, the Code tab and the applicability dialog)
    { name: 'variants', path: `${p}/variants`, wait: '.matrix, .empty-state' },
    { name: 'variants-manage', path: `${p}/variants?tab=manage`, wait: '.variant-list, .empty-state' },
    { name: 'variant-dialog', path: `${p}/variants`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog' },
    { name: 'code', path: `${p}/code`, wait: '#h-regression' },
    { name: 'code-builds', path: `${p}/code?tab=builds`, wait: '#h-builds' },
    { name: 'code-changes', path: `${p}/code?tab=changes`, wait: '#h-changes' },
    { name: 'requirement-code', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-code)', wait: '.code-panel' },
    { name: 'test-code', path: `${p}/tests?item=${tests[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-code)', wait: '.code-panel' },
    { name: 'applicability-dialog', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.p-drawer .applicability button:has(.pi-pencil)', wait: '.p-dialog' },
    { name: 'code-settings', path: `${p}/settings?tab=project`, wait: '#h-code-settings' },
    // CI/CD: the CI setup panel (templates) and the server-side posting to GitLab with its log
    { name: 'ci-setup', path: `${p}/settings?tab=project`, wait: '.ci-setup pre.ci-template' },
    { name: 'ci-posting', path: `${p}/settings?tab=project`, wait: '.ci-posting .ci-deliveries li' },
    // Generate test code: the dialog with the preview of the automated tests' stubs, and with the selection (bulk bar)
    { name: 'test-stubs-dialog', path: `${p}/tests?q=${encodeURIComponent('execution_type:automated')}`, open: 'button.stubs-button', wait: '.stubs-dialog [data-testid="stubs-code"]' },
    { name: 'test-stubs-dialog-selection', path: `${p}/tests`, open: ['.p-datatable-tbody tr:first-child .p-checkbox', '.bulkbar button:has(.pi-code)'], wait: '.stubs-dialog [data-testid="stubs-code"]' },
    { name: 'graph-tests', path: `${p}/graph?kind=test`, wait: '.graph-node, .empty' },
    { name: 'tags', path: `${p}/tags`, wait: '.tag-cloud, .empty' },
    // the requirement and test catalogue: both tabs, a selection, the preview, the import dialog, the areas drawer
    { name: 'catalog', path: `${p}/catalog?lang=en`, wait: '.cat-list li, .list-pane .empty' },
    { name: 'catalog-tests', path: `${p}/catalog?tab=tests&lang=sv`, wait: '.cat-list li, .list-pane .empty' },
    { name: 'catalog-selected', path: `${p}/catalog`, open: '.cat-list li .p-checkbox', wait: '.catalog-bar' },
    catalogReq && { name: 'catalog-preview', path: `${p}/catalog?item=${catalogReq.id}`, wait: '#cp-title' },
    { name: 'catalog-import-dialog', path: `${p}/catalog`, open: ['.cat-list li .p-checkbox', '.catalog-bar button:has(.pi-plus)'], wait: '.p-dialog #catalog-import-form' },
    { name: 'catalog-sources', path: `${p}/catalog?source=gdpr,cra&lang=en`, wait: '.cat-list li, .list-pane .empty' },
    { name: 'catalog-hardware', path: `${p}/catalog?catalogue=hardware&lang=sv`, wait: '.cat-list li, .list-pane .empty' },
    { name: 'catalog-source-picker', path: `${p}/catalog`, open: 'button:has(.pi-verified)', wait: '.p-dialog .src-search' },
    { name: 'catalog-areas-drawer', path: `${p}/catalog`, open: '.tabs-row button:has(.pi-sitemap)', wait: '.catalog-areas-drawer .p-tree', maxWidth: 1199 },
    // catalogue risks: the tab, a risk preview with its mitigating requirements, the import dialog with the risk options
    { name: 'catalog-risks', path: `${p}/catalog?tab=risks&lang=sv`, wait: '.cat-list li, .list-pane .empty' },
    { name: 'catalog-risk-preview', path: `${p}/catalog?tab=risks&item=AUTH-R01&lang=en`, wait: '#cp-title' },
    // MITRE ATT&CK: the risk filters, the coverage page (matrix and table) and a risk dialog with techniques
    { name: 'catalog-risks-attack', path: `${p}/catalog?tab=risks&attack=T1110&lang=en`, wait: '.cat-list li, .list-pane .empty' },
    { name: 'catalog-attack', path: `${p}/catalog/attack`, wait: '.atk-matrix' },
    { name: 'catalog-attack-ics', path: `${p}/catalog/attack?matrix=ics`, wait: '.atk-matrix' },
    // MITRE D3FEND and EMB3D: the framework tabs of the coverage page, previews with techniques, threats and defenses, the filters and the pickers
    { name: 'catalog-frameworks-d3fend', path: `${p}/catalog/frameworks?framework=d3fend`, wait: '.fw-matrix' },
    { name: 'catalog-frameworks-emb3d', path: `${p}/catalog/frameworks?framework=emb3d&lang=sv`, wait: '.emb-threats' },
    { name: 'catalog-preview-frameworks', path: `${p}/catalog?item=HWSEC-01&catalogue=hardware&lang=en`, wait: '#cp-d3fend' },
    { name: 'catalog-risk-preview-defenses', path: `${p}/catalog?tab=risks&item=EMBS-R03&lang=sv`, wait: '#cp-defenses' },
    { name: 'catalog-emb3d-filter', path: `${p}/catalog?emb3d=TID-201&lang=en`, wait: '.cat-list li, .list-pane .empty' },
    fwReq && { name: 'requirement-editor-frameworks', path: `${p}/requirements?item=${fwReq.id}`, wait: '.p-drawer #req-d3fend' },
    hasRisks && { name: 'risk-dialog-emb3d', path: `${p}/risks?q=`, open: '.p-datatable-tbody tr:first-child .link-button', wait: '.p-dialog #risk-emb3d' },
    { name: 'catalog-risk-import-dialog', path: `${p}/catalog?tab=risks`, open: ['.cat-list li .p-checkbox', '.catalog-bar button:has(.pi-plus)'], wait: '.p-dialog #ci-include-reqs' },
    hasTags && { name: 'tags-bulk', path: `${p}/tags`, open: '.tag-cloud .cloud-tag', wait: '.bulkbar' },
    { name: 'flow', path: `${p}/flow?test=${ex?.test_case_id ?? tests[0]?.id ?? ''}`, wait: '.flow-svg, .empty' },
    { name: 'flow-list', path: `${p}/flow?view=list&test=${ex?.test_case_id ?? tests[0]?.id ?? ''}`, wait: '.flow-list, .empty' },
    run && { name: 'flow-run', path: `${p}/flow?run=${run.id}`, wait: '.stages, .empty' },
    { name: 'analytics', path: `${p}/analytics`, wait: '.analytics-cards, .empty' },
    { name: 'analytics-compliance', path: `${p}/analytics`, wait: 'section.compliance' },
    { name: 'analytics-consistency', path: `${p}/analytics`, open: 'section.consistency .toggle', wait: 'section.consistency .violations' },
    { name: 'analytics-quality', path: `${p}/analytics`, wait: 'section.quality .qkpis' },
    { name: 'analytics-quality-table', path: `${p}/analytics`, open: 'section.quality .table-toggle', wait: 'section.quality .data-table' },
    { name: 'test-links', path: `${p}/tests?item=${tests[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-link)', wait: '.item-links' },
    { name: 'link-dialog', path: `${p}/tests?item=${tests[0]?.id ?? ''}`, open: ['.p-drawer [role="tab"]:has(.pi-link)', '.item-links button:has(.pi-link)'], wait: '.link-types' },
    { name: 'new-requirement', path: `${p}/requirements`, open: '.page-header .actions button:has(.pi-plus)', wait: '.obligation-help' },
    { name: 'prompts', path: `${p}/prompts`, wait: '.prompts-grid, .p-message' },
    readinessRun && { name: 'run-readiness', path: `${p}/runs/${readinessRun.id}`, open: 'button.readiness-cell', wait: '.readiness-pop' },
    { name: 'undo-menu', path: `${p}/tests`, open: '.undo-controls button:has(.pi-history)', wait: '.undo-menu' },
    { name: 'columns-menu', path: `${p}/requirements`, open: 'button:has(.pi-table)', wait: '.columns-menu' },
    { name: 'baselines', path: `${p}/baselines` },
    // electronic-records mode (on for these routes only): the signature a
    // baseline needs, the reason a requirement change needs, the signatures of a record
    {
      name: 'erecords-signature-dialog',
      path: `${p}/baselines`,
      erecords: true,
      open: '.page-header .actions button:has(.pi-plus)',
      act: async (page) => {
        await page.locator('#bl-name').fill('A11y signed baseline')
        await page.locator('.p-dialog button[type="submit"]').first().click()
      },
      wait: '.p-dialog #er-sign-meaning',
    },
    reqs[0] && {
      name: 'erecords-reason-dialog',
      path: `${p}/requirements?item=${reqs[0].id}`,
      erecords: true,
      act: async (page) => {
        await page.locator('.p-drawer form input').first().fill(`${reqs[0].title} `)
        await page.locator('.p-drawer button[type="submit"]').first().click()
      },
      wait: '.p-dialog #er-reason',
    },
    firstBaseline && { name: 'baseline-signatures', path: `${p}/baselines?b=${firstBaseline.id}`, wait: '.esigs, .signoffs' },
    { name: 'releases', path: `${p}/releases` },
    { name: 'risks', path: `${p}/risks`, wait: '.heat' },
    { name: 'risks-area', path: `${p}/risks?area=authentication`, wait: '.heat' },
    { name: 'requirement-mitigates-risks', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.requirement-risks button:has(.pi-plus)', wait: '.requirement-risks .add-row' },
    // duplicates and conflicts: the review page (requirements, tests, dismissed), the merge dialog, the similar panel of the editor
    // AI assistance: the Assist menus (requirement editor, run page), the dialog, the project and organisation settings
    aiOrg && { name: 'assist-menu-requirement', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.p-drawer button:has(.pi-sparkles)', wait: '.assist-menu' },
    aiOrg && failedRun && { name: 'assist-menu-run', path: `${p}/runs/${failedRun.id}`, open: '.page-header .actions button:has(.pi-sparkles)', wait: '.assist-menu' },
    aiOn && { name: 'assist-dialog', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: ['.p-drawer button:has(.pi-sparkles)', '.assist-menu .assist-action:not([disabled])'], wait: '.p-dialog .assist .p-message, .p-dialog .assist .assist-card' },
    { name: 'settings-project-ai', path: `${p}/settings?tab=project`, wait: 'h2:has(.pi-sparkles)' },
    { name: 'ai-settings', path: '/admin/ai', wait: '#ai-org-on' },
    { name: 'duplicates', path: `${p}/duplicates`, wait: '.dup-section' },
    { name: 'duplicates-tests', path: `${p}/duplicates?kind=test`, wait: '.dup-section' },
    { name: 'duplicates-dismissed', path: `${p}/duplicates`, open: '.dup-switch .p-toggleswitch', wait: '.dup-section' },
    { name: 'duplicates-merge-dialog', path: `${p}/duplicates`, open: '.pair-actions button:has(.pi-arrow-right-arrow-left)', wait: '.p-dialog .survivor' },
    simReq && { name: 'requirement-similar', path: `${p}/requirements?item=${simReq.id}`, wait: '.similar-panel' },
    // verification correlations: overview tabs, the requirement's verification tab and its dialogs, criterion rows of the matrix
    { name: 'verification', path: `${p}/verification`, wait: '.ver-counts' },
    { name: 'verification-elements', path: `${p}/verification?tab=elements`, wait: '.ver-counts' },
    { name: 'verification-inconsistent', path: `${p}/verification?tab=inconsistent`, wait: '.ver-counts' },
    { name: 'element-dialog', path: `${p}/verification?tab=elements`, open: 'button.add-element', wait: '.p-dialog form' },
    { name: 'requirement-verification', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-verified)', wait: '.verification-panel .ver-summary' },
    { name: 'verification-record-dialog', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: ['.p-drawer [role="tab"]:has(.pi-verified)', '.verification-panel h3 button:has(.pi-plus)'], wait: '.p-dialog form' },
    { name: 'link-criteria-dialog', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: ['.p-drawer [role="tab"]:has(.pi-verified)', '.verification-panel button:has(.pi-sliders-h)'], wait: '.p-dialog .map-row' },
    { name: 'traceability-criteria', path: `${p}/traceability?view=matrix`, open: '#tcrit', wait: 'tr.crit-row' },
    // waivers, residual risk and the Statement of Applicability (seed: an approved waiver, a deviation draft, SoA decisions)
    { name: 'risks-residual', path: `${p}/risks`, open: '.heat-card .p-selectbutton .p-togglebutton:nth-child(2)', wait: '.heat' },
    hasRisks && { name: 'risk-dialog-attack', path: `${p}/risks?q=`, open: '.p-datatable-tbody tr:first-child .link-button', wait: '.p-dialog #risk-attack' },
    hasRisks && { name: 'risk-dialog-residual', path: `${p}/risks`, open: '.p-datatable-tbody tr:first-child .link-button', wait: '.p-dialog fieldset.residual' },
    hasRisks && { name: 'risk-accept-dialog', path: `${p}/risks`, open: ['.p-datatable-tbody tr:first-child .link-button', '.p-dialog fieldset.residual button:has(.pi-pen-to-square)'], wait: '.p-dialog .esign' },
    { name: 'waivers', path: `${p}/waivers`, wait: '.p-datatable-tbody tr' },
    { name: 'waiver-new-dialog', path: `${p}/waivers`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog #wv-title' },
    hasWaivers && { name: 'waiver-detail', path: `${p}/waivers`, open: '.p-datatable-tbody tr:first-child .link-button', wait: '.p-dialog .wv-fields' },
    // change requests: the list, a new one, the detail (diff, impact tree, signatures, timeline), the board's signature, the editor's change control bar, the bulk proposal and the settings
    { name: 'change-requests', path: `${p}/change-requests`, wait: '.p-datatable-tbody tr' },
    { name: 'change-request-new-dialog', path: `${p}/change-requests`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog #cr-title' },
    crReview && { name: 'change-request-detail', path: `${p}/change-requests?cr=${crReview.id}`, wait: '.p-dialog .cr-fields' },
    crReview && { name: 'change-request-sign-dialog', path: `${p}/change-requests?cr=${crReview.id}`, open: '.p-dialog-footer button:has(.pi-pen-to-square)', wait: '.p-dialog .esign' },
    crReview && { name: 'requirement-change-control', path: `${p}/requirements?item=${crReview.items[0]?.entity_id ?? ''}`, wait: '.p-drawer .cc-bar' },
    { name: 'requirements-bulk-propose', path: `${p}/requirements`, open: ['.p-datatable-tbody tr:first-child .p-checkbox input', '.bulkbar button:has(.pi-file-edit)'], wait: '.p-dialog #crb-status' },
    { name: 'settings-change-control', path: `${p}/settings?tab=project`, wait: '#change-control' },
    // exploratory testing: the session list with its metrics, the new-session dialog, the session runner (notes, shortcuts, debrief, defect from a note)
    { name: 'exploratory', path: `${p}/exploratory`, wait: '.p-datatable-tbody tr' },
    { name: 'exploratory-new-dialog', path: `${p}/exploratory`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog #xs-title' },
    xDebriefed && { name: 'exploratory-session', path: `${p}/exploratory/${xDebriefed.id}`, wait: '.xs-fields' },
    xDebriefed && { name: 'exploratory-debrief-dialog', path: `${p}/exploratory/${xDebriefed.id}`, open: 'section[aria-labelledby="xs-debrief-h"] button:has(.pi-check-square)', wait: '.p-dialog #xs-db-sum' },
    xOpen && { name: 'exploratory-session-runner', path: `${p}/exploratory/${xOpen.id}`, open: 'button[aria-controls="xs-keys"]', wait: '#xs-keys dl' },
    xOpen && { name: 'exploratory-defect-dialog', path: `${p}/exploratory/${xOpen.id}`, open: '.xs-notes li:first-child button:has(.pi-exclamation-circle)', wait: '.p-dialog #xs-d-title' },
    // test environments: status board, list, booking calendar, the environment, its dialogs; test data sets
    { name: 'environments', path: `${p}/environments`, wait: '.env-board, .empty-state' },
    { name: 'environments-list', path: `${p}/environments?tab=list`, wait: '.p-datatable' },
    { name: 'environments-calendar', path: `${p}/environments?tab=calendar`, wait: '.tl' },
    { name: 'environment-new-dialog', path: `${p}/environments`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog .env-fieldset' },
    { name: 'environment-book-dialog', path: `${p}/environments`, open: '.page-header .actions button:has(.pi-calendar-plus)', wait: '.p-dialog input[type="datetime-local"]' },
    stagingEnv && { name: 'environment', path: `${p}/environments/${stagingEnv.id}`, wait: '.env-grid' },
    stagingEnv && { name: 'environment-edit-dialog', path: `${p}/environments/${stagingEnv.id}`, open: '.page-header .actions button:has(.pi-pencil)', wait: '.p-dialog .env-fieldset' },
    stagingEnv && { name: 'environment-booking-dialog', path: `${p}/environments/${stagingEnv.id}`, open: '.tl-bar', wait: '.p-dialog input[type="datetime-local"]' },
    { name: 'run-new-environment', path: `${p}/runs`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog .env-field' },
    { name: 'test-data', path: `${p}/test-data`, wait: '.p-datatable-tbody tr' },
    tableSet && { name: 'test-data-set', path: `${p}/test-data?set=${tableSet.id}`, wait: '.p-dialog .td-table' },
    tableSet && { name: 'test-data-refresh', path: `${p}/test-data?set=${tableSet.id}`, open: '.p-dialog [role="tab"]:nth-child(4)', wait: '.p-dialog textarea' },
    { name: 'soa', path: `${p}/soa`, wait: '[role="grid"] [role="gridcell"]' },
    { name: 'soa-control', path: `${p}/soa`, open: '.sheet-body [role="gridcell"][aria-colindex="3"]', keys: ['Control+Enter'], wait: '.p-dialog' },
    hasReleases && { name: 'release-waivers', path: `${p}/releases`, open: '.p-datatable-tbody tr:first-child .link-button', wait: '.rel-waivers' },
    // release gates per variant: the criteria × variants matrix, the target variants of the release dialog
    gatedRelease && { name: 'release-gates', path: `${p}/releases?r=${gatedRelease.id}&tab=status`, wait: '.gates .gate-table' },
    gatedRelease && { name: 'release-variants-dialog', path: `${p}/releases?r=${gatedRelease.id}&tab=status`, open: '#release-detail button:has(.pi-pencil)', wait: '.p-dialog #rel-variants' },
    // the history of an applicability change (diff of the variants)
    applicReq && { name: 'applicability-history', path: `${p}/requirements?item=${applicReq.id}`, open: '.p-drawer [role="tab"]:has(.pi-history)', wait: '.history .rev-list' },
    { name: 'defects', path: `${p}/defects` },
    // with a selected row (bulk toolbar); only when the project has defects / risks
    hasDefects && { name: 'defects-bulk', path: `${p}/defects`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    hasNotifications && { name: 'notifications-bulk', path: '/notifications', open: '.list li .p-checkbox', wait: '.bulkbar' },
    hasRequested && { name: 'my-work-reviews-bulk', path: '/my-work?view=reviews&side=requested', open: '.review-requests .p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    hasWebhooks && { name: 'webhooks-bulk', path: `${p}/settings?tab=project`, open: 'section.webhooks .p-datatable-tbody .p-checkbox', wait: 'section.webhooks .bulkbar' },
    // chat notifications: the channel list (seeded), the new-channel dialog, editing a seeded channel, its delivery log
    { name: 'chat-channels', path: `${p}/settings?tab=project`, wait: 'section.chats .p-datatable-tbody tr' },
    { name: 'chat-channel-dialog', path: `${p}/settings?tab=project`, open: 'section.chats .head button:has(.pi-plus)', wait: '.p-dialog #ch-name' },
    { name: 'chat-channel-edit-dialog', path: `${p}/settings?tab=project`, open: 'section.chats .p-datatable-tbody tr:first-child button:has(.pi-pencil)', wait: '.p-dialog #ch-name' },
    { name: 'chat-delivery-log', path: `${p}/settings?tab=project`, open: 'section.chats .p-datatable-tbody tr:first-child button:has(.pi-list)', wait: '.p-dialog .p-datatable' },
    hasDocuments && { name: 'documents-bulk', path: `${p}/documents`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    // the comparison with restore checkboxes (needs items changed since the baseline)
    firstBaseline && { name: 'baseline-compare', path: `${p}/baselines?b=${firstBaseline.id}&tab=compare`, wait: '.bl-section, [role="status"]' },
    hasBaselines && { name: 'baselines-bulk', path: `${p}/baselines`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    hasReleases && { name: 'releases-bulk', path: `${p}/releases`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    hasRisks && { name: 'risks-bulk', path: `${p}/risks`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    { name: 'defect-dialog', path: `${p}/defects`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog' },
    { name: 'requirement-defects', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-exclamation-circle)', wait: '.p-drawer [role="tabpanel"]' },
    { name: 'settings', path: `${p}/settings` },
    { name: 'configurations', path: `${p}/settings?tab=configurations`, wait: '#h-configs' },
    { name: 'web-tests-settings', path: `${p}/settings?tab=webtests`, wait: '#h-web-tests' },
    // change and defect correlations: settings, the Impact tabs, the confirmation before saving, a baseline's impact, regression history
    { name: 'correlation-settings', path: `${p}/settings?tab=correlations`, wait: '#h-corr' },
    // the logo on reports and documents: organisation (Administration → Documents) and project, with the first-page preview
    { name: 'logo-settings', path: '/admin/documents', wait: '.logo-card .logo-page img' },
    { name: 'logo-settings-project', path: `${p}/settings?tab=project`, wait: '.logo-card .logo-page' },
    // the variant filter of the verification view and the analytics
    { name: 'verification-variant', path: `${p}/verification?variant=EU`, wait: '.ver-counts' },
    { name: 'analytics-variant', path: `${p}/analytics?variant=EU`, wait: '.analytics-cards, .empty' },
    // the risk register import
    { name: 'risk-import-dialog', path: `${p}/risks`, open: '.page-header .actions button:has(.pi-file-import)', wait: '.p-dialog' },
    linkedReq && { name: 'requirement-impact', path: `${p}/requirements?item=${linkedReq.id}`, open: '.p-drawer [role="tab"]:has(.pi-share-alt)', wait: '.impact-panel .impact-list' },
    { name: 'test-impact', path: `${p}/tests?item=${tests[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-share-alt)', wait: '.impact-panel .impact-list' },
    linkedReq && { name: 'impact-confirm-dialog', path: `${p}/requirements?item=${linkedReq.id}`, open: '.p-drawer form input[required]', keys: ['End', 'X', 'Enter'], wait: '.impact-confirm-dialog' },
    // transitive impact analysis: graph and table of a requirement's impact, the impact since a baseline
    linkedReq && { name: 'impact-analysis', path: `${p}/impact?items=requirement:${linkedReq.id}`, wait: '.impact-node, .impact-view section.card + section.card' },
    linkedReq && { name: 'impact-analysis-table', path: `${p}/impact?items=requirement:${linkedReq.id}`, open: '.impact-view .p-selectbutton .p-togglebutton:nth-child(2)', wait: '.impact-table, .impact-view p.muted' },
    firstBaseline && { name: 'impact-analysis-baseline', path: `${p}/impact?baseline=${firstBaseline.id}`, wait: '.impact-view h2' },
    firstBaseline && { name: 'baseline-impact-dialog', path: `${p}/baselines?b=${firstBaseline.id}&tab=compare`, open: '.filterbar button:has(.pi-share-alt)', wait: '.baseline-impact-dialog .row' },
    regressedDefect && { name: 'defect-regressions', path: `${p}/defects?item=${regressedDefect.id}`, wait: '#defect-regressions-h' },
    // discussions (seed-demo: threads on requirements, a defect and the regression run): the project's list,
    // the Discussion tab with a reply box and the reactions menu, a defect's and a run's discussion
    { name: 'discussions', path: `${p}/discussions`, wait: '.p-datatable-tbody tr' },
    { name: 'discussions-all', path: `${p}/discussions?status=all&filter=mine`, wait: '.p-datatable' },
    { name: 'requirement-discussion', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}&tab=comments`, wait: '.p-drawer .discussion .thread' },
    { name: 'discussion-reply', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}&tab=comments`, open: '.p-drawer .discussion .thread-actions button:has(.pi-reply)', wait: '.reply-box' },
    { name: 'discussion-reactions', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}&tab=comments`, open: '.p-drawer .discussion .comment button:has(.pi-face-smile)', wait: '.p-menu' },
    regressedDefect && { name: 'defect-discussion', path: `${p}/defects?item=${regressedDefect.id}`, wait: '.p-dialog .discussion' },
    run && { name: 'run-discussion', path: `${p}/runs/${run.id}?tab=discussion`, wait: '.discussion' },
    // custom fields and item templates: the settings tabs, their dialogs, the picker of a new item, the field sets
    { name: 'custom-fields-settings', path: `${p}/settings?tab=customFields`, wait: '.cf-settings .cf-table' },
    { name: 'custom-field-dialog', path: `${p}/settings?tab=customFields`, open: '.cf-settings > .row button:has(.pi-plus)', wait: '.p-dialog .cf-form' },
    { name: 'templates-settings', path: `${p}/settings?tab=templates`, wait: '.tpl-settings .tpl-list' },
    { name: 'template-dialog', path: `${p}/settings?tab=templates`, open: '.tpl-settings > .row button:has(.pi-plus)', wait: '.p-dialog .tpl-form' },
    { name: 'new-requirement-template', path: `${p}/requirements`, open: '.page-header .actions button:has(.pi-plus)', wait: '.template-picker' },
    { name: 'field-sets', path: '/admin/field-sets', wait: '.cf-settings' },
    webTest && { name: 'web-test-editor', path: `${p}/tests?item=${webTest.id}`, wait: '.p-drawer' },
    webRun && webEx && { name: 'execute-web', path: `${p}/runs/${webRun.id}/exec/${webEx.id}`, wait: '.web-evidence' },
    jobsRun && { name: 'run-jobs', path: `${p}/runs/${jobsRun.id}`, open: '.run-jobs button[aria-controls="run-jobs-list"]', wait: '#run-jobs-list' },
    { name: 'configurations-bulk', path: `${p}/settings?tab=configurations`, open: 'section[aria-labelledby="h-configs"] .p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    { name: 'configuration-dialog', path: `${p}/settings?tab=configurations`, open: 'section[aria-labelledby="h-configs"] button:has(.pi-plus)', wait: '.p-dialog #cfg-form' },
    { name: 'configurations-generate', path: `${p}/settings?tab=configurations`, open: 'section[aria-labelledby="h-configs"] button:has(.pi-th-large)', wait: '.p-dialog .preview' },
    crossRun && { name: 'run-configurations', path: `${p}/runs/${crossRun.id}`, wait: '#h-per-config' },
    crossRun && { name: 'run-edit-configurations', path: `${p}/runs/${crossRun.id}`, open: '.page-header .actions button:has(.pi-pencil)', wait: '.p-dialog #run-edit-configs' },
    crossRun && ddEx && { name: 'execute-iterations', path: `${p}/runs/${crossRun.id}/exec/${ddEx.id}`, wait: '.iter-list' },
    ddEx && { name: 'test-data', path: `${p}/tests?item=${ddEx.test_case_id}`, wait: '.data-table' },
    ddEx && { name: 'test-data-paste', path: `${p}/tests?item=${ddEx.test_case_id}`, open: '.p-drawer button:has(.pi-clipboard)', wait: '.p-dialog #td-paste' },
    ddLinks.length > 0 && { name: 'requirement-coverage', path: `${p}/requirements?item=${ddLinks[0].requirement_id}`, wait: '.cov-table' },
    { name: 'settings-project', path: `${p}/settings?tab=project`, wait: '#h-tracker' },
    // project sharing: the Share dialog, the Sharing tab and its bulk toolbar
    { name: 'project-share-dialog', path: `${p}/dashboard`, open: 'button.project-share', wait: '.p-dialog .sharing-panel .members-table' },
    { name: 'settings-sharing', path: `/p/${vault.id}/settings?tab=sharing`, wait: '.sharing-panel .members-table' },
    { name: 'sharing-members-bulk', path: `/p/${vault.id}/settings?tab=sharing`, open: '.sharing-panel .members-table .p-datatable-tbody .p-checkbox', wait: '.sharing-panel .bulkbar' },
    { name: 'settings-project-load-dialog', path: `${p}/settings?tab=project`, open: 'section.project-archive button:has(.pi-upload)', wait: '.p-dialog #project-load-form' },
    // the folder tree is a side panel from 1200 px (a drawer below)
    { name: 'folders-bulk', path: `${p}/requirements`, open: ['button.p-button:has(.pi-check-square)', '.p-tree .p-checkbox'], wait: '.folder-bulk', minWidth: 1200 },
    { name: 'folders-bulk-drawer', path: `${p}/requirements`, open: ['.page-header .actions button:has(.pi-folder)', '.p-drawer button.p-button:has(.pi-check-square)', '.p-drawer .p-tree .p-checkbox'], wait: '.p-drawer .folder-bulk', maxWidth: 1199 },
    { name: 'categories-bulk', path: `${p}/settings?tab=requirement`, open: '.category-grid .p-checkbox', wait: '.bulkbar' },
    { name: 'export', path: `${p}/export` },
    // exchanging data: the Import menu, the ReqIF import with its preview (a DOORS-shaped fixture), the Excel round-trip import
    { name: 'exchange-menu', path: `${p}/requirements`, open: 'button.exchange-menu', wait: '#exchange-menu' },
    {
      name: 'exchange-reqif-preview',
      path: `${p}/requirements`,
      open: ['button.exchange-menu', '#exchange-menu .p-menu-item:nth-child(2) .p-menu-item-link'],
      upload: ['.exchange-dialog input[type="file"]', 'internal/interop/testdata/doors.reqif'],
      wait: '.exchange-dialog .ex-table',
    },
    // a DOORS Next–shaped document with test cases: object and relation type mappings, the tests' diff
    {
      name: 'exchange-reqif-tests',
      path: `${p}/requirements`,
      open: ['button.exchange-menu', '#exchange-menu .p-menu-item:nth-child(2) .p-menu-item-link'],
      upload: ['.exchange-dialog input[type="file"]', 'internal/interop/testdata/dng-tests.reqif'],
      wait: '.exchange-dialog .ex-table',
    },
    // the OSLC delegated dialogs (server-rendered pages other tools show)
    { name: 'oslc-dialog-select', path: `/api/v1/oslc/projects/${pid}/dialogs/requirement/select`, wait: '#list li' },
    { name: 'oslc-dialog-create', path: `/api/v1/oslc/projects/${pid}/dialogs/test/create?lang=sv`, wait: '#create:not([hidden])' },
    { name: 'exchange-xlsx-dialog', path: `${p}/tests`, open: ['button.exchange-menu', '#exchange-menu .p-menu-item:last-child .p-menu-item-link'], wait: '.exchange-dialog' },
    // importing from Jira and Azure DevOps: the query forms (the demo has both sources, without tokens) and the settings card
    { name: 'exchange-jira-dialog', path: `${p}/requirements`, open: ['button.exchange-menu', '#exchange-menu .p-menu-item:nth-child(3) .p-menu-item-link'], wait: '.exchange-dialog form textarea' },
    { name: 'exchange-azure-dialog', path: `${p}/requirements`, open: ['button.exchange-menu', '#exchange-menu .p-menu-item:nth-child(4) .p-menu-item-link'], wait: '.exchange-dialog form textarea' },
    { name: 'import-sources-settings', path: `${p}/settings?tab=project`, wait: '#h-import-sources + p + form' },
    // two-way sync with Jira and Azure DevOps (seed-demo: both configured without tokens, an open conflict, a log):
    // the overview, the conflict view with its field diff, the log with a run's entries, the settings with the field mapping
    { name: 'tracker-sync', path: `${p}/tracker-sync`, wait: '.sync-source dl' },
    { name: 'tracker-sync-conflicts', path: `${p}/tracker-sync?tab=conflicts`, wait: '.sync-conflict table.sync-diff' },
    { name: 'tracker-sync-log', path: `${p}/tracker-sync?tab=log`, open: '.sync-runs tbody tr:last-child button', wait: '.sync-runs .sync-entries' },
    { name: 'tracker-sync-settings', path: `${p}/tracker-sync?tab=settings&source=azure-devops`, wait: '.sync-form .sync-fields .value-maps' },
    // ReqIF selected: the aspects are disabled and a hint is shown
    { name: 'export-reqif', path: `${p}/export`, open: '.format-grid button:has(.pi-sitemap)', wait: 'fieldset.plain-fieldset[disabled]' },
    { name: 'documents', path: `${p}/documents` },
    // document generators: the "Create document" menu, the dialog for both kinds, a new document highlighted
    { name: 'documents-generate-menu', path: `${p}/documents`, open: 'button.doc-generate', wait: '#doc-generate-menu' },
    { name: 'documents-generate-spec', path: `${p}/documents?create=requirements_spec`, wait: '.generate-doc-dialog form' },
    { name: 'documents-generate-plan', path: `${p}/documents?create=test_plan`, wait: '.generate-doc-dialog form' },
    generatedDoc && { name: 'documents-generated', path: `${p}/documents?file=${generatedDoc.id}`, wait: '.created-banner' },
    { name: 'reports', path: `${p}/reports` },
    emailOn && { name: 'reports-email', path: `${p}/reports`, open: '.report-card button:has(.pi-envelope)', wait: '.report-mail-dialog' },
    emailOn && { name: 'reports-subscriptions', path: `${p}/reports`, wait: 'section.subscriptions' },
    emailOn && hasDefinitions && { name: 'reports-subscription-dialog', path: `${p}/reports`, open: 'section.subscriptions button:has(.pi-calendar-plus)', wait: '.subscription-dialog' },
    { name: 'reports-batch', path: `${p}/reports`, open: 'section:has(#h-batch) .p-datatable-tbody .p-checkbox', wait: 'section:has(#h-batch) .bulkbar' },
    { name: 'requirement-attachments', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-paperclip)', wait: '.dropzone' },
    run && { name: 'run-attachments', path: `${p}/runs/${run.id}`, open: '[role="tab"]:has(.pi-paperclip)', wait: '.dropzone' },
    { name: 'search', path: '/search?q=login' },
    // the autocomplete list of a text field (recent searches, seeded below): ArrowDown in the empty field
    { name: 'search-suggestions', path: '/search', press: ['.query-input input', 'ArrowDown'], wait: '.suggest-list' },
    { name: 'help', path: '/help' },
    // the concept pages of the guide: the glossary (long, anchored entries), how the tool works (with the item diagram) and the FAQ
    { name: 'help-glossary', path: '/help/glossary' },
    { name: 'help-how-it-works', path: '/help/how-it-works' },
    { name: 'help-faq', path: '/help/faq' },
    { name: 'profile', path: '/profile' },
    { name: 'relations-bulk', path: `${p}/requirements`, open: ['.p-datatable-tbody tr .p-checkbox', '.bulkbar button:has(.pi-sitemap)'], wait: '.p-dialog [role="status"]' },
    relReq && { name: 'requirement-relations', path: `${p}/requirements?item=${relReq.id}`, open: ['.p-drawer [role="tab"]:has(.pi-sitemap)', '.p-drawer [role="tabpanel"] .p-checkbox'], wait: '.p-drawer [role="tabpanel"] .p-button-danger' },
    { name: 'item-links-bulk', path: `${p}/requirements`, open: ['.p-datatable-tbody tr .p-checkbox', '.bulkbar button:has(.pi-link)'], wait: '.p-dialog [role="status"]' },
    // the issues of the selected requirements in the project's tracker (create, update, refresh)
    { name: 'requirement-issues-bulk', path: `${p}/requirements`, open: ['.p-datatable-tbody tr .p-checkbox', '.bulkbar button:has(.pi-ticket)'], wait: '.p-dialog .p-message, .p-dialog input#bulk-issue-update' },
    { name: 'run-links-bulk', path: `${p}/runs`, open: ['.p-datatable-tbody tr .p-checkbox', '.bulkbar button:has(.pi-link)'], wait: '.p-dialog [role="status"]' },
    linkReq && { name: 'requirement-links', path: `${p}/requirements?item=${linkReq.id}`, open: ['.p-drawer [role="tab"]:has(.pi-link)', '.item-links .p-checkbox'], wait: '.item-links .p-button-danger' },
    { name: 'bulk-review', path: `${p}/requirements`, open: '.page-header .actions button:has(.pi-verified)', wait: '.p-dialog .preview' },
    { name: 'notifications', path: '/notifications' },
    // the reviews view of My work (what was "My reviews"): to review, requested by me, in review status; a review opened from a row
    { name: 'my-work-reviews', path: '/my-work?view=reviews', wait: '.review-requests .p-datatable' },
    { name: 'my-work-reviews-all', path: '/my-work?view=reviews&status=all', wait: '.review-requests .p-datatable' },
    { name: 'my-work-reviews-in-review', path: '/my-work?view=reviews&side=in_review', wait: 'section.in-review' },
    hasToReview && { name: 'my-work-review-dialog', path: '/my-work', open: '.p-datatable-tbody .row-review', wait: '.p-dialog .checklist' },
    // the personal work list (My work) across projects, filtered by project and reason; the dashboard widget
    { name: 'my-work', path: '/my-work', wait: '.p-datatable-tbody tr' },
    { name: 'my-work-filtered', path: `/my-work?project=${pid}&reason=overdue`, wait: '.p-datatable' },
    { name: 'dashboard-my-work', path: `${p}/dashboard`, wait: 'section.my-work .count' },
    // bulk "Set owner" of the selected tests
    { name: 'owner-bulk', path: `${p}/tests`, open: ['.p-datatable-tbody tr .p-checkbox', '.bulkbar button:has(.pi-user)'], wait: '.p-dialog #bulk-owner' },
    { name: 'members', path: '/admin/members' },
    { name: 'members-add', path: '/admin/members', open: '.filterbar button:has(.pi-user-plus)', wait: '.p-dialog' },
    // public invitation page (an unknown token shows the explanation)
    { name: 'invite', path: '/invite/not-a-valid-token', anon: true, wait: '.p-message' },
    { name: 'members-bulk', path: '/admin/members', open: '.p-datatable-tbody tr .p-checkbox', wait: '.bulkbar' },
    { name: 'rbac', path: '/admin/roles' },
    { name: 'execution-settings', path: '/admin/execution', wait: '.setting' },
    // multi-factor authentication: policy page, set-up dialog in the profile, the sign-in step
    { name: 'mfa-policy', path: '/admin/mfa', wait: '#h-mfa-server' },
    { name: 'mfa-policy-roles', path: '/admin/mfa', open: 'label[for="mfa-t-mode-roles"]', wait: 'section[aria-labelledby="h-mfa-tenant"] .p-multiselect' },
    { name: 'profile-mfa-setup', path: '/profile', open: '#mfa button:has(.pi-mobile)', wait: '.p-dialog svg.qr' },
    { name: 'login-mfa', path: '/login?mfa=1', anon: true, mfa: true, wait: '#mfa-code' },
    { name: 'login-mfa-recovery', path: '/login?mfa=1', anon: true, mfa: true, open: '.mfa-login button.link-button', wait: '#mfa-code.recovery' },
    { name: 'backup', path: '/admin/backup' },
    { name: 'tokens', path: '/admin/tokens' },
    { name: 'tokens-create', path: '/admin/tokens', open: 'button:has(.pi-plus)', wait: '.p-dialog .role-picker' },
    { name: 'provisioning', path: '/admin/provisioning', wait: '#h-scim-events' },
    { name: 'provisioning-token-dialog', path: '/admin/provisioning', open: 'section:has(#h-scim-tokens) .filterbar button:has(.pi-plus)', wait: '.p-dialog' },
    { name: 'audit', path: '/admin/audit' },
    // the tamper-evident audit trail and electronic-records mode
    { name: 'audit-verify', path: '/admin/audit', open: '.chain-card button:has(.pi-verified)', wait: '.chain-result' },
    { name: 'erecords', path: '/admin/erecords', wait: '.er-grid' },
    { name: 'erecords-report', path: '/admin/erecords', open: 'button:has(.pi-list-check)', wait: '.er-table' },
    { name: 'tenants', path: '/admin/tenants' },
    { name: 'users', path: '/admin/users' },
    // system settings (super admins): every section, the OIDC provider dialog
    { name: 'system-settings', path: '/admin/system', wait: '.sys-field' },
    { name: 'system-settings-security', path: '/admin/system?section=security', wait: '.sys-field' },
    { name: 'system-settings-similarity', path: '/admin/system?section=similarity', wait: '.sys-field' },
    { name: 'system-settings-ai', path: '/admin/system?section=ai', wait: '.sys-field' },
    { name: 'system-settings-oidc', path: '/admin/system?section=oidc', wait: '.sys-section .row' },
    { name: 'system-settings-oidc-dialog', path: '/admin/system?section=oidc', open: '.sys-section button:has(.pi-plus)', wait: '.p-dialog #oidc-provider-form' },
    { name: 'system-settings-logging', path: '/admin/system?section=logging', wait: '.sys-field' },
    { name: 'system-settings-email', path: '/admin/system?section=email', wait: '.sys-field' },
    { name: 'system-settings-runner', path: '/admin/system?section=runner', wait: '.sys-field' },
    { name: 'system-settings-retention', path: '/admin/system?section=retention', wait: '.info-list' },
  ].filter(Boolean)
  return routes.filter((r) => !ONLY.length || ONLY.includes(r.name))
}

async function settle(page, wait) {
  await page.waitForLoadState('networkidle').catch(() => undefined)
  if (wait) await page.waitForSelector(wait, { timeout: 8000 }).catch(() => undefined)
  // no busy indicators left
  await page.waitForFunction(() => !document.querySelector('.p-datatable-mask, .p-skeleton, [aria-busy="true"]'), null, { timeout: 8000 }).catch(() => undefined)
  await page.waitForTimeout(250)
}

/** Focus management: skip link, route focus, drawer focus trap and return, runner shortcuts. */
async function keyboardChecks(browser, state, routes) {
  const out = []
  const check = (name, ok, detail = '') => out.push({ name, ok: !!ok, detail })
  const route = (n) => routes.find((r) => r.name === n)
  const active = (page) =>
    page.evaluate(() => {
      const a = document.activeElement
      return a ? { tag: a.tagName, id: a.id, text: (a.textContent || '').trim().slice(0, 60), label: a.getAttribute('aria-label'), cls: String(a.className || '') } : null
    })
  // desktop
  let ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, storageState: state, reducedMotion: 'reduce' })
  let page = await ctx.newPage()
  try {
    await page.goto(BASE + route('projects').path)
    await settle(page)
    await page.keyboard.press('Tab')
    const first = await active(page)
    check('skip link is the first focusable element', first?.cls.includes('skip-link'), JSON.stringify(first))
    await page.keyboard.press('Enter')
    const afterSkip = await active(page)
    check('skip link moves focus to the page heading', afterSkip?.tag === 'H1' || afterSkip?.id === 'main-content', JSON.stringify(afterSkip))
    // route change: focus goes to the new page's h1 and the title is announced
    await page.click('nav[aria-label] a[href$="/search"]')
    await page.waitForTimeout(900)
    const afterNav = await active(page)
    check('route change focuses the page heading', afterNav?.tag === 'H1', JSON.stringify(afterNav))
    const live = await page.evaluate(() => document.querySelector('#route-announcer')?.textContent?.trim())
    check('route change is announced', live && live.length > 0, String(live))
    // editor drawer: open with the keyboard, focus stays inside, Escape returns focus
    if (route('requirements')) {
      await page.goto(BASE + route('requirements').path)
      await settle(page)
      const opener = page.locator('button.item-title').first()
      await opener.focus()
      await page.keyboard.press('Enter')
      await page.waitForSelector('.p-drawer', { timeout: 5000 })
      await page.waitForTimeout(400)
      let inside = true
      for (let i = 0; i < 40; i++) {
        await page.keyboard.press('Tab')
        if (!(await page.evaluate(() => !!document.activeElement?.closest('.p-drawer')))) inside = false
      }
      check('editor drawer traps focus', inside)
      // (PrimeVue's Select swallows Escape even when closed, so close from a text field)
      await page.locator('.p-drawer input[type="text"], .p-drawer input:not([type])').first().focus()
      await page.keyboard.press('Escape')
      await page.waitForTimeout(500)
      const back = await active(page)
      check('closing the editor drawer returns focus to its opener', back?.cls.includes('item-title'), JSON.stringify(back))
    }
    // document generator: menu and dialog with the keyboard, focus stays inside, Escape returns focus
    if (route('documents-generate-menu')) {
      await page.goto(BASE + route('documents-generate-menu').path)
      await settle(page)
      await page.locator('button.doc-generate').focus()
      await page.keyboard.press('Enter')
      await page.waitForSelector('#doc-generate-menu', { timeout: 5000 })
      await page.keyboard.press('ArrowDown')
      await page.keyboard.press('Enter')
      await page.waitForSelector('.generate-doc-dialog form', { timeout: 5000 })
      await page.waitForTimeout(400)
      check('the document generator opens from its menu with the keyboard', await page.evaluate(() => !!document.activeElement?.closest('.generate-doc-dialog')))
      let inside = true
      for (let i = 0; i < 45; i++) {
        await page.keyboard.press('Tab')
        if (!(await page.evaluate(() => !!document.activeElement?.closest('.generate-doc-dialog')))) inside = false
      }
      check('the document generator dialog traps focus', inside)
      await page.locator('.generate-doc-dialog input[type="text"]').first().focus()
      await page.keyboard.press('Escape')
      await page.waitForTimeout(500)
      check('Escape closes the document generator', !(await page.locator('.generate-doc-dialog').isVisible()))
    }
    // spreadsheet: the grid keeps focus, arrows move the active cell, F2 edits, Escape returns to the grid
    if (route('requirements-sheet')) {
      await page.goto(BASE + route('requirements-sheet').path)
      await settle(page, route('requirements-sheet').wait)
      const grid = page.locator('[role="grid"]').first()
      await grid.focus()
      const first = await grid.getAttribute('aria-activedescendant')
      await page.keyboard.press('ArrowDown')
      await page.keyboard.press('ArrowRight')
      const moved = await grid.getAttribute('aria-activedescendant')
      check('spreadsheet arrows move the active cell', first && moved && first !== moved && /-r1-c1$/.test(moved), `${first} → ${moved}`)
      await page.keyboard.press('F2')
      const editor = await active(page)
      check('spreadsheet F2 opens the cell editor', editor?.cls.includes('cell-editor'), JSON.stringify(editor))
      await page.keyboard.press('Escape')
      const back = await page.evaluate(() => document.activeElement?.getAttribute('role'))
      check('spreadsheet Escape returns focus to the grid', back === 'grid', String(back))
      await page.keyboard.press('Control+End')
      const end = await grid.getAttribute('aria-activedescendant')
      const rows = Number(await grid.getAttribute('aria-rowcount'))
      check('spreadsheet Ctrl+End goes to the last cell', end && new RegExp(`-r${rows - 2}-c\\d+$`).test(end), `${end} of ${rows}`)
    }
    // runner: shortcuts do not fire while typing
    if (route('execute')) {
      await page.goto(BASE + route('execute').path)
      await settle(page)
      const ta = page.locator('.runner textarea').first()
      if (await ta.count()) {
        const before = await page.locator('.step-list button.active').textContent()
        await ta.focus()
        await page.keyboard.type('pfbs')
        const value = await ta.inputValue()
        const after = await page.locator('.step-list button.active').textContent()
        check('runner shortcuts do not fire in text fields', value.endsWith('pfbs') && before === after, `${value} / ${before} → ${after}`)
        await ta.fill('')
        await page.locator('h1').first().click()
        await page.keyboard.press('?')
        check('runner keyboard help opens with ?', await page.locator('#runner-keys').isVisible())
      }
    }
  } catch (e) {
    check('desktop keyboard checks ran', false, e.message)
  } finally {
    await ctx.close()
  }
  // phone: off-canvas navigation
  ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, storageState: state, reducedMotion: 'reduce' })
  page = await ctx.newPage()
  try {
    await page.goto(BASE + route('dashboard').path)
    await settle(page)
    const hidden = await page.evaluate(() => {
      const s = document.getElementById('app-sidebar')
      return !!s && (s.inert || getComputedStyle(s).visibility === 'hidden')
    })
    check('closed navigation drawer is out of the tab order', hidden)
    const menu = page.locator('button[aria-controls="app-sidebar"]')
    await menu.focus()
    await page.keyboard.press('Enter')
    await page.waitForTimeout(400)
    const inNav = await page.evaluate(() => !!document.activeElement?.closest('#app-sidebar'))
    check('opening the navigation drawer moves focus into it', inNav)
    let inside = true
    for (let i = 0; i < 25; i++) {
      await page.keyboard.press('Tab')
      if (!(await page.evaluate(() => !!document.activeElement?.closest('#app-sidebar')))) inside = false
    }
    check('navigation drawer traps focus', inside)
    await page.keyboard.press('Escape')
    await page.waitForTimeout(400)
    const back = await page.evaluate(() => document.activeElement?.getAttribute('aria-controls'))
    check('Escape closes the navigation drawer and returns focus', back === 'app-sidebar', String(back))
  } catch (e) {
    check('phone keyboard checks ran', false, e.message)
  } finally {
    await ctx.close()
  }
  return out
}

async function overflow(page) {
  return page.evaluate(() => {
    const doc = document.documentElement
    const res = { scrollWidth: doc.scrollWidth, innerWidth: window.innerWidth, offenders: [] }
    if (doc.scrollWidth > window.innerWidth) {
      // the widest elements reaching past the viewport, to help fixing
      const out = []
      for (const el of document.querySelectorAll('body *')) {
        const r = el.getBoundingClientRect()
        if (r.right > window.innerWidth + 1 && r.width > 0) {
          const cls = typeof el.className === 'string' ? el.className.trim().split(/\s+/).slice(0, 3).join('.') : ''
          out.push(`${el.tagName.toLowerCase()}${el.id ? '#' + el.id : ''}${cls ? '.' + cls : ''} (right ${Math.round(r.right)})`)
        }
      }
      res.offenders = out.slice(-6)
    }
    return res
  })
}

async function runAxe(page) {
  const builder = new AxeBuilder({ page }).withTags(TAGS)
  if (DISABLED_RULES.length) builder.disableRules(DISABLED_RULES.map((r) => r.id))
  const r = await builder.analyze()
  return r.violations
}

function short(v) {
  const nodes = v.nodes.slice(0, 3).map((n) => `      ${n.target.join(' ')}${n.failureSummary ? '\n        ' + n.failureSummary.split('\n').slice(1, 3).join(' / ') : ''}`)
  return `    [${v.impact}] ${v.id}: ${v.help} (${v.nodes.length} node${v.nodes.length === 1 ? '' : 's'})\n${nodes.join('\n')}`
}

async function main() {
  const browser = await chromium.launch({ executablePath: chromePath(), headless: true })
  const problems = []
  const warnings = []
  let checks = 0
  let axeRuns = 0
  try {
    // ---- sign in once; every configuration reuses the session cookie ----
    const authCtx = await browser.newContext()
    let me
    try {
      await api(authCtx.request, 'POST', '/auth/login', { email: EMAIL, password: PASSWORD })
      me = await api(authCtx.request, 'GET', '/auth/me')
    } catch (e) {
      console.error(`cannot sign in at ${BASE}: ${e.message}`)
      process.exitCode = 2
      return
    }
    const csrf = me.csrf_token
    const state = await authCtx.storageState()
    const routes = await discover(authCtx.request)
    const original = me.user?.preferences ?? {}
    // recent searches for the search-suggestions route
    await api(authCtx.request, 'POST', '/suggest/history', { entries: [{ field: 'search', value: 'status:approved' }, { field: 'search', value: 'login' }] }, csrf).catch(() => undefined)
    if (routes.some((r) => r.mfa)) await setupMfaUser(browser, authCtx.request, csrf)
    if (routes.some((r) => r.live) && (!ONLY.length || routes.some((r) => r.live && ONLY.includes(r.name)))) await setupLiveUser(browser, authCtx.request, csrf)
    // the electronic-records settings, restored after the routes that turn the mode on
    const erSaved = (await api(authCtx.request, 'GET', '/erecords').catch(() => null))?.settings ?? { enabled: false }

    for (const cfg of CONFIGS) {
      // the user's preferences decide theme and language once signed in
      await api(authCtx.request, 'PATCH', '/auth/me', { preferences: { theme: cfg.theme, language: cfg.lang, intro_done: true, autocomplete: null } }, csrf)
      for (const vp of VIEWPORTS) {
        const opts = { viewport: { width: vp.width, height: vp.height }, isMobile: vp.mobile, hasTouch: vp.mobile, reducedMotion: 'reduce', locale: cfg.lang === 'sv' ? 'sv-SE' : 'en-GB' }
        const local = ({ lang, theme }) => {
          const dark = ['dark', 'camouflage', 'city_camouflage', 'navy', 'midnight', 'green_console', 'amber_terminal', 'forest', 'high_contrast'].includes(theme)
          localStorage.setItem('qtf.lang', lang)
          localStorage.setItem('qtf.theme', dark ? 'dark' : 'light')
          if (theme !== 'light' && theme !== 'dark') localStorage.setItem('qtf.decor', theme)
          else localStorage.removeItem('qtf.decor')
          // every route starts in the list layout unless its address asks for the spreadsheet
          localStorage.removeItem('qtf.layout.requirement')
          localStorage.removeItem('qtf.layout.test')
          // and the project list in cards (projects-table switches to the table)
          localStorage.removeItem('qtf.projects.layout')
        }
        const pages = [{ name: 'login', path: '/login', anon: true }, ...routes].filter((r) => !ONLY.length || ONLY.includes(r.name))
        const ctx = await browser.newContext({ ...opts, storageState: state })
        const anon = await browser.newContext(opts)
        await ctx.addInitScript(local, { lang: cfg.lang, theme: cfg.theme })
        await anon.addInitScript(local, { lang: cfg.lang, theme: cfg.theme })
        for (const r of pages) {
          if (r.narrow && vp.width > 1023) continue
          if (r.minWidth && vp.width < r.minWidth) continue
          if (r.maxWidth && vp.width > r.maxWidth) continue
          // the second sign-in step: a context with a pending sign-in
          const mfaCtx = r.mfa ? await browser.newContext({ ...opts, storageState: await mfaPendingState(browser) }) : null
          if (mfaCtx) await mfaCtx.addInitScript(local, { lang: cfg.lang, theme: cfg.theme })
          // electronic-records mode for this route only
          if (r.erecords) await api(authCtx.request, 'PUT', '/erecords', { ...erSaved, enabled: true, reason_edits: ['requirement.update'], signature_actions: ['baseline.create'] }, csrf)
          const page = await (mfaCtx ?? (r.anon ? anon : ctx)).newPage()
          const label = `${cfg.name} ${vp.name} ${r.name}`
          const errors = []
          page.on('pageerror', (e) => errors.push(e.message))
          try {
            if (r.live && liveUser) await liveBefore(r.live)
            await page.goto(BASE + r.path, { waitUntil: 'domcontentloaded' })
            if (r.live && liveUser) {
              // the event stream opens a moment after the page has loaded
              await settle(page, '[data-testid="live-presence"]')
              await page.waitForTimeout(2500)
              await liveAfter(r.live)
            }
            // open: a selector, or several clicked one after another
            for (const sel of r.open ? [].concat(r.open) : []) {
              await settle(page)
              await page.locator(sel).first().click()
            }
            // act: a function of the page (fill a field and save)
            if (r.act) {
              await settle(page)
              await r.act(page)
            }
            // upload: [selector, file] — a file of the repository set in a file input (an import preview)
            if (r.upload) {
              await settle(page)
              await page.locator(r.upload[0]).first().setInputFiles(join(REPO, r.upload[1]))
            }
            // press: [selector, key] — a key pressed in a field (opens an autocomplete list)
            if (r.press) {
              await settle(page)
              await page.locator(r.press[0]).first().press(r.press[1])
            }
            // keys: keys pressed after that (e.g. F2 to edit a spreadsheet cell)
            for (const key of r.keys ?? []) await page.keyboard.press(key)
            if (r.keys) await page.mouse.move(1, 1)
            await settle(page, r.wait)
            if (!r.anon && page.url().includes('/login')) throw new Error('redirected to login')
            checks++
            const o = await overflow(page)
            if (o.scrollWidth > o.innerWidth) problems.push(`${label}: horizontal overflow ${o.scrollWidth}px > ${o.innerWidth}px\n      ${o.offenders.join('\n      ')}`)
            const violations = await runAxe(page)
            axeRuns++
            for (const v of violations) (FAIL_IMPACTS.has(v.impact) ? problems : warnings).push(`${label}:\n${short(v)}`)
            for (const e of errors) warnings.push(`${label}: page error: ${e}`)
            if (SHOTS) {
              mkdirSync(SHOTS, { recursive: true })
              await page.screenshot({ path: join(SHOTS, `${cfg.name.replace('/', '-')}_${vp.name}_${r.name}.png`), fullPage: false })
            }
          } catch (e) {
            problems.push(`${label}: ${e.message}`)
          } finally {
            await page.close()
            if (mfaCtx) await mfaCtx.close()
            if (r.erecords) await api(authCtx.request, 'PUT', '/erecords', erSaved, csrf).catch(() => undefined)
          }
        }
        await ctx.close()
        await anon.close()
        console.log(`checked ${cfg.name} at ${vp.width}×${vp.height}`)
      }
    }
    // ---- keyboard and focus behaviour (light/en) ----
    if (!ONLY.length && process.env.QTF_A11Y_KEYBOARD !== '0') {
      await api(authCtx.request, 'PATCH', '/auth/me', { preferences: { theme: 'light', language: 'en', intro_done: true } }, csrf)
      const results = await keyboardChecks(browser, state, routes)
      checks += results.length
      for (const r of results) if (!r.ok) problems.push(`keyboard: ${r.name}: ${r.detail}`)
      console.log(`keyboard checks: ${results.filter((r) => r.ok).length}/${results.length} passed`)
    }
    // drop the authenticator set-up the profile-mfa-setup route started
    await api(authCtx.request, 'POST', '/auth/mfa/disable', { code: '' }, csrf).catch(() => undefined)
    // restore the admin's own preferences
    await api(authCtx.request, 'PATCH', '/auth/me', { preferences: { theme: original.theme ?? null, language: original.language ?? null, intro_done: original.intro_done ?? null, autocomplete: original.autocomplete ?? null } }, csrf).catch(() => undefined)
    await authCtx.close()
  } finally {
    await browser.close()
  }

  console.log('')
  if (warnings.length) {
    console.log(`Minor/moderate findings and page errors (${warnings.length}, not failing):`)
    for (const w of warnings) console.log('  ' + w)
    console.log('')
  }
  if (DISABLED_RULES.length) console.log(`Disabled axe rules: ${DISABLED_RULES.map((r) => `${r.id} (${r.why})`).join(', ')}`)
  console.log(`Summary: ${checks} page checks, ${axeRuns} axe runs (${TAGS.join(', ')}), ${CONFIGS.length} configurations × ${VIEWPORTS.length} viewports`)
  if (problems.length) {
    console.log(`FAILED: ${problems.length} problem(s) (serious/critical axe violations, horizontal overflow or keyboard checks):`)
    for (const p of problems) console.log('  ' + p)
    process.exitCode = 1
  } else {
    console.log('OK: no serious or critical axe violations, no horizontal overflow, keyboard checks passed')
  }
}

main().catch((e) => {
  console.error(e)
  process.exitCode = 2
})
