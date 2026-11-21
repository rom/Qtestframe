import { afterEach, describe, expect, it, vi } from 'vitest'
import { graphMarkdown, printSvg, standaloneSvg, svgSize, svgToPng } from './graphExport'

function svg(): SVGSVGElement {
  const div = document.createElement('div')
  div.innerHTML = `<svg viewBox="-24 -24 300 200" width="150" height="100" class="x"><g><a href="/p/1/requirements?item=a"><rect x="0" y="0" width="10" height="10" style="fill: red"></rect><text>K-1 &amp; "x"</text></a></g></svg>`
  document.body.appendChild(div)
  return div.querySelector('svg')!
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

describe('standaloneSvg', () => {
  it('inlines styles, removes the zoom, adds title and background, makes links absolute', () => {
    const text = standaloneSvg(svg(), { title: 'DEMO graph', background: 'rgb(1, 2, 3)', baseUrl: 'https://qtf.example' })
    expect(text.startsWith('<?xml')).toBe(true)
    expect(text).toContain('width="300"')
    expect(text).toContain('height="200"')
    expect(text).not.toContain('class="x"')
    expect(text).toContain('<title>DEMO graph</title>')
    expect(text).toContain('fill="rgb(1, 2, 3)"')
    expect(text).toContain('href="https://qtf.example/p/1/requirements?item=a"')
    expect(text).toMatch(/style="fill: red;?/)
    expect(svgSize(text)).toEqual({ width: 300, height: 200 })
    expect(svgSize('<svg>')).toEqual({ width: 0, height: 0 })
  })
  it('copies computed styles but skips unresolved variables and defaults', () => {
    const s = svg()
    vi.spyOn(window, 'getComputedStyle').mockImplementation(
      () => ({ getPropertyValue: (p: string) => (p === 'stroke' ? 'blue' : p === 'fill' ? 'var(--x)' : p === 'font-weight' ? 'normal' : '') }) as unknown as CSSStyleDeclaration,
    )
    const text = standaloneSvg(s, { title: 't', background: '#fff', baseUrl: 'https://a.b' })
    expect(text).toContain('stroke:blue')
    expect(text).not.toContain('var(--x)')
  })
})

describe('svgToPng', () => {
  it('draws the SVG on a canvas at the scale', async () => {
    URL.createObjectURL = vi.fn(() => 'blob:x') as unknown as typeof URL.createObjectURL
    URL.revokeObjectURL = vi.fn()
    const draw = vi.fn()
    const scale = vi.fn()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage: draw, scale } as unknown as CanvasRenderingContext2D)
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(function (this: HTMLCanvasElement, cb: BlobCallback) {
      cb(new Blob([`${this.width}x${this.height}`], { type: 'image/png' }))
    })
    const orig = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'src')
    Object.defineProperty(HTMLImageElement.prototype, 'src', {
      configurable: true,
      set(this: HTMLImageElement) {
        setTimeout(() => this.onload?.(new Event('load')))
      },
    })
    try {
      const png = await svgToPng('<svg width="30" height="20"></svg>', 2)
      expect(png.type).toBe('image/png')
      expect(png.size).toBe('60x40'.length)
      expect(scale).toHaveBeenCalledWith(2, 2)
      expect(draw).toHaveBeenCalled()
      expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:x')
      // an image that cannot be drawn
      Object.defineProperty(HTMLImageElement.prototype, 'src', {
        configurable: true,
        set(this: HTMLImageElement) {
          setTimeout(() => this.onerror?.(new Event('error')))
        },
      })
      await expect(svgToPng('<svg width="1" height="1"></svg>')).rejects.toThrow(/image/)
      // no canvas context / no blob
      Object.defineProperty(HTMLImageElement.prototype, 'src', {
        configurable: true,
        set(this: HTMLImageElement) {
          setTimeout(() => this.onload?.(new Event('load')))
        },
      })
      vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
      await expect(svgToPng('<svg width="1" height="1"></svg>')).rejects.toThrow(/canvas/)
      vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage: draw, scale } as unknown as CanvasRenderingContext2D)
      vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((cb: BlobCallback) => cb(null))
      await expect(svgToPng('<svg width="1" height="1"></svg>')).rejects.toThrow(/no image/)
    } finally {
      if (orig) Object.defineProperty(HTMLImageElement.prototype, 'src', orig)
    }
  })
})

describe('printSvg', () => {
  it('prints in a hidden frame, landscape when wide, and removes the frame', () => {
    vi.useFakeTimers()
    const print = vi.fn()
    const realCreate = document.createElement.bind(document)
    let frame: HTMLIFrameElement | undefined
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = realCreate(tag)
      if (tag === 'iframe') frame = el as HTMLIFrameElement
      return el
    })
    printSvg('<?xml version="1.0"?>\n<svg width="300" height="100"><text>x</text></svg>', 'A <b> graph')
    expect(frame).toBeDefined()
    const doc = frame!.contentDocument!
    expect(doc.title).toBe('A <b> graph')
    expect(doc.head.innerHTML).toContain('landscape')
    expect(doc.body.innerHTML).not.toContain('<?xml')
    Object.defineProperty(frame!.contentWindow!, 'print', { value: print })
    Object.defineProperty(frame!.contentWindow!, 'focus', { value: vi.fn() })
    vi.advanceTimersByTime(60)
    expect(print).toHaveBeenCalled()
    vi.advanceTimersByTime(1100)
    expect(document.querySelector('iframe')).toBeNull()
    printSvg('<svg width="100" height="300"></svg>', 'tall')
    expect(frame!.contentDocument!.head.innerHTML).toContain('portrait')
    vi.useRealTimers()
  })
  it('gives up without a frame document', () => {
    const realCreate = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = realCreate(tag)
      if (tag === 'iframe') Object.defineProperty(el, 'contentDocument', { value: null })
      return el
    })
    printSvg('<svg width="1" height="1"></svg>', 't')
    expect(document.querySelector('iframe')).toBeNull()
  })
})

describe('graphMarkdown', () => {
  it('writes a Mermaid flowchart in the chosen direction and an escaped table', () => {
    const md = graphMarkdown({
      title: 'DEMO graph',
      meta: ['Project: Demo'],
      direction: 'vertical',
      nodes: [
        { id: 'a', key: 'R-1', title: 'Login "fast"', state: 'Passed' },
        { id: 'b', key: 'R-2', title: 'A | B\nC', state: 'None' },
      ],
      edges: [
        { source: 'b', target: 'a', label: 'depends on', from: 'a', to: 'b', directed: true },
        { source: 'a', target: 'b', label: 'relates to', from: 'a', to: 'b', directed: false },
        { source: 'a', target: 'x', label: 'gone', from: 'a', to: 'x', directed: true },
      ],
      headers: { key: 'Key', title: 'Title', state: 'Status', relations: 'Relations' },
      relationsOf: (id) => (id === 'a' ? 'provides R-2' : ''),
    })
    expect(md).toContain('# DEMO graph\n\n- Project: Demo')
    expect(md).toContain('```mermaid\nflowchart TB')
    expect(md).toContain(`n1["R-1<br/>Login 'fast'"]`)
    expect(md).toContain('n1 -->|depends on| n2')
    expect(md).toContain('n1 ---|relates to| n2')
    expect(md).not.toContain('gone')
    expect(md).toContain('| R-2 | A \\| B<br>C | None |  |')
    expect(graphMarkdown({ title: 't', meta: [], direction: 'horizontal', nodes: [], edges: [], headers: { key: 'k', title: 't', state: 's', relations: 'r' }, relationsOf: () => '' })).toContain('flowchart LR')
  })
})
