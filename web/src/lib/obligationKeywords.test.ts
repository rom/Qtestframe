import { describe, expect, it } from 'vitest'
import cases from './obligationKeywords.cases.json'
import table from './obligationKeywords.json'
import { checkKeywords, codeRanges, keywordExamples, detectLanguage, displayLevel, findObligations, highlightRanges, LANGUAGES, obligationPieces, splitPiece, strongestLevel } from './obligationKeywords'

const marked = (text: string, opts = {}) => obligationPieces(text, opts).filter((p) => p.keyword).map((p) => [p.text, p.display])

describe('parity with the Go analysis (internal/reqlang)', () => {
  // obligationKeywords.cases.json holds Analyze() results written by the Go test
  it.each(cases.map((c) => [c.text || '(empty)', c] as const))('%s', (_, c) => {
    if (c.text) expect(detectLanguage(c.text)).toBe(c.language)
    const ms = findObligations(c.text)
    expect(ms.map((m) => ({ text: m.text, keyword: m.keyword, level: m.level }))).toEqual(c.keywords)
    expect(strongestLevel(ms)).toBe(c.obligation)
  })
  it('uses the table generated from the language packs', () => {
    expect(LANGUAGES).toEqual(table.packs.map((p) => p.code))
    expect(LANGUAGES).toContain('sv')
  })
})

describe('highlighting', () => {
  it('marks English keywords, negations as one unit', () => {
    expect(marked('The system MUST NOT log it; it SHOULD warn and MAY retry. SHALL\n  NOT restart.')).toEqual([
      ['MUST NOT', 'must-not'],
      ['SHOULD', 'should'],
      ['MAY', 'may'],
      ['SHALL\n  NOT', 'must-not'],
    ])
  })
  it('marks Swedish keywords, negations as one unit', () => {
    expect(marked('Systemet SKA INTE spara det, BÖR INTE visa det och FÅR logga.')).toEqual([
      ['SKA INTE', 'must-not'],
      ['BÖR INTE', 'should-not'],
      ['FÅR', 'may'],
    ])
    // in lower case only on request (the help asks for capitals)
    expect(marked('Användaren får inte radera loggen men ska kunna läsa den.')).toEqual([])
    expect(marked('Användaren får inte radera loggen men ska kunna läsa den.', { lowercase: true })).toEqual([
      ['får inte', 'must-not'],
      ['ska', 'must'],
    ])
  })
  it('respects word boundaries', () => {
    expect(marked('MAYBE the MUSTER is SHALLOW')).toEqual([])
    expect(marked('Systemet FÅRA SKALA BÖRJA')).toEqual([])
    expect(marked('MUST_NOT MUST2 xMUST')).toEqual([])
    expect(marked('(MUST) "SHOULD"')).toEqual([
      ['MUST', 'must'],
      ['SHOULD', 'should'],
    ])
  })
  it('handles Swedish characters and offsets after them', () => {
    const text = 'Användaren FÅR ändra ålder; systemet BÖR spara.'
    const ms = highlightRanges(text)
    expect(ms.map((m) => text.slice(m.start, m.end))).toEqual(['FÅR', 'BÖR'])
  })
  it('marks lower-case English keywords only on request (RFC 8174)', () => {
    expect(marked('It may rain and the system must log.')).toEqual([])
    expect(marked('It may rain and the system must log.', { lowercase: true })).toEqual([
      ['may', 'may'],
      ['must', 'must'],
    ])
  })
  it('does not mark WILL or NOTE:', () => {
    expect(marked('The system WILL start. NOTE: it MUST log.')).toEqual([['MUST', 'must']])
    expect(displayLevel('will')).toBeNull()
    expect(displayLevel('describe')).toBeNull()
    expect(displayLevel('optional')).toBe('may')
    expect(displayLevel('shall')).toBe('must')
  })
  it('uses the given language pack', () => {
    // English text with a Swedish pack: only uppercase RFC words
    expect(marked('The user SKA do it and MUST log.', { lang: 'sv' })).toEqual([
      ['SKA', 'must'],
      ['MUST', 'must'],
    ])
    expect(marked('The user SKA do it and MUST log.', { lang: 'en' })).toEqual([['MUST', 'must']])
  })
  it('never marks inside code spans', () => {
    expect(marked('Use `MUST` here, but it MUST log.')).toEqual([['MUST', 'must']])
    expect(marked('Use ``a `MAY` b`` and SHOULD.')).toEqual([['SHOULD', 'should']])
    expect(marked('```\nThe MUST block\n```\nThe system SHALL log.')).toEqual([['SHALL', 'must']])
    expect(marked('An unclosed `MUST tick')).toEqual([['MUST', 'must']])
    expect(codeRanges('a `b` c')).toEqual([[2, 5]])
  })
  it('never marks inside excluded ranges (links)', () => {
    const text = 'See https://x.test/MUST and MUST log.'
    expect(marked(text, { exclude: [[4, 23]] })).toEqual([['MUST', 'must']])
  })
  it('splits pieces of a larger text', () => {
    const text = 'A MUST b SHOULD c'
    const rs = highlightRanges(text)
    expect(splitPiece('b SHOULD c', 7, rs).map((p) => p.text)).toEqual(['b ', 'SHOULD', ' c'])
    // a range only partly inside the piece is not marked
    expect(splitPiece('UST b', 3, rs)).toEqual([{ text: 'UST b' }])
    expect(obligationPieces('')).toEqual([])
    expect(obligationPieces('no keyword')).toEqual([{ text: 'no keyword' }])
  })
  it('keeps the text intact (pieces concatenate to the input)', () => {
    for (const c of cases) expect(obligationPieces(c.text).map((p) => p.text).join('')).toBe(c.text)
  })
  it('treats markup in the text as text (no HTML)', () => {
    const text = '<img src=x onerror=alert(1)> MUST <b>SHOULD</b>'
    const ps = obligationPieces(text)
    expect(ps.map((p) => p.text).join('')).toBe(text)
    expect(ps.filter((p) => p.keyword).map((p) => p.text)).toEqual(['MUST', 'SHOULD'])
  })
})

describe('checkKeywords', () => {
  it('flags a mismatch with the chosen obligation', () => {
    expect(checkKeywords('The system SHOULD log.', 'must')).toMatchObject({ detected: 'should', mismatch: true, multiple: false })
    expect(checkKeywords('The system SHOULD log.', 'should').mismatch).toBe(false)
    expect(checkKeywords('The system SHOULD log.', '').mismatch).toBe(false)
    expect(checkKeywords('No keyword.', 'must')).toMatchObject({ detected: '', mismatch: false })
  })
  it('flags several keywords and lowercase ones', () => {
    const c = checkKeywords('The system MUST log and should alert. NOTE: x', '')
    expect(c.distinct).toEqual(['MUST', 'SHOULD'])
    expect(c.multiple).toBe(true)
    expect(c.lowercase.map((m) => m.text)).toEqual(['should'])
    expect(checkKeywords('MUST and MUST', '').multiple).toBe(false)
    expect(checkKeywords('Systemet ska logga.', 'shall', 'sv').lowercase).toEqual([])
  })
  it('suggests keywords in the language of the text', () => {
    expect(keywordExamples('The system logs it.')).toContain('MUST')
    expect(keywordExamples('Systemet loggar det och användaren ser det.')).toContain('SKA')
    expect(keywordExamples('', 'sv')).toContain('BÖR')
  })
})
