<script setup lang="ts">
// Custom field definitions: of a project (project settings, projectId) or
// of the organisation's field set of a project type (administration,
// projectType). Deleting a project field asks for confirmation and keeps
// its values in the field archive.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import CustomFieldsEditor from './CustomFieldsEditor.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { formatDateTime } from '@/lib/format'
import {
  CUSTOM_FIELD_ENTITIES,
  CUSTOM_FIELD_TYPES,
  fieldsOf,
  keyFromLabel,
  type CustomField,
  type CustomFieldArchive,
  type CustomFieldEntity,
  type CustomFieldType,
  type CustomValues,
} from '@/lib/customFields'
import { useNotify } from '@/composables/notify'
import { useProjectStore } from '@/stores/project'

const props = defineProps<{ projectId?: string; projectType?: string; canEdit: boolean; projectTypes?: string[] }>()
const { t } = useI18n()
const notify = useNotify()
const proj = useProjectStore()
const base = uid('cfs')
const isProject = computed(() => !!props.projectId)

const entity = ref<CustomFieldEntity>('requirement')
const entityOptions = computed(() => CUSTOM_FIELD_ENTITIES.map((e) => ({ value: e, label: t(`customFields.entity.${e}`) })))
const typeOptions = computed(() => CUSTOM_FIELD_TYPES.map((v) => ({ value: v, label: t(`customFields.type.${v}`) })))
const all = ref<CustomField[]>([])
const error = ref('')
const list = computed(() => fieldsOf(all.value, entity.value))

async function load() {
  error.value = ''
  try {
    all.value = isProject.value
      ? asItems(await api.get<{ items: CustomField[] }>(`/projects/${props.projectId}/custom-fields`))
      : asItems(await api.get<{ items: CustomField[] }>('/field-sets', { project_type: props.projectType ?? '' }))
  } catch (e) {
    error.value = errorMessage(e)
  }
  if (isProject.value) void loadArchive()
}
watch(() => [props.projectId, props.projectType], load, { immediate: true })
async function changed() {
  await load()
  if (isProject.value) await proj.loadCustomFields()
}

// ---------------------------------------------------------------- edit
interface Draft {
  id?: string
  label: string
  key: string
  type: CustomFieldType
  unit: string
  options: string
  required: boolean
  default: CustomValues
  help: string
  show_in_list: boolean
}
const dialog = ref(false)
const draft = ref<Draft>(blank())
const keyTouched = ref(false)
const busy = ref(false)
const dialogError = ref('')
function blank(): Draft {
  return { label: '', key: '', type: 'text', unit: '', options: '', required: false, default: {}, help: '', show_in_list: false }
}
function openNew() {
  draft.value = blank()
  keyTouched.value = false
  dialogError.value = ''
  dialog.value = true
}
function openEdit(f: CustomField) {
  draft.value = { id: f.id, label: f.label, key: f.key, type: f.type, unit: f.unit, options: f.options.join('\n'), required: f.required, default: { [f.key]: f.default }, help: f.help, show_in_list: f.show_in_list }
  keyTouched.value = true
  dialogError.value = ''
  dialog.value = true
}
watch(
  () => draft.value.label,
  (l) => {
    if (!draft.value.id && !keyTouched.value) draft.value.key = l.trim() ? keyFromLabel(l) : ''
  },
)
const isSelect = computed(() => draft.value.type === 'single_select' || draft.value.type === 'multi_select')
const optionList = computed(() => draft.value.options.split('\n').map((s) => s.trim()).filter(Boolean))
/** the draft as a field, for the default value editor */
const draftField = computed<CustomField>(() => ({
  id: '',
  entity_type: entity.value,
  key: draft.value.key || 'value',
  label: t('customFields.default'),
  type: draft.value.type,
  unit: draft.value.unit,
  options: optionList.value,
  required: false,
  default: null,
  help: '',
  position: 0,
  show_in_list: false,
}))
const draftDefault = computed({
  get: () => ({ [draftField.value.key]: Object.values(draft.value.default)[0] ?? null }),
  set: (v: CustomValues) => (draft.value.default = v),
})

async function save() {
  const d = draft.value
  const body = {
    entity_type: entity.value,
    label: d.label,
    key: d.key,
    type: d.type,
    unit: d.type === 'number' ? d.unit : '',
    options: isSelect.value ? optionList.value : [],
    required: d.required,
    default: d.type === 'user' ? null : (Object.values(d.default)[0] ?? null),
    help: d.help,
    show_in_list: d.show_in_list,
  }
  busy.value = true
  dialogError.value = ''
  try {
    if (d.id) await api.patch(isProject.value ? `/custom-fields/${d.id}` : `/field-sets/fields/${d.id}`, body)
    else await api.post(isProject.value ? `/projects/${props.projectId}/custom-fields` : `/field-sets/${props.projectType}/fields`, body)
    dialog.value = false
    notify.ok(t('customFields.saved'))
    await changed()
  } catch (e) {
    dialogError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

async function move(f: CustomField, delta: number) {
  const l = list.value
  const i = l.indexOf(f)
  const other = l[i + delta]
  if (!other) return
  try {
    const path = (x: CustomField) => (isProject.value ? `/custom-fields/${x.id}` : `/field-sets/fields/${x.id}`)
    // positions follow the list order, then the two swap
    await Promise.all(l.map((x, j) => (x.position !== j + 1 && x !== f && x !== other ? api.patch(path(x), { position: j + 1 }) : null)))
    await api.patch(path(f), { position: i + delta + 1 })
    await api.patch(path(other), { position: i + 1 })
    await changed()
  } catch (e) {
    notify.fail(e)
  }
}

async function remove(f: CustomField) {
  if (!isProject.value) {
    notify.confirmDanger(t('customFields.confirmDeleteSet', { label: f.label }), async () => {
      try {
        await api.del(`/field-sets/fields/${f.id}`)
        await changed()
      } catch (e) {
        notify.fail(e)
      }
    })
    return
  }
  let usage = 0
  try {
    usage = (await api.get<{ items: number }>(`/custom-fields/${f.id}/usage`)).items
  } catch (e) {
    notify.fail(e)
    return
  }
  notify.confirmDanger(t('customFields.confirmDelete', { label: f.label, n: usage }, usage), async () => {
    try {
      await api.del(`/custom-fields/${f.id}?confirm=true`)
      notify.ok(t('customFields.deleted', { label: f.label, n: usage }, usage))
      await changed()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------------------------------------------------------------- field set (project)
const setType = ref('')
watch(
  () => proj.project?.type,
  (v) => (setType.value = v ?? 'general'),
  { immediate: true },
)
const setTypes = computed(() => (props.projectTypes ?? ['general', 'software', 'hardware', 'system', 'business']).map((v) => ({ value: v, label: t(`projectType.${v}`) })))
const applying = ref(false)
async function applySet() {
  applying.value = true
  try {
    const res = await api.post<{ added: string[]; skipped: string[] }>(`/projects/${props.projectId}/custom-fields/apply-set`, { project_type: setType.value })
    notify.ok(t('customFields.setApplied', { added: res.added.length, skipped: res.skipped.length }))
    await changed()
  } catch (e) {
    notify.fail(e)
  } finally {
    applying.value = false
  }
}

// ---------------------------------------------------------------- archive (project)
const archive = ref<CustomFieldArchive[]>([])
async function loadArchive() {
  try {
    archive.value = asItems(await api.get<{ items: CustomFieldArchive[] }>(`/projects/${props.projectId}/custom-fields/archive`))
  } catch {
    archive.value = []
  }
}
</script>

<template>
  <section class="card cf-settings" style="max-width: 980px" :aria-labelledby="`${base}-h`">
    <div class="row" style="margin-bottom: 8px">
      <h2 :id="`${base}-h`" class="plain-heading" style="margin: 0">{{ t('customFields.title') }}</h2>
      <span class="grow" />
      <Button v-if="canEdit" :label="t('customFields.new')" icon="pi pi-plus" aria-haspopup="dialog" @click="openNew" />
    </div>
    <p class="small muted" style="margin-top: 0">{{ isProject ? t('customFields.intro') : t('customFields.introSet') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <SelectButton v-model="entity" :options="entityOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('customFields.entityLabel')" class="cf-entities" />
    <div v-if="!list.length" class="empty small" style="padding: 18px 0"><i class="pi pi-th-large" aria-hidden="true" /> {{ t('customFields.none') }}</div>
    <table v-else class="cf-table" :aria-label="t('customFields.tableLabel', { entity: t(`customFields.entity.${entity}`) })">
      <thead>
        <tr>
          <th scope="col">{{ t('customFields.label') }}</th>
          <th scope="col">{{ t('customFields.key') }}</th>
          <th scope="col">{{ t('customFields.typeLabel') }}</th>
          <th scope="col">{{ t('customFields.flags') }}</th>
          <th v-if="canEdit" scope="col"><span class="sr-only">{{ t('common.actions') }}</span></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(f, i) in list" :key="f.id">
          <td>
            <b>{{ f.label }}</b><span v-if="f.unit" class="muted"> ({{ f.unit }})</span>
            <div v-if="f.help" class="small muted">{{ f.help }}</div>
            <div v-if="f.options.length" class="small muted">{{ f.options.join(', ') }}</div>
          </td>
          <td><code>cf.{{ f.key }}</code></td>
          <td>{{ t(`customFields.type.${f.type}`) }}</td>
          <td class="row" style="gap: 4px">
            <Tag v-if="f.required" :value="t('customFields.required')" severity="warn" />
            <Tag v-if="f.show_in_list" :value="t('customFields.inList')" severity="info" />
            <Tag v-if="f.default !== null && f.default !== undefined && f.default !== ''" :value="t('customFields.hasDefault')" severity="secondary" />
          </td>
          <td v-if="canEdit" class="row nowrap" style="gap: 0">
            <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :disabled="i === 0" :aria-label="t('customFields.moveUp', { label: f.label })" @click="move(f, -1)" />
            <Button icon="pi pi-arrow-down" text rounded size="small" severity="secondary" :disabled="i === list.length - 1" :aria-label="t('customFields.moveDown', { label: f.label })" @click="move(f, 1)" />
            <Button icon="pi pi-pencil" text rounded size="small" :aria-label="t('customFields.edit', { label: f.label })" aria-haspopup="dialog" @click="openEdit(f)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('customFields.delete', { label: f.label })" @click="remove(f)" />
          </td>
        </tr>
      </tbody>
    </table>

    <div v-if="isProject && canEdit" class="cf-apply">
      <h3 class="plain-heading" style="margin: 0 0 4px; font-size: 14px">{{ t('customFields.applySetTitle') }}</h3>
      <p class="small muted" style="margin: 0 0 8px">{{ t('customFields.applySetHint') }}</p>
      <div class="row">
        <label :for="`${base}-set`" class="sr-only">{{ t('projectType.label') }}</label>
        <Select v-model="setType" :input-id="`${base}-set`" :options="setTypes" option-label="label" option-value="value" style="min-width: 180px" />
        <Button :label="t('customFields.applySet')" icon="pi pi-download" severity="secondary" outlined :loading="applying" @click="applySet" />
      </div>
    </div>

    <details v-if="isProject && archive.length" class="cf-archive">
      <summary>{{ t('customFields.archiveTitle', { n: archive.length }, archive.length) }}</summary>
      <ul class="small">
        <li v-for="a in archive" :key="a.id">
          <b>{{ a.label }}</b> (<code>cf.{{ a.field_key }}</code>, {{ t(`customFields.entity.${a.entity_type}`) }}) –
          {{ t('customFields.archiveLine', { n: a.item_count, user: a.deleted_by, date: formatDateTime(a.deleted_at) }, a.item_count) }}
        </li>
      </ul>
    </details>
  </section>

  <Dialog v-model:visible="dialog" modal :header="draft.id ? t('customFields.editTitle') : t('customFields.newTitle')" :style="{ width: 'min(560px, 96vw)' }">
    <form class="grid cf-form" style="gap: 12px" @submit.prevent="save">
      <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
      <div class="field">
        <label :for="`${base}-label`">{{ t('customFields.label') }}</label>
        <InputText :id="`${base}-label`" v-model="draft.label" maxlength="100" required autofocus />
      </div>
      <div class="field">
        <label :for="`${base}-key`">{{ t('customFields.key') }}</label>
        <InputText :id="`${base}-key`" v-model="draft.key" class="mono" maxlength="40" :disabled="!!draft.id" :aria-describedby="`${base}-key-hint`" @input="keyTouched = true" />
        <small :id="`${base}-key-hint`" class="muted">{{ t('customFields.keyHint', { key: draft.key || 'key' }) }}</small>
      </div>
      <div class="field">
        <label :for="`${base}-type`">{{ t('customFields.typeLabel') }}</label>
        <Select v-model="draft.type" :input-id="`${base}-type`" :options="typeOptions" option-label="label" option-value="value" fluid @update:model-value="draft.default = {}" />
      </div>
      <div v-if="draft.type === 'number'" class="field">
        <label :for="`${base}-unit`">{{ t('customFields.unit') }}</label>
        <InputText :id="`${base}-unit`" v-model="draft.unit" maxlength="20" :placeholder="t('customFields.unitPlaceholder')" />
      </div>
      <div v-if="isSelect" class="field">
        <label :for="`${base}-opts`">{{ t('customFields.options') }}</label>
        <Textarea :id="`${base}-opts`" v-model="draft.options" rows="4" auto-resize :aria-describedby="`${base}-opts-hint`" />
        <small :id="`${base}-opts-hint`" class="muted">{{ t('customFields.optionsHint') }}</small>
      </div>
      <CustomFieldsEditor v-if="draft.type !== 'user' && (!isSelect || optionList.length)" v-model="draftDefault" :fields="[draftField]" />
      <div class="field">
        <label :for="`${base}-help`">{{ t('customFields.help') }}</label>
        <Textarea :id="`${base}-help`" v-model="draft.help" rows="2" auto-resize maxlength="1000" />
      </div>
      <div class="row" style="gap: 16px">
        <label class="row nowrap small" style="gap: 6px"><Checkbox v-model="draft.required" binary :input-id="`${base}-req`" />{{ t('customFields.requiredLabel') }}</label>
        <label class="row nowrap small" style="gap: 6px"><Checkbox v-model="draft.show_in_list" binary :input-id="`${base}-list`" />{{ t('customFields.showInList') }}</label>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="!draft.label.trim() || (isSelect && !optionList.length)" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.cf-entities {
  flex-wrap: wrap;
  margin: 6px 0 10px;
}
.cf-table {
  width: 100%;
  border-collapse: collapse;
}
.cf-table th,
.cf-table td {
  text-align: left;
  padding: 6px 8px;
  border-bottom: 1px solid var(--qtf-border);
  vertical-align: top;
}
.cf-table th {
  font-size: 12px;
  color: var(--qtf-muted);
}
.cf-apply,
.cf-archive {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--qtf-border);
}
@media (max-width: 640px) {
  .cf-table thead {
    display: none;
  }
  .cf-table tr {
    display: grid;
    grid-template-columns: 1fr auto;
    border-bottom: 1px solid var(--qtf-border);
  }
  .cf-table td {
    border: 0;
  }
}
</style>
