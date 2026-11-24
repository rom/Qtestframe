// System settings (Administration → System settings, super admins):
// types of GET /admin/settings and the form helpers of the view. Setting
// keys are dotted configuration keys ("logging.level"); their texts are
// sysSettings.fields.<id> with id = the key with "_" for "." (fieldId).

export type SettingSource = 'default' | 'file' | 'environment' | 'database'
export type SettingApply = 'runtime' | 'restart' | 'readonly'
export type SettingType = 'string' | 'int' | 'bool' | 'duration' | 'enum' | 'secret' | 'list'

export interface SettingView {
  key: string
  section: string
  type: SettingType
  apply: SettingApply
  options?: string[]
  min?: number
  max?: number
  value: unknown
  running_value?: unknown
  pending_restart: boolean
  file_value: unknown
  file_source: Exclude<SettingSource, 'database'>
  source: SettingSource
  stored: boolean
  ignored: boolean
  locked: boolean
  lock_reason?: 'environment' | 'config' | 'readonly'
  env?: string
  has_secret?: boolean
  file_has_secret?: boolean
  updated_by?: string
  updated_at?: string
}

export interface OidcProvider {
  name: string
  display_name: string
  /** icon of the sign-in button (PrimeIcons name, "" = default) */
  icon: string
  enabled: boolean
  issuer: string
  client_id: string
  client_secret?: string
  scopes: string[]
  email_claim: string
  name_claim: string
  groups_claim: string
  auto_provision: boolean
  default_tenant: string
  default_role: string
  role_mapping: Record<string, string> | null
  /** e-mail domains that may sign in (empty = any) */
  allowed_domains: string[] | null
  /** local second factor after this provider's sign-in */
  local_mfa: '' | 'off' | 'unless_idp_mfa' | 'always'
  mfa_amr_values: string[] | null
  mfa_acr_values: string[] | null
}

export interface OidcProviderView extends OidcProvider {
  has_secret: boolean
  secret_source: '' | 'file' | 'environment' | 'database'
  secret_env: string
  in_file: boolean
  callback_url: string
}

/** A SAML provider of the configuration file (read-only). */
export interface SamlProviderView {
  name: string
  display_name: string
  entity_id: string
  metadata_url: string
  metadata_file: string
  sp_cert_file: string
  email_attr: string
  name_attr: string
  groups_attr: string
  auto_provision: boolean
  default_tenant: string
  default_role: string
  role_mapping: Record<string, string> | null
  allowed_domains: string[]
  acs_url: string
  sp_metadata_url: string
}

export interface SystemSettings {
  sections: string[]
  settings: SettingView[]
  saml: SamlProviderView[]
  oidc_icons: string[]
  oidc: {
    providers: OidcProviderView[]
    source: SettingSource
    file_source: string
    stored: boolean
    locked: boolean
    lock_reason?: string
    updated_by?: string
    updated_at?: string
  }
  locked_patterns: string[]
  config_file: string
  restart_pending: boolean
  load_error?: string
  trash_retention_days: number
  logging_runtime: boolean
  /** encryption of stored secrets (read-only; section secrets of the file) */
  secrets?: SecretsInfo
  /** state of metrics and tracing (section monitoring) */
  monitoring?: MonitoringInfo
}

/** The state of the Prometheus endpoint and of the trace export. */
export interface MonitoringInfo {
  metrics_enabled: boolean
  /** separate address of /metrics in use ("" = the main listener) */
  metrics_listen: string
  metrics_protection: MetricsProtection[]
  tracing_enabled: boolean
  tracing_endpoint: string
  sample_ratio: number
  spans: {
    exported: number
    dropped: number
    failed: number
    last_export?: string
    last_error?: string
    last_error_at?: string
    queue_length: number
    queue_maximum: number
  }
}

export type MetricsProtection = 'token' | 'allowlist' | 'loopback'

/** A sample ratio as a percentage for display (0.1 → "10", 0.005 → "0.5"). */
export function ratioPercent(ratio: number): string {
  const pct = Math.round(Math.min(Math.max(ratio, 0), 1) * 10000) / 100
  return String(pct)
}

/** Whether the trace export needs attention: on, and its last error is newer than its last success. */
export function tracingFailing(m: MonitoringInfo): boolean {
  if (!m.tracing_enabled || !m.spans.last_error_at) return false
  if (!m.spans.last_export) return true
  return Date.parse(m.spans.last_error_at) > Date.parse(m.spans.last_export)
}

/** The key provider protecting the stored secrets (GET /admin/settings). */
export interface SecretsInfo {
  provider: 'local' | 'tpm' | 'pkcs11'
  key_id: string
  info: Record<string, string>
  active_key: string
  keys: number
  encrypted: number
  plaintext: number
}

/** Details of a key provider, in display order (keys of SecretsInfo.info). */
export const SECRETS_INFO = ['key_source', 'key_file', 'device', 'pcrs', 'module', 'token', 'slot', 'key_label'] as const

/** Rows describing the key provider; values of key_source are message ids. */
export function secretsRows(s: SecretsInfo): { id: (typeof SECRETS_INFO)[number]; value: string; message: boolean }[] {
  return SECRETS_INFO.filter((id) => s.info[id]).map((id) => ({ id, value: s.info[id], message: id === 'key_source' }))
}

export interface TestResult {
  ok: boolean
  message?: string
  error?: string
  details?: Record<string, unknown>
}

/**
 * Sections of the view in order, in groups of the side navigation.
 * "authorisation" and "mfa" have no settings of their own: they explain
 * and link to the pages that manage them.
 */
export const SECTION_GROUPS = {
  server: ['general', 'server', 'logging', 'monitoring', 'retention'],
  access: ['security', 'tokens', 'oidc', 'mfa', 'authorisation'],
  services: ['email', 'outbound', 'runner', 'live', 'similarity', 'ai'],
} as const
export const SECTIONS = [...SECTION_GROUPS.server, ...SECTION_GROUPS.access, ...SECTION_GROUPS.services] as const
export type SectionId = (typeof SECTIONS)[number]
/** Sections without settings in the registry (shown with their own content). */
export const INFO_SECTIONS: readonly SectionId[] = ['mfa', 'authorisation', 'retention']

/** Sections with a test button (POST /admin/settings/test/<kind>). */
export const TESTABLE: Partial<Record<SectionId, 'syslog' | 'email' | 'embeddings' | 'ai' | 'outbound'>> = { logging: 'syslog', email: 'email', similarity: 'embeddings', ai: 'ai', outbound: 'outbound' }

/** The id of a setting in the message catalogue ("logging.level" → "logging_level"). */
export function fieldId(key: string): string {
  return key.replace(/\./g, '_')
}

export function isSection(s: unknown): s is SectionId {
  return typeof s === 'string' && (SECTIONS as readonly string[]).includes(s)
}

export type FormValue = string | number | boolean | null

/** The form model of a setting (secrets start empty: write-only; lists one entry per line). */
export function formValue(s: SettingView): FormValue {
  if (s.type === 'secret') return ''
  if (s.type === 'list') return Array.isArray(s.value) ? s.value.join('\n') : ''
  if (s.type === 'int') return typeof s.value === 'number' ? s.value : Number(s.value ?? 0)
  if (s.type === 'bool') return s.value === true
  return s.value == null ? '' : String(s.value)
}

/** Form models of the settings of a section, by key. */
export function sectionForm(settings: SettingView[], section: string): Record<string, FormValue> {
  const out: Record<string, FormValue> = {}
  for (const s of settings) if (s.section === section) out[s.key] = formValue(s)
  return out
}

/**
 * The values to send for a section: changed, editable settings only. A
 * typed secret is sent; removeSecret lists secrets to remove (sent as "").
 */
export function changedValues(settings: SettingView[], section: string, form: Record<string, FormValue>, removeSecret: Record<string, boolean> = {}): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const s of settings) {
    if (s.section !== section || s.locked || !(s.key in form)) continue
    const v = form[s.key]
    if (s.type === 'secret') {
      if (typeof v === 'string' && v !== '') out[s.key] = v
      else if (removeSecret[s.key]) out[s.key] = ''
      continue
    }
    if (s.type === 'string' || s.type === 'duration') {
      if (String(v ?? '').trim() !== String(formValue(s)).trim()) out[s.key] = String(v ?? '').trim()
      continue
    }
    if (s.type === 'list') {
      const next = parseList(String(v ?? ''))
      if (next.join('\n') !== parseList(String(formValue(s))).join('\n')) out[s.key] = next
      continue
    }
    if (v !== formValue(s)) out[s.key] = v
  }
  return out
}

/** List text (lines, commas or spaces) → entries, trimmed and without duplicates. */
export function parseList(text: string): string[] {
  const out: string[] = []
  for (const s of text.split(/[\n,\s]+/)) if (s && !out.includes(s)) out.push(s)
  return out
}

/** One line of the confirmation before saving: a setting and its old and new value. */
export interface SettingChange {
  key: string
  section: string
  from: string
  to: string
  secret: boolean
  apply: SettingApply
}

/** A value as text for the diff ("" for empty; lists comma-separated). */
export function valueText(v: unknown): string {
  if (v == null) return ''
  if (Array.isArray(v)) return v.join(', ')
  return String(v)
}

/** The changes values make to the settings, for the confirmation dialog (secrets without values). */
export function settingsDiff(settings: SettingView[], values: Record<string, unknown>): SettingChange[] {
  const out: SettingChange[] = []
  for (const s of settings) {
    if (!(s.key in values)) continue
    const secret = s.type === 'secret'
    out.push({ key: s.key, section: s.section, from: secret ? '' : valueText(s.value), to: secret ? '' : valueText(values[s.key]), secret, apply: s.apply })
  }
  return out
}

/** Settings whose saved value applies after the next restart (running value differs). */
export function pendingRestart(settings: SettingView[]): SettingView[] {
  return settings.filter((s) => s.pending_restart)
}

/** A search hit: a setting (or a section) matching the query. */
export interface SearchHit {
  key: string
  section: string
}

/**
 * Settings matching a query (all words, case-insensitive) in their key,
 * label, hint or value; text(key) returns the label and hint of a setting.
 */
export function searchSettings(settings: SettingView[], query: string, text: (key: string) => string): SearchHit[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (!words.length) return []
  const out: SearchHit[] = []
  for (const s of settings) {
    const hay = `${s.key} ${text(s.key)} ${s.type === 'secret' ? '' : valueText(s.value)} ${s.env ?? ''}`.toLowerCase()
    if (words.every((w) => hay.includes(w))) out.push({ key: s.key, section: s.section })
  }
  return out
}

/** The JSON file of "Export settings": the values changed here, without secrets. */
export interface SettingsFile {
  format: 'qtestframe-settings'
  version: 1
  exported_at: string
  values: Record<string, unknown>
}

/** The settings changed here (stored, applied), without secrets and the OIDC provider list. */
export function exportSettings(settings: SettingView[], now = new Date()): SettingsFile {
  const values: Record<string, unknown> = {}
  for (const s of settings) {
    if (!s.stored || s.ignored || s.locked || s.type === 'secret') continue
    values[s.key] = s.value
  }
  return { format: 'qtestframe-settings', version: 1, exported_at: now.toISOString(), values }
}

/** What an imported file would change; problems name keys that are skipped. */
export interface ImportPlan {
  values: Record<string, unknown>
  unknown: string[]
  locked: string[]
  secrets: string[]
  unchanged: number
}

/** Reads an exported settings file (text) against the current settings; throws on a file of another format. */
export function importPlan(settings: SettingView[], text: string): ImportPlan {
  const doc = JSON.parse(text) as Partial<SettingsFile>
  if (!doc || doc.format !== 'qtestframe-settings' || typeof doc.values !== 'object' || doc.values === null) throw new Error('format')
  const byKey = new Map(settings.map((s) => [s.key, s]))
  const plan: ImportPlan = { values: {}, unknown: [], locked: [], secrets: [], unchanged: 0 }
  for (const [k, v] of Object.entries(doc.values)) {
    const s = byKey.get(k)
    if (!s) plan.unknown.push(k)
    else if (s.type === 'secret') plan.secrets.push(k)
    else if (s.locked) plan.locked.push(k)
    else if (valueText(v) === valueText(s.value)) plan.unchanged++
    else plan.values[k] = v
  }
  return plan
}

/** Values of a section for a test button (unsaved form values, secrets only when typed). */
export function testValues(settings: SettingView[], section: string, form: Record<string, FormValue>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const s of settings) {
    if (s.section !== section || s.locked || !(s.key in form)) continue
    const v = form[s.key]
    if (s.type === 'secret' && (v === '' || v == null)) continue
    out[s.key] = typeof v === 'string' ? v.trim() : v
  }
  return out
}

/** A value for display ("" → null, booleans and lists as given). */
export function displayValue(v: unknown): string | null {
  if (v == null || v === '') return null
  if (Array.isArray(v)) return v.length ? v.join(', ') : null
  return String(v)
}

/** Severity of the source badge. */
export function sourceSeverity(s: SettingSource): 'secondary' | 'info' | 'warn' | 'success' {
  return ({ default: 'secondary', file: 'info', environment: 'warn', database: 'success' } as const)[s]
}

/** Scopes typed as text (spaces or commas) → list. */
export function parseScopes(text: string): string[] {
  const out: string[] = []
  for (const s of text.split(/[\s,]+/)) if (s && !out.includes(s)) out.push(s)
  return out
}

/** "group=role" lines → mapping; bad is the 1-based number of the first bad line. */
export function parseRoleMapping(text: string): { mapping: Record<string, string>; bad: number } {
  const mapping: Record<string, string> = {}
  const lines = text.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line) continue
    const eq = line.indexOf('=')
    const group = eq > 0 ? line.slice(0, eq).trim() : ''
    const role = eq > 0 ? line.slice(eq + 1).trim() : ''
    if (!group || !role) return { mapping, bad: i + 1 }
    mapping[group] = role
  }
  return { mapping, bad: 0 }
}

export function formatRoleMapping(m: Record<string, string> | null | undefined): string {
  return Object.entries(m ?? {})
    .map(([g, r]) => `${g}=${r}`)
    .join('\n')
}

/** An empty provider for "Add provider". */
export function newProvider(): OidcProvider {
  return {
    name: '',
    display_name: '',
    enabled: true,
    issuer: '',
    client_id: '',
    client_secret: '',
    icon: '',
    scopes: ['openid', 'email', 'profile'],
    email_claim: 'email',
    name_claim: 'name',
    groups_claim: 'groups',
    auto_provision: false,
    default_tenant: '',
    default_role: '',
    role_mapping: null,
    allowed_domains: [],
    local_mfa: 'off',
    mfa_amr_values: null,
    mfa_acr_values: null,
  }
}

export const PROVIDER_NAME = /^[a-z0-9][a-z0-9_-]{0,39}$/
