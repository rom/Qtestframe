import { describe, expect, it } from 'vitest'
import { isPromptKind, isSoftware, PROJECT_TYPE_ICONS, PROJECT_TYPES, PROMPT_KINDS, promptFileName, promptForRequirements, promptNeedsTestType, TEST_TYPES } from './projectTypes'

describe('project types and prompt kinds', () => {
  it('knows software projects', () => {
    expect(isSoftware({ type: 'software' })).toBe(true)
    expect(isSoftware({ type: 'general' })).toBe(false)
    expect(isSoftware(null)).toBe(false)
    for (const t of PROJECT_TYPES) expect(PROJECT_TYPE_ICONS[t]).toMatch(/^pi pi-/)
  })
  it('classifies prompt kinds', () => {
    expect(PROMPT_KINDS.filter(promptForRequirements)).toEqual(['implement', 'review_requirements', 'design_tests', 'architecture', 'design', 'implementation_plan', 'full_solution'])
    expect(PROMPT_KINDS.filter(promptNeedsTestType)).toEqual(['design_tests', 'test_strategy'])
    expect(isPromptKind('automate_tests')).toBe(true)
    expect(isPromptKind('x')).toBe(false)
    expect(isPromptKind(['implement'])).toBe(false)
    expect(TEST_TYPES).toContain('accessibility')
  })
  it('names downloads', () => {
    expect(promptFileName('SHOP', 'automate_tests', new Date('2026-11-20T10:00:00Z'))).toBe('SHOP-automate-tests-2026-11-20.md')
    expect(promptFileName('', 'implement', new Date('2026-01-02T00:00:00Z'))).toBe('prompt-implement-2026-01-02.md')
  })
})
