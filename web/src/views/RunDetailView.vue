<script setup lang="ts">
import MentionText from '@/components/MentionText.vue'
import MentionInput from '@/components/MentionInput.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Panel from 'primevue/panel'
import ProgressSpinner from 'primevue/progressspinner'
import QueryInput from '@/components/QueryInput.vue'
import StatusTag from '@/components/StatusTag.vue'
import SummaryBar from '@/components/SummaryBar.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import DefectLinks from '@/components/DefectLinks.vue'
import AttachmentsPanel from '@/components/AttachmentsPanel.vue'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { anyOf, and } from '@/lib/query'
import { formatDateTime, formatDuration, formatNumber } from '@/lib/format'
import { summarize } from '@/lib/summary'
import { RESULT_COLORS } from '@/lib/colors'
import { P } from '@/lib/permissions'
import type { Execution, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'
const prefs = usePrefsStore()

const props = defineProps<{ runId: string }>()
const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const runStatusOptions = computed(() => enumOptions('status', meta.meta.run_statuses))
const resultOptions = computed(() => enumOptions('result', meta.meta.result_statuses))

const run = ref<TestRun | null>(null)
const executions = ref<Execution[]>([])
const loading = ref(false)
const error = ref('')
const q = ref('')
const statusFilter = ref<string[]>([])
const queryError = ref<unknown>(null)
const composed = computed(() => and(q.value, anyOf('status', statusFilter.value)))
const canWrite = computed(() => auth.can(P.runWrite))
const canExecute = computed(() => auth.can(P.runExecute))
const tab = ref('executions')
const attachmentCount = ref<number | null>(null)
const tabsId = 'run-tabs'
const panel = (v: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${v}`, 'aria-labelledby': `${tabsId}_tab_${v}` })

async function loadRun() {
  try {
    run.value = await api.get<TestRun>(`/runs/${props.runId}`)
  } catch (e) {
    error.value = errorMessage(e)
  }
}
async function loadExecutions() {
  loading.value = true
  queryError.value = null
  try {
    executions.value = asItems(await api.get<{ items: Execution[] }>(`/runs/${props.runId}/executions`, { q: composed.value }))
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
async function reload() {
  await Promise.all([loadRun(), loadExecutions()])
}
onMounted(reload)

const summary = computed(() => summarize(run.value?.summary))

async function setStatus(status: string) {
  if (!run.value) return
  try {
    run.value = await api.patch<TestRun>(`/runs/${run.value.id}`, { status })
    notify.ok(t('run.statusUpdated'), enumLabel('status', status))
  } catch (e) {
    notify.fail(e)
    loadRun()
  }
}

// ---------- edit ----------
const editDialog = ref(false)
const editForm = ref({ name: '', description: '', environment: '', build: '' })
function openEdit() {
  if (!run.value) return
  const r = run.value
  editForm.value = { name: r.name, description: r.description, environment: r.environment, build: r.build }
  editDialog.value = true
}
async function saveEdit() {
  try {
    run.value = await api.patch<TestRun>(`/runs/${props.runId}`, editForm.value)
    editDialog.value = false
    notify.ok(t('run.updated'))
  } catch (e) {
    notify.fail(e)
  }
}
function remove() {
  notify.confirmDanger(t('run.confirmDelete', { name: run.value?.name ?? '' }), async () => {
    try {
      await api.del(`/runs/${props.runId}`)
      router.push({ name: 'runs', params: { projectId: proj.pid } })
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- executions ----------
/** assignee being edited per execution id (committed on blur or Enter) */
const assigneeDraft = reactive<Record<string, string>>({})
async function setAssignee(ex: Execution, value: string) {
  if (value === ex.assignee) return
  try {
    const updated = await api.patch<Execution>(`/executions/${ex.id}`, { assignee: value })
    Object.assign(ex, updated ?? { assignee: value })
  } catch (e) {
    notify.fail(e)
  }
}
async function quickStatus(ex: Execution, status: string) {
  try {
    const updated = await api.patch<Execution>(`/executions/${ex.id}`, { status })
    Object.assign(ex, updated ?? { status })
    loadRun()
  } catch (e) {
    notify.fail(e)
  }
}
const picker = ref(false)
async function addTests(items: { id: string }[]) {
  try {
    await api.post(`/runs/${props.runId}/executions`, { test_ids: items.map((i) => i.id) })
    notify.ok(t('run.addedTests', items.length))
    reload()
  } catch (e) {
    notify.fail(e)
  }
}
function execute(ex: Execution) {
  router.push({ name: 'execute', params: { projectId: proj.pid, runId: props.runId, executionId: ex.id } })
}
function startNext() {
  const next = executions.value.find((e) => e.status === 'not_run' || e.status === 'in_progress') ?? executions.value[0]
  if (next) execute(next)
}

// ---------- CI hint ----------
const ciDialog = ref(false)
const origin = typeof window !== 'undefined' ? window.location.origin : ''
const ciSnippet = computed(
  () => `curl -X POST ${origin}/api/v1/runs/${props.runId}/results \\
  -H "Authorization: Bearer qtf_<api-token>" \\
  -H "Content-Type: application/json" \\
  -d '{"results":[{"test_key":"${executions.value[0]?.test_key ?? 'PROJ-TC-1'}","status":"passed","duration_ms":1234,"output":"ok"}]}'`,
)
async function copySnippet() {
  try {
    await navigator.clipboard.writeText(ciSnippet.value)
    notify.ok(t('run.copied'))
  } catch {
    notify.info(t('errors.copyFailed'), t('errors.copyManually'))
  }
}
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="!run" class="empty"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
  <template v-else>
    <div class="page-header">
      <div>
        <div class="row small" style="margin-bottom: 2px">
          <RouterLink :to="{ name: 'runs', params: { projectId: proj.pid } }" class="muted">{{ t('nav.operations') }}</RouterLink>
          <i class="pi pi-angle-right muted" />
        </div>
        <h1>{{ run.name }}</h1>
        <div class="sub row">
          <span v-if="run.environment"><i class="pi pi-server" aria-hidden="true" /><span class="sr-only">{{ t('common.environment') }}:</span> {{ run.environment }}</span>
          <span v-if="run.build" class="mono"><i class="pi pi-box" aria-hidden="true" /><span class="sr-only">{{ t('common.build') }}:</span> {{ run.build }}</span>
          <span>{{ t('run.createdAt', { date: formatDateTime(run.created_at) }) }}{{ run.created_by ? t('run.createdBy', { user: run.created_by }) : '' }}</span>
          <span v-if="run.started_at">{{ t('run.startedAt', { date: formatDateTime(run.started_at) }) }}</span>
          <span v-if="run.completed_at">{{ t('run.completedAt', { date: formatDateTime(run.completed_at) }) }}</span>
        </div>
      </div>
      <div class="actions">
        <Select
          v-if="canWrite"
          :model-value="run.status"
          :options="runStatusOptions"
          option-label="label"
          option-value="value"
          style="min-width: 160px"
          :aria-label="t('run.status')"
          @update:model-value="setStatus"
        >
          <template #value="{ value }"><StatusTag :value="value" /></template>
        </Select>
        <StatusTag v-else :value="run.status" />
        <Button v-if="canExecute && executions.length" :label="t('run.execute')" icon="pi pi-play" @click="startNext" />
        <Button v-if="canWrite" :label="t('run.addTests')" icon="pi pi-plus" severity="secondary" outlined @click="picker = true" />
        <Button :label="t('run.ciImport')" icon="pi pi-cloud-upload" severity="secondary" outlined @click="ciDialog = true" />
        <Button v-if="canWrite" icon="pi pi-pencil" severity="secondary" text rounded :aria-label="t('common.edit')" v-tooltip="t('common.edit')" @click="openEdit" />
        <Button v-if="canWrite" icon="pi pi-trash" severity="danger" text rounded :aria-label="t('common.delete')" v-tooltip="t('common.delete')" @click="remove" />
      </div>
    </div>
    <p v-if="run.description" class="muted" style="margin-top: -8px">{{ run.description }}</p>

    <div class="kpis" style="margin-bottom: 16px">
      <div class="card">
        <div class="small muted">{{ t('run.progress') }}</div>
        <div class="kpi"><div class="value">{{ formatNumber(summary.progress / 100, undefined, { style: 'percent' }) }}</div><span class="muted small">{{ t('run.executed', { executed: summary.executed, total: summary.total }) }}</span></div>
        <SummaryBar :summary="run.summary" style="margin-top: 8px" />
      </div>
      <div class="card">
        <div class="small muted">{{ t('run.passRate') }}</div>
        <div class="value" style="font-size: 24px; font-weight: 700">{{ formatNumber(summary.passRate / 100, undefined, { style: 'percent' }) }}</div>
      </div>
      <div v-for="s in summary.segments" :key="s.status" class="card">
        <div class="small muted row nowrap"><span aria-hidden="true" :style="{ width: '10px', height: '10px', borderRadius: '50%', background: RESULT_COLORS[s.status] ?? s.color }" />{{ enumLabel('result', s.status) }}</div>
        <div class="value" style="font-size: 24px; font-weight: 700">{{ s.count }}</div>
      </div>
    </div>

    <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs" scrollable>
      <TabList>
        <Tab value="executions"><i class="pi pi-list" aria-hidden="true" /> {{ t('common.executions') }} <span class="count">{{ executions.length }}</span></Tab>
        <Tab value="attachments"><i class="pi pi-paperclip" aria-hidden="true" /> {{ t('files.tab') }} <span v-if="attachmentCount !== null" class="count">{{ attachmentCount }}</span></Tab>
      </TabList>
    </Tabs>
    <div v-if="tab === 'attachments'" v-bind="panel('attachments')" class="card">
      <AttachmentsPanel type="run" :entity-id="runId" :project-id="proj.pid" :can-write="canExecute" @count="(n) => (attachmentCount = n)" />
    </div>
    <div v-show="tab === 'executions'" v-bind="panel('executions')" class="card">
      <div class="filterbar">
        <QueryInput v-model="q" scope="execution" :error="queryError" :composed="composed" :placeholder="t('run.filterPlaceholder', { example: 'assignee:alice title:login' })" @submit="loadExecutions" />
        <MultiSelect v-model="statusFilter" :options="resultOptions" option-label="label" option-value="value" :placeholder="t('common.status')" :aria-label="t('common.status')" style="min-width: 150px" show-clear @change="loadExecutions" />
        <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="reload" />
      </div>
      <DataTable v-stack :value="executions" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(50)" :rows-per-page-options="prefs.rowsPerPage" row-hover sort-field="position" :sort-order="1" size="small">
        <Column field="position" header="#" sortable style="width: 60px" />
        <Column field="test_key" :header="t('run.test')" sortable style="width: 140px">
          <template #body="{ data }">
            <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: data.test_case_id } }" class="key">{{ data.test_key }}</RouterLink>
          </template>
        </Column>
        <Column field="test_title" :header="t('common.title')" sortable style="min-width: 200px" body-class="stack-main" />
        <Column field="status" :header="t('common.status')" sortable style="width: 130px"><template #body="{ data }"><StatusTag :value="data.status" kind="result" /></template></Column>
        <Column field="assignee" :header="t('run.assignee')" sortable style="width: 180px">
          <template #body="{ data }">
            <MentionInput
              v-if="canWrite"
              :model-value="assigneeDraft[data.id] ?? data.assignee"
              size="small"
              :placeholder="t('run.unassigned')"
              :aria-label="t('run.assigneeOf', { key: data.test_key })"
              class="w-full"
              @update:model-value="(v: string) => (assigneeDraft[data.id] = v)"
              @blur="setAssignee(data, assigneeDraft[data.id] ?? data.assignee)"
              @keydown.enter="setAssignee(data, assigneeDraft[data.id] ?? data.assignee)"
            />
            <MentionText v-else-if="data.assignee" :text="data.assignee" />
            <span v-else>—</span>
          </template>
        </Column>
        <Column field="executed_by" :header="t('run.executedBy')" sortable style="width: 150px" />
        <Column :header="t('run.defects')" style="min-width: 120px"><template #body="{ data }"><DefectLinks :defects="data.defects" /></template></Column>
        <Column field="duration_ms" :header="t('common.duration')" sortable style="width: 110px"><template #body="{ data }">{{ formatDuration(data.duration_ms) }}</template></Column>
        <Column field="finished_at" :header="t('run.finished')" sortable style="width: 150px"><template #body="{ data }"><span class="small muted">{{ formatDateTime(data.finished_at) }}</span></template></Column>
        <Column style="width: 150px">
          <template #body="{ data }">
            <div class="row nowrap" style="gap: 0">
              <template v-if="canExecute">
                <Button icon="pi pi-check" text rounded size="small" severity="success" v-tooltip="t('run.markPassed')" :aria-label="t('run.markPassedKey', { key: data.test_key })" @click="quickStatus(data, 'passed')" />
                <Button icon="pi pi-times" text rounded size="small" severity="danger" v-tooltip="t('run.markFailed')" :aria-label="t('run.markFailedKey', { key: data.test_key })" @click="quickStatus(data, 'failed')" />
              </template>
              <Button :icon="canExecute ? 'pi pi-play' : 'pi pi-eye'" text rounded size="small" v-tooltip="canExecute ? t('run.execute') : t('run.view')" :aria-label="canExecute ? t('run.executeKey', { key: data.test_key }) : t('run.viewKey', { key: data.test_key })" @click="execute(data)" />
            </div>
          </template>
        </Column>
        <template #empty><div class="empty"><i class="pi pi-inbox" />{{ composed ? t('run.noneFilter') : t('run.none') }}</div></template>
      </DataTable>
    </div>

    <Dialog v-model:visible="editDialog" :header="t('run.edit')" modal style="width: min(560px, 95vw)">
      <div class="form-grid">
        <div v-field class="field full"><label>{{ t('common.name') }}</label><InputText v-model="editForm.name" /></div>
        <div v-field class="field"><label>{{ t('common.environment') }}</label><InputText v-model="editForm.environment" /></div>
        <div v-field class="field"><label>{{ t('common.build') }}</label><InputText v-model="editForm.build" /></div>
        <div v-field class="field full"><label>{{ t('common.description') }}</label><Textarea v-model="editForm.description" rows="3" auto-resize /></div>
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" text severity="secondary" @click="editDialog = false" />
        <Button :label="t('common.save')" icon="pi pi-check" :disabled="!editForm.name.trim()" @click="saveEdit" />
      </template>
    </Dialog>

    <Dialog v-model:visible="ciDialog" :header="t('run.ciHeader')" modal style="width: min(720px, 96vw)">
      <i18n-t keypath="run.ciIntro" tag="p">
        <template #link><RouterLink :to="{ name: 'tokens' }">{{ t('run.ciLink') }}</RouterLink></template>
      </i18n-t>
      <pre class="console" tabindex="0" :aria-label="t('run.ciHeader')">{{ ciSnippet }}</pre>
      <i18n-t keypath="run.ciStatus" tag="p" class="small muted">
        <template #status><code>status</code></template>
        <template #values>{{ meta.meta.result_statuses.join(', ') }}</template>
        <template #updated><code>updated</code></template>
        <template #unknown><code>unknown</code></template>
      </i18n-t>
      <Panel :header="t('run.endpoint')" toggleable collapsed>
        <div class="mono">POST /api/v1/runs/{{ runId }}/results</div>
      </Panel>
      <template #footer>
        <Button :label="t('common.copy')" icon="pi pi-copy" severity="secondary" @click="copySnippet" />
        <Button :label="t('common.close')" @click="ciDialog = false" />
      </template>
    </Dialog>
    <ItemPickerDialog v-model:visible="picker" kind="test" :project-id="proj.pid" :exclude="executions.map((e) => e.test_case_id)" :header="t('run.addTestsHeader')" @pick="addTests" />
  </template>
</template>
