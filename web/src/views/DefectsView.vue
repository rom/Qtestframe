<script setup lang="ts">
// Defects of the project: filter, report and edit them, and mirror them in
// the project's Jira or GitHub Issues (push, sync, import).
import { useLiveRefresh } from '@/composables/live'
import { computed, onMounted, ref, watch } from 'vue'
import { customCellText, customColumns } from '@/lib/customColumns'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import Message from 'primevue/message'
import QueryInput from '@/components/QueryInput.vue'
import DefectTag from '@/components/DefectTag.vue'
import DefectFormDialog from '@/components/DefectFormDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import MentionInput from '@/components/MentionInput.vue'
import { valuesBefore, type PushResult } from '@/lib/bulkRuns'
import { entry, recordUndo } from '@/lib/undoActions'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { and, anyOf } from '@/lib/query'
import {
  canPush,
  canSync,
  compareDefects,
  DEFECT_SEVERITIES,
  DEFECT_STATUSES,
  findImported,
  issueKeyExample,
  keyNumber,
  linkRoute,
  severityRank,
  statusRank,
  syncResult,
  trackerError,
  trackerName as trackerNameOf,
  validIssueKey,
  type SyncResult,
} from '@/lib/defectTracker'
import { formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { Defect, DefectLink, DefectTracker } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const prefs = usePrefsStore()
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

/** custom fields shown in lists (show_in_list) as columns */
const listedCols = computed(() => customColumns(useProjectStore().customFieldsFor('defect').filter((f) => f.show_in_list)))
const cellOf = (row: { custom_fields?: Record<string, unknown> }, field: string) => customCellText(useProjectStore().customFieldsFor('defect'), row, field, [t('common.yes'), t('common.no')])

const canReport = computed(() => auth.can(P.runExecute))
const canDelete = computed(() => auth.can(P.projectUpdate))

// ---------- list ----------
const defects = ref<Defect[]>([])
const loading = ref(false)
const q = ref('')
const statuses = ref<string[]>([])
const severities = ref<string[]>([])
const queryError = ref<unknown>(null)
const composed = computed(() => and(q.value, anyOf('status', statuses.value), anyOf('severity', severities.value)))
const statusOptions = computed(() => DEFECT_STATUSES.map((v) => ({ value: v, label: t(`defectsView.status.${v}`) })))
const severityOptions = computed(() => DEFECT_SEVERITIES.map((v) => ({ value: v, label: t(`defectsView.severity.${v}`) })))

/** set while reloading for live updates: no spinner (keeps selection and scroll) */
let quiet = false
async function load() {
  if (!quiet) loading.value = true
  queryError.value = null
  try {
    defects.value = asItems(await api.get<{ items: Defect[] }>(`/projects/${proj.pid}/defects`, { q: composed.value })).sort(compareDefects)
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
// live updates: reload quietly when others change what is shown
useLiveRefresh(
  () => ({ types: ['defect'], projectId: proj.pid }),
  () => {
    quiet = true
    const done = load()
    quiet = false
    return done
  }, { announce: () => t('live.listUpdated') },
)

// ---------- tracker ----------
const tracker = ref<DefectTracker | null>(null)
const trackerType = computed(() => tracker.value?.type ?? '')
const trackerName = computed(() => trackerNameOf(trackerType.value))
async function loadTracker() {
  try {
    tracker.value = await api.get<DefectTracker>(`/projects/${proj.pid}/defect-tracker`)
  } catch {
    tracker.value = null
  }
}
/** the last error the tracker reported (502 tracker_error), shown until dismissed */
const trackerMessage = ref('')
function failTracker(e: unknown) {
  const m = trackerError(e)
  if (m) trackerMessage.value = m
  else notify.fail(e)
}
const busy = ref<string | null>(null)
function replace(d: Defect) {
  const i = defects.value.findIndex((x) => x.id === d.id)
  if (i >= 0) defects.value[i] = d
}
async function push(d: Defect) {
  busy.value = d.id
  try {
    const res = await api.post<Defect>(`/defects/${d.id}/push`)
    replace(res)
    trackerMessage.value = ''
    notify.ok(t('defectsView.pushed', { key: res.external_key }), d.key)
  } catch (e) {
    failTracker(e)
  } finally {
    busy.value = null
  }
}
async function sync(d: Defect) {
  busy.value = d.id
  try {
    const res = await api.post<Defect>(`/defects/${d.id}/sync`)
    replace(res)
    trackerMessage.value = ''
    notify.ok(res.status !== d.status ? t('defectsView.syncedChanged', { status: t(`defectsView.status.${res.status}`) }) : t('defectsView.syncedSame'), d.key)
  } catch (e) {
    failTracker(e)
  } finally {
    busy.value = null
  }
}
const syncing = ref(false)
const lastSync = ref<SyncResult | null>(null)

// ---------- bulk ----------
const selection = ref<Defect[]>([])
const canBulk = computed(() => canReport.value || canDelete.value)
const selectedIds = computed(() => selection.value.map((d) => d.id))
const bulkStatus = ref<string | null>(null)
const bulkSeverity = ref<string | null>(null)
const assignDialog = ref(false)
const bulkAssignee = ref('')
const bulkBusy = ref('')
watch(defects, (list) => {
  const ids = new Set(list.map((d) => d.id))
  selection.value = selection.value.filter((s) => ids.has(s.id))
})
async function bulkSet(set: Partial<Pick<Defect, 'status' | 'severity' | 'assignee'>>) {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  const before = valuesBefore(defects.value, ids, Object.keys(set) as (keyof Defect & string)[])
  const path = `/projects/${proj.pid}/defects/bulk`
  try {
    await api.post(path, { ids, set })
    recordUndo(
      entry(
        t('defectsView.bulk.undo', { n: ids.length }),
        async () => {
          for (const b of before) await api.patch(`/defects/${b.id}`, b.before)
        },
        () => api.post(path, { ids, set }),
      ),
    )
    notify.ok(t('defectsView.bulk.updated', { n: ids.length }, ids.length))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
watch(bulkStatus, (v) => v && bulkSet({ status: v }).then(() => (bulkStatus.value = null)))
watch(bulkSeverity, (v) => v && bulkSet({ severity: v }).then(() => (bulkSeverity.value = null)))
function openAssign() {
  bulkAssignee.value = ''
  assignDialog.value = true
}
async function applyAssign() {
  assignDialog.value = false
  await bulkSet({ assignee: bulkAssignee.value.trim() })
}
async function bulkPush() {
  bulkBusy.value = 'push'
  try {
    const res = await api.post<PushResult>(`/projects/${proj.pid}/defects/push`, { ids: selectedIds.value })
    trackerMessage.value = ''
    if (res.failed.length) {
      lastSync.value = { synced: res.pushed.length, changed: 0, failed: res.failed }
      notify.failText(t('defectsView.bulk.pushFailed', { n: res.failed.length }, res.failed.length), res.failed.map((f) => `${f.key}: ${f.error}`).join('\n'))
    } else notify.ok(t('defectsView.bulk.pushed', { n: res.pushed.length }, res.pushed.length), res.skipped.length ? t('defectsView.bulk.alreadyLinked', { n: res.skipped.length }, res.skipped.length) : undefined)
    await load()
  } catch (e) {
    failTracker(e)
  } finally {
    bulkBusy.value = ''
  }
}
async function bulkSync() {
  bulkBusy.value = 'sync'
  lastSync.value = null
  try {
    const res = syncResult(await api.post<SyncResult>(`/projects/${proj.pid}/defects/sync`, { ids: selectedIds.value }))
    lastSync.value = res
    trackerMessage.value = ''
    notify.ok(t('defectsView.syncAllDone', { synced: res.synced, changed: res.changed }, res.synced))
    await load()
  } catch (e) {
    failTracker(e)
  } finally {
    bulkBusy.value = ''
  }
}
function bulkDelete() {
  const ids = [...selectedIds.value]
  notify.confirmDanger(t('defectsView.bulk.confirmDelete', { n: ids.length }, ids.length), async () => {
    try {
      await api.del(`/projects/${proj.pid}/defects/bulk`, { ids })
      notify.ok(t('defectsView.bulk.deleted', { n: ids.length }, ids.length))
      selection.value = []
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
async function syncAll() {
  syncing.value = true
  lastSync.value = null
  try {
    const res = syncResult(await api.post<SyncResult>(`/projects/${proj.pid}/defects/sync`))
    lastSync.value = res
    trackerMessage.value = ''
    notify.ok(t('defectsView.syncAllDone', { synced: res.synced, changed: res.changed }, res.synced))
    await load()
  } catch (e) {
    failTracker(e)
  } finally {
    syncing.value = false
  }
}

// ---------- import ----------
const importVisible = ref(false)
const importForm = ref({ key: '', severity: 'major' })
const importError = ref('')
const importing = ref(false)
const importExample = computed(() => issueKeyExample(tracker.value))
const importValid = computed(() => validIssueKey(tracker.value, importForm.value.key))
function openImport() {
  importForm.value = { key: '', severity: 'major' }
  importError.value = ''
  importVisible.value = true
}
async function doImport() {
  if (!importValid.value) {
    importError.value = t('defectsView.importBadKey', { example: importExample.value })
    return
  }
  importing.value = true
  importError.value = ''
  try {
    const all = asItems(await api.get<{ items: Defect[] }>(`/projects/${proj.pid}/defects`))
    const known = findImported(all, tracker.value, importForm.value.key)
    if (known) {
      importError.value = t('defectsView.alreadyImported', { key: known.key })
      return
    }
    const d = await api.post<Defect>(`/projects/${proj.pid}/defects/import`, { external_key: importForm.value.key.trim(), severity: importForm.value.severity })
    importVisible.value = false
    notify.ok(t('defectsView.imported', { key: d.external_key }), d.key)
    await load()
  } catch (e) {
    const m = trackerError(e)
    importError.value = m ? t('defectsView.trackerSaid', { message: m }) : errorMessage(e)
  } finally {
    importing.value = false
  }
}

// ---------- create / edit ----------
const formVisible = ref(false)
const editing = ref<Defect | null>(null)
function openNew() {
  editing.value = null
  formVisible.value = true
}
async function openEdit(d: Defect) {
  editing.value = d
  formVisible.value = true
}
function onSaved(d: Defect) {
  notify.ok(editing.value ? t('defectsView.saved') : t('defectsView.created'), d.key)
  void load()
}
// ?item=<id> or ?defect=<id> (panels, notifications, release status, /go/PROJ-BUG-n) opens the defect
async function openFromRoute() {
  const q = route.query.item ?? route.query.defect
  const id = typeof q === 'string' ? q : ''
  if (!id) return
  try {
    await openEdit(await api.get<Defect>(`/defects/${id}`))
  } catch (e) {
    notify.fail(e)
  }
}
watch(formVisible, (v) => {
  if (!v && (route.query.item || route.query.defect)) void router.replace({ query: { ...route.query, item: undefined, defect: undefined } })
})
watch(() => route.query.item ?? route.query.defect, (v) => v && void openFromRoute())

function remove(d: Defect) {
  notify.confirmDanger(t('defectsView.confirmDelete', { key: d.key }), async () => {
    try {
      await api.del(`/defects/${d.id}`)
      notify.ok(t('defectsView.deleted'), d.key)
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

const linkIcon = (l: DefectLink) =>
  l.entity_type === 'requirement' ? 'pi pi-list-check' : l.entity_type === 'test' ? 'pi pi-check-square' : l.entity_type === 'run' ? 'pi pi-play' : l.entity_type === 'exploratory_session' ? 'pi pi-compass' : 'pi pi-play-circle'
const pad = (n: number) => String(n).padStart(8, '0')
const sortKey = (d: Defect) => pad(keyNumber(d.key))
const sortStatus = (d: Defect) => pad(statusRank(d.status))
const sortSeverity = (d: Defect) => pad(severityRank(d.severity))

onMounted(() => {
  void load()
  void loadTracker()
  void openFromRoute()
})
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('titles.defects') }}</h1>
      <div class="sub">
        {{ t('defectsView.subtitle') }}
        <span v-if="trackerName"> · {{ t('defectsView.connectedTo', { tracker: trackerName }) }}</span>
      </div>
    </div>
    <div class="actions">
      <HelpLink page="defects" />
      <template v-if="canReport && trackerType">
        <Button :label="t('defectsView.syncAll')" icon="pi pi-sync" severity="secondary" outlined :loading="syncing" @click="syncAll" />
        <Button :label="t('defectsView.import')" icon="pi pi-download" severity="secondary" outlined @click="openImport" />
      </template>
      <Button v-if="canReport" :label="t('defectsView.new')" icon="pi pi-plus" @click="openNew" />
    </div>
  </div>

  <Message v-if="trackerMessage" severity="error" closable role="alert" style="margin-bottom: 12px" @close="trackerMessage = ''">
    <b>{{ t('defectsView.trackerErrorTitle', { tracker: trackerName || t('defectsView.theTracker') }) }}</b>
    <div class="mono small" style="overflow-wrap: anywhere">{{ trackerMessage }}</div>
    <div class="small">{{ t('defectsView.trackerErrorHint') }}</div>
  </Message>
  <Message v-if="lastSync?.failed.length" severity="warn" closable style="margin-bottom: 12px" @close="lastSync = null">
    <b>{{ t('defectsView.syncFailed', { count: lastSync.failed.length }, lastSync.failed.length) }}</b>
    <ul style="margin: 4px 0 0; padding-left: 18px">
      <li v-for="f in lastSync.failed" :key="f.key" class="small"><span class="key">{{ f.key }}</span> {{ f.error }}</li>
    </ul>
  </Message>
  <Message v-if="tracker && !trackerType && canDelete" severity="info" :closable="false" style="margin-bottom: 12px">
    {{ t('defectsView.noTracker') }}
    <RouterLink :to="{ name: 'settings', params: { projectId: proj.pid }, query: { tab: 'project' } }">{{ t('defectsView.noTrackerLink') }}</RouterLink>
  </Message>

  <div class="card">
    <div class="filterbar">
      <QueryInput v-model="q" scope="defect" :error="queryError" :composed="composed" :placeholder="t('defectsView.filterPlaceholder', { example: 'open:true linked:SHOP-REQ-*' })" @submit="load" />
      <MultiSelect v-model="statuses" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('common.status')" :aria-label="t('common.status')" style="min-width: 150px" show-clear @change="load" />
      <MultiSelect v-model="severities" :options="severityOptions" option-label="label" option-value="value" :placeholder="t('defectsView.severityLabel')" :aria-label="t('defectsView.severityLabel')" style="min-width: 150px" show-clear @change="load" />
    </div>
    <div v-if="selection.length && canBulk" class="bulkbar" role="toolbar" :aria-label="t('defectsView.bulk.toolbar')">
      <b>{{ t('common.selected', { count: selection.length }) }}</b>
      <template v-if="canReport">
        <Select v-model="bulkStatus" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('defectsView.bulk.setStatus')" :aria-label="t('defectsView.bulk.setStatus')" size="small" style="min-width: 140px" />
        <Select v-model="bulkSeverity" :options="severityOptions" option-label="label" option-value="value" :placeholder="t('defectsView.bulk.setSeverity')" :aria-label="t('defectsView.bulk.setSeverity')" size="small" style="min-width: 150px" />
        <Button :label="t('defectsView.bulk.assign')" icon="pi pi-user" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openAssign" />
        <template v-if="trackerType">
          <Button :label="t('defectsView.bulk.push', { tracker: trackerName })" icon="pi pi-upload" size="small" severity="secondary" outlined :loading="bulkBusy === 'push'" @click="bulkPush" />
          <Button :label="t('defectsView.bulk.sync')" icon="pi pi-sync" size="small" severity="secondary" outlined :loading="bulkBusy === 'sync'" @click="bulkSync" />
        </template>
      </template>
      <Button v-if="canDelete" :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
    </div>
    <DataTable v-model:selection="selection" v-stack :value="defects" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(25)" row-hover class="clickable" @row-click="(e) => openEdit(e.data as Defect)">
      <Column v-if="canBulk" selection-mode="multiple" style="width: 2.6rem" />
      <Column field="key" :header="t('common.key')" sortable :sort-field="sortKey" style="width: 120px">
        <template #body="{ data }">
          <a href="#" class="key" @click.prevent.stop="openEdit(data)">{{ data.key }}</a>
        </template>
      </Column>
      <Column field="title" :header="t('common.title')" sortable body-class="stack-main">
        <template #body="{ data }">
          <div style="min-width: 0">
            <button type="button" class="link-button ellipsis" style="display: block; max-width: 420px" tabindex="-1" :title="data.title" @click.stop="openEdit(data)">{{ data.title }}</button>
            <div class="small muted">{{ formatDateTime(data.created_at) }}<template v-if="data.created_by"> · {{ data.created_by }}</template></div>
          </div>
        </template>
      </Column>
      <Column :header="t('common.status')" sortable :sort-field="sortStatus" style="width: 130px"><template #body="{ data }"><DefectTag :value="data.status" /></template></Column>
      <Column :header="t('defectsView.severityLabel')" sortable :sort-field="sortSeverity" style="width: 130px"><template #body="{ data }"><DefectTag :value="data.severity" kind="severity" /></template></Column>
      <Column field="assignee" :header="t('defectsView.assignee')" sortable style="width: 150px"><template #body="{ data }"><span class="small">{{ data.assignee || '—' }}</span></template></Column>
      <Column v-for="c in listedCols" :key="c.field" :header="c.header" sortable :sort-field="c.sortField" :style="c.width ? { width: c.width } : {}">
        <template #body="{ data }"><span class="small">{{ cellOf(data, c.field) }}</span></template>
      </Column>
      <Column :header="t('defectsView.linkedItems')" style="min-width: 160px">
        <template #body="{ data }">
          <span v-if="data.links?.length" class="row" style="gap: 4px 8px">
            <template v-for="l in data.links as DefectLink[]" :key="l.entity_type + l.entity_id">
              <RouterLink v-if="linkRoute(l, proj.pid)" :to="linkRoute(l, proj.pid)!" class="key small" :title="l.title" @click.stop><i :class="linkIcon(l)" aria-hidden="true" style="font-size: 10px" /> {{ l.key }}</RouterLink>
              <span v-else class="key small" :title="`${t(`defectsView.linkType.${l.entity_type}`)}: ${l.title ?? ''}`"><i :class="linkIcon(l)" aria-hidden="true" style="font-size: 10px" /> {{ l.key }}</span>
            </template>
          </span>
          <span v-else class="muted small">—</span>
        </template>
      </Column>
      <Column :header="t('defectsView.externalIssue')" style="min-width: 150px">
        <template #body="{ data }">
          <div v-if="data.external_key" class="small">
            <a v-if="data.external_url" :href="data.external_url" target="_blank" rel="noopener noreferrer" class="mono" @click.stop>
              {{ data.external_key }}<i class="pi pi-external-link" aria-hidden="true" style="font-size: 0.75em; margin-left: 2px" /><span class="sr-only"> ({{ t('links.newTab') }})</span>
            </a>
            <span v-else class="mono">{{ data.external_key }}</span>
            <div v-if="data.external_status" class="muted">{{ data.external_status }}<template v-if="data.synced_at"> · {{ formatDateTime(data.synced_at) }}</template></div>
          </div>
          <span v-else class="muted small">—</span>
        </template>
      </Column>
      <Column v-if="canReport || canDelete" style="width: 130px">
        <template #body="{ data }">
          <span class="row nowrap" style="gap: 0">
            <Button
              v-if="canReport && trackerType && canPush(data)"
              icon="pi pi-upload"
              text
              rounded
              :loading="busy === data.id"
              :aria-label="t('defectsView.pushNamed', { key: data.key, tracker: trackerName })"
              v-tooltip="t('defectsView.push', { tracker: trackerName })"
              @click.stop="push(data)"
            />
            <Button
              v-if="canReport && canSync(data, trackerType)"
              icon="pi pi-sync"
              text
              rounded
              :loading="busy === data.id"
              :aria-label="t('defectsView.syncNamed', { key: data.key })"
              v-tooltip="t('defectsView.sync')"
              @click.stop="sync(data)"
            />
            <Button v-if="canDelete" icon="pi pi-trash" text rounded severity="danger" :aria-label="t('defectsView.deleteNamed', { key: data.key })" v-tooltip="t('common.delete')" @click.stop="remove(data)" />
          </span>
        </template>
      </Column>
      <template #empty><div class="empty"><i class="pi pi-exclamation-circle" />{{ q || statuses.length || severities.length ? t('defectsView.noneMatching') : t('defectsView.none') }}</div></template>
    </DataTable>
  </div>

  <DefectFormDialog v-model:visible="formVisible" :project-id="proj.pid" :defect="editing" @saved="onSaved" />

  <Dialog v-model:visible="importVisible" :header="t('defectsView.importHeader', { tracker: trackerName })" modal style="width: min(480px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="doImport">
      <p class="small muted" style="margin: 0">{{ t('defectsView.importIntro', { tracker: trackerName }) }}</p>
      <Message v-if="importError" severity="error" :closable="false" role="alert">{{ importError }}</Message>
      <div v-field class="field">
        <label>{{ t('defectsView.issueKey') }}</label>
        <InputText v-model="importForm.key" class="mono" autofocus required :placeholder="importExample" aria-describedby="import-key-hint" />
        <span id="import-key-hint" class="small muted">{{ t('defectsView.issueKeyHint', { example: importExample }) }}</span>
      </div>
      <div v-field class="field"><label>{{ t('defectsView.severityLabel') }}</label><Select v-model="importForm.severity" :options="severityOptions" option-label="label" option-value="value" /></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="importVisible = false" />
        <Button type="submit" :label="t('defectsView.importButton')" icon="pi pi-download" :loading="importing" :disabled="!importForm.key.trim()" />
      </div>
    </form>
  </Dialog>
  <Dialog v-model:visible="assignDialog" :header="t('defectsView.bulk.assignHeader', { n: selection.length }, selection.length)" modal style="width: min(440px, 96vw)">
    <label for="bk-def-assignee" class="block-label">{{ t('defectsView.assignee') }}</label>
    <MentionInput id="bk-def-assignee" v-model="bulkAssignee" :project-id="proj.pid" :placeholder="t('defectsView.bulk.emptyUnassigns')" :aria-label="t('defectsView.assignee')" />
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="assignDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" @click="applyAssign" />
    </template>
  </Dialog>
</template>

<style scoped>
.block-label {
  display: block;
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 4px;
}
</style>
