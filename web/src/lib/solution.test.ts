import { describe, expect, it } from 'vitest'
import {
  applyTemplate,
  ARCHITECTURE_STYLES,
  defaultFormats,
  emptyPreamble,
  estimateTokens,
  normalizePreamble,
  PREAMBLE_FIELDS,
  PREAMBLE_GROUPS,
  preambleBody,
  preambleFilled,
  preambleLimit,
  promptPartFileName,
} from './solution'

describe('solution preamble', () => {
  it('groups every field once', () => {
    const grouped = PREAMBLE_GROUPS.flatMap((g) => g.fields)
    expect(new Set(grouped).size).toBe(grouped.length)
    expect([...grouped].sort()).toEqual([...PREAMBLE_FIELDS, 'architecture_style'].sort())
    expect(ARCHITECTURE_STYLES).toContain('modular_monolith')
    expect(preambleLimit('additional')).toBe(16000)
    expect(preambleLimit('databases')).toBe(4000)
  })
  it('normalizes, counts and builds the body', () => {
    const p = normalizePreamble({ languages: 'Go', architecture_style: 'microservices', version: 3, updated_by: 'a@b' } as never)
    expect(p.frameworks).toBe('')
    expect(preambleFilled(p)).toBe(2)
    expect(preambleFilled(emptyPreamble())).toBe(0)
    expect(normalizePreamble(null)).toEqual(emptyPreamble())
    const body = preambleBody(p)
    expect(body.version).toBe(3)
    expect(body.languages).toBe('Go')
    expect('updated_by' in body).toBe(false)
    expect(Object.keys(body)).toHaveLength(PREAMBLE_FIELDS.length + 3)
  })
  it('applies a template over the current values', () => {
    const cur = { ...emptyPreamble(), additional: 'keep me', languages: 'COBOL' }
    const out = applyTemplate(cur, { id: 'go-postgres-vue', name: { en: 'Go' }, preamble: { languages: 'Go 1.26', architecture_style: 'modular_monolith', frontend: '' } })
    expect(out.template).toBe('go-postgres-vue')
    expect(out.languages).toBe('Go 1.26')
    expect(out.additional).toBe('keep me')
    expect(out.architecture_style).toBe('modular_monolith')
    expect(cur.languages).toBe('COBOL')
  })
})

describe('solution prompts', () => {
  it('estimates tokens like the server', () => {
    expect(estimateTokens('')).toBe(0)
    expect(estimateTokens('abcd')).toBe(1)
    expect(estimateTokens('abcde')).toBe(2)
    expect(estimateTokens('åäöå')).toBe(1)
  })
  it('has default formats per kind', () => {
    expect(defaultFormats('architecture')).toEqual(['mermaid', 'c4'])
    expect(defaultFormats('full_solution')).toHaveLength(4)
    expect(defaultFormats('implementation_plan')).toEqual([])
    expect(defaultFormats('implement')).toEqual([])
  })
  it('names part files', () => {
    const d = new Date('2026-11-20T10:00:00Z')
    expect(promptPartFileName('SHOP', 'full_solution', 2, 3, d)).toBe('SHOP-full-solution-2026-11-20-part-02.md')
    expect(promptPartFileName('', 'design', 1, 1, d)).toBe('prompt-design-2026-11-20.md')
  })
})
