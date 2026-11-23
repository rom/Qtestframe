// Electronic-records mode (21 CFR Part 11 / EU GMP Annex 11) and the
// tamper-evident audit trail: types and helpers shared by the signature
// dialogs (components/ERecordsHost.vue), the signature lists and the
// administration pages.
import { ApiError } from './api'

export interface ERecordsSettings {
  enabled: boolean
  meanings: string[]
  signature_actions: string[]
  reason_edits: string[]
  idle_timeout_minutes: number
  sso_reauth_minutes: number
  updated_by?: string
  updated_at?: string
}

export interface ERecordsInfo {
  settings: ERecordsSettings
  signature_actions: string[]
  reason_edits: string[]
}

export interface ESignature {
  id: string
  project_id: string
  entity_type: string
  entity_id: string
  entity_key: string
  action: string
  meaning: string
  comment: string
  user_id: string
  user_email: string
  user_name: string
  method: string
  time_zone: string
  content_hash: string
  signed_at: string
  valid: boolean
  current_hash?: string
}

export interface AuditCheckpoint {
  id: string
  seq: number
  hash: string
  key_id: string
  created_at: string
}

export interface AuditBreak {
  seq: number
  entry_id: string
  created_at?: string
  problem: string
  message: string
}

export interface AuditVerification {
  tenant_id: string
  ok: boolean
  entries: number
  last_seq: number
  last_hash: string
  checkpoints: number
  last_checkpoint?: AuditCheckpoint
  key_protected: boolean
  unchained: number
  broken?: AuditBreak
  verified_at: string
  duration_ms: number
}

export interface ComplianceControl {
  id: string
  references: string[]
  title: string
  provider: 'tool' | 'shared' | 'customer'
  status: 'met' | 'not_configured' | 'attention' | 'procedural'
  tool: string
  customer: string
  evidence: string[] | null
}

export interface ComplianceReport {
  tenant: string
  generated_at: string
  generated_by: string
  settings: ERecordsSettings
  audit: AuditVerification
  signatures: Record<string, number>
  controls: ComplianceControl[]
  disclaimer: string
}

/** The 428 answers of electronic-records mode. */
export type ERecordsChallenge =
  | { kind: 'reason'; editType: string; message: string }
  | { kind: 'signature'; action: string; meanings: string[]; message: string }

/** The challenge of an error answer (null for any other error). */
export function challengeOf(e: unknown): ERecordsChallenge | null {
  if (!(e instanceof ApiError) || e.status !== 428) return null
  const b = (e.body ?? {}) as { edit_type?: string; action?: string; meanings?: string[] }
  if (e.code === 'reason_required') return { kind: 'reason', editType: b.edit_type ?? '', message: e.message }
  if (e.code === 'signature_required') return { kind: 'signature', action: b.action ?? '', meanings: b.meanings ?? [], message: e.message }
  return null
}

/** Header value of a reason for change (headers carry ASCII only). */
export function encodeReason(reason: string): string {
  return encodeURIComponent(reason.trim())
}

/** The browser's IANA time zone ("UTC" when unknown). */
export function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

/** "2026-11-22 09:15:02 UTC" */
export function utcStamp(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())} UTC`
}

/** The time in the signer's zone: "2026-11-22 10:15 (Europe/Stockholm)"; "" for UTC or an unknown zone. */
export function zoneStamp(iso: string, zone: string): string {
  if (!zone || zone === 'UTC') return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  try {
    const parts = new Intl.DateTimeFormat('en-CA', {
      timeZone: zone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).formatToParts(d)
    const get = (t: string) => parts.find((p) => p.type === t)?.value ?? ''
    return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')} (${zone})`
  } catch {
    return ''
  }
}

/** Known meaning codes have translations (erecords.meaning.*); others are shown as written. */
export const KNOWN_MEANINGS = ['authored', 'reviewed', 'approved', 'verified', 'released', 'accepted', 'rejected', 'responsible']

export function meaningLabel(t: (k: string) => string, m: string): string {
  return KNOWN_MEANINGS.includes(m) ? t(`erecords.meaning.${m}`) : m
}

/** Short hash for display. */
export function short(h: string | undefined, n = 12): string {
  return (h ?? '').slice(0, n)
}

/** Severity of a control status. */
export function controlSeverity(s: string): 'success' | 'warn' | 'danger' | 'secondary' | 'info' {
  switch (s) {
    case 'met':
      return 'success'
    case 'attention':
      return 'danger'
    case 'not_configured':
      return 'warn'
    default:
      return 'secondary'
  }
}

/** Lines of a textarea list (meanings), trimmed, without empty lines and duplicates. */
export function parseLines(text: string): string[] {
  const out: string[] = []
  for (const l of text.split('\n')) {
    const v = l.trim()
    if (v && !out.includes(v)) out.push(v)
  }
  return out
}

/** The message key of a signing error ("" for other errors). */
export function signErrorKey(code: string): string {
  switch (code) {
    case 'reauth_failed':
    case 'reauth_required':
      return `esign.${code}`
    case 'mfa_required':
    case 'mfa_invalid':
    case 'meaning_required':
      return `erecords.err.${code}`
  }
  return ''
}
