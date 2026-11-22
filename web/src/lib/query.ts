// Composition of filter expressions for the Qtestframe query language.

const KEYWORDS = new Set(['and', 'or', 'not'])

/** Quote a value for use in an expression when needed. */
export function quoteValue(value: string): string {
  const v = String(value)
  if (/^[\w.\-]+$/.test(v) && !KEYWORDS.has(v.toLowerCase()) && !v.startsWith('-')) return v
  return '"' + v.replace(/\\/g, '\\\\').replace(/"/g, '\\"') + '"'
}

export type Operator = ':' | '=' | '!=' | '~' | '!~' | '>' | '>=' | '<' | '<='

/** A single `field<op>value` clause. */
export function clause(field: string, op: Operator, value: string): string {
  return `${field}${op}${quoteValue(value)}`
}

/** OR-combine values of one field; parenthesised when more than one. */
export function anyOf(field: string, values: readonly string[] | null | undefined, op: Operator = '='): string {
  const vs = (values ?? []).filter((v) => v !== '' && v != null)
  if (vs.length === 0) return ''
  const parts = vs.map((v) => clause(field, op, v))
  return parts.length === 1 ? parts[0] : `(${parts.join(' OR ')})`
}

/** Wrap an expression in parentheses unless it is a single token or already fully grouped. */
export function group(expr: string): string {
  const e = expr.trim()
  if (!e) return ''
  if (isSingleTerm(e) || isFullyGrouped(e)) return e
  return `(${e})`
}

function isSingleTerm(e: string): boolean {
  // no whitespace outside of quotes / regex literals
  let inQuote = false
  let inRegex = false
  for (let i = 0; i < e.length; i++) {
    const c = e[i]
    if (c === '\\') {
      i++
      continue
    }
    if (!inRegex && c === '"') inQuote = !inQuote
    else if (!inQuote && c === '/') inRegex = !inRegex
    else if (!inQuote && !inRegex && /\s/.test(c)) return false
  }
  return true
}

function isFullyGrouped(e: string): boolean {
  if (!e.startsWith('(') || !e.endsWith(')')) return false
  let depth = 0
  let inQuote = false
  for (let i = 0; i < e.length; i++) {
    const c = e[i]
    if (c === '\\') {
      i++
      continue
    }
    if (c === '"') inQuote = !inQuote
    if (inQuote) continue
    if (c === '(') depth++
    else if (c === ')') {
      depth--
      if (depth === 0 && i < e.length - 1) return false
    }
  }
  return depth === 0
}

/** AND-combine expressions, skipping empty ones. */
export function and(...exprs: (string | null | undefined)[]): string {
  const parts = exprs.map((e) => (e ?? '').trim()).filter(Boolean)
  if (parts.length <= 1) return parts[0] ?? ''
  return parts.map(group).join(' AND ')
}

export interface QuickFilters {
  status?: string[]
  obligation?: string[]
  priority?: string[]
  execution_type?: string[]
  last_result?: string[]
  coverage?: string[]
  /** Category references as "Dimension/Name" (matched via the `dimension` field). */
  categories?: string[]
}

const QUICK_FIELDS: (keyof QuickFilters)[] = ['status', 'obligation', 'priority', 'execution_type', 'last_result', 'coverage']

/**
 * Compose the free-text expression and quick filter selections into one
 * expression. Values of one field are OR-ed, fields and the free text AND-ed.
 */
export function composeQuery(base: string | null | undefined, filters: QuickFilters = {}): string {
  const parts: string[] = [base ?? '']
  for (const f of QUICK_FIELDS) parts.push(anyOf(f, filters[f]))
  parts.push(anyOf('dimension', filters.categories))
  return and(...parts)
}

/** Count active quick filter values. */
export function activeFilterCount(filters: QuickFilters): number {
  return Object.values(filters).reduce((n, v) => n + (Array.isArray(v) ? v.length : 0), 0)
}

/** Parse tags typed as comma / whitespace separated text into a unique list. */
export function parseTags(input: string): string[] {
  const seen = new Set<string>()
  for (const t of input.split(/[,\s]+/)) {
    const v = t.trim()
    if (v) seen.add(v)
  }
  return [...seen]
}

export interface QueryHelpEntry {
  example: string
  /** English text (the UI shows the translation `query.meaning.<id>`) */
  meaning: string
  id: string
}

/** Syntax reference shown in the filter help popover (docs/api.md). */
export const QUERY_HELP: QueryHelpEntry[] = [
  { example: 'status:approved priority=high', meaning: 'Implicit AND between terms', id: 'and' },
  { example: 'obligation:must OR obligation:shall', meaning: 'Either term matches', id: 'or' },
  { example: 'NOT status:obsolete   -status:obsolete', meaning: 'Negation', id: 'not' },
  { example: 'title~/^login.*(fail|error)/i', meaning: 'Regular expression (RE2)', id: 'regex' },
  { example: 'description!~/TBD/', meaning: 'Regex must not match', id: 'notRegex' },
  { example: '"exact phrase"', meaning: 'Free text over all fields', id: 'phrase' },
  { example: '/time[- ]?out/', meaning: 'Regex over all fields', id: 'regexAll' },
  { example: 'key:REQ-1*', meaning: 'Glob (* and ?)', id: 'glob' },
  { example: 'version>=2 created_at<2026-01-01', meaning: 'Numeric / date comparison', id: 'compare' },
  { example: 'category:Negative  category:"Network based"', meaning: 'Category name', id: 'category' },
  { example: 'dimension:"Polarity/Negative"', meaning: 'Category as Dimension/Name', id: 'dimension' },
  { example: 'has:criteria   (or field:*)', meaning: 'Field not empty', id: 'has' },
  { example: 'verification:verified  element:"Power unit"', meaning: 'Verification state; allocated to a system element (or below it)', id: 'verification' },
  { example: '(a OR b) AND c', meaning: 'Grouping', id: 'group' },
  { example: 'cf.supplier:"Acme"  cf.mass>2', meaning: 'Custom field (by its key)', id: 'custom' },
  { example: 'sort:-cf.mass  sort:title', meaning: 'Sort the result (- for descending)', id: 'sort' },
]

export const QUERY_OPERATORS: QueryHelpEntry[] = [
  { example: ':', meaning: 'contains (case-insensitive; glob with * / ?)', id: 'opContains' },
  { example: '=  !=', meaning: 'equals / not equals (case-insensitive)', id: 'opEquals' },
  { example: '~  !~', meaning: 'regex matches / does not match', id: 'opRegex' },
  { example: '>  >=  <  <=', meaning: 'numeric or date comparison', id: 'opCompare' },
]

export const QUERY_FIELDS: Record<'requirement' | 'test' | 'run' | 'execution' | 'document' | 'audit' | 'defect', string[]> = {
  requirement: [
    'key', 'title', 'description', 'obligation', 'type', 'priority', 'status', 'source', 'rationale', 'owner',
    'tag', 'version', 'folder', 'category', 'dimension', 'coverage', 'linked', 'created_by', 'created_at',
    'updated_at', 'criteria', 'related', 'parent', 'child', 'comments', 'mention', 'suspect', 'risk', 'defects', 'catalog',
    'blocked', 'stale', 'suspect_relations',
    'verification', 'element',
    'variant', 'applicability',
  ],
  test: [
    'key', 'title', 'description', 'preconditions', 'execution_type', 'type', 'script_language', 'script',
    'expected', 'step', 'priority', 'status', 'estimate', 'tag', 'folder', 'category', 'dimension', 'linked',
    'last_result', 'created_by', 'created_at', 'updated_at', 'defect', 'calls', 'comments', 'version', 'mention', 'depends_on', 'suspect', 'flaky', 'flakiness', 'catalog', 'stale',
    'element',
    'variant', 'applicability',
  ],
  run: ['name', 'description', 'status', 'environment', 'build', 'created_by', 'created_at'],
  execution: ['key', 'title', 'status', 'assignee', 'executed_by', 'notes', 'output', 'defect', 'mention'],
  document: ['name', 'description', 'tag', 'kind', 'type', 'uploaded_by', 'created_at', 'size', 'linked'],
  audit: ['action', 'entity', 'entity_id', 'user', 'details', 'created_at'],
  defect: ['key', 'title', 'description', 'status', 'severity', 'priority', 'assignee', 'external', 'created_by', 'open', 'linked'],
}
