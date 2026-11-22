<script setup lang="ts">
// Item templates: of a project (project settings, projectId: its own
// templates, the default template, and read-only organisation and built-in
// templates to copy) or of the organisation for a project type
// (administration, projectType).
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import TagsInput from './TagsInput.vue'
import CustomFieldsEditor from './CustomFieldsEditor.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { enumOptions } from '@/i18n/labels'
import { fieldsOf, groupTemplates, type CustomField, type CustomValues, type ItemTemplate, type TemplateContent, type TemplateKind } from '@/lib/customFields'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { useProjectStore } from '@/stores/project'

const props = defineProps<{ projectId?: string; projectType?: string; canEdit: boolean }>()
const { t, locale } = useI18n()
const notify = useNotify()
const meta = useMetaStore()
const proj = useProjectStore()
const base = uid('tpls')
const isProject = computed(() => !!props.projectId)

const kind = ref<TemplateKind>('requirement')
const kindOptions = computed(() => [
  { value: 'requirement', label: t('templates.kind.requirement') },
  { value: 'test', label: t('templates.kind.test') },
])
const all = ref<ItemTemplate[]>([])
const error = ref('')
const setFields = ref<CustomField[]>([])
const fields = computed(() => fieldsOf(isProject.value ? proj.customFields : setFields.value, kind.value))
const groups = computed(() => groupTemplates(all.value.filter((x) => x.kind === kind.value && (isProject.value || x.source !== 'project'))))

async function load() {
  error.value = ''
  try {
    if (isProject.value) {
      all.value = (await api.get<{ items: ItemTemplate[] }>(`/projects/${props.projectId}/templates`, { lang: locale.value })).items ?? []
    } else {
      all.value = asItems(await api.get<{ items: ItemTemplate[] }>('/template-sets', { project_type: props.projectType ?? '', lang: locale.value }))
      setFields.value = asItems(await api.get<{ items: CustomField[] }>('/field-sets', { project_type: props.projectType ?? '' }))
    }
  } catch (e) {
    error.value = errorMessage(e)
  }
}
watch(() => [props.projectId, props.projectType, locale.value], load, { immediate: true })

const editable = (x: ItemTemplate) => props.canEdit && (isProject.value ? x.source === 'project' : x.source === 'organisation')

// ---------------------------------------------------------------- default
async function setDefault(x: ItemTemplate | null) {
  try {
    await api.put(`/projects/${props.projectId}/templates/default`, { kind: kind.value, template_id: x?.id ?? '' })
    notify.ok(x ? t('templates.defaultSet', { name: x.name }) : t('templates.defaultCleared'))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}

// ---------------------------------------------------------------- edit
interface Draft {
  id?: string
  name: string
  description: string
  content: Required<Pick<TemplateContent, 'title' | 'description' | 'priority' | 'status' | 'tags'>> & TemplateContent
}
const dialog = ref(false)
const busy = ref(false)
const dialogError = ref('')
const draft = ref<Draft>(blank())
function blank(): Draft {
  return { name: '', description: '', content: { title: '', description: '', priority: '', status: '', tags: [], criteria: [], steps: [], custom_fields: {} } }
}
function open(x: ItemTemplate | null, copy = false) {
  const d = blank()
  if (x) {
    d.id = copy ? undefined : x.id
    d.name = copy ? t('templates.copyName', { name: x.name }) : x.name
    d.description = x.description
    const c = JSON.parse(JSON.stringify(x.content)) as TemplateContent
    d.content = { ...d.content, ...c, title: c.title ?? '', description: c.description ?? '', priority: c.priority ?? '', status: c.status ?? '', tags: c.tags ?? [] }
  }
  draft.value = d
  dialogError.value = ''
  dialog.value = true
}
const priorityOptions = computed(() => [{ label: t('templates.unset'), value: '' }, ...enumOptions('priority', meta.meta.priorities)])
const statusOptions = computed(() => [{ label: t('templates.unset'), value: '' }, ...enumOptions('status', kind.value === 'requirement' ? meta.meta.requirement_statuses : meta.meta.test_statuses)])
const obligationOptions = computed(() => [{ label: t('templates.unset'), value: '' }, ...enumOptions('obligation', meta.meta.obligations)])
const typeOptions = computed(() => [{ label: t('templates.unset'), value: '' }, ...enumOptions('requirementType', meta.meta.requirement_types)])
const methodOptions = computed(() => enumOptions('evaluationMethod', meta.meta.evaluation_methods))
const customValues = computed({
  get: () => (draft.value.content.custom_fields ?? {}) as CustomValues,
  set: (v: CustomValues) => (draft.value.content.custom_fields = v),
})

async function save() {
  const d = draft.value
  const c: TemplateContent = { ...d.content }
  c.criteria = (c.criteria ?? []).filter((x) => x.description.trim())
  c.steps = (c.steps ?? []).filter((x) => x.action.trim() || x.expected.trim())
  c.custom_fields = Object.fromEntries(Object.entries(c.custom_fields ?? {}).filter(([, v]) => v !== null && v !== '' && !(Array.isArray(v) && !v.length)))
  const body = { kind: kind.value, name: d.name, description: d.description, content: c }
  busy.value = true
  dialogError.value = ''
  try {
    if (d.id) await api.patch(isProject.value ? `/templates/${d.id}` : `/template-sets/templates/${d.id}`, body)
    else await api.post(isProject.value ? `/projects/${props.projectId}/templates` : `/template-sets/${props.projectType}/templates`, body)
    dialog.value = false
    notify.ok(t('templates.saved'))
    await load()
  } catch (e) {
    dialogError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
function remove(x: ItemTemplate) {
  notify.confirmDanger(t('templates.confirmDelete', { name: x.name }), async () => {
    try {
      await api.del(isProject.value ? `/templates/${x.id}` : `/template-sets/templates/${x.id}`)
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
</script>

<template>
  <section class="card tpl-settings" style="max-width: 980px" :aria-labelledby="`${base}-h`">
    <div class="row" style="margin-bottom: 8px">
      <h2 :id="`${base}-h`" class="plain-heading" style="margin: 0">{{ t('templates.title') }}</h2>
      <span class="grow" />
      <Button v-if="canEdit" :label="t('templates.new')" icon="pi pi-plus" aria-haspopup="dialog" @click="open(null)" />
    </div>
    <p class="small muted" style="margin-top: 0">{{ isProject ? t('templates.intro') : t('templates.introSet') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <SelectButton v-model="kind" :options="kindOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('templates.kindLabel')" style="margin: 6px 0 10px" />
    <div v-if="!groups.length" class="empty small" style="padding: 18px 0">{{ t('templates.noneYet') }}</div>
    <div v-for="g in groups" :key="g.source" class="tpl-group">
      <h3 class="plain-heading tpl-group-title">{{ t(`templates.source.${g.source}`) }}</h3>
      <ul class="tpl-list">
        <li v-for="x in g.items" :key="x.id" class="tpl-row">
          <div class="grow">
            <b>{{ x.name }}</b>
            <Tag v-if="x.default" :value="t('templates.default')" severity="success" style="margin-left: 6px" />
            <div v-if="x.description" class="small muted">{{ x.description }}</div>
            <div class="small muted">
              <template v-if="x.content.criteria?.length">{{ t('templates.criteriaCount', { n: x.content.criteria.length }, x.content.criteria.length) }}</template>
              <template v-if="x.content.steps?.length">{{ t('templates.stepsCount', { n: x.content.steps.length }, x.content.steps.length) }}</template>
            </div>
          </div>
          <div class="row nowrap" style="gap: 0">
            <Button v-if="isProject && canEdit && !x.default" icon="pi pi-star" text rounded size="small" :aria-label="t('templates.makeDefault', { name: x.name })" v-tooltip="t('templates.makeDefault', { name: x.name })" @click="setDefault(x)" />
            <Button v-if="isProject && canEdit && x.default" icon="pi pi-star-fill" text rounded size="small" :aria-label="t('templates.clearDefault', { name: x.name })" v-tooltip="t('templates.clearDefault', { name: x.name })" @click="setDefault(null)" />
            <Button v-if="editable(x)" icon="pi pi-pencil" text rounded size="small" aria-haspopup="dialog" :aria-label="t('templates.edit', { name: x.name })" @click="open(x)" />
            <Button v-if="canEdit" icon="pi pi-copy" text rounded size="small" aria-haspopup="dialog" :aria-label="t('templates.copy', { name: x.name })" v-tooltip="t('templates.copy', { name: x.name })" @click="open(x, true)" />
            <Button v-if="editable(x)" icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('templates.delete', { name: x.name })" @click="remove(x)" />
          </div>
        </li>
      </ul>
    </div>
  </section>

  <Dialog v-model:visible="dialog" modal :header="draft.id ? t('templates.editTitle') : t('templates.newTitle')" :style="{ width: 'min(760px, 96vw)' }">
    <form class="grid tpl-form" style="gap: 12px" @submit.prevent="save">
      <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
      <div class="tpl-grid">
        <div class="field">
          <label :for="`${base}-name`">{{ t('templates.name') }}</label>
          <InputText :id="`${base}-name`" v-model="draft.name" maxlength="200" required autofocus />
        </div>
        <div class="field">
          <label :for="`${base}-desc`">{{ t('templates.description') }}</label>
          <InputText :id="`${base}-desc`" v-model="draft.description" maxlength="2000" />
        </div>
      </div>
      <h3 class="plain-heading" style="margin: 4px 0 0; font-size: 14px">{{ t('templates.prefilled') }}</h3>
      <div class="field">
        <label :for="`${base}-title`">{{ t('common.title') }}</label>
        <InputText :id="`${base}-title`" v-model="draft.content.title" maxlength="500" />
      </div>
      <div class="field">
        <label :for="`${base}-body`">{{ t('templates.skeleton') }}</label>
        <Textarea :id="`${base}-body`" v-model="draft.content.description" rows="5" auto-resize class="mono" />
      </div>
      <div class="tpl-grid">
        <div class="field">
          <label :for="`${base}-prio`">{{ t('common.priority') }}</label>
          <Select v-model="draft.content.priority" :input-id="`${base}-prio`" :aria-label="t('common.priority')" :options="priorityOptions" option-label="label" option-value="value" fluid />
        </div>
        <div class="field">
          <label :for="`${base}-status`">{{ t('common.status') }}</label>
          <Select v-model="draft.content.status" :input-id="`${base}-status`" :aria-label="t('common.status')" :options="statusOptions" option-label="label" option-value="value" fluid />
        </div>
        <template v-if="kind === 'requirement'">
          <div class="field">
            <label :for="`${base}-obl`">{{ t('templates.obligation') }}</label>
            <Select v-model="draft.content.obligation" :input-id="`${base}-obl`" :aria-label="t('templates.obligation')" :options="obligationOptions" option-label="label" option-value="value" fluid />
          </div>
          <div class="field">
            <label :for="`${base}-type`">{{ t('templates.reqType') }}</label>
            <Select v-model="draft.content.type" :input-id="`${base}-type`" :aria-label="t('templates.reqType')" :options="typeOptions" option-label="label" option-value="value" fluid />
          </div>
        </template>
        <div v-else class="field">
          <label :for="`${base}-est`">{{ t('templates.estimate') }}</label>
          <InputNumber v-model="draft.content.estimated_minutes" :input-id="`${base}-est`" :min="0" fluid />
        </div>
      </div>
      <template v-if="kind === 'test'">
        <div class="field">
          <label :for="`${base}-pre`">{{ t('templates.preconditions') }}</label>
          <Textarea :id="`${base}-pre`" v-model="draft.content.preconditions" rows="2" auto-resize />
        </div>
        <div class="field">
          <label :for="`${base}-exp`">{{ t('templates.expected') }}</label>
          <Textarea :id="`${base}-exp`" v-model="draft.content.expected_result" rows="2" auto-resize />
        </div>
      </template>
      <div class="field">
        <span :id="`${base}-tags`" class="field-label">{{ t('common.tags') }}</span>
        <TagsInput v-model="draft.content.tags" :aria-label="t('common.tags')" />
      </div>

      <fieldset v-if="kind === 'requirement'" class="plain-fieldset">
        <legend class="section-title">{{ t('templates.criteria') }}</legend>
        <div v-for="(c, i) in draft.content.criteria" :key="i" class="tpl-row-edit">
          <Textarea v-model="c.description" rows="1" auto-resize :aria-label="t('templates.criterionN', { n: i + 1 })" class="grow" />
          <Select v-model="c.method" :options="methodOptions" option-label="label" option-value="value" :aria-label="t('templates.methodN', { n: i + 1 })" />
          <InputText v-model="c.threshold" :aria-label="t('templates.thresholdN', { n: i + 1 })" :placeholder="t('templates.threshold')" />
          <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('templates.removeCriterion', { n: i + 1 })" @click="draft.content.criteria!.splice(i, 1)" />
        </div>
        <Button :label="t('templates.addCriterion')" icon="pi pi-plus" text size="small" @click="draft.content.criteria!.push({ description: '', method: 'test', threshold: '' })" />
      </fieldset>
      <fieldset v-else class="plain-fieldset">
        <legend class="section-title">{{ t('templates.steps') }}</legend>
        <div v-for="(s, i) in draft.content.steps" :key="i" class="tpl-row-edit">
          <Textarea v-model="s.action" rows="1" auto-resize :aria-label="t('templates.actionN', { n: i + 1 })" :placeholder="t('templates.action')" class="grow" />
          <Textarea v-model="s.expected" rows="1" auto-resize :aria-label="t('templates.expectedN', { n: i + 1 })" :placeholder="t('templates.expectedStep')" class="grow" />
          <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('templates.removeStep', { n: i + 1 })" @click="draft.content.steps!.splice(i, 1)" />
        </div>
        <Button :label="t('templates.addStep')" icon="pi pi-plus" text size="small" @click="draft.content.steps!.push({ action: '', expected: '', data: '' })" />
      </fieldset>
      <CustomFieldsEditor v-if="fields.length" v-model="customValues" :fields="fields.map((f) => ({ ...f, required: false }))" :project-id="projectId" />
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="!draft.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.tpl-group-title {
  font-size: 13px;
  margin: 12px 0 4px;
  color: var(--qtf-muted);
}
.tpl-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.tpl-row {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding: 6px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.tpl-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(200px, 100%), 1fr));
  gap: 12px;
}
.tpl-row-edit {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: flex-start;
  margin-bottom: 6px;
}
</style>
