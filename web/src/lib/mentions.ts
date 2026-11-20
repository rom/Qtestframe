// "@name" mentions: finding the mention being typed, inserting a chosen
// person, ranking people for the suggestion list and splitting text into
// plain text and mentions for display. Handles follow internal/mention:
// letters, digits, ".", "_" and "-", not ending in punctuation, and an "@"
// directly after a letter or digit (an e-mail address) is not a mention.

export interface Person {
  handle: string
  name: string
  kind: 'user' | 'person' | string
  user_id?: string
  email?: string
  avatar?: string
  uses?: number
}

const HANDLE_CHAR = /[\p{L}\p{N}._-]/u
const WORD_CHAR = /[\p{L}\p{N}]/u

/** The mention being typed at `caret`: where its "@" is and what follows it, or null. */
export function activeMention(text: string, caret: number): { start: number; query: string } | null {
  let i = Math.min(caret, text.length) - 1
  while (i >= 0 && HANDLE_CHAR.test(text[i])) i--
  if (i < 0 || text[i] !== '@') return null
  if (i > 0 && WORD_CHAR.test(text[i - 1])) return null
  const query = text.slice(i + 1, caret)
  if (query.length > 64) return null
  return { start: i, query }
}

/** Replace the mention from `start` to `caret` by "@handle " and return the new text and caret. */
export function insertMention(text: string, start: number, caret: number, handle: string): { text: string; caret: number } {
  let end = caret
  while (end < text.length && HANDLE_CHAR.test(text[end])) end++
  const after = text.slice(end)
  const insert = `@${handle}` + (after.startsWith(' ') || after.startsWith('\n') ? '' : ' ')
  return { text: text.slice(0, start) + insert + after, caret: start + insert.length }
}

function fold(s: string): string {
  return s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase()
}

/**
 * People matching what was typed after "@", best first: handle or name
 * starting with the query, then a name word starting with it, then the
 * query anywhere; users before other people, frequent ones first.
 */
export function rankPeople(people: readonly Person[], query: string, limit = 8): Person[] {
  const q = fold(query.trim())
  const scored: { p: Person; s: number }[] = []
  for (const p of people) {
    const handle = fold(p.handle)
    const name = fold(p.name)
    const email = fold(p.email ?? '')
    let s: number
    if (!q) s = 1
    else if (handle.startsWith(q) || name.startsWith(q)) s = 4
    else if (name.split(/[\s.\-_]+/).some((w) => w.startsWith(q))) s = 3
    else if (handle.includes(q) || name.includes(q) || email.startsWith(q)) s = 2
    else continue
    scored.push({ p, s: s * 10 + (p.kind === 'user' ? 2 : 0) + Math.min(p.uses ?? 0, 9) / 10 })
  }
  scored.sort((a, b) => b.s - a.s || a.p.name.localeCompare(b.p.name))
  return scored.slice(0, limit).map((x) => x.p)
}

export type MentionSegment = { text: string; person?: Person; handle?: string }

/** Split text into plain parts and "@handle" mentions (with the person when known). */
export function splitMentions(text: string, people: readonly Person[]): MentionSegment[] {
  const byHandle = new Map(people.map((p) => [p.handle.toLowerCase(), p]))
  const out: MentionSegment[] = []
  const re = /@([\p{L}\p{N}._-]+)/gu
  let last = 0
  for (const m of text.matchAll(re)) {
    const at = m.index ?? 0
    if (at > 0 && WORD_CHAR.test(text[at - 1])) continue
    let handle = m[1]
    while (/[._-]$/.test(handle)) handle = handle.slice(0, -1)
    if (!handle) continue
    if (at > last) out.push({ text: text.slice(last, at) })
    out.push({ text: '@' + handle, handle: handle.toLowerCase(), person: byHandle.get(handle.toLowerCase()) })
    last = at + 1 + handle.length
  }
  if (last < text.length) out.push({ text: text.slice(last) })
  return out
}
