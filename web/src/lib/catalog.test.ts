import { describe, expect, it } from 'vitest'
import {
  activeFilters,
  areaName,
  catalogTree,
  complianceSet,
  createdQuery,
  defaultCatalogue,
  emptyFilters,
  filterCatalog,
  groupSources,
  importPlan,
  inProjectSet,
  isRequirement,
  itemSources,
  matchesNode,
  nodeCounts,
  nodeLabel,
  originText,
  searchScore,
  selectionState,
  sourceLabel,
  tagOptions,
  terms,
  toggleIds,
  type CatalogGroup,
  type CatalogRequirement,
  type CatalogSource,
  type CatalogTest,
} from './catalog'

const req = (id: string, over: Partial<CatalogRequirement> = {}): CatalogRequirement => ({
  id,
  area: 'authentication',
  group: 'security',
  type: 'security',
  obligation: 'must',
  priority: 'high',
  tags: ['password'],
  refs: ['NIST SP 800-63B §5.1.1.2'],
  tests: [],
  title: 'Minimum password length',
  description: 'The system MUST reject a new password shorter than 12 characters.',
  ...over,
})
const test = (id: string, over: Partial<CatalogTest> = {}): CatalogTest => ({
  id,
  area: 'authentication',
  group: 'security',
  priority: 'high',
  execution: 'manual',
  tags: ['password'],
  refs: [],
  requirements: [],
  steps: 2,
  title: 'Short password is rejected',
  description: 'Checks the minimum length.',
  ...over,
})

const reqs = [
  req('AUTH-01', { tests: ['AUTH-T01', 'AUTH-T02'] }),
  req('AUTH-02', { title: 'Account lockout', description: 'The system SHOULD lock an account after 10 failed attempts.', obligation: 'should', priority: 'medium', tags: ['lockout'] }),
  req('PERF-01', { area: 'performance', group: 'quality', type: 'performance', title: 'Response time of searches', description: 'A search MAY take at most 2 seconds.', obligation: 'may', priority: 'low', tags: ['latency'], refs: ['ISO/IEC 25010'] }),
]
const tests = [test('AUTH-T01', { requirements: ['AUTH-01'] }), test('AUTH-T02', { execution: 'automated', title: 'Password policy by API', requirements: ['AUTH-01'] }), test('PERF-T01', { area: 'performance', group: 'quality', tags: [] })]

const groups: CatalogGroup[] = [
  { id: 'security', name: 'Security', requirements: 2, tests: 2, areas: [{ id: 'authentication', prefix: 'AUTH', group: 'security', name: 'Authentication', description: '', requirements: 2, tests: 2 }] },
  {
    id: 'quality',
    name: 'Performance',
    requirements: 1,
    tests: 1,
    areas: [
      { id: 'performance', prefix: 'PERF', group: 'quality', name: 'Performance', description: '', requirements: 1, tests: 1 },
      { id: 'empty', prefix: 'EMP', group: 'quality', name: 'Empty', description: '', requirements: 0, tests: 1 },
    ],
  },
  { id: 'none', name: 'None', requirements: 0, tests: 0, areas: [] },
]

describe('search', () => {
  it('splits words', () => {
    expect(terms('  Lösenord, AUTH-01 §5.1 ')).toEqual(['lösenord', 'auth-01', '§5.1'])
    expect(terms('')).toEqual([])
  })
  it('scores fields and requires every word', () => {
    expect(searchScore(reqs[0], [])).toBe(1)
    expect(searchScore(reqs[0], ['auth-01'])).toBe(1000)
    expect(searchScore(reqs[0], ['password', 'nothing'])).toBe(0)
    // title at a word start beats description
    expect(searchScore(reqs[0], ['password'])).toBe(20)
    expect(searchScore(reqs[0], ['assword'])).toBe(10)
    expect(searchScore(reqs[0], ['characters'])).toBe(6)
    expect(searchScore(reqs[0], ['minimum', 'password'])).toBe(50 + 20 + 20)
    expect(searchScore(reqs[0], ['auth-t02'])).toBe(4)
    expect(searchScore(tests[0], ['auth-01'])).toBe(4)
  })
})

describe('filters', () => {
  const none = new Set<string>()
  it('filters by node and fields', () => {
    expect(filterCatalog(reqs, emptyFilters(), none).map((x) => x.id)).toEqual(['AUTH-01', 'AUTH-02', 'PERF-01'])
    expect(filterCatalog(reqs, { ...emptyFilters(), node: 'g:quality' }, none).map((x) => x.id)).toEqual(['PERF-01'])
    expect(filterCatalog(reqs, { ...emptyFilters(), node: 'a:authentication', obligations: ['should'] }, none).map((x) => x.id)).toEqual(['AUTH-02'])
    expect(filterCatalog(reqs, { ...emptyFilters(), types: ['performance'], priorities: ['low'] }, none)).toHaveLength(1)
    expect(filterCatalog(reqs, { ...emptyFilters(), tags: ['lockout', 'latency'] }, none)).toHaveLength(2)
    expect(filterCatalog(reqs, { ...emptyFilters(), hideInProject: true }, new Set(['AUTH-01']))).toHaveLength(2)
    expect(filterCatalog(tests, { ...emptyFilters(), executions: ['automated'] }, none).map((x) => x.id)).toEqual(['AUTH-T02'])
    // the node can be ignored (counts for the tree)
    expect(filterCatalog(reqs, { ...emptyFilters(), node: 'g:quality' }, none, { node: false })).toHaveLength(3)
  })
  it('ranks search hits', () => {
    const hits = filterCatalog(reqs, { ...emptyFilters(), q: 'account' }, new Set())
    expect(hits.map((x) => x.id)).toEqual(['AUTH-02'])
    const ranked = filterCatalog([reqs[1], reqs[0]], { ...emptyFilters(), q: 'password' }, new Set())
    expect(ranked.map((x) => x.id)).toEqual(['AUTH-01'])
    const ties = filterCatalog([test('X-T1', { title: 'a b' }), test('X-T2', { title: 'b a' })], { ...emptyFilters(), q: 'a b' }, new Set())
    expect(ties.map((x) => x.id)).toEqual(['X-T1', 'X-T2'])
    const order = filterCatalog([test('X-T1', { title: 'x', description: 'a' }), test('X-T2', { title: 'a' })], { ...emptyFilters(), q: 'a' }, new Set())
    expect(order.map((x) => x.id)).toEqual(['X-T2', 'X-T1'])
  })
  it('counts active filters and nodes', () => {
    expect(activeFilters(emptyFilters())).toBe(0)
    expect(activeFilters({ ...emptyFilters(), tags: ['a'], hideInProject: true, q: 'x' })).toBe(2)
    expect(matchesNode(reqs[0], '')).toBe(true)
    expect(matchesNode(reqs[0], 'x:y')).toBe(true)
    expect(matchesNode(reqs[0], 'a:performance')).toBe(false)
    expect(isRequirement(reqs[0])).toBe(true)
    expect(isRequirement(tests[0])).toBe(false)
  })
})

describe('tree', () => {
  it('builds group and area nodes with counts', () => {
    const counts = nodeCounts(reqs)
    expect(counts.get('all')).toBe(3)
    expect(counts.get('g:security')).toBe(2)
    expect(counts.get('a:performance')).toBe(1)
    const tree = catalogTree(groups, 'requirement', counts)
    expect(tree.map((n) => n.key)).toEqual(['g:security', 'g:quality'])
    expect(tree[1].children?.map((n) => n.key)).toEqual(['a:performance'])
    expect(tree[0]).toMatchObject({ label: 'Security', count: 2, total: 2 })
    const tt = catalogTree(groups, 'test', new Map())
    expect(tt[1].children?.map((n) => [n.key, n.count])).toEqual([
      ['a:performance', 0],
      ['a:empty', 0],
    ])
  })
  it('names nodes and areas', () => {
    expect(nodeLabel(groups, 'g:quality')).toBe('Performance')
    expect(nodeLabel(groups, 'a:authentication')).toBe('Authentication')
    expect(nodeLabel(groups, 'all')).toBe('')
    expect(areaName(groups, 'performance')).toBe('Performance')
    expect(areaName(groups, 'x')).toBe('x')
  })
  it('lists tags by use', () => {
    expect(tagOptions([...reqs, ...tests])).toEqual(['password', 'latency', 'lockout'])
  })
})

describe('selection', () => {
  it('toggles and reports the state', () => {
    let s = toggleIds(new Set(), ['a', 'b'], true)
    expect(selectionState(s, ['a', 'b'])).toBe('all')
    s = toggleIds(s, ['a'], false)
    expect([...s]).toEqual(['b'])
    expect(selectionState(s, ['a', 'b'])).toBe('some')
    expect(selectionState(s, ['c'])).toBe('none')
    expect(selectionState(s, [])).toBe('none')
  })
  it('reads the project map', () => {
    expect([...inProjectSet({ 'AUTH-01': [{ id: '1', key: 'R-1', title: 'x' }], 'AUTH-02': [] })]).toEqual(['AUTH-01'])
    expect(inProjectSet(null).size).toBe(0)
  })
  it('plans an import like the server', () => {
    const byId = new Map(reqs.map((r) => [r.id, r]))
    const plan = importPlan(['AUTH-01', 'AUTH-02', 'AUTH-01'], ['PERF-T01'], byId, new Set(['AUTH-02']), new Set(['AUTH-T02']), { includeTests: true, addCopies: false })
    expect(plan).toEqual({ requirements: ['AUTH-01'], tests: ['PERF-T01', 'AUTH-T01'], included: ['AUTH-T01'], skippedRequirements: ['AUTH-02'], skippedTests: ['AUTH-T02'] })
    const copies = importPlan(['AUTH-01', 'NOPE'], ['AUTH-T01'], byId, new Set(['AUTH-01']), new Set(['AUTH-T02']), { includeTests: true, addCopies: true })
    expect(copies.requirements).toEqual(['AUTH-01', 'NOPE'])
    expect(copies.tests).toEqual(['AUTH-T01', 'AUTH-T02'])
    expect(copies.included).toEqual(['AUTH-T02'])
    expect(importPlan(['AUTH-01'], [], byId, new Set(), new Set(), { includeTests: false, addCopies: false }).tests).toEqual([])
  })
})

describe('display', () => {
  it('builds the list query of created items', () => {
    expect(createdQuery(['P-REQ-1'])).toBe('key=P-REQ-1')
    expect(createdQuery(['P-REQ-1', 'P-REQ-2'])).toBe('(key=P-REQ-1 OR key=P-REQ-2)')
    expect(createdQuery(['P-REQ-1', 'P-REQ-2'], 10)).toBe('has:catalog')
    expect(createdQuery([])).toBe('')
  })
  it('describes the origin', () => {
    expect(originText('AUTH-01', ['NIST', 'OWASP'])).toBe('AUTH-01 · NIST · OWASP')
    expect(originText('AUTH-01', null)).toBe('AUTH-01')
  })
})

describe('catalogue types and sources', () => {
  const cra = req('CRA-01', { sources: ['cra', 'iso-27001'], tests: ['CRA-T01', 'AUTH-T01'], catalogue: 'hardware' })
  const gdpr = req('PRIV-01', { sources: ['gdpr'], tests: ['AUTH-T01'] })
  const all = [cra, gdpr, req('X-01')]
  const byId = new Map(all.map((r) => [r.id, r]))
  it('filters by catalogue type and source', () => {
    const f = { ...emptyFilters(), sources: ['cra'] }
    expect(filterCatalog(all, f, new Set()).map((x) => x.id)).toEqual(['CRA-01'])
    expect(filterCatalog(all, { ...emptyFilters(), sources: ['cra', 'gdpr'] }, new Set()).map((x) => x.id)).toEqual(['CRA-01', 'PRIV-01'])
    expect(filterCatalog(all, { ...emptyFilters(), catalogue: 'software' }, new Set()).map((x) => x.id)).toEqual(['PRIV-01', 'X-01'])
    // a test brings the sources of the requirements it verifies
    const ts = [test('CRA-T01', { requirements: ['CRA-01'] }), test('OWN-T01', { sources: ['gdpr'] }), test('NONE-T01')]
    expect(filterCatalog(ts, f, new Set(), { reqById: byId }).map((x) => x.id)).toEqual(['CRA-T01'])
    expect(itemSources(ts[1])).toEqual(['gdpr'])
    expect(itemSources(ts[0], byId).sort()).toEqual(['cra', 'iso-27001'])
  })
  it('lists the requirements and tests of sources', () => {
    expect(complianceSet(all, ['cra', 'gdpr'])).toEqual({ requirements: ['CRA-01', 'PRIV-01'], tests: ['CRA-T01', 'AUTH-T01'] })
    expect(complianceSet(all, [])).toEqual({ requirements: [], tests: [] })
  })
  it('defaults the catalogue type from the project type', () => {
    expect(defaultCatalogue('hardware', ['software', 'hardware'])).toBe('hardware')
    expect(defaultCatalogue('business', ['software', 'governance'])).toBe('governance')
    expect(defaultCatalogue('business', ['software'])).toBe('')
    expect(defaultCatalogue('general', ['software'])).toBe('')
    expect(defaultCatalogue(undefined, ['software'])).toBe('')
  })
  it('groups and searches sources', () => {
    const src = (id: string, kind: string, name: string, short?: string): CatalogSource => ({ id, kind, name, short, requirements: 1, tests: 1, catalogues: [], jurisdiction: 'EU' })
    const list = [src('gdpr', 'law', 'General Data Protection Regulation', 'GDPR'), src('nis2', 'directive', 'NIS2 Directive'), src('iso', 'standard', 'ISO/IEC 27001'), src('x', 'other', 'Other')]
    const kinds = [
      { id: 'law', name: 'Laws' },
      { id: 'directive', name: 'Directives' },
      { id: 'standard', name: 'Standards' },
    ]
    expect(groupSources(list, kinds).map((g) => [g.kind, g.label, g.sources.length])).toEqual([
      ['law', 'Laws', 1],
      ['directive', 'Directives', 1],
      ['standard', 'Standards', 1],
      ['other', 'other', 1],
    ])
    expect(groupSources(list, kinds, 'gdpr').map((g) => g.kind)).toEqual(['law'])
    expect(groupSources(list, kinds, '27001 iso').map((g) => g.sources[0].id)).toEqual(['iso'])
    expect(sourceLabel(list[0])).toBe('GDPR')
    expect(sourceLabel(list[1])).toBe('NIS2 Directive')
  })
  it('limits the tree to the areas of a catalogue type', () => {
    const gs: CatalogGroup[] = [
      {
        id: 'security',
        name: 'Security',
        requirements: 3,
        tests: 3,
        areas: [
          { id: 'a', prefix: 'A', group: 'security', catalogue: 'software', name: 'A', description: '', requirements: 1, tests: 1 },
          { id: 'b', prefix: 'B', group: 'security', catalogue: 'hardware', name: 'B', description: '', requirements: 2, tests: 2 },
        ],
      },
      { id: 'other', name: 'Other', requirements: 1, tests: 1, areas: [{ id: 'c', prefix: 'C', group: 'other', name: 'C', description: '', requirements: 1, tests: 1 }] },
    ]
    const tree = catalogTree(gs, 'requirement', new Map(), 'hardware')
    expect(tree.map((n) => [n.key, n.total, n.children?.map((c) => c.key)])).toEqual([['g:security', 2, ['a:b']]])
    expect(catalogTree(gs, 'requirement', new Map(), 'software').map((n) => n.key)).toEqual(['g:security', 'g:other'])
  })
})
