import { describe, expect, it } from 'vitest'
import { defectLabel, defectLinks, defectUrl, isUrl, normalizeDefects, templateProblem, validateTemplate } from './defects'

const T = 'https://jira.example.com/browse/{id}'

describe('defects', () => {
  it('detects URLs', () => {
    expect(isUrl('https://x.org/a')).toBe(true)
    expect(isUrl(' http://x ')).toBe(true)
    expect(isUrl('JIRA-1')).toBe(false)
    expect(isUrl('ftp://x')).toBe(false)
    expect(isUrl(null)).toBe(false)
    expect(isUrl('')).toBe(false)
  })
  it('builds links from the template', () => {
    expect(defectUrl('JIRA-123', T)).toBe('https://jira.example.com/browse/JIRA-123')
    expect(defectUrl(' a b ', T)).toBe('https://jira.example.com/browse/a%20b')
    expect(defectUrl('7', 'https://x.org/{id}?again={id}')).toBe('https://x.org/7?again=7')
    expect(defectUrl('7', 'https://x.org/issues/')).toBe('https://x.org/issues/7')
  })
  it('links URLs directly and returns null otherwise', () => {
    expect(defectUrl('https://github.com/o/r/issues/5', T)).toBe('https://github.com/o/r/issues/5')
    expect(defectUrl('https://github.com/o/r/issues/5', '')).toBe('https://github.com/o/r/issues/5')
    expect(defectUrl('JIRA-1', '')).toBeNull()
    expect(defectUrl('JIRA-1', null)).toBeNull()
    expect(defectUrl('JIRA-1', 'javascript:alert({id})')).toBeNull()
    expect(defectUrl('', T)).toBeNull()
    expect(defectUrl(undefined, T)).toBeNull()
  })
  it('labels', () => {
    expect(defectLabel('JIRA-1')).toBe('JIRA-1')
    expect(defectLabel('https://github.com/o/r/issues/5')).toBe('5')
    expect(defectLabel('https://jira.x/browse/ABC-9')).toBe('ABC-9')
    expect(defectLabel('https://bugs.x/show_bug.cgi?id=42')).toBe('42')
    expect(defectLabel('https://bugs.x/')).toBe('bugs.x')
    expect(defectLabel('https://bugs.x/a%20b')).toBe('a b')
    expect(defectLabel('https://[bad')).toBe('https://[bad')
    expect(defectLabel(null)).toBe('')
  })
  it('normalizes lists', () => {
    expect(normalizeDefects([' A-1', 'a-1', '', null, 'B-2 ', undefined])).toEqual(['A-1', 'B-2'])
    expect(normalizeDefects(null)).toEqual([])
  })
  it('prepares links', () => {
    expect(defectLinks(['A-1', 'https://x.org/i/2'], T)).toEqual([
      { value: 'A-1', label: 'A-1', url: 'https://jira.example.com/browse/A-1' },
      { value: 'https://x.org/i/2', label: '2', url: 'https://x.org/i/2' },
    ])
    expect(defectLinks(undefined)).toEqual([])
  })
  it('validates templates', () => {
    expect(validateTemplate('')).toBeNull()
    expect(validateTemplate(null)).toBeNull()
    expect(validateTemplate(T)).toBeNull()
    expect(validateTemplate('jira/{id}')).toMatch(/http/)
    expect(validateTemplate('https://x.org/')).toMatch(/appended/)
    expect(templateProblem('')).toBeNull()
    expect(templateProblem(T)).toBeNull()
    expect(templateProblem('jira/{id}')).toBe('not_url')
    expect(templateProblem('https://x.org/')).toBe('no_placeholder')
  })
})
