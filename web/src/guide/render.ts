// Markdown → sanitised HTML for the in-app user guide.
//
// markdown-it parses the page (inline HTML such as <kbd> is allowed), the
// token stream is post-processed (heading anchors, link and image
// rewriting, scrollable tables) and DOMPurify sanitises the result. The
// output contains no scripts or event handlers, so it is safe under the
// app's CSP (script-src 'self').
import MarkdownIt from 'markdown-it'
import type { Token } from 'markdown-it'
import DOMPurify from 'dompurify'
import { createSlugger } from './slug'
import { classifyHref, helpPath, resolveImage, type GuideBook } from './links'

export interface Heading {
  level: number
  text: string
  id: string
}

export interface RenderOptions {
  /** language of the page (for image fallbacks) */
  lang: string
  /** image file name → bundled URL */
  assets: Record<string, string>
  /** the guide the page belongs to (default the user guide): links between its pages stay in it */
  book?: GuideBook
  /** accessible name of the scrollable region around tables */
  tableLabel?: string
  /** text shown for a screenshot that is not available */
  missingImage?: (alt: string) => string
}

export interface RenderedPage {
  html: string
  title: string
  /** h2 and h3 headings, for the table of contents */
  headings: Heading[]
}

const md = new MarkdownIt({ html: true, linkify: false, typographer: false })

interface Env {
  [key: string]: unknown
  tableLabel: string
  missingImage: (alt: string) => string
}

const envOf = (env: unknown): Env => (env ?? {}) as Env
const attr = (tok: Token, name: string): string => String(tok.attrGet(name) ?? '')

md.renderer.rules.table_open = (_tokens, _idx, _opts, env) =>
  `<div class="guide-table" role="region" tabindex="0" aria-label="${md.utils.escapeHtml(envOf(env).tableLabel ?? '')}"><table>\n`
md.renderer.rules.table_close = () => '</table></div>\n'

// code blocks scroll sideways on phones: make them reachable with the keyboard
for (const rule of ['fence', 'code_block'] as const) {
  const base = md.renderer.rules[rule]!
  md.renderer.rules[rule] = (tokens, idx, opts, env, self) => base(tokens, idx, opts, env, self).replace(/^<pre(?=[ >])/, '<pre tabindex="0"')
}

const defaultImage = md.renderer.rules.image!
md.renderer.rules.image = (tokens, idx, opts, env, self) => {
  const tok = tokens[idx]
  const alt = self.renderInlineAsText(tok.children ?? [], opts, env ?? {})
  if (!attr(tok, 'src')) {
    const text = envOf(env).missingImage?.(alt) ?? alt
    return `<span class="guide-image-missing" role="img" aria-label="${md.utils.escapeHtml(alt)}">${md.utils.escapeHtml(text)}</span>`
  }
  return defaultImage(tokens, idx, opts, env, self)
}

/** Plain text of an inline token (text, code and image alt texts). */
export function inlineText(tok: Token | undefined): string {
  if (!tok?.children) return tok?.content ?? ''
  return tok.children
    .map((c) => (c.type === 'text' || c.type === 'code_inline' ? c.content : c.type === 'image' ? inlineText(c) : c.type === 'softbreak' ? ' ' : ''))
    .join('')
    .trim()
}

/** Parse a page and rewrite its tokens in place; returns the tokens, headings and title. */
export function processTokens(markdown: string, opts: Pick<RenderOptions, 'lang' | 'assets' | 'book'>): { tokens: Token[]; headings: Heading[]; title: string } {
  const tokens = md.parse(markdown ?? '', {})
  const slug = createSlugger()
  const headings: Heading[] = []
  let title = ''
  for (let i = 0; i < tokens.length; i++) {
    const tok = tokens[i]
    if (tok.type === 'heading_open') {
      const level = Number(tok.tag.slice(1))
      const text = inlineText(tokens[i + 1])
      const id = slug(text)
      tok.attrSet('id', id)
      if (level === 1) {
        tok.attrSet('tabindex', '-1')
        if (!title) title = text
      } else if (level <= 3) headings.push({ level, text, id })
    } else if (tok.type === 'inline' && tok.children) {
      for (const child of tok.children) rewriteInline(child, opts)
    }
  }
  return { tokens, headings, title }
}

function rewriteInline(tok: Token, opts: Pick<RenderOptions, 'lang' | 'assets' | 'book'>) {
  if (tok.type === 'link_open') {
    const target = classifyHref(attr(tok, 'href'))
    switch (target.kind) {
      case 'page': {
        const book = target.book ?? opts.book ?? 'user'
        tok.attrSet('href', helpPath(target.page, target.hash, book))
        tok.attrSet('data-guide-page', target.page)
        tok.attrSet('data-guide-book', book)
        if (target.hash) tok.attrSet('data-guide-hash', target.hash)
        break
      }
      case 'anchor':
        tok.attrSet('href', '#' + encodeURIComponent(target.hash))
        tok.attrSet('data-guide-hash', target.hash)
        break
      case 'app':
        tok.attrSet('href', target.path)
        tok.attrSet('data-guide-app', '')
        break
      case 'external':
        tok.attrSet('href', target.href)
        tok.attrSet('rel', 'noopener noreferrer')
        tok.attrSet('class', 'guide-external')
        break
      default:
        tok.attrs = (tok.attrs ?? []).filter(([k]) => k !== 'href')
    }
  } else if (tok.type === 'image') {
    const url = resolveImage(attr(tok, 'src'), opts.lang, opts.assets)
    if (url) {
      tok.attrSet('src', url)
      tok.attrSet('loading', 'lazy')
      tok.attrSet('decoding', 'async')
    } else {
      tok.attrSet('src', '')
    }
  }
}

/** Render a guide page to sanitised HTML with heading anchors and rewritten links. */
export function renderGuide(markdown: string, opts: RenderOptions): RenderedPage {
  const { tokens, headings, title } = processTokens(markdown, opts)
  const env: Env = { tableLabel: opts.tableLabel ?? 'Table', missingImage: opts.missingImage ?? ((alt) => `[${alt}]`) }
  const raw = md.renderer.render(tokens, md.options, env as unknown as Record<string, unknown>)
  return { html: sanitize(raw), headings, title }
}

/** DOMPurify with the attributes the guide needs; no scripts, handlers or forms survive. */
export function sanitize(html: string): string {
  return DOMPurify.sanitize(html, {
    ADD_ATTR: ['data-guide-page', 'data-guide-book', 'data-guide-hash', 'data-guide-app', 'loading', 'decoding'],
    FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select', 'iframe'],
    FORBID_ATTR: ['style'],
  })
}

/** One searchable section of a page: the text below a heading up to the next one. */
export interface Section {
  page: string
  /** the page title (h1) */
  title: string
  /** the heading of this section ('' for the text before the first h2) */
  heading: string
  /** anchor of the heading ('' for the page start) */
  anchor: string
  text: string
}

/** Split a page into sections at h1–h3, with the same anchors as renderGuide. */
export function pageSections(page: string, markdown: string): Section[] {
  const { tokens, title } = processTokens(markdown, { lang: 'en', assets: {} })
  const out: Section[] = []
  let cur: Section = { page, title, heading: '', anchor: '', text: '' }
  const push = () => {
    cur.text = cur.text.replace(/\s+/g, ' ').trim()
    if (cur.text || cur.heading) out.push(cur)
  }
  for (let i = 0; i < tokens.length; i++) {
    const tok = tokens[i]
    if (tok.type === 'heading_open') {
      const level = Number(tok.tag.slice(1))
      const text = inlineText(tokens[i + 1])
      if (level <= 3) {
        if (level > 1) {
          push()
          cur = { page, title, heading: text, anchor: attr(tok, 'id'), text: '' }
        }
        i++ // skip the heading's inline token
        continue
      }
    }
    if (tok.type === 'inline') cur.text += ' ' + inlineText(tok)
    else if (tok.type === 'fence' || tok.type === 'code_block') cur.text += ' ' + tok.content
  }
  push()
  return out
}
