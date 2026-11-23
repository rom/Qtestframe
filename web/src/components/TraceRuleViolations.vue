<script setup lang="ts">
// The traceability rules a requirement or test breaks (editor details tab).
import { computed, ref, watch } from 'vue'
import { api } from '@/lib/api'
import { rulesRoute, violationText } from '@/lib/traceRules'
import type { ItemTraceViolation } from '@/lib/types'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  kind: 'requirement' | 'test'
  itemId: string
  projectId: string
  /** changes when the item, its links or relations change (reloads) */
  stamp?: string
}>()
const { t } = useI18n()
const list = ref<ItemTraceViolation[]>([])

async function load() {
  try {
    list.value = (await api.get<{ items: ItemTraceViolation[] }>(`/${props.kind === 'test' ? 'tests' : 'requirements'}/${props.itemId}/trace-rules`)).items ?? []
  } catch {
    list.value = [] // the rules are a hint; the editor works without them
  }
}
watch(() => [props.itemId, props.stamp], load, { immediate: true })
const errors = computed(() => list.value.some((v) => v.rule.severity !== 'warning'))
defineExpose({ reload: load })
</script>

<template>
  <div v-if="list.length" class="trace-violations" :class="errors ? 'is-error' : 'is-warning'" role="region" :aria-label="t('traceRules.title')">
    <div class="row small" style="gap: 6px">
      <i :class="errors ? 'pi pi-times-circle' : 'pi pi-exclamation-triangle'" aria-hidden="true" />
      <b>{{ t('traceRules.breaks', { n: list.length }, list.length) }}</b>
      <RouterLink :to="rulesRoute(projectId)">{{ t('traceRules.openRules') }}</RouterLink>
    </div>
    <ul class="plain-list small" style="margin: 4px 0 0">
      <li v-for="v in list" :key="v.rule.id">
        <span class="sr-only">{{ t(`traceRules.severity.${v.rule.severity === 'warning' ? 'warning' : 'error'}`) }}:</span>
        {{ v.rule.name }} – {{ violationText(v, t) }}
      </li>
    </ul>
  </div>
</template>

<style scoped>
/* card colours with a coloured edge: the text keeps full contrast in every theme */
.trace-violations {
  padding: 8px 12px;
  border: 1px solid var(--qtf-border);
  border-left: 4px solid var(--p-red-600, #dc2626);
  border-radius: 8px;
  background: var(--qtf-card);
  color: var(--qtf-text);
}
.trace-violations.is-warning { border-left-color: var(--p-amber-500, #f59e0b); }
.trace-violations.is-error .pi { color: var(--p-red-600, #dc2626); }
.trace-violations.is-warning .pi { color: var(--p-amber-600, #d97706); }
.app-dark .trace-violations.is-error .pi { color: var(--p-red-400, #f87171); }
.app-dark .trace-violations.is-warning .pi { color: var(--p-amber-400, #fbbf24); }
.trace-violations a { color: var(--p-primary-color); }
</style>
