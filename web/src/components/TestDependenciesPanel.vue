<script setup lang="ts">
import ItemRef from './ItemRef.vue'
import { useUndoStore } from '@/stores/undo'
import { recordUndo, toggleEntry } from '@/lib/undoActions'
// Prerequisites of a test (GET/POST /tests/{id}/dependencies): tests that
// must have passed before this one may be started, and the tests that wait
// for this one.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import ItemPickerDialog from './ItemPickerDialog.vue'
import StatusTag from './StatusTag.vue'
import HelpTip from '@/guide/HelpTip.vue'
import { api, asItems } from '@/lib/api'
import type { TestDependency } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string; testId: string; canWrite: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const notify = useNotify()
const deps = ref<TestDependency[]>([])
const picker = ref(false)
const prerequisites = computed(() => deps.value.filter((d) => d.direction !== 'incoming'))
const dependents = computed(() => deps.value.filter((d) => d.direction === 'incoming'))

async function load() {
  try {
    deps.value = asItems(await api.get<{ items: TestDependency[] }>(`/tests/${props.testId}/dependencies`))
  } catch {
    deps.value = []
  }
}
watch(() => props.testId, load, { immediate: true })

function dependencyEntry(created: boolean, d: TestDependency) {
  return toggleEntry(
    t(created ? 'undo.a.dependency' : 'undo.a.undependency', { from: d.test_key, to: d.depends_on_key }),
    created,
    d.id,
    async () => (await api.post<TestDependency>(`/tests/${d.test_id}/dependencies`, { depends_on_id: d.depends_on_id })).id,
    async (id) => void (await api.del(`/test-dependencies/${id}`)),
  )
}
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => void load(),
)

async function add(items: { id: string }[]) {
  let added = 0
  for (const it of items) {
    try {
      const d = await api.post<TestDependency>(`/tests/${props.testId}/dependencies`, { depends_on_id: it.id })
      if (d?.id) recordUndo(dependencyEntry(true, d))
      added++
    } catch (e) {
      notify.fail(e, t('dependencies.addFailed'))
    }
  }
  if (added) {
    notify.ok(t('dependencies.added', { count: added }, added))
    emit('changed')
  }
  await load()
}
async function remove(d: TestDependency) {
  try {
    await api.del(`/test-dependencies/${d.id}`)
    recordUndo(dependencyEntry(false, d))
    deps.value = deps.value.filter((x) => x.id !== d.id)
    emit('changed')
  } catch (e) {
    notify.fail(e)
  }
}
const route = (id: string) => ({ name: 'tests', params: { projectId: props.projectId }, query: { item: id } })
</script>

<template>
  <div>
    <div class="section-title">
      <i class="pi pi-share-alt" aria-hidden="true" /> {{ t('dependencies.title', { count: prerequisites.length }) }}
      <HelpTip id="test.dependencies" :label="t('dependencies.titlePlain')" />
      <span class="grow" />
      <Button v-if="canWrite" size="small" :label="t('dependencies.add')" icon="pi pi-plus" text @click="picker = true" />
    </div>
    <div v-if="!prerequisites.length" class="muted small">{{ t('dependencies.none') }}</div>
    <div v-for="d in prerequisites" :key="d.id" class="row nowrap dep-row">
      <ItemRef :to="route(d.depends_on_id)" :item-key="d.depends_on_key" :title="d.depends_on_title" title-class="ellipsis" grow />
      <StatusTag :value="d.other_last_result" kind="result" />
      <Button
        v-if="canWrite"
        icon="pi pi-times"
        text
        rounded
        size="small"
        severity="secondary"
        :aria-label="t('dependencies.remove', { key: d.depends_on_key })"
        v-tooltip="t('dependencies.remove', { key: d.depends_on_key })"
        @click="remove(d)"
      />
    </div>
    <div v-if="dependents.length" class="small" style="margin-top: 8px">
      <span class="muted">{{ t('dependencies.requiredBy') }}</span>
      <template v-for="(d, i) in dependents" :key="d.id">
        <RouterLink :to="route(d.test_id)" class="key" :title="d.test_title">{{ d.test_key }}</RouterLink>{{ i < dependents.length - 1 ? ', ' : '' }}
      </template>
    </div>
    <ItemPickerDialog
      v-model:visible="picker"
      kind="test"
      :project-id="projectId"
      :exclude="[testId, ...prerequisites.map((d) => d.depends_on_id)]"
      :header="t('dependencies.add')"
      :confirm-label="t('dependencies.addConfirm')"
      @pick="add"
    />
  </div>
</template>

<style scoped>
.dep-row {
  padding: 5px 0;
  border-bottom: 1px solid var(--qtf-border);
}
</style>
