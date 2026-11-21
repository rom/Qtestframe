// Folder hierarchy helpers: the API returns a flat list with parent_id.
import type { Folder } from './types'

export interface FolderNode {
  key: string
  label: string
  data: Folder
  icon?: string
  children: FolderNode[]
  leaf?: boolean
}

function byPosition(a: Folder, b: Folder): number {
  return (a.position ?? 0) - (b.position ?? 0) || a.name.localeCompare(b.name)
}

/**
 * Build a tree from flat folders. Folders whose parent is unknown (or which
 * would form a cycle) are attached to the root.
 */
export function buildTree(folders: readonly Folder[] | null | undefined): FolderNode[] {
  const list = [...(folders ?? [])].sort(byPosition)
  const nodes = new Map<string, FolderNode>()
  for (const f of list) nodes.set(f.id, { key: f.id, label: f.name, data: f, icon: 'pi pi-folder', children: [] })
  const roots: FolderNode[] = []
  for (const f of list) {
    const node = nodes.get(f.id) as FolderNode
    const parent = f.parent_id ? nodes.get(f.parent_id) : undefined
    if (parent && !createsCycle(folders ?? [], f.id)) parent.children.push(node)
    else roots.push(node)
  }
  for (const n of nodes.values()) n.leaf = n.children.length === 0
  return roots
}

function createsCycle(folders: readonly Folder[], id: string): boolean {
  const byId = new Map(folders.map((f) => [f.id, f]))
  const seen = new Set<string>()
  let cur = byId.get(id)
  while (cur?.parent_id) {
    if (seen.has(cur.id)) return true
    seen.add(cur.id)
    if (cur.parent_id === id) return true
    cur = byId.get(cur.parent_id)
  }
  return false
}

/**
 * Effective colour of every folder: its own colour, else the nearest
 * coloured ancestor's ('' without one). Items take their folder's colour.
 */
export function folderColors(folders: readonly Folder[] | null | undefined): Map<string, string> {
  const byId = new Map((folders ?? []).map((f) => [f.id, f]))
  const out = new Map<string, string>()
  for (const f of folders ?? []) {
    let cur: Folder | undefined = f
    const seen = new Set<string>()
    while (cur && !cur.color && cur.parent_id && !seen.has(cur.id)) {
      seen.add(cur.id)
      cur = byId.get(cur.parent_id)
    }
    out.set(f.id, cur?.color ?? '')
  }
  return out
}

/** Ids of the folder and all its descendants. */
export function descendantIds(folders: readonly Folder[], id: string): Set<string> {
  const children = new Map<string, string[]>()
  for (const f of folders) {
    if (!f.parent_id) continue
    const arr = children.get(f.parent_id) ?? []
    arr.push(f.id)
    children.set(f.parent_id, arr)
  }
  const out = new Set<string>()
  const stack = [id]
  while (stack.length) {
    const cur = stack.pop() as string
    if (out.has(cur)) continue
    out.add(cur)
    stack.push(...(children.get(cur) ?? []))
  }
  return out
}

/** Whether folder `dragId` may be moved under `targetId` (null = root). */
export function canMoveFolder(folders: readonly Folder[], dragId: string, targetId: string | null): boolean {
  const drag = folders.find((f) => f.id === dragId)
  if (!drag) return false
  if ((drag.parent_id ?? null) === targetId) return false
  if (targetId === null) return true
  return !descendantIds(folders, dragId).has(targetId)
}

/** Slash separated path of folder names from the root. */
export function folderPath(folders: readonly Folder[], id: string | null | undefined, sep = ' / '): string {
  if (!id) return ''
  const byId = new Map(folders.map((f) => [f.id, f]))
  const names: string[] = []
  const seen = new Set<string>()
  let cur = byId.get(id)
  while (cur && !seen.has(cur.id)) {
    seen.add(cur.id)
    names.unshift(cur.name)
    cur = cur.parent_id ? byId.get(cur.parent_id) : undefined
  }
  return names.join(sep)
}

/** Every key in the tree mapped to true (for expandedKeys). */
export function allKeys(nodes: readonly FolderNode[]): Record<string, boolean> {
  const out: Record<string, boolean> = {}
  const walk = (ns: readonly FolderNode[]) => {
    for (const n of ns) {
      out[n.key] = true
      walk(n.children)
    }
  }
  walk(nodes)
  return out
}

/** Depth-first flattened list with depth, useful for indented selects. */
export function flatten(nodes: readonly FolderNode[], depth = 0): { id: string; label: string; depth: number }[] {
  const out: { id: string; label: string; depth: number }[] = []
  for (const n of nodes) {
    out.push({ id: n.key, label: n.label, depth })
    out.push(...flatten(n.children, depth + 1))
  }
  return out
}

/** Keys of the ancestors of a folder (to expand the path to it). */
export function ancestorKeys(folders: readonly Folder[], id: string): string[] {
  const byId = new Map(folders.map((f) => [f.id, f]))
  const out: string[] = []
  const seen = new Set<string>()
  let cur = byId.get(id)
  while (cur?.parent_id && !seen.has(cur.parent_id)) {
    seen.add(cur.parent_id)
    out.unshift(cur.parent_id)
    cur = byId.get(cur.parent_id)
  }
  return out
}

/** Of the given folder ids, those not inside another given folder (in tree order of ids). */
export function topMostFolders(folders: readonly Folder[], ids: readonly string[]): string[] {
  const inside = new Set<string>()
  for (const id of ids) for (const d of descendantIds(folders, id)) if (d !== id) inside.add(d)
  return ids.filter((id) => !inside.has(id))
}

/** Checked folder ids of a PrimeVue checkbox tree selection. */
export function checkedIds(keys: Record<string, { checked?: boolean; partialChecked?: boolean } | boolean | undefined>): string[] {
  return Object.entries(keys)
    .filter(([, v]) => v === true || (typeof v === 'object' && !!v?.checked))
    .map(([k]) => k)
}
