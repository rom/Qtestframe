<script setup lang="ts">
// The built-in catalogue of standard requirements and tests: browse by
// group and area, search and filter (client-side, see lib/catalog.ts),
// preview an item and add a selection to the project.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Drawer from 'primevue/drawer'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import ProgressSpinner from 'primevue/progressspinner'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import Tree from 'primevue/tree'
import StatusTag from '@/components/StatusTag.vue'
import CatalogPreview from '@/components/CatalogPreview.vue'
import CatalogImportDialog from '@/components/CatalogImportDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import {
  activeFilters,
  catalogTree,
  createdQuery,
  emptyFilters,
  filterCatalog,
  importPlan,
  inProjectSet,
  nodeCounts,
  nodeLabel,
  selectionState,
  tagOptions,
  toggleIds,
  type CatalogFilters,
  type CatalogImportResult,
  type CatalogItem,
  type CatalogKind,
  type CatalogOverview,
  type CatalogRequirement,
  type CatalogTest,
  type ProjectCatalog,
} from '@/lib/catalog'
import { addedItemsEntry, recordUndo } from '@/lib/undoActions'
import { useBreakpoint } from '@/lib/breakpoint'
import { P } from '@/lib/permissions'
import { enumLabel, enumOptions } from '@/i18n/labels'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useUndoStore } from '@/stores/undo'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const proj = useProjectStore()
const undoStore = useUndoStore()
const notify = useNotify()
const narrow = useBreakpoint('wide')

const PAGE = 100
const canReadReqs = computed(() => auth.can(P.requirementRead))
const canReadTests = computed(() => auth.can(P.testRead))
const canWriteReqs = computed(() => auth.can(P.requirementWrite))
const canWriteTests = computed(() => auth.can(P.testWrite))

const kind = ref<CatalogKind>(route.query.tab === 'tests' || !canReadReqs.value ? 'test' : 'requirement')
const lang = ref<string>(route.query.lang === 'sv' || route.query.lang === 'en' ? String(route.query.lang) : String(locale.value).startsWith('sv') ? 'sv' : 'en')

const overview = ref<CatalogOverview | null>(null)
const reqs = ref<CatalogRequirement[]>([])
const tests = ref<CatalogTest[]>([])
const inProject = ref<ProjectCatalog>({ requirements: {}, tests: {} })
const loading = ref(false)
const error = ref('')

async function loadCatalog() {
  loading.value = true
  error.value = ''
  try {
    const q = { lang: lang.value }
    const [ov, r, ts] = await Promise.all([
      api.get<CatalogOverview>('/catalog', q),
      canReadReqs.value ? api.get<{ items: CatalogRequirement[] }>('/catalog/requirements', q) : Promise.resolve({ items: [] }),
      canReadTests.value ? api.get<{ items: CatalogTest[] }>('/catalog/tests', q) : Promise.resolve({ items: [] }),
    ])
    overview.value = ov
    reqs.value = asItems(r)
    tests.value = asItems(ts)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
async function loadInProject() {
  try {
    inProject.value = await api.get<ProjectCatalog>(`/projects/${proj.pid}/catalog`)
  } catch {
    inProject.value = { requirements: {}, tests: {} }
  }
}
onMounted(() => Promise.all([loadCatalog(), loadInProject()]))
watch(lang, loadCatalog)
watch(() => undoStore.version, loadInProject)

const inReqs = computed(() => inProjectSet(inProject.value.requirements))
const inTests = computed(() => inProjectSet(inProject.value.tests))
const inSet = computed(() => (kind.value === 'requirement' ? inReqs.value : inTests.value))
const inMap = computed(() => (kind.value === 'requirement' ? inProject.value.requirements : inProject.value.tests))
const reqById = computed(() => new Map(reqs.value.map((r) => [r.id, r])))

// ---------- filters (per tab) ----------
const filters = ref<Record<CatalogKind, CatalogFilters>>({
  requirement: { ...emptyFilters(), node: typeof route.query.node === 'string' ? route.query.node : 'all', q: typeof route.query.q === 'string' ? route.query.q : '' },
  test: { ...emptyFilters(), node: typeof route.query.node === 'string' ? route.query.node : 'all', q: typeof route.query.q === 'string' ? route.query.q : '' },
})
const f = computed(() => filters.value[kind.value])
const items = computed<CatalogItem[]>(() => (kind.value === 'requirement' ? reqs.value : tests.value))
const shown = computed(() => filterCatalog(items.value, f.value, inSet.value))
const counts = computed(() => nodeCounts(filterCatalog(items.value, f.value, inSet.value, { node: false })))
const limit = ref(PAGE)
watch([shown, kind], () => (limit.value = PAGE))
const visibleItems = computed(() => shown.value.slice(0, limit.value))

const typeOptions = computed(() => enumOptions('requirementType', [...new Set(reqs.value.map((r) => r.type))]))
const obligationOptions = computed(() =>
  ['must', 'must_not', 'should', 'should_not', 'may'].filter((o) => reqs.value.some((r) => r.obligation === o)).map((o) => ({ label: o.replace('_', ' ').toUpperCase(), value: o })),
)
const priorityOptions = computed(() => enumOptions('priority', ['critical', 'high', 'medium', 'low']))
const executionOptions = computed(() => enumOptions('executionType', ['manual', 'automated']))
const tagOpts = computed(() => tagOptions(items.value).map((x) => ({ label: x, value: x })))
const kindOptions = computed(() => [
  ...(canReadReqs.value ? [{ label: `${t('catalog.requirements')} (${reqs.value.length})`, value: 'requirement' }] : []),
  ...(canReadTests.value ? [{ label: `${t('catalog.tests')} (${tests.value.length})`, value: 'test' }] : []),
])
const langOptions = [
  { label: 'English', value: 'en' },
  { label: 'Svenska', value: 'sv' },
]
function clearFilters() {
  filters.value[kind.value] = { ...emptyFilters(), node: f.value.node }
}

// ---------- tree ----------
const areasOpen = ref(false)
const treeNodes = computed(() => {
  const nodes = catalogTree(overview.value?.groups ?? [], kind.value, counts.value)
  const map = (n: (typeof nodes)[number]): Record<string, unknown> => ({ key: n.key, label: n.label, data: { count: n.count }, children: n.children?.map(map), leaf: !n.children?.length })
  return [{ key: 'all', label: t('catalog.all'), data: { count: counts.value.get('all') ?? 0 }, leaf: true }, ...nodes.map(map)]
})
const expanded = ref<Record<string, boolean>>({})
watch(
  () => overview.value,
  (ov) => {
    for (const g of ov?.groups ?? []) expanded.value[`g:${g.id}`] = true
  },
)
const selectionKeys = computed(() => ({ [f.value.node || 'all']: true }))
function selectNode(key: string) {
  f.value.node = key
  areasOpen.value = false
}
const nodeTitle = computed(() => nodeLabel(overview.value?.groups ?? [], f.value.node) || t('catalog.all'))

// ---------- selection ----------
const selected = ref<Record<CatalogKind, Set<string>>>({ requirement: new Set(), test: new Set() })
const sel = computed(() => selected.value[kind.value])
const shownIds = computed(() => shown.value.map((x) => x.id))
const headState = computed(() => selectionState(sel.value, shownIds.value))
function setAll(on: boolean) {
  selected.value[kind.value] = toggleIds(sel.value, shownIds.value, on)
}
function toggle(id: string, on: boolean) {
  selected.value[kind.value] = toggleIds(sel.value, [id], on)
}
function clearSelection() {
  selected.value = { requirement: new Set(), test: new Set() }
}
const plan = computed(() =>
  importPlan([...selected.value.requirement], canWriteTests.value ? [...selected.value.test] : [], reqById.value, inReqs.value, inTests.value, {
    includeTests: canWriteTests.value,
    addCopies: false,
  }),
)
const nReqs = (n: number) => t('catalog.nRequirements', { n }, n)
const nTests = (n: number) => t('catalog.nTests', { n }, n)
const resultSummary = (r: CatalogImportResult) =>
  t('catalog.result.summary', { requirements: nReqs(r.requirements.length), tests: nTests(r.tests.length), links: t('catalog.nLinks', { n: r.links.length }, r.links.length) })
const nSelected = computed(() => selected.value.requirement.size + selected.value.test.size)
const canImport = computed(() => (selected.value.requirement.size > 0 && canWriteReqs.value) || (selected.value.test.size > 0 && canWriteTests.value))
const addLabel = computed(() =>
  selected.value.requirement.size
    ? t('catalog.addButton', { requirements: nReqs(plan.value.requirements.length), tests: nTests(plan.value.tests.length) })
    : t('catalog.addTests', { n: plan.value.tests.length }, plan.value.tests.length),
)

// ---------- preview ----------
const previewId = ref<string>(typeof route.query.item === 'string' ? route.query.item : '')
const previewKind = ref<CatalogKind>(kind.value)
const previewOpen = computed({
  get: () => narrow.value && !!previewId.value,
  set: (v: boolean) => {
    if (!v) previewId.value = ''
  },
})
function openPreview(id: string, k: CatalogKind = kind.value) {
  previewKind.value = k
  previewId.value = id
}
function openLinked(k: CatalogKind, id: string) {
  if (k !== kind.value && ((k === 'requirement' && canReadReqs.value) || (k === 'test' && canReadTests.value))) kind.value = k
  openPreview(id, k)
  void nextTick(() => document.getElementById(`ci-${id}`)?.scrollIntoView({ block: 'nearest' }))
}
watch(kind, (k) => {
  if (previewKind.value !== k) previewId.value = ''
})
watch([kind, previewId, lang, () => f.value.node, () => f.value.q], () => {
  router.replace({
    query: {
      ...route.query,
      tab: kind.value === 'test' ? 'tests' : undefined,
      item: previewId.value || undefined,
      lang: lang.value,
      node: f.value.node !== 'all' ? f.value.node : undefined,
      q: f.value.q || undefined,
    },
  })
})

// ---------- import ----------
const importOpen = ref(false)
const result = ref<CatalogImportResult | null>(null)
const undoId = ref<number | null>(null)
const sources = computed(() => proj.project?.requirement_sources ?? [])
async function onImported(res: CatalogImportResult) {
  result.value = res
  clearSelection()
  const n = res.requirements.length + res.tests.length
  if (n) {
    const item = recordUndo(addedItemsEntry(t('catalog.undoLabel', { n }), proj.pid, res.requirements.map((r) => r.id), res.tests.map((x) => x.id)))
    undoId.value = item?.id ?? null
  }
  notify.ok(t('catalog.result.title'), resultSummary(res))
  await Promise.all([loadInProject(), res.folders_created ? Promise.all([proj.loadFolders('requirement'), proj.loadFolders('test')]) : Promise.resolve()])
  void nextTick(() => document.getElementById('catalog-result')?.focus())
}
const canUndoResult = computed(() => undoId.value !== null && undoStore.next?.id === undoId.value && !undoStore.busy)
async function undoImport() {
  if (undoId.value === null) return
  try {
    await undoStore.undoTo(undoId.value)
    notify.ok(t('catalog.result.undone'))
    result.value = null
    undoId.value = null
  } catch (e) {
    notify.fail(e)
  }
}
const reqQuery = computed(() => createdQuery(result.value?.requirements.map((r) => r.key) ?? []))
const testQuery = computed(() => createdQuery(result.value?.tests.map((r) => r.key) ?? []))
const keysOf = (id: string) => (inMap.value[id] ?? []).map((x) => x.key).join(', ')
</script>

<template>
  <div class="catalog-page">
    <div class="page-header">
      <div>
        <h1 id="catalog-title" tabindex="-1">{{ t('catalog.title') }}</h1>
        <div class="sub">{{ t('catalog.intro') }}</div>
      </div>
      <div class="actions">
        <HelpLink page="catalogue" />
        <SelectButton v-model="lang" :options="langOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('catalog.language')" />
      </div>
    </div>

    <Message v-if="error" severity="error" :closable="false">{{ t('catalog.loadFailed') }}: {{ error }}</Message>

    <div v-if="result" id="catalog-result" class="card result" tabindex="-1" role="region" :aria-label="t('catalog.result.title')">
      <div>
        <b>{{ t('catalog.result.title') }}</b> –
        {{ resultSummary(result) }}
        <span v-if="result.skipped.length" class="muted"> · {{ t('catalog.result.skipped', { n: result.skipped.length }, result.skipped.length) }}</span>
      </div>
      <div class="row" style="gap: 8px; flex-wrap: wrap">
        <RouterLink v-if="result.requirements.length" class="p-button p-button-sm p-button-outlined" :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { q: reqQuery } }">
          {{ t('catalog.result.openRequirements') }}
        </RouterLink>
        <RouterLink v-if="result.tests.length" class="p-button p-button-sm p-button-outlined" :to="{ name: 'tests', params: { projectId: proj.pid }, query: { q: testQuery } }">
          {{ t('catalog.result.openTests') }}
        </RouterLink>
        <Button v-if="canUndoResult" :label="t('catalog.result.undo')" icon="pi pi-undo" size="small" severity="secondary" outlined @click="undoImport" />
        <Button :label="t('catalog.result.dismiss')" size="small" text severity="secondary" @click="result = null" />
      </div>
    </div>

    <div class="row tabs-row">
      <SelectButton v-model="kind" :options="kindOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('catalog.tabs')" class="catalog-tabs" />
      <Button
        v-if="narrow"
        :label="t('catalog.areasButton')"
        icon="pi pi-sitemap"
        severity="secondary"
        outlined
        aria-haspopup="dialog"
        :aria-expanded="areasOpen"
        @click="areasOpen = true"
      />
    </div>

    <div class="catalog-grid" :class="{ narrow }">
      <aside v-if="!narrow" class="card tree-pane" :aria-label="t('catalog.areas')">
        <h2 class="pane-title">{{ t('catalog.areas') }}</h2>
        <Tree
          v-model:expanded-keys="expanded"
          :value="treeNodes as any"
          selection-mode="single"
          :selection-keys="selectionKeys"
          :meta-key-selection="false"
          :aria-label="t('catalog.areas')"
          class="catalog-tree"
          @node-select="(n) => selectNode(String(n.key))"
        >
          <template #default="{ node }">
            <span class="node-label">{{ node.label }}</span>
            <span class="node-count">{{ node.data.count }}</span>
          </template>
        </Tree>
      </aside>

      <section class="card list-pane" :aria-busy="loading" aria-labelledby="catalog-list-title">
        <h2 id="catalog-list-title" class="pane-title">{{ nodeTitle }}</h2>
        <div class="filterbar">
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="f.q" type="search" :placeholder="t('catalog.search')" :aria-label="t('catalog.searchLabel')" />
          </IconField>
        </div>
        <div class="filterbar">
          <template v-if="kind === 'requirement'">
            <MultiSelect v-model="f.types" :options="typeOptions" option-label="label" option-value="value" :placeholder="t('catalog.filterType')" :aria-label="t('catalog.filterType')" :max-selected-labels="2" show-clear />
            <MultiSelect v-model="f.obligations" :options="obligationOptions" option-label="label" option-value="value" :placeholder="t('catalog.filterObligation')" :aria-label="t('catalog.filterObligation')" :max-selected-labels="2" show-clear />
          </template>
          <MultiSelect v-else v-model="f.executions" :options="executionOptions" option-label="label" option-value="value" :placeholder="t('catalog.filterExecution')" :aria-label="t('catalog.filterExecution')" show-clear />
          <MultiSelect v-model="f.priorities" :options="priorityOptions" option-label="label" option-value="value" :placeholder="t('catalog.filterPriority')" :aria-label="t('catalog.filterPriority')" :max-selected-labels="2" show-clear />
          <MultiSelect v-model="f.tags" :options="tagOpts" option-label="label" option-value="value" filter :placeholder="t('catalog.filterTags')" :aria-label="t('catalog.filterTags')" :max-selected-labels="2" show-clear />
          <span class="row small nowrap">
            <ToggleSwitch v-model="f.hideInProject" input-id="cat-hide" />
            <label for="cat-hide">{{ t('catalog.hideInProject') }}</label>
          </span>
          <Button v-if="activeFilters(f) || f.q" :label="t('catalog.clearFilters')" icon="pi pi-filter-slash" text severity="secondary" @click="clearFilters" />
        </div>

        <div class="list-head">
          <Checkbox
            :model-value="headState === 'all'"
            :indeterminate="headState === 'some'"
            binary
            input-id="cat-all"
            :disabled="!shown.length"
            @update:model-value="(v: boolean) => setAll(v)"
          />
          <label for="cat-all" class="small">{{ t('catalog.selectAll') }}</label>
          <span class="small muted" role="status" aria-live="polite" style="margin-left: auto">{{ t('catalog.shown', { n: shown.length }, shown.length) }}</span>
        </div>

        <div v-if="loading && !items.length" class="empty"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
        <div v-else-if="!shown.length" class="empty">
          <i class="pi pi-search" aria-hidden="true" />
          <div>{{ t('catalog.noMatch') }}</div>
        </div>
        <ul v-else class="cat-list">
          <li v-for="it in visibleItems" :id="`ci-${it.id}`" :key="it.id" :class="{ active: previewId === it.id && previewKind === kind, chosen: sel.has(it.id) }">
            <Checkbox
              :model-value="sel.has(it.id)"
              binary
              :input-id="`cb-${it.id}`"
              :aria-label="t('catalog.selectItem', { id: it.id, title: it.title })"
              @update:model-value="(v: boolean) => toggle(it.id, v)"
            />
            <button type="button" class="cat-item" :aria-pressed="previewId === it.id && previewKind === kind" @click="openPreview(it.id)">
              <span class="cat-line">
                <span class="key">{{ it.id }}</span>
                <span class="cat-title">{{ it.title }}</span>
              </span>
              <span class="cat-meta">
                <template v-if="'obligation' in it">
                  <StatusTag :value="it.obligation" kind="obligation" />
                  <StatusTag :value="it.priority" kind="priority" />
                  <span class="small muted"><i class="pi pi-check-square" aria-hidden="true" /> {{ t('catalog.linkedTests', { n: it.tests.length }, it.tests.length) }}</span>
                </template>
                <template v-else>
                  <StatusTag :value="it.priority" kind="priority" />
                  <span class="small muted">{{ enumLabel('executionType', it.execution) }} · {{ t('catalog.steps', { n: it.steps }, it.steps) }}</span>
                  <span class="small muted"><i class="pi pi-list-check" aria-hidden="true" /> {{ t('catalog.linkedRequirements', { n: it.requirements.length }, it.requirements.length) }}</span>
                </template>
                <Tag v-if="inSet.has(it.id)" :value="t('catalog.inProject')" severity="success" v-tooltip.top="t('catalog.inProjectAs', { keys: keysOf(it.id) })" />
              </span>
            </button>
          </li>
        </ul>
        <div v-if="shown.length > limit" class="row" style="justify-content: center; margin-top: 8px">
          <Button :label="t('catalog.showMore', { n: shown.length - limit })" severity="secondary" text @click="limit += PAGE" />
        </div>
      </section>

      <aside v-if="!narrow" class="card preview-pane" :aria-label="t('catalog.preview')">
        <CatalogPreview v-if="previewId" :kind="previewKind" :id="previewId" :lang="lang" :project-id="proj.pid" @open="openLinked" />
        <div v-else class="empty">
          <i class="pi pi-eye" aria-hidden="true" />
          <div>{{ t('catalog.previewHint') }}</div>
        </div>
      </aside>
    </div>

    <div v-if="nSelected" class="bulkbar catalog-bar" role="toolbar" :aria-label="t('catalog.selected', { n: nSelected }, nSelected)">
      <b role="status" aria-live="polite">{{ t('catalog.selectedSummary', { requirements: nReqs(selected.requirement.size), tests: nTests(selected.test.size) }) }}</b>
      <Button v-if="canImport" :label="addLabel" icon="pi pi-plus" size="small" aria-haspopup="dialog" @click="importOpen = true" />
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('catalog.clearSelection')" v-tooltip.top="t('catalog.clearSelection')" @click="clearSelection" />
    </div>

    <Drawer v-if="narrow" v-model:visible="areasOpen" position="left" :header="t('catalog.areas')" block-scroll class="catalog-areas-drawer">
      <Tree
        v-model:expanded-keys="expanded"
        :value="treeNodes as any"
        selection-mode="single"
        :selection-keys="selectionKeys"
        :meta-key-selection="false"
        :aria-label="t('catalog.areas')"
        class="catalog-tree"
        @node-select="(n) => selectNode(String(n.key))"
      >
        <template #default="{ node }">
          <span class="node-label">{{ node.label }}</span>
          <span class="node-count">{{ node.data.count }}</span>
        </template>
      </Tree>
    </Drawer>
    <Drawer v-if="narrow" v-model:visible="previewOpen" position="right" :header="t('catalog.preview')" block-scroll class="catalog-preview-drawer">
      <CatalogPreview v-if="previewId" :kind="previewKind" :id="previewId" :lang="lang" :project-id="proj.pid" @open="openLinked" />
    </Drawer>

    <CatalogImportDialog
      v-if="canImport || importOpen"
      v-model:visible="importOpen"
      v-model:lang="lang"
      :project-id="proj.pid"
      :requirement-ids="canWriteReqs ? [...selected.requirement] : []"
      :test-ids="[...selected.test]"
      :req-by-id="reqById"
      :in-reqs="inReqs"
      :in-tests="inTests"
      :requirement-folders="proj.folders.requirement"
      :test-folders="proj.folders.test"
      :sources="sources"
      :can-tests="canWriteTests"
      @imported="onImported"
    />
  </div>
</template>

<style scoped>
.tabs-row { gap: 8px; margin-bottom: 10px; flex-wrap: wrap; }
.catalog-grid { display: grid; grid-template-columns: 260px minmax(0, 1fr) minmax(300px, 380px); gap: 16px; align-items: start; }
.catalog-grid.narrow { grid-template-columns: minmax(0, 1fr); }
.tree-pane, .preview-pane { position: sticky; top: calc(var(--qtf-topbar-h) + 60px); max-height: calc(100vh - var(--qtf-topbar-h) - 80px); overflow: auto; padding: 10px; }
.pane-title { font-size: 15px; margin: 0 0 8px; font-weight: 650; }
.catalog-tree { padding: 0; border: none; background: transparent; }
.catalog-tree :deep(.p-tree-node-content) { min-width: 0; }
.catalog-tree :deep(.p-tree-node-label) { display: flex; gap: 6px; align-items: center; width: 100%; min-width: 0; white-space: normal; }
.node-label { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.node-count { font-size: 12px; color: var(--qtf-muted); font-variant-numeric: tabular-nums; }
.search { width: 100%; }
.search :deep(input) { width: 100%; }
.list-head { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid var(--qtf-border); }
.cat-list { list-style: none; margin: 0; padding: 0; }
.cat-list li { display: flex; gap: 8px; align-items: flex-start; padding: 8px; border-bottom: 1px solid var(--qtf-border); }
.cat-list li > .p-checkbox { margin-top: 3px; flex: none; }
.cat-list li.active { background: color-mix(in srgb, var(--p-primary-color) 10%, transparent); }
.cat-item { flex: 1; min-width: 0; text-align: left; background: none; border: 0; padding: 0; color: inherit; font: inherit; cursor: pointer; display: grid; gap: 4px; }
.cat-item:focus-visible { outline: 2px solid var(--p-primary-color); outline-offset: 2px; border-radius: 4px; }
.cat-line { display: flex; gap: 8px; align-items: baseline; min-width: 0; }
.cat-title { font-weight: 550; overflow-wrap: anywhere; }
.cat-item:hover .cat-title { text-decoration: underline; }
.cat-meta { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
.result { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; justify-content: space-between; margin-bottom: 12px; padding: 10px 14px; }
.catalog-bar { position: sticky; bottom: 8px; margin-top: 12px; z-index: 2; }
</style>
