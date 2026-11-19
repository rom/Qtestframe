import { computed } from 'vue'
import { useUiStore } from '@/stores/ui'

/** Chart.js base options that follow the light/dark theme. */
export function useChartTheme() {
  const ui = useUiStore()
  return computed(() => {
    const text = ui.dark ? '#cbd5e1' : '#475569'
    const grid = ui.dark ? 'rgba(148,163,184,0.15)' : 'rgba(100,116,139,0.15)'
    return { text, grid, dark: ui.dark }
  })
}
