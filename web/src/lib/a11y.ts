// Small DOM helpers for accessibility: label association, typing detection,
// focus management after route changes and labels for stacked tables.

let seq = 0
/** A document-unique id with a prefix. */
export function uid(prefix = 'qtf'): string {
  seq++
  return `${prefix}-${seq}`
}

/** Elements a <label for> can name. */
const LABELABLE = new Set(['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON', 'METER', 'OUTPUT', 'PROGRESS'])

const CONTROL_SELECTOR = [
  'input:not([type="hidden"])',
  'textarea',
  'select',
  '[role="combobox"]',
  '[role="listbox"]',
  '[role="spinbutton"]',
  '[role="slider"]',
  '[role="switch"]',
  '[role="textbox"]',
].join(',')

/**
 * Associate the first `<label>` of a form field container with its first
 * control: `for`/`id` for native controls, `aria-labelledby` for ARIA
 * widgets such as PrimeVue's Select (a span with role="combobox").
 * Labels that already have a `for` matching a control are left alone.
 * Returns the control that was labelled, or null.
 */
export function linkFieldLabel(container: Element): Element | null {
  const label = Array.from(container.children).find((c) => c.tagName === 'LABEL') as HTMLLabelElement | undefined
  if (!label) return null
  const doc = container.ownerDocument
  if (label.htmlFor) {
    const target = doc.getElementById(label.htmlFor)
    if (target && container.contains(target)) {
      if (!LABELABLE.has(target.tagName)) labelBy(target, label)
      markRequired(label, target)
      return target
    }
  }
  const control = container.querySelector(CONTROL_SELECTOR)
  if (!control || label.contains(control)) return null
  if (LABELABLE.has(control.tagName)) {
    if (!control.id) control.id = uid('fld')
    label.htmlFor = control.id
  } else {
    labelBy(control, label)
  }
  markRequired(label, control)
  return control
}

function labelBy(control: Element, label: HTMLLabelElement) {
  if (!label.id) label.id = uid('lbl')
  const current = control.getAttribute('aria-labelledby')
  if (current !== label.id) control.setAttribute('aria-labelledby', label.id)
}

/** Visible "required" marker on the label of a required control. */
function markRequired(label: Element, control: Element) {
  const required = control.hasAttribute('required') || control.getAttribute('aria-required') === 'true'
  label.classList.toggle('required', required)
}

/** True when keyboard input goes into a text field or an open widget (no single-key shortcuts then). */
export function isTypingTarget(el: EventTarget | null): boolean {
  if (!el || typeof (el as Element).closest !== 'function') return false
  const e = el as HTMLElement
  if (e.isContentEditable) return true
  const tag = e.tagName
  if (tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (tag === 'INPUT') {
    const type = ((e as HTMLInputElement).type || 'text').toLowerCase()
    return !['button', 'checkbox', 'radio', 'submit', 'reset', 'image', 'range', 'color', 'file'].includes(type)
  }
  return !!e.closest('[role="combobox"],[role="listbox"],[role="textbox"],[role="searchbox"],[role="spinbutton"],[contenteditable="true"]')
}

/** Only a change of page (path), not of query or hash, moves focus and is announced. */
export function isPageChange(to: { path: string }, from: { path: string; matched?: unknown[] } | null | undefined): boolean {
  if (!from || !from.matched || !(from.matched as unknown[]).length) return false // first navigation: keep the skip link first
  return to.path !== from.path
}

/**
 * Move focus to the page's main heading (or the main landmark) after a page
 * change. The heading gets tabindex=-1 so it can receive focus without
 * entering the tab order. Returns the focused element.
 */
export function focusPageStart(doc: Document = document, mainId = 'main-content'): HTMLElement | null {
  const main = doc.getElementById(mainId)
  const scope: ParentNode = main ?? doc
  const h1 = scope.querySelector<HTMLElement>('h1')
  const target = h1 ?? main
  if (!target) return null
  if (!target.hasAttribute('tabindex')) target.setAttribute('tabindex', '-1')
  target.focus({ preventScroll: true })
  return target
}

/**
 * Give every body cell of a table a `data-label` with the text of its
 * column header, for the stacked (card) layout on phones. Header cells
 * without text (selection, actions) give an empty label.
 */
export function labelStackedCells(table: Element): number {
  const heads = Array.from(table.querySelectorAll('thead > tr:first-child > th')).map((th) => (th.textContent ?? '').replace(/\s+/g, ' ').trim())
  let n = 0
  for (const row of Array.from(table.querySelectorAll('tbody > tr'))) {
    let col = 0
    for (const cell of Array.from(row.children)) {
      if (cell.tagName === 'TD') {
        const label = heads[col] ?? ''
        if (cell.getAttribute('data-label') !== label) cell.setAttribute('data-label', label)
        n++
      }
      col += Number(cell.getAttribute('colspan') || 1)
    }
  }
  return n
}

/**
 * Remember the focused element (e.g. before opening a drawer) and return a
 * function that puts focus back on it, or on `fallback` when it is gone.
 */
export function rememberFocus(doc: Document = document): (fallback?: () => HTMLElement | null | undefined) => HTMLElement | null {
  const el = doc.activeElement as HTMLElement | null
  return (fallback) => {
    const target = el && el !== doc.body && el.isConnected ? el : (fallback?.() ?? null)
    target?.focus({ preventScroll: true })
    return target
  }
}
