<script setup lang="ts">
// Add or remove relations for many requirements at once: each selected
// requirement gets (or loses) the chosen relation to each target.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ItemPickerDialog from './ItemPickerDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { choiceValue, parseChoice, planAdd, planRemove, reasonCounts, relationChoices } from '@/lib/bulkRelations'
import { createRelations, deleteRelations, relationsUndo } from '@/composables/relationsBulk'
import { relationText } from '@/i18n/labels'
import type { Relation } from '@/lib/types'
import { recordUndo } from '@/lib/undoActions'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; selected: string[] }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const notify = useNotify()

const mode = ref<'add' | 'remove'>('add')
const choice = ref('refines:outgoing')
const targets = ref<{ id: string; key: string; title: string }[]>([])
const picker = ref(false)
const existing = ref<Relation[]>([])
const loadError = ref('')
const saving = ref(false)

const modeOptions = computed(() => [
  { value: 'add', label: t('relations.bulk.add') },
  { value: 'remove', label: t('relations.bulk.remove') },
])
const choiceOptions = computed(() => [
  ...(mode.value === 'remove' ? [{ value: 'any', label: t('relations.bulk.anyType') }] : []),
  ...relationChoices().map((c) => ({ value: choiceValue(c), label: relationText(c.type, c.direction) })),
])
watch(mode, (m) => {
  if (m === 'add' && choice.value === 'any') choice.value = 'refines:outgoing'
})

async function loadExisting() {
  loadError.value = ''
  try {
    existing.value = asItems(await api.get<Relation[]>(`/projects/${props.projectId}/relations`))
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
watch(visible, (v) => {
  if (!v) return
  mode.value = 'add'
  choice.value = 'refines:outgoing'
  targets.value = []
  void loadExisting()
})

const targetIds = computed(() => targets.value.map((x) => x.id))
const toAdd = computed(() => {
  const c = parseChoice(choice.value)
  return c && targets.value.length ? planAdd(existing.value, props.selected, targetIds.value, c) : []
})
const toRemove = computed(() => planRemove(existing.value, props.selected, targetIds.value, parseChoice(choice.value)))
const count = computed(() => (mode.value === 'add' ? toAdd.value.length : toRemove.value.length))

function pick(items: { id: string; key: string; title: string }[]) {
  const have = new Set(targetIds.value)
  targets.value = [...targets.value, ...items.filter((x) => !have.has(x.id)).map((x) => ({ id: x.id, key: x.key, title: x.title }))]
}
const dropTarget = (id: string) => (targets.value = targets.value.filter((x) => x.id !== id))

async function apply() {
  const pid = props.projectId
  saving.value = true
  try {
    if (mode.value === 'add') {
      const res = await createRelations(pid, toAdd.value)
      const n = res.created.length
      const label = t('relations.bulk.added', { n }, n)
      if (res.skipped.length) {
        const why = Object.entries(reasonCounts(res.skipped))
          .map(([r, k]) => `${t(`relations.bulk.reason.${r}`)}: ${k}`)
          .join('\n')
        notify.failText(t('relations.bulk.addedSome', { n, skipped: res.skipped.length }), why)
      } else notify.ok(label)
      if (n) recordUndo(relationsUndo(label, pid, res.created, true))
    } else {
      const deleted = await deleteRelations(pid, toRemove.value.map((r) => r.id))
      const n = deleted.length
      const label = t('relations.bulk.removed', { n }, n)
      notify.ok(label)
      if (n) recordUndo(relationsUndo(label, pid, deleted, false))
    }
    visible.value = false
    emit('done')
  } catch (e) {
    notify.fail(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('relations.bulk.header', { n: selected.length }, selected.length)" :style="{ width: '34rem' }" :breakpoints="{ '640px': '95vw' }">
    <div class="stack" style="gap: 14px">
      <SelectButton v-model="mode" :options="modeOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('relations.bulk.mode')" />
      <div class="stack" style="gap: 4px">
        <label for="bulk-rel-type" class="small">{{ mode === 'add' ? t('relations.bulk.eachSelected') : t('relations.bulk.removeType') }}</label>
        <Select v-model="choice" input-id="bulk-rel-type" :options="choiceOptions" option-label="label" option-value="value" />
      </div>
      <div class="stack" style="gap: 6px">
        <span class="small" id="bulk-rel-targets">{{ mode === 'add' ? t('relations.bulk.targets') : t('relations.bulk.targetsRemove') }}</span>
        <ul v-if="targets.length" class="row plain-list" style="gap: 6px" aria-labelledby="bulk-rel-targets">
          <li v-for="x in targets" :key="x.id" class="trace-chip linked">
            <span class="mono">{{ x.key }}</span>
            <span class="ellipsis" style="max-width: 14rem">{{ x.title }}</span>
            <button type="button" class="link-button" :aria-label="t('relations.bulk.dropTarget', { key: x.key })" @click="dropTarget(x.id)"><i class="pi pi-times" aria-hidden="true" /></button>
          </li>
        </ul>
        <div v-else class="small muted">{{ mode === 'add' ? t('relations.bulk.noTargets') : t('relations.bulk.anyTarget') }}</div>
        <div><Button :label="t('relations.bulk.pickTargets')" icon="pi pi-plus" size="small" text aria-haspopup="dialog" @click="picker = true" /></div>
      </div>
      <Message v-if="loadError" severity="error" :closable="false">{{ t('relations.unavailable', { error: loadError }) }}</Message>
      <p class="small" role="status" style="margin: 0">
        {{ mode === 'add' ? t('relations.bulk.willAdd', { n: count }, count) : t('relations.bulk.willRemove', { n: count }, count) }}
      </p>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
      <Button
        :label="mode === 'add' ? t('relations.bulk.addN', { n: count }, count) : t('relations.bulk.removeN', { n: count }, count)"
        :icon="mode === 'add' ? 'pi pi-plus' : 'pi pi-trash'"
        :severity="mode === 'add' ? undefined : 'danger'"
        :loading="saving"
        :disabled="!count || saving"
        @click="apply"
      />
    </template>
    <ItemPickerDialog v-model:visible="picker" kind="requirement" :project-id="projectId" :exclude="targetIds" :header="t('relations.bulk.pickTargets')" :confirm-label="t('relations.bulk.useTargets')" @pick="pick" />
  </Dialog>
</template>

<style scoped>
.stack { display: flex; flex-direction: column; }
</style>
