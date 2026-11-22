<script setup lang="ts">
// The shared test data sets of a test (Test data page): the set a
// data-driven test takes its rows from, and the sets it refers to.
import { computed, ref, watch } from 'vue'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import { api, asItems } from '@/lib/api'
import type { DataLink } from '@/lib/environments'

const props = defineProps<{ projectId: string; testId: string }>()
const { t } = useI18n()
const links = ref<DataLink[]>([])
watch(
  () => props.testId,
  async (id) => {
    links.value = []
    if (!id) return
    try {
      links.value = asItems(await api.get<{ items: DataLink[] }>(`/tests/${id}/data-sets`))
    } catch {
      links.value = []
    }
  },
  { immediate: true },
)
const source = computed(() => links.value.find((l) => l.usage === 'source'))
const others = computed(() => links.value.filter((l) => l.usage !== 'source'))
</script>

<template>
  <Message v-if="links.length" severity="info" :closable="false" class="small">
    <span v-if="source">
      {{ t('dataSets.sourceLine') }}
      <RouterLink :to="{ name: 'testData', params: { projectId }, query: { set: source.data_set_id } }">{{ source.data_set_name }}</RouterLink>
    </span>
    <span v-if="others.length">
      <template v-if="source"> · </template>{{ t('dataSets.sourceHeader') }}:
      <template v-for="(l, i) in others" :key="l.id"><RouterLink :to="{ name: 'testData', params: { projectId }, query: { set: l.data_set_id } }">{{ l.data_set_name }}</RouterLink>{{ i < others.length - 1 ? ', ' : '' }}</template>
    </span>
  </Message>
</template>
