// Formatting helpers.

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
  const d = toDate(v)
  if (!d) return ''
  return d.toISOString().slice(0, 10)
}

export function formatDateTime(v: string | number | Date | null | undefined): string {
  const d = toDate(v)
  if (!d) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** "3 minutes ago", "in 2 days", "just now". */
export function relativeTime(v: string | number | Date | null | undefined, now: Date = new Date()): string {
  const d = toDate(v)
  if (!d) return ''
  const diff = (d.getTime() - now.getTime()) / 1000
  const abs = Math.abs(diff)
  if (abs < 45) return 'just now'
  const units: [number, string][] = [
    [60, 'minute'],
    [3600, 'hour'],
    [86400, 'day'],
    [2592000, 'month'],
    [31536000, 'year'],
  ]
  let unit = units[0]
  for (const u of units) if (abs >= u[0] * 0.75) unit = u
  const n = Math.max(1, Math.round(abs / unit[0]))
  const label = `${n} ${unit[1]}${n === 1 ? '' : 's'}`
  return diff < 0 ? `${label} ago` : `in ${label}`
}

/** 1234 → "1.2 s", 65000 → "1 m 5 s", 0 → "". */
export function formatDuration(ms: number | null | undefined): string {
  if (!ms || ms < 0) return ''
  if (ms < 1000) return `${ms} ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1).replace(/\.0$/, '')} s`
  const totalSec = Math.round(ms / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  return h > 0 ? `${h} h ${m} m` : s ? `${m} m ${s} s` : `${m} m`
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

/** Options for PrimeVue selects from enum values. */
export function options(values: readonly string[] | null | undefined): { label: string; value: string }[] {
  return (values ?? []).map((v) => ({ label: humanize(v), value: v }))
}
