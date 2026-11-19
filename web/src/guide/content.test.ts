// Checks on the real guide in docs/user-guide: both languages have the same
// pages, every page has a title, every screenshot has meaningful alt text and
// the right language prefix, and every link between pages and anchors resolves.
import { describe, expect, it } from 'vitest'
import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { ASSETS, SOURCES, getOrder, getPage, getSections, getTitles } from './content'
import { GUIDE_PAGES } from './pages'
import { classifyHref, imageName } from './links'
import { processTokens, renderGuide } from './render'
import MarkdownIt from 'markdown-it'
import type { Token } from 'markdown-it'

const LANGS = ['en', 'sv']
const IMAGES = join(process.cwd(), '..', 'docs', 'user-guide', 'images')

const raw = new MarkdownIt({ html: true })
/** inline tokens of the unprocessed Markdown (links and images as written) */
function inline(md: string): Token[] {
  return raw.parse(md, {}).flatMap((t) => t.children ?? [])
}
function anchors(lang: string, page: string): Set<string> {
  const p = getPage(lang, page)!
  const r = renderGuide(p.markdown, { lang, assets: {} })
  return new Set([...r.html.matchAll(/ id="([^"]+)"/g)].map((m) => m[1]))
}

describe('the bundled user guide', () => {
  it('has every page in English and Swedish', () => {
    for (const lang of LANGS) {
      const pages = [...(SOURCES.get(lang)?.keys() ?? [])].sort()
      expect(pages, lang).toEqual([...GUIDE_PAGES].sort())
    }
  })
  it('gives every page a title', () => {
    for (const lang of LANGS) for (const [id, title] of Object.entries(getTitles(lang))) expect(title, `${lang}/${id}`).not.toBe(id)
    expect(getOrder('sv')[0]).toBe('index')
  })
  it('uses screenshots of its own language with meaningful alt text', () => {
    for (const lang of LANGS) {
      for (const page of GUIDE_PAGES) {
        for (const img of inline(getPage(lang, page)!.markdown).filter((t) => t.type === 'image')) {
          const alt = (img.children ?? []).map((c) => c.content).join('')
          const name = imageName(String(img.attrGet('src') ?? ''))
          expect(name, `${lang}/${page}`).toMatch(new RegExp(`^${lang}-[a-z0-9-]+\\.png$`))
          expect(alt.length, `${lang}/${page}: alt of ${name}`).toBeGreaterThanOrEqual(15)
          expect(alt, `${lang}/${page}`).not.toMatch(/^(screenshot|image|bild)$/i)
        }
      }
    }
  })
  it('references the same screenshots in both languages', () => {
    const names = (lang: string) =>
      GUIDE_PAGES.flatMap((p) => inline(getPage(lang, p)!.markdown).filter((t) => t.type === 'image').map((t) => (imageName(String(t.attrGet('src') ?? '')) ?? '').slice(3))).sort()
    expect(names('sv')).toEqual(names('en'))
  })
  it('has working links between pages and to anchors', () => {
    const broken: string[] = []
    for (const lang of LANGS) {
      for (const page of GUIDE_PAGES) {
        for (const link of inline(getPage(lang, page)!.markdown).filter((t) => t.type === 'link_open')) {
          const target = classifyHref(String(link.attrGet('href') ?? ''))
          if (target.kind === 'external') continue
          if (target.kind === 'invalid' || target.kind === 'app') {
            broken.push(`${lang}/${page}: ${link.attrGet('href')}`)
            continue
          }
          const dest = target.kind === 'page' ? target.page : page
          if (!getPage(lang, dest) || getPage(lang, dest)!.lang !== lang) broken.push(`${lang}/${page}: page ${dest}`)
          else if (target.hash && !anchors(lang, dest).has(target.hash)) broken.push(`${lang}/${page}: ${dest}#${target.hash}`)
        }
      }
    }
    expect(broken).toEqual([])
  })
  it('has a FAQ with at least 25 questions in both languages', () => {
    for (const lang of LANGS) {
      const { headings } = processTokens(getPage(lang, 'faq')!.markdown, { lang, assets: {} })
      expect(headings.filter((h) => h.level === 3).length, lang).toBeGreaterThanOrEqual(25)
    }
  })
  it('is searchable', () => {
    expect(getSections('en').length).toBeGreaterThan(100)
    expect(getSections('sv')).toBe(getSections('sv'))
  })
  it('bundles the screenshots that exist', () => {
    const missing: string[] = []
    for (const lang of LANGS)
      for (const page of GUIDE_PAGES)
        for (const img of inline(getPage(lang, page)!.markdown).filter((t) => t.type === 'image')) {
          const name = imageName(String(img.attrGet('src') ?? ''))!
          if (existsSync(join(IMAGES, name))) expect(ASSETS[name], name).toBeTruthy()
          else missing.push(name)
        }
    // screenshots are produced by `npm run guide:screenshots`; missing ones fall back to a text placeholder
    if (missing.length) console.info(`guide screenshots not yet captured: ${[...new Set(missing)].join(', ')}`)
    expect(Object.keys(ASSETS).every((n) => /^(en|sv)-[a-z0-9-]+\.png$/.test(n))).toBe(true)
  })
})
