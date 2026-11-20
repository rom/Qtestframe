import { describe, expect, it } from 'vitest'
import { ANALYSIS_RULES, issueMessage, issueParams } from './analysis'

const issue = (rule: string, message: string, offset = 0, length = 0) => ({ rule, message, offset, length })
const echo = (key: string, params: Record<string, string>) => `${key} ${JSON.stringify(params)}`

describe('issueParams', () => {
  it('recovers terms from the API text (Go %q quoting)', () => {
    expect(issueParams(issue('ambiguous-term', 'ambiguous term "fast" is not verifiable; quantify it'))).toEqual({ term: 'fast' })
    expect(issueParams(issue('tbd', 'placeholder "TBD": requirement is incomplete'))).toEqual({ term: 'TBD' })
    expect(issueParams(issue('and-or', '"and/or" is ambiguous; state whether both or either are required'))).toEqual({ term: 'and/or' })
    expect(issueParams(issue('unbounded-term', 'unbounded term "all"; check …'))).toEqual({ term: 'all' })
    expect(issueParams(issue('passive-capability', '"be able to" describes a capability …'))).toEqual({ term: 'be able to' })
    expect(issueParams(issue('escape-clause', 'escape clause "if possible" weakens …'))).toEqual({ term: 'if possible' })
  })
  it('falls back to the text fragment', () => {
    expect(issueParams(issue('ambiguous-term', 'changed wording', 4, 4), 'The fast system')).toEqual({ term: 'fast' })
    expect(issueParams(issue('ambiguous-term', 'changed wording'), '')).toBeNull()
  })
  it('handles keyword rules', () => {
    expect(issueParams(issue('missing-keyword', 'no obligation keyword (MUST, SHALL, SHOULD, MAY, ...) found'))).toEqual({ examples: 'MUST, SHALL, SHOULD, MAY' })
    expect(issueParams(issue('missing-keyword', 'none'))).toEqual({ examples: 'MUST, SHALL, SHOULD, MAY' })
    expect(issueParams(issue('lowercase-keyword', '"shall" is not in uppercase; per RFC 8174 only uppercase SHALL has normative meaning'))).toEqual({
      term: 'shall',
      keyword: 'SHALL',
    })
    expect(issueParams(issue('lowercase-keyword', '"must" is lowercase'))).toEqual({ term: 'must', keyword: 'MUST' })
    expect(issueParams(issue('lowercase-keyword', 'odd'))).toBeNull()
    expect(issueParams(issue('multiple-obligations', 'sentence contains several obligation keywords (MUST, SHOULD); consider …'))).toEqual({ keywords: 'MUST, SHOULD' })
    expect(issueParams(issue('multiple-obligations', 'no parens'))).toBeNull()
    expect(issueParams(issue('negative-requirement', 'negative requirement (MUST NOT) is harder to verify'))).toEqual({ keyword: 'MUST NOT' })
    expect(issueParams(issue('negative-requirement', 'x'))).toBeNull()
  })
  it('handles long sentences', () => {
    expect(issueParams(issue('long-sentence', 'sentence has 52 words (more than 40); consider splitting it'))).toEqual({ words: '52', max: '40' })
    expect(issueParams(issue('long-sentence', 'too long'))).toBeNull()
  })
  it('prefers structured params from the API', () => {
    const i = { ...issue('ambiguous-negation', 'whatever', 0, 10), params: { term: 'måste inte' } }
    expect(issueParams(i)).toEqual({ term: 'måste inte' })
    expect(issueParams({ ...issue('missing-keyword', 'x'), params: { examples: 'SKA, MÅSTE, BÖR, FÅR' } })).toEqual({ examples: 'SKA, MÅSTE, BÖR, FÅR' })
    expect(issueParams({ ...issue('ambiguous-negation', 'odd', 0, 4), params: {} }, 'måste inte')).toEqual({ term: 'måst' })
  })
  it('knows nothing about other rules', () => {
    expect(issueParams(issue('new-rule', 'x'))).toBeNull()
  })
})

describe('issueMessage', () => {
  it('translates known rules and falls back to the API text', () => {
    expect(issueMessage(issue('tbd', 'placeholder "TBD": requirement is incomplete'), '', echo)).toBe('analysis.rules.tbd {"term":"TBD"}')
    expect(issueMessage(issue('new-rule', 'API text'), '', echo)).toBe('API text')
    expect(issueMessage(issue('long-sentence', 'unexpected shape'), '', echo)).toBe('unexpected shape')
  })
  it('covers all twelve rules', () => {
    expect(ANALYSIS_RULES).toHaveLength(12)
  })
})
