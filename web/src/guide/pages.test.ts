import { describe, expect, it } from 'vitest'
import { collectSources, markdownTitle, neighbours, normalizePageId, pageOrder, parseGuidePath, resolvePage } from './pages'

const files = {
  '../../../docs/user-guide/en/index.md': '# Guide',
  '../../../docs/user-guide/en/faq.md': '# FAQ',
  '../../../docs/user-guide/en/tutorial.md': '# Tutorial',
  '../../../docs/user-guide/en/zz-extra.md': '# Extra',
  '../../../docs/user-guide/sv/index.md': '# Handbok',
  '../../../docs/user-guide/sv/aa-only-sv.md': '# Bara svenska',
  '../../../docs/user-guide/README.md': '# not a page',
}

describe('parseGuidePath', () => {
  it('reads language and page', () => {
    expect(parseGuidePath('/x/user-guide/sv/tests.md')).toEqual({ lang: 'sv', page: 'tests' })
    expect(parseGuidePath('en/getting-started.md')).toEqual({ lang: 'en', page: 'getting-started' })
  })
  it('ignores other files', () => {
    expect(parseGuidePath('/x/user-guide/README.md')).toBeNull()
    expect(parseGuidePath('/x/images/en-a.png')).toBeNull()
  })
})

describe('collectSources', () => {
  it('groups by language', () => {
    const s = collectSources({ ...files, 'en/bad.md': 42 as unknown as string })
    expect([...s.keys()].sort()).toEqual(['en', 'sv'])
    expect(s.get('en')!.size).toBe(4)
    expect(s.get('sv')!.get('index')).toBe('# Handbok')
  })
})

describe('normalizePageId', () => {
  it('defaults to the index', () => {
    expect(normalizePageId(undefined)).toBe('index')
    expect(normalizePageId('')).toBe('index')
    expect(normalizePageId('../etc')).toBe('index')
    expect(normalizePageId(['faq'])).toBe('faq')
  })
  it('strips .md and lower-cases', () => {
    expect(normalizePageId('FAQ.md')).toBe('faq')
  })
})

describe('resolvePage', () => {
  const s = collectSources(files)
  it('uses the wanted language', () => {
    expect(resolvePage(s, 'sv', 'index')).toEqual({ page: 'index', lang: 'sv', markdown: '# Handbok', fallback: false })
  })
  it('falls back to English', () => {
    expect(resolvePage(s, 'sv', 'faq')).toEqual({ page: 'faq', lang: 'en', markdown: '# FAQ', fallback: true })
    expect(resolvePage(s, 'en', 'faq')?.fallback).toBe(false)
    expect(resolvePage(s, 'de', 'faq')?.lang).toBe('en')
  })
  it('returns null for unknown pages', () => {
    expect(resolvePage(s, 'en', 'nope')).toBeNull()
  })
})

describe('pageOrder and neighbours', () => {
  const s = collectSources(files)
  it('orders known pages first, then the rest alphabetically', () => {
    expect(pageOrder(s, 'en')).toEqual(['index', 'tutorial', 'faq', 'zz-extra'])
    expect(pageOrder(s, 'sv')).toEqual(['index', 'tutorial', 'faq', 'aa-only-sv', 'zz-extra'])
  })
  it('finds previous and next', () => {
    const o = pageOrder(s, 'en')
    expect(neighbours(o, 'index')).toEqual({ prev: null, next: 'tutorial' })
    expect(neighbours(o, 'faq')).toEqual({ prev: 'tutorial', next: 'zz-extra' })
    expect(neighbours(o, 'zz-extra')).toEqual({ prev: 'faq', next: null })
    expect(neighbours(o, 'missing')).toEqual({ prev: null, next: null })
  })
})

describe('markdownTitle', () => {
  it('takes the first h1 outside code', () => {
    expect(markdownTitle('```\n# not this\n```\n\n# The *title* #\n\n# second')).toBe('The title')
    expect(markdownTitle('~~~sh\n# comment\n~~~\n# Real')).toBe('Real')
  })
  it('uses the fallback without a heading', () => {
    expect(markdownTitle('## only h2', 'x')).toBe('x')
    expect(markdownTitle('')).toBe('')
  })
})
