<script setup lang="ts">
// Change control in the requirement and test editors: whether the item is
// under change control (and why), its open change requests, and the
// "Propose change" action, which puts the fields edited in the form into a
// change request instead of saving them.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProposeChangeDialog, { type ProposedItem } from './ProposeChangeDialog.vue'
import { api } from '@/lib/api'
import { type ChangeRequest, type ItemChangeControl } from '@/lib/changeRequests'
import { useNotify } from '@/composables/notify'
import { useAuthStore } from '@/stores/auth'
import { P } from '@/lib/permissions'

const props = defineProps<{
  kind: 'requirement' | 'test'
  itemId: string
  projectId: string
  itemKey: string
  itemTitle: string
  version: number
  /** the proposed values: the fields changed in the form */
  proposal: () => { proposed: Record<string, unknown>; current: Record<string, unknown> }
}>()
const emit = defineEmits<{ proposed: [cr: ChangeRequest] }>()
const { t } = useI18n()
const notify = useNotify()
const auth = useAuthStore()

const state = ref<ItemChangeControl | null>(null)
async function load() {
  try {
    state.value = await api.get<ItemChangeControl>(`/${props.kind === 'test' ? 'tests' : 'requirements'}/${props.itemId}/change-control`)
  } catch {
    state.value = null
  }
}
watch(() => [props.itemId, props.version], load, { immediate: true })

const canPropose = computed(() => state.value?.can_propose ?? auth.can(P.changeCreate))
const reasons = computed(() => (state.value?.reasons ?? []).join('; '))
const showMessage = computed(() => !!state.value && ((state.value.protected && state.value.mode !== 'off') || state.value.open_change_requests.length > 0))
const open = ref(false)
const items = ref<ProposedItem[]>([])

/** Open the dialog with the form's changes (also after a blocked save). */
function propose() {
  const p = props.proposal()
  if (!Object.keys(p.proposed).length) {
    notify.info(t('changeRequests.propose.editFirst'))
    return
  }
  items.value = [{ entity_type: props.kind, entity_id: props.itemId, key: props.itemKey, title: props.itemTitle, proposed: p.proposed, current: p.current, base_version: props.version }]
  open.value = true
}
function onProposed(cr: ChangeRequest) {
  emit('proposed', cr)
  void load()
}
/** whether direct saves are refused (enforced change control) */
const blocked = () => !!state.value?.blocked
defineExpose({ propose, reload: load, blocked })
</script>

<template>
  <div v-if="showMessage || canPropose" class="cc-bar">
    <Message v-if="state && showMessage" :severity="state.blocked ? 'warn' : 'info'" :closable="false" class="cc-message">
      <span v-if="state.protected && state.mode !== 'off'">
        <i class="pi pi-lock" aria-hidden="true" />
        {{ state.blocked ? t('changeRequests.bar.enforced', { reasons }) : t('changeRequests.bar.warned', { reasons }) }}
      </span>
      <span v-if="state.open_change_requests.length" class="cc-open">
        {{ t('changeRequests.bar.open') }}
        <RouterLink v-for="c in state.open_change_requests" :key="c.id" :to="{ name: 'changeRequests', params: { projectId }, query: { cr: c.id } }" class="key" :title="c.title">{{ c.key }}</RouterLink>
      </span>
    </Message>
    <Button v-if="canPropose" type="button" :label="t('changeRequests.propose.button')" icon="pi pi-file-edit" size="small" severity="secondary" :outlined="!state?.blocked" @click="propose" />
  </div>
  <ProposeChangeDialog v-model:visible="open" :project-id="projectId" :items="items" @proposed="onProposed" />
</template>

<style scoped>
.cc-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}
.cc-message {
  flex: 1 1 260px;
  margin: 0;
}
.cc-open {
  margin-left: 6px;
}
.cc-open a {
  margin-left: 6px;
}
</style>
