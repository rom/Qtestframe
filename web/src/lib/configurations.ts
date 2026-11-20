// Test configurations (named combinations of dimension values) and results
// and coverage per configuration (mirrors internal/store/configurations.go).
import type { Configuration, DimensionValue, Execution } from './types'

export interface Dimension {
  dimension: string
  values: string[]
}

const clean = (s: string) => s.trim().replace(/\s+/g, ' ')

/** "Chrome, Firefox" or one value per line → ["Chrome", "Firefox"] (trimmed, no duplicates). */
export function splitValues(text: string): string[] {
  const out: string[] = []
  for (const v of text.split(/[,\n;]/).map(clean)) if (v && !out.some((o) => o.toLowerCase() === v.toLowerCase())) out.push(v)
  return out
}

/** The name the server gives a configuration without one: its values joined by " / ". */
export function configurationName(values: readonly DimensionValue[]): string {
  return values.map((v) => clean(v.value)).filter(Boolean).join(' / ')
}

/**
 * Every combination of the values of the dimensions (dimensions without a
 * name or values are left out), e.g. Browser × OS.
 */
export function combinations(dims: readonly Dimension[]): DimensionValue[][] {
  const used = dims.map((d) => ({ dimension: clean(d.dimension), values: d.values.map(clean).filter(Boolean) })).filter((d) => d.dimension && d.values.length)
  if (!used.length) return []
  let out: DimensionValue[][] = [[]]
  for (const d of used) out = out.flatMap((combo) => d.values.map((value) => [...combo, { dimension: d.dimension, value }]))
  return out
}

/** The dimensions used by configurations with their values (first-use order). */
export function dimensionsOf(configs: readonly Configuration[]): Dimension[] {
  const out: Dimension[] = []
  for (const c of configs) {
    for (const v of c.values ?? []) {
      let d = out.find((x) => x.dimension.toLowerCase() === v.dimension.toLowerCase())
      if (!d) out.push((d = { dimension: v.dimension, values: [] }))
      if (!d.values.includes(v.value)) d.values.push(v.value)
    }
  }
  return out
}

/** Configurations whose name matches (ignoring case) one of names. */
export function existingNames(configs: readonly Configuration[], names: readonly string[]): string[] {
  const have = new Set(configs.map((c) => c.name.toLowerCase()))
  return names.filter((n) => have.has(n.toLowerCase()))
}

export interface ConfigSummary {
  /** configuration id; '' for executions without configuration */
  id: string
  name: string
  summary: Record<string, number>
}

/** Result counts of executions per configuration, in the order given (then first appearance). */
export function summaryByConfiguration(executions: readonly Execution[], order: readonly { id: string; name: string }[] = []): ConfigSummary[] {
  const out: ConfigSummary[] = order.map((c) => ({ id: c.id, name: c.name, summary: { total: 0 } }))
  for (const e of executions) {
    const id = e.configuration_id ?? ''
    let s = out.find((x) => x.id === id)
    if (!s) out.push((s = { id, name: e.configuration_name ?? '', summary: { total: 0 } }))
    s.summary[e.status] = (s.summary[e.status] ?? 0) + 1
    s.summary.total++
  }
  return out
}

/**
 * Coverage of a requirement from the results of its linked tests (a result
 * per test, or per test and configuration): none without results to look
 * at, failed when one failed or is blocked, passed when all passed, partial
 * when some passed, else not_run.
 */
export function coverageOf(results: readonly (string | null | undefined)[]): string {
  if (!results.length) return 'none'
  let passed = 0
  for (const r of results) {
    if (r === 'failed' || r === 'blocked') return 'failed'
    if (r === 'passed') passed++
  }
  return passed === results.length ? 'passed' : passed ? 'partial' : 'not_run'
}

/** A linked test with its latest result per configuration ('' = without configuration). */
export interface CoverageTest {
  id: string
  key: string
  title: string
  results: Record<string, string>
  result?: string
}

/**
 * Overall coverage: every configuration a test is planned in counts as one
 * result (a test planned nowhere counts as not run).
 */
export function overallCoverage(tests: readonly CoverageTest[]): string {
  if (!tests.length) return 'none'
  return coverageOf(tests.flatMap((t) => (Object.keys(t.results ?? {}).length ? Object.values(t.results) : [''])))
}

/** Coverage in one configuration: over the tests planned in it (not_run when none is). */
export function coverageInConfiguration(tests: readonly CoverageTest[], cfg: string): string {
  if (!tests.length) return 'none'
  const planned = tests.filter((t) => cfg in (t.results ?? {}))
  return planned.length ? coverageOf(planned.map((t) => t.results[cfg])) : 'not_run'
}

/** The result of a test in a configuration: '' when not planned there, not_run when planned without result. */
export function resultIn(test: CoverageTest, cfg: string): string {
  if (!(cfg in (test.results ?? {}))) return ''
  return test.results[cfg] || 'not_run'
}
