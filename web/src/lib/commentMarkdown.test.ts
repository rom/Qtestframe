import { describe, expect, it } from 'vitest'
import { commentText, renderComment, sanitizeComment } from './commentMarkdown'
import type { Person } from './mentions'

const people: Person[] = [{ handle: 'anna.berg', name: 'Anna Berg', kind: 'user', user_id: 'u1', email: 'anna.berg@example.com' }]

describe('renderComment', () => {
  it('renders Markdown', () => {
    const html = renderComment('**bold** and *it*\n\n- one\n- two\n\n`x < y`')
    expect(html).toContain('<strong>bold</strong>')
    expect(html).toContain('<em>it</em>')
    expect(html).toContain('<li>one</li>')
    expect(html).toContain('<code>x &lt; y</code>')
  })

  it('keeps typed HTML as text and drops scripts, images and handlers', () => {
    const html = renderComment('<script>alert(1)</script><img src=x onerror=y()> ![a](http://x/y.png) [x](javascript:alert(1))')
    expect(html).not.toContain('<script')
    expect(html).not.toContain('<img')
    expect(html).not.toContain('href="javascript:')
    expect(html).toContain('&lt;script&gt;')
    expect(sanitizeComment('<p onclick="x()">a</p><iframe src="//x"></iframe>')).toBe('<p>a</p>')
  })

  it('opens web links in a new tab and links item keys into the tool', () => {
    const html = renderComment('See https://example.com/a and SHOP-REQ-12.', [], 'new tab')
    expect(html).toContain('href="https://example.com/a"')
    expect(html).toContain('target="_blank"')
    expect(html).toContain('rel="noopener noreferrer"')
    expect(html).toContain('(new tab)')
    expect(html).toContain('href="/go/SHOP-REQ-12"')
    expect(html).toContain('data-route="go"')
  })

  it('shows mentions as name chips, not inside links or code', () => {
    const html = renderComment('Hi @anna.berg, ask @nobody. `@anna.berg` mail anna@example.com', people)
    expect(html).toContain('<span class="mention-chip" title="@anna.berg · anna.berg@example.com">Anna Berg</span>')
    expect(html).toContain('<span class="mention-chip unknown">@nobody</span>')
    expect(html).toContain('<code>@anna.berg</code>')
    expect(html).not.toContain('mention-chip unknown">@example')
    // names are escaped
    expect(renderComment('@x', [{ handle: 'x', name: '<b>X</b>', kind: 'person' }])).toContain('&lt;b&gt;X&lt;/b&gt;')
  })

  it('handles empty input', () => {
    expect(renderComment(null)).toBe('')
    expect(renderComment('')).toBe('')
  })
})

describe('commentText', () => {
  it('strips the Markdown syntax', () => {
    expect(commentText('**Is** this\n\n- *testable*?\n\n```\ncode\n```')).toBe('Is this testable? code')
    expect(commentText(undefined)).toBe('')
  })
})
