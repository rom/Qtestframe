import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { codeComplete, daysLeft, groupSecret, isChallenge, isRecoveryCode, isTotpCode, mfaErrorKind, normalizeCode, qrModules, qrPath, recoveryCodesFile } from './mfa'

describe('codes', () => {
  it('groups the setup key', () => {
    expect(groupSecret('JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP')).toBe('JBSW Y3DP EHPK 3PXP JBSW Y3DP EHPK 3PXP')
    expect(groupSecret('ABCDEF')).toBe('ABCD EF')
    expect(groupSecret('')).toBe('')
  })
  it('recognises authenticator and recovery codes', () => {
    expect(normalizeCode(' 123 456 ')).toBe('123456')
    expect(isTotpCode('123 456')).toBe(true)
    expect(isTotpCode('12345')).toBe(false)
    expect(isTotpCode('12345a')).toBe(false)
    expect(isRecoveryCode('abcd-efgh-jkmn')).toBe(true)
    expect(isRecoveryCode('ABCDEFGHJKMN')).toBe(true)
    expect(isRecoveryCode('abcd-efgh')).toBe(false)
    expect(codeComplete('123456', false)).toBe(true)
    expect(codeComplete('123456', true)).toBe(false)
    expect(codeComplete('abcd-efgh-jkmn', true)).toBe(true)
  })
  it('detects a second-factor answer of the sign-in', () => {
    expect(isChallenge({ mfa_required: true, methods: [] })).toBe(true)
    expect(isChallenge({ user: {} })).toBe(false)
    expect(isChallenge(null)).toBe(false)
  })
})

describe('QR code', () => {
  it('encodes an otpauth URI as a square module matrix', () => {
    const m = qrModules('otpauth://totp/Qtestframe:anna%40example.com?secret=JBSWY3DPEHPK3PXP&issuer=Qtestframe')
    expect(m.length).toBeGreaterThanOrEqual(21)
    expect(m.every((row) => row.length === m.length)).toBe(true)
    // finder pattern in the top-left corner
    expect(m[0].slice(0, 7).every(Boolean)).toBe(true)
    expect(m[1][1]).toBe(false)
    const path = qrPath(m, 4)
    expect(path.startsWith('M4 4h1v1h-1z')).toBe(true)
    expect(path.split('M').length - 1).toBe(m.flat().filter(Boolean).length)
  })
})

describe('helpers', () => {
  it('writes the recovery codes file', () => {
    const f = recoveryCodesFile(['a', 'b'], 'anna@example.com', 'Codes', new Date('2026-01-02T10:00:00Z'))
    expect(f).toBe('Codes\nanna@example.com\n2026-01-02\n\na\nb\n')
  })
  it('maps API errors to kinds', () => {
    expect(mfaErrorKind(new ApiError(401, 'mfa_invalid', 'x'))).toBe('invalid')
    expect(mfaErrorKind(new ApiError(401, 'mfa_expired', 'x'))).toBe('expired')
    expect(mfaErrorKind(new ApiError(429, 'rate_limited', 'x'))).toBe('rateLimited')
    expect(mfaErrorKind(new ApiError(403, 'mfa_required', 'x'))).toBe('required')
    expect(mfaErrorKind(new ApiError(500, 'internal', 'x'))).toBe(null)
    expect(mfaErrorKind(new Error('x'))).toBe(null)
  })
  it('counts the days to a deadline', () => {
    const now = new Date('2026-01-01T00:00:00Z')
    expect(daysLeft(null, now)).toBe(null)
    expect(daysLeft('2025-12-31T00:00:00Z', now)).toBe(0)
    expect(daysLeft('2026-01-01T01:00:00Z', now)).toBe(1)
    expect(daysLeft('2026-01-08T00:00:00Z', now)).toBe(7)
  })
})
