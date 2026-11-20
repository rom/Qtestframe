// Requirement sources offered by the editor: the project's list, then other
// sources already used in the project; filtered as the user types.

/** Configured sources first, then used ones not configured (case-insensitive), without duplicates. */
export function mergeSources(configured: readonly string[], used: readonly string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const s of [...configured, ...used]) {
    const v = s.trim()
    if (!v || seen.has(v.toLowerCase())) continue
    seen.add(v.toLowerCase())
    out.push(v)
  }
  return out
}

/** Sources matching the typed text: starting with it first, then containing it; all for an empty query. */
export function filterSources(all: readonly string[], query: string): string[] {
  const q = query.trim().toLowerCase()
  if (!q) return [...all]
  const starts = all.filter((s) => s.toLowerCase().startsWith(q))
  const contains = all.filter((s) => !s.toLowerCase().startsWith(q) && s.toLowerCase().includes(q))
  return [...starts, ...contains]
}
