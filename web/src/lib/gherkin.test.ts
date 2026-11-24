import { describe, expect, it } from 'vitest'
import {
  applyCompletion,
  completions,
  firstParam,
  formatTables,
  highlight,
  keywordFor,
  languageOf,
  lineCol,
  lineMarks,
  lineStart,
  nextIndent,
  patternTemplate,
  splitRow,
  starter,
  stepKind,
  stepMatches,
  tokenize,
  undefinedSteps,
  type GherkinCheck,
  type StepDefinition,
} from './gherkin'

const def = (pattern: string, extra: Partial<StepDefinition> = {}): StepDefinition => ({
  id: pattern,
  project_id: 'p',
  keyword: '',
  pattern,
  kind: 'cucumber',
  description: '',
  created_by: '',
  created_at: '',
  updated_at: '',
  usage_count: 0,
  ...extra,
})

const src = `# language: en
@shop @REQ-1
Feature: Checkout
  Scenario Outline: Pay <amount>
    Given a customer
    When the customer pays <amount> EUR and "x"
    Then 3 items
      """
      Given not a step
      """
    Examples:
      | amount | a\\|b |
      | 5 | 10 |
`

describe('gherkin tokens', () => {
  it('splits lines into tokens that join back to the line', () => {
    const toks = tokenize(src)
    const lines = src.split('\n')
    toks.forEach((t, i) => expect(t.map((x) => x.text).join('')).toBe(lines[i]))
    expect(toks[0][0].kind).toBe('comment')
    expect(toks[1].filter((t) => t.kind === 'tag').map((t) => t.text)).toEqual(['@shop', '@REQ-1'])
    expect(toks[2][0]).toEqual({ kind: 'keyword', text: 'Feature:' })
    expect(toks[3].find((t) => t.kind === 'keyword')?.text).toBe('Scenario Outline:')
    expect(toks[5].map((t) => t.kind)).toEqual(['text', 'step', 'text', 'placeholder', 'text', 'string'])
    expect(toks[6].some((t) => t.kind === 'number')).toBe(true)
    expect(toks[8].every((t) => t.kind !== 'step')).toBe(true)
    expect(toks[11].filter((t) => t.kind === 'pipe')).toHaveLength(3)
  })
  it('highlights escaped html with line marks', () => {
    const html = highlight('Feature: <b>\n  Scenario: s\n\n    Given x', { 1: { error: true }, 4: { undefined: true, result: 'failed' } })
    expect(html).toContain('<div class="gh-line gh-error"><span class="gh-keyword">Feature:</span> <span class="gh-placeholder">&lt;b&gt;</span></div>')
    expect(html).toContain('gh-undefined gh-result-failed')
    expect(html).toContain('<div class="gh-line"> </div>')
    expect(tokenize('Givet en {int} sak\n* x')[0].map((t) => t.kind)).toEqual(['step', 'text', 'param', 'text'])
    expect(tokenize('* x')[0][0].kind).toBe('step')
  })
})

describe('gherkin tables', () => {
  it('splits rows with escapes', () => {
    expect(splitRow('| a | b\\|c |  |')).toEqual(['a', 'b\\|c', ''])
    expect(splitRow('| a | b')).toEqual(['a', 'b'])
  })
  it('aligns tables and leaves the rest', () => {
    const out = formatTables(src)
    expect(out).toContain('      | amount | a\\|b |\n      | 5      | 10   |')
    expect(out.split('\n')[5]).toBe(src.split('\n')[5])
    expect(formatTables('  """\n  | x |\n  """\n|a|bb|\n|ccc|')).toBe('  """\n  | x |\n  """\n| a   | bb |\n| ccc |    |')
    expect(formatTables('| å | b |\n| xx | y |')).toBe('| å  | b |\n| xx | y |')
  })
})

describe('gherkin completion', () => {
  const defs = [
    def('the customer pays {int} EUR', { keyword: 'when', usage_count: 3, description: 'Pays' }),
    def('I log in/on as {string}', { keyword: 'given' }),
    def('I have {int} cucumber(s)'),
    def('^the order is (\\w+)$', { kind: 'regex', keyword: 'then' }),
    def('a \\{literal\\}'),
  ]
  it('makes templates of patterns', () => {
    expect(patternTemplate(defs[1])).toBe('I log in as {string}')
    expect(patternTemplate(defs[2])).toBe('I have {int} cucumbers')
    expect(patternTemplate(defs[3])).toBe('the order is (\\w+)')
    expect(patternTemplate(defs[4])).toBe('a {literal}')
    expect(patternTemplate(def('broken (x'))).toBe('broken (x')
    expect(patternTemplate(def('broken {x'))).toBe('broken {x')
    expect(firstParam('pay {int} now')).toEqual([4, 9])
    expect(firstParam('pay <x>')).toEqual([4, 7])
    expect(firstParam('pay')).toBeNull()
  })
  it('matches typed words', () => {
    expect(stepMatches('', 'x')).toBe(true)
    expect(stepMatches('cust pa', 'the customer pays {int} EUR')).toBe(true)
    expect(stepMatches('order', 'the customer pays')).toBe(false)
  })
  it('offers library steps after a step keyword, preferring the kind', () => {
    const text = 'Feature: f\n  Scenario: s\n    When the cu'
    const r = completions(text, text.length, defs)!
    expect(r.items.map((i) => i.label)).toEqual(['the customer pays {int} EUR'])
    expect(r.from).toBe(text.length - 'the cu'.length)
    const applied = applyCompletion(text, r, r.items[0])
    expect(applied.text.endsWith('When the customer pays {int} EUR')).toBe(true)
    expect(applied.text.slice(applied.selStart, applied.selEnd)).toBe('{int}')
    // And continues Then: the then-step first
    const t2 = 'Feature: f\n  Scenario: s\n    Then x\n    And '
    const r2 = completions(t2, t2.length, defs, 'en', true)!
    expect(r2.items[0].label).toBe('the order is (\\w+)')
    // nothing typed and not asked for
    expect(completions(t2, t2.length, defs)).toBeNull()
    // Swedish keyword
    const t3 = 'Egenskap: f\n  Scenario: s\n    Givet I lo'
    expect(completions(t3, t3.length, defs, 'sv')!.items[0].label).toBe('I log in as {string}')
    // no match
    const t4 = 'Feature: f\n  Scenario: s\n    When zzz'
    expect(completions(t4, t4.length, defs)).toBeNull()
  })
  it('offers keywords at the start of a line', () => {
    const text = 'Feature: f\n  Scenario: s\n    Gi'
    const r = completions(text, text.length, defs)!
    expect(r.items.map((i) => i.label)).toEqual(['Given', 'Givet'])
    expect(applyCompletion(text, r, r.items[0]).text.endsWith('    Given ')).toBe(true)
    const sv = completions('S', 1, defs, 'sv')!
    expect(sv.items[0].label).toBe('Så')
    expect(sv.items.map((i) => i.label)).toContain('Scenario Outline')
    const outline = completions('Scenario O', 10, defs)!
    expect(outline.items[0].insert).toBe('Scenario Outline: ')
    expect(completions('', 0, defs)).toBeNull()
    expect(completions('', 0, defs, 'en', true)!.items.length).toBeGreaterThan(10)
    expect(completions('# Gi', 4, defs)).toBeNull()
    expect(completions('hello there', 11, defs)).toBeNull()
    const inDoc = 'Feature: f\n  Scenario: s\n    Given x\n      """\n      Gi'
    expect(completions(inDoc, inDoc.length, defs)).toBeNull()
    const afterDoc = 'Feature: f\n  Scenario: s\n    Given x\n      """\n      a\n      """\n    Th'
    expect(completions(afterDoc, afterDoc.length, defs)!.items[0].label).toBe('Then')
  })
})

describe('gherkin helpers', () => {
  it('knows languages, kinds and positions', () => {
    expect(languageOf('# language: sv\nFeature: x')).toBe('sv')
    expect(languageOf('# language: en\nEgenskap: x')).toBe('en')
    expect(languageOf('Egenskap: x')).toBe('sv')
    expect(languageOf('Feature: x')).toBe('en')
    expect(languageOf('')).toBe('en')
    expect(stepKind('När')).toBe('when')
    expect(stepKind('Men')).toBe('and')
    expect(stepKind('*')).toBe('and')
    expect(stepKind('Then')).toBe('then')
    expect(stepKind('Givet')).toBe('given')
    expect(stepKind('x')).toBe('')
    expect(nextIndent('  Scenario: x')).toBe('    ')
    expect(nextIndent('    Given x')).toBe('    ')
    expect(lineCol('ab\ncd', 4)).toEqual({ line: 2, col: 2 })
    expect(lineStart('ab\ncd\nef', 3)).toBe(6)
    expect(lineStart('ab', 5)).toBe(2)
    expect(keywordFor('then', 'sv')).toBe('Så')
    expect(keywordFor('when', 'en')).toBe('When')
    expect(keywordFor('', 'en')).toBe('Given')
    expect(starter('', 'sv')).toContain('# language: sv\nEgenskap: Ny egenskap')
    expect(starter('Pay', 'en')).toContain('Feature: Pay\n\n  Scenario: Pay\n    Given ')
  })
  it('marks lines of a check', () => {
    const c: GherkinCheck = {
      valid: false,
      errors: [{ line: 2, column: 1, message: 'x' }],
      language: 'en',
      feature: 'f',
      steps: [
        { line: 3, keyword: 'Given', text: 'a', defined: false, suggestion: 'a' },
        { line: 4, keyword: 'And', text: 'a', defined: false, suggestion: 'a' },
        { line: 5, keyword: 'Then', text: 'b', defined: true, definition: 'd' },
      ],
      scenarios: [],
      undefined: 2,
    }
    expect(lineMarks(c, { execution_id: 'e', steps: [{ line: 5, status: 'passed', results: 1 }] })).toEqual({ 2: { error: true }, 3: { undefined: true }, 4: { undefined: true }, 5: { result: 'passed' } })
    expect(undefinedSteps(c).map((s) => s.line)).toEqual([3])
    expect(undefinedSteps(null)).toEqual([])
    expect(lineMarks(null)).toEqual({})
  })
})
