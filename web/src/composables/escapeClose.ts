// Escape closes only the topmost dialog of ours. PrimeVue's Drawer and Dialog
// both listen on document, so without this an Escape in a dialog opened from
// the editor drawer would close the drawer too.
import { onBeforeUnmount, watch, type Ref } from 'vue'

export function useEscapeClose(visible: Ref<boolean>): void {
  function onKey(e: KeyboardEvent) {
    if (e.key !== 'Escape' || !visible.value) return
    // leave Escape to an open dropdown / overlay inside the dialog
    if ((e.target as Element | null)?.closest?.('.p-select-overlay, .p-datepicker-panel, .p-popover')) return
    e.stopImmediatePropagation()
    e.preventDefault()
    visible.value = false
  }
  watch(
    visible,
    (v) => {
      if (v) window.addEventListener('keydown', onKey, true)
      else window.removeEventListener('keydown', onKey, true)
    },
    { immediate: true },
  )
  onBeforeUnmount(() => window.removeEventListener('keydown', onKey, true))
}
