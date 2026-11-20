import { describe, expect, it } from 'vitest'
import { contrastRatio } from '@/themes/contrast'
import { isHexColor, PROJECT_COLORS, projectColorStyle, readableOn } from './projectColors'

describe('project colours', () => {
  it('validates colours', () => {
    expect(isHexColor('#1e88e5')).toBe(true)
    expect(isHexColor('#1E88E5')).toBe(true)
    expect(isHexColor('blue')).toBe(false)
    expect(isHexColor('')).toBe(false)
    expect(isHexColor(null)).toBe(false)
  })
  it('picks readable text with AA contrast on every palette colour', () => {
    for (const c of PROJECT_COLORS) expect(contrastRatio(readableOn(c), c), c).toBeGreaterThanOrEqual(4.5)
    expect(readableOn('#ffff00')).toBe('#111827')
    expect(readableOn('#000080')).toBe('#ffffff')
    expect(readableOn('nope')).toBe('#111827')
  })
  it('builds a style', () => {
    expect(projectColorStyle('#000080')).toEqual({ background: '#000080', color: '#ffffff' })
    expect(projectColorStyle('')).toEqual({})
  })
})
