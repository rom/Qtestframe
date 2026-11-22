// Markdown of discussion comments → sanitised HTML.
//
// markdown-it parses the comment without raw HTML (typed tags stay text)
// and without images (no remote content in comments); bare web addresses
// become links. In plain text "@handle" mentions of known people become
// name chips and item keys (SHOP-REQ-12) links into the tool. DOMPurify
// sanitises the result like the user guide (render.ts); external links open
// in a new tab.
import MarkdownIt from 'markdown-it'
import type { Token } from 'markdown-it'
import DOMPurify from 'dompurify'
import { splitMentions, type Person } from './mentions'
import { splitTextLinks } from './textLinks'

const md = new MarkdownIt({ html: false, linkify: true, breaks: true, typographer: false })
md.disable(['image', 'heading', 'lheading', 'hr'])

const esc = (s: string) => md.utils.escapeHtml(s)

interface Env {
  [key: string]: unknown
  people: readonly Person[]
  newTab: string
}

/** HTML of one plain text run: mention chips, item key links, the rest escaped. */
function textHtml(text: string, env: Env): string {
  let out = ''
  for (const seg of text.includes('@') ? splitMentions(text, env.people) : [{ text }]) {
    if (seg.person) {
      const title = seg.person.email ? `@${seg.person.handle} · ${seg.person.email}` : `@${seg.person.handle}`
      out += `<span class="mention-chip" title="${esc(title)}">${esc(seg.person.name)}</span>`
      continue
    }
    if (seg.handle) {
      out += `<span class="mention-chip unknown">${esc(seg.text)}</span>`
      continue
    }
    for (const p of splitTextLinks(seg.text)) {
      // web addresses are linked by markdown-it (linkify) already
      out += p.key ? `<a href="/go/${encodeURIComponent(p.key)}" class="key" data-route="go">${esc(p.text)}</a>` : esc(p.text)
    }
  }
  return out
}

md.core.ruler.push('comment_mentions', (state) => {
  const env = state.env as unknown as Env
  for (const block of state.tokens) {
    if (block.type !== 'inline' || !block.children) continue
    let inLink = 0
    const children: Token[] = []
    for (const tok of block.children) {
      if (tok.type === 'link_open') inLink++
      if (tok.type === 'link_close') inLink--
      if (tok.type === 'text' && !inLink && (tok.content.includes('@') || /-(?:REQ|TC|BUG)-\d/.test(tok.content))) {
        const t = new state.Token('html_inline', '', 0)
        t.content = textHtml(tok.content, env)
        children.push(t)
        continue
      }
      children.push(tok)
    }
    block.children = children
  }
})

const defaultLinkOpen = md.renderer.rules.link_open ?? ((tokens, idx, opts, _env, self) => self.renderToken(tokens, idx, opts))
md.renderer.rules.link_open = (tokens, idx, opts, env, self) => {
  const tok = tokens[idx]
  const href = tok.attrGet('href') ?? ''
  if (/^https?:/i.test(String(href)) || /^mailto:/i.test(String(href))) {
    tok.attrSet('target', '_blank')
    tok.attrSet('rel', 'noopener noreferrer')
    tok.attrSet('class', 'text-link')
  }
  return defaultLinkOpen(tokens, idx, opts, env, self)
}
const defaultLinkClose = md.renderer.rules.link_close ?? ((tokens, idx, opts, _env, self) => self.renderToken(tokens, idx, opts))
md.renderer.rules.link_close = (tokens, idx, opts, env, self) => {
  // the matching link_open: tell screen reader users about the new tab
  for (let i = idx - 1; i >= 0; i--) {
    if (tokens[i].type === 'link_open') {
      if (tokens[i].attrGet('target') === '_blank') return `<span class="sr-only"> (${esc((env as unknown as Env).newTab)})</span>` + defaultLinkClose(tokens, idx, opts, env, self)
      break
    }
  }
  return defaultLinkClose(tokens, idx, opts, env, self)
}

// code blocks scroll sideways: make them reachable with the keyboard
for (const rule of ['fence', 'code_block'] as const) {
  const base = md.renderer.rules[rule]!
  md.renderer.rules[rule] = (tokens, idx, opts, env, self) => base(tokens, idx, opts, env, self).replace(/^<pre(?=[ >])/, '<pre tabindex="0"')
}

// an instance of its own: the hook below must not touch the user guide
const purify = DOMPurify(window)
// links to other sites open in a new tab, without access to this window
purify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A' && /^(?:https?|mailto):/i.test(node.getAttribute('href') ?? '')) {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noopener noreferrer')
  }
})

/** Sanitise comment HTML: formatting, links and the spans above only. */
export function sanitizeComment(html: string): string {
  return purify.sanitize(html, {
    ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 's', 'del', 'code', 'pre', 'blockquote', 'ul', 'ol', 'li', 'a', 'span', 'table', 'thead', 'tbody', 'tr', 'th', 'td'],
    ALLOWED_ATTR: ['href', 'target', 'rel', 'class', 'title', 'data-route', 'tabindex', 'start', 'align'],
    ALLOWED_URI_REGEXP: /^(?:https?:|mailto:|\/(?!\/))/i,
  })
}

/** Render a comment body (Markdown) to sanitised HTML. */
export function renderComment(body: string | null | undefined, people: readonly Person[] = [], newTab = 'opens in a new tab'): string {
  const env: Env = { people, newTab }
  return sanitizeComment(md.render(body ?? '', env as never))
}

/** Plain text of a comment (for excerpts): Markdown syntax removed. */
export function commentText(body: string | null | undefined): string {
  const tokens = md.parse(body ?? '', { people: [], newTab: '' })
  const parts: string[] = []
  for (const t of tokens) {
    if (t.type === 'inline') parts.push((t.children ?? []).map((c) => (c.type === 'text' || c.type === 'code_inline' ? c.content : c.type === 'softbreak' || c.type === 'hardbreak' ? ' ' : '')).join(''))
    else if (t.type === 'fence' || t.type === 'code_block') parts.push(t.content.trim())
  }
  return parts.join(' ').replace(/\s+/g, ' ').trim()
}
