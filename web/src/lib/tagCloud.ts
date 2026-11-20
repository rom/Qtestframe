// Tag cloud helpers: font sizes by use and the query for selected tags.
import { and, anyOf, clause } from './query'

export interface TagCount {
  tag: string
  requirements: number
  tests: number
  documents: number
  total: number
}

/** Font size in px for a tag used `count` times, on a log scale between minPx and maxPx. */
export function cloudSize(count: number, min: number, max: number, minPx = 13, maxPx = 30): number {
  if (max <= min) return Math.round((minPx + maxPx) / 2)
  const f = (Math.log(Math.max(count, 1)) - Math.log(Math.max(min, 1))) / (Math.log(max) - Math.log(Math.max(min, 1)))
  return Math.round(minPx + Math.max(0, Math.min(1, f)) * (maxPx - minPx))
}

/** Query for items carrying all (`all`) or any (`any`) of the tags; '' for none. */
export function tagQuery(tags: readonly string[], mode: 'all' | 'any'): string {
  if (!tags.length) return ''
  return mode === 'all' ? and(...tags.map((t) => clause('tag', '=', t))) : anyOf('tag', tags)
}

/** Sort tags by use (most first) or by name. */
export function sortTags(tags: readonly TagCount[], by: 'count' | 'name'): TagCount[] {
  const out = [...tags]
  if (by === 'name') out.sort((a, b) => a.tag.localeCompare(b.tag, undefined, { sensitivity: 'base' }))
  else out.sort((a, b) => b.total - a.total || a.tag.localeCompare(b.tag, undefined, { sensitivity: 'base' }))
  return out
}
