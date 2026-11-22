// The guides bundled at build time: every Markdown page of
// docs/user-guide/<lang>/ and docs/admin-guide/<lang>/ as a string, and every
// screenshot as an asset URL.
import { ADMIN_GUIDE_PAGES, GUIDE_FALLBACK_LANG, GUIDE_PAGES, collectSources, markdownTitle, pageOrder, resolvePage, type GuideSources, type ResolvedPage } from './pages'
import { assetMap, type GuideBook } from './links'
import { pageSections, type Section } from './render'

const markdown = import.meta.glob('../../../docs/user-guide/*/*.md', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const adminMarkdown = import.meta.glob('../../../docs/admin-guide/*/*.md', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const images = import.meta.glob('../../../docs/user-guide/images/*.{png,jpg,jpeg,gif,webp,svg}', { query: '?url', import: 'default', eager: true }) as Record<string, string>

export const ASSETS: Record<string, string> = assetMap(images)

/** One bundled guide: its pages in every language, in reading order. */
export interface GuideContent {
  book: GuideBook
  sources: GuideSources
  /** the page ids in reading order */
  pages: readonly string[]
  getPage(lang: string, page: string): ResolvedPage | null
  getOrder(lang: string): string[]
  /** Page id → title in a language (falls back like the pages themselves). */
  getTitles(lang: string): Record<string, string>
  /** Searchable sections of all pages in a language (built on first use). */
  getSections(lang: string): Section[]
}

export function makeGuide(book: GuideBook, files: Record<string, string>, pages: readonly string[]): GuideContent {
  const sources = collectSources(files)
  const sectionCache = new Map<string, Section[]>()
  const g: GuideContent = {
    book,
    sources,
    pages,
    getPage: (lang, page) => resolvePage(sources, lang, page),
    getOrder: (lang) => pageOrder(sources, lang, GUIDE_FALLBACK_LANG, pages),
    getTitles(lang) {
      const out: Record<string, string> = {}
      for (const id of g.getOrder(lang)) out[id] = markdownTitle(g.getPage(lang, id)?.markdown ?? '', id)
      return out
    },
    getSections(lang) {
      let s = sectionCache.get(lang)
      if (!s) {
        s = g.getOrder(lang).flatMap((id) => pageSections(id, g.getPage(lang, id)?.markdown ?? ''))
        sectionCache.set(lang, s)
      }
      return s
    },
  }
  return g
}

/** The user guide (docs/user-guide). */
export const USER_GUIDE = makeGuide('user', markdown, GUIDE_PAGES)
/** The administrator guide (docs/admin-guide). */
export const ADMIN_GUIDE = makeGuide('admin', adminMarkdown, ADMIN_GUIDE_PAGES)

/** The content of a guide. */
export function guideContent(book: GuideBook): GuideContent {
  return book === 'admin' ? ADMIN_GUIDE : USER_GUIDE
}

// the user guide, as used throughout the app
export const SOURCES: GuideSources = USER_GUIDE.sources
export const getPage = USER_GUIDE.getPage
export const getOrder = USER_GUIDE.getOrder
export const getTitles = (lang: string) => USER_GUIDE.getTitles(lang)
export const getSections = (lang: string) => USER_GUIDE.getSections(lang)
