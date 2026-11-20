import { describe, expect, it } from 'vitest'
import { BULK_RESULTS, execCounts, execUndo, rejectedText, runsBefore, valuesBefore } from './bulkRuns'

describe('runsBefore', () => {
  const runs = [
    { id: 'a', status: 'planned', environment: 'qa', build: '1', release_id: 'r1' },
    { id: 'b', status: 'completed', environment: '', build: '2', release_id: null },
  ]
  it('keeps the previous values of the set fields', () => {
    expect(runsBefore(runs, ['a', 'b', 'x'], { status: 'aborted', release_id: 'r2' })).toEqual([
      { id: 'a', before: { status: 'planned', release_id: 'r1' } },
      { id: 'b', before: { status: 'completed', release_id: '' } },
    ])
    expect(runsBefore(runs, ['a'], { environment: 'prod', build: '3' })).toEqual([{ id: 'a', before: { environment: 'qa', build: '1' } }])
  })
})

describe('executions', () => {
  const it1 = (id: string, status: string, assignee = '', reason?: string) => ({ id, key: id.toUpperCase(), title: id, status, assignee, reason })
  it('counts', () => {
    expect(execCounts({ updated: [it1('a', 'x')], skipped: [] })).toEqual({ updated: 1, rejected: 0, skipped: 0 })
    expect(execCounts({ removed: 2 })).toEqual({ updated: 0, rejected: 0, skipped: 0 })
    expect(BULK_RESULTS).toContain('not_run')
  })
  it('undo restores the fields that were set', () => {
    const notes = new Map([['a', 'old']])
    expect(execUndo([it1('a', 'not_run', 'anna'), it1('b', 'failed')], { status: 'passed', notes_append: 'x' }, notes)).toEqual([
      { id: 'a', body: { status: 'not_run', notes: 'old' } },
      { id: 'b', body: { status: 'failed', notes: '' } },
    ])
    expect(execUndo([it1('a', 'passed', 'anna')], { assignee: 'bo' }, notes)).toEqual([{ id: 'a', body: { assignee: 'anna' } }])
  })
  it('rejected text', () => {
    const r = [it1('a', 'x', '', 'r1'), it1('b', 'x', '', 'r2'), it1('c', 'x', '', 'r3'), it1('d', 'x', '', 'r4')]
    expect(rejectedText(r.slice(0, 2))).toBe('A: r1\nB: r2')
    expect(rejectedText(r)).toBe('A: r1\nB: r2\nC: r3\n…')
    expect(rejectedText([{ ...it1('e', 'x'), reason: undefined }])).toBe('E: ')
  })
})

describe('valuesBefore', () => {
  it('copies the chosen fields of the chosen rows', () => {
    const rows = [
      { id: 'a', status: 'open', severity: 'minor', title: 'x' },
      { id: 'b', status: 'closed', severity: 'major', title: 'y' },
    ]
    expect(valuesBefore(rows, ['b', 'z'], ['status', 'severity'])).toEqual([{ id: 'b', before: { status: 'closed', severity: 'major' } }])
  })
})
