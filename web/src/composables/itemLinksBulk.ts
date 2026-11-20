// API calls and undo entries for bulk item link changes.
import { api } from '@/lib/api'
import { chunks } from '@/lib/bulkLinks'
import { linkBody, type NewItemLink } from '@/lib/bulkItemLinks'
import type { ItemLink } from '@/lib/types'
import { entry } from '@/lib/undoActions'

export interface CreatedItemLinks {
  created: ItemLink[]
  skipped: { reason: string; message?: string }[]
}

export async function createItemLinks(pid: string, links: NewItemLink[]): Promise<CreatedItemLinks> {
  const out: CreatedItemLinks = { created: [], skipped: [] }
  for (const c of chunks(links)) {
    const res = await api.post<CreatedItemLinks>(`/projects/${pid}/item-links/bulk`, { links: c })
    out.created.push(...res.created)
    out.skipped.push(...res.skipped)
  }
  return out
}

export async function deleteItemLinks(pid: string, ids: string[]): Promise<ItemLink[]> {
  const out: ItemLink[] = []
  for (const c of chunks(ids)) out.push(...(await api.del<{ deleted: ItemLink[] }>(`/projects/${pid}/item-links/bulk`, { ids: c })).deleted)
  return out
}

/** Undo entry for links that were just created (existsNow) or deleted. */
export function itemLinksUndo(label: string, pid: string, links: ItemLink[], existsNow: boolean) {
  const bodies = links.map(linkBody)
  let ids = links.map((l) => l.id)
  const add = async () => (ids = (await createItemLinks(pid, bodies)).created.map((l) => l.id))
  const drop = async () => deleteItemLinks(pid, ids)
  return existsNow ? entry(label, drop, add) : entry(label, add, drop)
}
