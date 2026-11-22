// Checks on the real guide in docs/user-guide: both languages have the same
// pages, every page has a title, every screenshot has meaningful alt text and
// the right language prefix, and every link between pages and anchors resolves.
import { describe, expect, it } from 'vitest'
import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { ADMIN_GUIDE, ASSETS, SOURCES, getOrder, getPage, getSections, getTitles } from './content'
import { GUIDE_PAGES } from './pages'
import { classifyHref, imageName } from './links'
import { processTokens, renderGuide } from './render'
import { searchGuide } from './search'
import MarkdownIt from 'markdown-it'
import type { Token } from 'markdown-it'

const LANGS = ['en', 'sv']
const IMAGES = join(process.cwd(), '..', 'docs', 'user-guide', 'images')

const raw = new MarkdownIt({ html: true })
/** inline tokens of the unprocessed Markdown (links and images as written) */
function inline(md: string): Token[] {
  return raw.parse(md, {}).flatMap((t) => t.children ?? [])
}
// rendered once per page: the link check asks for the same page many times
const anchorCache = new Map<string, Set<string>>()
function anchors(lang: string, page: string, book: 'user' | 'admin' = 'user'): Set<string> {
  const key = `${book}/${lang}/${page}`
  let set = anchorCache.get(key)
  if (!set) {
    const p = (book === 'admin' ? ADMIN_GUIDE.getPage : getPage)(lang, page)!
    const r = renderGuide(p.markdown, { lang, assets: {}, book })
    set = new Set([...r.html.matchAll(/ id="([^"]+)"/g)].map((m) => m[1]))
    anchorCache.set(key, set)
  }
  return set
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
          if (target.kind === 'page' && target.book === 'admin') {
            // a link into the administrator guide, in the same language
            const p = ADMIN_GUIDE.getPage(lang, dest)
            if (!p || p.lang !== lang || !String(link.attrGet('href')).includes(`/${lang}/`)) broken.push(`${lang}/${page}: admin page ${dest}`)
            else if (target.hash && !anchors(lang, dest, 'admin').has(target.hash)) broken.push(`${lang}/${page}: admin ${dest}#${target.hash}`)
            continue
          }
          if (!getPage(lang, dest) || getPage(lang, dest)!.lang !== lang) broken.push(`${lang}/${page}: page ${dest}`)
          else if (target.hash && !anchors(lang, dest).has(target.hash)) broken.push(`${lang}/${page}: ${dest}#${target.hash}`)
        }
      }
    }
    expect(broken).toEqual([])
  }, 30_000) // renders every page of both languages: slow on a loaded machine
  it('has a FAQ with at least 25 questions in both languages', () => {
    for (const lang of LANGS) {
      const { headings } = processTokens(getPage(lang, 'faq')!.markdown, { lang, assets: {} })
      expect(headings.filter((h) => h.level === 3).length, lang).toBeGreaterThanOrEqual(25)
    }
  })
  it('has a glossary of anchored, alphabetical entries with a topic index in both languages', () => {
    const LABELS: Record<string, { inTool: string; seeAlso: string }> = {
      en: { inTool: '**In Qtestframe:**', seeAlso: '**See also:**' },
      sv: { inTool: '**I Qtestframe:**', seeAlso: '**Se även:**' },
    }
    const counts: number[] = []
    for (const lang of LANGS) {
      const md = getPage(lang, 'glossary')!.markdown
      const { headings } = processTokens(md, { lang, assets: {} })
      // level 2: the topic index, then one heading per initial letter; level 3: the entries
      const [topics, ...letters] = headings.filter((h) => h.level === 2)
      expect(letters.every((h) => /^\p{Lu}$/u.test(h.text)), lang).toBe(true)
      const entries = headings.filter((h) => h.level === 3)
      counts.push(entries.length)
      expect(entries.length, lang).toBeGreaterThanOrEqual(150)
      // alphabetical in the language's own order (Swedish puts Å, Ä, Ö last), each under its letter
      const coll = new Intl.Collator(lang, { sensitivity: 'base' })
      const sorted = [...entries].sort((a, b) => coll.compare(a.text, b.text))
      expect(entries.map((e) => e.text), lang).toEqual(sorted.map((e) => e.text))
      let letter = ''
      for (const h of headings) {
        if (h.level === 2) letter = h.text
        else if (h.level === 3) expect(coll.compare(h.text[0], letter), `${lang}: ${h.text} under ${letter}`).toBe(0)
      }
      // every entry: a definition, "In Qtestframe" and "See also"
      const bodies = md.split(/^### .*$/m).slice(1)
      expect(bodies.length, lang).toBe(entries.length)
      bodies.forEach((body, i) => {
        const text = body.split(/^## /m)[0]
        expect(text, `${lang}: ${entries[i].text}`).toContain(LABELS[lang].inTool)
        expect(text, `${lang}: ${entries[i].text}`).toContain(LABELS[lang].seeAlso)
      })
      // the topic index lists every entry exactly once
      const index = md.slice(md.indexOf(`## ${topics.text}`), md.indexOf(`## ${letters[0].text}\n`))
      const listed = [...index.matchAll(/\]\(#([^)]+)\)/g)].map((m) => decodeURIComponent(m[1])).sort()
      expect(listed, lang).toEqual(entries.map((e) => e.id).sort())
    }
    expect(counts[1]).toBe(counts[0])
  })
  it('finds glossary entries with the guide search', () => {
    const cases: [string, string, string][] = [
      ['en', 'boundary value', 'boundary-value-analysis'],
      ['en', 'suspect link', 'suspect-link'],
      ['sv', 'gränsvärdesanalys', 'gränsvärdesanalys'],
      ['sv', 'misstänkt länk', 'misstänkt-länk'],
    ]
    for (const [lang, query, anchor] of cases) {
      const hits = searchGuide(getSections(lang), query, { order: getOrder(lang) })
      expect(hits.some((h) => h.page === 'glossary' && h.anchor === anchor), `${lang}: ${query}`).toBe(true)
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
