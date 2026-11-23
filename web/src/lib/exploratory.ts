// Exploratory testing, session based (mirrors internal/model
// ExploratorySession): states, the timer, note types and their keyboard
// shortcuts, the debrief's time split, filters.
import { isTypingTarget } from './a11y'
import type { Severity } from './colors'
import type { DefectLink } from './types'

/** A file attached to the session that a note refers to. */
export interface FileRef {
  id: string
  name: string
  kind: string
}

export const EXPLORATORY_STATUSES = ['planned', 'in_progress', 'paused', 'completed', 'debriefed'] as const
export type ExploratoryStatus = (typeof EXPLORATORY_STATUSES)[number]
export const NOTE_TYPES = ['observation', 'question', 'issue', 'idea', 'bug'] as const
export type NoteType = (typeof NOTE_TYPES)[number]
export const OUTCOMES = ['passed', 'failed', 'inconclusive'] as const
export const TIMEBOXES = [30, 60, 90] as const
export const SCOPE_TYPES = ['folder', 'requirement', 'test'] as const

export interface ScopeItem {
  /** folder (an area) | requirement | test */
  type: string
  id: string
  key?: string
  title?: string
}

export interface Debrief {
  summary: string
  coverage_pct: number
  setup_pct: number
  testing_pct: number
  bug_pct: number
  /** passed | failed | inconclusive */
  outcome: string
  by: string
  name: string
  comment: string
  at: string
  /** debriefed by someone other than the tester */
  reviewed: boolean
}

export interface ExploratorySession {
  id: string
  project_id: string
  key: string
  title: string
  mission: string
  tester: string
  timebox_minutes: number
  status: ExploratoryStatus | string
  /** the environment explored */
  environment_id?: string | null
  environment_name?: string
  started_at: string | null
  running_since: string | null
  /** at the time the server answered (the running part included) */
  elapsed_seconds: number
  ended_at: string | null
  scope: ScopeItem[]
  debrief: Debrief | null
  version: number
  created_by: string
  created_at: string
  updated_at: string
  note_count: number
  note_types: Record<string, number>
  bug_count: number
  defects?: DefectLink[]
}

export interface ExploratoryNote {
  id: string
  session_id: string
  type: NoteType | string
  body: string
  elapsed_seconds: number
  file_ids: string[]
  files: FileRef[]
  defect_id: string | null
  defect_key?: string
  created_by: string
  created_at: string
  updated_at: string
}

export interface ExploratoryGroup {
  id: string
  name: string
  sessions: number
  hours: number
  bugs: number
  bugs_per_hour: number
}

export interface ExploratoryMetrics {
  sessions: number
  by_status: Record<string, number>
  by_outcome: Record<string, number>
  hours: number
  bugs: number
  bugs_per_hour: number
  avg_setup_pct: number
  avg_testing_pct: number
  avg_bug_pct: number
  avg_coverage_pct: number
  by_area: ExploratoryGroup[]
  by_tester: ExploratoryGroup[]
}

/** h:mm:ss, or m:ss under an hour. */
export function formatClock(sec: number): string {
  const s = Math.max(0, Math.floor(sec || 0))
  const h = Math.floor(s / 3600)
  const m = Math.floor(s / 60) % 60
  const two = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${two(m)}:${two(s % 60)}` : `${m}:${two(s % 60)}`
}

/**
 * The session time now: the server's elapsed_seconds plus the time since it
 * answered while the timer runs (receivedAt: when the answer arrived, ms).
 */
export function elapsedNow(x: Pick<ExploratorySession, 'elapsed_seconds' | 'running_since' | 'status'>, receivedAt: number, now: number): number {
  const base = x.elapsed_seconds || 0
  if (!x.running_since || x.status !== 'in_progress') return base
  return base + Math.max(0, Math.floor((now - receivedAt) / 1000))
}

/** Share of the time box used (0–1+), and whether it is over. */
export function timeboxUse(elapsed: number, minutes: number): { ratio: number; over: boolean } {
  const box = Math.max(1, minutes || 0) * 60
  return { ratio: elapsed / box, over: elapsed > box }
}

export function statusSeverity(s: string): Severity {
  switch (s) {
    case 'in_progress':
      return 'info'
    case 'paused':
      return 'warn'
    case 'completed':
      return 'secondary'
    case 'debriefed':
      return 'success'
  }
  return 'secondary'
}

export function statusIcon(s: string): string {
  switch (s) {
    case 'in_progress':
      return 'pi pi-play'
    case 'paused':
      return 'pi pi-pause'
    case 'completed':
      return 'pi pi-stop'
    case 'debriefed':
      return 'pi pi-check-circle'
  }
  return 'pi pi-calendar'
}

export function outcomeSeverity(o: string | undefined): Severity {
  return o === 'passed' ? 'success' : o === 'failed' ? 'danger' : 'warn'
}

export function noteIcon(t: string): string {
  switch (t) {
    case 'question':
      return 'pi pi-question-circle'
    case 'issue':
      return 'pi pi-exclamation-triangle'
    case 'idea':
      return 'pi pi-lightbulb'
    case 'bug':
      return 'pi pi-bolt'
  }
  return 'pi pi-eye'
}

export function noteSeverity(t: string): Severity {
  switch (t) {
    case 'question':
      return 'info'
    case 'issue':
      return 'warn'
    case 'idea':
      return 'success'
    case 'bug':
      return 'danger'
  }
  return 'secondary'
}

/** Timer actions the status allows. */
export function timerActions(status: string): {
  start: boolean
  pause: boolean
  stop: boolean
} {
  return {
    start: status === 'planned' || status === 'paused',
    pause: status === 'in_progress',
    stop: status === 'in_progress' || status === 'paused',
  }
}

/** Notes can be taken from the start until the debrief. */
export function canTakeNotes(status: string): boolean {
  return status === 'in_progress' || status === 'paused' || status === 'completed'
}

/** Keyboard shortcuts of the session runner (the same in every language). */
export const NOTE_KEYS: Record<string, NoteType> = {
  o: 'observation',
  q: 'question',
  i: 'issue',
  d: 'idea',
  b: 'bug',
}
export const SESSION_SHORTCUTS = [
  { keys: ['O'], action: 'observation' },
  { keys: ['Q'], action: 'question' },
  { keys: ['I'], action: 'issue' },
  { keys: ['D'], action: 'idea' },
  { keys: ['B'], action: 'bug' },
  { keys: ['N'], action: 'note' },
  { keys: ['T'], action: 'timer' },
  { keys: ['Ctrl', 'Enter'], action: 'save' },
  { keys: ['Esc'], action: 'leave' },
  { keys: ['?'], action: 'help' },
] as const

export type SessionAction = { type: 'note'; noteType: NoteType | null } | { type: 'timer' } | { type: 'help' }

type KeyLike = Pick<KeyboardEvent, 'key' | 'target' | 'ctrlKey' | 'metaKey' | 'altKey'> & { defaultPrevented?: boolean }

/** The runner action of a key pressed outside text fields, or null. */
export function sessionAction(e: KeyLike): SessionAction | null {
  if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey) return null
  if (isTypingTarget(e.target)) return null
  const k = e.key
  if (k === '?') return { type: 'help' }
  const lower = k.length === 1 ? k.toLowerCase() : ''
  if (NOTE_KEYS[lower]) return { type: 'note', noteType: NOTE_KEYS[lower] }
  if (lower === 'n') return { type: 'note', noteType: null }
  if (lower === 't') return { type: 'timer' }
  return null
}

/** Ctrl/Cmd+Enter in the note field saves it. */
export function isSaveKey(e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey'>): boolean {
  return e.key === 'Enter' && (e.ctrlKey || e.metaKey)
}

/** The debrief's time split adds up to 100. */
export function splitTotal(d: { setup_pct: number | null; testing_pct: number | null; bug_pct: number | null }): number {
  return (d.setup_pct ?? 0) + (d.testing_pct ?? 0) + (d.bug_pct ?? 0)
}

/**
 * A time split suggested from the notes: bug investigation grows with the
 * bug and issue notes, set-up starts at 10 % (rounded to 5).
 */
export function suggestSplit(noteTypes: Record<string, number> | undefined, notes: number): { setup_pct: number; testing_pct: number; bug_pct: number } {
  const bugs = (noteTypes?.bug ?? 0) + (noteTypes?.issue ?? 0)
  const bug = notes > 0 ? Math.min(50, Math.round(((bugs / notes) * 60) / 5) * 5) : 0
  const setup = 10
  return { setup_pct: setup, testing_pct: 100 - setup - bug, bug_pct: bug }
}

/** Default title of a defect raised from a note: its first line. */
export function noteDefectTitle(body: string): string {
  const first = (body ?? '').trim().split('\n')[0].trim()
  return first.length > 200 ? first.slice(0, 200) + '…' : first
}

export interface SessionFilter {
  text?: string
  statuses?: string[]
  tester?: string
  area?: string
}

/** Client-side filter of the session list. */
export function filterSessions<T extends ExploratorySession>(list: readonly T[], f: SessionFilter): T[] {
  const text = (f.text ?? '').trim().toLowerCase()
  return list.filter((x) => {
    if (f.statuses?.length && !f.statuses.includes(x.status)) return false
    if (f.tester && x.tester !== f.tester) return false
    if (f.area && !x.scope.some((s) => s.type === 'folder' && s.id === f.area)) return false
    if (!text) return true
    return [x.key, x.title, x.mission, x.tester, ...x.scope.map((s) => `${s.key ?? ''} ${s.title ?? ''}`)].some((v) => (v ?? '').toLowerCase().includes(text))
  })
}

/** Areas, requirements and tests of a scope, grouped in that order. */
export function scopeGroups(scope: readonly ScopeItem[]): { type: string; items: ScopeItem[] }[] {
  return SCOPE_TYPES.map((type) => ({
    type,
    items: scope.filter((s) => s.type === type),
  })).filter((g) => g.items.length)
}

/** Body of PUT …/debrief from the form (numbers clamped to 0–100). */
export function debriefBody(f: { summary: string; coverage_pct: number | null; setup_pct: number | null; testing_pct: number | null; bug_pct: number | null; outcome: string; comment: string }) {
  const c = (n: number | null) => Math.max(0, Math.min(100, Math.round(n ?? 0)))
  return {
    summary: f.summary.trim(),
    coverage_pct: c(f.coverage_pct),
    setup_pct: c(f.setup_pct),
    testing_pct: c(f.testing_pct),
    bug_pct: c(f.bug_pct),
    outcome: f.outcome,
    comment: f.comment.trim(),
  }
}
