// Links and images inside guide pages: relative Markdown links between pages
// become router paths below /help, image paths become bundled asset URLs.

export type LinkTarget =
  | { kind: 'page'; page: string; hash: string }
  | { kind: 'anchor'; hash: string }
  | { kind: 'app'; path: string }
  | { kind: 'external'; href: string }
  | { kind: 'invalid' }

const SAFE_SCHEMES = /^(https?|mailto):/i

/** What a Markdown link points at. */
export function classifyHref(raw: string): LinkTarget {
  const href = (raw ?? '').trim()
  if (!href) return { kind: 'invalid' }
  if (href.startsWith('#')) return href.length > 1 ? { kind: 'anchor', hash: decodeHash(href.slice(1)) } : { kind: 'invalid' }
  if (/^[a-z][a-z0-9+.-]*:/i.test(href)) return SAFE_SCHEMES.test(href) ? { kind: 'external', href } : { kind: 'invalid' }
  if (href.startsWith('//')) return { kind: 'external', href: 'https:' + href }
  if (href.startsWith('/')) return { kind: 'app', path: href }
  const [pathPart, ...hashParts] = href.split('#')
  const hash = decodeHash(hashParts.join('#'))
  // ./x.md, x.md, ../en/x.md, ../sv/x.md (links into the other language resolve to the current one)
  const path = pathPart.replace(/^(\.\/)+/, '').replace(/^\.\.\/[a-z]{2}\//, '')
  if (!path) return hash ? { kind: 'anchor', hash } : { kind: 'invalid' }
  const m = /^([a-z0-9][a-z0-9-]*)(\.md)?$/i.exec(path)
  if (!m) return { kind: 'invalid' }
  return { kind: 'page', page: m[1].toLowerCase(), hash }
}

function decodeHash(h: string): string {
  try {
    return decodeURIComponent(h)
  } catch {
    return h
  }
}

/** The router path of a guide page: /help, /help/faq, /help/faq#why. */
export function helpPath(page: string, hash = ''): string {
  const base = page === 'index' ? '/help' : `/help/${page}`
  return hash ? `${base}#${encodeURIComponent(hash)}` : base
}

/** The file name of an image reference ("../images/en-x.png" → "en-x.png"), or null for remote/unsafe ones. */
export function imageName(src: string): string | null {
  const s = (src ?? '').trim()
  if (!s || /^[a-z][a-z0-9+.-]*:/i.test(s) || s.startsWith('//')) return null
  const name = s.split(/[?#]/)[0].split('/').pop() ?? ''
  return /^[\w.-]+\.(png|jpe?g|gif|webp|svg)$/i.test(name) ? name : null
}

/** Map of image file name → URL from import.meta.glob('…/images/*', { query: '?url', … }). */
export function assetMap(files: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [path, url] of Object.entries(files)) {
    const name = path.split('/').pop()
    if (name && typeof url === 'string') out[name] = url
  }
  return out
}

/**
 * The URL of an image for a page in `lang`. Screenshots are named
 * "<lang>-<name>.png"; when the language's own one is missing the English
 * one is used, then the file as named.
 */
export function resolveImage(src: string, lang: string, assets: Record<string, string>, fallbackLang = 'en'): string | null {
  const name = imageName(src)
  if (!name) return null
  const m = /^([a-z]{2})-(.+)$/.exec(name)
  const candidates = m ? [`${lang}-${m[2]}`, `${fallbackLang}-${m[2]}`, name] : [name]
  for (const c of candidates) if (assets[c]) return assets[c]
  return null
}
