<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ItemPickerDialog from './ItemPickerDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { groupRelations, otherEnd, RELATION_TYPES } from '@/lib/relations'
import type { Relation } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { relationText } from '@/i18n/labels'

const props = defineProps<{ projectId: string; requirementId: string; canWrite: boolean }>()
const emit = defineEmits<{ count: [n: number] }>()
const notify = useNotify()
const { t } = useI18n()

const relations = ref<Relation[]>([])
const loading = ref(false)
const error = ref('')
const type = ref('relates_to')
const picker = ref(false)
const typeOptions = computed(() => RELATION_TYPES.map((v) => ({ label: relationText(v, 'outgoing'), value: v as string })))

async function load() {
  loading.value = true
  error.value = ''
  try {
    relations.value = asItems(await api.get<Relation[]>(`/requirements/${props.requirementId}/relations`))
    emit('count', relations.value.length)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.requirementId, load, { immediate: true })

const groups = computed(() => groupRelations(relations.value, props.requirementId))
// exclude self and requirements this one already relates to with the chosen type
const exclude = computed(() => [
  props.requirementId,
  ...relations.value.filter((r) => r.type === type.value && r.source_id === props.requirementId).map((r) => r.target_id),
])

async function add(items: { id: string }[]) {
  let ok = 0
  for (const it of items) {
    try {
      await api.post(`/requirements/${props.requirementId}/relations`, { target_id: it.id, type: type.value })
      ok++
    } catch (e) {
      notify.fail(e, t('relations.addFailed'))
    }
  }
  if (ok) notify.ok(t('relations.added', ok))
  await load()
}
async function remove(r: Relation) {
  try {
    await api.del(`/relations/${r.id}`)
    relations.value = relations.value.filter((x) => x.id !== r.id)
    emit('count', relations.value.length)
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <div>
    <div v-if="canWrite && !error" class="row" style="margin-bottom: 12px">
      <span class="small muted">{{ t('relations.thisRequirement') }}</span>
      <Select v-model="type" :options="typeOptions" option-label="label" option-value="value" size="small" style="min-width: 200px" :aria-label="t('relations.type')" />
      <Button :label="t('relations.add')" icon="pi pi-plus" size="small" text @click="picker = true" />
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ t('relations.unavailable', { error }) }}</Message>
    <div v-else-if="loading && !relations.length" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="!relations.length" class="muted small">{{ t('relations.none') }}</div>
    <div v-for="g in groups" :key="g.type + g.direction" style="margin-bottom: 12px">
      <div class="section-title" style="margin-top: 6px">
        <i :class="g.direction === 'incoming' ? 'pi pi-arrow-left' : 'pi pi-arrow-right'" /> {{ relationText(g.type, g.direction) }} ({{ g.items.length }})
      </div>
      <div v-for="r in g.items" :key="r.id" class="row nowrap" style="padding: 5px 0; border-bottom: 1px solid var(--qtf-border)">
        <RouterLink :to="{ name: 'requirements', params: { projectId }, query: { item: otherEnd(r, requirementId).id } }" class="key">
          {{ otherEnd(r, requirementId).key }}
        </RouterLink>
        <span class="grow ellipsis">{{ otherEnd(r, requirementId).title }}</span>
        <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('relations.remove')" v-tooltip="t('relations.remove')" @click="remove(r)" />
      </div>
    </div>
    <ItemPickerDialog
      v-model:visible="picker"
      kind="requirement"
      :project-id="projectId"
      :exclude="exclude"
      :header="t('relations.pickerHeader', { type: typeOptions.find((o) => o.value === type)?.label ?? type })"
      :confirm-label="t('relations.confirm')"
      @pick="add"
    />
  </div>
</template>
