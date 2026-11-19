import { describe, expect, it } from 'vitest'
import { contextHelpPath, guidePageForRoute } from './context'

describe('context help', () => {
  it('maps app pages to guide pages', () => {
    expect(guidePageForRoute('requirements')).toBe('requirements')
    expect(guidePageForRoute('execute')).toBe('runs')
    expect(guidePageForRoute('members')).toBe('administration')
    expect(guidePageForRoute('admin-backup')).toBe('administration')
  })
  it('has no page for unknown routes', () => {
    expect(guidePageForRoute('help')).toBeNull()
    expect(guidePageForRoute(undefined)).toBeNull()
    expect(contextHelpPath('help')).toBe('/help')
    expect(contextHelpPath('runs')).toBe('/help/runs')
  })
})
