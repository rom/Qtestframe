<script setup lang="ts">
// Test flow (lib/testFlow.ts): the steps of one test as a flowchart, with
// shared steps that can be expanded in place, or the run order of a test
// run in stages derived from the test dependencies. Both have a text view
// that says the same as the picture.
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import HelpLink from '@/guide/HelpLink.vue'
import StatusTag from '@/components/StatusTag.vue'
import FlowList from '@/components/FlowList'
import { api, asItems, errorMessage } from '@/lib/api'
import { buildFlow, callsToLoad, layoutFlow, runOrder, wrapText, type FlowNode, type FlowTest } from '@/lib/testFlow'
import type { Execution, TestCase, TestDependency, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const proj = useProjectStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()

const mode = ref<'test' | 'run'>(route.query.run ? 'run' : 'test')
const view = ref<'diagram' | 'list'>(route.query.view === 'list' ? 'list' : 'diagram')
const testId = ref<string | null>(typeof route.query.test === 'string' ? route.query.test : null)
const runId = ref<string | null>(typeof route.query.run === 'string' ? route.query.run : null)
const zoom = ref(1)

const modeOptions = computed(() => [
  { label: t('flow.modeTest'), value: 'test', icon: 'pi pi-list' },
  { label: t('flow.modeRun'), value: 'run', icon: 'pi pi-play-circle' },
])
const viewOptions = computed(() => [
  { label: t('flow.diagram'), value: 'diagram', icon: 'pi pi-share-alt' },
  { label: t('flow.list'), value: 'list', icon: 'pi pi-align-left' },
])

watch([mode, view, testId, runId], () => {
  router.replace({
    query: {
      ...(mode.value === 'test' && testId.value ? { test: testId.value } : {}),
      ...(mode.value === 'run' && runId.value ? { run: runId.value } : {}),
      ...(view.value === 'list' ? { view: 'list' } : {}),
    },
  })
})

// ---------------------------------------------------------------- pickers
const tests = ref<TestCase[]>([])
const runs = ref<TestRun[]>([])
const listsError = ref('')
async function loadLists() {
  listsError.value = ''
  try {
    const [ts, rs] = await Promise.all([
      api.get<{ items: TestCase[] }>(`/projects/${proj.pid}/tests`).then(asItems),
      api.get<{ items: TestRun[] }>(`/projects/${proj.pid}/runs`).then(asItems).catch(() => [] as TestRun[]),
    ])
    tests.value = ts.sort((a, b) => a.key.localeCompare(b.key, undefined, { numeric: true }))
    runs.value = rs
  } catch (e) {
    listsError.value = errorMessage(e)
  }
}
watch(() => proj.pid, loadLists, { immediate: true })
const testOptions = computed(() => tests.value.map((x) => ({ label: `${x.key} ${x.title}`, value: x.id })))
const runOptions = computed(() => runs.value.map((r) => ({ label: r.name + (r.build ? ` (${r.build})` : ''), value: r.id })))

// ---------------------------------------------------------------- one test
const test = ref<TestCase | null>(null)
const prereqs = ref<TestDependency[]>([])
const calls = ref(new Map<string, FlowTest>())
const expanded = ref(new Set<string>())
const loading = ref(false)
const error = ref('')

async function loadTest() {
  test.value = null
  prereqs.value = []
  expanded.value = new Set()
  error.value = ''
  if (!testId.value) return
  loading.value = true
  try {
    const [tc, deps] = await Promise.all([
      api.get<TestCase>(`/tests/${testId.value}`),
      api.get<{ items: TestDependency[] }>(`/tests/${testId.value}/dependencies`).then(asItems).catch(() => [] as TestDependency[]),
    ])
    test.value = tc
    prereqs.value = deps.filter((d) => d.test_id === tc.id)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(testId, loadTest, { immediate: true })

const labels = computed(() => ({
  start: t('flow.start'),
  passed: t('flow.passed'),
  failed: t('flow.failed'),
  blocked: t('flow.blocked'),
  prereq: t('flow.prereq'),
  expected: t('flow.expected'),
  overall: t('flow.overall'),
  script: t('flow.script'),
  exit: t('flow.exit'),
  noSteps: t('flow.noSteps'),
  precondition: t('flow.precondition'),
}))
const flow = computed(() =>
  test.value
    ? buildFlow(test.value, {
        calls: calls.value,
        expanded: expanded.value,
        prerequisites: prereqs.value.map((d) => ({ key: d.depends_on_key, title: d.depends_on_title })),
        labels: labels.value,
      })
    : null,
)
const layout = computed(() => (flow.value ? layoutFlow(flow.value) : null))
const callNodes = computed(() => flow.value?.nodes.filter((n) => n.kind === 'call') ?? [])

// load called tests when their call is expanded
watch(flow, async (f) => {
  if (!f) return
  for (const id of callsToLoad(f, calls.value)) {
    try {
      const c = await api.get<TestCase>(`/tests/${id}`)
      calls.value = new Map(calls.value).set(id, c)
    } catch (e) {
      const key = f.nodes.find((n) => n.callId === id)?.text ?? id
      notify.fail(e, t('flow.loadFailed', { key }))
      const next = new Set(expanded.value)
      for (const n of f.nodes) if (n.callId === id && n.callKey) next.delete(n.callKey)
      expanded.value = next
    }
  }
})
function toggleCall(n: FlowNode) {
  if (!n.callKey || n.note === 'cycle' || n.note === 'depth') return
  const next = new Set(expanded.value)
  if (next.has(n.callKey)) {
    for (const k of [...next]) if (k === n.callKey || k.startsWith(n.callKey + '-')) next.delete(k)
  } else next.add(n.callKey)
  expanded.value = next
}
async function expandAll() {
  // a few rounds: nested calls appear once their parent is loaded
  for (let i = 0; i < 5; i++) {
    const next = new Set(expanded.value)
    for (const n of callNodes.value) if (n.callKey && !n.note) next.add(n.callKey)
    if (next.size === expanded.value.size) break
    expanded.value = next
    await nextTick()
    await new Promise((r) => setTimeout(r, 50))
  }
}
const stats = computed(() => ({
  steps: flow.value?.nodes.filter((n) => n.kind === 'step' || n.kind === 'call').length ?? 0,
  checks: flow.value?.nodes.filter((n) => n.kind === 'check').length ?? 0,
}))

/** the list view: the main path with "otherwise" branches and nested shared steps */
interface ListItem {
  node: FlowNode
  fail?: FlowNode
  children: ListItem[]
}
const listItems = computed<ListItem[]>(() => {
  const f = flow.value
  if (!f) return []
  const byId = new Map(f.nodes.map((n) => [n.id, n]))
  const root: ListItem[] = []
  const stack: { group: string | undefined; items: ListItem[] }[] = [{ group: undefined, items: root }]
  let lastCall: ListItem | null = null
  for (const n of f.nodes) {
    if (n.kind === 'fail' || n.kind === 'blocked') continue
    while (stack.length > 1 && stack[stack.length - 1].group !== n.group) stack.pop()
    if (n.group && stack[stack.length - 1].group !== n.group && lastCall) stack.push({ group: n.group, items: lastCall.children })
    const item: ListItem = { node: n, fail: n.failId ? byId.get(n.failId) : undefined, children: [] }
    stack[stack.length - 1].items.push(item)
    if (n.kind === 'call') lastCall = item
  }
  return root
})

// ---------------------------------------------------------------- drawing helpers
const DIAMOND_W = 220
function diamond(n: { x: number; y: number; w: number; h: number }) {
  const cx = n.x + n.w / 2
  const cy = n.y + n.h / 2
  const hw = DIAMOND_W / 2
  return `${cx},${n.y} ${cx + hw},${cy} ${cx},${n.y + n.h} ${cx - hw},${cy}`
}
function hexagon(n: { x: number; y: number; w: number; h: number }) {
  const d = 16
  return `${n.x + d},${n.y} ${n.x + n.w - d},${n.y} ${n.x + n.w},${n.y + n.h / 2} ${n.x + n.w - d},${n.y + n.h} ${n.x + d},${n.y + n.h} ${n.x},${n.y + n.h / 2}`
}
const lines = (s: string, width: number, n: number) => wrapText(s, width, n)
/** room for the step number (monospace, about 7.5 px per character) */
const numWidth = (n: FlowNode) => (n.number ? Math.round((n.number.length + 1) * 7.5) + 6 : 0)
function nodeTitle(n: FlowNode): string {
  const parts = [n.number ? `${n.number}.` : '', n.text, n.detail ?? '']
  if (n.kind === 'call' && n.note) parts.push(`(${t('flow.' + n.note)})`)
  return parts.filter(Boolean).join(' ')
}
const svgWidth = computed(() => (layout.value ? layout.value.width + 40 : 0))
const svgHeight = computed(() => (layout.value ? layout.value.height + 40 : 0))

// ---------------------------------------------------------------- run order
const execs = ref<Execution[]>([])
const runDeps = ref<TestDependency[]>([])
const runLoading = ref(false)
const runError = ref('')
async function loadRun() {
  execs.value = []
  runDeps.value = []
  runError.value = ''
  if (!runId.value) return
  runLoading.value = true
  try {
    const [ex, deps] = await Promise.all([
      api.get<{ items: Execution[] }>(`/runs/${runId.value}/executions`).then(asItems),
      api.get<{ items: TestDependency[] }>(`/projects/${proj.pid}/test-dependencies`).then(asItems).catch(() => [] as TestDependency[]),
    ])
    execs.value = ex
    runDeps.value = deps
  } catch (e) {
    runError.value = errorMessage(e)
  } finally {
    runLoading.value = false
  }
}
watch(runId, loadRun, { immediate: true })
const execByTest = computed(() => new Map(execs.value.map((e) => [e.test_case_id, e])))
const order = computed(() =>
  runOrder(
    execs.value.map((e) => ({ id: e.test_case_id, key: e.test_key, title: e.test_title, status: e.status, position: e.position })),
    runDeps.value,
  ),
)
const keyOf = (id: string) => execByTest.value.get(id)?.test_key ?? id
const runName = computed(() => runs.value.find((r) => r.id === runId.value)?.name ?? '')
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('flow.title') }}</h1>
      <div class="sub">{{ t('flow.intro') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="tests" :anchor="{ en: 'test-flow', sv: 'testflöde' }" />
      <SelectButton v-model="mode" :options="modeOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('flow.mode')">
        <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /> {{ option.label }}</template>
      </SelectButton>
      <SelectButton v-model="view" :options="viewOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('flow.view')">
        <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /> {{ option.label }}</template>
      </SelectButton>
    </div>
  </div>

  <Message v-if="listsError" severity="error" :closable="false">{{ listsError }}</Message>

  <!-- ============================== steps of a test ============================== -->
  <div v-if="mode === 'test'" class="card">
    <div class="filterbar flow-bar">
      <label for="flow-test" class="small muted">{{ t('flow.pickTest') }}</label>
      <Select
        v-model="testId"
        input-id="flow-test"
        :options="testOptions"
        option-label="label"
        option-value="value"
        filter
        :placeholder="t('flow.pickTestPlaceholder')"
        :empty-message="t('flow.noTests')"
        class="flow-select"
      />
      <RouterLink v-if="test" :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: test.id } }" class="small">{{ t('flow.openTest') }}</RouterLink>
      <span class="spacer" />
      <template v-if="callNodes.length">
        <Button :label="t('flow.expandAll')" icon="pi pi-plus-circle" text size="small" @click="expandAll" />
        <Button :label="t('flow.collapseAll')" icon="pi pi-minus-circle" text size="small" :disabled="!expanded.size" @click="expanded = new Set()" />
      </template>
      <span v-if="view === 'diagram' && test" class="row nowrap" style="gap: 0">
        <Button icon="pi pi-search-minus" text rounded severity="secondary" :aria-label="t('graph.zoomOut')" v-tooltip="t('graph.zoomOut')" :disabled="zoom <= 0.4" @click="zoom = Math.max(0.4, +(zoom - 0.2).toFixed(1))" />
        <span class="small muted" aria-live="polite" style="min-width: 3em; text-align: center">{{ Math.round(zoom * 100) }} %</span>
        <Button icon="pi pi-search-plus" text rounded severity="secondary" :aria-label="t('graph.zoomIn')" v-tooltip="t('graph.zoomIn')" :disabled="zoom >= 2" @click="zoom = Math.min(2, +(zoom + 0.2).toFixed(1))" />
      </span>
    </div>

    <div v-if="callNodes.length" class="row flow-calls" role="group" :aria-label="t('flow.sharedSteps')">
      <span class="small muted">{{ t('flow.sharedSteps') }}:</span>
      <Button
        v-for="n in callNodes"
        :key="n.id"
        size="small"
        :outlined="!n.expanded"
        :severity="n.note ? 'secondary' : undefined"
        :icon="n.expanded ? 'pi pi-minus' : 'pi pi-plus'"
        :label="`${n.number}. ${n.text}`"
        :aria-pressed="!!n.expanded"
        :aria-label="(n.expanded ? t('flow.collapse', { key: n.text }) : t('flow.expand', { key: n.text })) + (n.note && n.note !== 'missing' ? ` (${t('flow.' + n.note)})` : '')"
        :disabled="n.note === 'cycle' || n.note === 'depth'"
        :title="n.note && n.note !== 'missing' ? t('flow.' + n.note) : n.detail"
        @click="toggleCall(n)"
      />
    </div>

    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="loading" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="!test" class="empty"><i class="pi pi-share-alt" aria-hidden="true" /><div>{{ t('flow.noTest') }}</div></div>

    <div v-else-if="view === 'diagram' && layout" class="graph-scroll flow-scroll" tabindex="0" role="region" :aria-label="t('flow.title')">
      <svg
        :width="svgWidth * zoom"
        :height="svgHeight * zoom"
        :viewBox="`-20 -20 ${svgWidth} ${svgHeight}`"
        role="img"
        :aria-label="t('flow.svgLabel', { key: test.key, steps: stats.steps, checks: stats.checks })"
        class="flow-svg"
      >
        <defs>
          <marker id="flow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" class="flow-arrow" />
          </marker>
        </defs>
        <rect v-for="f in layout.frames" :key="f.id" :x="f.x" :y="f.y" :width="f.w" :height="f.h" rx="10" class="flow-frame" />
        <text v-for="f in layout.frames" :key="f.id + 't'" :x="f.x + 10" :y="f.y + 14" class="flow-frame-label">{{ f.label }}</text>
        <g>
          <template v-for="(e, i) in layout.edges" :key="i">
            <path :d="e.path" class="flow-edge" :class="{ no: e.label === 'no' }" marker-end="url(#flow-arrow)" />
            <text v-if="e.label" :x="e.lx" :y="e.ly" class="flow-edge-label" :text-anchor="e.label === 'no' ? 'middle' : 'start'">{{ e.label === 'no' ? t('flow.no') : t('flow.yes') }}</text>
          </template>
        </g>
        <g v-for="n in layout.nodes" :key="n.id" class="flow-node" :class="['k-' + n.kind, { clickable: n.kind === 'call' && !n.note }]" @click="n.kind === 'call' && toggleCall(n)">
          <title>{{ nodeTitle(n) }}</title>
          <!-- terminals -->
          <rect v-if="['start', 'end', 'fail', 'blocked'].includes(n.kind)" :x="n.x" :y="n.y" :width="n.w" :height="n.h" :rx="n.h / 2" class="shape" />
          <polygon v-else-if="n.kind === 'check'" :points="diamond(n)" class="shape" />
          <polygon v-else-if="n.kind === 'precondition'" :points="hexagon(n)" class="shape" />
          <template v-else-if="n.kind === 'call'">
            <rect :x="n.x" :y="n.y" :width="n.w" :height="n.h" rx="4" class="shape" />
            <line :x1="n.x + 10" :y1="n.y" :x2="n.x + 10" :y2="n.y + n.h" class="bar" />
            <line :x1="n.x + n.w - 10" :y1="n.y" :x2="n.x + n.w - 10" :y2="n.y + n.h" class="bar" />
          </template>
          <rect v-else :x="n.x" :y="n.y" :width="n.w" :height="n.h" rx="6" class="shape" :stroke-dasharray="n.kind === 'empty' ? '5 4' : undefined" />

          <template v-if="n.kind === 'check'">
            <text :x="n.x + n.w / 2" :y="n.y + n.h / 2 - (n.detail ? 4 : -4)" text-anchor="middle" class="t-main">{{ lines(n.text, 26, 1)[0] }}</text>
            <text v-if="n.detail" :x="n.x + n.w / 2" :y="n.y + n.h / 2 + 12" text-anchor="middle" class="t-detail">{{ lines(n.detail, 24, 1)[0] }}</text>
          </template>
          <text v-else-if="['start', 'end', 'fail', 'blocked'].includes(n.kind)" :x="n.x + n.w / 2" :y="n.y + n.h / 2 + 4" text-anchor="middle" class="t-main">
            {{ n.kind === 'start' && n.detail ? lines(`${n.text}: ${n.detail}`, 40, 1)[0] : n.text }}
          </text>
          <template v-else>
            <text v-if="n.number" :x="n.x + (n.kind === 'call' ? 18 : 12)" :y="n.y + 22" class="t-num">{{ `${n.number}.` }}</text>
            <text :x="n.x + (n.kind === 'call' ? 18 : 12) + numWidth(n)" :y="n.y + 22" class="t-main">{{ lines(n.kind === 'call' ? `${t('flow.calls')} ${n.text}` : n.text, n.number ? 36 : 40, 1)[0] }}</text>
            <text v-if="n.detail || n.note" :x="n.x + (n.kind === 'call' ? 18 : 12)" :y="n.y + 42" class="t-detail">
              {{ lines(n.note && n.note !== 'missing' ? t('flow.' + n.note) : n.detail ?? '', 44, 1)[0] }}
            </text>
            <text v-if="n.kind === 'call' && !n.note" :x="n.x + n.w - 18" :y="n.y + 22" text-anchor="end" class="t-toggle">{{ n.expanded ? '−' : '+' }}</text>
          </template>
        </g>
      </svg>
    </div>

    <FlowList v-else-if="view === 'list'" :items="listItems" />
  </div>

  <!-- ============================== run order ============================== -->
  <div v-else class="card">
    <div class="filterbar flow-bar">
      <label for="flow-run" class="small muted">{{ t('flow.pickRun') }}</label>
      <Select v-model="runId" input-id="flow-run" :options="runOptions" option-label="label" option-value="value" filter :placeholder="t('flow.pickRunPlaceholder')" :empty-message="t('flow.noRuns')" class="flow-select" />
      <RouterLink v-if="runId" :to="{ name: 'run', params: { projectId: proj.pid, runId } }" class="small">{{ runName }}</RouterLink>
    </div>
    <Message v-if="runError" severity="error" :closable="false">{{ runError }}</Message>
    <div v-else-if="runLoading" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="!runId" class="empty"><i class="pi pi-play-circle" aria-hidden="true" /><div>{{ t('flow.noRun') }}</div></div>
    <template v-else>
      <p class="small muted" style="margin: 0 0 10px">{{ t('flow.runHint') }}</p>
      <Message v-for="x in order.external" :key="x.test + x.on" severity="warn" :closable="false">{{ t('flow.external', { test: keyOf(x.test), on: x.on }) }}</Message>
      <Message v-for="x in order.cyclic" :key="'c' + x.test + x.on" severity="warn" :closable="false">{{ t('flow.cyclic', { test: keyOf(x.test), on: keyOf(x.on) }) }}</Message>
      <ol class="stages plain-list" :class="{ list: view === 'list' }" :aria-label="t('flow.stagesLabel', { name: runName, stages: order.stages.length })">
        <li v-for="s in order.stages" :key="s.index" class="stage">
          <h2 class="plain-heading stage-title">{{ t('flow.stage', { n: s.index }) }}</h2>
          <ul class="plain-list">
            <li v-for="x in s.tests" :key="x.id" class="stage-test">
              <div class="row nowrap" style="justify-content: space-between; gap: 6px">
                <RouterLink
                  v-if="execByTest.get(x.id)"
                  :to="{ name: 'execute', params: { projectId: proj.pid, runId: runId!, executionId: execByTest.get(x.id)!.id } }"
                  class="item-ref stage-test-link"
                  :title="x.title"
                  ><span class="key">{{ x.key }}</span><span class="item-ref-title small ellipsis">{{ x.title }}</span></RouterLink
                >
                <span v-else class="stage-test-link"><span class="key">{{ x.key }}</span><span class="small ellipsis" :title="x.title">{{ x.title }}</span></span>
                <StatusTag :value="x.status" kind="result" />
              </div>
              <div class="small muted">{{ x.after.length ? t('flow.after', { keys: x.after.join(', ') }) : t('flow.noDeps') }}</div>
              <RouterLink :to="{ name: 'flow', params: { projectId: proj.pid }, query: { test: x.id } }" class="small" @click="mode = 'test'; testId = x.id">{{ t('flow.flowOfTest') }}</RouterLink>
            </li>
          </ul>
        </li>
      </ol>
    </template>
  </div>
</template>


<style>
.flow-bar {
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}
.flow-select {
  min-width: min(360px, 100%);
}
.flow-calls {
  flex-wrap: wrap;
  gap: 6px;
  margin: 0 0 10px;
}
.flow-scroll {
  max-height: calc(100vh - 280px);
}
.flow-svg .flow-edge {
  fill: none;
  stroke: var(--qtf-muted);
  stroke-width: 1.6;
}
.flow-svg .flow-edge.no {
  stroke: #dc2626;
  stroke-dasharray: 5 3;
}
.flow-svg .flow-arrow {
  fill: var(--qtf-muted);
}
.flow-svg .flow-edge-label {
  font-size: 11px;
  fill: var(--qtf-muted);
}
.flow-svg .flow-frame {
  fill: none;
  stroke: var(--p-primary-color);
  stroke-dasharray: 4 4;
  opacity: 0.7;
}
.flow-svg .flow-frame-label {
  font: 600 11px var(--qtf-mono);
  fill: var(--p-primary-color);
}
.flow-node .shape {
  fill: var(--qtf-card);
  stroke: var(--qtf-border);
  stroke-width: 1.6;
}
.flow-node .bar {
  stroke: var(--qtf-border);
  stroke-width: 1.6;
}
.flow-node.k-start .shape,
.flow-node.k-end .shape {
  stroke: #15803d;
  stroke-width: 2.2;
}
.flow-node.k-fail .shape {
  stroke: #dc2626;
  stroke-width: 2.2;
}
.flow-node.k-blocked .shape {
  stroke: #d97706;
  stroke-width: 2.2;
}
.flow-node.k-check .shape {
  stroke: var(--p-primary-color);
}
.flow-node.k-call .shape,
.flow-node.k-call .bar {
  stroke: #0e7490;
}
.flow-node.k-script .shape {
  stroke: #7c3aed;
}
.flow-node.clickable {
  cursor: pointer;
}
.flow-node.clickable:hover .shape {
  stroke-width: 3;
}
.flow-node .t-main {
  font-size: 13px;
  fill: var(--qtf-text);
}
.flow-node .t-num {
  font: 600 12px var(--qtf-mono);
  fill: var(--p-primary-color);
}
.flow-node .t-detail {
  font-size: 11.5px;
  fill: var(--qtf-muted);
}
.flow-node .t-toggle {
  font: 700 16px var(--qtf-mono);
  fill: var(--p-primary-color);
}
.flow-list {
  margin: 6px 0;
  padding-left: 4px;
  list-style: none;
  line-height: 1.5;
}
.flow-list > li {
  margin: 4px 0;
}
.flow-list.nested {
  border-left: 2px dashed var(--qtf-border);
  padding-left: 18px;
  margin-left: 2px;
}
.flow-fail-text {
  color: var(--p-red-600, #b91c1c);
}
.app-dark .flow-fail-text {
  color: #fca5a5;
}
.stages {
  display: flex;
  gap: 28px;
  overflow-x: auto;
  padding-bottom: 8px;
  align-items: flex-start;
}
.stages.list {
  flex-direction: column;
  gap: 12px;
}
.stage {
  min-width: 220px;
  max-width: 280px;
  position: relative;
}
.stages:not(.list) .stage + .stage::before {
  content: '';
  position: absolute;
  left: -22px;
  top: 4px;
  border: 8px solid transparent;
  border-left-color: var(--qtf-muted);
}
.stage-title {
  font-size: 14px;
  margin: 0 0 6px;
}
.stage-test {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 8px 10px;
  margin-bottom: 8px;
  background: var(--qtf-card);
  display: grid;
  gap: 2px;
}
</style>
