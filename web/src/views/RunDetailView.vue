<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { anyOf, and } from '@/lib/query'
import { formatDateTime, formatDuration, humanize, options, pluralize } from '@/lib/format'
import { summarize } from '@/lib/summary'
import { RESULT_COLORS } from '@/lib/colors'
import { P } from '@/lib/permissions'
import type { Execution, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
const prefs = usePrefsStore()

const props = defineProps<{ runId: string }>()
const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const router = useRouter()
const notify = useNotify()

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
    notify.ok('Run status updated', humanize(status))
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
    notify.ok('Run updated')
  } catch (e) {
    notify.fail(e)
  }
}
function remove() {
  notify.confirmDanger(`Delete run "${run.value?.name}"?`, async () => {
    try {
      await api.del(`/runs/${props.runId}`)
      router.push({ name: 'runs', params: { projectId: proj.pid } })
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- executions ----------
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
    notify.ok(`Added ${pluralize(items.length, 'test')}`)
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
    notify.ok('Copied to clipboard')
  } catch {
    notify.info('Copy failed', 'Select the text and copy it manually.')
  }
}
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="!run" class="empty"><ProgressSpinner style="width: 36px; height: 36px" /></div>
  <template v-else>
    <div class="page-header">
      <div>
        <div class="row small" style="margin-bottom: 2px">
          <RouterLink :to="{ name: 'runs', params: { projectId: proj.pid } }" class="muted">Operations</RouterLink>
          <i class="pi pi-angle-right muted" />
        </div>
        <h1>{{ run.name }}</h1>
        <div class="sub row">
          <span v-if="run.environment"><i class="pi pi-server" /> {{ run.environment }}</span>
          <span v-if="run.build" class="mono"><i class="pi pi-box" /> {{ run.build }}</span>
          <span>Created {{ formatDateTime(run.created_at) }}{{ run.created_by ? ' by ' + run.created_by : '' }}</span>
          <span v-if="run.started_at">· started {{ formatDateTime(run.started_at) }}</span>
          <span v-if="run.completed_at">· completed {{ formatDateTime(run.completed_at) }}</span>
        </div>
      </div>
      <div class="actions">
        <Select
          v-if="canWrite"
          :model-value="run.status"
          :options="options(meta.meta.run_statuses)"
          option-label="label"
          option-value="value"
          style="width: 160px"
          aria-label="Run status"
          @update:model-value="setStatus"
        >
          <template #value="{ value }"><StatusTag :value="value" /></template>
        </Select>
        <StatusTag v-else :value="run.status" />
        <Button v-if="canExecute && executions.length" label="Execute" icon="pi pi-play" @click="startNext" />
        <Button v-if="canWrite" label="Add tests" icon="pi pi-plus" severity="secondary" outlined @click="picker = true" />
        <Button label="CI import" icon="pi pi-cloud-upload" severity="secondary" outlined @click="ciDialog = true" />
        <Button v-if="canWrite" icon="pi pi-pencil" severity="secondary" text rounded aria-label="Edit" @click="openEdit" />
        <Button v-if="canWrite" icon="pi pi-trash" severity="danger" text rounded aria-label="Delete" @click="remove" />
      </div>
    </div>
    <p v-if="run.description" class="muted" style="margin-top: -8px">{{ run.description }}</p>

    <div class="kpis" style="margin-bottom: 16px">
      <div class="card">
        <div class="small muted">Progress</div>
        <div class="kpi"><div class="value">{{ summary.progress }}%</div><span class="muted small">{{ summary.executed }}/{{ summary.total }} executed</span></div>
        <SummaryBar :summary="run.summary" style="margin-top: 8px" />
      </div>
      <div class="card">
        <div class="small muted">Pass rate</div>
        <div class="value" style="font-size: 24px; font-weight: 700">{{ summary.passRate }}%</div>
      </div>
      <div v-for="s in summary.segments" :key="s.status" class="card">
        <div class="small muted row nowrap"><span :style="{ width: '10px', height: '10px', borderRadius: '50%', background: RESULT_COLORS[s.status] ?? s.color }" />{{ humanize(s.status) }}</div>
        <div class="value" style="font-size: 24px; font-weight: 700">{{ s.count }}</div>
      </div>
    </div>

    <div class="card">
      <div class="filterbar">
        <QueryInput v-model="q" scope="execution" :error="queryError" :composed="composed" placeholder="Filter executions, e.g. assignee:alice title:login" @submit="loadExecutions" />
        <MultiSelect v-model="statusFilter" :options="options(meta.meta.result_statuses)" option-label="label" option-value="value" placeholder="Status" style="min-width: 150px" show-clear @change="loadExecutions" />
        <Button icon="pi pi-refresh" text rounded severity="secondary" aria-label="Refresh" @click="reload" />
      </div>
      <DataTable :value="executions" data-key="id" :loading="loading" paginator :rows="prefs.rowsFor(50)" :rows-per-page-options="prefs.rowsPerPage" row-hover sort-field="position" :sort-order="1" size="small">
        <Column field="position" header="#" sortable style="width: 60px" />
        <Column field="test_key" header="Test" sortable style="width: 140px">
          <template #body="{ data }">
            <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: data.test_case_id } }" class="key">{{ data.test_key }}</RouterLink>
          </template>
        </Column>
        <Column field="test_title" header="Title" sortable style="min-width: 200px" />
        <Column field="status" header="Status" sortable style="width: 130px"><template #body="{ data }"><StatusTag :value="data.status" kind="result" /></template></Column>
        <Column field="assignee" header="Assignee" sortable style="width: 180px">
          <template #body="{ data }">
            <InputText v-if="canWrite" :model-value="data.assignee" size="small" placeholder="Unassigned" class="w-full" @change="(e: Event) => setAssignee(data, (e.target as HTMLInputElement).value)" />
            <span v-else>{{ data.assignee || '—' }}</span>
          </template>
        </Column>
        <Column field="executed_by" header="Executed by" sortable style="width: 150px" />
        <Column header="Defects" style="min-width: 120px"><template #body="{ data }"><DefectLinks :defects="data.defects" /></template></Column>
        <Column field="duration_ms" header="Duration" sortable style="width: 100px"><template #body="{ data }">{{ formatDuration(data.duration_ms) }}</template></Column>
        <Column field="finished_at" header="Finished" sortable style="width: 150px"><template #body="{ data }"><span class="small muted">{{ formatDateTime(data.finished_at) }}</span></template></Column>
        <Column style="width: 150px">
          <template #body="{ data }">
            <div class="row nowrap" style="gap: 0">
              <template v-if="canExecute">
                <Button icon="pi pi-check" text rounded size="small" severity="success" v-tooltip="'Mark passed'" aria-label="Passed" @click="quickStatus(data, 'passed')" />
                <Button icon="pi pi-times" text rounded size="small" severity="danger" v-tooltip="'Mark failed'" aria-label="Failed" @click="quickStatus(data, 'failed')" />
              </template>
              <Button :icon="canExecute ? 'pi pi-play' : 'pi pi-eye'" text rounded size="small" v-tooltip="canExecute ? 'Execute' : 'View'" aria-label="Execute" @click="execute(data)" />
            </div>
          </template>
        </Column>
        <template #empty><div class="empty"><i class="pi pi-inbox" />No executions{{ composed ? ' match the filter' : '' }}.</div></template>
      </DataTable>
    </div>

    <Dialog v-model:visible="editDialog" header="Edit run" modal style="width: min(560px, 95vw)">
      <div class="form-grid">
        <div class="field full"><label>Name</label><InputText v-model="editForm.name" /></div>
        <div class="field"><label>Environment</label><InputText v-model="editForm.environment" /></div>
        <div class="field"><label>Build</label><InputText v-model="editForm.build" /></div>
        <div class="field full"><label>Description</label><Textarea v-model="editForm.description" rows="3" auto-resize /></div>
      </div>
      <template #footer>
        <Button label="Cancel" text severity="secondary" @click="editDialog = false" />
        <Button label="Save" icon="pi pi-check" :disabled="!editForm.name.trim()" @click="saveEdit" />
      </template>
    </Dialog>

    <Dialog v-model:visible="ciDialog" header="Import results from CI" modal style="width: min(720px, 96vw)">
      <p>
        Automated pipelines can report results for this run by test key. Create an API token under
        <RouterLink :to="{ name: 'tokens' }">Administration → API tokens</RouterLink> (role tester or higher) and post:
      </p>
      <pre class="console">{{ ciSnippet }}</pre>
      <p class="small muted">
        <code>status</code> is one of {{ meta.meta.result_statuses.join(', ') }}. The response reports <code>updated</code> and any
        <code>unknown</code> test keys.
      </p>
      <Panel header="Endpoint" toggleable collapsed>
        <div class="mono">POST /api/v1/runs/{{ runId }}/results</div>
      </Panel>
      <template #footer>
        <Button label="Copy" icon="pi pi-copy" severity="secondary" @click="copySnippet" />
        <Button label="Close" @click="ciDialog = false" />
      </template>
    </Dialog>
    <ItemPickerDialog v-model:visible="picker" kind="test" :project-id="proj.pid" :exclude="executions.map((e) => e.test_case_id)" header="Add tests to run" @pick="addTests" />
  </template>
</template>
