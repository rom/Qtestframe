<script setup lang="ts">
import SuspectLink from './SuspectLink.vue'
import ObligationHelp from './ObligationHelp.vue'
import ItemLinksPanel from '@/components/ItemLinksPanel.vue'
import RequirementRisks from './RequirementRisks.vue'
import DefectsPanel from '@/components/DefectsPanel.vue'
import { isSoftware } from '@/lib/projectTypes'
import SplitButton from 'primevue/splitbutton'
import AutoComplete from 'primevue/autocomplete'
import { useProjectStore } from '@/stores/project'
import { filterSources, mergeSources } from '@/lib/sources'
import ReviewBar from './ReviewBar.vue'
import { P } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth'
import TagSuggestions from './TagSuggestions.vue'
import MentionInput from './MentionInput.vue'
import HelpTip from '@/guide/HelpTip.vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import Checkbox from 'primevue/checkbox'
import StatusTag from './StatusTag.vue'
import TagsInput from './TagsInput.vue'
import CategorySelect from './CategorySelect.vue'
import ItemPickerDialog from './ItemPickerDialog.vue'
import CommentsPanel from './CommentsPanel.vue'
import AttachmentsPanel from './AttachmentsPanel.vue'
import HistoryPanel from './HistoryPanel.vue'
import RelationsPanel from './RelationsPanel.vue'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import { api, asItems, errorMessage } from '@/lib/api'
import { debounce, moveItem } from '@/lib/collections'
import { segments } from '@/lib/highlight'
import { formatDateTime } from '@/lib/format'
import { useI18n } from 'vue-i18n'
import { uid } from '@/lib/a11y'
import { enumOptions, obligationHint, obligationKeyword } from '@/i18n/labels'
import { issueMessage } from '@/i18n/analysis'
import { buildTree, flatten, folderPath } from '@/lib/tree'
import type { Analysis, Category, EvaluationCriterion, Folder, Requirement, TestCase, TraceLink } from '@/lib/types'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useUndoStore } from '@/stores/undo'
import { changedKeys, clone, pickBefore } from '@/lib/undo'
import ConflictDialog from './ConflictDialog.vue'
import { fieldConflicts, REQUIREMENT_FIELDS, staleOf, type FieldConflict, type Stale } from '@/lib/conflicts'
import { createdEntry, deletedEntry, patchEntry, recordUndo, toggleEntry } from '@/lib/undoActions'

const props = defineProps<{
  projectId: string
  itemId: string | null
  folderId?: string | null
  categories: Category[]
  folders: Folder[]
  canWrite: boolean
  canLink: boolean
}>()
const emit = defineEmits<{ saved: [item: Requirement]; deleted: [id: string]; copied: [item: Requirement]; close: [] }>()
const meta = useMetaStore()
const notify = useNotify()
const prefs = usePrefsStore()
const { t, te } = useI18n()
const tabsId = uid('req-tabs')
const linkCount = ref<number | null>(null)
watch(
  () => props.itemId,
  () => (linkCount.value = null),
)
/** tab panel attributes for the content of a tab (only when the tabs are shown) */
function panel(value: string) {
  return item.value ? { role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` } : {}
}

interface CriterionRow {
  id?: string
  description: string
  method: string
  threshold: string
}

function blank() {
  return {
    title: '',
    description: '',
    obligation: '',
    type: 'functional',
    priority: 'medium',
    status: 'draft',
    source: '',
    rationale: '',
    owner: '',
    tags: [] as string[],
    category_ids: [] as string[],
    folder_id: (props.folderId ?? null) as string | null,
    criteria: [] as CriterionRow[],
  }
}

const form = ref(blank())
const item = ref<Requirement | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const links = ref<TraceLink[]>([])
const tests = ref<TestCase[]>([])
const pickTests = ref(false)
const linkCriterion = ref<string | null>(null)

const isNew = computed(() => !props.itemId)
// New requirements get a linked test by default: personal preference
// create_linked_test, else the server setting requirements.auto_create_test.
const createTest = ref(prefs.createLinkedTest(meta.meta.auto_create_test))
watch([() => meta.meta.auto_create_test, () => prefs.prefs.create_linked_test], () => {
  if (isNew.value) createTest.value = prefs.createLinkedTest(meta.meta.auto_create_test)
})

// ---------- tabs ----------
const tab = ref('details')
const commentCount = ref(0)
const attachmentCount = ref<number | null>(null)
const relationCount = ref(0)
/** open defects (of the requirement, its tests and their executions) */
const openDefects = ref(0)
function resolveId(field: string, id: string): string | undefined {
  if (field === 'category_ids') {
    const c = props.categories.find((x) => x.id === id)
    return c ? `${c.dimension}/${c.name}` : undefined
  }
  if (field === 'folder_id') return folderPath(props.folders, id) || undefined
  return undefined
}
async function onRestored(r: unknown) {
  await load()
  if (r && typeof r === 'object') emit('saved', r as Requirement)
}
const folderOptions = computed(() => [
  { id: null as string | null, label: t('common.topLevelOption') },
  ...flatten(buildTree(props.folders)).map((f) => ({ id: f.id as string | null, label: '  '.repeat(f.depth) + f.label })),
])
const obligationOptions = computed(() => [{ label: t('requirements.autoDetect'), value: '' }, ...enumOptions('obligation', meta.meta.obligations)])
const auth = useAuthStore()
const proj = useProjectStore()
/** review decisions need the review permission (the current status stays selectable) */
const REVIEW_DECISIONS = ['approved', 'rejected']
const canReview = computed(() => auth.can(P.requirementReview))
const statusOptions = computed(() =>
  enumOptions('status', meta.meta.requirement_statuses).map((o) => ({ ...o, disabled: !canReview.value && REVIEW_DECISIONS.includes(o.value) && o.value !== item.value?.status })),
)
// ---------- requirement source: the project's list plus sources already used ----------
const sources = ref<string[]>([])
async function loadSources() {
  try {
    const res = await api.get<{ configured: string[]; used: { source: string; count: number }[] }>(`/projects/${props.projectId}/requirement-sources`)
    sources.value = mergeSources(res.configured ?? [], (res.used ?? []).map((u) => u.source))
  } catch {
    sources.value = proj.project?.requirement_sources ?? []
  }
}
const sourceSuggestions = ref<string[]>([])
function completeSource(e: { query: string }) {
  sourceSuggestions.value = filterSources(sources.value, e.query)
}
watch(() => props.projectId, loadSources, { immediate: true })

/** copy as the basis of a new requirement; derived also relates it ("derived from") */
async function copy(derived = false) {
  if (!item.value) return
  try {
    const c = await api.post<Requirement>(`/requirements/${item.value.id}/copy`, derived ? { relation: 'derived_from' } : {})
    notify.ok(t('requirements.copied'), c?.key)
    if (c?.id) recordUndo(createdEntry(t('undo.a.copy', { key: item.value.key }), delRequirement(c.id)))
    emit('copied', c)
  } catch (e) {
    notify.fail(e)
  }
}
const copyItems = computed(() => [{ label: t('requirements.copyDerived'), icon: 'pi pi-sitemap', command: () => copy(true) }])
async function onReviewed(updated: unknown) {
  await load()
  if (updated && typeof updated === 'object') emit('saved', updated as Requirement)
}
const typeOptions = computed(() => enumOptions('requirementType', meta.meta.requirement_types))
const priorityOptions = computed(() => enumOptions('priority', meta.meta.priorities))
const methodOptions = computed(() => enumOptions('evaluationMethod', meta.meta.evaluation_methods))

async function load() {
  error.value = ''
  analysis.value = null
  links.value = []
  if (!props.itemId) {
    item.value = null
    form.value = blank()
    tab.value = 'details'
    return
  }
  loading.value = true
  try {
    const r = await api.get<Requirement>(`/requirements/${props.itemId}`)
    item.value = r
    commentCount.value = r.comment_count ?? 0
    attachmentCount.value = null
    relationCount.value = r.relation_count ?? 0
    openDefects.value = r.open_defects ?? 0
    form.value = {
      title: r.title,
      description: r.description,
      obligation: r.obligation,
      type: r.type,
      priority: r.priority,
      status: r.status,
      source: r.source,
      rationale: r.rationale,
      owner: r.owner,
      tags: [...(r.tags ?? [])],
      category_ids: [...(r.category_ids ?? [])],
      folder_id: r.folder_id,
      criteria: [...(r.criteria ?? [])]
        .sort((a, b) => a.position - b.position)
        .map((c) => ({ id: c.id, description: c.description, method: c.method, threshold: c.threshold })),
    }
    loaded.value = clone(bodyOf(form.value))
    await loadLinks()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

const suspectCount = computed(() => links.value.filter((l) => l.suspect_since).length)
async function onSuspectConfirmed() {
  await load()
}
async function confirmAll() {
  try {
    await api.post(`/requirements/${props.itemId}/links/confirm`)
    notify.ok(t('suspect.confirmed'))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
async function loadLinks() {
  if (!props.itemId) return
  try {
    links.value = asItems(await api.get<TraceLink[]>(`/requirements/${props.itemId}/links`))
    if (links.value.length && !tests.value.length) {
      tests.value = asItems(await api.get<{ items: TestCase[] }>(`/projects/${props.projectId}/tests`))
    }
  } catch (e) {
    notify.fail(e, t('editor.loadLinksFailed'))
  }
}

// ---------- language analysis ----------
const analysis = ref<Analysis | null>(null)
const analyzing = ref(false)
const LANG_KEY = 'qtf.analysisLanguage'
function storedLanguage(): string {
  try {
    return localStorage.getItem(LANG_KEY) || 'auto'
  } catch {
    return 'auto'
  }
}
/** "auto" detects the language of the text; otherwise a pack code. */
const analysisLanguage = ref(storedLanguage())
const languageLabel = (code: string) => (te(`analysis.languages.${code}`) ? t(`analysis.languages.${code}`) : code)
const languageOptions = computed(() => [
  { value: 'auto', label: t('analysis.languageAuto') },
  ...meta.meta.analysis_languages.map((c) => ({ value: c, label: languageLabel(c) })),
])
watch(analysisLanguage, (v) => {
  try {
    localStorage.setItem(LANG_KEY, v)
  } catch {
    /* per-viewer convenience only */
  }
  runAnalysis(form.value.description ?? '')
  runAnalysis.flush()
})
const runAnalysis = debounce(async (text: string) => {
  if (!text.trim() || prefs.hideAnalysis) {
    analysis.value = null
    return
  }
  analyzing.value = true
  try {
    analysis.value = await api.post<Analysis>('/reqlang/analyze', { text, language: analysisLanguage.value })
  } catch {
    analysis.value = null
  } finally {
    analyzing.value = false
  }
}, 450)
watch(
  () => form.value.description,
  (t) => runAnalysis(t ?? ''),
)
watch(
  () => item.value?.id,
  () => runAnalysis.flush(),
)
onBeforeUnmount(() => runAnalysis.cancel())
watch(() => props.itemId, load, { immediate: true })

function strength(level: string): string {
  if (/^(must|shall)/.test(level)) return 'kw-strong'
  if (/^should/.test(level)) return 'kw-medium'
  return 'kw-weak'
}
const highlighted = computed(() => {
  const a = analysis.value
  const text = form.value.description
  if (!a || !text) return []
  return segments(
    text,
    [
      ...(a.keywords ?? []).map((k) => ({ offset: k.offset, length: k.length, kind: strength(k.level), title: [obligationKeyword(k.level), obligationHint(k.level)].filter(Boolean).join(' – ') })),
      ...(a.issues ?? []).map((i) => ({ offset: i.offset, length: i.length, kind: 'kw-issue', title: issueText(i) })),
    ],
  )
})
const issues = computed(() => {
  const text = form.value.description
  return (analysis.value?.issues ?? []).map((i) => {
    const seg = segments(text, [{ offset: i.offset, length: i.length }]).find((s) => s.mark)
    return { ...i, fragment: seg?.text ?? '', text: issueText(i) }
  })
})
/** Issue message in the active language (known rules), else the API's English text. */
function issueText(i: { rule: string; message: string; offset: number; length: number; params?: Record<string, string> }): string {
  return issueMessage(i, form.value.description ?? '', (key, params) => t(key, params))
}
const noKeywordExamples = computed(() => analysis.value?.issues?.find((i) => i.rule === 'missing-keyword')?.params?.examples ?? 'MUST, SHALL, SHOULD, MAY')
const detectedDiffers = computed(() => !!analysis.value?.obligation && analysis.value.obligation !== form.value.obligation)
function applyDetected() {
  if (analysis.value?.obligation) form.value.obligation = analysis.value.obligation
}
const scoreSeverity = computed(() => {
  const s = analysis.value?.score ?? 0
  return s >= 80 ? 'success' : s >= 50 ? 'warn' : 'danger'
})

// ---------- criteria ----------
function addCriterion() {
  form.value.criteria.push({ description: '', method: meta.meta.evaluation_methods[0] ?? 'test', threshold: '' })
}
function removeCriterion(i: number) {
  form.value.criteria.splice(i, 1)
}
function moveCriterion(i: number, d: number) {
  form.value.criteria = moveItem(form.value.criteria, i, i + d)
}

// ---------- save / delete ----------
/** the PATCH body of a form */
function bodyOf(f: typeof form.value) {
  return {
    ...f,
    criteria: f.criteria
      .filter((c) => c.description.trim())
      .map((c, i): Partial<EvaluationCriterion> => ({ ...(c.id ? { id: c.id } : {}), description: c.description, method: c.method, threshold: c.threshold, position: i + 1 })),
  }
}
/** the body as loaded, for undoing an edit */
const loaded = ref<Record<string, unknown> | null>(null)
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => {
    // reload after an undo/redo unless there are unsaved changes
    if (props.itemId && JSON.stringify(bodyOf(form.value)) === JSON.stringify(loaded.value)) void load()
  },
)
function delRequirement(id: string) {
  return () => api.delTrash(`/requirements/${id}`)
}

// ---------- someone saved in between ----------
const conflictOpen = ref(false)
const conflictList = ref<FieldConflict[]>([])
const stale = ref<Stale<Requirement> | null>(null)
function showConflict(st: Stale<Requirement>) {
  stale.value = st
  conflictList.value = item.value ? fieldConflicts(REQUIREMENT_FIELDS.fields, item.value, st.current, form.value, REQUIREMENT_FIELDS.normalize) : []
  conflictOpen.value = true
}
async function reloadTheirs() {
  conflictOpen.value = false
  await load()
}
/** save only what was changed here, on top of the newer version */
async function mergeMine() {
  const st = stale.value
  if (!st || !loaded.value) return
  saving.value = true
  const body = bodyOf(form.value)
  const keys = changedKeys(loaded.value, body, Object.keys(body))
  try {
    const saved = await api.patch<Requirement>(`/requirements/${props.itemId}`, pickBefore(body, keys), { ifMatch: st.current.version })
    conflictOpen.value = false
    notify.ok(t('conflict.merged'), saved?.key)
    if (saved?.id && keys.length) recordUndo(patchEntry(t('undo.a.edit', { key: saved.key }), `/requirements/${saved.id}`, pickBefore(st.current, keys), pickBefore(body, keys)))
    emit('saved', saved)
    await load()
  } catch (e) {
    const again = staleOf<Requirement>(e)
    if (again) showConflict(again)
    else error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

async function save() {
  saving.value = true
  error.value = ''
  const body = bodyOf(form.value)
  const before = loaded.value
  try {
    const saved = isNew.value
      ? await api.post<Requirement & { linked_test?: TestCase }>(`/projects/${props.projectId}/requirements`, { ...body, create_test: createTest.value })
      : await api.patch<Requirement>(`/requirements/${props.itemId}`, body, { ifMatch: item.value?.version })
    const linked = isNew.value ? (saved as Requirement & { linked_test?: TestCase })?.linked_test : undefined
    notify.ok(
      isNew.value ? t('requirements.created') : t('requirements.saved'),
      linked ? t('requirements.withLinkedTest', { key: saved?.key ?? '', test: linked.key }) : saved?.key,
    )
    if (saved?.id) {
      if (isNew.value) recordUndo(createdEntry(t('undo.a.create', { key: saved.key }), delRequirement(saved.id)))
      else if (before) {
        const keys = changedKeys(before, body, Object.keys(body))
        if (keys.length) recordUndo(patchEntry(t('undo.a.edit', { key: saved.key }), `/requirements/${saved.id}`, pickBefore(before, keys), pickBefore(body, keys)))
      }
    }
    emit('saved', saved)
    if (!isNew.value) await load() // refresh version and counts
  } catch (e) {
    const st = staleOf<Requirement>(e)
    if (st) showConflict(st)
    else error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

function remove() {
  if (!item.value) return
  const it = item.value
  notify.confirmDanger(t('requirements.confirmDelete', { key: it.key }), async () => {
    try {
      const trash = await api.delTrash(`/requirements/${it.id}`)
      if (trash) recordUndo(deletedEntry(t('undo.a.delete', { key: it.key }), trash, delRequirement(it.id)))
      notify.ok(t('requirements.deleted'), t('undo.hint'))
      emit('deleted', it.id)
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- links ----------
function linkEntry(created: boolean, id: string, body: Record<string, unknown>, testKey: string) {
  const pid = props.projectId
  const label = t(created ? 'undo.a.link' : 'undo.a.unlink', { from: item.value?.key ?? '', to: testKey })
  return toggleEntry(
    label,
    created,
    id,
    async () => (await api.post<TraceLink>(`/projects/${pid}/links`, body)).id,
    async (x) => void (await api.del(`/links/${x}`)),
  )
}
const testById = computed(() => new Map(tests.value.map((t) => [t.id, t])))
async function addLinks(picked: { id: string }[]) {
  try {
    for (const tc of picked) {
      const body = { requirement_id: props.itemId, test_case_id: tc.id, ...(linkCriterion.value ? { criterion_id: linkCriterion.value } : {}) }
      const l = await api.post<TraceLink>(`/projects/${props.projectId}/links`, body)
      if (l?.id) recordUndo(linkEntry(true, l.id, body, (tc as { key?: string }).key ?? ''))
    }
    tests.value = []
    await loadLinks()
    notify.ok(t('editor.linksAdded', picked.length))
  } catch (e) {
    notify.fail(e, t('editor.addLinkFailed'))
  }
}
async function removeLink(l: TraceLink) {
  try {
    await api.del(`/links/${l.id}`)
    recordUndo(
      linkEntry(false, l.id, { requirement_id: l.requirement_id, test_case_id: l.test_case_id, ...(l.criterion_id ? { criterion_id: l.criterion_id } : {}) }, testById.value.get(l.test_case_id)?.key ?? l.test_case_key ?? ''),
    )
    links.value = links.value.filter((x) => x.id !== l.id)
  } catch (e) {
    notify.fail(e)
  }
}
const criterionOptions = computed(() => [
  { label: t('requirements.wholeRequirement'), value: null as string | null },
  ...(item.value?.criteria ?? []).map((c, i) => ({ label: `#${i + 1} ${c.description}`.slice(0, 60), value: c.id as string | null })),
])
function criterionLabel(id: string | null) {
  if (!id) return ''
  const i = (item.value?.criteria ?? []).findIndex((c) => c.id === id)
  return i >= 0 ? t('requirements.criterionNumber', { n: i + 1 }) : t('requirements.criterionGeneric')
}
</script>

<template>
  <div v-if="loading" class="empty" aria-busy="true"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
  <template v-else>
  <Tabs v-if="item" :id="tabsId" v-model:value="tab" class="editor-tabs" scrollable>
    <TabList>
      <Tab value="details"><i class="pi pi-file-edit" aria-hidden="true" /> {{ t('common.details') }}</Tab>
      <Tab value="relations"><i class="pi pi-sitemap" aria-hidden="true" /> {{ t('common.relations') }} <span class="count">{{ relationCount }}</span></Tab>
      <Tab value="links"><i class="pi pi-link" aria-hidden="true" /> {{ t('links.title') }} <span v-if="linkCount !== null" class="count">{{ linkCount }}</span></Tab>
      <Tab value="defects"><i class="pi pi-exclamation-circle" aria-hidden="true" /> {{ t('defectsView.tab') }} <span v-if="openDefects" class="count" :title="t('defectsView.openCount', { n: openDefects }, openDefects)">{{ openDefects }}</span></Tab>
      <Tab value="comments"><i class="pi pi-comments" aria-hidden="true" /> {{ t('common.comments') }} <span class="count">{{ commentCount }}</span></Tab>
      <Tab value="attachments"><i class="pi pi-paperclip" aria-hidden="true" /> {{ t('files.tab') }} <span v-if="attachmentCount !== null" class="count">{{ attachmentCount }}</span></Tab>
      <Tab value="history"><i class="pi pi-history" aria-hidden="true" /> {{ t('common.history') }}</Tab>
    </TabList>
  </Tabs>
  <div v-if="item && tab === 'defects'" v-bind="panel('defects')"><DefectsPanel entity-type="requirement" :entity-id="item.id" :project-id="projectId" @count="(n) => (openDefects = n)" /></div>
  <div v-if="item && tab === 'relations'" v-bind="panel('relations')"><RelationsPanel :project-id="projectId" :requirement-id="item.id" :can-write="canWrite" @count="(n) => (relationCount = n)" /></div>
  <div v-if="item && tab === 'links'" v-bind="panel('links')"><ItemLinksPanel item-type="requirement" :item-id="item.id" :project-id="projectId" :can-write="canWrite" @count="(n) => (linkCount = n)" /></div>
  <div v-if="item && tab === 'comments'" v-bind="panel('comments')"><CommentsPanel entity="requirement" :entity-id="item.id" @count="(n) => (commentCount = n)" /></div>
  <div v-if="item && tab === 'attachments'" v-bind="panel('attachments')"><AttachmentsPanel type="requirement" :entity-id="item.id" :project-id="projectId" :can-write="canWrite" @count="(n) => (attachmentCount = n)" /></div>
  <div v-if="item && tab === 'history'" v-bind="panel('history')">
  <HistoryPanel
    entity="requirement"
    :entity-id="item.id"
    :can-restore="canWrite"
    :current-version="item.version"
    :resolve="resolveId"
    @restored="onRestored"
  />
  </div>
  <form v-show="!item || tab === 'details'" v-bind="panel('details')" class="grid" style="gap: 14px" @submit.prevent="save">
    <div class="row">
      <span v-if="item" class="key" style="font-size: 14px">{{ item.key }}</span>
      <Tag v-if="item" :value="t('common.versionShort', { version: item.version })" severity="secondary" />
      <StatusTag v-if="item?.coverage" :value="item.coverage" kind="coverage" />
      <button v-if="item && openDefects" type="button" class="open-defects-badge" @click="tab = 'defects'">
        <i class="pi pi-exclamation-circle" aria-hidden="true" />{{ t('defectsView.openCount', { n: openDefects }, openDefects) }}
      </button>
      <span v-if="item" class="muted small">{{ t('editor.updatedBy', { date: formatDateTime(item.updated_at) }) }}{{ item.created_by ? t('editor.createdBy', { user: item.created_by }) : '' }}</span>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <ReviewBar v-if="item" kind="requirement" :item="item" @changed="onReviewed" />

    <div v-field class="field">
      <label>{{ t('common.title') }}</label>
      <InputText v-model="form.title" :disabled="!canWrite" required autofocus />
    </div>
    <div v-field class="field">
      <label>{{ t('common.description') }}</label>
      <MentionInput v-model="form.description" multiline rows="5" auto-resize :project-id="projectId" :disabled="!canWrite" :placeholder="t('requirements.descriptionPlaceholder')" />
      <ObligationHelp v-if="canWrite" :open="isNew" />
    </div>

    <div v-if="form.description.trim() && !prefs.hideAnalysis" class="analysis" role="region" :aria-label="t('analysis.title')" :aria-busy="analyzing">
      <div class="row" style="justify-content: space-between; margin-bottom: 8px">
        <span class="row">
          <i class="pi pi-sparkles" style="color: var(--p-primary-color)" />
          <b class="small">{{ t('analysis.title') }}</b>
          <HelpTip id="req.analysis" :label="t('analysis.title')" />
          <ProgressSpinner v-if="analyzing" style="width: 14px; height: 14px" stroke-width="6" :aria-label="t('common.loading')" />
          <Select
            v-model="analysisLanguage"
            :options="languageOptions"
            option-label="label"
            option-value="value"
            size="small"
            :aria-label="t('analysis.language')"
            :title="t('analysis.language')"
          />
          <span v-if="analysis?.language && analysisLanguage === 'auto'" class="small muted">{{ t('analysis.detectedLanguage', { language: languageLabel(analysis.language) }) }}</span>
        </span>
        <span v-if="analysis" class="row" aria-live="polite" aria-atomic="true">
          <span class="small muted">{{ t('analysis.detected') }}</span>
          <StatusTag :value="analysis.obligation || 'describe'" kind="obligation" />
          <Tag :value="t('analysis.score', { score: analysis.score })" :severity="scoreSeverity" />
          <Button v-if="detectedDiffers && canWrite" size="small" :label="t('analysis.applyDetected')" icon="pi pi-check" text @click="applyDetected" />
        </span>
      </div>
      <div v-if="analysis" class="text">
        <template v-for="(s, i) in highlighted" :key="i">
          <span v-if="s.mark" :class="['kw', s.kind]" :title="s.title">{{ s.text }}</span>
          <template v-else>{{ s.text }}</template>
        </template>
      </div>
      <div v-if="!analysis?.keywords?.length && analysis" class="small muted" style="margin-top: 6px">
        {{ t('analysis.noKeyword', { examples: noKeywordExamples }) }}
      </div>
      <ul v-if="issues.length" style="margin: 8px 0 0; padding-left: 18px" aria-live="polite" :aria-label="t('analysis.issues')">
        <li v-for="(i, n) in issues" :key="n" class="small">
          <i class="pi" :class="i.severity === 'error' ? 'pi-times-circle' : 'pi-exclamation-triangle'" :style="{ color: i.severity === 'error' ? 'var(--p-red-500)' : 'var(--p-amber-500)' }" aria-hidden="true" />
          <span class="sr-only">{{ i.severity === 'error' ? t('analysis.error') : t('analysis.warning') }}:</span>
          <b v-if="i.fragment"> “{{ i.fragment }}”</b> {{ i.text }} <span class="muted">({{ i.rule }})</span>
        </li>
      </ul>
    </div>

    <div class="form-grid">
      <div v-field class="field">
        <label>{{ t('requirements.obligation') }}</label>
        <HelpTip id="req.obligation" :label="t('requirements.obligation')" />
        <Select v-model="form.obligation" :options="obligationOptions" option-label="label" option-value="value" :disabled="!canWrite">
          <template #value="{ value }">
            <StatusTag v-if="value" :value="value" kind="obligation" />
            <span v-else class="muted">{{ t('requirements.autoDetect') }}</span>
          </template>
        </Select>
      </div>
      <div v-field class="field">
        <label>{{ t('common.status') }}</label>
        <HelpTip id="req.status" :label="t('common.status')" />
        <Select v-model="form.status" :options="statusOptions" option-label="label" option-value="value" option-disabled="disabled" :disabled="!canWrite" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.type') }}</label>
        <HelpTip id="req.type" :label="t('common.type')" />
        <Select v-model="form.type" :options="typeOptions" option-label="label" option-value="value" :disabled="!canWrite" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.priority') }}</label>
        <HelpTip id="req.priority" :label="t('common.priority')" />
        <Select v-model="form.priority" :options="priorityOptions" option-label="label" option-value="value" :disabled="!canWrite" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.owner') }}</label>
        <HelpTip id="req.owner" :label="t('common.owner')" />
        <MentionInput v-model="form.owner" :project-id="projectId" :disabled="!canWrite" :placeholder="t('requirements.ownerPlaceholder', { at: '@' })" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.source') }}</label>
        <HelpTip id="req.source" :label="t('common.source')" />
        <AutoComplete
          v-model="form.source"
          :suggestions="sourceSuggestions"
          dropdown
          dropdown-mode="blank"
          complete-on-focus
          :disabled="!canWrite"
          :placeholder="t('requirements.sourcePlaceholder')"
          :dropdown-icon="'pi pi-chevron-down'"
          :pt="{ dropdown: { 'aria-label': t('requirements.sourceList') } }"
          fluid
          @complete="completeSource"
        />
      </div>
      <div v-field class="field full">
        <label>{{ t('requirements.rationale') }}</label>
        <HelpTip id="req.rationale" :label="t('requirements.rationale')" />
        <MentionInput v-model="form.rationale" multiline rows="2" auto-resize :project-id="projectId" :disabled="!canWrite" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.folder') }}</label>
        <HelpTip id="req.folder" :label="t('common.folder')" />
        <Select v-model="form.folder_id" :options="folderOptions" option-label="label" option-value="id" filter :disabled="!canWrite" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.categories') }}</label>
        <HelpTip id="req.categories" :label="t('common.categories')" />
        <CategorySelect v-model="form.category_ids" :categories="categories" :disabled="!canWrite" />
      </div>
      <div v-field class="field full">
        <label>{{ t('common.tags') }}</label>
        <HelpTip id="req.tags" :label="t('common.tags')" />
        <TagsInput v-model="form.tags" :disabled="!canWrite" />
        <TagSuggestions v-model="form.tags" :project-id="projectId" :title="form.title" :description="form.description" :disabled="!canWrite" />
      </div>
    </div>

    <div>
      <div class="section-title">
        <i class="pi pi-verified" /> {{ t('requirements.criteria') }}
        <HelpTip id="req.criteria" :label="t('requirements.criteria')" />
        <span class="grow" />
        <Button v-if="canWrite" size="small" :label="t('requirements.addCriterion')" icon="pi pi-plus" text @click="addCriterion" />
      </div>
      <div v-if="!form.criteria.length" class="muted small">{{ t('requirements.noCriteria') }}</div>
      <div v-for="(c, i) in form.criteria" :key="i" class="criterion-row">
        <span class="step-dot" style="background: var(--p-primary-color); margin-top: 8px" aria-hidden="true">{{ i + 1 }}</span>
        <div class="grow criterion-fields">
          <Textarea v-model="c.description" rows="1" auto-resize :placeholder="t('requirements.criterion')" :aria-label="t('requirements.criterionN', { n: i + 1 })" :disabled="!canWrite" />
          <Select v-model="c.method" :options="methodOptions" option-label="label" option-value="value" :aria-label="t('requirements.criterionMethodN', { n: i + 1 })" :disabled="!canWrite" />
          <InputText v-model="c.threshold" :placeholder="t('requirements.thresholdPlaceholder')" :aria-label="t('requirements.criterionThresholdN', { n: i + 1 })" :disabled="!canWrite" />
        </div>
        <div v-if="canWrite" class="row nowrap criterion-actions">
          <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :disabled="i === 0" :aria-label="t('requirements.moveCriterionUp', { n: i + 1 })" @click="moveCriterion(i, -1)" />
          <Button icon="pi pi-arrow-down" text rounded size="small" severity="secondary" :disabled="i === form.criteria.length - 1" :aria-label="t('requirements.moveCriterionDown', { n: i + 1 })" @click="moveCriterion(i, 1)" />
          <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('requirements.removeCriterion', { n: i + 1 })" @click="removeCriterion(i)" />
        </div>
      </div>
    </div>

    <div v-if="item">
      <div class="section-title">
        <i class="pi pi-link" /> {{ t('requirements.linkedTests', { count: links.length }) }}
        <span class="grow" />
        <template v-if="canLink">
          <Select v-if="(item.criteria ?? []).length" v-model="linkCriterion" :options="criterionOptions" option-label="label" option-value="value" size="small" style="max-width: 220px" :aria-label="t('requirements.linkCriterion')" />
          <Button v-if="suspectCount" size="small" :label="t('suspect.confirmAll', { n: suspectCount })" icon="pi pi-check" text @click="confirmAll" />
          <Button size="small" :label="t('requirements.linkTests')" icon="pi pi-plus" text @click="pickTests = true" />
        </template>
      </div>
      <div v-if="!links.length" class="muted small">{{ t('requirements.notCovered') }}</div>
      <div v-for="l in links" :key="l.id" class="row nowrap" style="padding: 5px 0; border-bottom: 1px solid var(--qtf-border)">
        <RouterLink :to="{ name: 'tests', params: { projectId }, query: { item: l.test_case_id } }" class="key">{{ l.test_case_key || testById.get(l.test_case_id)?.key }}</RouterLink>
        <span class="grow ellipsis">{{ testById.get(l.test_case_id)?.title }}</span>
        <span v-if="l.criterion_id" class="small muted">{{ criterionLabel(l.criterion_id) }}</span>
        <SuspectLink :link="l" :can-confirm="canLink" @confirmed="onSuspectConfirmed" />
        <StatusTag :value="testById.get(l.test_case_id)?.last_result" kind="result" empty="" />
        <Button v-if="canLink" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('common.unlinkItem', { key: l.test_case_key || testById.get(l.test_case_id)?.key || '' })" v-tooltip="t('common.unlink')" @click="removeLink(l)" />
      </div>
    </div>

    <RequirementRisks v-if="item" :requirement-id="item.id" :project-id="projectId" />

    <div class="row editor-footer">
      <Button v-if="item && canWrite" :label="t('common.delete')" icon="pi pi-trash" severity="danger" text @click="remove" />
      <SplitButton
        v-if="item && canWrite"
        :label="t('common.copy')"
        icon="pi pi-copy"
        severity="secondary"
        text
        :model="copyItems"
        :button-props="{ 'aria-label': t('requirements.copyHint') }"
        :menu-button-props="{ 'aria-label': t('requirements.copyMore') }"
        style="margin-right: auto"
        @click="copy()"
      />
      <span v-else style="margin-right: auto" />
      <RouterLink v-if="item && isSoftware(proj.project)" :to="{ name: 'prompts', params: { projectId }, query: { kind: 'implement', ids: item.id } }" class="small row nowrap" style="gap: 4px"><i class="pi pi-sparkles" aria-hidden="true" />{{ t('prompts.forItem') }}</RouterLink>
      <label v-if="isNew && canWrite && canLink" class="row nowrap small" style="gap: 6px" :title="t('editor.alsoCreateTestHint')">
        <Checkbox v-model="createTest" binary input-id="create-test" />
        {{ t('editor.alsoCreateTest') }}
      </label>
      <HelpTip v-if="isNew && canWrite && canLink" id="req.linkedTest" :label="t('editor.alsoCreateTest')" />
      <Button :label="t('common.close')" severity="secondary" text @click="emit('close')" />
      <Button v-if="canWrite" type="submit" :label="isNew ? t('common.create') : t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!form.title.trim()" />
    </div>
    <ItemPickerDialog
      v-if="item"
      v-model:visible="pickTests"
      kind="test"
      :project-id="projectId"
      :exclude="links.map((l) => l.test_case_id)"
      :header="t('requirements.linkTests')"
      :confirm-label="t('common.link')"
      @pick="addLinks"
    />
  </form>
  </template>
  <ConflictDialog v-model:visible="conflictOpen" :conflicts="conflictList" :changed-by="stale?.changedBy ?? ''" :changed-at="stale?.changedAt ?? ''" :saving="saving" @reload="reloadTheirs" @merge="mergeMine" />
</template>
