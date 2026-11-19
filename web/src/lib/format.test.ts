import { describe, expect, it } from 'vitest'
import { formatDate, formatDatePref, formatDateTime, formatDateTimePref, formatDuration, formatFullDateTime, getDatePrefs, humanize, initials, isValidTimeZone, options, percent, pluralize, relativeTime, setDatePrefs, truncate } from './format'

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

describe('preference based dates', () => {
  const d = '2026-01-10T11:55:00Z'
  const now = new Date('2026-01-10T12:00:00Z')
  it('formats iso in a time zone', () => {
    expect(formatDateTimePref(d, { timezone: 'UTC' }, now)).toBe('2026-01-10 11:55')
    expect(formatDateTimePref(d, { date_format: 'iso', timezone: 'Asia/Tokyo' }, now)).toBe('2026-01-10 20:55')
    expect(formatDateTimePref('2026-01-10T23:30:00Z', { timezone: 'Europe/Stockholm' })).toBe('2026-01-11 00:30')
    expect(formatDatePref('2026-01-10T23:30:00Z', { timezone: 'Europe/Stockholm' })).toBe('2026-01-11')
    expect(formatDatePref(d, { timezone: 'UTC', date_format: 'iso' })).toBe('2026-01-10')
  })
  it('falls back to the browser zone for unknown zones', () => {
    const local = new Date(2026, 0, 2, 3, 4)
    expect(formatDateTimePref(local, { timezone: 'Nowhere/Land' })).toBe('2026-01-02 03:04')
    expect(formatDateTimePref(local, {})).toBe('2026-01-02 03:04')
    expect(formatDatePref(local, null)).toBe('2026-01-02')
    expect(isValidTimeZone('UTC')).toBe(true)
    expect(isValidTimeZone('Nowhere/Land')).toBe(false)
    expect(isValidTimeZone('')).toBe(false)
  })
  it('formats relative', () => {
    expect(formatDateTimePref(d, { date_format: 'relative' }, now)).toBe('5 min ago')
    expect(formatDatePref('2026-01-08T12:00:00Z', { date_format: 'relative' }, now)).toBe('2 days ago')
    expect(relativeTime('2026-01-10T11:59:00Z', now, true)).toBe('1 min ago')
  })
  it('formats locale', () => {
    const s = formatDateTimePref(d, { date_format: 'locale', timezone: 'UTC' }, now)
    expect(s).toMatch(/2026/)
    expect(s).toMatch(/11/)
    expect(formatDatePref(d, { date_format: 'locale', timezone: 'UTC' })).toMatch(/2026/)
  })
  it('returns empty for missing values', () => {
    expect(formatDateTimePref(null, {})).toBe('')
    expect(formatDatePref('', {})).toBe('')
    expect(formatFullDateTime(null)).toBe('')
  })
  it('applies active preferences to formatDate / formatDateTime', () => {
    try {
      setDatePrefs({ timezone: 'Asia/Tokyo' })
      expect(getDatePrefs()).toEqual({ date_format: '', timezone: 'Asia/Tokyo' })
      expect(formatDateTime(d)).toBe('2026-01-10 20:55')
      expect(formatDate('2026-01-10T20:00:00Z')).toBe('2026-01-11')
      expect(formatFullDateTime(d)).toBe('2026-01-10 20:55 (Asia/Tokyo)')
      setDatePrefs({ date_format: 'relative' })
      expect(formatDateTime(new Date(Date.now() - 120000))).toBe('2 min ago')
      expect(formatFullDateTime(new Date(2026, 0, 2, 3, 4))).toBe('2026-01-02 03:04')
    } finally {
      setDatePrefs(null)
    }
    expect(getDatePrefs()).toEqual({ date_format: '', timezone: '' })
  })
})
