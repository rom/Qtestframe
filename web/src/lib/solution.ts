// The solution preamble of a software project and the options of the
// solution prompts (mirrors model.SolutionPreamble and package prompt).
// The preamble holds descriptions only: the server warns about anything
// that looks like a credential and masks it in prompts.

/** Architecture styles (mirrors model.ArchitectureStyles). */
export const ARCHITECTURE_STYLES = ['monolith', 'modular_monolith', 'microservices', 'serverless', 'event_driven', 'layered', 'hexagonal', 'other'] as const

/** The text fields of a preamble in display order (mirrors model.PreambleFields). */
export const PREAMBLE_FIELDS = [
  'languages',
  'frameworks',
  'environments',
  'architecture_notes',
  'frontend',
  'backend',
  'messaging',
  'caching',
  'databases',
  'integrations',
  'deployment',
  'performance',
  'availability',
  'scalability',
  'security',
  'conventions',
  'testing',
  'repository',
  'out_of_scope',
  'additional',
] as const
export type PreambleField = (typeof PREAMBLE_FIELDS)[number]

/** Groups of the preamble form; architecture_style is a choice in the architecture group. */
export const PREAMBLE_GROUPS: { id: string; fields: (PreambleField | 'architecture_style')[] }[] = [
  { id: 'technology', fields: ['languages', 'frameworks', 'environments'] },
  { id: 'architecture', fields: ['architecture_style', 'architecture_notes'] },
  { id: 'stack', fields: ['frontend', 'backend', 'messaging', 'caching'] },
  { id: 'data', fields: ['databases', 'integrations'] },
  { id: 'quality', fields: ['performance', 'availability', 'scalability', 'security'] },
  { id: 'delivery', fields: ['deployment', 'testing', 'conventions', 'repository'] },
  { id: 'scope', fields: ['out_of_scope', 'additional'] },
]

/** Longest text of a field (mirrors model.MaxPreambleField / MaxPreambleAdditional). */
export const preambleLimit = (f: PreambleField): number => (f === 'additional' ? 16000 : 4000)

export type SolutionPreamble = Record<PreambleField, string> & {
  template: string
  architecture_style: string
  version: number
  updated_at?: string | null
  updated_by?: string
}

export interface PreambleWarning {
  field: PreambleField
  kind: 'password_in_url' | 'password_assignment' | 'private_key' | 'token'
  message: string
  excerpt: string
}

export interface PreambleView {
  preamble: SolutionPreamble
  warnings: PreambleWarning[]
  filled: number
  software: boolean
}

export interface PreambleTemplate {
  id: string
  name: Record<string, string>
  preamble: Partial<SolutionPreamble>
}

export function emptyPreamble(): SolutionPreamble {
  const p = { template: '', architecture_style: '', version: 0 } as SolutionPreamble
  for (const f of PREAMBLE_FIELDS) p[f] = ''
  return p
}

/** The preamble with every field present (as strings). */
export function normalizePreamble(p: Partial<SolutionPreamble> | null | undefined): SolutionPreamble {
  const out = emptyPreamble()
  if (!p) return out
  for (const f of PREAMBLE_FIELDS) out[f] = typeof p[f] === 'string' ? (p[f] as string) : ''
  out.template = p.template ?? ''
  out.architecture_style = p.architecture_style ?? ''
  out.version = p.version ?? 0
  out.updated_at = p.updated_at ?? null
  out.updated_by = p.updated_by ?? ''
  return out
}

/** Number of filled-in fields (the style included). */
export function preambleFilled(p: SolutionPreamble): number {
  return (p.architecture_style ? 1 : 0) + PREAMBLE_FIELDS.filter((f) => p[f]?.trim()).length
}

/** The body of PUT /solution-preamble: the content without the bookkeeping. */
export function preambleBody(p: SolutionPreamble): Record<string, string | number> {
  const body: Record<string, string | number> = { template: p.template, architecture_style: p.architecture_style, version: p.version }
  for (const f of PREAMBLE_FIELDS) body[f] = p[f]
  return body
}

/** A template applied over a preamble: the template's fields replace the
 * current ones; fields it leaves empty stay as they are. */
export function applyTemplate(cur: SolutionPreamble, t: PreambleTemplate): SolutionPreamble {
  const out = { ...cur, template: t.id }
  for (const f of PREAMBLE_FIELDS) {
    const v = t.preamble[f]
    if (typeof v === 'string' && v.trim()) out[f] = v
  }
  if (t.preamble.architecture_style) out.architecture_style = t.preamble.architecture_style
  return out
}

/** Output formats of solution prompts (mirrors prompt.Formats). */
export const SOLUTION_FORMATS = ['mermaid', 'c4', 'openapi', 'sql'] as const
export type SolutionFormat = (typeof SOLUTION_FORMATS)[number]

/** The formats a kind asks for by default (mirrors prompt.DefaultFormats). */
export function defaultFormats(kind: string): SolutionFormat[] {
  switch (kind) {
    case 'architecture':
      return ['mermaid', 'c4']
    case 'design':
      return ['mermaid', 'openapi', 'sql']
    case 'full_solution':
      return ['mermaid', 'c4', 'openapi', 'sql']
  }
  return []
}

/** Split modes (mirrors prompt.Splits). */
export const SPLITS = ['none', 'folder', 'size'] as const
export const DEFAULT_PART_TOKENS = 8000
export const MIN_PART_TOKENS = 1000

/** Estimated tokens of a text: characters / 4 (as the server). */
export const estimateTokens = (s: string): number => Math.ceil([...s].length / 4)

export interface PromptPart {
  index: number
  title: string
  label: string
  prompt: string
  items: number
  tokens: number
}

/** File name of one part: "SHOP-architecture-2026-11-20-part-02.md". */
export function promptPartFileName(projectKey: string, kind: string, index: number, total: number, date = new Date()): string {
  const d = date.toISOString().slice(0, 10)
  const base = `${projectKey || 'prompt'}-${kind.replace(/_/g, '-')}-${d}`
  return total > 1 ? `${base}-part-${String(index).padStart(2, '0')}.md` : `${base}.md`
}
