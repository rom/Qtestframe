<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'
import StatusTag from '@/components/StatusTag.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { RESULT_COLORS } from '@/lib/colors'
import { formatDateTime, formatDuration, humanize, options } from '@/lib/format'
import { deriveStatus } from '@/lib/summary'
import { P } from '@/lib/permissions'
import type { Execution, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ runId: string; executionId: string }>()
const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const router = useRouter()
const notify = useNotify()

interface StepState {
  id: string
  position: number
  action: string
  expected: string
  data: string
  status: string
  actual: string
}

const run = ref<TestRun | null>(null)
const list = ref<Execution[]>([])
const ex = ref<Execution | null>(null)
const steps = ref<StepState[]>([])
const current = ref(0)
const notes = ref('')
const assignee = ref('')
const output = ref('')
const AUTO = 'auto'
const override = ref<string>(AUTO)
const loading = ref(false)
const saving = ref(false)
const running = ref(false)
const snapshot = ref('')
function stateKey() {
  return JSON.stringify([notes.value, assignee.value, output.value, override.value, steps.value.map((x) => [x.status, x.actual])])
}
const dirty = computed(() => !!ex.value && stateKey() !== snapshot.value)
const error = ref('')

const canExecute = computed(() => auth.can(P.runExecute))
const canScript = computed(() => auth.can(P.scriptExecute))
const test = computed(() => ex.value?.test)
const isAutomated = computed(() => test.value?.execution_type === 'automated')
const derived = computed(() => deriveStatus(steps.value.map((s) => s.status)))
const effectiveStatus = computed(() =>
  override.value !== AUTO ? override.value : steps.value.length ? derived.value : ex.value?.status || 'not_run',
)
const index = computed(() => list.value.findIndex((e) => e.id === props.executionId))
const prevEx = computed(() => (index.value > 0 ? list.value[index.value - 1] : null))
const nextEx = computed(() => (index.value >= 0 && index.value < list.value.length - 1 ? list.value[index.value + 1] : null))
const doneSteps = computed(() => steps.value.filter((s) => s.status && s.status !== 'not_run' && s.status !== 'in_progress').length)

async function loadList() {
  try {
    const [r, l] = await Promise.all([
      api.get<TestRun>(`/runs/${props.runId}`),
      api.get<{ items: Execution[] }>(`/runs/${props.runId}/executions`),
    ])
    run.value = r
    list.value = asItems(l).sort((a, b) => a.position - b.position)
  } catch (e) {
    notify.fail(e)
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const e = await api.get<Execution>(`/executions/${props.executionId}`)
    ex.value = e
    const testSteps = new Map((e.test?.steps ?? []).map((s) => [s.id, s]))
    steps.value = [...(e.step_results ?? [])]
      .sort((a, b) => a.position - b.position)
      .map((r) => {
        const ts = testSteps.get(r.step_id)
        return {
          id: r.id,
          position: r.position,
          action: r.action || ts?.action || '',
          expected: r.expected || ts?.expected || '',
          data: r.data ?? ts?.data ?? '',
          status: r.status || 'not_run',
          actual: r.actual || '',
        }
      })
    const firstOpen = steps.value.findIndex((s) => s.status === 'not_run' || !s.status)
    current.value = firstOpen >= 0 ? firstOpen : 0
    notes.value = e.notes || ''
    assignee.value = e.assignee || ''
    output.value = e.output || ''
    override.value = steps.value.length || !e.status ? AUTO : e.status
    snapshot.value = stateKey()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadList()
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
watch(() => props.executionId, load, { immediate: true })

function mark(status: string) {
  const s = steps.value[current.value]
  if (!s) return
  s.status = status
  if (current.value < steps.value.length - 1) current.value++
}

function onKey(e: KeyboardEvent) {
  const t = e.target as HTMLElement | null
  if (!canExecute.value || isAutomated.value || !steps.value.length) return
  if (t && (t.tagName === 'TEXTAREA' || t.tagName === 'INPUT' || t.isContentEditable)) return
  if (e.ctrlKey || e.metaKey || e.altKey) return
  const map: Record<string, string> = { p: 'passed', f: 'failed', b: 'blocked', s: 'skipped' }
  if (map[e.key]) {
    e.preventDefault()
    mark(map[e.key])
  } else if (e.key === 'ArrowDown' || e.key === 'j') current.value = Math.min(steps.value.length - 1, current.value + 1)
  else if (e.key === 'ArrowUp' || e.key === 'k') current.value = Math.max(0, current.value - 1)
}

async function save(): Promise<boolean> {
  if (!ex.value) return false
  saving.value = true
  try {
    const body: Record<string, unknown> = { notes: notes.value, assignee: assignee.value }
    if (steps.value.length) body.step_results = steps.value.map((s) => ({ id: s.id, status: s.status, actual: s.actual }))
    if (override.value !== AUTO || !steps.value.length) body.status = effectiveStatus.value
    if (isAutomated.value) body.output = output.value
    const updated = await api.patch<Execution>(`/executions/${ex.value.id}`, body)
    if (updated) ex.value = { ...ex.value, ...updated, test: ex.value.test }
    const row = list.value.find((e) => e.id === ex.value?.id)
    if (row) row.status = updated?.status ?? effectiveStatus.value
    snapshot.value = stateKey()
    notify.ok('Execution saved', humanize(updated?.status ?? effectiveStatus.value))
    return true
  } catch (e) {
    notify.fail(e, 'Could not save')
    return false
  } finally {
    saving.value = false
  }
}

async function go(target: Execution | null, saveFirst = false) {
  if (!target) return
  if ((saveFirst || dirty.value) && canExecute.value) {
    if (!(await save())) return
  }
  router.push({ name: 'execute', params: { projectId: proj.pid, runId: props.runId, executionId: target.id } })
}

async function runScript() {
  if (!ex.value) return
  running.value = true
  try {
    const updated = await api.post<Execution>(`/executions/${ex.value.id}/run-script`)
    if (updated) {
      ex.value = { ...ex.value, ...updated, test: updated.test ?? ex.value.test }
      output.value = updated.output || ''
      override.value = AUTO
      snapshot.value = stateKey()
      const row = list.value.find((e) => e.id === updated.id)
      if (row) row.status = updated.status
    }
    notify.ok('Script finished', humanize(updated?.status))
  } catch (e) {
    notify.fail(e, 'Script execution failed')
  } finally {
    running.value = false
  }
}

const stepButtons = [
  { status: 'passed', label: 'Pass', icon: 'pi pi-check', severity: 'success', key: 'P' },
  { status: 'failed', label: 'Fail', icon: 'pi pi-times', severity: 'danger', key: 'F' },
  { status: 'blocked', label: 'Blocked', icon: 'pi pi-ban', severity: 'warn', key: 'B' },
  { status: 'skipped', label: 'Skip', icon: 'pi pi-forward', severity: 'secondary', key: 'S' },
] as const
function dotColor(status: string) {
  return RESULT_COLORS[status] ?? RESULT_COLORS.not_run
}
const statusOptions = computed(() => [
  { label: steps.value.length ? `Derived from steps (${humanize(derived.value)})` : `Unchanged (${humanize(ex.value?.status || 'not_run')})`, value: AUTO },
  ...options(meta.meta.result_statuses),
])
</script>

<template>
  <div class="page-header">
    <div>
      <div class="row small" style="margin-bottom: 2px">
        <RouterLink :to="{ name: 'runs', params: { projectId: proj.pid } }" class="muted">Operations</RouterLink>
        <i class="pi pi-angle-right muted" />
        <RouterLink :to="{ name: 'run', params: { projectId: proj.pid, runId } }" class="muted">{{ run?.name ?? 'Run' }}</RouterLink>
        <i class="pi pi-angle-right muted" />
      </div>
      <h1 class="row">
        <span class="key" style="font-size: 16px">{{ ex?.test_key }}</span>
        {{ ex?.test_title }}
      </h1>
      <div class="sub row">
        <Tag v-if="test" :value="isAutomated ? 'Automated' : 'Manual'" :icon="isAutomated ? 'pi pi-bolt' : 'pi pi-user'" severity="secondary" />
        <span v-if="index >= 0">Execution {{ index + 1 }} of {{ list.length }}</span>
        <span v-if="ex?.executed_by">· last executed by {{ ex.executed_by }} {{ formatDateTime(ex.finished_at) }}</span>
        <span v-if="ex?.duration_ms">· {{ formatDuration(ex.duration_ms) }}</span>
      </div>
    </div>
    <div class="actions">
      <Button icon="pi pi-chevron-left" label="Previous" severity="secondary" outlined :disabled="!prevEx" @click="go(prevEx)" />
      <Button icon="pi pi-chevron-right" icon-pos="right" label="Next" severity="secondary" outlined :disabled="!nextEx" @click="go(nextEx)" />
    </div>
  </div>

  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="loading || !ex" class="empty"><ProgressSpinner style="width: 36px; height: 36px" /></div>
  <template v-else>
    <div v-if="test?.preconditions || test?.description" class="card" style="margin-bottom: 16px">
      <div v-if="test?.description" style="white-space: pre-wrap">{{ test.description }}</div>
      <div v-if="test?.preconditions" style="margin-top: 8px">
        <b class="small">Preconditions</b>
        <div style="white-space: pre-wrap">{{ test.preconditions }}</div>
      </div>
    </div>

    <!-- automated -->
    <div v-if="isAutomated" class="grid">
      <div class="card">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h3 style="margin: 0"><i class="pi pi-code" /> Script <span class="muted small">({{ test?.script_language || 'sh' }})</span></h3>
          <Button
            v-if="canScript"
            label="Run script"
            icon="pi pi-play"
            :loading="running"
            :disabled="!test?.script"
            @click="runScript"
          />
          <span v-else class="small muted">Running scripts requires the script.execute permission.</span>
        </div>
        <pre class="console" style="color: #e2e8f0">{{ test?.script || '# no script' }}</pre>
      </div>
      <div class="card">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h3 style="margin: 0"><i class="pi pi-desktop" /> Output</h3>
          <StatusTag :value="ex.status" kind="result" />
        </div>
        <ProgressBar v-if="running" mode="indeterminate" style="height: 4px; margin-bottom: 8px" />
        <pre class="console">{{ output || 'No output yet.' }}</pre>
      </div>
    </div>

    <!-- manual -->
    <div v-else-if="steps.length" class="runner">
      <div class="card">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <b>Steps</b><span class="small muted">{{ doneSteps }}/{{ steps.length }}</span>
        </div>
        <ProgressBar :value="Math.round((doneSteps / steps.length) * 100)" :show-value="false" style="height: 6px; margin-bottom: 10px" />
        <div class="step-list">
          <button v-for="(s, i) in steps" :key="s.id" :class="{ active: i === current }" @click="current = i">
            <span class="step-dot" :style="{ background: dotColor(s.status) }">
              <i v-if="s.status === 'passed'" class="pi pi-check" style="font-size: 10px" />
              <i v-else-if="s.status === 'failed'" class="pi pi-times" style="font-size: 10px" />
              <template v-else>{{ i + 1 }}</template>
            </span>
            <span class="ellipsis">{{ s.action || 'Step ' + (i + 1) }}</span>
          </button>
        </div>
      </div>
      <div class="card" v-if="steps[current]">
        <div class="row" style="justify-content: space-between">
          <h3 style="margin: 0">Step {{ current + 1 }} of {{ steps.length }}</h3>
          <StatusTag :value="steps[current].status" kind="result" />
        </div>
        <div class="grid" style="gap: 12px; margin-top: 12px">
          <div class="field"><label>Action</label><div class="step-box">{{ steps[current].action || '—' }}</div></div>
          <div class="field"><label>Expected result</label><div class="step-box">{{ steps[current].expected || '—' }}</div></div>
          <div v-if="steps[current].data" class="field"><label>Test data</label><div class="step-box mono">{{ steps[current].data }}</div></div>
          <div class="field">
            <label>Actual result</label>
            <Textarea v-model="steps[current].actual" rows="3" auto-resize :disabled="!canExecute" placeholder="What happened?" />
          </div>
          <div v-if="canExecute" class="row">
            <Button
              v-for="b in stepButtons"
              :key="b.status"
              :label="b.label"
              :icon="b.icon"
              :severity="b.severity"
              :outlined="steps[current].status !== b.status"
              v-tooltip.bottom="'Shortcut: ' + b.key"
              @click="mark(b.status)"
            />
            <span class="grow" />
            <Button icon="pi pi-arrow-up" text rounded severity="secondary" :disabled="current === 0" aria-label="Previous step" @click="current--" />
            <Button icon="pi pi-arrow-down" text rounded severity="secondary" :disabled="current === steps.length - 1" aria-label="Next step" @click="current++" />
          </div>
        </div>
      </div>
    </div>
    <div v-else class="card">
      <div class="muted">This test has no steps. Record the overall result below.</div>
      <div v-if="test?.expected_result" style="margin-top: 8px"><b class="small">Expected result</b><div class="step-box">{{ test.expected_result }}</div></div>
    </div>

    <div class="card" style="margin-top: 16px">
      <div class="form-grid">
        <div class="field">
          <label>Overall status</label>
          <Select v-model="override" :options="statusOptions" option-label="label" option-value="value" :disabled="!canExecute" />
          <span class="small">Will be saved as <StatusTag :value="effectiveStatus" kind="result" /></span>
        </div>
        <div class="field">
          <label>Assignee</label>
          <InputText v-model="assignee" :disabled="!canExecute" placeholder="Unassigned" />
        </div>
        <div class="field full">
          <label>Notes</label>
          <Textarea v-model="notes" rows="3" auto-resize :disabled="!canExecute" placeholder="Defects, observations, links…" />
        </div>
      </div>
      <div v-if="canExecute" class="row" style="justify-content: flex-end; margin-top: 12px">
        <span v-if="dirty" class="small muted" style="margin-right: auto"><i class="pi pi-circle-fill" style="font-size: 8px; color: var(--p-amber-500)" /> Unsaved changes</span>
        <Button label="Save" icon="pi pi-save" severity="secondary" :loading="saving" @click="save" />
        <Button label="Save & next" icon="pi pi-angle-double-right" icon-pos="right" :loading="saving" :disabled="!nextEx" @click="go(nextEx, true)" />
      </div>
    </div>
  </template>
</template>
