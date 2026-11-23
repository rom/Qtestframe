<script setup lang="ts">
// The personal cross-project home dashboard: My work, discussions, notes and
// any project widget (each names its project in its settings).
import { computed, onMounted, ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import Skeleton from 'primevue/skeleton'
import DashboardBoard from '@/components/dashboard/DashboardBoard.vue'
import { api } from '@/lib/api'
import { saveBlob } from '@/lib/download'
import { ImportError, dashboardName, exportDashboard, exportFileName, parseImport, type Dashboard } from '@/lib/dashboards'
import type { DashboardWidget } from '@/lib/widgets'
import { useNotify } from '@/composables/notify'
import { useLiveRefresh } from '@/composables/live'
import { refreshDashboards } from '@/composables/widgetData'

// live updates: any visible change loads the widgets on screen again
useLiveRefresh(() => ({}), () => refreshDashboards())

const notify = useNotify()
const { t } = useI18n()
const dash = ref<Dashboard | null>(null)
const loading = ref(true)
const name = computed(() => (dash.value ? dashboardName(dash.value, (k) => t(`dashboards.builtin.${k}`)) : t('dashboards.builtin.home')))

async function load() {
  loading.value = true
  try {
    dash.value = await api.get<Dashboard>('/dashboards/home')
  } catch (e) {
    notify.fail(e, t('dashboard.loadFailed'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

const editing = ref(false)
const draft = ref<DashboardWidget[]>([])
const board = ref<InstanceType<typeof DashboardBoard> | null>(null)
const shownWidgets = computed({
  get: () => (editing.value ? draft.value : (dash.value?.widgets ?? [])),
  set: (v: DashboardWidget[]) => (draft.value = v),
})
const dirty = computed(() => editing.value && JSON.stringify(draft.value) !== JSON.stringify(dash.value?.widgets ?? []))
onBeforeRouteLeave(() => !dirty.value || window.confirm(t('dashboards.discardConfirm')))
function startEditing() {
  draft.value = (dash.value?.widgets ?? []).map((w) => ({ ...w, settings: w.settings ? { ...w.settings } : undefined }))
  editing.value = true
}
const saving = ref(false)
async function save(widgets = draft.value) {
  saving.value = true
  try {
    dash.value = await api.put<Dashboard>('/dashboards/home', { widgets })
    editing.value = false
    notify.ok(t('dashboards.saved'))
  } catch (e) {
    notify.fail(e, t('dashboards.saveFailed'))
  } finally {
    saving.value = false
  }
}
function reset() {
  notify.confirmDanger(
    t('dashboards.resetHomeConfirm'),
    async () => {
      try {
        dash.value = await api.del<Dashboard>('/dashboards/home')
        editing.value = false
        notify.ok(t('dashboards.resetDone'))
      } catch (e) {
        notify.fail(e)
      }
    },
    t('dashboards.reset'),
    t('dashboards.reset'),
  )
}
function exportJson() {
  if (!dash.value) return
  saveBlob(new Blob([JSON.stringify(exportDashboard(dash.value, name.value), null, 2)], { type: 'application/json' }), exportFileName(name.value))
}
const fileInput = ref<HTMLInputElement | null>(null)
async function importJson(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    const parsed = parseImport(await file.text())
    await save(parsed.widgets)
    if (parsed.skipped) notify.info(t('dashboards.importSkipped', { n: parsed.skipped }, parsed.skipped))
  } catch (err) {
    if (err instanceof ImportError) notify.failText(t('dashboards.importFailed'), t(`dashboards.import.${err.key}`, err.params))
    else notify.fail(err, t('dashboards.importFailed'))
  }
}
const menu = ref<InstanceType<typeof Menu> | null>(null)
const menuItems = computed(() => [
  { label: t('dashboards.export'), icon: 'pi pi-download', command: exportJson },
  { label: t('dashboards.importReplace'), icon: 'pi pi-upload', command: () => fileInput.value?.click() },
  ...(dash.value?.stored ? [{ label: t('dashboards.reset'), icon: 'pi pi-undo', command: reset }] : []),
])
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ name }}</h1>
      <div class="sub">{{ t('dashboards.homeIntro') }}</div>
    </div>
    <div class="actions">
      <template v-if="!editing">
        <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="board?.refresh()" />
        <Button icon="pi pi-th-large" :label="t('dashboards.customise')" severity="secondary" outlined data-test="customise" @click="startEditing" />
        <Button icon="pi pi-ellipsis-v" text rounded severity="secondary" :aria-label="t('dashboards.more')" aria-haspopup="true" aria-controls="home-menu" @click="menu?.toggle($event)" />
        <Menu id="home-menu" ref="menu" :model="menuItems" popup />
      </template>
      <template v-else>
        <Button icon="pi pi-plus" :label="t('dashboards.addWidget')" severity="secondary" outlined data-test="add-widget" @click="board?.openAdd()" />
        <Button icon="pi pi-align-justify" :label="t('dashboards.tidy')" severity="secondary" text @click="board?.tidyUp()" />
        <Button :label="t('common.cancel')" severity="secondary" text @click="editing = false" />
        <Button icon="pi pi-check" :label="t('dashboards.save')" :loading="saving" data-test="save-dashboard" @click="save()" />
      </template>
      <input ref="fileInput" type="file" accept="application/json,.json" class="sr-only" tabindex="-1" aria-hidden="true" @change="importJson" />
    </div>
  </div>
  <div v-if="loading && !dash" class="charts" aria-busy="true" :aria-label="t('common.loading')"><Skeleton v-for="i in 3" :key="i" height="240px" /></div>
  <DashboardBoard v-else-if="dash" ref="board" v-model:widgets="shownWidgets" :editing="editing" project-id="" uid="home" />
</template>

<style scoped>
.actions { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
</style>
