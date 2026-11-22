// Checks on the real administrator guide in docs/admin-guide: both languages
// have the same pages, every page has a title, every link between its pages,
// to anchors and into the user guide resolves, and the generated parts are in
// place.
import { describe, expect, it } from 'vitest'
import MarkdownIt from 'markdown-it'
import type { Token } from 'markdown-it'
import { ADMIN_GUIDE, USER_GUIDE, guideContent, type GuideContent } from './content'
import { ADMIN_GUIDE_PAGES } from './pages'
import { classifyHref } from './links'
import { renderGuide } from './render'

const LANGS = ['en', 'sv']
const raw = new MarkdownIt({ html: true })
const inline = (md: string): Token[] => raw.parse(md, {}).flatMap((t) => t.children ?? [])

const anchorCache = new Map<string, Set<string>>()
function anchors(g: GuideContent, lang: string, page: string): Set<string> {
  const key = `${g.book}/${lang}/${page}`
  let set = anchorCache.get(key)
  if (!set) {
    const r = renderGuide(g.getPage(lang, page)!.markdown, { lang, assets: {}, book: g.book })
    set = new Set([...r.html.matchAll(/ id="([^"]+)"/g)].map((m) => m[1]))
    anchorCache.set(key, set)
  }
  return set
}

describe('the bundled administrator guide', () => {
  it('has every page in English and Swedish', () => {
    for (const lang of LANGS) expect([...(ADMIN_GUIDE.sources.get(lang)?.keys() ?? [])].sort(), lang).toEqual([...ADMIN_GUIDE_PAGES].sort())
    expect(guideContent('admin')).toBe(ADMIN_GUIDE)
    expect(guideContent('user')).toBe(USER_GUIDE)
  })
  it('gives every page a title and starts with the overview page', () => {
    for (const lang of LANGS) for (const [id, title] of Object.entries(ADMIN_GUIDE.getTitles(lang))) expect(title, `${lang}/${id}`).not.toBe(id)
    expect(ADMIN_GUIDE.getOrder('sv')).toEqual([...ADMIN_GUIDE_PAGES])
    expect(ADMIN_GUIDE.getTitles('en').index).toBe('Qtestframe administrator guide')
  })
  it('has working links between its pages, to anchors and into the user guide', () => {
    const broken: string[] = []
    for (const lang of LANGS) {
      for (const page of ADMIN_GUIDE_PAGES) {
        for (const link of inline(ADMIN_GUIDE.getPage(lang, page)!.markdown).filter((t) => t.type === 'link_open')) {
          const href = String(link.attrGet('href') ?? '')
          const target = classifyHref(href)
          if (target.kind === 'external') continue
          if (target.kind !== 'page' && target.kind !== 'anchor') {
            broken.push(`${lang}/${page}: ${href}`)
            continue
          }
          const g = target.kind === 'page' && target.book ? guideContent(target.book) : ADMIN_GUIDE
          // links into the user guide name the folder of their own language
          if (target.kind === 'page' && target.book && !href.includes(`/${lang}/`)) broken.push(`${lang}/${page}: ${href} (language)`)
          const dest = target.kind === 'page' ? target.page : page
          const p = g.getPage(lang, dest)
          if (!p || p.lang !== lang) broken.push(`${lang}/${page}: page ${g.book}/${dest}`)
          else if (target.hash && !anchors(g, lang, dest).has(target.hash)) broken.push(`${lang}/${page}: ${g.book}/${dest}#${target.hash}`)
        }
      }
    }
    expect(broken).toEqual([])
  })
  it('contains the generated reference parts in both languages', () => {
    for (const lang of LANGS) {
      const conf = ADMIN_GUIDE.getPage(lang, 'configuration')!.markdown
      expect(conf, lang).toContain('`server.listen`')
      expect(conf, lang).toContain('`QTF_SECRETS_KEY`')
      expect(ADMIN_GUIDE.getPage(lang, 'cli-reference')!.markdown, lang).toContain('### `qtestframe serve`')
      expect(ADMIN_GUIDE.getPage(lang, 'system-settings')!.markdown, lang).toContain('`auth.session_ttl`')
    }
  })
  it('is searchable', () => {
    expect(ADMIN_GUIDE.getSections('en').length).toBeGreaterThan(50)
    expect(ADMIN_GUIDE.getSections('en').every((s) => (ADMIN_GUIDE_PAGES as readonly string[]).includes(s.page))).toBe(true)
  })
})
