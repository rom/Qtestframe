// Avatar / project icon image math: square crop, output size and format.
// The canvas work itself lives in composables/avatarImage.ts.

/** Edge length of stored avatars and icons. */
export const AVATAR_SIZE = 256
/** Server limit for avatar and icon uploads. */
export const MAX_IMAGE_BYTES = 1024 * 1024

export interface Crop {
  /** 1 = the largest square that fits, 4 = a quarter of it */
  zoom: number
  /** horizontal position of the crop, -1 (left) … 1 (right) */
  x: number
  /** vertical position, -1 (top) … 1 (bottom) */
  y: number
}

export const DEFAULT_CROP: Crop = { zoom: 1, x: 0, y: 0 }
export const MIN_ZOOM = 1
export const MAX_ZOOM = 4

export interface Rect {
  sx: number
  sy: number
  sw: number
  sh: number
}

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

/** Normalise a crop (zoom 1–4, positions -1…1, NaN → default). */
export function normalizeCrop(c: Partial<Crop> | null | undefined): Crop {
  const n = (v: unknown, d: number) => (typeof v === 'number' && Number.isFinite(v) ? v : d)
  return { zoom: clamp(n(c?.zoom, 1), MIN_ZOOM, MAX_ZOOM), x: clamp(n(c?.x, 0), -1, 1), y: clamp(n(c?.y, 0), -1, 1) }
}

/**
 * Source rectangle (in image pixels) of a square crop: the largest centred
 * square, shrunk by `zoom` and moved within the free space by x/y.
 */
export function cropRect(width: number, height: number, crop: Partial<Crop> = DEFAULT_CROP): Rect {
  if (!(width > 0) || !(height > 0)) return { sx: 0, sy: 0, sw: 0, sh: 0 }
  const c = normalizeCrop(crop)
  const side = Math.min(width, height) / c.zoom
  const freeX = width - side
  const freeY = height - side
  const sx = (freeX / 2) * (1 + c.x)
  const sy = (freeY / 2) * (1 + c.y)
  return { sx: Math.round(sx), sy: Math.round(sy), sw: Math.round(side), sh: Math.round(side) }
}

/** Output edge: AVATAR_SIZE, or the crop side when the image is smaller (never upscaled). */
export function outputSize(rect: Rect, max = AVATAR_SIZE): number {
  return Math.max(1, Math.min(max, Math.round(rect.sw)))
}

/** Encoders to try in order: webp when the browser can encode it, else png. */
export function outputTypes(supportsWebp: boolean): string[] {
  return supportsWebp ? ['image/webp', 'image/png'] : ['image/png']
}

/** Next lower quality for lossy re-encoding (null when out of steps). */
export function nextQuality(q: number): number | null {
  const steps = [0.92, 0.85, 0.75, 0.6, 0.45]
  const next = steps.find((s) => s < q - 1e-9)
  return next ?? null
}

/** File name for an encoded avatar. */
export function imageFileName(base: string, type: string): string {
  const ext = type === 'image/webp' ? 'webp' : type === 'image/jpeg' ? 'jpg' : 'png'
  const clean = (base || 'image').replace(/\.[a-z0-9]+$/i, '').replace(/[^\w.-]+/g, '-').slice(0, 40) || 'image'
  return `${clean}.${ext}`
}
