// The requirement and test catalogue (GET /catalog…): types and the pure
// logic of the catalogue page – filtering, ranked search, the area tree,
// selection and the import summary. The page loads every item of a kind
// once per language and filters here, so typing in the search field stays
// instant with several hundred items.
import { anyOf } from './query'

export type CatalogKind = 'requirement' | 'test' | 'risk'
export type CatalogLang = 'en' | 'sv'

export interface CatalogArea {
  id: string
  prefix: string
  group: string
  /** catalogue type (software, hardware, …) */
  catalogue?: string
  name: string
  description: string
  requirements: number
  tests: number
  risks?: number
}

export interface CatalogGroup {
  id: string
  name: string
  requirements: number
  tests: number
  risks?: number
  areas: CatalogArea[]
}

export interface CatalogTagCount {
  tag: string
  count: number
}

/** A type of catalogue (software, hardware, …) with its counts. */
export interface CatalogType {
  id: string
  name: string
  description: string
  areas: number
  requirements: number
  tests: number
}

export interface CatalogOverview {
  languages: string[]
  lang: string
  groups: CatalogGroup[]
  requirements: number
  tests: number
  requirement_tags: CatalogTagCount[]
  test_tags: CatalogTagCount[]
  catalogues?: CatalogType[]
  risks?: number
  risk_tags?: CatalogTagCount[]
  /** risk categories in the language */
  risk_categories?: { id: string; name: string }[]
}

/** A law, regulation, standard or best practice the catalogue cites (GET /catalog/sources). */
export interface CatalogSource {
  id: string
  /** law | directive | standard | practice | guideline */
  kind: string
  jurisdiction?: string
  short?: string
  name: string
  /** the official or publisher page */
  url?: string
  /** requirements citing it */
  requirements: number
  /** tests citing it or verifying such a requirement */
  tests: number
  /** risks citing it */
  risks?: number
  catalogues: string[]
}

export interface CatalogSourceKind {
  id: string
  name: string
}

/** GET /catalog/sources */
export interface CatalogSources {
  lang: string
  kinds: CatalogSourceKind[]
  sources: CatalogSource[]
}

/** A project item with the catalogue id it was copied from. */
export interface InProjectItem {
  id: string
  key: string
  title: string
}

interface CatalogBase {
  id: string
  area: string
  group: string
  catalogue?: string
  /** ids of the reference sources its refs cite */
  sources?: string[]
  /** requirements and tests: critical, high, medium, low (risks have none) */
  priority?: string
  tags: string[]
  refs: string[]
  title: string
  description: string
  in_project?: InProjectItem[]
}

export interface CatalogRequirement extends CatalogBase {
  type: string
  obligation: string
  /** catalogue tests that verify it */
  tests: string[]
  /** catalogue risks it mitigates */
  risks?: string[]
}

export interface CatalogTest extends CatalogBase {
  execution: string
  /** catalogue requirements it verifies */
  requirements: string[]
  /** number of steps */
  steps: number
}

/** A typical risk of an area with the catalogue requirements that mitigate it. */
export interface CatalogRisk extends CatalogBase {
  /** security, privacy, safety, availability, … */
  category: string
  category_name: string
  /** typical likelihood and impact on the project scale 1–5 */
  likelihood: number
  impact: number
  score: number
  /** low | medium | high | critical */
  level: string
  /** catalogue requirements that mitigate it */
  mitigated_by: string[]
  /** catalogue tests that check it directly */
  tests: string[]
  mitigation: string
}

export type CatalogItem = CatalogRequirement | CatalogTest | CatalogRisk

export interface CatalogLinked {
  id: string
  area: string
  title: string
  in_project?: InProjectItem[]
}

export interface CatalogCriterion {
  method: string
  text: string
  threshold?: string
}

export interface CatalogStep {
  action: string
  expected: string
  data?: string
}

/** A ref with its source and the link to the cited text (catalogue item details). */
export interface CatalogRefTarget {
  ref: string
  source?: string
  /** the source's name */
  name?: string
  /** the cited document, else the source */
  url?: string
}

/** The refs of an item with their sources, linked where the registry knows the text (https links only). */
export function refLinks(refs: readonly string[], targets: readonly CatalogRefTarget[] | null | undefined): CatalogRefTarget[] {
  const byRef = new Map((targets ?? []).map((x) => [x.ref, x]))
  return refs.map((ref) => {
    const x = byRef.get(ref)
    const url = x?.url && /^https:\/\/\S+$/.test(x.url) ? x.url : undefined
    return { ref, source: x?.source, name: x?.name, url }
  })
}

export interface CatalogRequirementDetail extends CatalogRequirement {
  area_name: string
  rationale: string
  criteria: CatalogCriterion[]
  ref_targets?: CatalogRefTarget[]
  linked_tests: CatalogLinked[]
  mitigates_risks?: CatalogLinked[]
}

export interface CatalogTestDetail extends CatalogTest {
  area_name: string
  preconditions: string
  expected: string
  step_list: CatalogStep[]
  ref_targets?: CatalogRefTarget[]
  linked_requirements: CatalogLinked[]
}

export interface CatalogRiskDetail extends CatalogRisk {
  area_name: string
  ref_targets?: CatalogRefTarget[]
  mitigating_requirements: CatalogLinked[]
  linked_tests: CatalogLinked[]
}

/** GET /projects/{pid}/catalog */
export interface ProjectCatalog {
  requirements: Record<string, InProjectItem[]>
  tests: Record<string, InProjectItem[]>
  risks?: Record<string, InProjectItem[]>
}

export interface CatalogCreated {
  id: string
  key: string
  title: string
  catalog_ref: string
  folder_id: string | null
  /** requirements of the project the new one is similar to (suspected duplicates) */
  similar?: { id: string; key: string; title: string; score: number }[]
}

export interface CatalogImportResult {
  requirements: CatalogCreated[]
  tests: CatalogCreated[]
  risks?: CatalogCreated[]
  links: { id: string; requirement_id: string; requirement_key: string; test_case_id: string; test_case_key: string }[]
  risk_links?: { risk_id: string; risk_key: string; requirement_id: string; requirement_key: string }[]
  skipped: { kind: CatalogKind; catalog_ref: string; reason: string; existing?: InProjectItem[] }[]
  folders_created: number
}

export const isRequirement = (x: CatalogItem): x is CatalogRequirement => 'obligation' in x
export const isRisk = (x: CatalogItem): x is CatalogRisk => 'mitigated_by' in x

/** The ids at the other end of an item's catalogue links (tests of a requirement, requirements of a test or risk). */
export const linkedIds = (x: CatalogItem): string[] => (isRequirement(x) ? x.tests : isRisk(x) ? x.mitigated_by : x.requirements)

// ---------------------------------------------------------------- search

/** Lower-case search words of a query (letters, digits, "-", ".", "§"). */
export function terms(q: string): string[] {
  return q
    .toLowerCase()
    .split(/[^\p{L}\p{N}\-.§]+/u)
    .filter(Boolean)
}

const isWordChar = (c: string | undefined) => !!c && /[\p{L}\p{N}]/u.test(c)

/**
 * Relevance of an item for the search words, like the server: 0 when a
 * word occurs nowhere; per word the weight of the best field (title 10,
 * id 8, tags 6, standards 4, description 3), doubled at a word start; an
 * exact id counts 1000 and the whole query in the title adds 50.
 */
export function searchScore(item: CatalogItem, words: readonly string[]): number {
  if (!words.length) return 1
  const fields: [string, number][] = [
    [item.title.toLowerCase(), 10],
    [item.id.toLowerCase(), 8],
    [item.tags.join(' ').toLowerCase(), 6],
    [item.refs.join(' ').toLowerCase(), 4],
    [item.description.toLowerCase(), 3],
    [linkedIds(item).join(' ').toLowerCase(), 2],
    ...(isRisk(item) ? ([[item.mitigation.toLowerCase(), 1]] as [string, number][]) : []),
  ]
  let total = words.length > 1 && fields[0][0].includes(words.join(' ')) ? 50 : 0
  const id = item.id.toLowerCase()
  for (const w of words) {
    if (w === id) {
      total += 1000
      continue
    }
    let best = 0
    for (const [text, weight] of fields) {
      const i = text.indexOf(w)
      if (i < 0) continue
      best = Math.max(best, i === 0 || !isWordChar(text[i - 1]) ? weight * 2 : weight)
    }
    if (!best) return 0
    total += best
  }
  return total
}

// ---------------------------------------------------------------- filters

/** Tree node keys: "all", "g:<group>" or "a:<area>". */
export type NodeKey = string

export interface CatalogFilters {
  node: NodeKey
  q: string
  types: string[]
  obligations: string[]
  priorities: string[]
  tags: string[]
  executions: string[]
  /** risks: categories and levels */
  categories?: string[]
  levels?: string[]
  hideInProject: boolean
  /** catalogue type ("" = every type) */
  catalogue?: string
  /** reference sources to comply with (any of them) */
  sources?: string[]
}

export function emptyFilters(): CatalogFilters {
  return { node: 'all', q: '', types: [], obligations: [], priorities: [], tags: [], executions: [], categories: [], levels: [], hideInProject: false, catalogue: '', sources: [] }
}

/** Number of filters set besides the tree node and the search. */
export function activeFilters(f: CatalogFilters): number {
  return [f.types, f.obligations, f.priorities, f.tags, f.executions, f.categories ?? [], f.levels ?? []].filter((x) => x.length).length + (f.hideInProject ? 1 : 0)
}

export function matchesNode(item: CatalogItem, node: NodeKey): boolean {
  if (!node || node === 'all') return true
  if (node.startsWith('g:')) return item.group === node.slice(2)
  if (node.startsWith('a:')) return item.area === node.slice(2)
  return true
}

const okAny = (want: readonly string[], v: string) => !want.length || want.includes(v)

/**
 * The sources an item brings: a requirement those it cites, a test those it
 * cites and those of the requirements it verifies, a risk those it cites and
 * those of the requirements that mitigate it (as the server's ?source=).
 */
export function itemSources(item: CatalogItem, reqById?: ReadonlyMap<string, CatalogRequirement>): string[] {
  const own = item.sources ?? []
  if (isRequirement(item) || !reqById) return own
  const out = new Set(own)
  for (const r of linkedIds(item)) for (const s of reqById.get(r)?.sources ?? []) out.add(s)
  return [...out]
}

/** Whether the item passes the field filters (not the node or the search). */
export function matchesFields(item: CatalogItem, f: CatalogFilters, inProject: ReadonlySet<string>, reqById?: ReadonlyMap<string, CatalogRequirement>): boolean {
  if (f.hideInProject && inProject.has(item.id)) return false
  if (f.catalogue && (item.catalogue ?? 'software') !== f.catalogue) return false
  if (f.sources?.length && !itemSources(item, reqById).some((s) => f.sources!.includes(s))) return false
  if (f.tags.length && !item.tags.some((t) => f.tags.includes(t))) return false
  if (isRisk(item)) return okAny(f.categories ?? [], item.category) && okAny(f.levels ?? [], item.level)
  if (!okAny(f.priorities, item.priority ?? '')) return false
  if (isRequirement(item)) return okAny(f.types, item.type) && okAny(f.obligations, item.obligation)
  return okAny(f.executions, item.execution)
}

/**
 * Items passing the filters: ranked by relevance when searching, else in
 * catalogue order. `node: false` ignores the tree node (for the counts).
 */
export function filterCatalog<T extends CatalogItem>(
  items: readonly T[],
  f: CatalogFilters,
  inProject: ReadonlySet<string>,
  opts: { node?: boolean; reqById?: ReadonlyMap<string, CatalogRequirement> } = {},
): T[] {
  const words = terms(f.q)
  const hits: { item: T; score: number; i: number }[] = []
  items.forEach((item, i) => {
    if (opts.node !== false && !matchesNode(item, f.node)) return
    if (!matchesFields(item, f, inProject, opts.reqById)) return
    const score = searchScore(item, words)
    if (score > 0) hits.push({ item, score, i })
  })
  if (words.length) hits.sort((a, b) => b.score - a.score || a.i - b.i)
  return hits.map((h) => h.item)
}

/** Counts per tree node key ("all", "g:…", "a:…") of the given items. */
export function nodeCounts(items: readonly CatalogItem[]): Map<NodeKey, number> {
  const out = new Map<NodeKey, number>([['all', items.length]])
  for (const x of items) {
    out.set(`g:${x.group}`, (out.get(`g:${x.group}`) ?? 0) + 1)
    out.set(`a:${x.area}`, (out.get(`a:${x.area}`) ?? 0) + 1)
  }
  return out
}

export interface CatalogTreeNode {
  key: NodeKey
  label: string
  /** items under the node passing the filters */
  count: number
  /** all items under the node */
  total: number
  children?: CatalogTreeNode[]
}

/** Groups and areas as tree nodes with counts for the kind; with a catalogue type only its areas (and groups with such areas). */
export function catalogTree(groups: readonly CatalogGroup[], kind: CatalogKind, counts: ReadonlyMap<NodeKey, number>, catalogue = ''): CatalogTreeNode[] {
  const total = (x: { requirements: number; tests: number; risks?: number }) => (kind === 'requirement' ? x.requirements : kind === 'risk' ? (x.risks ?? 0) : x.tests)
  const inType = (a: CatalogArea) => !catalogue || (a.catalogue ?? 'software') === catalogue
  return groups
    .map((g) => ({ g, areas: g.areas.filter((a) => inType(a) && total(a) > 0) }))
    .filter(({ areas }) => areas.length > 0)
    .map(({ g, areas }) => ({
      key: `g:${g.id}`,
      label: g.name,
      count: counts.get(`g:${g.id}`) ?? 0,
      total: areas.reduce((n, a) => n + total(a), 0),
      children: areas.map((a) => ({ key: `a:${a.id}`, label: a.name, count: counts.get(`a:${a.id}`) ?? 0, total: total(a) })),
    }))
}

// ---------------------------------------------------------------- catalogue types and sources

/** The catalogue type that fits a project type (model.ProjectTypes), "" (every type) when none fits or it has no areas. */
export function defaultCatalogue(projectType: string | undefined, available: readonly string[]): string {
  const want: Record<string, string> = { software: 'software', hardware: 'hardware', system: 'system', business: 'governance' }
  const id = want[projectType ?? ''] ?? ''
  return id && available.includes(id) ? id : ''
}

export interface SourceGroup {
  kind: string
  label: string
  sources: CatalogSource[]
}

/** Sources matching the search (name, short name, id or jurisdiction), grouped by kind in the order of kinds; empty groups are left out. */
export function groupSources(sources: readonly CatalogSource[], kinds: readonly CatalogSourceKind[], q = ''): SourceGroup[] {
  const words = terms(q)
  const hit = (s: CatalogSource) => {
    const text = [s.name, s.short ?? '', s.id, s.jurisdiction ?? ''].join(' ').toLowerCase()
    return words.every((w) => text.includes(w))
  }
  const order = kinds.map((k) => k.id)
  for (const s of sources) if (!order.includes(s.kind)) order.push(s.kind)
  return order
    .map((k) => ({ kind: k, label: kinds.find((x) => x.id === k)?.name ?? k, sources: sources.filter((s) => s.kind === k && hit(s)) }))
    .filter((g) => g.sources.length > 0)
}

/** Short label of a source: its short name, else its name. */
export const sourceLabel = (s: Pick<CatalogSource, 'name' | 'short'>) => s.short || s.name

/** The catalogue requirements citing any of the sources, and the tests that verify them (ids, catalogue order). */
export function complianceSet(reqs: readonly CatalogRequirement[], sources: readonly string[]): { requirements: string[]; tests: string[] } {
  const out = { requirements: [] as string[], tests: [] as string[] }
  if (!sources.length) return out
  const tests = new Set<string>()
  for (const r of reqs) {
    if (!(r.sources ?? []).some((s) => sources.includes(s))) continue
    out.requirements.push(r.id)
    for (const t of r.tests) {
      if (tests.has(t)) continue
      tests.add(t)
      out.tests.push(t)
    }
  }
  return out
}

/** The label of a tree node ("" for "all" or an unknown key). */
export function nodeLabel(groups: readonly CatalogGroup[], node: NodeKey): string {
  for (const g of groups) {
    if (node === `g:${g.id}`) return g.name
    for (const a of g.areas) if (node === `a:${a.id}`) return a.name
  }
  return ''
}

/** Name of an area by id (the id when unknown). */
export function areaName(groups: readonly CatalogGroup[], id: string): string {
  return nodeLabel(groups, `a:${id}`) || id
}

/** Distinct tags of the items, most used first. */
export function tagOptions(items: readonly CatalogItem[]): string[] {
  const n = new Map<string, number>()
  for (const x of items) for (const t of x.tags) n.set(t, (n.get(t) ?? 0) + 1)
  return [...n].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([t]) => t)
}

// ---------------------------------------------------------------- selection

/** The selection with ids added (on) or removed. */
export function toggleIds(selected: ReadonlySet<string>, ids: readonly string[], on: boolean): Set<string> {
  const out = new Set(selected)
  for (const id of ids) {
    if (on) out.add(id)
    else out.delete(id)
  }
  return out
}

/** How many of ids are selected: all, some or none (no ids: none). */
export function selectionState(selected: ReadonlySet<string>, ids: readonly string[]): 'all' | 'some' | 'none' {
  const n = ids.filter((id) => selected.has(id)).length
  if (!n) return 'none'
  return n === ids.length ? 'all' : 'some'
}

/** Project items by catalogue id as a set of ids. */
export function inProjectSet(map: Record<string, InProjectItem[]> | null | undefined): Set<string> {
  return new Set(Object.entries(map ?? {}).filter(([, v]) => v?.length).map(([k]) => k))
}

export interface ImportPlan {
  /** requirements that will be created */
  requirements: string[]
  /** of requirements, the ones added because they mitigate a chosen risk */
  includedRequirements: string[]
  /** risks that will be created: chosen ones and, with includeRisks, those the requirements mitigate */
  risks: string[]
  /** of risks, the ones added because a requirement mitigates them */
  includedRisks: string[]
  skippedRisks: string[]
  /** tests that will be created: chosen ones and, with includeTests, those verifying the requirements */
  tests: string[]
  /** of tests, the ones added because they verify a chosen requirement */
  included: string[]
  /** chosen items skipped because the project has them */
  skippedRequirements: string[]
  skippedTests: string[]
}

/** What an import of the selection would create (mirrors the server). */
export function importPlan(
  reqIds: readonly string[],
  testIds: readonly string[],
  reqById: ReadonlyMap<string, CatalogRequirement>,
  inReqs: ReadonlySet<string>,
  inTests: ReadonlySet<string>,
  opts: {
    includeTests: boolean
    addCopies: boolean
    riskIds?: readonly string[]
    riskById?: ReadonlyMap<string, CatalogRisk>
    inRisks?: ReadonlySet<string>
    /** add the requirements that mitigate the chosen risks (default true) */
    includeRequirements?: boolean
    /** add the risks the requirements mitigate (default false) */
    includeRisks?: boolean
  },
): ImportPlan {
  const plan: ImportPlan = { requirements: [], includedRequirements: [], risks: [], includedRisks: [], skippedRisks: [], tests: [], included: [], skippedRequirements: [], skippedTests: [] }
  const riskById = opts.riskById ?? new Map<string, CatalogRisk>()
  const chosenRisks = [...new Set(opts.riskIds ?? [])]
  const allReqs = [...new Set(reqIds)]
  const chosenReqs = new Set(allReqs)
  if (opts.includeRequirements ?? true) {
    for (const k of chosenRisks) for (const r of riskById.get(k)?.mitigated_by ?? []) if (!allReqs.includes(r)) allReqs.push(r)
  }
  const allRisks = [...chosenRisks]
  if (opts.includeRisks) {
    for (const r of allReqs) for (const k of reqById.get(r)?.risks ?? []) if (!allRisks.includes(k)) allRisks.push(k)
  }
  for (const id of allReqs) {
    if (inReqs.has(id) && !opts.addCopies) {
      if (chosenReqs.has(id)) plan.skippedRequirements.push(id)
      continue
    }
    plan.requirements.push(id)
    if (!chosenReqs.has(id)) plan.includedRequirements.push(id)
  }
  const inRisks = opts.inRisks ?? new Set<string>()
  for (const id of allRisks) {
    if (inRisks.has(id) && !opts.addCopies) plan.skippedRisks.push(id)
    else {
      plan.risks.push(id)
      if (!chosenRisks.includes(id)) plan.includedRisks.push(id)
    }
  }
  const chosen = new Set(testIds)
  const all = [...chosen]
  if (opts.includeTests) {
    for (const id of allReqs) for (const t of reqById.get(id)?.tests ?? []) if (!chosen.has(t) && !all.includes(t)) all.push(t)
    for (const id of allRisks) for (const t of riskById.get(id)?.tests ?? []) if (!chosen.has(t) && !all.includes(t)) all.push(t)
  }
  for (const id of all) {
    if (inTests.has(id) && !opts.addCopies) plan.skippedTests.push(id)
    else {
      plan.tests.push(id)
      if (!chosen.has(id)) plan.included.push(id)
    }
  }
  return plan
}

// ---------------------------------------------------------------- display

// Obligation keywords in catalogue texts: see obligationKeywords.ts.

/** A list filter for the created items: their keys, or every catalogue item when too long. */
export function createdQuery(keys: readonly string[], max = 3500): string {
  const q = anyOf('key', keys)
  return q.length <= max ? q : 'has:catalog'
}

/** "Catalogue id · standards" shown in the editors for an item's origin. */
export function originText(ref: string, refs: readonly string[] | null | undefined): string {
  return [ref, ...(refs ?? [])].filter(Boolean).join(' · ')
}

/** A catalogue requirement similar to requirements of the project (POST /projects/{pid}/catalog/similar). */
export interface CatalogSimilar {
  catalog_ref: string
  title: string
  similar: { id: string; key: string; title: string; status: string; score: number }[]
}
