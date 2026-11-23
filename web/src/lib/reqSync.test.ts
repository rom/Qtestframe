import { describe, expect, it } from 'vitest'
import {
  fullyChosen,
  hookAddress,
  mapText,
  nextField,
  parseMapText,
  randomSecret,
  resolveBody,
  runSeverity,
  summaryParts,
  syncForm,
  syncPayload,
  validateSync,
  type ReqSyncConfig,
  type ReqSyncConflict,
} from './reqSync'

const config: ReqSyncConfig = {
  id: 'c1',
  system: 'jira',
  enabled: true,
  interval_minutes: 15,
  settings: {
    fields: [
      { field: 'title', remote: 'summary', direction: 'both' },
      { field: 'status', remote: 'status', direction: 'pull', values: { 'To Do': 'draft', Done: 'implemented' }, push_values: { draft: 'To Do' } },
    ],
    query: 'project = SHOP',
    import_new: true,
    folder_id: 'f1',
    push: { create: true, folder_id: '', tag: 'jira', query: '', item_type: 'Story', project_key: 'SHOP' },
    conflict_policy: 'tracker',
    impact: 'notify',
    comments: true,
    links: false,
    max_writes_per_minute: 30,
  },
  has_webhook_secret: true,
  webhook_url: 'https://qtf.example/api/v1/hooks/req-sync/jira/p1',
  run_as: 'lead@example.com',
  next_run_at: null,
  last_run_at: null,
  last_status: '',
  last_error: '',
  failures: 0,
  open_conflicts: 0,
  linked: 2,
  version: 3,
  updated_by: '',
  updated_at: '',
  connected: true,
}

describe('reqSync', () => {
  it('maps value tables to text and back', () => {
    expect(mapText({ b: '2', a: '1' })).toBe('a = 1\nb = 2')
    expect(mapText(undefined)).toBe('')
    expect(parseMapText('In Progress = approved\n\nnoise\n a=b=c \n = x')).toEqual({ 'In Progress': 'approved', 'a=b': 'c' })
  })

  it('round-trips a configuration through the form', () => {
    const f = syncForm(config)
    expect(f.fields[1].values).toBe('Done = implemented\nTo Do = draft')
    expect(f.version).toBe(3)
    const body = syncPayload(f) as { settings: Record<string, unknown>; version: number; webhook_secret?: string }
    expect(body.version).toBe(3)
    expect(body.webhook_secret).toBeUndefined()
    expect(body.settings).toMatchObject({ query: 'project = SHOP', conflict_policy: 'tracker', impact: 'notify', comments: true, max_writes_per_minute: 30 })
    const fields = body.settings.fields as Array<Record<string, unknown>>
    expect(fields[0]).toEqual({ field: 'title', remote: 'summary', direction: 'both' })
    expect(fields[1].values).toEqual({ 'To Do': 'draft', Done: 'implemented' })
    f.secret = '  0123456789abcdef  '
    expect(syncPayload(f).webhook_secret).toBe('0123456789abcdef')
    expect(syncPayload(f, true)).toMatchObject({ clear_webhook_secret: true })
    expect(syncPayload(f, true).webhook_secret).toBeUndefined()
  })

  it('has defaults without a configuration', () => {
    const f = syncForm(null)
    expect(f).toMatchObject({ enabled: false, interval: 0, policy: 'manual', impact: 'suspect', maxWrites: 60, fields: [] })
  })

  it('validates like the server', () => {
    const f = syncForm(config)
    expect(validateSync(f)).toEqual({})
    f.interval = 3
    f.maxWrites = 0
    f.pushTag = ''
    f.secret = 'short'
    f.fields.push({ field: 'title', remote: ' ', direction: 'both', values: '', pushValues: '' })
    expect(validateSync(f)).toEqual({ interval: 'interval', maxWrites: 'maxWrites', pushScope: 'pushScope', secret: 'secret', remote2: 'remote', field2: 'duplicate' })
  })

  it('offers the next unmapped field', () => {
    const f = syncForm(config)
    expect(nextField(f)).toBe('description')
    f.fields = ['title', 'description', 'status', 'priority', 'tags', 'criteria', 'rationale'].map((x) => ({ field: x as never, remote: x, direction: 'both', values: '', pushValues: '' }))
    expect(nextField(f)).toBeNull()
  })

  it('makes secrets and hook addresses', () => {
    const s = randomSecret()
    expect(s).toMatch(/^[0-9a-f]{48}$/)
    expect(randomSecret()).not.toBe(s)
    expect(hookAddress(config, true)).toBe(config.webhook_url + '?secret=…')
    expect(hookAddress({ ...config, system: 'azure-devops' }, true)).toBe(config.webhook_url)
  })

  it('summarises runs', () => {
    expect(runSeverity('ok')).toBe('success')
    expect(runSeverity('partial')).toBe('warn')
    expect(runSeverity('error')).toBe('danger')
    expect(runSeverity('running')).toBe('info')
    const s = { pulled: 2, pushed: 0, created_local: 0, created_remote: 1, conflicts: 0, comments: 0, links: 0, unchanged: 4, errors: 1 }
    expect(summaryParts(s)).toEqual([
      ['pulled', 2],
      ['created_remote', 1],
      ['errors', 1],
      ['unchanged', 4],
    ])
    expect(summaryParts(null)).toEqual([])
  })

  it('builds conflict decisions', () => {
    const c = { id: 'x', fields: [{ field: 'title' }, { field: 'status' }] } as unknown as ReqSyncConflict
    expect(fullyChosen(c, { title: 'tracker' })).toBe(false)
    expect(fullyChosen(c, { title: 'tracker', status: 'dismiss' })).toBe(true)
    expect(resolveBody(c, { title: 'tracker', other: 'x' })).toEqual({ fields: { title: 'tracker' } })
    expect(resolveBody(c, {}, 'qtestframe')).toEqual({ resolution: 'qtestframe' })
    expect(fullyChosen({ ...c, fields: [] }, {})).toBe(false)
  })
})
