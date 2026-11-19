<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ProgressSpinner from 'primevue/progressspinner'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import QueryInput from '@/components/QueryInput.vue'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { formatDateTime, formatFullDateTime, pluralize } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { Baseline, BaselineChange, BaselineDiff, BaselineItem } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'

const proj = useProjectStore()
const auth = useAuthStore()
const prefs = usePrefsStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const canWrite = computed(() => auth.can(P.requirementWrite))

const baselines = ref<Baseline[]>([])
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    baselines.value = asItems(await api.get<Baseline[]>(`/projects/${proj.pid}/baselines`))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---------- selected baseline ----------
const compareTo = ref('current')
const diff = ref<BaselineDiff | null>(null)
const diffLoading = ref(false)
const diffError = ref('')
const selectedId = computed(() => (route.query.b as string) || null)
const detail = ref<Baseline | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
const tab = ref<'items' | 'compare'>((route.query.tab as 'items' | 'compare') || 'items')
const typeFilter = ref<'all' | 'requirement' | 'test'>('all')

function open(b: Baseline, t: 'items' | 'compare' = 'items') {
  tab.value = t
  router.replace({ query: { ...route.query, b: b.id, tab: t } })
}
function closeDetail() {
  router.replace({ query: { ...route.query, b: undefined, tab: undefined } })
}
watch(tab, (t) => {
  if (selectedId.value && route.query.tab !== t) router.replace({ query: { ...route.query, tab: t } })
})
watch(
  selectedId,
  async (id) => {
    detail.value = null
    diff.value = null
    diffError.value = ''
    compareTo.value = 'current'
    if (!id) return
    detailLoading.value = true
    detailError.value = ''
    try {
      detail.value = await api.get<Baseline>(`/baselines/${id}`)
      if (tab.value === 'compare') compare()
    } catch (e) {
      detailError.value = errorMessage(e)
    } finally {
      detailLoading.value = false
    }
  },
  { immediate: true },
)
const items = computed<BaselineItem[]>(() =>
  [...(detail.value?.items ?? [])]
    .filter((i) => typeFilter.value === 'all' || i.entity_type === typeFilter.value)
    .sort((a, b) => a.entity_type.localeCompare(b.entity_type) || a.key.localeCompare(b.key, undefined, { numeric: true })),
)
const typeOptions = [
  { label: 'All', value: 'all' },
  { label: 'Requirements', value: 'requirement' },
  { label: 'Tests', value: 'test' },
]

function itemRoute(type: string, id: string) {
  return { name: type === 'test' ? 'tests' : 'requirements', params: { projectId: proj.pid }, query: { item: id } }
}

// ---------- compare ----------
const compareOptions = computed(() => [
  { label: 'Current state', value: 'current' },
  ...baselines.value.filter((b) => b.id !== selectedId.value).map((b) => ({ label: `${b.name} (${formatDateTime(b.created_at)})`, value: b.id })),
])
async function compare() {
  if (!selectedId.value) return
  diffLoading.value = true
  diffError.value = ''
  try {
    diff.value = await api.get<BaselineDiff>(`/baselines/${selectedId.value}/compare`, { to: compareTo.value })
  } catch (e) {
    diff.value = null
    diffError.value = errorMessage(e)
  } finally {
    diffLoading.value = false
  }
}
watch(compareTo, () => {
  if (tab.value === 'compare') compare()
})
watch(tab, (t) => {
  if (t === 'compare' && !diff.value && !diffLoading.value) compare()
})
const toLabel = computed(() => compareOptions.value.find((o) => o.value === compareTo.value)?.label ?? compareTo.value)
const sections = computed(() => {
  const d = diff.value
  if (!d) return []
  const sort = (l: BaselineChange[] | null) => [...(l ?? [])].sort((a, b) => a.entity_type.localeCompare(b.entity_type) || a.key.localeCompare(b.key, undefined, { numeric: true }))
  return [
    { kind: 'added', label: 'Added', icon: 'pi pi-plus-circle', color: 'var(--p-green-500)', severity: 'success', items: sort(d.added) },
    { kind: 'removed', label: 'Removed', icon: 'pi pi-minus-circle', color: 'var(--p-red-500)', severity: 'danger', items: sort(d.removed) },
    { kind: 'changed', label: 'Changed', icon: 'pi pi-pencil', color: 'var(--p-amber-500)', severity: 'warn', items: sort(d.changed) },
  ]
})

// ---------- create / delete ----------
const dialog = ref(false)
const form = ref({ name: '', description: '', query: '' })
const formError = ref<unknown>(null)
const busy = ref(false)
function openCreate() {
  form.value = { name: '', description: '', query: '' }
  formError.value = null
  dialog.value = true
}
async function create() {
  busy.value = true
  formError.value = null
  try {
    const b = await api.post<Baseline>(`/projects/${proj.pid}/baselines`, {
      name: form.value.name.trim(),
      description: form.value.description,
      ...(form.value.query.trim() ? { query: form.value.query.trim() } : {}),
    })
    dialog.value = false
    notify.ok('Baseline created', b ? `${b.name}: ${pluralize(b.requirement_count ?? 0, 'requirement')}, ${pluralize(b.test_count ?? 0, 'test')}` : undefined)
    await load()
    if (b?.id) open(b)
  } catch (e) {
    formError.value = e
  } finally {
    busy.value = false
  }
}
const formMessage = computed(() => (formError.value && !(formError.value instanceof ApiError && formError.value.code === 'invalid_query') ? errorMessage(formError.value) : ''))
function remove(b: Baseline) {
  notify.confirmDanger(`Delete baseline "${b.name}"? The items themselves are not affected.`, async () => {
    try {
      await api.del(`/baselines/${b.id}`)
      notify.ok('Baseline deleted')
      if (selectedId.value === b.id) closeDetail()
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>Baselines</h1>
      <div class="sub">Frozen versions of the project's requirements and tests, e.g. for a release or an audit. Compare them with each other or with the current state.</div>
    </div>
    <div class="actions">
      <Button icon="pi pi-refresh" text rounded severity="secondary" aria-label="Refresh" @click="load" />
      <Button v-if="canWrite" label="New baseline" icon="pi pi-plus" @click="openCreate" />
    </div>
  </div>

  <Message v-if="error" severity="error" :closable="false">Baselines are unavailable: {{ error }}</Message>
  <div v-else class="card">
    <DataTable
      :value="baselines"
      data-key="id"
      :loading="loading"
      paginator
      :rows="prefs.rows"
      :always-show-paginator="false"
      row-hover
      size="small"
      :row-class="(b: Baseline) => (b.id === selectedId ? 'p-datatable-row-selected' : '')"
      @row-click="(e) => open(e.data as Baseline)"
    >
      <Column field="name" header="Name" sortable>
        <template #body="{ data }"><span class="clickable" style="font-weight: 600"><i class="pi pi-flag muted" /> {{ data.name }}</span></template>
      </Column>
      <Column field="description" header="Description"><template #body="{ data }"><span class="small muted">{{ data.description }}</span></template></Column>
      <Column field="requirement_count" header="Requirements" sortable style="width: 120px" />
      <Column field="test_count" header="Tests" sortable style="width: 90px" />
      <Column field="created_by" header="Created by" sortable style="width: 170px"><template #body="{ data }"><span class="small">{{ data.created_by }}</span></template></Column>
      <Column field="created_at" header="Created" sortable style="width: 150px">
        <template #body="{ data }"><span class="small muted" :title="formatFullDateTime(data.created_at)">{{ formatDateTime(data.created_at) }}</span></template>
      </Column>
      <Column style="width: 120px">
        <template #body="{ data }">
          <div class="row nowrap" style="gap: 0">
            <Button icon="pi pi-list" text rounded size="small" severity="secondary" v-tooltip="'Items'" aria-label="Items" @click.stop="open(data, 'items')" />
            <Button icon="pi pi-arrow-right-arrow-left" text rounded size="small" v-tooltip="'Compare'" aria-label="Compare" @click.stop="open(data, 'compare')" />
            <Button v-if="canWrite" icon="pi pi-trash" text rounded size="small" severity="danger" v-tooltip="'Delete'" aria-label="Delete" @click.stop="remove(data)" />
          </div>
        </template>
      </Column>
      <template #empty>
        <div class="empty"><i class="pi pi-flag" />No baselines yet.{{ canWrite ? ' Create one to freeze the current versions.' : '' }}</div>
      </template>
    </DataTable>
  </div>

  <div v-if="selectedId" class="card" style="margin-top: 16px">
    <Message v-if="detailError" severity="error" :closable="false">{{ detailError }}</Message>
    <div v-else-if="detailLoading || !detail" class="empty"><ProgressSpinner style="width: 32px; height: 32px" /></div>
    <template v-else>
      <div class="row" style="margin-bottom: 6px">
        <h3 style="margin: 0"><i class="pi pi-flag" /> {{ detail.name }}</h3>
        <Tag :value="pluralize(detail.requirement_count ?? 0, 'requirement')" severity="secondary" />
        <Tag :value="pluralize(detail.test_count ?? 0, 'test')" severity="secondary" />
        <span class="small muted">created {{ formatDateTime(detail.created_at) }}{{ detail.created_by ? ' by ' + detail.created_by : '' }}</span>
        <span class="grow" />
        <Button icon="pi pi-times" text rounded severity="secondary" aria-label="Close" @click="closeDetail" />
      </div>
      <p v-if="detail.description" class="muted" style="margin: 0 0 8px">{{ detail.description }}</p>
      <Tabs v-model:value="tab" class="editor-tabs" style="margin-top: 4px">
        <TabList>
          <Tab value="items"><i class="pi pi-list" /> Items <span class="count">{{ detail.items?.length ?? 0 }}</span></Tab>
          <Tab value="compare"><i class="pi pi-arrow-right-arrow-left" /> Compare</Tab>
        </TabList>
      </Tabs>

      <template v-if="tab === 'items'">
        <div class="filterbar">
          <SelectButton v-model="typeFilter" :options="typeOptions" option-label="label" option-value="value" :allow-empty="false" size="small" />
        </div>
        <DataTable :value="items" data-key="entity_id" size="small" paginator :rows="prefs.rowsFor(50)" :always-show-paginator="false" row-hover>
          <Column field="entity_type" header="Type" style="width: 130px">
            <template #body="{ data }">
              <span class="row nowrap small"><i :class="data.entity_type === 'test' ? 'pi pi-check-square' : 'pi pi-list-check'" class="muted" />{{ data.entity_type === 'test' ? 'Test' : 'Requirement' }}</span>
            </template>
          </Column>
          <Column field="key" header="Key" style="width: 150px">
            <template #body="{ data }"><RouterLink :to="itemRoute(data.entity_type, data.entity_id)" class="key">{{ data.key }}</RouterLink></template>
          </Column>
          <Column field="title" header="Title" />
          <Column field="version" header="Version" style="width: 90px"><template #body="{ data }">v{{ data.version }}</template></Column>
          <template #empty><div class="empty">This baseline holds no items.</div></template>
        </DataTable>
      </template>

      <template v-else>
        <div class="filterbar">
          <span class="small muted">Compare <b>{{ detail.name }}</b> with</span>
          <Select v-model="compareTo" :options="compareOptions" option-label="label" option-value="value" style="min-width: 240px" aria-label="Compare with" />
          <Button icon="pi pi-refresh" text rounded severity="secondary" aria-label="Compare again" :loading="diffLoading" @click="compare" />
        </div>
        <Message v-if="diffError" severity="error" :closable="false">Comparison failed: {{ diffError }}</Message>
        <div v-else-if="diffLoading && !diff" class="empty"><ProgressSpinner style="width: 30px; height: 30px" /></div>
        <template v-else-if="diff">
          <div class="row" style="margin-bottom: 12px">
            <Tag v-for="s in sections" :key="s.kind" :value="`${s.items.length} ${s.label.toLowerCase()}`" :severity="s.severity" />
            <Tag :value="`${diff.unchanged} unchanged`" severity="secondary" />
            <span class="small muted">{{ detail.name }} → {{ toLabel }}</span>
          </div>
          <div v-if="!sections.some((s) => s.items.length)" class="muted small"><i class="pi pi-check-circle" style="color: var(--p-green-500)" /> No differences.</div>
          <div v-for="s in sections" v-show="s.items.length" :key="s.kind" class="bl-section" style="margin-bottom: 14px">
            <h4><i :class="s.icon" :style="{ color: s.color }" /> {{ s.label }} ({{ s.items.length }})</h4>
            <div v-for="c in s.items" :key="c.entity_type + c.entity_id" class="bl-row" :class="'bl-' + s.kind">
              <i :class="c.entity_type === 'test' ? 'pi pi-check-square' : 'pi pi-list-check'" class="muted" :title="c.entity_type" />
              <RouterLink v-if="s.kind !== 'removed' || compareTo === 'current'" :to="itemRoute(c.entity_type, c.entity_id)" class="key">{{ c.key }}</RouterLink>
              <span v-else class="key">{{ c.key }}</span>
              <span class="grow ellipsis">{{ c.title }}</span>
              <span v-if="s.kind === 'changed'" class="small mono">v{{ c.from_version }} → v{{ c.to_version }}</span>
              <span v-else-if="s.kind === 'added' && c.to_version" class="small mono muted">v{{ c.to_version }}</span>
              <span v-else-if="s.kind === 'removed' && c.from_version" class="small mono muted">v{{ c.from_version }}</span>
              <span v-if="c.changed_fields?.length" style="flex-basis: 100%; padding-left: 22px">
                <span v-for="f in c.changed_fields" :key="f" class="field-chip">{{ f }}</span>
              </span>
            </div>
          </div>
          <div v-if="compareTo === 'current'" class="small muted">The current state includes all items of the project, also those outside the baseline's filter.</div>
        </template>
      </template>
    </template>
  </div>

  <Dialog v-model:visible="dialog" header="New baseline" modal style="width: min(620px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="create">
      <Message v-if="formMessage" severity="error" :closable="false">{{ formMessage }}</Message>
      <div class="field"><label for="bl-name">Name</label><InputText id="bl-name" v-model="form.name" placeholder="e.g. Release 2.0" autofocus /></div>
      <div class="field"><label for="bl-desc">Description</label><Textarea id="bl-desc" v-model="form.description" rows="2" auto-resize /></div>
      <div class="field">
        <label>Filter (optional)</label>
        <QueryInput v-model="form.query" scope="requirement" :error="formError" placeholder="e.g. status:approved  (empty = all requirements and tests)" @submit="create" />
        <span class="small muted">Applied to requirements and tests like the export filter.</span>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" text severity="secondary" @click="dialog = false" />
        <Button type="submit" label="Create baseline" icon="pi pi-flag" :loading="busy" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>
