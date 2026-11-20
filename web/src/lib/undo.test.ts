import { describe, expect, it } from 'vitest'
import { changedKeys, pickBefore, undoKey, UndoStack } from './undo'

function counter() {
  const log: string[] = []
  const entry = (name: string, fail = { undo: false, redo: false }) => ({
    label: name,
    undo: async () => {
      if (fail.undo) throw new Error('nope')
      log.push('undo ' + name)
    },
    redo: async () => {
      if (fail.redo) throw new Error('nope')
      log.push('redo ' + name)
    },
  })
  return { log, entry }
}

describe('UndoStack', () => {
  it('undoes and redoes in order', async () => {
    const s = new UndoStack()
    const { log, entry } = counter()
    s.push(entry('a'))
    s.push(entry('b'))
    expect(s.canUndo).toBe(true)
    expect(s.canRedo).toBe(false)
    expect((await s.undo())?.label).toBe('b')
    expect((await s.undo())?.label).toBe('a')
    expect(await s.undo()).toBeNull()
    expect((await s.redo())?.label).toBe('a')
    expect(log).toEqual(['undo b', 'undo a', 'redo a'])
    expect(s.canRedo).toBe(true)
    s.push(entry('c')) // a new change drops the redo list
    expect(s.canRedo).toBe(false)
    expect(await s.redo()).toBeNull()
  })
  it('keeps a failed entry', async () => {
    const s = new UndoStack()
    const { entry } = counter()
    s.push(entry('x', { undo: true, redo: false }))
    await expect(s.undo()).rejects.toThrow('nope')
    expect(s.past).toHaveLength(1)
    s.clear()
    s.push(entry('y', { undo: false, redo: true }))
    await s.undo()
    await expect(s.redo()).rejects.toThrow()
    expect(s.future).toHaveLength(1)
  })
  it('is bounded', () => {
    const s = new UndoStack(3)
    const { entry } = counter()
    for (const n of ['1', '2', '3', '4', '5']) s.push(entry(n))
    expect(s.past.map((x) => x.label)).toEqual(['3', '4', '5'])
  })
  it('undoes several at once', async () => {
    const s = new UndoStack()
    const { log, entry } = counter()
    const a = s.push(entry('a'))
    s.push(entry('b'))
    s.push(entry('c'))
    expect(await s.undoTo(999)).toEqual([])
    const done = await s.undoTo(a.id)
    expect(done.map((x) => x.label)).toEqual(['c', 'b', 'a'])
    expect(log).toEqual(['undo c', 'undo b', 'undo a'])
    expect(s.future.map((x) => x.label)).toEqual(['c', 'b', 'a'])
  })
})

describe('helpers', () => {
  it('picks old values', () => {
    const before = { title: 'A', tags: ['x'], status: 'draft' }
    const p = pickBefore(before, ['title', 'tags', 'missing'])
    expect(p).toEqual({ title: 'A', tags: ['x'] })
    ;(p.tags as string[]).push('y')
    expect(before.tags).toEqual(['x'])
  })
  it('finds changed keys', () => {
    expect(changedKeys({ a: 1, b: [1], c: null }, { a: 1, b: [2], c: undefined }, ['a', 'b', 'c'])).toEqual(['b'])
  })
  it('maps keys', () => {
    const k = (key: string, o: Partial<KeyboardEvent> = {}) => undoKey({ key, ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, ...o })
    expect(k('z', { ctrlKey: true })).toBe('undo')
    expect(k('Z', { metaKey: true, shiftKey: true })).toBe('redo')
    expect(k('y', { ctrlKey: true })).toBe('redo')
    expect(k('z')).toBeNull()
    expect(k('z', { ctrlKey: true, altKey: true })).toBeNull()
    expect(k('a', { ctrlKey: true })).toBeNull()
  })
})
