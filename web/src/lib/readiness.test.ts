import { describe, expect, it } from 'vitest'
import { explainIssue, orList, readinessOfError, readinessSummary } from './readiness'
import type { ReadinessIssue } from './types'

// a fake translator: "key{params}" so the tests see which text and values are used
const t = (key: string, p?: Record<string, unknown>) => (key === 'readiness.orList' ? `${p!.list} or ${p!.last}` : `${key}${p ? JSON.stringify(p) : ''}`)
const label = (g: string, v: string) => `${g}:${v}`
const issue = (o: Partial<ReadinessIssue>): ReadinessIssue => ({ kind: 'test_status', id: 'i', key: 'K-1', title: 'T', status: 'draft', blocking: true, ...o })

describe('readiness explanations', () => {
  it('joins alternatives', () => {
    expect(orList([], t)).toBe('')
    expect(orList(['a'], t)).toBe('a')
    expect(orList(['a', 'b', 'c'], t)).toBe('a, b or c')
  })
  it('explains a test that is not ready', () => {
    const draft = explainIssue(issue({ required: ['ready'], setting: 'allow_unready_tests' }), t, label)
    expect(draft.reason).toBe('readiness.reasonTest{"key":"K-1","status":"status:draft","required":"status:ready"}')
    expect(draft.fix).toBe('readiness.fixTestDraft')
    expect(draft.rule).toBe('readiness.ruleBlocks{"setting":"execSettings.allowUnreadyTests"}')
    expect(explainIssue(issue({ status: 'review' }), t, label).fix).toBe('readiness.fixTestReview')
    const old = explainIssue(issue({ status: 'deprecated', blocking: false, setting: 'allow_unready_tests' }), t, label)
    expect(old.fix).toBe('readiness.fixTestOther')
    expect(old.rule).toContain('readiness.ruleAllows')
    expect(old.reason).toContain('status:ready') // default when the server sends no list
    expect(explainIssue(issue({ setting: 'unknown' }), t, label).rule).toBe('')
  })
  it('explains a requirement that is not ready for testing', () => {
    const req = (status: string, required = ['approved', 'implemented', 'verified']) =>
      explainIssue(issue({ kind: 'requirement_status', status, required, setting: 'allow_unready_requirements' }), t, label)
    const draft = req('draft')
    expect(draft.reason).toContain('"required":"status:approved, status:implemented or status:verified"')
    expect(draft.fix).toContain('readiness.fixRequirementApprove')
    expect(draft.rule).toContain('execSettings.allowUnreadyRequirements')
    expect(req('rejected').fix).toContain('readiness.fixRequirementDead')
    expect(req('approved', ['implemented', 'verified']).fix).toContain('readiness.fixRequirementOther')
    expect(req('review', ['implemented']).fix).toContain('readiness.fixRequirementOther')
  })
  it('explains a prerequisite that has not passed', () => {
    const inRun = explainIssue(issue({ kind: 'dependency', status: 'failed', in_run: true, setting: 'allow_unmet_dependencies' }), t, label)
    expect(inRun.reason).toBe('readiness.reasonDependencyInRun{"key":"K-1","result":"result:failed"}')
    expect(inRun.fix).toBe('readiness.fixDependency{"key":"K-1"}')
    expect(explainIssue(issue({ kind: 'dependency', status: '' }), t, label).reason).toContain('readiness.reasonDependency{"key":"K-1","result":"result:not_run"}')
  })
  it('summarises blocking reasons first', () => {
    expect(readinessSummary(null, t, label)).toBe('')
    const rd = { ready: false, allowed: false, issues: [issue({ blocking: false, status: 'review' }), issue({ kind: 'dependency', key: 'K-2', status: 'failed' })] }
    expect(readinessSummary(rd, t, label)).toBe('readiness.reasonDependency{"key":"K-2","result":"result:failed"}')
    rd.issues[1].blocking = false
    expect(readinessSummary(rd, t, label).split(' ').length).toBe(2)
  })
  it('reads the readiness of a 409 error', () => {
    const rd = { ready: false, allowed: false, issues: [] }
    expect(readinessOfError({ status: 409, code: 'not_ready', body: { readiness: rd } })).toBe(rd)
    expect(readinessOfError({ status: 409, code: 'conflict', body: {} })).toBeNull()
    expect(readinessOfError(null)).toBeNull()
    expect(readinessOfError({ status: 409, code: 'not_ready' })).toBeNull()
  })
})
