import { afterEach, describe, expect, it } from 'vitest'
import { focusPageStart, isPageChange, isTypingTarget, labelStackedCells, linkFieldLabel, uid } from './a11y'

function html(s: string): HTMLElement {
  const el = document.createElement('div')
  el.innerHTML = s
  document.body.appendChild(el)
  return el
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('uid', () => {
  it('is unique and prefixed', () => {
    const a = uid()
    const b = uid('x')
    expect(a).not.toBe(b)
    expect(a).toMatch(/^qtf-\d+$/)
    expect(b).toMatch(/^x-\d+$/)
  })
})

describe('linkFieldLabel', () => {
  it('links a label to a native input with for/id', () => {
    const el = html('<div class="field"><label>Name</label><input required></div>')
    const c = linkFieldLabel(el.firstElementChild!) as HTMLInputElement
    const label = el.querySelector('label')!
    expect(c.tagName).toBe('INPUT')
    expect(c.id).toMatch(/^fld-/)
    expect(label.htmlFor).toBe(c.id)
    expect(label.classList.contains('required')).toBe(true)
  })
  it('keeps an existing id of the control', () => {
    const el = html('<div><label>Notes</label><textarea id="notes"></textarea></div>')
    linkFieldLabel(el.firstElementChild!)
    expect(el.querySelector('label')!.htmlFor).toBe('notes')
    expect(el.querySelector('label')!.classList.contains('required')).toBe(false)
  })
  it('uses aria-labelledby for ARIA widgets such as a combobox span', () => {
    const el = html('<div><label>Status</label><div class="p-select"><span role="combobox" aria-required="true"></span></div></div>')
    const c = linkFieldLabel(el.firstElementChild!)!
    const label = el.querySelector('label')!
    expect(label.id).toMatch(/^lbl-/)
    expect(c.getAttribute('aria-labelledby')).toBe(label.id)
    expect(label.classList.contains('required')).toBe(true)
    // idempotent
    linkFieldLabel(el.firstElementChild!)
    expect(c.getAttribute('aria-labelledby')).toBe(label.id)
  })
  it('respects an explicit for that points into the field', () => {
    const el = html('<div><label for="pf-x">Theme</label><span id="pf-x" role="combobox"></span><input id="other"></div>')
    const c = linkFieldLabel(el.firstElementChild!)!
    expect(c.id).toBe('pf-x')
    expect(c.getAttribute('aria-labelledby')).toBe(el.querySelector('label')!.id)
    const el2 = html('<div><label for="i2">X</label><input id="i2" required></div>')
    expect(linkFieldLabel(el2.firstElementChild!)!.id).toBe('i2')
    expect(el2.querySelector('label')!.classList.contains('required')).toBe(true)
  })
  it('ignores an explicit for that points elsewhere and links the control inside', () => {
    html('<input id="elsewhere">')
    const el = html('<div><label for="elsewhere">X</label><input></div>')
    const c = linkFieldLabel(el.firstElementChild!) as HTMLInputElement
    expect(el.querySelector('label')!.htmlFor).toBe(c.id)
  })
  it('does nothing without label or control, or when the label wraps the control', () => {
    expect(linkFieldLabel(html('<div><span>No label</span><input></div>').firstElementChild!)).toBeNull()
    expect(linkFieldLabel(html('<div><label>Only text</label><div class="step-box">x</div></div>').firstElementChild!)).toBeNull()
    expect(linkFieldLabel(html('<div><label>Wrap <input type="checkbox"></label></div>').firstElementChild!)).toBeNull()
  })
})

describe('isTypingTarget', () => {
  it('detects text fields and widgets', () => {
    const el = html(`
      <input id="t"><input id="cb" type="checkbox"><input id="btn" type="button"><textarea id="ta"></textarea><select id="s"></select>
      <div id="ce" contenteditable="true"></div><span id="combo" role="combobox"></span><div role="listbox"><span id="opt"></span></div>
      <button id="b">x</button><div id="d"></div>`)
    const $ = (id: string) => el.querySelector('#' + id)
    expect(isTypingTarget($('t'))).toBe(true)
    expect(isTypingTarget($('ta'))).toBe(true)
    expect(isTypingTarget($('s'))).toBe(true)
    expect(isTypingTarget($('combo'))).toBe(true)
    expect(isTypingTarget($('opt'))).toBe(true)
    expect(isTypingTarget($('cb'))).toBe(false)
    expect(isTypingTarget($('btn'))).toBe(false)
    expect(isTypingTarget($('b'))).toBe(false)
    expect(isTypingTarget($('d'))).toBe(false)
    expect(isTypingTarget(null)).toBe(false)
    expect(isTypingTarget(window)).toBe(false)
    const ce = $('ce') as HTMLElement
    Object.defineProperty(ce, 'isContentEditable', { value: true })
    expect(isTypingTarget(ce)).toBe(true)
  })
})

describe('isPageChange', () => {
  it('only counts path changes after the first navigation', () => {
    expect(isPageChange({ path: '/a' }, null)).toBe(false)
    expect(isPageChange({ path: '/a' }, { path: '/', matched: [] })).toBe(false)
    expect(isPageChange({ path: '/a' }, { path: '/b', matched: [{}] })).toBe(true)
    expect(isPageChange({ path: '/a' }, { path: '/a', matched: [{}] })).toBe(false)
    expect(isPageChange({ path: '/a' }, { path: '/b' })).toBe(false)
  })
})

describe('focusPageStart', () => {
  it('focuses the h1 inside main', () => {
    html('<h1 id="outside">x</h1><main id="main-content" tabindex="-1"><h1 id="page">Page</h1></main>')
    const f = focusPageStart(document)!
    expect(f.id).toBe('page')
    expect(f.getAttribute('tabindex')).toBe('-1')
    expect(document.activeElement).toBe(f)
  })
  it('falls back to main, then to any h1, then null', () => {
    html('<main id="main-content" tabindex="-1"><p>no heading</p></main>')
    expect(focusPageStart(document)!.id).toBe('main-content')
    document.body.innerHTML = ''
    html('<h1 id="h">Login</h1>')
    expect(focusPageStart(document)!.id).toBe('h')
    document.body.innerHTML = ''
    expect(focusPageStart(document)).toBeNull()
  })
})

describe('labelStackedCells', () => {
  it('labels body cells with their column header', () => {
    const el = html(`<table>
      <thead><tr><th></th><th> Key </th><th>Title</th><th>Status</th></tr></thead>
      <tbody><tr><td>x</td><td>K-1</td><td colspan="2">long</td></tr><tr><th>row</th><td>a</td><td>b</td><td>c</td></tr></tbody></table>`)
    const n = labelStackedCells(el.querySelector('table')!)
    expect(n).toBe(6)
    const r1 = el.querySelectorAll('tbody tr')[0].querySelectorAll('td')
    expect(r1[0].getAttribute('data-label')).toBe('')
    expect(r1[1].getAttribute('data-label')).toBe('Key')
    expect(r1[2].getAttribute('data-label')).toBe('Title')
    const r2 = el.querySelectorAll('tbody tr')[1].querySelectorAll('td')
    expect([...r2].map((c) => c.getAttribute('data-label'))).toEqual(['Key', 'Title', 'Status'])
    // stable on a second run
    expect(labelStackedCells(el.querySelector('table')!)).toBe(6)
  })
})

describe('rememberFocus', () => {
  it('returns focus to the remembered element, or to a fallback', async () => {
    const { rememberFocus } = await import('./a11y')
    const el = html('<button id="a">a</button><button id="b">b</button><h1 id="h" tabindex="-1">H</h1>')
    const a = el.querySelector<HTMLElement>('#a')!
    a.focus()
    const restore = rememberFocus(document)
    el.querySelector<HTMLElement>('#b')!.focus()
    expect(restore()).toBe(a)
    expect(document.activeElement).toBe(a)
    // the opener is gone: the fallback gets focus
    const restore2 = rememberFocus(document)
    a.remove()
    expect(restore2(() => el.querySelector<HTMLElement>('#h'))!.id).toBe('h')
    // nothing focused and no fallback
    ;(document.activeElement as HTMLElement).blur()
    expect(rememberFocus(document)()).toBeNull()
  })
})
