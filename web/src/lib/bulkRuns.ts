// Bulk operations on test runs and on the executions of a run.

/** Run fields a bulk update can set. */
export interface RunBulkSet {
  status?: string
  release_id?: string
  environment?: string
  build?: string
}

/** The values of the fields in set before the change, per run (for undo). */
export function runsBefore(runs: { id: string; status: string; environment: string; build: string; release_id?: string | null }[], ids: string[], set: RunBulkSet): { id: string; before: RunBulkSet }[] {
  const byId = new Map(runs.map((r) => [r.id, r]))
  return ids.flatMap((id) => {
    const r = byId.get(id)
    if (!r) return []
    const before: RunBulkSet = {}
    if (set.status !== undefined) before.status = r.status
    if (set.environment !== undefined) before.environment = r.environment
    if (set.build !== undefined) before.build = r.build
    if (set.release_id !== undefined) before.release_id = r.release_id ?? ''
    return [{ id, before }]
  })
}

/** Results a bulk update can set on executions. */
export const BULK_RESULTS = ['passed', 'failed', 'blocked', 'skipped', 'not_run'] as const

export interface ExecBulkItem {
  id: string
  key: string
  title: string
  status: string
  assignee: string
  reason?: string
}

export interface ExecBulkResult {
  updated?: ExecBulkItem[]
  rejected?: ExecBulkItem[]
  skipped?: ExecBulkItem[]
  removed?: number
}

/** Counts of a bulk execution result. */
export function execCounts(res: ExecBulkResult): { updated: number; rejected: number; skipped: number } {
  return { updated: res.updated?.length ?? 0, rejected: res.rejected?.length ?? 0, skipped: res.skipped?.length ?? 0 }
}

/**
 * How to undo a bulk execution update: one PATCH per updated execution
 * with its previous values of the fields that were set. Notes are restored
 * from the list the caller had before the change.
 */
export function execUndo(updated: ExecBulkItem[], set: { status?: string; assignee?: string; notes_append?: string }, notesBefore: Map<string, string>): { id: string; body: Record<string, string> }[] {
  return updated.map((u) => {
    const body: Record<string, string> = {}
    if (set.status !== undefined) body.status = u.status
    if (set.assignee !== undefined) body.assignee = u.assignee
    if (set.notes_append) body.notes = notesBefore.get(u.id) ?? ''
    return { id: u.id, body }
  })
}

/** A short text listing the first rejected executions and why. */
export function rejectedText(items: ExecBulkItem[], max = 3): string {
  const head = items.slice(0, max).map((i) => `${i.key}: ${i.reason ?? ''}`)
  return items.length > max ? `${head.join('\n')}\n…` : head.join('\n')
}
