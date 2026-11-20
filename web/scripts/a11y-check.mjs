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
import { existsSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { chromium } from 'playwright-core'
import { AxeBuilder } from '@axe-core/playwright'

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

/** Routes to visit, with ids from the seed data. */
async function discover(request) {
  const projects = items(await api(request, 'GET', '/projects'))
  const project = projects.find((p) => p.key === 'DEMO') ?? projects[0]
  if (!project) throw new Error('no project found – run `qtestframe seed-demo` first')
  const pid = project.id
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
  const manual = new Set(tests.filter((t) => t.execution_type === 'manual').map((t) => t.id))
  const ex = execs.find((e) => manual.has(e.test_case_id)) ?? execs[0]
  const hasDefects = items(await api(request, 'GET', `/projects/${pid}/defects`)).length > 0
  const hasRisks = items(await api(request, 'GET', `/projects/${pid}/risks`)).length > 0
  const hasReleases = items(await api(request, 'GET', `/projects/${pid}/releases`)).length > 0
  const hasBaselines = items(await api(request, 'GET', `/projects/${pid}/baselines`)).length > 0
  const firstBaseline = hasBaselines ? items(await api(request, 'GET', `/projects/${pid}/baselines`))[0] : null
  const hasDocuments = items(await api(request, 'GET', `/projects/${pid}/documents`)).length > 0
  const hasWebhooks = items(await api(request, 'GET', `/projects/${pid}/webhooks`).catch(() => [])).length > 0
  const hasRequested = items(await api(request, 'GET', '/reviews?requested_by=me').catch(() => [])).length > 0
  const hasNotifications = items(await api(request, 'GET', '/notifications').catch(() => [])).length > 0
  // a requirement that breaks a traceability rule (seed-demo has rules)
  const ruleReport = await api(request, 'GET', `/projects/${pid}/trace-rules/evaluate`).catch(() => null)
  const ruleViolator = (ruleReport?.results ?? []).flatMap((r) => r.violations ?? []).find((v) => v.kind === 'requirement')
  const p = `/p/${pid}`
  const routes = [
    { name: 'projects', path: '/projects' },
    { name: 'dashboard', path: `${p}/dashboard`, wait: 'canvas' },
    { name: 'requirements', path: `${p}/requirements` },
    { name: 'requirement-editor', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, wait: '.p-drawer' },
    { name: 'tests', path: `${p}/tests` },
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
    { name: 'trace-rule-dialog', path: `${p}/traceability?view=rules`, open: '.trace-rules .filterbar button:has(.pi-plus)', wait: '.p-dialog .trace-rule-form' },
    ruleViolator && { name: 'requirement-trace-rules', path: `${p}/requirements?item=${ruleViolator.id}`, wait: '.p-drawer .trace-violations' },
    { name: 'traceability-selected', path: `${p}/traceability`, open: '.trace-bulk .p-checkbox', wait: '.trace-bulk' },
    { name: 'graph', path: `${p}/graph`, wait: '.graph-node, .empty' },
    { name: 'graph-tests', path: `${p}/graph?kind=test`, wait: '.graph-node, .empty' },
    { name: 'tags', path: `${p}/tags`, wait: '.tag-cloud, .empty' },
    { name: 'tags-bulk', path: `${p}/tags`, open: '.tag-cloud .cloud-tag', wait: '.bulkbar' },
    { name: 'flow', path: `${p}/flow?test=${ex?.test_case_id ?? tests[0]?.id ?? ''}`, wait: '.flow-svg, .empty' },
    { name: 'flow-list', path: `${p}/flow?view=list&test=${ex?.test_case_id ?? tests[0]?.id ?? ''}`, wait: '.flow-list, .empty' },
    run && { name: 'flow-run', path: `${p}/flow?run=${run.id}`, wait: '.stages, .empty' },
    { name: 'analytics', path: `${p}/analytics`, wait: '.analytics-cards, .empty' },
    { name: 'test-links', path: `${p}/tests?item=${tests[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-link)', wait: '.item-links' },
    { name: 'link-dialog', path: `${p}/tests?item=${tests[0]?.id ?? ''}`, open: ['.p-drawer [role="tab"]:has(.pi-link)', '.item-links button:has(.pi-link)'], wait: '.link-types' },
    { name: 'new-requirement', path: `${p}/requirements`, open: '.page-header .actions button:has(.pi-plus)', wait: '.obligation-help' },
    { name: 'prompts', path: `${p}/prompts`, wait: '.prompts-grid, .p-message' },
    run && { name: 'run-readiness', path: `${p}/runs/${run.id}`, open: 'button.readiness-cell', wait: '.readiness-pop' },
    { name: 'undo-menu', path: `${p}/tests`, open: '.undo-controls button:has(.pi-history)', wait: '.undo-menu' },
    { name: 'columns-menu', path: `${p}/requirements`, open: 'button:has(.pi-table)', wait: '.columns-menu' },
    { name: 'baselines', path: `${p}/baselines` },
    { name: 'releases', path: `${p}/releases` },
    { name: 'risks', path: `${p}/risks`, wait: '.heat' },
    { name: 'defects', path: `${p}/defects` },
    // with a selected row (bulk toolbar); only when the project has defects / risks
    hasDefects && { name: 'defects-bulk', path: `${p}/defects`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    hasNotifications && { name: 'notifications-bulk', path: '/notifications', open: '.list li .p-checkbox', wait: '.bulkbar' },
    hasRequested && { name: 'reviews-bulk', path: '/reviews', open: ['.page-header .actions .p-selectbutton:first-child .p-togglebutton:nth-child(2)', '.p-datatable-tbody .p-checkbox'], wait: '.bulkbar' },
    hasWebhooks && { name: 'webhooks-bulk', path: `${p}/settings?tab=project`, open: 'section.webhooks .p-datatable-tbody .p-checkbox', wait: 'section.webhooks .bulkbar' },
    hasDocuments && { name: 'documents-bulk', path: `${p}/documents`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    // the comparison with restore checkboxes (needs items changed since the baseline)
    firstBaseline && { name: 'baseline-compare', path: `${p}/baselines?b=${firstBaseline.id}&tab=compare`, wait: '.bl-section, [role="status"]' },
    hasBaselines && { name: 'baselines-bulk', path: `${p}/baselines`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    hasReleases && { name: 'releases-bulk', path: `${p}/releases`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    hasRisks && { name: 'risks-bulk', path: `${p}/risks`, open: '.p-datatable-tbody .p-checkbox', wait: '.bulkbar' },
    { name: 'defect-dialog', path: `${p}/defects`, open: '.page-header .actions button:has(.pi-plus)', wait: '.p-dialog' },
    { name: 'requirement-defects', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-exclamation-circle)', wait: '.p-drawer [role="tabpanel"]' },
    { name: 'settings', path: `${p}/settings` },
    { name: 'settings-project', path: `${p}/settings?tab=project`, wait: '#h-tracker' },
    // the folder tree is a side panel from 1200 px (a drawer below)
    { name: 'folders-bulk', path: `${p}/requirements`, open: ['button.p-button:has(.pi-check-square)', '.p-tree .p-checkbox'], wait: '.folder-bulk', minWidth: 1200 },
    { name: 'folders-bulk-drawer', path: `${p}/requirements`, open: ['.page-header .actions button:has(.pi-folder)', '.p-drawer button.p-button:has(.pi-check-square)', '.p-drawer .p-tree .p-checkbox'], wait: '.p-drawer .folder-bulk', maxWidth: 1199 },
    { name: 'categories-bulk', path: `${p}/settings?tab=requirement`, open: '.category-grid .p-checkbox', wait: '.bulkbar' },
    { name: 'export', path: `${p}/export` },
    { name: 'documents', path: `${p}/documents` },
    { name: 'reports', path: `${p}/reports` },
    { name: 'reports-batch', path: `${p}/reports`, open: 'section:has(#h-batch) .p-datatable-tbody .p-checkbox', wait: 'section:has(#h-batch) .bulkbar' },
    { name: 'requirement-attachments', path: `${p}/requirements?item=${reqs[0]?.id ?? ''}`, open: '.p-drawer [role="tab"]:has(.pi-paperclip)', wait: '.dropzone' },
    run && { name: 'run-attachments', path: `${p}/runs/${run.id}`, open: '[role="tab"]:has(.pi-paperclip)', wait: '.dropzone' },
    { name: 'search', path: '/search?q=login' },
    { name: 'help', path: '/help' },
    { name: 'profile', path: '/profile' },
    { name: 'relations-bulk', path: `${p}/requirements`, open: ['.p-datatable-tbody tr .p-checkbox', '.bulkbar button:has(.pi-sitemap)'], wait: '.p-dialog [role="status"]' },
    relReq && { name: 'requirement-relations', path: `${p}/requirements?item=${relReq.id}`, open: ['.p-drawer [role="tab"]:has(.pi-sitemap)', '.p-drawer [role="tabpanel"] .p-checkbox'], wait: '.p-drawer [role="tabpanel"] .p-button-danger' },
    { name: 'item-links-bulk', path: `${p}/requirements`, open: ['.p-datatable-tbody tr .p-checkbox', '.bulkbar button:has(.pi-link)'], wait: '.p-dialog [role="status"]' },
    { name: 'run-links-bulk', path: `${p}/runs`, open: ['.p-datatable-tbody tr .p-checkbox', '.bulkbar button:has(.pi-link)'], wait: '.p-dialog [role="status"]' },
    linkReq && { name: 'requirement-links', path: `${p}/requirements?item=${linkReq.id}`, open: ['.p-drawer [role="tab"]:has(.pi-link)', '.item-links .p-checkbox'], wait: '.item-links .p-button-danger' },
    { name: 'bulk-review', path: `${p}/requirements`, open: '.page-header .actions button:has(.pi-verified)', wait: '.p-dialog .preview' },
    { name: 'notifications', path: '/notifications' },
    { name: 'reviews', path: '/reviews' },
    { name: 'members', path: '/admin/members' },
    { name: 'members-bulk', path: '/admin/members', open: '.p-datatable-tbody tr .p-checkbox', wait: '.bulkbar' },
    { name: 'rbac', path: '/admin/roles' },
    { name: 'execution-settings', path: '/admin/execution', wait: '.setting' },
    { name: 'backup', path: '/admin/backup' },
    { name: 'tokens', path: '/admin/tokens' },
    { name: 'audit', path: '/admin/audit' },
    { name: 'tenants', path: '/admin/tenants' },
    { name: 'users', path: '/admin/users' },
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
    const live = await page.evaluate(() => document.querySelector('[role="status"][aria-live="polite"].sr-only')?.textContent?.trim())
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

    for (const cfg of CONFIGS) {
      // the user's preferences decide theme and language once signed in
      await api(authCtx.request, 'PATCH', '/auth/me', { preferences: { theme: cfg.theme, language: cfg.lang, intro_done: true } }, csrf)
      for (const vp of VIEWPORTS) {
        const opts = { viewport: { width: vp.width, height: vp.height }, isMobile: vp.mobile, hasTouch: vp.mobile, reducedMotion: 'reduce', locale: cfg.lang === 'sv' ? 'sv-SE' : 'en-GB' }
        const local = ({ lang, theme }) => {
          const dark = ['dark', 'camouflage', 'city_camouflage', 'navy', 'green_console'].includes(theme)
          localStorage.setItem('qtf.lang', lang)
          localStorage.setItem('qtf.theme', dark ? 'dark' : 'light')
          if (theme !== 'light' && theme !== 'dark') localStorage.setItem('qtf.decor', theme)
          else localStorage.removeItem('qtf.decor')
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
          const page = await (r.anon ? anon : ctx).newPage()
          const label = `${cfg.name} ${vp.name} ${r.name}`
          const errors = []
          page.on('pageerror', (e) => errors.push(e.message))
          try {
            await page.goto(BASE + r.path, { waitUntil: 'domcontentloaded' })
            // open: a selector, or several clicked one after another
            for (const sel of r.open ? [].concat(r.open) : []) {
              await settle(page)
              await page.locator(sel).first().click()
            }
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
    // restore the admin's own preferences
    await api(authCtx.request, 'PATCH', '/auth/me', { preferences: { theme: original.theme ?? null, language: original.language ?? null, intro_done: original.intro_done ?? null } }, csrf).catch(() => undefined)
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
