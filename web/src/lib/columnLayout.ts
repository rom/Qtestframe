// Column layout of tables: which optional columns are shown and in which
// order. The order is the list of visible column ids (pure helpers; see
// composables/columns.ts for the stored, reactive version).

/** A saved order, cleaned: only known ids, no duplicates; the defaults when nothing usable was saved. */
export function normalizeOrder(saved: unknown, available: readonly string[], defaults: readonly string[], rename: Record<string, string> = {}): string[] {
  const known = new Set(available)
  const out: string[] = []
  if (Array.isArray(saved)) {
    for (const raw of saved) {
      const id = typeof raw === 'string' ? (rename[raw] ?? raw) : ''
      if (known.has(id) && !out.includes(id)) out.push(id)
    }
    return out
  }
  return defaults.filter((d) => known.has(d))
}

/** Show or hide a column; a newly shown column is added at the end. */
export function toggleColumn(order: readonly string[], id: string, on: boolean): string[] {
  if (on) return order.includes(id) ? [...order] : [...order, id]
  return order.filter((x) => x !== id)
}

/** Move a visible column by delta places (clamped). */
export function moveColumn(order: readonly string[], id: string, delta: number): string[] {
  const i = order.indexOf(id)
  if (i < 0) return [...order]
  const j = Math.max(0, Math.min(order.length - 1, i + delta))
  const out = [...order]
  out.splice(i, 1)
  out.splice(j, 0, id)
  return out
}

/**
 * The order after a drag in the table header: the table reports the keys of
 * all its columns (fixed ones included) in their new order; the visible
 * optional columns follow that order.
 */
export function orderFromKeys(keys: readonly (string | null | undefined)[], order: readonly string[]): string[] {
  const visible = new Set(order)
  const out = keys.filter((k): k is string => !!k && visible.has(k))
  return [...new Set([...out, ...order])]
}
