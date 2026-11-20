// Undo stack: every change the user makes through the UI pushes an entry
// that knows how to revert it (undo) and to apply it again (redo). Undoing
// moves an entry to the redo list; a new change clears the redo list.

export interface UndoEntry {
  /** what was done, e.g. "Edit TC-12" (already translated) */
  label: string
  undo: () => Promise<void>
  redo: () => Promise<void>
}

export interface StackItem extends UndoEntry {
  id: number
  at: number
}

export const UNDO_LIMIT = 50

export class UndoStack {
  past: StackItem[] = []
  future: StackItem[] = []
  private seq = 0

  constructor(private limit = UNDO_LIMIT) {}

  push(e: UndoEntry, now = Date.now()): StackItem {
    const item = { ...e, id: ++this.seq, at: now }
    this.past.push(item)
    if (this.past.length > this.limit) this.past.splice(0, this.past.length - this.limit)
    this.future = []
    return item
  }

  get canUndo(): boolean {
    return this.past.length > 0
  }
  get canRedo(): boolean {
    return this.future.length > 0
  }

  /** Undo the newest change; on failure it stays on the stack. */
  async undo(): Promise<StackItem | null> {
    const item = this.past[this.past.length - 1]
    if (!item) return null
    await item.undo()
    this.past.pop()
    this.future.push(item)
    return item
  }

  /** Redo the newest undone change; on failure it stays on the redo list. */
  async redo(): Promise<StackItem | null> {
    const item = this.future[this.future.length - 1]
    if (!item) return null
    await item.redo()
    this.future.pop()
    this.past.push(item)
    return item
  }

  /** Undo changes up to and including the entry with id (newest first). */
  async undoTo(id: number): Promise<StackItem[]> {
    const done: StackItem[] = []
    if (!this.past.some((x) => x.id === id)) return done
    while (this.past.length) {
      const top = this.past[this.past.length - 1]
      const item = await this.undo()
      if (item) done.push(item)
      if (top.id === id) break
    }
    return done
  }

  clear(): void {
    this.past = []
    this.future = []
  }
}

/**
 * The values of `keys` in `before`, for a PATCH that puts them back. Keys
 * that are missing in `before` are left out.
 */
export function pickBefore<T extends object>(before: T, keys: Iterable<string>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  const src = before as Record<string, unknown>
  for (const k of keys) if (k in src) out[k] = clone(src[k])
  return out
}

/** Keys whose values differ between a and b (by JSON). */
export function changedKeys(a: Record<string, unknown>, b: Record<string, unknown>, keys: Iterable<string>): string[] {
  const out: string[] = []
  for (const k of keys) if (JSON.stringify(a[k] ?? null) !== JSON.stringify(b[k] ?? null)) out.push(k)
  return out
}

export function clone<T>(v: T): T {
  return v === undefined ? v : (JSON.parse(JSON.stringify(v)) as T)
}

/** True for Ctrl/Cmd+Z (undo); shift or Y means redo. */
export function undoKey(e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'altKey'>): 'undo' | 'redo' | null {
  if (!(e.ctrlKey || e.metaKey) || e.altKey) return null
  const k = e.key.toLowerCase()
  if (k === 'z') return e.shiftKey ? 'redo' : 'undo'
  if (k === 'y' && !e.shiftKey) return 'redo'
  return null
}
