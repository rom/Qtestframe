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
const prefs = usePrefsStore()

type Item = Requirement | TestCase
const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ kind: 'requirement' | 'test'; projectId: string; exclude?: string[]; header?: string; confirmLabel?: string }>()
const emit = defineEmits<{ pick: [items: Item[]] }>()

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
  <Dialog v-model:visible="visible" :header="header ?? (kind === 'requirement' ? 'Select requirements' : 'Select tests')" modal maximizable style="width: min(900px, 96vw)">
    <div class="filterbar">
      <QueryInput v-model="q" :scope="kind" :error="error" @submit="load" />
      <Button label="Apply" icon="pi pi-search" severity="secondary" @click="load" />
    </div>
    <DataTable
      v-model:selection="selection"
      :value="items"
      data-key="id"
      :loading="loading"
      scrollable
      scroll-height="50vh"
      size="small"
      paginator
      :rows="prefs.rowsFor(50)"
      sort-field="key"
      :sort-order="1"
    >
      <Column selection-mode="multiple" style="width: 3rem" />
      <Column field="key" header="Key" sortable style="width: 140px"><template #body="{ data }"><span class="key">{{ data.key }}</span></template></Column>
      <Column field="title" header="Title" sortable />
      <Column v-if="kind === 'test'" field="execution_type" header="Type" sortable style="width: 110px" />
      <Column field="status" header="Status" sortable style="width: 120px"><template #body="{ data }"><StatusTag :value="data.status" /></template></Column>
      <Column field="folder_path" header="Folder" sortable />
      <template #empty><div class="empty">Nothing found.</div></template>
    </DataTable>
    <template #footer>
      <span class="muted small" style="margin-right: auto">{{ selection.length }} selected</span>
      <Button label="Cancel" text severity="secondary" @click="visible = false" />
      <Button :label="confirmLabel ?? 'Add'" icon="pi pi-check" :disabled="!selection.length" @click="confirm" />
    </template>
  </Dialog>
</template>
