import { describe, expect, it } from 'vitest'
import { channelState, chatBody, chatFormProblems, deliverySeverity, eventOptionGroups, eventsText, validChatUrl, validMatrixRoom, type ChatEvent } from './chat'

const events: ChatEvent[] = [
  { event: 'run.completed', group: 'runs', labels: { en: 'Test run completed', sv: 'Testkörning avslutad' } },
  { event: 'defect.created', group: 'defects', labels: { en: 'Defect reported', sv: 'Defekt rapporterad' } },
  { event: 'cycle.started', group: 'mystery', labels: { en: 'Cycle started' } },
]
const groups = [
  { id: 'runs', labels: { en: 'Test runs', sv: 'Testkörningar' } },
  { id: 'defects', labels: { en: 'Defects', sv: 'Defekter' } },
  { id: 'other', labels: { en: 'Other', sv: 'Övrigt' } },
]

describe('chat events', () => {
  it('groups the registry events; unknown groups go last', () => {
    const g = eventOptionGroups(events, groups, 'sv')
    expect(g.map((x) => x.label)).toEqual(['Testkörningar', 'Defekter', 'Övrigt'])
    expect(g[0].items).toEqual([{ label: 'Testkörning avslutad', value: 'run.completed' }])
    // no Swedish label: English
    expect(g[2].items[0].label).toBe('Cycle started')
  })
  it('summarises the chosen events', () => {
    expect(eventsText(['*'], events, 'en', 'All')).toBe('All')
    expect(eventsText(['run.completed', 'defect.created', 'x.y'], events, 'en', 'All', 2)).toBe('Test run completed, Defect reported +1')
  })
})

describe('chat forms', () => {
  it('validates URLs and rooms', () => {
    expect(validChatUrl('https://hooks.example.test/x')).toBe(true)
    expect(validChatUrl('https://u:p@hooks.example.test/x')).toBe(false)
    expect(validChatUrl('ftp://x')).toBe(false)
    expect(validMatrixRoom('!abc:example.org')).toBe(true)
    expect(validMatrixRoom('#qa:example.org')).toBe(true)
    expect(validMatrixRoom('qa')).toBe(false)
  })
  it('requires the secret only when creating', () => {
    const base = { name: 'QA', provider: 'slack' as const, events: ['*'] }
    expect(chatFormProblems({ ...base, url: '' }, true)).toEqual({ url: true })
    expect(chatFormProblems({ ...base, url: '' }, false)).toEqual({})
    expect(chatFormProblems({ ...base, url: 'nope' }, false)).toEqual({ url: true })
    expect(chatFormProblems({ ...base, provider: 'matrix', homeserver: 'https://m.example.org', room: '!a:b' }, true)).toEqual({ token: true })
    expect(chatFormProblems({ provider: 'discord', url: 'https://x.test', events: [], digest_minutes: 2000 }, true)).toEqual({ name: true, events: true, digest: true })
  })
  it('leaves a blank secret out of the body', () => {
    expect(chatBody({ name: ' QA ', provider: 'slack', url: ' ', events: ['*'] })).not.toHaveProperty('url')
    expect(chatBody({ name: 'QA', provider: 'slack', url: 'https://h.test/x', events: ['*'] }).url).toBe('https://h.test/x')
    const m = chatBody({ name: 'M', provider: 'matrix', homeserver: ' https://m.test ', room: '!a:b', token: '', events: ['*'] })
    expect(m).toMatchObject({ homeserver: 'https://m.test', room: '!a:b' })
    expect(m).not.toHaveProperty('token')
  })
  it('names the channel state and delivery severity', () => {
    const c = { enabled: true, disabled_reason: '', last_delivery_at: null, last_status: 0, last_error: '' }
    expect(channelState(c)).toBe('never')
    expect(channelState({ ...c, last_delivery_at: 'x', last_status: 200 })).toBe('ok')
    expect(channelState({ ...c, last_delivery_at: 'x', last_status: 500, last_error: 'HTTP 500' })).toBe('failed')
    expect(channelState({ ...c, enabled: false })).toBe('off')
    expect(channelState({ ...c, enabled: false, disabled_reason: 'turned off' })).toBe('disabled')
    expect(deliverySeverity('sent')).toBe('success')
    expect(deliverySeverity('failed')).toBe('danger')
    expect(deliverySeverity('retry')).toBe('warn')
    expect(deliverySeverity('queued')).toBe('info')
  })
})
