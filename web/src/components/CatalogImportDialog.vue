<script setup lang="ts">
// Options of a catalogue import (POST /projects/{pid}/catalog/import):
// language, target folders, subfolders, status, the verifying tests, the
// risks and their mitigating requirements, and a summary of what will be
// added, with a warning about catalogue requirements similar to
// requirements the project already has (POST /projects/{pid}/catalog/similar)
// and the choice to leave them out.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import Message from 'primevue/message'
import { api } from '@/lib/api'
import { importPlan, type CatalogImportResult, type CatalogRequirement, type CatalogRisk, type CatalogSimilar } from '@/lib/catalog'
import { percent } from '@/lib/similarity'
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
  riskIds?: string[]
  riskById?: Map<string, CatalogRisk>
  inRisks?: Set<string>
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
const includeRequirements = ref(true)
const includeRisks = ref(false)
const riskStatus = ref('identified')
const source = ref(NONE)
const busy = ref(false)
const skipSimilar = ref(false)
const similar = ref<CatalogSimilar[]>([])
let similarSeq = 0
async function loadSimilar() {
  const n = ++similarSeq
  if (!visible.value || !props.requirementIds.length) {
    similar.value = []
    return
  }
  try {
    const res = await api.post<{ items: CatalogSimilar[] }>(`/projects/${props.projectId}/catalog/similar`, { requirement_ids: props.requirementIds, lang: lang.value })
    if (n === similarSeq) similar.value = res.items ?? []
  } catch {
    if (n === similarSeq) similar.value = [] // a warning only
  }
}
watch(visible, (v) => {
  if (v) includeTests.value = props.canTests
})
watch([visible, lang, () => props.requirementIds], () => void loadSimilar(), { immediate: true })

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
const riskStatusOptions = computed(() => ['identified', 'mitigating'].map((v) => ({ label: t(`risksView.status.${v}`), value: v })))
const sourceOptions = computed(() => [{ label: t('catalog.dialog.sourceRefs'), value: NONE }, ...props.sources.map((s) => ({ label: s, value: s }))])

const plan = computed(() =>
  importPlan(props.requirementIds, props.canTests ? props.testIds : [], props.reqById, props.inReqs, props.inTests, {
    includeTests: includeTests.value && props.canTests,
    addCopies: addCopies.value,
    riskIds: props.riskIds ?? [],
    riskById: props.riskById,
    inRisks: props.inRisks,
    includeRequirements: includeRequirements.value,
    includeRisks: includeRisks.value,
  }),
)
const total = computed(() => plan.value.requirements.length + plan.value.tests.length + plan.value.risks.length)
const skipped = computed(() => plan.value.skippedRequirements.length + plan.value.skippedTests.length + plan.value.skippedRisks.length)
const hasRisks = computed(() => (props.riskIds ?? []).length > 0)
const hasReqs = computed(() => props.requirementIds.length > 0 || plan.value.requirements.length > 0)
/** risks the chosen requirements mitigate (offered with "also add the risks") */
const mitigatedRisks = computed(() => {
  const chosen = new Set(props.riskIds ?? [])
  const out = new Set<string>()
  for (const id of props.requirementIds) for (const k of props.reqById.get(id)?.risks ?? []) if (!chosen.has(k)) out.add(k)
  return out.size
})
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
      ...(hasRisks.value ? { risk_ids: props.riskIds, include_requirements: includeRequirements.value, risk_status: riskStatus.value } : {}),
      ...(includeRisks.value && mitigatedRisks.value ? { include_risks: true } : {}),
      skip_similar: skipSimilar.value && similar.value.length > 0,
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
      <div v-if="hasRisks" class="field">
        <label for="ci-risk-status">{{ t('catalog.dialog.riskStatus') }}</label>
        <Select v-model="riskStatus" input-id="ci-risk-status" :aria-label="t('catalog.dialog.riskStatus')" :options="riskStatusOptions" option-label="label" option-value="value" />
      </div>
      <div v-if="hasRisks" class="row switch-row">
        <ToggleSwitch v-model="includeRequirements" input-id="ci-include-reqs" />
        <label for="ci-include-reqs">{{ t('catalog.dialog.includeRequirements') }}</label>
      </div>
      <div v-if="mitigatedRisks" class="row switch-row">
        <ToggleSwitch v-model="includeRisks" input-id="ci-include-risks" />
        <label for="ci-include-risks">{{ t('catalog.dialog.includeRisks', { n: mitigatedRisks }, mitigatedRisks) }}</label>
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
          <li v-if="plan.risks.length || hasRisks">
            {{ t('catalog.dialog.willAddRisks', { n: plan.risks.length }, plan.risks.length) }}
            <span v-if="plan.includedRisks.length" class="muted"> – {{ t('catalog.dialog.includedRisks', { n: plan.includedRisks.length }) }}</span>
          </li>
          <li v-if="hasReqs">
            {{ t('catalog.dialog.willAddRequirements', { n: plan.requirements.length }, plan.requirements.length) }}
            <span v-if="plan.includedRequirements.length" class="muted"> – {{ t('catalog.dialog.includedRequirements', { n: plan.includedRequirements.length }) }}</span>
          </li>
          <li>
            {{ t('catalog.dialog.willAddTests', { n: plan.tests.length }, plan.tests.length) }}
            <span v-if="plan.included.length" class="muted"> – {{ t('catalog.dialog.includedTests', { n: plan.included.length }) }}</span>
          </li>
          <li v-if="skipped">{{ t('catalog.dialog.skipped', { n: skipped }) }}</li>
          <li v-if="plan.requirements.length + plan.tests.length && canTests" class="muted">{{ t('catalog.dialog.links') }}</li>
          <li v-if="plan.risks.length || (plan.requirements.length && hasRisks)" class="muted">{{ t('catalog.dialog.riskLinks') }}</li>
        </ul>
        </div>
        <div v-if="similar.length" class="ci-similar" role="group" aria-labelledby="ci-similar-h">
          <Message severity="warn" :closable="false">
            <span id="ci-similar-h">{{ t('catalog.dialog.similar', { n: similar.length }, similar.length) }}</span>
          </Message>
          <ul class="ci-similar-list">
            <li v-for="s in similar.slice(0, 8)" :key="s.catalog_ref">
              <span class="mono">{{ s.catalog_ref }}</span> {{ s.title }}
              <span class="muted">→ {{ s.similar.map((x) => `${x.key} (${t('similar.score', { pct: percent(x.score) })})`).join(', ') }}</span>
            </li>
            <li v-if="similar.length > 8" class="muted">{{ t('traceRules.more', { n: similar.length - 8 }) }}</li>
          </ul>
          <div class="row switch-row">
            <ToggleSwitch v-model="skipSimilar" input-id="ci-skip-similar" />
            <label for="ci-skip-similar">{{ t('catalog.dialog.skipSimilar') }}</label>
          </div>
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
.ci-similar { display: grid; gap: 4px; }
.ci-similar-list { font-size: 13px; overflow-wrap: anywhere; }
</style>
