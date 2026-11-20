<script setup lang="ts">
// Add or remove item links for many requirements, tests or runs at once:
// each selected item gets (or loses) a link to each chosen target.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ItemPickerDialog from './ItemPickerDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { planAddLinks, planRemoveLinks, validLinkUrl, type LinkTargetRef, type SourceType } from '@/lib/bulkItemLinks'
import { LINK_TYPES, type LinkType } from '@/lib/itemLinks'
import { reasonCounts } from '@/lib/bulkRelations'
import { P } from '@/lib/permissions'
import type { ItemLink } from '@/lib/types'
import { recordUndo } from '@/lib/undoActions'
import { createItemLinks, deleteItemLinks, itemLinksUndo } from '@/composables/itemLinksBulk'
import { useNotify } from '@/composables/notify'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ sourceType: SourceType; projectId: string; selected: string[] }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const notify = useNotify()
const auth = useAuthStore()

const mode = ref<'add' | 'remove'>('add')
const type = ref<LinkType | 'any'>('url')
const picked = ref<{ id: string; key: string; title: string }[]>([])
const optionIds = ref<string[]>([])
const options = ref<{ value: string; label: string }[]>([])
const url = ref('')
const label = ref('')
const picker = ref(false)
const existing = ref<ItemLink[]>([])
const loadError = ref('')
const saving = ref(false)

const allowedTypes = computed(() =>
  LINK_TYPES.filter((ty) => (ty === 'requirement' ? auth.can(P.requirementRead) : ty === 'test' ? auth.can(P.testRead) : ty === 'run' ? auth.can(P.runRead) : true)),
)
const modeOptions = computed(() => [
  { value: 'add', label: t('links.bulk.add') },
  { value: 'remove', label: t('links.bulk.remove') },
])
const typeOptions = computed(() => [
  ...(mode.value === 'remove' ? [{ value: 'any', label: t('links.bulk.anyType') }] : []),
  ...allowedTypes.value.map((v) => ({ value: v, label: t(`links.type.${v}`) })),
])
const isPicker = computed(() => type.value === 'requirement' || type.value === 'test')
const isOptions = computed(() => type.value === 'run' || type.value === 'file' || type.value === 'report')

async function loadExisting() {
  loadError.value = ''
  try {
    existing.value = asItems(await api.get<{ items: ItemLink[] }>(`/projects/${props.projectId}/item-links?source_type=${props.sourceType}`))
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
watch(visible, (v) => {
  if (!v) return
  mode.value = 'add'
  type.value = 'url'
  label.value = ''
  void loadExisting()
})
watch(mode, (m) => {
  if (m === 'add' && type.value === 'any') type.value = 'url'
})
watch([type, visible], async () => {
  picked.value = []
  optionIds.value = []
  options.value = []
  url.value = ''
  if (!visible.value || !isOptions.value) return
  try {
    if (type.value === 'run') {
      const runs = asItems(await api.get<{ items: { id: string; name: string; build: string }[] }>(`/projects/${props.projectId}/runs`))
      options.value = runs.map((r) => ({ value: r.id, label: r.name + (r.build ? ` (${r.build})` : '') }))
    } else if (type.value === 'file') {
      const files = asItems(await api.get<{ items: { id: string; name: string; kind: string }[] }>(`/projects/${props.projectId}/documents`))
      options.value = files.filter((f) => f.kind === 'document' || f.kind === 'report').map((f) => ({ value: f.id, label: f.kind === 'report' ? `${f.name} (${t('links.savedReport')})` : f.name }))
    } else {
      const defs = asItems(await api.get<{ items: { id: string; name: string }[] }>(`/projects/${props.projectId}/reports/definitions`))
      options.value = defs.map((d) => ({ value: d.id, label: d.name }))
    }
  } catch (e) {
    loadError.value = errorMessage(e)
  }
})

const targets = computed<LinkTargetRef[]>(() => {
  const ty = type.value
  if (ty === 'any') return []
  if (ty === 'url') return mode.value === 'add' && validLinkUrl(url.value) ? [{ target_type: 'url', url: url.value.trim() }] : []
  if (isPicker.value) return picked.value.map((x) => ({ target_type: ty, target_id: x.id }))
  return optionIds.value.map((id) => ({ target_type: ty, target_id: id }))
})
const toAdd = computed(() => (mode.value === 'add' ? planAddLinks(existing.value, props.sourceType, props.selected, targets.value, label.value) : []))
const toRemove = computed(() => (mode.value === 'remove' ? planRemoveLinks(existing.value, props.sourceType, props.selected, type.value === 'any' ? null : type.value, targets.value) : []))
const count = computed(() => (mode.value === 'add' ? toAdd.value.length : toRemove.value.length))

function pick(items: { id: string; key: string; title: string }[]) {
  const have = new Set(picked.value.map((x) => x.id))
  picked.value = [...picked.value, ...items.filter((x) => !have.has(x.id)).map((x) => ({ id: x.id, key: x.key, title: x.title }))]
}
const drop = (id: string) => (picked.value = picked.value.filter((x) => x.id !== id))

async function apply() {
  const pid = props.projectId
  saving.value = true
  try {
    if (mode.value === 'add') {
      const res = await createItemLinks(pid, toAdd.value)
      const n = res.created.length
      const text = t('links.bulk.added', { n }, n)
      if (res.skipped.length) {
        const why = Object.entries(reasonCounts(res.skipped))
          .map(([r, k]) => `${t(`links.bulk.reason.${r}`)}: ${k}`)
          .join('\n')
        notify.failText(t('links.bulk.addedSome', { n, skipped: res.skipped.length }), why)
      } else notify.ok(text)
      if (n) recordUndo(itemLinksUndo(text, pid, res.created, true))
    } else {
      const deleted = await deleteItemLinks(pid, toRemove.value.map((l) => l.id))
      const n = deleted.length
      const text = t('links.bulk.removed', { n }, n)
      notify.ok(text)
      if (n) recordUndo(itemLinksUndo(text, pid, deleted, false))
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
  <Dialog v-model:visible="visible" modal :header="t('links.bulk.header', { n: selected.length }, selected.length)" :style="{ width: '34rem' }" :breakpoints="{ '640px': '95vw' }">
    <div class="stack" style="gap: 14px">
      <SelectButton v-model="mode" :options="modeOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('links.bulk.mode')" />
      <div class="stack" style="gap: 4px">
        <label for="bulk-link-type" class="small">{{ t('links.linkTo') }}</label>
        <Select v-model="type" input-id="bulk-link-type" :options="typeOptions" option-label="label" option-value="value" />
      </div>
      <template v-if="type === 'url'">
        <div v-if="mode === 'add'" class="stack" style="gap: 4px">
          <label for="bulk-link-url" class="small">{{ t('links.url') }}</label>
          <InputText id="bulk-link-url" v-model="url" type="url" inputmode="url" placeholder="https://" maxlength="2000" :invalid="!!url.trim() && !validLinkUrl(url)" aria-describedby="bulk-link-url-hint" />
          <span id="bulk-link-url-hint" class="small muted">{{ t('links.urlHint') }}</span>
        </div>
        <div v-else class="small muted">{{ t('links.bulk.anyAddress') }}</div>
      </template>
      <div v-else-if="isPicker" class="stack" style="gap: 6px">
        <span id="bulk-link-targets" class="small">{{ mode === 'add' ? t('links.bulk.targets') : t('links.bulk.targetsRemove') }}</span>
        <ul v-if="picked.length" class="row plain-list" style="gap: 6px" aria-labelledby="bulk-link-targets">
          <li v-for="x in picked" :key="x.id" class="trace-chip linked">
            <span class="mono">{{ x.key }}</span>
            <span class="ellipsis" style="max-width: 14rem">{{ x.title }}</span>
            <button type="button" class="link-button" :aria-label="t('links.bulk.dropTarget', { key: x.key })" @click="drop(x.id)"><i class="pi pi-times" aria-hidden="true" /></button>
          </li>
        </ul>
        <div v-else class="small muted">{{ mode === 'add' ? t('links.bulk.noTargets') : t('links.bulk.anyTarget') }}</div>
        <div><Button :label="t('links.chooseItems')" icon="pi pi-plus" size="small" text aria-haspopup="dialog" @click="picker = true" /></div>
      </div>
      <div v-else-if="isOptions" class="stack" style="gap: 4px">
        <label for="bulk-link-options" class="small">{{ mode === 'add' ? t('links.bulk.targets') : t('links.bulk.targetsRemove') }}</label>
        <MultiSelect
          v-model="optionIds"
          input-id="bulk-link-options"
          :options="options"
          option-label="label"
          option-value="value"
          filter
          :placeholder="mode === 'add' ? t('links.choose') : t('links.bulk.anyTarget')"
          :empty-message="t('links.noOptions')"
          :max-selected-labels="2"
        />
      </div>
      <div v-if="mode === 'add'" class="stack" style="gap: 4px">
        <label for="bulk-link-label" class="small">{{ t('links.label') }} {{ t('prompts.optional') }}</label>
        <InputText id="bulk-link-label" v-model="label" maxlength="200" :placeholder="t('links.labelPlaceholder')" />
      </div>
      <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
      <p class="small" role="status" style="margin: 0">
        {{ mode === 'add' ? t('links.bulk.willAdd', { n: count }, count) : t('links.bulk.willRemove', { n: count }, count) }}
      </p>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
      <Button
        :label="mode === 'add' ? t('links.bulk.addN', { n: count }, count) : t('links.bulk.removeN', { n: count }, count)"
        :icon="mode === 'add' ? 'pi pi-link' : 'pi pi-trash'"
        :severity="mode === 'add' ? undefined : 'danger'"
        :loading="saving"
        :disabled="!count || saving"
        @click="apply"
      />
    </template>
    <ItemPickerDialog
      v-if="type === 'requirement' || type === 'test'"
      v-model:visible="picker"
      :kind="type"
      :project-id="projectId"
      :exclude="picked.map((x) => x.id)"
      :header="t('links.chooseItems')"
      :confirm-label="t('links.bulk.useTargets')"
      @pick="pick"
    />
  </Dialog>
</template>

<style scoped>
.stack { display: flex; flex-direction: column; }
</style>
