// Variants, applicability and code links (commits, merge / pull requests,
// builds): types and pure helpers for the variant and code views.

export const VARIANT_KINDS = ['product', 'market', 'customer', 'other'] as const

export interface Variant {
  id: string
  project_id: string
  parent_id: string | null
  name: string
  description: string
  kind: string
  attributes: Record<string, string>
  configuration_ids: string[]
  position: number
  created_by: string
  created_at: string
  updated_at: string
  /** "EU / EU model DE" */
  path?: string
}

export interface VariantRef {
  id: string
  name: string
}

export interface Applicability {
  variant_ids: string[]
  expression: string
  variants?: VariantRef[]
  updated_by?: string
  updated_at?: string
}

/** GET/PUT …/applicability */
export interface ApplicabilityView extends Applicability {
  all: boolean
  applies_to: VariantRef[]
}

export interface VariantCoverageRow {
  variant: Variant
  applies: boolean
  coverage: string
  tests: string[]
  configurations: { id: string; name: string; coverage: string }[]
}

export interface MatrixRequirement {
  id: string
  key: string
  title: string
  obligation: string
  coverage: Record<string, string>
}

export interface VariantMatrix {
  variants: Variant[]
  requirements: MatrixRequirement[]
  summary: Record<string, Record<string, number>>
  configuration?: string
}

export interface Build {
  id: string
  project_id: string
  external_id: string
  number: string
  commit_sha: string
  branch: string
  pipeline_url: string
  created_by: string
  created_at: string
  updated_at: string
  runs: number
  summary?: Record<string, number>
}

export interface CodeChangeItem {
  item_type: 'requirement' | 'test'
  item_id: string
  key: string
  title: string
  source: 'auto' | 'manual'
  created_by?: string
  created_at: string
}

export interface CodeChange {
  id: string
  project_id: string
  kind: 'commit' | 'merge_request'
  provider: 'gitlab' | 'github' | 'manual'
  repository: string
  ref: string
  title: string
  url: string
  author: string
  branch: string
  target_branch: string
  status: string
  occurred_at: string
  created_at: string
  updated_at: string
  items: CodeChangeItem[]
  builds?: Build[]
}

export interface BuildCoverage {
  build: Build
  coverage: string
}

export interface TestBuild {
  build: Build
  result: string
  results: Record<string, string>
}

export interface ItemCode {
  changes: CodeChange[]
  builds: BuildCoverage[] | TestBuild[]
  verified_build?: Build
}

export interface CodeSettings {
  has_secret: boolean
  gitlab_url?: string
  github_url?: string
  updated_by?: string
  updated_at?: string
  /** server-side posting of commit statuses and notes (lib/ci.ts) */
  ci?: import('./ci').CISettings
  has_ci_token?: boolean
}

export interface RegressionReason {
  kind: 'requirement' | 'change' | 'related' | 'verifies' | 'prerequisite' | 'key' | 'tag' | 'all'
  ref?: string
  type?: string
}

export interface RegressionItem {
  id: string
  key: string
  title: string
  last_result?: string
  reasons: RegressionReason[]
}

export interface RegressionSelection {
  requirements: RegressionItem[]
  tests: RegressionItem[]
  changes: CodeChange[]
  variant?: VariantRef
  since_build?: Build
}

/** Depth of a variant in the hierarchy (0 for a root). */
export function variantDepth(v: Pick<Variant, 'id' | 'parent_id'>, all: Pick<Variant, 'id' | 'parent_id'>[]): number {
  const byId = new Map(all.map((x) => [x.id, x]))
  let depth = 0
  const seen = new Set([v.id])
  let p = v.parent_id ? byId.get(v.parent_id) : undefined
  while (p && !seen.has(p.id)) {
    seen.add(p.id)
    depth++
    p = p.parent_id ? byId.get(p.parent_id) : undefined
  }
  return depth
}

/** The variant and its descendants (ids). */
export function descendants(id: string, all: Pick<Variant, 'id' | 'parent_id'>[]): Set<string> {
  const out = new Set([id])
  let grew = true
  while (grew) {
    grew = false
    for (const v of all) {
      if (v.parent_id && out.has(v.parent_id) && !out.has(v.id)) {
        out.add(v.id)
        grew = true
      }
    }
  }
  return out
}

/** Options for selects: the path as label, indented by depth. */
export function variantOptions(all: Variant[]): { label: string; value: string; depth: number }[] {
  return all.map((v) => ({ label: v.path || v.name, value: v.id, depth: variantDepth(v, all) }))
}

/** "region=EU, voltage=230V" → attributes; invalid lines are reported. */
export function parseAttributes(rows: { name: string; value: string }[]): { attributes: Record<string, string>; problems: string[] } {
  const attributes: Record<string, string> = {}
  const problems: string[] = []
  const seen = new Set<string>()
  for (const r of rows) {
    const name = r.name.trim().replace(/\s+/g, '_')
    const value = r.value.trim()
    if (!name && !value) continue
    if (!name) {
      problems.push('name')
      continue
    }
    if (seen.has(name.toLowerCase())) {
      problems.push('duplicate')
      continue
    }
    seen.add(name.toLowerCase())
    attributes[name] = value
  }
  return { attributes, problems }
}

/** Attribute rows for the editor, sorted by name. */
export function attributeRows(attrs: Record<string, string> | null | undefined): { name: string; value: string }[] {
  return Object.entries(attrs ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, value]) => ({ name, value }))
}

/** "region=EU · voltage=230V" */
export function attributeText(attrs: Record<string, string> | null | undefined): string {
  return attributeRows(attrs)
    .map((r) => (r.value ? `${r.name}=${r.value}` : r.name))
    .join(' · ')
}

/** Whether an applicability covers every variant. */
export function appliesToAll(a: Applicability | null | undefined): boolean {
  return !a || ((a.variant_ids ?? []).length === 0 && !(a.expression ?? '').trim())
}

/** The mode an applicability editor starts in. */
export function applicabilityMode(a: Applicability | null | undefined): 'all' | 'selected' | 'expression' {
  if (appliesToAll(a)) return 'all'
  return (a?.expression ?? '').trim() && !(a?.variant_ids ?? []).length ? 'expression' : 'selected'
}

/** The request body of an applicability editor. */
export function applicabilityBody(mode: 'all' | 'selected' | 'expression', ids: string[], expression: string): { variant_ids: string[]; expression: string } {
  if (mode === 'all') return { variant_ids: [], expression: '' }
  if (mode === 'selected') return { variant_ids: [...new Set(ids)], expression: '' }
  return { variant_ids: [], expression: expression.trim() }
}

/** Short label of a code change: an 8-character SHA, !12 (GitLab) or #12 (GitHub). */
export function changeRef(c: Pick<CodeChange, 'kind' | 'provider' | 'ref'>): string {
  if (c.kind === 'commit') return /^[0-9a-f]{9,}$/i.test(c.ref) ? c.ref.slice(0, 8) : c.ref
  if (/^\d+$/.test(c.ref)) return (c.provider === 'github' ? '#' : '!') + c.ref
  return c.ref
}

/** How a build is shown: its number, else its CI id. */
export function buildLabel(b: Pick<Build, 'number' | 'external_id'> | null | undefined): string {
  if (!b) return ''
  return b.number || b.external_id
}

/** Pass rate of a build summary in percent (null without results). */
export function passRate(summary: Record<string, number> | null | undefined): number | null {
  const s = summary ?? {}
  const done = (s.passed ?? 0) + (s.failed ?? 0) + (s.blocked ?? 0)
  if (!done) return null
  return Math.round(((s.passed ?? 0) / done) * 100)
}

/** Percentage of the applicable requirements of a variant that passed. */
export function variantPassed(summary: Record<string, number> | null | undefined): { passed: number; applicable: number; percent: number | null } {
  const s = summary ?? {}
  const applicable = s.applicable ?? 0
  const passed = s.passed ?? 0
  return { passed, applicable, percent: applicable ? Math.round((passed / applicable) * 100) : null }
}

/** A random webhook secret (hex). */
export function randomSecret(bytes = 24): string {
  const a = new Uint8Array(bytes)
  globalThis.crypto.getRandomValues(a)
  return Array.from(a, (b) => b.toString(16).padStart(2, '0')).join('')
}

/** Splits "P-REQ-1, P-REQ-2 P-REQ-3" into keys. */
export function splitKeys(s: string): string[] {
  return [...new Set(s.split(/[\s,;]+/).map((x) => x.trim()).filter(Boolean))]
}
