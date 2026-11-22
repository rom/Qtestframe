<script setup lang="ts">
// "My work": what needs the signed-in user across the projects they can see
// (owned and assigned items, reviews, waivers and risk acceptances waiting
// for them, overdue and flagged items), filterable by project and reason.
// Each row links to the item and says why it is listed.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ItemRef from '@/components/ItemRef.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate } from '@/lib/format'
import { enumLabel } from '@/i18n/labels'
import { emptySummary, filterByReason, presentReasons, reasonSeverity, workItemRoute, workKindIcon, type WorkItem, type WorkSummary } from '@/lib/worklist'
import { useWorkListStore } from '@/stores/worklist'
import type { Project } from '@/lib/types'

const { t, te } = useI18n()
const route = useRoute()
const router = useRouter()
const badge = useWorkListStore()

const items = ref<WorkItem[]>([])
const summary = ref<WorkSummary>(emptySummary())
const projects = ref<Project[]>([])
const loading = ref(false)
const error = ref('')
const project = ref(typeof route.query.project === 'string' ? route.query.project : '')
const reason = ref(typeof route.query.reason === 'string' ? route.query.reason : '')

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
  { value: '', label: t('workList.allReasons', { n: summary.value.count }) },
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
</script>

<template>
  <section>
    <div class="page-header">
      <div>
        <h1>{{ t('titles.myWork') }}</h1>
        <div class="sub">{{ t('workList.intro') }}</div>
      </div>
      <div class="actions">
        <Select v-model="project" :options="projectOptions" option-label="label" option-value="value" filter :aria-label="t('workList.project')" style="min-width: 220px" />
        <Select v-model="reason" :options="reasonOptions" option-label="label" option-value="value" :aria-label="t('workList.reason')" style="min-width: 200px" />
      </div>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div class="summary row" role="status" aria-live="polite">
      <span><b>{{ summary.count }}</b> {{ t('workList.items', summary.count) }}</span>
      <span v-if="summary.overdue" class="error-text"><i class="pi pi-clock" aria-hidden="true" /> {{ t('workList.overdueCount', { n: summary.overdue }, summary.overdue) }}</span>
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
        <Column :header="t('workList.project')" style="width: 170px">
          <template #body="{ data }"><span class="small" :title="data.project_name">{{ data.project_key }}</span></template>
        </Column>
        <Column :header="t('common.status')" style="width: 140px">
          <template #body="{ data }"><span class="small">{{ statusLabel(data) }}</span></template>
        </Column>
        <Column :header="t('workList.due')" style="width: 120px">
          <template #body="{ data }">
            <span v-if="data.due" class="small" :class="{ 'error-text late': data.overdue }">{{ formatDate(data.due) }}</span>
            <span v-if="data.overdue" class="sr-only">{{ t('workList.reasons.overdue') }}</span>
          </template>
        </Column>
        <template #empty>
          <div class="empty">{{ reason || project ? t('workList.noneFiltered') : t('workList.none') }}</div>
        </template>
      </DataTable>
    </div>
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
</style>
