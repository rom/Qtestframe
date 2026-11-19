// Keyboard shortcuts of the manual execution runner. Single keys only fire
// outside text fields and widgets (see isTypingTarget) and without modifiers.
import { isTypingTarget } from './a11y'

export type RunnerAction =
  | { type: 'mark'; status: 'passed' | 'failed' | 'blocked' | 'skipped' }
  | { type: 'step'; delta: 1 | -1 }
  | { type: 'help' }

/** Shortcut keys, documented in the runner's keyboard help (keys are the same in every language). */
export const RUNNER_SHORTCUTS = [
  { keys: ['P'], action: 'passed' },
  { keys: ['F'], action: 'failed' },
  { keys: ['B'], action: 'blocked' },
  { keys: ['S'], action: 'skipped' },
  { keys: ['J', '↓'], action: 'next' },
  { keys: ['K', '↑'], action: 'previous' },
  { keys: ['?'], action: 'help' },
] as const

const MARK: Record<string, 'passed' | 'failed' | 'blocked' | 'skipped'> = { p: 'passed', f: 'failed', b: 'blocked', s: 'skipped' }

type KeyLike = Pick<KeyboardEvent, 'key' | 'target' | 'ctrlKey' | 'metaKey' | 'altKey'> & { defaultPrevented?: boolean }

/** The runner action for a key press, or null when the key should be left alone. */
export function runnerAction(e: KeyLike): RunnerAction | null {
  if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey) return null
  if (isTypingTarget(e.target)) return null
  const k = e.key
  if (k === '?') return { type: 'help' }
  const lower = k.length === 1 ? k.toLowerCase() : k
  if (MARK[lower]) return { type: 'mark', status: MARK[lower] }
  if (k === 'ArrowDown' || lower === 'j') return { type: 'step', delta: 1 }
  if (k === 'ArrowUp' || lower === 'k') return { type: 'step', delta: -1 }
  return null
}
