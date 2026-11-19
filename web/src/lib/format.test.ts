import { describe, expect, it } from 'vitest'
import { formatDate, formatDateTime, formatDuration, humanize, initials, options, percent, pluralize, relativeTime, truncate } from './format'

describe('humanize', () => {
  it('turns identifiers into labels', () => {
    expect(humanize('must_not')).toBe('Must not')
    expect(humanize('non-functional')).toBe('Non functional')
    expect(humanize('')).toBe('')
    expect(humanize(null)).toBe('')
  })
})

describe('dates', () => {
  it('formats dates', () => {
    expect(formatDate('2026-03-04T10:00:00Z')).toBe('2026-03-04')
    expect(formatDate('')).toBe('')
    expect(formatDate(null)).toBe('')
    expect(formatDate('garbage')).toBe('')
    expect(formatDate('0001-01-01T00:00:00Z')).toBe('')
    const d = new Date(2026, 0, 2, 3, 4)
    expect(formatDateTime(d)).toBe('2026-01-02 03:04')
    expect(formatDateTime(undefined)).toBe('')
  })
  it('relative times', () => {
    const now = new Date('2026-01-10T12:00:00Z')
    expect(relativeTime('2026-01-10T11:59:50Z', now)).toBe('just now')
    expect(relativeTime('2026-01-10T11:55:00Z', now)).toBe('5 minutes ago')
    expect(relativeTime('2026-01-10T11:59:00Z', now)).toBe('1 minute ago')
    expect(relativeTime('2026-01-10T09:00:00Z', now)).toBe('3 hours ago')
    expect(relativeTime('2026-01-12T12:00:00Z', now)).toBe('in 2 days')
    expect(relativeTime('2025-01-10T12:00:00Z', now)).toBe('1 year ago')
    expect(relativeTime('2025-11-10T12:00:00Z', now)).toBe('2 months ago')
    expect(relativeTime(null, now)).toBe('')
    expect(relativeTime(now.getTime())).not.toBe('')
  })
})

describe('numbers', () => {
  it('durations', () => {
    expect(formatDuration(0)).toBe('')
    expect(formatDuration(null)).toBe('')
    expect(formatDuration(-5)).toBe('')
    expect(formatDuration(250)).toBe('250 ms')
    expect(formatDuration(1500)).toBe('1.5 s')
    expect(formatDuration(2000)).toBe('2 s')
    expect(formatDuration(65000)).toBe('1 m 5 s')
    expect(formatDuration(120000)).toBe('2 m')
    expect(formatDuration(3_900_000)).toBe('1 h 5 m')
  })
  it('percent and pluralize', () => {
    expect(percent(1, 3)).toBe(33)
    expect(percent(1, 0)).toBe(0)
    expect(pluralize(1, 'test')).toBe('1 test')
    expect(pluralize(2, 'test')).toBe('2 tests')
    expect(pluralize(2, 'entry', 'entries')).toBe('2 entries')
  })
})

describe('strings', () => {
  it('initials', () => {
    expect(initials('Ada Lovelace')).toBe('AL')
    expect(initials('ada.lovelace@example.com')).toBe('AL')
    expect(initials('admin@example.com')).toBe('AD')
    expect(initials('')).toBe('?')
    expect(initials(null)).toBe('?')
    expect(initials('...')).toBe('?')
  })
  it('truncate', () => {
    expect(truncate('hello world', 5)).toBe('hell…')
    expect(truncate('hi', 5)).toBe('hi')
    expect(truncate(null, 5)).toBe('')
  })
  it('options', () => {
    expect(options(['must_not'])).toEqual([{ label: 'Must not', value: 'must_not' }])
    expect(options(null)).toEqual([])
  })
})
