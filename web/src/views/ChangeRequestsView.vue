<script setup lang="ts">
// Change requests: formal change control of requirements and tests. A
// change request proposes new field values (proposed in the editors or in
// bulk), is assessed (its transitive impact is stored), decided by the
// change control board with electronic signatures, applied atomically,
// verified and closed.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ChangeDiff from '@/components/ChangeDiff.vue'
import ESignDialog from '@/components/ESignDialog.vue'
import ImpactTree from '@/components/ImpactTree.vue'
import ItemRef from '@/components/ItemRef.vue'
import StatusTag from '@/components/StatusTag.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { shortHash } from '@/lib/notifications'
import { P } from '@/lib/permissions'
import { generateBody, reportFileName } from '@/lib/reports'
import { currentLang } from '@/i18n'
import {
  boardProgress,
  crActions,
  crDecisionBlock,
  crOpen,
  crStatusIcon,
  crStatusSeverity,
  crStepIndex,
  CR_CATEGORIES,
  CR_DECISIONS,
  CR_STATUSES,
  CR_STEPS,
  filterCRs,
  onBoard,
  type ChangeControlSettings,
  type ChangeRequest,
} from '@/lib/changeRequests'
import { enumLabel, enumOptions } from '@/i18n/labels'
import type { Defect, Risk } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'

const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const prefs = usePrefsStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const canCreate = computed(() => auth.can(P.changeCreate))
const canApprove = computed(() => auth.can(P.changeApprove))

const statusLabel = (v: string) => t(`changeRequests.status.${v}`)
const categoryLabel = (v: string) => t(`changeRequests.category.${v}`)
const impactLabel = (v: string) => t(`changeRequests.impactAction.${v}`)

// ---------- data ----------
const list = ref<ChangeRequest[]>([])
const settings = ref<ChangeControlSettings | null>(null)
const defects = ref<Defect[]>([])
const risks = ref<Risk[]>([])
const loading = ref(false)
const error = ref('')
async function load() {
  loading.value = true
  error.value = ''
  try {
    list.value = asItems(await api.get<{ items: ChangeRequest[] }>(`/projects/${proj.pid}/change-requests`))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
async function loadContext() {
  const [cc, d, r] = await Promise.allSettled([
    api.get<{ settings: ChangeControlSettings }>(`/projects/${proj.pid}/change-control`),
    api.get<{ items: Defect[] }>(`/projects/${proj.pid}/defects`),
    api.get<{ items: Risk[] }>(`/projects/${proj.pid}/risks`),
  ])
  settings.value = cc.status === 'fulfilled' ? cc.value.settings : null
  defects.value = d.status === 'fulfilled' ? asItems(d.value) : []
  risks.value = r.status === 'fulfilled' ? asItems(r.value) : []
}
onMounted(async () => {
  await Promise.all([load(), loadContext()])
  fromRoute()
})

// ---------- filters ----------
const text = ref('')
const statuses = ref<string[]>([])
const categories = ref<string[]>([])
const openOnly = ref(false)
const filtered = computed(() => filterCRs(list.value, { text: text.value, statuses: statuses.value, categories: categories.value, openOnly: openOnly.value }))
const statusOptions = computed(() => CR_STATUSES.map((v) => ({ label: statusLabel(v), value: v })))
const categoryOptions = computed(() => CR_CATEGORIES.map((v) => ({ label: categoryLabel(v), value: v })))
const priorityOptions = computed(() => enumOptions('priority', meta.meta.priorities))
const impactOptions = computed(() => ['suspect', 'notify', 'skip'].map((v) => ({ label: impactLabel(v), value: v })))
const hasFilter = computed(() => !!text.value.trim() || !!statuses.value.length || !!categories.value.length || openOnly.value)
function clearFilters() {
  text.value = ''
  statuses.value = []
  categories.value = []
  openOnly.value = false
}
const counts = computed(() => ({
  open: list.value.filter((c) => crOpen(c.status)).length,
  review: list.value.filter((c) => c.status === 'in_review').length,
  approved: list.value.filter((c) => c.status === 'approved').length,
}))
const modeText = computed(() => (settings.value ? t(`changeRequests.settings.mode.${settings.value.mode}`) : ''))

// ---------- detail ----------
const detailOpen = ref(false)
const detail = ref<ChangeRequest | null>(null)
async function openDetail(c: ChangeRequest) {
  detail.value = c
  detailOpen.value = true
  if (route.query.cr !== c.id) router.replace({ query: { ...route.query, cr: c.id } })
  try {
    replaceCR(await api.get<ChangeRequest>(`/change-requests/${c.id}`))
  } catch (e) {
    notify.fail(e)
  }
}
watch(detailOpen, (v) => {
  if (!v && route.query.cr) router.replace({ query: { ...route.query, cr: undefined } })
})
function fromRoute() {
  const id = route.query.cr
  if (typeof id === 'string' && detail.value?.id !== id) {
    const c = list.value.find((x) => x.id === id || x.key === id)
    if (c) void openDetail(c)
  }
  if (route.query.new === '1' && canCreate.value) {
    openCreate()
    router.replace({ query: { ...route.query, new: undefined } })
  }
}
watch(() => route.query.cr, fromRoute)
function replaceCR(c: ChangeRequest) {
  list.value = list.value.some((x) => x.id === c.id) ? list.value.map((x) => (x.id === c.id ? c : x)) : [c, ...list.value]
  if (detail.value?.id === c.id) detail.value = c
}
const actions = computed(() => (detail.value ? crActions(detail.value, { create: canCreate.value }) : []))
const has = (a: string) => actions.value.includes(a)
const board = computed(() => settings.value?.board ?? { users: [], roles: [], quorum: 'all' })
const myRoles = computed(() => (proj.project?.my_access?.roles?.length ? proj.project.my_access.roles : auth.me?.roles ?? []))
const block = computed(() => (detail.value ? crDecisionBlock(detail.value, auth.user?.email, onBoard(board.value, auth.user?.email, myRoles.value)) : 'not_in_review'))
const progress = computed(() => (detail.value ? boardProgress(board.value, detail.value) : { approved: 0, needed: 1 }))
const stepIndex = computed(() => (detail.value ? crStepIndex(detail.value.status) : 0))
const boardText = computed(() => {
  const b = board.value
  const who = [...b.users, ...b.roles.map((r) => t('changeRequests.roleMembers', { role: enumLabel('role', r) }))]
  return who.length ? who.join(', ') : t('changeRequests.anyApprover')
})
const quorumText = computed(() => {
  const q = board.value.quorum
  return q === 'all' || q === 'any' ? t(`changeRequests.quorum.${q}`) : t('changeRequests.quorum.n', { n: q })
})
function eventLabel(a: string) {
  if (a.startsWith('signed.')) return t('changeRequests.event.signed', { decision: t(`esign.${a.slice(7)}`) })
  return t(`changeRequests.event.${a}`)
}

async function act(path: string, body: unknown, done: string) {
  if (!detail.value) return false
  try {
    const c = await api.post<ChangeRequest>(`/change-requests/${detail.value.id}/${path}`, body)
    replaceCR(c)
    notify.ok(done, c.key)
    return true
  } catch (e) {
    notify.fail(e)
    return false
  }
}
const submit = () => act('submit', {}, t('changeRequests.submitted'))
const review = () => act('review', {}, t('changeRequests.sentToBoard'))
const withdraw = () => act('withdraw', {}, t('changeRequests.withdrawn'))
async function removeItem(itemId: string) {
  if (!detail.value) return
  try {
    replaceCR(await api.del<ChangeRequest>(`/change-requests/${detail.value.id}/items/${itemId}`))
  } catch (e) {
    notify.fail(e)
  }
}
function remove(c: ChangeRequest) {
  notify.confirmDanger(t('changeRequests.confirmDelete', { key: c.key, title: c.title }), async () => {
    try {
      await api.del(`/change-requests/${c.id}`)
      notify.ok(t('changeRequests.deleted'))
      detailOpen.value = false
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// assess
const assessOpen = ref(false)
const assessNotes = ref('')
const assessEffort = ref('')
const assessReview = ref(true)
function openAssess() {
  assessNotes.value = detail.value?.impact_notes ?? ''
  assessEffort.value = detail.value?.effort ?? ''
  assessReview.value = true
  assessOpen.value = true
}
async function assess() {
  if (await act('assess', { notes: assessNotes.value, effort: assessEffort.value, review: assessReview.value }, t('changeRequests.assessed'))) assessOpen.value = false
}

// decide (electronic signature)
const signOpen = ref(false)
const signBusy = ref(false)
const signError = ref('')
async function sign(v: { decision: string; comment: string; password: string }) {
  if (!detail.value) return
  signBusy.value = true
  signError.value = ''
  try {
    const c = await api.post<ChangeRequest>(`/change-requests/${detail.value.id}/decision`, v)
    replaceCR(c)
    signOpen.value = false
    notify.ok(t(`changeRequests.signed.${v.decision}`), c.key)
  } catch (e) {
    if (e instanceof ApiError && (e.code === 'reauth_failed' || e.code === 'reauth_required')) signError.value = t(`esign.${e.code}`)
    else signError.value = errorMessage(e)
  } finally {
    signBusy.value = false
  }
}

// apply
const applyOpen = ref(false)
const applyImpact = ref('suspect')
function openApply() {
  applyImpact.value = detail.value?.impact_action ?? 'suspect'
  applyOpen.value = true
}
async function applyCR() {
  if (await act('apply', { impact: applyImpact.value }, t('changeRequests.applied'))) applyOpen.value = false
}

// verify / close
const verifyOpen = ref(false)
const verifyComment = ref('')
async function verify() {
  if (await act('verify', { comment: verifyComment.value.trim() }, t('changeRequests.verified'))) verifyOpen.value = false
}
const closeOpen = ref(false)
const closeBaseline = ref(false)
const closeName = ref('')
const closeComment = ref('')
function openClose() {
  closeBaseline.value = detail.value?.status === 'verified' && !!detail.value.baseline_on_close
  closeName.value = ''
  closeComment.value = ''
  closeOpen.value = true
}
async function closeCR() {
  if (await act('close', { create_baseline: closeBaseline.value, baseline_name: closeName.value.trim(), comment: closeComment.value.trim() }, t('changeRequests.closed'))) closeOpen.value = false
}

// report
async function exportPdf() {
  if (!detail.value) return
  try {
    const body = generateBody({ template: 'change_requests', params: { change_request: detail.value.key } }, 'pdf', false, undefined, currentLang())
    const file = await api.postDownload(`/projects/${proj.pid}/reports/generate`, body, reportFileName(detail.value.key, 'pdf'))
    notify.ok(t('reports.downloaded'), file)
  } catch (e) {
    notify.fail(e)
  }
}

// ---------- create / edit ----------
interface Form {
  title: string
  reason: string
  category: string
  priority: string
  impact_action: string
  defect_ids: string[]
  risk_ids: string[]
  issues: string
  baseline_on_close: boolean
}
const blank = (): Form => ({ title: '', reason: '', category: 'correction', priority: 'medium', impact_action: 'suspect', defect_ids: [], risk_ids: [], issues: '', baseline_on_close: settings.value?.baseline_on_close ?? false })
const formOpen = ref(false)
const editing = ref<ChangeRequest | null>(null)
const form = ref<Form>(blank())
const formError = ref('')
const busy = ref(false)
const defectOptions = computed(() => defects.value.map((d) => ({ label: `${d.key} ${d.title}`, value: d.id })))
const riskOptions = computed(() => risks.value.map((r) => ({ label: `${r.key} ${r.title}`, value: r.id })))
function openCreate() {
  editing.value = null
  form.value = blank()
  formError.value = ''
  formOpen.value = true
}
function openEdit(c: ChangeRequest) {
  editing.value = c
  form.value = {
    title: c.title,
    reason: c.reason,
    category: c.category,
    priority: c.priority,
    impact_action: c.impact_action,
    defect_ids: [...c.links.defect_ids],
    risk_ids: [...c.links.risk_ids],
    issues: c.links.issues.join(', '),
    baseline_on_close: c.baseline_on_close,
  }
  formError.value = ''
  formOpen.value = true
}
async function save() {
  busy.value = true
  formError.value = ''
  const f = form.value
  const body = {
    title: f.title.trim(),
    reason: f.reason,
    category: f.category,
    priority: f.priority,
    impact_action: f.impact_action,
    baseline_on_close: f.baseline_on_close,
    links: { defect_ids: f.defect_ids, risk_ids: f.risk_ids, issues: f.issues.split(/[,;\n]+/).map((s) => s.trim()).filter(Boolean) },
  }
  try {
    const c = editing.value ? await api.patch<ChangeRequest>(`/change-requests/${editing.value.id}`, body) : await api.post<ChangeRequest>(`/projects/${proj.pid}/change-requests`, body)
    notify.ok(editing.value ? t('changeRequests.updated') : t('changeRequests.created'), c.key)
    formOpen.value = false
    replaceCR(c)
    void openDetail(c)
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
const itemRoute = (kind: string, id: string) => ({ name: kind === 'test' ? 'tests' : 'requirements', params: { projectId: proj.pid }, query: { item: id } })
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('changeRequests.title') }}</h1>
      <div class="sub">{{ t('changeRequests.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="change-requests" />
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
      <Button v-if="canCreate" :label="t('changeRequests.new')" icon="pi pi-plus" @click="openCreate()" />
    </div>
  </div>

  <Message v-if="settings" :severity="settings.mode === 'enforce' ? 'warn' : 'info'" :closable="false">
    <i class="pi pi-lock" aria-hidden="true" /> {{ t('changeRequests.modeNote', { mode: modeText }) }}
    <RouterLink :to="{ name: 'settings', params: { projectId: proj.pid }, query: { tab: 'project' }, hash: '#change-control' }">{{ t('changeRequests.settingsLink') }}</RouterLink>
  </Message>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <section v-else class="card" aria-labelledby="cr-list-title">
    <h2 id="cr-list-title" class="sr-only">{{ t('changeRequests.list') }}</h2>
    <div class="row small" style="margin-bottom: 8px; gap: 14px">
      <span><b>{{ counts.open }}</b> {{ t('changeRequests.countOpen') }}</span>
      <span><b>{{ counts.review }}</b> {{ t('changeRequests.countReview') }}</span>
      <span><b>{{ counts.approved }}</b> {{ t('changeRequests.countApproved') }}</span>
    </div>
    <div class="filterbar">
      <InputText v-model="text" :placeholder="t('changeRequests.searchPlaceholder')" :aria-label="t('changeRequests.searchPlaceholder')" style="min-width: 200px" />
      <MultiSelect v-model="statuses" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('common.status')" :aria-label="t('common.status')" show-clear style="min-width: 140px" />
      <MultiSelect v-model="categories" :options="categoryOptions" option-label="label" option-value="value" :placeholder="t('changeRequests.categoryLabel')" :aria-label="t('changeRequests.categoryLabel')" show-clear style="min-width: 140px" />
      <label class="row nowrap small" style="gap: 6px"><Checkbox v-model="openOnly" binary input-id="cr-open" /> {{ t('changeRequests.openOnly') }}</label>
      <Button v-if="hasFilter" :label="t('common.clear')" text size="small" severity="secondary" @click="clearFilters" />
      <span class="small muted" role="status">{{ t('changeRequests.shown', { count: filtered.length, total: list.length }) }}</span>
    </div>
    <DataTable v-stack :value="filtered" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(25)" :always-show-paginator="false" row-hover size="small" @row-click="(e) => openDetail(e.data as ChangeRequest)">
      <Column field="key" :header="t('common.key')" sortable style="width: 120px"><template #body="{ data }"><span class="key">{{ data.key }}</span></template></Column>
      <Column field="title" :header="t('common.title')" sortable body-class="stack-main" style="min-width: 200px">
        <template #body="{ data }">
          <button type="button" class="link-button" style="font-weight: 600" :aria-label="t('changeRequests.open', { key: data.key, title: data.title })" @click.stop="openDetail(data as ChangeRequest)">{{ data.title }}</button>
          <Tag v-if="data.outdated" :value="t('changeRequests.outdatedTag')" severity="warn" icon="pi pi-exclamation-triangle" style="margin-left: 6px" />
        </template>
      </Column>
      <Column field="status" :header="t('common.status')" sortable style="width: 150px">
        <template #body="{ data }"><Tag :value="statusLabel(data.status)" :severity="crStatusSeverity(data.status)" :icon="crStatusIcon(data.status)" /></template>
      </Column>
      <Column field="category" :header="t('changeRequests.categoryLabel')" sortable style="width: 130px"><template #body="{ data }">{{ categoryLabel(data.category) }}</template></Column>
      <Column field="priority" :header="t('common.priority')" sortable style="width: 110px"><template #body="{ data }"><StatusTag :value="data.priority" kind="priority" /></template></Column>
      <Column :header="t('changeRequests.items')" style="min-width: 160px">
        <template #body="{ data }">
          <span class="row small" style="gap: 4px">
            <RouterLink v-for="it in (data as ChangeRequest).items" :key="it.id" :to="itemRoute(it.entity_type, it.entity_id)" class="key" :title="it.title" @click.stop>{{ it.key }}</RouterLink>
          </span>
        </template>
      </Column>
      <Column field="created_by" :header="t('changeRequests.requestedBy')" sortable style="width: 170px"><template #body="{ data }"><span class="small">{{ data.created_by }}</span></template></Column>
      <template #empty>
        <div class="empty"><i class="pi pi-file-edit" />{{ list.length ? t('changeRequests.noneMatch') : t('changeRequests.none') }}</div>
      </template>
    </DataTable>
  </section>

  <!-- detail -->
  <Dialog v-model:visible="detailOpen" :header="detail ? `${detail.key} ${detail.title}` : ''" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(900px, 96vw)">
    <template v-if="detail">
      <div class="row" style="margin-bottom: 8px">
        <Tag :value="statusLabel(detail.status)" :severity="crStatusSeverity(detail.status)" :icon="crStatusIcon(detail.status)" />
        <Tag :value="categoryLabel(detail.category)" severity="secondary" />
        <StatusTag :value="detail.priority" kind="priority" />
        <Tag v-if="detail.outdated" :value="t('changeRequests.outdatedTag')" severity="warn" icon="pi pi-exclamation-triangle" />
      </div>
      <ol class="cr-steps" :aria-label="t('changeRequests.workflow')">
        <li v-for="(s, i) in CR_STEPS" :key="s" :class="{ done: i < stepIndex, current: i === stepIndex }" :aria-current="i === stepIndex ? 'step' : undefined">
          <i :class="i < stepIndex ? 'pi pi-check' : crStatusIcon(s)" aria-hidden="true" /> {{ i === stepIndex ? statusLabel(detail.status) : statusLabel(s) }}
        </li>
      </ol>
      <Message v-if="detail.outdated" severity="warn" :closable="false">{{ t('changeRequests.outdatedHint') }}</Message>
      <dl class="cr-fields">
        <dt>{{ t('changeRequests.reason') }}</dt>
        <dd class="pre">{{ detail.reason || '—' }}</dd>
        <dt>{{ t('changeRequests.requestedBy') }}</dt>
        <dd>{{ detail.created_by }} · {{ formatDateTime(detail.created_at) }}</dd>
        <dt>{{ t('changeRequests.impactActionLabel') }}</dt>
        <dd>{{ impactLabel(detail.impact_action) }}</dd>
        <template v-if="detail.defects.length || detail.risks.length || detail.links.issues.length">
          <dt>{{ t('changeRequests.links') }}</dt>
          <dd class="row" style="gap: 6px">
            <RouterLink v-for="d in detail.defects" :key="d.id" :to="{ name: 'defects', params: { projectId: proj.pid }, query: { item: d.id } }" class="key" :title="d.title">{{ d.key }}</RouterLink>
            <RouterLink v-for="r in detail.risks" :key="r.id" :to="{ name: 'risks', params: { projectId: proj.pid }, query: { risk: r.id } }" class="key" :title="r.title">{{ r.key }}</RouterLink>
            <span v-for="i in detail.links.issues" :key="i" class="key">{{ i }}</span>
          </dd>
        </template>
        <template v-if="detail.assessed_by">
          <dt>{{ t('changeRequests.assessment') }}</dt>
          <dd>
            {{ detail.assessed_by }} · {{ detail.assessed_at ? formatDateTime(detail.assessed_at) : '' }}
            <span v-if="detail.effort"> · {{ t('changeRequests.effort') }}: {{ detail.effort }}</span>
            <div v-if="detail.impact_notes" class="pre small">{{ detail.impact_notes }}</div>
          </dd>
        </template>
        <template v-if="detail.decided_by">
          <dt>{{ t('changeRequests.decision') }}</dt>
          <dd>{{ detail.decided_by }} · {{ detail.decided_at ? formatDateTime(detail.decided_at) : '' }}<span v-if="detail.decision_comment"> – {{ detail.decision_comment }}</span></dd>
        </template>
        <template v-if="detail.baseline_id">
          <dt>{{ t('changeRequests.baseline') }}</dt>
          <dd><RouterLink :to="{ name: 'baselines', params: { projectId: proj.pid }, query: { b: detail.baseline_id } }">{{ t('changeRequests.openBaseline') }}</RouterLink></dd>
        </template>
      </dl>

      <h3 class="plain-heading small cr-h"><i class="pi pi-file-edit" aria-hidden="true" /> {{ t('changeRequests.proposedChanges', { n: detail.items.length }, detail.items.length) }}</h3>
      <p v-if="!detail.items.length" class="small muted">{{ t('changeRequests.noItems') }}</p>
      <div v-for="it in detail.items" :key="it.id" class="cr-item">
        <div class="row nowrap">
          <ItemRef :to="itemRoute(it.entity_type, it.entity_id)" :item-key="it.key || it.entity_id.slice(0, 8)" :title="it.title" title-class="ellipsis" grow />
          <span class="small muted">{{ t('changeRequests.versions', { base: it.base_version, current: it.current_version }) }}</span>
          <Tag v-if="it.applied_version" :value="t('changeRequests.appliedAs', { version: it.applied_version })" severity="success" icon="pi pi-check" />
          <Tag v-else-if="it.outdated" :value="t('changeRequests.outdatedTag')" severity="warn" icon="pi pi-exclamation-triangle" />
          <Button v-if="has('edit')" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('changeRequests.removeItem', { key: it.key })" @click="removeItem(it.id)" />
        </div>
        <ChangeDiff :item="it" />
      </div>
      <p v-if="has('edit')" class="small muted">{{ t('changeRequests.addHint') }}</p>

      <h3 class="plain-heading small cr-h"><i class="pi pi-sitemap" aria-hidden="true" /> {{ t('changeRequests.impact') }}</h3>
      <p v-if="!detail.impact" class="small muted">{{ t('changeRequests.noImpact') }}</p>
      <template v-else>
        <p class="small muted" style="margin: 0 0 6px">{{ t('changeRequests.impactSnapshot', { n: detail.impact.total }) }}</p>
        <ImpactTree :impact="detail.impact" :project-id="proj.pid" :heading-level="4" />
      </template>

      <h3 class="plain-heading small cr-h">
        <i class="pi pi-pen-to-square" aria-hidden="true" /> {{ t('changeRequests.approvals') }}
        <span v-if="detail.status === 'in_review'" class="muted"> · {{ t('changeRequests.progress', { approved: progress.approved, needed: progress.needed }) }}</span>
      </h3>
      <p class="small muted" style="margin: 0 0 6px">{{ t('changeRequests.boardLine', { who: boardText, quorum: quorumText }) }}</p>
      <p v-if="!detail.approvals.length" class="small muted">{{ t('changeRequests.noApprovals') }}</p>
      <ul v-else class="plain-list cr-approvals">
        <li v-for="a in detail.approvals" :key="a.id" class="row">
          <Tag :value="t(`esign.${a.decision}`)" :severity="a.decision === 'approve' ? 'success' : a.decision === 'reject' ? 'danger' : 'secondary'" :icon="a.decision === 'approve' ? 'pi pi-check' : a.decision === 'reject' ? 'pi pi-times' : 'pi pi-pause'" />
          <span class="grow small">
            <b>{{ a.user_name || a.user_email }}</b> · {{ formatDateTime(a.signed_at) }}<span v-if="a.comment"> – {{ a.comment }}</span>
            <span v-if="a.meaning" class="muted"><br />{{ a.meaning }}</span>
          </span>
          <span class="small mono" :title="a.content_hash">{{ shortHash(a.content_hash) }}</span>
          <Tag v-if="a.valid" :value="t('signoff.valid')" severity="success" icon="pi pi-check" />
          <Tag v-else :value="t('signoff.invalid')" severity="danger" icon="pi pi-exclamation-triangle" />
        </li>
      </ul>
      <Message v-if="detail.status === 'in_review' && canApprove && block && block !== 'not_in_review'" severity="info" :closable="false">{{ t(`changeRequests.block.${block}`) }}</Message>

      <h3 class="plain-heading small cr-h"><i class="pi pi-history" aria-hidden="true" /> {{ t('changeRequests.timeline') }}</h3>
      <ol class="plain-list cr-timeline">
        <li v-for="e in detail.events" :key="e.id" class="small">
          <span class="muted">{{ formatDateTime(e.at) }}</span> · <b>{{ eventLabel(e.action) }}</b> · {{ e.actor }}<span v-if="e.comment"> – {{ e.comment }}</span>
        </li>
      </ol>
    </template>
    <template #footer>
      <template v-if="detail">
        <Button v-if="has('delete')" :label="t('common.delete')" icon="pi pi-trash" severity="danger" text @click="remove(detail)" />
        <Button :label="t('changeRequests.exportPdf')" icon="pi pi-file-pdf" severity="secondary" text style="margin-right: auto" @click="exportPdf" />
        <Button v-if="has('edit')" :label="t('common.edit')" icon="pi pi-pencil" severity="secondary" outlined @click="openEdit(detail)" />
        <Button v-if="has('withdraw')" :label="t('changeRequests.withdraw')" icon="pi pi-undo" severity="secondary" outlined @click="withdraw" />
        <Button v-if="has('submit')" :label="t('changeRequests.submit')" icon="pi pi-send" @click="submit" />
        <Button v-if="has('assess')" :label="t('changeRequests.assess')" icon="pi pi-sitemap" :outlined="detail.status === 'assessed'" @click="openAssess" />
        <Button v-if="has('review')" :label="t('changeRequests.sendToBoard')" icon="pi pi-users" @click="review" />
        <Button v-if="canApprove && block === null" :label="t('changeRequests.decide')" icon="pi pi-pen-to-square" @click="(signError = ''), (signOpen = true)" />
        <Button v-if="has('apply')" :label="t('changeRequests.apply')" icon="pi pi-cog" @click="openApply" />
        <Button v-if="has('verify')" :label="t('changeRequests.verify')" icon="pi pi-verified" @click="(verifyComment = ''), (verifyOpen = true)" />
        <Button v-if="has('close')" :label="t('changeRequests.close')" icon="pi pi-lock" :outlined="detail.status !== 'verified'" @click="openClose" />
      </template>
    </template>
  </Dialog>

  <ESignDialog
    v-model:visible="signOpen"
    :header="t('changeRequests.signHeader', { key: detail?.key ?? '' })"
    :explain="t('changeRequests.signExplain', { hash: shortHash(detail?.content_hash ?? '') })"
    :meaning="settings?.signature_meaning || t('changeRequests.defaultMeaning')"
    decisions
    :choices="[...CR_DECISIONS]"
    comment-required="reject"
    :busy="signBusy"
    :error="signError"
    @sign="sign"
  />

  <Dialog v-model:visible="assessOpen" :header="t('changeRequests.assessHeader', { key: detail?.key ?? '' })" modal style="width: min(520px, 96vw)">
    <form class="grid" style="gap: 8px" @submit.prevent="assess">
      <p class="small" style="margin: 0">{{ t('changeRequests.assessExplain') }}</p>
      <label for="cr-notes">{{ t('changeRequests.assessNotes') }}</label>
      <Textarea id="cr-notes" v-model="assessNotes" rows="3" auto-resize />
      <label for="cr-effort">{{ t('changeRequests.effort') }}</label>
      <InputText id="cr-effort" v-model="assessEffort" :placeholder="t('changeRequests.effortPlaceholder')" />
      <label class="row nowrap small" style="gap: 6px"><Checkbox v-model="assessReview" binary input-id="cr-assess-review" /> {{ t('changeRequests.assessAndSend') }}</label>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="assessOpen = false" />
        <Button type="submit" :label="t('changeRequests.assess')" icon="pi pi-sitemap" />
      </div>
    </form>
  </Dialog>

  <Dialog v-model:visible="applyOpen" :header="t('changeRequests.applyHeader', { key: detail?.key ?? '' })" modal style="width: min(520px, 96vw)">
    <form class="grid" style="gap: 8px" @submit.prevent="applyCR">
      <p class="small" style="margin: 0">{{ t('changeRequests.applyExplain', { n: detail?.items.length ?? 0 }) }}</p>
      <span id="cr-apply-impact" class="field-label">{{ t('changeRequests.impactActionLabel') }}</span>
      <SelectButton v-model="applyImpact" :options="impactOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="cr-apply-impact" aria-describedby="cr-apply-hint" />
      <small id="cr-apply-hint" class="muted">{{ t(`changeRequests.impactHint.${applyImpact}`) }}</small>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="applyOpen = false" />
        <Button type="submit" :label="t('changeRequests.apply')" icon="pi pi-cog" />
      </div>
    </form>
  </Dialog>

  <Dialog v-model:visible="verifyOpen" :header="t('changeRequests.verifyHeader', { key: detail?.key ?? '' })" modal style="width: min(460px, 96vw)">
    <form class="grid" style="gap: 8px" @submit.prevent="verify">
      <label for="cr-verify">{{ t('changeRequests.verifyComment') }}</label>
      <Textarea id="cr-verify" v-model="verifyComment" rows="2" auto-resize />
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="verifyOpen = false" />
        <Button type="submit" :label="t('changeRequests.verify')" icon="pi pi-verified" />
      </div>
    </form>
  </Dialog>

  <Dialog v-model:visible="closeOpen" :header="t('changeRequests.closeHeader', { key: detail?.key ?? '' })" modal style="width: min(460px, 96vw)">
    <form class="grid" style="gap: 8px" @submit.prevent="closeCR">
      <label v-if="detail?.status === 'verified'" class="row nowrap small" style="gap: 6px"><Checkbox v-model="closeBaseline" binary input-id="cr-close-bl" /> {{ t('changeRequests.createBaseline') }}</label>
      <template v-if="closeBaseline">
        <label for="cr-close-name">{{ t('changeRequests.baselineName') }}</label>
        <InputText id="cr-close-name" v-model="closeName" :placeholder="t('changeRequests.baselineNamePlaceholder', { key: detail?.key ?? '' })" />
      </template>
      <label for="cr-close-comment">{{ t('esign.comment') }}</label>
      <Textarea id="cr-close-comment" v-model="closeComment" rows="2" auto-resize />
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="closeOpen = false" />
        <Button type="submit" :label="t('changeRequests.close')" icon="pi pi-lock" />
      </div>
    </form>
  </Dialog>

  <!-- create / edit -->
  <Dialog v-model:visible="formOpen" :header="editing ? t('changeRequests.editHeader', { key: editing.key }) : t('changeRequests.new')" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(720px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <p v-if="!editing" class="small muted" style="margin: 0">{{ t('changeRequests.createHint') }}</p>
      <div class="form-grid">
        <div v-field class="field full"><label for="cr-title">{{ t('common.title') }}</label><InputText id="cr-title" v-model="form.title" autofocus /></div>
        <div v-field class="field full"><label for="cr-reason">{{ t('changeRequests.reason') }}</label><Textarea id="cr-reason" v-model="form.reason" rows="3" auto-resize /></div>
        <div v-field class="field">
          <label for="cr-cat">{{ t('changeRequests.categoryLabel') }}</label>
          <Select v-model="form.category" input-id="cr-cat" :options="categoryOptions" option-label="label" option-value="value" />
        </div>
        <div v-field class="field">
          <label for="cr-prio">{{ t('common.priority') }}</label>
          <Select v-model="form.priority" input-id="cr-prio" :options="priorityOptions" option-label="label" option-value="value" />
        </div>
        <div v-field class="field">
          <label for="cr-impact">{{ t('changeRequests.impactActionLabel') }}</label>
          <Select v-model="form.impact_action" input-id="cr-impact" :options="impactOptions" option-label="label" option-value="value" />
        </div>
        <div v-field class="field">
          <label for="cr-defects">{{ t('changeRequests.defects') }}</label>
          <MultiSelect v-model="form.defect_ids" input-id="cr-defects" :options="defectOptions" option-label="label" option-value="value" filter display="chip" />
        </div>
        <div v-field class="field">
          <label for="cr-risks">{{ t('changeRequests.risks') }}</label>
          <MultiSelect v-model="form.risk_ids" input-id="cr-risks" :options="riskOptions" option-label="label" option-value="value" filter display="chip" />
        </div>
        <div v-field class="field">
          <label for="cr-issues">{{ t('changeRequests.issues') }}</label>
          <InputText id="cr-issues" v-model="form.issues" :placeholder="t('changeRequests.issuesPlaceholder')" />
        </div>
        <label class="row nowrap field full" style="gap: 6px"><Checkbox v-model="form.baseline_on_close" binary input-id="cr-bl" /> {{ t('changeRequests.baselineOnClose') }}</label>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="formOpen = false" />
        <Button type="submit" :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.title.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.cr-fields {
  display: grid;
  grid-template-columns: minmax(120px, max-content) minmax(0, 1fr);
  gap: 6px 14px;
  margin: 8px 0 0;
}
.cr-fields dt {
  font-weight: 600;
  color: var(--qtf-muted);
}
.cr-fields dd {
  margin: 0;
}
.pre {
  white-space: pre-wrap;
}
.cr-h {
  margin: 16px 0 6px;
}
.cr-item {
  padding: 6px 0 10px;
  border-bottom: 1px solid var(--qtf-border);
}
.cr-approvals li,
.cr-timeline li {
  padding: 4px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.cr-steps {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  list-style: none;
  padding: 0;
  margin: 0 0 8px;
}
.cr-steps li {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 99px;
  border: 1px solid var(--qtf-border);
  color: var(--qtf-muted);
}
.cr-steps li.done {
  color: var(--qtf-text);
}
.cr-steps li.current {
  color: var(--qtf-text);
  font-weight: 700;
  border-color: var(--p-primary-color);
}
@media (max-width: 560px) {
  .cr-fields {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
