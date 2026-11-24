// Gherkin authoring in the browser: syntax highlighting, keyword and step
// completion from the project's step library, table alignment and the
// types of the BDD endpoints (see internal/gherkin and docs/api.md
// "Gherkin tests"). Parsing and validation are done by the server
// (POST /projects/{pid}/gherkin/check) so there is one parser.

export type GherkinLang = 'en' | 'sv'

export interface Dialect {
  feature: string[]
  background: string[]
  rule: string[]
  scenario: string[]
  outline: string[]
  examples: string[]
  given: string[]
  when: string[]
  then: string[]
  and: string[]
  but: string[]
}

/** the keywords the server understands (internal/gherkin/keywords.go) */
export const DIALECTS: Record<GherkinLang, Dialect> = {
  en: {
    feature: ['Feature', 'Business Need', 'Ability'],
    background: ['Background'],
    rule: ['Rule'],
    scenario: ['Scenario', 'Example'],
    outline: ['Scenario Outline', 'Scenario Template'],
    examples: ['Examples', 'Scenarios'],
    given: ['Given'],
    when: ['When'],
    then: ['Then'],
    and: ['And'],
    but: ['But'],
  },
  sv: {
    feature: ['Egenskap'],
    background: ['Bakgrund'],
    rule: ['Regel'],
    scenario: ['Scenario'],
    outline: ['Abstrakt Scenario', 'Abstrakt scenario', 'Scenariomall'],
    examples: ['Exempel'],
    given: ['Givet'],
    when: ['När'],
    then: ['Så'],
    and: ['Och'],
    but: ['Men'],
  },
}

const byLength = (a: string, b: string) => b.length - a.length
const all = (pick: (d: Dialect) => string[]) => [...new Set([...pick(DIALECTS.en), ...pick(DIALECTS.sv)])].sort(byLength)

/** header keywords (followed by a colon), longest first */
export const HEADER_KEYWORDS = all((d) => [...d.feature, ...d.background, ...d.rule, ...d.outline, ...d.scenario, ...d.examples])
/** step keywords (followed by a space), longest first */
export const STEP_KEYWORDS = all((d) => [...d.given, ...d.when, ...d.then, ...d.and, ...d.but])

export type StepKind = 'given' | 'when' | 'then' | 'and'

/** the kind of a step keyword ("" when it is not one) */
export function stepKind(word: string): StepKind | '' {
  for (const d of Object.values(DIALECTS)) {
    if (d.given.includes(word)) return 'given'
    if (d.when.includes(word)) return 'when'
    if (d.then.includes(word)) return 'then'
    if (d.and.includes(word) || d.but.includes(word)) return 'and'
  }
  return word === '*' ? 'and' : ''
}

/** the language of a document: its "# language:" header or its Feature keyword */
export function languageOf(src: string): GherkinLang {
  for (const raw of src.split('\n')) {
    const line = raw.trim()
    const m = /^#\s*language\s*:\s*([A-Za-z_-]+)/.exec(line)
    if (m) return m[1].toLowerCase() === 'sv' ? 'sv' : 'en'
    if (line.startsWith('Egenskap:')) return 'sv'
    if (DIALECTS.en.feature.some((k) => line.startsWith(k + ':'))) return 'en'
  }
  return 'en'
}

// ---------------------------------------------------------------- highlighting

export type TokenKind = 'keyword' | 'step' | 'tag' | 'comment' | 'string' | 'number' | 'placeholder' | 'pipe' | 'docstring' | 'param' | 'text'

export interface Token {
  kind: TokenKind
  text: string
}

/** split the text of a step, description or cell into tokens */
function textTokens(s: string, out: Token[]) {
  const re = /("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')|(<[^<>\s][^<>]*>)|(\{[a-z]*\})|((?<![\w.])-?\d+(?:[.,]\d+)?(?![\w]))/g
  let last = 0
  for (let m = re.exec(s); m; m = re.exec(s)) {
    if (m.index > last) out.push({ kind: 'text', text: s.slice(last, m.index) })
    const kind: TokenKind = m[1] ? 'string' : m[2] ? 'placeholder' : m[3] ? 'param' : 'number'
    out.push({ kind, text: m[0] })
    last = m.index + m[0].length
  }
  if (last < s.length) out.push({ kind: 'text', text: s.slice(last) })
}

/**
 * Tokens of each line of a document (doc strings are followed across
 * lines). Joining the texts of a line's tokens gives the line back.
 */
export function tokenize(src: string): Token[][] {
  const lines = src.split('\n')
  const out: Token[][] = []
  let doc = ''
  for (const line of lines) {
    const toks: Token[] = []
    const trimmed = line.trim()
    const indent = line.slice(0, line.length - line.trimStart().length)
    if (indent) toks.push({ kind: 'text', text: indent })
    const body = line.slice(indent.length)
    if (doc) {
      if (trimmed === doc) doc = ''
      if (body) toks.push({ kind: 'docstring', text: body })
    } else if (trimmed.startsWith('"""') || trimmed.startsWith('```')) {
      doc = trimmed.slice(0, 3)
      toks.push({ kind: 'docstring', text: body })
    } else if (trimmed.startsWith('#')) {
      toks.push({ kind: 'comment', text: body })
    } else if (trimmed.startsWith('@')) {
      for (const part of body.split(/(\s+)/)) if (part) toks.push({ kind: part.trim() ? 'tag' : 'text', text: part })
    } else if (trimmed.startsWith('|')) {
      for (const part of body.split(/((?<!\\)\|)/)) {
        if (part === '|') toks.push({ kind: 'pipe', text: part })
        else if (part) textTokens(part, toks)
      }
    } else {
      const h = HEADER_KEYWORDS.find((k) => body.startsWith(k + ':'))
      const s = h ? undefined : STEP_KEYWORDS.find((k) => body === k || body.startsWith(k + ' ')) ?? (body === '*' || body.startsWith('* ') ? '*' : undefined)
      if (h) {
        toks.push({ kind: 'keyword', text: h + ':' })
        if (body.length > h.length + 1) textTokens(body.slice(h.length + 1), toks)
      } else if (s) {
        toks.push({ kind: 'step', text: s })
        textTokens(body.slice(s.length), toks)
      } else if (body) {
        textTokens(body, toks)
      }
    }
    out.push(toks)
  }
  return out
}

export function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

export interface LineMark {
  /** an error on the line */
  error?: boolean
  /** a step no definition matches */
  undefined?: boolean
  /** the latest result of the step */
  result?: string
}

/**
 * HTML of the highlighted document for the overlay behind the textarea:
 * one div per line (marked lines get gh-error, gh-undefined or
 * gh-result-<status> classes). The text is escaped.
 */
export function highlight(src: string, marks: Record<number, LineMark> = {}): string {
  return tokenize(src)
    .map((toks, i) => {
      const m = marks[i + 1]
      const cls = ['gh-line']
      if (m?.error) cls.push('gh-error')
      if (m?.undefined) cls.push('gh-undefined')
      if (m?.result) cls.push(`gh-result-${m.result.replace(/[^a-z_]/g, '')}`)
      const inner = toks.map((t) => (t.kind === 'text' ? escapeHtml(t.text) : `<span class="gh-${t.kind}">${escapeHtml(t.text)}</span>`)).join('')
      // an empty line still takes its height
      return `<div class="${cls.join(' ')}">${inner || ' '}</div>`
    })
    .join('')
}

// ---------------------------------------------------------------- tables

/** the cells of a table row (\| and \\ escapes as written) */
export function splitRow(line: string): string[] {
  const s = line.trim()
  const cells: string[] = []
  let cur = ''
  for (let i = 1; i < s.length; i++) {
    const c = s[i]
    if (c === '\\' && i + 1 < s.length) {
      cur += c + s[i + 1]
      i++
    } else if (c === '|') {
      cells.push(cur.trim())
      cur = ''
    } else cur += c
  }
  if (cur.trim()) cells.push(cur.trim())
  return cells
}

const width = (s: string) => [...s].length

/**
 * Aligns the columns of every table (runs of lines that start with |),
 * keeping the indentation of the table's first row. Nothing else changes;
 * doc strings are left alone.
 */
export function formatTables(src: string): string {
  const lines = src.split('\n')
  let doc = ''
  for (let i = 0; i < lines.length; i++) {
    const t = lines[i].trim()
    if (doc) {
      if (t === doc) doc = ''
      continue
    }
    if (t.startsWith('"""') || t.startsWith('```')) {
      doc = t.slice(0, 3)
      continue
    }
    if (!t.startsWith('|')) continue
    let j = i
    while (j < lines.length && lines[j].trim().startsWith('|')) j++
    const indent = lines[i].slice(0, lines[i].length - lines[i].trimStart().length)
    const rows = lines.slice(i, j).map(splitRow)
    const cols = Math.max(...rows.map((r) => r.length))
    const w = Array.from({ length: cols }, (_, c) => Math.max(...rows.map((r) => width(r[c] ?? ''))))
    for (let k = 0; k < rows.length; k++) {
      lines[i + k] = indent + '| ' + w.map((n, c) => (rows[k][c] ?? '') + ' '.repeat(n - width(rows[k][c] ?? ''))).join(' | ') + ' |'
    }
    i = j - 1
  }
  return lines.join('\n')
}

// ---------------------------------------------------------------- completion

export interface StepDefinition {
  id: string
  project_id: string
  keyword: '' | 'given' | 'when' | 'then'
  pattern: string
  kind: 'cucumber' | 'regex'
  description: string
  created_by: string
  created_at: string
  updated_at: string
  usage_count: number
}

export interface Completion {
  /** what the list shows */
  label: string
  /** what replaces the text from `from` to `to` */
  insert: string
  detail?: string
  kind: 'keyword' | 'step'
}

export interface CompletionResult {
  from: number
  to: number
  items: Completion[]
}

/**
 * The text a step definition inserts: optional text included, the first
 * of alternatives, escapes removed; {parameters} stay for the user to
 * replace. A regular expression loses its anchors.
 */
export function patternTemplate(d: Pick<StepDefinition, 'pattern' | 'kind'>): string {
  if (d.kind === 'regex') return d.pattern.replace(/^\^/, '').replace(/\$$/, '')
  let out = ''
  const p = d.pattern
  for (let i = 0; i < p.length; i++) {
    const c = p[i]
    if (c === '\\' && i + 1 < p.length) {
      out += p[++i]
    } else if (c === '(') {
      const j = p.indexOf(')', i)
      if (j < 0) return d.pattern
      out += p.slice(i + 1, j)
      i = j
    } else if (c === '{') {
      const j = p.indexOf('}', i)
      if (j < 0) return d.pattern
      out += p.slice(i, j + 1)
      i = j
    } else if (c !== ' ' && (i === 0 || p[i - 1] === ' ')) {
      // a word; with a/b alternatives keep the first
      let j = i
      while (j < p.length && p[j] !== ' ' && p[j] !== '{' && p[j] !== '(' && p[j] !== '\\') j++
      const word = p.slice(i, j)
      out += word.includes('/') ? word.split('/')[0] : word
      i = j - 1
    } else out += c
  }
  return out
}

/** the first {parameter} of an inserted step text (to select it) */
export function firstParam(s: string): [number, number] | null {
  const m = /\{[a-z]*\}|<[^<>]+>/.exec(s)
  return m ? [m.index, m.index + m[0].length] : null
}

function words(s: string): string[] {
  return s.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean)
}

/** a library step matches typed text when every typed word starts a word of it */
export function stepMatches(typed: string, template: string): boolean {
  const want = words(typed)
  if (!want.length) return true
  const have = words(template)
  return want.every((w, i) => (i === want.length - 1 ? have.some((h) => h.startsWith(w)) : have.includes(w) || have.some((h) => h.startsWith(w))))
}

function preferred(kind: StepKind | '', d: StepDefinition): number {
  if (!d.keyword || kind === 'and' || !kind) return 1
  return d.keyword === kind ? 0 : 2
}

/** the keyword kind the step at a line continues (And/But → the one before) */
function contextKind(lines: string[], lineIndex: number): StepKind | '' {
  for (let i = lineIndex; i >= 0; i--) {
    const t = lines[i].trim()
    const kw = STEP_KEYWORDS.find((k) => t === k || t.startsWith(k + ' '))
    if (!kw) {
      if (i < lineIndex && HEADER_KEYWORDS.some((k) => t.startsWith(k + ':'))) return ''
      continue
    }
    const k = stepKind(kw)
    if (k !== 'and') return k
  }
  return ''
}

/**
 * Completions at a position: header and step keywords while the first
 * word of a line is typed, library steps after a step keyword. Returns
 * null when there is nothing to offer.
 */
export function completions(src: string, pos: number, defs: StepDefinition[], lang: GherkinLang = 'en', explicit = false): CompletionResult | null {
  const start = src.lastIndexOf('\n', pos - 1) + 1
  const before = src.slice(start, pos)
  const lines = src.slice(0, pos).split('\n')
  // inside a doc string: nothing
  let doc = ''
  for (const l of lines.slice(0, -1)) {
    const t = l.trim()
    if (doc) {
      if (t === doc) doc = ''
    } else if (t.startsWith('"""') || t.startsWith('```')) doc = t.slice(0, 3)
  }
  if (doc) return null
  const indent = before.length - before.trimStart().length
  const typed = before.slice(indent)
  if (typed.startsWith('#') || typed.startsWith('@') || typed.startsWith('|')) return null
  const kw = STEP_KEYWORDS.find((k) => typed.startsWith(k + ' ')) ?? (typed.startsWith('* ') ? '*' : undefined)
  if (kw) {
    const text = typed.slice(kw.length + 1)
    if (!explicit && !text.trim()) return null
    const kind = stepKind(kw) === 'and' ? contextKind(lines, lines.length - 2) : stepKind(kw)
    const items = defs
      .map((d) => ({ d, tpl: patternTemplate(d) }))
      .filter((x) => stepMatches(text, x.tpl) && x.tpl !== text)
      .sort((a, b) => preferred(kind, a.d) - preferred(kind, b.d) || b.d.usage_count - a.d.usage_count || a.tpl.localeCompare(b.tpl))
      .slice(0, 50)
      .map((x) => ({ label: x.tpl, insert: x.tpl, detail: x.d.description, kind: 'step' as const }))
    return items.length ? { from: start + indent + kw.length + 1, to: pos, items } : null
  }
  if (/\s/.test(typed) && !HEADER_KEYWORDS.some((k) => k.includes(' ') && k.toLowerCase().startsWith(typed.toLowerCase()))) return null
  if (!explicit && !typed) return null
  const d = DIALECTS[lang]
  const other = DIALECTS[lang === 'en' ? 'sv' : 'en']
  const order = (x: Dialect) => [
    ...x.given.map((k) => k + ' '),
    ...x.when.map((k) => k + ' '),
    ...x.then.map((k) => k + ' '),
    ...x.and.map((k) => k + ' '),
    ...x.but.map((k) => k + ' '),
    ...x.scenario.map((k) => k + ': '),
    ...x.outline.map((k) => k + ': '),
    ...x.examples.map((k) => k + ':'),
    ...x.background.map((k) => k + ':'),
    ...x.feature.map((k) => k + ': '),
    ...x.rule.map((k) => k + ': '),
  ]
  const seen = new Set<string>()
  const items: Completion[] = []
  for (const k of [...order(d), ...order(other)]) {
    if (seen.has(k)) continue
    seen.add(k)
    if (k.toLowerCase().startsWith(typed.toLowerCase()) && k.trim() !== typed.trim()) items.push({ label: k.replace(/:?\s*$/, ''), insert: k, kind: 'keyword' })
  }
  return items.length ? { from: start + indent, to: pos, items: items.slice(0, 30) } : null
}

/** apply a completion to the text; returns the text and the selection after it */
export function applyCompletion(src: string, r: CompletionResult, c: Completion): { text: string; selStart: number; selEnd: number } {
  const text = src.slice(0, r.from) + c.insert + src.slice(r.to)
  const end = r.from + c.insert.length
  if (c.kind === 'step') {
    const p = firstParam(c.insert)
    if (p) return { text, selStart: r.from + p[0], selEnd: r.from + p[1] }
  }
  return { text, selStart: end, selEnd: end }
}

/** the indentation for a new line after a line (one level deeper after a header) */
export function nextIndent(line: string): string {
  const indent = line.slice(0, line.length - line.trimStart().length)
  const t = line.trim()
  if (HEADER_KEYWORDS.some((k) => t.startsWith(k + ':'))) return indent + '  '
  return indent
}

/** the line (1-based) and column (1-based) of a position */
export function lineCol(src: string, pos: number): { line: number; col: number } {
  const before = src.slice(0, pos)
  const line = before.split('\n').length
  return { line, col: pos - before.lastIndexOf('\n') }
}

/** the position of the start of a line (1-based) */
export function lineStart(src: string, line: number): number {
  let pos = 0
  for (let i = 1; i < line; i++) {
    const n = src.indexOf('\n', pos)
    if (n < 0) return src.length
    pos = n + 1
  }
  return pos
}

// ---------------------------------------------------------------- API types

export interface GherkinError {
  line: number
  column: number
  message: string
}

export interface GherkinStepInfo {
  line: number
  keyword: string
  text: string
  defined: boolean
  definition?: string
  suggestion?: string
}

/** POST /projects/{pid}/gherkin/check */
export interface GherkinCheck {
  valid: boolean
  errors: GherkinError[]
  language: GherkinLang
  feature: string
  steps: GherkinStepInfo[]
  scenarios: { name: string; line: number; steps: number }[]
  undefined: number
}

export interface StepUse {
  test_id: string
  test_key: string
  test_title: string
  line: number
  keyword: string
  text: string
}

export interface UndefinedStep {
  text: string
  keyword: string
  suggestion: string
  uses: StepUse[]
}

/** GET /projects/{pid}/step-definitions */
export interface StepLibrary {
  items: StepDefinition[]
  uses: Record<string, StepUse[]>
  undefined: UndefinedStep[]
  unparsed: Record<string, string>
}

/** GET /tests/{id}/gherkin-results */
export interface GherkinResults {
  execution_id: string
  steps: { line: number; status: string; results: number }[]
}

/** the undefined steps of a check, one per text */
export function undefinedSteps(c: GherkinCheck | null): GherkinStepInfo[] {
  const seen = new Set<string>()
  return (c?.steps ?? []).filter((s) => !s.defined && s.text && !seen.has(s.text) && (seen.add(s.text), true))
}

/** the line marks of a check and results for highlight() */
export function lineMarks(c: GherkinCheck | null, results?: GherkinResults | null): Record<number, LineMark> {
  const out: Record<number, LineMark> = {}
  for (const e of c?.errors ?? []) out[e.line] = { ...out[e.line], error: true }
  for (const s of c?.steps ?? []) if (!s.defined) out[s.line] = { ...out[s.line], undefined: true }
  for (const r of results?.steps ?? []) out[r.line] = { ...out[r.line], result: r.status }
  return out
}

/** a step keyword for a library definition's keyword in a language */
export function keywordFor(kind: StepDefinition['keyword'], lang: GherkinLang): string {
  const d = DIALECTS[lang]
  return kind === 'then' ? d.then[0] : kind === 'when' ? d.when[0] : d.given[0]
}

/** a starter feature for a new Gherkin test */
export function starter(title: string, lang: GherkinLang): string {
  const d = DIALECTS[lang]
  const name = title.trim() || (lang === 'sv' ? 'Ny egenskap' : 'New feature')
  return `${lang === 'sv' ? '# language: sv\n' : ''}${d.feature[0]}: ${name}\n\n  ${d.scenario[0]}: ${name}\n    ${d.given[0]} \n    ${d.when[0]} \n    ${d.then[0]} \n`
}
