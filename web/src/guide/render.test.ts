import { describe, expect, it } from 'vitest'
import { inlineText, pageSections, processTokens, renderGuide, sanitize } from './render'

const assets = { 'en-editor.png': '/assets/en-editor.png', 'sv-editor.png': '/assets/sv-editor.png' }
const MD = `# Requirements

Intro with **bold** and \`code\`.

## Write a requirement

See [the FAQ](faq.md#why), [above](#write-a-requirement), [projects](/projects),
[RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) and [bad](javascript:alert(1)).

![The requirement editor](../images/en-editor.png)

![Missing shot](../images/en-nothing.png)

### Fields

| Field | Meaning |
|---|---|
| Title | short |

## Write a requirement

\`\`\`text
# not a heading
status:approved
\`\`\`

<script>alert(1)</script>
`

describe('renderGuide', () => {
  const r = renderGuide(MD, { lang: 'sv', assets, tableLabel: 'Tabell', missingImage: (a) => `Saknas: ${a}` })
  const doc = new DOMParser().parseFromString(r.html, 'text/html')

  it('returns the title and the h2/h3 headings with unique anchors', () => {
    expect(r.title).toBe('Requirements')
    expect(r.headings).toEqual([
      { level: 2, text: 'Write a requirement', id: 'write-a-requirement' },
      { level: 3, text: 'Fields', id: 'fields' },
      { level: 2, text: 'Write a requirement', id: 'write-a-requirement-1' },
    ])
    expect(doc.querySelector('h1')?.getAttribute('tabindex')).toBe('-1')
    expect(doc.querySelectorAll('h2')[1].id).toBe('write-a-requirement-1')
  })
  it('rewrites guide links to router paths', () => {
    const a = doc.querySelector('a[data-guide-page]')!
    expect(a.getAttribute('href')).toBe('/help/faq#why')
    expect(a.getAttribute('data-guide-page')).toBe('faq')
    expect(a.getAttribute('data-guide-hash')).toBe('why')
    const anchor = [...doc.querySelectorAll('a')].find((x) => x.textContent === 'above')!
    expect(anchor.getAttribute('href')).toBe('#write-a-requirement')
    const app = [...doc.querySelectorAll('a')].find((x) => x.textContent === 'projects')!
    expect(app.hasAttribute('data-guide-app')).toBe(true)
  })
  it('marks external links and drops unsafe ones', () => {
    const ext = doc.querySelector('a.guide-external')!
    expect(ext.getAttribute('rel')).toBe('noopener noreferrer')
    const bad = [...doc.querySelectorAll('a')].find((x) => x.textContent === 'bad')
    expect(bad?.getAttribute('href') ?? null).toBeNull()
    expect(r.html).not.toMatch(/href="javascript/i)
  })
  it('uses the bundled screenshot of the language, with alt text', () => {
    const img = doc.querySelector('img')!
    expect(img.getAttribute('src')).toBe('/assets/sv-editor.png')
    expect(img.getAttribute('alt')).toBe('The requirement editor')
    expect(img.getAttribute('loading')).toBe('lazy')
    const missing = doc.querySelector('.guide-image-missing')!
    expect(missing.getAttribute('role')).toBe('img')
    expect(missing.getAttribute('aria-label')).toBe('Missing shot')
    expect(missing.textContent).toBe('Saknas: Missing shot')
  })
  it('wraps tables in a labelled scroll region', () => {
    const wrap = doc.querySelector('.guide-table')!
    expect(wrap.getAttribute('role')).toBe('region')
    expect(wrap.getAttribute('aria-label')).toBe('Tabell')
    expect(wrap.querySelector('table')).not.toBeNull()
  })
  it('makes code blocks keyboard-scrollable', () => {
    expect(doc.querySelector('pre')?.getAttribute('tabindex')).toBe('0')
    expect(renderGuide('    indented code', { lang: 'en', assets: {} }).html).toContain('<pre tabindex="0">')
  })
  it('never emits raw HTML or scripts', () => {
    expect(doc.querySelector('script')).toBeNull()
    expect(r.html).not.toContain('alert(1)</')
    expect(renderGuide('Press <kbd>Enter</kbd> <img src=x onerror="alert(1)">', { lang: 'en', assets: {} }).html).toBe('<p>Press <kbd>Enter</kbd> <img src="x"></p>\n')
  })
  it('has defaults for labels', () => {
    const d = renderGuide('| a |\n|---|\n| b |\n\n![x](../images/zz.png)', { lang: 'en', assets: {} })
    expect(d.html).toContain('aria-label="Table"')
    expect(d.html).toContain('[x]')
    expect(d.title).toBe('')
    expect(renderGuide(undefined as unknown as string, { lang: 'en', assets: {} }).html).toBe('')
  })
})

describe('sanitize', () => {
  it('removes handlers, styles and forms', () => {
    const out = sanitize('<p onclick="x()" style="color:red">a</p><form><input></form><img src="x" onerror="y()"><iframe src="//x"></iframe>')
    expect(out).toBe('<p>a</p><img src="x">')
  })
  it('keeps the attributes the guide uses', () => {
    expect(sanitize('<a href="/help/x" data-guide-page="x" data-guide-hash="y">x</a>')).toContain('data-guide-page="x"')
  })
})

describe('inlineText', () => {
  it('collects text, code and alt texts', () => {
    const { tokens } = processTokens('a *b* `c`\nd ![e](f.png) [g](h)', { lang: 'en', assets: {} })
    expect(inlineText(tokens[1])).toBe('a b c d e g')
    expect(inlineText(undefined)).toBe('')
  })
})

describe('pageSections', () => {
  it('splits at h2/h3 with the rendered anchors', () => {
    const s = pageSections('requirements', MD)
    expect(s.map((x) => [x.heading, x.anchor])).toEqual([
      ['', ''],
      ['Write a requirement', 'write-a-requirement'],
      ['Fields', 'fields'],
      ['Write a requirement', 'write-a-requirement-1'],
    ])
    expect(s[0].title).toBe('Requirements')
    expect(s[0].text).toBe('Intro with bold and code.')
    expect(s[1].text).toContain('The requirement editor')
    expect(s[2].text).toContain('Title')
    expect(s[3].text).toContain('status:approved')
  })
  it('keeps h4 text in its section', () => {
    const s = pageSections('x', '# T\n\n## A\n\n#### Deep\n\ntext')
    expect(s).toHaveLength(1)
    expect(s[0].text).toBe('Deep text')
  })
  it('drops empty leading sections', () => {
    expect(pageSections('x', '# Only title')).toEqual([])
  })
})
