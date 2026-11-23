<script setup lang="ts">
// The project dashboards: the shared ones (the built-in overview, trends and
// quality layouts until an administrator changes them, and further shared
// dashboards) and the user's personal ones, as tabs (?tab= keeps the one
// shown). "Customise" edits the layout; a shared dashboard the user may not
// change is saved as a personal copy.
import { computed, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import Skeleton from 'primevue/skeleton'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Checkbox from 'primevue/checkbox'
import Tag from 'primevue/tag'
import DashboardBoard from '@/components/dashboard/DashboardBoard.vue'
import { api } from '@/lib/api'
import { saveBlob } from '@/lib/download'
import { ImportError, dashboardName, exportDashboard, exportFileName, parseImport, type Dashboard, type DashboardList } from '@/lib/dashboards'
import { sameLayout } from '@/lib/dashboardGrid'
import type { DashboardWidget } from '@/lib/widgets'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { useLiveRefresh } from '@/composables/live'
import { refreshDashboards } from '@/composables/widgetData'

const proj = useProjectStore()
// live updates: changes in the project load the widgets on screen again
useLiveRefresh(() => (proj.pid ? { projectId: proj.pid } : null), () => refreshDashboards())
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()

const list = ref<DashboardList | null>(null)
const loading = ref(true)
const tabKey = (d: Dashboard) => (d.shared && d.builtin ? d.builtin : d.id)
const name = (d: Dashboard) => dashboardName(d, (k) => t(`dashboards.builtin.${k}`))
const current = computed(() => {
  const items = list.value?.items ?? []
  const want = String(route.query.tab ?? '')
  return items.find((d) => tabKey(d) === want || d.id === want) ?? items.find((d) => d.default) ?? items[0] ?? null
})
const section = computed({
  get: () => (current.value ? tabKey(current.value) : ''),
  set: (v: string) => select(v),
})
function select(key: string) {
  if (editing.value && dirty.value) {
    notify.confirmDanger(t('dashboards.discardConfirm'), () => {
      stopEditing()
      select(key)
    }, t('dashboards.unsaved'), t('dashboards.discard'))
    return
  }
  stopEditing()
  const def = list.value?.items.find((d) => d.default)
  const isDefault = !!def && tabKey(def) === key
  if ((route.query.tab ?? '') !== key) void router.replace({ query: { ...route.query, tab: isDefault ? undefined : key } })
}

async function load() {
  loading.value = true
  try {
    list.value = await api.get<DashboardList>(`/projects/${proj.pid}/dashboards`)
  } catch (e) {
    notify.fail(e, t('dashboard.loadFailed'))
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => proj.pid, load)

// ---------- editing ----------
const editing = ref(false)
const draft = ref<DashboardWidget[]>([])
const board = ref<InstanceType<typeof DashboardBoard> | null>(null)
const shownWidgets = computed({
  get: () => (editing.value ? draft.value : (current.value?.widgets ?? [])),
  set: (v: DashboardWidget[]) => (draft.value = v),
})
const dirty = computed(() => editing.value && !!current.value && (!sameLayout(draft.value, current.value.widgets) || JSON.stringify(draft.value) !== JSON.stringify(current.value.widgets)))
function startEditing() {
  if (!current.value) return
  draft.value = current.value.widgets.map((w) => ({ ...w, settings: w.settings ? { ...w.settings } : undefined }))
  editing.value = true
}
function stopEditing() {
  editing.value = false
  draft.value = []
}
onBeforeRouteLeave(() => !dirty.value || window.confirm(t('dashboards.discardConfirm')))

function replaceItem(d: Dashboard, oldId?: string) {
  if (!list.value) return
  const items = [...list.value.items]
  const i = items.findIndex((x) => x.id === (oldId ?? d.id))
  if (i >= 0) items[i] = { ...d, default: items[i].default || d.default }
  else items.push(d)
  list.value = { ...list.value, items }
}
const saving = ref(false)
async function save(asCopy = false) {
  const d = current.value
  if (!d) return
  saving.value = true
  try {
    if (asCopy || !d.editable) {
      const created = await api.post<Dashboard>(`/projects/${proj.pid}/dashboards`, { source_id: d.id, name: t('dashboards.copyName', { name: name(d) }), widgets: draft.value })
      list.value = { ...list.value!, items: [...list.value!.items, created] }
      stopEditing()
      void router.replace({ query: { ...route.query, tab: created.id } })
      notify.ok(t('dashboards.savedCopy'))
    } else {
      const saved = await api.put<Dashboard>(`/projects/${proj.pid}/dashboards/${d.id}`, { widgets: draft.value })
      replaceItem(saved, d.id)
      stopEditing()
      notify.ok(t('dashboards.saved'))
    }
  } catch (e) {
    notify.fail(e, t('dashboards.saveFailed'))
  } finally {
    saving.value = false
  }
}

// ---------- dashboards: new, rename, duplicate, delete, default, reset ----------
const dialog = ref<{ mode: 'new' | 'rename'; name: string; shared: boolean; copy: boolean } | null>(null)
const canManage = computed(() => !!list.value?.can_manage)
function openNew() {
  dialog.value = { mode: 'new', name: '', shared: false, copy: false }
}
function openRename() {
  if (current.value) dialog.value = { mode: 'rename', name: name(current.value), shared: current.value.shared, copy: false }
}
async function submitDialog() {
  const dl = dialog.value
  if (!dl || !dl.name.trim()) return
  try {
    if (dl.mode === 'new') {
      const body: Record<string, unknown> = { name: dl.name.trim(), shared: dl.shared }
      if (dl.copy && current.value) body.source_id = current.value.id
      const created = await api.post<Dashboard>(`/projects/${proj.pid}/dashboards`, body)
      list.value = { ...list.value!, items: [...list.value!.items, created] }
      dialog.value = null
      await router.replace({ query: { ...route.query, tab: created.id } })
      if (!dl.copy) startEditing()
    } else if (current.value) {
      const saved = await api.put<Dashboard>(`/projects/${proj.pid}/dashboards/${current.value.id}`, { name: dl.name.trim() })
      replaceItem(saved, current.value.id)
      dialog.value = null
      if (route.query.tab) void router.replace({ query: { ...route.query, tab: tabKey(saved) } })
    }
  } catch (e) {
    notify.fail(e)
  }
}
function remove() {
  const d = current.value
  if (!d) return
  notify.confirmDanger(t('dashboards.deleteConfirm', { name: name(d) }), async () => {
    try {
      await api.del(`/projects/${proj.pid}/dashboards/${d.id}`)
      await load()
      void router.replace({ query: { ...route.query, tab: undefined } })
    } catch (e) {
      notify.fail(e)
    }
  })
}
async function makeDefault() {
  const d = current.value
  if (!d) return
  try {
    list.value = await api.put<DashboardList>(`/projects/${proj.pid}/dashboards/default`, { dashboard_id: d.id })
    void router.replace({ query: { ...route.query, tab: undefined } })
    notify.ok(t('dashboards.defaultSet', { name: name(d) }))
  } catch (e) {
    notify.fail(e)
  }
}
function reset() {
  const d = current.value
  if (!d) return
  notify.confirmDanger(
    t(d.shared ? 'dashboards.resetSharedConfirm' : 'dashboards.resetConfirm', { name: name(d) }),
    async () => {
      try {
        const r = await api.post<Dashboard>(`/projects/${proj.pid}/dashboards/${d.id}/reset`)
        replaceItem(r, d.id)
        stopEditing()
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
  const d = current.value
  if (!d) return
  const data = exportDashboard(editing.value ? { ...d, widgets: draft.value } : d, name(d))
  saveBlob(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }), exportFileName(name(d)))
}
const fileInput = ref<HTMLInputElement | null>(null)
async function importJson(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    const parsed = parseImport(await file.text())
    const created = await api.post<Dashboard>(`/projects/${proj.pid}/dashboards`, {
      format: 'qtestframe-dashboard',
      version: 1,
      name: parsed.name || file.name.replace(/\.json$/i, ''),
      widgets: parsed.widgets,
    })
    list.value = { ...list.value!, items: [...list.value!.items, created] }
    void router.replace({ query: { ...route.query, tab: created.id } })
    notify.ok(t('dashboards.imported', { name: created.name }), parsed.skipped ? t('dashboards.importSkipped', { n: parsed.skipped }, parsed.skipped) : undefined)
  } catch (err) {
    if (err instanceof ImportError) notify.failText(t('dashboards.importFailed'), t(`dashboards.import.${err.key}`, err.params))
    else notify.fail(err, t('dashboards.importFailed'))
  }
}

const menu = ref<InstanceType<typeof Menu> | null>(null)
const menuItems = computed(() => {
  const d = current.value
  const items: { label: string; icon: string; command: () => void; class?: string }[] = [
    { label: t('dashboards.new'), icon: 'pi pi-plus', command: openNew },
    { label: t('dashboards.import.action'), icon: 'pi pi-upload', command: () => fileInput.value?.click() },
  ]
  if (!d) return items
  items.push({ label: t('dashboards.export'), icon: 'pi pi-download', command: exportJson })
  if (d.editable && (d.stored || !d.shared)) items.push({ label: t('dashboards.rename'), icon: 'pi pi-pencil', command: openRename })
  if (canManage.value && d.shared && !d.default) items.push({ label: t('dashboards.makeDefault'), icon: 'pi pi-star', command: makeDefault })
  if (d.editable && d.stored && (d.builtin || d.source_id || !d.shared)) items.push({ label: t('dashboards.reset'), icon: 'pi pi-undo', command: reset })
  if (d.editable && d.stored) items.push({ label: t('dashboards.delete'), icon: 'pi pi-trash', command: remove, class: 'danger-item' })
  return items
})
const uid = computed(() => `db-${current.value?.id ?? 'none'}`)
const refresh = () => board.value?.refresh()
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('dashboard.title') }}</h1>
      <div class="sub">{{ proj.project?.description }}</div>
    </div>
    <div class="actions">
      <template v-if="!editing">
        <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="refresh" />
        <Button v-if="current" icon="pi pi-th-large" :label="t('dashboards.customise')" severity="secondary" outlined data-test="customise" @click="startEditing" />
        <Button icon="pi pi-ellipsis-v" text rounded severity="secondary" :aria-label="t('dashboards.more')" aria-haspopup="true" aria-controls="dash-menu" @click="menu?.toggle($event)" />
        <Menu id="dash-menu" ref="menu" :model="menuItems" popup />
      </template>
      <template v-else>
        <Button icon="pi pi-plus" :label="t('dashboards.addWidget')" severity="secondary" outlined data-test="add-widget" @click="board?.openAdd()" />
        <Button icon="pi pi-align-justify" :label="t('dashboards.tidy')" severity="secondary" text @click="board?.tidyUp()" />
        <Button :label="t('common.cancel')" severity="secondary" text @click="stopEditing" />
        <Button v-if="current?.editable && current.shared" :label="t('dashboards.saveCopy')" severity="secondary" outlined :loading="saving" @click="save(true)" />
        <Button icon="pi pi-check" :label="current?.editable ? t('dashboards.save') : t('dashboards.saveCopy')" :loading="saving" data-test="save-dashboard" @click="save(false)" />
      </template>
      <input ref="fileInput" type="file" accept="application/json,.json" class="sr-only" tabindex="-1" aria-hidden="true" @change="importJson" />
    </div>
  </div>

  <Tabs v-if="list && list.items.length > 1" id="dash-tabs" v-model:value="section" class="editor-tabs dash-tabs" scrollable>
    <TabList :aria-label="t('dashboards.list')">
      <Tab v-for="d in list.items" :key="d.id" :value="tabKey(d)">
        <i :class="d.shared ? (d.builtin === 'trends' ? 'pi pi-chart-line' : d.builtin === 'quality' ? 'pi pi-verified' : 'pi pi-th-large') : 'pi pi-user'" aria-hidden="true" />
        {{ name(d) }}
        <span v-if="d.default" class="sr-only">({{ t('dashboards.isDefault') }})</span>
        <i v-if="d.default" class="pi pi-star-fill default-star" aria-hidden="true" v-tooltip="t('dashboards.isDefault')" />
      </Tab>
    </TabList>
  </Tabs>

  <p v-if="editing && current" class="edit-note small" role="status">
    <Tag :value="current.shared ? t('dashboards.sharedTag') : t('dashboards.personalTag')" :severity="current.shared ? 'info' : 'secondary'" />
    {{ current.editable ? (current.shared ? t('dashboards.editingShared') : t('dashboards.editingPersonal')) : t('dashboards.editingCopy') }}
  </p>

  <div v-if="loading && !list" class="grid" aria-busy="true" :aria-label="t('common.loading')">
    <div class="kpis"><Skeleton v-for="i in 5" :key="i" height="78px" /></div>
    <div class="charts"><Skeleton v-for="i in 4" :key="i" height="280px" /></div>
  </div>
  <div v-else-if="current" :id="list && list.items.length > 1 ? `dash-tabs_tabpanel_${section}` : undefined" :role="list && list.items.length > 1 ? 'tabpanel' : undefined" :aria-labelledby="list && list.items.length > 1 ? `dash-tabs_tab_${section}` : undefined">
    <DashboardBoard ref="board" v-model:widgets="shownWidgets" :editing="editing" :project-id="proj.pid" :uid="uid" />
  </div>

  <Dialog :visible="!!dialog" modal :header="dialog?.mode === 'rename' ? t('dashboards.rename') : t('dashboards.new')" :style="{ width: 'min(440px, calc(100vw - 16px))' }" @update:visible="(v: boolean) => !v && (dialog = null)">
    <form v-if="dialog" class="grid" style="gap: 12px" @submit.prevent="submitDialog">
      <div class="field">
        <label for="dash-name">{{ t('dashboards.name') }}</label>
        <InputText id="dash-name" v-model="dialog.name" maxlength="100" class="w-full" autofocus />
      </div>
      <template v-if="dialog.mode === 'new'">
        <div v-if="canManage" class="row" style="gap: 8px">
          <Checkbox v-model="dialog.shared" input-id="dash-shared" binary />
          <label for="dash-shared">{{ t('dashboards.sharedLabel') }}</label>
        </div>
        <div v-if="current" class="row" style="gap: 8px">
          <Checkbox v-model="dialog.copy" input-id="dash-copy" binary />
          <label for="dash-copy">{{ t('dashboards.copyFrom', { name: name(current) }) }}</label>
        </div>
      </template>
      <div class="row" style="justify-content: flex-end; gap: 8px">
        <Button type="button" :label="t('common.cancel')" severity="secondary" outlined @click="dialog = null" />
        <Button type="submit" :label="dialog.mode === 'rename' ? t('dashboards.rename') : t('dashboards.create')" :disabled="!dialog.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.dash-tabs { margin-bottom: 12px; }
.default-star { font-size: 10px; margin-left: 4px; color: var(--p-yellow-500, #eab308); }
.edit-note { display: flex; align-items: center; gap: 8px; margin: 0 0 12px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
</style>
