<script setup lang="ts">
// Releases: scope (requirements), runs, exit criteria and go / no-go.
import ItemRef from '../components/ItemRef.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ProgressSpinner from 'primevue/progressspinner'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import QueryInput from '@/components/QueryInput.vue'
import StatusTag from '@/components/StatusTag.vue'
import SummaryBar from '@/components/SummaryBar.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { valuesBefore } from '@/lib/bulkRuns'
import { entry, recordUndo } from '@/lib/undoActions'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import {
  canRelease,
  changeCriterionType,
  cleanCriteria,
  criteriaCounts,
  criterionParam,
  criterionText,
  newCriterion,
  releaseBaselineName,
  releaseItemRoute,
  resultValues,
  runsOfRelease,
  CRITERION_TYPES,
  DEFECT_SEVERITIES,
  RULE_LEVELS,
  MIN_RELEASE_NOTE,
  RELEASE_STATUSES,
} from '@/lib/releases'
import { RISK_LEVELS, riskLevelSeverity } from '@/lib/risks'
import type { Baseline, CriterionResult, ExitCriterion, Release, ReleaseStatusReport, Requirement, TestCycle, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import SuggestInput from '@/components/SuggestInput.vue'
import { rememberValues, savedEntries } from '@/lib/suggest'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'
import VariantSelect from '@/components/VariantSelect.vue'
import { useVariants } from '@/composables/variants'

const proj = useProjectStore()
const auth = useAuthStore()
const prefs = usePrefsStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const canWrite = computed(() => auth.can(P.projectUpdate))
const tabsId = 'rel-tabs'
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })

const statusLabel = (s: string) => t(`releasesView.status.${s}`)
const statusSeverity = (s: string) => ({ planned: 'secondary', active: 'info', released: 'success', cancelled: 'contrast' })[s] ?? 'secondary'
const levelLabel = (kind: 'severity' | 'riskLevel', v: string) => t(kind === 'severity' ? `releasesView.severity.${v}` : `risksView.level.${v}`)
const cycles = ref<TestCycle[]>([])
const cycleOptions = computed(() => cycles.value.map((c) => ({ label: c.name, value: c.id })))
const cycleName = (cid: string) => cycles.value.find((c) => c.id === cid)?.name ?? ''
async function loadCycles() {
  try {
    cycles.value = asItems(await api.get<{ items: TestCycle[] }>(`/projects/${proj.pid}/cycles`))
  } catch {
    cycles.value = [] // without run.read there are no cycles to choose from
  }
}
const describe = (c: ExitCriterion) => criterionText(c, t, levelLabel, cycleName)

// ---------- list ----------
const releases = ref<Release[]>([])
const reports = ref<Record<string, ReleaseStatusReport | null>>({})
const loading = ref(false)
const error = ref('')

// readiness for one variant (?variant=): its requirements, tests and results
const variants = useVariants(() => proj.pid)
const readinessVariant = ref<string | null>(null)
watch(readinessVariant, () => void Promise.all(releases.value.map((r) => loadReport(r.id))))
async function loadReport(id: string) {
  let rep: ReleaseStatusReport | null = null
  try {
    rep = await api.get<ReleaseStatusReport>(`/releases/${id}/status`, { variant: readinessVariant.value || undefined })
  } catch {
    rep = null
  }
  // spread after the await: concurrent loads must not overwrite each other
  reports.value = { ...reports.value, [id]: rep }
}
async function load() {
  loading.value = true
  error.value = ''
  try {
    releases.value = asItems(await api.get<{ items: Release[] }>(`/projects/${proj.pid}/releases`))
    await Promise.all(releases.value.map((r) => loadReport(r.id)))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

// ---------- bulk ----------
const selection = ref<Release[]>([])
const selectedIds = computed(() => selection.value.map((r) => r.id))
const bulkStatusOptions = computed(() => RELEASE_STATUSES.map((s) => ({ label: statusLabel(s), value: s })))
const bulkStatus = ref<string | null>(null)
const dateDialog = ref(false)
const bulkDate = ref('')
const criteriaDialog = ref(false)
const criteriaFrom = ref<string | null>(null)
const criteriaSources = computed(() => releases.value.map((r) => ({ label: `${r.name} – ${t('releasesView.bulk.criteriaCount', { n: r.exit_criteria.length }, r.exit_criteria.length)}`, value: r.id })))
const scopePicker = ref(false)
const bulkQueryDialog = ref(false)
const bulkQuery = ref('')
watch(releases, (list) => {
  const ids = new Set(list.map((r) => r.id))
  selection.value = selection.value.filter((s) => ids.has(s.id))
})
type BulkSet = Partial<Pick<Release, 'status' | 'target_date' | 'exit_criteria'>>
interface BulkResult {
  updated: string[]
  skipped: { id: string; name: string; reason: string }[]
}
async function bulkApply(set: BulkSet, extra: Record<string, unknown> = {}) {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  const path = `/projects/${proj.pid}/releases/bulk`
  // releasing creates baselines and cannot be undone here
  const undoable = Object.keys(set).length > 0 && set.status !== 'released'
  const before = undoable ? valuesBefore(releases.value, ids, Object.keys(set) as (keyof Release & string)[]) : []
  try {
    const res = await api.post<BulkResult>(path, { ids, set, ...extra })
    if (undoable && res.updated.length) {
      recordUndo(
        entry(
          t('releasesView.bulk.undo', { n: ids.length }),
          async () => {
            for (const b of before) await api.patch(`/releases/${b.id}`, b.before)
          },
          () => api.post(path, { ids, set }),
        ),
      )
    }
    if (res.skipped.length)
      notify.failText(
        t('releasesView.bulk.someNoGo', { n: res.skipped.length, updated: res.updated.length }, res.skipped.length),
        res.skipped.map((s) => `${s.name}: ${t(`releasesView.bulk.reason.${s.reason}`)}`).join('\n'),
      )
    else notify.ok(t('releasesView.bulk.updated', { n: res.updated.length }, res.updated.length))
    await load()
    if (selectedId.value && ids.includes(selectedId.value)) await loadReport(selectedId.value)
  } catch (e) {
    notify.fail(e)
  }
}
watch(bulkStatus, (v) => v && bulkApply({ status: v }).then(() => (bulkStatus.value = null)))
function openDate() {
  bulkDate.value = ''
  dateDialog.value = true
}
async function applyDate() {
  dateDialog.value = false
  await bulkApply({ target_date: bulkDate.value })
}
function openCriteria() {
  criteriaFrom.value = null
  criteriaDialog.value = true
}
async function applyCriteria() {
  const src = releases.value.find((r) => r.id === criteriaFrom.value)
  criteriaDialog.value = false
  if (src) await bulkApply({ exit_criteria: src.exit_criteria.map((c) => ({ ...c })) })
}
async function bulkAddScope(items: { id: string }[]) {
  if (items.length) await bulkApply({}, { add_requirement_ids: items.map((i) => i.id) })
}
function openBulkQuery() {
  bulkQuery.value = ''
  bulkQueryDialog.value = true
}
async function applyBulkQuery() {
  bulkQueryDialog.value = false
  if (bulkQuery.value.trim()) await bulkApply({}, { query: bulkQuery.value.trim() })
}
function bulkDelete() {
  const ids = [...selectedIds.value]
  notify.confirmDanger(t('releasesView.bulk.confirmDelete', { n: ids.length }, ids.length), async () => {
    try {
      await api.del(`/projects/${proj.pid}/releases/bulk`, { ids })
      notify.ok(t('releasesView.bulk.deleted', { n: ids.length }, ids.length))
      selection.value = []
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// requirements and runs of the project (scope names, runs of a release)
const requirements = ref<Requirement[]>([])
const runs = ref<TestRun[]>([])
const reqById = computed(() => new Map(requirements.value.map((r) => [r.id, r])))
async function loadRequirements() {
  try {
    requirements.value = asItems(await api.get<{ items: Requirement[] }>(`/projects/${proj.pid}/requirements`))
  } catch {
    requirements.value = []
  }
}
async function loadRuns() {
  if (!auth.can(P.runRead)) return
  try {
    runs.value = asItems(await api.get<{ items: TestRun[] }>(`/projects/${proj.pid}/runs`))
  } catch {
    runs.value = []
  }
}
onMounted(() => Promise.all([load(), loadRequirements(), loadRuns(), variants.load(), loadCycles()]))

// ---------- selected release ----------
type TabName = 'status' | 'scope' | 'runs' | 'criteria'
const selectedId = computed(() => (route.query.r as string) || null)
const tab = ref<TabName>((route.query.tab as TabName) || 'status')
const detail = computed(() => releases.value.find((r) => r.id === selectedId.value) ?? null)
const report = computed(() => (selectedId.value ? reports.value[selectedId.value] : null))
function open(r: Release, to: TabName = 'status') {
  tab.value = to
  router.replace({ query: { ...route.query, r: r.id, tab: to } })
}
function closeDetail() {
  router.replace({ query: { ...route.query, r: undefined, tab: undefined } })
}
watch(tab, (v) => {
  if (selectedId.value && route.query.tab !== v) router.replace({ query: { ...route.query, tab: v } })
})
function replaceRelease(r: Release) {
  releases.value = releases.value.map((x) => (x.id === r.id ? r : x))
}
async function refreshSelected() {
  if (!selectedId.value) return
  try {
    replaceRelease(await api.get<Release>(`/releases/${selectedId.value}`))
  } catch (e) {
    notify.fail(e)
  }
  await loadReport(selectedId.value)
}

const counts = computed(() => criteriaCounts(report.value?.criteria))
function goTag(r: Release) {
  if (r.status === 'released') return { value: t('releasesView.releasedTag'), severity: 'success', icon: 'pi pi-check-circle' }
  const rep = reports.value[r.id]
  if (rep === undefined) return null
  if (rep === null) return { value: t('releasesView.unknown'), severity: 'secondary', icon: 'pi pi-question-circle' }
  return rep.go ? { value: t('releasesView.go'), severity: 'success', icon: 'pi pi-check' } : { value: t('releasesView.noGo'), severity: 'danger', icon: 'pi pi-times' }
}
const summarySegments = computed(() => {
  const s = report.value?.summary ?? {}
  return { passed: s.passed ?? 0, failed: s.failed ?? 0, not_run: s.not_run ?? 0 }
})

// ---------- create / edit ----------
const dialog = ref(false)
const editing = ref<Release | null>(null)
const form = ref({ name: '', version: '', description: '', status: 'planned', target_date: '' })
const formError = ref('')
const busy = ref(false)
const statusOptions = computed(() =>
  RELEASE_STATUSES.filter((s) => s !== 'released' || editing.value?.status === 'released').map((s) => ({ label: statusLabel(s), value: s })),
)
function openCreate() {
  editing.value = null
  form.value = { name: '', version: '', description: '', status: 'planned', target_date: '' }
  formError.value = ''
  dialog.value = true
}
function openEdit(r: Release) {
  editing.value = r
  form.value = { name: r.name, version: r.version, description: r.description, status: r.status, target_date: r.target_date }
  formError.value = ''
  dialog.value = true
}
async function saveRelease() {
  busy.value = true
  formError.value = ''
  const body = { ...form.value, name: form.value.name.trim(), version: form.value.version.trim() }
  try {
    if (editing.value) {
      const r = await api.patch<Release>(`/releases/${editing.value.id}`, body)
      replaceRelease(r)
      await loadReport(r.id)
      notify.ok(t('releasesView.updated'), r.name)
    } else {
      const r = await api.post<Release>(`/projects/${proj.pid}/releases`, body)
      notify.ok(t('releasesView.created'), r?.name)
      await load()
      if (r?.id) open(r, 'scope')
    }
    void rememberValues(prefs.prefs, proj.pid, savedEntries({ 'release.name': body.name }, { 'release.name': editing.value?.name }))
    dialog.value = false
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
function remove(r: Release) {
  notify.confirmDanger(t('releasesView.confirmDelete', { name: r.name }), async () => {
    try {
      await api.del(`/releases/${r.id}`)
      notify.ok(t('releasesView.deleted'))
      if (selectedId.value === r.id) closeDetail()
      await load()
      loadRuns()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- scope ----------
const picker = ref(false)
const scopeQuery = ref('')
const scopeQueryError = ref<unknown>(null)
const scopeBusy = ref(false)
const scope = computed(() =>
  (detail.value?.requirement_ids ?? []).map((id) => reqById.value.get(id) ?? ({ id, key: '', title: t('releasesView.unknownRequirement') } as Requirement)),
)
async function changeScope(body: { add?: string[]; remove?: string[]; query?: string }) {
  if (!detail.value) return
  scopeBusy.value = true
  try {
    const res = await api.post<{ added: number; removed: number; release: Release }>(`/releases/${detail.value.id}/requirements`, body)
    replaceRelease(res.release)
    if (res.added) notify.ok(t('releasesView.scopeAdded', { count: res.added }, res.added))
    else if (res.removed) notify.ok(t('releasesView.scopeRemoved', { count: res.removed }, res.removed))
    else notify.info(t('releasesView.scopeUnchanged'))
    await loadReport(res.release.id)
    return true
  } catch (e) {
    if (e instanceof ApiError && e.code === 'invalid_query') scopeQueryError.value = e
    else notify.fail(e)
    return false
  } finally {
    scopeBusy.value = false
  }
}
function addPicked(items: { id: string }[]) {
  if (items.length) changeScope({ add: items.map((i) => i.id) })
}
async function addByQuery() {
  scopeQueryError.value = null
  const q = scopeQuery.value.trim()
  if (!q) return
  if (await changeScope({ query: q })) scopeQuery.value = ''
}

// ---------- runs ----------
const releaseRuns = computed(() => (detail.value ? runsOfRelease(runs.value, detail.value.id) : []))

// ---------- exit criteria ----------
const criteria = ref<ExitCriterion[]>([])
const criteriaDirty = ref(false)
const criteriaBusy = ref(false)
watch(
  detail,
  (d) => {
    if (!criteriaDirty.value || !d) criteria.value = (d?.exit_criteria ?? []).map((c) => ({ ...c }))
    if (!d) criteriaDirty.value = false
  },
  { immediate: true },
)
watch(selectedId, () => {
  criteriaDirty.value = false
  criteria.value = (detail.value?.exit_criteria ?? []).map((c) => ({ ...c }))
})
const typeOptions = computed(() => CRITERION_TYPES.map((v) => ({ label: t(`releasesView.criterion.${v}.name`), value: v })))
const severityOptions = computed(() => [{ label: t('releasesView.anySeverity'), value: '' }, ...DEFECT_SEVERITIES.map((v) => ({ label: t(`releasesView.severity.${v}`), value: v }))])
const ruleLevelOptions = computed(() => RULE_LEVELS.map((v) => ({ label: t(`releasesView.ruleLevel.${v}`), value: v })))
const riskLevelOptions = computed(() => [{ label: t('releasesView.anyLevel'), value: '' }, ...RISK_LEVELS.map((v) => ({ label: t(`risksView.level.${v}`), value: v }))])
const thresholdOptions = computed(() => [{ label: t('residual.projectThreshold'), value: '' }, ...RISK_LEVELS.map((v) => ({ label: t(`risksView.level.${v}`), value: v }))])
function touch() {
  criteriaDirty.value = true
}
function setType(i: number, type: string) {
  criteria.value[i] = changeCriterionType(criteria.value[i], type)
  touch()
}
function addCriterion() {
  const used = new Set(criteria.value.map((c) => c.type))
  criteria.value.push(newCriterion(CRITERION_TYPES.find((x) => !used.has(x)) ?? 'coverage'))
  touch()
}
function removeCriterion(i: number) {
  criteria.value.splice(i, 1)
  touch()
}
function resetCriteria() {
  criteriaDirty.value = false
  criteria.value = (detail.value?.exit_criteria ?? []).map((c) => ({ ...c }))
}
async function saveCriteria() {
  if (!detail.value) return
  criteriaBusy.value = true
  try {
    const r = await api.patch<Release>(`/releases/${detail.value.id}`, { exit_criteria: cleanCriteria(criteria.value) })
    criteriaDirty.value = false
    replaceRelease(r)
    await loadReport(r.id)
    notify.ok(t('releasesView.criteriaSaved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    criteriaBusy.value = false
  }
}

// ---------- mark released ----------
const releaseDialog = ref(false)
const releaseNote = ref('')
const releaseBusy = ref(false)
const releaseError = ref('')
/** the report the dialog shows (refreshed when the server answers no-go) */
const dialogReport = ref<ReleaseStatusReport | null>(null)
const dialogNoGo = computed(() => !!dialogReport.value && !dialogReport.value.go)
const dialogUnmet = computed(() => (dialogReport.value?.criteria ?? []).filter((c) => !c.met))
async function openRelease() {
  if (!detail.value) return
  releaseNote.value = ''
  releaseError.value = ''
  await loadReport(detail.value.id)
  dialogReport.value = report.value ?? null
  releaseDialog.value = true
}
async function markReleased() {
  if (!detail.value) return
  releaseBusy.value = true
  releaseError.value = ''
  const override = dialogNoGo.value
  try {
    const r = await api.patch<Release>(`/releases/${detail.value.id}`, override ? { status: 'released', override: true, release_note: releaseNote.value.trim() } : { status: 'released' })
    replaceRelease(r)
    releaseDialog.value = false
    notify.ok(t('releasesView.released'), t('releasesView.releasedDetail', { name: releaseBaselineName(r) }))
    await Promise.all([loadReport(r.id), loadBaseline()])
  } catch (e) {
    if (e instanceof ApiError && e.status === 409 && e.code === 'no_go') {
      const body = e.body as { status?: ReleaseStatusReport } | undefined
      if (body?.status) dialogReport.value = body.status
      releaseError.value = t('releasesView.noGoNow')
    } else releaseError.value = errorMessage(e)
  } finally {
    releaseBusy.value = false
  }
}
const noteTooShort = computed(() => dialogNoGo.value && releaseNote.value.trim().length < MIN_RELEASE_NOTE)

// the baseline created when the release was marked released
const baseline = ref<Baseline | null>(null)
async function loadBaseline() {
  baseline.value = null
  const d = detail.value
  if (!d || d.status !== 'released' || !auth.can(P.requirementRead)) return
  try {
    const name = releaseBaselineName(d)
    const list = asItems(await api.get<{ items: Baseline[] }>(`/projects/${proj.pid}/baselines`)).filter((b) => b.name === name)
    baseline.value = list.sort((a, b) => b.created_at.localeCompare(a.created_at))[0] ?? null
  } catch {
    baseline.value = null
  }
}
watch(() => [detail.value?.id, detail.value?.status], loadBaseline, { immediate: true })

const itemKindLabel = (kind: string) => t(`releasesView.kind.${kind === 'test' || kind === 'risk' || kind === 'defect' || kind === 'trace_rule' ? kind : 'requirement'}`)
/** note of a blocking item: a risk level, a severity, a result or a requirement status */
function itemNote(c: CriterionResult, note?: string) {
  if (!note) return ''
  if (c.type === 'risks_mitigated' || c.type === 'residual_risk') return t(`risksView.level.${note}`)
  if (c.type === 'no_open_defects') return t(`releasesView.severity.${note}`)
  if (c.type === 'trace_rules') return note // the names of the rules the item breaks
  if (c.type === 'no_blocking_defects') return note // the blocking defect: key and severity
  if (note === 'stale') return t('correlations.staleResult')
  if (note === 'none') return t('releasesView.noteNoTests')
  return c.type === 'approved' ? enumLabel('status', note) : enumLabel('result', note)
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('releasesView.title') }}</h1>
      <div class="sub">{{ t('releasesView.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="releases-and-risks" />
      <VariantSelect v-if="variants.variants.value.length" v-model="readinessVariant" :variants="variants.variants.value" :aria-label="t('variants.readinessFor')" :placeholder="t('variants.readinessAll')" />
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
      <Button v-if="canWrite" :label="t('releasesView.new')" icon="pi pi-plus" @click="openCreate" />
    </div>
  </div>

  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else class="card">
    <div v-if="selection.length && canWrite" class="bulkbar" role="toolbar" :aria-label="t('releasesView.bulk.toolbar')">
      <b>{{ t('common.selected', { count: selection.length }) }}</b>
      <Select v-model="bulkStatus" :options="bulkStatusOptions" option-label="label" option-value="value" :placeholder="t('releasesView.bulk.setStatus')" :aria-label="t('releasesView.bulk.setStatus')" size="small" style="min-width: 140px" />
      <Button :label="t('releasesView.bulk.targetDate')" icon="pi pi-calendar" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openDate" />
      <Button :label="t('releasesView.bulk.copyCriteria')" icon="pi pi-copy" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openCriteria" />
      <Button :label="t('releasesView.bulk.addRequirements')" icon="pi pi-plus" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="scopePicker = true" />
      <Button :label="t('releasesView.bulk.addByQuery')" icon="pi pi-filter" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openBulkQuery" />
      <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
    </div>
    <DataTable
      v-model:selection="selection"
      v-stack
      :value="releases"
      data-key="id"
      :loading="loading"
      :aria-busy="loading"
      paginator
      :rows="prefs.rows"
      :always-show-paginator="false"
      row-hover
      size="small"
      :row-class="(r: Release) => (r.id === selectedId ? 'p-datatable-row-selected' : '')"
      @row-click="(e) => open(e.data as Release)"
    >
      <Column v-if="canWrite" selection-mode="multiple" style="width: 2.6rem" />
      <Column field="name" :header="t('common.name')" sortable body-class="stack-main">
        <template #body="{ data }">
          <button type="button" class="link-button" style="font-weight: 600" :aria-pressed="data.id === selectedId" @click.stop="open(data as Release)"><i class="pi pi-box muted" aria-hidden="true" /> {{ data.name }}</button>
        </template>
      </Column>
      <Column field="version" :header="t('common.version')" sortable style="width: 110px"><template #body="{ data }"><span class="mono small">{{ data.version }}</span></template></Column>
      <Column field="status" :header="t('common.status')" sortable style="width: 120px">
        <template #body="{ data }"><Tag :value="statusLabel(data.status)" :severity="statusSeverity(data.status)" /></template>
      </Column>
      <Column field="target_date" :header="t('releasesView.targetDate')" sortable style="width: 130px">
        <template #body="{ data }"><span class="small">{{ data.target_date ? formatDate(data.target_date) : '—' }}</span></template>
      </Column>
      <Column field="requirement_count" :header="t('releasesView.requirements')" sortable style="width: 120px" />
      <Column field="run_count" :header="t('releasesView.runs')" sortable style="width: 90px" />
      <Column :header="t('releasesView.goNoGo')" style="width: 130px">
        <template #body="{ data }">
          <Tag v-if="goTag(data)" :value="goTag(data)!.value" :severity="goTag(data)!.severity" :icon="goTag(data)!.icon" />
          <span v-else class="muted small">…</span>
        </template>
      </Column>
      <Column v-if="canWrite" style="width: 96px">
        <template #body="{ data }">
          <div class="row nowrap" style="gap: 0">
            <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" v-tooltip="t('common.edit')" :aria-label="t('releasesView.editNamed', { name: data.name })" @click.stop="openEdit(data)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" v-tooltip="t('common.delete')" :aria-label="t('releasesView.deleteNamed', { name: data.name })" @click.stop="remove(data)" />
          </div>
        </template>
      </Column>
      <template #empty>
        <div class="empty"><i class="pi pi-box" />{{ canWrite ? t('releasesView.noneCreate') : t('releasesView.none') }}</div>
      </template>
    </DataTable>
  </div>

  <section v-if="selectedId" id="release-detail" class="card" style="margin-top: 16px" aria-labelledby="rel-detail-title">
    <div v-if="!detail" class="empty">
      <ProgressSpinner v-if="loading" style="width: 32px; height: 32px" :aria-label="t('common.loading')" />
      <span v-else>{{ t('releasesView.notFound') }}</span>
    </div>
    <template v-else>
      <div class="row" style="margin-bottom: 6px">
        <h2 id="rel-detail-title" class="plain-heading"><i class="pi pi-box" aria-hidden="true" /> {{ detail.name }}</h2>
        <span v-if="detail.version" class="mono small">{{ detail.version }}</span>
        <Tag :value="statusLabel(detail.status)" :severity="statusSeverity(detail.status)" />
        <Tag v-if="detail.status !== 'released' && goTag(detail)" :value="goTag(detail)!.value" :severity="goTag(detail)!.severity" :icon="goTag(detail)!.icon" />
        <span v-if="detail.target_date" class="small muted">{{ t('releasesView.targetOn', { date: formatDate(detail.target_date) }) }}</span>
        <span class="grow" />
        <Button v-if="canWrite && canRelease(detail.status)" :label="t('releasesView.markReleased')" icon="pi pi-send" size="small" @click="openRelease" />
        <Button v-if="canWrite" icon="pi pi-pencil" text rounded severity="secondary" :aria-label="t('releasesView.editNamed', { name: detail.name })" v-tooltip="t('common.edit')" @click="openEdit(detail)" />
        <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="refreshSelected" />
        <Button icon="pi pi-times" text rounded severity="secondary" :aria-label="t('common.close')" v-tooltip="t('common.close')" @click="closeDetail" />
      </div>
      <p v-if="detail.description" class="muted" style="margin: 0 0 8px">{{ detail.description }}</p>
      <Message v-if="detail.status === 'released'" severity="success" :closable="false" class="released-msg">
        <div>{{ t('releasesView.releasedAt', { date: detail.released_at ? formatDateTime(detail.released_at) : '—', user: detail.released_by || '—' }) }}</div>
        <div v-if="detail.release_note">{{ t('releasesView.releaseNoteShown', { note: detail.release_note }) }}</div>
        <div v-if="baseline">
          <i18n-t keypath="releasesView.baselineLink" tag="span">
            <template #name><RouterLink :to="{ name: 'baselines', params: { projectId: proj.pid }, query: { b: baseline.id } }">{{ baseline.name }}</RouterLink></template>
          </i18n-t>
        </div>
      </Message>

      <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs" style="margin-top: 4px" scrollable>
        <TabList>
          <Tab value="status"><i class="pi pi-flag" aria-hidden="true" /> {{ t('releasesView.tabStatus') }}</Tab>
          <Tab value="scope"><i class="pi pi-list-check" aria-hidden="true" /> {{ t('releasesView.tabScope') }} <span class="count">{{ detail.requirement_count }}</span></Tab>
          <Tab value="runs"><i class="pi pi-play-circle" aria-hidden="true" /> {{ t('releasesView.tabRuns') }} <span class="count">{{ detail.run_count }}</span></Tab>
          <Tab value="criteria"><i class="pi pi-sliders-h" aria-hidden="true" /> {{ t('releasesView.tabCriteria') }} <span class="count">{{ detail.exit_criteria.length }}</span></Tab>
        </TabList>
      </Tabs>

      <!-- status -->
      <div v-if="tab === 'status'" v-bind="panel('status')">
        <div v-if="report === undefined" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
        <Message v-else-if="report === null" severity="warn" :closable="false">{{ t('releasesView.statusUnavailable') }}</Message>
        <template v-else>
          <div class="row" style="margin: 10px 0" role="status">
            <Tag :value="report.go ? t('releasesView.go') : t('releasesView.noGo')" :severity="report.go ? 'success' : 'danger'" :icon="report.go ? 'pi pi-check' : 'pi pi-times'" class="go-tag" />
            <span class="small">{{ t('releasesView.criteriaMet', { met: counts.met, total: report.criteria.length }) }}</span>
            <span class="small muted">{{ t('releasesView.summaryLine', { requirements: t('counts.requirements', report.summary.requirements ?? 0), covered: report.summary.covered ?? 0, runs: t('counts.runs', report.summary.runs ?? 0), executed: report.summary.executed ?? 0, executions: report.summary.executions ?? 0 }) }}</span>
          </div>
          <div v-if="report.summary.requirements" class="row" style="margin-bottom: 12px">
            <Tag :value="t('releasesView.sumPassed', { count: summarySegments.passed })" severity="success" />
            <Tag :value="t('releasesView.sumFailed', { count: summarySegments.failed })" severity="danger" />
            <Tag :value="t('releasesView.sumNotRun', { count: summarySegments.not_run })" severity="secondary" />
          </div>
          <div v-if="!report.criteria.length" class="muted small">{{ t('releasesView.noCriteria') }}</div>
          <ul class="criteria-list">
            <li v-for="(c, i) in report.criteria" :key="i" class="criterion" :class="c.met ? 'met' : 'unmet'">
              <div class="row criterion-head">
                <i :class="c.met ? 'pi pi-check-circle' : 'pi pi-times-circle'" aria-hidden="true" />
                <span class="sr-only">{{ c.met ? t('releasesView.met') : t('releasesView.notMet') }}:</span>
                <span class="grow">{{ describe(c) }}</span>
                <span v-if="c.available" class="small mono values">
                  {{ t('releasesView.actualTarget', { actual: resultValues(c).actual, target: resultValues(c).target }) }}
                  <span v-if="c.total" class="muted"> · {{ t('releasesView.ofTotal', { total: c.total }) }}</span>
                </span>
                <span v-else class="small muted">{{ t('releasesView.notAvailable') }}</span>
              </div>
              <ul v-if="c.items?.length" class="blocking" :aria-label="t('releasesView.blockingOf', { criterion: describe(c) })">
                <li v-for="it in c.items" :key="it.kind + it.id" class="row nowrap">
                  <span class="sr-only">{{ itemKindLabel(it.kind) }}</span>
                  <ItemRef :to="releaseItemRoute(it, proj.pid)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
                  <Tag v-if="(c.type === 'risks_mitigated' || c.type === 'residual_risk') && it.note" :value="itemNote(c, it.note)" :severity="riskLevelSeverity(it.note)" />
                  <span v-else-if="it.note" class="small muted">{{ itemNote(c, it.note) }}</span>
                </li>
              </ul>
              <ul v-if="c.waived?.length || c.accepted?.length" class="blocking waived" :aria-label="t('waivers.acceptedItemsOf', { criterion: describe(c) })">
                <li v-for="it in [...(c.waived ?? []), ...(c.accepted ?? [])]" :key="'w' + it.kind + it.id" class="row nowrap">
                  <span class="sr-only">{{ itemKindLabel(it.kind) }}</span>
                  <ItemRef :to="releaseItemRoute(it, proj.pid)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
                  <Tag v-if="it.kind === 'risk'" :value="t('waivers.riskAccepted')" severity="contrast" icon="pi pi-pen-to-square" />
                  <Tag v-else :value="t('waivers.waivedBy', { keys: it.note || '' })" severity="contrast" icon="pi pi-verified" />
                </li>
              </ul>
            </li>
          </ul>
          <section class="rel-waivers" :aria-labelledby="`rel-waivers-${detail.id}`">
            <div class="row">
              <h3 :id="`rel-waivers-${detail.id}`" class="plain-heading small grow"><i class="pi pi-verified" aria-hidden="true" /> {{ t('waivers.inForceForRelease', { count: report.waivers?.length ?? 0 }, report.waivers?.length ?? 0) }}</h3>
              <RouterLink v-if="auth.can(P.requirementWrite)" :to="{ name: 'waivers', params: { projectId: proj.pid }, query: { new: '1', release: detail.id } }" class="small">{{ t('waivers.request') }}</RouterLink>
              <RouterLink :to="{ name: 'waivers', params: { projectId: proj.pid } }" class="small">{{ t('waivers.all') }}</RouterLink>
            </div>
            <div v-if="!report.waivers?.length" class="small muted">{{ t('waivers.noneForRelease') }}</div>
            <div v-for="w in report.waivers ?? []" :key="w.id" class="row nowrap" style="padding: 3px 0">
              <ItemRef :to="{ name: 'waivers', params: { projectId: proj.pid }, query: { waiver: w.id } }" :item-key="w.key" :title="w.title" title-class="ellipsis" grow />
              <span class="small muted">{{ w.release_id ? w.release_name : t('waivers.wholeProject') }}</span>
              <span class="small muted">{{ w.requirements.map((r) => r.key).join(', ') }}</span>
            </div>
            <h3 class="plain-heading small" style="margin-top: 10px">
              <i class="pi pi-shield" aria-hidden="true" /> {{ t('residual.aboveThresholdRelease', { count: report.residual_risks?.length ?? 0, threshold: t(`risksView.level.${report.risk_threshold || 'medium'}`) }, report.residual_risks?.length ?? 0) }}
            </h3>
            <div v-for="it in report.residual_risks ?? []" :key="'r' + it.id" class="row nowrap" style="padding: 3px 0">
              <ItemRef :to="releaseItemRoute(it, proj.pid)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
              <Tag :value="t(`risksView.level.${(it.note || '').split(',')[0]}`)" :severity="riskLevelSeverity((it.note || '').split(',')[0])" />
              <Tag v-if="(it.note || '').includes('accepted')" :value="t('waivers.riskAccepted')" severity="contrast" icon="pi pi-pen-to-square" />
            </div>
          </section>
        </template>
      </div>

      <!-- scope -->
      <div v-else-if="tab === 'scope'" v-bind="panel('scope')">
        <div v-if="canWrite" class="filterbar" style="margin-top: 10px">
          <Button :label="t('releasesView.addRequirements')" icon="pi pi-search" severity="secondary" outlined :loading="scopeBusy" @click="picker = true" />
          <QueryInput v-model="scopeQuery" scope="requirement" :error="scopeQueryError" :placeholder="t('releasesView.queryPlaceholder', { example: 'tag:checkout risk:high' })" @submit="addByQuery" />
          <Button :label="t('releasesView.addMatching')" icon="pi pi-plus" severity="secondary" :disabled="!scopeQuery.trim()" :loading="scopeBusy" @click="addByQuery" />
        </div>
        <DataTable v-stack :value="scope" data-key="id" size="small" paginator :rows="prefs.rowsFor(50)" :always-show-paginator="false" row-hover>
          <Column field="key" :header="t('common.key')" sortable style="width: 150px">
            <template #body="{ data }"><RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: data.id } }" class="key">{{ data.key || '—' }}</RouterLink></template>
          </Column>
          <Column field="title" :header="t('common.title')" sortable body-class="stack-main">
            <template #body="{ data }"><RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: data.id } }" class="item-ref" tabindex="-1"><span class="item-ref-title">{{ data.title }}</span></RouterLink></template>
          </Column>
          <Column field="status" :header="t('common.status')" style="width: 120px"><template #body="{ data }"><StatusTag :value="data.status" /></template></Column>
          <Column field="coverage" :header="t('workspace.colCoverage')" style="width: 120px"><template #body="{ data }"><StatusTag :value="data.coverage" kind="coverage" /></template></Column>
          <Column field="risk_level" :header="t('risksView.colRisk')" style="width: 110px">
            <template #body="{ data }"><Tag v-if="data.risk_level" :value="t(`risksView.level.${data.risk_level}`)" :severity="riskLevelSeverity(data.risk_level)" /><span v-else class="muted small">—</span></template>
          </Column>
          <Column v-if="canWrite" style="width: 60px">
            <template #body="{ data }">
              <Button icon="pi pi-times" text rounded size="small" severity="secondary" v-tooltip="t('common.remove')" :aria-label="t('common.removeItem', { key: data.key })" :disabled="scopeBusy" @click="changeScope({ remove: [data.id] })" />
            </template>
          </Column>
          <template #empty><div class="empty">{{ t('releasesView.noScope') }}</div></template>
        </DataTable>
      </div>

      <!-- runs -->
      <div v-else-if="tab === 'runs'" v-bind="panel('runs')">
        <p class="small muted">{{ t('releasesView.runsHint') }}</p>
        <DataTable v-stack :value="releaseRuns" data-key="id" size="small" row-hover sort-field="created_at" :sort-order="-1">
          <Column field="name" :header="t('common.name')" sortable body-class="stack-main">
            <template #body="{ data }"><RouterLink :to="{ name: 'run', params: { projectId: proj.pid, runId: data.id } }" class="project-link">{{ data.name }}</RouterLink></template>
          </Column>
          <Column field="status" :header="t('common.status')" style="width: 130px"><template #body="{ data }"><StatusTag :value="data.status" /></template></Column>
          <Column field="environment" :header="t('common.environment')" />
          <Column field="build" :header="t('common.build')"><template #body="{ data }"><span class="mono">{{ data.build }}</span></template></Column>
          <Column :header="t('runs.progress')" style="min-width: 200px"><template #body="{ data }"><SummaryBar :summary="data.summary" show-text /></template></Column>
          <Column field="created_at" :header="t('common.created')" sortable style="width: 150px"><template #body="{ data }"><span class="small muted">{{ formatDateTime(data.created_at) }}</span></template></Column>
          <template #empty><div class="empty">{{ t('releasesView.noRuns') }}</div></template>
        </DataTable>
      </div>

      <!-- exit criteria -->
      <div v-else v-bind="panel('criteria')">
        <p class="small muted">{{ t('releasesView.criteriaHint') }}</p>
        <div v-if="!criteria.length" class="muted small" style="margin-bottom: 8px">{{ t('releasesView.noCriteria') }}</div>
        <div v-for="(c, i) in criteria" :key="i" class="crit-row">
          <span class="step-dot" style="background: var(--p-primary-color)" aria-hidden="true">{{ i + 1 }}</span>
          <Select :model-value="c.type" :options="typeOptions" option-label="label" option-value="value" class="crit-type" :disabled="!canWrite" :aria-label="t('releasesView.criterionTypeN', { n: i + 1 })" @update:model-value="(v: string) => setType(i, v)" />
          <InputNumber
            v-if="criterionParam(c.type) === 'threshold'"
            v-model="c.threshold"
            :min="1"
            :max="100"
            :max-fraction-digits="1"
            suffix=" %"
            class="crit-param"
            :disabled="!canWrite"
            :aria-label="t('releasesView.thresholdN', { n: i + 1 })"
            @update:model-value="touch"
          />
          <Select
            v-else-if="criterionParam(c.type) === 'severity'"
            v-model="c.level"
            :options="severityOptions"
            option-label="label"
            option-value="value"
            class="crit-param"
            :disabled="!canWrite"
            :aria-label="t('releasesView.severityN', { n: i + 1 })"
            @update:model-value="touch"
          />
          <Select
            v-else-if="criterionParam(c.type) === 'riskLevel'"
            v-model="c.level"
            :options="c.type === 'residual_risk' ? thresholdOptions : riskLevelOptions"
            option-label="label"
            option-value="value"
            class="crit-param"
            :disabled="!canWrite"
            :aria-label="t('releasesView.riskLevelN', { n: i + 1 })"
            @update:model-value="touch"
          />
          <Select
            v-else-if="criterionParam(c.type) === 'ruleSeverity'"
            v-model="c.level"
            :options="ruleLevelOptions"
            option-label="label"
            option-value="value"
            class="crit-param"
            :disabled="!canWrite"
            :aria-label="t('releasesView.ruleLevelN', { n: i + 1 })"
            @update:model-value="touch"
          />
          <span v-else class="crit-param small muted">{{ t('releasesView.noParameter') }}</span>
          <Select
            v-if="c.type === 'cycle_executed'"
            v-model="c.cycle_id"
            :options="cycleOptions"
            option-label="label"
            option-value="value"
            filter
            class="crit-param"
            :disabled="!canWrite"
            :placeholder="t('releasesView.chooseCycle')"
            :aria-label="t('releasesView.cycleN', { n: i + 1 })"
            @update:model-value="touch"
          />
          <span class="small muted crit-desc">{{ describe(c) }}</span>
          <Button v-if="canWrite" icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('releasesView.removeCriterionN', { n: i + 1 })" v-tooltip="t('common.remove')" @click="removeCriterion(i)" />
        </div>
        <div v-if="canWrite" class="row" style="margin-top: 10px">
          <Button :label="t('releasesView.addCriterion')" icon="pi pi-plus" text :disabled="criteria.length >= 30" @click="addCriterion" />
          <span class="grow" />
          <Button v-if="criteriaDirty" :label="t('common.discard')" text severity="secondary" @click="resetCriteria" />
          <Button :label="t('releasesView.saveCriteria')" icon="pi pi-check" :disabled="!criteriaDirty" :loading="criteriaBusy" @click="saveCriteria" />
        </div>
      </div>
    </template>
  </section>

  <!-- create / edit -->
  <Dialog v-model:visible="dialog" :header="editing ? t('releasesView.editHeader') : t('releasesView.new')" modal style="width: min(620px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="saveRelease">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="form-grid">
        <div v-field class="field"><label for="rel-name">{{ t('common.name') }}</label><SuggestInput id="rel-name" v-model="form.name" field="release.name" :project-id="proj.pid" :placeholder="t('releasesView.namePlaceholder')" autofocus /></div>
        <div v-field class="field"><label for="rel-version">{{ t('common.version') }}</label><InputText id="rel-version" v-model="form.version" :placeholder="t('releasesView.versionPlaceholder')" /></div>
        <div v-field class="field">
          <label for="rel-status">{{ t('common.status') }}</label>
          <Select v-model="form.status" input-id="rel-status" :options="statusOptions" option-label="label" option-value="value" :disabled="editing?.status === 'released'" />
        </div>
        <div v-field class="field"><label for="rel-date">{{ t('releasesView.targetDate') }}</label><InputText id="rel-date" v-model="form.target_date" type="date" /></div>
        <div v-field class="field full"><label for="rel-desc">{{ t('common.description') }}</label><Textarea id="rel-desc" v-model="form.description" rows="3" auto-resize /></div>
      </div>
      <div v-if="!editing" class="small muted">{{ t('releasesView.defaultsHint') }}</div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="editing ? t('common.save') : t('releasesView.create')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>

  <!-- mark released -->
  <Dialog v-model:visible="releaseDialog" :header="t('releasesView.releaseHeader', { name: detail?.name ?? '' })" modal style="width: min(640px, 96vw)">
    <div class="grid" style="gap: 12px">
      <Message v-if="releaseError" severity="error" :closable="false">{{ releaseError }}</Message>
      <template v-if="dialogNoGo">
        <Message severity="warn" :closable="false">{{ t('releasesView.noGoIntro', { count: dialogUnmet.length }, dialogUnmet.length) }}</Message>
        <ul class="unmet-list">
          <li v-for="(c, i) in dialogUnmet" :key="i"><i class="pi pi-times-circle" aria-hidden="true" /> {{ describe(c) }} <span v-if="c.available" class="small mono muted">({{ t('releasesView.actualTarget', { actual: resultValues(c).actual, target: resultValues(c).target }) }})</span></li>
        </ul>
        <div v-field class="field">
          <label for="rel-note">{{ t('releasesView.overrideNote') }}</label>
          <Textarea id="rel-note" v-model="releaseNote" rows="3" auto-resize :invalid="!!releaseNote && noteTooShort" aria-describedby="rel-note-hint" />
          <span id="rel-note-hint" class="small muted">{{ t('releasesView.overrideHint', { min: MIN_RELEASE_NOTE }) }}</span>
        </div>
      </template>
      <p v-else style="margin: 0">{{ t('releasesView.goIntro') }}</p>
      <p class="small muted" style="margin: 0">{{ t('releasesView.baselineHint', { name: detail ? releaseBaselineName(detail) : '' }) }}</p>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="releaseDialog = false" />
      <Button
        :label="dialogNoGo ? t('releasesView.releaseAnyway') : t('releasesView.markReleased')"
        :icon="dialogNoGo ? 'pi pi-exclamation-triangle' : 'pi pi-send'"
        :severity="dialogNoGo ? 'danger' : undefined"
        :loading="releaseBusy"
        :disabled="noteTooShort"
        @click="markReleased"
      />
    </template>
  </Dialog>

  <ItemPickerDialog
    v-if="detail"
    v-model:visible="picker"
    kind="requirement"
    :project-id="proj.pid"
    :exclude="detail.requirement_ids ?? []"
    :header="t('releasesView.addRequirements')"
    :confirm-label="t('common.add')"
    @pick="addPicked"
  />
  <Dialog v-model:visible="dateDialog" :header="t('releasesView.bulk.dateHeader', { n: selection.length }, selection.length)" modal style="width: min(420px, 96vw)">
    <label for="bk-rel-date" class="block-label">{{ t('releasesView.targetDate') }}</label>
    <InputText id="bk-rel-date" v-model="bulkDate" type="date" fluid aria-describedby="bk-rel-date-help" />
    <small id="bk-rel-date-help" class="muted">{{ t('releasesView.bulk.emptyClears') }}</small>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="dateDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" @click="applyDate" />
    </template>
  </Dialog>
  <Dialog v-model:visible="criteriaDialog" :header="t('releasesView.bulk.criteriaHeader', { n: selection.length }, selection.length)" modal style="width: min(480px, 96vw)">
    <label for="bk-rel-crit" class="block-label">{{ t('releasesView.bulk.copyFrom') }}</label>
    <Select v-model="criteriaFrom" input-id="bk-rel-crit" :options="criteriaSources" option-label="label" option-value="value" fluid aria-describedby="bk-rel-crit-help" />
    <small id="bk-rel-crit-help" class="muted">{{ t('releasesView.bulk.copyHelp') }}</small>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="criteriaDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!criteriaFrom" @click="applyCriteria" />
    </template>
  </Dialog>
  <Dialog v-model:visible="bulkQueryDialog" :header="t('releasesView.bulk.queryHeader', { n: selection.length }, selection.length)" modal style="width: min(560px, 96vw)">
    <QueryInput v-model="bulkQuery" scope="requirement" :placeholder="t('releasesView.bulk.queryPlaceholder', { example: 'obligation:must tag:payments' })" @submit="applyBulkQuery" />
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="bulkQueryDialog = false" />
      <Button :label="t('common.add')" icon="pi pi-plus" :disabled="!bulkQuery.trim()" @click="applyBulkQuery" />
    </template>
  </Dialog>
  <ItemPickerDialog v-model:visible="scopePicker" kind="requirement" :project-id="proj.pid" :header="t('releasesView.bulk.pickHeader', { n: selection.length }, selection.length)" :confirm-label="t('common.add')" @pick="bulkAddScope" />
</template>

<style scoped>
.block-label {
  display: block;
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 4px;
}
.criteria-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
.criterion { border: 1px solid var(--qtf-border); border-radius: var(--qtf-radius); padding: 8px 10px; }
.criterion.met .criterion-head > i { color: var(--p-green-500); }
.criterion.unmet { border-color: color-mix(in srgb, var(--p-red-500) 45%, var(--qtf-border)); }
.criterion.unmet .criterion-head > i { color: var(--p-red-500); }
.criterion-head { flex-wrap: wrap; }
.criterion-head > .grow { min-width: 12em; }
.values { white-space: nowrap; }
.blocking { list-style: none; margin: 6px 0 0; padding: 0 0 0 24px; display: grid; gap: 2px; max-height: 260px; overflow: auto; }
.blocking li { padding: 2px 0; border-bottom: 1px dashed var(--qtf-border); }
.blocking.waived { border-left: 3px solid var(--qtf-border); margin-left: 24px; padding-left: 8px; }
.rel-waivers { margin-top: 14px; padding-top: 10px; border-top: 1px solid var(--qtf-border); }
.crit-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; padding: 6px 0; border-bottom: 1px solid var(--qtf-border); }
.crit-type { width: 260px; max-width: 100%; }
.crit-param { width: 170px; max-width: 100%; }
.crit-desc { flex: 1; min-width: 160px; }
.unmet-list { margin: 0; padding-left: 4px; list-style: none; display: grid; gap: 4px; }
.unmet-list i { color: var(--p-red-500); }
.released-msg { margin-bottom: 8px; }
@media (max-width: 640px) {
  .crit-type, .crit-param { width: 100%; }
}
</style>
