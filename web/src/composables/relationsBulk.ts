// API calls and undo entries for bulk relation changes.
import { api } from '@/lib/api'
import { chunks } from '@/lib/bulkLinks'
import type { NewRelation } from '@/lib/bulkRelations'
import type { Relation } from '@/lib/types'
import { entry } from '@/lib/undoActions'

export interface CreatedRelations {
  created: Relation[]
  skipped: { reason: string }[]
}

export async function createRelations(pid: string, rels: NewRelation[]): Promise<CreatedRelations> {
  const out: CreatedRelations = { created: [], skipped: [] }
  for (const c of chunks(rels)) {
    const res = await api.post<CreatedRelations>(`/projects/${pid}/relations/bulk`, { relations: c })
    out.created.push(...res.created)
    out.skipped.push(...res.skipped)
  }
  return out
}

export async function deleteRelations(pid: string, ids: string[]): Promise<Relation[]> {
  const out: Relation[] = []
  for (const c of chunks(ids)) out.push(...(await api.del<{ deleted: Relation[] }>(`/projects/${pid}/relations/bulk`, { ids: c })).deleted)
  return out
}

/** Undo entry for relations that were just created (existsNow) or deleted. */
export function relationsUndo(label: string, pid: string, rels: Relation[], existsNow: boolean) {
  const body = rels.map((r) => ({ source_id: r.source_id, target_id: r.target_id, type: r.type }))
  let ids = rels.map((r) => r.id)
  const add = async () => (ids = (await createRelations(pid, body)).created.map((r) => r.id))
  const drop = async () => deleteRelations(pid, ids)
  return existsNow ? entry(label, drop, add) : entry(label, add, drop)
}
