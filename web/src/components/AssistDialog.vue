<script setup lang="ts">
// Runs one AI request (streamed, cancellable) and shows its suggestions for
// review: everything is editable and nothing is saved until the user
// accepts. Drafted tests and defects are created here (mode "create");
// texts, criteria and mappings are handed to the editor (emit "apply"), which
// saves them with its own Save. Tokens, model, time and the estimated cost
// are shown with the answer.
import { computed, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import RadioButton from 'primevue/radiobutton'
import { ApiError, api, errorMessage } from '@/lib/api'
import {
  clearAiStatus,
  draftTestBody,
  formatCost,
  runAssist,
  splitFixed,
  type AiFeature,
  type AiMeta,
  type CriteriaResult,
  type DraftTest,
  type DraftTestsResult,
  type ImproveResult,
  type MappingsResult,
  type RunSummaryResult,
  type SuggestedCriterion,
  type SuggestedMapping,
} from '@/lib/ai'
import { diffWords } from '@/lib/diff'
import { issueMessage } from '@/i18n/analysis'
import { enumLabel } from '@/i18n/labels'
import { DEFECT_SEVERITIES } from '@/lib/defectTracker'

export type AssistApply =
  | { feature: 'improve_requirement'; title: string; description: string }
  | { feature: 'suggest_criteria'; criteria: SuggestedCriterion[] }
  | { feature: 'suggest_mappings'; mappings: SuggestedMapping[] }
  | { feature: 'draft_tests'; test: DraftTest }

const props = defineProps<{
  projectId: string
  feature: AiFeature | null
  /** the request body (item id, the editor's text) without lang */
  body: Record<string, unknown>
  /** drafted tests: create them linked to the requirement, or hand one to the editor */
  mode?: 'create' | 'emit'
}>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ apply: [value: AssistApply]; created: [count: number] }>()
const { t, locale } = useI18n()
const uid = useId()

type Phase = 'running' | 'done' | 'error' | 'cancelled'
const phase = ref<Phase>('running')
const chars = ref(0)
const meta = ref<AiMeta | null>(null)
const error = ref('')
const saving = ref(false)
const saveError = ref('')
let ctrl: AbortController | null = null

// editable copies of the suggestions
const tests = ref<(DraftTest & { pick: boolean; open: boolean })[]>([])
const chosenTest = ref(0)
const requirement = ref<{ id: string; key: string }>({ id: '', key: '' })
const improve = ref<ImproveResult | null>(null)
const improved = ref({ title: '', description: '' })
const criteria = ref<(SuggestedCriterion & { pick: boolean })[]>([])
const mappings = ref<(SuggestedMapping & { pick: boolean })[]>([])
const dropped = ref<string[]>([])
const mappingKind = ref<'requirement' | 'risk'>('requirement')
const run = ref<RunSummaryResult | null>(null)
const defects = ref<(RunSummaryResult['defects'][number] & { pick: boolean })[]>([])

const lang = computed(() => (String(locale.value).startsWith('sv') ? 'sv' : 'en'))
const header = computed(() => (props.feature ? t(`ai.action.${props.feature}`) : ''))

function reset() {
  phase.value = 'running'
  chars.value = 0
  meta.value = null
  error.value = ''
  saveError.value = ''
  tests.value = []
  improve.value = null
  criteria.value = []
  mappings.value = []
  dropped.value = []
  run.value = null
  defects.value = []
}

function take(feature: AiFeature, result: unknown) {
  switch (feature) {
    case 'draft_tests': {
      const r = result as DraftTestsResult
      requirement.value = { id: r.requirement_id, key: r.requirement_key }
      tests.value = r.tests.map((x) => ({ ...x, steps: x.steps.map((s) => ({ ...s })), pick: true, open: false }))
      chosenTest.value = 0
      break
    }
    case 'improve_requirement': {
      const r = result as ImproveResult
      improve.value = r
      improved.value = { title: r.title, description: r.description }
      break
    }
    case 'suggest_criteria':
      criteria.value = (result as CriteriaResult).criteria.map((c) => ({ ...c, threshold: c.threshold ?? '', pick: true }))
      break
    case 'suggest_mappings': {
      const r = result as MappingsResult
      mappingKind.value = r.kind
      mappings.value = r.mappings.map((m) => ({ ...m, pick: true }))
      dropped.value = r.dropped.map((d) => d.id)
      break
    }
    case 'summarize_run': {
      const r = result as RunSummaryResult
      run.value = r
      defects.value = r.defects.map((d) => ({ ...d, pick: true }))
      break
    }
  }
}

async function start() {
  if (!props.feature) return
  const feature = props.feature
  reset()
  ctrl?.abort()
  ctrl = new AbortController()
  try {
    const ans = await runAssist(props.projectId, feature, { ...props.body, lang: lang.value }, { signal: ctrl.signal, onProgress: (n) => (chars.value = n) })
    meta.value = ans.meta
    take(feature, ans.result)
    phase.value = 'done'
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') {
      phase.value = 'cancelled'
      return
    }
    phase.value = 'error'
    if (e instanceof ApiError) {
      if (e.code === 'ai_unavailable') clearAiStatus(props.projectId)
      const known = ['ai_unavailable', 'rate_limited', 'ai_refused', 'ai_service', 'ai_invalid_answer', 'ai_timeout']
      error.value = known.includes(e.code) ? t(`ai.errors.${e.code}`, { error: e.message }) : e.message
    } else error.value = errorMessage(e)
  } finally {
    ctrl = null
  }
}

function cancel() {
  ctrl?.abort()
}

watch(
  () => visible.value,
  (v) => {
    if (v) void start()
    else cancel()
  },
)

const metaText = computed(() => {
  const m = meta.value
  if (!m) return ''
  let s = t('ai.dialog.meta', { model: m.model || m.provider, input: m.usage.input_tokens, output: m.usage.output_tokens, seconds: (m.duration_ms / 1000).toFixed(1) })
  const c = formatCost(m.cost, String(locale.value))
  if (c) s += ' · ' + t('ai.dialog.cost', { cost: c })
  return s
})

// ---------------------------------------------------------------- improve

const titleDiff = computed(() => (improve.value ? diffWords(improve.value.original.title, improved.value.title) : []))
const descDiff = computed(() => (improve.value ? diffWords(improve.value.original.description, improved.value.description) : []))
const findings = computed(() => (improve.value ? splitFixed(improve.value.before.issues ?? [], improve.value.fixed) : { fixed: [], remaining: [] }))
const newFindings = computed(() => {
  const r = improve.value
  if (!r) return []
  const left = new Map<string, number>()
  for (const x of r.new) left.set(x, (left.get(x) ?? 0) + 1)
  return (r.after.issues ?? []).filter((i) => {
    const n = left.get(i.rule) ?? 0
    if (n <= 0) return false
    left.set(i.rule, n - 1)
    return true
  })
})
const issueText = (issue: Parameters<typeof issueMessage>[0], text: string) => issueMessage(issue, text, (k, p) => t(k, p))

// ---------------------------------------------------------------- accept

const methodOptions = computed(() => ['test', 'inspection', 'analysis', 'demonstration', 'review'].map((v) => ({ value: v, label: enumLabel('evaluationMethod', v) })))
const severityOptions = computed(() => DEFECT_SEVERITIES.map((v) => ({ value: v, label: t(`defectsView.severity.${v}`) })))
const picked = computed(() => {
  switch (props.feature) {
    case 'draft_tests':
      return props.mode === 'emit' ? (tests.value.length ? 1 : 0) : tests.value.filter((x) => x.pick && x.title.trim()).length
    case 'suggest_criteria':
      return criteria.value.filter((x) => x.pick && x.description.trim()).length
    case 'suggest_mappings':
      return mappings.value.filter((x) => x.pick).length
    case 'summarize_run':
      return defects.value.filter((x) => x.pick && x.title.trim()).length
    case 'improve_requirement':
      return improved.value.description.trim() ? 1 : 0
  }
  return 0
})
const acceptLabel = computed(() => {
  const n = picked.value
  switch (props.feature) {
    case 'draft_tests':
      return props.mode === 'emit' ? t('ai.tests.useThis') : t('ai.tests.createN', { n }, n)
    case 'suggest_criteria':
      return t('ai.criteria.add', { n }, n)
    case 'suggest_mappings':
      return t('ai.mappings.add', { n }, n)
    case 'summarize_run':
      return t('ai.run.createN', { n }, n)
  }
  return t('ai.improve.apply')
})

function addStep(x: DraftTest) {
  x.steps.push({ action: '', data: '', expected: '' })
}
function removeStep(x: DraftTest, i: number) {
  x.steps.splice(i, 1)
}

async function accept() {
  saveError.value = ''
  const f = props.feature
  if (f === 'improve_requirement') {
    emit('apply', { feature: f, title: improved.value.title.trim(), description: improved.value.description.trim() })
    visible.value = false
    return
  }
  if (f === 'suggest_criteria') {
    emit('apply', { feature: f, criteria: criteria.value.filter((x) => x.pick && x.description.trim()).map(({ pick: _p, ...c }) => c) })
    visible.value = false
    return
  }
  if (f === 'suggest_mappings') {
    emit('apply', { feature: f, mappings: mappings.value.filter((x) => x.pick).map(({ pick: _p, ...m }) => m) })
    visible.value = false
    return
  }
  if (f === 'draft_tests' && props.mode === 'emit') {
    const x = tests.value[chosenTest.value]
    if (x) emit('apply', { feature: f, test: x })
    visible.value = false
    return
  }
  saving.value = true
  let n = 0
  try {
    if (f === 'draft_tests') {
      for (const x of tests.value.filter((y) => y.pick && y.title.trim())) {
        const tc = await api.post<{ id: string }>(`/projects/${props.projectId}/tests`, draftTestBody(x, t('ai.tests.testData')))
        await api.post(`/projects/${props.projectId}/links`, {
          requirement_id: requirement.value.id,
          test_case_id: tc.id,
          criteria: x.criterion_ids.map((id) => ({ criterion_id: id })),
        })
        n++
        x.pick = false
      }
    } else if (f === 'summarize_run') {
      for (const d of defects.value.filter((y) => y.pick && y.title.trim())) {
        const links = d.tests.map((r) => ({ entity_type: 'execution', entity_id: r.execution_id }))
        if (run.value) links.push({ entity_type: 'run', entity_id: run.value.run_id })
        await api.post(`/projects/${props.projectId}/defects`, { title: d.title.trim(), description: d.description.trim(), severity: d.severity, links })
        n++
        d.pick = false
      }
    }
    emit('created', n)
    visible.value = false
  } catch (e) {
    saveError.value = t('ai.dialog.saveFailed', { n, error: errorMessage(e) })
    if (n) emit('created', n)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    :header="header"
    modal
    maximizable
    :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }"
    style="width: min(860px, 96vw)"
    :pt="{ content: { 'aria-busy': phase === 'running' ? 'true' : 'false' } }"
  >
    <div class="assist">
      <div class="sr-only" role="status" aria-live="polite">{{ phase === 'running' ? t('ai.dialog.working') : phase === 'done' ? t('ai.dialog.ready') : error }}</div>
      <div v-if="phase === 'running'" class="row assist-running">
        <ProgressSpinner style="width: 22px; height: 22px" stroke-width="6" :aria-label="t('ai.dialog.working')" />
        <span>{{ t('ai.dialog.working') }}</span>
        <span v-if="chars" class="small muted">{{ t('ai.dialog.written', { n: chars }) }}</span>
        <span class="grow" />
        <Button :label="t('ai.dialog.cancel')" icon="pi pi-stop" severity="secondary" outlined size="small" @click="cancel" />
      </div>
      <Message v-else-if="phase === 'error'" severity="error" :closable="false">{{ error }}</Message>
      <Message v-else-if="phase === 'cancelled'" severity="secondary" :closable="false">{{ t('ai.dialog.cancelled') }}</Message>

      <template v-if="phase === 'done'">
        <p class="small muted assist-note"><i class="pi pi-info-circle" aria-hidden="true" /> {{ t('ai.dialog.reviewNote') }}</p>

        <!-- 1. drafted tests -->
        <template v-if="feature === 'draft_tests'">
          <p v-if="!tests.length">{{ t('ai.tests.none') }}</p>
          <div v-for="(x, i) in tests" :key="i" class="assist-card" role="group" :aria-labelledby="`${uid}-t${i}`">
            <div class="row nowrap">
              <RadioButton v-if="mode === 'emit'" v-model="chosenTest" :value="i" :input-id="`${uid}-pick${i}`" :name="`${uid}-pick`" />
              <Checkbox v-else v-model="x.pick" binary :input-id="`${uid}-pick${i}`" />
              <label :id="`${uid}-t${i}`" :for="`${uid}-pick${i}`" class="sr-only">{{ t('ai.tests.select', { n: i + 1, title: x.title }) }}</label>
              <InputText v-model="x.title" class="grow" :aria-label="t('ai.tests.titleOf', { n: i + 1 })" />
              <Button
                :icon="x.open ? 'pi pi-chevron-up' : 'pi pi-chevron-down'"
                text
                rounded
                severity="secondary"
                :aria-expanded="x.open"
                :aria-controls="`${uid}-d${i}`"
                :aria-label="t('ai.tests.details', { n: i + 1 })"
                @click="x.open = !x.open"
              />
            </div>
            <p class="small muted assist-sub">
              {{ t('ai.tests.stepCount', { n: x.steps.length }, x.steps.length) }}<template v-if="x.criteria.length"> · {{ t('ai.tests.covers', { list: x.criteria.map((c) => '#' + c).join(', ') }) }}</template>
            </p>
            <div v-show="x.open" :id="`${uid}-d${i}`" class="grid assist-details">
              <div class="field">
                <label :for="`${uid}-desc${i}`">{{ t('common.description') }}</label>
                <Textarea :id="`${uid}-desc${i}`" v-model="x.description" rows="2" auto-resize />
              </div>
              <div class="field">
                <label :for="`${uid}-pre${i}`">{{ t('ai.tests.preconditions') }}</label>
                <Textarea :id="`${uid}-pre${i}`" v-model="x.preconditions" rows="2" auto-resize />
              </div>
              <div class="field">
                <label :for="`${uid}-data${i}`">{{ t('ai.tests.testData') }}</label>
                <Textarea :id="`${uid}-data${i}`" v-model="x.test_data" rows="1" auto-resize />
              </div>
              <div class="field">
                <label :for="`${uid}-exp${i}`">{{ t('ai.tests.expectedResult') }}</label>
                <Textarea :id="`${uid}-exp${i}`" v-model="x.expected_result" rows="1" auto-resize />
              </div>
              <table class="assist-steps">
                <caption class="sr-only">{{ t('ai.tests.stepsOf', { n: i + 1 }) }}</caption>
                <thead>
                  <tr>
                    <th scope="col">#</th>
                    <th scope="col">{{ t('ai.tests.action') }}</th>
                    <th scope="col">{{ t('ai.tests.data') }}</th>
                    <th scope="col">{{ t('ai.tests.expected') }}</th>
                    <th scope="col"><span class="sr-only">{{ t('common.actions') }}</span></th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(s, j) in x.steps" :key="j">
                    <td>{{ j + 1 }}</td>
                    <td><Textarea v-model="s.action" rows="1" auto-resize :aria-label="t('ai.tests.actionOf', { n: j + 1 })" /></td>
                    <td><Textarea v-model="s.data" rows="1" auto-resize :aria-label="t('ai.tests.dataOf', { n: j + 1 })" /></td>
                    <td><Textarea v-model="s.expected" rows="1" auto-resize :aria-label="t('ai.tests.expectedOf', { n: j + 1 })" /></td>
                    <td><Button icon="pi pi-trash" text rounded severity="secondary" size="small" :aria-label="t('ai.tests.removeStep', { n: j + 1 })" @click="removeStep(x, j)" /></td>
                  </tr>
                </tbody>
              </table>
              <div><Button :label="t('ai.tests.addStep')" icon="pi pi-plus" text size="small" @click="addStep(x)" /></div>
            </div>
          </div>
          <p v-if="mode !== 'emit' && tests.length" class="small muted">{{ t('ai.tests.createHint', { key: requirement.key }) }}</p>
        </template>

        <!-- 2. improved requirement -->
        <template v-if="feature === 'improve_requirement' && improve">
          <section class="assist-card" :aria-labelledby="`${uid}-diff`">
            <h3 :id="`${uid}-diff`" class="plain-heading small-heading">{{ t('ai.improve.diff') }}</h3>
            <p v-if="improve.original.title !== improved.title" class="assist-diff">
              <b>{{ t('ai.improve.titleLabel') }}:</b>
              <template v-for="(s, k) in titleDiff" :key="'t' + k"><ins v-if="s.op === 'insert'">{{ s.text }}</ins><del v-else-if="s.op === 'delete'">{{ s.text }}</del><span v-else>{{ s.text }}</span></template>
            </p>
            <p class="assist-diff">
              <template v-for="(s, k) in descDiff" :key="'d' + k"><ins v-if="s.op === 'insert'">{{ s.text }}</ins><del v-else-if="s.op === 'delete'">{{ s.text }}</del><span v-else>{{ s.text }}</span></template>
            </p>
            <p class="small"><b>{{ t('ai.improve.explanation') }}:</b> {{ improve.explanation }}</p>
            <p class="small muted">{{ t('ai.improve.score', { before: improve.before.score, after: improve.after.score }) }}</p>
          </section>
          <div class="assist-findings">
            <section :aria-labelledby="`${uid}-fixed`">
              <h3 :id="`${uid}-fixed`" class="plain-heading small-heading"><i class="pi pi-check-circle ok" aria-hidden="true" /> {{ t('ai.improve.fixed', { n: findings.fixed.length }) }}</h3>
              <ul v-if="findings.fixed.length" class="small">
                <li v-for="(f, k) in findings.fixed" :key="k">{{ issueText(f, improve.original.description) }}</li>
              </ul>
              <p v-else class="small muted">{{ t('ai.improve.none') }}</p>
            </section>
            <section :aria-labelledby="`${uid}-left`">
              <h3 :id="`${uid}-left`" class="plain-heading small-heading">{{ t('ai.improve.remaining', { n: findings.remaining.length }) }}</h3>
              <ul v-if="findings.remaining.length" class="small">
                <li v-for="(f, k) in findings.remaining" :key="k">{{ issueText(f, improve.original.description) }}</li>
              </ul>
              <p v-else class="small muted">{{ t('ai.improve.none') }}</p>
            </section>
            <section v-if="newFindings.length" :aria-labelledby="`${uid}-new`">
              <h3 :id="`${uid}-new`" class="plain-heading small-heading"><i class="pi pi-exclamation-triangle warn" aria-hidden="true" /> {{ t('ai.improve.new', { n: newFindings.length }) }}</h3>
              <ul class="small">
                <li v-for="(f, k) in newFindings" :key="k">{{ issueText(f, improved.description) }}</li>
              </ul>
            </section>
          </div>
          <div class="field">
            <label :for="`${uid}-it`">{{ t('ai.improve.titleLabel') }}</label>
            <InputText :id="`${uid}-it`" v-model="improved.title" />
          </div>
          <div class="field">
            <label :for="`${uid}-id`">{{ t('ai.improve.descriptionLabel') }}</label>
            <Textarea :id="`${uid}-id`" v-model="improved.description" rows="4" auto-resize />
          </div>
          <p class="small muted">{{ t('ai.improve.editHint') }}</p>
        </template>

        <!-- 5. acceptance criteria -->
        <template v-if="feature === 'suggest_criteria'">
          <p v-if="!criteria.length">{{ t('ai.criteria.none') }}</p>
          <div v-for="(c, i) in criteria" :key="i" class="assist-card assist-criterion" role="group" :aria-label="t('ai.criteria.item', { n: i + 1 })">
            <Checkbox v-model="c.pick" binary :input-id="`${uid}-c${i}`" />
            <label :for="`${uid}-c${i}`" class="sr-only">{{ t('ai.criteria.use', { n: i + 1 }) }}</label>
            <Textarea v-model="c.description" rows="1" auto-resize class="grow" :aria-label="t('ai.criteria.descriptionOf', { n: i + 1 })" />
            <Select v-model="c.method" :options="methodOptions" option-label="label" option-value="value" :aria-label="t('ai.criteria.methodOf', { n: i + 1 })" />
            <InputText v-model="c.threshold" :placeholder="t('ai.criteria.threshold')" :aria-label="t('ai.criteria.thresholdOf', { n: i + 1 })" />
          </div>
        </template>

        <!-- 4. framework mappings -->
        <template v-if="feature === 'suggest_mappings'">
          <p v-if="!mappings.length">{{ t('ai.mappings.none') }}</p>
          <ul v-else class="plain-list grid assist-mappings">
            <li v-for="(m, i) in mappings" :key="m.framework + m.id" class="assist-card row nowrap">
              <Checkbox v-model="m.pick" binary :input-id="`${uid}-m${i}`" />
              <label :for="`${uid}-m${i}`" class="grow">
                <span class="assist-fw">{{ t(`ai.mappings.framework.${m.framework}`) }}</span>
                <b class="mono">{{ m.id }}</b> <span v-if="m.name">{{ m.name }}</span>
                <span class="small muted assist-reason-text">{{ m.reason }}</span>
              </label>
            </li>
          </ul>
          <p v-if="dropped.length" class="small muted">{{ t('ai.mappings.dropped', { n: dropped.length, ids: dropped.join(', ') }, dropped.length) }}</p>
          <p v-if="mappingKind === 'requirement' && mappings.some((m) => m.framework === 'cwe')" class="small muted">{{ t('ai.mappings.cweNote') }}</p>
        </template>

        <!-- 3. run summary -->
        <template v-if="feature === 'summarize_run' && run">
          <section class="assist-card" :aria-labelledby="`${uid}-sum`">
            <h3 :id="`${uid}-sum`" class="plain-heading small-heading">{{ t('ai.run.summary') }}</h3>
            <p class="assist-text">{{ run.summary }}</p>
          </section>
          <section v-if="run.causes.length" :aria-labelledby="`${uid}-causes`">
            <h3 :id="`${uid}-causes`" class="plain-heading small-heading">{{ t('ai.run.causes') }}</h3>
            <ul>
              <li v-for="(c, i) in run.causes" :key="i">
                <b>{{ c.title }}</b>
                <span v-if="c.explanation"> — {{ c.explanation }}</span>
                <span v-if="c.tests.length" class="small muted"> ({{ c.tests.join(', ') }})</span>
              </li>
            </ul>
            <p class="small muted">{{ t('ai.run.hypothesis') }}</p>
          </section>
          <section :aria-labelledby="`${uid}-defects`">
            <h3 :id="`${uid}-defects`" class="plain-heading small-heading">{{ t('ai.run.defects') }}</h3>
            <p v-if="!defects.length" class="small">{{ t('ai.run.noDefects') }}</p>
            <div v-for="(d, i) in defects" :key="i" class="assist-card grid" role="group" :aria-label="t('ai.run.draftOf', { n: i + 1 })">
              <div class="row nowrap">
                <Checkbox v-model="d.pick" binary :input-id="`${uid}-dd${i}`" />
                <label :for="`${uid}-dd${i}`" class="sr-only">{{ t('ai.run.use', { n: i + 1 }) }}</label>
                <InputText v-model="d.title" class="grow" :aria-label="t('ai.run.titleOf', { n: i + 1 })" />
                <Select v-model="d.severity" :options="severityOptions" option-label="label" option-value="value" :aria-label="t('ai.run.severityOf', { n: i + 1 })" />
              </div>
              <Textarea v-model="d.description" rows="3" auto-resize :aria-label="t('ai.run.descriptionOf', { n: i + 1 })" />
              <p v-if="d.tests.length" class="small muted assist-sub">{{ t('ai.run.links', { list: d.tests.map((x) => x.key).join(', ') }) }}</p>
            </div>
          </section>
        </template>

        <p v-if="meta" class="small muted assist-meta">
          {{ metaText }}<template v-if="meta.retried"> · {{ t('ai.dialog.retried') }}</template>
        </p>
        <Message v-if="saveError" severity="error" :closable="false">{{ saveError }}</Message>
      </template>
    </div>
    <template #footer>
      <Button v-if="phase === 'error' || phase === 'cancelled' || phase === 'done'" :label="t('ai.dialog.again')" icon="pi pi-refresh" text severity="secondary" :disabled="saving" @click="start" />
      <Button :label="t('common.close')" text severity="secondary" :disabled="saving" @click="visible = false" />
      <Button v-if="phase === 'done'" :label="acceptLabel" icon="pi pi-check" :loading="saving" :disabled="!picked" @click="accept" />
    </template>
  </Dialog>
</template>

<style scoped>
.assist {
  display: grid;
  gap: 12px;
}
.assist-running {
  gap: 10px;
}
.assist-note,
.assist-meta,
.assist-sub {
  margin: 0;
}
.assist-card {
  border: 1px solid var(--p-content-border-color);
  border-radius: 8px;
  padding: 10px;
  display: grid;
  gap: 6px;
}
.assist-details {
  gap: 8px;
}
.assist-steps {
  width: 100%;
  border-collapse: collapse;
}
.assist-steps th {
  text-align: left;
  font-weight: 600;
  font-size: 13px;
  padding: 2px 4px;
}
.assist-steps td {
  padding: 2px 4px;
  vertical-align: top;
}
.assist-steps td :deep(textarea) {
  width: 100%;
}
.assist-diff {
  white-space: pre-wrap;
  margin: 0;
  line-height: 1.6;
}
.assist-diff ins {
  background: color-mix(in srgb, var(--p-green-500) 22%, transparent);
  text-decoration: underline;
}
.assist-diff del {
  background: color-mix(in srgb, var(--p-red-500) 18%, transparent);
}
.assist-findings {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
}
.assist-findings ul {
  margin: 4px 0 0;
  padding-left: 18px;
}
.assist-criterion {
  grid-template-columns: auto 1fr minmax(130px, 170px) minmax(110px, 160px);
  align-items: start;
}
@media (max-width: 640px) {
  .assist-criterion {
    grid-template-columns: auto 1fr;
  }
}
.assist-fw {
  display: inline-block;
  min-width: 64px;
  font-size: 12px;
  font-weight: 600;
}
.assist-reason-text {
  display: block;
}
.assist-text {
  white-space: pre-wrap;
  margin: 0;
}
.small-heading {
  font-size: 14px;
  margin: 0 0 4px;
}
.ok {
  color: var(--p-green-600);
}
.warn {
  color: var(--p-orange-600);
}
</style>
