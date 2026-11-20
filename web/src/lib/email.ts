// Pure helpers for e-mail: notification preference, invitations, report
// e-mails and scheduled report subscriptions.
import type { InvitationInfo, ReportSubscription } from './types'

/** E-mail for notifications (preference email_notifications). */
export const EMAIL_MODES = ['off', 'immediate', 'digest'] as const
export type EmailMode = (typeof EMAIL_MODES)[number]

/** A stored preference value as a mode ('' and unknown values = off). */
export function emailMode(v: unknown): EmailMode {
  return (EMAIL_MODES as readonly unknown[]).includes(v) ? (v as EmailMode) : 'off'
}

/** State of a member's invitation. */
export type InvitationState = 'none' | 'pending' | 'expired'

export function invitationState(inv: InvitationInfo | null | undefined, now: Date = new Date()): InvitationState {
  if (!inv) return 'none'
  if (inv.expired) return 'expired'
  const exp = Date.parse(inv.expires_at)
  return Number.isFinite(exp) && exp <= now.getTime() ? 'expired' : 'pending'
}

const EMAIL_RE = /^[^\s@<>(),;:"[\]\\]+@[^\s@<>(),;:"[\]\\]+\.[^\s@<>(),;:"[\]\\]+$/

/** Whether s looks like a bare e-mail address (the server checks again). */
export function isEmail(s: string): boolean {
  return s.length <= 254 && EMAIL_RE.test(s)
}

/** Addresses typed in one field (separated by commas, semicolons, spaces or lines), lower-cased and without duplicates. */
export function parseAddresses(text: string): { valid: string[]; invalid: string[] } {
  const valid: string[] = []
  const invalid: string[] = []
  for (const raw of text.split(/[\s,;]+/)) {
    const a = raw.trim().toLowerCase()
    if (!a) continue
    const list = isEmail(a) ? valid : invalid
    if (!list.includes(a)) list.push(a)
  }
  return { valid, invalid }
}

// ---------- report e-mails ----------

/** What to send: a standard template, a custom configuration or a saved report. */
export type ReportSource = { template: string; params?: Record<string, string> } | { custom: unknown } | { definition_id: string }

export interface ReportMailForm {
  recipients: string[]
  addresses: string
  message: string
  format: string
}

/** Problems of a report e-mail form (ids, see REPORT_MAIL_PROBLEMS). */
export const REPORT_MAIL_PROBLEMS = ['recipients', 'addresses', 'message'] as const
export type ReportMailProblem = (typeof REPORT_MAIL_PROBLEMS)[number]

export const MAX_MESSAGE = 4000

export function reportMailProblems(f: ReportMailForm, addressesAllowed: boolean): ReportMailProblem[] {
  const out: ReportMailProblem[] = []
  const { valid, invalid } = parseAddresses(addressesAllowed ? f.addresses : '')
  if (!f.recipients.length && !valid.length) out.push('recipients')
  if (invalid.length) out.push('addresses')
  if (f.message.length > MAX_MESSAGE) out.push('message')
  return out
}

/** Request body of POST /projects/{pid}/reports/email. */
export function reportMailBody(source: ReportSource, f: ReportMailForm, addressesAllowed: boolean) {
  const message = f.message.trim()
  const addresses = addressesAllowed ? parseAddresses(f.addresses).valid : []
  return {
    ...source,
    format: f.format,
    recipients: [...new Set(f.recipients)],
    ...(addresses.length ? { addresses } : {}),
    ...(message ? { message } : {}),
  }
}

// ---------- subscriptions ----------

export const FREQUENCIES = ['daily', 'weekly', 'monthly'] as const
export type Frequency = (typeof FREQUENCIES)[number]

/** Weekdays in display order (Monday first); values as the server: 0 = Sunday. */
export const WEEKDAYS = [1, 2, 3, 4, 5, 6, 0] as const

export interface SubscriptionForm {
  definition_id: string
  frequency: Frequency
  weekday: number
  day_of_month: number
  time: string
  timezone: string
  format: string
  recipients: string[]
  addresses: string
  message: string
  enabled: boolean
}

export function blankSubscription(definitionId = '', timezone = 'UTC'): SubscriptionForm {
  return { definition_id: definitionId, frequency: 'weekly', weekday: 1, day_of_month: 1, time: '07:00', timezone: timezone || 'UTC', format: 'pdf', recipients: [], addresses: '', message: '', enabled: true }
}

export function subscriptionForm(s: ReportSubscription): SubscriptionForm {
  return {
    definition_id: s.definition_id,
    frequency: (FREQUENCIES as readonly string[]).includes(s.frequency) ? (s.frequency as Frequency) : 'weekly',
    weekday: s.weekday ?? 1,
    day_of_month: s.day_of_month || 1,
    time: s.time || '07:00',
    timezone: s.timezone || 'UTC',
    format: s.format || 'pdf',
    recipients: [...(s.recipients ?? [])],
    addresses: (s.addresses ?? []).join(', '),
    message: s.message ?? '',
    enabled: !!s.enabled,
  }
}

export const SUBSCRIPTION_PROBLEMS = ['definition', 'time', 'day', 'recipients', 'addresses', 'message'] as const
export type SubscriptionProblem = (typeof SUBSCRIPTION_PROBLEMS)[number]

const TIME_RE = /^([01]\d|2[0-3]):[0-5]\d$/

export function subscriptionProblems(f: SubscriptionForm, addressesAllowed: boolean): SubscriptionProblem[] {
  const out: SubscriptionProblem[] = []
  if (!f.definition_id) out.push('definition')
  if (!TIME_RE.test(f.time)) out.push('time')
  if (f.frequency === 'monthly' && !(Number.isInteger(f.day_of_month) && f.day_of_month >= 1 && f.day_of_month <= 28)) out.push('day')
  const { valid, invalid } = parseAddresses(f.addresses)
  if (!f.recipients.length && !(addressesAllowed && valid.length)) out.push('recipients')
  if (invalid.length || (!addressesAllowed && valid.length)) out.push('addresses')
  if (f.message.length > MAX_MESSAGE) out.push('message')
  return out
}

/** Request body to create (with definition_id) or update a subscription. */
export function subscriptionBody(f: SubscriptionForm, create: boolean) {
  return {
    ...(create ? { definition_id: f.definition_id } : {}),
    frequency: f.frequency,
    weekday: f.weekday,
    day_of_month: f.day_of_month,
    time: f.time,
    timezone: f.timezone,
    format: f.format,
    recipients: [...new Set(f.recipients)],
    addresses: parseAddresses(f.addresses).valid,
    message: f.message.trim(),
    enabled: f.enabled,
  }
}

/** Localised weekday name (0 = Sunday). */
export function weekdayName(day: number, locale: string, style: 'long' | 'short' = 'long'): string {
  // 2023-01-01 was a Sunday
  return new Intl.DateTimeFormat(locale, { weekday: style, timeZone: 'UTC' }).format(new Date(Date.UTC(2023, 0, 1 + (((day % 7) + 7) % 7))))
}

/** Hours and minutes options "00:00" … "23:30" for a time picker. */
export function timeOptions(step = 30): string[] {
  const out: string[] = []
  for (let m = 0; m < 24 * 60; m += step) out.push(`${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`)
  return out
}

/** Severity of the last outcome of a subscription. */
export function subscriptionStatus(s: Pick<ReportSubscription, 'enabled' | 'last_error' | 'last_sent_at' | 'last_run_at'>): 'error' | 'sent' | 'paused' | 'waiting' {
  if (s.last_error) return 'error'
  if (!s.enabled) return 'paused'
  return s.last_sent_at ? 'sent' : 'waiting'
}
