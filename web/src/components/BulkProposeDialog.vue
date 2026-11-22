<script setup lang="ts">
// The bulk "Propose change": the same new values (status, priority,
// obligation or execution type, owner) for the selected requirements or
// tests, proposed in a change request instead of saved.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import ProposeChangeDialog, { type ProposedItem } from './ProposeChangeDialog.vue'
import { bulkProposal, type ChangeRequest } from '@/lib/changeRequests'
import { enumOptions } from '@/i18n/labels'
import { useMetaStore } from '@/stores/meta'

interface Row {
  id: string
  key: string
  title: string
  version: number
  status?: string
  priority?: string
  owner?: string
  obligation?: string
  execution_type?: string
}
const props = defineProps<{ projectId: string; kind: 'requirement' | 'test'; rows: Row[] }>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ proposed: [cr: ChangeRequest] }>()
const { t } = useI18n()
const meta = useMetaStore()

const status = ref<string | null>(null)
const priority = ref<string | null>(null)
const third = ref<string | null>(null)
const owner = ref('')
watch(visible, (v) => {
  if (v) {
    status.value = priority.value = third.value = null
    owner.value = ''
  }
})
const isReq = computed(() => props.kind === 'requirement')
const statusOptions = computed(() => enumOptions('status', isReq.value ? meta.meta.requirement_statuses : meta.meta.test_statuses))
const priorityOptions = computed(() => enumOptions('priority', meta.meta.priorities))
const thirdOptions = computed(() => (isReq.value ? enumOptions('obligation', meta.meta.obligations) : enumOptions('executionType', ['manual', 'automated'])))
const thirdField = computed(() => (isReq.value ? 'obligation' : 'execution_type'))
const set = computed(() => bulkProposal({ status: status.value, priority: priority.value, [thirdField.value]: third.value, owner: owner.value.trim() }))

const proposeOpen = ref(false)
const items = ref<ProposedItem[]>([])
function next() {
  items.value = props.rows.map((r) => {
    const cur = r as unknown as Record<string, unknown>
    const proposed: Record<string, unknown> = {}
    const current: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(set.value)) {
      if (cur[k] === v) continue
      proposed[k] = v
      current[k] = cur[k] ?? null
    }
    return { entity_type: props.kind, entity_id: r.id, key: r.key, title: r.title, proposed, current, base_version: r.version }
  })
  visible.value = false
  proposeOpen.value = true
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('changeRequests.bulk.header', { n: rows.length }, rows.length)" style="width: min(520px, 96vw)">
    <form class="grid" style="gap: 10px" @submit.prevent="next">
      <p class="small" style="margin: 0">{{ t('changeRequests.bulk.explain') }}</p>
      <div class="form-grid">
        <div class="field">
          <label for="crb-status">{{ t('common.status') }}</label>
          <Select v-model="status" input-id="crb-status" :options="statusOptions" option-label="label" option-value="value" show-clear :placeholder="t('changeRequests.bulk.keep')" />
        </div>
        <div class="field">
          <label for="crb-prio">{{ t('common.priority') }}</label>
          <Select v-model="priority" input-id="crb-prio" :options="priorityOptions" option-label="label" option-value="value" show-clear :placeholder="t('changeRequests.bulk.keep')" />
        </div>
        <div class="field">
          <label for="crb-third">{{ isReq ? t('changeRequests.field.obligation') : t('changeRequests.field.execution_type') }}</label>
          <Select v-model="third" input-id="crb-third" :options="thirdOptions" option-label="label" option-value="value" show-clear :placeholder="t('changeRequests.bulk.keep')" />
        </div>
        <div class="field">
          <label for="crb-owner">{{ t('common.owner') }}</label>
          <InputText id="crb-owner" v-model="owner" :placeholder="t('changeRequests.bulk.keep')" />
        </div>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
        <Button type="submit" :label="t('changeRequests.bulk.next')" icon="pi pi-arrow-right" :disabled="!Object.keys(set).length" />
      </div>
    </form>
  </Dialog>
  <ProposeChangeDialog v-model:visible="proposeOpen" :project-id="projectId" :items="items" @proposed="(cr) => emit('proposed', cr)" />
</template>
