import { describe, expect, it } from 'vitest'
import { actionSeverity, auditParams, auditQuery, prettyDetails, rangeInvalid, toDateParam } from './audit'

describe('toDateParam', () => {
  it('formats local dates', () => {
    expect(toDateParam(new Date(2026, 0, 5, 23, 30))).toBe('2026-01-05')
    expect(toDateParam('2026-02-03')).toBe('2026-02-03')
    expect(toDateParam('2026-02-03T10:00:00Z')).toBe('2026-02-03T10:00:00Z')
    expect(toDateParam('yesterday')).toBe('')
    expect(toDateParam(new Date('x'))).toBe('')
    expect(toDateParam(null)).toBe('')
  })
})

describe('rangeInvalid', () => {
  it('detects from after to', () => {
    expect(rangeInvalid('2026-02-02', '2026-02-01')).toBe(true)
    expect(rangeInvalid('2026-02-01', '2026-02-01')).toBe(false)
    expect(rangeInvalid(new Date(2026, 1, 1), null)).toBe(false)
  })
})

describe('auditParams', () => {
  it('combines field filters with the free query', () => {
    expect(auditQuery({ user: 'alice', action: 'delete', entity: 'test', q: 'details:x OR details:y' })).toBe('user:alice AND action:delete AND entity=test AND (details:x OR details:y)')
    expect(auditQuery({ user: 'a b' })).toBe('user:"a b"')
    expect(auditQuery({})).toBe('')
  })
  it('builds request parameters', () => {
    expect(auditParams({ from: new Date(2026, 0, 1), to: '2026-01-31', user: 'bob', limit: 9000 }, 'csv')).toEqual({
      q: 'user:bob',
      from: '2026-01-01',
      to: '2026-01-31',
      limit: 5000,
      format: 'csv',
    })
    expect(auditParams({ limit: 0 })).toEqual({})
    expect(auditParams({ q: 'action:login' }, 'md')).toEqual({ q: 'action:login', format: 'md' })
  })
})

describe('presentation', () => {
  it('chooses severities', () => {
    expect(actionSeverity('requirement.delete')).toBe('danger')
    expect(actionSeverity('login.failed')).toBe('warn')
    expect(actionSeverity('file.upload')).toBe('success')
    expect(actionSeverity('auth.login')).toBe('info')
    expect(actionSeverity('requirement.update')).toBe('secondary')
  })
  it('pretty prints JSON details', () => {
    expect(prettyDetails('{"a":1}')).toEqual({ json: true, text: '{\n  "a": 1\n}' })
    expect(prettyDetails('{broken')).toEqual({ json: false, text: '{broken' })
    expect(prettyDetails(null)).toEqual({ json: false, text: '' })
  })
})
