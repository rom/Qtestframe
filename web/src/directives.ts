// Global directives for accessibility and the responsive table layout.
import type { Directive } from 'vue'
import { labelStackedCells, linkFieldLabel } from '@/lib/a11y'

/**
 * v-field on a `.field` container: ties its `<label>` to the control inside
 * (for/id, or aria-labelledby for PrimeVue widgets) and marks required ones.
 */
export const vField: Directive<HTMLElement> = {
  mounted: (el) => void linkFieldLabel(el),
  updated: (el) => void linkFieldLabel(el),
}

const observers = new WeakMap<HTMLElement, MutationObserver>()

/**
 * v-stack on a DataTable: below 640 px the rows become cards (CSS class
 * stack-table); every cell is labelled with its column header (data-label).
 */
export const vStack: Directive<HTMLElement> = {
  mounted(el) {
    el.classList.add('stack-table')
    let queued = false
    const relabel = () => {
      queued = false
      const table = el.querySelector('table')
      if (table) labelStackedCells(table)
    }
    relabel()
    if (typeof MutationObserver === 'undefined') return
    const mo = new MutationObserver(() => {
      if (queued) return
      queued = true
      queueMicrotask(relabel)
    })
    mo.observe(el, { childList: true, subtree: true, characterData: true })
    observers.set(el, mo)
  },
  unmounted(el) {
    observers.get(el)?.disconnect()
    observers.delete(el)
  },
}
