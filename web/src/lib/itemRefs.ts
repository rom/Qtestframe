// References from a list item to related items (the "relations" field of
// requirements and tests in list responses), grouped for the Relations
// column: "depends on REQ-2, REQ-3 · verified by TC-1".
import type { ItemRef } from './types'

export interface RefGroup {
  type: string
  direction: 'outgoing' | 'incoming'
  /** kind of the items at the other end */
  kind: 'requirement' | 'test'
  items: ItemRef[]
}

/** Order of the groups: requirement relations first (outgoing before incoming), then tests. */
const ORDER = ['refines', 'depends_on', 'provides', 'derived_from', 'conflicts_with', 'duplicates', 'relates_to', 'verifies', 'calls']

/** The kind of item at the other end of a reference. */
export function refKind(r: Pick<ItemRef, 'type' | 'direction'> & { kind?: string }): 'requirement' | 'test' {
  if (r.kind === 'requirement' || r.kind === 'test') return r.kind
  if (r.type === 'calls') return 'test'
  if (r.type === 'verifies') return r.direction === 'outgoing' ? 'requirement' : 'test'
  return 'requirement'
}

export function groupRefs(refs: readonly ItemRef[] | null | undefined): RefGroup[] {
  const map = new Map<string, RefGroup>()
  for (const r of refs ?? []) {
    const direction = r.direction === 'incoming' ? 'incoming' : 'outgoing'
    const k = `${r.type}/${direction}`
    let g = map.get(k)
    if (!g) {
      g = { type: r.type, direction, kind: refKind(r), items: [] }
      map.set(k, g)
    }
    if (!g.items.some((x) => x.id === r.id)) g.items.push(r)
  }
  const rank = (g: RefGroup) => {
    const i = ORDER.indexOf(g.type)
    return (i < 0 ? ORDER.length : i) * 2 + (g.direction === 'incoming' ? 1 : 0)
  }
  return [...map.values()].sort((a, b) => rank(a) - rank(b))
}

/** Plain-text form of the references, used for sorting, CSV and screen readers. */
export function refsText(groups: readonly RefGroup[], label: (type: string, direction: string) => string): string {
  return groups.map((g) => `${label(g.type, g.direction)} ${g.items.map((i) => i.key).join(', ')}`).join('; ')
}
