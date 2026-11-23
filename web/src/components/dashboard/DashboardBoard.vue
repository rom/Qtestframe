<script setup lang="ts">
// One dashboard: its grid of widgets, their data (loaded when they come near
// the screen, in batches: composables/widgetData.ts) and, in edit mode,
// adding, configuring and removing widgets. The layout being edited is the
// v-model; the views save it.
import { computed, ref, watch, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import DashboardGrid from './DashboardGrid.vue'
import WidgetBody from './WidgetBody.vue'
import WidgetSettingsDialog from './WidgetSettingsDialog.vue'
import AddWidgetDialog from './AddWidgetDialog.vue'
import { useWidgetLoader } from '@/composables/widgetData'
import { resolve, tidy } from '@/lib/dashboardGrid'
import { dataRequest, loadsData, newWidget, setting, type DashboardWidget } from '@/lib/widgets'

const props = defineProps<{
  widgets: DashboardWidget[]
  editing: boolean
  /** the project ('' : the home dashboard, widgets name theirs) */
  projectId: string
  uid: string
}>()
const emit = defineEmits<{ 'update:widgets': [w: DashboardWidget[]] }>()
const { t, te } = useI18n()
const home = computed(() => !props.projectId)

// ---------- titles ----------
/** The titles of the former fixed dashboard. */
const LEGACY: Record<string, string> = {
  key_figures: 'dashboard.kpis',
  coverage: 'dashboard.requirementCoverage',
  execution_results: 'dashboard.executionResults',
  active_runs: 'dashboard.activeRuns',
  recent_runs: 'dashboard.recentRuns',
  test_types: 'dashboard.testTypes',
  release_progress: 'dashboard.releaseProgress',
  test_results_trend: 'dashboard.trendTitle',
  recent_activity: 'dashboard.activity',
  defects_per_week: 'dashboard.defectsPerWeek',
  release_burnup: 'dashboard.burnup',
  correlations: 'correlations.dashboardTitle',
  quality_metrics: 'quality.title',
  consistency: 'consistency.title',
  flaky_tests: 'dashboard.flakyTests',
  coverage_by_group: 'dashboard.coverageByGroup',
  risk_heat: 'dashboard.riskHeat',
  defect_age: 'dashboard.defectAge',
  trace_rules: 'dashboard.traceRules',
  my_work: 'workList.widgetTitle',
}
const LEGACY_DIM: Record<string, string> = {
  requirement_status: 'dashboard.requirementsByStatus',
  requirement_obligation: 'dashboard.requirementsByObligation',
  test_status: 'dashboard.testsByStatus',
  review_state: 'dashboard.reviewState',
  defect_severity: 'dashboard.defectsBySeverity',
  risk_level: 'dashboard.risksByLevel',
  risk_mitigation: 'dashboard.mitigationTitle',
}
function defaultTitle(w: DashboardWidget): string {
  if (w.type === 'breakdown') {
    const dim = setting<string>(w, 'dimension')
    return LEGACY_DIM[dim] ? t(LEGACY_DIM[dim]) : t(`dashboards.dimension.${dim}`)
  }
  if (LEGACY[w.type]) return t(LEGACY[w.type])
  return te(`dashboards.types.${w.type}`) ? t(`dashboards.types.${w.type}`) : w.type
}
const titleOf = (w: DashboardWidget) => w.title || defaultTitle(w)

// ---------- data ----------
const loader = useWidgetLoader()
const visible = ref(new Set<string>())
const overrides = ref<Record<string, Record<string, unknown>>>({})
const projectOf = (w: DashboardWidget) => props.projectId || String(w.settings?.project_id ?? '')
const requests = computed(() => new Map(props.widgets.map((w) => [w.id, dataRequest(w, props.projectId || undefined, overrides.value[w.id])])))
watchEffect(() => {
  for (const w of props.widgets) {
    if (!visible.value.has(w.id) || !loadsData(w.type)) continue
    if (WIDGET_NEEDS_PROJECT(w) && !projectOf(w)) continue
    const r = requests.value.get(w.id)!
    loader.request(r.key, r.body)
  }
})
function WIDGET_NEEDS_PROJECT(w: DashboardWidget) {
  return w.type !== 'my_work' && w.type !== 'discussions'
}
const entryOf = (w: DashboardWidget) => loader.entries.get(requests.value.get(w.id)!.key)
function onVisible(id: string) {
  visible.value = new Set([...visible.value, id])
}
function setOverride(id: string, key: string, value: unknown) {
  overrides.value = { ...overrides.value, [id]: { ...(overrides.value[id] ?? {}), [key]: value } }
}
const grid = ref<InstanceType<typeof DashboardGrid> | null>(null)
// another dashboard: start over
watch(
  () => props.uid,
  () => {
    visible.value = new Set()
    overrides.value = {}
    grid.value?.reset()
  },
)

/** Loads the widgets again (refresh button; live updates: refreshDashboards). */
function refresh() {
  loader.refresh()
}

// ---------- editing ----------
const adding = ref(false)
const configuring = ref<DashboardWidget | null>(null)
function update(list: DashboardWidget[]) {
  emit('update:widgets', list)
}
function add(type: string) {
  const w = newWidget(type, props.widgets)
  update([...props.widgets, w])
  // open the settings of widgets that need them
  if (home.value && WIDGET_NEEDS_PROJECT(w) && type !== 'note') configuring.value = w
}
function remove(w: DashboardWidget) {
  update(props.widgets.filter((x) => x.id !== w.id))
}
function saveSettings(w: DashboardWidget) {
  const list = props.widgets.map((x) => (x.id === w.id ? w : x))
  // a larger widget pushes the others down
  update(resolve(list, w.id))
  const { [w.id]: _drop, ...rest } = overrides.value
  overrides.value = rest
  configuring.value = null
}
function tidyUp() {
  update(tidy(props.widgets))
}
defineExpose({ refresh, openAdd: () => (adding.value = true), tidyUp, titleOf })
</script>

<template>
  <div>
    <div v-if="!widgets.length" class="card empty">
      <i class="pi pi-th-large" aria-hidden="true" />
      {{ editing ? t('dashboards.emptyEditing') : t('dashboards.empty') }}
    </div>
    <DashboardGrid
      ref="grid"
      :items="widgets"
      :editing="editing"
      :title-of="titleOf"
      :uid="uid"
      @update:items="update"
      @visible="onVisible"
      @configure="configuring = $event"
      @remove="remove"
    >
      <template #default="{ item }">
        <WidgetBody
          :widget="item"
          :entry="entryOf(item)"
          :project-id="projectOf(item)"
          :overrides="overrides[item.id]"
          :tick="loader.tick.value"
          :uid="`${uid}-${item.id}`"
          @override="(k, v) => setOverride(item.id, k, v)"
        />
      </template>
    </DashboardGrid>
    <AddWidgetDialog v-model:visible="adding" @add="add" />
    <WidgetSettingsDialog :widget="configuring" :project-id="projectId" :home="home" :default-title="defaultTitle" @save="saveSettings" @close="configuring = null" />
  </div>
</template>
