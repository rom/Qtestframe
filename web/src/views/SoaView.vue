<script setup lang="ts">
// Statement of Applicability: every control of a reference source (ISO/IEC
// 27001:2022 Annex A ships its 93 controls) with the project's decision
// whether it applies, why and who owns it — edited in the spreadsheet grid
// (type, paste, fill down) — and the implementation status derived from
// the adopted requirements and their tests. The document is the standard
// report "soa" (PDF, Word, Excel, Markdown), downloaded or archived.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ProgressSpinner from 'primevue/progressspinner'
import SpreadsheetGrid, { type ApplyKind, type CellState } from '@/components/SpreadsheetGrid.vue'
import ItemRef from '@/components/ItemRef.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage, postDownload } from '@/lib/api'
import { P } from '@/lib/permissions'
import { sortRows, type Plan } from '@/lib/sheet'
import type { SheetRow } from '@/lib/sheetFields'
import { decisionsFor, filterSoARows, soaGroups, soaSheetFields, soaSheetRow, soaStatusSeverity, unjustifiedExclusions, APPLICABILITY, SOA_STATUSES, type SoARow, type SoASource, type SoAStatement } from '@/lib/soa'
import { FORMAT_INFO, type ReportFormat } from '@/lib/reports'
import { enumLabel } from '@/i18n/labels'
import { currentLang } from '@/i18n'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const canWrite = computed(() => auth.can(P.requirementWrite))
const canExport = computed(() => auth.can(P.export))

// ---------- sources and statement ----------
const sources = ref<SoASource[]>([])
const source = ref(typeof route.query.source === 'string' ? route.query.source : 'iso-27001')
const statement = ref<SoAStatement | null>(null)
const loading = ref(false)
const error = ref('')
const sourceOptions = computed(() => sources.value.map((s) => ({ label: s.shipped ? s.title : `${s.title} (${t('soa.derived')})`, value: s.id })))

async function loadSources() {
  try {
    sources.value = asItems(await api.get<{ items: SoASource[] }>(`/projects/${proj.pid}/soa/sources`, { lang: currentLang() }))
  } catch {
    sources.value = []
  }
}
async function load() {
  loading.value = true
  error.value = ''
  try {
    statement.value = await api.get<SoAStatement>(`/projects/${proj.pid}/soa`, { source: source.value, lang: currentLang() })
  } catch (e) {
    error.value = errorMessage(e)
    statement.value = null
  } finally {
    loading.value = false
  }
}
onMounted(() => Promise.all([loadSources(), load()]))
watch(source, (v) => {
  router.replace({ query: { ...route.query, source: v } })
  cellStates.clear()
  void load()
})

// ---------- filters ----------
const text = ref('')
const applicability = ref<string[]>([])
const statuses = ref<string[]>([])
const group = ref('')
const rows = computed(() => statement.value?.rows ?? [])
const groups = computed(() => soaGroups(rows.value))
const groupOptions = computed(() => [{ label: t('soa.allGroups'), value: '' }, ...groups.value.map((g) => ({ label: `${g.id} ${g.title}`, value: g.id }))])
const applicabilityOptions = computed(() => APPLICABILITY.map((v) => ({ label: t(`soa.applicability.${v || 'undecided'}`), value: v })))
const statusOptions = computed(() => SOA_STATUSES.map((v) => ({ label: t(`soa.status.${v}`), value: v })))
const filtered = computed(() => filterSoARows(rows.value, { text: text.value, applicability: applicability.value, statuses: statuses.value, group: group.value }))
const hasFilter = computed(() => !!text.value.trim() || !!applicability.value.length || !!statuses.value.length || !!group.value)
function clearFilters() {
  text.value = ''
  applicability.value = []
  statuses.value = []
  group.value = ''
}

// ---------- grid ----------
const fields = computed(() => soaSheetFields(t, canWrite.value))
const widths = ref<Record<string, number>>({})
const cellStates = reactive(new Map<string, CellState>())
const conflicts = reactive(new Set<string>())
const selected = reactive(new Set<string>())
const sort = ref<{ field: string; dir: 1 | -1 } | null>(null)
const sheetRows = computed<SheetRow[]>(() => {
  const list = filtered.value.map(soaSheetRow)
  const s = sort.value
  return s ? sortRows(list, fields.value.find((f) => f.id === s.field), s.dir) : list
})
function onSort(id: string) {
  const s = sort.value
  sort.value = !s || s.field !== id ? { field: id, dir: 1 } : s.dir === 1 ? { field: id, dir: -1 } : null
}
function onResize(id: string, w: number) {
  widths.value = { ...widths.value, [id]: w }
}
function onSelectRows(ids: string[], on: boolean) {
  for (const id of ids) {
    if (on) selected.add(id)
    else selected.delete(id)
  }
}
const status = ref('')
const alertText = ref('')
function announce(msg: string, urgent = false) {
  if (urgent) alertText.value = msg
  else status.value = msg
}
const readOnlyMessage = () => (canWrite.value ? t('soa.readOnlyColumn') : t('soa.readOnly'))

async function apply(plan: Plan, _kind: ApplyKind) {
  if (!plan.changes.length || !statement.value) return
  const decisions = decisionsFor(rows.value, plan.changes, sheetRows.value)
  const bad = unjustifiedExclusions(decisions)
  const keys = plan.changes.map((c) => `${sheetRows.value[c.row]?.id}:${c.field}`)
  if (bad.length) {
    announce(t('soa.needJustification', { ids: bad.join(', ') }), true)
    for (const k of keys) cellStates.set(k, { state: 'error', message: t('soa.needJustificationShort') })
    return
  }
  for (const k of keys) cellStates.set(k, { state: 'pending' })
  try {
    const res = await api.put<{ changed: number; statement: SoAStatement }>(`/projects/${proj.pid}/soa/decisions`, { source: source.value, decisions })
    statement.value = res.statement
    for (const k of keys) cellStates.set(k, { state: 'saved' })
    announce(t('soa.saved', { n: res.changed }, res.changed))
  } catch (e) {
    for (const k of keys) cellStates.set(k, { state: 'error', message: errorMessage(e) })
    announce(errorMessage(e), true)
  }
}

// ---------- control details ----------
const detail = ref<SoARow | null>(null)
const detailOpen = ref(false)
function openControl(id: string) {
  detail.value = rows.value.find((r) => r.id === id) ?? null
  detailOpen.value = !!detail.value
}

// ---------- document ----------
const format = ref<ReportFormat>('pdf')
const formats: ReportFormat[] = ['pdf', 'docx', 'xlsx', 'md']
const formatOptions = formats.map((f) => ({ label: FORMAT_INFO[f].label, value: f }))
const docBusy = ref<'' | 'download' | 'archive'>('')
async function generate(archive: boolean) {
  docBusy.value = archive ? 'archive' : 'download'
  const body = { template: 'soa', params: { source: source.value }, format: format.value, archive, lang: currentLang() }
  try {
    if (archive) {
      await api.post(`/projects/${proj.pid}/reports/generate`, body)
      notify.ok(t('reports.archived'))
    } else {
      await postDownload(`/projects/${proj.pid}/reports/generate`, body, `soa.${format.value}`)
      notify.ok(t('reports.downloaded'))
    }
  } catch (e) {
    notify.fail(e)
  } finally {
    docBusy.value = ''
  }
}
const summary = computed(() => statement.value?.summary)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('soa.title') }}</h1>
      <div class="sub">{{ t('soa.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="waivers-and-soa" />
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
    </div>
  </div>

  <section class="card soa-top" aria-labelledby="soa-src-title">
    <h2 id="soa-src-title" class="sr-only">{{ t('soa.source') }}</h2>
    <div class="filterbar">
      <label for="soa-source" class="small"><b>{{ t('soa.source') }}</b></label>
      <Select v-model="source" input-id="soa-source" :options="sourceOptions" option-label="label" option-value="value" filter style="min-width: 260px; max-width: 100%" />
      <span class="grow" />
      <template v-if="canExport">
        <Select v-model="format" :options="formatOptions" option-label="label" option-value="value" :aria-label="t('reports.format')" style="min-width: 110px" />
        <Button :label="t('reports.download')" icon="pi pi-download" size="small" :loading="docBusy === 'download'" :disabled="!!docBusy || !statement" @click="generate(false)" />
        <Button :label="t('reports.saveToArchive')" icon="pi pi-folder-plus" size="small" severity="secondary" outlined :loading="docBusy === 'archive'" :disabled="!!docBusy || !statement" @click="generate(true)" />
      </template>
    </div>
    <p v-if="statement && !statement.source.shipped" class="small muted" style="margin: 6px 0 0">{{ t('soa.derivedHint') }}</p>
  </section>

  <Message v-if="error" severity="error" :closable="false" style="margin-top: 12px">{{ error }}</Message>
  <div v-else-if="loading && !statement" class="empty"><ProgressSpinner style="width: 32px; height: 32px" :aria-label="t('common.loading')" /></div>
  <template v-else-if="statement && summary">
    <ul class="plain-list soa-kpis" :aria-label="t('soa.summary')">
      <li class="card"><span class="kpi">{{ summary.controls }}</span><span class="small muted">{{ t('soa.kpi.controls') }}</span></li>
      <li class="card"><span class="kpi">{{ summary.applicable }}</span><span class="small muted">{{ t('soa.kpi.applicable') }}</span></li>
      <li class="card"><span class="kpi">{{ summary.excluded }}</span><span class="small muted">{{ t('soa.kpi.excluded') }}</span></li>
      <li class="card"><span class="kpi">{{ summary.undecided }}</span><span class="small muted">{{ t('soa.kpi.undecided') }}</span></li>
      <li v-for="s in SOA_STATUSES" :key="s" class="card">
        <span class="kpi">{{ summary[s] }}</span>
        <Tag :value="t(`soa.status.${s}`)" :severity="soaStatusSeverity(s)" />
      </li>
    </ul>

    <section class="card" style="margin-top: 12px" aria-labelledby="soa-table-title">
      <h2 id="soa-table-title" class="plain-heading" style="margin-bottom: 6px">{{ statement.source.title }}</h2>
      <p class="small muted" style="margin: 0 0 8px">{{ canWrite ? t('soa.editHint') : t('soa.readOnly') }}</p>
      <div class="filterbar">
        <InputText v-model="text" :placeholder="t('soa.searchPlaceholder')" :aria-label="t('soa.searchPlaceholder')" style="min-width: 180px" />
        <MultiSelect v-model="applicability" :options="applicabilityOptions" option-label="label" option-value="value" :placeholder="t('soa.col.applicable')" :aria-label="t('soa.col.applicable')" show-clear style="min-width: 150px" />
        <MultiSelect v-model="statuses" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('soa.col.status')" :aria-label="t('soa.col.status')" show-clear style="min-width: 150px" />
        <Select v-if="groups.length > 1" v-model="group" :options="groupOptions" option-label="label" option-value="value" :aria-label="t('soa.group')" style="min-width: 160px" />
        <Button v-if="hasFilter" :label="t('common.clear')" text size="small" severity="secondary" @click="clearFilters" />
        <span class="small muted" role="status">{{ t('soa.shown', { count: filtered.length, total: rows.length }) }}</span>
      </div>
      <div class="soa-grid">
        <SpreadsheetGrid
          :rows="sheetRows"
          :fields="fields"
          :widths="widths"
          :label="t('soa.gridLabel', { source: statement.source.title })"
          :cell-states="cellStates"
          :conflicts="conflicts"
          :selected-ids="selected"
          :sort="sort"
          :copy-headers="false"
          :read-only-message="readOnlyMessage"
          @apply="apply"
          @announce="announce"
          @select-rows="onSelectRows"
          @sort="onSort"
          @resize="onResize"
          @open="openControl"
        />
      </div>
      <div class="sr-only" role="status" aria-live="polite" aria-atomic="true">{{ status }}</div>
      <div class="sr-only" role="alert" aria-atomic="true">{{ alertText }}</div>
      <p class="small muted" style="margin: 8px 0 0">{{ t('soa.openHint') }}</p>
    </section>
  </template>

  <Dialog v-model:visible="detailOpen" :header="detail ? `${detail.id} ${detail.title}` : ''" modal style="width: min(640px, 96vw)">
    <template v-if="detail">
      <div class="row" style="margin-bottom: 8px">
        <Tag :value="t(`soa.applicability.${detail.applicable === true ? 'yes' : detail.applicable === false ? 'no' : 'undecided'}`)" severity="secondary" />
        <Tag v-if="detail.status" :value="t(`soa.status.${detail.status}`)" :severity="soaStatusSeverity(detail.status)" />
        <span v-if="detail.group_title" class="small muted">{{ detail.group }} {{ detail.group_title }}</span>
      </div>
      <p v-if="detail.justification" class="pre">{{ detail.justification }}</p>
      <p class="small"><b>{{ t('soa.col.owner') }}:</b> {{ detail.owner || '—' }}</p>
      <p v-if="detail.suggested" class="small muted">{{ t('soa.suggested') }}</p>
      <p class="small muted">{{ t('soa.catalogueCounts', { catalogue: detail.catalogue, missing: detail.missing }) }}</p>
      <h3 class="plain-heading small">{{ t('soa.col.requirements') }}</h3>
      <div v-if="!detail.requirements.length" class="small muted">{{ t('soa.noRequirements') }}</div>
      <div v-for="q in detail.requirements" :key="q.id" class="row nowrap" style="padding: 3px 0">
        <ItemRef :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: q.id } }" :item-key="q.key" :title="q.title" title-class="ellipsis" grow />
        <Tag v-if="q.waiver" :value="t('soa.waived', { keys: q.waiver })" severity="contrast" icon="pi pi-verified" />
        <Tag :value="t(`soa.state.${q.state}`)" severity="secondary" />
      </div>
      <h3 class="plain-heading small" style="margin-top: 10px">{{ t('soa.col.evidence') }}</h3>
      <div v-if="!detail.evidence.length" class="small muted">{{ t('soa.noEvidence') }}</div>
      <div v-for="e in detail.evidence" :key="e.id" class="row nowrap" style="padding: 3px 0">
        <ItemRef :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: e.id } }" :item-key="e.key" :title="e.title" title-class="ellipsis" grow />
        <Tag :value="enumLabel('result', e.result || 'not_run')" severity="secondary" />
      </div>
    </template>
  </Dialog>
</template>

<style scoped>
.soa-kpis {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 10px;
  margin-top: 12px;
}
.soa-kpis li {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-start;
  padding: 10px 12px;
}
.kpi {
  font-size: 22px;
  font-weight: 700;
}
.soa-grid {
  height: min(70vh, 720px);
  min-height: 320px;
  display: flex;
  flex-direction: column;
}
.pre {
  white-space: pre-wrap;
}
</style>
