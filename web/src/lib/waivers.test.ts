import { describe, expect, it } from 'vitest'
import { approvalProgress, daysToExpiry, decisionBlock, expiresSoon, filterWaivers, parseApprovers, waiverEditable, waiverStatusIcon, waiverStatusSeverity, WAIVER_STATUSES, type Waiver } from './waivers'

const w = (over: Partial<Waiver> = {}): Waiver => ({
  id: 'w1',
  project_id: 'p1',
  key: 'P-WVR-1',
  type: 'waiver',
  title: 'Ship without rotation',
  release_id: null,
  requirement_ids: ['r1'],
  requirements: [{ id: 'r1', key: 'P-REQ-1', title: 'Rotate keys' }],
  justification: 'HSM late',
  compensating: 'manual',
  conditions: '',
  risk_id: null,
  requested_by: 'ann@example.com',
  approvers: [],
  status: 'requested',
  expires_on: '',
  decided_at: null,
  decided_by: '',
  decision_comment: '',
  created_by: 'ann@example.com',
  created_at: '',
  updated_at: '',
  approvals: [],
  active: false,
  content_hash: 'abc',
  ...over,
})

describe('waiver states', () => {
  it('has a severity and an icon for every state', () => {
    for (const s of WAIVER_STATUSES) {
      expect(waiverStatusSeverity(s)).toBeTruthy()
      expect(waiverStatusIcon(s)).toMatch(/^pi pi-/)
    }
    expect(waiverStatusSeverity('approved')).toBe('success')
    expect(waiverStatusSeverity('draft')).toBe('secondary')
    expect(waiverEditable({ status: 'draft' })).toBe(true)
    expect(waiverEditable({ status: 'approved' })).toBe(false)
  })
})

describe('filterWaivers', () => {
  const list = [w(), w({ id: 'w2', key: 'P-WVR-2', type: 'deviation', release_id: 'rel1', status: 'approved', active: true, title: 'Other KMS' })]
  it('filters by text, status, type, scope and in force', () => {
    expect(filterWaivers(list, {}).length).toBe(2)
    expect(filterWaivers(list, { text: 'kms' }).map((x) => x.id)).toEqual(['w2'])
    expect(filterWaivers(list, { text: 'p-req-1' }).length).toBe(2)
    expect(filterWaivers(list, { statuses: ['requested'] }).map((x) => x.id)).toEqual(['w1'])
    expect(filterWaivers(list, { types: ['deviation'] }).map((x) => x.id)).toEqual(['w2'])
    expect(filterWaivers(list, { release: 'project' }).map((x) => x.id)).toEqual(['w1'])
    expect(filterWaivers(list, { release: 'rel1' }).map((x) => x.id)).toEqual(['w2'])
    expect(filterWaivers(list, { activeOnly: true }).map((x) => x.id)).toEqual(['w2'])
  })
})

describe('expiry', () => {
  it('counts days and spots waivers that expire soon', () => {
    expect(daysToExpiry('2026-12-01', '2026-11-22')).toBe(9)
    expect(daysToExpiry('2026-11-01', '2026-11-22')).toBe(-21)
    expect(daysToExpiry('', '2026-11-22')).toBeNull()
    expect(daysToExpiry('bogus', '2026-11-22')).toBeNull()
    expect(expiresSoon({ status: 'approved', expires_on: '2026-12-01' }, '2026-11-22')).toBe(true)
    expect(expiresSoon({ status: 'approved', expires_on: '2027-12-01' }, '2026-11-22')).toBe(false)
    expect(expiresSoon({ status: 'requested', expires_on: '2026-12-01' }, '2026-11-22')).toBe(false)
  })
})

describe('decisions', () => {
  it('mirrors who may sign', () => {
    expect(decisionBlock(w({ status: 'draft' }), 'bob@example.com')).toBe('not_requested')
    expect(decisionBlock(w(), 'ANN@example.com')).toBe('own')
    expect(decisionBlock(w(), undefined)).toBe('own')
    expect(decisionBlock(w({ approvers: ['carl@example.com'] }), 'bob@example.com')).toBe('not_approver')
    expect(decisionBlock(w(), 'bob@example.com')).toBeNull()
    const signed = w({ approvals: [{ id: 'a', waiver_id: 'w1', user_id: 'u', user_email: 'bob@example.com', user_name: 'Bob', decision: 'approve', comment: '', content_hash: 'abc', signed_at: '', valid: true }] })
    expect(decisionBlock(signed, 'bob@example.com')).toBe('already')
    expect(decisionBlock({ ...signed, approvals: signed.approvals.map((a) => ({ ...a, valid: false })) }, 'bob@example.com')).toBeNull()
  })
  it('shows the approval progress', () => {
    const a = (email: string, valid = true, decision = 'approve') => ({ id: email, waiver_id: 'w1', user_id: email, user_email: email, user_name: '', decision, comment: '', content_hash: 'x', signed_at: '', valid })
    expect(approvalProgress(w())).toEqual({ approved: 0, needed: 1 })
    expect(approvalProgress(w({ approvals: [a('x@e.com'), a('y@e.com')] }))).toEqual({ approved: 1, needed: 1 })
    expect(approvalProgress(w({ approvers: ['x@e.com', 'y@e.com'], approvals: [a('x@e.com'), a('y@e.com', false), a('z@e.com')] }))).toEqual({ approved: 1, needed: 2 })
  })
  it('parses approvers', () => {
    expect(parseApprovers(' Ann@x.com, bob@x.com;ann@x.com\ncarl@x.com ')).toEqual(['ann@x.com', 'bob@x.com', 'carl@x.com'])
    expect(parseApprovers('')).toEqual([])
  })
})
