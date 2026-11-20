// Links between items (see ItemLinksPanel): what they can point to and
// where a link leads in the client.
import type { RouteLocationRaw } from 'vue-router'
import { contentUrl } from './files'
import type { ItemLink } from './types'

export const LINK_TYPES = ['requirement', 'test', 'run', 'file', 'report', 'url'] as const
export type LinkType = (typeof LINK_TYPES)[number]

export const LINK_ICONS: Record<LinkType, string> = {
  requirement: 'pi pi-list-check',
  test: 'pi pi-check-square',
  run: 'pi pi-play-circle',
  file: 'pi pi-file',
  report: 'pi pi-file-pdf',
  url: 'pi pi-globe',
}

export interface LinkDestination {
  /** a route in the tool */
  to?: RouteLocationRaw
  /** an address (archive file or web address) */
  href?: string
  /** opens outside the tool */
  external?: boolean
}

/**
 * Where a link leads: the target, or with fromSide the item that carries
 * an incoming link.
 */
export function linkTarget(l: ItemLink, projectId: string, fromSide = false): LinkDestination {
  const type = fromSide ? l.source_type : l.target_type
  const id = fromSide ? l.source_id : l.target_id
  const pid = l.project_id || projectId
  if (type === 'url') return { href: l.url, external: true }
  if (!id) return {}
  switch (type) {
    case 'requirement':
      return { to: { name: 'requirements', params: { projectId: pid }, query: { item: id } } }
    case 'test':
      return { to: { name: 'tests', params: { projectId: pid }, query: { item: id } } }
    case 'run':
      return { to: { name: 'run', params: { projectId: pid, runId: id } } }
    case 'file':
      return { href: contentUrl(id, true) }
    case 'report':
      return { to: { name: 'reports', params: { projectId: pid }, query: { definition: id } } }
  }
  return {}
}
