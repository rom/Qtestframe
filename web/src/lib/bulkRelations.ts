// Bulk relations: which relations a choice of type, direction and target
// requirements adds to or removes from a selection of requirements.
import { RELATION_TYPES } from './relations'
import type { Relation } from './types'

/** Types that read the same in both directions. */
export const SYMMETRIC = new Set(['conflicts_with', 'relates_to'])

export type Direction = 'outgoing' | 'incoming'
export interface RelationChoice {
  type: string
  direction: Direction
}
export interface NewRelation {
  source_id: string
  target_id: string
  type: string
}

/** Every type in both directions; symmetric types only once. */
export function relationChoices(): RelationChoice[] {
  const out: RelationChoice[] = []
  for (const type of RELATION_TYPES) {
    out.push({ type, direction: 'outgoing' })
    if (!SYMMETRIC.has(type)) out.push({ type, direction: 'incoming' })
  }
  return out
}
export const choiceValue = (c: RelationChoice): string => `${c.type}:${c.direction}`
export function parseChoice(v: string): RelationChoice | null {
  const [type, direction] = v.split(':')
  return type && (direction === 'outgoing' || direction === 'incoming') ? { type, direction } : null
}

const tripleKey = (s: string, d: string, t: string) => `${s}|${d}|${t}`

/**
 * The relations to create so that each selected requirement has the chosen
 * relation to each target. Self relations and existing ones are left out.
 */
export function planAdd(existing: Relation[], selected: string[], targets: string[], c: RelationChoice): NewRelation[] {
  const have = new Set(existing.map((r) => tripleKey(r.source_id, r.target_id, r.type)))
  if (SYMMETRIC.has(c.type)) for (const r of existing) if (r.type === c.type) have.add(tripleKey(r.target_id, r.source_id, r.type))
  const out: NewRelation[] = []
  const seen = new Set<string>()
  for (const sel of selected)
    for (const tgt of targets) {
      if (sel === tgt) continue
      const [source_id, target_id] = c.direction === 'outgoing' ? [sel, tgt] : [tgt, sel]
      const k = tripleKey(source_id, target_id, c.type)
      if (have.has(k) || seen.has(k)) continue
      seen.add(k)
      out.push({ source_id, target_id, type: c.type })
    }
  return out
}

/**
 * The existing relations of the selected requirements that match the choice
 * (null: any type, either direction) and have a target at the other end
 * (no targets: any requirement).
 */
export function planRemove(existing: Relation[], selected: string[], targets: string[], c: RelationChoice | null): Relation[] {
  const sel = new Set(selected)
  const tg = new Set(targets)
  const other = (id: string) => !tg.size || tg.has(id)
  return existing.filter((r) => {
    if (c && r.type !== c.type) return false
    const out = sel.has(r.source_id) && other(r.target_id)
    const inc = sel.has(r.target_id) && other(r.source_id)
    if (!c || SYMMETRIC.has(c.type)) return out || inc
    return c.direction === 'outgoing' ? out : inc
  })
}

/** Counts of skip reasons, e.g. {exists: 2, invalid: 1}. */
export function reasonCounts(skipped: { reason: string }[]): Record<string, number> {
  const out: Record<string, number> = {}
  for (const s of skipped) out[s.reason] = (out[s.reason] ?? 0) + 1
  return out
}
