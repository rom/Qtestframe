// Avatar and project icon values: "" (initials), "preset:<name>", "file:<id>".
import { API_BASE } from './api'

export type ImageRef = { kind: 'initials' } | { kind: 'preset'; name: string } | { kind: 'file'; id: string }

export const PRESET_NAME = /^[a-z0-9-]{1,32}$/

/** Parse an avatar / icon value; anything unknown is shown as initials. */
export function parseImageRef(v: string | null | undefined): ImageRef {
  const s = (v ?? '').trim()
  if (s.startsWith('preset:')) {
    const name = s.slice(7)
    if (PRESET_NAME.test(name)) return { kind: 'preset', name }
  } else if (s.startsWith('file:')) {
    const id = s.slice(5)
    if (/^[\w-]{1,64}$/.test(id)) return { kind: 'file', id }
  }
  return { kind: 'initials' }
}

export function presetValue(name: string): string {
  return `preset:${name}`
}

/** Preset name from a file path ("…/avatars/fox.svg" → "fox"). */
export function presetName(path: string): string {
  return (path.split('/').pop() ?? '').replace(/\.svg$/i, '')
}

/** Build a name → URL map from an import.meta.glob result. */
export function presetMap(modules: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [path, url] of Object.entries(modules)) {
    const name = presetName(path)
    if (PRESET_NAME.test(name)) out[name] = url
  }
  return out
}

/**
 * Image URL of an avatar (`/avatars/{id}`) or project icon
 * (`/files/{id}/content?inline=1`); null for initials or an unknown preset.
 */
export function imageUrl(v: string | null | undefined, kind: 'avatar' | 'icon', presets: Record<string, string>): string | null {
  const r = parseImageRef(v)
  if (r.kind === 'preset') return presets[r.name] ?? null
  if (r.kind === 'file') return kind === 'avatar' ? `${API_BASE}/avatars/${r.id}` : `${API_BASE}/files/${r.id}/content?inline=1`
  return null
}

/** Stable hue (0–359) for an initials badge. */
export function hueFor(seed: string | null | undefined): number {
  let h = 0
  for (const c of seed ?? '') h = (h * 31 + c.charCodeAt(0)) >>> 0
  return h % 360
}
