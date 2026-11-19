<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Tree from 'primevue/tree'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import QueryInput from '@/components/QueryInput.vue'
import StatusTag from '@/components/StatusTag.vue'
import SummaryBar from '@/components/SummaryBar.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { anyOf, and } from '@/lib/query'
import { buildTree, allKeys } from '@/lib/tree'
import { formatDateTime } from '@/lib/format'
import { summarize } from '@/lib/summary'
import { P } from '@/lib/permissions'
import type { TestCase, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'
import { enumOptions } from '@/i18n/labels'
const prefs = usePrefsStore()

const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const statusOptions = computed(() => enumOptions('status', meta.meta.run_statuses))

const runs = ref<TestRun[]>([])
const loading = ref(false)
const q = ref('')
const statuses = ref<string[]>([])
const queryError = ref<unknown>(null)
const composed = computed(() => and(q.value, anyOf('status', statuses.value)))

async function load() {
  loading.value = true
  queryError.value = null
  try {
    runs.value = asItems(await api.get<{ items: TestRun[] }>(`/projects/${proj.pid}/runs`, { q: composed.value }))
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---------- create ----------
const dialog = ref(false)
const form = ref({ name: '', description: '', environment: '', build: '', query: '' })
const pickedTests = ref<TestCase[]>([])
const folderKeys = ref<Record<string, { checked: boolean; partialChecked: boolean }>>({})
const folderExpanded = ref<Record<string, boolean>>({})
const picker = ref(false)
const saving = ref(false)
const createError = ref('')
const testFolders = computed(() => buildTree(proj.folders.test))
const selectedFolderIds = computed(() => Object.entries(folderKeys.value).filter(([, v]) => v.checked).map(([k]) => k))
const previewCount = ref<number | null>(null)
const previewError = ref<unknown>(null)

function openCreate() {
  const d = new Date()
  form.value = { name: t('runs.defaultName', { date: d.toISOString().slice(0, 10) }), description: '', environment: '', build: '', query: '' }
  pickedTests.value = []
  folderKeys.value = {}
  folderExpanded.value = allKeys(testFolders.value)
  createError.value = ''
  previewCount.value = null
  dialog.value = true
}
async function preview() {
  previewError.value = null
  previewCount.value = null
  if (!form.value.query.trim()) return
  try {
    const res = await api.get<{ items: TestCase[]; total: number }>(`/projects/${proj.pid}/tests`, { q: form.value.query })
    previewCount.value = res?.total ?? asItems(res).length
  } catch (e) {
    previewError.value = e
  }
}
function addPicked(items: { id: string }[]) {
  const have = new Set(pickedTests.value.map((t) => t.id))
  pickedTests.value = [...pickedTests.value, ...(items as TestCase[]).filter((t) => !have.has(t.id))]
}
const nothingSelected = computed(() => !pickedTests.value.length && !selectedFolderIds.value.length && !form.value.query.trim())
async function create() {
  saving.value = true
  createError.value = ''
  try {
    const run = await api.post<TestRun>(`/projects/${proj.pid}/runs`, {
      ...form.value,
      test_ids: pickedTests.value.map((t) => t.id),
      folder_ids: selectedFolderIds.value,
    })
    dialog.value = false
    notify.ok(t('runs.created'), run?.name)
    if (run?.id) router.push({ name: 'run', params: { projectId: proj.pid, runId: run.id } })
    else load()
  } catch (e) {
    createError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

function remove(r: TestRun) {
  notify.confirmDanger(t('runs.confirmDelete', { name: r.name }), async () => {
    try {
      await api.del(`/runs/${r.id}`)
      notify.ok(t('runs.deleted'))
      load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
const progress = (r: TestRun) => summarize(r.summary)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('runs.title') }}</h1>
      <div class="sub">{{ t('runs.subtitle') }}</div>
    </div>
    <div class="actions">
      <Button v-if="auth.can(P.runWrite)" :label="t('runs.new')" icon="pi pi-plus" @click="openCreate" />
    </div>
  </div>
  <div class="card">
    <div class="filterbar">
      <QueryInput v-model="q" scope="run" :error="queryError" :composed="composed" :placeholder="t('runs.filterPlaceholder', { example: 'environment:staging build:1.4*' })" @submit="load" />
      <MultiSelect v-model="statuses" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('common.status')" :aria-label="t('common.status')" style="min-width: 150px" show-clear @change="load" />
    </div>
    <DataTable v-stack :value="runs" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(25)" row-hover sort-field="created_at" :sort-order="-1" class="clickable" @row-click="(e) => router.push({ name: 'run', params: { projectId: proj.pid, runId: (e.data as TestRun).id } })">
      <Column field="name" :header="t('common.name')" sortable body-class="stack-main">
        <template #body="{ data }">
          <div style="min-width: 0">
            <RouterLink :to="{ name: 'run', params: { projectId: proj.pid, runId: data.id } }" class="project-link" @click.stop>{{ data.name }}</RouterLink>
            <div class="small muted ellipsis" style="max-width: 360px">{{ data.description }}</div>
          </div>
        </template>
      </Column>
      <Column field="status" :header="t('common.status')" sortable style="width: 130px"><template #body="{ data }"><StatusTag :value="data.status" /></template></Column>
      <Column field="environment" :header="t('common.environment')" sortable />
      <Column field="build" :header="t('common.build')" sortable><template #body="{ data }"><span class="mono">{{ data.build }}</span></template></Column>
      <Column :header="t('runs.progress')" style="min-width: 220px">
        <template #body="{ data }"><SummaryBar :summary="data.summary" show-text /></template>
      </Column>
      <Column :header="t('runs.executions')" style="width: 120px"><template #body="{ data }">{{ progress(data).total }}</template></Column>
      <Column field="created_at" :header="t('common.created')" sortable style="width: 160px">
        <template #body="{ data }"><span class="small muted">{{ formatDateTime(data.created_at) }}</span></template>
      </Column>
      <Column v-if="auth.can(P.runWrite)" style="width: 60px">
        <template #body="{ data }"><Button icon="pi pi-trash" text rounded severity="danger" :aria-label="t('runs.deleteNamed', { name: data.name })" v-tooltip="t('common.delete')" @click.stop="remove(data)" /></template>
      </Column>
      <template #empty><div class="empty"><i class="pi pi-play-circle" />{{ t('runs.none') }}</div></template>
    </DataTable>
  </div>

  <Dialog v-model:visible="dialog" :header="t('runs.newHeader')" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(760px, 96vw)">
    <div class="grid" style="gap: 14px">
      <Message v-if="createError" severity="error" :closable="false">{{ createError }}</Message>
      <div class="form-grid">
        <div v-field class="field full"><label>{{ t('common.name') }}</label><InputText v-model="form.name" autofocus /></div>
        <div v-field class="field"><label>{{ t('common.environment') }}</label><InputText v-model="form.environment" :placeholder="t('runs.envPlaceholder')" /></div>
        <div v-field class="field"><label>{{ t('common.build') }}</label><InputText v-model="form.build" :placeholder="t('runs.buildPlaceholder')" /></div>
        <div v-field class="field full"><label>{{ t('common.description') }}</label><Textarea v-model="form.description" rows="2" auto-resize /></div>
      </div>
      <div class="section-title" style="margin: 0">{{ t('runs.include') }}</div>
      <Tabs value="tests" scrollable>
        <TabList>
          <Tab value="tests">{{ t('runs.tabTests', { count: pickedTests.length }) }}</Tab>
          <Tab value="folders">{{ t('runs.tabFolders', { count: selectedFolderIds.length }) }}</Tab>
          <Tab value="query">{{ form.query.trim() ? t('runs.tabQueryActive') : t('runs.tabQuery') }}</Tab>
        </TabList>
        <TabPanels>
          <TabPanel value="tests">
            <div class="row" style="margin-bottom: 8px">
              <Button :label="t('runs.pickTests')" icon="pi pi-search" severity="secondary" outlined @click="picker = true" />
              <Button v-if="pickedTests.length" :label="t('common.clear')" text severity="secondary" @click="pickedTests = []" />
            </div>
            <div style="max-height: 220px; overflow: auto">
              <div v-for="tc in pickedTests" :key="tc.id" class="row nowrap" style="padding: 4px 0; border-bottom: 1px solid var(--qtf-border)">
                <span class="key">{{ tc.key }}</span><span class="grow ellipsis">{{ tc.title }}</span>
                <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('common.removeItem', { key: tc.key })" @click="pickedTests = pickedTests.filter((x) => x.id !== tc.id)" />
              </div>
              <div v-if="!pickedTests.length" class="muted small">{{ t('runs.noneSelectedTests') }}</div>
            </div>
          </TabPanel>
          <TabPanel value="folders">
            <div v-if="!testFolders.length" class="muted small">{{ t('runs.noFolders') }}</div>
            <Tree v-else v-model:selection-keys="folderKeys" v-model:expanded-keys="folderExpanded" :value="testFolders" selection-mode="checkbox" :aria-label="t('runs.tabFolders', { count: selectedFolderIds.length })" style="max-height: 260px; overflow: auto; padding: 0; border: none" />
            <div class="small muted">{{ t('runs.foldersHint') }}</div>
          </TabPanel>
          <TabPanel value="query">
            <div class="row">
              <QueryInput v-model="form.query" scope="test" :error="previewError" :placeholder="t('runs.queryPlaceholder', { example: 'category:Negative priority=high' })" @submit="preview" />
              <Button :label="t('runs.preview')" severity="secondary" outlined @click="preview" />
            </div>
            <div v-if="previewCount !== null" class="small" style="margin-top: 6px">{{ t('runs.previewCount', previewCount) }}</div>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </div>
    <template #footer>
      <span v-if="nothingSelected" class="small muted" style="margin-right: auto">{{ t('runs.startsEmpty') }}</span>
      <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
      <Button :label="t('runs.create')" icon="pi pi-check" :loading="saving" :disabled="!form.name.trim()" @click="create" />
    </template>
  </Dialog>
  <ItemPickerDialog v-model:visible="picker" kind="test" :project-id="proj.pid" :exclude="pickedTests.map((t) => t.id)" @pick="addPicked" />
</template>
