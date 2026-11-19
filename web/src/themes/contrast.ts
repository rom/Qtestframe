// WCAG 2.1 contrast maths (pure, no DOM).

export interface RGB {
  r: number
  g: number
  b: number
  a: number
}

/** Parse #rgb, #rgba, #rrggbb, #rrggbbaa, rgb(...) and rgba(...). Throws on anything else. */
export function parseColor(input: string): RGB {
  const s = input.trim().toLowerCase()
  let m = /^#([0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/.exec(s)
  if (m) {
    let h = m[1]
    if (h.length <= 4) h = [...h].map((c) => c + c).join('')
    const n = (i: number) => parseInt(h.slice(i, i + 2), 16)
    return { r: n(0), g: n(2), b: n(4), a: h.length === 8 ? n(6) / 255 : 1 }
  }
  m = /^rgba?\(\s*([\d.]+)[\s,]+([\d.]+)[\s,]+([\d.]+)(?:[\s,/]+([\d.]+%?))?\s*\)$/.exec(s)
  if (m) {
    const a = m[4] === undefined ? 1 : m[4].endsWith('%') ? parseFloat(m[4]) / 100 : parseFloat(m[4])
    return { r: +m[1], g: +m[2], b: +m[3], a }
  }
  throw new Error(`unsupported colour: ${input}`)
}

/** Composite a (possibly translucent) colour over an opaque background. */
export function blend(fg: string | RGB, bg: string | RGB): RGB {
  const f = typeof fg === 'string' ? parseColor(fg) : fg
  const b = typeof bg === 'string' ? parseColor(bg) : bg
  const mix = (x: number, y: number) => Math.round(x * f.a + y * (1 - f.a))
  return { r: mix(f.r, b.r), g: mix(f.g, b.g), b: mix(f.b, b.b), a: 1 }
}

function channel(v: number): number {
  const c = v / 255
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

/** Relative luminance (0 = black, 1 = white). */
export function luminance(color: string | RGB): number {
  const c = typeof color === 'string' ? parseColor(color) : color
  return 0.2126 * channel(c.r) + 0.7152 * channel(c.g) + 0.0722 * channel(c.b)
}

/**
 * Contrast ratio between two colours (1–21). A translucent foreground is
 * composited over the background first; the background must be opaque.
 */
export function contrastRatio(fg: string | RGB, bg: string | RGB): number {
  const b = typeof bg === 'string' ? parseColor(bg) : bg
  const f = blend(fg, b)
  const l1 = luminance(f)
  const l2 = luminance(b)
  const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1]
  return (hi + 0.05) / (lo + 0.05)
}

export type ContrastKind = 'text' | 'large' | 'ui'

/** WCAG 2.1 AA minimum: 4.5 for normal text, 3 for large text and UI components. */
export function aaMinimum(kind: ContrastKind): number {
  return kind === 'text' ? 4.5 : 3
}

export function meetsAA(fg: string, bg: string, kind: ContrastKind = 'text'): boolean {
  return contrastRatio(fg, bg) >= aaMinimum(kind)
}
