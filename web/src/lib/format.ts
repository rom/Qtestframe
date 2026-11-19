// Formatting helpers.
import { shallowRef } from 'vue'

/** "must_not" → "Must not", "non_functional" → "Non functional". */
export function humanize(v: string | null | undefined): string {
  if (!v) return ''
  const words = v.replace(/[_-]+/g, ' ').trim().split(/\s+/)
  return words.map((w, i) => (i === 0 ? w.charAt(0).toUpperCase() + w.slice(1) : w)).join(' ')
}

function toDate(v: string | number | Date | null | undefined): Date | null {
  if (v === null || v === undefined || v === '') return null
  const d = v instanceof Date ? v : new Date(v)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1) return null
  return d
}

export function formatDate(v: string | number | Date | null | undefined): string {
  const p = datePrefs.value
  if (p.date_format || p.timezone) return formatDatePref(v, p, new Date(), formatLocale.value)
  const d = toDate(v)
  if (!d) return ''
  return d.toISOString().slice(0, 10)
}

export function formatDateTime(v: string | number | Date | null | undefined): string {
  const p = datePrefs.value
  if (p.date_format || p.timezone) return formatDateTimePref(v, p, new Date(), formatLocale.value)
  const d = toDate(v)
  if (!d) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// ---------- user preferences (date_format, timezone) ----------

export interface DatePrefs {
  /** locale | iso | relative; empty = iso-like default */
  date_format?: string
  /** IANA zone; empty = browser */
  timezone?: string
}

/** Active preferences used by formatDate / formatDateTime (reactive for templates). */
const datePrefs = shallowRef<DatePrefs>({})

export function setDatePrefs(p: DatePrefs | null | undefined): void {
  datePrefs.value = { date_format: p?.date_format || '', timezone: p?.timezone || '' }
}

export function getDatePrefs(): DatePrefs {
  return datePrefs.value
}

// ---------- active locale (BCP 47 tag used for Intl formatting) ----------

const formatLocale = shallowRef('en')

/** Set the locale used by the helpers below when no locale is passed (i18n calls this). */
export function setFormatLocale(tag: string | null | undefined): void {
  formatLocale.value = tag || 'en'
}

export function getFormatLocale(): string {
  return formatLocale.value
}

function isEnglish(locale: string | undefined): boolean {
  return !locale || /^en\b/i.test(locale)
}

/** Locale-aware number, e.g. 1234.5 → "1,234.5" (en) / "1 234,5" (sv). */
export function formatNumber(n: number | null | undefined, locale: string = formatLocale.value, opts?: Intl.NumberFormatOptions): string {
  if (n === null || n === undefined || Number.isNaN(n)) return ''
  try {
    return new Intl.NumberFormat(locale, opts).format(n)
  } catch {
    return String(n)
  }
}

/** Whether the runtime knows the IANA time zone. */
export function isValidTimeZone(tz: string | null | undefined): boolean {
  if (!tz) return false
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: tz })
    return true
  } catch {
    return false
  }
}

function zone(tz: string | undefined): string | undefined {
  return tz && isValidTimeZone(tz) ? tz : undefined
}

/** Calendar parts of `d` in the given zone (browser zone when undefined). */
export function zonedParts(d: Date, tz?: string): { year: number; month: number; day: number; hour: number; minute: number } {
  const z = zone(tz)
  if (!z) return { year: d.getFullYear(), month: d.getMonth() + 1, day: d.getDate(), hour: d.getHours(), minute: d.getMinutes() }
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: z,
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
    hourCycle: 'h23',
  }).formatToParts(d)
  const get = (t: string) => Number(parts.find((p) => p.type === t)?.value ?? 0)
  return { year: get('year'), month: get('month'), day: get('day'), hour: get('hour') % 24, minute: get('minute') }
}

const pad2 = (n: number) => String(n).padStart(2, '0')

/**
 * Format a timestamp according to the user's preferences:
 * `iso` (default) "2026-01-02 03:04", `locale` (Intl medium date + short
 * time in `locale`), `relative` ("5 min ago" / "för 5 minuter sedan"); all
 * in `prefs.timezone` when valid.
 */
export function formatDateTimePref(value: string | number | Date | null | undefined, prefs: DatePrefs | null | undefined, now: Date = new Date(), locale = 'en'): string {
  const d = toDate(value)
  if (!d) return ''
  const tz = zone(prefs?.timezone)
  switch (prefs?.date_format) {
    case 'relative':
      return relativeTime(d, now, true, locale)
    case 'locale':
      return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short', timeZone: tz }).format(d)
    default: {
      const p = zonedParts(d, tz)
      return `${p.year}-${pad2(p.month)}-${pad2(p.day)} ${pad2(p.hour)}:${pad2(p.minute)}`
    }
  }
}

/** Date-only variant of formatDateTimePref. */
export function formatDatePref(value: string | number | Date | null | undefined, prefs: DatePrefs | null | undefined, now: Date = new Date(), locale = 'en'): string {
  const d = toDate(value)
  if (!d) return ''
  const tz = zone(prefs?.timezone)
  switch (prefs?.date_format) {
    case 'relative':
      return relativeTime(d, now, true, locale)
    case 'locale':
      return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: tz }).format(d)
    default: {
      const p = zonedParts(d, tz)
      return `${p.year}-${pad2(p.month)}-${pad2(p.day)}`
    }
  }
}

/** Unambiguous full timestamp (for tooltips next to relative dates). */
export function formatFullDateTime(value: string | number | Date | null | undefined, prefs: DatePrefs | null | undefined = datePrefs.value): string {
  const d = toDate(value)
  if (!d) return ''
  return formatDateTimePref(d, { timezone: prefs?.timezone, date_format: 'iso' }) + (zone(prefs?.timezone) ? ` (${prefs?.timezone})` : '')
}

const JUST_NOW: Record<string, string> = { en: 'just now', sv: 'just nu' }

/**
 * "3 minutes ago", "in 2 days", "just now" (Intl.RelativeTimeFormat in
 * `locale`, e.g. sv "för 3 minuter sedan"); `short` writes "3 min ago" in
 * English.
 */
export function relativeTime(v: string | number | Date | null | undefined, now: Date = new Date(), short = false, locale: string = formatLocale.value): string {
  const d = toDate(v)
  if (!d) return ''
  const diff = (d.getTime() - now.getTime()) / 1000
  const abs = Math.abs(diff)
  const base = (locale || 'en').toLowerCase().split(/[-_]/)[0]
  if (abs < 45) return JUST_NOW[base] ?? new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(0, 'second')
  const units: [number, Intl.RelativeTimeFormatUnit][] = [
    [60, 'minute'],
    [3600, 'hour'],
    [86400, 'day'],
    [2592000, 'month'],
    [31536000, 'year'],
  ]
  let unit = units[0]
  for (const u of units) if (abs >= u[0] * 0.75) unit = u
  const n = Math.max(1, Math.round(abs / unit[0]))
  if (short && unit[1] === 'minute' && isEnglish(locale)) return diff < 0 ? `${n} min ago` : `in ${n} min`
  return new Intl.RelativeTimeFormat(locale, { numeric: 'always' }).format(diff < 0 ? -n : n, unit[1])
}

/** 1234 → "1.2 s", 65000 → "1 m 5 s", 0 → ""; sv: "1,2 s", "1 min 5 s". */
export function formatDuration(ms: number | null | undefined, locale: string = formatLocale.value): string {
  if (!ms || ms < 0) return ''
  if (ms < 1000) return `${formatNumber(ms, locale)} ms`
  if (ms < 60000) return `${formatNumber(Math.round(ms / 100) / 10, locale, { maximumFractionDigits: 1 })} s`
  const min = isEnglish(locale) ? 'm' : 'min'
  const totalSec = Math.round(ms / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  return h > 0 ? `${h} h ${m} ${min}` : s ? `${m} ${min} ${s} s` : `${m} ${min}`
}

/** Integer percentage; 0 when total is 0. */
export function percent(part: number, total: number): number {
  if (!total || total <= 0) return 0
  return Math.round((part / total) * 100)
}

export function pluralize(n: number, word: string, plural = word + 's'): string {
  return `${n} ${n === 1 ? word : plural}`
}

/** Up to two initials from a display name or e-mail. */
export function initials(name: string | null | undefined): string {
  if (!name) return '?'
  const base = name.includes('@') && !name.includes(' ') ? name.split('@')[0] : name
  const parts = base.split(/[\s._-]+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}

export function truncate(s: string | null | undefined, max: number): string {
  if (!s) return ''
  return s.length > max ? s.slice(0, Math.max(0, max - 1)) + '…' : s
}

/** Options for PrimeVue selects from enum values; `label` turns a value into its display text. */
export function options(values: readonly string[] | null | undefined, label: (v: string) => string = humanize): { label: string; value: string }[] {
  return (values ?? []).map((v) => ({ label: label(v), value: v }))
}
