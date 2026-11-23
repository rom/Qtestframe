<script setup lang="ts">
// "Propose change": puts proposed field values of one or more items into a
// change request, a new one or a draft of the user's. The proposals come
// from the editor (the fields edited in propose mode) or from a bulk
// change; nothing is changed until the change request is approved and
// applied.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import { api, asItems, errorMessage } from '@/lib/api'
import { CR_CATEGORIES, type ChangeRequest } from '@/lib/changeRequests'
import ChangeDiff from './ChangeDiff.vue'
import { enumOptions } from '@/i18n/labels'
import { useMetaStore } from '@/stores/meta'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useRouter } from 'vue-router'

export interface ProposedItem {
  entity_type: 'requirement' | 'test'
  entity_id: string
  key: string
  title: string
  proposed: Record<string, unknown>
  /** the current values of the proposed fields (for the preview) */
  current?: Record<string, unknown>
  base_version?: number
}

const props = defineProps<{ projectId: string; items: ProposedItem[] }>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ proposed: [cr: ChangeRequest] }>()
const { t } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const router = useRouter()

const target = ref<'new' | 'draft'>('new')
const draftId = ref('')
const drafts = ref<ChangeRequest[]>([])
const title = ref('')
const reason = ref('')
const category = ref('correction')
const priority = ref('medium')
const busy = ref(false)
const error = ref('')

const targetOptions = computed(() => [
  { label: t('changeRequests.propose.new'), value: 'new' },
  { label: t('changeRequests.propose.toDraft'), value: 'draft' },
])
const categoryOptions = computed(() => CR_CATEGORIES.map((v) => ({ label: t(`changeRequests.category.${v}`), value: v })))
const meta = useMetaStore()
const priorityOptions = computed(() => enumOptions('priority', meta.meta.priorities))
const draftOptions = computed(() => drafts.value.map((d) => ({ label: `${d.key} ${d.title}`, value: d.id })))
const previews = computed(() => props.items.map((it) => ({ it })))
const empty = computed(() => !props.items.length || props.items.every((i) => !Object.keys(i.proposed).length))

watch(visible, async (v) => {
  if (!v) return
  error.value = ''
  target.value = 'new'
  title.value = props.items.length === 1 ? t('changeRequests.propose.defaultTitle', { key: props.items[0].key }) : ''
  reason.value = ''
  draftId.value = ''
  try {
    const me = (auth.user?.email ?? '').toLowerCase()
    drafts.value = asItems(await api.get<{ items: ChangeRequest[] }>(`/projects/${props.projectId}/change-requests`, { status: 'draft' })).filter(
      (d) => d.created_by.toLowerCase() === me,
    )
  } catch {
    drafts.value = []
  }
})

async function propose() {
  busy.value = true
  error.value = ''
  const items = props.items.filter((i) => Object.keys(i.proposed).length).map((i) => ({ entity_type: i.entity_type, entity_id: i.entity_id, proposed: i.proposed, base_version: i.base_version }))
  try {
    let cr: ChangeRequest
    if (target.value === 'draft') {
      if (!draftId.value) throw new Error(t('changeRequests.propose.chooseDraft'))
      cr = await api.post<ChangeRequest>(`/change-requests/${draftId.value}/items`, { items })
    } else {
      cr = await api.post<ChangeRequest>(`/projects/${props.projectId}/change-requests`, { title: title.value.trim(), reason: reason.value.trim(), category: category.value, priority: priority.value, items })
    }
    visible.value = false
    notify.ok(t('changeRequests.propose.done', { key: cr.key }), t('changeRequests.propose.doneHint'))
    emit('proposed', cr)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
function openCR(id: string) {
  void router.push({ name: 'changeRequests', params: { projectId: props.projectId }, query: { cr: id } })
}
defineExpose({ openCR })
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('changeRequests.propose.header')" maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(720px, 96vw)">
    <form class="grid" style="gap: 10px" @submit.prevent="propose">
      <p class="small" style="margin: 0">{{ t('changeRequests.propose.explain') }}</p>
      <Message v-if="empty" severity="warn" :closable="false">{{ t('changeRequests.propose.nothing') }}</Message>
      <section v-else aria-labelledby="cr-propose-items" class="cr-preview">
        <h3 id="cr-propose-items" class="plain-heading small">{{ t('changeRequests.propose.items', { n: items.length }, items.length) }}</h3>
        <div v-for="p in previews" :key="p.it.entity_id" class="cr-preview-item">
          <div class="small"><span class="key">{{ p.it.key }}</span> {{ p.it.title }}</div>
          <ChangeDiff :item="p.it" />
        </div>
      </section>
      <span id="cr-propose-target" class="field-label">{{ t('changeRequests.propose.target') }}</span>
      <SelectButton v-model="target" :options="targetOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="cr-propose-target" />
      <template v-if="target === 'new'">
        <label for="cr-propose-title">{{ t('common.title') }}</label>
        <InputText id="cr-propose-title" v-model="title" fluid />
        <label for="cr-propose-reason">{{ t('changeRequests.reason') }}</label>
        <Textarea id="cr-propose-reason" v-model="reason" rows="3" auto-resize fluid />
        <div class="form-grid">
          <div class="field">
            <label for="cr-propose-cat">{{ t('changeRequests.categoryLabel') }}</label>
            <Select v-model="category" input-id="cr-propose-cat" :options="categoryOptions" option-label="label" option-value="value" />
          </div>
          <div class="field">
            <label for="cr-propose-prio">{{ t('common.priority') }}</label>
            <Select v-model="priority" input-id="cr-propose-prio" :options="priorityOptions" option-label="label" option-value="value" />
          </div>
        </div>
      </template>
      <template v-else>
        <label for="cr-propose-draft">{{ t('changeRequests.propose.draft') }}</label>
        <Select v-model="draftId" input-id="cr-propose-draft" :options="draftOptions" option-label="label" option-value="value" :placeholder="t('changeRequests.propose.chooseDraft')" :empty-message="t('changeRequests.propose.noDrafts')" filter />
      </template>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
        <Button type="submit" :label="t('changeRequests.propose.submit')" icon="pi pi-file-edit" :loading="busy" :disabled="empty || (target === 'new' ? !title.trim() : !draftId)" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.cr-preview {
  max-height: 40vh;
  overflow: auto;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 8px 10px;
}
.cr-preview-item + .cr-preview-item {
  border-top: 1px solid var(--qtf-border);
  margin-top: 8px;
  padding-top: 8px;
}
</style>
