<script setup lang="ts">
// Import an edited Excel round-trip workbook (requirements or tests) or a
// ReqIF document (DOORS, DOORS Next, Polarion, Qtestframe), with a diff
// preview: new, changed, unchanged, conflicting and erroneous rows, cell
// by cell. A ReqIF document can hold requirements and test cases (by
// object type, adjustable), relations and trace links (by relation type,
// adjustable) and embedded files. In the source modes (jira, azure-devops)
// a query instead of a file picks the issues or work items of Jira (JQL)
// or Azure DevOps (WIQL, or area path and work item types). Nothing is
// saved before Save; the save is one transaction, after the impact gate.
import { computed, nextTick, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import FileDropZone from './FileDropZone.vue'
import FolderPickerDialog from './FolderPickerDialog.vue'
import ExchangeDiffTable from './ExchangeDiffTable.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { upload } from '@/lib/upload'
import { api, errorMessage } from '@/lib/api'
import { folderPath } from '@/lib/tree'
import { uid } from '@/lib/a11y'
import { useProjectStore } from '@/stores/project'
import type { Folder, ImportSources } from '@/lib/types'
import { externalImportBody, IMPORT_SOURCES, sourceConfigured, type ImportSourceId } from '@/lib/importSources'
import { impactGateFor } from '@/composables/impact'
import type { ImpactDryRun } from '@/lib/correlations'
import {
  acceptsFile,
  changedAttrMapping,
  changedTypeMapping,
  clip,
  EXCHANGE_ACCEPT,
  EXCHANGE_MAX_MB,
  exchangeFields,
  REQIF_OBJECT_KINDS,
  saveCount,
  type ExchangeDiff,
  type ExchangeKind,
  type ExchangeMode,
  type ExchangeResult,
  type ReqIFAttribute,
  type ReqIFPreview,
  type ReqIFTarget,
} from '@/lib/exchange'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ projectId: string; kind: ExchangeKind; mode: ExchangeMode | ImportSourceId; folders: Folder[]; folderId?: string | null; canExport?: boolean }>()
const emit = defineEmits<{ imported: [result: ExchangeResult] }>()
const { t, te } = useI18n()
const proj = useProjectStore()

const file = ref<File | null>(null)
const preview = ref<ExchangeDiff | ReqIFPreview | null>(null)
const result = ref<ExchangeResult | null>(null)
const busy = ref(false)
const error = ref('')
const downloading = ref(false)
const choices = ref<Record<string, string>>({})
const testChoices = ref<Record<string, string>>({})
const typeChoices = ref<Record<string, string>>({})
const relChoices = ref<Record<string, string>>({})
const folderId = ref<string | null>(null)
const testFolderId = ref<string | null>(null)
const hierarchy = ref(true)
const refines = ref(false)
const overwrite = ref(false)
const create = ref(true)
const excluded = ref(new Set<number>())
const excludedTests = ref(new Set<number>())
const pickFolder = ref<'' | 'requirement' | 'test'>('')
const ids = {
  mapping: uid('ex-map'),
  testMapping: uid('ex-tmap'),
  types: uid('ex-types'),
  relTypes: uid('ex-rtypes'),
  preview: uid('ex-prev'),
  testPreview: uid('ex-tprev'),
  status: uid('ex-status'),
  rels: uid('ex-rels'),
  hier: uid('ex-hier'),
  refines: uid('ex-ref'),
  query: uid('ex-query'),
}

/** an import from Jira or Azure DevOps (no file: a query) */
const source = computed<ImportSourceId | null>(() => ((IMPORT_SOURCES as readonly string[]).includes(props.mode) ? (props.mode as ImportSourceId) : null))
const isReqIF = computed(() => props.mode === 'reqif')
const fileMode = computed<ExchangeMode>(() => (isReqIF.value ? 'reqif' : 'xlsx'))
/** a preview with attributes and relations: ReqIF or a source */
const reqif = computed(() => (isReqIF.value || source.value ? (preview.value as ReqIFPreview | null) : null))
const sourceName = computed(() => (source.value === 'jira' ? 'Jira' : 'Azure DevOps'))
const sources = ref<ImportSources | null>(null)
const sourcesError = ref('')
const query = ref('')
const areaPath = ref('')
const types = ref('')
const relations = ref(true)
const ready = computed(() => !!source.value && sourceConfigured(sources.value, source.value))
const testFolders = computed(() => proj.folders.test ?? [])

watch(visible, (v) => {
  if (!v) return
  file.value = null
  preview.value = null
  result.value = null
  error.value = ''
  choices.value = {}
  testChoices.value = {}
  typeChoices.value = {}
  relChoices.value = {}
  folderId.value = props.folderId ?? null
  testFolderId.value = null
  hierarchy.value = true
  refines.value = false
  overwrite.value = false
  create.value = true
  excluded.value = new Set()
  excludedTests.value = new Set()
  relations.value = true
  if (source.value) void loadSources()
})

async function loadSources() {
  sources.value = null
  sourcesError.value = ''
  try {
    const s = await api.get<ImportSources>(`/projects/${props.projectId}/import-sources`)
    sources.value = s
    query.value = source.value === 'jira' ? s.jira.jql || (s.jira_from_tracker && s.tracker_jira_key ? `project = ${s.tracker_jira_key} ORDER BY key` : '') : ''
    areaPath.value = s.azure_devops.area_path
    types.value = s.azure_devops.work_item_types
  } catch (e) {
    sourcesError.value = errorMessage(e)
  }
}

const title = computed(() => {
  if (source.value) return t('exchange.sourceTitle', { source: sourceName.value })
  if (isReqIF.value) return t('exchange.reqifTitle')
  return props.kind === 'test' ? t('exchange.xlsxTitleTests') : t('exchange.xlsxTitleRequirements')
})
function targetOptions(targets: readonly ReqIFTarget[] | null | undefined) {
  return [{ label: t('exchange.ignore'), value: '' }, ...(targets ?? []).map((x) => ({ label: x.type === 'role' ? t(`exchange.role.${x.key}`) : x.label, value: x.key }))]
}
const reqTargetOptions = computed(() => targetOptions(reqif.value?.targets))
const testTargetOptions = computed(() => targetOptions(reqif.value?.test_targets))
const kindOptions = computed(() =>
  REQIF_OBJECT_KINDS.map((k) => ({ label: t(`exchange.objectKind.${k}`), value: k, disabled: k === 'test' && !reqif.value?.can_import_tests })),
)
function relLabel(code: string) {
  if (code === 'verifies' || code === 'ignore') return t(`exchange.relTarget.${code}`)
  return te(`traceRules.relationType.${code}`) ? t(`traceRules.relationType.${code}`) : code
}
const relOptions = computed(() => (reqif.value?.relation_targets ?? []).map((v) => ({ label: relLabel(v), value: v })))
const rootPath = computed(() => (folderId.value ? folderPath(props.folders, folderId.value) : ''))
const testRootPath = computed(() => (testFolderId.value ? folderPath(testFolders.value, testFolderId.value) : ''))
const opts = computed(() => ({ overwrite: overwrite.value, create: create.value }))
const tests = computed(() => reqif.value?.tests ?? null)
const summaryConflicts = computed(() => (preview.value?.summary.conflicts ?? 0) + (tests.value?.summary.conflicts ?? 0))
const summaryNew = computed(() => (preview.value?.summary.new ?? 0) + (tests.value?.summary.new ?? 0))
const toSave = computed(() => {
  if (!preview.value) return 0
  return saveCount(preview.value.rows, opts.value, excluded.value) + (tests.value ? saveCount(tests.value.rows, opts.value, excludedTests.value) : 0)
})
const relationChanges = computed(() => (reqif.value ? reqif.value.relations.changes.filter((c) => c.action !== 'keep') : []))
const newFiles = computed(() => (reqif.value?.files ?? []).filter((f) => !f.existing).length)
const showTypes = computed(() => !source.value && (reqif.value?.object_types.length ?? 0) > 0)

function sourceBody(apply = false) {
  const mapping = reqif.value ? changedAttrMapping(reqif.value.attributes, choices.value) : undefined
  const refs = (preview.value?.rows ?? []).filter((r) => excluded.value.has(r.index) && r.ref).map((r) => r.ref as string)
  return externalImportBody(
    source.value as ImportSourceId,
    { query: query.value, areaPath: areaPath.value, types: types.value, relations: relations.value },
    { mapping, folderId: folderId.value },
    apply ? { overwrite: overwrite.value, create: create.value, excludedRefs: refs } : undefined,
  )
}

function fields(apply = false) {
  const r = reqif.value
  return exchangeFields(
    fileMode.value,
    {
      mapping: r ? changedAttrMapping(r.attributes, choices.value) : undefined,
      testMapping: r ? changedAttrMapping(r.test_attributes ?? [], testChoices.value) : undefined,
      objectTypes: r ? changedTypeMapping(r.object_types ?? [], typeChoices.value) : undefined,
      relationTypes: r ? changedTypeMapping((r.relation_type_mapping ?? []).map((x) => ({ name: x.name, guess: x.guess })), relChoices.value) : undefined,
      folderId: folderId.value,
      testFolderId: testFolderId.value,
      hierarchy: hierarchy.value,
      refines: refines.value,
      kind: props.kind,
    },
    apply ? { overwrite: overwrite.value, create: create.value, excluded: excluded.value, excludedTests: excludedTests.value } : undefined,
  )
}

function fieldsOf(attrs: readonly ReqIFAttribute[] | null | undefined) {
  const next: Record<string, string> = {}
  for (const a of attrs ?? []) next[a.name] = a.field || ''
  return next
}

async function runPreview() {
  if (!file.value && !source.value) return
  busy.value = true
  error.value = ''
  result.value = null
  try {
    const res: ExchangeDiff | ReqIFPreview = source.value
      ? await api.post<ReqIFPreview>(`/projects/${props.projectId}/requirements/import/${source.value}/preview`, sourceBody())
      : await upload<ExchangeDiff | ReqIFPreview>(`/projects/${props.projectId}/exchange/${props.mode}/preview`, file.value as File, { fields: fields() })
    preview.value = res
    if ('attributes' in res) {
      const rp = res as ReqIFPreview
      choices.value = fieldsOf(rp.attributes)
      testChoices.value = fieldsOf(rp.test_attributes)
      typeChoices.value = Object.fromEntries((rp.object_types ?? []).map((x) => [x.name, x.kind]))
      relChoices.value = Object.fromEntries((rp.relation_type_mapping ?? []).map((x) => [x.name, x.type]))
      if (rp.tests && !proj.folders.test?.length) void proj.loadFolders('test')
    }
    excluded.value = new Set()
    excludedTests.value = new Set()
  } catch (e) {
    error.value = errorMessage(e)
    preview.value = null
  } finally {
    busy.value = false
  }
}

function onFiles(files: File[]) {
  const f = files[0]
  if (!f) return
  result.value = null
  if (!acceptsFile(fileMode.value, f.name)) {
    error.value = t(isReqIF.value ? 'exchange.unsupportedReqif' : 'exchange.unsupportedXlsx', { name: f.name })
    return
  }
  file.value = f
  preview.value = null
  choices.value = {}
  testChoices.value = {}
  typeChoices.value = {}
  relChoices.value = {}
  void runPreview()
}

const mappings = { attr: choices, test: testChoices, type: typeChoices, rel: relChoices }

function choose(which: keyof typeof mappings, name: string, value: string) {
  const target = mappings[which]
  target.value = { ...target.value, [name]: value }
  void runPreview()
}

function onFolder(id: string | null) {
  if (pickFolder.value === 'test') testFolderId.value = id
  else folderId.value = id
  if (!source.value || preview.value) void runPreview()
}

function toggle(which: 'req' | 'test', index: number, on: boolean) {
  const set = which === 'test' ? excludedTests : excluded
  const s = new Set(set.value)
  if (on) s.delete(index)
  else s.add(index)
  set.value = s
}

async function exportWorkbook() {
  downloading.value = true
  error.value = ''
  try {
    await api.download(`/projects/${props.projectId}/exchange/xlsx`, { kind: props.kind }, `${props.kind}s-roundtrip.xlsx`)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    downloading.value = false
  }
}

async function save() {
  if ((!file.value && !source.value) || !preview.value || !toSave.value) return
  busy.value = true
  error.value = ''
  try {
    // the impact of the updates first (impact gate)
    let res: ExchangeResult
    if (source.value) {
      const path = `/projects/${props.projectId}/requirements/import/${source.value}/apply`
      const body = sourceBody(true)
      const gate = await impactGateFor(props.projectId, (q) => api.post<ImpactDryRun>(`${path}?dry_run=${q.dry_run}`, body))
      if (!gate) return
      res = await api.post<ExchangeResult>(`${path}?impact=${gate.impact}`, body)
    } else {
      const path = `/projects/${props.projectId}/exchange/${props.mode}/apply`
      const f = file.value as File
      const gate = await impactGateFor(props.projectId, (q) => upload<ImpactDryRun>(`${path}?dry_run=${q.dry_run}`, f, { fields: fields(true) }))
      if (!gate) return
      res = await upload<ExchangeResult>(`${path}?impact=${gate.impact}`, f, { fields: fields(true) })
    }
    result.value = res
    emit('imported', res)
    await nextTick()
    document.getElementById(ids.status)?.focus()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    :header="title"
    modal
    maximizable
    :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }"
    :style="{ width: 'min(1100px, calc(100vw - 16px))' }"
    :breakpoints="{ '640px': '100vw' }"
    :close-on-escape="!busy"
    class="exchange-dialog"
  >
    <div class="grid" style="gap: 14px" :aria-busy="busy">
      <div class="row small muted" style="align-items: flex-start">
        <p class="grow" style="margin: 0">{{ source ? t('exchange.sourceIntro', { source: sourceName }) : isReqIF ? t('exchange.reqifIntro') : t('exchange.xlsxIntro') }}</p>
        <HelpLink v-if="source" page="reports-and-export" :anchor="{ en: 'jira-and-azure-devops', sv: 'jira-och-azure-devops' }" />
        <HelpLink v-else-if="isReqIF" page="interoperability" :anchor="{ en: 'reqif-import', sv: 'reqif-import' }" />
        <HelpLink v-else page="reports-and-export" :anchor="{ en: 'exchanging-data-with-other-tools', sv: 'utbyta-data-med-andra-verktyg' }" />
      </div>
      <div v-if="!isReqIF && !source && canExport && !preview && !result" class="row">
        <Button :label="t('exchange.downloadWorkbook')" icon="pi pi-download" severity="secondary" outlined :loading="downloading" @click="exportWorkbook" />
      </div>

      <template v-if="source">
        <Message v-if="sourcesError" severity="error" :closable="false" role="alert">{{ sourcesError }}</Message>
        <Message v-else-if="sources && !ready" severity="warn" :closable="false">{{ t('exchange.sourceMissing', { source: sourceName }) }}</Message>
        <form v-else-if="sources" class="grid" style="gap: 10px" :aria-labelledby="ids.query" @submit.prevent="runPreview">
          <h3 :id="ids.query" class="section-title" style="margin: 0">{{ t('exchange.sourceQuery') }}</h3>
          <div v-if="source === 'azure-devops'" class="form-grid">
            <div v-field class="field">
              <label>{{ t('importSources.areaPath') }}</label>
              <InputText v-model="areaPath" class="mono" :disabled="busy || !!query.trim()" placeholder="Shop\Web" />
            </div>
            <div v-field class="field">
              <label>{{ t('importSources.workItemTypes') }}</label>
              <InputText v-model="types" :disabled="busy || !!query.trim()" placeholder="User Story, Requirement" />
            </div>
          </div>
          <div v-field class="field">
            <label>{{ source === 'jira' ? t('exchange.jql') : t('exchange.wiql') }}</label>
            <Textarea v-model="query" rows="2" auto-resize class="mono" :disabled="busy" :aria-describedby="ids.query + '-hint'" />
            <span :id="ids.query + '-hint'" class="small muted">{{ source === 'jira' ? t('exchange.jqlHint') : t('exchange.wiqlHint') }}</span>
          </div>
          <div class="form-grid">
            <div class="field">
              <span :id="ids.query + '-target'" class="label">{{ t('exchange.target') }}</span>
              <Button
                :label="rootPath || t('common.topLevel')"
                icon="pi pi-folder"
                severity="secondary"
                outlined
                class="ellipsis"
                :aria-describedby="ids.query + '-target'"
                aria-haspopup="dialog"
                :disabled="busy"
                @click="pickFolder = 'requirement'"
              />
            </div>
            <div class="field">
              <span class="row nowrap"><Checkbox v-model="relations" binary input-id="ex-rels-on" :disabled="busy" /><label for="ex-rels-on">{{ t('exchange.importLinks') }}</label></span>
            </div>
          </div>
          <div class="row">
            <Button type="submit" :label="preview ? t('exchange.previewAgain') : t('exchange.previewButton')" icon="pi pi-search" :loading="busy" :disabled="busy || (source === 'jira' && !query.trim())" />
            <span v-if="reqif?.tool" class="small muted ellipsis">{{ reqif.tool }}</span>
          </div>
        </form>
        <span v-else class="small muted"><i class="pi pi-spin pi-spinner" aria-hidden="true" /> {{ t('exchange.reading') }}</span>
      </template>
      <FileDropZone
        v-else
        :multiple="false"
        :max-mb="EXCHANGE_MAX_MB"
        :accept="EXCHANGE_ACCEPT[fileMode]"
        :label="file ? t('exchange.otherFile') : t('exchange.chooseFile')"
        :disabled="busy"
        @files="onFiles"
      />
      <div v-if="file" class="row small" aria-live="polite">
        <i class="pi pi-file" aria-hidden="true" />
        <b class="ellipsis">{{ file.name }}</b>
        <span v-if="reqif" class="muted">{{ [reqif.title, reqif.tool].filter(Boolean).join(' · ') }}</span>
        <span v-if="busy" class="muted"><i class="pi pi-spin pi-spinner" aria-hidden="true" /> {{ t('exchange.reading') }}</span>
      </div>
      <Message v-if="error" severity="error" :closable="false" role="alert">{{ error }}</Message>

      <template v-if="result">
        <div :id="ids.status" tabindex="-1" role="status">
          <Message severity="success" :closable="false">
            {{ t('exchange.done', { created: result.created, updated: result.updated, unchanged: result.unchanged, skipped: result.skipped }) }}
            <template v-if="result.tests"> {{ t('exchange.doneTests', { created: result.tests.created, updated: result.tests.updated }) }}</template>
            <template v-if="result.folders_created"> {{ t('exchange.doneFolders', { count: result.folders_created }) }}</template>
            <template v-if="result.relations_added || result.relations_removed"> {{ t('exchange.doneRelations', { added: result.relations_added, removed: result.relations_removed }) }}</template>
            <template v-if="result.links_added || result.links_removed"> {{ t('exchange.doneLinks', { added: result.links_added, removed: result.links_removed }) }}</template>
            <template v-if="result.files_attached"> {{ t('exchange.doneFiles', { count: result.files_attached }) }}</template>
          </Message>
        </div>
        <ul v-if="result.items.length" class="ex-list small">
          <li v-for="(it, i) in result.items.slice(0, 50)" :key="i + it.id">
            <span class="key">{{ it.key }}</span> {{ it.title }} — {{ t(`exchange.action.${it.action}`) }}
          </li>
          <li v-if="result.items.length > 50" class="muted">{{ t('exchange.andMore', { count: result.items.length - 50 }) }}</li>
        </ul>
        <details v-if="result.skipped_rows.length" class="small">
          <summary>{{ t('exchange.skippedList', { count: result.skipped_rows.length }) }}</summary>
          <ul class="ex-list">
            <li v-for="(s, i) in result.skipped_rows" :key="i">{{ s.location }}: {{ s.title }} — {{ t(`exchange.reason.${s.reason}`) }}</li>
          </ul>
        </details>
      </template>

      <template v-else-if="preview">
        <Message v-for="(w, i) in preview.warnings" :key="'w' + i" severity="warn" :closable="false">{{ w }}</Message>
        <p v-if="preview.ignored.length" class="small muted" style="margin: 0">{{ t('exchange.ignoredColumns', { columns: preview.ignored.join(', ') }) }}</p>
        <p v-if="reqif && reqif.previous" class="small" style="margin: 0"><i class="pi pi-replay" aria-hidden="true" /> {{ t(source ? 'exchange.reimportSource' : 'exchange.reimport', { count: reqif.previous, source: sourceName }) }}</p>
        <p v-if="reqif && reqif.files.length" class="small" style="margin: 0">
          <i class="pi pi-paperclip" aria-hidden="true" /> {{ t('exchange.files', { count: reqif.files.length, attached: reqif.files.length - newFiles }) }}
        </p>
        <details v-if="reqif && (reqif.errors.length || reqif.skipped.length)" class="small">
          <summary>{{ t('exchange.problems', { count: reqif.errors.length + reqif.skipped.length }) }}</summary>
          <ul class="ex-list">
            <li v-for="(e, i) in [...reqif.errors, ...reqif.skipped]" :key="i">{{ e }}</li>
          </ul>
        </details>

        <section v-if="reqif && showTypes" :aria-labelledby="ids.types">
          <h3 :id="ids.types" class="section-title">{{ t('exchange.objectTypes') }}</h3>
          <p class="small muted" style="margin: 0 0 8px">{{ t('exchange.objectTypesHint') }}</p>
          <div class="ex-attrs">
            <div v-for="x in reqif.object_types" :key="x.name" v-field class="field">
              <label class="ellipsis" :title="x.name">{{ x.name || t('exchange.untyped') }} <span class="muted small">({{ x.objects }})</span></label>
              <Select
                :model-value="typeChoices[x.name] ?? x.kind"
                :options="kindOptions"
                option-label="label"
                option-value="value"
                option-disabled="disabled"
                :disabled="busy"
                @update:model-value="(v: string) => choose('type', x.name, v)"
              />
            </div>
          </div>
        </section>

        <section v-if="reqif" :aria-labelledby="ids.mapping">
          <h3 :id="ids.mapping" class="section-title">{{ source ? t('exchange.sourceFields') : reqif.tests ? t('exchange.attributesRequirements') : t('exchange.attributes') }}</h3>
          <p class="small muted" style="margin: 0 0 8px">{{ source ? t('exchange.sourceFieldsHint') : t('exchange.attributesHint') }}</p>
          <div class="ex-attrs">
            <div v-for="a in reqif.attributes" :key="a.name" v-field class="field">
              <label class="ellipsis" :title="a.name">{{ a.name }} <span class="muted small">({{ a.type }})</span></label>
              <Select
                :model-value="choices[a.name] ?? ''"
                :options="reqTargetOptions"
                option-label="label"
                option-value="value"
                :disabled="busy"
                @update:model-value="(v: string) => choose('attr', a.name, v)"
              />
              <span class="small muted ellipsis" :title="a.sample">{{ a.sample ? clip(a.sample, 60) : t('exchange.noValues') }}</span>
            </div>
          </div>
        </section>

        <section v-if="reqif && reqif.tests && reqif.test_attributes.length" :aria-labelledby="ids.testMapping">
          <h3 :id="ids.testMapping" class="section-title">{{ t('exchange.attributesTests') }}</h3>
          <div class="ex-attrs">
            <div v-for="a in reqif.test_attributes" :key="a.name" v-field class="field">
              <label class="ellipsis" :title="a.name">{{ a.name }} <span class="muted small">({{ a.type }})</span></label>
              <Select
                :model-value="testChoices[a.name] ?? ''"
                :options="testTargetOptions"
                option-label="label"
                option-value="value"
                :disabled="busy"
                @update:model-value="(v: string) => choose('test', a.name, v)"
              />
              <span class="small muted ellipsis" :title="a.sample">{{ a.sample ? clip(a.sample, 60) : t('exchange.noValues') }}</span>
            </div>
          </div>
        </section>

        <section v-if="reqif && !source && reqif.relation_type_mapping.length" :aria-labelledby="ids.relTypes">
          <h3 :id="ids.relTypes" class="section-title">{{ t('exchange.relationTypes') }}</h3>
          <p class="small muted" style="margin: 0 0 8px">{{ t('exchange.relationTypesHint') }}</p>
          <div class="ex-attrs">
            <div v-for="x in reqif.relation_type_mapping" :key="x.name" v-field class="field">
              <label class="ellipsis" :title="x.name">{{ x.name }} <span class="muted small">({{ x.relations }})</span></label>
              <Select
                :model-value="relChoices[x.name] ?? x.type"
                :options="relOptions"
                option-label="label"
                option-value="value"
                :disabled="busy"
                @update:model-value="(v: string) => choose('rel', x.name, v)"
              />
            </div>
          </div>
        </section>

        <section v-if="reqif && !source" :aria-label="t('exchange.placement')">
          <div class="form-grid">
            <div class="field">
              <span :id="ids.mapping + '-target'" class="label">{{ t('exchange.target') }}</span>
              <Button
                :label="rootPath || t('common.topLevel')"
                icon="pi pi-folder"
                severity="secondary"
                outlined
                class="ellipsis"
                :aria-describedby="ids.mapping + '-target'"
                aria-haspopup="dialog"
                :disabled="busy"
                @click="pickFolder = 'requirement'"
              />
            </div>
            <div v-if="reqif.tests" class="field">
              <span :id="ids.testMapping + '-target'" class="label">{{ t('exchange.testTarget') }}</span>
              <Button
                :label="testRootPath || t('common.topLevel')"
                icon="pi pi-folder"
                severity="secondary"
                outlined
                class="ellipsis"
                :aria-describedby="ids.testMapping + '-target'"
                aria-haspopup="dialog"
                :disabled="busy"
                @click="pickFolder = 'test'"
              />
            </div>
            <div class="field">
              <span class="row nowrap"><Checkbox v-model="hierarchy" binary :input-id="ids.hier" :disabled="busy" @update:model-value="runPreview" /><label :for="ids.hier">{{ t('exchange.hierarchy', { count: reqif.folders + reqif.test_folders }) }}</label></span>
              <span class="row nowrap"><Checkbox v-model="refines" binary :input-id="ids.refines" :disabled="busy" @update:model-value="runPreview" /><label :for="ids.refines">{{ t('exchange.nestedRefines') }}</label></span>
            </div>
          </div>
        </section>

        <div class="ex-checks small">
          <span v-if="summaryConflicts" class="row nowrap">
            <Checkbox v-model="overwrite" binary input-id="ex-overwrite" /><label for="ex-overwrite">{{ t('exchange.overwrite', { count: summaryConflicts }) }}</label>
          </span>
          <span v-if="summaryNew" class="row nowrap">
            <Checkbox v-model="create" binary input-id="ex-create" /><label for="ex-create">{{ t('exchange.createNew', { count: summaryNew }) }}</label>
          </span>
        </div>

        <section :aria-labelledby="ids.preview">
          <h3 :id="ids.preview" class="section-title">{{ tests ? t('exchange.changesRequirements') : t('exchange.changes') }}</h3>
          <ExchangeDiffTable :diff="preview" :opts="opts" :excluded="excluded" :labelledby="ids.preview" @toggle="(i, on) => toggle('req', i, on)" />
        </section>

        <section v-if="tests" :aria-labelledby="ids.testPreview">
          <h3 :id="ids.testPreview" class="section-title">{{ t('exchange.changesTests') }}</h3>
          <ExchangeDiffTable :diff="tests" :opts="opts" :excluded="excludedTests" :labelledby="ids.testPreview" @toggle="(i, on) => toggle('test', i, on)" />
        </section>

        <section v-if="reqif && (!source || relations)" :aria-labelledby="ids.rels">
          <h3 :id="ids.rels" class="section-title">{{ t('exchange.relations') }}</h3>
          <p class="small" style="margin: 0">
            {{ t('exchange.relationsSummary', { add: reqif.relations.add, remove: reqif.relations.remove, keep: reqif.relations.keep }) }}
            <template v-if="reqif.relations.links"> {{ t('exchange.relationsLinks', { count: reqif.relations.links }) }}</template>
          </p>
          <ul v-if="relationChanges.length" class="ex-list small">
            <li v-for="c in relationChanges.slice(0, 50)" :key="c.ref + c.action">
              {{ t(`exchange.relationAction.${c.action}`) }}: {{ c.source }} <i>{{ relLabel(c.type) }}</i> {{ c.target }}<span v-if="c.file_type" class="muted"> ({{ c.file_type }})</span>
            </li>
          </ul>
        </section>
      </template>
    </div>
    <template #footer>
      <Button :label="result ? t('common.close') : t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button v-if="preview && !result" :label="t('exchange.saveN', { count: toSave }, toSave)" icon="pi pi-check" :loading="busy" :disabled="busy || !toSave" @click="save" />
    </template>
  </Dialog>
  <FolderPickerDialog
    :visible="pickFolder !== ''"
    :folders="pickFolder === 'test' ? testFolders : folders"
    :header="pickFolder === 'test' ? t('exchange.testTarget') : t('exchange.target')"
    @update:visible="(v: boolean) => { if (!v) pickFolder = '' }"
    @pick="onFolder"
  />
</template>

<style scoped>
.ex-attrs {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 10px 12px;
}
.ex-attrs .field {
  min-width: 0;
}
.ex-checks {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
}
.ex-list {
  margin: 2px 0 0;
  padding-left: 18px;
}
.label {
  font-weight: 600;
  font-size: 0.875rem;
}
</style>
