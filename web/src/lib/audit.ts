// Audit trail filters: date range and field filters → GET /audit parameters.
import { and, clause } from './query'

export interface AuditFilters {
  from?: Date | string | null
  to?: Date | string | null
  user?: string
  action?: string
  entity?: string
  /** free query expression */
  q?: string
  limit?: number
}

/** Local calendar date "YYYY-MM-DD" of a Date, or a valid date string unchanged. */
export function toDateParam(v: Date | string | null | undefined): string {
  if (!v) return ''
  if (typeof v === 'string') return /^\d{4}-\d{2}-\d{2}$/.test(v) || !isNaN(Date.parse(v)) ? v : ''
  if (isNaN(v.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${v.getFullYear()}-${pad(v.getMonth() + 1)}-${pad(v.getDate())}`
}

/** True when both dates are set and `from` is after `to`. */
export function rangeInvalid(from: Date | string | null | undefined, to: Date | string | null | undefined): boolean {
  const a = toDateParam(from)
  const b = toDateParam(to)
  return !!a && !!b && a.slice(0, 10) > b.slice(0, 10)
}

/** The query expression combining the field filters and the free query. */
export function auditQuery(f: AuditFilters): string {
  const parts: string[] = []
  if (f.user?.trim()) parts.push(clause('user', ':', f.user.trim()))
  if (f.action?.trim()) parts.push(clause('action', ':', f.action.trim()))
  if (f.entity?.trim()) parts.push(clause('entity', '=', f.entity.trim()))
  return and(...parts, f.q ?? '')
}

/** Parameters of GET /audit (empty ones left out). */
export function auditParams(f: AuditFilters, format?: 'csv' | 'md'): Record<string, string | number> {
  const out: Record<string, string | number> = {}
  const q = auditQuery(f)
  if (q) out.q = q
  const from = toDateParam(f.from)
  const to = toDateParam(f.to)
  if (from) out.from = from
  if (to) out.to = to
  if (f.limit && f.limit > 0) out.limit = Math.min(5000, Math.floor(f.limit))
  if (format) out.format = format
  return out
}

/** Severity of an action tag. */
export function actionSeverity(action: string): 'danger' | 'success' | 'info' | 'warn' | 'secondary' {
  if (/delete|remove|revoke|restore/i.test(action)) return 'danger'
  if (/fail|reset/i.test(action)) return 'warn'
  if (/create|add|upload/i.test(action)) return 'success'
  if (/login|logout|auth|password/i.test(action)) return 'info'
  return 'secondary'
}

/** Details as pretty JSON when they are JSON, else unchanged. */
export function prettyDetails(details: string | null | undefined): { json: boolean; text: string } {
  const s = (details ?? '').trim()
  if (s.startsWith('{') || s.startsWith('[')) {
    try {
      return { json: true, text: JSON.stringify(JSON.parse(s), null, 2) }
    } catch {
      /* plain text */
    }
  }
  return { json: false, text: s }
}
