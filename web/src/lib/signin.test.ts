import { describe, expect, it } from 'vitest'
import { failedAttempts, generatePassword, mustChangePassword, noticeKey, shouldShowNotice } from './signin'
import type { Me } from './types'

const me = (over: Partial<Me> = {}, user: Record<string, unknown> = {}): Me =>
  ({
    user: { id: 'u1', email: 'a@x', display_name: '', super_admin: false, auth_provider: 'local', disabled: false, created_at: '', preferences: { intro_done: true }, ...user },
    tenant: null,
    role: 'admin',
    permissions: [],
    memberships: [],
    csrf_token: 'csrf-token-value-123',
    previous_login: { created_at: '2026-01-01T00:00:00Z', ip: '1.2.3.4', user_agent: 'x', method: 'password' },
    ...over,
  }) as Me

describe('sign-in notice', () => {
  it('shows once per session', () => {
    const m = me()
    expect(shouldShowNotice(m, null)).toBe(true)
    expect(shouldShowNotice(m, noticeKey(m))).toBe(false)
    expect(shouldShowNotice(me({ csrf_token: 'other-session-xyz' }), noticeKey(m))).toBe(true)
  })
  it('is not shown on the first login, without a previous login or when the password must change', () => {
    expect(shouldShowNotice(me({ first_login: true }), null)).toBe(false)
    expect(shouldShowNotice(me({ previous_login: null }), null)).toBe(false)
    expect(shouldShowNotice(me({}, { must_change_password: true }), null)).toBe(false)
    expect(shouldShowNotice(null, null)).toBe(false)
    expect(shouldShowNotice(me({}, { preferences: {} }), null)).toBe(false)
    expect(shouldShowNotice(me({}, { preferences: null }), null)).toBe(false)
    expect(noticeKey(me({ previous_login: null }))).toBe('')
  })
  it('counts failed attempts', () => {
    expect(failedAttempts(me({ failed_logins_since_previous: 3 }))).toBe(3)
    expect(failedAttempts(me({ failed_logins_since_previous: 0 }))).toBe(0)
    expect(failedAttempts(me())).toBe(0)
  })
  it('reads must_change_password', () => {
    expect(mustChangePassword(me({}, { must_change_password: true }))).toBe(true)
    expect(mustChangePassword(null)).toBe(false)
  })
})

describe('generatePassword', () => {
  it('has every character class and the length', () => {
    for (let i = 0; i < 20; i++) {
      const p = generatePassword(16)
      expect(p).toHaveLength(16)
      expect(p).toMatch(/[A-Z]/)
      expect(p).toMatch(/[a-z]/)
      expect(p).toMatch(/[2-9]/)
      expect(p).toMatch(/[-_.!+]/)
      expect(p).not.toMatch(/[0O1lI]/)
    }
  })
  it('is at least 12 characters and uses the given random source', () => {
    expect(generatePassword(4, () => 0)).toHaveLength(12)
  })
})
