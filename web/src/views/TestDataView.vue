<script setup lang="ts">
// Test data sets of the project: named, versioned tables of parameters (or
// files) with a classification, an owner and an expiry date, linked to
// tests (a data-driven test can take its rows from a set), configurations
// and runs; their use by runs, refresh requests and history.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import Textarea from 'primevue/textarea'
import QueryInput from '@/components/QueryInput.vue'
import TagsInput from '@/components/TagsInput.vue'
import AttachmentsPanel from '@/components/AttachmentsPanel.vue'
import HistoryPanel from '@/components/HistoryPanel.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import { uid } from '@/lib/a11y'
import { classificationSeverity, DATA_CLASSIFICATIONS, DATA_SET_KINDS, DATA_SET_STATUSES, expiryState, parseTable, type DataSet, type DataUsage, type RefreshRequest } from '@/lib/environments'
import type { DataRow, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useProjectPeopleStore } from '@/stores/projectPeople'
import { useConfigurations } from '@/composables/configurations'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const people = useProjectPeopleStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const pid = computed(() => proj.pid)
const id = uid('td')
const cfg = useConfigurations(() => proj.pid)
const today = new Date().toISOString().slice(0, 10)
const canWrite = computed(() => auth.can(P.testWrite))
const canAsk = computed(() => auth.can(P.testWrite) || auth.can(P.runExecute))

// ---------- list ----------
const sets = ref<DataSet[]>([])
const loading = ref(false)
const q = ref((route.query.q as string) ?? '')
const queryError = ref<unknown>(null)
async function load() {
  loading.value = true
  queryError.value = null
  try {
    sets.value = asItems(await api.get<{ items: DataSet[] }>(`/projects/${pid.value}/test-data-sets`, { q: q.value || undefined }))
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
const classLabel = (c: string) => t(`dataSets.classification.${c}`)
const personName = (uidv: string) => {
  if (!uidv) return ''
  const p = people.people.find((x) => x.user_id === uidv)
  return p ? p.name || p.email || uidv : uidv
}
const ownerOptions = computed(() => [
  { label: t('environments.noOwner'), value: '' },
  ...people.people.filter((p) => p.kind === 'user' && p.user_id).map((p) => ({ label: p.name && p.email ? `${p.name} (${p.email})` : p.name || p.email || '', value: p.user_id! })),
])

// ---------- the open set ----------
const open = ref<DataSet | null>(null)
const dialog = ref(false)
const tab = ref<'data' | 'links' | 'usage' | 'refresh' | 'files' | 'history'>('data')
const tabsId = uid('td-tabs')
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })
const blank = () => ({ name: '', description: '', kind: 'table', classification: 'synthetic', owner_id: auth.user?.id ?? '', expires_on: '', tags: [] as string[], status: 'active' })
const form = reactive(blank())
const params = ref<string[]>([])
const rows = ref<DataRow[]>([])
const formError = ref('')
const busy = ref(false)
const pasteText = ref('')
const pasteOpen = ref(false)
function fill(d: DataSet | null) {
  Object.assign(form, blank())
  params.value = []
  rows.value = []
  if (d) {
    Object.assign(form, { name: d.name, description: d.description, kind: d.kind, classification: d.classification, owner_id: d.owner_id, expires_on: d.expires_on, tags: [...d.tags], status: d.status })
    params.value = [...d.parameters]
    rows.value = d.data_rows.map((r) => ({ name: r.name, values: { ...r.values } }))
  }
  formError.value = ''
}
function openNew() {
  open.value = null
  fill(null)
  params.value = ['']
  tab.value = 'data'
  dialog.value = true
}
async function openSet(setId: string) {
  try {
    open.value = await api.get<DataSet>(`/test-data-sets/${setId}`)
    fill(open.value)
    tab.value = 'data'
    dialog.value = true
    void loadExtras()
  } catch (e) {
    notify.fail(e)
  }
}
watch(dialog, (v) => {
  if (!v && route.query.set) router.replace({ query: { ...route.query, set: undefined } })
})
function addParam() {
  params.value.push('')
}
function removeParam(i: number) {
  const name = params.value[i]
  params.value.splice(i, 1)
  for (const r of rows.value) delete r.values[name]
}
function addRow() {
  rows.value.push({ name: '', values: {} })
}
function applyPaste() {
  const p = parseTable(pasteText.value)
  if (p.parameters.length) {
    params.value = p.parameters
    rows.value = p.rows
  }
  pasteOpen.value = false
  pasteText.value = ''
}
async function save() {
  busy.value = true
  formError.value = ''
  const body: Record<string, unknown> = { ...form, parameters: form.kind === 'table' ? params.value.map((p) => p.trim()).filter(Boolean) : [], data_rows: form.kind === 'table' ? rows.value : [] }
  try {
    if (open.value) {
      open.value = await api.patch<DataSet>(`/test-data-sets/${open.value.id}`, { ...body, version: open.value.version })
      notify.ok(t('dataSets.saved'), open.value.name)
    } else {
      open.value = await api.post<DataSet>(`/projects/${pid.value}/test-data-sets`, body)
      notify.ok(t('dataSets.created'), open.value.name)
    }
    fill(open.value)
    await load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
function remove(d: DataSet) {
  notify.confirmDanger(t('dataSets.confirmDelete', { name: d.name }), async () => {
    try {
      await api.del(`/test-data-sets/${d.id}`)
      notify.ok(t('dataSets.deleted'), d.name)
      dialog.value = false
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- links, usage, refresh ----------
const usage = ref<DataUsage[]>([])
const refreshes = ref<RefreshRequest[]>([])
const runs = ref<TestRun[]>([])
async function loadExtras() {
  if (!open.value) return
  const sid = open.value.id
  try {
    const [u, r] = await Promise.all([
      api.get<{ items: DataUsage[] }>(`/test-data-sets/${sid}/usage`),
      api.get<{ items: RefreshRequest[] }>(`/test-data-sets/${sid}/refresh-requests`),
    ])
    usage.value = asItems(u)
    refreshes.value = asItems(r)
  } catch {
    usage.value = []
    refreshes.value = []
  }
}
const testPicker = ref(false)
const asSource = ref(false)
const linkCfg = ref<string | null>(null)
const linkRun = ref<string | null>(null)
async function link(item_type: string, item_id: string, usageKind = 'reference') {
  if (!open.value) return
  try {
    open.value = await api.post<DataSet>(`/test-data-sets/${open.value.id}/links`, { item_type, item_id, usage: usageKind })
  } catch (e) {
    notify.fail(e)
  }
}
async function pickTests(items: { id: string }[]) {
  for (const it of items) await link('test', it.id, asSource.value && open.value?.kind === 'table' ? 'source' : 'reference')
}
async function unlink(linkId: string) {
  if (!open.value) return
  try {
    await api.del(`/test-data-sets/${open.value.id}/links/${linkId}`)
    open.value = await api.get<DataSet>(`/test-data-sets/${open.value.id}`)
  } catch (e) {
    notify.fail(e)
  }
}
watch(linkCfg, (v) => v && link('configuration', v).then(() => (linkCfg.value = null)))
watch(linkRun, (v) => v && link('run', v).then(() => (linkRun.value = null)))
watch(tab, async (v) => {
  if (v === 'links' && !runs.value.length) {
    try {
      runs.value = asItems(await api.get<{ items: TestRun[] }>(`/projects/${pid.value}/runs`))
    } catch {
      runs.value = []
    }
  }
})
const runOptions = computed(() => runs.value.map((r) => ({ label: r.name, value: r.id })))
const refreshReason = ref('')
const refreshNeeded = ref('')
async function askRefresh() {
  if (!open.value) return
  try {
    await api.post(`/test-data-sets/${open.value.id}/refresh-requests`, { reason: refreshReason.value, needed_by: refreshNeeded.value })
    refreshReason.value = ''
    refreshNeeded.value = ''
    notify.ok(t('dataSets.refreshAsked'))
    await loadExtras()
  } catch (e) {
    notify.fail(e)
  }
}
async function resolve(r: RefreshRequest, status: 'done' | 'rejected') {
  try {
    await api.post(`/test-data-refresh-requests/${r.id}/resolve`, { status })
    await loadExtras()
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
const canResolve = computed(() => canWrite.value || open.value?.owner_id === auth.user?.id)
const kindOptions = computed(() => DATA_SET_KINDS.map((v) => ({ label: t(`dataSets.kind.${v}`), value: v })))
const classOptions = computed(() => DATA_CLASSIFICATIONS.map((v) => ({ label: classLabel(v), value: v })))
const statusOptions = computed(() => DATA_SET_STATUSES.map((v) => ({ label: t(`dataSets.status.${v}`), value: v })))
const expiry = computed(() => expiryState(form.expires_on, today))

onMounted(async () => {
  void people.ensure(pid.value)
  void cfg.load()
  await load()
  if (typeof route.query.set === 'string') await openSet(route.query.set)
})
watch(
  () => route.query.set,
  (v) => {
    if (typeof v === 'string' && v !== open.value?.id) void openSet(v)
  },
)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('dataSets.title') }}</h1>
      <div class="sub">{{ t('dataSets.subtitle', { n: sets.length }, sets.length) }}</div>
    </div>
    <div class="actions">
      <HelpLink page="test-environments" />
      <Button :label="t('environments.title')" icon="pi pi-server" severity="secondary" text @click="router.push({ name: 'environments', params: { projectId: pid } })" />
      <Button v-if="canWrite" :label="t('dataSets.new')" icon="pi pi-plus" aria-haspopup="dialog" @click="openNew" />
    </div>
  </div>
  <div class="filterbar">
    <QueryInput v-model="q" scope="dataset" :error="queryError" :placeholder="t('dataSets.filterPlaceholder', { example: 'classification:anonymised expiring:true' })" @submit="load" />
  </div>
  <DataTable :value="sets" :loading="loading" data-key="id" size="small" class="card clickable" striped-rows :aria-label="t('dataSets.tableLabel')" @row-click="(e) => openSet((e.data as DataSet).id)">
    <template #empty>{{ t('dataSets.none') }}</template>
    <Column field="name" :header="t('common.name')" sortable>
      <template #body="{ data }">
        <a href="#" class="strong" @click.prevent.stop="openSet(data.id)">{{ data.name }}</a>
        <div v-if="data.description" class="small muted ellipsis" style="max-width: 360px">{{ data.description }}</div>
      </template>
    </Column>
    <Column field="classification" :header="t('dataSets.classificationLabel')" sortable>
      <template #body="{ data }"><Tag :value="classLabel(data.classification)" :severity="classificationSeverity(data.classification)" :icon="data.classification === 'production_copy' ? 'pi pi-exclamation-triangle' : undefined" /></template>
    </Column>
    <Column field="kind" :header="t('dataSets.kindLabel')" sortable><template #body="{ data }">{{ t(`dataSets.kind.${data.kind}`) }}</template></Column>
    <Column :header="t('dataSets.size')"><template #body="{ data }">{{ data.kind === 'table' ? t('dataSets.rowsParams', { rows: data.data_rows.length, params: data.parameters.length }) : t('dataSets.files', { n: data.file_count }) }}</template></Column>
    <Column field="version" :header="t('common.version')" sortable style="width: 90px" />
    <Column field="owner_id" :header="t('common.owner')"><template #body="{ data }">{{ personName(data.owner_id) }}</template></Column>
    <Column field="expires_on" :header="t('dataSets.expires')" sortable>
      <template #body="{ data }">
        <span v-if="data.expires_on">{{ formatDate(data.expires_on) }}</span>
        <Tag v-if="data.expired" :value="t('dataSets.expired')" severity="danger" style="margin-left: 4px" />
        <Tag v-else-if="data.expiring" :value="t('dataSets.expiring')" severity="warn" style="margin-left: 4px" />
      </template>
    </Column>
    <Column field="usage_count" :header="t('dataSets.usage')" sortable style="width: 90px" />
    <Column field="open_refreshes" :header="t('dataSets.refreshes')" sortable style="width: 110px" />
    <Column field="status" :header="t('common.status')" sortable><template #body="{ data }">{{ t(`dataSets.status.${data.status}`) }}</template></Column>
  </DataTable>

  <Dialog v-model:visible="dialog" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" :header="open ? open.name : t('dataSets.new')" style="width: min(920px, 96vw)">
    <Tabs v-if="open" :id="tabsId" v-model:value="tab" class="editor-tabs">
      <TabList :aria-label="t('dataSets.sections')">
        <Tab value="data">{{ t('dataSets.tabData') }}</Tab>
        <Tab value="links">{{ t('dataSets.tabLinks', { n: open.links.length }) }}</Tab>
        <Tab value="usage">{{ t('dataSets.tabUsage', { n: open.usage_count }) }}</Tab>
        <Tab value="refresh">{{ t('dataSets.tabRefresh', { n: open.open_refreshes }) }}</Tab>
        <Tab value="files">{{ t('dataSets.tabFiles', { n: open.file_count }) }}</Tab>
        <Tab value="history">{{ t('common.history') }}</Tab>
      </TabList>
    </Tabs>
    <section v-if="!open || tab === 'data'" v-bind="open ? panel('data') : {}" class="grid" style="gap: 12px; margin-top: 8px">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <Message v-if="form.classification === 'production_copy'" severity="error" :closable="false" icon="pi pi-exclamation-triangle">{{ t('dataSets.productionWarning') }}</Message>
      <Message v-if="expiry === 'expired'" severity="warn" :closable="false">{{ t('dataSets.expiredHint') }}</Message>
      <div class="form-grid">
        <div class="field">
          <label :for="`${id}-name`">{{ t('common.name') }}</label>
          <InputText :id="`${id}-name`" v-model="form.name" maxlength="200" :disabled="!canWrite" />
        </div>
        <div class="field">
          <label :for="`${id}-class`">{{ t('dataSets.classificationLabel') }}</label>
          <Select v-model="form.classification" :input-id="`${id}-class`" :options="classOptions" option-label="label" option-value="value" :disabled="!canWrite" />
        </div>
        <div class="field">
          <label :for="`${id}-kind`">{{ t('dataSets.kindLabel') }}</label>
          <Select v-model="form.kind" :input-id="`${id}-kind`" :options="kindOptions" option-label="label" option-value="value" :disabled="!canWrite" />
        </div>
        <div class="field">
          <label :for="`${id}-owner`">{{ t('common.owner') }}</label>
          <Select v-model="form.owner_id" :input-id="`${id}-owner`" :options="ownerOptions" option-label="label" option-value="value" filter :disabled="!canWrite" />
        </div>
        <div class="field">
          <label :for="`${id}-exp`">{{ t('dataSets.expires') }}</label>
          <InputText :id="`${id}-exp`" v-model="form.expires_on" type="date" :disabled="!canWrite" />
        </div>
        <div class="field">
          <label :for="`${id}-status`">{{ t('common.status') }}</label>
          <Select v-model="form.status" :input-id="`${id}-status`" :options="statusOptions" option-label="label" option-value="value" :disabled="!canWrite" />
        </div>
        <div class="field full">
          <label :for="`${id}-desc`">{{ t('common.description') }}</label>
          <Textarea :id="`${id}-desc`" v-model="form.description" rows="2" auto-resize :disabled="!canWrite" />
        </div>
        <div class="field full">
          <span class="strong small">{{ t('common.tags') }}</span>
          <TagsInput v-model="form.tags" :aria-label="t('common.tags')" :disabled="!canWrite" />
        </div>
      </div>
      <template v-if="form.kind === 'table'">
        <div class="row" style="gap: 6px">
          <h3 style="margin: 0">{{ t('dataSets.table') }}</h3>
          <span class="grow" />
          <Button v-if="canWrite" :label="t('dataSets.addParameter')" icon="pi pi-plus" text size="small" @click="addParam" />
          <Button v-if="canWrite" :label="t('dataSets.addRow')" icon="pi pi-plus" text size="small" :disabled="!params.length" @click="addRow" />
          <Button v-if="canWrite" :label="t('dataSets.paste')" icon="pi pi-clipboard" text size="small" aria-haspopup="dialog" @click="pasteOpen = true" />
        </div>
        <div class="td-wrap">
          <table class="td-table">
            <thead>
              <tr>
                <th scope="col">{{ t('dataSets.rowName') }}</th>
                <th v-for="(p, i) in params" :key="i" scope="col">
                  <div class="row nowrap" style="gap: 2px">
                    <InputText v-model="params[i]" size="small" :aria-label="t('dataSets.parameterName', { n: i + 1 })" :disabled="!canWrite" style="min-width: 90px" />
                    <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('dataSets.removeParameter', { name: p || i + 1 })" @click="removeParam(i)" />
                  </div>
                </th>
                <th scope="col"><span class="sr-only">{{ t('common.actions') }}</span></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(r, ri) in rows" :key="ri">
                <td><InputText v-model="r.name" size="small" :aria-label="t('dataSets.rowNameN', { n: ri + 1 })" :disabled="!canWrite" /></td>
                <td v-for="(p, i) in params" :key="i"><InputText v-model="r.values[p]" size="small" :aria-label="t('dataSets.cell', { row: ri + 1, param: p || i + 1 })" :disabled="!canWrite || !p" /></td>
                <td><Button v-if="canWrite" icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('dataSets.removeRow', { n: ri + 1 })" @click="rows.splice(ri, 1)" /></td>
              </tr>
            </tbody>
          </table>
          <p v-if="!rows.length" class="muted small">{{ t('dataSets.noRows') }}</p>
        </div>
      </template>
      <p v-else class="small muted">{{ open ? t('dataSets.filesHint') : t('dataSets.filesHintNew') }}</p>
      <div class="row" style="gap: 8px; justify-content: flex-end">
        <Button v-if="open && canWrite" :label="t('common.delete')" icon="pi pi-trash" severity="danger" text @click="remove(open)" />
        <span class="grow" />
        <span v-if="open" class="small muted">{{ t('dataSets.versionLine', { v: open.version, at: formatDateTime(open.updated_at) }) }}</span>
        <Button v-if="canWrite" :label="open ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim()" @click="save" />
      </div>
    </section>
    <section v-else-if="tab === 'links' && open" v-bind="panel('links')" style="margin-top: 8px">
      <h3 class="sr-only">{{ t('dataSets.tabLinks', { n: open.links.length }) }}</h3>
      <p class="small muted">{{ t('dataSets.linksHelp') }}</p>
      <ul class="plain-list">
        <li v-for="l in open.links" :key="l.id" class="row" style="gap: 8px; padding: 4px 0; border-bottom: 1px solid var(--qtf-border)">
          <Tag :value="t(`dataSets.itemType.${l.item_type}`)" severity="secondary" />
          <span class="key">{{ l.item_key }}</span>
          <span class="grow ellipsis">{{ l.item_title }}</span>
          <Tag v-if="l.usage === 'source'" :value="t('dataSets.source')" severity="info" icon="pi pi-table" />
          <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('dataSets.unlink', { key: l.item_key })" @click="unlink(l.id)" />
        </li>
      </ul>
      <p v-if="!open.links.length" class="muted small">{{ t('dataSets.noLinks') }}</p>
      <div v-if="canWrite" class="form-grid" style="margin-top: 8px">
        <div class="field">
          <span class="strong small">{{ t('dataSets.linkTests') }}</span>
          <div class="row" style="gap: 8px">
            <Button :label="t('dataSets.pickTests')" icon="pi pi-search" severity="secondary" outlined size="small" aria-haspopup="dialog" @click="testPicker = true" />
            <template v-if="open.kind === 'table'">
              <Checkbox v-model="asSource" binary :input-id="`${id}-src`" />
              <label :for="`${id}-src`" class="small">{{ t('dataSets.asSource') }}</label>
            </template>
          </div>
        </div>
        <div v-if="cfg.configurations.value.length" class="field">
          <label :for="`${id}-lcfg`">{{ t('dataSets.linkConfiguration') }}</label>
          <Select v-model="linkCfg" :input-id="`${id}-lcfg`" :options="cfg.options.value" option-label="label" option-value="value" filter show-clear />
        </div>
        <div class="field">
          <label :for="`${id}-lrun`">{{ t('dataSets.linkRun') }}</label>
          <Select v-model="linkRun" :input-id="`${id}-lrun`" :options="runOptions" option-label="label" option-value="value" filter show-clear />
        </div>
      </div>
    </section>
    <section v-else-if="tab === 'usage' && open" v-bind="panel('usage')" style="margin-top: 8px">
      <h3 class="sr-only">{{ t('dataSets.tabUsage', { n: open.usage_count }) }}</h3>
      <DataTable :value="usage" size="small" :aria-label="t('dataSets.usageTable')">
        <template #empty>{{ t('dataSets.noUsage') }}</template>
        <Column field="used_at" :header="t('common.date')"><template #body="{ data }">{{ formatDateTime(data.used_at) }}</template></Column>
        <Column field="run_name" :header="t('dataSets.run')">
          <template #body="{ data }"><RouterLink v-if="data.run_id" :to="{ name: 'run', params: { projectId: pid, runId: data.run_id } }">{{ data.run_name }}</RouterLink></template>
        </Column>
        <Column field="test_key" :header="t('dataSets.test')" />
        <Column field="data_version" :header="t('common.version')" />
      </DataTable>
    </section>
    <section v-else-if="tab === 'refresh' && open" v-bind="panel('refresh')" style="margin-top: 8px">
      <h3 class="sr-only">{{ t('dataSets.tabRefresh', { n: open.open_refreshes }) }}</h3>
      <ul class="plain-list">
        <li v-for="r in refreshes" :key="r.id" style="padding: 6px 0; border-bottom: 1px solid var(--qtf-border)">
          <div class="row" style="gap: 8px">
            <Tag :value="t(`dataSets.refreshStatus.${r.status}`)" :severity="r.status === 'open' ? 'warn' : r.status === 'done' ? 'success' : 'secondary'" />
            <span class="small">{{ r.requested_by }} · {{ formatDateTime(r.created_at) }}</span>
            <span v-if="r.needed_by" class="small muted">{{ t('dataSets.neededBy', { date: formatDate(r.needed_by) }) }}</span>
            <span class="grow" />
            <template v-if="r.status === 'open' && canResolve">
              <Button :label="t('dataSets.markDone')" icon="pi pi-check" size="small" text @click="resolve(r, 'done')" />
              <Button :label="t('dataSets.reject')" icon="pi pi-times" size="small" text severity="danger" @click="resolve(r, 'rejected')" />
            </template>
          </div>
          <div class="small" style="white-space: pre-wrap">{{ r.reason }}</div>
          <div v-if="r.resolution_note" class="small muted">{{ r.resolved_by }}: {{ r.resolution_note }}</div>
        </li>
      </ul>
      <p v-if="!refreshes.length" class="muted small">{{ t('dataSets.noRefreshes') }}</p>
      <form v-if="canAsk" class="grid" style="gap: 8px; margin-top: 8px" @submit.prevent="askRefresh">
        <div class="field">
          <label :for="`${id}-rr`">{{ t('dataSets.refreshReason') }}</label>
          <Textarea :id="`${id}-rr`" v-model="refreshReason" rows="2" auto-resize maxlength="5000" />
        </div>
        <div class="row" style="gap: 8px">
          <label :for="`${id}-rn`" class="small">{{ t('dataSets.neededByLabel') }}</label>
          <InputText :id="`${id}-rn`" v-model="refreshNeeded" type="date" />
          <span class="grow" />
          <Button type="submit" :label="t('dataSets.askRefresh')" icon="pi pi-refresh" :disabled="!refreshReason.trim()" />
        </div>
      </form>
    </section>
    <section v-else-if="tab === 'files' && open" v-bind="panel('files')" style="margin-top: 8px">
      <h3 class="sr-only">{{ t('dataSets.tabFiles', { n: open.file_count }) }}</h3>
      <Message v-if="open.classification === 'production_copy'" severity="error" :closable="false" icon="pi pi-exclamation-triangle">{{ t('dataSets.productionWarning') }}</Message>
      <AttachmentsPanel type="test_data_set" :entity-id="open.id" :project-id="pid" :can-write="canWrite" />
    </section>
    <section v-else-if="tab === 'history' && open" v-bind="panel('history')" style="margin-top: 8px">
      <h3 class="sr-only">{{ t('common.history') }}</h3>
      <HistoryPanel entity="test_data_set" :entity-id="open.id" :can-restore="false" />
    </section>
  </Dialog>
  <ItemPickerDialog v-if="open" v-model:visible="testPicker" kind="test" :project-id="pid" :exclude="open.links.filter((l) => l.item_type === 'test').map((l) => l.item_id)" @pick="pickTests" />
  <Dialog v-model:visible="pasteOpen" modal :header="t('dataSets.pasteHeader')" style="width: min(560px, 96vw)">
    <p class="small muted">{{ t('dataSets.pasteHelp') }}</p>
    <label :for="`${id}-paste`" class="sr-only">{{ t('dataSets.pasteHeader') }}</label>
    <Textarea :id="`${id}-paste`" v-model="pasteText" rows="8" style="width: 100%" class="mono" />
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="pasteOpen = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!pasteText.trim()" @click="applyPaste" />
    </template>
  </Dialog>
</template>

<style scoped>
.td-wrap {
  overflow-x: auto;
}
.td-table {
  border-collapse: collapse;
}
.td-table th,
.td-table td {
  padding: 2px 4px;
  border-bottom: 1px solid var(--qtf-border);
  text-align: left;
}
.plain-list {
  list-style: none;
  padding: 0;
  margin: 0;
}
</style>
