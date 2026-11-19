import { describe, expect, it } from 'vitest'
import type { Section } from './render'
import { fold, scoreSection, searchGuide, snippet, terms, wordStarts } from './search'

const sec = (page: string, heading: string, text: string, title = page): Section => ({ page, title, heading, anchor: heading.toLowerCase().replace(/\s+/g, '-'), text })

const SECTIONS: Section[] = [
  sec('runs', '', 'Runs collect executions.', 'Test runs'),
  sec('runs', 'CI import', 'Post results from a pipeline with an API token.', 'Test runs'),
  sec('runs', 'Keyboard shortcuts', 'Press P to pass a step and F to fail it.', 'Test runs'),
  sec('faq', 'How do I import CI results?', 'Use the CI import button on the run page.', 'FAQ'),
  sec('faq', 'Why is my query invalid?', 'Unknown fields are an error.', 'FAQ'),
  sec('sv', 'Översikt', 'Täckning visas på översikten.', 'Översikt och täckning'),
]

describe('fold and terms', () => {
  it('lower-cases and removes diacritics, keeping a position map', () => {
    const f = fold('Ärende ﬁ')
    expect(f.text).toBe('arende fi')
    expect(f.map[0]).toBe(0)
    expect(f.map[f.text.length]).toBe('Ärende ﬁ'.length)
  })
  it('splits queries into distinct terms', () => {
    expect(terms('CI  import, ci')).toEqual(['ci', 'import'])
    expect(terms('a 5 x')).toEqual(['5'])
    expect(terms('')).toEqual([])
  })
  it('finds word starts only', () => {
    expect(wordStarts('import reimport imports', 'import')).toEqual([0, 16])
    expect(wordStarts('abc', '')).toEqual([])
  })
})

describe('scoreSection', () => {
  it('needs every term', () => {
    expect(scoreSection(SECTIONS[1], ['ci', 'banana'])).toBe(0)
    expect(scoreSection(SECTIONS[1], [])).toBe(0)
  })
  it('weights headings above body text', () => {
    const inHeading = scoreSection(SECTIONS[1], ['import'])
    const inBody = scoreSection(sec('x', 'Other', 'import import'), ['import'])
    expect(inHeading).toBeGreaterThan(inBody)
  })
  it('rewards the phrase', () => {
    expect(scoreSection(SECTIONS[3], ['ci', 'import'], 'ci import')).toBeGreaterThan(scoreSection(SECTIONS[3], ['ci', 'import']))
    expect(scoreSection(sec('x', 'h', 'ci import here'), ['ci', 'import'], 'ci import')).toBeGreaterThan(scoreSection(sec('x', 'h', 'ci import here'), ['ci', 'import']))
  })
})

describe('searchGuide', () => {
  it('ranks heading hits first, the exact phrase highest', () => {
    const hits = searchGuide(SECTIONS, 'ci import', { order: ['runs', 'faq'] })
    expect(hits[0]).toMatchObject({ page: 'runs', heading: 'CI import', anchor: 'ci-import' })
    expect(hits[1]).toMatchObject({ page: 'faq', heading: 'How do I import CI results?' })
  })
  it('matches without diacritics and case', () => {
    expect(searchGuide(SECTIONS, 'OVERSIKT')[0].page).toBe('sv')
    expect(searchGuide(SECTIONS, 'täck')[0].heading).toBe('Översikt')
  })
  it('limits hits overall and per page', () => {
    expect(searchGuide(SECTIONS, 'runs', { perPage: 1 })).toHaveLength(1)
    expect(searchGuide(SECTIONS, 'the', { limit: 1 })).toHaveLength(1)
    expect(searchGuide(SECTIONS, 'zzz')).toEqual([])
    expect(searchGuide(SECTIONS, '  ')).toEqual([])
  })
  it('orders ties by page order, then position', () => {
    const s = [sec('b', 'x', 'term'), sec('a', 'x', 'term'), sec('a', 'y', 'term')]
    expect(searchGuide(s, 'term', { order: ['a', 'b'] }).map((h) => h.page + h.heading)).toEqual(['ax', 'ay', 'bx'])
    expect(searchGuide(s, 'term').map((h) => h.page + h.heading)).toEqual(['bx', 'ax', 'ay'])
  })
})

describe('snippet', () => {
  const long = 'Lorem ipsum dolor sit amet. '.repeat(10) + 'The CI import posts results. ' + 'Tail words go on and on. '.repeat(10)
  it('shows the text around the first hit', () => {
    const s = snippet(long, ['import'], 80)
    expect(s).toContain('import')
    expect(s.startsWith('… ')).toBe(true)
    expect(s.endsWith(' …')).toBe(true)
    expect(s.length).toBeLessThanOrEqual(90)
  })
  it('returns short text unchanged and cuts long text without hits', () => {
    expect(snippet('short', ['x'])).toBe('short')
    expect(snippet(long, ['zzz'], 40).endsWith(' …')).toBe(true)
  })
  it('starts at the beginning for early hits', () => {
    expect(snippet(long, ['lorem'], 60).startsWith('Lorem')).toBe(true)
  })
})
