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
