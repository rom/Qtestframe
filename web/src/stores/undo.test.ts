import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUndoStore } from './undo'

describe('undo store', () => {
  beforeEach(() => setActivePinia(createPinia()))
  it('undoes, redoes and bumps the version', async () => {
    const s = useUndoStore()
    let v = 0
    s.record({ label: 'inc', undo: async () => void v--, redo: async () => void v++ })
    expect(s.canUndo).toBe(true)
    expect(s.next?.label).toBe('inc')
    await s.undo()
    expect(v).toBe(-1)
    expect(s.version).toBe(1)
    expect(s.nextRedo?.label).toBe('inc')
    await s.redo()
    expect(v).toBe(0)
    expect(s.history.map((h) => h.label)).toEqual(['inc'])
    const id = s.history[0].id
    await s.undoTo(id)
    expect(s.canUndo).toBe(false)
    expect(s.version).toBe(3)
  })
  it('clears when the scope changes', () => {
    const s = useUndoStore()
    s.bind('u1|t1')
    s.record({ label: 'x', undo: async () => {}, redo: async () => {} })
    s.bind('u1|t1')
    expect(s.canUndo).toBe(true)
    s.bind('u1|t2')
    expect(s.canUndo).toBe(false)
    s.record({ label: 'y', undo: async () => {}, redo: async () => {} })
    s.clear()
    expect(s.history).toEqual([])
  })
  it('stays busy-free after a failure', async () => {
    const s = useUndoStore()
    s.record({ label: 'bad', undo: async () => Promise.reject(new Error('x')), redo: async () => {} })
    await expect(s.undo()).rejects.toThrow('x')
    expect(s.busy).toBe(false)
    expect(s.canUndo).toBe(true)
  })
})
