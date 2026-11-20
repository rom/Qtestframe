// Colours of tags: every tag gets one of TAG_COLOR_COUNT colours, chosen from
// a hash of its name (case-insensitive), so a tag looks the same everywhere.
// The colours are CSS classes tag-c0 … tag-c9 in styles.css, with a light
// and a dark variant that both keep the text at WCAG AA contrast.

export const TAG_COLOR_COUNT = 10

/** The colour index of a tag (FNV-1a hash of the lower-cased name). */
export function tagColorIndex(tag: string): number {
  let h = 0x811c9dc5
  for (const ch of tag.trim().toLowerCase()) {
    h ^= ch.codePointAt(0)!
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h % TAG_COLOR_COUNT
}

/** CSS class of a tag's colour. */
export function tagClass(tag: string): string {
  return `tag-c${tagColorIndex(tag)}`
}

/** The light and dark palettes (background, text), mirrored in styles.css and checked for contrast. */
export const TAG_PALETTE: { light: [bg: string, fg: string]; dark: [bg: string, fg: string] }[] = [
  { light: ['#dbeafe', '#1e3a8a'], dark: ['#1e3a5f', '#bfdbfe'] }, // blue
  { light: ['#dcfce7', '#14532d'], dark: ['#163d27', '#bbf7d0'] }, // green
  { light: ['#fef3c7', '#78350f'], dark: ['#45340f', '#fde68a'] }, // amber
  { light: ['#fce7f3', '#831843'], dark: ['#4a1a33', '#fbcfe8'] }, // pink
  { light: ['#ede9fe', '#4c1d95'], dark: ['#33245c', '#ddd6fe'] }, // violet
  { light: ['#ccfbf1', '#134e4a'], dark: ['#123f3c', '#99f6e4'] }, // teal
  { light: ['#ffedd5', '#7c2d12'], dark: ['#4a2614', '#fed7aa'] }, // orange
  { light: ['#e0e7ff', '#312e81'], dark: ['#2a2e5c', '#c7d2fe'] }, // indigo
  { light: ['#fee2e2', '#7f1d1d'], dark: ['#4b1d1d', '#fecaca'] }, // red
  { light: ['#e2e8f0', '#1e293b'], dark: ['#334155', '#e2e8f0'] }, // slate
]
