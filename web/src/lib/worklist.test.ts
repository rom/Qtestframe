import { describe, expect, it } from 'vitest'
import { emptySummary, filterByReason, presentReasons, reasonSeverity, workItemRoute, workKindIcon, type WorkItem } from './worklist'

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
})
