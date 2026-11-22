<script setup lang="ts">
// "My work": what needs the signed-in user across the projects they can see
// (owned and assigned items, reviews, waivers and risk acceptances waiting
// for them, overdue and flagged items), filterable by project and reason.
// Each row links to the item and says why it is listed; a row with a review
// request for the user opens the review here.
//
// The reviews view (?view=reviews) holds what was the "My reviews" page:
// review requests to the user (side=mine) or asked by them
// (side=requested), pending or all (status=all), with bulk actions, and the
// items in review status that concern them (side=in_review).
// ?review=<id> opens a review request in either view (notifications,
// discussions and the old /reviews address lead here).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import ItemRef from '@/components/ItemRef.vue'
import InReviewList from '@/components/InReviewList.vue'
import ReviewDialog from '@/components/ReviewDialog.vue'
import ReviewRequestsList from '@/components/ReviewRequestsList.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate } from '@/lib/format'
import { enumLabel } from '@/i18n/labels'
import { useNotify } from '@/composables/notify'
import {
  emptySummary,
  filterByReason,
  presentReasons,
  reasonSeverity,
  reviewsRoute,
  rowReviewId,
  workItemRoute,
  workKindIcon,
  workViewQuery,
  workViewState,
  type ReviewSide,
  type WorkItem,
  type WorkSummary,
} from '@/lib/worklist'
import { useWorkListStore } from '@/stores/worklist'
import type { Project, ReviewRequest } from '@/lib/types'

const { t, te } = useI18n()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const badge = useWorkListStore()

const items = ref<WorkItem[]>([])
const summary = ref<WorkSummary>(emptySummary())
const projects = ref<Project[]>([])
const loading = ref(false)
const error = ref('')
const project = ref(typeof route.query.project === 'string' ? route.query.project : '')
const reason = ref(typeof route.query.reason === 'string' ? route.query.reason : '')

// ---------- view: all work or reviews ----------
const state = computed(() => workViewState(route.query))
function setState(patch: Partial<ReturnType<typeof workViewState>>) {
  void router.replace({ query: { ...route.query, ...workViewQuery({ ...state.value, ...patch }) } })
}
const view = computed({ get: () => state.value.view, set: (v) => setState({ view: v }) })
const side = computed({ get: () => state.value.side, set: (v: ReviewSide) => setState({ side: v }) })
const status = computed({ get: () => state.value.status, set: (v: 'pending' | 'all') => setState({ status: v }) })
const toReview = computed(() => summary.value.by_reason.review_requested ?? 0)
const views = computed(() => [
  { label: t('workList.viewAll'), value: 'all' },
  { label: toReview.value ? t('workList.viewReviewsCount', { n: toReview.value }) : t('workList.viewReviews'), value: 'reviews' },
])
const sides = computed(() => [
  { label: t('reviews.toReview'), value: 'mine' },
  { label: t('reviews.requestedByMe'), value: 'requested' },
  { label: t('reviews.inReviewStatus'), value: 'in_review' },
])
const statuses = computed(() => [
  { label: t('reviews.filterPending'), value: 'pending' },
  { label: t('reviews.filterAll'), value: 'all' },
])
const intro = computed(() => {
  if (view.value === 'all') return t('workList.intro')
  return side.value === 'mine' ? t('reviews.intro') : side.value === 'requested' ? t('reviews.introRequested') : t('reviews.introInReview')
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ items: WorkItem[]; summary: WorkSummary }>('/my-work', project.value ? { project: project.value } : undefined)
    items.value = res.items ?? []
    summary.value = { ...emptySummary(), ...res.summary }
    if (!project.value) badge.summary = summary.value
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  void load()
  try {
    projects.value = asItems(await api.get<{ items: Project[] }>('/projects'))
  } catch {
    /* the project filter falls back to "all projects" */
  }
})
watch(project, () => {
  void router.replace({ query: { ...route.query, project: project.value || undefined } })
  void load()
})
watch(reason, () => void router.replace({ query: { ...route.query, reason: reason.value || undefined } }))

const projectOptions = computed(() => [
  { value: '', label: t('workList.allProjects') },
  ...projects.value.map((p) => ({ value: p.id, label: `${p.key} · ${p.name}` })),
])
const reasonOptions = computed(() => [
  { value: '', label: t('workList.allReasons', { n: items.value.length }) },
  ...presentReasons(summary.value).map((r) => ({ value: r, label: `${reasonLabel(r)} (${summary.value.by_reason[r]})` })),
])
const shown = computed(() => filterByReason(items.value, reason.value))

const reasonLabel = (r: string) => (te(`workList.reasons.${r}`) ? t(`workList.reasons.${r}`) : r.replace(/_/g, ' '))
const kindLabel = (k: string) => (te(`workList.kinds.${k}`) ? t(`workList.kinds.${k}`) : k)
function statusLabel(it: WorkItem): string {
  if (!it.status) return ''
  const s = it.status
  if (it.kind === 'defect' && te(`defectsView.status.${s}`)) return t(`defectsView.status.${s}`)
  if (it.kind === 'risk' && te(`risksView.status.${s}`)) return t(`risksView.status.${s}`)
  if (it.kind === 'waiver' && te(`waivers.status.${s}`)) return t(`waivers.status.${s}`)
  return enumLabel(it.kind === 'execution' ? 'result' : 'status', it.status)
}
/** the sentence that says why a row is listed */
function why(it: WorkItem): string {
  return it.reasons.map((r) => (te(`workList.why.${r}`) ? t(`workList.why.${r}`, { detail: it.detail ?? '' }) : reasonLabel(r))).join(' · ')
}

// ---------- the review dialog ----------
const current = ref<ReviewRequest | null>(null)
const working = ref(false)
const requests = ref<InstanceType<typeof ReviewRequestsList> | null>(null)
const inReview = ref<InstanceType<typeof InReviewList> | null>(null)
function work(r: ReviewRequest) {
  current.value = r
  working.value = true
}
async function openReview(id: string) {
  if (!id) return
  try {
    work(await api.get<ReviewRequest>(`/reviews/${id}`))
  } catch (e) {
    notify.fail(e)
  }
}
const routeReview = () => (typeof route.query.review === 'string' ? route.query.review : '')
onMounted(() => void openReview(routeReview()))
watch(routeReview, (id) => {
  if (id && id !== current.value?.id) void openReview(id)
})
// closing the dialog drops ?review= so a reload does not open it again
watch(working, (v) => {
  if (!v && routeReview()) void router.replace({ query: { ...route.query, review: undefined } })
})
function refresh() {
  void load()
  void requests.value?.load()
  void inReview.value?.load()
}
</script>

<template>
  <section>
    <div class="page-header">
      <div>
        <h1>{{ t('titles.myWork') }}</h1>
        <div class="sub">{{ intro }}</div>
      </div>
      <div class="actions">
        <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('workList.view')" class="view-switch" />
        <template v-if="view === 'all'">
          <Select v-model="project" :options="projectOptions" option-label="label" option-value="value" filter :placeholder="t('workList.allProjects')" :aria-label="t('workList.project')" style="min-width: 220px" />
          <Select v-model="reason" :options="reasonOptions" option-label="label" option-value="value" :placeholder="reasonOptions[0].label" :aria-label="t('workList.reason')" style="min-width: 200px" />
        </template>
      </div>
    </div>
    <template v-if="view === 'all'">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div class="summary row" role="status" aria-live="polite">
        <span><b>{{ summary.count }}</b> {{ t('workList.items', summary.count) }}</span>
        <span v-if="summary.overdue" class="error-text"><i class="pi pi-clock" aria-hidden="true" /> {{ t('workList.overdueCount', { n: summary.overdue }, summary.overdue) }}</span>
        <span v-if="summary.waiting" class="muted">{{ t('workList.waitingCount', { n: summary.waiting }, summary.waiting) }}</span>
      </div>
      <div class="card">
        <DataTable :value="shown" :loading="loading" data-key="id" size="small" striped-rows :row-class="(r: WorkItem) => (r.overdue ? 'overdue-row' : '')">
          <Column :header="t('workList.item')">
            <template #body="{ data }">
              <span class="row nowrap item">
                <i :class="workKindIcon(data.kind)" class="muted" aria-hidden="true" v-tooltip.top="kindLabel(data.kind)" />
                <span class="sr-only">{{ kindLabel(data.kind) }}:</span>
                <ItemRef :to="workItemRoute(data)" :item-key="data.key" :title="data.title" title-class="title" />
              </span>
              <div class="small muted why">{{ why(data) }}</div>
            </template>
          </Column>
          <Column :header="t('workList.whyHeader')" style="width: 260px">
            <template #body="{ data }">
              <span class="row" style="gap: 4px">
                <Tag v-for="r in data.reasons" :key="r" :value="reasonLabel(r)" :severity="reasonSeverity(r)" />
              </span>
            </template>
          </Column>
          <Column :header="t('workList.project')" style="width: 120px">
            <template #body="{ data }"><span class="small" :title="data.project_name">{{ data.project_key }}</span></template>
          </Column>
          <Column :header="t('common.status')" style="width: 130px">
            <template #body="{ data }"><span class="small">{{ statusLabel(data) }}</span></template>
          </Column>
          <Column :header="t('workList.due')" style="width: 110px">
            <template #body="{ data }">
              <span v-if="data.due" class="small" :class="{ 'error-text late': data.overdue }">{{ formatDate(data.due) }}</span>
              <span v-if="data.overdue" class="sr-only">{{ t('workList.reasons.overdue') }}</span>
            </template>
          </Column>
          <Column style="width: 120px">
            <template #header><span class="sr-only">{{ t('common.actions') }}</span></template>
            <template #body="{ data }">
              <Button
                v-if="rowReviewId(data)"
                class="row-review"
                :label="t('reviews.start')"
                icon="pi pi-eye"
                size="small"
                aria-haspopup="dialog"
                :aria-label="t('workList.reviewItem', { key: data.key })"
                @click="openReview(rowReviewId(data))"
              />
              <RouterLink v-else-if="data.requested_review_ids?.length" :to="reviewsRoute(undefined, 'requested')" class="small">{{ t('workList.myRequests') }}</RouterLink>
            </template>
          </Column>
          <template #empty>
            <div class="empty">{{ reason || project ? t('workList.noneFiltered') : t('workList.none') }}</div>
          </template>
        </DataTable>
      </div>
    </template>
    <template v-else>
      <div class="row review-filters">
        <SelectButton v-model="side" :options="sides" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('reviews.side')" class="review-side" />
        <SelectButton v-if="side !== 'in_review'" v-model="status" :options="statuses" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('reviews.filter')" />
      </div>
      <InReviewList v-if="side === 'in_review'" ref="inReview" />
      <ReviewRequestsList v-else ref="requests" :side="side" :status="status" @work="work" @changed="load" />
    </template>
    <ReviewDialog v-model:visible="working" :review="current" show-item @saved="refresh" />
  </section>
</template>

<style scoped>
.summary {
  gap: 16px;
  margin: 0 0 10px;
}
.item {
  gap: 6px;
  min-width: 0;
}
.title {
  margin-left: 6px;
  overflow-wrap: anywhere;
}
.why {
  margin-top: 2px;
}
.late {
  font-weight: 600;
}
.review-filters {
  gap: 12px;
  margin: 0 0 12px;
  flex-wrap: wrap;
}
</style>
