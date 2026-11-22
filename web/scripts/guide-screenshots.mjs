#!/usr/bin/env node
// Screenshots for the user guide (docs/user-guide/images/<lang>-<name>.png).
//
// Runs against a server with the demo data (`qtestframe seed-demo`) and a
// super admin account. For each language it creates (or recreates) an
// organisation "guide-<lang>" with the tutorial project SHOP in that
// language, then drives the UI to every state the guide shows: editors,
// dialogs, the runner, reports, documents, the admin pages, the intro tour,
// and a few phone layouts. Pages that do not exist (yet) are skipped with a
// warning. See docs/user-guide/README.md.
//
//   QTF_URL              server URL (default http://127.0.0.1:8080)
//   QTF_EMAIL / QTF_PASSWORD   super admin login (default admin@example.com / Change-me-1234)
//   QTF_CHROME           Chrome/Chromium executable (default: Playwright's, else a system Chrome)
//   QTF_SHOTS_LANGS      languages (default "en,sv")
//   QTF_SHOTS_ONLY       comma-separated screenshot names (e.g. "runner,dashboard")
//   QTF_SHOTS_DIR        output directory (default ../docs/user-guide/images)
//   QTF_SHOTS_QUANTIZE   0 = keep the full-colour PNGs (default: reduce to 256 colours
//                        with ImageMagick `convert` when it is installed and the file gets smaller)
//
// Exit code 0 when the run completed (skipped shots are warnings), 2 when the
// server cannot be reached or the login fails.
import { existsSync, mkdirSync, renameSync, statSync, unlinkSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import { chromium, request as pwRequest } from 'playwright-core'

const HERE = dirname(fileURLToPath(import.meta.url))
const BASE = (process.env.QTF_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '')
const EMAIL = process.env.QTF_EMAIL || 'admin@example.com'
const PASSWORD = process.env.QTF_PASSWORD || 'Change-me-1234'
const LANGS = (process.env.QTF_SHOTS_LANGS || 'en,sv').split(',').map((s) => s.trim()).filter(Boolean)
const ONLY = (process.env.QTF_SHOTS_ONLY || '').split(',').map((s) => s.trim()).filter(Boolean)
const OUT = resolve(process.env.QTF_SHOTS_DIR || join(HERE, '../../docs/user-guide/images'))
const QUANTIZE = process.env.QTF_SHOTS_QUANTIZE !== '0' && spawnSync('convert', ['-version']).status === 0
const DESKTOP = { width: 1440, height: 900 }
const PHONE = { width: 390, height: 844 }

const done = []
const skipped = []
const warn = (msg) => {
  skipped.push(msg)
  console.warn(`  ! ${msg}`)
}

// ---------------------------------------------------------------- labels used to find controls

const L = {
  en: {
    addCriterion: 'Add criterion',
    history: 'History',
    executions: 'Executions',
    ciImport: 'CI import',
    compare: 'Compare',
    restartTour: 'Restart intro tour',
    next: 'Next',
    roleMatrix: 'Role matrix',
    methods: { inspection: 'Inspection' },
  },
  sv: {
    addCriterion: 'Lägg till kriterium',
    history: 'Versionshistorik',
    executions: 'Exekveringar',
    ciImport: 'CI-import',
    compare: 'Jämför',
    restartTour: 'Starta introduktionsturen igen',
    next: 'Nästa',
    roleMatrix: 'Rollmatris',
    methods: { inspection: 'Inspektion' },
  },
}

// ---------------------------------------------------------------- tutorial data per language

/** Further members of the guide organisation, several with more than one role (members and role screenshots). */
const MEMBERS = [
  { email: 'anna.lind@example.com', name: 'Anna Lind', roles: ['test_lead', 'reviewer'] },
  { email: 'erik.berg@example.com', name: 'Erik Berg', roles: ['tester'] },
  { email: 'maria.holm@example.com', name: 'Maria Holm', roles: ['sme', 'reviewer'] },
  { email: 'olof.ek@example.com', name: 'Olof Ek', roles: ['manager', 'tester'] },
  { email: 'sara.nyberg@example.com', name: 'Sara Nyberg', roles: ['viewer'] },
]

/** The organisation's logo on reports and documents (shapes only: the logo renderer draws no text). */
const LOGO_SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 80">
  <rect x="4" y="4" width="72" height="72" rx="14" fill="#2f5496"/>
  <path d="M22 30 h36 l-6 30 h-24 z" fill="#ffffff"/>
  <circle cx="30" cy="66" r="5" fill="#f2b134"/><circle cx="50" cy="66" r="5" fill="#f2b134"/>
  <rect x="92" y="18" width="136" height="12" rx="6" fill="#2f5496"/>
  <rect x="92" y="38" width="104" height="10" rx="5" fill="#5b7fc7"/>
  <rect x="92" y="56" width="72" height="8" rx="4" fill="#f2b134"/>
</svg>`

const DATA = {
  en: {
    tenant: { slug: 'guide-en', name: 'Web Shop Ltd' },
    project: { key: 'SHOP', name: 'Web shop', description: 'Checkout and customer accounts of the online shop.' },
    folders: { checkout: 'Checkout', payment: 'Payment', accounts: 'Accounts' },
    reqs: [
      { id: 'timeout', folder: 'payment', title: 'Card payment timeout', priority: 'high', area: 'Core', description: 'The system MUST cancel a card payment that is not confirmed by the payment provider within 30 seconds.', criteria: [{ description: 'Unconfirmed payment is cancelled', method: 'test', threshold: '≤ 30 s' }, { description: 'Cancellation is written to the audit log', method: 'inspection', threshold: '' }] },
      { id: 'confirm', folder: 'payment', title: 'Order confirmation e-mail', priority: 'medium', area: 'Core', description: 'The system SHOULD send an order confirmation by e-mail within 5 minutes after payment.', criteria: [{ description: 'Confirmation e-mail is received', method: 'test', threshold: '≤ 5 min' }] },
      { id: 'total', folder: 'checkout', title: 'Basket total', priority: 'high', area: 'UI', description: 'The system MUST show the basket total including VAT on every checkout page.', criteria: [{ description: 'Total equals the sum of the items including VAT', method: 'test', threshold: '± 0.01' }] },
      { id: 'lockout', folder: 'accounts', title: 'Account lockout', priority: 'critical', area: 'Security', description: 'The system MUST lock a customer account after 5 failed sign-ins within 10 minutes.', criteria: [{ description: 'Sixth attempt is rejected', method: 'test', threshold: '5 attempts / 10 min' }, { description: 'Lockout is logged', method: 'inspection', threshold: '' }] },
      { id: 'password', folder: 'accounts', title: 'Minimum password length', priority: 'high', area: 'Security', description: 'The system MUST NOT accept passwords shorter than 12 characters.', criteria: [{ description: 'An 11-character password is rejected', method: 'test', threshold: '' }] },
      { id: 'cards', folder: 'accounts', title: 'Saved cards', priority: 'low', area: 'Core', description: 'The customer MAY save a card for later purchases.', criteria: [] },
      { id: 'guest', folder: 'checkout', title: 'Guest checkout', priority: 'medium', area: 'UI', description: 'The system SHOULD allow a checkout without a customer account.', criteria: [], noTest: true },
    ],
    steps: [
      { action: 'Put an item in the basket and start the card payment', expected: 'The payment page of the provider opens', data: 'test card 4111 1111 1111 1111' },
      { action: 'Do not confirm the payment; wait 30 seconds', expected: 'The order shows “Payment cancelled”', data: '' },
      { action: 'Open the audit log', expected: 'A “payment cancelled” entry exists for the order', data: '' },
    ],
    shared: { title: 'Sign in as a customer', steps: [{ action: 'Open the shop and select Sign in', expected: 'The sign-in form is shown' }, { action: 'Enter the customer e-mail and password', expected: 'The account page is shown', data: 'anna@example.com / Test-pass-123' }] },
    auto: { title: 'Order total calculation', script: '#!/bin/sh\n# exit code 0 = passed, non-zero = failed\ntotal=$((2 * 15 + 5))\necho "order total: $total"\n[ "$total" -eq 35 ] || { echo "unexpected total"; exit 1; }\n' },
    runs: { main: 'Sprint 1 regression', smoke: 'Smoke test 0.9', next: 'Sprint 2 regression' },
    actual: 'No audit log entry for the cancelled payment',
    notes: 'Reproduced twice on staging.',
    comments: ['Is 30 seconds aligned with the payment provider’s own timeout?', 'Yes, the provider times out after 45 s, so 30 s is safe.'],
    baseline: { name: 'Release 1.0', description: 'Specification as approved for release 1.0' },
    document: { name: 'Payment provider interface.txt', description: 'Interface description of the payment provider', tags: 'spec,payment', text: 'Payment provider interface\n\nCallback: POST /payments/{id}/confirm within 30 s.\n' },
    token: 'GitLab CI',
    search: 'payment',
    weak: 'The system should respond fast and/or retry TBD.',
    analysis: 'The checkout shall be user-friendly and should respond as soon as possible, if possible. TBD',
    newReqTitle: 'Card payment timeout',
  },
  sv: {
    tenant: { slug: 'guide-sv', name: 'Webbutiken AB' },
    project: { key: 'SHOP', name: 'Webbutik', description: 'Kassa och kundkonton i webbutiken.' },
    folders: { checkout: 'Kassa', payment: 'Betalning', accounts: 'Konton' },
    reqs: [
      { id: 'timeout', folder: 'payment', title: 'Tidsgräns för kortbetalning', priority: 'high', area: 'Core', description: 'Systemet MUST avbryta en kortbetalning som inte har bekräftats av betalningsleverantören inom 30 sekunder.', criteria: [{ description: 'Obekräftad betalning avbryts', method: 'test', threshold: '≤ 30 s' }, { description: 'Avbrottet skrivs till granskningsloggen', method: 'inspection', threshold: '' }] },
      { id: 'confirm', folder: 'payment', title: 'Orderbekräftelse via e-post', priority: 'medium', area: 'Core', description: 'Systemet SHOULD skicka en orderbekräftelse via e-post inom 5 minuter efter betalningen.', criteria: [{ description: 'Bekräftelsen tas emot', method: 'test', threshold: '≤ 5 min' }] },
      { id: 'total', folder: 'checkout', title: 'Varukorgens summa', priority: 'high', area: 'UI', description: 'Systemet MUST visa varukorgens summa inklusive moms på varje sida i kassan.', criteria: [{ description: 'Summan motsvarar varornas pris inklusive moms', method: 'test', threshold: '± 0,01' }] },
      { id: 'lockout', folder: 'accounts', title: 'Spärr av konto', priority: 'critical', area: 'Security', description: 'Systemet MUST spärra ett kundkonto efter 5 misslyckade inloggningar inom 10 minuter.', criteria: [{ description: 'Sjätte försöket avvisas', method: 'test', threshold: '5 försök / 10 min' }, { description: 'Spärren loggas', method: 'inspection', threshold: '' }] },
      { id: 'password', folder: 'accounts', title: 'Minsta lösenordslängd', priority: 'high', area: 'Security', description: 'Systemet MUST NOT godta lösenord som är kortare än 12 tecken.', criteria: [{ description: 'Ett lösenord med 11 tecken avvisas', method: 'test', threshold: '' }] },
      { id: 'cards', folder: 'accounts', title: 'Sparade kort', priority: 'low', area: 'Core', description: 'Kunden MAY spara ett kort till senare köp.', criteria: [] },
      { id: 'guest', folder: 'checkout', title: 'Köp utan konto', priority: 'medium', area: 'UI', description: 'Systemet SHOULD tillåta köp utan kundkonto.', criteria: [], noTest: true },
    ],
    steps: [
      { action: 'Lägg en vara i varukorgen och starta kortbetalningen', expected: 'Leverantörens betalningssida öppnas', data: 'testkort 4111 1111 1111 1111' },
      { action: 'Bekräfta inte betalningen; vänta 30 sekunder', expected: 'Ordern visar ”Betalningen avbruten”', data: '' },
      { action: 'Öppna granskningsloggen', expected: 'Det finns en post ”betalning avbruten” för ordern', data: '' },
    ],
    shared: { title: 'Logga in som kund', steps: [{ action: 'Öppna butiken och välj Logga in', expected: 'Inloggningsformuläret visas' }, { action: 'Ange kundens e-post och lösenord', expected: 'Kontosidan visas', data: 'anna@example.com / Test-pass-123' }] },
    auto: { title: 'Beräkning av ordersumma', script: '#!/bin/sh\n# slutkod 0 = godkänt, annat = underkänt\ntotal=$((2 * 15 + 5))\necho "ordersumma: $total"\n[ "$total" -eq 35 ] || { echo "oväntad summa"; exit 1; }\n' },
    runs: { main: 'Regressionstest sprint 1', smoke: 'Röktest 0.9', next: 'Regressionstest sprint 2' },
    actual: 'Ingen post i granskningsloggen för den avbrutna betalningen',
    notes: 'Återskapat två gånger i staging.',
    comments: ['Stämmer 30 sekunder med betalningsleverantörens egen tidsgräns?', 'Ja, leverantören avbryter efter 45 s, så 30 s är säkert.'],
    baseline: { name: 'Release 1.0', description: 'Specifikationen som den godkändes för release 1.0' },
    document: { name: 'Gränssnitt mot betalningsleverantören.txt', description: 'Gränssnittsbeskrivning för betalningsleverantören', tags: 'specifikation,betalning', text: 'Gränssnitt mot betalningsleverantören\n\nÅteranrop: POST /payments/{id}/confirm inom 30 s.\n' },
    token: 'GitLab CI',
    search: 'betalning',
    weak: 'The system should respond fast and/or retry TBD.',
    analysis: 'Kassan ska vara användarvänlig och bör svara så snart som möjligt, om möjligt. TBD',
    newReqTitle: 'Tidsgräns för kortbetalning',
  },
}

// ---------------------------------------------------------------- API

class Api {
  constructor(ctx) {
    this.ctx = ctx
    this.csrf = ''
  }
  async call(method, path, body, opts = {}) {
    const headers = { Origin: BASE, ...(this.csrf ? { 'X-CSRF-Token': this.csrf } : {}) }
    const init = { method, headers, failOnStatusCode: false }
    if (opts.multipart) init.multipart = opts.multipart
    else if (body !== undefined) {
      init.data = body
      headers['Content-Type'] = 'application/json'
    }
    const res = await this.ctx.fetch(`${BASE}/api/v1${path}`, init)
    const text = await res.text()
    if (!res.ok()) {
      const err = new Error(`${method} ${path}: HTTP ${res.status()} ${text.slice(0, 200)}`)
      err.status = res.status()
      throw err
    }
    const json = text ? JSON.parse(text) : null
    if (json && typeof json === 'object' && json.csrf_token) this.csrf = json.csrf_token
    return json
  }
  get(p) {
    return this.call('GET', p)
  }
  post(p, b) {
    return this.call('POST', p, b ?? {})
  }
  patch(p, b) {
    return this.call('PATCH', p, b)
  }
  put(p, b) {
    return this.call('PUT', p, b)
  }
  del(p) {
    return this.call('DELETE', p)
  }
}
const items = (x) => (Array.isArray(x) ? x : (x?.items ?? []))
const soft = async (what, fn) => {
  try {
    return await fn()
  } catch (e) {
    warn(`${what}: ${e.message}`)
    return null
  }
}

async function ensureTenant(api, me, { slug, name }) {
  let tenant = items(await api.get('/tenants')).find((t) => t.slug === slug)
  if (!tenant) tenant = await api.post('/tenants', { slug, name })
  let now = await api.post('/auth/switch-tenant', { tenant_id: tenant.id })
  const members = items(await soft('list members', () => api.get('/members')))
  if (!members.some((m) => (m.email || '').toLowerCase() === EMAIL.toLowerCase())) {
    await soft('add membership', () => api.post('/members', { email: EMAIL, role: 'admin' }))
    now = await api.get('/auth/me')
  }
  // further members, several with more than one role
  for (const m of MEMBERS) {
    const have = members.find((x) => (x.email || '').toLowerCase() === m.email)
    if (!have) await soft(`add member ${m.email}`, () => api.post('/members', { email: m.email, display_name: m.name, roles: m.roles }))
    else await soft(`roles of ${m.email}`, () => api.patch(`/members/${have.user_id ?? have.id}`, { roles: m.roles }))
  }
  // the logo on the first page of reports and documents (Administration → Documents)
  await soft('logo', () =>
    api.call('PUT', '/settings/logo/image', undefined, { multipart: { file: { name: 'logo.svg', mimeType: 'image/svg+xml', buffer: Buffer.from(LOGO_SVG) } } }),
  )
  await soft('logo placement', () => api.put('/settings/logo', { placement: 'right', max_width_mm: 45, max_height_mm: 15 }))
  return { tenant, me: now ?? me }
}

/** (Re)create the tutorial project with requirements, tests, runs, a baseline and a document. */
async function seedTutorial(api, D) {
  for (const p of items(await api.get('/projects'))) if (p.key === D.project.key) await api.del(`/projects/${p.id}`)
  const project = await api.post('/projects', { ...D.project, seed_categories: true })
  const pid = project.id
  await soft('defect template', () => api.patch(`/projects/${pid}`, { defect_url_template: 'https://jira.example.com/browse/{id}' }))
  const cats = items(await api.get(`/projects/${pid}/categories`))
  const cat = (dim, name) => cats.find((c) => c.dimension === dim && c.name === name)?.id
  const folder = (name, parent, kind = 'requirement') => api.post(`/projects/${pid}/folders`, { kind, name, description: '', parent_id: parent ?? null })
  const checkout = await folder(D.folders.checkout)
  const payment = await folder(D.folders.payment, checkout.id)
  const accounts = await folder(D.folders.accounts)
  const rfolder = { checkout: checkout.id, payment: payment.id, accounts: accounts.id }

  const reqs = {}
  const linked = {}
  for (const r of D.reqs) {
    const res = await api.post(`/projects/${pid}/requirements`, {
      title: r.title,
      description: r.description,
      priority: r.priority,
      status: 'review',
      type: r.area === 'Security' ? 'security' : 'functional',
      folder_id: rfolder[r.folder],
      category_ids: [cat('Area', r.area)].filter(Boolean),
      criteria: r.criteria,
      owner: 'anna',
      create_test: !r.noTest,
    })
    reqs[r.id] = res.requirement ?? res
    if (res.linked_test) linked[r.id] = res.linked_test
  }
  await soft('relation', () => api.post(`/requirements/${reqs.password.id}/relations`, { target_id: reqs.lockout.id, type: 'refines' }))
  for (const id of ['total', 'lockout']) await soft('approve', () => api.patch(`/requirements/${reqs[id].id}`, { status: 'approved' }))

  const tfolders = items(await api.get(`/projects/${pid}/folders?kind=test`))
  const tf = (name) => tfolders.find((f) => f.name === name)?.id ?? null
  const shared = await api.post(`/projects/${pid}/tests`, { title: D.shared.title, execution_type: 'manual', status: 'ready', priority: 'medium', folder_id: tf(D.folders.accounts), steps: D.shared.steps })
  const tc1 = linked.timeout
  if (tc1) {
    await api.patch(`/tests/${tc1.id}`, {
      status: 'ready',
      preconditions: D.shared.steps[1].data,
      category_ids: [cat('Polarity', 'Negative'), cat('Interface', 'External'), cat('Execution', 'Manual'), cat('Level', 'System')].filter(Boolean),
      steps: [{ call_test_id: shared.id, action: '', expected: '', data: '' }, ...D.steps],
    })
  }
  for (const id of ['total', 'lockout', 'password', 'confirm']) {
    if (linked[id]) await soft('ready test', () => api.patch(`/tests/${linked[id].id}`, { status: 'ready', category_ids: [cat('Polarity', id === 'password' || id === 'lockout' ? 'Negative' : 'Positive'), cat('Level', 'System')].filter(Boolean) }))
  }
  const auto = await api.post(`/projects/${pid}/tests`, {
    title: D.auto.title,
    execution_type: 'automated',
    script_language: 'sh',
    script: D.auto.script,
    status: 'ready',
    priority: 'high',
    folder_id: tf(D.folders.checkout),
    category_ids: [cat('Execution', 'Automated'), cat('Level', 'Unit'), cat('Polarity', 'Positive')].filter(Boolean),
  })
  if (linked.total) await soft('link auto', () => api.post(`/projects/${pid}/links`, { requirement_id: reqs.total.id, test_case_id: auto.id }))

  // an older run, all passed
  const smoke = await api.post(`/projects/${pid}/runs`, { name: D.runs.smoke, environment: 'staging', build: '0.9.0', test_ids: [linked.total?.id, linked.lockout?.id, shared.id].filter(Boolean) })
  for (const ex of items(await api.get(`/runs/${smoke.id}/executions`))) await soft('smoke result', () => api.patch(`/executions/${ex.id}`, { status: 'passed', duration_ms: 60000 + Math.round(Math.random() * 120000) }))
  await soft('smoke status', () => api.patch(`/runs/${smoke.id}`, { status: 'completed' }))

  // the tutorial run
  const run = await api.post(`/projects/${pid}/runs`, { name: D.runs.main, description: '', environment: 'staging', build: '1.0.0', folder_ids: tfolders.filter((f) => !f.parent_id).map((f) => f.id) })
  const execs = items(await api.get(`/runs/${run.id}/executions`))
  let tc1Exec = null
  let autoExec = null
  for (const e of execs) {
    if (tc1 && e.test_case_id === tc1.id) tc1Exec = e
    if (e.test_case_id === auto.id) autoExec = e
  }
  const passAll = async (e, status = 'passed') => {
    const full = await api.get(`/executions/${e.id}`)
    const step_results = (full.step_results ?? []).map((s) => ({ id: s.id, status, actual: '' }))
    return api.patch(`/executions/${e.id}`, step_results.length ? { step_results, duration_ms: 240000 } : { status, duration_ms: 240000 })
  }
  for (const e of execs) {
    if (e === tc1Exec || e === autoExec) continue
    if (linked.password && e.test_case_id === linked.password.id) continue // stays not run
    if (linked.cards && e.test_case_id === linked.cards.id) continue
    await soft('result', () => passAll(e, linked.lockout && e.test_case_id === linked.lockout.id ? 'blocked' : 'passed'))
  }
  if (tc1Exec) {
    const full = await api.get(`/executions/${tc1Exec.id}`)
    const sr = full.step_results ?? []
    await api.patch(`/executions/${tc1Exec.id}`, {
      step_results: sr.map((s, i) => ({ id: s.id, status: i === sr.length - 1 ? 'failed' : 'passed', actual: i === sr.length - 1 ? D.actual : '' })),
      defects: ['SHOP-101'],
      notes: D.notes,
      assignee: 'anna',
      duration_ms: 420000,
    })
  }
  if (autoExec) {
    const ran = await soft('run script', () => api.post(`/executions/${autoExec.id}/run-script`))
    if (!ran) await soft('auto result', () => api.patch(`/executions/${autoExec.id}`, { status: 'passed', output: 'order total: 35\n', duration_ms: 12 }))
  }
  await soft('run status', () => api.patch(`/runs/${run.id}`, { status: 'in_progress' }))
  await soft('planned run', () => api.post(`/projects/${pid}/runs`, { name: D.runs.next, environment: 'staging', build: '1.1.0', query: 'status:ready' }))

  // the administrator owns some requirements and a test (My work)
  for (const k of ['lockout', 'password', 'guest']) if (reqs[k]) await soft('owner', () => api.patch(`/requirements/${reqs[k].id}`, { owner: EMAIL }))
  if (linked.total) await soft('test owner', () => api.patch(`/tests/${linked.total.id}`, { owner: EMAIL }))

  // collaboration, baseline and a later change
  for (const body of D.comments) await soft('comment', () => api.post(`/requirements/${reqs.timeout.id}/comments`, { body }))
  const baseline = await soft('baseline', () => api.post(`/projects/${pid}/baselines`, D.baseline))
  const crit = D.reqs[0].criteria.map((c, i) => (i === 0 ? { ...c, threshold: '≤ 20 s' } : c))
  await soft('change after baseline', () => api.patch(`/requirements/${reqs.timeout.id}`, { status: 'approved', criteria: crit }))

  // a document in the archive and an API token
  const doc = await soft('document upload', () =>
    api.call('POST', `/projects/${pid}/documents`, undefined, {
      multipart: { file: { name: D.document.name, mimeType: 'text/plain', buffer: Buffer.from(D.document.text) }, description: D.document.description, tags: D.document.tags },
    }),
  )
  if (doc?.id) await soft('attach document', () => api.post(`/requirements/${reqs.timeout.id}/attachments/link`, { file_id: doc.id }))
  if (!items(await soft('tokens', () => api.get('/tokens'))).some((t) => t.name === D.token)) await soft('token', () => api.post('/tokens', { name: D.token, role: 'tester', expires_days: 365 }))
  await soft('report to archive', () => api.post(`/projects/${pid}/reports/generate`, { template: 'requirements_coverage', format: 'pdf', archive: true }))

  return { pid, project, reqs, linked, shared, auto, run, smoke, tc1Exec, autoExec, baseline, folders: rfolder }
}

// ---------------------------------------------------------------- browser helpers

function chromePath() {
  if (process.env.QTF_CHROME) return process.env.QTF_CHROME
  try {
    const p = chromium.executablePath()
    if (p && existsSync(p)) return undefined
  } catch {
    /* no bundled browser */
  }
  for (const p of ['/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/google-chrome', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome']) if (existsSync(p)) return p
  return undefined
}

async function settle(page, wait, timeout = 8000) {
  await page.waitForLoadState('networkidle', { timeout }).catch(() => undefined)
  if (wait) await page.waitForSelector(wait, { timeout, state: 'visible' })
  await page.waitForFunction(() => !document.querySelector('.p-datatable-mask, .p-skeleton, .p-progressspinner'), null, { timeout }).catch(() => undefined)
  await page.waitForTimeout(350)
}

async function go(page, path, wait) {
  const res = await page.goto(BASE + path, { waitUntil: 'domcontentloaded' })
  if (res && res.status() >= 400) throw new Error(`HTTP ${res.status()} for ${path}`)
  await settle(page, wait)
  if (page.url().includes('/login') && !path.startsWith('/login')) throw new Error('redirected to sign-in')
  if (await page.locator('#main-content h1').filter({ hasText: /Page not found|Sidan hittades inte/ }).count()) throw new Error(`page ${path} does not exist`)
}

/** Save a screenshot of the page (optionally clipped) or of an element; reduce colours when possible. */
async function save(target, file, opts = {}) {
  const path = join(OUT, file)
  const tmp = path + '.tmp.png'
  await target.screenshot({ path: tmp, animations: 'disabled', caret: 'hide', ...opts })
  if (QUANTIZE) {
    const q = path + '.q.png'
    const r = spawnSync('convert', [tmp, '-strip', '-dither', 'None', '-colors', '256', `PNG8:${q}`])
    if (r.status === 0 && existsSync(q) && statSync(q).size < statSync(tmp).size) {
      unlinkSync(tmp)
      renameSync(q, path)
    } else {
      if (existsSync(q)) unlinkSync(q)
      renameSync(tmp, path)
    }
  } else renameSync(tmp, path)
  const kb = Math.round(statSync(path).size / 1024)
  done.push({ file, kb })
  console.log(`  ✓ ${file} (${kb} KB)`)
}

/** The visible drawer (editor) as screenshot target. */
const drawer = (page) => page.locator('.p-drawer:visible').last()
const dialog = (page) => page.locator('.p-dialog:visible').last()

async function openEditor(page, path, itemId) {
  await go(page, `${path}${path.includes('?') ? '&' : '?'}item=${itemId}`, '.p-drawer')
  await page.waitForTimeout(500)
}

async function clickTab(page, name) {
  await page.getByRole('tab', { name: new RegExp(`^\\s*${name}`) }).first().click()
  await settle(page)
}

// ---------------------------------------------------------------- the screenshots

function shotsFor(lang, D, ids) {
  const P = `/p/${ids.pid}`
  const T = L[lang]
  return [
    {
      name: 'login',
      anon: true,
      run: async (page) => {
        await go(page, '/login', 'form')
        await page.locator('#email').fill(EMAIL)
        await save(page, `${lang}-login.png`)
      },
    },
    {
      name: 'layout',
      run: async (page) => {
        await go(page, '/projects', '.page-header')
        await save(page, `${lang}-layout.png`)
      },
    },
    {
      name: 'intro-tour',
      run: async (page) => {
        await go(page, `${P}/dashboard`, 'canvas')
        await page.locator('.qtf-help-button').click()
        await page.getByRole('menuitem', { name: T.restartTour }).click()
        await page.waitForSelector('.tour-card', { timeout: 5000 })
        await page.locator('.tour-next').click()
        await page.locator('.tour-next').click()
        await page.locator('.tour-next').click()
        await page.waitForTimeout(600)
        await save(page, `${lang}-intro-tour.png`)
        await page.keyboard.press('Escape')
      },
    },
    {
      name: 'profile',
      run: async (page) => {
        await go(page, '/profile', '#main-content h1')
        await save(page, `${lang}-profile.png`)
      },
    },
    {
      name: 'tutorial-new-project',
      run: async (page) => {
        await go(page, '/projects', '.page-header')
        await page.locator('.page-header button:has(.pi-plus)').first().click()
        const d = dialog(page)
        await d.waitFor()
        const inputs = d.locator('input[type="text"], input:not([type])')
        await inputs.nth(0).fill(D.project.name)
        await inputs.nth(1).fill('SHOP')
        await d.locator('textarea').fill(D.project.description)
        await page.waitForTimeout(300)
        await save(d, `${lang}-tutorial-new-project.png`)
      },
    },
    {
      name: 'tutorial-folders',
      run: async (page) => {
        await go(page, `${P}/requirements?folder=${ids.folders.payment}`, '.p-datatable')
        await save(page, `${lang}-tutorial-folders.png`)
      },
    },
    ...[
      ['tutorial-analysis-weak', D.weak, false],
      ['tutorial-analysis-good', D.reqs[0].description, false],
      ['tutorial-criteria', D.reqs[0].description, true],
    ].map(([name, text, criteria]) => ({
      name,
      run: async (page) => {
        await go(page, `${P}/requirements?folder=${ids.folders.payment}`, '.p-datatable')
        await page.locator('.page-header .actions button:has(.pi-plus)').first().click()
        const d = drawer(page)
        await d.waitFor()
        await d.locator('input[type="text"], input:not([type])').first().fill(D.newReqTitle)
        await d.locator('textarea').first().fill(text)
        await page.waitForSelector('.analysis .text', { timeout: 8000 })
        await page.waitForTimeout(900)
        if (criteria) {
          for (const c of D.reqs[0].criteria) {
            await d.getByRole('button', { name: T.addCriterion }).click()
            const row = d.locator('.criterion-row').last()
            await row.locator('textarea').fill(c.description)
            await row.locator('input').last().fill(c.threshold)
            if (c.method !== 'test') {
              await row.locator('.p-select').click()
              await page.getByRole('option', { name: T.methods[c.method] }).click()
            }
          }
          await d.locator('.criterion-row').last().evaluate((el) => el.scrollIntoView({ block: 'center' }))
          await page.waitForTimeout(300)
        }
        await save(d, `${lang}-${name}.png`)
      },
    })),
    {
      name: 'tutorial-test-steps',
      run: async (page) => {
        await openEditor(page, `${P}/tests`, ids.linked.timeout.id)
        await save(drawer(page), `${lang}-tutorial-test-steps.png`)
      },
    },
    ...['tutorial-shared-step', 'tests-steps'].map((name) => ({
      name,
      run: async (page) => {
        await openEditor(page, `${P}/tests`, ids.linked.timeout.id)
        await drawer(page).locator('.call-step').first().scrollIntoViewIfNeeded()
        await page.waitForTimeout(300)
        await save(drawer(page), `${lang}-${name}.png`)
      },
    })),
    {
      name: 'tutorial-new-run',
      run: async (page) => {
        await go(page, `${P}/runs`, '.page-header')
        await page.locator('.page-header .actions button:has(.pi-plus)').first().click()
        const d = dialog(page)
        await d.waitFor()
        const inputs = d.locator('.form-grid input')
        await inputs.nth(0).fill(D.runs.main)
        await inputs.nth(1).fill('staging')
        await inputs.nth(2).fill('1.0.0')
        await d.getByRole('tab').nth(1).click()
        await page.waitForTimeout(300)
        await d.locator('.p-tree .p-checkbox').first().click()
        await page.waitForTimeout(400)
        await save(d, `${lang}-tutorial-new-run.png`)
      },
    },
    ...['tutorial-runner', 'runner'].map((name) => ({
      name,
      run: async (page) => {
        await go(page, `${P}/runs/${ids.run.id}/exec/${ids.tc1Exec.id}`, '.step-list')
        await page.locator('.step-list button').last().click()
        await page.waitForTimeout(300)
        await save(page, `${lang}-${name}.png`)
      },
    })),
    {
      name: 'runner-shortcuts',
      run: async (page) => {
        await go(page, `${P}/runs/${ids.run.id}/exec/${ids.tc1Exec.id}`, '.step-list')
        await page.locator('button[aria-controls="runner-keys"]').click()
        await page.waitForSelector('#runner-keys', { state: 'visible' })
        await page.waitForTimeout(300)
        const card = page.locator('section[aria-labelledby="step-heading"]')
        await card.scrollIntoViewIfNeeded()
        await save(card, `${lang}-runner-shortcuts.png`)
      },
    },
    {
      name: 'tutorial-script',
      run: async (page) => {
        await go(page, `${P}/runs/${ids.run.id}/exec/${ids.autoExec.id}`, 'pre.console')
        await save(page, `${lang}-tutorial-script.png`)
      },
    },
    ...['tutorial-traceability', 'traceability'].map((name) => ({
      name,
      run: async (page) => {
        await go(page, `${P}/traceability`, 'table')
        await save(page, `${lang}-${name}.png`)
      },
    })),
    ...['tutorial-dashboard', 'dashboard'].map((name) => ({
      name,
      run: async (page) => {
        await go(page, `${P}/dashboard`, 'canvas')
        await page.waitForTimeout(800)
        await save(page, `${lang}-${name}.png`)
      },
    })),
    ...['tutorial-baseline', 'baselines'].map((name) => ({
      name,
      run: async (page) => {
        if (!ids.baseline) throw new Error('no baseline')
        await go(page, `${P}/baselines?b=${ids.baseline.id}&tab=compare`, '#main-content h1')
        const btn = page.getByRole('button', { name: new RegExp(`^${T.compare}$`) })
        if (await btn.count()) await btn.first().click()
        await settle(page)
        await save(page, `${lang}-${name}.png`)
      },
    })),
    ...['tutorial-export', 'export'].map((name) => ({
      name,
      run: async (page) => {
        await go(page, `${P}/export`, '#main-content h1')
        await save(page, `${lang}-${name}.png`)
      },
    })),
    {
      name: 'requirements-list',
      run: async (page) => {
        await go(page, `${P}/requirements`, '.p-datatable')
        await save(page, `${lang}-requirements-list.png`)
      },
    },
    {
      name: 'requirements-analysis',
      run: async (page) => {
        await go(page, `${P}/requirements`, '.p-datatable')
        await page.locator('.page-header .actions button:has(.pi-plus)').first().click()
        const d = drawer(page)
        await d.waitFor()
        await d.locator('textarea').first().fill(D.analysis)
        await page.waitForSelector('.analysis .text', { timeout: 8000 })
        await page.waitForTimeout(900)
        await save(d.locator('.analysis'), `${lang}-requirements-analysis.png`)
      },
    },
    {
      name: 'requirements-history',
      run: async (page) => {
        await openEditor(page, `${P}/requirements`, ids.reqs.timeout.id)
        await clickTab(page, T.history)
        await page.waitForTimeout(500)
        await save(drawer(page), `${lang}-requirements-history.png`)
      },
    },
    {
      name: 'requirements-bulk',
      run: async (page) => {
        await go(page, `${P}/requirements`, '.p-datatable')
        const boxes = page.locator('.p-datatable-tbody .p-checkbox')
        await boxes.nth(0).click()
        await boxes.nth(1).click()
        await page.waitForSelector('.bulkbar')
        await save(page, `${lang}-requirements-bulk.png`)
      },
    },
    {
      name: 'tests-list',
      run: async (page) => {
        await go(page, `${P}/tests`, '.p-datatable')
        await save(page, `${lang}-tests-list.png`)
      },
    },
    {
      name: 'tests-script',
      run: async (page) => {
        await openEditor(page, `${P}/tests`, ids.auto.id)
        await save(drawer(page), `${lang}-tests-script.png`)
      },
    },
    {
      name: 'tests-executions',
      run: async (page) => {
        await openEditor(page, `${P}/tests`, ids.linked.total?.id ?? ids.linked.timeout.id)
        await clickTab(page, T.executions)
        await page.waitForTimeout(500)
        await save(drawer(page), `${lang}-tests-executions.png`)
      },
    },
    {
      name: 'settings-categories',
      run: async (page) => {
        await go(page, `${P}/settings`, '#main-content h1')
        await save(page, `${lang}-settings-categories.png`)
      },
    },
    {
      name: 'runs-list',
      run: async (page) => {
        await go(page, `${P}/runs`, '.p-datatable')
        await save(page, `${lang}-runs-list.png`)
      },
    },
    {
      name: 'run-detail',
      run: async (page) => {
        await go(page, `${P}/runs/${ids.run.id}`, '.p-datatable')
        await save(page, `${lang}-run-detail.png`)
      },
    },
    {
      name: 'ci-import',
      run: async (page) => {
        await go(page, `${P}/runs/${ids.run.id}`, '.p-datatable')
        await page.getByRole('button', { name: T.ciImport }).click()
        await save(dialog(page), `${lang}-ci-import.png`)
      },
    },
    {
      name: 'search',
      run: async (page) => {
        await go(page, `/search?q=${encodeURIComponent(D.search)}`, '#main-content h1')
        await page.waitForTimeout(500)
        await save(page, `${lang}-search.png`)
      },
    },
    {
      name: 'query-help',
      run: async (page) => {
        await go(page, `${P}/requirements`, '.p-datatable')
        await page.locator('.filterbar button[aria-haspopup="dialog"]').first().click()
        const pop = page.locator('.p-popover:visible').first()
        await pop.waitFor()
        await page.waitForTimeout(300)
        const box = await pop.boundingBox()
        const bar = await page.locator('.filterbar').first().boundingBox()
        const top = Math.max(0, Math.min(bar.y, box.y) - 8)
        const left = Math.max(0, Math.min(bar.x, box.x) - 8)
        const right = Math.min(DESKTOP.width, Math.max(bar.x + bar.width, box.x + box.width) + 8)
        const bottom = Math.min(DESKTOP.height, box.y + box.height + 8)
        await save(page, `${lang}-query-help.png`, { clip: { x: left, y: top, width: right - left, height: bottom - top } })
      },
    },
    {
      name: 'reports',
      run: async (page) => {
        await go(page, `${P}/reports`, '#main-content h1')
        await save(page, `${lang}-reports.png`)
      },
    },
    {
      name: 'report-builder',
      run: async (page) => {
        await go(page, `${P}/reports`, '#main-content h1')
        const builder = page.locator('section[aria-labelledby="h-custom"]')
        if (!(await builder.count())) throw new Error('no custom report builder on the page')
        await builder.scrollIntoViewIfNeeded()
        await page.waitForTimeout(300)
        await save(builder, `${lang}-report-builder.png`)
      },
    },
    {
      name: 'documents',
      run: async (page) => {
        await go(page, `${P}/documents`, '#main-content h1')
        await save(page, `${lang}-documents.png`)
      },
    },
    {
      name: 'admin-members',
      run: async (page) => {
        await go(page, '/admin/members', '.p-datatable')
        await save(page, `${lang}-admin-members.png`)
      },
    },
    {
      name: 'admin-roles',
      run: async (page) => {
        try {
          await go(page, '/admin/roles', 'table')
          await save(page, `${lang}-admin-roles.png`)
        } catch {
          await go(page, '/admin/members', '.p-datatable')
          await page.getByRole('button', { name: T.roleMatrix }).click()
          await save(dialog(page), `${lang}-admin-roles.png`)
        }
      },
    },
    {
      name: 'admin-documents',
      run: async (page) => {
        await go(page, '/admin/documents', '.logo-card .logo-page img')
        await page.waitForTimeout(400)
        await save(page.locator('.logo-card'), `${lang}-admin-documents.png`)
      },
    },
    {
      name: 'my-work',
      run: async (page) => {
        await go(page, '/my-work', '#main-content h1')
        await page.waitForTimeout(600)
        await save(page, `${lang}-my-work.png`)
      },
    },
    {
      name: 'catalogue',
      run: async (page) => {
        await go(page, `${P}/catalog`, '.cat-list li')
        await page.waitForTimeout(400)
        await save(page, `${lang}-catalogue.png`)
      },
    },
    {
      name: 'admin-tokens',
      run: async (page) => {
        await go(page, '/admin/tokens', '.p-datatable')
        await save(page, `${lang}-admin-tokens.png`)
      },
    },
    {
      name: 'admin-audit',
      run: async (page) => {
        await go(page, '/admin/audit', '.p-datatable')
        await save(page, `${lang}-admin-audit.png`)
      },
    },
    {
      name: 'admin-users',
      run: async (page) => {
        await go(page, '/admin/users', '.p-datatable')
        await save(page, `${lang}-admin-users.png`)
      },
    },
    {
      name: 'admin-backup',
      run: async (page) => {
        await go(page, '/admin/backup', '#main-content h1')
        await save(page, `${lang}-admin-backup.png`)
      },
    },
    {
      name: 'theme-picker',
      run: async (page) => {
        await go(page, `${P}/dashboard`, 'canvas')
        await page.locator('.topbar .theme-picker').click()
        const list = page.locator('.p-select-overlay:visible').first()
        await list.waitFor()
        await page.waitForTimeout(300)
        const box = await list.boundingBox()
        const x = Math.max(0, box.x - 260)
        await save(page, `${lang}-theme-picker.png`, { clip: { x, y: 0, width: Math.min(DESKTOP.width - x, box.width + 520), height: Math.min(DESKTOP.height, box.y + box.height + 12) } })
      },
    },
    {
      name: 'phone-nav',
      phone: true,
      run: async (page) => {
        await go(page, `${P}/dashboard`, 'canvas')
        await page.locator('button[aria-controls="app-sidebar"]').click()
        await page.waitForSelector('.sidebar.open')
        await page.waitForTimeout(500)
        await save(page, `${lang}-phone-nav.png`)
      },
    },
    {
      name: 'phone-requirements',
      phone: true,
      run: async (page) => {
        await go(page, `${P}/requirements`, '.p-datatable')
        await save(page, `${lang}-phone-requirements.png`)
      },
    },
  ].filter((s) => !ONLY.length || ONLY.includes(s.name))
}

// ---------------------------------------------------------------- main

async function main() {
  mkdirSync(OUT, { recursive: true })
  const reqCtx = await pwRequest.newContext({ baseURL: BASE })
  const api = new Api(reqCtx)
  let me
  try {
    me = await api.post('/auth/login', { email: EMAIL, password: PASSWORD })
  } catch (e) {
    console.error(`cannot sign in at ${BASE}: ${e.message}`)
    process.exit(2)
  }
  if (!me.user?.super_admin) console.warn('! the account is not a super admin: organisation setup and admin pages may be skipped')
  const original = { prefs: me.user?.preferences ?? {}, tenant: me.tenant?.id }
  const browser = await chromium.launch({ executablePath: chromePath(), args: ['--font-render-hinting=none'] })
  console.log(`guide screenshots → ${OUT}${QUANTIZE ? ' (256 colours)' : ''}`)
  try {
    for (const lang of LANGS) {
      const D = DATA[lang]
      if (!D) {
        warn(`no tutorial data for language ${lang}`)
        continue
      }
      console.log(`\n[${lang}] preparing organisation ${D.tenant.slug} and project ${D.project.key}`)
      const t = await ensureTenant(api, me, D.tenant)
      me = t.me
      // most tutorial requirements stay in review: allow their tests to run (the runner then shows a warning)
      await api.put('/settings', { allow_unready_requirements: true }).catch(() => undefined)
      await api.patch('/auth/me', { preferences: { theme: 'light', language: lang, intro_done: true, date_format: 'iso', compact_tables: false } })
      const ids = await seedTutorial(api, D)
      me = await api.get('/auth/me')
      const state = await reqCtx.storageState()
      const noticeKey = me.previous_login ? `${me.user?.id ?? ''}|${me.previous_login.created_at}|${(me.csrf_token ?? '').slice(0, 12)}` : ''
      const init = ({ lang, noticeKey }) => {
        try {
          localStorage.setItem('qtf.lang', lang)
          localStorage.setItem('qtf.theme', 'light')
          localStorage.removeItem('qtf.decor')
          localStorage.setItem('qtf.sidebar', 'expanded')
          sessionStorage.setItem('qtf.introShown', '1')
          if (noticeKey) {
            localStorage.setItem('qtf.signin.shown', noticeKey)
            sessionStorage.setItem('qtf.signin.shown', noticeKey)
          }
        } catch {
          /* ignore */
        }
      }
      const locale = lang === 'sv' ? 'sv-SE' : 'en-GB'
      const contexts = {}
      const ctxFor = async (kind) => {
        if (contexts[kind]) return contexts[kind]
        const opts =
          kind === 'phone'
            ? { viewport: PHONE, isMobile: true, hasTouch: true, deviceScaleFactor: 2 }
            : { viewport: DESKTOP, deviceScaleFactor: 1 }
        const ctx = await browser.newContext({ ...opts, locale, timezoneId: 'Europe/Stockholm', reducedMotion: 'reduce', colorScheme: 'light', ...(kind === 'anon' ? {} : { storageState: state }) })
        await ctx.addInitScript(init, { lang, noticeKey })
        contexts[kind] = ctx
        return ctx
      }
      for (const s of shotsFor(lang, D, ids)) {
        const ctx = await ctxFor(s.anon ? 'anon' : s.phone ? 'phone' : 'desktop')
        const page = await ctx.newPage()
        try {
          await s.run(page)
        } catch (e) {
          warn(`${lang}-${s.name}: skipped (${e.message.split('\n')[0]})`)
        } finally {
          await page.close()
        }
      }
      for (const c of Object.values(contexts)) await c.close()
    }
  } finally {
    await browser.close()
    await api.patch('/auth/me', { preferences: { theme: original.prefs.theme ?? null, language: original.prefs.language ?? null, date_format: original.prefs.date_format ?? null, intro_done: original.prefs.intro_done ?? true } }).catch(() => undefined)
    if (original.tenant) await api.post('/auth/switch-tenant', { tenant_id: original.tenant }).catch(() => undefined)
    await reqCtx.dispose()
  }
  const total = done.reduce((n, d) => n + d.kb, 0)
  console.log(`\n${done.length} screenshots, ${(total / 1024).toFixed(1)} MB in total; ${skipped.length} warnings`)
  const big = done.filter((d) => d.kb > 250)
  if (big.length) console.log(`larger than 250 KB: ${big.map((d) => `${d.file} (${d.kb} KB)`).join(', ')}`)
  if (skipped.length) console.log(`warnings:\n  ${skipped.join('\n  ')}`)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
