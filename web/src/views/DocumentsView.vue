<script setup lang="ts">
// Document archive: every file of the project (attachments, documents, reports).
import { tagClass } from '@/lib/tagColors'
import { computed, onMounted, ref, watch } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import QueryInput from '@/components/QueryInput.vue'
import TagsInput from '@/components/TagsInput.vue'
import FileDropZone from '@/components/FileDropZone.vue'
import UploadQueue from '@/components/UploadQueue.vue'
import FilePreviewDialog from '@/components/FilePreviewDialog.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { api, ApiError, asItems, errorMessage, postDownload } from '@/lib/api'
import { ARCHIVE_KINDS, archiveParams, contentUrl, fileIcon, formatFileSize, linkIcon, linkTarget, previewKind, sortFiles, tagsField } from '@/lib/files'
import { formatDateTime, formatFullDateTime, getFormatLocale } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { FileInfo } from '@/lib/types'
import ColumnsMenu from '@/components/ColumnsMenu.vue'
import { useColumnLayout, type ColumnDef } from '@/composables/columns'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'
import { useUploads } from '@/composables/uploads'
import { useI18n } from 'vue-i18n'
import HelpLink from '@/guide/HelpLink.vue'
import SelectButton from 'primevue/selectbutton'
import { valuesBefore } from '@/lib/bulkRuns'
import { entry, recordUndo } from '@/lib/undoActions'

const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const prefs = usePrefsStore()
const notify = useNotify()
const { t } = useI18n()
const maxMb = computed(() => (meta.meta as { max_upload_mb?: number }).max_upload_mb || undefined)

const files = ref<FileInfo[]>([])

// ---------- bulk ----------
const selection = ref<FileInfo[]>([])
const selectedIds = computed(() => selection.value.map((f) => f.id))
/** selected files the user may change (uploader or project manager) */
const manageable = computed(() => selection.value.filter((f) => canManage(f)))
const zipping = ref(false)
const tagDialog = ref(false)
const tagMode = ref<'add' | 'remove'>('add')
const tagValues = ref<string[]>([])
const descDialog = ref(false)
const descValue = ref('')
watch(files, (list) => {
  const ids = new Set(list.map((f) => f.id))
  selection.value = selection.value.filter((s) => ids.has(s.id))
})
interface BulkFile {
  id: string
  name: string
  reason?: string
}
async function bulkZip() {
  zipping.value = true
  try {
    await postDownload(`/projects/${proj.pid}/documents/zip`, { ids: selectedIds.value }, `${proj.project?.key ?? 'documents'}.zip`)
  } catch (e) {
    notify.fail(e)
  } finally {
    zipping.value = false
  }
}
async function bulkUpdate(body: Record<string, unknown>) {
  const ids = manageable.value.map((f) => f.id)
  if (!ids.length) return
  const before = valuesBefore(files.value, ids, ['tags', 'description'])
  const path = `/projects/${proj.pid}/documents/bulk`
  try {
    const res = await api.post<{ updated: BulkFile[]; skipped: BulkFile[] }>(path, { ids, ...body })
    const done = new Set(res.updated.map((u) => u.id))
    const undo = before.filter((b) => done.has(b.id))
    if (undo.length) {
      recordUndo(
        entry(
          t('documents.bulk.undo', { n: undo.length }),
          async () => {
            for (const b of undo) await api.patch(`/files/${b.id}`, b.before)
          },
          () => api.post(path, { ids: undo.map((b) => b.id), ...body }),
        ),
      )
    }
    notify.ok(t('documents.bulk.updated', { n: res.updated.length }, res.updated.length))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
function openTags(mode: 'add' | 'remove') {
  tagMode.value = mode
  tagValues.value = []
  tagDialog.value = true
}
async function applyTags() {
  tagDialog.value = false
  if (tagValues.value.length) await bulkUpdate(tagMode.value === 'add' ? { add_tags: tagValues.value } : { remove_tags: tagValues.value })
}
function openDesc() {
  descValue.value = ''
  descDialog.value = true
}
async function applyDesc() {
  descDialog.value = false
  await bulkUpdate({ description: descValue.value.trim() })
}
function bulkDelete() {
  const ids = manageable.value.map((f) => f.id)
  notify.confirmDanger(t('documents.bulk.confirmDelete', { n: ids.length }, ids.length), async () => {
    try {
      const res = await api.del<{ deleted: number; skipped: BulkFile[] }>(`/projects/${proj.pid}/documents/bulk`, { ids })
      notify.ok(t('documents.bulk.deleted', { n: res.deleted }, res.deleted))
      selection.value = []
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
const loading = ref(false)
const error = ref<unknown>(null)
const q = ref('')
const kind = ref('')
const kindOptions = computed(() => [{ label: t('documents.allKinds'), value: '' }, ...ARCHIVE_KINDS.map((k) => ({ label: t(`documents.kind.${k}`), value: k }))])
const canUpload = computed(() => auth.can(P.requirementWrite) || auth.can(P.testWrite))
const canManage = (f: FileInfo) => auth.can(P.projectUpdate) || (!!auth.user && f.uploaded_by === auth.user.email)
const kindSeverity = (k: string) => (k === 'report' ? 'info' : k === 'document' ? 'success' : 'secondary')

async function load() {
  loading.value = true
  error.value = null
  try {
    files.value = sortFiles(asItems(await api.get<{ items: FileInfo[] }>(`/projects/${proj.pid}/documents`, archiveParams(kind.value, q.value))))
  } catch (e) {
    error.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e, t('documents.loadFailed'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---------- upload ----------
const uploadOpen = ref(false)
const pending = ref<File[]>([])
const upDescription = ref('')
const upTags = ref<string[]>([])
const { queue, announcement, run, dismiss } = useUploads()
function openUpload() {
  pending.value = []
  upDescription.value = ''
  upTags.value = []
  uploadOpen.value = true
}
function addPending(list: File[]) {
  pending.value = [...pending.value, ...list]
}
async function startUpload() {
  const list = pending.value
  uploadOpen.value = false
  const added = await run<FileInfo>(list, `/projects/${proj.pid}/documents`, { fields: { description: upDescription.value.trim(), tags: tagsField(upTags.value) } }, {
    done: (name) => t('files.uploadedNamed', { name }),
    failed: (name, err) => t('files.uploadFailedNamed', { name, error: err }),
  })
  if (added.length) await load()
}

// ---------- edit ----------
const editOpen = ref(false)
const editing = ref<FileInfo | null>(null)
const form = ref({ name: '', description: '', tags: [] as string[] })
const formError = ref('')
const saving = ref(false)
function openEdit(f: FileInfo) {
  editing.value = f
  form.value = { name: f.name, description: f.description ?? '', tags: [...(f.tags ?? [])] }
  formError.value = ''
  editOpen.value = true
}
async function saveEdit() {
  if (!editing.value) return
  saving.value = true
  formError.value = ''
  try {
    const u = await api.patch<FileInfo>(`/files/${editing.value.id}`, { name: form.value.name.trim(), description: form.value.description.trim(), tags: form.value.tags })
    Object.assign(editing.value, u ?? { ...form.value })
    editOpen.value = false
    notify.ok(t('documents.saved'))
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

function remove(f: FileInfo) {
  const n = f.links?.length ?? 0
  notify.confirmDanger(n ? t('documents.confirmDeleteLinked', { name: f.name, count: n }, n) : t('documents.confirmDelete', { name: f.name }), async () => {
    try {
      await api.del(`/files/${f.id}`)
      files.value = files.value.filter((x) => x.id !== f.id)
      notify.ok(t('documents.deleted'), f.name)
    } catch (e) {
      notify.fail(e)
    }
  })
}

const preview = ref<FileInfo | null>(null)
const previewOpen = ref(false)
function open(f: FileInfo) {
  preview.value = f
  previewOpen.value = true
}
// ---------- columns ----------
const docColumns = computed<ColumnDef[]>(() => [
  { field: 'kind', header: t('documents.kindCol') },
  { field: 'size', header: t('files.size') },
  { field: 'uploaded_by', header: t('files.uploader') },
  { field: 'created_at', header: t('files.date') },
  { field: 'links', header: t('documents.linkedTo'), sortable: false },
])
const docLayout = useColumnLayout(() => 'qtf.cols.documents', docColumns, () => ['kind', 'size', 'uploaded_by', 'created_at', 'links'])
const entityLabel = (type: string) => t(`documents.entity.${['requirement', 'test', 'run', 'execution', 'step'].includes(type) ? type : 'other'}`)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('documents.title') }}</h1>
      <div class="sub">{{ t('documents.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="documents" />
      <Button v-if="canUpload" :label="t('documents.upload')" icon="pi pi-upload" @click="openUpload" />
    </div>
  </div>
  <UploadQueue :queue="queue" :announcement="announcement" @dismiss="dismiss" />
  <div class="card">
    <div class="filterbar">
      <QueryInput v-model="q" scope="document" :error="error" :placeholder="t('documents.filterPlaceholder', { example: 'tag:spec type:pdf linked:*REQ-1' })" @submit="load" />
      <Select v-model="kind" :options="kindOptions" option-label="label" option-value="value" :placeholder="t('documents.allKinds')" :aria-label="t('documents.kindFilter')" style="min-width: 160px" @change="load" />
      <ColumnsMenu
        :columns="docColumns"
        :order="docLayout.order.value"
        @toggle="(id, on) => docLayout.toggle(id, on)"
        @move="(id, d) => docLayout.move(id, d)"
        @reset="docLayout.reset()"
      />
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
    </div>
    <div v-if="selection.length" class="bulkbar" role="toolbar" :aria-label="t('documents.bulk.toolbar')">
      <b>{{ t('common.selected', { count: selection.length }) }}</b>
      <Button :label="t('documents.bulk.zip')" icon="pi pi-download" size="small" severity="secondary" outlined :loading="zipping" @click="bulkZip" />
      <template v-if="manageable.length">
        <Button :label="t('documents.bulk.addTags')" icon="pi pi-tag" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openTags('add')" />
        <Button :label="t('documents.bulk.removeTags')" icon="pi pi-minus" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openTags('remove')" />
        <Button :label="t('documents.bulk.description')" icon="pi pi-align-left" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openDesc" />
        <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
        <span v-if="manageable.length < selection.length" class="small muted">{{ t('documents.bulk.onlyYours', { n: manageable.length, total: selection.length }) }}</span>
      </template>
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
    </div>
    <DataTable
      :ref="docLayout.bind"
      v-model:selection="selection"
      v-stack
      :value="files"
      data-key="id"
      :loading="loading"
      :aria-busy="loading"
      paginator
      :rows="prefs.rowsFor(25)"
      :rows-per-page-options="prefs.rowsPerPage"
      size="small"
      sort-field="created_at"
      :sort-order="-1"
      reorderable-columns
      @column-reorder="docLayout.onReorder()"
    >
      <Column selection-mode="multiple" column-key="_select" :reorderable-column="false" style="width: 2.6rem" />
      <Column field="name" :header="t('common.name')" sortable :reorderable-column="false" style="min-width: 220px" body-class="stack-main">
        <template #body="{ data }">
          <div class="row nowrap" style="min-width: 0">
            <i :class="fileIcon(data.content_type, data.name)" style="font-size: 18px; color: var(--p-primary-color)" aria-hidden="true" />
            <div style="min-width: 0">
              <button v-if="previewKind(data) !== 'download'" type="button" class="link-button doc-name" @click="open(data)">{{ data.name }}</button>
              <a v-else :href="contentUrl(data.id)" :download="data.name" class="doc-name">{{ data.name }}</a>
              <div v-if="data.description" class="small muted">{{ data.description }}</div>
              <div v-if="data.tags?.length" class="row" style="gap: 4px; margin-top: 2px"><span v-for="tg in data.tags" :key="tg" class="tag-chip" :class="tagClass(tg)">{{ tg }}</span></div>
            </div>
          </div>
        </template>
      </Column>
      <template v-for="c in docLayout.visible.value" :key="c.field">
      <Column v-if="c.field === 'kind'" field="kind" column-key="kind" :header="c.header" sortable style="width: 120px"><template #body="{ data }"><Tag :value="t(`documents.kind.${data.kind}`)" :severity="kindSeverity(data.kind)" /></template></Column>
      <Column v-else-if="c.field === 'size'" field="size" column-key="size" :header="c.header" sortable style="width: 100px"><template #body="{ data }"><span class="small">{{ formatFileSize(data.size, getFormatLocale()) }}</span></template></Column>
      <Column v-else-if="c.field === 'uploaded_by'" field="uploaded_by" column-key="uploaded_by" :header="c.header" sortable style="width: 200px">
        <template #body="{ data }"><span class="row nowrap small" style="gap: 6px; min-width: 0"><UserAvatar :email="data.uploaded_by" :name="data.uploaded_by" :size="20" /><span class="ellipsis">{{ data.uploaded_by }}</span></span></template>
      </Column>
      <Column v-else-if="c.field === 'created_at'" field="created_at" column-key="created_at" :header="c.header" sortable style="width: 150px"><template #body="{ data }"><span class="small" :title="formatFullDateTime(data.created_at)">{{ formatDateTime(data.created_at) }}</span></template></Column>
      <Column v-else-if="c.field === 'links'" column-key="links" :header="c.header" style="min-width: 160px">
        <template #body="{ data }">
          <div v-if="data.links?.length" class="row" style="gap: 4px">
            <template v-for="l in data.links" :key="l.entity_type + l.entity_id">
              <RouterLink v-if="linkTarget(l, proj.pid)" :to="linkTarget(l, proj.pid)!" class="trace-chip" :aria-label="`${entityLabel(l.entity_type)} ${l.key}`">
                <i :class="linkIcon(l.entity_type)" aria-hidden="true" /> {{ l.key }}
              </RouterLink>
              <span v-else class="trace-chip">{{ l.key }}</span>
            </template>
          </div>
          <span v-else class="small muted">—</span>
        </template>
      </Column>
      </template>
      <Column column-key="_actions" :reorderable-column="false" style="width: 150px">
        <template #body="{ data }">
          <div class="row nowrap" style="gap: 0">
            <Button v-if="previewKind(data) !== 'download'" icon="pi pi-eye" text rounded size="small" severity="secondary" :aria-label="t('files.previewNamed', { name: data.name })" v-tooltip="t('files.preview')" @click="open(data)" />
            <a :href="contentUrl(data.id)" :download="data.name" class="p-button p-button-text p-button-rounded p-button-secondary p-button-icon-only p-button-sm" :aria-label="t('files.downloadNamed', { name: data.name })" v-tooltip="t('files.download')"><i class="pi pi-download" aria-hidden="true" /></a>
            <template v-if="canManage(data)">
              <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('documents.editNamed', { name: data.name })" v-tooltip="t('common.edit')" @click="openEdit(data)" />
              <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('documents.deleteNamed', { name: data.name })" v-tooltip="t('common.delete')" @click="remove(data)" />
            </template>
          </div>
        </template>
      </Column>
      <template #empty><div class="empty"><i class="pi pi-folder-open" />{{ q || kind ? t('documents.noneFilter') : t('documents.none') }}</div></template>
    </DataTable>
  </div>

  <Dialog v-model:visible="uploadOpen" :header="t('documents.upload')" modal :style="{ width: 'min(560px, calc(100vw - 16px))' }">
    <form class="grid" style="gap: 12px" @submit.prevent="startUpload">
      <FileDropZone :max-mb="maxMb" @files="addPending" />
      <ul v-if="pending.length" class="plain-list small" :aria-label="t('documents.selectedFiles')">
        <li v-for="(f, i) in pending" :key="i" class="row nowrap" style="justify-content: space-between">
          <span class="ellipsis">{{ f.name }} <span class="muted">{{ formatFileSize(f.size, getFormatLocale()) }}</span></span>
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('files.dismiss', { name: f.name })" @click="pending.splice(i, 1)" />
        </li>
      </ul>
      <div v-field class="field"><label>{{ t('common.description') }}</label><Textarea v-model="upDescription" rows="2" auto-resize /></div>
      <div v-field class="field"><label>{{ t('common.tags') }}</label><TagsInput v-model="upTags" /></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="uploadOpen = false" />
        <Button type="submit" :label="t('documents.uploadN', { count: pending.length }, pending.length)" icon="pi pi-upload" :disabled="!pending.length" />
      </div>
    </form>
  </Dialog>

  <Dialog v-model:visible="editOpen" :header="t('documents.edit')" modal :style="{ width: 'min(520px, calc(100vw - 16px))' }">
    <form class="grid" style="gap: 12px" @submit.prevent="saveEdit">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div v-field class="field"><label>{{ t('common.name') }}</label><InputText v-model="form.name" required /></div>
      <div v-field class="field"><label>{{ t('common.description') }}</label><Textarea v-model="form.description" rows="3" auto-resize /></div>
      <div v-field class="field"><label>{{ t('common.tags') }}</label><TagsInput v-model="form.tags" /></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="editOpen = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>
  <FilePreviewDialog v-model:visible="previewOpen" :file="preview" />
  <Dialog v-model:visible="tagDialog" :header="tagMode === 'add' ? t('documents.bulk.addTagsHeader', { n: manageable.length }, manageable.length) : t('documents.bulk.removeTagsHeader', { n: manageable.length }, manageable.length)" modal style="width: min(460px, 96vw)">
    <div class="bulk-form">
      <SelectButton v-model="tagMode" :options="[{ l: t('common.add'), v: 'add' }, { l: t('common.remove'), v: 'remove' }]" option-label="l" option-value="v" :allow-empty="false" :aria-label="t('common.tags')" />
      <TagsInput v-model="tagValues" :aria-label="t('common.tags')" />
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="tagDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!tagValues.length" @click="applyTags" />
    </template>
  </Dialog>
  <Dialog v-model:visible="descDialog" :header="t('documents.bulk.descHeader', { n: manageable.length }, manageable.length)" modal style="width: min(480px, 96vw)">
    <label for="bk-doc-desc" class="block-label">{{ t('common.description') }}</label>
    <Textarea id="bk-doc-desc" v-model="descValue" rows="3" auto-resize fluid aria-describedby="bk-doc-desc-help" />
    <small id="bk-doc-desc-help" class="muted">{{ t('documents.bulk.descHelp') }}</small>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="descDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" @click="applyDesc" />
    </template>
  </Dialog>
</template>

<style scoped>
.bulk-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.block-label {
  display: block;
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 4px;
}
.doc-name {
  font-weight: 600;
  text-align: left;
  word-break: break-word;
}
</style>
