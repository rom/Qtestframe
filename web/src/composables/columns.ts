// A table's column layout stored per browser (visible optional columns and
// their order), with header drag & drop (PrimeVue reorderableColumns) and
// the ColumnsMenu for keyboard users.
import { computed, ref, shallowRef, watch, type ComputedRef } from 'vue'
import { moveColumn, normalizeOrder, orderFromKeys, toggleColumn } from '@/lib/columnLayout'
import { readPref, writePref } from '@/stores/ui'

export interface ColumnDef {
  field: string
  header: string
  width?: string
  sortable?: boolean
  /** a field name, or a function of the row (e.g. a rank to sort levels by) */
  sortField?: string | ((item: any) => string)
}

/** PrimeVue DataTable instance: d_columnOrder is its own copy of the order. */
type Table = { d_columnOrder?: (string | null)[] | null }

export function useColumnLayout(storageKey: () => string, available: ComputedRef<ColumnDef[]>, defaults: () => string[], rename: Record<string, string> = {}) {
  const ids = computed(() => available.value.map((c) => c.field))
  const load = () => {
    let saved: unknown = null
    try {
      saved = JSON.parse(readPref(storageKey()) || 'null')
    } catch {
      saved = null
    }
    return normalizeOrder(saved, ids.value, defaults(), rename)
  }
  const order = ref<string[]>(load())
  watch(storageKey, () => (order.value = load()))
  watch(order, (v) => writePref(storageKey(), JSON.stringify(v)), { deep: true })
  const visible = computed(() => order.value.map((id) => available.value.find((c) => c.field === id)).filter((c): c is ColumnDef => !!c))
  // the DataTable (bind with :ref="layout.bind"); our order is the source of
  // truth, so the table's own copy is cleared after every change
  const table = shallowRef<Table | null>(null)
  const sync = () => {
    if (table.value) table.value.d_columnOrder = null
  }
  return {
    order,
    visible,
    bind: (el: unknown) => (table.value = (el as Table | null) ?? null),
    toggle: (id: string, on: boolean) => {
      order.value = toggleColumn(order.value, id, on)
      sync()
    },
    move: (id: string, delta: number) => {
      order.value = moveColumn(order.value, id, delta)
      sync()
    },
    reset: () => {
      order.value = defaults().filter((d) => ids.value.includes(d))
      sync()
    },
    /** handler for the DataTable's column-reorder event (header drag) */
    onReorder() {
      if (!table.value) return
      order.value = orderFromKeys(table.value.d_columnOrder ?? [], order.value)
      sync()
    },
  }
}
