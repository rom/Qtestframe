// The whole user guide of one language as a single document (the first
// page, index, has the table of contents): one Markdown
// file (links between pages become anchors inside it, images point to an
// images/ folder next to it) and, from that, one printable HTML page for
// the PDF. Used by scripts/guide-export.mjs (make guide).
import MarkdownIt from 'markdown-it'
import type { Token } from 'markdown-it'
import { createSlugger } from './slug'
import { classifyHref, imageName, resolveImage, type GuideBook } from './links'
import { inlineText, processTokens } from './render'

export interface GuidePageSource {
  page: string
  markdown: string
}

export interface CompileOptions {
  /** language of the guide (for screenshots: "<lang>-name.png", else English) */
  lang: string
  /** image file names that exist */
  images: Iterable<string>
  /** folder of the images, relative to the compiled file */
  imageDir?: string
  /** the guide being compiled (default the user guide) */
  book?: GuideBook
  /** the URL of a page of the other guide, for links into it (null leaves the link as written) */
  otherGuideUrl?: (book: GuideBook, page: string, hash: string) => string | null
}

export interface CompiledGuide {
  title: string
  markdown: string
  /** image file names the document uses (to copy next to it) */
  images: string[]
}

const md = new MarkdownIt({ html: true, linkify: false, typographer: false })

/** The text of every heading of a page in order (all levels, like the renderer slugs them). */
function headings(markdown: string): { level: number; text: string }[] {
  const tokens: Token[] = md.parse(markdown, {})
  const out: { level: number; text: string }[] = []
  tokens.forEach((t, i) => {
    if (t.type === 'heading_open') out.push({ level: Number(t.tag.slice(1)), text: inlineText(tokens[i + 1]) })
  })
  return out
}

/** Calls fn on the text outside fenced code blocks and inline code spans. */
function outsideCode(markdown: string, fn: (text: string) => string): string {
  const out: string[] = []
  let fence: string | null = null
  let buf: string[] = []
  const flush = () => {
    if (buf.length) out.push(buf.join('\n').split(/(`+[^`]*`+)/).map((part, i) => (i % 2 ? part : fn(part))).join(''))
    buf = []
  }
  for (const line of markdown.split('\n')) {
    const f = /^\s{0,3}(`{3,}|~{3,})/.exec(line)
    if (f && (!fence || f[1][0] === fence)) {
      if (!fence) {
        flush()
        fence = f[1][0]
      } else fence = null
      out.push(line)
      continue
    }
    if (fence) out.push(line)
    else buf.push(line)
  }
  flush()
  return out.join('\n')
}

/**
 * Join the pages (in reading order) into one Markdown document. Heading
 * anchors are those of the joined document (GitHub style, repeated
 * headings get "-1", "-2", …), so links between pages keep working.
 */
export function compileGuide(pages: GuidePageSource[], opts: CompileOptions): CompiledGuide {
  const imageDir = (opts.imageDir ?? 'images').replace(/\/+$/, '')
  const assets: Record<string, string> = {}
  for (const name of opts.images) assets[name] = name
  const used = new Set<string>()

  // anchors: page → its own slug → the slug in the joined document
  const global = createSlugger()
  const anchors = new Map<string, Map<string, string>>()
  const first = new Map<string, string>()
  let title = ''
  for (const p of pages) {
    const local = createSlugger()
    const map = new Map<string, string>()
    for (const h of headings(p.markdown)) {
      const g = global(h.text)
      map.set(local(h.text), g)
      if (!first.has(p.page)) first.set(p.page, g)
      if (h.level === 1 && !title) title = h.text
    }
    anchors.set(p.page, map)
  }
  const anchorOf = (page: string, hash: string): string | null => (hash && anchors.get(page)?.get(hash)) || first.get(page) || null

  const LINK = /(!?)\[((?:[^\]\\]|\\.)*)\]\(\s*<?([^)\s>]*)>?(\s+"[^"]*")?\s*\)/g
  const parts = pages.map((p) =>
    outsideCode(p.markdown.trim(), (text) =>
      text.replace(LINK, (all, bang: string, label: string, href: string, title = '') => {
        if (bang) {
          const url = resolveImage(href, opts.lang, assets)
          if (!url) return all
          used.add(url)
          return `![${label}](${imageDir}/${url}${title})`
        }
        const target = classifyHref(href)
        let to: string | null = null
        if (target.kind === 'page' && target.book && target.book !== (opts.book ?? 'user')) {
          const url = opts.otherGuideUrl?.(target.book, target.page, target.hash)
          return url ? `[${label}](${url}${title})` : all
        }
        if (target.kind === 'page') to = anchorOf(target.page, target.hash)
        else if (target.kind === 'anchor') to = anchorOf(p.page, target.hash)
        return to ? `[${label}](#${to}${title})` : all
      }),
    ),
  )
  return { title, markdown: parts.join('\n\n') + '\n', images: [...used].sort() }
}

export interface PrintOptions {
  lang: string
  /** image file name → URL the browser can load (e.g. file://…) */
  assets: Record<string, string>
  tableLabel?: string
}

/** A self-contained HTML page of a compiled guide, styled for printing (A4, a chapter per page). */
export function printableHtml(guide: CompiledGuide, opts: PrintOptions): string {
  const { tokens } = processTokens(guide.markdown, { lang: opts.lang, assets: opts.assets })
  // print every screenshot (the renderer loads them lazily)
  for (const t of tokens) {
    for (const c of t.children ?? []) {
      if (c.type === 'image') c.attrSet('loading', 'eager')
    }
  }
  const body = md.renderer.render(tokens, md.options, {})
  const esc = md.utils.escapeHtml
  return `<!doctype html>
<html lang="${esc(opts.lang)}">
<head>
<meta charset="utf-8">
<title>${esc(guide.title)}</title>
<style>
@page { size: A4; margin: 18mm 16mm 20mm; }
:root { color-scheme: light; }
body { font: 10.5pt/1.5 system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; color: #1e293b; margin: 0; }
h1, h2, h3, h4 { color: #0f172a; line-height: 1.25; break-after: avoid; }
h1 { font-size: 22pt; margin: 0 0 12pt; padding-bottom: 6pt; border-bottom: 2px solid #4f46e5; }
h1:not(:first-of-type) { break-before: page; }
h2 { font-size: 15pt; margin: 18pt 0 6pt; }
h3 { font-size: 12pt; margin: 14pt 0 4pt; }
p, li { orphans: 3; widows: 3; }
a { color: #3730a3; text-decoration: none; }
code, kbd, pre { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 9pt; }
code { background: #f1f5f9; padding: 0 3px; border-radius: 3px; }
kbd { border: 1px solid #cbd5e1; border-bottom-width: 2px; border-radius: 3px; padding: 0 4px; }
pre { background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 4px; padding: 8px 10px; white-space: pre-wrap; break-inside: avoid; }
pre code { background: none; padding: 0; }
table { border-collapse: collapse; width: 100%; margin: 8pt 0; font-size: 9.5pt; break-inside: auto; }
tr { break-inside: avoid; }
th, td { border: 1px solid #cbd5e1; padding: 4px 6px; text-align: left; vertical-align: top; }
th { background: #eef2ff; }
img { max-width: 100%; height: auto; border: 1px solid #e2e8f0; border-radius: 4px; display: block; margin: 6pt 0; break-inside: avoid; }
blockquote { margin: 8pt 0; padding: 4pt 10pt; border-left: 3px solid #a5b4fc; background: #f8fafc; }
.guide-table { overflow: visible; }
</style>
</head>
<body>
${body}
</body>
</html>
`
}

/** The image file name a Markdown image source refers to (re-exported for the export script). */
export { imageName }
