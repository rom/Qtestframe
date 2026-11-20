// Building blocks for undo entries of the usual kinds of change.
import { api } from './api'
import { clone, type UndoEntry } from './undo'
import { useUndoStore } from '@/stores/undo'

/** Record an entry (no-op outside a pinia app, e.g. in some unit tests). */
export function recordUndo(e: UndoEntry): void {
  try {
    useUndoStore().record(e)
  } catch {
    /* no store */
  }
}

/** A PATCH of `path` from `before` to `after` (only the changed fields). */
export function patchEntry(label: string, path: string, before: Record<string, unknown>, after: Record<string, unknown>): UndoEntry {
  const b = clone(before)
  const a = clone(after)
  return {
    label,
    undo: async () => void (await api.patch(path, b)),
    redo: async () => void (await api.patch(path, a)),
  }
}

/** Several PATCHes undone and redone together (bulk edit). */
export function patchesEntry(label: string, changes: { path: string; before: Record<string, unknown>; after: Record<string, unknown> }[]): UndoEntry {
  const list = clone(changes)
  return {
    label,
    undo: async () => {
      for (const c of list) await api.patch(c.path, c.before)
    },
    redo: async () => {
      for (const c of list) await api.patch(c.path, c.after)
    },
  }
}

export function restoreTrash(id: string) {
  return api.post(`/trash/${id}/restore`)
}

/**
 * A deletion that went to the trash: undo restores it (same IDs and keys),
 * redo deletes again with `remove`, which returns the new trash entry.
 */
export function deletedEntry(label: string, trashId: string, remove: () => Promise<string>): UndoEntry {
  let id = trashId
  return {
    label,
    undo: async () => void (await restoreTrash(id)),
    redo: async () => {
      id = await remove()
    },
  }
}

/** Something just created: undo deletes it into the trash, redo restores it. */
export function createdEntry(label: string, remove: () => Promise<string>): UndoEntry {
  let id = ''
  return {
    label,
    undo: async () => {
      id = await remove()
    },
    redo: async () => void (await restoreTrash(id)),
  }
}

/**
 * Something that can be recreated (links, relations, dependencies, folders):
 * `create` returns the new ID, `remove` deletes by ID. `existsNow` is true
 * when the change created it, false when it removed it.
 */
export function toggleEntry(label: string, existsNow: boolean, id: string, create: () => Promise<string>, remove: (id: string) => Promise<void>): UndoEntry {
  let cur = id
  const make = async () => {
    cur = await create()
  }
  const drop = async () => {
    await remove(cur)
  }
  return existsNow ? { label, undo: drop, redo: make } : { label, undo: make, redo: drop }
}

/** Run two directions of a change (e.g. moves) as an entry. */
export function entry(label: string, undo: () => Promise<unknown>, redo: () => Promise<unknown>): UndoEntry {
  return { label, undo: async () => void (await undo()), redo: async () => void (await redo()) }
}
