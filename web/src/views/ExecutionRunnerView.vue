<script setup lang="ts">
import { useUndoStore } from '@/stores/undo'
import { patchEntry, recordUndo } from '@/lib/undoActions'
import MentionInput from '@/components/MentionInput.vue'
import HelpTip from '@/guide/HelpTip.vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ReadinessBanner from '@/components/ReadinessBanner.vue'
import { readinessOfError, readinessSummary } from '@/lib/readiness'
import Button from 'primevue/button'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'
import StatusTag from '@/components/StatusTag.vue'
import TagsInput from '@/components/TagsInput.vue'
import DefectLinks from '@/components/DefectLinks.vue'
import AttachmentsPanel from '@/components/AttachmentsPanel.vue'
import WebTestEvidence from '@/components/WebTestEvidence.vue'
import { jobActive, pollDelay, PLAYWRIGHT } from '@/lib/webTests'
import DefectsPanel from '@/components/DefectsPanel.vue'
import { draftFromExecution } from '@/lib/defectTracker'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import { normalizeDefects } from '@/lib/defects'
import { api, asItems, errorMessage } from '@/lib/api'
import { RESULT_COLORS } from '@/lib/colors'
import { formatDateTime, formatDuration } from '@/lib/format'
import { deriveStatus } from '@/lib/summary'
import { executionStatus, iterationStatus, resolvePlaceholders } from '@/lib/testData'
import { P } from '@/lib/permissions'
import type { Execution, Iteration, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'
import { runnerAction, RUNNER_SHORTCUTS } from '@/lib/shortcuts'

const props = defineProps<{ runId: string; executionId: string }>()
const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const router = useRouter()
const route = useRoute()
const notify = useNotify()
const { t } = useI18n()

interface StepState {
  id: string
  position: number
  action: string
  expected: string
  data: string
  status: string
  actual: string
  evidence: string
  attachments: number
  iteration_id: string | null
}

const run = ref<TestRun | null>(null)
const list = ref<Execution[]>([])
const ex = ref<Execution | null>(null)
/** the step results of all iterations */
const allSteps = ref<StepState[]>([])
/** iterations of a data-driven test (one per data row) */
const iterations = ref<Iteration[]>([])
const iter = ref(0)
const currentIteration = computed(() => iterations.value[iter.value] ?? null)
/** the steps shown: those of the current iteration */
const steps = computed(() => (currentIteration.value ? allSteps.value.filter((s) => s.iteration_id === currentIteration.value!.id) : allSteps.value))
const stepsOf = (it: Iteration) => allSteps.value.filter((s) => s.iteration_id === it.id)
/** result of an iteration: from its steps, else as set */
function iterResult(it: Iteration): string {
  const st = stepsOf(it)
  return st.length ? iterationStatus(st.map((s) => s.status)) : it.status || 'not_run'
}
/** text with the parameters of the current iteration filled in */
function resolve(text: string | undefined | null): string {
  return resolvePlaceholders(text ?? '', currentIteration.value?.values)
}
const current = ref(0)
const notes = ref('')
const assignee = ref('')
const output = ref('')
const defects = ref<string[]>([])
const AUTO = 'auto'
const override = ref<string>(AUTO)
const loading = ref(false)
const saving = ref(false)
const running = ref(false)
const snapshot = ref('')
function stateKey() {
  return JSON.stringify([
    notes.value,
    assignee.value,
    output.value,
    override.value,
    normalizeDefects(defects.value),
    allSteps.value.map((x) => [x.status, x.actual, x.evidence]),
    iterations.value.map((x) => [x.status, x.notes]),
  ])
}
const dirty = computed(() => !!ex.value && stateKey() !== snapshot.value)
const error = ref('')

const canExecute = computed(() => auth.can(P.runExecute))
/** the organisation settings forbid executing this test now (see ReadinessBanner) */
const blocked = computed(() => !!ex.value?.readiness && !ex.value.readiness.allowed)
/** results that count as executing the test (blocked/skipped/not run are always possible) */
const EXECUTING = ['passed', 'failed', 'in_progress']
const evidenceOpen = ref(false)
const resultTab = ref('result')
const attachmentCount = ref<number | null>(null)
const rTabs = 'runner-tabs'
const rPanel = (v: string) => ({ role: 'tabpanel', id: `${rTabs}_tabpanel_${v}`, 'aria-labelledby': `${rTabs}_tab_${v}` })
watch(
  () => props.executionId,
  () => {
    attachmentCount.value = null
    defectCount.value = 0
  },
)
// ---------- defects ----------
const defectCount = ref(0)
const defectsPanel = ref<InstanceType<typeof DefectsPanel> | null>(null)
/** a new defect prefilled with the test, the failed steps, the notes and the execution */
function defectDraft() {
  return draftFromExecution(ex.value!, {
    notes: notes.value,
    runName: run.value?.name,
    failedSteps: allSteps.value
      .map((s, i) => ({ ...s, n: (iterations.value.length ? steps.value.indexOf(s) : i) + 1 }))
      .filter((s) => s.status === 'failed'),
    labels: { steps: t('defectsView.draftSteps'), expected: t('runner.expected'), actual: t('runner.actual'), notes: t('runner.notes'), run: t('defectsView.run') },
  })
}
function reportDefect() {
  resultTab.value = 'defects'
  defectsPanel.value?.report()
}
const canScript = computed(() => auth.can(P.scriptExecute))
const test = computed(() => ex.value?.test)
const isAutomated = computed(() => test.value?.execution_type === 'automated')
const derived = computed(() => (iterations.value.length ? executionStatus(iterations.value.map(iterResult)) : deriveStatus(allSteps.value.map((s) => s.status))))
/** the overall result follows the steps or iterations */
const derives = computed(() => allSteps.value.length > 0 || iterations.value.length > 0)
const effectiveStatus = computed(() =>
  override.value !== AUTO ? override.value : derives.value ? derived.value : ex.value?.status || 'not_run',
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
    iterations.value = [...(e.iterations ?? [])].sort((a, b) => a.position - b.position).map((it) => ({ ...it, values: { ...(it.values ?? {}) } }))
    const iterPos = new Map(iterations.value.map((it) => [it.id, it.position]))
    allSteps.value = [...(e.step_results ?? [])]
      .sort((a, b) => (iterPos.get(a.iteration_id ?? '') ?? -1) - (iterPos.get(b.iteration_id ?? '') ?? -1) || a.position - b.position)
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
          evidence: r.evidence || '',
          attachments: r.attachment_count ?? 0,
          iteration_id: r.iteration_id ?? null,
        }
      })
    // ?step=<step result id> (links from the document archive) opens that step
    const wantedStep = typeof route.query.step === 'string' ? allSteps.value.find((s) => s.id === route.query.step) : undefined
    const firstOpenStep = allSteps.value.find((s) => s.status === 'not_run' || !s.status)
    const open = wantedStep ?? firstOpenStep
    iter.value = Math.max(0, open?.iteration_id ? iterations.value.findIndex((it) => it.id === open.iteration_id) : iterations.value.findIndex((it) => iterResult(it) === 'not_run'))
    const wanted = wantedStep ? steps.value.indexOf(wantedStep) : -1
    const firstOpen = steps.value.findIndex((s) => s.status === 'not_run' || !s.status)
    current.value = wanted >= 0 ? wanted : firstOpen >= 0 ? firstOpen : 0
    evidenceOpen.value = wanted >= 0
    notes.value = e.notes || ''
    assignee.value = e.assignee || ''
    output.value = e.output || ''
    defects.value = normalizeDefects(e.defects)
    // a result set directly on a data-driven execution stays as chosen
    override.value = !e.status || (iterations.value.length ? e.status === derived.value : allSteps.value.length) ? AUTO : e.status
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
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => {
    if (!dirty.value) void load()
    void loadList()
  },
)

function mark(status: string) {
  const s = steps.value[current.value]
  if (!s) return
  if (blocked.value && EXECUTING.includes(status)) {
    announce.value = t('readiness.blockedTitle')
    return
  }
  s.status = status
  announce.value = t('runner.marked', { n: current.value + 1, status: enumLabel('result', status) })
  if (current.value < steps.value.length - 1) current.value++
  else if (iter.value < iterations.value.length - 1) {
    // the last step of an iteration: on to the next data row
    iter.value++
    current.value = 0
    announce.value += ' ' + t('testData.nextIteration', { n: iter.value + 1, name: iterationName(iterations.value[iter.value]) })
  }
}
function iterationName(it: Iteration): string {
  return it.name || t('testData.rowN', { n: it.position + 1 })
}
function selectIteration(i: number) {
  iter.value = i
  current.value = Math.max(0, steps.value.findIndex((s) => s.status === 'not_run' || !s.status))
}
/** result of an iteration without steps */
function markIteration(status: string) {
  const it = currentIteration.value
  if (!it) return
  if (blocked.value && EXECUTING.includes(status)) {
    announce.value = t('readiness.blockedTitle')
    return
  }
  it.status = status
  announce.value = t('testData.iterationMarked', { name: iterationName(it), status: enumLabel('result', status) })
  if (iter.value < iterations.value.length - 1) iter.value++
}

const helpOpen = ref(false)
const announce = ref('')
function onKey(e: KeyboardEvent) {
  if (isAutomated.value || (!steps.value.length && !currentIteration.value)) return
  // shortcuts never fire while typing in a field or inside an open dialog/overlay
  if ((e.target as Element | null)?.closest?.('.p-dialog, .p-overlay-mask, .p-select-overlay, .p-multiselect-overlay, [role="tablist"]')) return
  const a = runnerAction(e)
  if (!a) return
  if (a.type === 'help') {
    e.preventDefault()
    helpOpen.value = !helpOpen.value
    return
  }
  if (a.type === 'mark') {
    if (!canExecute.value) return
    e.preventDefault()
    if (steps.value.length) mark(a.status)
    else markIteration(a.status)
    return
  }
  if (!steps.value.length) {
    e.preventDefault()
    iter.value = Math.min(iterations.value.length - 1, Math.max(0, iter.value + a.delta))
    return
  }
  // only arrows/j/k that are not scrolling something else
  e.preventDefault()
  current.value = Math.min(steps.value.length - 1, Math.max(0, current.value + a.delta))
}
const shortcutHelp = computed(() =>
  RUNNER_SHORTCUTS.map((s) => ({ keys: s.keys, text: t(`runner.keys.${s.action}`) })),
)

async function save(): Promise<boolean> {
  if (!ex.value) return false
  saving.value = true
  try {
    const body: Record<string, unknown> = { notes: notes.value, assignee: assignee.value, defects: normalizeDefects(defects.value) }
    if (allSteps.value.length) body.step_results = allSteps.value.map((s) => ({ id: s.id, status: s.status, actual: s.actual, evidence: s.evidence }))
    if (iterations.value.length) body.iterations = iterations.value.map((it) => (stepsOf(it).length ? { id: it.id, notes: it.notes } : { id: it.id, status: it.status, notes: it.notes }))
    if (override.value !== AUTO || !derives.value) body.status = effectiveStatus.value
    if (isAutomated.value) body.output = output.value
    const prev = ex.value
    const before: Record<string, unknown> = { notes: prev.notes ?? '', assignee: prev.assignee ?? '', defects: normalizeDefects(prev.defects), status: prev.status || 'not_run' }
    if (body.step_results) before.step_results = (prev.step_results ?? []).map((s) => ({ id: s.id, status: s.status, actual: s.actual, evidence: s.evidence }))
    if (body.iterations) before.iterations = (prev.iterations ?? []).map((it) => ({ id: it.id, status: it.status, notes: it.notes }))
    if (body.iterations && !body.status) before.status = prev.status || 'not_run'
    if (body.output !== undefined) before.output = prev.output ?? ''
    const updated = await api.patch<Execution>(`/executions/${ex.value.id}`, body)
    recordUndo(patchEntry(t('undo.a.result', { key: prev.test?.key ?? '' }), `/executions/${prev.id}`, before, body))
    if (updated) ex.value = { ...ex.value, ...updated, test: ex.value.test }
    for (const it of updated?.iterations ?? []) {
      const mine = iterations.value.find((x) => x.id === it.id)
      if (mine) mine.status = it.status
    }
    const row = list.value.find((e) => e.id === ex.value?.id)
    if (row) row.status = updated?.status ?? effectiveStatus.value
    snapshot.value = stateKey()
    notify.ok(t('runner.saved'), enumLabel('result', updated?.status ?? effectiveStatus.value))
    return true
  } catch (e) {
    const rd = readinessOfError(e)
    if (rd && ex.value) {
      ex.value = { ...ex.value, readiness: rd }
      notify.failText(t('readiness.saveRefused'), readinessSummary(rd, t, enumLabel))
    } else notify.fail(e, t('runner.saveFailed'))
    void refreshReadiness()
    return false
  } finally {
    saving.value = false
  }
}

/** reload only the readiness (e.g. after the server refused a result) */
async function refreshReadiness() {
  if (!ex.value) return
  try {
    const e = await api.get<Execution>(`/executions/${ex.value.id}`)
    if (ex.value && e) ex.value = { ...ex.value, readiness: e.readiness }
  } catch {
    /* keep the old state */
  }
}

async function go(target: Execution | null, saveFirst = false) {
  if (!target) return
  if ((saveFirst || dirty.value) && canExecute.value) {
    if (!(await save())) return
  }
  router.push({ name: 'execute', params: { projectId: proj.pid, runId: props.runId, executionId: target.id } })
}

// the last execution: save, then back to the run (which completes itself
// once every test has a result), or on to a test that still has none
const OPEN = ['not_run', 'in_progress']
async function finish() {
  if (canExecute.value && !(await save())) return
  const open = list.value.filter((e) => OPEN.includes(e.status))
  if (open.length) {
    notify.info(t('runner.stillOpen', { count: open.length }, open.length))
    if (open[0].id !== ex.value?.id) router.push({ name: 'execute', params: { projectId: proj.pid, runId: props.runId, executionId: open[0].id } })
    return
  }
  notify.ok(t('runner.runCompleted'))
  router.push({ name: 'run', params: { projectId: proj.pid, runId: props.runId } })
}

// ---------- scripts and web tests ----------
const isWeb = computed(() => test.value?.script_language === PLAYWRIGHT)
const job = computed(() => ex.value?.job ?? null)
/** a web test (or a script of "Run automated tests") is queued or running in the background */
const jobRunning = computed(() => jobActive(job.value))
/** changes when a job ends, so the evidence reloads */
const evidenceKey = computed(() => (job.value ? `${job.value.id}:${job.value.state}` : ''))
let pollTimer: ReturnType<typeof setTimeout> | undefined
let pollAttempt = 0
function stopPolling() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = undefined
  pollAttempt = 0
}
function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = setTimeout(pollJob, pollDelay(pollAttempt++))
}
/** follows a background job until it ends, then shows the result */
async function pollJob() {
  pollTimer = undefined
  const id = props.executionId
  try {
    const e = await api.get<Execution>(`/executions/${id}`)
    if (id !== props.executionId || !ex.value) return
    const row = list.value.find((x) => x.id === e.id)
    if (row) row.status = e.status
    if (jobActive(e.job)) {
      ex.value = { ...ex.value, status: e.status, job: e.job }
      schedulePoll()
      return
    }
    stopPolling()
    if (dirty.value) {
      ex.value = { ...ex.value, ...e, test: e.test ?? ex.value.test }
      output.value = e.output || ''
    } else await load()
    const state = e.job?.state ?? ''
    if (state === 'passed' || state === 'failed' || state === 'skipped') notify.ok(t('webTests.finished'), enumLabel('result', e.status))
    else if (state) notify.info(t('webTests.jobEnded', { state: t(`webTests.state.${state}`) }))
    void loadList()
  } catch (err) {
    if (id === props.executionId) schedulePoll()
    else stopPolling()
    void err
  }
}
watch(
  () => [props.executionId, jobRunning.value] as const,
  ([, active]) => {
    if (active && !pollTimer) schedulePoll()
    if (!active) stopPolling()
  },
)
onBeforeUnmount(stopPolling)

async function runScript() {
  if (!ex.value) return
  running.value = true
  try {
    const updated = await api.post<Execution>(`/executions/${ex.value.id}/run-script`)
    if (updated) {
      ex.value = { ...ex.value, ...updated, test: updated.test ?? ex.value.test }
      const row = list.value.find((e) => e.id === updated.id)
      if (row) row.status = updated.status
      if (jobActive(updated.job)) {
        notify.info(t('webTests.queuedToast'), t('webTests.queuedHint'))
        schedulePoll()
        return
      }
      output.value = updated.output || ''
      override.value = AUTO
      snapshot.value = stateKey()
    }
    notify.ok(t('runner.scriptFinished'), enumLabel('result', updated?.status))
  } catch (e) {
    notify.fail(e, t('runner.scriptFailed'))
  } finally {
    running.value = false
  }
}
async function cancelJob() {
  if (!ex.value) return
  try {
    await api.post(`/executions/${ex.value.id}/run-script/cancel`)
    notify.ok(t('webTests.cancelled'))
    pollAttempt = 0
    schedulePoll()
  } catch (e) {
    notify.fail(e, t('webTests.cancelFailed'))
  }
}

// keyboard shortcuts stay P / F / B / S in every language
const stepButtons = computed(() => [
  { status: 'passed', label: t('runner.pass'), icon: 'pi pi-check', severity: 'success', key: 'P' },
  { status: 'failed', label: t('runner.fail'), icon: 'pi pi-times', severity: 'danger', key: 'F' },
  { status: 'blocked', label: t('runner.blocked'), icon: 'pi pi-ban', severity: 'warn', key: 'B' },
  { status: 'skipped', label: t('runner.skip'), icon: 'pi pi-forward', severity: 'secondary', key: 'S' },
] as const)
function dotColor(status: string) {
  return RESULT_COLORS[status] ?? RESULT_COLORS.not_run
}
const statusOptions = computed(() => [
  {
    label: derives.value ? t('runner.derived', { status: enumLabel('result', derived.value) }) : t('runner.unchanged', { status: enumLabel('result', ex.value?.status || 'not_run') }),
    value: AUTO,
  },
  // the saved result stays selectable (saving notes is not a new execution)
  ...enumOptions('result', meta.meta.result_statuses).map((o) => ({ ...o, disabled: blocked.value && EXECUTING.includes(o.value) && o.value !== ex.value?.status })),
])
</script>

<template>
  <div class="page-header">
    <div>
      <div class="row small" style="margin-bottom: 2px">
        <RouterLink :to="{ name: 'runs', params: { projectId: proj.pid } }" class="muted">{{ t('nav.operations') }}</RouterLink>
        <i class="pi pi-angle-right muted" />
        <RouterLink :to="{ name: 'run', params: { projectId: proj.pid, runId } }" class="muted">{{ run?.name ?? t('runner.runFallback') }}</RouterLink>
        <i class="pi pi-angle-right muted" />
      </div>
      <h1 class="row">
        <span class="key" style="font-size: 16px">{{ ex?.test_key }}</span>
        {{ ex?.test_title }}
      </h1>
      <div class="sub row">
        <Tag v-if="test" :value="enumLabel('executionType', isAutomated ? 'automated' : 'manual')" :icon="isAutomated ? 'pi pi-bolt' : 'pi pi-user'" severity="secondary" />
        <Tag v-if="ex?.configuration_name" :value="ex.configuration_name" icon="pi pi-sliders-h" severity="info" :aria-label="t('configurations.runsIn', { name: ex.configuration_name })" />
        <span v-if="index >= 0">{{ t('runner.executionOf', { n: index + 1, total: list.length }) }}</span>
        <span v-if="ex?.executed_by">{{ t('runner.lastExecuted', { user: ex.executed_by, date: formatDateTime(ex.finished_at) }) }}</span>
        <span v-if="ex?.duration_ms">· {{ formatDuration(ex.duration_ms) }}</span>
      </div>
    </div>
    <div class="actions">
      <Button icon="pi pi-chevron-left" :label="t('common.previous')" severity="secondary" outlined :disabled="!prevEx" @click="go(prevEx)" />
      <Button icon="pi pi-chevron-right" icon-pos="right" :label="t('common.next')" severity="secondary" outlined :disabled="!nextEx" @click="go(nextEx)" />
    </div>
  </div>

  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="loading || !ex" class="empty"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
  <template v-else>
    <ReadinessBanner :readiness="ex.readiness" :project-id="proj.pid" style="margin-bottom: 16px" />
    <div v-if="test?.preconditions || test?.description" class="card" style="margin-bottom: 16px">
      <div v-if="test?.description" style="white-space: pre-wrap">{{ resolve(test.description) }}</div>
      <div v-if="test?.preconditions" style="margin-top: 8px">
        <b class="small">{{ t('runner.preconditions') }}</b>
        <div style="white-space: pre-wrap">{{ resolve(test.preconditions) }}</div>
      </div>
    </div>

    <!-- data-driven: one iteration per data row -->
    <section v-if="iterations.length" class="card iterations" style="margin-bottom: 16px" aria-labelledby="iter-heading">
      <div class="row" style="justify-content: space-between; margin-bottom: 8px">
        <h2 id="iter-heading" class="plain-heading">{{ t('testData.iterations') }} <HelpTip id="runner.iterations" :label="t('testData.iterations')" /></h2>
        <span class="small muted">{{ t('testData.iterationsDone', { done: iterations.filter((it) => !['not_run', 'in_progress'].includes(iterResult(it))).length, total: iterations.length }) }}</span>
      </div>
      <ol class="iter-list plain-list">
        <li v-for="(it, i) in iterations" :key="it.id">
          <button type="button" :class="{ active: i === iter }" :aria-current="i === iter ? 'step' : undefined" @click="selectIteration(i)">
            <span class="step-dot" aria-hidden="true" :style="{ background: dotColor(iterResult(it)) }">{{ i + 1 }}</span>
            <span class="ellipsis">{{ iterationName(it) }}</span>
            <span class="sr-only">({{ enumLabel('result', iterResult(it)) }})</span>
          </button>
        </li>
      </ol>
      <div v-if="currentIteration" class="iter-current">
        <div class="row" style="justify-content: space-between">
          <h3 class="plain-heading" style="font-size: 14px">{{ t('testData.iterationOf', { n: iter + 1, total: iterations.length, name: iterationName(currentIteration) }) }}</h3>
          <StatusTag :value="iterResult(currentIteration)" kind="result" />
        </div>
        <dl class="iter-values">
          <template v-for="(v, k) in currentIteration.values" :key="k">
            <dt class="mono">{{ k }}</dt>
            <dd class="mono">{{ v || '—' }}</dd>
          </template>
        </dl>
        <div v-if="!steps.length && !isAutomated && canExecute" class="runner-marks" role="toolbar" :aria-label="t('testData.markIteration', { name: iterationName(currentIteration) })">
          <Button
            v-for="b in stepButtons"
            :key="b.status"
            :label="b.label"
            :icon="b.icon"
            :severity="b.severity"
            :outlined="currentIteration.status !== b.status"
            :aria-pressed="currentIteration.status === b.status"
            :aria-keyshortcuts="b.key"
            :aria-label="`${b.label} (${b.key})`"
            :disabled="blocked && EXECUTING.includes(b.status)"
            size="small"
            @click="markIteration(b.status)"
          />
        </div>
        <div v-field class="field" style="margin-top: 8px">
          <label>{{ t('testData.iterationNotes') }}</label>
          <Textarea v-model="currentIteration.notes" rows="2" auto-resize :maxlength="20000" :disabled="!canExecute" />
        </div>
      </div>
    </section>

    <!-- automated -->
    <div v-if="isAutomated" class="grid">
      <div class="card">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 class="plain-heading"><i class="pi pi-code" aria-hidden="true" /> {{ t('runner.script') }} <span class="muted small">({{ test?.script_language || 'sh' }})</span></h2>
          <span class="row nowrap">
            <Tag v-if="jobRunning" :value="t(`webTests.state.${job?.state}`)" :icon="job?.state === 'running' ? 'pi pi-spin pi-spinner' : 'pi pi-clock'" severity="info" />
            <Button v-if="jobRunning && canScript" :label="t('webTests.cancel')" icon="pi pi-times" size="small" severity="secondary" outlined @click="cancelJob" />
            <Button
              v-if="canScript"
              :label="t('runner.runScript')"
              icon="pi pi-play"
              :loading="running || jobRunning"
              :disabled="!test?.script || blocked || jobRunning"
              @click="runScript"
            />
            <span v-else class="small muted">{{ t('runner.needsPermission') }}</span>
          </span>
        </div>
        <p v-if="jobRunning" class="small muted" role="status">{{ t('webTests.queuedHint') }}</p>
        <pre class="console" tabindex="0" :aria-label="t('runner.script')" style="color: #e2e8f0">{{ test?.script || t('runner.noScript') }}</pre>
      </div>
      <div class="card">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 class="plain-heading"><i class="pi pi-desktop" aria-hidden="true" /> {{ t('runner.output') }}</h2>
          <StatusTag :value="ex.status" kind="result" />
        </div>
        <ProgressBar v-if="running || jobRunning" mode="indeterminate" :aria-label="t('runner.running')" style="height: 4px; margin-bottom: 8px" />
        <pre class="console" tabindex="0" :aria-label="t('runner.output')" aria-live="polite" :aria-busy="running || jobRunning">{{ output || t('runner.noOutput') }}</pre>
        <p v-if="job?.message && !jobRunning && job.state !== 'passed' && job.state !== 'failed'" class="small" style="margin-top: 8px; white-space: pre-wrap">{{ t(`webTests.state.${job.state}`) }}: {{ job.message }}</p>
      </div>
    </div>
    <WebTestEvidence v-if="isAutomated && isWeb" :execution-id="executionId" :reload-key="evidenceKey" style="margin-top: 16px" />

    <!-- manual -->
    <div v-else-if="steps.length" class="runner" :class="{ 'has-bar': canExecute }">
      <section class="card" aria-labelledby="steps-heading">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 id="steps-heading" class="plain-heading">{{ t('runner.steps') }}</h2><span class="small muted">{{ doneSteps }}/{{ steps.length }}</span>
        </div>
        <ProgressBar :value="Math.round((doneSteps / steps.length) * 100)" :show-value="false" :aria-label="t('runner.progress', { done: doneSteps, total: steps.length })" style="height: 6px; margin-bottom: 10px" />
        <ol class="step-list plain-list">
          <li v-for="(s, i) in steps" :key="s.id">
            <button type="button" :class="{ active: i === current }" :aria-current="i === current ? 'step' : undefined" @click="current = i">
              <span class="step-dot" aria-hidden="true" :style="{ background: dotColor(s.status) }">
                <i v-if="s.status === 'passed'" class="pi pi-check" style="font-size: 10px" />
                <i v-else-if="s.status === 'failed'" class="pi pi-times" style="font-size: 10px" />
                <i v-else-if="s.status === 'blocked'" class="pi pi-ban" style="font-size: 10px" />
                <i v-else-if="s.status === 'skipped'" class="pi pi-forward" style="font-size: 10px" />
                <template v-else>{{ i + 1 }}</template>
              </span>
              <span class="ellipsis">{{ resolve(s.action) || t('runner.stepN', { n: i + 1 }) }}</span>
              <span class="sr-only">({{ t('runner.stepN', { n: i + 1 }) }}, {{ enumLabel('result', s.status) }})</span>
            </button>
          </li>
        </ol>
      </section>
      <section class="card" v-if="steps[current]" aria-labelledby="step-heading">
        <div class="row" style="justify-content: space-between">
          <h2 id="step-heading" class="plain-heading" style="font-size: 15px">{{ t('runner.stepOf', { n: current + 1, total: steps.length }) }}</h2>
          <StatusTag :value="steps[current].status" kind="result" />
        </div>
        <div class="grid" style="gap: 12px; margin-top: 12px">
          <div class="field"><span class="field-label">{{ t('runner.action') }}</span><div class="step-box">{{ resolve(steps[current].action) || '—' }}</div></div>
          <div class="field"><span class="field-label">{{ t('runner.expected') }}</span><div class="step-box">{{ resolve(steps[current].expected) || '—' }}</div></div>
          <div v-if="steps[current].data" class="field"><span class="field-label">{{ t('runner.testData') }}</span><div class="step-box mono">{{ resolve(steps[current].data) }}</div></div>
          <div v-field class="field">
            <label>{{ t('runner.actual') }}</label>
            <Textarea v-model="steps[current].actual" rows="3" auto-resize :disabled="!canExecute" :placeholder="t('runner.actualPlaceholder')" />
          </div>
          <section class="evidence" :aria-labelledby="`evidence-${steps[current].id}`">
            <button
              :id="`evidence-${steps[current].id}`"
              type="button"
              class="evidence-toggle"
              :aria-expanded="evidenceOpen"
              :aria-controls="`evidence-body-${steps[current].id}`"
              @click="evidenceOpen = !evidenceOpen"
            >
              <i :class="evidenceOpen ? 'pi pi-chevron-down' : 'pi pi-chevron-right'" aria-hidden="true" />
              <i class="pi pi-camera" aria-hidden="true" />
              <b>{{ t('evidence.title') }}</b>
              <span v-if="steps[current].evidence || steps[current].attachments" class="count">{{ t('evidence.summary', { files: steps[current].attachments }) }}</span>
            </button>
            <HelpTip id="runner.evidence" :label="t('evidence.title')" />
            <div v-show="evidenceOpen" :id="`evidence-body-${steps[current].id}`" class="grid" style="gap: 8px; margin-top: 8px">
              <div v-field class="field">
                <label>{{ t('evidence.text') }}</label>
                <Textarea v-model="steps[current].evidence" rows="2" auto-resize :maxlength="20000" :disabled="!canExecute" :placeholder="t('evidence.placeholder')" />
              </div>
              <div>
                <span class="field-label">{{ t('evidence.files') }}</span>
                <AttachmentsPanel
                  v-if="evidenceOpen"
                  :key="steps[current].id"
                  type="step"
                  :entity-id="steps[current].id"
                  :project-id="proj.pid"
                  :can-write="canExecute"
                  @count="(n) => steps[current] && (steps[current].attachments = n)"
                />
              </div>
            </div>
          </section>
          <div v-if="canExecute" class="runner-bar" role="toolbar" :aria-label="t('runner.markStep', { n: current + 1 })">
            <div class="runner-marks">
              <Button
                v-for="b in stepButtons"
                :key="b.status"
                :label="b.label"
                :icon="b.icon"
                :severity="b.severity"
                :outlined="steps[current].status !== b.status"
                :aria-pressed="steps[current].status === b.status"
                :aria-keyshortcuts="b.key"
                :aria-label="`${b.label} (${b.key})`"
                :disabled="blocked && EXECUTING.includes(b.status)"
                class="runner-mark"
                v-tooltip.bottom="t('runner.shortcut', { key: b.key })"
                @click="mark(b.status)"
              />
            </div>
            <div class="runner-steps">
              <Button icon="pi pi-arrow-up" text rounded severity="secondary" :disabled="current === 0" :aria-label="t('runner.prevStep')" v-tooltip="t('runner.prevStep')" @click="current--" />
              <Button icon="pi pi-arrow-down" text rounded severity="secondary" :disabled="current === steps.length - 1" :aria-label="t('runner.nextStep')" v-tooltip="t('runner.nextStep')" @click="current++" />
            </div>
          </div>
          <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>
          <div class="shortcuts">
            <Button
              :label="t('runner.keyboardHelp')"
              icon="pi pi-question-circle"
              text
              size="small"
              severity="secondary"
              aria-controls="runner-keys"
              :aria-expanded="helpOpen"
              aria-keyshortcuts="?"
              @click="helpOpen = !helpOpen"
            />
            <div v-show="helpOpen" id="runner-keys" class="shortcut-help" role="region" :aria-label="t('runner.keyboardHelp')">
              <p class="small muted" style="margin: 0 0 6px">{{ t('runner.keyboardHint') }}</p>
              <dl>
                <template v-for="h in shortcutHelp" :key="h.text">
                  <dt><template v-for="(k, i) in h.keys" :key="k"><kbd>{{ k }}</kbd><span v-if="i < h.keys.length - 1"> / </span></template></dt>
                  <dd>{{ h.text }}</dd>
                </template>
              </dl>
            </div>
          </div>
        </div>
      </section>
    </div>
    <div v-else class="card">
      <div class="muted">{{ t('runner.noSteps') }}</div>
      <div v-if="test?.expected_result" style="margin-top: 8px"><b class="small">{{ t('runner.expected') }}</b><div class="step-box">{{ resolve(test.expected_result) }}</div></div>
    </div>

    <Tabs :id="rTabs" v-model:value="resultTab" class="editor-tabs" scrollable style="margin-top: 16px">
      <TabList>
        <Tab value="result"><i class="pi pi-check-circle" aria-hidden="true" /> {{ t('runner.resultTab') }}</Tab>
        <Tab value="attachments"><i class="pi pi-paperclip" aria-hidden="true" /> {{ t('files.tab') }} <span v-if="attachmentCount !== null" class="count">{{ attachmentCount }}</span></Tab>
        <Tab value="defects"><i class="pi pi-exclamation-circle" aria-hidden="true" /> {{ t('defectsView.tab') }} <span v-if="defectCount" class="count" :title="t('defectsView.openCount', { n: defectCount }, defectCount)">{{ defectCount }}</span></Tab>
      </TabList>
    </Tabs>
    <div v-show="resultTab === 'defects'" v-bind="rPanel('defects')" class="card">
      <DefectsPanel ref="defectsPanel" entity-type="execution" :entity-id="executionId" :project-id="proj.pid" :draft="defectDraft" @count="(n) => (defectCount = n)" />
    </div>
    <div v-if="resultTab === 'attachments'" v-bind="rPanel('attachments')" class="card">
      <AttachmentsPanel type="execution" :entity-id="executionId" :project-id="proj.pid" :can-write="canExecute" @count="(n) => (attachmentCount = n)" />
    </div>
    <div v-show="resultTab === 'result'" v-bind="rPanel('result')" class="card">
      <Message v-if="canExecute && effectiveStatus === 'failed'" severity="warn" :closable="false" style="margin-bottom: 12px">
        <span class="row" style="justify-content: space-between">
          <span>{{ defectCount ? t('defectsView.failedHasDefects', { n: defectCount }, defectCount) : t('defectsView.failedHint') }}</span>
          <Button :label="t('defectsView.report')" icon="pi pi-exclamation-circle" size="small" severity="danger" @click="reportDefect" />
        </span>
      </Message>
      <div class="form-grid">
        <div v-field class="field">
          <label>{{ t('runner.overallStatus') }}</label>
          <HelpTip id="runner.status" :label="t('runner.overallStatus')" />
          <Select v-model="override" :options="statusOptions" option-label="label" option-value="value" option-disabled="disabled" :disabled="!canExecute" />
          <span class="small" aria-live="polite">{{ t('runner.savedAs') }} <StatusTag :value="effectiveStatus" kind="result" /></span>
        </div>
        <div v-field class="field">
          <label>{{ t('runner.assignee') }}</label>
          <HelpTip id="runner.assignee" :label="t('runner.assignee')" />
          <MentionInput v-model="assignee" :disabled="!canExecute" :placeholder="t('runner.unassigned')" />
        </div>
        <div v-field class="field full">
          <label>{{ t('runner.defects') }}</label>
          <HelpTip id="runner.defects" :label="t('runner.defects')" />
          <TagsInput v-model="defects" plain :disabled="!canExecute" icon="pi pi-exclamation-circle" :placeholder="t('runner.defectsPlaceholder')" />
          <span class="small row" style="gap: 4px">
            <DefectLinks :defects="defects" />
            <span v-if="defects.length && !proj.project?.defect_url_template" class="muted">{{ t('runner.defectsHint') }}</span>
          </span>
        </div>
        <div v-field class="field full">
          <label>{{ t('runner.notes') }}</label>
          <HelpTip id="runner.notes" :label="t('runner.notes')" />
          <MentionInput v-model="notes" multiline rows="3" auto-resize :disabled="!canExecute" :placeholder="t('runner.notesPlaceholder')" />
        </div>
      </div>
      <div v-if="canExecute" class="row" style="justify-content: flex-end; margin-top: 12px">
        <span v-if="dirty" class="small muted" style="margin-right: auto"><i class="pi pi-circle-fill" aria-hidden="true" style="font-size: 8px; color: var(--p-amber-500)" /> {{ t('runner.unsaved') }}</span>
        <Button :label="t('common.save')" icon="pi pi-save" severity="secondary" :loading="saving" @click="save" />
        <Button v-if="nextEx" :label="t('runner.saveNext')" icon="pi pi-angle-double-right" icon-pos="right" :loading="saving" @click="go(nextEx, true)" />
        <Button v-else :label="t('runner.saveFinish')" icon="pi pi-flag-fill" icon-pos="right" :loading="saving" @click="finish" />
      </div>
    </div>
  </template>
</template>

<style>
.iter-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0 0 10px;
}
.iter-list button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 220px;
  padding: 4px 10px 4px 4px;
  border: 1px solid var(--qtf-border);
  border-radius: 999px;
  background: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.iter-list button.active {
  border-color: var(--p-primary-color);
  box-shadow: inset 0 0 0 1px var(--p-primary-color);
}
.iter-list button:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
.iter-values {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 2px 12px;
  margin: 8px 0;
}
.iter-values dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.evidence {
  border: 1px dashed var(--qtf-border);
  border-radius: 8px;
  padding: 8px 10px;
}
.evidence-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.evidence-toggle:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
</style>
