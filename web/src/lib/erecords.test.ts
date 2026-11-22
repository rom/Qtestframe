import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, setChallengeHandler, setCsrfToken } from './api'
import {
  browserTimeZone,
  challengeOf,
  controlSeverity,
  encodeReason,
  meaningLabel,
  parseLines,
  short,
  signErrorKey,
  utcStamp,
  zoneStamp,
} from './erecords'

function res(status: number, body?: unknown): Response {
  return new Response(body === undefined ? '' : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

const fetchMock = vi.fn()
beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  setCsrfToken('tok')
})
afterEach(() => {
  vi.unstubAllGlobals()
  setChallengeHandler(null)
})

describe('challenges of electronic-records mode', () => {
  it('recognises the 428 answers', () => {
    expect(challengeOf(new ApiError(428, 'reason_required', 'why?', undefined, { edit_type: 'requirement.update' }))).toEqual({
      kind: 'reason',
      editType: 'requirement.update',
      message: 'why?',
    })
    expect(challengeOf(new ApiError(428, 'signature_required', 'sign', undefined, { action: 'release.go', meanings: ['approved'] }))).toEqual({
      kind: 'signature',
      action: 'release.go',
      meanings: ['approved'],
      message: 'sign',
    })
    expect(challengeOf(new ApiError(428, 'other', 'x'))).toBeNull()
    expect(challengeOf(new ApiError(400, 'reason_required', 'x'))).toBeNull()
    expect(challengeOf(new Error('x'))).toBeNull()
  })

  it('repeats the request with the headers the handler gives', async () => {
    fetchMock
      .mockResolvedValueOnce(res(428, { code: 'reason_required', error: 'why?', edit_type: 'requirement.update' }))
      .mockResolvedValueOnce(res(428, { code: 'signature_required', error: 'sign', action: 'requirement.approve', meanings: ['approved'] }))
      .mockResolvedValueOnce(res(200, { ok: true }))
    const seen: string[] = []
    setChallengeHandler(async (e) => {
      seen.push(e.code)
      return e.code === 'reason_required' ? { 'X-Change-Reason': encodeReason('CAPA 7') } : { 'X-Signature-Ticket': 't1' }
    })
    expect(await api.patch('/requirements/r1', { status: 'approved' })).toEqual({ ok: true })
    expect(seen).toEqual(['reason_required', 'signature_required'])
    const last = fetchMock.mock.calls[2][1]
    expect(last.headers['X-Change-Reason']).toBe('CAPA%207')
    expect(last.headers['X-Signature-Ticket']).toBe('t1')
    expect(last.headers['X-CSRF-Token']).toBe('tok')
    expect(last.body).toBe('{"status":"approved"}')
  })

  it('gives up when the handler declines', async () => {
    fetchMock.mockResolvedValue(res(428, { code: 'signature_required', error: 'sign', action: 'release.go' }))
    setChallengeHandler(async () => null)
    await expect(api.patch('/releases/x', { status: 'released' })).rejects.toMatchObject({ status: 428, code: 'signature_required' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('marks polling as background', async () => {
    fetchMock.mockImplementation(async () => res(200, {}))
    await api.get('/notifications', undefined, { background: true })
    expect(fetchMock.mock.calls[0][1].headers['X-Qtf-Background']).toBe('1')
    await api.get('/notifications')
    expect(fetchMock.mock.calls[1][1].headers['X-Qtf-Background']).toBeUndefined()
  })
})

describe('signature manifestation', () => {
  it('formats UTC and the signer zone', () => {
    expect(utcStamp('2026-11-22T09:05:03Z')).toBe('2026-11-22 09:05:03 UTC')
    expect(utcStamp('nope')).toBe('nope')
    expect(zoneStamp('2026-11-22T09:05:03Z', 'Europe/Stockholm')).toBe('2026-11-22 10:05 (Europe/Stockholm)')
    expect(zoneStamp('2026-07-01T09:05:03Z', 'Europe/Stockholm')).toBe('2026-07-01 11:05 (Europe/Stockholm)')
    expect(zoneStamp('2026-11-22T09:05:03Z', 'UTC')).toBe('')
    expect(zoneStamp('2026-11-22T09:05:03Z', 'Mars/Base')).toBe('')
    expect(zoneStamp('bad', 'Europe/Stockholm')).toBe('')
    expect(browserTimeZone()).toBeTruthy()
  })

  it('labels meanings, errors and statuses', () => {
    const t = (k: string) => `[${k}]`
    expect(meaningLabel(t, 'approved')).toBe('[erecords.meaning.approved]')
    expect(meaningLabel(t, 'QA release')).toBe('QA release')
    expect(signErrorKey('reauth_failed')).toBe('esign.reauth_failed')
    expect(signErrorKey('mfa_required')).toBe('erecords.err.mfa_required')
    expect(signErrorKey('other')).toBe('')
    expect(controlSeverity('met')).toBe('success')
    expect(controlSeverity('attention')).toBe('danger')
    expect(controlSeverity('not_configured')).toBe('warn')
    expect(controlSeverity('procedural')).toBe('secondary')
    expect(short('abcdefabcdefabcdef')).toBe('abcdefabcdef')
    expect(short(undefined)).toBe('')
    expect(parseLines(' approved \n\nreviewed\napproved\n')).toEqual(['approved', 'reviewed'])
  })
})
