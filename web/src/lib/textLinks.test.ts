import { describe, expect, it } from 'vitest'
import { hasTextLinks, splitTextLinks } from './textLinks'

describe('text links', () => {
  it('finds item keys and addresses', () => {
    expect(splitTextLinks('See SHOP-REQ-12 and SHOP-TC-3, also https://example.com/a?b=1.')).toEqual([
      { text: 'See ' },
      { text: 'SHOP-REQ-12', key: 'SHOP-REQ-12' },
      { text: ' and ' },
      { text: 'SHOP-TC-3', key: 'SHOP-TC-3' },
      { text: ', also ' },
      { text: 'https://example.com/a?b=1', url: 'https://example.com/a?b=1' },
      { text: '.' },
    ])
  })
  it('finds defect keys', () => {
    expect(splitTextLinks('Fails, see SHOP-BUG-4.')).toEqual([{ text: 'Fails, see ' }, { text: 'SHOP-BUG-4', key: 'SHOP-BUG-4' }, { text: '.' }])
    expect(hasTextLinks('SHOP-BUG-4')).toBe(true)
    expect(splitTextLinks('SHOP-BUGS-4')).toEqual([{ text: 'SHOP-BUGS-4' }])
  })
  it('keeps parentheses that belong to an address', () => {
    expect(splitTextLinks('(https://en.wikipedia.org/wiki/Foo_(bar))')[1]).toEqual({ text: 'https://en.wikipedia.org/wiki/Foo_(bar)', url: 'https://en.wikipedia.org/wiki/Foo_(bar)' })
    expect(splitTextLinks('(see https://x.org)')[1].url).toBe('https://x.org')
  })
  it('ignores what only looks similar', () => {
    expect(splitTextLinks('shop-req-1 SHOP-REQ- XSHOP-TC-1y https:// ftp://x.org')).toEqual([{ text: 'shop-req-1 SHOP-REQ- XSHOP-TC-1y https:// ftp://x.org' }])
    expect(splitTextLinks('')).toEqual([])
    expect(splitTextLinks(null)).toEqual([])
  })
  it('checks cheaply', () => {
    expect(hasTextLinks('A-REQ-1')).toBe(true)
    expect(hasTextLinks('go to HTTPS://x.org')).toBe(true)
    expect(hasTextLinks('plain')).toBe(false)
    expect(hasTextLinks(undefined)).toBe(false)
  })
})
