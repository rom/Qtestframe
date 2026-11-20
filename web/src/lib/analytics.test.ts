import { describe, expect, it } from 'vitest'
import { findingText, severityIcon, type AnalyticsFinding } from './analytics'

const t = (k: string, p?: Record<string, unknown>) => `${k}${p ? JSON.stringify(p) : ''}`
const known = new Set(['analytics.code.inconsistent_keyword.title', 'analytics.code.inconsistent_keyword.fix', 'analytics.family.mandatory', 'analytics.code.language_rule.title', 'analytics.rule.tbd'])
const te = (k: string) => known.has(k)
const finding = (o: Partial<AnalyticsFinding>): AnalyticsFinding => ({ code: 'inconsistent_keyword', severity: 'warning', category: 'consistency', params: {}, items: [], total: 1, ...o })

describe('analytics texts', () => {
  it('translates codes, families and rules', () => {
    const x = findingText(finding({ params: { family: 'mandatory', dominant: 'MUST' } }), t, te)
    expect(x.title).toContain('analytics.code.inconsistent_keyword.title')
    expect(x.title).toContain('"family":"analytics.family.mandatory"')
    expect(x.title).toContain('"n":1')
    expect(x.detail).toBe('')
    expect(x.fix).toContain('.fix')
    const r = findingText(finding({ code: 'language_rule', params: { rule: 'tbd' } }), t, te)
    expect(r.title).toContain('"rule":"analytics.rule.tbd"')
    const unknownRule = findingText(finding({ code: 'language_rule', params: { rule: 'new-rule', family: 'odd' } }), t, te)
    expect(unknownRule.title).toContain('"rule":"new-rule"')
    expect(unknownRule.title).toContain('"family":"odd"')
    expect(findingText(finding({ code: 'brand_new' }), t, te).title).toBe('brand_new')
  })
  it('picks icons', () => {
    expect(severityIcon('problem')).toContain('times')
    expect(severityIcon('warning')).toContain('exclamation')
    expect(severityIcon('info')).toContain('info')
  })
})

describe('language names', () => {
  it('names language codes', async () => {
    const { languageNames, itemNote } = await import('./analytics')
    const tt = (k: string) => ({ 'analysis.languages.en': 'English', 'analysis.languages.sv': 'Swedish' })[k] ?? k
    const tte = (k: string) => k.startsWith('analysis.languages.')
    expect(languageNames('en 90%, sv 10%', tt, tte)).toBe('English 90%, Swedish 10%')
    expect(itemNote({ code: 'mixed_language_text', severity: 'warning', category: 'x', params: {}, items: [], total: 0 }, 'en + sv', tt, tte)).toBe('English + Swedish')
    expect(itemNote({ code: 'rfc_casing', severity: 'warning', category: 'x', params: {}, items: [], total: 0 }, 'en', tt, tte)).toBe('en')
    expect(itemNote({ code: 'rfc_casing', severity: 'warning', category: 'x', params: {}, items: [], total: 0 }, undefined, tt, tte)).toBe('')
    const f = { code: 'mixed_languages', severity: 'warning', category: 'consistency', params: { dominant: 'en', languages: 'en 90%, sv 10%' }, items: [], total: 1 }
    expect(findingText(f, (k, p) => `${k}${JSON.stringify(p)}`, () => true).title).toContain('"dominant":"analysis.languages.en')
  })
})
