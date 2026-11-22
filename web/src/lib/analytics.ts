// Requirement analytics (GET /projects/{pid}/analytics): types and the
// translated texts of findings (the server sends codes and parameters).

export interface AnalyticsItem {
  kind: 'requirement' | 'test' | string
  id: string
  key: string
  title: string
  note?: string
}
export interface AnalyticsFinding {
  code: string
  severity: 'problem' | 'warning' | 'info' | string
  category: string
  params: Record<string, string>
  items: AnalyticsItem[]
  total: number
}
export interface AnalyticsCount {
  value: string
  count: number
  percent: number
}
export interface AnalyticsFamily {
  family: string
  keywords: AnalyticsCount[]
  total: number
  dominant: string
}
export interface AnalyticsReport {
  summary: {
    requirements: number
    tests: number
    with_findings: number
    health: number
    problems: number
    warnings: number
    infos: number
    covered: number
    coverage_percent: number
    average_score: number
  }
  obligations: AnalyticsCount[]
  families: AnalyticsFamily[]
  statuses: AnalyticsCount[]
  coverage: AnalyticsCount[]
  /** languages of the requirement texts (only texts whose language is clear) */
  languages?: AnalyticsCount[]
  /** rolled-up verification states (criteria, records, children) */
  verification?: AnalyticsCount[]
  findings: AnalyticsFinding[]
}

const LANGUAGE_CODES = /\b(en|sv)\b/g

/** Language codes in a text replaced by their names ("en 90%" → "English 90%"). */
export function languageNames(text: string, t: T, te: Te): string {
  return text.replace(LANGUAGE_CODES, (code) => (te(`analysis.languages.${code}`) ? t(`analysis.languages.${code}`) : code))
}

/** The note of an item of a finding, with language codes named. */
export function itemNote(f: AnalyticsFinding, note: string | undefined, t: T, te: Te): string {
  if (!note) return ''
  return f.code === 'mixed_languages' || f.code === 'mixed_language_text' ? languageNames(note, t, te) : note
}

/** Colours of the keywords within a family (contrast ≥ 3:1 against light and dark cards). */
export const FAMILY_COLORS = ['#2563eb', '#d97706', '#7c3aed', '#059669', '#db2777']

export function severityIcon(severity: string): string {
  return severity === 'problem' ? 'pi pi-times-circle' : severity === 'warning' ? 'pi pi-exclamation-triangle' : 'pi pi-info-circle'
}

type T = (key: string, params?: Record<string, unknown>) => string
type Te = (key: string) => boolean

/** Title, explanation and advice of a finding in the active language. */
export function findingText(f: AnalyticsFinding, t: T, te: Te): { title: string; detail: string; fix: string } {
  const params: Record<string, unknown> = { ...f.params, n: f.total }
  if (f.params.family) params.family = te(`analytics.family.${f.params.family}`) ? t(`analytics.family.${f.params.family}`) : f.params.family
  if (f.code === 'mixed_languages') {
    if (f.params.dominant) params.dominant = languageNames(f.params.dominant, t, te)
    if (f.params.languages) params.languages = languageNames(f.params.languages, t, te)
  }
  if (f.code === 'language_rule') {
    const rule = f.params.rule ?? ''
    params.rule = te(`analytics.rule.${rule}`) ? t(`analytics.rule.${rule}`) : rule
  }
  const base = `analytics.code.${f.code}`
  const get = (part: string) => (te(`${base}.${part}`) ? t(`${base}.${part}`, params) : '')
  return { title: get('title') || f.code, detail: get('detail'), fix: get('fix') }
}
