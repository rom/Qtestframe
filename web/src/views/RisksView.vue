<script setup lang="ts">
// Risk register: likelihood × impact, a 5×5 heat map that filters the
// table, mitigation and the requirements that address each risk.
import ItemRef from '../components/ItemRef.vue'
import { customCellText, customColumns } from '@/lib/customColumns'
import CustomFieldsEditor from '@/components/CustomFieldsEditor.vue'
import { defaultValues, type CustomValues } from '@/lib/customFields'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import StatusTag from '@/components/StatusTag.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { valuesBefore } from '@/lib/bulkRuns'
import { entry, recordUndo } from '@/lib/undoActions'
import { api, asItems, errorMessage } from '@/lib/api'
import { P } from '@/lib/permissions'
import { filterRisks, heatGrid, levelCounts, levelRank, riskLevel, riskLevelSeverity, riskScore, riskStatusSeverity, RISK_LEVELS, RISK_SCALE, RISK_STATUSES, type HeatCell } from '@/lib/risks'
import type { Requirement, Risk } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'
import SuggestInput from '@/components/SuggestInput.vue'
import { rememberValues, savedEntries } from '@/lib/suggest'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const prefs = usePrefsStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()

/** custom fields shown in lists (show_in_list) as columns */
const listedCols = computed(() => customColumns(useProjectStore().customFieldsFor('risk').filter((f) => f.show_in_list)))
const cellOf = (row: { custom_fields?: Record<string, unknown> }, field: string) => customCellText(useProjectStore().customFieldsFor('risk'), row, field, [t('common.yes'), t('common.no')])
const canWrite = computed(() => auth.can(P.requirementWrite))

const levelLabel = (l: string) => (l ? t(`risksView.level.${l}`) : '')
const statusLabel = (s: string) => t(`risksView.status.${s}`)
const likelihoodLabel = (n: number) => t(`risksView.likelihoodScale.${n}`)
const impactLabel = (n: number) => t(`risksView.impactScale.${n}`)

// ---------- data ----------
const risks = ref<Risk[]>([])
const loading = ref(false)
const error = ref('')
const requirements = ref<Requirement[]>([])
const reqById = computed(() => new Map(requirements.value.map((r) => [r.id, r])))

async function load() {
  loading.value = true
  error.value = ''
  try {
    risks.value = asItems(await api.get<{ items: Risk[] }>(`/projects/${proj.pid}/risks`))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
async function loadRequirements() {
  try {
    requirements.value = asItems(await api.get<{ items: Requirement[] }>(`/projects/${proj.pid}/requirements`))
  } catch {
    requirements.value = []
  }
}
onMounted(async () => {
  await Promise.all([load(), loadRequirements()])
  openFromRoute()
})

// ---------- filters and heat map ----------
const text = ref('')
const levels = ref<string[]>([])
const statuses = ref<string[]>([])
const includeClosed = ref(false)
const cell = ref<{ likelihood: number; impact: number } | null>(null)
const grid = computed(() => heatGrid(risks.value, includeClosed.value))
const filtered = computed(() =>
  filterRisks(
    risks.value.filter((r) => includeClosed.value || r.status !== 'closed' || statuses.value.includes('closed')),
    { likelihood: cell.value?.likelihood, impact: cell.value?.impact, levels: levels.value, statuses: statuses.value, text: text.value },
  ),
)
const counts = computed(() => levelCounts(risks.value.filter((r) => r.status !== 'closed')))
const levelOptions = computed(() => RISK_LEVELS.map((v) => ({ label: levelLabel(v), value: v })))
const statusOptions = computed(() => RISK_STATUSES.map((v) => ({ label: statusLabel(v), value: v })))
const scaleOptions = (label: (n: number) => string) => RISK_SCALE.map((n) => ({ label: `${n} – ${label(n)}`, value: n }))

// ---------- bulk ----------
const selection = ref<Risk[]>([])
const selectedIds = computed(() => selection.value.map((k) => k.id))
const bulkStatus = ref<string | null>(null)
const bulkLikelihood = ref<number | null>(null)
const bulkImpact = ref<number | null>(null)
const ownerDialog = ref(false)
const bulkOwner = ref('')
const bulkCategory = ref('')
const bulkOwnerOn = ref(true)
const bulkCategoryOn = ref(false)
const bulkPicker = ref(false)
// only rows that are shown stay selected, so a bulk action never changes hidden risks
watch(filtered, (list) => {
  const ids = new Set(list.map((k) => k.id))
  selection.value = selection.value.filter((s) => ids.has(s.id))
})
type RiskSet = Partial<Pick<Risk, 'status' | 'likelihood' | 'impact' | 'owner' | 'category'>>
async function bulkSet(set: RiskSet, extra: Record<string, unknown> = {}) {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  const before = valuesBefore(risks.value, ids, Object.keys(set) as (keyof Risk & string)[])
  const path = `/projects/${proj.pid}/risks/bulk`
  try {
    await api.post(path, { ids, set, ...extra })
    if (before.length && Object.keys(set).length) {
      recordUndo(
        entry(
          t('risksView.bulk.undo', { n: ids.length }),
          async () => {
            for (const b of before) await api.patch(`/risks/${b.id}`, b.before)
          },
          () => api.post(path, { ids, set }),
        ),
      )
    }
    notify.ok(t('risksView.bulk.updated', { n: ids.length }, ids.length))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
watch(bulkStatus, (v) => v && bulkSet({ status: v }).then(() => (bulkStatus.value = null)))
watch(bulkLikelihood, (v) => v && bulkSet({ likelihood: v }).then(() => (bulkLikelihood.value = null)))
watch(bulkImpact, (v) => v && bulkSet({ impact: v }).then(() => (bulkImpact.value = null)))
function openOwner() {
  bulkOwner.value = ''
  bulkCategory.value = ''
  bulkOwnerOn.value = true
  bulkCategoryOn.value = false
  ownerDialog.value = true
}
async function applyOwner() {
  const set: RiskSet = {}
  if (bulkOwnerOn.value) set.owner = bulkOwner.value.trim()
  if (bulkCategoryOn.value) set.category = bulkCategory.value.trim()
  ownerDialog.value = false
  await bulkSet(set)
}
async function bulkLink(items: { id: string }[]) {
  if (!items.length) return
  await bulkSet({}, { add_requirement_ids: items.map((i) => i.id) })
}
function bulkDelete() {
  const ids = [...selectedIds.value]
  notify.confirmDanger(t('risksView.bulk.confirmDelete', { n: ids.length }, ids.length), async () => {
    try {
      await api.del(`/projects/${proj.pid}/risks/bulk`, { ids })
      notify.ok(t('risksView.bulk.deleted', { n: ids.length }, ids.length))
      selection.value = []
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
const isCell = (c: HeatCell) => cell.value?.likelihood === c.likelihood && cell.value?.impact === c.impact
function toggleCell(c: HeatCell) {
  cell.value = isCell(c) ? null : { likelihood: c.likelihood, impact: c.impact }
}
function cellLabel(c: HeatCell) {
  return t('risksView.cellLabel', { likelihood: c.likelihood, impact: c.impact, score: c.score, level: levelLabel(c.level), count: t('risksView.riskCount', { count: c.count }, c.count) })
}
const hasFilter = computed(() => !!cell.value || !!levels.value.length || !!statuses.value.length || !!text.value.trim())
function clearFilters() {
  cell.value = null
  levels.value = []
  statuses.value = []
  text.value = ''
}

// ---------- create / edit ----------
interface Form {
  title: string
  description: string
  category: string
  likelihood: number
  impact: number
  status: string
  owner: string
  mitigation: string
  requirement_ids: string[]
  custom_fields: CustomValues
}
const blank = (): Form => ({ title: '', description: '', category: '', likelihood: 3, impact: 3, status: 'identified', owner: '', mitigation: '', requirement_ids: [], custom_fields: defaultValues(proj.customFieldsFor('risk')) })
const dialog = ref(false)
const editing = ref<Risk | null>(null)
const form = ref<Form>(blank())
const formError = ref('')
const busy = ref(false)
const picker = ref(false)
const formScore = computed(() => riskScore(form.value.likelihood, form.value.impact))
const formLevel = computed(() => riskLevel(form.value.likelihood, form.value.impact))
const linked = computed(() => form.value.requirement_ids.map((id) => reqById.value.get(id) ?? ({ id, key: id.slice(0, 8), title: '' } as Requirement)))

function openCreate() {
  editing.value = null
  form.value = blank()
  formError.value = ''
  dialog.value = true
}
function openEdit(r: Risk) {
  editing.value = r
  form.value = {
    title: r.title,
    description: r.description,
    category: r.category,
    likelihood: r.likelihood,
    impact: r.impact,
    status: r.status,
    owner: r.owner,
    mitigation: r.mitigation,
    requirement_ids: [...(r.requirement_ids ?? [])],
    custom_fields: { ...(r.custom_fields ?? {}) } as CustomValues,
  }
  formError.value = ''
  dialog.value = true
  if (route.query.risk !== r.id) router.replace({ query: { ...route.query, risk: r.id } })
}
function openFromRoute() {
  const id = route.query.risk as string | undefined
  if (!id) return
  const r = risks.value.find((x) => x.id === id)
  if (r && (!dialog.value || editing.value?.id !== id)) openEdit(r)
}
watch(() => route.query.risk, openFromRoute)
watch(dialog, (v) => {
  if (!v && route.query.risk) router.replace({ query: { ...route.query, risk: undefined } })
})
function addLinked(items: { id: string }[]) {
  const have = new Set(form.value.requirement_ids)
  form.value.requirement_ids = [...form.value.requirement_ids, ...items.map((i) => i.id).filter((id) => !have.has(id))]
}
function unlink(id: string) {
  form.value.requirement_ids = form.value.requirement_ids.filter((x) => x !== id)
}
async function save() {
  busy.value = true
  formError.value = ''
  const { requirement_ids, ...fields } = form.value
  const body = { ...fields, title: fields.title.trim(), category: fields.category.trim(), owner: fields.owner.trim() }
  try {
    if (editing.value) {
      const id = editing.value.id
      await api.patch<Risk>(`/risks/${id}`, body)
      const before = new Set(editing.value.requirement_ids ?? [])
      const after = new Set(requirement_ids)
      const add = requirement_ids.filter((x) => !before.has(x))
      const remove = [...before].filter((x) => !after.has(x))
      if (add.length || remove.length) await api.post<Risk>(`/risks/${id}/requirements`, { add, remove })
      notify.ok(t('risksView.updated'), editing.value.key)
    } else {
      const r = await api.post<Risk>(`/projects/${proj.pid}/risks`, { ...body, requirement_ids })
      notify.ok(t('risksView.created'), r?.key)
    }
    const old = editing.value
    void rememberValues(
      prefs.prefs,
      proj.pid,
      savedEntries({ 'risk.title': body.title, 'risk.mitigation': body.mitigation, 'risk.owner': body.owner }, { 'risk.title': old?.title, 'risk.mitigation': old?.mitigation, 'risk.owner': old?.owner }),
    )
    dialog.value = false
    await load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
function remove(r: Risk) {
  notify.confirmDanger(t('risksView.confirmDelete', { key: r.key, title: r.title }), async () => {
    try {
      await api.del(`/risks/${r.id}`)
      notify.ok(t('risksView.deleted'))
      dialog.value = false
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
const reqKeys = (r: Risk) => (r.requirement_ids ?? []).map((id) => reqById.value.get(id)).filter((x): x is Requirement => !!x)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('risksView.title') }}</h1>
      <div class="sub">{{ t('risksView.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="releases-and-risks" />
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
      <Button v-if="canWrite" :label="t('risksView.new')" icon="pi pi-plus" @click="openCreate" />
    </div>
  </div>

  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <template v-else>
    <div class="risk-top">
      <section class="card heat-card" aria-labelledby="heat-title">
        <h2 id="heat-title">{{ t('risksView.heatMap') }}</h2>
        <div class="heat-wrap">
          <table class="heat">
            <caption class="sr-only">{{ t('risksView.heatCaption') }}</caption>
            <thead>
              <tr>
                <th scope="col" class="corner"><span class="axis">{{ t('risksView.likelihood') }} ↓ / {{ t('risksView.impact') }} →</span></th>
                <th v-for="i in RISK_SCALE" :key="i" scope="col" :title="impactLabel(i)">{{ i }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in grid" :key="row.likelihood">
                <th scope="row" :title="likelihoodLabel(row.likelihood)">{{ row.likelihood }}</th>
                <td v-for="c in row.cells" :key="c.impact">
                  <button type="button" class="heat-cell" :class="['lvl-' + c.level, { active: isCell(c), empty: !c.count }]" :aria-pressed="isCell(c)" :aria-label="cellLabel(c)" :title="cellLabel(c)" @click="toggleCell(c)">
                    {{ c.count || '' }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="row small" style="margin-top: 8px">
          <label class="row nowrap" style="gap: 6px"><Checkbox v-model="includeClosed" binary input-id="risk-closed" /> {{ t('risksView.includeClosed') }}</label>
        </div>
      </section>
      <section class="card level-card" aria-labelledby="levels-title">
        <h2 id="levels-title">{{ t('risksView.openByLevel') }}</h2>
        <ul class="levels">
          <li v-for="l in [...RISK_LEVELS].reverse()" :key="l">
            <button type="button" class="link-button" :aria-pressed="levels.length === 1 && levels[0] === l" @click="levels = levels.length === 1 && levels[0] === l ? [] : [l]">
              <Tag :value="levelLabel(l)" :severity="riskLevelSeverity(l)" />
              <span class="lvl-count">{{ counts[l] }}</span>
            </button>
          </li>
        </ul>
        <p class="small muted" style="margin: 8px 0 0">{{ t('risksView.levelsHint') }}</p>
      </section>
    </div>

    <section class="card" style="margin-top: 16px" aria-labelledby="register-title">
      <h2 id="register-title">{{ t('risksView.register') }}</h2>
      <div class="filterbar">
        <InputText v-model="text" :placeholder="t('risksView.searchPlaceholder')" :aria-label="t('risksView.searchPlaceholder')" style="min-width: 200px" />
        <MultiSelect v-model="levels" :options="levelOptions" option-label="label" option-value="value" :placeholder="t('risksView.colLevel')" :aria-label="t('risksView.colLevel')" show-clear style="min-width: 140px" />
        <MultiSelect v-model="statuses" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('common.status')" :aria-label="t('common.status')" show-clear style="min-width: 140px" />
        <Tag v-if="cell" severity="info" class="cell-chip">
          <span>{{ t('risksView.cellFilter', { likelihood: cell.likelihood, impact: cell.impact }) }}</span>
          <button type="button" class="link-button" :aria-label="t('risksView.clearCell')" @click="cell = null"><i class="pi pi-times" aria-hidden="true" /></button>
        </Tag>
        <Button v-if="hasFilter" :label="t('common.clear')" text size="small" severity="secondary" @click="clearFilters" />
        <span class="small muted" role="status">{{ t('risksView.shown', { count: filtered.length, total: risks.length }) }}</span>
      </div>
      <div v-if="selection.length && canWrite" class="bulkbar" role="toolbar" :aria-label="t('risksView.bulk.toolbar')">
        <b>{{ t('common.selected', { count: selection.length }) }}</b>
        <Select v-model="bulkStatus" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('risksView.bulk.setStatus')" :aria-label="t('risksView.bulk.setStatus')" size="small" style="min-width: 140px" />
        <Select v-model="bulkLikelihood" :options="scaleOptions(likelihoodLabel)" option-label="label" option-value="value" :placeholder="t('risksView.bulk.setLikelihood')" :aria-label="t('risksView.bulk.setLikelihood')" size="small" style="min-width: 150px" />
        <Select v-model="bulkImpact" :options="scaleOptions(impactLabel)" option-label="label" option-value="value" :placeholder="t('risksView.bulk.setImpact')" :aria-label="t('risksView.bulk.setImpact')" size="small" style="min-width: 150px" />
        <Button :label="t('risksView.bulk.ownerCategory')" icon="pi pi-user" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openOwner" />
        <Button :label="t('risksView.bulk.link')" icon="pi pi-link" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="bulkPicker = true" />
        <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
        <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
      </div>
      <DataTable
        v-model:selection="selection"
        v-stack
        :value="filtered"
        data-key="id"
        :loading="loading"
        :aria-busy="loading"
        paginator
        :rows="prefs.rowsFor(25)"
        :always-show-paginator="false"
        row-hover
        size="small"
        sort-field="score"
        :sort-order="-1"
        @row-click="(e) => openEdit(e.data as Risk)"
      >
        <Column v-if="canWrite" selection-mode="multiple" style="width: 2.6rem" />
        <Column field="key" :header="t('common.key')" sortable style="width: 130px"><template #body="{ data }"><button type="button" class="link-button key-link" tabindex="-1" @click.stop="openEdit(data as Risk)"><span class="key">{{ data.key }}</span></button></template></Column>
        <Column field="title" :header="t('common.title')" sortable body-class="stack-main" style="min-width: 200px">
          <template #body="{ data }">
            <button type="button" class="link-button" style="font-weight: 600" :aria-label="t('risksView.openRisk', { key: data.key, title: data.title })" @click.stop="openEdit(data as Risk)">{{ data.title }}</button>
            <div v-if="data.category" class="small muted">{{ data.category }}</div>
          </template>
        </Column>
        <Column field="likelihood" :header="t('risksView.likelihood')" sortable style="width: 110px"><template #body="{ data }"><span :title="likelihoodLabel(data.likelihood)">{{ data.likelihood }}</span></template></Column>
        <Column field="impact" :header="t('risksView.impact')" sortable style="width: 100px"><template #body="{ data }"><span :title="impactLabel(data.impact)">{{ data.impact }}</span></template></Column>
        <Column field="score" :header="t('risksView.score')" sortable style="width: 90px"><template #body="{ data }"><b>{{ data.score }}</b></template></Column>
        <Column field="level" :header="t('risksView.colLevel')" sortable :sort-field="(r: Risk) => String(levelRank(r.level))" style="width: 110px">
          <template #body="{ data }"><Tag :value="levelLabel(data.level)" :severity="riskLevelSeverity(data.level)" /></template>
        </Column>
        <Column field="status" :header="t('common.status')" sortable style="width: 120px">
          <template #body="{ data }"><Tag :value="statusLabel(data.status)" :severity="riskStatusSeverity(data.status)" /></template>
        </Column>
        <Column field="owner" :header="t('common.owner')" sortable><template #body="{ data }"><span class="small">{{ data.owner }}</span></template></Column>
        <Column v-for="c in listedCols" :key="c.field" :header="c.header" sortable :sort-field="c.sortField" :style="c.width ? { width: c.width } : {}">
          <template #body="{ data }"><span class="small">{{ cellOf(data, c.field) }}</span></template>
        </Column>
        <Column :header="t('risksView.requirements')" style="min-width: 160px">
          <template #body="{ data }">
            <span class="row small" style="gap: 4px">
              <RouterLink v-for="q in reqKeys(data)" :key="q.id" :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: q.id } }" class="key" :title="q.title" @click.stop>{{ q.key }}</RouterLink>
              <span v-if="!data.requirement_ids?.length" class="muted">—</span>
            </span>
          </template>
        </Column>
        <Column :header="t('risksView.coveredVerified')" style="width: 140px">
          <template #body="{ data }">
            <span v-if="data.requirement_ids?.length" class="small" :aria-label="t('risksView.coveredVerifiedLong', { covered: data.covered, verified: data.verified, total: data.requirement_ids.length })">
              {{ data.covered }} / {{ data.verified }} <span class="muted">({{ data.requirement_ids.length }})</span>
            </span>
            <span v-else class="muted small">—</span>
          </template>
        </Column>
        <template #empty>
          <div class="empty"><i class="pi pi-shield" />{{ risks.length ? t('risksView.noneMatch') : canWrite ? t('risksView.noneCreate') : t('risksView.none') }}</div>
        </template>
      </DataTable>
    </section>
  </template>

  <Dialog v-model:visible="dialog" :header="editing ? t('risksView.editHeader', { key: editing.key }) : t('risksView.new')" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(720px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="form-grid">
        <div v-field class="field full"><label for="risk-title">{{ t('common.title') }}</label><SuggestInput id="risk-title" v-model="form.title" field="risk.title" :project-id="proj.pid" :disabled="!canWrite" autofocus /></div>
        <div v-field class="field">
          <label for="risk-likelihood">{{ t('risksView.likelihood') }}</label>
          <Select v-model="form.likelihood" input-id="risk-likelihood" :options="scaleOptions(likelihoodLabel)" option-label="label" option-value="value" :disabled="!canWrite" />
        </div>
        <div v-field class="field">
          <label for="risk-impact">{{ t('risksView.impact') }}</label>
          <Select v-model="form.impact" input-id="risk-impact" :options="scaleOptions(impactLabel)" option-label="label" option-value="value" :disabled="!canWrite" />
        </div>
        <div class="field full">
          <span class="field-label">{{ t('risksView.score') }}</span>
          <div class="row" role="status">
            <b>{{ t('risksView.scoreLine', { likelihood: form.likelihood, impact: form.impact, score: formScore }) }}</b>
            <Tag :value="levelLabel(formLevel)" :severity="riskLevelSeverity(formLevel)" />
          </div>
        </div>
        <div v-field class="field">
          <label for="risk-status">{{ t('common.status') }}</label>
          <Select v-model="form.status" input-id="risk-status" :options="statusOptions" option-label="label" option-value="value" :disabled="!canWrite" />
        </div>
        <div v-field class="field"><label for="risk-owner">{{ t('common.owner') }}</label><SuggestInput id="risk-owner" v-model="form.owner" field="risk.owner" :project-id="proj.pid" :disabled="!canWrite" /></div>
        <div v-field class="field"><label for="risk-category">{{ t('common.category') }}</label><InputText id="risk-category" v-model="form.category" :placeholder="t('risksView.categoryPlaceholder')" :disabled="!canWrite" /></div>
        <div v-field class="field full"><label for="risk-desc">{{ t('common.description') }}</label><Textarea id="risk-desc" v-model="form.description" rows="2" auto-resize :disabled="!canWrite" /></div>
        <div v-field class="field full"><label for="risk-mitigation">{{ t('risksView.mitigation') }}</label><SuggestInput id="risk-mitigation" v-model="form.mitigation" field="risk.mitigation" :project-id="proj.pid" multiline rows="3" auto-resize :placeholder="t('risksView.mitigationPlaceholder')" :disabled="!canWrite" /></div>
      </div>
      <div>
        <div class="section-title" style="margin-top: 4px">
          <i class="pi pi-list-check" aria-hidden="true" /> {{ t('risksView.linkedRequirements', { count: linked.length }, linked.length) }}
          <span class="grow" />
          <Button v-if="canWrite" size="small" :label="t('risksView.linkRequirements')" icon="pi pi-plus" text @click="picker = true" />
        </div>
        <div v-if="!linked.length" class="muted small">{{ t('risksView.noLinked') }}</div>
        <div v-for="q in linked" :key="q.id" class="row nowrap" style="padding: 4px 0; border-bottom: 1px solid var(--qtf-border)">
          <ItemRef :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: q.id } }" :item-key="q.key" :title="q.title" title-class="ellipsis" grow />
          <StatusTag :value="q.coverage" kind="coverage" empty="" />
          <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('common.unlinkItem', { key: q.key })" v-tooltip="t('common.unlink')" @click="unlink(q.id)" />
        </div>
      </div>
      <CustomFieldsEditor v-model="form.custom_fields" :fields="proj.customFieldsFor('risk')" :disabled="!canWrite" :project-id="proj.pid" />
      <div class="row" style="justify-content: flex-end">
        <Button v-if="editing && canWrite" :label="t('common.delete')" icon="pi pi-trash" severity="danger" text style="margin-right: auto" @click="remove(editing)" />
        <Button :label="canWrite ? t('common.cancel') : t('common.close')" text severity="secondary" @click="dialog = false" />
        <Button v-if="canWrite" type="submit" :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.title.trim()" />
      </div>
    </form>
  </Dialog>
  <ItemPickerDialog v-model:visible="bulkPicker" kind="requirement" :project-id="proj.pid" :header="t('risksView.bulk.linkHeader', { n: selection.length }, selection.length)" :confirm-label="t('common.link')" @pick="bulkLink" />
  <Dialog v-model:visible="ownerDialog" :header="t('risksView.bulk.ownerHeader', { n: selection.length }, selection.length)" modal style="width: min(440px, 96vw)">
    <div class="bulk-form">
      <div class="row">
        <Checkbox v-model="bulkOwnerOn" binary input-id="bk-risk-owner-on" />
        <label for="bk-risk-owner-on">{{ t('risksView.bulk.setOwner') }}</label>
      </div>
      <InputText v-model="bulkOwner" :disabled="!bulkOwnerOn" :aria-label="t('risksView.bulk.setOwner')" :placeholder="t('risksView.bulk.emptyClears')" />
      <div class="row">
        <Checkbox v-model="bulkCategoryOn" binary input-id="bk-risk-cat-on" />
        <label for="bk-risk-cat-on">{{ t('risksView.bulk.setCategory') }}</label>
      </div>
      <InputText v-model="bulkCategory" :disabled="!bulkCategoryOn" :aria-label="t('risksView.bulk.setCategory')" :placeholder="t('risksView.bulk.emptyClears')" />
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="ownerDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!bulkOwnerOn && !bulkCategoryOn" @click="applyOwner" />
    </template>
  </Dialog>
  <ItemPickerDialog v-model:visible="picker" kind="requirement" :project-id="proj.pid" :exclude="form.requirement_ids" :header="t('risksView.linkRequirements')" :confirm-label="t('common.link')" @pick="addLinked" />
</template>

<style scoped>
.bulk-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.bulk-form .row {
  gap: 8px;
}
.risk-top { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); gap: 16px; }
.heat-wrap { overflow-x: auto; }
.heat { border-collapse: separate; border-spacing: 4px; margin: 0 auto; }
.heat th { font-size: 12px; font-weight: 600; color: var(--qtf-muted); padding: 2px 6px; text-align: center; }
.heat .corner { text-align: left; max-width: 88px; }
.heat .axis { font-weight: 600; font-size: 11px; }
.heat td { padding: 0; vertical-align: middle; }
.heat-cell { all: unset; box-sizing: border-box; display: grid; place-items: center; width: 56px; height: 44px; border-radius: 6px; border: 2px solid transparent; font-weight: 700; font-size: 15px; cursor: pointer; color: #111; }
.heat-cell.empty { opacity: 0.55; }
.heat-cell:hover { filter: brightness(0.95); }
.heat-cell:focus-visible { outline: 3px solid var(--p-primary-color); outline-offset: 2px; }
.heat-cell.active { border-color: var(--p-text-color, #000); box-shadow: 0 0 0 2px var(--qtf-card); }
.lvl-low { background: #bbf7d0; }
.lvl-medium { background: #fde68a; }
.lvl-high { background: #fdba74; }
.lvl-critical { background: #fca5a5; }
.levels { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
.levels button { display: inline-flex; align-items: center; gap: 10px; padding: 2px 4px; }
.lvl-count { font-size: 20px; font-weight: 700; }
.cell-chip { display: inline-flex; gap: 6px; align-items: center; }
@media (max-width: 900px) {
  .risk-top { grid-template-columns: minmax(0, 1fr); }
}
@media (max-width: 480px) {
  .heat-cell { width: 44px; height: 44px; }
  .heat { border-spacing: 3px; }
}
</style>
