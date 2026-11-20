import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import {
  EMAIL_MODES,
  FREQUENCIES,
  REPORT_MAIL_PROBLEMS,
  SUBSCRIPTION_PROBLEMS,
  WEEKDAYS,
  blankSubscription,
  emailMode,
  invitationState,
  isEmail,
  parseAddresses,
  reportMailBody,
  reportMailProblems,
  subscriptionBody,
  subscriptionForm,
  subscriptionProblems,
  subscriptionStatus,
  timeOptions,
  weekdayName,
} from './email'
import type { ReportSubscription } from './types'

function flat(o: Record<string, unknown>, prefix = ''): Set<string> {
  const out = new Set<string>()
  for (const [k, v] of Object.entries(o)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object') for (const x of flat(v as Record<string, unknown>, key)) out.add(x)
    else out.add(key)
  }
  return out
}
const EN = flat(en as unknown as Record<string, unknown>)

describe('dynamic e-mail keys', () => {
  it('exist in English', () => {
    const keys = [
      ...EMAIL_MODES.map((m) => `email.mode.${m}`),
      ...EMAIL_MODES.map((m) => `email.modeHint.${m}`),
      ...FREQUENCIES.map((f) => `email.subs.frequency.${f}`),
      ...FREQUENCIES.map((f) => `email.subs.every.${f}`),
      ...SUBSCRIPTION_PROBLEMS.map((p) => `email.subs.problem.${p}`),
      ...REPORT_MAIL_PROBLEMS.map((p) => `email.send.problem.${p}`),
      ...['error', 'sent', 'paused', 'waiting'].map((s) => `email.subs.status.${s}`),
      ...['no_access', 'disabled'].map((r) => `email.send.skippedReason.${r}`),
    ]
    expect(keys.filter((k) => !EN.has(k))).toEqual([])
  })
})

describe('email helpers', () => {
  it('normalises the mode', () => {
    expect(emailMode('digest')).toBe('digest')
    expect(emailMode('')).toBe('off')
    expect(emailMode(undefined)).toBe('off')
    expect(emailMode('weekly')).toBe('off')
  })

  it('tells the invitation state', () => {
    const now = new Date('2026-11-20T10:00:00Z')
    const inv = { id: 'i', sent_at: '2026-11-19T10:00:00Z', expires_at: '2026-11-26T10:00:00Z', sso: false, expired: false, invited_by: 'a' }
    expect(invitationState(undefined, now)).toBe('none')
    expect(invitationState(null)).toBe('none')
    expect(invitationState(inv, now)).toBe('pending')
    expect(invitationState({ ...inv, expired: true }, now)).toBe('expired')
    expect(invitationState({ ...inv, expires_at: '2026-11-20T09:00:00Z' }, now)).toBe('expired')
    expect(invitationState({ ...inv, expires_at: 'bad' }, now)).toBe('pending')
  })

  it('parses addresses', () => {
    expect(isEmail('a@b.se')).toBe(true)
    expect(isEmail('a@b')).toBe(false)
    expect(isEmail('A <a@b.se>')).toBe(false)
    expect(isEmail('x'.repeat(250) + '@b.se')).toBe(false)
    expect(parseAddresses(' A@B.se, c@d.org;\nnope a@b.se  bad@ ')).toEqual({ valid: ['a@b.se', 'c@d.org'], invalid: ['nope', 'bad@'] })
    expect(parseAddresses('')).toEqual({ valid: [], invalid: [] })
  })

  it('checks and builds a report e-mail', () => {
    const f = { recipients: [] as string[], addresses: '', message: '', format: 'pdf' }
    expect(reportMailProblems(f, true)).toEqual(['recipients'])
    expect(reportMailProblems({ ...f, addresses: 'x@y.se' }, true)).toEqual([])
    expect(reportMailProblems({ ...f, addresses: 'x@y.se' }, false)).toEqual(['recipients'])
    expect(reportMailProblems({ ...f, recipients: ['u'], addresses: 'bad', message: 'x'.repeat(4001) }, true)).toEqual(['addresses', 'message'])
    expect(reportMailBody({ template: 'traceability', params: { query: 'a' } }, { ...f, recipients: ['u', 'u'], addresses: 'x@y.se', message: ' hi ' }, true)).toEqual({
      template: 'traceability',
      params: { query: 'a' },
      format: 'pdf',
      recipients: ['u'],
      addresses: ['x@y.se'],
      message: 'hi',
    })
    expect(reportMailBody({ definition_id: 'd' }, { ...f, recipients: ['u'], addresses: 'x@y.se' }, false)).toEqual({ definition_id: 'd', format: 'pdf', recipients: ['u'] })
  })

  it('handles subscription forms', () => {
    const b = blankSubscription('d1', 'Europe/Stockholm')
    expect(b).toMatchObject({ definition_id: 'd1', frequency: 'weekly', weekday: 1, time: '07:00', timezone: 'Europe/Stockholm', enabled: true })
    expect(blankSubscription().timezone).toBe('UTC')
    expect(blankSubscription('', '').timezone).toBe('UTC')
    expect(subscriptionProblems(b, false)).toEqual(['recipients'])
    expect(subscriptionProblems({ ...b, recipients: ['u'] }, false)).toEqual([])
    expect(subscriptionProblems({ ...b, definition_id: '', time: '7:00', frequency: 'monthly', day_of_month: 31, addresses: 'x@y.se', message: 'x'.repeat(4001) }, false)).toEqual([
      'definition',
      'time',
      'day',
      'recipients',
      'addresses',
      'message',
    ])
    expect(subscriptionProblems({ ...b, addresses: 'x@y.se' }, true)).toEqual([])
    expect(subscriptionProblems({ ...b, recipients: ['u'], addresses: 'bad' }, true)).toEqual(['addresses'])
    const body = subscriptionBody({ ...b, recipients: ['u', 'u'], addresses: 'X@y.se', message: ' m ' }, true)
    expect(body).toMatchObject({ definition_id: 'd1', recipients: ['u'], addresses: ['x@y.se'], message: 'm' })
    expect('definition_id' in subscriptionBody(b, false)).toBe(false)
    const sub: ReportSubscription = {
      id: 's',
      project_id: 'p',
      definition_id: 'd',
      owner_id: 'o',
      frequency: 'monthly',
      weekday: 3,
      day_of_month: 5,
      time: '06:30',
      timezone: 'UTC',
      format: 'csv',
      recipients: ['u'],
      addresses: ['a@b.se', 'c@d.se'],
      message: 'hi',
      enabled: false,
      next_run_at: null,
      last_run_at: null,
      last_sent_at: null,
      last_error: '',
      created_at: '',
      updated_at: '',
    }
    expect(subscriptionForm(sub)).toMatchObject({ frequency: 'monthly', day_of_month: 5, addresses: 'a@b.se, c@d.se', enabled: false })
    expect(subscriptionForm({ ...sub, frequency: 'hourly', time: '', timezone: '', format: '', day_of_month: 0, recipients: undefined as unknown as string[], addresses: undefined as unknown as string[], message: undefined as unknown as string })).toMatchObject({
      frequency: 'weekly',
      time: '07:00',
      timezone: 'UTC',
      format: 'pdf',
      day_of_month: 1,
      recipients: [],
      addresses: '',
      message: '',
    })
    expect(subscriptionForm({ ...sub, weekday: undefined as unknown as number }).weekday).toBe(1)
    expect(subscriptionStatus(sub)).toBe('paused')
    expect(subscriptionStatus({ ...sub, enabled: true })).toBe('waiting')
    expect(subscriptionStatus({ ...sub, enabled: true, last_sent_at: 'x' })).toBe('sent')
    expect(subscriptionStatus({ ...sub, last_error: 'x' })).toBe('error')
  })

  it('names weekdays and times', () => {
    expect(WEEKDAYS[0]).toBe(1)
    expect(weekdayName(0, 'en')).toBe('Sunday')
    expect(weekdayName(1, 'sv')).toBe('måndag')
    expect(weekdayName(8, 'en', 'short')).toBe('Mon')
    expect(weekdayName(-1, 'en')).toBe('Saturday')
    const opts = timeOptions()
    expect(opts.length).toBe(48)
    expect(opts[0]).toBe('00:00')
    expect(opts[47]).toBe('23:30')
    expect(timeOptions(60)[7]).toBe('07:00')
  })
})
