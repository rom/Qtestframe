// The logotype on the first page of reports and generated documents
// (GET/PUT /settings/logo, /projects/{pid}/logo; see docs/api.md "Logo").

export const LOGO_PLACEMENTS = ['left', 'center', 'right'] as const
export type LogoPlacement = (typeof LOGO_PLACEMENTS)[number]

/** Bounds of the maximum size (millimetres), as the server's. */
export const LOGO_MIN_MM = 5
export const LOGO_MAX_WIDTH_MM = 170
export const LOGO_MAX_HEIGHT_MM = 40
/** The largest upload (bytes) and the accepted types. */
export const LOGO_MAX_BYTES = 2 << 20
export const LOGO_ACCEPT = 'image/png,image/jpeg,image/svg+xml,.svg,.png,.jpg,.jpeg'

export interface LogoSettings {
  /** "" without an image */
  file_id: string
  content_type?: string
  width?: number
  height?: number
  /** the uploaded format: png, jpeg or svg */
  source?: string
  name?: string
  placement: LogoPlacement
  max_width_mm: number
  max_height_mm: number
  /** projects: no logo at all */
  off?: boolean
  /** the image ("" without one) */
  url: string
}

/** GET /projects/{pid}/logo */
export interface ProjectLogo {
  own: LogoSettings
  organisation: LogoSettings
  effective: 'project' | 'organisation' | 'none'
}

/** Whether a file looks like an acceptable logo (by type or name) and is small enough. */
export function logoProblem(file: { size: number; type?: string; name?: string }): 'empty' | 'too_large' | 'type' | null {
  if (!file.size) return 'empty'
  if (file.size > LOGO_MAX_BYTES) return 'too_large'
  const type = (file.type ?? '').toLowerCase()
  const name = (file.name ?? '').toLowerCase()
  if (['image/png', 'image/jpeg', 'image/svg+xml'].includes(type) || /\.(png|jpe?g|svg)$/.test(name)) return null
  return 'type'
}

/**
 * The printed size of a logo (millimetres) as the documents show it: its
 * own size at 96 dpi, scaled down to the maximum width and height.
 */
export function logoSizeMM(l: Pick<LogoSettings, 'width' | 'height' | 'max_width_mm' | 'max_height_mm'>): { w: number; h: number } {
  if (!l.width || !l.height) return { w: 0, h: 0 }
  const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi)
  const mw = clamp(l.max_width_mm || 50, LOGO_MIN_MM, LOGO_MAX_WIDTH_MM)
  const mh = clamp(l.max_height_mm || 20, LOGO_MIN_MM, LOGO_MAX_HEIGHT_MM)
  const w = (l.width * 25.4) / 96
  const h = (l.height * 25.4) / 96
  const k = Math.min(1, mw / w, mh / h)
  return { w: w * k, h: h * k }
}
