// Requirement relation helpers (labels, grouping).
import type { Relation } from './types'

export const RELATION_TYPES = ['refines', 'depends_on', 'derived_from', 'conflicts_with', 'duplicates', 'relates_to'] as const

const LABELS: Record<string, [outgoing: string, incoming: string]> = {
  refines: ['refines (parent)', 'refined by (child)'],
  depends_on: ['depends on', 'required by'],
  derived_from: ['derived from', 'source of'],
  conflicts_with: ['conflicts with', 'conflicts with'],
  duplicates: ['duplicates', 'duplicated by'],
  relates_to: ['relates to', 'relates to'],
}

/** Readable label of a relation seen from the queried requirement. */
export function relationLabel(type: string, direction: string | null | undefined): string {
  const l = LABELS[type]
  const incoming = direction === 'incoming'
  if (!l) return (incoming ? 'incoming ' : '') + type.replace(/_/g, ' ')
  return incoming ? l[1] : l[0]
}

/** Options for the "add relation" type select (outgoing direction). */
export function relationTypeOptions(): { label: string; value: string }[] {
  return RELATION_TYPES.map((t) => ({ label: LABELS[t][0], value: t }))
}

export interface RelationEnd {
  id: string
  key: string
  title: string
}

/** The requirement at the other end of the relation. */
export function otherEnd(r: Relation, selfId?: string): RelationEnd {
  const incoming = r.direction ? r.direction === 'incoming' : selfId ? r.target_id === selfId : false
  return incoming
    ? { id: r.source_id, key: r.source_key, title: r.source_title }
    : { id: r.target_id, key: r.target_key, title: r.target_title }
}

export interface RelationGroup {
  label: string
  type: string
  direction: 'outgoing' | 'incoming'
  items: Relation[]
}

/** Group relations by type and direction, in RELATION_TYPES order (outgoing first). */
export function groupRelations(list: readonly Relation[] | null | undefined, selfId?: string): RelationGroup[] {
  const map = new Map<string, RelationGroup>()
  for (const r of list ?? []) {
    const direction: 'outgoing' | 'incoming' = r.direction === 'incoming' || (!r.direction && !!selfId && r.target_id === selfId) ? 'incoming' : 'outgoing'
    const k = `${r.type}/${direction}`
    let g = map.get(k)
    if (!g) {
      g = { label: relationLabel(r.type, direction), type: r.type, direction, items: [] }
      map.set(k, g)
    }
    g.items.push(r)
  }
  const rank = (g: RelationGroup) => {
    const i = (RELATION_TYPES as readonly string[]).indexOf(g.type)
    return (i < 0 ? RELATION_TYPES.length : i) * 2 + (g.direction === 'incoming' ? 1 : 0)
  }
  const groups = [...map.values()].sort((a, b) => rank(a) - rank(b) || a.type.localeCompare(b.type))
  for (const g of groups) g.items.sort((a, b) => otherEnd(a, selfId).key.localeCompare(otherEnd(b, selfId).key, undefined, { numeric: true }))
  return groups
}
