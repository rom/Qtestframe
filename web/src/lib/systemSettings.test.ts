import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import {
  changedValues,
  displayValue,
  fieldId,
  formatRoleMapping,
  formValue,
  isSection,
  newProvider,
  parseRoleMapping,
  parseScopes,
  PROVIDER_NAME,
  SECTIONS,
  secretsRows,
  sectionForm,
  sourceSeverity,
  testValues,
  type SettingView,
} from './systemSettings'

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
  'server.base_url', 'files.max_upload_mb', 'requirements.auto_create_test', 'server.listen', 'database.driver',
  'auth.local', 'auth.session_ttl', 'auth.password_min_length', 'auth.login_max_attempts', 'auth.login_lockout', 'auth.cookie_secure', 'server.hsts', 'server.trusted_proxies',
  'logging.level', 'logging.format', 'logging.syslog.enabled', 'logging.syslog.network', 'logging.syslog.address', 'logging.syslog.format', 'logging.syslog.facility', 'logging.syslog.tag', 'logging.syslog.ca_file', 'logging.syslog.cert_file',
  'email.enabled', 'email.host', 'email.port', 'email.tls', 'email.username', 'email.password', 'email.from', 'email.from_name', 'email.timeout', 'email.digest_hour',
  'runner.enabled', 'runner.timeout', 'runner.max_output_bytes', 'runner.deny_network', 'runner.sandbox', 'runner.web.enabled', 'runner.web.browser', 'runner.web.headless',
  'runner.web.timeout', 'runner.web.job_timeout', 'runner.web.max_parallel', 'runner.web.max_evidence_mb', 'runner.web.screenshots', 'runner.web.trace', 'runner.web.video', 'runner.web.playwright_dir',
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
    expect(formValue(setting({ type: 'list', value: ['a', 'b'] }))).toBe('a, b')
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
