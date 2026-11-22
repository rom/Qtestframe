<script setup lang="ts">
// Dashboard widget "My work": the signed-in user's work (in one project, or
// in all on the home dashboard): the count per reason and the first rows,
// with links to the full work list. The data comes from the dashboard's
// widget loader (GET /my-work, see internal/api/dashboardwidgets.go).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Tag from 'primevue/tag'
import ItemRef from './ItemRef.vue'
import { emptySummary, presentReasons, reasonSeverity, workItemRoute, workKindIcon, type WorkItem, type WorkSummary } from '@/lib/worklist'

const props = defineProps<{ projectId?: string; data: { items?: WorkItem[]; summary?: WorkSummary } | null }>()
const { t, te } = useI18n()
const summary = computed<WorkSummary>(() => ({ ...emptySummary(), ...(props.data?.summary ?? {}) }))
const top = computed(() => props.data?.items ?? [])
const reasons = computed(() => presentReasons(summary.value))
const reasonLabel = (r: string) => (te(`workList.reasons.${r}`) ? t(`workList.reasons.${r}`) : r.replace(/_/g, ' '))
const kindLabel = (k: string) => (te(`workList.kinds.${k}`) ? t(`workList.kinds.${k}`) : k)
const query = (extra: Record<string, string> = {}) => ({ ...(props.projectId ? { project: props.projectId } : {}), ...extra })
</script>

<template>
  <div class="my-work">
    <div class="row head">
      <RouterLink :to="{ name: 'myWork', query: query() }" class="small">{{ t('workList.openAll') }}</RouterLink>
    </div>
    <template v-if="data">
      <p class="count">
        <b>{{ summary.count }}</b> {{ t('workList.items', summary.count) }}
        <span v-if="summary.overdue" class="error-text small"> · {{ t('workList.overdueCount', { n: summary.overdue }, summary.overdue) }}</span>
      </p>
      <ul v-if="reasons.length" class="plain-list reasons" :aria-label="t('workList.byReason')">
        <li v-for="r in reasons" :key="r">
          <RouterLink :to="{ name: 'myWork', query: query({ reason: r }) }" class="reason-link">
            <Tag :value="`${reasonLabel(r)} ${summary.by_reason[r]}`" :severity="reasonSeverity(r)" />
          </RouterLink>
        </li>
      </ul>
      <ul v-if="top.length" class="plain-list rows">
        <li v-for="it in top" :key="it.kind + it.id" class="row nowrap">
          <i :class="workKindIcon(it.kind)" class="muted" aria-hidden="true" />
          <span class="sr-only">{{ kindLabel(it.kind) }}:</span>
          <ItemRef :to="workItemRoute(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
          <span class="small muted nowrap">{{ it.reasons.map(reasonLabel).join(', ') }}</span>
        </li>
      </ul>
      <p v-else class="small muted">{{ t('workList.noneHere') }}</p>
    </template>
  </div>
</template>

<style scoped>
.head {
  justify-content: flex-end;
  align-items: baseline;
  margin-bottom: 6px;
}
.count {
  margin: 4px 0 8px;
}
.reasons {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}
.reason-link {
  text-decoration: none;
}
.rows li {
  gap: 6px;
  padding: 4px 0;
  border-top: 1px solid var(--qtf-border);
  min-width: 0;
}
.nowrap {
  white-space: nowrap;
}
</style>
