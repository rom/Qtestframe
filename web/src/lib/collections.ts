// Small array / object helpers.

/** Return a copy with the element at `from` moved to `to` (clamped). */
export function moveItem<T>(arr: readonly T[], from: number, to: number): T[] {
  const out = [...arr]
  if (from < 0 || from >= out.length) return out
  const target = Math.max(0, Math.min(out.length - 1, to))
  const [it] = out.splice(from, 1)
  out.splice(target, 0, it)
  return out
}

/** Assign `position` = index (1-based) to each element. */
export function renumber<T extends { position?: number }>(arr: readonly T[]): T[] {
  return arr.map((x, i) => ({ ...x, position: i + 1 }))
}

/** Group by a key function preserving first-seen order of groups. */
export function groupBy<T>(arr: readonly T[], key: (t: T) => string): { key: string; items: T[] }[] {
  const map = new Map<string, T[]>()
  for (const x of arr) {
    const k = key(x)
    const g = map.get(k)
    if (g) g.push(x)
    else map.set(k, [x])
  }
  return [...map.entries()].map(([k, items]) => ({ key: k, items }))
}

/** Map record values, sorted descending by count; useful for charts. */
export function entriesByCount(rec: Record<string, number> | null | undefined, order?: readonly string[]): [string, number][] {
  const entries = Object.entries(rec ?? {}).filter(([, v]) => typeof v === 'number')
  if (order) {
    const idx = (k: string) => {
      const i = order.indexOf(k)
      return i < 0 ? order.length : i
    }
    return entries.sort((a, b) => idx(a[0]) - idx(b[0]) || a[0].localeCompare(b[0]))
  }
  return entries.sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
}

/** Toggle a value in a list (returns a new list). */
export function toggle<T>(arr: readonly T[], v: T): T[] {
  return arr.includes(v) ? arr.filter((x) => x !== v) : [...arr, v]
}

/** Key for a requirement/test link pair. */
export function linkKey(requirementId: string, testId: string): string {
  return `${requirementId}|${testId}`
}

/** Debounce a function; the returned function has `cancel()` and `flush()`. */
export function debounce<A extends unknown[]>(fn: (...args: A) => void, wait: number) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let lastArgs: A | null = null
  const d = (...args: A) => {
    lastArgs = args
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      const a = lastArgs as A
      lastArgs = null
      fn(...a)
    }, wait)
  }
  d.cancel = () => {
    if (timer) clearTimeout(timer)
    timer = null
    lastArgs = null
  }
  d.flush = () => {
    if (timer && lastArgs) {
      clearTimeout(timer)
      timer = null
      const a = lastArgs
      lastArgs = null
      fn(...a)
    }
  }
  return d
}
