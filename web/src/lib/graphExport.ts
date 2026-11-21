// Saving and printing the dependency graph: a self-contained SVG (styles
// inlined, so it looks the same outside the app), a PNG rendered from it,
// Markdown with a Mermaid flowchart, and printing through a hidden frame.
import type { Direction } from './graphLayout'

/** CSS properties copied from the live elements into the saved SVG. */
const STYLE_PROPS = ['fill', 'stroke', 'stroke-width', 'stroke-dasharray', 'opacity', 'font-family', 'font-size', 'font-weight'] as const

/**
 * A standalone SVG document of the rendered graph: computed styles of the
 * live nodes inlined, the zoom removed (natural size from the viewBox), a
 * background and a title added, links made absolute.
 */
export function standaloneSvg(svg: SVGSVGElement, opts: { title: string; background: string; baseUrl: string }): string {
  const clone = svg.cloneNode(true) as SVGSVGElement
  const live = svg.querySelectorAll<SVGElement>('*')
  const copies = clone.querySelectorAll<SVGElement>('*')
  live.forEach((el, i) => {
    const cs = getComputedStyle(el)
    const decl = STYLE_PROPS.map((p) => [p, cs.getPropertyValue(p)] as const)
      .filter(([, v]) => v && v !== 'normal' && v !== 'none' && !v.includes('var('))
      .map(([p, v]) => `${p}:${v}`)
      .join(';')
    const copy = copies[i]
    if (decl && copy) copy.setAttribute('style', [copy.getAttribute('style'), decl].filter(Boolean).join(';'))
  })
  for (const a of clone.querySelectorAll('a')) {
    const href = a.getAttribute('href')
    if (href && href.startsWith('/')) a.setAttribute('href', new URL(href, opts.baseUrl).href)
  }
  const [x, y, w, h] = (clone.getAttribute('viewBox') ?? '0 0 0 0').split(/\s+/).map(Number)
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  clone.setAttribute('xmlns:xlink', 'http://www.w3.org/1999/xlink')
  clone.setAttribute('width', String(w))
  clone.setAttribute('height', String(h))
  clone.removeAttribute('class')
  const ns = 'http://www.w3.org/2000/svg'
  const bg = document.createElementNS(ns, 'rect')
  for (const [k, v] of [['x', x], ['y', y], ['width', w], ['height', h]] as const) bg.setAttribute(k, String(v))
  bg.setAttribute('fill', opts.background)
  const title = document.createElementNS(ns, 'title')
  title.textContent = opts.title
  clone.insertBefore(bg, clone.firstChild)
  clone.insertBefore(title, clone.firstChild)
  return '<?xml version="1.0" encoding="UTF-8"?>\n' + new XMLSerializer().serializeToString(clone)
}

/** Natural size of a standalone SVG (its width and height attributes). */
export function svgSize(svgText: string): { width: number; height: number } {
  const m = (attr: string) => Number(new RegExp(`<svg[^>]*\\s${attr}="([\\d.]+)"`).exec(svgText)?.[1] ?? 0)
  return { width: m('width'), height: m('height') }
}

/** Renders a standalone SVG to a PNG at `scale` × its natural size. */
export async function svgToPng(svgText: string, scale = 2): Promise<Blob> {
  const { width, height } = svgSize(svgText)
  const url = URL.createObjectURL(new Blob([svgText], { type: 'image/svg+xml;charset=utf-8' }))
  try {
    const img = new Image()
    await new Promise<void>((resolve, reject) => {
      img.onload = () => resolve()
      img.onerror = () => reject(new Error('the graph could not be drawn as an image'))
      img.src = url
    })
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(width * scale))
    canvas.height = Math.max(1, Math.round(height * scale))
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('no canvas')
    ctx.scale(scale, scale)
    ctx.drawImage(img, 0, 0, width, height)
    return await new Promise<Blob>((resolve, reject) => canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('no image'))), 'image/png'))
  } finally {
    URL.revokeObjectURL(url)
  }
}

/** Prints a standalone SVG on its own page (landscape when wide), through a hidden frame. */
export function printSvg(svgText: string, title: string): void {
  const { width, height } = svgSize(svgText)
  const frame = document.createElement('iframe')
  frame.setAttribute('aria-hidden', 'true')
  frame.tabIndex = -1
  frame.style.cssText = 'position:fixed;right:0;bottom:0;width:0;height:0;border:0'
  document.body.appendChild(frame)
  const doc = frame.contentDocument
  const win = frame.contentWindow
  if (!doc || !win) {
    frame.remove()
    return
  }
  const body = svgText.replace(/^<\?xml[^>]*>\s*/, '')
  doc.open()
  doc.write(
    `<!doctype html><html><head><meta charset="utf-8"><title>${escapeHtml(title)}</title><style>` +
      `@page{size:${width > height ? 'landscape' : 'portrait'};margin:12mm}html,body{margin:0}` +
      `h1{font:600 14px system-ui,sans-serif;margin:0 0 8px}svg{width:100%;height:auto;max-height:calc(100vh - 40px)}` +
      `</style></head><body><h1>${escapeHtml(title)}</h1>${body}</body></html>`,
  )
  doc.close()
  const cleanup = () => setTimeout(() => frame.remove(), 1000)
  win.addEventListener('afterprint', cleanup, { once: true })
  setTimeout(() => {
    win.focus()
    win.print()
    cleanup()
  }, 50)
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)
}

export interface MdNode {
  id: string
  key: string
  title: string
  state: string
}
export interface MdEdge {
  source: string
  target: string
  label: string
  /** draw upstream → downstream (an arrow) or as an undirected link */
  from: string
  to: string
  directed: boolean
}

/** Markdown of the graph: a Mermaid flowchart (rendered on GitHub and in many editors) and a table of the items. */
export function graphMarkdown(opts: {
  title: string
  meta: string[]
  direction: Direction
  nodes: MdNode[]
  edges: MdEdge[]
  headers: { key: string; title: string; state: string; relations: string }
  relationsOf: (id: string) => string
}): string {
  const ids = new Map(opts.nodes.map((n, i) => [n.id, `n${i + 1}`]))
  const label = (s: string) => s.replace(/["\\]/g, "'").replace(/[\r\n]+/g, ' ')
  const lines = [`# ${opts.title}`, '', ...opts.meta.map((m) => `- ${m}`), '', '```mermaid', `flowchart ${opts.direction === 'vertical' ? 'TB' : 'LR'}`]
  for (const n of opts.nodes) lines.push(`  ${ids.get(n.id)}["${label(n.key)}<br/>${label(n.title)}"]`)
  for (const e of opts.edges) {
    const a = ids.get(e.from)
    const b = ids.get(e.to)
    if (a && b) lines.push(`  ${a} ${e.directed ? '-->' : '---'}|${label(e.label)}| ${b}`)
  }
  lines.push('```', '')
  const cell = (s: string) => s.replace(/\|/g, '\\|').replace(/[\r\n]+/g, '<br>')
  const h = opts.headers
  lines.push(`| ${h.key} | ${h.title} | ${h.state} | ${h.relations} |`, '|---|---|---|---|')
  for (const n of opts.nodes) lines.push(`| ${cell(n.key)} | ${cell(n.title)} | ${cell(n.state)} | ${cell(opts.relationsOf(n.id))} |`)
  return lines.join('\n') + '\n'
}
