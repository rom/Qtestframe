<script setup lang="ts">
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
import { enumOptions, obligationHint, obligationKeyword } from '@/i18n/labels'
import { issueMessage } from '@/i18n/analysis'
import { buildTree, flatten, folderPath } from '@/lib/tree'
import type { Analysis, Category, EvaluationCriterion, Folder, Requirement, TestCase, TraceLink } from '@/lib/types'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'

const props = defineProps<{
  projectId: string
  itemId: string | null
  folderId?: string | null
  categories: Category[]
  folders: Folder[]
  canWrite: boolean
  canLink: boolean
}>()
const emit = defineEmits<{ saved: [item: Requirement]; deleted: [id: string]; close: [] }>()
const meta = useMetaStore()
const notify = useNotify()
const prefs = usePrefsStore()
const { t } = useI18n()

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
const relationCount = ref(0)
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
const statusOptions = computed(() => enumOptions('status', meta.meta.requirement_statuses))
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
    relationCount.value = r.relation_count ?? 0
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
    await loadLinks()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
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
const runAnalysis = debounce(async (text: string) => {
  if (!text.trim() || prefs.hideAnalysis) {
    analysis.value = null
    return
  }
  analyzing.value = true
  try {
    analysis.value = await api.post<Analysis>('/reqlang/analyze', { text })
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
function issueText(i: { rule: string; message: string; offset: number; length: number }): string {
  return issueMessage(i, form.value.description ?? '', (key, params) => t(key, params))
}
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
async function save() {
  saving.value = true
  error.value = ''
  const f = form.value
  const body = {
    ...f,
    criteria: f.criteria
      .filter((c) => c.description.trim())
      .map((c, i): Partial<EvaluationCriterion> => ({ ...(c.id ? { id: c.id } : {}), description: c.description, method: c.method, threshold: c.threshold, position: i + 1 })),
  }
  try {
    const saved = isNew.value
      ? await api.post<Requirement & { linked_test?: TestCase }>(`/projects/${props.projectId}/requirements`, { ...body, create_test: createTest.value })
      : await api.patch<Requirement>(`/requirements/${props.itemId}`, body)
    const linked = isNew.value ? (saved as Requirement & { linked_test?: TestCase })?.linked_test : undefined
    notify.ok(
      isNew.value ? t('requirements.created') : t('requirements.saved'),
      linked ? t('requirements.withLinkedTest', { key: saved?.key ?? '', test: linked.key }) : saved?.key,
    )
    emit('saved', saved)
    if (!isNew.value) await load() // refresh version and counts
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

function remove() {
  if (!item.value) return
  const it = item.value
  notify.confirmDanger(t('requirements.confirmDelete', { key: it.key }), async () => {
    try {
      await api.del(`/requirements/${it.id}`)
      notify.ok(t('requirements.deleted'))
      emit('deleted', it.id)
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- links ----------
const testById = computed(() => new Map(tests.value.map((t) => [t.id, t])))
async function addLinks(picked: { id: string }[]) {
  try {
    for (const t of picked) {
      await api.post(`/projects/${props.projectId}/links`, {
        requirement_id: props.itemId,
        test_case_id: t.id,
        ...(linkCriterion.value ? { criterion_id: linkCriterion.value } : {}),
      })
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
  <div v-if="loading" class="empty"><ProgressSpinner style="width: 36px; height: 36px" /></div>
  <template v-else>
  <Tabs v-if="item" v-model:value="tab" class="editor-tabs">
    <TabList>
      <Tab value="details"><i class="pi pi-file-edit" /> {{ t('common.details') }}</Tab>
      <Tab value="relations"><i class="pi pi-sitemap" /> {{ t('common.relations') }} <span class="count">{{ relationCount }}</span></Tab>
      <Tab value="comments"><i class="pi pi-comments" /> {{ t('common.comments') }} <span class="count">{{ commentCount }}</span></Tab>
      <Tab value="history"><i class="pi pi-history" /> {{ t('common.history') }}</Tab>
    </TabList>
  </Tabs>
  <RelationsPanel v-if="item && tab === 'relations'" :project-id="projectId" :requirement-id="item.id" :can-write="canWrite" @count="(n) => (relationCount = n)" />
  <CommentsPanel v-if="item && tab === 'comments'" entity="requirement" :entity-id="item.id" @count="(n) => (commentCount = n)" />
  <HistoryPanel
    v-if="item && tab === 'history'"
    entity="requirement"
    :entity-id="item.id"
    :can-restore="canWrite"
    :current-version="item.version"
    :resolve="resolveId"
    @restored="onRestored"
  />
  <form v-show="!item || tab === 'details'" class="grid" style="gap: 14px" @submit.prevent="save">
    <div class="row">
      <span v-if="item" class="key" style="font-size: 14px">{{ item.key }}</span>
      <Tag v-if="item" :value="t('common.versionShort', { version: item.version })" severity="secondary" />
      <StatusTag v-if="item?.coverage" :value="item.coverage" kind="coverage" />
      <span v-if="item" class="muted small">{{ t('editor.updatedBy', { date: formatDateTime(item.updated_at) }) }}{{ item.created_by ? t('editor.createdBy', { user: item.created_by }) : '' }}</span>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

    <div class="field">
      <label>{{ t('common.title') }}</label>
      <InputText v-model="form.title" :disabled="!canWrite" required autofocus />
    </div>
    <div class="field">
      <label>{{ t('common.description') }}</label>
      <Textarea v-model="form.description" rows="5" auto-resize :disabled="!canWrite" :placeholder="t('requirements.descriptionPlaceholder')" />
    </div>

    <div v-if="form.description.trim() && !prefs.hideAnalysis" class="analysis">
      <div class="row" style="justify-content: space-between; margin-bottom: 8px">
        <span class="row">
          <i class="pi pi-sparkles" style="color: var(--p-primary-color)" />
          <b class="small">{{ t('analysis.title') }}</b>
          <ProgressSpinner v-if="analyzing" style="width: 14px; height: 14px" stroke-width="6" />
        </span>
        <span v-if="analysis" class="row">
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
        {{ t('analysis.noKeyword') }}
      </div>
      <ul v-if="issues.length" style="margin: 8px 0 0; padding-left: 18px">
        <li v-for="(i, n) in issues" :key="n" class="small">
          <i class="pi" :class="i.severity === 'error' ? 'pi-times-circle' : 'pi-exclamation-triangle'" :style="{ color: i.severity === 'error' ? 'var(--p-red-500)' : 'var(--p-amber-500)' }" />
          <b v-if="i.fragment"> “{{ i.fragment }}”</b> {{ i.text }} <span class="muted">({{ i.rule }})</span>
        </li>
      </ul>
    </div>

    <div class="form-grid">
      <div class="field">
        <label>{{ t('requirements.obligation') }}</label>
        <Select v-model="form.obligation" :options="obligationOptions" option-label="label" option-value="value" :disabled="!canWrite">
          <template #value="{ value }">
            <StatusTag v-if="value" :value="value" kind="obligation" />
            <span v-else class="muted">{{ t('requirements.autoDetect') }}</span>
          </template>
        </Select>
      </div>
      <div class="field">
        <label>{{ t('common.status') }}</label>
        <Select v-model="form.status" :options="statusOptions" option-label="label" option-value="value" :disabled="!canWrite" />
      </div>
      <div class="field">
        <label>{{ t('common.type') }}</label>
        <Select v-model="form.type" :options="typeOptions" option-label="label" option-value="value" :disabled="!canWrite" />
      </div>
      <div class="field">
        <label>{{ t('common.priority') }}</label>
        <Select v-model="form.priority" :options="priorityOptions" option-label="label" option-value="value" :disabled="!canWrite" />
      </div>
      <div class="field">
        <label>{{ t('common.owner') }}</label>
        <InputText v-model="form.owner" :disabled="!canWrite" />
      </div>
      <div class="field">
        <label>{{ t('common.source') }}</label>
        <InputText v-model="form.source" :disabled="!canWrite" :placeholder="t('requirements.sourcePlaceholder')" />
      </div>
      <div class="field full">
        <label>{{ t('requirements.rationale') }}</label>
        <Textarea v-model="form.rationale" rows="2" auto-resize :disabled="!canWrite" />
      </div>
      <div class="field">
        <label>{{ t('common.folder') }}</label>
        <Select v-model="form.folder_id" :options="folderOptions" option-label="label" option-value="id" filter :disabled="!canWrite" />
      </div>
      <div class="field">
        <label>{{ t('common.categories') }}</label>
        <CategorySelect v-model="form.category_ids" :categories="categories" :disabled="!canWrite" />
      </div>
      <div class="field full">
        <label>{{ t('common.tags') }}</label>
        <TagsInput v-model="form.tags" :disabled="!canWrite" />
      </div>
    </div>

    <div>
      <div class="section-title">
        <i class="pi pi-verified" /> {{ t('requirements.criteria') }}
        <span class="grow" />
        <Button v-if="canWrite" size="small" :label="t('requirements.addCriterion')" icon="pi pi-plus" text @click="addCriterion" />
      </div>
      <div v-if="!form.criteria.length" class="muted small">{{ t('requirements.noCriteria') }}</div>
      <div v-for="(c, i) in form.criteria" :key="i" class="row nowrap" style="align-items: flex-start; margin-bottom: 8px">
        <span class="step-dot" style="background: var(--p-primary-color); margin-top: 8px">{{ i + 1 }}</span>
        <div class="grow grid" style="gap: 6px; grid-template-columns: minmax(0, 1fr) 150px 170px">
          <Textarea v-model="c.description" rows="1" auto-resize :placeholder="t('requirements.criterion')" :aria-label="t('requirements.criterion')" :disabled="!canWrite" />
          <Select v-model="c.method" :options="methodOptions" option-label="label" option-value="value" :disabled="!canWrite" />
          <InputText v-model="c.threshold" :placeholder="t('requirements.thresholdPlaceholder')" :disabled="!canWrite" />
        </div>
        <div v-if="canWrite" class="row nowrap">
          <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :disabled="i === 0" :aria-label="t('common.moveUp')" @click="moveCriterion(i, -1)" />
          <Button icon="pi pi-arrow-down" text rounded size="small" severity="secondary" :disabled="i === form.criteria.length - 1" :aria-label="t('common.moveDown')" @click="moveCriterion(i, 1)" />
          <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('common.remove')" @click="removeCriterion(i)" />
        </div>
      </div>
    </div>

    <div v-if="item">
      <div class="section-title">
        <i class="pi pi-link" /> {{ t('requirements.linkedTests', { count: links.length }) }}
        <span class="grow" />
        <template v-if="canLink">
          <Select v-if="(item.criteria ?? []).length" v-model="linkCriterion" :options="criterionOptions" option-label="label" option-value="value" size="small" style="max-width: 220px" />
          <Button size="small" :label="t('requirements.linkTests')" icon="pi pi-plus" text @click="pickTests = true" />
        </template>
      </div>
      <div v-if="!links.length" class="muted small">{{ t('requirements.notCovered') }}</div>
      <div v-for="l in links" :key="l.id" class="row nowrap" style="padding: 5px 0; border-bottom: 1px solid var(--qtf-border)">
        <RouterLink :to="{ name: 'tests', params: { projectId }, query: { item: l.test_case_id } }" class="key">{{ l.test_case_key || testById.get(l.test_case_id)?.key }}</RouterLink>
        <span class="grow ellipsis">{{ testById.get(l.test_case_id)?.title }}</span>
        <span v-if="l.criterion_id" class="small muted">{{ criterionLabel(l.criterion_id) }}</span>
        <StatusTag :value="testById.get(l.test_case_id)?.last_result" kind="result" empty="" />
        <Button v-if="canLink" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('common.unlink')" v-tooltip="t('common.unlink')" @click="removeLink(l)" />
      </div>
    </div>

    <div class="row" style="justify-content: flex-end; position: sticky; bottom: -1px; background: var(--p-drawer-background, var(--qtf-card)); padding: 10px 0; border-top: 1px solid var(--qtf-border)">
      <Button v-if="item && canWrite" :label="t('common.delete')" icon="pi pi-trash" severity="danger" text style="margin-right: auto" @click="remove" />
      <label v-if="isNew && canWrite && canLink" class="row nowrap small" style="gap: 6px" :title="t('editor.alsoCreateTestHint')">
        <Checkbox v-model="createTest" binary input-id="create-test" />
        {{ t('editor.alsoCreateTest') }}
      </label>
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
</template>
