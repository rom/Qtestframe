<script setup lang="ts">
// Reports: standard templates, a custom report builder, saved definitions;
// download or save to the document archive.
import HelpTip from '@/guide/HelpTip.vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Checkbox from 'primevue/checkbox'
import ToggleSwitch from 'primevue/toggleswitch'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Tag from 'primevue/tag'
import QueryInput from '@/components/QueryInput.vue'
import SourceMultiSelect from '@/components/SourceMultiSelect.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { P } from '@/lib/permissions'
import { moveItem } from '@/lib/collections'
import {
  FALLBACK_FIELDS,
  batchBody,
  batchFormats,
  batchMissing,
  type BatchRow,
  FORMAT_INFO,
  KNOWN_TEMPLATES,
  REPORT_ENTITIES,
  REPORT_FORMATS,
  blankCustomForm,
  buildCustomConfig,
  formFromConfig,
  generateBody,
  missingParams,
  normalizeTemplate,
  reportFileName,
  templateParams,
  toggleColumn,
  validateCustom,
  type CustomForm,
  type ReportField,
  type ReportFormat,
  type ReportTemplate,
} from '@/lib/reports'
import type { FileInfo, ReportDefinition, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { currentLang } from '@/i18n'
import { uid } from '@/lib/a11y'
import HelpLink from '@/guide/HelpLink.vue'
import ReportEmailDialog from '@/components/ReportEmailDialog.vue'
import ReportSubscriptions from '@/components/ReportSubscriptions.vue'
import { useMetaStore } from '@/stores/meta'
import type { ReportSource } from '@/lib/email'

const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const { t, te } = useI18n()
const route = useRoute()
const canExport = computed(() => auth.can(P.export))
const canSave = computed(() => auth.can(P.requirementWrite) || auth.can(P.testWrite))
const meta = useMetaStore()
const emailOn = computed(() => !!meta.meta.email_enabled)

// ---------- send by e-mail ----------
const mail = reactive({ open: false, source: null as ReportSource | null, name: '', formats: REPORT_FORMATS as readonly ReportFormat[], format: 'pdf' as ReportFormat })
function openMail(source: ReportSource, name: string, formats: readonly ReportFormat[], format: ReportFormat) {
  Object.assign(mail, { source, name, formats, format, open: true })
}
const base = computed(() => `/projects/${proj.pid}/reports`)
const formatOptions = (formats: readonly ReportFormat[] = REPORT_FORMATS) => formats.map((f) => ({ label: FORMAT_INFO[f].label, value: f, icon: FORMAT_INFO[f].icon }))

// ---------- runs (run_summary, execution reports) ----------
const runs = ref<TestRun[]>([])
const runOptions = computed(() => runs.value.map((r) => ({ label: `${r.name}${r.environment ? ' · ' + r.environment : ''}`, value: r.id })))
async function loadRuns() {
  try {
    runs.value = asItems(await api.get<{ items: TestRun[] }>(`/projects/${proj.pid}/runs`))
  } catch {
    runs.value = []
  }
}

// ---------- templates ----------
const templates = ref<ReportTemplate[]>([])
const tplLoading = ref(true)
const tplError = ref('')
interface CardState {
  params: Record<string, string>
  format: ReportFormat
  busy: '' | 'download' | 'archive'
  error: unknown
}
const cards = reactive<Record<string, CardState>>({})
async function loadTemplates() {
  tplLoading.value = true
  tplError.value = ''
  try {
    const raw = asItems(await api.get<unknown[]>('/reports/templates'))
    templates.value = raw.map(normalizeTemplate).filter((x): x is ReportTemplate => !!x)
  } catch (e) {
    tplError.value = errorMessage(e)
    templates.value = KNOWN_TEMPLATES.map((id) => normalizeTemplate({ id })!)
  } finally {
    for (const tpl of templates.value) if (!cards[tpl.id]) cards[tpl.id] = { params: {}, format: tpl.formats.includes('pdf') ? 'pdf' : tpl.formats[0], busy: '', error: null }
    tplLoading.value = false
  }
}
const tplName = (tpl: ReportTemplate) => (te(`reports.templates.${tpl.id}.name`) ? t(`reports.templates.${tpl.id}.name`) : tpl.name)
const tplDesc = (tpl: ReportTemplate) => (te(`reports.templates.${tpl.id}.description`) ? t(`reports.templates.${tpl.id}.description`) : tpl.description)

async function runReport(body: ReturnType<typeof generateBody>, name: string, state: { busy: string; error: unknown }) {
  state.busy = body.archive ? 'archive' : 'download'
  state.error = null
  try {
    if (body.archive) {
      const f = await api.post<FileInfo>(`${base.value}/generate`, body)
      notify.ok(t('reports.archived'), f?.name ?? name)
    } else {
      const file = await api.postDownload(`${base.value}/generate`, body, reportFileName(`${proj.project?.key ?? ''}-${name}`, body.format))
      notify.ok(t('reports.downloaded'), file)
    }
  } catch (e) {
    state.error = e
    notify.fail(e, t('reports.failed'))
  } finally {
    state.busy = ''
  }
}
function generateTemplate(tpl: ReportTemplate, archive: boolean) {
  const c = cards[tpl.id]
  const missing = missingParams(tpl, c.params)
  if (missing.length) {
    c.error = new Error(t('reports.missingRun'))
    return
  }
  const name = tplName(tpl)
  runReport(generateBody({ template: tpl.id, params: templateParams(tpl, c.params) }, c.format, archive, archive ? `${name}` : undefined, currentLang()), tpl.id, c)
}

function mailTemplate(tpl: ReportTemplate) {
  const c = cards[tpl.id]
  if (missingParams(tpl, c.params).length) {
    c.error = new Error(t('reports.missingRun'))
    return
  }
  c.error = null
  openMail({ template: tpl.id, params: templateParams(tpl, c.params) }, tplName(tpl), tpl.formats, c.format)
}

// ---------- custom builder ----------
const form = ref<CustomForm>(blankCustomForm())
const customFormat = ref<ReportFormat>('pdf')
const custom = reactive({ busy: '', error: null as unknown })
const fields = ref<ReportField[]>([])
const fieldsError = ref('')
const entityOptions = computed(() => REPORT_ENTITIES.map((e) => ({ label: t(`reports.entity.${e}`), value: e })))
// Column labels: translated per entity where the meaning differs (e.g. an
// execution's status is its result), then generally, else the server label.
function translatedField(id: string, fallback: string): string {
  for (const k of [`reportFields.${form.value.entity}_${id}`, `reportFields.${id}`]) {
    if (te(k)) return t(k)
  }
  return fallback
}
const fieldLabel = (id: string) => translatedField(id, fields.value.find((f) => f.id === id)?.label ?? id)
const fieldOptions = computed(() => [{ label: t('common.none'), value: '' }, ...fields.value.map((f) => ({ label: translatedField(f.id, f.label), value: f.id }))])
async function loadFields() {
  fieldsError.value = ''
  const entity = form.value.entity || 'requirement'
  try {
    const res = asItems(await api.get<ReportField[]>(`${base.value}/fields`, { entity }))
    fields.value = res.map((f) => (typeof f === 'string' ? { id: f, label: f } : f))
  } catch (e) {
    fieldsError.value = errorMessage(e)
    fields.value = FALLBACK_FIELDS[entity].map((id) => ({ id, label: id }))
  }
  // keep only columns of this entity
  const known = new Set(fields.value.map((f) => f.id))
  form.value.columns = form.value.columns.filter((c) => known.has(c))
  if (!known.has(form.value.group_by)) form.value.group_by = ''
  if (!known.has(form.value.sort_by)) form.value.sort_by = ''
}
watch(() => form.value.entity, loadFields)
const problems = computed(() => validateCustom(form.value, fields.value.map((f) => f.id)))
const showProblems = ref(false)
const problemText = computed(() => problems.value.map((p) => t(`reports.problem.${p}`)).join(' '))
const colsId = uid('cols')
const announce = ref('')
function move(i: number, d: number) {
  const j = i + d
  if (j < 0 || j >= form.value.columns.length) return
  form.value.columns = moveItem(form.value.columns, i, j)
  announce.value = t('reports.moved', { column: fieldLabel(form.value.columns[j]), position: j + 1, total: form.value.columns.length })
}
function generateCustom(archive: boolean) {
  showProblems.value = true
  if (problems.value.length) return
  const cfg = buildCustomConfig(form.value)
  const name = cfg.title || current.value?.name || t('reports.customName')
  runReport(generateBody({ custom: cfg }, customFormat.value, archive, archive ? name : undefined, currentLang()), name, custom)
}
const scope = computed(() => form.value.entity || 'requirement')
function mailCustom() {
  showProblems.value = true
  if (problems.value.length) return
  const cfg = buildCustomConfig(form.value)
  openMail({ custom: cfg }, cfg.title || current.value?.name || t('reports.customName'), REPORT_FORMATS, customFormat.value)
}

// ---------- saved definitions ----------
const definitions = ref<ReportDefinition[]>([])
const current = ref<ReportDefinition | null>(null)
const selectedDef = ref<string | null>(null)
const defOptions = computed(() => definitions.value.map((d) => ({ label: d.name, value: d.id })))
async function loadDefinitions() {
  try {
    definitions.value = asItems(await api.get<ReportDefinition[]>(`${base.value}/definitions`)).sort((a, b) => a.name.localeCompare(b.name))
  } catch {
    definitions.value = []
  }
}
function loadDefinition(id: string | null) {
  const d = definitions.value.find((x) => x.id === id) ?? null
  current.value = d
  if (!d) return
  let cfg: unknown = d.config
  if (typeof cfg === 'string') {
    try {
      cfg = JSON.parse(cfg)
    } catch {
      cfg = {}
    }
  }
  form.value = formFromConfig(cfg)
  showProblems.value = false
  notify.info(t('reports.loaded'), d.name)
}
function newReport() {
  current.value = null
  selectedDef.value = null
  form.value = blankCustomForm(form.value.entity || 'requirement')
  showProblems.value = false
}
const saveOpen = ref(false)
const saveForm = ref({ name: '', description: '', asNew: false })
const saveError = ref('')
const saving = ref(false)
function openSave() {
  showProblems.value = true
  if (problems.value.length) return
  saveForm.value = { name: current.value?.name ?? form.value.title, description: current.value?.description ?? '', asNew: !current.value }
  saveError.value = ''
  saveOpen.value = true
}
async function saveDefinition() {
  saving.value = true
  saveError.value = ''
  const body = { name: saveForm.value.name.trim(), description: saveForm.value.description.trim(), config: buildCustomConfig(form.value) }
  try {
    const d = current.value && !saveForm.value.asNew ? await api.patch<ReportDefinition>(`/reports/definitions/${current.value.id}`, body) : await api.post<ReportDefinition>(`${base.value}/definitions`, body)
    await loadDefinitions()
    current.value = definitions.value.find((x) => x.id === d?.id) ?? d ?? null
    selectedDef.value = current.value?.id ?? null
    saveOpen.value = false
    notify.ok(t('reports.definitionSaved'), body.name)
  } catch (e) {
    saveError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
// ---------- several reports at once ----------
const batchRows = computed<BatchRow[]>(() => [
  ...templates.value.map((tpl) => ({
    kind: 'template' as const,
    id: tpl.id,
    name: tplName(tpl),
    description: tplDesc(tpl),
    formats: tpl.formats,
    required: tpl.params.filter((p) => p.required).map((p) => p.name),
  })),
  ...definitions.value.map((d) => ({ kind: 'definition' as const, id: d.id, name: d.name, description: d.description ?? '', formats: REPORT_FORMATS, required: [] })),
])
const batchSel = ref<BatchRow[]>([])
const batchParams = reactive<Record<string, string>>({})
const batchFormat = ref<ReportFormat>('pdf')
const batchBusy = ref('')
const batchFailed = ref<{ name: string; error: string }[]>([])
const batchFormatOptions = computed(() => formatOptions(batchFormats(batchSel.value)))
const batchNeedsRun = computed(() => batchSel.value.some((r) => r.required.includes('run_id')))
const batchMissingParams = computed(() => batchMissing(batchSel.value, batchParams))
const batchDefinitions = computed(() => batchSel.value.filter((r) => r.kind === 'definition'))
watch(batchFormatOptions, (opts) => {
  if (opts.length && !opts.some((o) => o.value === batchFormat.value)) batchFormat.value = opts[0].value
})
watch(batchRows, (rows) => {
  const keys = new Set(rows.map((r) => r.kind + r.id))
  batchSel.value = batchSel.value.filter((r) => keys.has(r.kind + r.id))
})
watch(runOptions, (opts) => {
  if (!batchParams.run_id && opts.length) batchParams.run_id = opts[0].value
})
async function runBatch(archive: boolean) {
  batchFailed.value = []
  batchBusy.value = archive ? 'archive' : 'zip'
  const body = batchBody(batchSel.value, batchFormat.value, archive, batchParams, currentLang())
  try {
    if (archive) {
      const res = await api.post<{ files: FileInfo[]; failed: { name: string; error: string }[] }>(`${base.value}/batch`, body)
      batchFailed.value = res.failed
      notify.ok(t('reports.batch.savedToArchive', { n: res.files.length }, res.files.length))
    } else {
      await api.postDownload(`${base.value}/batch`, body, `${proj.project?.key ?? 'reports'}-reports.zip`)
    }
  } catch (e) {
    notify.fail(e)
  } finally {
    batchBusy.value = ''
  }
}
function deleteBatchDefinitions() {
  const ids = batchDefinitions.value.map((r) => r.id)
  notify.confirmDanger(t('reports.batch.confirmDelete', { n: ids.length }, ids.length), async () => {
    try {
      await api.del(`${base.value}/definitions/bulk`, { ids })
      if (current.value && ids.includes(current.value.id)) newReport()
      await loadDefinitions()
      notify.ok(t('reports.batch.deleted', { n: ids.length }, ids.length))
    } catch (e) {
      notify.fail(e)
    }
  })
}

function deleteDefinition() {
  const d = current.value
  if (!d) return
  notify.confirmDanger(t('reports.confirmDeleteDefinition', { name: d.name }), async () => {
    try {
      await api.del(`/reports/definitions/${d.id}`)
      newReport()
      await loadDefinitions()
      notify.ok(t('reports.definitionDeleted'), d.name)
    } catch (e) {
      notify.fail(e)
    }
  })
}

onMounted(() => {
  void meta.load()
  if (!canExport.value) return
  loadTemplates()
  loadRuns()
  loadFields()
  void loadDefinitions().then(() => {
    // opened from a link (?definition=<id>)
    const id = typeof route.query.definition === 'string' ? route.query.definition : null
    if (id && definitions.value.some((d) => d.id === id)) {
      selectedDef.value = id
      loadDefinition(id)
    }
  })
})
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('reports.title') }}</h1>
      <div class="sub">{{ t('reports.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="reports-and-export" />
      <RouterLink :to="{ name: 'documents', params: { projectId: proj.pid }, query: {} }" class="p-button p-button-text p-button-secondary"><i class="pi pi-folder" aria-hidden="true" />&nbsp;{{ t('reports.toArchive') }}</RouterLink>
    </div>
  </div>
  <Message v-if="!canExport" severity="warn" :closable="false">{{ t('reports.noPermission') }}</Message>
  <template v-else>
    <section aria-labelledby="h-templates">
      <h2 id="h-templates" class="plain-heading" style="margin-bottom: 10px">{{ t('reports.standard') }}</h2>
      <Message v-if="tplError" severity="warn" :closable="false" style="margin-bottom: 10px">{{ t('reports.templatesUnavailable', { error: tplError }) }}</Message>
      <div v-if="tplLoading" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
      <ul v-else class="plain-list report-cards">
        <li v-for="tpl in templates" :key="tpl.id" class="card report-card">
          <h3 :id="`tpl-${tpl.id}`" class="plain-heading">{{ tplName(tpl) }}</h3>
          <p class="small muted" style="margin: 0">{{ tplDesc(tpl) }}</p>
          <div class="grid" style="gap: 8px">
            <div v-for="p in tpl.params" :key="p.name" v-field class="field">
              <template v-if="p.name === 'run_id'">
                <label>{{ t('reports.run') }}</label>
                <Select v-model="cards[tpl.id].params.run_id" :options="runOptions" option-label="label" option-value="value" filter :placeholder="t('reports.chooseRun')" :invalid="!!cards[tpl.id].error && !cards[tpl.id].params.run_id" />
              </template>
              <template v-else-if="p.name === 'query'">
                <span class="field-label" aria-hidden="true">{{ t('reports.filter') }}</span>
                <QueryInput v-model="cards[tpl.id].params.query" :scope="tpl.id === 'test_inventory' ? 'test' : 'requirement'" :error="cards[tpl.id].error" :placeholder="t('reports.filterPlaceholder')" />
              </template>
              <template v-else-if="p.name === 'sources'">
                <label :for="`tpl-src-${tpl.id}`">{{ t('reports.sources') }}</label>
                <SourceMultiSelect
                  :input-id="`tpl-src-${tpl.id}`"
                  :model-value="(cards[tpl.id].params.sources ?? '').split(',').filter(Boolean)"
                  :placeholder="t('analytics.compliance.usedSources')"
                  @update:model-value="(v: string[]) => (cards[tpl.id].params.sources = v.join(','))"
                />
              </template>
              <template v-else>
                <label>{{ p.name }}</label>
                <InputText v-model="cards[tpl.id].params[p.name]" />
              </template>
            </div>
            <div v-field class="field">
              <label>{{ t('reports.format') }}</label><HelpTip id="report.format" :label="t('reports.format')" />
              <Select v-model="cards[tpl.id].format" :options="formatOptions(tpl.formats)" option-label="label" option-value="value" />
            </div>
          </div>
          <Message v-if="cards[tpl.id].error && !(cards[tpl.id].error as { code?: string }).code" severity="error" :closable="false">{{ (cards[tpl.id].error as Error).message }}</Message>
          <div class="row card-actions">
            <Button :label="t('reports.download')" icon="pi pi-download" size="small" :loading="cards[tpl.id].busy === 'download'" :disabled="!!cards[tpl.id].busy" :aria-describedby="`tpl-${tpl.id}`" @click="generateTemplate(tpl, false)" />
            <Button :label="t('reports.saveToArchive')" icon="pi pi-folder-plus" size="small" severity="secondary" outlined :loading="cards[tpl.id].busy === 'archive'" :disabled="!!cards[tpl.id].busy" :aria-describedby="`tpl-${tpl.id}`" @click="generateTemplate(tpl, true)" />
            <Button v-if="emailOn" icon="pi pi-envelope" size="small" severity="secondary" text :aria-label="t('email.send.buttonOf', { name: tplName(tpl) })" v-tooltip="t('email.send.button')" @click="mailTemplate(tpl)" />
          </div>
        </li>
      </ul>
    </section>

    <section class="card" style="margin-top: 20px" aria-labelledby="h-custom">
      <div class="row" style="margin-bottom: 10px">
        <h2 id="h-custom" class="plain-heading">{{ t('reports.custom') }}<template v-if="current"> · {{ current.name }}</template></h2>
        <span class="grow" />
        <Select v-model="selectedDef" :options="defOptions" option-label="label" option-value="value" :placeholder="definitions.length ? t('reports.loadDefinition') : t('reports.noDefinitions')" :disabled="!definitions.length" :aria-label="t('reports.loadDefinition')" style="min-width: 200px" @change="loadDefinition(selectedDef)" />
        <Button :label="t('reports.new')" icon="pi pi-file" text severity="secondary" size="small" @click="newReport" />
      </div>
      <div class="builder">
        <div class="grid" style="gap: 12px">
          <div v-field class="field">
            <label>{{ t('reports.entityLabel') }}</label>
            <Select v-model="form.entity" :options="entityOptions" option-label="label" option-value="value" />
          </div>
          <div v-if="form.entity === 'execution'" v-field class="field">
            <label>{{ t('reports.run') }}</label>
            <Select v-model="form.run_id" :options="runOptions" option-label="label" option-value="value" filter :placeholder="t('reports.chooseRun')" :invalid="showProblems && problems.includes('run')" />
          </div>
          <div class="field">
            <span class="field-label" aria-hidden="true">{{ t('reports.filter') }}</span>
            <QueryInput v-model="form.query" :scope="scope" :error="custom.error" :placeholder="t('reports.filterPlaceholder')" />
          </div>
          <div v-field class="field"><label>{{ t('reports.reportTitle') }}</label><InputText v-model="form.title" :maxlength="200" /></div>
          <div class="form-grid" style="gap: 12px">
            <div v-field class="field"><label>{{ t('reports.groupBy') }}</label><HelpTip id="report.groupBy" :label="t('reports.groupBy')" /><Select v-model="form.group_by" :options="fieldOptions" option-label="label" option-value="value" :placeholder="t('common.none')" /></div>
            <div v-field class="field"><label>{{ t('reports.sortBy') }}</label><Select v-model="form.sort_by" :options="fieldOptions" option-label="label" option-value="value" :placeholder="t('common.none')" /></div>
          </div>
          <div class="row" style="gap: 18px">
            <label class="row nowrap small" for="rp-desc"><ToggleSwitch v-model="form.sort_desc" input-id="rp-desc" :disabled="!form.sort_by" /> {{ t('reports.descending') }}</label>
            <label class="row nowrap small" for="rp-sum"><ToggleSwitch v-model="form.include_summary" input-id="rp-sum" /> {{ t('reports.summary') }}</label>
          </div>
        </div>
        <div class="grid" style="gap: 10px; align-content: start">
          <fieldset class="plain-fieldset">
            <legend class="field-label">{{ t('reports.availableColumns') }} <HelpTip id="report.columns" :label="t('reports.availableColumns')" /></legend>
            <Message v-if="fieldsError" severity="warn" :closable="false" class="small">{{ t('reports.fieldsUnavailable') }}</Message>
            <div class="col-pick">
              <span v-for="f in fields" :key="f.id" class="row nowrap small">
                <Checkbox :model-value="form.columns.includes(f.id)" binary :input-id="`${colsId}-${f.id}`" @update:model-value="form.columns = toggleColumn(form.columns, f.id)" />
                <label :for="`${colsId}-${f.id}`">{{ fieldLabel(f.id) }}</label>
              </span>
            </div>
          </fieldset>
          <div>
            <div :id="`${colsId}-order`" class="field-label">{{ t('reports.columnOrder') }}</div>
            <ol v-if="form.columns.length" class="col-order" :aria-labelledby="`${colsId}-order`">
              <li v-for="(c, i) in form.columns" :key="c">
                <span class="grow ellipsis">{{ i + 1 }}. {{ fieldLabel(c) }}</span>
                <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :disabled="i === 0" :aria-label="t('reports.moveUp', { column: fieldLabel(c) })" @click="move(i, -1)" />
                <Button icon="pi pi-arrow-down" text rounded size="small" severity="secondary" :disabled="i === form.columns.length - 1" :aria-label="t('reports.moveDown', { column: fieldLabel(c) })" @click="move(i, 1)" />
                <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('reports.removeColumn', { column: fieldLabel(c) })" @click="form.columns = toggleColumn(form.columns, c)" />
              </li>
            </ol>
            <div v-else class="small muted">{{ t('reports.noColumns') }}</div>
            <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>
          </div>
        </div>
      </div>
      <Message v-if="showProblems && problems.length" severity="error" :closable="false" style="margin-top: 12px"><span role="alert">{{ problemText }}</span></Message>
      <div class="row" style="margin-top: 14px">
        <div v-field class="field" style="min-width: 140px">
          <label>{{ t('reports.format') }}</label>
          <Select v-model="customFormat" :options="formatOptions()" option-label="label" option-value="value" />
        </div>
        <span class="grow" />
        <template v-if="canSave">
          <Button v-if="current" :label="t('common.delete')" icon="pi pi-trash" severity="danger" text @click="deleteDefinition" />
          <Button :label="t('reports.saveDefinition')" icon="pi pi-save" severity="secondary" text @click="openSave" />
        </template>
        <Button v-if="emailOn" :label="t('email.send.button')" icon="pi pi-envelope" severity="secondary" text @click="mailCustom" />
        <Button :label="t('reports.saveToArchive')" icon="pi pi-folder-plus" severity="secondary" outlined :loading="custom.busy === 'archive'" :disabled="!!custom.busy" @click="generateCustom(true)" />
        <Button :label="t('reports.download')" icon="pi pi-download" :loading="custom.busy === 'download'" :disabled="!!custom.busy" @click="generateCustom(false)" />
      </div>
    </section>

    <section v-if="canExport && batchRows.length" class="card" style="margin-top: 20px" aria-labelledby="h-batch">
      <h2 id="h-batch" class="plain-heading">{{ t('reports.batch.title') }}</h2>
      <p class="small muted" style="margin: 4px 0 10px">{{ t('reports.batch.intro') }}</p>
      <DataTable v-model:selection="batchSel" v-stack :value="batchRows" :data-key="(r: BatchRow) => r.kind + r.id" size="small">
        <Column selection-mode="multiple" style="width: 2.6rem" />
        <Column :header="t('common.name')" body-class="stack-main">
          <template #body="{ data }">
            <b>{{ data.name }}</b>
            <div v-if="data.description" class="small muted">{{ data.description }}</div>
          </template>
        </Column>
        <Column :header="t('common.type')" style="width: 160px">
          <template #body="{ data }"><Tag :value="data.kind === 'template' ? t('reports.batch.standard') : t('reports.batch.savedReport')" :severity="data.kind === 'template' ? 'secondary' : 'info'" /></template>
        </Column>
      </DataTable>
      <div v-if="batchSel.length" class="bulkbar" role="toolbar" :aria-label="t('reports.batch.toolbar')" style="margin-top: 10px">
        <b>{{ t('common.selected', { count: batchSel.length }) }}</b>
        <Select v-model="batchFormat" :options="batchFormatOptions" option-label="label" option-value="value" :aria-label="t('reports.format')" size="small" style="min-width: 110px" />
        <Select v-if="batchNeedsRun" v-model="batchParams.run_id" :options="runOptions" option-label="label" option-value="value" :placeholder="t('reports.batch.chooseRun')" :aria-label="t('reports.batch.runForSummary')" size="small" style="min-width: 180px" />
        <Button :label="t('reports.batch.zip')" icon="pi pi-download" size="small" :loading="batchBusy === 'zip'" :disabled="!!batchBusy || !batchFormatOptions.length || batchMissingParams.length > 0" @click="runBatch(false)" />
        <Button v-if="canSave" :label="t('reports.saveToArchive')" icon="pi pi-folder-plus" size="small" severity="secondary" outlined :loading="batchBusy === 'archive'" :disabled="!!batchBusy || !batchFormatOptions.length || batchMissingParams.length > 0" @click="runBatch(true)" />
        <Button v-if="canSave && batchDefinitions.length" :label="t('reports.batch.deleteSaved', { n: batchDefinitions.length }, batchDefinitions.length)" icon="pi pi-trash" size="small" severity="danger" outlined @click="deleteBatchDefinitions" />
        <span v-if="!batchFormatOptions.length" class="small muted">{{ t('reports.batch.noCommonFormat') }}</span>
        <span v-else-if="batchMissingParams.length" class="small muted">{{ t('reports.batch.needRun') }}</span>
      </div>
      <Message v-if="batchFailed.length" severity="warn" closable style="margin-top: 10px" @close="batchFailed = []">
        <b>{{ t('reports.batch.someFailed', { n: batchFailed.length }, batchFailed.length) }}</b>
        <ul style="margin: 4px 0 0; padding-left: 18px">
          <li v-for="f in batchFailed" :key="f.name" class="small"><b>{{ f.name }}</b>: {{ f.error }}</li>
        </ul>
      </Message>
    </section>
    <ReportSubscriptions v-if="emailOn" :definitions="definitions" :can-save="canSave" />
  </template>

  <ReportEmailDialog v-model:visible="mail.open" :source="mail.source" :name="mail.name" :formats="mail.formats" :format="mail.format" />

  <Dialog v-model:visible="saveOpen" modal :header="t('reports.saveDefinition')" :style="{ width: 'min(460px, calc(100vw - 16px))' }">
    <form class="grid" style="gap: 12px" @submit.prevent="saveDefinition">
      <Message v-if="saveError" severity="error" :closable="false">{{ saveError }}</Message>
      <div v-field class="field"><label>{{ t('common.name') }}</label><InputText v-model="saveForm.name" required autofocus /></div>
      <div v-field class="field"><label>{{ t('common.description') }}</label><Textarea v-model="saveForm.description" rows="2" auto-resize /></div>
      <div v-if="current" class="row nowrap small"><Checkbox v-model="saveForm.asNew" binary input-id="rp-asnew" /><label for="rp-asnew">{{ t('reports.saveAsNew') }}</label></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="saveOpen = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!saveForm.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.report-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr));
  gap: 14px;
}
.report-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.card-actions {
  margin-top: auto;
}
.builder {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr));
  gap: 18px;
}
.col-pick {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 6px 10px;
  margin-top: 6px;
}
.col-order {
  margin: 6px 0 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
}
.col-order li {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 2px 4px 2px 10px;
  border-top: 1px solid var(--qtf-border);
  font-size: 13px;
}
.col-order li:first-child {
  border-top: none;
}
</style>
