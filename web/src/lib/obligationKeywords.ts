// Obligation keywords (RFC 2119 / RFC 8174 MUST, SHALL NOT …, Swedish SKA,
// BÖR INTE, FÅR …) in requirement texts, found in the browser exactly as
// the server's language analysis (internal/reqlang) finds them. The keyword
// tables come from obligationKeywords.json, which Go writes from the
// language packs and a Go test keeps in sync; this module only holds the
// matching rules, and obligationKeywords.test.ts checks them against
// results of the Go analysis (obligationKeywords.cases.json).
import table from './obligationKeywords.json'

export interface KeywordDef {
  text: string
  level: string
  case_sensitive?: boolean
}
export interface PackDef {
  code: string
  uppercase_rule: boolean
  keywords: KeywordDef[]
  markers: string[]
  letters?: string
  /** keywords to suggest when a text has none */
  examples: string
}
export interface KeywordTable {
  levels: string[]
  packs: PackDef[]
}

/** A keyword found in a text; start/end are JS string indices. */
export interface ObligationMatch {
  start: number
  end: number
  /** the text as written */
  text: string
  /** canonical keyword, e.g. "MUST NOT" */
  keyword: string
  /** obligation level (must, shall_not, …) */
  level: string
  /** not uppercase where only uppercase keywords are normative (RFC 8174) */
  lowercase: boolean
}

interface CompiledPack {
  code: string
  uppercaseRule: boolean
  keywords: { def: KeywordDef; re: RegExp }[]
  markers: Set<string>
  letters: string
  examples: string
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
// Go's regexp \s: ASCII whitespace without \v
const SPACE = '[\\t\\n\\f\\r ]+'

function compilePack(p: PackDef): CompiledPack {
  return {
    code: p.code,
    uppercaseRule: p.uppercase_rule,
    keywords: p.keywords.map((def) => ({ def, re: new RegExp(def.text.split(/\s+/).filter(Boolean).map(escape).join(SPACE), def.case_sensitive ? 'gu' : 'giu') })),
    markers: new Set(p.markers),
    letters: p.letters ?? '',
    examples: p.examples,
  }
}

const TABLE = table as KeywordTable
const PACKS = TABLE.packs.map(compilePack).sort((a, b) => (a.code < b.code ? -1 : a.code > b.code ? 1 : 0))
const BY_CODE = new Map(PACKS.map((p) => [p.code, p]))

/** Obligation levels from strongest to weakest. */
export const LEVELS: readonly string[] = TABLE.levels
/** Language codes with a keyword table. */
export const LANGUAGES: readonly string[] = PACKS.map((p) => p.code)

const WORD = /[\p{L}\p{Nd}_]/u
const isWord = (ch: string | undefined) => !!ch && WORD.test(ch)
/** previous / next code point around an index */
function before(text: string, i: number): string | undefined {
  if (i <= 0) return undefined
  const c = text.charCodeAt(i - 1)
  return c >= 0xdc00 && c <= 0xdfff && i >= 2 ? text.slice(i - 2, i) : text[i - 1]
}
function after(text: string, i: number): string | undefined {
  if (i >= text.length) return undefined
  return String.fromCodePoint(text.codePointAt(i) as number)
}
function bounded(text: string, s: number, e: number): boolean {
  const first = after(text, s)
  if (isWord(first) && isWord(before(text, s))) return false
  const last = before(text, e)
  if (isWord(last) && isWord(after(text, e))) return false
  return true
}

const isUpper = (s: string) => s === s.toUpperCase() && s !== s.toLowerCase()

function score(pk: CompiledPack, words: string[], lower: string): number {
  let n = 0
  for (const w of words) if (pk.markers.has(w)) n++
  if (pk.letters && [...pk.letters].some((ch) => lower.includes(ch))) n += 2
  return n
}

/**
 * The language of a text, guessed from frequent function words like the
 * server does (English wins ties and empty input).
 */
export function detectLanguage(text: string): string {
  const lower = (text ?? '').toLowerCase()
  const words = lower.split(/[^\p{L}']+/u).filter(Boolean)
  let best = 'en'
  let bestScore = 0
  for (const pk of PACKS) {
    const s = score(pk, words, lower)
    if (s > bestScore || (s === bestScore && pk.code === 'en')) {
      best = pk.code
      bestScore = s
    }
  }
  return best
}

/**
 * The obligation keywords of a text, in order, with the language pack
 * `lang` ("", "auto" or unknown: detected), as the server's analysis finds
 * them: longest phrase first, case-insensitive (RFC keywords inside another
 * language only in uppercase), on word boundaries, never overlapping.
 */
export function findObligations(text: string, lang = ''): ObligationMatch[] {
  if (!text || !text.trim()) return []
  const pk = BY_CODE.get(lang) ?? BY_CODE.get(detectLanguage(text)) ?? PACKS[0]
  if (!pk) return []
  const out: ObligationMatch[] = []
  const overlaps = (s: number, e: number) => out.some((m) => s < m.end && m.start < e)
  for (const { def, re } of pk.keywords) {
    for (const m of text.matchAll(re)) {
      const s = m.index ?? 0
      const e = s + m[0].length
      if (bounded(text, s, e) && !overlaps(s, e)) {
        out.push({ start: s, end: e, text: m[0], keyword: def.text, level: def.level, lowercase: pk.uppercaseRule && !def.case_sensitive && !isUpper(m[0]) })
      }
    }
  }
  return out.sort((a, b) => a.start - b.start)
}

/** Keywords to suggest for a text without one, in its language pack. */
export function keywordExamples(text: string, lang = ''): string {
  const pk = BY_CODE.get(lang) ?? BY_CODE.get(detectLanguage(text)) ?? PACKS[0]
  return pk?.examples ?? ''
}

const rank = (level: string) => {
  const i = LEVELS.indexOf(level)
  return i < 0 ? LEVELS.length : i
}

/** The strongest obligation level of the matches ("" if none), like the analysis' "obligation". */
export function strongestLevel(matches: readonly { level: string }[]): string {
  let best = ''
  for (const m of matches) if (!best || rank(m.level) < rank(best)) best = m.level
  return best
}

// ------------------------------------------------------------------ display

/** The five highlight styles. */
export type DisplayLevel = 'must' | 'must-not' | 'should' | 'should-not' | 'may'
export const DISPLAY_LEVELS: readonly DisplayLevel[] = ['must', 'must-not', 'should', 'should-not', 'may']

/**
 * The highlight style of an obligation level; null for levels that are not
 * an obligation (will, describe).
 */
export function displayLevel(level: string): DisplayLevel | null {
  switch (level) {
    case 'must':
    case 'shall':
      return 'must'
    case 'must_not':
    case 'shall_not':
      return 'must-not'
    case 'should':
      return 'should'
    case 'should_not':
      return 'should-not'
    case 'may':
    case 'optional':
      return 'may'
    default:
      return null
  }
}

/**
 * Code spans of a text (Markdown style): fenced blocks (``` or ~~~ lines)
 * and inline spans between equally long backtick runs. Keywords in code
 * are not highlighted.
 */
export function codeRanges(text: string): [number, number][] {
  const out: [number, number][] = []
  // fenced blocks
  const fence = /^[ ]{0,3}(`{3,}|~{3,})[^\n]*\n[\s\S]*?(?:\n[ ]{0,3}\1[`~]*[ \t]*(?=\n|$)|$)/gm
  for (const m of text.matchAll(fence)) out.push([m.index ?? 0, (m.index ?? 0) + m[0].length])
  const inFence = (i: number) => out.some(([s, e]) => i >= s && i < e)
  // inline spans
  const ticks = /`+/g
  let m: RegExpExecArray | null
  while ((m = ticks.exec(text))) {
    if (inFence(m.index)) continue
    const run = m[0]
    let j = m.index + run.length
    let close = -1
    while ((j = text.indexOf(run, j)) >= 0) {
      const tickBefore = text[j - 1] === '`'
      const tickAfter = text[j + run.length] === '`'
      if (!tickBefore && !tickAfter) {
        close = j
        break
      }
      while (text[j] === '`') j++
    }
    if (close < 0) continue
    out.push([m.index, close + run.length])
    ticks.lastIndex = close + run.length
  }
  return out.sort((a, b) => a[0] - b[0])
}

export interface HighlightOptions {
  /** language pack; "" / "auto" = detect */
  lang?: string
  /** also mark keywords not written in capitals (default false) */
  lowercase?: boolean
  /** further ranges (start, end) where nothing is marked, e.g. links */
  exclude?: readonly (readonly [number, number])[]
}

/**
 * The keywords to highlight in a read-only text: obligations only (not
 * WILL or NOTE:), outside code spans and excluded ranges, and only when
 * written in capitals, as the obligation help asks for (RFC 8174: "must"
 * is ordinary English, "får" ordinary Swedish).
 */
export function highlightRanges(text: string, opts: HighlightOptions = {}): (ObligationMatch & { display: DisplayLevel })[] {
  if (!text) return []
  const skip = [...codeRanges(text), ...(opts.exclude ?? [])]
  const out: (ObligationMatch & { display: DisplayLevel })[] = []
  for (const m of findObligations(text, opts.lang === 'auto' ? '' : (opts.lang ?? ''))) {
    const display = displayLevel(m.level)
    if (!display || (!opts.lowercase && !isUpper(m.text))) continue
    if (skip.some(([s, e]) => m.start < e && s < m.end)) continue
    out.push({ ...m, display })
  }
  return out
}

/** A piece of text, marked when it is an obligation keyword. */
export interface ObligationPiece {
  text: string
  /** set for keywords */
  keyword?: string
  level?: string
  display?: DisplayLevel
}

/**
 * Split text[from, from + piece.length) into plain and keyword pieces for
 * the given ranges (found in the whole text); ranges not wholly inside the
 * piece are ignored. The pieces' texts concatenate to `piece`.
 */
export function splitPiece(piece: string, from: number, ranges: readonly (ObligationMatch & { display: DisplayLevel })[]): ObligationPiece[] {
  const out: ObligationPiece[] = []
  const end = from + piece.length
  let pos = from
  for (const r of ranges) {
    if (r.start < pos || r.end > end) continue
    if (r.start > pos) out.push({ text: piece.slice(pos - from, r.start - from) })
    out.push({ text: piece.slice(r.start - from, r.end - from), keyword: r.keyword, level: r.level, display: r.display })
    pos = r.end
  }
  if (pos < end) out.push({ text: piece.slice(pos - from) })
  return out
}

/** The text split into plain and keyword pieces for display. */
export function obligationPieces(text: string, opts: HighlightOptions = {}): ObligationPiece[] {
  if (!text) return []
  return splitPiece(text, 0, highlightRanges(text, opts))
}

// ------------------------------------------------------------------ editing

export interface KeywordCheck {
  /** every obligation keyword (WILL / NOTE: included), in order */
  matches: ObligationMatch[]
  /** strongest level found ("" = none) */
  detected: string
  /** distinct canonical obligation keywords (not "describe") */
  distinct: string[]
  /** the detected level differs from the chosen obligation */
  mismatch: boolean
  /** more than one distinct obligation keyword */
  multiple: boolean
  /** keywords written in lowercase where only uppercase is normative */
  lowercase: ObligationMatch[]
}

/**
 * What the editor tells the author about the keywords of a description,
 * given the obligation chosen in the form ("" = detect automatically).
 */
export function checkKeywords(text: string, chosen: string | null | undefined, lang = ''): KeywordCheck {
  const matches = findObligations(text, lang === 'auto' ? '' : lang)
  const detected = strongestLevel(matches)
  const distinct = [...new Set(matches.filter((m) => m.level !== 'describe').map((m) => m.keyword))]
  return {
    matches,
    detected,
    distinct,
    mismatch: !!chosen && !!detected && detected !== chosen,
    multiple: distinct.length > 1,
    lowercase: matches.filter((m) => m.lowercase),
  }
}
