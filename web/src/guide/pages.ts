// The pages of the user guide (docs/user-guide/<lang>/<page>.md): their
// order, titles, and which language version to show.

/** Pages in reading order; files not listed here follow alphabetically. */
export const GUIDE_PAGES = [
  'index',
  'getting-started',
  'tutorial',
  'requirements',
  'tests',
  'catalogue',
  'runs',
  'releases-and-risks',
  'defects',
  'reviews-and-notifications',
  'traceability-and-coverage',
  'baselines',
  'search-and-filters',
  'reports-and-export',
  'documents',
  'administration',
  'accessibility-and-languages',
  'faq',
  'references',
] as const

/** A page id of the bundled guide. */
export type GuidePage = (typeof GUIDE_PAGES)[number]

/** The language every page exists in; other languages fall back to it. */
export const GUIDE_FALLBACK_LANG = 'en'

/** lang → page → Markdown source */
export type GuideSources = Map<string, Map<string, string>>

/** "…/user-guide/sv/tests.md" → { lang: "sv", page: "tests" } (null for other paths). */
export function parseGuidePath(path: string): { lang: string; page: string } | null {
  const m = /(?:^|\/)([a-z]{2})\/([a-z0-9][a-z0-9-]*)\.md$/.exec(path)
  return m ? { lang: m[1], page: m[2] } : null
}

/** Group the result of import.meta.glob('…/user-guide/**\/*.md') by language and page. */
export function collectSources(files: Record<string, string>): GuideSources {
  const out: GuideSources = new Map()
  for (const [path, md] of Object.entries(files)) {
    const p = parseGuidePath(path)
    if (!p || typeof md !== 'string') continue
    if (!out.has(p.lang)) out.set(p.lang, new Map())
    out.get(p.lang)!.set(p.page, md)
  }
  return out
}

/** Route parameter → page id ("" / undefined → "index", "faq.md" → "faq"). */
export function normalizePageId(raw: unknown): string {
  const v = Array.isArray(raw) ? raw[0] : raw
  const s = typeof v === 'string' ? v.trim().toLowerCase().replace(/\.md$/, '') : ''
  return /^[a-z0-9][a-z0-9-]*$/.test(s) ? s : 'index'
}

export interface ResolvedPage {
  page: string
  /** language of the returned Markdown */
  lang: string
  markdown: string
  /** true when the requested language had no such page */
  fallback: boolean
}

/** The page in the wanted language, else in the fallback language, else null. */
export function resolvePage(sources: GuideSources, lang: string, page: string, fallbackLang = GUIDE_FALLBACK_LANG): ResolvedPage | null {
  const own = sources.get(lang)?.get(page)
  if (own !== undefined) return { page, lang, markdown: own, fallback: false }
  const fb = sources.get(fallbackLang)?.get(page)
  if (fb !== undefined) return { page, lang: fallbackLang, markdown: fb, fallback: lang !== fallbackLang }
  return null
}

/** Page ids available in a language (including fallbacks), in reading order. */
export function pageOrder(sources: GuideSources, lang: string, fallbackLang = GUIDE_FALLBACK_LANG): string[] {
  const ids = new Set([...(sources.get(fallbackLang)?.keys() ?? []), ...(sources.get(lang)?.keys() ?? [])])
  const known = GUIDE_PAGES.filter((p) => ids.has(p)) as string[]
  const extra = [...ids].filter((p) => !known.includes(p)).sort()
  return [...known, ...extra]
}

/** The previous and next page of `page` in `order`. */
export function neighbours(order: readonly string[], page: string): { prev: string | null; next: string | null } {
  const i = order.indexOf(page)
  if (i < 0) return { prev: null, next: null }
  return { prev: i > 0 ? order[i - 1] : null, next: i < order.length - 1 ? order[i + 1] : null }
}

/** The text of the first level-1 heading (ATX "# Title"), outside code fences. */
export function markdownTitle(markdown: string, fallback = ''): string {
  let fence: string | null = null
  for (const line of markdown.split(/\r?\n/)) {
    const f = /^\s{0,3}(`{3,}|~{3,})/.exec(line)
    if (f) {
      if (!fence) fence = f[1][0]
      else if (f[1][0] === fence) fence = null
      continue
    }
    if (fence) continue
    const m = /^\s{0,3}#\s+(.+?)\s*#*\s*$/.exec(line)
    if (m) return m[1].replace(/[*_`]/g, '').trim()
  }
  return fallback
}
