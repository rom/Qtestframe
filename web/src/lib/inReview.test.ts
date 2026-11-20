import { describe, expect, it } from 'vitest'
import { daysWaiting, filterInReview, isPersonal, type InReviewItem } from './inReview'

const item = (id: string, reasons: string[]) => ({ entity_id: id, reasons }) as InReviewItem

describe('in review', () => {
  const list = [item('a', ['owner', 'reviewer']), item('b', ['reviewer']), item('c', ['requested']), item('d', ['author'])]
  it('keeps everything, or only what is personally yours', () => {
    expect(filterInReview(list, false)).toHaveLength(4)
    expect(filterInReview(list, true).map((x) => x.entity_id)).toEqual(['a', 'c', 'd'])
    expect(isPersonal(item('x', ['reviewer']))).toBe(false)
  })
  it('counts the days waited', () => {
    const now = new Date('2026-01-10T12:00:00Z')
    expect(daysWaiting('2026-01-08T13:00:00Z', now)).toBe(1)
    expect(daysWaiting('2026-01-11T00:00:00Z', now)).toBe(0)
    expect(daysWaiting('nonsense', now)).toBe(0)
  })
})
