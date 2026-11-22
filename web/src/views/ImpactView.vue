<script setup lang="ts">
// Impact analysis (POST /projects/{pid}/impact): pick requirements and
// tests, or a baseline (its changes since then), and see everything a change
// of them affects, transitively along the project's impact rules: as a
// graph (the layered layout of the dependency graph, lib/graphLayout.ts) or
// as a table with the path to each item; export as CSV or PDF
// (GET /projects/{pid}/impact/export). The choice is kept in the URL
// (?items=kind:id,…&baseline=&depth=) so an analysis can be linked.
import ItemRef from '../components/ItemRef.vue'
import ItemPickerDialog from '../components/ItemPickerDialog.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { layoutGraph } from '@/lib/graphLayout'
import { DEFAULT_IMPACT_DEPTH, depthCounts, impactGraph, impactItems, impactSections, MAX_IMPACT_DEPTH, type ChangeImpact, type CorrelationSettings } from '@/lib/correlations'
import { useImpactLabels } from '@/composables/impact'
import { useNotify } from '@/composables/notify'
import { useProjectStore } from '@/stores/project'
import { useI18n } from 'vue-i18n'

interface Chosen {
  kind: 'requirement' | 'test'
  id: string
  key: string
  title: string
}

const { t } = useI18n()
const proj = useProjectStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { edgeLabel, pathOf } = useImpactLabels()

const chosen = ref<Chosen[]>([])
const baselines = ref<{ id: string; name: string }[]>([])
const baseline = ref<string | null>(null)
const depth = ref<number>(DEFAULT_IMPACT_DEPTH)
const impact = ref<ChangeImpact | null>(null)
const loading = ref(false)
const error = ref('')
const view = ref<'graph' | 'table'>('graph')
const picker = ref<'requirement' | 'test' | null>(null)
const pickerOpen = computed({
  get: () => picker.value !== null,
  set: (v: boolean) => {
    if (!v) picker.value = null
  },
})

const viewOptions = computed(() => [
  { value: 'graph', label: t('correlations.viewGraph') },
  { value: 'table', label: t('correlations.viewTable') },
])
const baselineOptions = computed(() => [{ id: null as string | null, name: t('correlations.noBaseline') }, ...baselines.value])
const canAnalyse = computed(() => chosen.value.length > 0 || !!baseline.value)

function itemsParam() {
  return chosen.value.map((c) => `${c.kind}:${c.id}`).join(',')
}

async function analyse() {
  if (!canAnalyse.value) return
  loading.value = true
  error.value = ''
  try {
    impact.value = await api.post<ChangeImpact>(`/projects/${proj.pid}/impact`, {
      items: chosen.value.map((c) => ({ kind: c.kind, id: c.id })),
      baseline_id: baseline.value ?? '',
      depth: depth.value,
    })
    void router.replace({ query: { ...route.query, items: itemsParam() || undefined, baseline: baseline.value || undefined, depth: String(depth.value) } })
  } catch (e) {
    error.value = errorMessage(e)
    impact.value = null
  } finally {
    loading.value = false
  }
}

function add(kind: 'requirement' | 'test', list: { id: string; key: string; title: string }[]) {
  for (const it of list) if (!chosen.value.some((c) => c.id === it.id)) chosen.value.push({ kind, id: it.id, key: it.key, title: it.title })
}
function remove(c: Chosen) {
  chosen.value = chosen.value.filter((x) => x.id !== c.id)
}

async function exportAs(format: 'csv' | 'pdf') {
  try {
    await api.download(`/projects/${proj.pid}/impact/export`, { format, items: itemsParam(), baseline: baseline.value ?? '', depth: depth.value }, `impact.${format}`)
  } catch (e) {
    notify.fail(e)
  }
}

onMounted(async () => {
  try {
    const [bl, set] = await Promise.all([
      api.get<{ id: string; name: string }[] | { items: { id: string; name: string }[] }>(`/projects/${proj.pid}/baselines`),
      api.get<CorrelationSettings>(`/projects/${proj.pid}/correlation-settings`).catch(() => null),
    ])
    baselines.value = asItems(bl).map((b) => ({ id: b.id, name: b.name }))
    if (set?.impact_depth) depth.value = set.impact_depth
  } catch {
    /* the baselines are optional */
  }
  // an analysis linked by URL
  const q = route.query
  if (q.depth) depth.value = Math.min(MAX_IMPACT_DEPTH, Math.max(1, Number(q.depth) || DEFAULT_IMPACT_DEPTH))
  if (typeof q.baseline === 'string') baseline.value = q.baseline
  if (typeof q.items === 'string' && q.items) {
    const want = q.items.split(',').map((s) => s.split(':') as [string, string])
    const found = await Promise.all(
      want.map(async ([kind, id]) => {
        try {
          const it = await api.get<{ id: string; key: string; title: string }>(`/${kind === 'test' ? 'tests' : 'requirements'}/${id}`)
          return { kind: kind === 'test' ? 'test' : 'requirement', id: it.id, key: it.key, title: it.title } as Chosen
        } catch {
          return null
        }
      }),
    )
    chosen.value = found.filter((x): x is Chosen => !!x)
  }
  if (canAnalyse.value) await analyse()
})
watch(
  () => proj.pid,
  () => {
    impact.value = null
  },
)

// ---------------------------------------------------------------- results
const sections = computed(() => impactSections(impact.value))
const depths = computed(() => depthCounts(impact.value))
const rows = computed(() => impactItems(impact.value))
const rootKeys = computed(() => (impact.value?.roots ?? []).map((r) => r.key).join(', '))

function routeOf(kind: string, id: string) {
  const pid = proj.pid
  switch (kind) {
    case 'requirement':
      return { name: 'requirements', params: { projectId: pid }, query: { item: id } }
    case 'test':
    case 'result':
      return { name: 'tests', params: { projectId: pid }, query: { item: id } }
    case 'risk':
      return { name: 'risks', params: { projectId: pid } }
    case 'defect':
      return { name: 'defects', params: { projectId: pid }, query: { item: id } }
    case 'document':
      return { name: 'documents', params: { projectId: pid } }
    case 'variant':
      return { name: 'variants', params: { projectId: pid } }
    case 'code':
      return { name: 'code', params: { projectId: pid } }
  }
  return undefined
}

// graph: the changed items first, each item right of the one it is reached from
const W = 190
const H = 50
const PAD = 16
const graph = computed(() => impactGraph(impact.value))
const byId = computed(() => new Map(graph.value.nodes.map((n) => [n.id, n])))
const layout = computed(() =>
  layoutGraph(
    graph.value.nodes.map((n) => n.id),
    graph.value.edges.map((e) => ({ id: e.id, source: e.source, target: e.target, type: 'provides' })),
    { nodeWidth: W, nodeHeight: H },
  ),
)
const edgeById = computed(() => new Map(graph.value.edges.map((e) => [e.id, e])))
const svgWidth = computed(() => layout.value.width + 2 * PAD)
const svgHeight = computed(() => layout.value.height + 2 * PAD)
function href(id: string) {
  const n = byId.value.get(id)
  const r = n && routeOf(n.kind, id.slice(n.kind.length + 1))
  return r ? router.resolve(r).href : undefined
}
function short(s: string, n = 26) {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}
function kindLabel(kind: string) {
  const sec = kind === 'run' ? 'links' : kind === 'code' ? 'code' : `${kind}s`
  return t(`correlations.section.${sec}`)
}
function nodeName(id: string) {
  const n = byId.value.get(id)
  if (!n) return id
  const effect = n.root ? '' : `, ${t(`correlations.mode.${n.mode}`)}`
  return `${n.key} ${n.title}${effect}`
}
</script>

<template>
  <div class="grid impact-view" style="gap: 16px">
    <div class="row" style="align-items: baseline">
      <h1 style="margin: 0">{{ t('correlations.viewTitle') }}</h1>
      <HelpLink page="traceability-and-coverage" :anchor="{ en: 'impact-analysis', sv: 'paverkansanalys' }" />
    </div>
    <p class="muted small" style="margin: 0">{{ t('correlations.viewIntro') }}</p>

    <section class="card grid" style="gap: 12px" :aria-label="t('correlations.pickItems')">
      <div class="row" style="flex-wrap: wrap; gap: 8px">
        <Button :label="t('correlations.pickRequirements')" icon="pi pi-plus" size="small" outlined @click="picker = 'requirement'" />
        <Button :label="t('correlations.pickTests')" icon="pi pi-plus" size="small" outlined @click="picker = 'test'" />
        <Button v-if="chosen.length" :label="t('correlations.clearItems')" icon="pi pi-times" size="small" text severity="secondary" @click="chosen = []" />
      </div>
      <ul v-if="chosen.length" class="plain-list impact-chosen" :aria-label="t('correlations.chosen', { n: chosen.length }, chosen.length)">
        <li v-for="c in chosen" :key="c.id" class="impact-chip-item">
          <span class="key">{{ c.key }}</span> <span class="small">{{ short(c.title, 40) }}</span>
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="`${t('common.remove')} ${c.key}`" @click="remove(c)" />
        </li>
      </ul>
      <div class="row" style="flex-wrap: wrap; gap: 12px; align-items: flex-end">
        <div class="field" style="min-width: 220px">
          <label for="impact-baseline">{{ t('correlations.baseline') }}</label>
          <Select input-id="impact-baseline" v-model="baseline" :options="baselineOptions" option-label="name" option-value="id" />
        </div>
        <div class="field" style="width: 130px">
          <label for="impact-depth">{{ t('correlations.depthField') }}</label>
          <InputNumber v-model="depth" input-id="impact-depth" :min="1" :max="MAX_IMPACT_DEPTH" show-buttons />
        </div>
        <Button :label="t('correlations.analyse')" icon="pi pi-bullseye" :disabled="!canAnalyse" :loading="loading" @click="analyse" />
      </div>
      <p v-if="!canAnalyse" class="small muted" style="margin: 0">{{ t('correlations.noneChosen') }}</p>
    </section>

    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="loading && !impact" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>

    <section v-if="impact" class="card grid" style="gap: 12px" aria-labelledby="impact-result-h">
      <div class="row" style="flex-wrap: wrap; gap: 8px">
        <h2 id="impact-result-h" class="plain-heading" style="margin: 0">{{ t('correlations.totalAffected', { n: rows.length }, rows.length) }}</h2>
        <span class="small muted">{{ t('correlations.changedItems', { keys: rootKeys }) }}</span>
        <span class="grow" />
        <SelectButton v-model="view" :options="viewOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('correlations.viewAs')" />
        <Button :label="t('correlations.exportCsv')" icon="pi pi-download" size="small" outlined :aria-label="t('correlations.exportLabel', { format: 'CSV' })" @click="exportAs('csv')" />
        <Button :label="t('correlations.exportPdf')" icon="pi pi-file-pdf" size="small" outlined :aria-label="t('correlations.exportLabel', { format: 'PDF' })" @click="exportAs('pdf')" />
      </div>
      <p class="small" style="margin: 0">{{ t('correlations.confirmSummary', { suspect: impact.suspect ?? 0, review: impact.review ?? 0 }) }}</p>
      <div class="impact-counts small">
        <div>
          <span class="muted">{{ t('correlations.byKind') }}:</span>
          <span v-for="s in sections" :key="s.section" class="impact-chip">{{ t(`correlations.section.${s.section}`) }} <strong>{{ s.items.length }}</strong></span>
        </div>
        <div>
          <span class="muted">{{ t('correlations.byDepth') }}:</span>
          <span v-for="d in depths" :key="d.depth" class="impact-chip">{{ t('correlations.depth', { n: d.depth }) }} <strong>{{ d.count }}</strong></span>
        </div>
      </div>

      <p v-if="!rows.length" class="muted">{{ t('correlations.nothing') }}</p>
      <template v-else-if="view === 'graph'">
        <div class="impact-graph-scroll">
          <svg :width="svgWidth" :height="svgHeight" :viewBox="`${-PAD} ${-PAD} ${svgWidth} ${svgHeight}`" role="group" :aria-label="t('correlations.graphLabel', { keys: rootKeys })">
            <defs>
              <marker id="impact-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M 0 0 L 10 5 L 0 10 z" class="impact-arrow" />
              </marker>
            </defs>
            <g aria-hidden="true">
              <path
                v-for="e in layout.edges"
                :key="e.id"
                :d="e.path"
                fill="none"
                class="impact-edge"
                :stroke-dasharray="byId.get(e.target)?.mode === 'review' ? '6 4' : undefined"
                marker-end="url(#impact-arrow)"
              >
                <title>{{ byId.get(e.source)?.key }} {{ edgeLabel(edgeById.get(e.id)?.type ?? '', edgeById.get(e.id)?.direction) }} {{ byId.get(e.target)?.key }}</title>
              </path>
            </g>
            <g v-for="n in layout.nodes" :key="n.id">
              <a :href="href(n.id)" class="impact-node" :class="[`impact-node-${byId.get(n.id)?.mode}`]" :aria-label="nodeName(n.id)">
                <rect :x="n.x" :y="n.y" :width="W" :height="H" rx="8" class="node-box" :stroke-dasharray="byId.get(n.id)?.mode === 'review' ? '6 4' : undefined" />
                <text :x="n.x + 10" :y="n.y + 19" class="node-key">{{ byId.get(n.id)?.key }}</text>
                <text :x="n.x + W - 8" :y="n.y + 19" class="node-kind" text-anchor="end">{{ short(kindLabel(byId.get(n.id)?.kind ?? ''), 16) }}</text>
                <text :x="n.x + 10" :y="n.y + 38" class="node-title">{{ short(byId.get(n.id)?.title ?? '') }}</text>
              </a>
            </g>
          </svg>
        </div>
        <p class="small muted" style="margin: 0">{{ t('correlations.graphLegend') }}</p>
      </template>
      <div v-else class="table-scroll">
        <table class="impact-table">
          <caption class="sr-only">{{ t('correlations.viewTitle') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('correlations.keyCol') }}</th>
              <th scope="col">{{ t('correlations.kindCol') }}</th>
              <th scope="col">{{ t('correlations.depthLabel') }}</th>
              <th scope="col">{{ t('correlations.effectCol') }}</th>
              <th scope="col">{{ t('correlations.owner') }}</th>
              <th scope="col">{{ t('correlations.path') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in rows" :key="`${r.kind}:${r.id}`">
              <th scope="row">
                <ItemRef v-if="routeOf(r.kind, r.test_id || r.id)" :to="routeOf(r.kind, r.test_id || r.id)" :item-key="r.key" :title="r.title" title-class="small" />
                <span v-else class="mono">{{ r.key }}</span>
              </th>
              <td class="small">{{ t(`correlations.section.${r.section}`) }}</td>
              <td class="small">{{ r.depth }}</td>
              <td class="small">{{ r.already ? t('correlations.alreadyMarked') : t(`correlations.mode.${r.mode ?? 'suspect'}`) }}</td>
              <td class="small">{{ r.owner }}</td>
              <td class="small impact-path">{{ pathOf(r.path) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <ItemPickerDialog
      v-if="picker"
      v-model:visible="pickerOpen"
      :kind="picker"
      :project-id="proj.pid"
      :exclude="chosen.map((c) => c.id)"
      :header="picker === 'test' ? t('correlations.pickTests') : t('correlations.pickRequirements')"
      @pick="(list) => add(picker ?? 'requirement', list as { id: string; key: string; title: string }[])"
    />
  </div>
</template>

<style scoped>
.impact-chosen {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.impact-chip-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 0 0 8px;
  border: 1px solid var(--qtf-border);
  border-radius: 99px;
}
.impact-counts {
  display: grid;
  gap: 4px;
}
.impact-chip {
  display: inline-block;
  margin: 0 0 0 6px;
  padding: 0 6px;
  border: 1px solid var(--qtf-border);
  border-radius: 99px;
}
.impact-graph-scroll {
  overflow: auto;
  max-height: 70vh;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
}
.impact-edge {
  stroke: var(--qtf-muted);
  stroke-width: 1.8;
}
.impact-arrow {
  fill: var(--qtf-muted);
}
.impact-node .node-box {
  fill: var(--qtf-card);
  stroke: var(--p-primary-color);
  stroke-width: 1.5;
}
.impact-node-root .node-box {
  stroke-width: 3;
}
.impact-node-review .node-box {
  stroke: var(--qtf-muted);
}
.impact-node:focus-visible .node-box,
.impact-node:hover .node-box {
  stroke-width: 3;
}
.impact-node .node-key {
  font: 600 12px var(--qtf-mono);
  fill: var(--p-primary-color);
}
.impact-node .node-kind {
  font-size: 11px;
  fill: var(--qtf-muted);
}
.impact-node .node-title {
  font-size: 12px;
  fill: var(--qtf-text);
}
.impact-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.impact-table th,
.impact-table td {
  text-align: left;
  padding: 6px 8px;
  border-bottom: 1px solid var(--qtf-border);
  vertical-align: top;
}
.impact-path {
  overflow-wrap: anywhere;
}
</style>
