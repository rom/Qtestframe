// Finds item keys (SHOP-REQ-12, SHOP-TC-3) and web addresses in plain text
// so they can be shown as links: keys open the item in the tool, addresses
// open in a new tab.

export type TextPart = { text: string; key?: string; url?: string }

const KEY = String.raw`\b[A-Z][A-Z0-9]{1,15}-(?:REQ|TC)-\d+\b`
const URL = String.raw`\bhttps?:\/\/[^\s<>"'\]]+`
const PATTERN = new RegExp(`(${KEY})|(${URL})`, 'g')
// punctuation that usually ends a sentence rather than an address
const TRAILING = /[.,;:!?)]+$/

export function splitTextLinks(text: string | null | undefined): TextPart[] {
  const s = text ?? ''
  const out: TextPart[] = []
  let last = 0
  for (const m of s.matchAll(PATTERN)) {
    let value = m[0]
    let end = m.index! + value.length
    if (m[2]) {
      const trail = value.match(TRAILING)?.[0] ?? ''
      // keep a closing parenthesis that belongs to the address, e.g. …/Foo_(bar)
      const keep = trail.startsWith(')') && value.includes('(') ? 1 : 0
      const cut = trail.length - keep
      if (cut > 0) {
        value = value.slice(0, value.length - cut)
        end -= cut
      }
      if (/^https?:\/\/$/i.test(value)) continue
    }
    if (m.index! > last) out.push({ text: s.slice(last, m.index) })
    out.push(m[1] ? { text: value, key: value } : { text: value, url: value })
    last = end
  }
  if (last < s.length) out.push({ text: s.slice(last) })
  return out
}

/** Whether a text contains anything to link (cheap check before splitting). */
export const hasTextLinks = (text: string | null | undefined): boolean => !!text && (/-(?:REQ|TC)-\d/.test(text) || /https?:\/\//i.test(text))
