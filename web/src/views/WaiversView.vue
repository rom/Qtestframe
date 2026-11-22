<script setup lang="ts">
// Waivers and deviations: formal acceptance of requirements that are not
// (or differently) met, for a release or the whole project, approved with
// an electronic signature. Approved, unexpired waivers count as accepted in
// release readiness and are shown separately in coverage and compliance.
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
import ItemRef from '@/components/ItemRef.vue'
import ESignaturesPanel from '@/components/ESignaturesPanel.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import ESignDialog from '@/components/ESignDialog.vue'
import { signErrorKey } from '@/lib/erecords'
import VariantMultiSelect from '@/components/VariantMultiSelect.vue'
import { useVariants } from '@/composables/variants'
import HelpLink from '@/guide/HelpLink.vue'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime } from '@/lib/format'
import { shortHash } from '@/lib/notifications'
import { P } from '@/lib/permissions'
import {
  approvalProgress,
  decisionBlock,
  expiresSoon,
  filterWaivers,
  parseApprovers,
  waiverEditable,
  waiverStatusIcon,
  waiverStatusSeverity,
  WAIVER_STATUSES,
  WAIVER_TYPES,
  type Waiver,
} from '@/lib/waivers'
import { RISK_SCALE } from '@/lib/risks'
import type { Release, Requirement, Risk } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const prefs = usePrefsStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const canWrite = computed(() => auth.can(P.requirementWrite))
const canApprove = computed(() => auth.can(P.waiverApprove))
const today = new Date().toISOString().slice(0, 10)

const typeLabel = (v: string) => t(`waivers.type.${v}`)
const statusLabel = (v: string) => t(`waivers.status.${v}`)
const scopeLabel = (w: Waiver) => {
  const base = w.release_id ? w.release_name || t('waivers.release') : t('waivers.wholeProject')
  return w.variants?.length ? `${base} · ${t('waivers.onlyVariants', { names: w.variants.map((v) => v.name).join(', ') })}` : base
}
// the variants a waiver can be limited to
const variantList = useVariants(() => proj.pid)
onMounted(() => void variantList.load())

// ---------- data ----------
const waivers = ref<Waiver[]>([])
const releases = ref<Release[]>([])
const risks = ref<Risk[]>([])
const requirements = ref<Requirement[]>([])
const reqById = computed(() => new Map(requirements.value.map((r) => [r.id, r])))
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    waivers.value = asItems(await api.get<{ items: Waiver[] }>(`/projects/${proj.pid}/waivers`))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
async function loadContext() {
  const [rel, rk, rq] = await Promise.allSettled([
    api.get<{ items: Release[] }>(`/projects/${proj.pid}/releases`),
    api.get<{ items: Risk[] }>(`/projects/${proj.pid}/risks`),
    api.get<{ items: Requirement[] }>(`/projects/${proj.pid}/requirements`),
  ])
  releases.value = rel.status === 'fulfilled' ? asItems(rel.value) : []
  risks.value = rk.status === 'fulfilled' ? asItems(rk.value) : []
  requirements.value = rq.status === 'fulfilled' ? asItems(rq.value) : []
}
onMounted(async () => {
  await Promise.all([load(), loadContext()])
  fromRoute()
})

// ---------- filters ----------
const text = ref('')
const statuses = ref<string[]>([])
const types = ref<string[]>([])
const releaseFilter = ref('')
const activeOnly = ref(false)
const filtered = computed(() => filterWaivers(waivers.value, { text: text.value, statuses: statuses.value, types: types.value, release: releaseFilter.value, activeOnly: activeOnly.value }))
const statusOptions = computed(() => WAIVER_STATUSES.map((v) => ({ label: statusLabel(v), value: v })))
const typeOptions = computed(() => WAIVER_TYPES.map((v) => ({ label: typeLabel(v), value: v })))
const releaseFilterOptions = computed(() => [
  { label: t('waivers.anyScope'), value: '' },
  { label: t('waivers.wholeProject'), value: 'project' },
  ...releases.value.map((r) => ({ label: r.name, value: r.id })),
])
const hasFilter = computed(() => !!text.value.trim() || !!statuses.value.length || !!types.value.length || !!releaseFilter.value || activeOnly.value)
function clearFilters() {
  text.value = ''
  statuses.value = []
  types.value = []
  releaseFilter.value = ''
  activeOnly.value = false
}
const counts = computed(() => ({
  active: waivers.value.filter((w) => w.active).length,
  requested: waivers.value.filter((w) => w.status === 'requested').length,
  soon: waivers.value.filter((w) => expiresSoon(w, today)).length,
}))

// ---------- detail ----------
const detailOpen = ref(false)
const detail = ref<Waiver | null>(null)
function openDetail(w: Waiver) {
  detail.value = w
  detailOpen.value = true
  if (route.query.waiver !== w.id) router.replace({ query: { ...route.query, waiver: w.id } })
}
watch(detailOpen, (v) => {
  if (!v && route.query.waiver) router.replace({ query: { ...route.query, waiver: undefined } })
})
function fromRoute() {
  const id = route.query.waiver
  if (typeof id === 'string') {
    const w = waivers.value.find((x) => x.id === id)
    if (w) openDetail(w)
  }
  if (route.query.new === '1' && canWrite.value) {
    const req = typeof route.query.requirement === 'string' ? [route.query.requirement] : []
    const rel = typeof route.query.release === 'string' ? route.query.release : ''
    openCreate(req, rel)
    router.replace({ query: { ...route.query, new: undefined, requirement: undefined, release: undefined } })
  }
}
watch(() => route.query.waiver, fromRoute)
function replaceWaiver(w: Waiver) {
  waivers.value = waivers.value.map((x) => (x.id === w.id ? w : x))
  if (detail.value?.id === w.id) detail.value = w
}
const block = computed(() => (detail.value ? decisionBlock(detail.value, auth.user?.email) : 'not_requested'))
const progress = computed(() => (detail.value ? approvalProgress(detail.value) : { approved: 0, needed: 1 }))

async function act(path: string, body: unknown, done: string) {
  if (!detail.value) return
  try {
    const w = await api.post<Waiver>(`/waivers/${detail.value.id}/${path}`, body)
    replaceWaiver(w)
    notify.ok(done, w.key)
  } catch (e) {
    notify.fail(e)
  }
}
const submit = () => act('submit', {}, t('waivers.submitted'))
const withdraw = () => act('withdraw', {}, t('waivers.withdrawn'))

// sign (approve / reject)
const signOpen = ref(false)
const signBusy = ref(false)
const signError = ref('')
function openSign() {
  signError.value = ''
  signOpen.value = true
}
async function sign(v: { decision: string; comment: string; password: string; meaning?: string; mfa_code?: string; time_zone?: string }) {
  if (!detail.value) return
  signBusy.value = true
  signError.value = ''
  try {
    const w = await api.post<Waiver>(`/waivers/${detail.value.id}/decision`, v)
    replaceWaiver(w)
    signOpen.value = false
    notify.ok(v.decision === 'reject' ? t('waivers.rejectedNote') : t('waivers.signedNote'), w.key)
  } catch (e) {
    if (e instanceof ApiError && signErrorKey(e.code)) signError.value = t(signErrorKey(e.code))
    else signError.value = errorMessage(e)
  } finally {
    signBusy.value = false
  }
}

// revoke
const revokeOpen = ref(false)
const revokeReason = ref('')
async function revoke() {
  await act('revoke', { comment: revokeReason.value.trim() }, t('waivers.revoked'))
  revokeOpen.value = false
}
function remove(w: Waiver) {
  notify.confirmDanger(t('waivers.confirmDelete', { key: w.key, title: w.title }), async () => {
    try {
      await api.del(`/waivers/${w.id}`)
      notify.ok(t('waivers.deleted'))
      detailOpen.value = false
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- create / edit ----------
interface Form {
  type: string
  title: string
  release_id: string
  requirement_ids: string[]
  variant_ids: string[]
  justification: string
  compensating: string
  conditions: string
  risk: string
  new_risk_likelihood: number
  new_risk_impact: number
  approvers: string
  expires_on: string
  submit: boolean
}
const blank = (): Form => ({ type: 'waiver', title: '', release_id: '', requirement_ids: [], variant_ids: [], justification: '', compensating: '', conditions: '', risk: '', new_risk_likelihood: 3, new_risk_impact: 3, approvers: '', expires_on: '', submit: false })
const formOpen = ref(false)
const editing = ref<Waiver | null>(null)
const form = ref<Form>(blank())
const formError = ref('')
const busy = ref(false)
const picker = ref(false)
const releaseOptions = computed(() => [{ label: t('waivers.wholeProject'), value: '' }, ...releases.value.map((r) => ({ label: r.name, value: r.id }))])
const riskOptions = computed(() => [
  { label: t('waivers.noRisk'), value: '' },
  ...(canWrite.value ? [{ label: t('waivers.newRisk'), value: 'new' }] : []),
  ...risks.value.map((r) => ({ label: `${r.key} ${r.title}`, value: r.id })),
])
const scaleOptions = RISK_SCALE.map((n) => ({ label: String(n), value: n }))
const linked = computed(() => form.value.requirement_ids.map((id) => reqById.value.get(id) ?? ({ id, key: id.slice(0, 8), title: '' } as Requirement)))

function openCreate(reqIds: string[] = [], releaseId = '') {
  editing.value = null
  form.value = { ...blank(), requirement_ids: reqIds, release_id: releaseId }
  formError.value = ''
  formOpen.value = true
}
function openEdit(w: Waiver) {
  editing.value = w
  form.value = {
    type: w.type,
    title: w.title,
    release_id: w.release_id ?? '',
    requirement_ids: [...w.requirement_ids],
    variant_ids: [...(w.variant_ids ?? [])],
    justification: w.justification,
    compensating: w.compensating,
    conditions: w.conditions,
    risk: w.risk_id ?? '',
    new_risk_likelihood: 3,
    new_risk_impact: 3,
    approvers: w.approvers.join(', '),
    expires_on: w.expires_on,
    submit: false,
  }
  formError.value = ''
  formOpen.value = true
}
function addLinked(items: { id: string }[]) {
  const have = new Set(form.value.requirement_ids)
  form.value.requirement_ids = [...form.value.requirement_ids, ...items.map((i) => i.id).filter((id) => !have.has(id))]
}
async function save() {
  busy.value = true
  formError.value = ''
  const f = form.value
  const body: Record<string, unknown> = {
    type: f.type,
    title: f.title.trim(),
    release_id: f.release_id || null,
    requirement_ids: f.requirement_ids,
    variant_ids: f.variant_ids,
    justification: f.justification,
    compensating: f.compensating,
    conditions: f.conditions,
    approvers: parseApprovers(f.approvers),
    expires_on: f.expires_on,
  }
  if (f.risk === 'new') body.new_risk = { title: f.title.trim(), likelihood: f.new_risk_likelihood, impact: f.new_risk_impact }
  else body.risk_id = f.risk || null
  try {
    let w: Waiver
    if (editing.value) w = await api.patch<Waiver>(`/waivers/${editing.value.id}`, body)
    else w = await api.post<Waiver>(`/projects/${proj.pid}/waivers`, { ...body, submit: f.submit })
    notify.ok(editing.value ? t('waivers.updated') : t('waivers.created'), w.key)
    formOpen.value = false
    await Promise.all([load(), f.risk === 'new' ? loadContext() : Promise.resolve()])
    const fresh = waivers.value.find((x) => x.id === w.id)
    if (fresh) openDetail(fresh)
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('waivers.title') }}</h1>
      <div class="sub">{{ t('waivers.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="waivers-and-soa" />
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
      <Button v-if="canWrite" :label="t('waivers.new')" icon="pi pi-plus" @click="openCreate()" />
    </div>
  </div>

  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <section v-else class="card" aria-labelledby="waivers-list-title">
    <h2 id="waivers-list-title" class="sr-only">{{ t('waivers.list') }}</h2>
    <div class="row small" style="margin-bottom: 8px; gap: 14px">
      <span><b>{{ counts.active }}</b> {{ t('waivers.inForce') }}</span>
      <span><b>{{ counts.requested }}</b> {{ t('waivers.awaiting') }}</span>
      <span><b>{{ counts.soon }}</b> {{ t('waivers.expiringSoon') }}</span>
    </div>
    <div class="filterbar">
      <InputText v-model="text" :placeholder="t('waivers.searchPlaceholder')" :aria-label="t('waivers.searchPlaceholder')" style="min-width: 200px" />
      <MultiSelect v-model="statuses" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('common.status')" :aria-label="t('common.status')" show-clear style="min-width: 140px" />
      <MultiSelect v-model="types" :options="typeOptions" option-label="label" option-value="value" :placeholder="t('waivers.typeLabel')" :aria-label="t('waivers.typeLabel')" show-clear style="min-width: 140px" />
      <Select v-model="releaseFilter" :options="releaseFilterOptions" option-label="label" option-value="value" :aria-label="t('waivers.scope')" style="min-width: 160px" />
      <label class="row nowrap small" style="gap: 6px"><Checkbox v-model="activeOnly" binary input-id="wv-active" /> {{ t('waivers.activeOnly') }}</label>
      <Button v-if="hasFilter" :label="t('common.clear')" text size="small" severity="secondary" @click="clearFilters" />
      <span class="small muted" role="status">{{ t('waivers.shown', { count: filtered.length, total: waivers.length }) }}</span>
    </div>
    <DataTable v-stack :value="filtered" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(25)" :always-show-paginator="false" row-hover size="small" @row-click="(e) => openDetail(e.data as Waiver)">
      <Column field="key" :header="t('common.key')" sortable style="width: 130px"><template #body="{ data }"><span class="key">{{ data.key }}</span></template></Column>
      <Column field="title" :header="t('common.title')" sortable body-class="stack-main" style="min-width: 200px">
        <template #body="{ data }">
          <button type="button" class="link-button" style="font-weight: 600" :aria-label="t('waivers.open', { key: data.key, title: data.title })" @click.stop="openDetail(data as Waiver)">{{ data.title }}</button>
        </template>
      </Column>
      <Column field="type" :header="t('waivers.typeLabel')" sortable style="width: 120px"><template #body="{ data }">{{ typeLabel(data.type) }}</template></Column>
      <Column field="status" :header="t('common.status')" sortable style="width: 140px">
        <template #body="{ data }">
          <Tag :value="statusLabel(data.status)" :severity="waiverStatusSeverity(data.status)" :icon="waiverStatusIcon(data.status)" />
        </template>
      </Column>
      <Column :header="t('waivers.scope')" style="width: 150px"><template #body="{ data }"><span class="small">{{ scopeLabel(data as Waiver) }}</span></template></Column>
      <Column :header="t('waivers.requirements')" style="min-width: 160px">
        <template #body="{ data }">
          <span class="row small" style="gap: 4px">
            <RouterLink v-for="q in data.requirements" :key="q.id" :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: q.id } }" class="key" :title="q.title" @click.stop>{{ q.key }}</RouterLink>
          </span>
        </template>
      </Column>
      <Column field="expires_on" :header="t('waivers.expires')" sortable style="width: 140px">
        <template #body="{ data }">
          <span v-if="data.expires_on" class="small">{{ formatDate(data.expires_on) }}</span>
          <span v-else class="small muted">{{ t('waivers.noExpiry') }}</span>
          <Tag v-if="expiresSoon(data as Waiver, today)" :value="t('waivers.soon')" severity="warn" icon="pi pi-hourglass" style="margin-left: 4px" />
        </template>
      </Column>
      <template #empty>
        <div class="empty"><i class="pi pi-verified" />{{ waivers.length ? t('waivers.noneMatch') : t('waivers.none') }}</div>
      </template>
    </DataTable>
  </section>

  <!-- detail -->
  <Dialog v-model:visible="detailOpen" :header="detail ? `${detail.key} ${detail.title}` : ''" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(760px, 96vw)">
    <template v-if="detail">
      <div class="row" style="margin-bottom: 8px">
        <Tag :value="typeLabel(detail.type)" severity="secondary" />
        <Tag :value="statusLabel(detail.status)" :severity="waiverStatusSeverity(detail.status)" :icon="waiverStatusIcon(detail.status)" />
        <Tag v-if="detail.active" :value="t('waivers.inForceTag')" severity="success" icon="pi pi-check" />
        <span class="small muted">{{ scopeLabel(detail) }}</span>
      </div>
      <dl class="wv-fields">
        <dt>{{ t('waivers.requirements') }}</dt>
        <dd>
          <div v-for="q in detail.requirements" :key="q.id"><ItemRef :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: q.id } }" :item-key="q.key" :title="q.title" /></div>
          <span v-if="!detail.requirements.length" class="muted">—</span>
        </dd>
        <dt>{{ t('waivers.justification') }}</dt>
        <dd class="pre">{{ detail.justification || '—' }}</dd>
        <dt>{{ detail.type === 'deviation' ? t('waivers.solution') : t('waivers.compensating') }}</dt>
        <dd class="pre">{{ detail.compensating || '—' }}</dd>
        <dt>{{ t('waivers.conditions') }}</dt>
        <dd class="pre">{{ detail.conditions || '—' }}</dd>
        <dt>{{ t('waivers.risk') }}</dt>
        <dd>
          <RouterLink v-if="detail.risk_id" :to="{ name: 'risks', params: { projectId: proj.pid }, query: { risk: detail.risk_id } }" class="key">{{ detail.risk_key || detail.risk_id }}</RouterLink>
          <span v-else class="muted">—</span>
        </dd>
        <dt>{{ t('waivers.expires') }}</dt>
        <dd>{{ detail.expires_on ? formatDate(detail.expires_on) : t('waivers.noExpiry') }}</dd>
        <dt>{{ t('waivers.requestedBy') }}</dt>
        <dd>{{ detail.requested_by }} · {{ formatDateTime(detail.created_at) }}</dd>
        <dt>{{ t('waivers.approvers') }}</dt>
        <dd>{{ detail.approvers.length ? detail.approvers.join(', ') : t('waivers.anyApprover') }}</dd>
        <template v-if="detail.decided_by">
          <dt>{{ t('waivers.decision') }}</dt>
          <dd>{{ detail.decided_by }} · {{ detail.decided_at ? formatDateTime(detail.decided_at) : '' }}<span v-if="detail.decision_comment"> – {{ detail.decision_comment }}</span></dd>
        </template>
      </dl>
      <h3 class="plain-heading small" style="margin: 12px 0 6px">
        <i class="pi pi-pen-to-square" aria-hidden="true" /> {{ t('waivers.approvals') }}
        <span v-if="detail.status === 'requested'" class="muted"> · {{ t('waivers.progress', { approved: progress.approved, needed: progress.needed }) }}</span>
      </h3>
      <p v-if="!detail.approvals.length" class="small muted">{{ t('waivers.noApprovals') }}</p>
      <ul v-else class="plain-list approvals">
        <li v-for="a in detail.approvals" :key="a.id" class="row">
          <Tag :value="t(`esign.${a.decision}`)" :severity="a.decision === 'approve' ? 'success' : 'danger'" :icon="a.decision === 'approve' ? 'pi pi-check' : 'pi pi-times'" />
          <span class="grow small"><b>{{ a.user_name || a.user_email }}</b> · {{ formatDateTime(a.signed_at) }}<span v-if="a.comment"> – {{ a.comment }}</span></span>
          <span class="small mono" :title="a.content_hash">{{ shortHash(a.content_hash) }}</span>
          <Tag v-if="a.valid" :value="t('signoff.valid')" severity="success" icon="pi pi-check" />
          <Tag v-else :value="t('signoff.invalid')" severity="danger" icon="pi pi-exclamation-triangle" />
        </li>
      </ul>
      <Message v-if="detail.status === 'requested' && canApprove && block && block !== 'not_requested'" severity="info" :closable="false">{{ t(`waivers.block.${block}`) }}</Message>
      <ESignaturesPanel entity-type="waiver" :entity-id="detail.id" :stamp="detail.approvals.length + detail.status" />
    </template>
    <template #footer>
      <template v-if="detail">
        <Button v-if="canWrite && detail.status !== 'approved'" :label="t('common.delete')" icon="pi pi-trash" severity="danger" text style="margin-right: auto" @click="remove(detail)" />
        <Button v-if="canWrite && waiverEditable(detail)" :label="t('common.edit')" icon="pi pi-pencil" severity="secondary" outlined @click="openEdit(detail)" />
        <Button v-if="canWrite && (detail.status === 'requested' || detail.status === 'rejected')" :label="t('waivers.withdraw')" icon="pi pi-undo" severity="secondary" outlined @click="withdraw" />
        <Button v-if="canWrite && detail.status === 'draft'" :label="t('waivers.submit')" icon="pi pi-send" @click="submit" />
        <Button v-if="canApprove && detail.status === 'approved'" :label="t('waivers.revoke')" icon="pi pi-ban" severity="danger" outlined @click="(revokeReason = ''), (revokeOpen = true)" />
        <Button v-if="canApprove && block === null" :label="t('waivers.decide')" icon="pi pi-pen-to-square" @click="openSign" />
      </template>
    </template>
  </Dialog>

  <ESignDialog
    v-model:visible="signOpen"
    :header="t('waivers.signHeader', { key: detail?.key ?? '' })"
    :explain="t('waivers.signExplain', { hash: shortHash(detail?.content_hash ?? '') })"
    decisions
    comment-required="reject"
    :busy="signBusy"
    :error="signError"
    @sign="sign"
  />

  <Dialog v-model:visible="revokeOpen" :header="t('waivers.revokeHeader', { key: detail?.key ?? '' })" modal style="width: min(460px, 96vw)">
    <form class="grid" style="gap: 8px" @submit.prevent="revoke">
      <p class="small" style="margin: 0">{{ t('waivers.revokeExplain') }}</p>
      <label for="wv-revoke">{{ t('waivers.revokeReason') }}</label>
      <Textarea id="wv-revoke" v-model="revokeReason" rows="2" auto-resize aria-required="true" />
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="revokeOpen = false" />
        <Button type="submit" :label="t('waivers.revoke')" icon="pi pi-ban" severity="danger" :disabled="revokeReason.trim().length < 3" />
      </div>
    </form>
  </Dialog>

  <!-- create / edit -->
  <Dialog v-model:visible="formOpen" :header="editing ? t('waivers.editHeader', { key: editing.key }) : t('waivers.new')" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(760px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="form-grid">
        <div v-field class="field">
          <label for="wv-type">{{ t('waivers.typeLabel') }}</label>
          <Select v-model="form.type" input-id="wv-type" :options="typeOptions" option-label="label" option-value="value" />
          <small class="muted">{{ t(`waivers.typeHint.${form.type}`) }}</small>
        </div>
        <div v-field class="field">
          <label for="wv-release">{{ t('waivers.scope') }}</label>
          <Select v-model="form.release_id" input-id="wv-release" :options="releaseOptions" option-label="label" option-value="value" />
        </div>
        <div v-if="variantList.variants.value.length" class="field full">
          <label for="wv-variants">{{ t('waivers.variantsLabel') }}</label>
          <VariantMultiSelect v-model="form.variant_ids" input-id="wv-variants" :variants="variantList.variants.value" :placeholder="t('waivers.variantsAll')" aria-describedby="wv-variants-help" />
          <small id="wv-variants-help" class="muted">{{ t('waivers.variantsHelp') }}</small>
        </div>
        <div v-field class="field full"><label for="wv-title">{{ t('common.title') }}</label><InputText id="wv-title" v-model="form.title" autofocus /></div>
        <div class="field full">
          <div class="section-title">
            <i class="pi pi-list-check" aria-hidden="true" /> {{ t('waivers.requirementsCount', { count: linked.length }, linked.length) }}
            <span class="grow" />
            <Button size="small" :label="t('waivers.addRequirements')" icon="pi pi-plus" text @click="picker = true" />
          </div>
          <div v-if="!linked.length" class="muted small">{{ t('waivers.noRequirements') }}</div>
          <div v-for="q in linked" :key="q.id" class="row nowrap" style="padding: 4px 0; border-bottom: 1px solid var(--qtf-border)">
            <ItemRef :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: q.id } }" :item-key="q.key" :title="q.title" title-class="ellipsis" grow />
            <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('common.unlinkItem', { key: q.key })" @click="form.requirement_ids = form.requirement_ids.filter((x) => x !== q.id)" />
          </div>
        </div>
        <div v-field class="field full"><label for="wv-just">{{ t('waivers.justification') }}</label><Textarea id="wv-just" v-model="form.justification" rows="3" auto-resize /></div>
        <div v-field class="field full">
          <label for="wv-comp">{{ form.type === 'deviation' ? t('waivers.solution') : t('waivers.compensating') }}</label>
          <Textarea id="wv-comp" v-model="form.compensating" rows="2" auto-resize />
        </div>
        <div v-field class="field full"><label for="wv-cond">{{ t('waivers.conditions') }}</label><Textarea id="wv-cond" v-model="form.conditions" rows="2" auto-resize /></div>
        <div v-field class="field">
          <label for="wv-risk">{{ t('waivers.risk') }}</label>
          <Select v-model="form.risk" input-id="wv-risk" :options="riskOptions" option-label="label" option-value="value" filter />
        </div>
        <div v-field class="field">
          <label for="wv-expires">{{ t('waivers.expires') }}</label>
          <InputText id="wv-expires" v-model="form.expires_on" type="date" />
        </div>
        <template v-if="form.risk === 'new'">
          <div v-field class="field">
            <label for="wv-rl">{{ t('risksView.likelihood') }}</label>
            <Select v-model="form.new_risk_likelihood" input-id="wv-rl" :options="scaleOptions" option-label="label" option-value="value" />
          </div>
          <div v-field class="field">
            <label for="wv-ri">{{ t('risksView.impact') }}</label>
            <Select v-model="form.new_risk_impact" input-id="wv-ri" :options="scaleOptions" option-label="label" option-value="value" />
          </div>
        </template>
        <div v-field class="field full">
          <label for="wv-approvers">{{ t('waivers.approvers') }}</label>
          <InputText id="wv-approvers" v-model="form.approvers" :placeholder="t('waivers.approversPlaceholder')" aria-describedby="wv-approvers-hint" />
          <small id="wv-approvers-hint" class="muted">{{ t('waivers.approversHint') }}</small>
        </div>
        <label v-if="!editing" class="row nowrap field full" style="gap: 6px"><Checkbox v-model="form.submit" binary input-id="wv-submit" /> {{ t('waivers.submitNow') }}</label>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="formOpen = false" />
        <Button type="submit" :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.title.trim()" />
      </div>
    </form>
  </Dialog>
  <ItemPickerDialog v-model:visible="picker" kind="requirement" :project-id="proj.pid" :exclude="form.requirement_ids" :header="t('waivers.addRequirements')" :confirm-label="t('common.link')" @pick="addLinked" />
</template>

<style scoped>
.wv-fields {
  display: grid;
  grid-template-columns: minmax(120px, max-content) minmax(0, 1fr);
  gap: 6px 14px;
  margin: 0;
}
.wv-fields dt {
  font-weight: 600;
  color: var(--qtf-muted);
}
.wv-fields dd {
  margin: 0;
}
.pre {
  white-space: pre-wrap;
}
.approvals li {
  padding: 4px 0;
  border-bottom: 1px solid var(--qtf-border);
}
@media (max-width: 560px) {
  .wv-fields {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
