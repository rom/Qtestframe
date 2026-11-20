<script setup lang="ts">
import SuspectLink from './SuspectLink.vue'
import ItemLinksPanel from '@/components/ItemLinksPanel.vue'
import { isSoftware } from '@/lib/projectTypes'
import { useProjectStore } from '@/stores/project'
import { useUndoStore } from '@/stores/undo'
import { changedKeys, clone, pickBefore } from '@/lib/undo'
import { createdEntry, deletedEntry, patchEntry, recordUndo, toggleEntry } from '@/lib/undoActions'
import ReviewBar from './ReviewBar.vue'
import { P } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth'
import TagSuggestions from './TagSuggestions.vue'
import MentionInput from './MentionInput.vue'
import HelpTip from '@/guide/HelpTip.vue'
import { computed, ref, watch } from 'vue'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import StatusTag from './StatusTag.vue'
import TagsInput from './TagsInput.vue'
import CategorySelect from './CategorySelect.vue'
import ItemPickerDialog from './ItemPickerDialog.vue'
import TestDependenciesPanel from './TestDependenciesPanel.vue'
import CommentsPanel from './CommentsPanel.vue'
import AttachmentsPanel from './AttachmentsPanel.vue'
import HistoryPanel from './HistoryPanel.vue'
import TestExecutionsPanel from './TestExecutionsPanel.vue'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import Tag from 'primevue/tag'
import { api, asItems, errorMessage } from '@/lib/api'
import { moveItem } from '@/lib/collections'
import { formatDateTime } from '@/lib/format'
import { useI18n } from 'vue-i18n'
import { uid as newId } from '@/lib/a11y'
import { enumLabel, enumOptions } from '@/i18n/labels'
import { buildTree, flatten, folderPath } from '@/lib/tree'
import type { Category, Folder, Requirement, TestCase, TraceLink } from '@/lib/types'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'

const props = defineProps<{
  projectId: string
  itemId: string | null
  folderId?: string | null
  categories: Category[]
  folders: Folder[]
  canWrite: boolean
  canLink: boolean
}>()
const emit = defineEmits<{ saved: [item: TestCase]; deleted: [id: string]; copied: [item: TestCase]; close: [] }>()
const meta = useMetaStore()
const proj = useProjectStore()
const notify = useNotify()
const { t } = useI18n()
const tabsId = newId('test-tabs')
const linkCount = ref<number | null>(null)
watch(
  () => props.itemId,
  () => (linkCount.value = null),
)
/** tab panel attributes for the content of a tab (only when the tabs are shown) */
function panel(value: string) {
  return item.value ? { role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` } : {}
}
const auth = useAuthStore()
/** review decisions need the review permission (the current status stays selectable) */
const REVIEW_DECISIONS = ['ready']
const canReview = computed(() => auth.can(P.testReview))
const statusOptions = computed(() =>
  enumOptions('status', meta.meta.test_statuses).map((o) => ({ ...o, disabled: !canReview.value && REVIEW_DECISIONS.includes(o.value) && o.value !== item.value?.status })),
)
async function onReviewed(updated: unknown) {
  await load()
  if (updated && typeof updated === 'object') emit('saved', updated as TestCase)
}
const priorityOptions = computed(() => enumOptions('priority', meta.meta.priorities))
const languageOptions = computed(() => enumOptions('scriptLanguage', meta.meta.script_languages))

interface StepRow {
  uid: number
  id?: string
  action: string
  expected: string
  data: string
  call_test_id?: string | null
  call_test_key?: string
  call_test_title?: string
}
let uid = 0

function blank() {
  return {
    title: '',
    description: '',
    preconditions: '',
    execution_type: 'manual',
    script_language: 'sh',
    script: '',
    expected_result: '',
    priority: 'medium',
    status: 'draft',
    estimated_minutes: 0 as number | null,
    tags: [] as string[],
    category_ids: [] as string[],
    folder_id: (props.folderId ?? null) as string | null,
    steps: [] as StepRow[],
  }
}

const form = ref(blank())
/** text the tag suggestions look at: description, preconditions, expected result and step texts */
const tagText = computed(() =>
  [form.value.description, form.value.preconditions, form.value.expected_result, ...form.value.steps.flatMap((s) => [s.action, s.expected])].filter(Boolean).join('\n'),
)
const item = ref<TestCase | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const links = ref<TraceLink[]>([])
const reqs = ref<Requirement[]>([])
const pickReqs = ref(false)
const isNew = computed(() => !props.itemId)

const folderOptions = computed(() => [
  { id: null as string | null, label: t('common.topLevelOption') },
  ...flatten(buildTree(props.folders)).map((f) => ({ id: f.id as string | null, label: '  '.repeat(f.depth) + f.label })),
])

async function load() {
  error.value = ''
  links.value = []
  if (!props.itemId) {
    item.value = null
    form.value = blank()
    tab.value = 'details'
    return
  }
  loading.value = true
  try {
    const t = await api.get<TestCase>(`/tests/${props.itemId}`)
    item.value = t
    commentCount.value = t.comment_count ?? 0
    attachmentCount.value = null
    form.value = {
      title: t.title,
      description: t.description,
      preconditions: t.preconditions,
      execution_type: t.execution_type || 'manual',
      script_language: t.script_language || 'sh',
      script: t.script,
      expected_result: t.expected_result,
      priority: t.priority,
      status: t.status,
      estimated_minutes: t.estimated_minutes,
      tags: [...(t.tags ?? [])],
      category_ids: [...(t.category_ids ?? [])],
      folder_id: t.folder_id,
      steps: [...(t.steps ?? [])]
        .sort((a, b) => a.position - b.position)
        .map((s) => ({
          uid: ++uid,
          id: s.id,
          action: s.action,
          expected: s.expected,
          data: s.data,
          call_test_id: s.call_test_id || null,
          call_test_key: s.call_test_key,
          call_test_title: s.call_test_title,
        })),
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
    await api.post(`/tests/${props.itemId}/links/confirm`)
    notify.ok(t('suspect.confirmed'))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
async function loadLinks() {
  if (!props.itemId) return
  try {
    links.value = asItems(await api.get<TraceLink[]>(`/tests/${props.itemId}/links`))
    if (links.value.length && !reqs.value.length) {
      reqs.value = asItems(await api.get<{ items: Requirement[] }>(`/projects/${props.projectId}/requirements`))
    }
  } catch (e) {
    notify.fail(e, t('editor.loadLinksFailed'))
  }
}
watch(() => props.itemId, load, { immediate: true })

// ---------- steps ----------
function addStep(after?: number) {
  const row = { uid: ++uid, action: '', expected: '', data: '' }
  if (after === undefined) form.value.steps.push(row)
  else form.value.steps.splice(after + 1, 0, row)
}
// ---------- shared steps (call another test) ----------
const pickCall = ref(false)
const callAfter = ref<number | undefined>(undefined)
function openCallPicker(after?: number) {
  callAfter.value = after
  pickCall.value = true
}
function insertCalls(picked: { id: string; key: string; title: string }[]) {
  const rows = picked.map((t) => ({ uid: ++uid, action: '', expected: '', data: '', call_test_id: t.id, call_test_key: t.key, call_test_title: t.title }))
  if (callAfter.value === undefined) form.value.steps.push(...rows)
  else form.value.steps.splice(callAfter.value + 1, 0, ...rows)
}
const calledBy = computed(() => item.value?.called_by ?? [])

// ---------- tabs ----------
const tab = ref('details')
const commentCount = ref(0)
const attachmentCount = ref<number | null>(null)
const execCount = ref<number | null>(null)
function resolveId(field: string, id: string): string | undefined {
  if (field === 'category_ids') {
    const c = props.categories.find((x) => x.id === id)
    return c ? `${c.dimension}/${c.name}` : undefined
  }
  if (field === 'folder_id') return folderPath(props.folders, id) || undefined
  return undefined
}
async function onRestored(t: unknown) {
  await load()
  if (t && typeof t === 'object') emit('saved', t as TestCase)
}

function removeStep(i: number) {
  form.value.steps.splice(i, 1)
}
function moveStep(i: number, d: number) {
  form.value.steps = moveItem(form.value.steps, i, i + d)
}
function onReorder(e: { value: StepRow[] }) {
  form.value.steps = e.value
}

// ---------- save ----------
const loaded = ref<Record<string, unknown> | null>(null)
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => {
    // reload after an undo/redo unless there are unsaved changes
    if (props.itemId && JSON.stringify(bodyOf(form.value)) === JSON.stringify(loaded.value)) void load()
  },
)
function delTest(id: string) {
  return () => api.delTrash(`/tests/${id}`)
}
function bodyOf(f: typeof form.value) {
  return {
    ...f,
    // only automated tests have a script language (avoids spurious history changes)
    script_language: f.execution_type === 'automated' ? f.script_language : item.value?.script_language ?? '',
    estimated_minutes: f.estimated_minutes ?? 0,
    steps: f.steps
      .filter((s) => s.call_test_id || s.action.trim() || s.expected.trim() || s.data.trim())
      .map((s, i) =>
        s.call_test_id
          ? { ...(s.id ? { id: s.id } : {}), position: i + 1, action: '', expected: '', data: '', call_test_id: s.call_test_id }
          : { ...(s.id ? { id: s.id } : {}), position: i + 1, action: s.action, expected: s.expected, data: s.data, call_test_id: null },
      ),
  }
}
async function save() {
  saving.value = true
  error.value = ''
  const body = bodyOf(form.value)
  const before = loaded.value
  try {
    const saved = isNew.value
      ? await api.post<TestCase>(`/projects/${props.projectId}/tests`, body)
      : await api.patch<TestCase>(`/tests/${props.itemId}`, body)
    notify.ok(isNew.value ? t('tests.created') : t('tests.saved'), saved?.key)
    if (saved?.id) {
      if (isNew.value) recordUndo(createdEntry(t('undo.a.create', { key: saved.key }), delTest(saved.id)))
      else if (before) {
        const keys = changedKeys(before, body, Object.keys(body))
        if (keys.length) recordUndo(patchEntry(t('undo.a.edit', { key: saved.key }), `/tests/${saved.id}`, pickBefore(before, keys), pickBefore(body, keys)))
      }
    }
    emit('saved', saved)
    if (!isNew.value) await load() // refresh version, call-step keys and called_by
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

async function copy() {
  if (!item.value) return
  try {
    const c = await api.post<TestCase>(`/tests/${item.value.id}/copy`)
    notify.ok(t('tests.copied'), c?.key)
    if (c?.id) recordUndo(createdEntry(t('undo.a.copy', { key: item.value.key }), delTest(c.id)))
    emit('copied', c)
  } catch (e) {
    notify.fail(e)
  }
}

function remove() {
  if (!item.value) return
  const it = item.value
  notify.confirmDanger(t('tests.confirmDelete', { key: it.key }), async () => {
    try {
      const trash = await api.delTrash(`/tests/${it.id}`)
      if (trash) recordUndo(deletedEntry(t('undo.a.delete', { key: it.key }), trash, delTest(it.id)))
      notify.ok(t('tests.deleted'), t('undo.hint'))
      emit('deleted', it.id)
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- links ----------
function linkEntry(created: boolean, id: string, body: Record<string, unknown>, reqKey: string) {
  const pid = props.projectId
  return toggleEntry(
    t(created ? 'undo.a.link' : 'undo.a.unlink', { from: reqKey, to: item.value?.key ?? '' }),
    created,
    id,
    async () => (await api.post<TraceLink>(`/projects/${pid}/links`, body)).id,
    async (x) => void (await api.del(`/links/${x}`)),
  )
}
const reqById = computed(() => new Map(reqs.value.map((r) => [r.id, r])))
async function addLinks(picked: { id: string }[]) {
  try {
    for (const r of picked) {
      const body = { requirement_id: r.id, test_case_id: props.itemId }
      const l = await api.post<TraceLink>(`/projects/${props.projectId}/links`, body)
      if (l?.id) recordUndo(linkEntry(true, l.id, body, (r as { key?: string }).key ?? ''))
    }
    reqs.value = []
    await loadLinks()
    notify.ok(t('editor.linksAdded', picked.length))
  } catch (e) {
    notify.fail(e, t('editor.addLinkFailed'))
  }
}
async function removeLink(l: TraceLink) {
  try {
    await api.del(`/links/${l.id}`)
    recordUndo(linkEntry(false, l.id, { requirement_id: l.requirement_id, test_case_id: l.test_case_id, ...(l.criterion_id ? { criterion_id: l.criterion_id } : {}) }, reqById.value.get(l.requirement_id)?.key ?? l.requirement_key ?? ''))
    links.value = links.value.filter((x) => x.id !== l.id)
  } catch (e) {
    notify.fail(e)
  }
}
const execOptions = computed(() => [
  { label: enumLabel('executionType', 'manual'), value: 'manual', icon: 'pi pi-user' },
  { label: enumLabel('executionType', 'automated'), value: 'automated', icon: 'pi pi-bolt' },
])
const scriptPlaceholder = computed(() => {
  switch (form.value.script_language) {
    case 'python':
      return `import sys\n# ${t('tests.scriptComment')}\nsys.exit(0)`
    case 'javascript':
      return `// ${t('tests.scriptCommentShort')}\nprocess.exit(0)`
    default:
      return `#!/bin/sh\nset -e\n# ${t('tests.scriptComment')}\ncurl -fsS http://localhost:8080/healthz`
  }
})
</script>

<template>
  <div v-if="loading" class="empty" aria-busy="true"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
  <template v-else>
  <Tabs v-if="item" :id="tabsId" v-model:value="tab" class="editor-tabs" scrollable>
    <TabList>
      <Tab value="details"><i class="pi pi-file-edit" aria-hidden="true" /> {{ t('common.details') }}</Tab>
      <Tab value="executions"><i class="pi pi-play-circle" aria-hidden="true" /> {{ t('common.executions') }} <span v-if="execCount !== null" class="count">{{ execCount }}</span></Tab>
      <Tab value="links"><i class="pi pi-link" aria-hidden="true" /> {{ t('links.title') }} <span v-if="linkCount !== null" class="count">{{ linkCount }}</span></Tab>
      <Tab value="comments"><i class="pi pi-comments" aria-hidden="true" /> {{ t('common.comments') }} <span class="count">{{ commentCount }}</span></Tab>
      <Tab value="attachments"><i class="pi pi-paperclip" aria-hidden="true" /> {{ t('files.tab') }} <span v-if="attachmentCount !== null" class="count">{{ attachmentCount }}</span></Tab>
      <Tab value="history"><i class="pi pi-history" aria-hidden="true" /> {{ t('common.history') }}</Tab>
    </TabList>
  </Tabs>
  <div v-if="item && tab === 'executions'" v-bind="panel('executions')"><TestExecutionsPanel :project-id="projectId" :test-id="item.id" @count="(n) => (execCount = n)" /></div>
  <div v-if="item && tab === 'links'" v-bind="panel('links')"><ItemLinksPanel item-type="test" :item-id="item.id" :project-id="projectId" :can-write="canWrite" @count="(n) => (linkCount = n)" /></div>
  <div v-if="item && tab === 'comments'" v-bind="panel('comments')"><CommentsPanel entity="test" :entity-id="item.id" @count="(n) => (commentCount = n)" /></div>
  <div v-if="item && tab === 'attachments'" v-bind="panel('attachments')"><AttachmentsPanel type="test" :entity-id="item.id" :project-id="projectId" :can-write="canWrite" @count="(n) => (attachmentCount = n)" /></div>
  <div v-if="item && tab === 'history'" v-bind="panel('history')">
  <HistoryPanel
    entity="test"
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
      <Tag v-if="item?.version" :value="t('common.versionShort', { version: item.version })" severity="secondary" />
      <StatusTag v-if="item?.last_result" :value="item.last_result" kind="result" />
      <span v-if="item" class="muted small">{{ t('editor.updatedBy', { date: formatDateTime(item.updated_at) }) }}{{ item.created_by ? t('editor.createdBy', { user: item.created_by }) : '' }}</span>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <ReviewBar v-if="item" kind="test" :item="item" @changed="onReviewed" />

    <div v-field class="field">
      <label>{{ t('common.title') }}</label>
      <InputText v-model="form.title" required autofocus :disabled="!canWrite" />
    </div>
    <div class="row">
      <SelectButton v-model="form.execution_type" :options="execOptions" option-label="label" option-value="value" :allow-empty="false" :disabled="!canWrite" :aria-label="t('tests.executionType')">
        <template #option="{ option }"><i :class="option.icon" /> {{ option.label }}</template>
      </SelectButton>
    </div>
    <div class="form-grid">
      <div v-field class="field full">
        <label>{{ t('common.description') }}</label>
        <MentionInput v-model="form.description" multiline rows="2" auto-resize :project-id="projectId" :disabled="!canWrite" />
      </div>
      <div v-field class="field full">
        <label>{{ t('tests.preconditions') }}</label>
        <HelpTip id="test.preconditions" :label="t('tests.preconditions')" />
        <MentionInput v-model="form.preconditions" multiline rows="2" auto-resize :project-id="projectId" :disabled="!canWrite" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.status') }}</label>
        <HelpTip id="test.status" :label="t('common.status')" />
        <Select v-model="form.status" :options="statusOptions" option-label="label" option-value="value" option-disabled="disabled" :disabled="!canWrite" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.priority') }}</label>
        <Select v-model="form.priority" :options="priorityOptions" option-label="label" option-value="value" :disabled="!canWrite" />
      </div>
      <div v-field class="field">
        <label>{{ t('tests.estimate') }}</label>
        <HelpTip id="test.estimate" :label="t('tests.estimate')" />
        <InputNumber v-model="form.estimated_minutes" :min="0" show-buttons :disabled="!canWrite" fluid />
      </div>
      <div v-field class="field">
        <label>{{ t('common.folder') }}</label>
        <Select v-model="form.folder_id" :options="folderOptions" option-label="label" option-value="id" filter :disabled="!canWrite" />
      </div>
      <div v-field class="field full">
        <label>{{ t('common.categories') }}</label>
        <CategorySelect v-model="form.category_ids" :categories="categories" :disabled="!canWrite" />
      </div>
      <div v-field class="field full">
        <label>{{ t('common.tags') }}</label>
        <TagsInput v-model="form.tags" :disabled="!canWrite" />
        <TagSuggestions v-model="form.tags" :project-id="projectId" :title="form.title" :description="tagText" :disabled="!canWrite" />
      </div>
    </div>

    <template v-if="form.execution_type === 'automated'">
      <div class="section-title"><i class="pi pi-code" /> {{ t('tests.script') }} <HelpTip id="test.script" :label="t('tests.script')" /></div>
      <div class="row">
        <Select v-model="form.script_language" :options="languageOptions" option-label="label" option-value="value" :disabled="!canWrite" style="width: 160px" :aria-label="t('fields.script_language')" />
        <span class="small muted">{{ t('tests.scriptHint') }}</span>
      </div>
      <Textarea v-model="form.script" class="code-input w-full" rows="12" :placeholder="scriptPlaceholder" :aria-label="t('tests.script')" spellcheck="false" :disabled="!canWrite" style="font-family: var(--qtf-mono)" />
    </template>

    <div>
      <div class="section-title">
        <i class="pi pi-list" /> {{ t('tests.steps', { count: form.steps.length }) }}
        <HelpTip id="test.steps" :label="t('tests.steps', { count: form.steps.length })" />
        <span class="grow" />
        <Button v-if="canWrite" size="small" :label="t('tests.insertCall')" icon="pi pi-directions" text v-tooltip.bottom="t('tests.insertCallHint')" @click="openCallPicker()" />
        <HelpTip id="test.calls" :label="t('tests.insertCall')" />
        <Button v-if="canWrite" size="small" :label="t('tests.addStep')" icon="pi pi-plus" text @click="addStep()" />
      </div>
      <DataTable v-if="form.steps.length" v-stack :value="form.steps" data-key="uid" size="small" :reorderable-rows="canWrite" class="steps-table" @row-reorder="onReorder">
        <Column v-if="canWrite" row-reorder style="width: 2rem" />
        <Column header="#" style="width: 2.5rem"><template #body="{ index }"><b>{{ index + 1 }}</b></template></Column>
        <Column :header="t('tests.action')" style="width: 40%">
          <template #body="{ data, index }">
            <div v-if="data.call_test_id" class="call-step" :title="t('tests.callsHint', { test: data.call_test_key || t('tests.calledTest') })">
              <i class="pi pi-directions" style="color: var(--p-primary-color)" />
              <span>
                {{ t('tests.calls') }}
                <RouterLink :to="{ name: 'tests', params: { projectId }, query: { item: data.call_test_id } }" class="key">{{ data.call_test_key || t('tests.callTestFallback') }}</RouterLink>
                <template v-if="data.call_test_title"> – {{ data.call_test_title }}</template>
              </span>
            </div>
            <Textarea v-else v-model="data.action" rows="1" auto-resize class="w-full" :disabled="!canWrite" :placeholder="t('tests.actionPlaceholder')" :aria-label="t('tests.stepActionN', { n: index + 1 })" />
          </template>
        </Column>
        <Column :header="t('tests.expectedResult')" style="width: 32%">
          <template #body="{ data, index }">
            <span v-if="data.call_test_id" class="small muted">{{ t('tests.calledExpected') }}</span>
            <Textarea v-else v-model="data.expected" rows="1" auto-resize class="w-full" :disabled="!canWrite" :placeholder="t('tests.expectedPlaceholder')" :aria-label="t('tests.stepExpectedN', { n: index + 1 })" />
          </template>
        </Column>
        <Column :header="t('tests.data')" style="width: 22%">
          <template #body="{ data, index }"><Textarea v-if="!data.call_test_id" v-model="data.data" rows="1" auto-resize class="w-full mono" :disabled="!canWrite" :aria-label="t('tests.stepDataN', { n: index + 1 })" /></template>
        </Column>
        <Column v-if="canWrite" style="width: 7.5rem">
          <template #body="{ index }">
            <div class="row nowrap" style="gap: 0">
              <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :disabled="index === 0" :aria-label="t('tests.moveStepUp', { n: index + 1 })" v-tooltip="t('common.moveUp')" @click="moveStep(index, -1)" />
              <Button icon="pi pi-arrow-down" text rounded size="small" severity="secondary" :disabled="index === form.steps.length - 1" :aria-label="t('tests.moveStepDown', { n: index + 1 })" v-tooltip="t('common.moveDown')" @click="moveStep(index, 1)" />
              <Button icon="pi pi-plus" text rounded size="small" severity="secondary" :aria-label="t('tests.insertStepBelow', { n: index + 1 })" v-tooltip="t('common.insertBelow')" @click="addStep(index)" />
              <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('tests.removeStep', { n: index + 1 })" v-tooltip="t('common.remove')" @click="removeStep(index)" />
            </div>
          </template>
        </Column>
      </DataTable>
      <div v-else class="muted small">{{ form.execution_type === 'manual' ? t('tests.noStepsManual') : t('tests.noStepsAutomated') }}</div>
      <div v-if="calledBy.length" class="row small" style="margin-top: 8px; gap: 6px">
        <i class="pi pi-share-alt muted" /><span class="muted">{{ t('tests.usedBy') }}</span>
        <span v-for="(c, i) in calledBy" :key="c.id">
          <RouterLink :to="{ name: 'tests', params: { projectId }, query: { item: c.id } }" class="key" :title="c.title">{{ c.key }}</RouterLink>
          <span class="muted">&nbsp;{{ c.title }}</span>{{ i < calledBy.length - 1 ? ',' : '' }}
        </span>
      </div>
    </div>

    <div v-field class="field">
      <label>{{ t('tests.overallExpected') }}</label>
      <HelpTip id="test.expected" :label="t('tests.overallExpected')" />
      <Textarea v-model="form.expected_result" rows="2" auto-resize :disabled="!canWrite" />
    </div>

    <div v-if="item">
      <div class="section-title">
        <i class="pi pi-link" /> {{ t('tests.linkedRequirements', { count: links.length }) }}
        <span class="grow" />
        <Button v-if="canLink && suspectCount" size="small" :label="t('suspect.confirmAll', { n: suspectCount })" icon="pi pi-check" text @click="confirmAll" />
        <Button v-if="canLink" size="small" :label="t('tests.linkRequirements')" icon="pi pi-plus" text @click="pickReqs = true" />
      </div>
      <div v-if="!links.length" class="muted small">{{ t('tests.verifiesNothing') }}</div>
      <div v-for="l in links" :key="l.id" class="row nowrap" style="padding: 5px 0; border-bottom: 1px solid var(--qtf-border)">
        <RouterLink :to="{ name: 'requirements', params: { projectId }, query: { item: l.requirement_id } }" class="key">{{ l.requirement_key || reqById.get(l.requirement_id)?.key }}</RouterLink>
        <span class="grow ellipsis">{{ reqById.get(l.requirement_id)?.title }}</span>
        <SuspectLink :link="l" :can-confirm="canLink" @confirmed="onSuspectConfirmed" />
        <StatusTag :value="reqById.get(l.requirement_id)?.obligation" kind="obligation" empty="" />
        <Button v-if="canLink" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('common.unlinkItem', { key: l.requirement_key || reqById.get(l.requirement_id)?.key || '' })" v-tooltip="t('common.unlink')" @click="removeLink(l)" />
      </div>
    </div>

    <TestDependenciesPanel v-if="item" :project-id="projectId" :test-id="item.id" :can-write="canWrite" @changed="emit('saved', item)" />

    <div class="row editor-footer">
      <Button v-if="item && canWrite" :label="t('common.delete')" icon="pi pi-trash" severity="danger" text @click="remove" />
      <Button v-if="item && canWrite" :label="t('common.copy')" icon="pi pi-copy" severity="secondary" text style="margin-right: auto" @click="copy" />
      <span v-else style="margin-right: auto" />
      <RouterLink v-if="item && isSoftware(proj.project)" :to="{ name: 'prompts', params: { projectId }, query: { kind: 'automate_tests', ids: item.id } }" class="small row nowrap" style="gap: 4px"><i class="pi pi-sparkles" aria-hidden="true" />{{ t('prompts.forItem') }}</RouterLink>
      <RouterLink v-if="item" :to="{ name: 'flow', params: { projectId }, query: { test: item.id } }" class="small row nowrap" style="gap: 4px"><i class="pi pi-share-alt" aria-hidden="true" />{{ t('flow.flowOfTest') }}</RouterLink>
      <Button :label="t('common.close')" severity="secondary" text @click="emit('close')" />
      <Button v-if="canWrite" type="submit" :label="isNew ? t('common.create') : t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!form.title.trim()" />
    </div>
    <ItemPickerDialog
      v-if="item"
      v-model:visible="pickReqs"
      kind="requirement"
      :project-id="projectId"
      :exclude="links.map((l) => l.requirement_id)"
      :header="t('tests.linkRequirements')"
      :confirm-label="t('common.link')"
      @pick="addLinks"
    />
    <ItemPickerDialog
      v-model:visible="pickCall"
      kind="test"
      :project-id="projectId"
      :exclude="item ? [item.id] : []"
      :header="t('tests.insertCallHeader')"
      :confirm-label="t('tests.insertCallConfirm')"
      @pick="(p) => insertCalls(p as TestCase[])"
    />
  </form>
  </template>
</template>
