import { describe, expect, it } from 'vitest'
import {
  canTakeNotes,
  debriefBody,
  elapsedNow,
  filterSessions,
  formatClock,
  isSaveKey,
  noteDefectTitle,
  noteIcon,
  noteSeverity,
  outcomeSeverity,
  scopeGroups,
  sessionAction,
  splitTotal,
  statusIcon,
  statusSeverity,
  suggestSplit,
  timeboxUse,
  timerActions,
  type ExploratorySession,
} from './exploratory'

const session = (over: Partial<ExploratorySession> = {}): ExploratorySession => ({
  id: 'x1',
  project_id: 'p',
  key: 'P-EXP-1',
  title: 'Explore checkout',
  mission: 'with expired cards',
  tester: 'ann@example.com',
  timebox_minutes: 60,
  status: 'planned',
  started_at: null,
  running_since: null,
  elapsed_seconds: 0,
  ended_at: null,
  scope: [],
  debrief: null,
  version: 1,
  created_by: 'ann@example.com',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  note_count: 0,
  note_types: {},
  bug_count: 0,
  ...over,
})

const key = (k: string, extra: Partial<KeyboardEvent> = {}) => ({ key: k, target: document.body, ctrlKey: false, metaKey: false, altKey: false, ...extra }) as KeyboardEvent

describe('exploratory', () => {
  it('formats the session clock', () => {
    expect(formatClock(0)).toBe('0:00')
    expect(formatClock(65)).toBe('1:05')
    expect(formatClock(3725)).toBe('1:02:05')
    expect(formatClock(-3)).toBe('0:00')
  })

  it('adds the running part to the server time', () => {
    const s = session({ status: 'in_progress', running_since: '2026-01-01T00:00:00Z', elapsed_seconds: 100 })
    expect(elapsedNow(s, 1000, 6500)).toBe(105)
    expect(elapsedNow({ ...s, status: 'paused', running_since: null }, 1000, 60000)).toBe(100)
    expect(timeboxUse(1800, 60)).toEqual({ ratio: 0.5, over: false })
    expect(timeboxUse(3700, 60).over).toBe(true)
  })

  it('knows the timer actions and when notes are taken', () => {
    expect(timerActions('planned')).toEqual({ start: true, pause: false, stop: false })
    expect(timerActions('in_progress')).toEqual({ start: false, pause: true, stop: true })
    expect(timerActions('paused')).toEqual({ start: true, pause: false, stop: true })
    expect(timerActions('debriefed')).toEqual({ start: false, pause: false, stop: false })
    expect(canTakeNotes('planned')).toBe(false)
    expect(canTakeNotes('completed')).toBe(true)
    expect(canTakeNotes('debriefed')).toBe(false)
  })

  it('maps keys to runner actions outside text fields', () => {
    expect(sessionAction(key('q'))).toEqual({ type: 'note', noteType: 'question' })
    expect(sessionAction(key('B'))).toEqual({ type: 'note', noteType: 'bug' })
    expect(sessionAction(key('n'))).toEqual({ type: 'note', noteType: null })
    expect(sessionAction(key('t'))).toEqual({ type: 'timer' })
    expect(sessionAction(key('?'))).toEqual({ type: 'help' })
    expect(sessionAction(key('x'))).toBeNull()
    expect(sessionAction(key('q', { ctrlKey: true }))).toBeNull()
    const ta = document.createElement('textarea')
    expect(sessionAction(key('q', { target: ta }))).toBeNull()
    expect(isSaveKey(key('Enter', { ctrlKey: true }))).toBe(true)
    expect(isSaveKey(key('Enter', { metaKey: true }))).toBe(true)
    expect(isSaveKey(key('Enter'))).toBe(false)
  })

  it('checks and suggests the time split', () => {
    expect(splitTotal({ setup_pct: 10, testing_pct: 60, bug_pct: 30 })).toBe(100)
    expect(splitTotal({ setup_pct: null, testing_pct: 60, bug_pct: null })).toBe(60)
    expect(suggestSplit({}, 0)).toEqual({ setup_pct: 10, testing_pct: 90, bug_pct: 0 })
    const s = suggestSplit({ bug: 2, issue: 1, observation: 3 }, 6)
    expect(s.setup_pct + s.testing_pct + s.bug_pct).toBe(100)
    expect(s.bug_pct).toBe(30)
    expect(debriefBody({ summary: ' ok ', coverage_pct: 120, setup_pct: 10.4, testing_pct: 80, bug_pct: null, outcome: 'passed', comment: '' })).toEqual({
      summary: 'ok',
      coverage_pct: 100,
      setup_pct: 10,
      testing_pct: 80,
      bug_pct: 0,
      outcome: 'passed',
      comment: '',
    })
  })

  it('prefills a defect title and filters sessions', () => {
    expect(noteDefectTitle('  First line\nmore')).toBe('First line')
    expect(noteDefectTitle('x'.repeat(250)).length).toBe(201)
    const a = session({
      scope: [
        { type: 'folder', id: 'f1', title: 'Checkout' },
        { type: 'requirement', id: 'r1', key: 'P-REQ-1', title: 'Pay' },
      ],
    })
    const b = session({ id: 'x2', key: 'P-EXP-2', title: 'Refunds', tester: '', status: 'debriefed' })
    expect(filterSessions([a, b], { text: 'p-req-1' })).toEqual([a])
    expect(filterSessions([a, b], { statuses: ['debriefed'] })).toEqual([b])
    expect(filterSessions([a, b], { area: 'f1' })).toEqual([a])
    expect(filterSessions([a, b], { tester: 'ann@example.com' })).toEqual([a])
    expect(scopeGroups(a.scope).map((g) => g.type)).toEqual(['folder', 'requirement'])
  })

  it('gives tags and icons', () => {
    expect(statusSeverity('debriefed')).toBe('success')
    expect(statusSeverity('paused')).toBe('warn')
    expect(statusIcon('in_progress')).toBe('pi pi-play')
    expect(statusIcon('planned')).toBe('pi pi-calendar')
    expect(outcomeSeverity('failed')).toBe('danger')
    expect(outcomeSeverity('inconclusive')).toBe('warn')
    expect(noteIcon('bug')).toBe('pi pi-bolt')
    expect(noteSeverity('idea')).toBe('success')
    expect(noteSeverity('observation')).toBe('secondary')
  })
})
