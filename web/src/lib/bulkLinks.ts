// Bulk operations on traceability links: what a selection of requirements
// and tests would link, unlink or confirm.
import { linkKey } from './collections'
import type { TraceLink } from './types'

export interface LinkPair {
  requirement_id: string
  test_case_id: string
}

export interface LinkPlan {
  /** pairs of the selection that are not linked yet */
  toLink: LinkPair[]
  /** existing links in scope: between selected requirements and selected tests; an empty side means all */
  inScope: TraceLink[]
  /** the suspect links in scope */
  suspect: TraceLink[]
}

export function linkPlan(links: TraceLink[], reqIds: string[], testIds: string[]): LinkPlan {
  const have = new Set(links.map((l) => linkKey(l.requirement_id, l.test_case_id)))
  const toLink: LinkPair[] = []
  for (const r of reqIds) for (const t of testIds) if (!have.has(linkKey(r, t))) toLink.push({ requirement_id: r, test_case_id: t })
  if (!reqIds.length && !testIds.length) return { toLink, inScope: [], suspect: [] }
  const rs = new Set(reqIds)
  const ts = new Set(testIds)
  const inScope = links.filter((l) => (!rs.size || rs.has(l.requirement_id)) && (!ts.size || ts.has(l.test_case_id)))
  return { toLink, inScope, suspect: inScope.filter((l) => !!l.suspect_since) }
}

/** At most `max` pairs per request; the server takes 1000. */
export function chunks<T>(items: T[], max = 1000): T[][] {
  const out: T[][] = []
  for (let i = 0; i < items.length; i += max) out.push(items.slice(i, i + max))
  return out
}
