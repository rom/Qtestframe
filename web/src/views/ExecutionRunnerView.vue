<script setup lang="ts">
import MentionInput from '@/components/MentionInput.vue'
import HelpTip from '@/guide/HelpTip.vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
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
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import { normalizeDefects } from '@/lib/defects'
import { api, asItems, errorMessage } from '@/lib/api'
import { RESULT_COLORS } from '@/lib/colors'
import { formatDateTime, formatDuration } from '@/lib/format'
import { deriveStatus } from '@/lib/summary'
import { P } from '@/lib/permissions'
import type { Execution, TestRun } from '@/lib/types'
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
}

const run = ref<TestRun | null>(null)
const list = ref<Execution[]>([])
const ex = ref<Execution | null>(null)
const steps = ref<StepState[]>([])
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
  return JSON.stringify([notes.value, assignee.value, output.value, override.value, normalizeDefects(defects.value), steps.value.map((x) => [x.status, x.actual])])
}
const dirty = computed(() => !!ex.value && stateKey() !== snapshot.value)
const error = ref('')

const canExecute = computed(() => auth.can(P.runExecute))
const resultTab = ref('result')
const attachmentCount = ref<number | null>(null)
const rTabs = 'runner-tabs'
const rPanel = (v: string) => ({ role: 'tabpanel', id: `${rTabs}_tabpanel_${v}`, 'aria-labelledby': `${rTabs}_tab_${v}` })
watch(() => props.executionId, () => (attachmentCount.value = null))
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
    defects.value = normalizeDefects(e.defects)
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
  announce.value = t('runner.marked', { n: current.value + 1, status: enumLabel('result', status) })
  if (current.value < steps.value.length - 1) current.value++
}

const helpOpen = ref(false)
const announce = ref('')
function onKey(e: KeyboardEvent) {
  if (isAutomated.value || !steps.value.length) return
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
    mark(a.status)
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
    if (steps.value.length) body.step_results = steps.value.map((s) => ({ id: s.id, status: s.status, actual: s.actual }))
    if (override.value !== AUTO || !steps.value.length) body.status = effectiveStatus.value
    if (isAutomated.value) body.output = output.value
    const updated = await api.patch<Execution>(`/executions/${ex.value.id}`, body)
    if (updated) ex.value = { ...ex.value, ...updated, test: ex.value.test }
    const row = list.value.find((e) => e.id === ex.value?.id)
    if (row) row.status = updated?.status ?? effectiveStatus.value
    snapshot.value = stateKey()
    notify.ok(t('runner.saved'), enumLabel('result', updated?.status ?? effectiveStatus.value))
    return true
  } catch (e) {
    notify.fail(e, t('runner.saveFailed'))
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
    notify.ok(t('runner.scriptFinished'), enumLabel('result', updated?.status))
  } catch (e) {
    notify.fail(e, t('runner.scriptFailed'))
  } finally {
    running.value = false
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
    label: steps.value.length ? t('runner.derived', { status: enumLabel('result', derived.value) }) : t('runner.unchanged', { status: enumLabel('result', ex.value?.status || 'not_run') }),
    value: AUTO,
  },
  ...enumOptions('result', meta.meta.result_statuses),
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
    <div v-if="test?.preconditions || test?.description" class="card" style="margin-bottom: 16px">
      <div v-if="test?.description" style="white-space: pre-wrap">{{ test.description }}</div>
      <div v-if="test?.preconditions" style="margin-top: 8px">
        <b class="small">{{ t('runner.preconditions') }}</b>
        <div style="white-space: pre-wrap">{{ test.preconditions }}</div>
      </div>
    </div>

    <!-- automated -->
    <div v-if="isAutomated" class="grid">
      <div class="card">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 class="plain-heading"><i class="pi pi-code" aria-hidden="true" /> {{ t('runner.script') }} <span class="muted small">({{ test?.script_language || 'sh' }})</span></h2>
          <Button
            v-if="canScript"
            :label="t('runner.runScript')"
            icon="pi pi-play"
            :loading="running"
            :disabled="!test?.script"
            @click="runScript"
          />
          <span v-else class="small muted">{{ t('runner.needsPermission') }}</span>
        </div>
        <pre class="console" tabindex="0" :aria-label="t('runner.script')" style="color: #e2e8f0">{{ test?.script || t('runner.noScript') }}</pre>
      </div>
      <div class="card">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 class="plain-heading"><i class="pi pi-desktop" aria-hidden="true" /> {{ t('runner.output') }}</h2>
          <StatusTag :value="ex.status" kind="result" />
        </div>
        <ProgressBar v-if="running" mode="indeterminate" :aria-label="t('runner.running')" style="height: 4px; margin-bottom: 8px" />
        <pre class="console" tabindex="0" :aria-label="t('runner.output')" aria-live="polite" :aria-busy="running">{{ output || t('runner.noOutput') }}</pre>
      </div>
    </div>

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
              <span class="ellipsis">{{ s.action || t('runner.stepN', { n: i + 1 }) }}</span>
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
          <div class="field"><span class="field-label">{{ t('runner.action') }}</span><div class="step-box">{{ steps[current].action || '—' }}</div></div>
          <div class="field"><span class="field-label">{{ t('runner.expected') }}</span><div class="step-box">{{ steps[current].expected || '—' }}</div></div>
          <div v-if="steps[current].data" class="field"><span class="field-label">{{ t('runner.testData') }}</span><div class="step-box mono">{{ steps[current].data }}</div></div>
          <div v-field class="field">
            <label>{{ t('runner.actual') }}</label>
            <Textarea v-model="steps[current].actual" rows="3" auto-resize :disabled="!canExecute" :placeholder="t('runner.actualPlaceholder')" />
          </div>
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
      <div v-if="test?.expected_result" style="margin-top: 8px"><b class="small">{{ t('runner.expected') }}</b><div class="step-box">{{ test.expected_result }}</div></div>
    </div>

    <Tabs :id="rTabs" v-model:value="resultTab" class="editor-tabs" scrollable style="margin-top: 16px">
      <TabList>
        <Tab value="result"><i class="pi pi-check-circle" aria-hidden="true" /> {{ t('runner.resultTab') }}</Tab>
        <Tab value="attachments"><i class="pi pi-paperclip" aria-hidden="true" /> {{ t('files.tab') }} <span v-if="attachmentCount !== null" class="count">{{ attachmentCount }}</span></Tab>
      </TabList>
    </Tabs>
    <div v-if="resultTab === 'attachments'" v-bind="rPanel('attachments')" class="card">
      <AttachmentsPanel type="execution" :entity-id="executionId" :project-id="proj.pid" :can-write="canExecute" @count="(n) => (attachmentCount = n)" />
    </div>
    <div v-show="resultTab === 'result'" v-bind="rPanel('result')" class="card">
      <div class="form-grid">
        <div v-field class="field">
          <label>{{ t('runner.overallStatus') }}</label>
          <HelpTip id="runner.status" :label="t('runner.overallStatus')" />
          <Select v-model="override" :options="statusOptions" option-label="label" option-value="value" :disabled="!canExecute" />
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
          <TagsInput v-model="defects" :disabled="!canExecute" icon="pi pi-exclamation-circle" :placeholder="t('runner.defectsPlaceholder')" />
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
        <Button :label="t('runner.saveNext')" icon="pi pi-angle-double-right" icon-pos="right" :loading="saving" :disabled="!nextEx" @click="go(nextEx, true)" />
      </div>
    </div>
  </template>
</template>
