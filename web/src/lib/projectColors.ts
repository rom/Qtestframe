// Project colours: a palette to choose from and the readable text colour on
// a colour (black or white, whichever contrasts more).
import { contrastRatio } from '@/themes/contrast'

/** Colours offered for projects; any #rrggbb can also be entered. */
export const PROJECT_COLORS = [
  '#4f46e5', '#2563eb', '#0891b2', '#0d9488', '#16a34a', '#65a30d',
  '#ca8a04', '#ea580c', '#dc2626', '#db2777', '#9333ea', '#475569',
] as const

const HEX = /^#[0-9a-f]{6}$/i

/** Whether a value is a usable #rrggbb colour. */
export function isHexColor(v: unknown): v is string {
  return typeof v === 'string' && HEX.test(v)
}

/** Black or white, whichever is more readable on `bg`. */
export function readableOn(bg: string): '#ffffff' | '#111827' {
  if (!isHexColor(bg)) return '#111827'
  return contrastRatio('#ffffff', bg) >= contrastRatio('#111827', bg) ? '#ffffff' : '#111827'
}

/** Inline style for an element filled with a project colour ({} without one). */
export function projectColorStyle(color: string | null | undefined): Record<string, string> {
  return isHexColor(color) ? { background: color, color: readableOn(color) } : {}
}
