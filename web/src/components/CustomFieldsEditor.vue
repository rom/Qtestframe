<script setup lang="ts">
// The custom fields of an item in its editor (requirements, tests, defects,
// risks, runs). v-model is the item's custom_fields object; a cleared
// value becomes null so that a PATCH removes it. Kept apart from the
// editors so that they only mount this component.
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import Checkbox from 'primevue/checkbox'
import { uid } from '@/lib/a11y'
import { isEmpty, type CustomField, type CustomValue, type CustomValues } from '@/lib/customFields'
import { useProjectPeopleStore } from '@/stores/projectPeople'

const props = withDefaults(
  defineProps<{
    modelValue: CustomValues | undefined
    fields: CustomField[]
    disabled?: boolean
    projectId?: string
    /** mark empty required fields (after a failed save) */
    showErrors?: boolean
  }>(),
  { disabled: false, projectId: '', showErrors: false },
)
const emit = defineEmits<{ 'update:modelValue': [value: CustomValues] }>()
const { t } = useI18n()
const base = uid('cf')
const people = useProjectPeopleStore()
const hasUser = computed(() => props.fields.some((f) => f.type === 'user'))
onMounted(() => {
  if (hasUser.value && props.projectId) void people.ensure(props.projectId)
})
const userOptions = computed(() =>
  people.people
    .filter((p) => p.kind === 'user' && p.email)
    .map((p) => ({ value: p.email!.toLowerCase(), label: p.name && p.name !== p.email ? `${p.name} <${p.email}>` : p.email! })),
)

const id = (f: CustomField) => `${base}-${f.key}`
const helpId = (f: CustomField) => `${base}-${f.key}-help`
const value = (f: CustomField) => props.modelValue?.[f.key] ?? null
function set(f: CustomField, v: CustomValue | undefined) {
  const out: CustomValues = { ...(props.modelValue ?? {}) }
  out[f.key] = v === undefined || v === '' || (Array.isArray(v) && !v.length) ? null : v
  emit('update:modelValue', out)
}
const invalid = (f: CustomField) => props.showErrors && f.required && f.type !== 'checkbox' && isEmpty(value(f))
function describedBy(f: CustomField) {
  return f.help ? helpId(f) : undefined
}
/** options of a user field: the members, plus a value that is not one of them */
function usersOf(f: CustomField) {
  const v = value(f)
  if (typeof v === 'string' && v && !userOptions.value.some((o) => o.value === v)) return [{ value: v, label: v }, ...userOptions.value]
  return userOptions.value
}
// kept in script: vue-tsc mis-parses "//" inside a static attribute
const urlPlaceholder = 'https://'
/** the value of a URL field when it is a link that can be opened */
function linkOf(f: CustomField): string {
  const v = value(f)
  return typeof v === 'string' && /^https?:[/][/]/i.test(v) ? v : ''
}
const selectOptions = (f: CustomField) => {
  const v = value(f)
  // a stored value whose option was removed is still shown
  const extra = typeof v === 'string' && v && !f.options.includes(v) ? [v] : Array.isArray(v) ? v.filter((x) => !f.options.includes(x)) : []
  return [...f.options, ...extra].map((o) => ({ value: o, label: o }))
}
</script>

<template>
  <fieldset v-if="fields.length" class="plain-fieldset cf-editor" :aria-describedby="`${base}-intro`">
    <legend class="section-title"><i class="pi pi-th-large" aria-hidden="true" /> {{ t('customFields.section') }}</legend>
    <span :id="`${base}-intro`" class="sr-only">{{ t('customFields.sectionHint') }}</span>
    <div class="cf-grid">
      <div v-for="f in fields" :key="f.key" class="field" :class="{ full: f.type === 'long_text' }" :data-cf="f.key">
        <div v-if="f.type === 'checkbox'" class="row nowrap" style="gap: 8px; min-height: 36px">
          <Checkbox :model-value="value(f) === true" binary :input-id="id(f)" :disabled="disabled" :aria-describedby="describedBy(f)" @update:model-value="(v: boolean) => set(f, v)" />
          <label :for="id(f)" class="cf-check-label">{{ f.label }}</label>
        </div>
        <template v-else>
          <label :for="id(f)">
            {{ f.label }}<template v-if="f.unit"> ({{ f.unit }})</template>
            <span v-if="f.required" aria-hidden="true"> *</span>
            <span v-if="f.required" class="sr-only"> ({{ t('customFields.required') }})</span>
          </label>
          <InputText v-if="f.type === 'text'" :id="id(f)" :model-value="(value(f) as string) ?? ''" maxlength="1000" :disabled="disabled" :invalid="invalid(f)" :aria-required="f.required" :aria-describedby="describedBy(f)" @update:model-value="(v) => set(f, v ?? '')" />
          <Textarea v-else-if="f.type === 'long_text'" :id="id(f)" :model-value="(value(f) as string) ?? ''" rows="3" auto-resize :disabled="disabled" :invalid="invalid(f)" :aria-required="f.required" :aria-describedby="describedBy(f)" @update:model-value="(v) => set(f, v)" />
          <InputNumber
            v-else-if="f.type === 'number'"
            :input-id="id(f)"
            :model-value="typeof value(f) === 'number' ? (value(f) as number) : null"
            :min-fraction-digits="0"
            :max-fraction-digits="10"
            :use-grouping="false"
            :suffix="f.unit ? ` ${f.unit}` : undefined"
            :disabled="disabled"
            :invalid="invalid(f)"
            :aria-required="f.required"
            :aria-describedby="describedBy(f)"
            fluid
            @update:model-value="(v: number | null) => set(f, v)"
          />
          <InputText v-else-if="f.type === 'date'" :id="id(f)" type="date" :model-value="(value(f) as string) ?? ''" :disabled="disabled" :invalid="invalid(f)" :aria-required="f.required" :aria-describedby="describedBy(f)" @update:model-value="(v) => set(f, v ?? '')" />
          <Select
            v-else-if="f.type === 'single_select'"
            :input-id="id(f)"
            :model-value="value(f)"
            :options="selectOptions(f)"
            option-label="label"
            option-value="value"
            :show-clear="!f.required"
            :placeholder="t('customFields.choose')"
            :disabled="disabled"
            :invalid="invalid(f)"
            :aria-describedby="describedBy(f)"
            fluid
            @update:model-value="(v) => set(f, v)"
          />
          <MultiSelect
            v-else-if="f.type === 'multi_select'"
            :input-id="id(f)"
            :model-value="Array.isArray(value(f)) ? (value(f) as string[]) : []"
            :options="selectOptions(f)"
            option-label="label"
            option-value="value"
            display="chip"
            :placeholder="t('customFields.choose')"
            :disabled="disabled"
            :invalid="invalid(f)"
            :aria-describedby="describedBy(f)"
            fluid
            @update:model-value="(v: string[]) => set(f, v)"
          />
          <Select
            v-else-if="f.type === 'user'"
            :input-id="id(f)"
            :model-value="value(f)"
            :options="usersOf(f)"
            option-label="label"
            option-value="value"
            filter
            show-clear
            :placeholder="t('customFields.chooseUser')"
            :disabled="disabled"
            :invalid="invalid(f)"
            :aria-describedby="describedBy(f)"
            fluid
            @update:model-value="(v) => set(f, v)"
          />
          <div v-else-if="f.type === 'url'" class="row nowrap" style="gap: 6px">
            <InputText :id="id(f)" type="url" class="grow" :model-value="(value(f) as string) ?? ''" :placeholder="urlPlaceholder" :disabled="disabled" :invalid="invalid(f)" :aria-required="f.required" :aria-describedby="describedBy(f)" @update:model-value="(v) => set(f, v ?? '')" />
            <a v-if="linkOf(f)" :href="linkOf(f)" target="_blank" rel="noopener noreferrer" class="small" :aria-label="t('customFields.openLink', { field: f.label })"><i class="pi pi-external-link" aria-hidden="true" /></a>
          </div>
        </template>
        <small v-if="f.help" :id="helpId(f)" class="muted">{{ f.help }}</small>
        <small v-if="invalid(f)" class="warn-text" role="alert">{{ t('customFields.requiredError', { field: f.label }) }}</small>
      </div>
    </div>
  </fieldset>
</template>

<style scoped>
.cf-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr));
  gap: 12px;
}
.cf-grid .full {
  grid-column: 1 / -1;
}
.cf-check-label {
  font-size: 13px;
  font-weight: 600;
}
</style>
