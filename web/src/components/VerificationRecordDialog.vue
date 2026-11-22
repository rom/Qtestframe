<script setup lang="ts">
// Create or change a verification record: evidence other than a test
// result (inspection, analysis, demonstration, review) about a criterion.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ArchivePickerDialog from './ArchivePickerDialog.vue'
import { useI18n } from 'vue-i18n'
import { useEscapeClose } from '@/composables/escapeClose'
import { api, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { enumOptions } from '@/i18n/labels'
import { RECORD_RESULTS, recordProblems, today } from '@/lib/verification'
import type { EvaluationCriterion, VerificationRecord } from '@/lib/types'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{
  projectId: string
  requirementId: string
  criteria: EvaluationCriterion[]
  /** the record to change (null: a new one) */
  record: VerificationRecord | null
  /** preselected criterion of a new record */
  criterionId?: string | null
}>()
const emit = defineEmits<{ saved: [record: VerificationRecord] }>()
const { t } = useI18n()
useEscapeClose(visible)
const id = uid('vrec')
const METHODS = ['inspection', 'analysis', 'demonstration', 'review', 'test']

interface Form {
  criterion_id: string
  method: string
  result: string
  performed_by: string
  performed_on: string
  valid_until: string
  until_change: boolean
  note: string
  file_ids: string[]
  files: { id: string; name: string }[]
}
const form = ref<Form>(blank())
const error = ref('')
const saving = ref(false)
const picking = ref(false)

function blank(): Form {
  return { criterion_id: '', method: '', result: 'pass', performed_by: '', performed_on: today(), valid_until: '', until_change: true, note: '', file_ids: [], files: [] }
}
watch(visible, (v) => {
  if (!v) return
  error.value = ''
  const r = props.record
  if (r) {
    form.value = {
      criterion_id: r.criterion_id ?? '',
      method: r.method,
      result: r.result,
      performed_by: r.performed_by,
      performed_on: r.performed_on,
      valid_until: r.valid_until,
      until_change: r.until_change,
      note: r.note,
      file_ids: [...r.file_ids],
      files: r.files.map((f) => ({ id: f.id, name: f.name })),
    }
  } else {
    form.value = blank()
    form.value.criterion_id = props.criterionId ?? ''
    onCriterion()
  }
})
function onCriterion() {
  const c = props.criteria.find((x) => x.id === form.value.criterion_id)
  if (c && c.method !== 'test') form.value.method = c.method
  else if (!form.value.method) form.value.method = 'inspection'
}

const criterionOptions = computed(() => [
  { label: t('verification.wholeRequirement'), value: '' },
  ...props.criteria.map((c, i) => ({ label: `${i + 1}. ${c.description}`, value: c.id })),
])
const methodOptions = computed(() => enumOptions('evaluationMethod', METHODS))
const resultOptions = computed(() => RECORD_RESULTS.map((r) => ({ label: t(`verification.result.${r}`), value: r })))
const problems = computed(() => recordProblems(form.value))

function addFiles(ids: string[], names?: Map<string, string>) {
  for (const fid of ids)
    if (!form.value.file_ids.includes(fid)) {
      form.value.file_ids.push(fid)
      form.value.files.push({ id: fid, name: names?.get(fid) ?? fid })
    }
}
async function onPick(ids: string[]) {
  const names = new Map<string, string>()
  await Promise.all(
    ids.map(async (fid) => {
      try {
        const f = await api.get<{ name: string }>(`/files/${fid}`)
        names.set(fid, f.name)
      } catch {
        /* the id is shown instead */
      }
    }),
  )
  addFiles(ids, names)
}
function removeFile(fid: string) {
  form.value.file_ids = form.value.file_ids.filter((x) => x !== fid)
  form.value.files = form.value.files.filter((x) => x.id !== fid)
}

async function save() {
  if (problems.value.length) return
  saving.value = true
  error.value = ''
  const f = form.value
  const body = {
    criterion_id: f.criterion_id || null,
    method: f.method,
    result: f.result,
    performed_by: f.performed_by.trim(),
    performed_on: f.performed_on,
    valid_until: f.valid_until,
    until_change: f.until_change,
    note: f.note,
    file_ids: f.file_ids,
  }
  try {
    const r = props.record
      ? await api.patch<VerificationRecord>(`/verification-records/${props.record.id}`, body)
      : await api.post<VerificationRecord>(`/requirements/${props.requirementId}/verification-records`, body)
    emit('saved', r)
    visible.value = false
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    :close-on-escape="false"
    modal
    :header="record ? t('verification.editRecord') : t('verification.addRecord')"
    :style="{ width: 'min(640px, calc(100vw - 16px))' }"
    :breakpoints="{ '640px': '100vw' }"
  >
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <p class="small muted" style="margin: 0">{{ t('verification.recordHint') }}</p>
      <div class="field">
        <label :for="`${id}-crit`">{{ t('verification.criterion') }}</label>
        <Select v-model="form.criterion_id" :input-id="`${id}-crit`" :options="criterionOptions" option-label="label" option-value="value" @change="onCriterion" />
      </div>
      <div class="form-grid">
        <div class="field">
          <label :for="`${id}-method`">{{ t('verification.method') }}</label>
          <Select v-model="form.method" :input-id="`${id}-method`" :options="methodOptions" option-label="label" option-value="value" :invalid="problems.includes('method')" />
        </div>
        <div class="field">
          <label :for="`${id}-result`">{{ t('verification.resultLabel') }}</label>
          <Select v-model="form.result" :input-id="`${id}-result`" :options="resultOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label :for="`${id}-by`">{{ t('verification.performedBy') }}</label>
          <InputText :id="`${id}-by`" v-model="form.performed_by" maxlength="200" :placeholder="t('verification.performedByPlaceholder')" />
        </div>
        <div class="field">
          <label :for="`${id}-on`">{{ t('verification.performedOn') }}</label>
          <InputText :id="`${id}-on`" v-model="form.performed_on" type="date" :invalid="problems.includes('date')" />
        </div>
        <div class="field">
          <label :for="`${id}-until`">{{ t('verification.validUntil') }}</label>
          <InputText :id="`${id}-until`" v-model="form.valid_until" type="date" :invalid="problems.includes('validUntil')" :aria-describedby="`${id}-until-hint`" />
          <small :id="`${id}-until-hint`" class="muted">{{ t('verification.validUntilHint') }}</small>
        </div>
        <div class="field row nowrap" style="align-self: end; gap: 8px">
          <Checkbox v-model="form.until_change" binary :input-id="`${id}-change`" />
          <label :for="`${id}-change`">{{ t('verification.untilChange') }}</label>
        </div>
      </div>
      <div class="field">
        <label :for="`${id}-note`">{{ t('verification.note') }}</label>
        <Textarea :id="`${id}-note`" v-model="form.note" rows="3" auto-resize maxlength="20000" />
      </div>
      <div class="field">
        <span class="field-label">{{ t('verification.documents') }}</span>
        <div class="row" style="gap: 6px; flex-wrap: wrap">
          <Tag v-for="f in form.files" :key="f.id" severity="secondary" class="row nowrap" style="gap: 4px">
            <i class="pi pi-file" aria-hidden="true" /><span>{{ f.name }}</span>
            <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('verification.removeDocument', { name: f.name })" @click="removeFile(f.id)" />
          </Tag>
          <Button size="small" text icon="pi pi-folder-open" :label="t('verification.addDocuments')" @click="picking = true" />
        </div>
      </div>
      <ul v-if="problems.length" class="small plain-list" role="alert">
        <li v-for="p in problems" :key="p" class="warn-text">{{ t(`verification.problem.${p}`) }}</li>
      </ul>
      <div class="row" style="justify-content: flex-end; gap: 8px">
        <Button :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" :disabled="problems.length > 0" />
      </div>
    </form>
    <ArchivePickerDialog v-model:visible="picking" :project-id="projectId" :exclude="form.file_ids" @pick="onPick" />
  </Dialog>
</template>
