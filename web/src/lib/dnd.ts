// Drag and drop payloads between the item tables and the folder tree.

export const ITEMS_MIME = 'application/x-qtf-items'
export const FOLDER_MIME = 'application/x-qtf-folder'

export function encodeIds(ids: readonly string[]): string {
  return JSON.stringify([...ids])
}

/** Parse an id list payload; returns [] for anything malformed. */
export function decodeIds(payload: string | null | undefined): string[] {
  if (!payload) return []
  try {
    const v = JSON.parse(payload)
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string' && x !== '') : []
  } catch {
    return []
  }
}

/**
 * Ids to drag when a row is dragged: the whole selection if the row is part
 * of it, otherwise just the row.
 */
export function dragIds(rowId: string, selection: readonly { id: string }[] | null | undefined): string[] {
  const sel = (selection ?? []).map((s) => s.id)
  return sel.includes(rowId) ? sel : [rowId]
}
