import { describe, expect, it } from 'vitest'
import { compileGuide, printableHtml } from './compile'

const pages = [
  { page: 'index', markdown: '# Guide\n\nStart with [Tests](tests.md) or [their steps](tests.md#steps).\n\n![Layout](../images/en-layout.png)\n\n## Overview\n' },
  {
    page: 'tests',
    markdown: '# Tests\n\n## Overview\n\nSee [steps](#steps), [the guide](index.md), [the FAQ](faq.md), [web](https://example.org) and [/go/X](/go/X).\n\n## Steps\n\n```\n[not a link](tests.md)\n```\n\nUse `[x](tests.md)` literally. ![Missing](../images/en-nothing.png)\n\n![Run](../images/en-run.png "The runner")\n',
  },
]

describe('compileGuide', () => {
  const g = compileGuide(pages, { lang: 'sv', images: ['en-layout.png', 'sv-layout.png', 'en-run.png'] })

  it('joins the pages in order, titled by the first', () => {
    expect(g.title).toBe('Guide')
    expect(g.markdown.indexOf('# Guide')).toBeLessThan(g.markdown.indexOf('# Tests'))
  })
  it('turns links between pages into anchors of the joined document', () => {
    expect(g.markdown).toContain('[Tests](#tests)')
    expect(g.markdown).toContain('[their steps](#steps)')
    // the second "Overview" is overview-1 in the joined document
    expect(g.markdown).toContain('[steps](#steps)')
    expect(g.markdown).toContain('[the guide](#guide)')
  })
  it('keeps unknown pages, web and app links, and code as they are', () => {
    expect(g.markdown).toContain('[the FAQ](faq.md)')
    expect(g.markdown).toContain('[web](https://example.org)')
    expect(g.markdown).toContain('[/go/X](/go/X)')
    expect(g.markdown).toContain('```\n[not a link](tests.md)\n```')
    expect(g.markdown).toContain('`[x](tests.md)`')
  })
  it('points images into the images folder, in the language or else English', () => {
    expect(g.markdown).toContain('![Layout](images/sv-layout.png)')
    expect(g.markdown).toContain('![Run](images/en-run.png "The runner")')
    expect(g.markdown).toContain('![Missing](../images/en-nothing.png)')
    expect(g.images).toEqual(['en-run.png', 'sv-layout.png'])
  })
  it('handles a single page, no pages and another image folder', () => {
    const one = compileGuide([pages[0]], { lang: 'en', images: ['en-layout.png'], imageDir: 'pics/' })
    expect(one.markdown).toContain('![Layout](pics/en-layout.png)')
    expect(compileGuide([], { lang: 'en', images: [] })).toEqual({ title: '', markdown: '\n', images: [] })
  })
})

describe('compileGuide with two guides', () => {
  const admin = [{ page: 'index', markdown: '# Admin\n\nSee [members](../../user-guide/en/administration.md#members), [security](security.md) and [x](../../admin-guide/en/index.md).\n' }]
  it('links into the other guide with its URL, and keeps own links as anchors', () => {
    const g = compileGuide(admin, { lang: 'en', images: [], book: 'admin', otherGuideUrl: (book, page, hash) => `https://example.org/${book}/${page}.md#${hash}` })
    expect(g.markdown).toContain('[members](https://example.org/user/administration.md#members)')
    expect(g.markdown).toContain('[security](security.md)')
    expect(g.markdown).toContain('[x](#admin)')
  })
  it('leaves links into the other guide as written without a URL', () => {
    const g = compileGuide(admin, { lang: 'en', images: [], book: 'admin' })
    expect(g.markdown).toContain('[members](../../user-guide/en/administration.md#members)')
  })
})

describe('printableHtml', () => {
  it('renders the joined document with anchors that match its links', () => {
    const g = compileGuide(pages, { lang: 'en', images: ['en-layout.png', 'en-run.png'] })
    const html = printableHtml(g, { lang: 'en', assets: { 'en-layout.png': 'file:///x/en-layout.png', 'en-run.png': 'file:///x/en-run.png' } })
    expect(html).toContain('<html lang="en">')
    expect(html).toContain('<title>Guide</title>')
    expect(html).toContain('id="overview-1"')
    expect(html).toContain('id="steps"')
    expect(html).toContain('href="#steps"')
    expect(html).toContain('src="file:///x/en-layout.png"')
    expect(html).toContain('loading="eager"')
    expect(html).toContain('@page')
  })
})
