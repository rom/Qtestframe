import { describe, expect, it } from 'vitest'
import {
  colorFor,
  coverageSeverity,
  defaultCategoryColor,
  normalizeHex,
  obligationSeverity,
  prioritySeverity,
  readableTextColor,
  RESULT_COLORS,
  resultSeverity,
  statusSeverity,
} from './colors'

describe('severities', () => {
  it('maps known values', () => {
    expect(statusSeverity('verified')).toBe('success')
    expect(statusSeverity('review')).toBe('warn')
    expect(resultSeverity('failed')).toBe('danger')
    expect(resultSeverity('blocked')).toBe('warn')
    expect(coverageSeverity('partial')).toBe('warn')
    expect(obligationSeverity('must')).toBe('danger')
    expect(obligationSeverity('may')).toBe('info')
    expect(prioritySeverity('high')).toBe('warn')
  })
  it('falls back to secondary', () => {
    for (const fn of [statusSeverity, resultSeverity, coverageSeverity, obligationSeverity, prioritySeverity]) {
      expect(fn('???')).toBe('secondary')
      expect(fn(null)).toBe('secondary')
      expect(fn(undefined)).toBe('secondary')
    }
  })
})

describe('colours', () => {
  it('colorFor uses map or palette', () => {
    expect(colorFor(RESULT_COLORS, 'passed')).toBe('#22c55e')
    expect(colorFor(RESULT_COLORS, 'x', 1)).toMatch(/^#/)
    expect(colorFor({}, 'x', -9)).toMatch(/^#/)
  })
  it('normalizeHex', () => {
    expect(normalizeHex('#ABC')).toBe('#aabbcc')
    expect(normalizeHex('10B981')).toBe('#10b981')
    expect(normalizeHex('red')).toBeNull()
    expect(normalizeHex('')).toBeNull()
    expect(normalizeHex(null)).toBeNull()
  })
  it('readableTextColor', () => {
    expect(readableTextColor('#ffffff')).toBe('#111827')
    expect(readableTextColor('#000')).toBe('#ffffff')
    expect(readableTextColor('bogus')).toBe('#ffffff')
  })
  it('defaultCategoryColor is stable', () => {
    expect(defaultCategoryColor('Polarity')).toBe(defaultCategoryColor('Polarity'))
    expect(defaultCategoryColor('')).toMatch(/^#/)
  })
})
