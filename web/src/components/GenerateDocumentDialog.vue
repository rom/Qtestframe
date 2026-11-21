<script setup lang="ts">
// "Create document": a requirements specification or a test plan, generated
// by the server from the requirements and tests (or a baseline) and saved in
// the document archive. Emits the new file.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import QueryInput from '@/components/QueryInput.vue'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { FORMAT_INFO } from '@/lib/reports'
import { DOC_FIELDS, DOC_FORMATS, DOC_GROUPINGS, DOC_KINDS, blankDocForm, docFormProblems, generateDocBody, switchDocKind, type DocForm, type DocKind } from '@/lib/documents'
import type { Baseline, FileInfo } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'
import { currentLang } from '@/i18n'
import { LANGUAGES } from '@/i18n/select'
import { uid } from '@/lib/a11y'
import { formatDateTime } from '@/lib/format'

const props = defineProps<{ kind?: DocKind }>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ generated: [file: FileInfo] }>()
const proj = useProjectStore()
const auth = useAuthStore()
const { t } = useI18n()
const id = uid('gd')

const form = ref<DocForm>(blankDocForm(props.kind ?? 'requirements_spec', currentLang()))
const baselines = ref<Baseline[]>([])
const busy = ref(false)
const elapsed = ref(0)
const error = ref<unknown>(null)
const showProblems = ref(false)
let timer: ReturnType<typeof setInterval> | undefined

const problems = computed(() => docFormProblems(form.value))
const isSpec = computed(() => form.value.kind === 'requirements_spec')
const kindOptions = computed(() => DOC_KINDS.map((k) => ({ label: t(`documents.generate.kind.${k}`), value: k })))
const formatOptions = computed(() => DOC_FORMATS.map((f) => ({ label: FORMAT_INFO[f].label, value: f, icon: FORMAT_INFO[f].icon })))
const langOptions = computed(() => LANGUAGES.map((l) => ({ label: t(`documents.generate.lang.${l}`), value: l })))
const groupOptions = computed(() => DOC_GROUPINGS.map((g) => ({ label: t(`documents.generate.group.${g}`), value: g })))
const dimensionOptions = computed(() => {
  const dims = [...new Set(proj.categoriesFor('requirement').map((c) => c.dimension))].sort((a, b) => a.localeCompare(b))
  return [{ label: t('documents.generate.firstDimension'), value: '' }, ...dims.map((d) => ({ label: d, value: d }))]
})
const baselineOptions = computed(() => [
  { label: t('documents.generate.currentData'), value: '' },
  ...baselines.value.map((b) => ({ label: `${b.name} · ${formatDateTime(b.created_at)}`, value: b.id })),
])
const fieldOptions = computed(() => DOC_FIELDS[form.value.kind].map((f) => ({ label: t(`documents.generate.field.${f}`), value: f })))
const queryError = computed(() => (error.value instanceof ApiError && error.value.code === 'invalid_query' ? error.value : null))
const otherError = computed(() => (error.value && !queryError.value ? errorMessage(error.value) : ''))

function setKind(k: DocKind | null) {
  if (k) form.value = switchDocKind(form.value, k)
}

async function loadBaselines() {
  try {
    baselines.value = asItems(await api.get<{ items: Baseline[] }>(`/projects/${proj.pid}/baselines`))
  } catch {
    baselines.value = [] // the current data stays available
  }
}

watch(
  visible,
  (v) => {
    if (!v) return
    form.value = blankDocForm(props.kind ?? form.value.kind, currentLang())
    error.value = null
    showProblems.value = false
    void loadBaselines()
  },
  { immediate: true },
)

async function generate() {
  showProblems.value = true
  if (problems.value.length || busy.value) return
  busy.value = true
  error.value = null
  elapsed.value = 0
  timer = setInterval(() => elapsed.value++, 1000)
  try {
    const file = await api.post<FileInfo>(`/projects/${proj.pid}/documents/generate`, generateDocBody(form.value))
    visible.value = false
    emit('generated', file)
  } catch (e) {
    error.value = e
  } finally {
    clearInterval(timer)
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('documents.generate.title')" :closable="!busy" :close-on-escape="!busy" :style="{ width: 'min(640px, calc(100vw - 16px))' }" class="generate-doc-dialog">
    <form :id="`${id}-form`" class="grid" style="gap: 14px" :aria-busy="busy" @submit.prevent="generate">
      <div>
        <SelectButton :model-value="form.kind" :options="kindOptions" option-label="label" option-value="value" :allow-empty="false" :disabled="busy" :aria-label="t('documents.generate.kindLabel')" :aria-describedby="`${id}-kind-help`" @update:model-value="setKind" />
        <p :id="`${id}-kind-help`" class="small muted" style="margin: 6px 0 0">{{ t(`documents.generate.kindHelp.${form.kind}`) }}</p>
      </div>

      <div class="gd-grid">
        <div v-field class="field">
          <label :for="`${id}-title`">{{ t('documents.generate.docTitle') }}</label>
          <InputText :id="`${id}-title`" v-model="form.title" :placeholder="t(`documents.generate.kind.${form.kind}`)" :maxlength="200" :disabled="busy" />
        </div>
        <div v-field class="field">
          <label :for="`${id}-version`">{{ t('documents.generate.version') }}</label>
          <InputText :id="`${id}-version`" v-model="form.version" :placeholder="t('documents.generate.versionAuto')" :maxlength="40" :disabled="busy" :aria-describedby="`${id}-version-help`" />
          <small :id="`${id}-version-help`" class="muted">{{ t('documents.generate.versionHelp') }}</small>
        </div>
        <div v-field class="field">
          <label :for="`${id}-author`">{{ t('documents.generate.author') }}</label>
          <InputText :id="`${id}-author`" v-model="form.author" :placeholder="auth.user?.display_name || auth.user?.email" :maxlength="200" :disabled="busy" />
        </div>
        <div v-field class="field">
          <label :for="`${id}-lang`">{{ t('documents.generate.language') }}</label>
          <Select v-model="form.lang" :input-id="`${id}-lang`" :options="langOptions" option-label="label" option-value="value" :disabled="busy" />
        </div>
      </div>

      <div class="field">
        <span :id="`${id}-format-label`" class="field-label">{{ t('documents.generate.format') }}</span>
        <SelectButton v-model="form.format" :options="formatOptions" option-label="label" option-value="value" :allow-empty="false" :aria-labelledby="`${id}-format-label`" :disabled="busy">
          <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /> {{ option.label }}</template>
        </SelectButton>
      </div>

      <div v-field class="field">
        <label :for="`${id}-baseline`">{{ t('documents.generate.source') }}</label>
        <Select v-model="form.baselineId" :input-id="`${id}-baseline`" :options="baselineOptions" option-label="label" option-value="value" :disabled="busy" :placeholder="t('documents.generate.currentData')" :aria-describedby="`${id}-baseline-help`" />
        <small :id="`${id}-baseline-help`" class="muted">{{ t('documents.generate.sourceHelp') }}</small>
      </div>

      <div class="field">
        <span class="field-label" aria-hidden="true">{{ isSpec ? t('documents.generate.filterRequirements') : t('documents.generate.filterTests') }}</span>
        <QueryInput :key="form.kind" v-model="form.query" :scope="isSpec ? 'requirement' : 'test'" :error="queryError" :placeholder="t('documents.generate.filterPlaceholder')" @submit="generate" />
      </div>
      <div class="row nowrap">
        <Checkbox v-model="form.includeInactive" :input-id="`${id}-inactive`" binary :disabled="busy" />
        <label :for="`${id}-inactive`">{{ isSpec ? t('documents.generate.inactiveRequirements') : t('documents.generate.inactiveTests') }}</label>
      </div>

      <div v-if="isSpec" class="gd-grid">
        <div v-field class="field">
          <label :for="`${id}-group`">{{ t('documents.generate.groupBy') }}</label>
          <Select v-model="form.groupBy" :input-id="`${id}-group`" :options="groupOptions" option-label="label" option-value="value" :disabled="busy" />
        </div>
        <div v-if="form.groupBy === 'category'" v-field class="field">
          <label :for="`${id}-dim`">{{ t('documents.generate.dimension') }}</label>
          <Select v-model="form.dimension" :input-id="`${id}-dim`" :options="dimensionOptions" option-label="label" option-value="value" :disabled="busy" :placeholder="t('documents.generate.firstDimension')" />
        </div>
      </div>

      <fieldset class="plain-fieldset" :disabled="busy" :aria-describedby="showProblems && problems.includes('fields') ? `${id}-fields-error` : undefined">
        <legend class="field-label">{{ isSpec ? t('documents.generate.fieldsRequirements') : t('documents.generate.fieldsTests') }}</legend>
        <div class="gd-checks">
          <span v-for="f in fieldOptions" :key="f.value" class="row nowrap">
            <Checkbox v-model="form.fields" :input-id="`${id}-f-${f.value}`" :value="f.value" />
            <label :for="`${id}-f-${f.value}`">{{ f.label }}</label>
          </span>
        </div>
      </fieldset>
      <fieldset class="plain-fieldset" :disabled="busy">
        <legend class="field-label">{{ t('documents.generate.appendices') }}</legend>
        <div class="gd-checks">
          <span class="row nowrap">
            <Checkbox v-model="form.traceability" :input-id="`${id}-trace`" binary />
            <label :for="`${id}-trace`">{{ isSpec ? t('documents.generate.traceability') : t('documents.generate.coverage') }}</label>
          </span>
          <span class="row nowrap">
            <Checkbox v-model="form.glossary" :input-id="`${id}-glossary`" binary />
            <label :for="`${id}-glossary`">{{ t('documents.generate.glossary') }}</label>
          </span>
        </div>
      </fieldset>

      <Message v-if="showProblems && problems.length" severity="error" :closable="false">
        <span :id="`${id}-fields-error`" role="alert">{{ problems.map((p) => t(`documents.generate.problem.${p}`)).join(' ') }}</span>
      </Message>
      <Message v-if="otherError" severity="error" :closable="false"><span role="alert">{{ otherError }}</span></Message>
      <div v-if="busy" class="gd-progress" role="status" aria-live="polite">
        <ProgressBar mode="indeterminate" style="height: 6px" :aria-label="t('documents.generate.generating')" />
        <span class="small muted">{{ t('documents.generate.generatingFor', { n: elapsed }, elapsed) }}</span>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" :disabled="busy" @click="visible = false" />
        <Button type="submit" :label="t('documents.generate.create')" icon="pi pi-file-plus" :loading="busy" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.gd-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
}
.gd-checks {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 18px;
  margin-top: 4px;
}
.gd-progress {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.field-label {
  font-weight: 600;
  font-size: 12px;
  color: var(--qtf-muted);
}
</style>
