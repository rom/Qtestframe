import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { DEFAULT_REQUIREMENT_LABELS } from './defectTracker'
import { bulkIssueResult, existingIssue, issueStateTag, issueSyncResult, previewLabels } from './requirementIssues'
import type { ItemLink } from './types'

const link = (id: string, tracker?: string, direction = 'outgoing') =>
  ({ id, direction, target_type: 'url', url: 'https://x/' + id, issue: tracker ? { tracker, key: 'g/p#' + id, status: 'opened', state: 'open', synced_at: null } : undefined }) as unknown as ItemLink

describe('requirement issues', () => {
  it('colours the state', () => {
    expect(issueStateTag('open')).toBe('info')
    expect(issueStateTag('closed')).toBe('success')
    expect(issueStateTag('')).toBe('secondary')
    expect(issueStateTag(undefined)).toBe('secondary')
  })
  it('reads the existing issue of a conflict', () => {
    const l = link('9', 'gitlab')
    expect(existingIssue(new ApiError(409, 'issue_exists', 'x', undefined, { link: l }))).toBe(l)
    expect(existingIssue(new ApiError(409, 'issue_exists', 'x'))).toBeNull()
    expect(existingIssue(new ApiError(409, 'conflict', 'x', undefined, { link: l }))).toBeNull()
    expect(existingIssue(new Error('x'))).toBeNull()
  })
  it('normalises bulk answers', () => {
    expect(bulkIssueResult(null)).toEqual({ created: [], updated: [], skipped: [], failed: [] })
    expect(bulkIssueResult({ created: [{ key: 'A', issue_key: 'g/p#1', url: 'u' }] }).created).toHaveLength(1)
    expect(issueSyncResult(undefined)).toEqual({ synced: 0, changed: 0, commented: 0, failed: [] })
    expect(issueSyncResult({ synced: 2, failed: [{ key: 'A', error: 'e' }] })).toEqual({ synced: 2, changed: 0, commented: 0, failed: [{ key: 'A', error: 'e' }] })
  })
  it('maps labels like the server', () => {
    const req = { type: 'functional', priority: 'high', obligation: '', status: 'draft', tags: ['a', 'b,c', ' '] }
    const cases: [string, string][] = [
      ['', 'requirement,type::functional,priority::high,a,b c'],
      ['none', ''],
      ['NONE', ''],
      ['{obligation}, x', 'x'],
      ['tag::{tags}, {status}', 'tag::a,tag::b c,draft'],
      ['{type}/{priority}, , X', 'functional/high,X'],
      ['a, A', 'a'],
    ]
    for (const [tmpl, want] of cases) expect(previewLabels(tmpl, req, DEFAULT_REQUIREMENT_LABELS).join(','), tmpl).toBe(want)
    expect(previewLabels('{tags}', {}, '')).toEqual([])
  })
})
