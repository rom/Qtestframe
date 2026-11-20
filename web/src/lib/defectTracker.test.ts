import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import {
  canPush,
  canSync,
  compareDefects,
  countOpen,
  diffLinks,
  draftFromExecution,
  findImported,
  groupLinks,
  isDefectKey,
  isOpenDefect,
  issueKeyExample,
  keyNumber,
  linkBody,
  linkRoute,
  matchDefect,
  mergeLinks,
  normalizeIssueKey,
  severityRank,
  severityTag,
  sortDefects,
  statusRank,
  statusTag,
  syncResult,
  trackerDirty,
  trackerError,
  trackerForm,
  trackerPayload,
  validateTracker,
  validIssueKey,
  viaKind,
} from './defectTracker'

const d = (key: string, status: string, severity: string) => ({ key, status, severity })

describe('defect order', () => {
  it('knows open statuses', () => {
    expect(isOpenDefect('open')).toBe(true)
    expect(isOpenDefect('in_progress')).toBe(true)
    expect(isOpenDefect('resolved')).toBe(false)
    expect(isOpenDefect(null)).toBe(false)
    expect(countOpen([d('A', 'open', ''), d('B', 'closed', ''), d('C', 'in_progress', '')])).toBe(2)
  })
  it('ranks statuses and severities, unknown last', () => {
    expect(statusRank('open')).toBe(0)
    expect(statusRank('rejected')).toBe(4)
    expect(statusRank('odd')).toBe(5)
    expect(severityRank('critical')).toBe(0)
    expect(severityRank('trivial')).toBe(3)
    expect(severityRank(undefined)).toBe(4)
  })
  it('sorts open first, then by severity, then newest', () => {
    const list = [d('P-BUG-1', 'closed', 'critical'), d('P-BUG-2', 'open', 'minor'), d('P-BUG-3', 'open', 'critical'), d('P-BUG-4', 'open', 'minor')]
    expect(sortDefects(list).map((x) => x.key)).toEqual(['P-BUG-3', 'P-BUG-4', 'P-BUG-2', 'P-BUG-1'])
    expect(list[0].key).toBe('P-BUG-1')
    expect(compareDefects(list[0], list[0])).toBe(0)
    expect(keyNumber('X')).toBe(0)
  })
  it('colours tags', () => {
    expect(['open', 'in_progress', 'resolved', 'closed', 'rejected'].map(statusTag)).toEqual(['danger', 'warn', 'info', 'success', 'secondary'])
    expect(['critical', 'major', 'minor', 'trivial', ''].map(severityTag)).toEqual(['danger', 'warn', 'info', 'secondary', 'secondary'])
  })
})

describe('defect keys', () => {
  it('recognises keys', () => {
    expect(isDefectKey('SHOP-BUG-3')).toBe(true)
    expect(isDefectKey(' shop-bug-3 ')).toBe(true)
    expect(isDefectKey('SHOP-TC-3')).toBe(false)
    expect(isDefectKey('JIRA-123')).toBe(false)
    expect(isDefectKey(undefined)).toBe(false)
  })
  it('matches free-text references to defects', () => {
    const list = [{ key: 'SHOP-BUG-1', external_key: 'SHOP-77' }, { key: 'SHOP-BUG-2', external_key: '' }]
    expect(matchDefect('shop-bug-2', list)?.key).toBe('SHOP-BUG-2')
    expect(matchDefect('shop-77', list)?.key).toBe('SHOP-BUG-1')
    expect(matchDefect('', list)).toBeUndefined()
    expect(matchDefect('OTHER-1', [{ key: 'SHOP-BUG-9' }])).toBeUndefined()
  })
})

describe('defect links', () => {
  const r = { entity_type: 'requirement', entity_id: 'r1' }
  const t = { entity_type: 'test', entity_id: 't1' }
  const e = { entity_type: 'execution', entity_id: 'e1', key: 'P-TC-1' }
  it('merges and diffs', () => {
    expect(mergeLinks([r], [r, t])).toEqual([r, t])
    expect(diffLinks([r, t], [t, e])).toEqual({ add: [e], remove: [r] })
    expect(linkBody(e)).toEqual({ entity_type: 'execution', entity_id: 'e1' })
  })
  it('routes to linked items', () => {
    expect(linkRoute(r, 'p')).toEqual({ name: 'requirements', params: { projectId: 'p' }, query: { item: 'r1' } })
    expect(linkRoute(t, 'p')).toEqual({ name: 'tests', params: { projectId: 'p' }, query: { item: 't1' } })
    expect(linkRoute({ entity_type: 'run', entity_id: 'x' }, 'p')).toEqual({ name: 'run', params: { projectId: 'p', runId: 'x' } })
    expect(linkRoute(e, 'p')).toBeNull()
    expect(linkRoute(e, 'p', 'run1')).toEqual({ name: 'execute', params: { projectId: 'p', runId: 'run1', executionId: 'e1' } })
    expect(linkRoute({ entity_type: 'odd', entity_id: 'x' }, 'p')).toBeNull()
  })
  it('groups by type', () => {
    expect(groupLinks([e, r, t, { entity_type: 'run', entity_id: 'u' }]).map((g) => g.type)).toEqual(['requirement', 'test', 'execution', 'run'])
    expect(groupLinks(undefined)).toEqual([])
  })
  it('tells how a defect is linked', () => {
    expect(viaKind('requirement', 'requirement')).toBe('direct')
    expect(viaKind('', 'test')).toBe('direct')
    expect(viaKind('test', 'requirement')).toBe('test')
    expect(viaKind('execution', 'test')).toBe('execution')
  })
})

describe('reporting from an execution', () => {
  it('prefills title, description and the execution link', () => {
    const draft = draftFromExecution(
      { id: 'e1', test_key: 'P-TC-4', test_title: 'Login' },
      { notes: ' Seen on staging ', runName: 'Sprint 3', failedSteps: [{ n: 2, action: 'Click sign in', expected: 'Dashboard', actual: 'Error 500' }, { n: 3, action: 'Retry', expected: '', actual: '' }] },
    )
    expect(draft.title).toBe('P-TC-4 Login')
    expect(draft.severity).toBe('major')
    expect(draft.links).toEqual([{ entity_type: 'execution', entity_id: 'e1', key: 'P-TC-4', title: 'Login' }])
    expect(draft.description).toBe('Run: Sprint 3\n\nFailed steps:\n2. Click sign in\n   Expected: Dashboard\n   Actual: Error 500\n3. Retry\n\nNotes:\nSeen on staging')
  })
  it('uses translated labels and leaves out what is missing', () => {
    const labels = { steps: 'Steg', expected: 'Förväntat', actual: 'Faktiskt', notes: 'Anteckningar', run: 'Körning' }
    expect(draftFromExecution({ id: 'e', test_key: 'K', test_title: '' }, { labels }).description).toBe('')
    expect(draftFromExecution({ id: 'e', test_key: 'K', test_title: '' }, { labels, notes: 'x' }).description).toBe('Anteckningar:\nx')
    expect(draftFromExecution({ id: 'e', test_key: 'K', test_title: 'x'.repeat(400) }).title.length).toBe(300)
  })
})

describe('tracker settings', () => {
  const jira = { ...trackerForm(null), type: 'jira', url: 'https://acme.atlassian.net/', project_key: 'shop', user: ' qa@acme.test ', issue_type: '' }
  const gh = { ...trackerForm(null), type: 'github', repo: 'acme/shop', labels: 'bug ' }
  it('fills the form from saved settings', () => {
    expect(trackerForm({ type: 'github', repo: 'a/b', has_token: true })).toEqual({ type: 'github', url: '', project_key: '', repo: 'a/b', issue_type: '', labels: '', user: '', token: '' })
    expect(trackerForm(undefined).type).toBe('')
  })
  it('validates like the server', () => {
    expect(validateTracker(trackerForm(null))).toEqual({})
    expect(validateTracker(jira)).toEqual({})
    expect(validateTracker({ ...jira, url: 'acme.atlassian.net', project_key: '1X' })).toEqual({ url: 'jiraUrl', project_key: 'projectKey' })
    expect(validateTracker(gh)).toEqual({})
    expect(validateTracker({ ...gh, url: 'ftp://x', repo: 'shop' })).toEqual({ url: 'githubUrl', repo: 'repo' })
    expect(validateTracker({ ...gh, type: 'gitlab' })).toEqual({ type: 'type' })
  })
  it('sends only the fields of the type and a token when typed', () => {
    expect(trackerPayload(trackerForm(null))).toEqual({ type: '' })
    expect(trackerPayload(jira)).toEqual({ type: 'jira', url: 'https://acme.atlassian.net', project_key: 'SHOP', issue_type: '', user: 'qa@acme.test' })
    expect(trackerPayload({ ...gh, token: ' ghp_x ' })).toEqual({ type: 'github', url: '', repo: 'acme/shop', labels: 'bug', token: 'ghp_x' })
    expect(trackerPayload(gh, true)).toEqual({ type: 'github', url: '', repo: 'acme/shop', labels: 'bug', clear_token: true })
  })
  it('notices changes', () => {
    const saved = { type: 'github', url: 'https://api.github.com', repo: 'acme/shop', labels: '', has_token: true }
    expect(trackerDirty(trackerForm(saved), saved)).toBe(false)
    expect(trackerDirty({ ...trackerForm(saved), repo: 'acme/web' }, saved)).toBe(true)
    expect(trackerDirty({ ...trackerForm(saved), token: 'x' }, saved)).toBe(true)
  })
  it('explains issue keys', () => {
    expect(issueKeyExample({ type: 'jira', project_key: 'SHOP' })).toBe('SHOP-123')
    expect(issueKeyExample({ type: 'jira' })).toBe('SHOP-123')
    expect(issueKeyExample({ type: 'github', repo: 'a/b' })).toBe('a/b#123')
    expect(issueKeyExample({ type: 'github' })).toBe('owner/repo#123')
    expect(issueKeyExample(null)).toBe('')
    expect(validIssueKey({ type: 'jira' }, 'shop-12')).toBe(true)
    expect(validIssueKey({ type: 'jira' }, '#12')).toBe(false)
    expect(validIssueKey({ type: 'github' }, 'a/b#12')).toBe(true)
    expect(validIssueKey({ type: 'github' }, '#12')).toBe(true)
    expect(validIssueKey({ type: 'github' }, '12')).toBe(true)
    expect(validIssueKey({ type: 'github' }, 'abc')).toBe(false)
    expect(validIssueKey(null, '12')).toBe(false)
  })
  it('finds issues that were already imported', () => {
    const gh = { type: 'github', repo: 'Acme/Shop' }
    const list = [{ key: 'P-BUG-1', external_key: 'acme/shop#41' }, { key: 'P-BUG-2', external_key: '' }]
    expect(findImported(list, gh, '#41')?.key).toBe('P-BUG-1')
    expect(findImported(list, gh, '41')?.key).toBe('P-BUG-1')
    expect(findImported(list, gh, 'ACME/shop#41')?.key).toBe('P-BUG-1')
    expect(findImported(list, gh, '#42')).toBeUndefined()
    expect(normalizeIssueKey(gh, 'odd key')).toBe('odd key')
    expect(normalizeIssueKey({ type: 'github' }, '#7')).toBe('#7')
    expect(findImported([{ external_key: 'SHOP-7' }], { type: 'jira' }, ' shop-7 ')).toBeTruthy()
    expect(findImported([{ external_key: 'SHOP-7' }], null, 'SHOP-8')).toBeUndefined()
  })
  it('picks the tracker message out of 502 errors', () => {
    expect(trackerError(new ApiError(502, 'tracker_error', 'the tracker answered 401: Bad credentials'))).toBe('the tracker answered 401: Bad credentials')
    expect(trackerError(new ApiError(400, 'bad_request', 'x'))).toBeNull()
    expect(trackerError(new ApiError(502, 'internal', 'x'))).toBeNull()
    expect(trackerError(new Error('x'))).toBeNull()
  })
  it('normalises sync results', () => {
    expect(syncResult(null)).toEqual({ synced: 0, changed: 0, failed: [] })
    expect(syncResult({ synced: 2, changed: 1, failed: [{ key: 'K', error: 'e' }] })).toEqual({ synced: 2, changed: 1, failed: [{ key: 'K', error: 'e' }] })
  })
  it('knows when push and sync are possible', () => {
    expect(canPush({ external_key: '' })).toBe(true)
    expect(canPush({ external_key: 'A-1' })).toBe(false)
    expect(canSync({ external_key: 'A-1', external_system: 'jira' }, 'jira')).toBe(true)
    expect(canSync({ external_key: 'A-1', external_system: '' }, 'jira')).toBe(true)
    expect(canSync({ external_key: 'A-1', external_system: 'github' }, 'jira')).toBe(false)
    expect(canSync({ external_key: 'A-1', external_system: 'jira' }, '')).toBe(false)
    expect(canSync({ external_key: '', external_system: '' }, 'jira')).toBe(false)
  })
})
