import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import {
  changedValues,
  displayValue,
  exportSettings,
  fieldId,
  formatRoleMapping,
  formValue,
  importPlan,
  isSection,
  newProvider,
  parseList,
  parseRoleMapping,
  parseScopes,
  pendingRestart,
  PROVIDER_NAME,
  ratioPercent,
  searchSettings,
  SECTION_GROUPS,
  SECTIONS,
  secretsRows,
  sectionForm,
  settingsDiff,
  sourceSeverity,
  testValues,
  tracingFailing,
  type MonitoringInfo,
  type SettingView,
} from './systemSettings'

describe('monitoring', () => {
  const base: MonitoringInfo = {
    metrics_enabled: true,
    metrics_listen: '',
    metrics_protection: ['loopback'],
    tracing_enabled: true,
    tracing_endpoint: 'http://otel:4318/v1/traces',
    sample_ratio: 1,
    spans: { exported: 0, dropped: 0, failed: 0, queue_length: 0, queue_maximum: 4096 },
  }
  it('shows the sample ratio as a percentage', () => {
    expect(ratioPercent(1)).toBe('100')
    expect(ratioPercent(0.1)).toBe('10')
    expect(ratioPercent(0.005)).toBe('0.5')
    expect(ratioPercent(2)).toBe('100')
    expect(ratioPercent(-1)).toBe('0')
  })
  it('flags an export whose last error is newer than its last success', () => {
    expect(tracingFailing(base)).toBe(false)
    expect(tracingFailing({ ...base, spans: { ...base.spans, last_error: 'x', last_error_at: '2026-11-24T10:00:00Z' } })).toBe(true)
    expect(tracingFailing({ ...base, spans: { ...base.spans, last_error: 'x', last_error_at: '2026-11-24T10:00:00Z', last_export: '2026-11-24T10:01:00Z' } })).toBe(false)
    expect(tracingFailing({ ...base, spans: { ...base.spans, last_error: 'x', last_error_at: '2026-11-24T10:02:00Z', last_export: '2026-11-24T10:01:00Z' } })).toBe(true)
    expect(tracingFailing({ ...base, tracing_enabled: false, spans: { ...base.spans, last_error_at: '2026-11-24T10:00:00Z' } })).toBe(false)
  })
  it('is a section of the server group with texts for every protection', () => {
    expect(SECTION_GROUPS.server).toContain('monitoring')
    for (const p of ['token', 'allowlist', 'loopback'] as const) expect(en.sysSettings.monitoring.protectionKinds[p]).toBeTruthy()
  })
})

describe('stored secrets', () => {
  it('lists the provider details in order and marks the key source as a message', () => {
    const rows = secretsRows({
      provider: 'local',
      key_id: 'local:ab',
      info: { key_file: '/var/lib/qtestframe/qtestframe-secrets.key', key_source: 'file', unknown: 'x' },
      active_key: 'k1',
      keys: 1,
      encrypted: 2,
      plaintext: 0,
    })
    expect(rows).toEqual([
      { id: 'key_source', value: 'file', message: true },
      { id: 'key_file', value: '/var/lib/qtestframe/qtestframe-secrets.key', message: false },
    ])
  })
})

function setting(p: Partial<SettingView>): SettingView {
  return {
    key: 'x.y',
    section: 'logging',
    type: 'string',
    apply: 'runtime',
    value: '',
    pending_restart: false,
    file_value: '',
    file_source: 'default',
    source: 'default',
    stored: false,
    ignored: false,
    locked: false,
    ...p,
  }
}

// the keys of internal/settings.Registry (GET /admin/settings)
const SERVER_KEYS = [
  'server.base_url', 'files.max_upload_mb', 'requirements.auto_create_test',
  'server.listen', 'server.tls_cert', 'server.tls_key', 'server.read_timeout', 'server.write_timeout', 'server.frame_ancestors', 'server.trusted_proxies', 'server.hsts',
  'database.driver', 'database.dsn', 'database.max_open_conns', 'bootstrap.tenant_slug', 'bootstrap.tenant_name', 'bootstrap.admin_email',
  'hardening.no_new_privs', 'hardening.landlock', 'hardening.seccomp', 'hardening.drop_capabilities', 'hardening.read_only_paths', 'hardening.read_write_paths', 'secrets.provider',
  'auth.local', 'auth.session_ttl', 'auth.session_idle_timeout', 'auth.password_min_length', 'auth.password_min_classes', 'auth.password_history', 'auth.password_max_age_days',
  'auth.login_max_attempts', 'auth.login_lockout', 'auth.cookie_secure', 'auth.token_max_days', 'auth.token_require_expiry',
  'logging.level', 'logging.format', 'logging.syslog.enabled', 'logging.syslog.network', 'logging.syslog.address', 'logging.syslog.format', 'logging.syslog.facility', 'logging.syslog.tag', 'logging.syslog.ca_file', 'logging.syslog.cert_file', 'logging.syslog.key_file',
  'metrics.enabled', 'metrics.listen', 'metrics.token', 'metrics.allowed_ips', 'tracing.enabled', 'tracing.endpoint', 'tracing.headers', 'tracing.sample_ratio', 'tracing.service_name',
  'email.enabled', 'email.host', 'email.port', 'email.tls', 'email.username', 'email.password', 'email.from', 'email.from_name', 'email.reply_to', 'email.timeout', 'email.digest_hour',
  'outbound.proxy', 'outbound.no_proxy', 'outbound.allowed_hosts', 'outbound.denied_hosts', 'outbound.block_private',
  'runner.enabled', 'runner.timeout', 'runner.max_output_bytes', 'runner.deny_network', 'runner.sandbox', 'runner.work_dir', 'runner.interpreters', 'runner.web.enabled', 'runner.web.browser', 'runner.web.channel', 'runner.web.browser_channels', 'runner.web.headless',
  'runner.web.timeout', 'runner.web.job_timeout', 'runner.web.max_parallel', 'runner.web.max_evidence_mb', 'runner.web.screenshots', 'runner.web.trace', 'runner.web.video',
  'runner.web.node', 'runner.web.playwright_dir', 'runner.web.command', 'runner.web.browsers_path', 'runner.web.executable_path',
  'live.enabled', 'live.heartbeat', 'live.presence_ttl', 'live.max_connections', 'live.max_connections_per_user', 'live.replay_buffer',
  'similarity.embeddings.enabled', 'similarity.embeddings.url', 'similarity.embeddings.model', 'similarity.embeddings.api_key', 'similarity.embeddings.timeout',
  'ai.enabled', 'ai.provider', 'ai.url', 'ai.model', 'ai.api_key', 'ai.timeout', 'ai.max_tokens', 'ai.temperature', 'ai.rate_limit',
  'ai.features.draft_tests', 'ai.features.improve_requirement', 'ai.features.summarize_run', 'ai.features.suggest_mappings', 'ai.features.suggest_criteria',
  'ai.price_input', 'ai.price_output',
]

describe('system settings texts', () => {
  it('has a label and a hint for every setting and a name for every section', () => {
    const fields = en.sysSettings.fields as Record<string, { label: string; hint: string }>
    expect(SERVER_KEYS.filter((k) => !fields[fieldId(k)]?.label || !fields[fieldId(k)]?.hint)).toEqual([])
    expect(Object.keys(fields).sort()).toEqual(SERVER_KEYS.map(fieldId).sort())
    const s = en.sysSettings
    for (const id of SECTIONS) {
      expect(s.section[id]).toBeTruthy()
      expect(s.sectionIntro[id]).toBeTruthy()
    }
    for (const g of Object.keys(SECTION_GROUPS) as (keyof typeof SECTION_GROUPS)[]) expect(s.group[g]).toBeTruthy()
    expect(new Set(SECTIONS).size).toBe(SECTIONS.length)
    for (const src of ['default', 'file', 'environment', 'database'] as const) expect(s.source[src]).toBeTruthy()
  })
})

describe('system settings forms', () => {
  it('maps keys and sections', () => {
    expect(fieldId('logging.syslog.address')).toBe('logging_syslog_address')
    expect(isSection('oidc')).toBe(true)
    expect(isSection('nope')).toBe(false)
    expect(isSection(1)).toBe(false)
  })
  it('builds form values', () => {
    expect(formValue(setting({ type: 'secret', value: null }))).toBe('')
    expect(formValue(setting({ type: 'list', value: ['a', 'b'] }))).toBe('a\nb')
    expect(formValue(setting({ type: 'list', value: null }))).toBe('')
    expect(formValue(setting({ type: 'int', value: 5 }))).toBe(5)
    expect(formValue(setting({ type: 'int', value: '7' }))).toBe(7)
    expect(formValue(setting({ type: 'bool', value: true }))).toBe(true)
    expect(formValue(setting({ type: 'duration', value: '12h' }))).toBe('12h')
    expect(formValue(setting({ value: null }))).toBe('')
  })
  it('sends only changed, editable values', () => {
    const list = [
      setting({ key: 'a', value: 'x' }),
      setting({ key: 'b', type: 'int', value: 3 }),
      setting({ key: 'c', type: 'bool', value: false }),
      setting({ key: 'd', type: 'secret', value: null, has_secret: true }),
      setting({ key: 'e', locked: true, value: 'l' }),
      setting({ key: 'f', type: 'duration', value: '5m' }),
      setting({ key: 'g', section: 'email', value: 'other' }),
      setting({ key: 'h', type: 'secret', value: null }),
    ]
    const form = sectionForm(list, 'logging')
    expect(form).toEqual({ a: 'x', b: 3, c: false, d: '', e: 'l', f: '5m', h: '' })
    expect(changedValues(list, 'logging', form)).toEqual({})
    form.a = ' y '
    form.b = 4
    form.c = true
    form.d = 'new-secret'
    form.e = 'changed'
    form.f = ' 5m '
    expect(changedValues(list, 'logging', form)).toEqual({ a: 'y', b: 4, c: true, d: 'new-secret' })
    form.d = ''
    expect(changedValues(list, 'logging', form, { d: true })).toEqual({ a: 'y', b: 4, c: true, d: '' })
    expect(changedValues(list, 'logging', { a: null })).toEqual({ a: '' })
    form.a = ' y '
    expect(testValues(list, 'logging', form)).toEqual({ a: 'y', b: 4, c: true, f: '5m' })
    form.h = 'typed'
    expect(testValues(list, 'logging', form).h).toBe('typed')
  })
  it('formats values and badges', () => {
    expect(displayValue('')).toBeNull()
    expect(displayValue(null)).toBeNull()
    expect(displayValue([])).toBeNull()
    expect(displayValue(['a', 'b'])).toBe('a, b')
    expect(displayValue(5)).toBe('5')
    expect(sourceSeverity('database')).toBe('success')
    expect(sourceSeverity('environment')).toBe('warn')
  })
})

describe('system settings lists, search, diff and files', () => {
  it('edits lists one entry per line', () => {
    expect(parseList(' a.example.com\n\n*.b.example.com, a.example.com c ')).toEqual(['a.example.com', '*.b.example.com', 'c'])
    const list = [setting({ key: 'outbound.allowed_hosts', section: 'outbound', type: 'list', value: ['a', 'b'] })]
    const form = sectionForm(list, 'outbound')
    expect(form['outbound.allowed_hosts']).toBe('a\nb')
    expect(changedValues(list, 'outbound', form)).toEqual({})
    form['outbound.allowed_hosts'] = 'a, b'
    expect(changedValues(list, 'outbound', form)).toEqual({})
    form['outbound.allowed_hosts'] = 'b\nc'
    expect(changedValues(list, 'outbound', form)).toEqual({ 'outbound.allowed_hosts': ['b', 'c'] })
    form['outbound.allowed_hosts'] = ''
    expect(changedValues(list, 'outbound', form)).toEqual({ 'outbound.allowed_hosts': [] })
  })
  it('searches all sections by key, label, hint and value', () => {
    const list = [
      setting({ key: 'logging.level', section: 'logging', value: 'info' }),
      setting({ key: 'email.host', section: 'email', value: 'smtp.example.com' }),
      setting({ key: 'ai.api_key', section: 'ai', type: 'secret', value: 'never-shown' }),
      setting({ key: 'auth.session_idle_timeout', section: 'security', value: '30m', env: 'QTF_X' }),
    ]
    const text = (k: string) => ({ 'logging.level': 'Log level debug info', 'email.host': 'SMTP host', 'ai.api_key': 'API key', 'auth.session_idle_timeout': 'Inactivity timeout' })[k] ?? ''
    expect(searchSettings(list, '', text)).toEqual([])
    expect(searchSettings(list, 'SMTP', text)).toEqual([{ key: 'email.host', section: 'email' }])
    expect(searchSettings(list, 'example.com', text).map((h) => h.key)).toEqual(['email.host'])
    expect(searchSettings(list, 'never-shown', text)).toEqual([])
    expect(searchSettings(list, 'timeout 30m', text).map((h) => h.key)).toEqual(['auth.session_idle_timeout'])
    expect(searchSettings(list, 'qtf_x', text).map((h) => h.key)).toEqual(['auth.session_idle_timeout'])
    expect(searchSettings(list, 'level nope', text)).toEqual([])
  })
  it('lists the changes before saving, secrets without values', () => {
    const list = [
      setting({ key: 'a', value: 'x' }),
      setting({ key: 'b', type: 'secret', value: null }),
      setting({ key: 'c', type: 'list', value: ['p'], apply: 'restart' }),
      setting({ key: 'd', value: 'untouched' }),
    ]
    expect(settingsDiff(list, { a: 'y', b: 'top-secret', c: ['p', 'q'] })).toEqual([
      { key: 'a', section: 'logging', from: 'x', to: 'y', secret: false, apply: 'runtime' },
      { key: 'b', section: 'logging', from: '', to: '', secret: true, apply: 'runtime' },
      { key: 'c', section: 'logging', from: 'p', to: 'p, q', secret: false, apply: 'restart' },
    ])
    expect(pendingRestart([setting({ key: 'r', pending_restart: true }), setting({ key: 's' })]).map((s) => s.key)).toEqual(['r'])
  })
  it('exports the values changed here without secrets and imports them back', () => {
    const list = [
      setting({ key: 'a', value: 'x', stored: true }),
      setting({ key: 'b', type: 'secret', value: null, stored: true }),
      setting({ key: 'c', value: 'file', stored: false }),
      setting({ key: 'd', value: 'locked', stored: true, locked: true }),
      setting({ key: 'e', value: 'ignored', stored: true, ignored: true }),
      setting({ key: 'f', type: 'int', value: 3, stored: true }),
    ]
    const file = exportSettings(list, new Date('2026-01-02T03:04:05Z'))
    expect(file).toEqual({ format: 'qtestframe-settings', version: 1, exported_at: '2026-01-02T03:04:05.000Z', values: { a: 'x', f: 3 } })
    const plan = importPlan(list, JSON.stringify({ ...file, values: { a: 'x', c: 'new', b: 'secret', d: 'z', zz: 1, f: 4 } }))
    expect(plan).toEqual({ values: { c: 'new', f: 4 }, unknown: ['zz'], locked: ['d'], secrets: ['b'], unchanged: 1 })
    expect(() => importPlan(list, '{"values": {}}')).toThrow()
    expect(() => importPlan(list, 'not json')).toThrow()
  })
})

describe('OIDC provider form', () => {
  it('parses scopes and role mappings', () => {
    expect(parseScopes('openid, email  profile,email')).toEqual(['openid', 'email', 'profile'])
    expect(parseScopes('')).toEqual([])
    expect(parseRoleMapping('qa = test_lead\n\nadmins=admin')).toEqual({ mapping: { qa: 'test_lead', admins: 'admin' }, bad: 0 })
    expect(parseRoleMapping('ok=a\nbroken').bad).toBe(2)
    expect(parseRoleMapping('=x').bad).toBe(1)
    expect(formatRoleMapping({ a: 'b', c: 'd' })).toBe('a=b\nc=d')
    expect(formatRoleMapping(null)).toBe('')
  })
  it('starts a new provider with the usual claims and validates names', () => {
    const p = newProvider()
    expect(p.enabled).toBe(true)
    expect(p.scopes).toContain('openid')
    expect(PROVIDER_NAME.test('corp-sso_1')).toBe(true)
    expect(PROVIDER_NAME.test('Corp')).toBe(false)
    expect(PROVIDER_NAME.test('')).toBe(false)
  })
})
