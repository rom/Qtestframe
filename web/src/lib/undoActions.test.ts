import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const calls: string[] = []
vi.mock('./api', () => ({
  api: {
    patch: vi.fn(async (p: string, b: unknown) => void calls.push(`PATCH ${p} ${JSON.stringify(b)}`)),
    post: vi.fn(async (p: string, b?: unknown) => {
      calls.push(`POST ${p}${b ? ' ' + JSON.stringify(b) : ''}`)
      return { id: 'new' }
    }),
  },
}))

import { createdEntry, deletedEntry, entry, patchEntry, patchesEntry, recordUndo, toggleEntry } from './undoActions'
import { useUndoStore } from '@/stores/undo'

describe('undo actions', () => {
  beforeEach(() => {
    calls.length = 0
    setActivePinia(createPinia())
  })
  it('patches back and forth', async () => {
    const before = { title: 'a', tags: ['x'] }
    const e = patchEntry('Edit', '/tests/1', before, { title: 'b' })
    before.tags.push('y') // later changes do not leak in
    await e.undo()
    await e.redo()
    expect(calls).toEqual(['PATCH /tests/1 {"title":"a","tags":["x"]}', 'PATCH /tests/1 {"title":"b"}'])
    calls.length = 0
    const m = patchesEntry('Bulk', [{ path: '/tests/1', before: { status: 'draft' }, after: { status: 'ready' } }])
    await m.undo()
    await m.redo()
    expect(calls).toEqual(['PATCH /tests/1 {"status":"draft"}', 'PATCH /tests/1 {"status":"ready"}'])
  })
  it('restores deletions and deletes creations', async () => {
    let n = 0
    const remove = vi.fn(async () => `trash${++n}`)
    const d = deletedEntry('Delete', 'trash0', remove)
    await d.undo()
    await d.redo()
    await d.undo()
    expect(calls).toEqual(['POST /trash/trash0/restore', 'POST /trash/trash1/restore'])
    calls.length = 0
    const c = createdEntry('Create', remove)
    await c.undo()
    await c.redo()
    expect(calls).toEqual(['POST /trash/trash2/restore'])
  })
  it('toggles links', async () => {
    const log: string[] = []
    const create = async () => {
      log.push('create')
      return 'id2'
    }
    const remove = async (id: string) => void log.push('remove ' + id)
    const made = toggleEntry('Link', true, 'id1', create, remove)
    await made.undo()
    await made.redo()
    await made.undo()
    expect(log).toEqual(['remove id1', 'create', 'remove id2'])
    log.length = 0
    const gone = toggleEntry('Unlink', false, 'id1', create, remove)
    await gone.undo()
    await gone.redo()
    expect(log).toEqual(['create', 'remove id2'])
    const e = entry('Move', async () => log.push('back'), async () => log.push('again'))
    await e.undo()
    await e.redo()
    expect(log.slice(-2)).toEqual(['back', 'again'])
  })
  it('records into the store', () => {
    recordUndo(entry('x', async () => {}, async () => {}))
    expect(useUndoStore().history.map((h) => h.label)).toEqual(['x'])
  })
})
