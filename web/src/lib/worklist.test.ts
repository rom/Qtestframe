import { describe, expect, it } from 'vitest'
import { emptySummary, filterByReason, isWaiting, presentReasons, reasonSeverity, reviewsRoute, rowReviewId, workItemRoute, workKindIcon, workViewQuery, workViewState, type WorkItem } from './worklist'

const row = (over: Partial<WorkItem>): WorkItem => ({
  kind: 'requirement',
  id: 'r1',
  entity_type: 'requirement',
  entity_id: 'r1',
  key: 'P-REQ-1',
  title: 'T',
  status: 'draft',
  project_id: 'p1',
  project_key: 'P',
  project_name: 'Project',
  reasons: ['owner'],
  overdue: false,
  updated_at: '2026-01-01T00:00:00Z',
  sources: ['owned'],
  ...over,
})

describe('worklist', () => {
  it('routes each kind to its page', () => {
    expect(workItemRoute(row({}))).toEqual({ name: 'requirements', params: { projectId: 'p1' }, query: { item: 'r1' } })
    expect(workItemRoute(row({ entity_type: 'test', entity_id: 't1' }))).toEqual({ name: 'tests', params: { projectId: 'p1' }, query: { item: 't1' } })
    expect(workItemRoute(row({ entity_type: 'defect', entity_id: 'd' }))).toMatchObject({ name: 'defects', query: { item: 'd' } })
    expect(workItemRoute(row({ entity_type: 'risk', entity_id: 'k' }))).toMatchObject({ name: 'risks', query: { risk: 'k' } })
    expect(workItemRoute(row({ entity_type: 'waiver', entity_id: 'w' }))).toMatchObject({ name: 'waivers', query: { waiver: 'w' } })
    expect(workItemRoute(row({ entity_type: 'execution', entity_id: 'e', run_id: 'run' }))).toEqual({
      name: 'execute',
      params: { projectId: 'p1', runId: 'run', executionId: 'e' },
    })
    expect(workItemRoute(row({ entity_type: 'execution', entity_id: 'e' }))).toMatchObject({ name: 'runs' })
    expect(workItemRoute(row({ entity_type: 'session' }))).toMatchObject({ name: 'dashboard' })
  })
  it('styles reasons and kinds', () => {
    expect(reasonSeverity('overdue')).toBe('danger')
    expect(reasonSeverity('blocked')).toBe('danger')
    expect(reasonSeverity('review_requested')).toBe('warn')
    expect(reasonSeverity('assigned')).toBe('info')
    expect(reasonSeverity('owner')).toBe('secondary')
    expect(reasonSeverity('mention')).toBe('secondary')
    for (const k of ['requirement', 'test', 'defect', 'risk', 'waiver', 'execution', 'other']) expect(workKindIcon(k)).toMatch(/^pi pi-/)
  })
  it('filters and lists reasons', () => {
    const list = [row({}), row({ id: 'r2', reasons: ['owner', 'overdue'] })]
    expect(filterByReason(list, '')).toHaveLength(2)
    expect(filterByReason(list, 'overdue').map((r) => r.id)).toEqual(['r2'])
    const s = emptySummary()
    s.by_reason = { owner: 2, overdue: 1, mention: 3, stale: 0 }
    expect(presentReasons(s)).toEqual(['overdue', 'owner', 'mention'])
  })
  it('routes review requests to the reviews view of My work', () => {
    expect(workItemRoute(row({ entity_type: 'review', entity_id: 'rv' }))).toEqual({ name: 'myWork', query: { view: 'reviews', review: 'rv' } })
    expect(reviewsRoute()).toEqual({ name: 'myWork', query: { view: 'reviews' } })
    expect(reviewsRoute(undefined, 'requested')).toEqual({ name: 'myWork', query: { view: 'reviews', side: 'requested' } })
    expect(reviewsRoute('x', 'mine')).toEqual({ name: 'myWork', query: { view: 'reviews', review: 'x' } })
    expect(rowReviewId(row({}))).toBe('')
    expect(rowReviewId(row({ review_ids: ['a', 'b'] }))).toBe('a')
  })
  it('tells rows that only wait for others', () => {
    expect(isWaiting(row({ reasons: ['awaiting_review'] }))).toBe(true)
    expect(isWaiting(row({ reasons: ['in_review', 'awaiting_review'] }))).toBe(true)
    expect(isWaiting(row({ reasons: ['in_review', 'review_returned'] }))).toBe(false)
    expect(isWaiting(row({ reasons: [] }))).toBe(false)
    expect(reasonSeverity('review_returned')).toBe('warn')
    expect(reasonSeverity('in_review')).toBe('info')
    expect(reasonSeverity('awaiting_review')).toBe('secondary')
    const s = emptySummary()
    s.by_reason = { awaiting_review: 1, review_returned: 1, owner: 1 }
    expect(presentReasons(s)).toEqual(['review_returned', 'owner', 'awaiting_review'])
  })
  it('keeps the view in the query', () => {
    expect(workViewState({})).toEqual({ view: 'all', side: 'mine', status: 'pending' })
    expect(workViewState({ view: 'reviews', side: 'in_review', status: 'all' })).toEqual({ view: 'reviews', side: 'in_review', status: 'all' })
    expect(workViewState({ view: 'other', side: 'nope', status: 'x' })).toEqual({ view: 'all', side: 'mine', status: 'pending' })
    expect(workViewQuery({ view: 'all', side: 'requested', status: 'all' })).toEqual({ view: undefined, side: undefined, status: undefined })
    expect(workViewQuery({ view: 'reviews', side: 'mine', status: 'pending' })).toEqual({ view: 'reviews', side: undefined, status: undefined })
    expect(workViewQuery({ view: 'reviews', side: 'requested', status: 'all' })).toEqual({ view: 'reviews', side: 'requested', status: 'all' })
  })
})
