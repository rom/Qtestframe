<script setup lang="ts">
// Links of a requirement, test or run (GET/POST …/item-links): to other
// requirements, tests and runs of the project, documents and saved reports
// of the archive, report definitions and web addresses. Links pointing at
// this item from elsewhere are listed as "Linked from".
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ItemPickerDialog from './ItemPickerDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { linkTarget, LINK_ICONS, LINK_TYPES, type LinkType } from '@/lib/itemLinks'
import { recordUndo, toggleEntry } from '@/lib/undoActions'
import type { ItemLink } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useUndoStore } from '@/stores/undo'
import { useNotify } from '@/composables/notify'
import { P } from '@/lib/permissions'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ itemType: 'requirement' | 'test' | 'run'; itemId: string; projectId: string; canWrite: boolean }>()
const emit = defineEmits<{ count: [n: number] }>()
const { t } = useI18n()
const notify = useNotify()
const auth = useAuthStore()

const links = ref<ItemLink[]>([])
const loading = ref(false)
const error = ref('')
const base = computed(() => `/${props.itemType === 'requirement' ? 'requirements' : props.itemType === 'test' ? 'tests' : 'runs'}/${props.itemId}/item-links`)

async function load() {
  loading.value = true
  error.value = ''
  try {
    links.value = asItems(await api.get<{ items: ItemLink[] }>(base.value))
    emit('count', links.value.length)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.itemId, load, { immediate: true })
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => void load(),
)

const outgoing = computed(() => links.value.filter((l) => l.direction !== 'incoming'))
const incoming = computed(() => links.value.filter((l) => l.direction === 'incoming'))

// ---------- add ----------
const dialog = ref(false)
const type = ref<LinkType>('test')
const targetId = ref<string | null>(null)
const url = ref('')
const label = ref('')
const saving = ref(false)
const formError = ref('')
const picker = ref(false)
const options = ref<{ value: string; label: string }[]>([])
const allowedTypes = computed(() =>
  LINK_TYPES.filter((ty) => {
    if (ty === 'requirement') return auth.can(P.requirementRead)
    if (ty === 'test') return auth.can(P.testRead)
    if (ty === 'run') return auth.can(P.runRead)
    return true
  }),
)
const typeOptions = computed(() => allowedTypes.value.map((v) => ({ value: v, label: t(`links.type.${v}`), icon: LINK_ICONS[v] })))

function openAdd() {
  type.value = allowedTypes.value.includes('test') ? 'test' : allowedTypes.value[0]
  targetId.value = null
  url.value = ''
  label.value = ''
  formError.value = ''
  dialog.value = true
}
watch([type, dialog], async () => {
  targetId.value = null
  options.value = []
  if (!dialog.value) return
  try {
    if (type.value === 'run') {
      const runs = asItems(await api.get<{ items: { id: string; name: string; build: string }[] }>(`/projects/${props.projectId}/runs`))
      options.value = runs.filter((r) => !(props.itemType === 'run' && r.id === props.itemId)).map((r) => ({ value: r.id, label: r.name + (r.build ? ` (${r.build})` : '') }))
    } else if (type.value === 'file') {
      const files = asItems(await api.get<{ items: { id: string; name: string; kind: string }[] }>(`/projects/${props.projectId}/documents`))
      options.value = files.filter((f) => f.kind === 'document' || f.kind === 'report').map((f) => ({ value: f.id, label: f.kind === 'report' ? `${f.name} (${t('links.savedReport')})` : f.name }))
    } else if (type.value === 'report') {
      const defs = asItems(await api.get<{ items: { id: string; name: string }[] }>(`/projects/${props.projectId}/reports/definitions`))
      options.value = defs.map((d) => ({ value: d.id, label: d.name }))
    }
  } catch (e) {
    formError.value = errorMessage(e)
  }
})

async function create(body: Record<string, unknown>): Promise<ItemLink> {
  return api.post<ItemLink>(base.value, body)
}
function linkUndo(created: boolean, l: ItemLink) {
  const body = { target_type: l.target_type, target_id: l.target_id ?? undefined, url: l.url || undefined, label: l.label || undefined }
  const target = l.target_type === 'url' ? l.label || l.url : l.target_key
  const path = base.value
  return toggleEntry(
    t(created ? 'undo.a.itemLink' : 'undo.a.itemUnlink', { from: l.source_key, to: target }),
    created,
    l.id,
    async () => (await api.post<ItemLink>(path, body)).id,
    async (id) => void (await api.del(`/item-links/${id}`)),
  )
}
async function save() {
  saving.value = true
  formError.value = ''
  try {
    const body = type.value === 'url' ? { target_type: 'url', url: url.value.trim(), label: label.value.trim() } : { target_type: type.value, target_id: targetId.value, label: label.value.trim() }
    const l = await create(body)
    recordUndo(linkUndo(true, l))
    dialog.value = false
    notify.ok(t('links.added'))
    await load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
/** requirements and tests are picked with the item picker (several at once) */
async function addPicked(items: { id: string }[]) {
  let ok = 0
  for (const it of items) {
    try {
      const l = await create({ target_type: type.value, target_id: it.id, label: label.value.trim() })
      recordUndo(linkUndo(true, l))
      ok++
    } catch (e) {
      notify.fail(e, t('links.addFailed'))
    }
  }
  if (ok) {
    dialog.value = false
    notify.ok(t('links.addedCount', { count: ok }, ok))
  }
  await load()
}
async function remove(l: ItemLink) {
  try {
    await api.del(`/item-links/${l.id}`)
    recordUndo(linkUndo(false, l))
    links.value = links.value.filter((x) => x.id !== l.id)
    emit('count', links.value.length)
  } catch (e) {
    notify.fail(e)
  }
}

const exclude = computed(() => [
  ...(type.value === props.itemType ? [props.itemId] : []),
  ...outgoing.value.filter((l) => l.target_type === type.value && l.target_id).map((l) => l.target_id as string),
])
const canSave = computed(() => (type.value === 'url' ? /^(https?:\/\/\S+|mailto:\S+)$/i.test(url.value.trim()) : !!targetId.value))
const target = (l: ItemLink, incomingSide = false) => linkTarget(l, props.projectId, incomingSide)
</script>

<template>
  <section class="item-links" :aria-label="t('links.title')">
    <div class="row" style="justify-content: space-between; margin-bottom: 6px">
      <h3 class="plain-heading small-heading">{{ t('links.title') }} <span v-if="links.length" class="count">{{ links.length }}</span></h3>
      <Button v-if="canWrite" :label="t('links.add')" icon="pi pi-link" size="small" text @click="openAdd" />
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <p v-else-if="!loading && !links.length" class="small muted" style="margin: 0">{{ t('links.none') }}</p>
    <ul v-if="outgoing.length" class="plain-list link-list">
      <li v-for="l in outgoing" :key="l.id" class="row nowrap">
        <i :class="LINK_ICONS[l.target_type as LinkType] ?? 'pi pi-link'" aria-hidden="true" class="muted" />
        <span class="sr-only">{{ t(`links.type.${l.target_type}`) }}:</span>
        <template v-if="target(l).external">
          <a :href="target(l).href" target="_blank" rel="noopener noreferrer" class="grow ellipsis link-text" :title="l.url"
            >{{ l.label || l.url }} <i class="pi pi-external-link small" aria-hidden="true" /><span class="sr-only"> ({{ t('links.newTab') }})</span></a
          >
        </template>
        <a v-else-if="target(l).href" :href="target(l).href" target="_blank" rel="noopener" class="grow ellipsis link-text">
          <b v-if="l.label">{{ l.label + ': ' }}</b>{{ l.target_key || t('links.missing') }}
        </a>
        <RouterLink v-else-if="target(l).to" :to="target(l).to!" class="grow ellipsis link-text">
          <b v-if="l.label">{{ l.label + ': ' }}</b><span :class="{ key: l.target_type === 'requirement' || l.target_type === 'test' }">{{ l.target_key }}</span>
          <span v-if="l.target_title && (l.target_type === 'requirement' || l.target_type === 'test')" class="muted">{{ ' ' + l.target_title }}</span>
        </RouterLink>
        <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('links.remove', { name: l.label || l.target_key || l.url })" @click="remove(l)" />
      </li>
    </ul>
    <template v-if="incoming.length">
      <h4 class="plain-heading small muted" style="margin: 8px 0 4px">{{ t('links.linkedFrom') }}</h4>
      <ul class="plain-list link-list">
        <li v-for="l in incoming" :key="l.id" class="row nowrap">
          <i :class="LINK_ICONS[l.source_type as LinkType] ?? 'pi pi-link'" aria-hidden="true" class="muted" />
          <RouterLink v-if="target(l, true).to" :to="target(l, true).to!" class="grow ellipsis link-text">
            <span :class="{ key: l.source_type !== 'run' }">{{ l.source_key }}</span> <span class="muted">{{ l.source_title }}</span>
            <span v-if="l.label" class="muted small"> · {{ l.label }}</span>
          </RouterLink>
        </li>
      </ul>
    </template>

    <Dialog v-model:visible="dialog" modal :header="t('links.addTitle')" style="width: min(560px, 95vw)">
      <form class="grid" style="gap: 12px" @submit.prevent="save">
        <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
        <div class="field">
          <span id="link-type" class="field-label">{{ t('links.linkTo') }}</span>
          <SelectButton v-model="type" :options="typeOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="link-type" class="link-types">
            <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /> {{ option.label }}</template>
          </SelectButton>
        </div>
        <div v-if="type === 'url'" class="field">
          <label for="link-url">{{ t('links.url') }}</label>
          <InputText id="link-url" v-model="url" type="url" inputmode="url" placeholder="https://" maxlength="2000" autofocus />
          <span class="small muted">{{ t('links.urlHint') }}</span>
        </div>
        <div v-else-if="type === 'requirement' || type === 'test'" class="field">
          <span class="small muted">{{ t('links.pickHint') }}</span>
        </div>
        <div v-else class="field">
          <label for="link-target">{{ t(`links.type.${type}`) }}</label>
          <Select v-model="targetId" input-id="link-target" :options="options" option-label="label" option-value="value" filter :placeholder="t('links.choose')" :empty-message="t('links.noOptions')" />
        </div>
        <div class="field">
          <label for="link-label">{{ t('links.label') }} {{ t('prompts.optional') }}</label>
          <InputText id="link-label" v-model="label" maxlength="200" :placeholder="t('links.labelPlaceholder')" />
        </div>
        <div class="row" style="justify-content: flex-end">
          <Button :label="t('common.cancel')" severity="secondary" text @click="dialog = false" />
          <Button v-if="type === 'requirement' || type === 'test'" :label="t('links.chooseItems')" icon="pi pi-search" @click="picker = true" />
          <Button v-else type="submit" :label="t('links.add')" icon="pi pi-check" :loading="saving" :disabled="!canSave" />
        </div>
      </form>
    </Dialog>
    <ItemPickerDialog
      v-if="type === 'requirement' || type === 'test'"
      v-model:visible="picker"
      :kind="type"
      :project-id="projectId"
      :exclude="exclude"
      :header="t('links.chooseItems')"
      :confirm-label="t('links.add')"
      @pick="addPicked"
    />
  </section>
</template>

<style>
.item-links .link-list li {
  gap: 8px;
  padding: 3px 0;
  min-height: 32px;
}
.item-links .link-text {
  min-width: 0;
}
.small-heading {
  font-size: 14px;
  margin: 0;
}
.link-types {
  flex-wrap: wrap;
}
</style>
