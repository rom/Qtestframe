import { describe, expect, it } from 'vitest'
import { ciForm, ciPayload, deliverySeverity, shortSHA, splitConditions, validateCI } from './ci'

describe('CI posting form', () => {
  it('reads the saved settings', () => {
    const f = ciForm({ provider: 'gitlab', api_url: 'https://gl/api/v4', project: 'g/p', post_status: true, post_note: false, release_id: 'r1', conditions: ['must', 'no_failed'] })
    expect(f).toEqual({ enabled: true, api_url: 'https://gl/api/v4', project: 'g/p', post_status: true, post_note: false, release_id: 'r1', conditions: 'must, no_failed', token: '' })
    expect(ciForm(null)).toMatchObject({ enabled: false, post_status: true, post_note: true, conditions: '' })
    expect(ciForm({ provider: '', post_status: false, post_note: false })).toMatchObject({ post_status: true, post_note: true })
  })

  it('builds the request: token only when given or cleared', () => {
    const f = { ...ciForm(null), enabled: true, project: ' g/p ', conditions: 'must, , coverage>=80', token: ' tok ' }
    expect(ciPayload(f)).toEqual({ ci: { provider: 'gitlab', api_url: '', project: 'g/p', post_status: true, post_note: true, release_id: '', conditions: ['must', 'coverage>=80'] }, ci_token: 'tok' })
    expect(ciPayload({ ...f, token: '' }).ci_token).toBeUndefined()
    expect(ciPayload(f, true)).toMatchObject({ clear_ci_token: true })
    expect(ciPayload(f, true).ci_token).toBeUndefined()
    const off = ciPayload({ ...f, enabled: false })
    expect(off.ci).toMatchObject({ provider: '', post_status: false, post_note: false })
  })

  it('validates only what posting needs', () => {
    const f = { ...ciForm(null), enabled: true }
    expect(validateCI(f, false)).toEqual({ project: 'project', token: 'token' })
    expect(validateCI({ ...f, project: 'g/p' }, true)).toEqual({})
    expect(validateCI({ ...f, project: 'g/p', api_url: 'gitlab.example.com' }, true)).toEqual({ api_url: 'url' })
    expect(validateCI({ ...f, project: 'g/p', token: 'a b' }, true)).toEqual({ token: 'tokenSpace' })
    expect(validateCI({ ...f, enabled: false }, false)).toEqual({})
  })

  it('helpers', () => {
    expect(splitConditions(' a,b ,, ')).toEqual(['a', 'b'])
    expect(deliverySeverity('sent')).toBe('success')
    expect(deliverySeverity('failed')).toBe('danger')
    expect(deliverySeverity('retry')).toBe('warn')
    expect(deliverySeverity('queued')).toBe('info')
    expect(shortSHA('0123456789abcdef')).toBe('01234567')
  })
})
