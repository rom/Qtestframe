import { describe, expect, it } from 'vitest'
import { chunks, linkPlan } from './bulkLinks'
import type { TraceLink } from './types'

const l = (id: string, r: string, t: string, suspect = false) => ({ id, requirement_id: r, test_case_id: t, suspect_since: suspect ? '2026-01-01' : null }) as TraceLink

describe('linkPlan', () => {
  const links = [l('1', 'r1', 't1'), l('2', 'r1', 't2', true), l('3', 'r2', 't1', true)]
  it('links the missing pairs of the selection', () => {
    const p = linkPlan(links, ['r1', 'r2'], ['t1', 't2'])
    expect(p.toLink).toEqual([{ requirement_id: 'r2', test_case_id: 't2' }])
    expect(p.inScope.map((x) => x.id)).toEqual(['1', '2', '3'])
    expect(p.suspect.map((x) => x.id)).toEqual(['2', '3'])
  })
  it('takes all tests or all requirements when one side is empty', () => {
    expect(linkPlan(links, ['r1'], []).inScope.map((x) => x.id)).toEqual(['1', '2'])
    expect(linkPlan(links, [], ['t1']).inScope.map((x) => x.id)).toEqual(['1', '3'])
    expect(linkPlan(links, ['r1'], []).toLink).toEqual([])
  })
  it('has nothing in scope without a selection', () => {
    expect(linkPlan(links, [], [])).toEqual({ toLink: [], inScope: [], suspect: [] })
  })
})

describe('helpers', () => {
  it('chunks', () => {
    expect(chunks([1, 2, 3], 2)).toEqual([[1, 2], [3]])
    expect(chunks([])).toEqual([])
  })
})
