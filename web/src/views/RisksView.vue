<script setup lang="ts">
// Risk register: likelihood × impact, a 5×5 heat map that filters the
// table, mitigation and the requirements that address each risk.
import DiscussionPanel from '@/components/DiscussionPanel.vue'
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
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { P } from '@/lib/permissions'
import { atResidual, filterRisks, heatGrid, levelCounts, levelRank, riskLevel, riskLevelSeverity, riskScore, riskStatusSeverity, RISK_LEVELS, RISK_SCALE, RISK_STATUSES, type HeatCell } from '@/lib/risks'
import ESignDialog from '@/components/ESignDialog.vue'
import SelectButton from 'primevue/selectbutton'
import { formatDate, formatDateTime } from '@/lib/format'
import type { Requirement, Risk } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'
import SuggestInput from '@/components/SuggestInput.vue'
import MentionInput from '@/components/MentionInput.vue'
import { rememberValues, savedEntries } from '@/lib/suggest'
import { useI18n } from 'vue-i18n'
import { loadTechniques, techniqueUrl, type TechniqueList } from '@/lib/attack'

const proj = useProjectStore()
const auth = useAuthStore()
const prefs = usePrefsStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t, locale } = useI18n()

/** custom fields shown in lists (show_in_list) as columns */
const listedCols = computed(() => customColumns(useProjectStore().customFieldsFor('risk').filter((f) => f.show_in_list)))
const cellOf = (row: { custom_fields?: Record<string, unknown> }, field: string) => customCellText(useProjectStore().customFieldsFor('risk'), row, field, [t('common.yes'), t('common.no')])
const canWrite = computed(() => auth.can(P.requirementWrite))
const canApprove = computed(() => auth.can(P.waiverApprove))
const canSetThreshold = computed(() => auth.can(P.projectUpdate))

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
  await Promise.all([load(), loadRequirements(), loadThreshold(), loadAreaNames()])
  openFromRoute()
})

// ---------- residual risk: acceptance threshold, before / after ----------
const threshold = ref('')
const effectiveThreshold = computed(() => threshold.value || 'medium')
const thresholdOptions = computed(() => [{ label: t('residual.defaultThreshold'), value: '' }, ...RISK_LEVELS.map((v) => ({ label: levelLabel(v), value: v }))])
async function loadThreshold() {
  try {
    threshold.value = (await api.get<{ threshold: string }>(`/projects/${proj.pid}/risk-settings`)).threshold ?? ''
  } catch {
    threshold.value = ''
  }
}
async function setThreshold(v: string) {
  try {
    await api.put(`/projects/${proj.pid}/risk-settings`, { threshold: v })
    threshold.value = v
    notify.ok(t('residual.thresholdSaved'))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
const view = ref<'before' | 'after'>('before')
const viewOptions = computed(() => [
  { label: t('residual.before'), value: 'before' },
  { label: t('residual.after'), value: 'after' },
])
const sourceLabel = (s?: string) => t(`residual.source.${s || 'inherent'}`)
const aboveCount = computed(() => risks.value.filter((r) => r.above_threshold && !r.acceptance_valid).length)

// ---------- catalogue area: narrows the heat map, the level counts and the register ----------
const area = ref<string>(typeof route.query.area === 'string' ? route.query.area : '')
const areaNames = ref<Record<string, string>>({})
async function loadAreaNames() {
  try {
    const ov = await api.get<{ groups: { areas: { id: string; name: string }[] }[] }>('/catalog', { lang: String(locale.value).startsWith('sv') ? 'sv' : 'en' })
    areaNames.value = Object.fromEntries(ov.groups.flatMap((g) => g.areas.map((a) => [a.id, a.name])))
  } catch {
    areaNames.value = {}
  }
}
const ALL_AREAS = '*'
const NO_AREA = '-'
const areaOptions = computed(() => {
  const ids = [...new Set(risks.value.map((r) => r.catalog_area ?? '').filter(Boolean))]
  ids.sort((a, b) => (areaNames.value[a] ?? a).localeCompare(areaNames.value[b] ?? b))
  return [
    { label: t('risksView.area.all'), value: ALL_AREAS },
    ...ids.map((id) => ({ label: areaNames.value[id] ?? id, value: id })),
    ...(ids.length && risks.value.some((r) => !r.catalog_area) ? [{ label: t('risksView.area.none'), value: NO_AREA }] : []),
  ]
})
const areaSel = computed({ get: () => area.value || ALL_AREAS, set: (v: string) => (area.value = v === ALL_AREAS ? '' : v) })
const scoped = computed(() => (!area.value ? risks.value : risks.value.filter((r) => (area.value === NO_AREA ? !r.catalog_area : r.catalog_area === area.value))))
watch(area, (v) => router.replace({ query: { ...route.query, area: v || undefined } }))

// ---------- filters and heat map ----------
const text = ref('')
const levels = ref<string[]>([])
const statuses = ref<string[]>([])
const includeClosed = ref(false)
const cell = ref<{ likelihood: number; impact: number } | null>(null)
const grid = computed(() => heatGrid(view.value === 'after' ? atResidual(scoped.value) : scoped.value, includeClosed.value))
const filtered = computed(() =>
  filterRisks(
    scoped.value.filter((r) => includeClosed.value || r.status !== 'closed' || statuses.value.includes('closed')),
    { residual: view.value === 'after', likelihood: cell.value?.likelihood, impact: cell.value?.impact, levels: levels.value, statuses: statuses.value, text: text.value },
  ),
)
const counts = computed(() => levelCounts(scoped.value.filter((r) => r.status !== 'closed')))
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
const hasFilter = computed(() => !!cell.value || !!levels.value.length || !!statuses.value.length || !!text.value.trim() || !!area.value)
function clearFilters() {
  cell.value = null
  area.value = ''
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
  residual_likelihood: number
  residual_impact: number
  residual_manual: boolean
  residual_justification: string
  review_date: string
  review_owner: string
  custom_fields: CustomValues
  attack: string[]
}
const blank = (): Form => ({
  title: '',
  description: '',
  category: '',
  likelihood: 3,
  impact: 3,
  status: 'identified',
  owner: '',
  mitigation: '',
  requirement_ids: [],
  residual_likelihood: 0,
  residual_impact: 0,
  residual_manual: false,
  residual_justification: '',
  review_date: '',
  review_owner: '',
  custom_fields: defaultValues(proj.customFieldsFor('risk')),
  attack: [],
})
const residualScale = (label: (n: number) => string) => [{ label: t('residual.notAssessed'), value: 0 }, ...RISK_SCALE.map((n) => ({ label: `${n} – ${label(n)}`, value: n }))]
const formResidualLevel = computed(() => (form.value.residual_likelihood && form.value.residual_impact ? riskLevel(form.value.residual_likelihood, form.value.residual_impact) : ''))
const residualInvalid = computed(() => (form.value.residual_likelihood === 0) !== (form.value.residual_impact === 0) || (form.value.residual_manual && (!form.value.residual_likelihood || form.value.residual_justification.trim().length < 3)))

// accept risk (electronic signature)
const acceptOpen = ref(false)
const acceptBusy = ref(false)
const acceptError = ref('')
async function acceptRisk(v: { comment: string; password: string }) {
  if (!editing.value) return
  acceptBusy.value = true
  acceptError.value = ''
  try {
    const r = await api.post<Risk>(`/risks/${editing.value.id}/accept`, { comment: v.comment, password: v.password })
    editing.value = r
    form.value.status = r.status
    acceptOpen.value = false
    notify.ok(t('residual.accepted'), r.key)
    await load()
  } catch (e) {
    if (e instanceof ApiError && (e.code === 'reauth_failed' || e.code === 'reauth_required')) acceptError.value = t(`esign.${e.code}`)
    else acceptError.value = errorMessage(e)
  } finally {
    acceptBusy.value = false
  }
}
async function withdrawAcceptance() {
  if (!editing.value) return
  try {
    const r = await api.del<Risk>(`/risks/${editing.value.id}/accept`)
    if (r) {
      editing.value = r
      form.value.status = r.status
    }
    notify.ok(t('residual.acceptanceWithdrawn'))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
const dialog = ref(false)
const editing = ref<Risk | null>(null)
const form = ref<Form>(blank())
// MITRE ATT&CK and ATLAS techniques: chosen from the catalogue's list
const attackList = ref<TechniqueList | null>(null)
const attackById = computed(() => new Map((attackList.value?.items ?? []).map((x) => [x.id, x])))
watch(dialog, (open) => {
  if (open && !attackList.value) loadTechniques().then((l) => (attackList.value = l), () => (attackList.value = null))
})
const attackOptions = computed(() => {
  const opts = (attackList.value?.items ?? []).map((x) => ({ label: `${x.id} ${x.name}`, value: x.id }))
  // ids no longer in the list stay visible so that they can be removed
  for (const id of form.value.attack) if (attackList.value && !attackById.value.has(id)) opts.unshift({ label: id, value: id })
  return opts
})
const unknownAttack = computed(() => (attackList.value ? form.value.attack.filter((id) => !attackById.value.has(id)) : []))
const formTactics = computed(() => [...new Set(form.value.attack.flatMap((id) => attackById.value.get(id)?.tactics.map((x) => x.name) ?? []))])

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
    residual_likelihood: r.residual_likelihood ?? 0,
    residual_impact: r.residual_impact ?? 0,
    residual_manual: !!r.residual_manual,
    residual_justification: r.residual_justification ?? '',
    review_date: r.review_date ?? '',
    review_owner: r.review_owner ?? '',
    custom_fields: { ...(r.custom_fields ?? {}) } as CustomValues,
    attack: [...(r.attack ?? [])],
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
  const body = { ...fields, title: fields.title.trim(), category: fields.category.trim(), owner: fields.owner.trim(), review_owner: fields.review_owner.trim() }
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
        <div class="row">
          <h2 id="heat-title" class="grow">{{ view === 'after' ? t('residual.heatAfter') : t('risksView.heatMap') }}</h2>
          <SelectButton v-model="view" :options="viewOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('residual.view')" size="small" />
        </div>
        <div v-if="areaOptions.length > 1" class="row small area-row">
          <label for="risk-area">{{ t('risksView.area.label') }}</label>
          <Select v-model="areaSel" input-id="risk-area" :options="areaOptions" option-label="label" option-value="value" size="small" filter class="area-select" />
        </div>
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
        <div class="row small threshold-row">
          <label for="risk-threshold"><b>{{ t('residual.threshold') }}</b></label>
          <Select :model-value="threshold" input-id="risk-threshold" :aria-label="t('residual.threshold')" :options="thresholdOptions" option-label="label" option-value="value" size="small" :disabled="!canSetThreshold" @update:model-value="(v: string) => setThreshold(v)" />
          <span class="muted">{{ t('residual.aboveCount', { count: aboveCount, level: levelLabel(effectiveThreshold) }, aboveCount) }}</span>
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
        <Tag v-if="area" severity="info" class="cell-chip">
          <span>{{ t('risksView.area.chip', { area: areaNames[area] ?? (area === NO_AREA ? t('risksView.area.none') : area) }) }}</span>
          <button type="button" class="link-button" :aria-label="t('risksView.area.clear')" @click="area = ''"><i class="pi pi-times" aria-hidden="true" /></button>
        </Tag>
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
            <div v-if="data.category || data.catalog_ref" class="small muted">{{ data.category }}<template v-if="data.catalog_ref"><template v-if="data.category"> · </template><span :title="t('risksView.fromCatalogue')">{{ data.catalog_ref }}</span></template></div>
            <div v-if="data.attack?.length" class="small muted mono" :title="t('attack.techniques')">{{ data.attack.join(' ') }}</div>
          </template>
        </Column>
        <Column field="likelihood" :header="t('risksView.likelihood')" sortable style="width: 110px"><template #body="{ data }"><span :title="likelihoodLabel(data.likelihood)">{{ data.likelihood }}</span></template></Column>
        <Column field="impact" :header="t('risksView.impact')" sortable style="width: 100px"><template #body="{ data }"><span :title="impactLabel(data.impact)">{{ data.impact }}</span></template></Column>
        <Column field="score" :header="t('risksView.score')" sortable style="width: 90px"><template #body="{ data }"><b>{{ data.score }}</b></template></Column>
        <Column field="level" :header="t('risksView.colLevel')" sortable :sort-field="(r: Risk) => String(levelRank(r.level))" style="width: 110px">
          <template #body="{ data }"><Tag :value="levelLabel(data.level)" :severity="riskLevelSeverity(data.level)" /></template>
        </Column>
        <Column field="residual_score" :header="t('residual.col')" sortable style="width: 150px">
          <template #body="{ data }">
            <span class="row nowrap" style="gap: 4px">
              <b>{{ data.residual_score ?? data.score }}</b>
              <Tag :value="levelLabel(data.residual_level || data.level)" :severity="riskLevelSeverity(data.residual_level || data.level)" />
              <i v-if="data.residual_source === 'manual'" class="pi pi-user-edit muted" role="img" :aria-label="sourceLabel('manual')" v-tooltip="sourceLabel('manual')" />
              <i v-else-if="data.residual_source === 'verified'" class="pi pi-check-circle muted" role="img" :aria-label="sourceLabel('verified')" v-tooltip="sourceLabel('verified')" />
              <i v-if="data.above_threshold && !data.acceptance_valid" class="pi pi-exclamation-triangle above" role="img" :aria-label="t('residual.aboveTip')" v-tooltip="t('residual.aboveTip')" />
              <i v-else-if="data.acceptance_valid" class="pi pi-pen-to-square muted" role="img" :aria-label="t('residual.acceptedTip')" v-tooltip="t('residual.acceptedTip')" />
            </span>
          </template>
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
        <div v-field class="field"><label for="risk-owner">{{ t('common.owner') }}</label><MentionInput id="risk-owner" v-model="form.owner" suggest="risk.owner" :project-id="proj.pid" :disabled="!canWrite" :placeholder="t('requirements.ownerPlaceholder', { at: '@' })" /></div>
        <div v-field class="field"><label for="risk-category">{{ t('common.category') }}</label><InputText id="risk-category" v-model="form.category" :placeholder="t('risksView.categoryPlaceholder')" :disabled="!canWrite" /></div>
        <div v-field class="field full">
          <label for="risk-attack">{{ t('attack.techniques') }}</label>
          <MultiSelect
            v-model="form.attack"
            input-id="risk-attack"
            :options="attackOptions"
            option-label="label"
            option-value="value"
            filter
            display="chip"
            :virtual-scroller-options="{ itemSize: 36 }"
            :loading="dialog && !attackList"
            :placeholder="t('attack.choose')"
            :disabled="!canWrite"
            :aria-describedby="'risk-attack-hint'"
            fluid
          />
          <small id="risk-attack-hint" class="muted">
            {{ formTactics.length ? t('attack.tacticsOf', { tactics: formTactics.join(', ') }) : t('attack.hint') }}
            <template v-for="id in form.attack.filter((x) => attackById.has(x))" :key="id">
              · <a :href="techniqueUrl(id)" target="_blank" rel="noopener noreferrer">{{ id }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
            </template>
          </small>
          <Message v-if="unknownAttack.length" severity="warn" :closable="false" class="small">{{ t('attack.unknown', { ids: unknownAttack.join(', ') }) }}</Message>
        </div>
        <div v-field class="field full"><label for="risk-desc">{{ t('common.description') }}</label><Textarea id="risk-desc" v-model="form.description" rows="2" auto-resize :disabled="!canWrite" /></div>
        <div v-field class="field full"><label for="risk-mitigation">{{ t('risksView.mitigation') }}</label><SuggestInput id="risk-mitigation" v-model="form.mitigation" field="risk.mitigation" :project-id="proj.pid" multiline rows="3" auto-resize :placeholder="t('risksView.mitigationPlaceholder')" :disabled="!canWrite" /></div>
      </div>
      <fieldset class="residual">
        <legend>{{ t('residual.title') }}</legend>
        <p class="small muted" style="margin: 0 0 8px">{{ t('residual.intro') }}</p>
        <div class="form-grid">
          <div v-field class="field">
            <label for="risk-rl">{{ t('residual.likelihood') }}</label>
            <Select v-model="form.residual_likelihood" input-id="risk-rl" :options="residualScale(likelihoodLabel)" option-label="label" option-value="value" :disabled="!canWrite" />
          </div>
          <div v-field class="field">
            <label for="risk-ri">{{ t('residual.impact') }}</label>
            <Select v-model="form.residual_impact" input-id="risk-ri" :options="residualScale(impactLabel)" option-label="label" option-value="value" :disabled="!canWrite" />
          </div>
          <label class="row nowrap field full" style="gap: 6px"><Checkbox v-model="form.residual_manual" binary input-id="risk-rmanual" :disabled="!canWrite" /> {{ t('residual.manual') }}</label>
          <div v-if="form.residual_manual" v-field class="field full">
            <label for="risk-rjust">{{ t('residual.justification') }}</label>
            <Textarea id="risk-rjust" v-model="form.residual_justification" rows="2" auto-resize :disabled="!canWrite" aria-required="true" />
          </div>
          <div class="field full" role="status">
            <span v-if="formResidualLevel" class="small">{{ t('residual.planned', { score: form.residual_likelihood * form.residual_impact }) }} <Tag :value="levelLabel(formResidualLevel)" :severity="riskLevelSeverity(formResidualLevel)" /></span>
            <span v-if="editing" class="small muted"> · {{ t('residual.inEffect', { score: editing.residual_score ?? editing.score, level: levelLabel(editing.residual_level || editing.level), source: sourceLabel(editing.residual_source) }) }}</span>
            <Message v-if="editing?.above_threshold && !editing.acceptance_valid" severity="warn" :closable="false" style="margin-top: 6px">{{ t('residual.aboveMessage', { level: levelLabel(editing.threshold || 'medium') }) }}</Message>
            <Message v-if="residualInvalid" severity="error" :closable="false" style="margin-top: 6px">{{ t('residual.invalid') }}</Message>
          </div>
          <div v-field class="field">
            <label for="risk-review-date">{{ t('residual.reviewDate') }}</label>
            <InputText id="risk-review-date" v-model="form.review_date" type="date" :disabled="!canWrite" />
          </div>
          <div v-field class="field"><label for="risk-review-owner">{{ t('residual.reviewOwner') }}</label><MentionInput id="risk-review-owner" v-model="form.review_owner" :project-id="proj.pid" :disabled="!canWrite" :placeholder="t('requirements.ownerPlaceholder', { at: '@' })" /></div>
        </div>
        <div v-if="editing" class="row small" style="margin-top: 8px">
          <template v-if="editing.accepted_by">
            <Tag :value="editing.acceptance_valid ? t('residual.acceptedTag') : t('residual.acceptanceStale')" :severity="editing.acceptance_valid ? 'success' : 'danger'" :icon="editing.acceptance_valid ? 'pi pi-check' : 'pi pi-exclamation-triangle'" />
            <span class="grow">{{ t('residual.acceptedBy', { name: editing.accepted_name || editing.accepted_by, date: editing.accepted_at ? formatDateTime(editing.accepted_at) : '' }) }}<span v-if="editing.acceptance_comment"> – {{ editing.acceptance_comment }}</span></span>
            <Button v-if="canApprove" :label="t('residual.withdraw')" icon="pi pi-undo" size="small" text severity="secondary" @click="withdrawAcceptance" />
          </template>
          <span v-else class="grow muted">{{ t('residual.notAccepted') }}</span>
          <span v-if="editing.review_due" class="small"><Tag :value="t('residual.reviewDue', { date: formatDate(editing.review_date) })" severity="warn" icon="pi pi-calendar" /></span>
          <Button v-if="canApprove && editing.status !== 'closed'" :label="t('residual.accept')" icon="pi pi-pen-to-square" size="small" outlined @click="(acceptError = ''), (acceptOpen = true)" />
        </div>
      </fieldset>
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
        <Button v-if="canWrite" type="submit" :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.title.trim() || residualInvalid" />
      </div>
    </form>
    <DiscussionPanel v-if="editing" entity="risk" :entity-id="editing.id" :project-id="proj.pid" compact class="dialog-discussion" />
  </Dialog>
  <ItemPickerDialog v-model:visible="bulkPicker" kind="requirement" :project-id="proj.pid" :header="t('risksView.bulk.linkHeader', { n: selection.length }, selection.length)" :confirm-label="t('common.link')" @pick="bulkLink" />
  <Dialog v-model:visible="ownerDialog" :header="t('risksView.bulk.ownerHeader', { n: selection.length }, selection.length)" modal style="width: min(440px, 96vw)">
    <div class="bulk-form">
      <div class="row">
        <Checkbox v-model="bulkOwnerOn" binary input-id="bk-risk-owner-on" />
        <label for="bk-risk-owner-on">{{ t('risksView.bulk.setOwner') }}</label>
      </div>
      <MentionInput v-model="bulkOwner" :project-id="proj.pid" :disabled="!bulkOwnerOn" :aria-label="t('risksView.bulk.setOwner')" :placeholder="t('risksView.bulk.emptyClears')" />
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
  <ESignDialog
    v-model:visible="acceptOpen"
    :header="t('residual.acceptHeader', { key: editing?.key ?? '' })"
    :explain="t('residual.acceptExplain', { score: editing?.residual_score ?? 0, level: levelLabel(editing?.residual_level || 'low') })"
    :confirm-label="t('residual.accept')"
    :busy="acceptBusy"
    :error="acceptError"
    @sign="acceptRisk"
  />
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
.threshold-row { margin-top: 8px; gap: 8px; flex-wrap: wrap; }
.above { color: var(--p-orange-600, #c2410c); }
.residual { border: 1px solid var(--qtf-border); border-radius: 8px; padding: 8px 12px 12px; margin: 0; min-width: 0; }
.residual legend { font-weight: 600; padding: 0 4px; }
@media (max-width: 900px) {
  .risk-top { grid-template-columns: minmax(0, 1fr); }
}
@media (max-width: 480px) {
  .heat-cell { width: 44px; height: 44px; }
  .heat { border-spacing: 3px; }
}
.area-row { gap: 8px; align-items: center; margin: 4px 0 8px; }
.area-select { min-width: 12rem; }
</style>
