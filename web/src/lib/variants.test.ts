import { describe, expect, it } from 'vitest'
import {
  applicabilityBody,
  applicabilityMode,
  appliesToAll,
  attributeRows,
  attributeText,
  buildLabel,
  changeRef,
  descendants,
  parseAttributes,
  passRate,
  randomSecret,
  splitKeys,
  variantDepth,
  variantOptions,
  variantPassed,
  type Variant,
} from './variants'

const v = (id: string, parent: string | null, name = id): Variant => ({
  id,
  parent_id: parent,
  name,
  path: name,
  project_id: 'p',
  description: '',
  kind: '',
  attributes: {},
  configuration_ids: [],
  position: 0,
  created_by: '',
  created_at: '',
  updated_at: '',
})

describe('variant hierarchy', () => {
  const all = [v('eu', null), v('de', 'eu'), v('ber', 'de'), v('us', null), v('loop', 'loop')]
  it('computes depths and descendants', () => {
    expect(variantDepth(all[2], all)).toBe(2)
    expect(variantDepth(all[3], all)).toBe(0)
    expect(variantDepth(all[4], all)).toBe(0)
    expect([...descendants('eu', all)].sort()).toEqual(['ber', 'de', 'eu'])
    expect(variantOptions(all).map((o) => o.depth)).toEqual([0, 1, 2, 0, 0])
  })
})

describe('attributes', () => {
  it('parses rows', () => {
    expect(parseAttributes([{ name: ' region ', value: 'EU ' }, { name: '', value: '' }, { name: 'max load', value: '5' }])).toEqual({
      attributes: { region: 'EU', max_load: '5' },
      problems: [],
    })
    expect(parseAttributes([{ name: '', value: 'x' }, { name: 'a', value: '1' }, { name: 'A', value: '2' }]).problems).toEqual(['name', 'duplicate'])
  })
  it('formats', () => {
    expect(attributeRows({ b: '2', a: '1' })).toEqual([
      { name: 'a', value: '1' },
      { name: 'b', value: '2' },
    ])
    expect(attributeText({ voltage: '230V', region: 'EU', flag: '' })).toBe('flag · region=EU · voltage=230V')
    expect(attributeText(null)).toBe('')
  })
})

describe('applicability', () => {
  it('knows "all variants"', () => {
    expect(appliesToAll(null)).toBe(true)
    expect(appliesToAll({ variant_ids: [], expression: ' ' })).toBe(true)
    expect(appliesToAll({ variant_ids: ['x'], expression: '' })).toBe(false)
  })
  it('picks the editor mode and builds the body', () => {
    expect(applicabilityMode(undefined)).toBe('all')
    expect(applicabilityMode({ variant_ids: [], expression: 'region:EU' })).toBe('expression')
    expect(applicabilityMode({ variant_ids: ['a'], expression: 'x' })).toBe('selected')
    expect(applicabilityBody('all', ['a'], 'x')).toEqual({ variant_ids: [], expression: '' })
    expect(applicabilityBody('selected', ['a', 'a', 'b'], 'x')).toEqual({ variant_ids: ['a', 'b'], expression: '' })
    expect(applicabilityBody('expression', ['a'], ' region:EU ')).toEqual({ variant_ids: [], expression: 'region:EU' })
  })
})

describe('code helpers', () => {
  it('labels changes and builds', () => {
    expect(changeRef({ kind: 'commit', provider: 'github', ref: 'abcdef0123456789' })).toBe('abcdef01')
    expect(changeRef({ kind: 'commit', provider: 'manual', ref: 'v1.2' })).toBe('v1.2')
    expect(changeRef({ kind: 'merge_request', provider: 'gitlab', ref: '12' })).toBe('!12')
    expect(changeRef({ kind: 'merge_request', provider: 'github', ref: '7' })).toBe('#7')
    expect(changeRef({ kind: 'merge_request', provider: 'manual', ref: 'https://x' })).toBe('https://x')
    expect(buildLabel({ number: '', external_id: '41' })).toBe('41')
    expect(buildLabel({ number: '1.4', external_id: '41' })).toBe('1.4')
    expect(buildLabel(null)).toBe('')
  })
  it('computes rates', () => {
    expect(passRate({ passed: 3, failed: 1, not_run: 5 })).toBe(75)
    expect(passRate({ not_run: 2 })).toBeNull()
    expect(passRate(undefined)).toBeNull()
    expect(variantPassed({ passed: 1, applicable: 4 })).toEqual({ passed: 1, applicable: 4, percent: 25 })
    expect(variantPassed({}).percent).toBeNull()
  })
  it('makes secrets and splits keys', () => {
    expect(randomSecret()).toMatch(/^[0-9a-f]{48}$/)
    expect(splitKeys('P-REQ-1, P-REQ-2\nP-REQ-1;P-TC-3 ')).toEqual(['P-REQ-1', 'P-REQ-2', 'P-TC-3'])
  })
})
