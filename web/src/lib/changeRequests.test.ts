import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import {
  boardProgress,
  bulkProposal,
  changeBlocked,
  crActions,
  crDecisionBlock,
  crOpen,
  crStatusIcon,
  crStatusSeverity,
  crStepIndex,
  CR_STATUSES,
  filterCRs,
  itemDiffs,
  onBoard,
  parseEmails,
  proposalOf,
  type ChangeRequest,
} from './changeRequests'

function cr(over: Partial<ChangeRequest> = {}): ChangeRequest {
  return {
    id: 'c1', project_id: 'p', key: 'P-CR-1', title: 'Rename', reason: 'Clearer', category: 'correction', priority: 'medium', status: 'in_review',
    impact_action: 'suspect', impact: null, impact_notes: '', effort: '', assessed_by: '', assessed_at: null, links: { defect_ids: [], risk_ids: [], issues: [] },
    defects: [], risks: [], baseline_on_close: false, baseline_id: '', decided_by: '', decided_at: null, decision_comment: '', applied_by: '', applied_at: null,
    verified_by: '', verified_at: null, closed_by: '', closed_at: null, created_by: 'sara@x.org', created_at: '', updated_at: '',
    items: [{ id: 'i1', entity_type: 'requirement', entity_id: 'r1', key: 'P-REQ-1', title: 'Login', status: 'approved', base_version: 2, current_version: 2,
      proposed: { title: 'Sign in', tags: ['a', 'b'] }, current: { title: 'Log in', tags: ['a'] }, outdated: false, applied_version: 0 }],
    approvals: [], events: [], content_hash: 'h', outdated: false, ...over,
  }
}

describe('change requests', () => {
  it('labels states', () => {
    for (const s of CR_STATUSES) {
      expect(crStatusIcon(s)).toMatch(/^pi pi-/)
      expect(crStatusSeverity(s)).toBeTruthy()
    }
    expect(crOpen('deferred')).toBe(true)
    expect(crOpen('closed')).toBe(false)
    expect(crStepIndex('rejected')).toBe(crStepIndex('in_review'))
    expect(crStepIndex('closed')).toBe(7)
    expect(crStepIndex('nope')).toBe(0)
  })
  it('diffs proposed fields', () => {
    const d = itemDiffs(cr().items[0])
    expect(d.map((x) => x.field)).toEqual(['title', 'tags'])
    expect(d[0].segments?.some((s) => s.op === 'insert' && s.text.includes('Sign'))).toBe(true)
    expect(d[1]).toMatchObject({ before: 'a', after: 'a, b', segments: null })
  })
  it('builds a proposal from an edit', () => {
    expect(proposalOf('requirement', { title: 'A', tags: [], criteria: [] }, { title: 'B', tags: [], criteria: [{}], version: 3 })).toEqual({ title: 'B' })
    expect(proposalOf('test', { expected_result: '' }, { expected_result: 'ok', title: undefined })).toEqual({ expected_result: 'ok' })
    expect(bulkProposal({ status: 'review', priority: '', owner: null })).toEqual({ status: 'review' })
  })
  it('decides who may sign', () => {
    const b = { users: ['mgr@x.org'], roles: ['manager'], quorum: 'all' }
    expect(onBoard(b, 'MGR@x.org', [])).toBe(true)
    expect(onBoard(b, 'x@x.org', ['manager'])).toBe(true)
    expect(onBoard(b, 'x@x.org', ['tester'])).toBe(false)
    expect(onBoard({ users: [], roles: [], quorum: 'any' }, 'x@x.org', [])).toBe(true)
    expect(crDecisionBlock(cr({ status: 'draft' }), 'mgr@x.org', true)).toBe('not_in_review')
    expect(crDecisionBlock(cr(), 'sara@x.org', true)).toBe('own')
    expect(crDecisionBlock(cr(), 'mgr@x.org', false)).toBe('not_on_board')
    expect(crDecisionBlock(cr({ outdated: true }), 'mgr@x.org', true)).toBe('outdated')
    const signed = cr({ approvals: [{ id: 'a', user_id: 'u', user_email: 'mgr@x.org', user_name: '', decision: 'approve', meaning: '', comment: '', content_hash: 'h', signed_at: '', valid: true }] })
    expect(crDecisionBlock(signed, 'mgr@x.org', true)).toBe('already')
    expect(crDecisionBlock(cr(), 'mgr@x.org', true)).toBeNull()
    expect(boardProgress(b, signed)).toEqual({ approved: 1, needed: 2 })
    expect(boardProgress({ users: [], roles: [], quorum: '3' }, signed)).toEqual({ approved: 1, needed: 3 })
    expect(boardProgress({ users: [], roles: [], quorum: 'all' }, cr())).toEqual({ approved: 0, needed: 1 })
  })
  it('offers the actions of a state', () => {
    expect(crActions(cr({ status: 'draft' }), { create: true })).toContain('submit')
    expect(crActions(cr({ status: 'draft', items: [] }), { create: true })).not.toContain('submit')
    expect(crActions(cr({ status: 'approved' }), { create: true })).toEqual(['apply'])
    expect(crActions(cr({ status: 'verified' }), { create: true })).toEqual(['close'])
    expect(crActions(cr({ status: 'closed' }), { create: true })).toEqual([])
    expect(crActions(cr({ status: 'draft' }), { create: false })).toEqual([])
    for (const s of ['submitted', 'assessed', 'in_review', 'rejected', 'deferred', 'implemented']) expect(crActions(cr({ status: s }), { create: true }).length).toBeGreaterThan(0)
  })
  it('filters', () => {
    const list = [cr(), cr({ id: 'c2', key: 'P-CR-2', status: 'closed', category: 'regulatory', title: 'Policy' })]
    expect(filterCRs(list, { openOnly: true }).map((c) => c.id)).toEqual(['c1'])
    expect(filterCRs(list, { categories: ['regulatory'] }).map((c) => c.id)).toEqual(['c2'])
    expect(filterCRs(list, { text: 'p-req-1' }).length).toBe(2)
    expect(filterCRs(list, { text: 'policy', statuses: ['closed'] }).length).toBe(1)
  })
  it('reads a blocked save', () => {
    const e = new ApiError(409, 'change_control', 'x', undefined, { key: 'P-REQ-1', reasons: ['status approved'], open_change_requests: ['P-CR-1'] })
    expect(changeBlocked(e)).toEqual({ key: 'P-REQ-1', reasons: ['status approved'], open: ['P-CR-1'] })
    expect(changeBlocked(new ApiError(409, 'conflict', 'x'))).toBeNull()
    expect(changeBlocked(new Error('x'))).toBeNull()
    expect(changeBlocked(new ApiError(409, 'change_control', 'x'))).toEqual({ key: '', reasons: [], open: [] })
    expect(parseEmails('A@x.org, b@y.org;a@x.org')).toEqual(['a@x.org', 'b@y.org'])
  })
})
