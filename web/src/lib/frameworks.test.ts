import { afterEach, describe, expect, it, vi } from 'vitest'
import { emb3dKind, frameworkLabel, frameworkUrl, loadD3fend, loadEmb3d, normalizeFrameworkId, pickerOptions, resetFrameworks, share, unknownIds } from './frameworks'
import { activeFilters, emptyFilters, filterCatalog, type CatalogItem, type CatalogRequirement, type CatalogRisk, type CatalogTest } from './catalog'
import { api } from './api'

const known = new Map([
  ['D3-MFA', { name: 'Multi-factor Authentication', url: 'https://d3fend.mitre.org/technique/d3f:Multi-factorAuthentication/' }],
  ['MID-026', { name: 'Secure Firmware Update', url: 'https://emb3d.mitre.org/mitigations/MID-026.html' }],
])

describe('D3FEND and EMB3D ids', () => {
  it('normalises, labels and links ids', () => {
    expect(normalizeFrameworkId(' d3-mfa ')).toBe('D3-MFA')
    expect(emb3dKind('TID-201')).toBe('threat')
    expect(emb3dKind('MID-026')).toBe('mitigation')
    expect(emb3dKind('PID-11')).toBe('')
    expect(frameworkLabel('D3-MFA', known)).toBe('D3-MFA Multi-factor Authentication')
    expect(frameworkLabel('D3-XX', known)).toBe('D3-XX')
    expect(frameworkUrl('D3-MFA', known)).toContain('d3fend.mitre.org')
    expect(frameworkUrl('TID-201')).toBe('https://emb3d.mitre.org/threats/TID-201.html')
    expect(frameworkUrl('MID-001')).toBe('https://emb3d.mitre.org/mitigations/MID-001.html')
    expect(frameworkUrl('D3-XX')).toBe('')
  })
  it('flags unknown ids and keeps them in the picker options', () => {
    expect(unknownIds(['D3-MFA', 'D3-XX'], known)).toEqual(['D3-XX'])
    expect(unknownIds(['D3-XX'], null)).toEqual([])
    const list = [{ id: 'D3-MFA', name: 'Multi-factor Authentication' }]
    expect(pickerOptions(list, ['D3-XX'], true)).toEqual([
      { label: 'D3-XX', value: 'D3-XX' },
      { label: 'D3-MFA Multi-factor Authentication', value: 'D3-MFA' },
    ])
    expect(pickerOptions(list, ['D3-XX'], false)).toHaveLength(1)
    expect(share(1, 4)).toBe('25 %')
    expect(share(0, 0)).toBe('–')
  })
})

describe('loading the lists', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    resetFrameworks()
  })
  it('fetches each list once and again after a failure', async () => {
    const get = vi.spyOn(api, 'get').mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ items: [] })
    await expect(loadD3fend()).rejects.toThrow('offline')
    await loadD3fend()
    await loadD3fend()
    await loadEmb3d()
    expect(get.mock.calls.map((c) => c[0])).toEqual(['/catalog/d3fend', '/catalog/d3fend', '/catalog/emb3d'])
  })
})

describe('catalogue filters by D3FEND and EMB3D', () => {
  const base = { area: 'a', group: 'g', tags: [], refs: [], description: '' }
  const req = (id: string, d3fend: string[], emb3d: string[]): CatalogRequirement =>
    ({ ...base, id, title: id, type: 'security', obligation: 'must', priority: 'high', tests: [], d3fend, emb3d }) as CatalogRequirement
  const test = (id: string, d3fend: string[]): CatalogTest => ({ ...base, id, title: id, execution: 'manual', priority: 'high', requirements: [], steps: 1, d3fend }) as CatalogTest
  const risk = (id: string, emb3d: string[]): CatalogRisk =>
    ({ ...base, id, title: id, category: 'security', category_name: 'Security', likelihood: 3, impact: 3, score: 9, level: 'medium', mitigated_by: [], tests: [], mitigation: '', emb3d }) as CatalogRisk
  const items: CatalogItem[] = [req('A-01', ['D3-MFA'], ['MID-026']), req('A-02', [], []), test('A-T01', ['D3-BA']), risk('A-R01', ['TID-201'])]
  const ids = (f: Partial<ReturnType<typeof emptyFilters>>) => filterCatalog(items, { ...emptyFilters(), ...f }, new Set()).map((x) => x.id)
  it('keeps the items that list one of the ids', () => {
    expect(ids({ d3fend: ['D3-MFA'] })).toEqual(['A-01'])
    expect(ids({ d3fend: ['D3-BA'] })).toEqual(['A-T01'])
    expect(ids({ emb3d: ['TID-201', 'MID-026'] })).toEqual(['A-01', 'A-R01'])
    expect(ids({ q: 'd3-mfa' })).toEqual(['A-01'])
    expect(ids({ q: 'tid-201' })).toEqual(['A-R01'])
    expect(activeFilters({ ...emptyFilters(), d3fend: ['D3-MFA'], emb3d: ['TID-201'] })).toBe(2)
  })
})
