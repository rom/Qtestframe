// The requirement and test catalogue (GET /catalog…): types and the pure
// logic of the catalogue page – filtering, ranked search, the area tree,
// selection and the import summary. The page loads every item of a kind
// once per language and filters here, so typing in the search field stays
// instant with several hundred items.
import { anyOf } from './query'
import type { Segment } from './highlight'

export type CatalogKind = 'requirement' | 'test'
export type CatalogLang = 'en' | 'sv'

export interface CatalogArea {
  id: string
  prefix: string
  group: string
  name: string
  description: string
  requirements: number
  tests: number
}

export interface CatalogGroup {
  id: string
  name: string
  requirements: number
  tests: number
  areas: CatalogArea[]
}

export interface CatalogTagCount {
  tag: string
  count: number
}

export interface CatalogOverview {
  languages: string[]
  lang: string
  groups: CatalogGroup[]
  requirements: number
  tests: number
  requirement_tags: CatalogTagCount[]
  test_tags: CatalogTagCount[]
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
  priority: string
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
}

export interface CatalogTest extends CatalogBase {
  execution: string
  /** catalogue requirements it verifies */
  requirements: string[]
  /** number of steps */
  steps: number
}

export type CatalogItem = CatalogRequirement | CatalogTest

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

export interface CatalogRequirementDetail extends CatalogRequirement {
  area_name: string
  rationale: string
  criteria: CatalogCriterion[]
  linked_tests: CatalogLinked[]
}

export interface CatalogTestDetail extends CatalogTest {
  area_name: string
  preconditions: string
  expected: string
  step_list: CatalogStep[]
  linked_requirements: CatalogLinked[]
}

/** GET /projects/{pid}/catalog */
export interface ProjectCatalog {
  requirements: Record<string, InProjectItem[]>
  tests: Record<string, InProjectItem[]>
}

export interface CatalogCreated {
  id: string
  key: string
  title: string
  catalog_ref: string
  folder_id: string | null
}

export interface CatalogImportResult {
  requirements: CatalogCreated[]
  tests: CatalogCreated[]
  links: { id: string; requirement_id: string; requirement_key: string; test_case_id: string; test_case_key: string }[]
  skipped: { kind: CatalogKind; catalog_ref: string; reason: string; existing?: InProjectItem[] }[]
  folders_created: number
}

export const isRequirement = (x: CatalogItem): x is CatalogRequirement => 'obligation' in x

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
    [(isRequirement(item) ? item.tests : item.requirements).join(' ').toLowerCase(), 2],
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
  hideInProject: boolean
}

export function emptyFilters(): CatalogFilters {
  return { node: 'all', q: '', types: [], obligations: [], priorities: [], tags: [], executions: [], hideInProject: false }
}

/** Number of filters set besides the tree node and the search. */
export function activeFilters(f: CatalogFilters): number {
  return [f.types, f.obligations, f.priorities, f.tags, f.executions].filter((x) => x.length).length + (f.hideInProject ? 1 : 0)
}

export function matchesNode(item: CatalogItem, node: NodeKey): boolean {
  if (!node || node === 'all') return true
  if (node.startsWith('g:')) return item.group === node.slice(2)
  if (node.startsWith('a:')) return item.area === node.slice(2)
  return true
}

const okAny = (want: readonly string[], v: string) => !want.length || want.includes(v)

/** Whether the item passes the field filters (not the node or the search). */
export function matchesFields(item: CatalogItem, f: CatalogFilters, inProject: ReadonlySet<string>): boolean {
  if (f.hideInProject && inProject.has(item.id)) return false
  if (!okAny(f.priorities, item.priority)) return false
  if (f.tags.length && !item.tags.some((t) => f.tags.includes(t))) return false
  if (isRequirement(item)) return okAny(f.types, item.type) && okAny(f.obligations, item.obligation)
  return okAny(f.executions, item.execution)
}

/**
 * Items passing the filters: ranked by relevance when searching, else in
 * catalogue order. `node: false` ignores the tree node (for the counts).
 */
export function filterCatalog<T extends CatalogItem>(items: readonly T[], f: CatalogFilters, inProject: ReadonlySet<string>, opts: { node?: boolean } = {}): T[] {
  const words = terms(f.q)
  const hits: { item: T; score: number; i: number }[] = []
  items.forEach((item, i) => {
    if (opts.node !== false && !matchesNode(item, f.node)) return
    if (!matchesFields(item, f, inProject)) return
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

/** Groups and areas as tree nodes with counts for the kind. */
export function catalogTree(groups: readonly CatalogGroup[], kind: CatalogKind, counts: ReadonlyMap<NodeKey, number>): CatalogTreeNode[] {
  const total = (x: { requirements: number; tests: number }) => (kind === 'requirement' ? x.requirements : x.tests)
  return groups
    .filter((g) => total(g) > 0)
    .map((g) => ({
      key: `g:${g.id}`,
      label: g.name,
      count: counts.get(`g:${g.id}`) ?? 0,
      total: total(g),
      children: g.areas.filter((a) => total(a) > 0).map((a) => ({ key: `a:${a.id}`, label: a.name, count: counts.get(`a:${a.id}`) ?? 0, total: total(a) })),
    }))
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
  opts: { includeTests: boolean; addCopies: boolean },
): ImportPlan {
  const plan: ImportPlan = { requirements: [], tests: [], included: [], skippedRequirements: [], skippedTests: [] }
  for (const id of new Set(reqIds)) (inReqs.has(id) && !opts.addCopies ? plan.skippedRequirements : plan.requirements).push(id)
  const chosen = new Set(testIds)
  const all = [...chosen]
  if (opts.includeTests) {
    for (const id of new Set(reqIds)) for (const t of reqById.get(id)?.tests ?? []) if (!chosen.has(t) && !all.includes(t)) all.push(t)
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

const KEYWORDS: [RegExp, string][] = [
  [/MUST NOT|SHALL NOT|SKA INTE/u, 'kw-strong'],
  [/MUST|SHALL|SKA/u, 'kw-strong'],
  [/SHOULD NOT|BÖR INTE/u, 'kw-medium'],
  [/SHOULD|BÖR/u, 'kw-medium'],
  [/MAY|FÅR/u, 'kw-weak'],
]
const KEYWORD_RE = /(?<![\p{L}\p{N}])(MUST NOT|SHALL NOT|SKA INTE|SHOULD NOT|BÖR INTE|MUST|SHALL|SKA|SHOULD|BÖR|MAY|FÅR)(?![\p{L}\p{N}])/gu

/** The text split at its upper-case obligation keywords (as the requirement editor marks them). */
export function keywordSegments(text: string): Segment[] {
  const out: Segment[] = []
  let last = 0
  for (const m of text.matchAll(KEYWORD_RE)) {
    const i = m.index ?? 0
    if (i > last) out.push({ text: text.slice(last, i), mark: false })
    out.push({ text: m[0], mark: true, kind: KEYWORDS.find(([re]) => re.test(m[0]))?.[1] })
    last = i + m[0].length
  }
  if (last < text.length) out.push({ text: text.slice(last), mark: false })
  return out
}

/** A list filter for the created items: their keys, or every catalogue item when too long. */
export function createdQuery(keys: readonly string[], max = 3500): string {
  const q = anyOf('key', keys)
  return q.length <= max ? q : 'has:catalog'
}

/** "Catalogue id · standards" shown in the editors for an item's origin. */
export function originText(ref: string, refs: readonly string[] | null | undefined): string {
  return [ref, ...(refs ?? [])].filter(Boolean).join(' · ')
}
