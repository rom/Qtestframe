import { describe, expect, it } from 'vitest'
import { cloudSize, sortTags, tagQuery } from './tagCloud'

const tc = (tag: string, total: number) => ({ tag, total, requirements: total, tests: 0, documents: 0 })

describe('tag cloud', () => {
  it('scales font sizes logarithmically', () => {
    expect(cloudSize(1, 1, 100)).toBe(13)
    expect(cloudSize(100, 1, 100)).toBe(30)
    expect(cloudSize(10, 1, 100)).toBe(22)
    expect(cloudSize(5, 5, 5)).toBe(22)
    expect(cloudSize(0, 1, 10)).toBe(13)
  })
  it('builds queries for selected tags', () => {
    expect(tagQuery([], 'all')).toBe('')
    expect(tagQuery(['gdpr'], 'all')).toBe('tag=gdpr')
    expect(tagQuery(['gdpr', 'two words'], 'all')).toBe('tag=gdpr AND tag="two words"')
    expect(tagQuery(['gdpr', 'login'], 'any')).toBe('(tag=gdpr OR tag=login)')
  })
  it('sorts by use or name', () => {
    const list = [tc('b', 2), tc('A', 5), tc('c', 2)]
    expect(sortTags(list, 'count').map((x) => x.tag)).toEqual(['A', 'b', 'c'])
    expect(sortTags(list, 'name').map((x) => x.tag)).toEqual(['A', 'b', 'c'])
    expect(sortTags([tc('z', 1), tc('a', 1)], 'count').map((x) => x.tag)).toEqual(['a', 'z'])
  })
})
