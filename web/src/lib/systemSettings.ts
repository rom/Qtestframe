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

export interface SystemSettings {
  sections: string[]
  settings: SettingView[]
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

/** Sections of the view in order; "mfa" links to the MFA page. */
export const SECTIONS = ['general', 'security', 'oidc', 'logging', 'email', 'runner', 'similarity', 'retention', 'mfa'] as const
export type SectionId = (typeof SECTIONS)[number]

/** Sections with a test button (POST /admin/settings/test/<kind>). */
export const TESTABLE: Partial<Record<SectionId, 'syslog' | 'email' | 'embeddings'>> = { logging: 'syslog', email: 'email', similarity: 'embeddings' }

/** The id of a setting in the message catalogue ("logging.level" → "logging_level"). */
export function fieldId(key: string): string {
  return key.replace(/\./g, '_')
}

export function isSection(s: unknown): s is SectionId {
  return typeof s === 'string' && (SECTIONS as readonly string[]).includes(s)
}

export type FormValue = string | number | boolean | null

/** The form model of a setting (secrets start empty: write-only). */
export function formValue(s: SettingView): FormValue {
  if (s.type === 'secret') return ''
  if (s.type === 'list') return Array.isArray(s.value) ? s.value.join(', ') : ''
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
    if (v !== formValue(s)) out[s.key] = v
  }
  return out
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
    scopes: ['openid', 'email', 'profile'],
    email_claim: 'email',
    name_claim: 'name',
    groups_claim: 'groups',
    auto_provision: false,
    default_tenant: '',
    default_role: '',
    role_mapping: null,
    local_mfa: 'off',
    mfa_amr_values: null,
    mfa_acr_values: null,
  }
}

export const PROVIDER_NAME = /^[a-z0-9][a-z0-9_-]{0,39}$/
