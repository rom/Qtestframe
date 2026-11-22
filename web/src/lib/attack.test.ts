import { describe, expect, it } from 'vitest'
import { hasTechnique, normalizeTechnique, parseTechniques, searchTechniques, techniqueLabel, techniqueUrl, type AttackTechnique } from './attack'
import { emptyFilters, filterCatalog, activeFilters, type CatalogRisk } from './catalog'

const list: AttackTechnique[] = [
  { id: 'T1110', name: 'Brute Force', matrix: 'enterprise', tactics: [{ id: 'TA0006', name: 'Credential Access' }], url: 'https://attack.mitre.org/techniques/T1110' },
  { id: 'T1110.004', name: 'Brute Force: Credential Stuffing', matrix: 'enterprise', tactics: [{ id: 'TA0006', name: 'Credential Access' }], url: 'https://attack.mitre.org/techniques/T1110/004' },
  { id: 'T1190', name: 'Exploit Public-Facing Application', matrix: 'enterprise', tactics: [{ id: 'TA0001', name: 'Initial Access' }], url: 'https://attack.mitre.org/techniques/T1190' },
  { id: 'AML.T0051', name: 'LLM Prompt Injection', matrix: 'atlas', tactics: [{ id: 'AML.TA0005', name: 'Execution' }], url: 'https://atlas.mitre.org/techniques/AML.T0051' },
]
const known = new Map(list.map((x) => [x.id, x]))

describe('ATT&CK techniques', () => {
  it('normalises and parses ids against the list', () => {
    expect(normalizeTechnique(' aml.t0051 ')).toBe('AML.T0051')
    expect(parseTechniques('t1110.004, T1190;T1190 T9999', known)).toEqual({ ids: ['T1110.004', 'T1190'], unknown: ['T9999'] })
  })
  it('searches by id and name, exact id first', () => {
    expect(searchTechniques(list, 'T1110').map((x) => x.id)).toEqual(['T1110', 'T1110.004'])
    expect(searchTechniques(list, 'stuffing').map((x) => x.id)).toEqual(['T1110.004'])
    expect(searchTechniques(list, '', 2)).toHaveLength(2)
  })
  it('labels, matches sub-techniques and links', () => {
    expect(techniqueLabel('T1190', known)).toBe('T1190 Exploit Public-Facing Application')
    expect(techniqueLabel('T0000', known)).toBe('T0000')
    expect(hasTechnique(['T1110.004'], 't1110')).toBe(true)
    expect(hasTechnique(['T1110'], 'T1110.004')).toBe(false)
    expect(hasTechnique(['T11100'], 'T1110')).toBe(false)
    expect(techniqueUrl('T1110.004')).toBe('https://attack.mitre.org/techniques/T1110/004/')
    expect(techniqueUrl('T1190')).toBe('https://attack.mitre.org/techniques/T1190/')
    expect(techniqueUrl('AML.T0051')).toBe('https://atlas.mitre.org/techniques/AML.T0051')
  })
})

describe('catalogue risk filters by tactic and technique', () => {
  const risk = (id: string, attack: string[], tactics: string[]): CatalogRisk =>
    ({ id, area: 'a', group: 'g', tags: [], refs: [], title: id, description: '', category: 'security', category_name: 'Security', likelihood: 3, impact: 3, score: 9, level: 'medium', mitigated_by: [], tests: [], mitigation: '', attack, tactics }) as CatalogRisk
  const risks = [risk('A-R01', ['T1110.004'], ['TA0006']), risk('A-R02', ['T1190'], ['TA0001']), risk('A-R03', [], [])]
  it('keeps risks with a technique of the tactic, or the technique or one of its sub-techniques', () => {
    const ids = (f: Partial<ReturnType<typeof emptyFilters>>) => filterCatalog(risks, { ...emptyFilters(), ...f }, new Set()).map((x) => x.id)
    expect(ids({ tactics: ['TA0006'] })).toEqual(['A-R01'])
    expect(ids({ techniques: ['T1110'] })).toEqual(['A-R01'])
    expect(ids({ techniques: ['T1190', 'T1110.004'] })).toEqual(['A-R01', 'A-R02'])
    expect(ids({ q: 't1190' })).toEqual(['A-R02'])
    expect(activeFilters({ ...emptyFilters(), tactics: ['TA0001'], techniques: ['T1190'] })).toBe(2)
  })
})
