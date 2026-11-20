import { describe, expect, it } from 'vitest'
import { linkBody, planAddLinks, planRemoveLinks, validLinkUrl } from './bulkItemLinks'
import type { ItemLink } from './types'

const l = (id: string, src: string, type: string, target: string | null, url = '', st = 'requirement', label = '') =>
  ({ id, source_type: st, source_id: src, target_type: type, target_id: target, url, label }) as ItemLink

const existing = [l('1', 'a', 'url', null, 'https://x.org'), l('2', 'a', 'test', 't1'), l('3', 'b', 'run', 'r1'), l('4', 'a', 'test', 't1', '', 'test')]

describe('planAddLinks', () => {
  it('links each selected item to each target, skipping existing links and self', () => {
    const p = planAddLinks(existing, 'requirement', ['a', 'b'], [{ target_type: 'url', url: 'https://x.org' }, { target_type: 'test', target_id: 't1' }, { target_type: 'requirement', target_id: 'a' }], ' Spec ')
    expect(p).toEqual([
      { source_type: 'requirement', source_id: 'b', target_type: 'url', url: 'https://x.org', label: 'Spec' },
      { source_type: 'requirement', source_id: 'b', target_type: 'test', target_id: 't1', label: 'Spec' },
      { source_type: 'requirement', source_id: 'b', target_type: 'requirement', target_id: 'a', label: 'Spec' },
    ])
  })
  it('drops duplicates and needs no label', () => {
    expect(planAddLinks([], 'test', ['a', 'a'], [{ target_type: 'run', target_id: 'r1' }])).toEqual([{ source_type: 'test', source_id: 'a', target_type: 'run', target_id: 'r1' }])
  })
})

describe('planRemoveLinks', () => {
  const ids = (x: ItemLink[]) => x.map((y) => y.id)
  it('takes the links of the selection, of a type and to targets', () => {
    expect(ids(planRemoveLinks(existing, 'requirement', ['a'], null, []))).toEqual(['1', '2'])
    expect(ids(planRemoveLinks(existing, 'requirement', ['a', 'b'], 'run', []))).toEqual(['3'])
    expect(ids(planRemoveLinks(existing, 'requirement', ['a'], 'url', [{ target_type: 'url', url: 'https://x.org' }]))).toEqual(['1'])
    expect(ids(planRemoveLinks(existing, 'requirement', ['a'], 'test', [{ target_type: 'test', target_id: 'other' }]))).toEqual([])
    expect(ids(planRemoveLinks(existing, 'test', ['a'], null, []))).toEqual(['4'])
  })
})

describe('helpers', () => {
  it('recreates a link body', () => {
    expect(linkBody(l('1', 'a', 'url', null, 'https://x.org', 'run', 'Doc'))).toEqual({ source_type: 'run', source_id: 'a', target_type: 'url', url: 'https://x.org', label: 'Doc' })
    expect(linkBody(l('2', 'a', 'test', 't1'))).toEqual({ source_type: 'requirement', source_id: 'a', target_type: 'test', target_id: 't1' })
  })
  it('checks addresses', () => {
    expect(validLinkUrl(' https://x.org ')).toBe(true)
    expect(validLinkUrl('mailto:a')).toBe(true)
    expect(validLinkUrl('javascript:x')).toBe(false)
  })
})
