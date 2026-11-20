// The user's undo/redo history (see lib/undo.ts). It belongs to one
// organisation: switching organisation or signing out clears it. Views that
// show undoable data watch `version` and reload after an undo or redo.
import { defineStore } from 'pinia'
import { UndoStack, type StackItem, type UndoEntry } from '@/lib/undo'

export const useUndoStore = defineStore('undo', {
  state: () => ({
    stack: new UndoStack(),
    busy: false,
    /** bumped after every undo/redo */
    version: 0,
    scope: '' as string,
  }),
  getters: {
    canUndo: (s) => s.stack.past.length > 0 && !s.busy,
    canRedo: (s) => s.stack.future.length > 0 && !s.busy,
    next: (s): StackItem | null => s.stack.past[s.stack.past.length - 1] ?? null,
    nextRedo: (s): StackItem | null => s.stack.future[s.stack.future.length - 1] ?? null,
    /** newest first */
    history: (s): StackItem[] => [...s.stack.past].reverse(),
  },
  actions: {
    /** Record a change that was just made. */
    record(e: UndoEntry) {
      this.stack.push(e)
    },
    /** Clear the history when the organisation changes. */
    bind(scope: string) {
      if (scope !== this.scope) {
        this.stack.clear()
        this.scope = scope
      }
    },
    clear() {
      this.stack.clear()
    },
    async run<T>(fn: () => Promise<T>): Promise<T> {
      this.busy = true
      try {
        return await fn()
      } finally {
        this.busy = false
        this.version++
      }
    },
    undo() {
      return this.run(() => this.stack.undo())
    },
    redo() {
      return this.run(() => this.stack.redo())
    },
    undoTo(id: number) {
      return this.run(() => this.stack.undoTo(id))
    },
  },
})
