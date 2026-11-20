// Bulk item links: which links a choice of targets adds to, or removes
// from, a selection of requirements, tests or runs.
import type { LinkType } from './itemLinks'
import type { ItemLink } from './types'

export type SourceType = 'requirement' | 'test' | 'run'

export interface LinkTargetRef {
  target_type: LinkType
  target_id?: string | null
  url?: string
}
export interface NewItemLink {
  source_type: string
  source_id: string
  target_type: string
  target_id?: string | null
  url?: string
  label?: string
}

const targetKey = (t: { target_type: string; target_id?: string | null; url?: string }) => `${t.target_type}|${t.target_id ?? ''}|${t.target_type === 'url' ? (t.url ?? '') : ''}`

/**
 * One link from each selected item to each target, without links of an
 * item to itself and links that already exist.
 */
export function planAddLinks(existing: ItemLink[], sourceType: SourceType, selected: string[], targets: LinkTargetRef[], label = ''): NewItemLink[] {
  const have = new Set(existing.filter((l) => l.source_type === sourceType).map((l) => `${l.source_id}|${targetKey(l)}`))
  const out: NewItemLink[] = []
  for (const src of selected)
    for (const tg of targets) {
      if (tg.target_type === sourceType && tg.target_id === src) continue
      const k = `${src}|${targetKey(tg)}`
      if (have.has(k)) continue
      have.add(k)
      const l: NewItemLink = { source_type: sourceType, source_id: src, target_type: tg.target_type }
      if (tg.target_type === 'url') l.url = tg.url
      else l.target_id = tg.target_id
      if (label.trim()) l.label = label.trim()
      out.push(l)
    }
  return out
}

/**
 * The links of the selected items that point to the given type (null: any)
 * and, if targets are given, to one of them.
 */
export function planRemoveLinks(existing: ItemLink[], sourceType: SourceType, selected: string[], type: LinkType | null, targets: LinkTargetRef[]): ItemLink[] {
  const sel = new Set(selected)
  const tg = new Set(targets.map(targetKey))
  return existing.filter((l) => l.source_type === sourceType && sel.has(l.source_id) && (!type || l.target_type === type) && (!tg.size || tg.has(targetKey(l))))
}

/** The request body that recreates a link (for undo). */
export function linkBody(l: ItemLink): NewItemLink {
  const b: NewItemLink = { source_type: l.source_type, source_id: l.source_id, target_type: l.target_type }
  if (l.target_type === 'url') b.url = l.url
  else b.target_id = l.target_id
  if (l.label) b.label = l.label
  return b
}

/** A web address the server accepts. */
export const validLinkUrl = (u: string): boolean => /^(https?:\/\/\S+|mailto:\S+)$/i.test(u.trim())
