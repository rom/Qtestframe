// Split text into segments for keyword / issue highlighting.

export interface Range {
  offset: number
  length: number
  /** Arbitrary classification carried to the segment (e.g. obligation level). */
  kind?: string
  title?: string
}

export interface Segment {
  text: string
  kind?: string
  title?: string
  mark: boolean
}

/**
 * Convert a UTF-8 byte offset (as produced by Go) into a JS string index.
 * Offsets beyond the end clamp to text.length.
 */
export function byteToIndex(text: string, byteOffset: number): number {
  if (byteOffset <= 0) return 0
  let bytes = 0
  let i = 0
  while (i < text.length) {
    const cp = text.codePointAt(i) as number
    const len = cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4
    if (bytes + len > byteOffset) return i
    bytes += len
    i += cp > 0xffff ? 2 : 1
    if (bytes === byteOffset) return i
  }
  return text.length
}

/** Whether the string contains non-ASCII characters. */
export function hasNonAscii(text: string): boolean {
  // eslint-disable-next-line no-control-regex
  return /[^\u0000-\u007f]/.test(text)
}

/**
 * Split `text` into plain and marked segments. Ranges may be unsorted and
 * overlapping; overlaps are resolved in favour of the earlier range. When
 * `bytes` is true the offsets are interpreted as UTF-8 byte offsets.
 */
export function segments(text: string, ranges: Range[] | null | undefined, bytes = true): Segment[] {
  if (!text) return []
  const conv = bytes && hasNonAscii(text)
  const rs = (ranges ?? [])
    .filter((r) => r && r.length > 0 && r.offset >= 0)
    .map((r) => {
      const start = conv ? byteToIndex(text, r.offset) : Math.min(r.offset, text.length)
      const end = conv ? byteToIndex(text, r.offset + r.length) : Math.min(r.offset + r.length, text.length)
      return { start, end, kind: r.kind, title: r.title }
    })
    .filter((r) => r.end > r.start)
    .sort((a, b) => a.start - b.start || b.end - a.end)

  const out: Segment[] = []
  let pos = 0
  for (const r of rs) {
    if (r.start < pos) {
      if (r.end <= pos) continue
      r.start = pos
    }
    if (r.start > pos) out.push({ text: text.slice(pos, r.start), mark: false })
    out.push({ text: text.slice(r.start, r.end), mark: true, kind: r.kind, title: r.title })
    pos = r.end
  }
  if (pos < text.length) out.push({ text: text.slice(pos), mark: false })
  return out
}

/** Index in `text` (JS) for a byte offset when text has non-ASCII, else identity. */
export function positionToIndex(text: string, position: number): number {
  return hasNonAscii(text) ? byteToIndex(text, position) : Math.max(0, Math.min(position, text.length))
}

/**
 * Plain words of a query expression usable for emphasising search snippets:
 * strips field prefixes, operators, quotes, globs, keywords and regexes.
 */
export function queryTerms(query: string | null | undefined): string[] {
  if (!query) return []
  const out = new Set<string>()
  for (const raw of query.replace(/[()"]/g, ' ').split(/\s+/)) {
    if (!raw || raw.includes('/')) continue
    const w = raw
      .replace(/^-/, '')
      .replace(/^[a-z_]+(?:!=|!~|>=|<=|[:=~<>])/i, '')
      .replace(/[*?]/g, '')
    if (w.length > 1 && !/^(and|or|not)$/i.test(w)) out.add(w)
  }
  return [...out]
}

/** Split text into parts marking case-insensitive occurrences of terms. */
export function splitByTerms(text: string, terms: readonly string[]): { text: string; mark: boolean }[] {
  if (!text) return []
  const ts = terms.filter(Boolean).map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
  if (!ts.length) return [{ text, mark: false }]
  const re = new RegExp(`(${ts.join('|')})`, 'gi')
  return text
    .split(re)
    .map((t, i) => ({ text: t, mark: i % 2 === 1 }))
    .filter((p) => p.text !== '')
}
