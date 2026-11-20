// The user guide bundled at build time: every Markdown page of
// docs/user-guide/<lang>/ as a string and every screenshot as an asset URL.
import { collectSources, markdownTitle, pageOrder, resolvePage, type GuideSources, type ResolvedPage } from './pages'
import { assetMap } from './links'
import { pageSections, type Section } from './render'

const markdown = import.meta.glob('../../../docs/user-guide/*/*.md', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const images = import.meta.glob('../../../docs/user-guide/images/*.{png,jpg,jpeg,gif,webp,svg}', { query: '?url', import: 'default', eager: true }) as Record<string, string>

export const SOURCES: GuideSources = collectSources(markdown)
export const ASSETS: Record<string, string> = assetMap(images)

export function getPage(lang: string, page: string): ResolvedPage | null {
  return resolvePage(SOURCES, lang, page)
}

export function getOrder(lang: string): string[] {
  return pageOrder(SOURCES, lang)
}

/** Page id → title in a language (falls back like the pages themselves). */
export function getTitles(lang: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const id of getOrder(lang)) out[id] = markdownTitle(getPage(lang, id)?.markdown ?? '', id)
  return out
}

const sectionCache = new Map<string, Section[]>()
/** Searchable sections of all pages in a language (built on first use). */
export function getSections(lang: string): Section[] {
  let s = sectionCache.get(lang)
  if (!s) {
    s = getOrder(lang).flatMap((id) => pageSections(id, getPage(lang, id)?.markdown ?? ''))
    sectionCache.set(lang, s)
  }
  return s
}
