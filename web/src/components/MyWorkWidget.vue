<script setup lang="ts">
// Dashboard widget "My work": the signed-in user's work in this project (the
// count per reason and the first rows), with links to the full work list.
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Tag from 'primevue/tag'
import ItemRef from './ItemRef.vue'
import { api } from '@/lib/api'
import { emptySummary, presentReasons, reasonSeverity, workItemRoute, workKindIcon, type WorkItem, type WorkSummary } from '@/lib/worklist'

const props = defineProps<{ projectId: string }>()
const { t, te } = useI18n()
const items = ref<WorkItem[]>([])
const summary = ref<WorkSummary>(emptySummary())
const loaded = ref(false)
const failed = ref(false)

async function load() {
  failed.value = false
  try {
    const res = await api.get<{ items: WorkItem[]; summary: WorkSummary }>('/my-work', { project: props.projectId })
    items.value = res.items ?? []
    summary.value = { ...emptySummary(), ...res.summary }
  } catch {
    failed.value = true
  } finally {
    loaded.value = true
  }
}
onMounted(load)
watch(() => props.projectId, load)
defineExpose({ load })

const top = computed(() => items.value.slice(0, 5))
const reasons = computed(() => presentReasons(summary.value))
const reasonLabel = (r: string) => (te(`workList.reasons.${r}`) ? t(`workList.reasons.${r}`) : r.replace(/_/g, ' '))
const kindLabel = (k: string) => (te(`workList.kinds.${k}`) ? t(`workList.kinds.${k}`) : k)
</script>

<template>
  <section class="card my-work" aria-labelledby="ch-mywork">
    <div class="row head">
      <h2 id="ch-mywork" style="margin: 0">{{ t('workList.widgetTitle') }}</h2>
      <RouterLink :to="{ name: 'myWork', query: { project: projectId } }" class="small">{{ t('workList.openAll') }}</RouterLink>
    </div>
    <p v-if="failed" class="small muted">{{ t('workList.loadFailed') }}</p>
    <template v-else-if="loaded">
      <p class="count">
        <b>{{ summary.count }}</b> {{ t('workList.items', summary.count) }}
        <span v-if="summary.overdue" class="error-text small"> · {{ t('workList.overdueCount', { n: summary.overdue }, summary.overdue) }}</span>
      </p>
      <ul v-if="reasons.length" class="plain-list reasons" :aria-label="t('workList.byReason')">
        <li v-for="r in reasons" :key="r">
          <RouterLink :to="{ name: 'myWork', query: { project: projectId, reason: r } }" class="reason-link">
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
  </section>
</template>

<style scoped>
.head {
  justify-content: space-between;
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
