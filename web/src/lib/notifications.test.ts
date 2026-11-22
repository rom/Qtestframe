import { describe, expect, it } from 'vitest'
import {
  badgeText,
  canApprove,
  canRequestChanges,
  checklistText,
  deliveryState,
  eventKey,
  isoDay,
  notificationIcon,
  notificationTarget,
  openChecklistItems,
  overdue,
  parseChecklist,
  reviewSeverity,
  shortHash,
  signoffsByMeaning,
  validWebhookUrl,
} from './notifications'
import type { Signoff } from './types'

describe('notificationTarget', () => {
  const n = (entity_type: string) => ({ project_id: 'p1', entity_type, entity_id: 'x' })
  it('leads to the item', () => {
    expect(notificationTarget(n('requirement'))).toEqual({ name: 'requirements', params: { projectId: 'p1' }, query: { item: 'x' } })
    expect(notificationTarget(n('cycle'))).toEqual({ name: 'cycle', params: { projectId: 'p1', cycleId: 'x' } })
    expect(notificationTarget(n('test'))?.name).toBe('tests')
    expect(notificationTarget(n('defect'))?.name).toBe('defects')
    expect(notificationTarget(n('waiver'))).toEqual({ name: 'waivers', params: { projectId: 'p1' }, query: { waiver: 'x' } })
    expect(notificationTarget(n('risk'))?.query).toEqual({ risk: 'x' })
    expect(notificationTarget(n('execution'))).toEqual({ name: 'runs', params: { projectId: 'p1' } })
    expect(notificationTarget(n('run'))).toEqual({ name: 'run', params: { projectId: 'p1', runId: 'x' }, query: { tab: 'discussion' } })
    expect(notificationTarget(n('review'))).toEqual({ name: 'myWork', query: { view: 'reviews', review: 'x' } })
    expect(notificationTarget(n('chat_channel'))).toEqual({ name: 'settings', params: { projectId: 'p1' } })
    expect(notificationTarget(n('other'))?.name).toBe('dashboard')
  })
  it('needs a project', () => {
    expect(notificationTarget({ project_id: '', entity_type: 'test', entity_id: 'x' })).toBeNull()
  })
})

describe('labels and icons', () => {
  it('icons per event', () => {
    expect(notificationIcon('mention')).toBe('pi pi-at')
    expect(notificationIcon('assigned')).toBe('pi pi-user')
    expect(notificationIcon('defect.assigned')).toBe('pi pi-user')
    expect(notificationIcon('review.requested')).toBe('pi pi-eye')
    expect(notificationIcon('waiver.requested')).toBe('pi pi-pen-to-square')
    expect(notificationIcon('waiver.approved')).toBe('pi pi-verified')
    expect(notificationIcon('waiver.expired')).toBe('pi pi-ban')
    expect(notificationIcon('review.completed')).toBe('pi pi-verified')
    expect(notificationIcon('defect.status')).toBe('pi pi-exclamation-circle')
    expect(notificationIcon('x')).toBe('pi pi-bell')
  })
  it('event keys and badges', () => {
    expect(eventKey('defect.status')).toBe('defect_status')
    expect(badgeText(0)).toBe('')
    expect(badgeText(-1)).toBe('')
    expect(badgeText(7)).toBe('7')
    expect(badgeText(120)).toBe('99+')
  })
  it('review severities', () => {
    expect(reviewSeverity('approved')).toBe('success')
    expect(reviewSeverity('changes_requested')).toBe('danger')
    expect(reviewSeverity('cancelled')).toBe('secondary')
    expect(reviewSeverity('pending')).toBe('info')
  })
})

describe('checklists', () => {
  it('approval needs every point checked or explained', () => {
    const list = [
      { text: 'a', checked: true },
      { text: 'b', checked: false, note: ' n/a ' },
      { text: 'c', checked: false },
    ]
    expect(openChecklistItems(list).map((c) => c.text)).toEqual(['c'])
    expect(canApprove(list)).toBe(false)
    expect(canApprove(list.slice(0, 2))).toBe(true)
    expect(canRequestChanges('  ')).toBe(false)
    expect(canRequestChanges('Fix it')).toBe(true)
  })
  it('text round trip', () => {
    const items = parseChecklist(' One \n\nTwo\n')
    expect(items).toEqual([
      { text: 'One', checked: false },
      { text: 'Two', checked: false },
    ])
    expect(checklistText(items)).toBe('One\nTwo')
  })
})

describe('dates', () => {
  it('overdue only when pending and past due', () => {
    expect(overdue({ status: 'pending', due_date: '2026-01-01' }, '2026-01-02')).toBe(true)
    expect(overdue({ status: 'pending', due_date: '2026-01-02' }, '2026-01-02')).toBe(false)
    expect(overdue({ status: 'approved', due_date: '2026-01-01' }, '2026-01-02')).toBe(false)
    expect(overdue({ status: 'pending', due_date: '' }, '2026-01-02')).toBe(false)
  })
  it('isoDay', () => {
    expect(isoDay(new Date(2026, 0, 5))).toBe('2026-01-05')
    expect(isoDay()).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})

describe('sign-offs', () => {
  const so = (meaning: string, id: string) => ({ id, meaning }) as Signoff
  it('groups by meaning in order', () => {
    const g = signoffsByMeaning([so('approved', '1'), so('authored', '2'), so('approved', '3'), so('odd', '4')])
    expect(g.map((x) => [x.meaning, x.items.length])).toEqual([
      ['authored', 1],
      ['approved', 2],
    ])
    expect(shortHash('abcdef0123456789')).toBe('abcdef012345')
  })
})

describe('webhooks', () => {
  it('validates URLs', () => {
    expect(validWebhookUrl('https://hooks.example/x')).toBe(true)
    expect(validWebhookUrl(' http://localhost:8080/h ')).toBe(true)
    expect(validWebhookUrl('ftp://x')).toBe(false)
    expect(validWebhookUrl('https://u:p@x.example')).toBe(false)
    expect(validWebhookUrl('nope')).toBe(false)
  })
  it('delivery state', () => {
    expect(deliveryState({ last_delivery_at: null, last_status: 0, last_error: '' })).toBe('never')
    expect(deliveryState({ last_delivery_at: 'x', last_status: 204, last_error: '' })).toBe('ok')
    expect(deliveryState({ last_delivery_at: 'x', last_status: 500, last_error: 'HTTP 500' })).toBe('failed')
    expect(deliveryState({ last_delivery_at: 'x', last_status: 0, last_error: '' })).toBe('failed')
  })
})
