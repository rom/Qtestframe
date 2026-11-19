// Client-side full-text search over the guide pages. Small enough to scan
// every section on each query: terms are matched as prefixes of words,
// case- and accent-insensitively; all terms must occur in a section.
import type { Section } from './render'

export interface SearchHit {
  page: string
  title: string
  heading: string
  anchor: string
  score: number
  snippet: string
}

/** Lower case without diacritics, plus the index in `s` of every output character. */
export function fold(s: string): { text: string; map: number[] } {
  let text = ''
  const map: number[] = []
  let i = 0
  for (const ch of s) {
    const f = ch.normalize('NFKD').replace(/\p{M}/gu, '').toLowerCase()
    for (let k = 0; k < f.length; k++) map.push(i)
    text += f
    i += ch.length
  }
  map.push(i)
  return { text, map }
}

/** Search terms of a query: folded words of at least two characters (or a digit), without duplicates. */
export function terms(query: string): string[] {
  const words = fold(query ?? '').text.split(/[^\p{L}\p{N}_]+/u)
  return [...new Set(words.filter((w) => w.length >= 2 || /\d/.test(w)))]
}

const WORD = /[\p{L}\p{N}_]/u

/** Positions where `term` starts a word in the folded `text`. */
export function wordStarts(text: string, term: string): number[] {
  const out: number[] = []
  if (!term) return out
  let from = 0
  for (;;) {
    const i = text.indexOf(term, from)
    if (i < 0) return out
    if (i === 0 || !WORD.test(text[i - 1])) out.push(i)
    from = i + 1
  }
}

/** Score of a section for the terms (0 = not all terms present). */
export function scoreSection(sec: Section, ts: readonly string[], phrase = ''): number {
  if (!ts.length) return 0
  const title = fold(sec.title).text
  const heading = fold(sec.heading).text
  const body = fold(sec.text).text
  let score = 0
  for (const t of ts) {
    const inTitle = wordStarts(title, t).length
    const inHeading = wordStarts(heading, t).length
    const inBody = wordStarts(body, t).length
    if (!inTitle && !inHeading && !inBody) return 0
    score += (inTitle ? 6 : 0) + (inHeading ? 10 : 0) + Math.min(inBody, 5)
  }
  if (phrase && ts.length > 1) {
    if (heading.includes(phrase)) score += 12
    else if (body.includes(phrase)) score += 4
  }
  // a hit on the page start (title only) ranks below a matching section heading
  if (!sec.heading) score -= 1
  return score
}

/** A ~160-character excerpt of `text` around the first occurrence of a term. */
export function snippet(text: string, ts: readonly string[], width = 160): string {
  const { text: folded, map } = fold(text)
  let at = -1
  for (const t of ts) {
    const pos = wordStarts(folded, t)[0]
    if (pos !== undefined && (at < 0 || pos < at)) at = pos
  }
  if (text.length <= width) return text
  if (at < 0) return text.slice(0, width).trimEnd() + ' …'
  const center = map[at]
  let start = Math.max(0, center - Math.floor(width / 3))
  let end = Math.min(text.length, start + width)
  start = Math.max(0, end - width)
  // do not cut words
  if (start > 0) {
    const sp = text.indexOf(' ', start)
    if (sp >= 0 && sp < center) start = sp + 1
  }
  if (end < text.length) {
    const sp = text.lastIndexOf(' ', end)
    if (sp > center) end = sp
  }
  return (start > 0 ? '… ' : '') + text.slice(start, end).trim() + (end < text.length ? ' …' : '')
}

/**
 * Search the sections; best hits first (ties: page order, then position on
 * the page). At most `limit` hits, and at most `perPage` per page.
 */
export function searchGuide(sections: readonly Section[], query: string, opts: { limit?: number; perPage?: number; order?: readonly string[] } = {}): SearchHit[] {
  const ts = terms(query)
  if (!ts.length) return []
  const phrase = ts.join(' ')
  const order = opts.order ?? []
  const rank = (p: string) => {
    const i = order.indexOf(p)
    return i < 0 ? order.length : i
  }
  const scored = sections
    .map((sec, i) => ({ sec, i, score: scoreSection(sec, ts, phrase) }))
    .filter((x) => x.score > 0)
    .sort((a, b) => b.score - a.score || rank(a.sec.page) - rank(b.sec.page) || a.i - b.i)
  const perPage = opts.perPage ?? 3
  const count = new Map<string, number>()
  const out: SearchHit[] = []
  for (const { sec, score } of scored) {
    const n = count.get(sec.page) ?? 0
    if (n >= perPage) continue
    count.set(sec.page, n + 1)
    out.push({ page: sec.page, title: sec.title, heading: sec.heading, anchor: sec.anchor, score, snippet: snippet(sec.text, ts) })
    if (out.length >= (opts.limit ?? 20)) break
  }
  return out
}
