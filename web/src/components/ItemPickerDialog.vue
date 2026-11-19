<script setup lang="ts">
import { ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import QueryInput from './QueryInput.vue'
import StatusTag from './StatusTag.vue'
import { api, asItems } from '@/lib/api'
import type { Requirement, TestCase } from '@/lib/types'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'
const prefs = usePrefsStore()

type Item = Requirement | TestCase
const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ kind: 'requirement' | 'test'; projectId: string; exclude?: string[]; header?: string; confirmLabel?: string }>()
const emit = defineEmits<{ pick: [items: Item[]] }>()
const { t } = useI18n()

const q = ref('')
const items = ref<Item[]>([])
const selection = ref<Item[]>([])
const loading = ref(false)
const error = ref<unknown>(null)

async function load() {
  loading.value = true
  error.value = null
  try {
    const path = props.kind === 'requirement' ? 'requirements' : 'tests'
    const res = await api.get<{ items: Item[] }>(`/projects/${props.projectId}/${path}`, { q: q.value.trim() })
    const ex = new Set(props.exclude ?? [])
    items.value = asItems(res).filter((i) => !ex.has(i.id))
  } catch (e) {
    error.value = e
    items.value = []
  } finally {
    loading.value = false
  }
}

watch(visible, (v) => {
  if (v) {
    selection.value = []
    load()
  }
})

function confirm() {
  emit('pick', selection.value)
  visible.value = false
}
</script>

<template>
  <Dialog v-model:visible="visible" :header="header ?? (kind === 'requirement' ? t('picker.selectRequirements') : t('picker.selectTests'))" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(900px, 96vw)">
    <div class="filterbar">
      <QueryInput v-model="q" :scope="kind" :error="error" @submit="load" />
      <Button :label="t('common.apply')" icon="pi pi-search" severity="secondary" @click="load" />
    </div>
    <DataTable
      v-stack
      v-model:selection="selection"
      :value="items"
      data-key="id"
      :loading="loading"
      :aria-busy="loading"
      scrollable
      scroll-height="50vh"
      size="small"
      paginator
      :rows="prefs.rowsFor(50)"
      sort-field="key"
      :sort-order="1"
    >
      <Column selection-mode="multiple" style="width: 3rem" />
      <Column field="key" :header="t('common.key')" sortable style="width: 140px"><template #body="{ data }"><span class="key">{{ data.key }}</span></template></Column>
      <Column field="title" :header="t('common.title')" sortable />
      <Column v-if="kind === 'test'" field="execution_type" :header="t('common.type')" sortable style="width: 130px"><template #body="{ data }">{{ enumLabel('executionType', data.execution_type) }}</template></Column>
      <Column field="status" :header="t('common.status')" sortable style="width: 120px"><template #body="{ data }"><StatusTag :value="data.status" /></template></Column>
      <Column field="folder_path" :header="t('common.folder')" sortable />
      <template #empty><div class="empty">{{ t('picker.nothingFound') }}</div></template>
    </DataTable>
    <template #footer>
      <span class="muted small" style="margin-right: auto">{{ t('common.selected', { count: selection.length }) }}</span>
      <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button :label="confirmLabel ?? t('common.add')" icon="pi pi-check" :disabled="!selection.length" @click="confirm" />
    </template>
  </Dialog>
</template>
