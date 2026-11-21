// Visual vocabulary: PrimeVue Tag severities and chart colours per enum value.

export type Severity = 'success' | 'info' | 'warn' | 'danger' | 'secondary' | 'contrast'

const STATUS_SEVERITY: Record<string, Severity> = {
  // requirement
  draft: 'secondary',
  review: 'warn',
  approved: 'info',
  implemented: 'info',
  verified: 'success',
  rejected: 'danger',
  obsolete: 'contrast',
  // test
  ready: 'success',
  deprecated: 'contrast',
  // run
  planned: 'secondary',
  in_progress: 'info',
  completed: 'success',
  aborted: 'danger',
}

const RESULT_SEVERITY: Record<string, Severity> = {
  not_run: 'secondary',
  in_progress: 'info',
  passed: 'success',
  failed: 'danger',
  blocked: 'warn',
  skipped: 'contrast',
}

const COVERAGE_SEVERITY: Record<string, Severity> = {
  none: 'contrast',
  not_run: 'secondary',
  passed: 'success',
  failed: 'danger',
  partial: 'warn',
}

const OBLIGATION_SEVERITY: Record<string, Severity> = {
  must: 'danger',
  must_not: 'danger',
  shall: 'danger',
  shall_not: 'danger',
  should: 'warn',
  should_not: 'warn',
  may: 'info',
  optional: 'info',
  will: 'secondary',
  describe: 'secondary',
}

const PRIORITY_SEVERITY: Record<string, Severity> = {
  critical: 'danger',
  high: 'warn',
  medium: 'info',
  low: 'secondary',
}

export function statusSeverity(v: string | null | undefined): Severity {
  return STATUS_SEVERITY[v ?? ''] ?? 'secondary'
}
export function resultSeverity(v: string | null | undefined): Severity {
  return RESULT_SEVERITY[v ?? ''] ?? 'secondary'
}
export function coverageSeverity(v: string | null | undefined): Severity {
  return COVERAGE_SEVERITY[v ?? ''] ?? 'secondary'
}
export function obligationSeverity(v: string | null | undefined): Severity {
  return OBLIGATION_SEVERITY[v ?? ''] ?? 'secondary'
}
export function prioritySeverity(v: string | null | undefined): Severity {
  return PRIORITY_SEVERITY[v ?? ''] ?? 'secondary'
}

/** Chart / bar colours (hex) for result statuses. */
export const RESULT_COLORS: Record<string, string> = {
  passed: '#22c55e',
  failed: '#ef4444',
  blocked: '#f59e0b',
  skipped: '#a855f7',
  in_progress: '#3b82f6',
  not_run: '#94a3b8',
}

/** Icons for result statuses, so a colour-coded mark never relies on colour alone. */
export const RESULT_ICONS: Record<string, string> = {
  passed: 'pi pi-check',
  failed: 'pi pi-times',
  blocked: 'pi pi-ban',
  skipped: 'pi pi-forward',
  in_progress: 'pi pi-clock',
  not_run: 'pi pi-minus',
}

/** Colour and icon of a traceability mark: the linked test's latest result (not run without one). */
export function traceMark(result: string | null | undefined): { result: string; color: string; icon: string } {
  const r = result && RESULT_ICONS[result] ? result : 'not_run'
  return { result: r, color: RESULT_COLORS[r]!, icon: RESULT_ICONS[r]! }
}

export const COVERAGE_COLORS: Record<string, string> = {
  passed: '#22c55e',
  failed: '#ef4444',
  partial: '#f59e0b',
  not_run: '#94a3b8',
  none: '#cbd5e1',
}

export const OBLIGATION_COLORS: Record<string, string> = {
  must: '#dc2626',
  must_not: '#b91c1c',
  shall: '#ea580c',
  shall_not: '#c2410c',
  should: '#d97706',
  should_not: '#b45309',
  may: '#0284c7',
  optional: '#0891b2',
  will: '#7c3aed',
  describe: '#64748b',
}

export const STATUS_COLORS: Record<string, string> = {
  draft: '#94a3b8',
  review: '#f59e0b',
  approved: '#3b82f6',
  implemented: '#6366f1',
  verified: '#22c55e',
  rejected: '#ef4444',
  obsolete: '#475569',
  ready: '#22c55e',
  deprecated: '#475569',
  planned: '#94a3b8',
  in_progress: '#3b82f6',
  completed: '#22c55e',
  aborted: '#ef4444',
}

const FALLBACK = ['#10b981', '#3b82f6', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899', '#84cc16']

/** Look a colour up in a map, falling back to a stable palette colour. */
export function colorFor(map: Record<string, string>, key: string, index = 0): string {
  return map[key] ?? FALLBACK[Math.abs(index) % FALLBACK.length]
}

/** Normalise "#abc" / "abc" / "#aabbcc" to "#aabbcc"; invalid → null. */
export function normalizeHex(color: string | null | undefined): string | null {
  if (!color) return null
  let c = color.trim().replace(/^#/, '')
  if (/^[0-9a-f]{3}$/i.test(c)) c = c.split('').map((x) => x + x).join('')
  if (!/^[0-9a-f]{6}$/i.test(c)) return null
  return '#' + c.toLowerCase()
}

/** WCAG relative luminance of a "#rrggbb" colour. */
function relLuminance(hex: string): number {
  const ch = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * ch[0] + 0.7152 * ch[1] + 0.0722 * ch[2]
}
const ratio = (a: number, b: number) => (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)

/**
 * Dark or white text, whichever has the higher WCAG contrast on the
 * background (falls back to pure black when the dark text stays below 4.5:1).
 */
export function readableTextColor(bg: string | null | undefined): string {
  const hex = normalizeHex(bg)
  if (!hex) return '#ffffff'
  const l = relLuminance(hex)
  const white = ratio(1, l)
  const dark = ratio(relLuminance('#111827'), l)
  if (white >= 4.5 && white >= dark) return '#ffffff'
  if (dark >= 4.5) return '#111827'
  // mid-tones: pure black or white, whichever is better
  return ratio(0, l) > white ? '#000000' : '#ffffff'
}

/** Default colour for a new category, chosen by dimension name. */
export function defaultCategoryColor(dimension: string): string {
  let h = 0
  for (const ch of dimension) h = (h * 31 + ch.charCodeAt(0)) | 0
  return FALLBACK[Math.abs(h) % FALLBACK.length]
}
