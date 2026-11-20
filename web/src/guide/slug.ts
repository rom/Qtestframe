// Heading anchors for the user guide: GitHub-style slugs (lower case,
// letters and digits of any script kept, spaces → "-"), made unique per page.

/** The slug of a heading text, e.g. "3. Write requirements" → "3-write-requirements". */
export function slugify(text: string): string {
  const s = text
    .normalize('NFC')
    .toLowerCase()
    .trim()
    .replace(/[^\p{L}\p{N}\s_-]/gu, '')
    .replace(/\s+/g, '-')
    .replace(/-{2,}/g, '-')
    .replace(/^-+|-+$/g, '')
  return s || 'section'
}

/**
 * A slug generator for one document: repeated headings get "-1", "-2", …
 * appended, like GitHub does, so every anchor on a page is unique.
 */
export function createSlugger(): (text: string) => string {
  const used = new Set<string>()
  return (text: string) => {
    const base = slugify(text)
    let slug = base
    for (let n = 1; used.has(slug); n++) slug = `${base}-${n}`
    used.add(slug)
    return slug
  }
}
