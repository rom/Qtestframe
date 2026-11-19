import { computed } from 'vue'
import { useUiStore } from '@/stores/ui'
import { chartColors } from '@/themes/chart'

/** Chart.js colours and font of the active (painted) theme. */
export function useChartTheme() {
  const ui = useUiStore()
  return computed(() => chartColors(ui.theme))
}
