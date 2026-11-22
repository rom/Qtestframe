<script setup lang="ts">
// Import an edited Excel round-trip workbook (requirements or tests), a
// ReqIF document (DOORS, DOORS Next, Polarion, Qtestframe) or the issues
// and work items of a Jira (JQL) or Azure DevOps (WIQL, or area path and
// work item types) query, with a diff preview: new, changed, unchanged,
// conflicting and erroneous rows, cell by cell. Nothing is saved before
// Save; the save is one transaction.
import { computed, nextTick, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import { useI18n } from 'vue-i18n'
import FileDropZone from './FileDropZone.vue'
import FolderPickerDialog from './FolderPickerDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { upload } from '@/lib/upload'
import { api, errorMessage } from '@/lib/api'
import { folderPath } from '@/lib/tree'
import { uid } from '@/lib/a11y'
import type { Folder, ImportSources } from '@/lib/types'
import { externalImportBody, IMPORT_SOURCES, sourceConfigured, type ImportSourceId } from '@/lib/importSources'
import {
  acceptsFile,
  changedAttrMapping,
  clip,
  EXCHANGE_ACCEPT,
  EXCHANGE_MAX_MB,
  exchangeFields,
  ROW_STATUSES,
  saveCount,
  savable,
  statusCount,
  visibleRows,
  type ExchangeDiff,
  type ExchangeKind,
  type ExchangeMode,
  type ExchangeResult,
  type ExchangeRow,
  type ReqIFPreview,
  type RowStatus,
} from '@/lib/exchange'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ projectId: string; kind: ExchangeKind; mode: ExchangeMode | ImportSourceId; folders: Folder[]; folderId?: string | null; canExport?: boolean }>()
const emit = defineEmits<{ imported: [result: ExchangeResult] }>()
const { t } = useI18n()

/** rows shown at once in the table */
const SHOWN = 200

const file = ref<File | null>(null)
const preview = ref<ExchangeDiff | ReqIFPreview | null>(null)
const result = ref<ExchangeResult | null>(null)
const busy = ref(false)
const error = ref('')
const downloading = ref(false)
const choices = ref<Record<string, string>>({})
const folderId = ref<string | null>(null)
const hierarchy = ref(true)
const overwrite = ref(false)
const create = ref(true)
const excluded = ref(new Set<number>())
const filter = ref<RowStatus[]>([])
const pickFolder = ref(false)
const ids = { mapping: uid('ex-map'), preview: uid('ex-prev'), status: uid('ex-status'), rels: uid('ex-rels'), filter: uid('ex-filter'), query: uid('ex-query') }

/** an import from Jira or Azure DevOps (no file: a query) */
const source = computed<ImportSourceId | null>(() => ((IMPORT_SOURCES as readonly string[]).includes(props.mode) ? (props.mode as ImportSourceId) : null))
const isReqIF = computed(() => props.mode === 'reqif')
const fileMode = computed<ExchangeMode>(() => (isReqIF.value ? 'reqif' : 'xlsx'))
/** a preview with attributes and relations: ReqIF or a source */
const reqif = computed(() => (isReqIF.value || source.value ? (preview.value as (ReqIFPreview & { truncated?: boolean; query?: string }) | null) : null))
const sourceName = computed(() => (source.value === 'jira' ? 'Jira' : 'Azure DevOps'))
const sources = ref<ImportSources | null>(null)
const sourcesError = ref('')
const query = ref('')
const areaPath = ref('')
const types = ref('')
const relations = ref(true)
const ready = computed(() => !!source.value && sourceConfigured(sources.value, source.value))

watch(visible, (v) => {
  if (!v) return
  file.value = null
  preview.value = null
  result.value = null
  error.value = ''
  choices.value = {}
  folderId.value = props.folderId ?? null
  hierarchy.value = true
  overwrite.value = false
  create.value = true
  excluded.value = new Set()
  filter.value = []
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
const targetOptions = computed(() => [
  { label: t('exchange.ignore'), value: '' },
  ...(reqif.value?.targets ?? []).map((x) => ({ label: x.type === 'role' ? t(`exchange.role.${x.key}`) : x.label, value: x.key })),
])
const rootPath = computed(() => (folderId.value ? folderPath(props.folders, folderId.value) : ''))
const opts = computed(() => ({ overwrite: overwrite.value, create: create.value }))
const shown = computed(() => (preview.value ? visibleRows(preview.value.rows, filter.value).slice(0, SHOWN) : []))
const filtered = computed(() => (preview.value ? visibleRows(preview.value.rows, filter.value).length : 0))
const toSave = computed(() => (preview.value ? saveCount(preview.value.rows, opts.value, excluded.value) : 0))
const relationChanges = computed(() => (reqif.value ? reqif.value.relations.changes.filter((c) => c.action !== 'keep') : []))

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
  const mapping = reqif.value ? changedAttrMapping(reqif.value.attributes, choices.value) : undefined
  return exchangeFields(
    fileMode.value,
    { mapping, folderId: folderId.value, hierarchy: hierarchy.value, kind: props.kind },
    apply ? { overwrite: overwrite.value, create: create.value, excluded: excluded.value } : undefined,
  )
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
    const attrs = (res as Partial<ReqIFPreview>).attributes
    if (attrs) {
      const next: Record<string, string> = {}
      for (const a of attrs) next[a.name] = a.field || ''
      choices.value = next
    }
    excluded.value = new Set()
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
  void runPreview()
}

function onMapping(name: string, field: string) {
  choices.value = { ...choices.value, [name]: field }
  void runPreview()
}

function onFolder(id: string | null) {
  folderId.value = id
  if (!source.value || preview.value) void runPreview()
}

function toggleRow(index: number, on: boolean) {
  const s = new Set(excluded.value)
  if (on) s.delete(index)
  else s.add(index)
  excluded.value = s
}

function toggleFilter(st: RowStatus) {
  filter.value = filter.value.includes(st) ? filter.value.filter((x) => x !== st) : [...filter.value, st]
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
    const res = source.value
      ? await api.post<ExchangeResult>(`/projects/${props.projectId}/requirements/import/${source.value}/apply`, sourceBody(true))
      : await upload<ExchangeResult>(`/projects/${props.projectId}/exchange/${props.mode}/apply`, file.value as File, { fields: fields(true) })
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

const severity: Record<RowStatus, string> = { new: 'info', changed: 'success', conflict: 'warn', error: 'danger', unchanged: 'secondary' }
const icon: Record<RowStatus, string> = { new: 'pi pi-plus', changed: 'pi pi-pencil', conflict: 'pi pi-exclamation-triangle', error: 'pi pi-times-circle', unchanged: 'pi pi-check' }

function rowLabel(r: ExchangeRow) {
  return [r.key, r.title].filter(Boolean).join(' ')
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
        <HelpLink page="reports-and-export" :anchor="{ en: 'exchanging-data-with-other-tools', sv: 'utbyta-data-med-andra-verktyg' }" />
      </div>
      <div v-if="!isReqIF && canExport && !preview && !result" class="row">
        <Button :label="t('exchange.downloadWorkbook')" icon="pi pi-download" severity="secondary" outlined :loading="downloading" @click="exportWorkbook" />
      </div>

      <template v-if="source">
        <Message v-if="sourcesError" severity="error" :closable="false" role="alert">{{ sourcesError }}</Message>
        <Message v-else-if="sources && !ready" severity="warn" :closable="false">{{ t('exchange.sourceMissing', { source: sourceName }) }}</Message>
        <form v-else-if="sources" class="grid" style="gap: 10px" :aria-labelledby="ids.query" @submit.prevent="runPreview">
          <h3 :id="ids.query" class="section-title" style="margin: 0">{{ t('exchange.sourceQuery') }}</h3>
          <template v-if="source === 'azure-devops'">
            <div class="form-grid">
              <div v-field class="field">
                <label>{{ t('importSources.areaPath') }}</label>
                <InputText v-model="areaPath" class="mono" :disabled="busy || !!query.trim()" placeholder="Shop\Web" />
              </div>
              <div v-field class="field">
                <label>{{ t('importSources.workItemTypes') }}</label>
                <InputText v-model="types" :disabled="busy || !!query.trim()" placeholder="User Story, Requirement" />
              </div>
            </div>
          </template>
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
                @click="pickFolder = true"
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
            <template v-if="result.folders_created"> {{ t('exchange.doneFolders', { count: result.folders_created }) }}</template>
            <template v-if="result.relations_added || result.relations_removed"> {{ t('exchange.doneRelations', { added: result.relations_added, removed: result.relations_removed }) }}</template>
            <template v-if="result.links_added || result.links_removed"> {{ t('exchange.doneLinks', { added: result.links_added, removed: result.links_removed }) }}</template>
          </Message>
        </div>
        <ul v-if="result.items.length" class="ex-list small">
          <li v-for="it in result.items.slice(0, 50)" :key="it.id">
            <span class="key">{{ it.key }}</span> {{ it.title }} — {{ t(`exchange.action.${it.action}`) }}
          </li>
          <li v-if="result.items.length > 50" class="muted">{{ t('exchange.andMore', { count: result.items.length - 50 }) }}</li>
        </ul>
        <details v-if="result.skipped_rows.length" class="small">
          <summary>{{ t('exchange.skippedList', { count: result.skipped_rows.length }) }}</summary>
          <ul class="ex-list">
            <li v-for="s in result.skipped_rows" :key="s.index">{{ s.location }}: {{ s.title }} — {{ t(`exchange.reason.${s.reason}`) }}</li>
          </ul>
        </details>
      </template>

      <template v-else-if="preview">
        <Message v-for="(w, i) in preview.warnings" :key="'w' + i" severity="warn" :closable="false">{{ w }}</Message>
        <p v-if="preview.ignored.length" class="small muted" style="margin: 0">{{ t('exchange.ignoredColumns', { columns: preview.ignored.join(', ') }) }}</p>
        <p v-if="reqif && reqif.previous" class="small" style="margin: 0"><i class="pi pi-replay" aria-hidden="true" /> {{ t(source ? 'exchange.reimportSource' : 'exchange.reimport', { count: reqif.previous, source: sourceName }) }}</p>
        <details v-if="reqif && (reqif.errors.length || reqif.skipped.length)" class="small">
          <summary>{{ t('exchange.problems', { count: reqif.errors.length + reqif.skipped.length }) }}</summary>
          <ul class="ex-list">
            <li v-for="(e, i) in [...reqif.errors, ...reqif.skipped]" :key="i">{{ e }}</li>
          </ul>
        </details>

        <section v-if="reqif" :aria-labelledby="ids.mapping">
          <h3 :id="ids.mapping" class="section-title">{{ source ? t('exchange.sourceFields') : t('exchange.attributes') }}</h3>
          <p class="small muted" style="margin: 0 0 8px">{{ source ? t('exchange.sourceFieldsHint') : t('exchange.attributesHint') }}</p>
          <div class="ex-attrs">
            <div v-for="a in reqif.attributes" :key="a.name" v-field class="field">
              <label class="ellipsis" :title="a.name">{{ a.name }} <span class="muted small">({{ a.type }})</span></label>
              <Select
                :model-value="choices[a.name] ?? ''"
                :options="targetOptions"
                option-label="label"
                option-value="value"
                :disabled="busy"
                @update:model-value="(v: string) => onMapping(a.name, v)"
              />
              <span class="small muted ellipsis" :title="a.sample">{{ a.sample ? clip(a.sample, 60) : t('exchange.noValues') }}</span>
            </div>
          </div>
          <div v-if="!source" class="form-grid" style="margin-top: 10px">
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
                @click="pickFolder = true"
              />
            </div>
            <div class="field">
              <span class="row nowrap"><Checkbox v-model="hierarchy" binary input-id="ex-hier" :disabled="busy" @update:model-value="runPreview" /><label for="ex-hier">{{ t('exchange.hierarchy', { count: reqif.folders }) }}</label></span>
            </div>
          </div>
        </section>

        <section :aria-labelledby="ids.preview">
          <h3 :id="ids.preview" class="section-title">{{ t('exchange.changes') }}</h3>
          <div class="row wrap ex-filter" role="group" :aria-label="t('exchange.filterLabel')">
            <Button
              v-for="st in ROW_STATUSES"
              :key="st"
              size="small"
              :severity="filter.includes(st) ? undefined : 'secondary'"
              :outlined="!filter.includes(st)"
              :icon="icon[st]"
              :label="t(`exchange.status.${st}`) + ' ' + statusCount(preview.summary, st)"
              :aria-pressed="filter.includes(st)"
              @click="toggleFilter(st)"
            />
          </div>
          <div class="ex-checks small">
            <span v-if="preview.summary.conflicts" class="row nowrap">
              <Checkbox v-model="overwrite" binary input-id="ex-overwrite" /><label for="ex-overwrite">{{ t('exchange.overwrite', { count: preview.summary.conflicts }) }}</label>
            </span>
            <span v-if="preview.summary.new" class="row nowrap">
              <Checkbox v-model="create" binary input-id="ex-create" /><label for="ex-create">{{ t('exchange.createNew', { count: preview.summary.new }) }}</label>
            </span>
          </div>
          <p class="small muted" style="margin: 6px 0">
            {{ filtered > SHOWN ? t('exchange.showingFirst', { shown: SHOWN, total: filtered }) : t('exchange.showingAll', { total: filtered }) }}
          </p>
          <DataTable :value="shown" data-key="index" size="small" scrollable scroll-height="45vh" class="ex-table" :aria-labelledby="ids.preview">
            <Column :header="t('exchange.include')" style="width: 70px">
              <template #body="{ data }">
                <Checkbox
                  v-if="savable(data, opts)"
                  :model-value="!excluded.has(data.index)"
                  binary
                  :input-id="`ex-inc-${data.index}`"
                  :aria-label="t('exchange.includeRow', { item: rowLabel(data) })"
                  @update:model-value="(v: boolean) => toggleRow(data.index, v)"
                />
              </template>
            </Column>
            <Column field="location" :header="t('exchange.location')" style="width: 130px" />
            <Column :header="t('exchange.item')" style="min-width: 180px" body-class="stack-main">
              <template #body="{ data }">
                <span v-if="data.key" class="key">{{ data.key }}</span> {{ data.title }}
              </template>
            </Column>
            <Column :header="t('exchange.statusColumn')" style="width: 130px">
              <template #body="{ data }">
                <Tag :severity="severity[data.status as RowStatus]" :icon="icon[data.status as RowStatus]" :value="t(`exchange.status.${data.status}`)" style="white-space: nowrap" />
                <div v-if="data.status === 'conflict'" class="small muted">{{ t('exchange.versions', { base: data.base_version, now: data.version }) }}</div>
              </template>
            </Column>
            <Column :header="t('exchange.details')" style="min-width: 260px">
              <template #body="{ data }">
                <ul v-if="data.errors.length" class="ex-list small error-text">
                  <li v-for="(e, i) in data.errors" :key="'e' + i"><span v-if="e.cell" class="mono">{{ e.cell }}</span> <b>{{ e.label }}:</b> {{ e.message }}</li>
                </ul>
                <ul v-if="data.changes.length" class="ex-list small">
                  <li v-for="(c, i) in data.changes" :key="'c' + i">
                    <b>{{ c.label }}:</b>
                    <template v-if="data.status !== 'new'">
                      <del :aria-label="t('exchange.oldValue', { value: c.old || t('exchange.empty') })">{{ clip(c.old) || t('exchange.empty') }}</del>
                      <span aria-hidden="true"> → </span>
                    </template>
                    <ins :aria-label="t('exchange.newValue', { value: c.new || t('exchange.empty') })">{{ clip(c.new) || t('exchange.empty') }}</ins>
                  </li>
                </ul>
                <ul v-if="data.warnings.length" class="ex-list small muted">
                  <li v-for="(w, i) in data.warnings" :key="'w' + i">{{ w }}</li>
                </ul>
              </template>
            </Column>
            <template #empty><div class="empty">{{ t('exchange.none') }}</div></template>
          </DataTable>
        </section>

        <section v-if="reqif && (!source || relations)" :aria-labelledby="ids.rels">
          <h3 :id="ids.rels" class="section-title">{{ t('exchange.relations') }}</h3>
          <p class="small" style="margin: 0">{{ t('exchange.relationsSummary', { add: reqif.relations.add, remove: reqif.relations.remove, keep: reqif.relations.keep }) }}</p>
          <ul v-if="relationChanges.length" class="ex-list small">
            <li v-for="c in relationChanges.slice(0, 50)" :key="c.ref + c.action">
              {{ t(`exchange.relationAction.${c.action}`) }}: {{ c.source }} <i>{{ c.type }}</i> {{ c.target }}<span v-if="c.file_type" class="muted"> ({{ c.file_type }})</span>
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
  <FolderPickerDialog v-model:visible="pickFolder" :folders="folders" :header="t('exchange.target')" @pick="onFolder" />
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
  margin-top: 8px;
}
.ex-filter {
  gap: 6px;
}
.ex-list {
  margin: 2px 0 0;
  padding-left: 18px;
}
.ex-list del {
  text-decoration: line-through;
  opacity: 0.85;
}
.ex-list ins {
  text-decoration: underline;
}
.label {
  font-weight: 600;
  font-size: 0.875rem;
}
</style>
