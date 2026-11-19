import { describe, expect, it } from 'vitest'
import { runnerAction, RUNNER_SHORTCUTS } from './shortcuts'

const key = (k: string, extra: Partial<KeyboardEvent> = {}, target: EventTarget | null = document.body) =>
  ({ key: k, target, ctrlKey: false, metaKey: false, altKey: false, ...extra }) as KeyboardEvent

describe('runnerAction', () => {
  it('maps P/F/B/S in either case', () => {
    expect(runnerAction(key('p'))).toEqual({ type: 'mark', status: 'passed' })
    expect(runnerAction(key('F'))).toEqual({ type: 'mark', status: 'failed' })
    expect(runnerAction(key('b'))).toEqual({ type: 'mark', status: 'blocked' })
    expect(runnerAction(key('s'))).toEqual({ type: 'mark', status: 'skipped' })
  })
  it('moves between steps and opens the help', () => {
    expect(runnerAction(key('ArrowDown'))).toEqual({ type: 'step', delta: 1 })
    expect(runnerAction(key('j'))).toEqual({ type: 'step', delta: 1 })
    expect(runnerAction(key('ArrowUp'))).toEqual({ type: 'step', delta: -1 })
    expect(runnerAction(key('K'))).toEqual({ type: 'step', delta: -1 })
    expect(runnerAction(key('?'))).toEqual({ type: 'help' })
    expect(runnerAction(key('x'))).toBeNull()
    expect(runnerAction(key('Enter'))).toBeNull()
  })
  it('never fires with modifiers, after preventDefault or while typing', () => {
    expect(runnerAction(key('p', { ctrlKey: true }))).toBeNull()
    expect(runnerAction(key('p', { metaKey: true }))).toBeNull()
    expect(runnerAction(key('p', { altKey: true }))).toBeNull()
    expect(runnerAction({ ...key('p'), defaultPrevented: true })).toBeNull()
    const input = document.createElement('input')
    const ta = document.createElement('textarea')
    const combo = document.createElement('span')
    combo.setAttribute('role', 'combobox')
    document.body.append(input, ta, combo)
    expect(runnerAction(key('p', {}, input))).toBeNull()
    expect(runnerAction(key('f', {}, ta))).toBeNull()
    expect(runnerAction(key('ArrowDown', {}, combo))).toBeNull()
  })
  it('documents every shortcut', () => {
    expect(RUNNER_SHORTCUTS.map((s) => s.action)).toEqual(['passed', 'failed', 'blocked', 'skipped', 'next', 'previous', 'help'])
    for (const s of RUNNER_SHORTCUTS) {
      const k: string = s.keys[0]
      expect(runnerAction(key(k))).not.toBeNull()
    }
  })
})
