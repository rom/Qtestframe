<script setup lang="ts">
// Dependency graph of a project (GET /projects/{pid}/graph): requirement
// relations, or test dependencies and shared-step calls. Layered layout
// (lib/graphLayout.ts), left to right or rotated top to bottom, filters by
// relation type, search, zoom, a list view that shows the same relations as
// text, and print / save as SVG, PNG or Markdown (lib/graphExport.ts).
import ItemRef from '../components/ItemRef.vue'
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import HelpLink from '@/guide/HelpLink.vue'
import StatusTag from '@/components/StatusTag.vue'
import { api, errorMessage } from '@/lib/api'
import { COVERAGE_COLORS, RESULT_COLORS } from '@/lib/colors'
import { flow, layoutGraph, neighbours, SYMMETRIC, type Direction, type GraphEdgeIn } from '@/lib/graphLayout'
import { graphMarkdown, printSvg, standaloneSvg, svgToPng } from '@/lib/graphExport'
import { saveBlob } from '@/lib/download'
import { useNotify } from '@/composables/notify'
import { useProjectStore } from '@/stores/project'
import { useI18n } from 'vue-i18n'
import { enumLabel, relationText } from '@/i18n/labels'

interface GNode {
  id: string
  key: string
  title: string
  status: string
  result: string
  folder?: string
}
interface GraphData {
  kind: string
  nodes: GNode[]
  edges: GraphEdgeIn[]
}

const W = 210
const H = 54
const { t } = useI18n()
const proj = useProjectStore()
const route = useRoute()
const router = useRouter()

const kind = ref<'requirement' | 'test'>(route.query.kind === 'test' ? 'test' : 'requirement')
const data = ref<GraphData | null>(null)
const loading = ref(false)
const error = ref('')
const view = ref<'graph' | 'list'>('graph')
const search = ref('')
const zoom = ref(1)
const showAll = ref(false)
const selected = ref<string | null>(null)
// direction: left to right (default) or rotated 90° (top to bottom); kept in the URL and per browser
const DIR_KEY = 'qtf.graph.direction'
function storedDirection(): Direction | null {
  try {
    const v = localStorage.getItem(DIR_KEY)
    return v === 'vertical' || v === 'horizontal' ? v : null
  } catch {
    return null
  }
}
const direction = ref<Direction>(route.query.dir === 'vertical' ? 'vertical' : route.query.dir === 'horizontal' ? 'horizontal' : (storedDirection() ?? 'horizontal'))
watch(direction, (d) => {
  try {
    localStorage.setItem(DIR_KEY, d)
  } catch {
    /* private mode */
  }
  void router.replace({ query: { ...route.query, dir: d === 'vertical' ? 'vertical' : undefined } })
  void nextTick(fit)
})
const vertical = computed(() => direction.value === 'vertical')

/** colour and dash pattern per relation type (contrast ≥ 3:1 in light and dark themes) */
const STYLE: Record<string, { color: string; dash?: string }> = {
  depends_on: { color: '#2563eb' },
  provides: { color: '#059669' },
  refines: { color: '#7c3aed' },
  derived_from: { color: '#c2410c' },
  calls: { color: '#0e7490', dash: '6 3' },
  conflicts_with: { color: '#dc2626', dash: '2 3' },
  duplicates: { color: '#64748b', dash: '8 3 2 3' },
  relates_to: { color: '#64748b', dash: '3 4' },
}
const styleOf = (type: string) => STYLE[type] ?? { color: '#64748b', dash: '3 3' }

async function load() {
  loading.value = true
  error.value = ''
  selected.value = null
  try {
    data.value = await api.get<GraphData>(`/projects/${proj.pid}/graph`, { kind: kind.value })
    enabled.value = new Set(types.value)
    await nextTick()
    fit()
  } catch (e) {
    error.value = errorMessage(e)
    data.value = null
  } finally {
    loading.value = false
  }
}
watch(
  () => [proj.pid, kind.value] as const,
  ([pid]) => {
    if (!pid) return
    if ((route.query.kind ?? 'requirement') !== kind.value) router.replace({ query: { ...route.query, kind: kind.value } })
    void load()
  },
  { immediate: true },
)

const kindOptions = computed(() => [
  { label: t('graph.requirements'), value: 'requirement' },
  { label: t('graph.tests'), value: 'test' },
])
const viewOptions = computed(() => [
  { label: t('graph.viewGraph'), value: 'graph', icon: 'pi pi-sitemap' },
  { label: t('graph.viewList'), value: 'list', icon: 'pi pi-list' },
])

/** relation types present in the data */
const types = computed(() => [...new Set((data.value?.edges ?? []).map((e) => e.type))].sort((a, b) => order(a) - order(b)))
const TYPE_ORDER = ['depends_on', 'provides', 'refines', 'derived_from', 'calls', 'conflicts_with', 'duplicates', 'relates_to']
const order = (x: string) => (TYPE_ORDER.indexOf(x) + 1 || 99)
const enabled = ref<Set<string>>(new Set())
function toggleType(type: string, on: boolean) {
  const s = new Set(enabled.value)
  if (on) s.add(type)
  else s.delete(type)
  enabled.value = s
}
const edges = computed(() => (data.value?.edges ?? []).filter((e) => enabled.value.has(e.type)))
const byId = computed(() => new Map((data.value?.nodes ?? []).map((n) => [n.id, n])))
const connected = computed(() => {
  const s = new Set<string>()
  for (const e of edges.value) {
    s.add(e.source)
    s.add(e.target)
  }
  return s
})
const shownNodes = computed(() => (data.value?.nodes ?? []).filter((n) => showAll.value || connected.value.has(n.id)))
const hiddenCount = computed(() => (data.value?.nodes.length ?? 0) - shownNodes.value.length)
const layout = computed(() => layoutGraph(shownNodes.value.map((n) => n.id), edges.value, { nodeWidth: W, nodeHeight: H, direction: direction.value }))
const PAD = 24
const EXTRA = 120 // room for arcs between items of the same column (or row, when rotated)
const svgWidth = computed(() => layout.value.width + PAD * 2 + (vertical.value ? 0 : EXTRA))
const svgHeight = computed(() => layout.value.height + PAD * 2 + (vertical.value ? EXTRA - 30 : 0))
const scroller = ref<HTMLElement>()
/** zoom so that the whole graph fits the width (not above 100 %) */
function fit() {
  const w = scroller.value?.clientWidth ?? 0
  if (w > 0) zoom.value = Math.max(0.4, Math.min(1, Math.floor((w / svgWidth.value) * 10) / 10))
}

// search and selection highlight
const matches = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return null
  return new Set(shownNodes.value.filter((n) => `${n.key} ${n.title}`.toLowerCase().includes(q)).map((n) => n.id))
})
const focusSet = computed(() => {
  if (!selected.value) return null
  const s = neighbours(selected.value, edges.value)
  s.add(selected.value)
  return s
})
function dimmed(id: string): boolean {
  if (focusSet.value) return !focusSet.value.has(id)
  if (matches.value) return !matches.value.has(id)
  return false
}
function edgeDimmed(e: GraphEdgeIn): boolean {
  if (focusSet.value) return !(e.source === selected.value || e.target === selected.value)
  if (matches.value) return !(matches.value.has(e.source) || matches.value.has(e.target))
  return false
}

const colorOf = (n: GNode) => (kind.value === 'requirement' ? (COVERAGE_COLORS as Record<string, string>)[n.result] : RESULT_COLORS[n.result]) ?? '#94a3b8'
const resultLabel = (n: GNode) => enumLabel(kind.value === 'requirement' ? 'coverage' : 'result', n.result || (kind.value === 'requirement' ? 'none' : 'not_run'))
const short = (s: string, n = 26) => (s.length > n ? s.slice(0, n - 1) + '…' : s)
const itemRoute = (id: string) => ({ name: kind.value === 'requirement' ? 'requirements' : 'tests', params: { projectId: proj.pid }, query: { item: id } })
const href = (id: string) => router.resolve(itemRoute(id)).href

/** relations of a node in words, for the list view and screen readers */
function relationsOf(id: string): { label: string; ids: string[] }[] {
  const groups = new Map<string, string[]>()
  for (const e of edges.value) {
    let label = ''
    let other = ''
    if (e.source === id) {
      label = relationText(e.type, 'outgoing')
      other = e.target
    } else if (e.target === id) {
      label = relationText(e.type, SYMMETRIC.has(e.type) ? 'outgoing' : 'incoming')
      other = e.source
    } else continue
    groups.set(label, [...(groups.get(label) ?? []), other])
  }
  return [...groups.entries()].map(([label, ids]) => ({ label, ids }))
}
/** accessible name: exactly the visible texts, in reading order */
function nodeName(n: GNode): string {
  return `${n.key} ${short(resultLabel(n), 14)} ${short(n.title)}`
}
/** accessible description: full title, status and relations */
function nodeDescription(n: GNode): string {
  const rel = relationsOf(n.id)
    .map((g) => `${g.label} ${g.ids.map((x) => byId.value.get(x)?.key ?? '').join(', ')}`)
    .join('; ')
  return `${n.title}. ${enumLabel('status', n.status)}${rel ? '. ' + rel : ''}`
}
function open(e: MouseEvent, id: string) {
  if (e.ctrlKey || e.metaKey || e.shiftKey || e.button !== 0) return
  e.preventDefault()
  void router.push(itemRoute(id))
}
function select(id: string) {
  selected.value = selected.value === id ? null : id
}
const listNodes = computed(() => [...shownNodes.value].sort((a, b) => a.key.localeCompare(b.key, undefined, { numeric: true })))

// ---------- print and save ----------
const notify = useNotify()
const svgEl = ref<SVGSVGElement>()
const exportMenu = ref<InstanceType<typeof Menu>>()
const graphTitle = computed(() => `${proj.project?.key ?? ''} ${kind.value === 'requirement' ? t('graph.requirements') : t('graph.tests')} – ${t('graph.title')}`.trim())
const fileBase = computed(() => `${(proj.project?.key ?? 'project').toLowerCase()}-${kind.value === 'requirement' ? 'requirements' : 'tests'}-graph`)
function svgText(): string | null {
  if (!svgEl.value) return null
  const bg = scroller.value ? getComputedStyle(scroller.value).backgroundColor : '#ffffff'
  return standaloneSvg(svgEl.value, { title: graphTitle.value, background: bg || '#ffffff', baseUrl: location.origin })
}
async function exportAs(what: 'print' | 'svg' | 'png' | 'md') {
  try {
    if (what === 'md') {
      const md = graphMarkdown({
        title: graphTitle.value,
        meta: [t('graph.mdProject', { name: proj.project?.name ?? '' }), t('graph.mdGenerated', { date: new Date().toLocaleString() }), t('graph.mdCounts', { nodes: shownNodes.value.length, edges: edges.value.length })],
        direction: direction.value,
        nodes: listNodes.value.map((n) => ({ id: n.id, key: n.key, title: n.title, state: resultLabel(n) })),
        edges: edges.value.map((e) => {
          const f = flow(e)
          return { source: e.source, target: e.target, label: relationText(e.type, 'outgoing'), from: f ? f[0] : e.source, to: f ? f[1] : e.target, directed: !!f }
        }),
        headers: { key: t('common.key'), title: t('common.title'), state: t('common.status'), relations: t('workspace.colRelations') },
        relationsOf: (id) => relationsOf(id).map((g) => `${g.label} ${g.ids.map((x) => byId.value.get(x)?.key ?? '').join(', ')}`).join('; '),
      })
      saveBlob(new Blob([md], { type: 'text/markdown;charset=utf-8' }), `${fileBase.value}.md`)
      return
    }
    if (view.value !== 'graph') {
      view.value = 'graph'
      await nextTick()
    }
    const text = svgText()
    if (!text) return
    if (what === 'print') printSvg(text, graphTitle.value)
    else if (what === 'svg') saveBlob(new Blob([text], { type: 'image/svg+xml;charset=utf-8' }), `${fileBase.value}.svg`)
    else saveBlob(await svgToPng(text, 2), `${fileBase.value}.png`)
  } catch (e) {
    notify.fail(e, t('graph.exportFailed'))
  }
}
const exportItems = computed(() => [
  { label: t('graph.print'), icon: 'pi pi-print', command: () => exportAs('print') },
  { label: t('graph.saveSvg'), icon: 'pi pi-image', command: () => exportAs('svg') },
  { label: t('graph.savePng'), icon: 'pi pi-image', command: () => exportAs('png') },
  { label: t('graph.saveMd'), icon: 'pi pi-file', command: () => exportAs('md') },
])
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('graph.title') }}</h1>
      <div class="sub">{{ kind === 'requirement' ? t('graph.introRequirements') : t('graph.introTests') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="traceability-and-coverage" :anchor="{ en: 'dependency-graphs', sv: 'beroendegrafer' }" />
      <SelectButton v-model="kind" :options="kindOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('graph.kind')" />
      <SelectButton v-model="view" :options="viewOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('graph.view')">
        <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /> {{ option.label }}</template>
      </SelectButton>
    </div>
  </div>

  <div class="card">
    <div class="filterbar graph-filters">
      <span class="p-input-icon-left grow" style="min-width: 200px">
        <InputText v-model="search" :placeholder="t('graph.search')" :aria-label="t('graph.search')" class="w-full" />
      </span>
      <fieldset class="plain-fieldset row" style="gap: 10px">
        <legend class="sr-only">{{ t('graph.relationTypes') }}</legend>
        <span v-for="ty in types" :key="ty" class="row nowrap small" style="gap: 4px">
          <Checkbox :model-value="enabled.has(ty)" binary :input-id="`gt-${ty}`" @update:model-value="(v: boolean) => toggleType(ty, v)" />
          <svg width="26" height="10" aria-hidden="true"><line x1="1" y1="5" x2="25" y2="5" :stroke="styleOf(ty).color" stroke-width="2.5" :stroke-dasharray="styleOf(ty).dash" /></svg>
          <label :for="`gt-${ty}`">{{ relationText(ty, 'outgoing') }}</label>
        </span>
      </fieldset>
      <span class="row nowrap small"><ToggleSwitch v-model="showAll" input-id="g-all" /><label for="g-all">{{ t('graph.showAll') }}</label></span>
      <span v-if="view === 'graph'" class="row nowrap" style="gap: 0">
        <Button icon="pi pi-search-minus" text rounded severity="secondary" :aria-label="t('graph.zoomOut')" v-tooltip="t('graph.zoomOut')" :disabled="zoom <= 0.4" @click="zoom = Math.max(0.4, +(zoom - 0.2).toFixed(1))" />
        <span class="small muted" aria-live="polite" style="min-width: 3em; text-align: center">{{ Math.round(zoom * 100) }} %</span>
        <Button icon="pi pi-search-plus" text rounded severity="secondary" :aria-label="t('graph.zoomIn')" v-tooltip="t('graph.zoomIn')" :disabled="zoom >= 2" @click="zoom = Math.min(2, +(zoom + 0.2).toFixed(1))" />
        <Button icon="pi pi-arrows-h" text rounded severity="secondary" :aria-label="t('graph.fit')" v-tooltip="t('graph.fit')" @click="fit" />
        <Button
          icon="pi pi-replay"
          text
          rounded
          severity="secondary"
          class="rotate-button"
          :class="{ on: vertical }"
          :aria-label="t('graph.rotate')"
          :aria-pressed="vertical"
          v-tooltip="vertical ? t('graph.rotateBack') : t('graph.rotate')"
          @click="direction = vertical ? 'horizontal' : 'vertical'"
        />
      </span>
      <Button
        :label="t('graph.printSave')"
        icon="pi pi-download"
        severity="secondary"
        outlined
        size="small"
        aria-haspopup="menu"
        :disabled="!shownNodes.length"
        @click="(e: Event) => exportMenu?.toggle(e)"
      />
      <Menu ref="exportMenu" :model="exportItems" popup :aria-label="t('graph.printSave')" />
    </div>

    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="loading && !data" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="!shownNodes.length" class="empty">
      <i class="pi pi-sitemap" aria-hidden="true" />
      <div>{{ kind === 'requirement' ? t('graph.emptyRequirements') : t('graph.emptyTests') }}</div>
    </div>
    <template v-else>
      <p v-if="hiddenCount && !showAll" class="small muted" style="margin: 0 0 6px">{{ t('graph.hidden', { count: hiddenCount }, hiddenCount) }}</p>
      <p v-if="selected" class="small" style="margin: 0 0 6px">
        {{ t('graph.focused', { key: byId.get(selected)?.key }) }}
        <Button :label="t('graph.clearFocus')" text size="small" @click="selected = null" />
      </p>

      <div v-if="view === 'graph'" ref="scroller" class="graph-scroll" tabindex="0" role="region" :aria-label="t('graph.title')">
        <svg
          ref="svgEl"
          :width="svgWidth * zoom"
          :height="svgHeight * zoom"
          :viewBox="`${-PAD} ${-PAD} ${svgWidth} ${svgHeight}`"
          role="group"
          :aria-label="t('graph.svgLabel', { nodes: shownNodes.length, edges: edges.length })"
        >
          <defs>
            <marker v-for="ty in types" :id="`arrow-${ty}`" :key="ty" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" :fill="styleOf(ty).color" />
            </marker>
          </defs>
          <g aria-hidden="true">
            <path
              v-for="e in layout.edges"
              :key="e.id"
              :d="e.path"
              fill="none"
              :stroke="styleOf(e.type).color"
              :stroke-width="selected && !edgeDimmed(e) ? 3 : 2"
              :stroke-dasharray="styleOf(e.type).dash"
              :marker-end="e.flow ? `url(#arrow-${e.type})` : undefined"
              :opacity="edgeDimmed(e) ? 0.15 : 0.9"
            >
              <title>{{ byId.get(e.source)?.key }} {{ relationText(e.type, 'outgoing') }} {{ byId.get(e.target)?.key }}</title>
            </path>
          </g>
          <g v-for="n in layout.nodes" :key="n.id">
            <!-- tooltip and description stay outside the link: its name is exactly its visible text -->
            <title>{{ byId.get(n.id)?.key }} {{ byId.get(n.id)?.title }}</title>
            <desc :id="`gdesc-${n.id}`">{{ nodeDescription(byId.get(n.id)!) }}</desc>
            <a
              :href="href(n.id)"
              class="graph-node"
              :class="{ dim: dimmed(n.id), selected: selected === n.id, match: matches?.has(n.id) }"
              :aria-label="nodeName(byId.get(n.id)!)"
              :aria-describedby="`gdesc-${n.id}`"
              @click="open($event, n.id)"
              @keydown.space.prevent="select(n.id)"
              @contextmenu.prevent="select(n.id)"
            >
              <rect :x="n.x" :y="n.y" :width="W" :height="H" rx="8" class="node-box" />
              <rect :x="n.x" :y="n.y" width="6" :height="H" rx="3" :fill="colorOf(byId.get(n.id)!)" />
              <text :x="n.x + 14" :y="n.y + 20" class="node-key">{{ `${byId.get(n.id)?.key} ` }}</text>
              <text :x="n.x + W - 8" :y="n.y + 20" class="node-status" text-anchor="end">{{ `${short(resultLabel(byId.get(n.id)!), 14)} ` }}</text>
              <text :x="n.x + 14" :y="n.y + 40" class="node-title">{{ short(byId.get(n.id)?.title ?? '') }}</text>
            </a>
          </g>
        </svg>
      </div>
      <p v-if="view === 'graph'" class="small muted" style="margin: 8px 0 0">{{ vertical ? t('graph.hintVertical') : t('graph.hint') }}</p>

      <div v-else class="table-scroll">
        <table class="graph-list">
          <caption class="sr-only">{{ t('graph.title') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ kind === 'requirement' ? t('graph.requirement') : t('graph.test') }}</th>
              <th scope="col">{{ t('common.status') }}</th>
              <th scope="col">{{ t('workspace.colRelations') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="n in listNodes" :key="n.id" :class="{ dim: dimmed(n.id) }">
              <th scope="row">
                <ItemRef :to="itemRoute(n.id)" :item-key="n.key" :title="n.title" title-class="small" />
              </th>
              <td><StatusTag :value="n.result" :kind="kind === 'requirement' ? 'coverage' : 'result'" /></td>
              <td class="small">
                <div v-for="g in relationsOf(n.id)" :key="g.label">
                  <span class="muted">{{ g.label }}</span>
                  <template v-for="(id, i) in g.ids" :key="id">
                    <RouterLink :to="itemRoute(id)" class="key">{{ byId.get(id)?.key }}</RouterLink>{{ i < g.ids.length - 1 ? ', ' : '' }}
                  </template>
                </div>
                <span v-if="!relationsOf(n.id).length" class="muted">–</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>

<style>
.graph-scroll {
  overflow: auto;
  max-height: calc(100vh - 300px);
  min-height: 240px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  background: var(--qtf-bg);
}
.rotate-button .pi {
  transform: rotate(90deg);
}
.rotate-button.on {
  background: color-mix(in srgb, var(--p-primary-color) 14%, transparent);
}
.graph-node {
  cursor: pointer;
  outline: none;
}
.graph-node .node-box {
  fill: var(--qtf-card);
  stroke: var(--qtf-border);
  stroke-width: 1.5;
}
.graph-node:hover .node-box,
.graph-node.match .node-box {
  stroke: var(--p-primary-color);
  stroke-width: 2;
}
.graph-node.selected .node-box,
.graph-node:focus-visible .node-box {
  stroke: var(--p-primary-color);
  stroke-width: 3;
}
.graph-node.dim {
  opacity: 0.25;
}
.graph-node .node-key {
  font: 600 12px var(--qtf-mono);
  fill: var(--p-primary-color);
}
.graph-node .node-status {
  font-size: 11px;
  fill: var(--qtf-muted);
}
.graph-node .node-title {
  font-size: 12px;
  fill: var(--qtf-text);
}
.graph-list {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.graph-list th,
.graph-list td {
  text-align: left;
  vertical-align: top;
  padding: 6px 8px;
  border-top: 1px solid var(--qtf-border);
}
.graph-list th[scope='row'] {
  font-weight: normal;
}
.graph-list tr.dim {
  opacity: 0.4;
}
.graph-filters {
  flex-wrap: wrap;
  gap: 10px;
}
</style>
