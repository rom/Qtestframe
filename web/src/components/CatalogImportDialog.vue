<script setup lang="ts">
// Options of a catalogue import (POST /projects/{pid}/catalog/import):
// language, target folders, subfolders, status, the verifying tests and a
// summary of what will be added.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import Message from 'primevue/message'
import { api } from '@/lib/api'
import { importPlan, type CatalogImportResult, type CatalogRequirement } from '@/lib/catalog'
import { buildTree, flatten } from '@/lib/tree'
import type { Folder } from '@/lib/types'
import { enumOptions } from '@/i18n/labels'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const MAX = 500
const visible = defineModel<boolean>('visible', { default: false })
const lang = defineModel<string>('lang', { required: true })
const props = defineProps<{
  projectId: string
  requirementIds: string[]
  testIds: string[]
  reqById: Map<string, CatalogRequirement>
  inReqs: Set<string>
  inTests: Set<string>
  requirementFolders: Folder[]
  testFolders: Folder[]
  sources: string[]
  canTests: boolean
}>()
const emit = defineEmits<{ imported: [res: CatalogImportResult] }>()
const { t } = useI18n()
const meta = useMetaStore()
const notify = useNotify()

// NONE = top level / the cited standards (a Select shows no label for an empty value)
const NONE = '-'
const reqFolder = ref(NONE)
const testFolder = ref(NONE)
const folders = ref<'none' | 'area' | 'group'>('area')
const status = ref('draft')
const testStatus = ref('draft')
const includeTests = ref(true)
const addCopies = ref(false)
const source = ref(NONE)
const busy = ref(false)
watch(visible, (v) => {
  if (v) includeTests.value = props.canTests
})

const folderOptions = (fs: Folder[]) => [
  { label: t('common.topLevel'), value: NONE },
  ...flatten(buildTree(fs)).map((f) => ({ label: `${'  '.repeat(f.depth)}${f.depth ? '└ ' : ''}${f.label}`, value: f.id })),
]
const reqFolderOptions = computed(() => folderOptions(props.requirementFolders))
const testFolderOptions = computed(() => folderOptions(props.testFolders))
const subfolderOptions = computed(() => [
  { label: t('catalog.dialog.foldersNone'), value: 'none' },
  { label: t('catalog.dialog.foldersArea'), value: 'area' },
  { label: t('catalog.dialog.foldersGroup'), value: 'group' },
])
const langOptions = [
  { label: 'English', value: 'en' },
  { label: 'Svenska', value: 'sv' },
]
const statusOptions = computed(() => enumOptions('status', meta.meta.requirement_statuses))
const testStatusOptions = computed(() => enumOptions('status', meta.meta.test_statuses))
const sourceOptions = computed(() => [{ label: t('catalog.dialog.sourceRefs'), value: NONE }, ...props.sources.map((s) => ({ label: s, value: s }))])

const plan = computed(() =>
  importPlan(props.requirementIds, props.canTests ? props.testIds : [], props.reqById, props.inReqs, props.inTests, {
    includeTests: includeTests.value && props.canTests,
    addCopies: addCopies.value,
  }),
)
const total = computed(() => plan.value.requirements.length + plan.value.tests.length)
const skipped = computed(() => plan.value.skippedRequirements.length + plan.value.skippedTests.length)
const hasReqs = computed(() => props.requirementIds.length > 0)
const hasTests = computed(() => plan.value.tests.length > 0 || props.testIds.length > 0)

async function submit() {
  busy.value = true
  try {
    const res = await api.post<CatalogImportResult>(`/projects/${props.projectId}/catalog/import`, {
      requirement_ids: props.requirementIds,
      test_ids: props.canTests ? props.testIds : [],
      lang: lang.value,
      requirement_folder_id: reqFolder.value === NONE ? '' : reqFolder.value,
      test_folder_id: testFolder.value === NONE ? '' : testFolder.value,
      folders: folders.value,
      status: status.value,
      test_status: testStatus.value,
      include_tests: includeTests.value && props.canTests,
      add_copies: addCopies.value,
      ...(source.value !== NONE ? { source: source.value } : {}),
    })
    visible.value = false
    emit('imported', res)
  } catch (e) {
    notify.fail(e, t('catalog.result.failed'))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" :header="t('catalog.dialog.title')" modal class="catalog-import-dialog" style="width: min(560px, 96vw)">
    <form id="catalog-import-form" class="ci-form" @submit.prevent="submit">
      <div class="field">
        <span id="ci-lang" class="label">{{ t('catalog.dialog.language') }}</span>
        <SelectButton v-model="lang" :options="langOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="ci-lang" />
      </div>
      <div v-if="hasReqs" class="field">
        <label for="ci-req-folder">{{ t('catalog.dialog.requirementFolder') }}</label>
        <Select v-model="reqFolder" input-id="ci-req-folder" :aria-label="t('catalog.dialog.requirementFolder')" :options="reqFolderOptions" option-label="label" option-value="value" filter />
      </div>
      <div v-if="hasTests && canTests" class="field">
        <label for="ci-test-folder">{{ t('catalog.dialog.testFolder') }}</label>
        <Select v-model="testFolder" input-id="ci-test-folder" :aria-label="t('catalog.dialog.testFolder')" :options="testFolderOptions" option-label="label" option-value="value" filter />
      </div>
      <div class="field">
        <label for="ci-folders">{{ t('catalog.dialog.folders') }}</label>
        <Select v-model="folders" input-id="ci-folders" :aria-label="t('catalog.dialog.folders')" :options="subfolderOptions" option-label="label" option-value="value" />
      </div>
      <div v-if="hasReqs" class="field">
        <label for="ci-status">{{ t('catalog.dialog.status') }}</label>
        <Select v-model="status" input-id="ci-status" :aria-label="t('catalog.dialog.status')" :options="statusOptions" option-label="label" option-value="value" />
      </div>
      <div v-if="hasTests && canTests" class="field">
        <label for="ci-test-status">{{ t('catalog.dialog.testStatus') }}</label>
        <Select v-model="testStatus" input-id="ci-test-status" :aria-label="t('catalog.dialog.testStatus')" :options="testStatusOptions" option-label="label" option-value="value" />
      </div>
      <div v-if="hasReqs" class="field">
        <label for="ci-source">{{ t('catalog.dialog.source') }}</label>
        <Select v-model="source" input-id="ci-source" :aria-label="t('catalog.dialog.source')" :options="sourceOptions" option-label="label" option-value="value" />
      </div>
      <div v-if="hasReqs && canTests" class="row switch-row">
        <ToggleSwitch v-model="includeTests" input-id="ci-include" />
        <label for="ci-include">{{ t('catalog.dialog.includeTests') }}</label>
      </div>
      <div class="row switch-row">
        <ToggleSwitch v-model="addCopies" input-id="ci-copies" />
        <label for="ci-copies">{{ t('catalog.dialog.addCopies') }}</label>
      </div>
      <Message v-if="hasReqs && !canTests" severity="info" :closable="false">{{ t('catalog.dialog.noTestWrite') }}</Message>

      <section class="summary" aria-labelledby="ci-summary">
        <h3 id="ci-summary">{{ t('catalog.dialog.summary') }}</h3>
        <div role="status" aria-live="polite">
        <ul>
          <li v-if="hasReqs">{{ t('catalog.dialog.willAddRequirements', { n: plan.requirements.length }, plan.requirements.length) }}</li>
          <li>
            {{ t('catalog.dialog.willAddTests', { n: plan.tests.length }, plan.tests.length) }}
            <span v-if="plan.included.length" class="muted"> – {{ t('catalog.dialog.includedTests', { n: plan.included.length }) }}</span>
          </li>
          <li v-if="skipped">{{ t('catalog.dialog.skipped', { n: skipped }) }}</li>
          <li v-if="plan.requirements.length + plan.tests.length && canTests" class="muted">{{ t('catalog.dialog.links') }}</li>
        </ul>
        </div>
        <Message v-if="!total" severity="warn" :closable="false">{{ t('catalog.dialog.nothing') }}</Message>
        <Message v-if="total > MAX" severity="error" :closable="false">{{ t('catalog.dialog.tooMany', { max: MAX }) }}</Message>
      </section>
    </form>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button type="submit" form="catalog-import-form" :label="t('catalog.dialog.add')" icon="pi pi-plus" :loading="busy" :disabled="!total || total > MAX" />
    </template>
  </Dialog>
</template>

<style scoped>
.ci-form { display: grid; gap: 12px; grid-template-columns: minmax(0, 1fr); }
.field { display: grid; gap: 4px; }
.field > label, .field > .label { font-weight: 600; font-size: 13px; }
.switch-row { gap: 8px; align-items: center; }
.summary h3 { font-size: 14px; margin: 4px 0; }
.summary ul { margin: 0 0 6px; padding-left: 18px; }
</style>
