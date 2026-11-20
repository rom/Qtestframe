<script setup lang="ts">
import { entry, patchEntry, recordUndo, toggleEntry } from '@/lib/undoActions'
import { computed, ref, watch } from 'vue'
import Tree from 'primevue/tree'
import ContextMenu from 'primevue/contextmenu'
import Menu from 'primevue/menu'
import FolderPickerDialog from './FolderPickerDialog.vue'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Message from 'primevue/message'
import { api, asItems, errorMessage } from '@/lib/api'
import { allKeys, ancestorKeys, buildTree, canMoveFolder, checkedIds, descendantIds, folderPath, topMostFolders, type FolderNode } from '@/lib/tree'
import type { Folder, ItemKind, Project } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { decodeIds, FOLDER_MIME, ITEMS_MIME } from '@/lib/dnd'
import { useI18n } from 'vue-i18n'


const props = defineProps<{ kind: ItemKind; projectId: string; folders: Folder[]; canWrite: boolean; hideTitle?: boolean }>()
const selected = defineModel<string | null>('selected', { default: null })
const emit = defineEmits<{ changed: []; dropItems: [ids: string[], folderId: string | null] }>()
const notify = useNotify()
const { t } = useI18n()

const nodes = computed(() => buildTree(props.folders))
const expanded = ref<Record<string, boolean>>({})
let initialised = false
watch(
  nodes,
  (n) => {
    if (!initialised && n.length) {
      // expand the first two levels initially
      const keys: Record<string, boolean> = {}
      for (const r of n) keys[r.key] = true
      expanded.value = keys
      initialised = true
    }
  },
  { immediate: true },
)
watch(selected, (id) => {
  if (id) for (const k of ancestorKeys(props.folders, id)) expanded.value[k] = true
})
const selectionKeys = computed(() => (selected.value ? { [selected.value]: true } : {}))

function onSelect(node: { key?: string | number }) {
  selected.value = String(node.key)
}

// ---------- drag and drop ----------
const dragFolder = ref<string | null>(null)
const dropTarget = ref<string | null | undefined>(undefined) // undefined = none, null = root

function onDragStart(e: DragEvent, node: FolderNode) {
  if (!props.canWrite || !e.dataTransfer) return
  dragFolder.value = node.key
  e.dataTransfer.setData(FOLDER_MIME, node.key)
  e.dataTransfer.setData('text/plain', node.label)
  e.dataTransfer.effectAllowed = 'move'
}
function onDragEnd() {
  dragFolder.value = null
  dropTarget.value = undefined
}
function accepts(e: DragEvent, target: string | null): boolean {
  if (!props.canWrite || !e.dataTransfer) return false
  const types = Array.from(e.dataTransfer.types)
  if (types.includes(ITEMS_MIME)) return true
  if (types.includes(FOLDER_MIME) && dragFolder.value) return canMoveFolder(props.folders, dragFolder.value, target)
  return false
}
function onDragOver(e: DragEvent, target: string | null) {
  if (!accepts(e, target)) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dropTarget.value = target
}
function onDragLeave(target: string | null) {
  if (dropTarget.value === target) dropTarget.value = undefined
}
async function moveFolder(fid: string, target: string | null) {
  if (!canMoveFolder(props.folders, fid, target)) return
  try {
    const old = props.folders.find((x) => x.id === fid)
    await api.post(`/folders/${fid}/move`, { parent_id: target })
    if (old) recordUndo(entry(t('undo.a.folderMove', { name: old.name }), () => api.post(`/folders/${fid}/move`, { parent_id: old.parent_id ?? null }), () => api.post(`/folders/${fid}/move`, { parent_id: target })))
    if (target) expanded.value[target] = true
    notify.ok(t('folders.moved'))
    emit('changed')
  } catch (err) {
    notify.fail(err, t('folders.moveFailed'))
  }
}

async function onDrop(e: DragEvent, target: string | null) {
  e.preventDefault()
  dropTarget.value = undefined
  const dt = e.dataTransfer
  if (!dt) return
  if (Array.from(dt.types).includes(ITEMS_MIME)) {
    const ids = decodeIds(dt.getData(ITEMS_MIME))
    if (ids.length) emit('dropItems', ids, target)
    return
  }
  const fid = dt.getData(FOLDER_MIME) || dragFolder.value
  dragFolder.value = null
  if (fid) await moveFolder(fid, target)
}

// keyboard alternative to dragging a folder: "Move to folder…" picks the new parent
const moveFolderDialog = ref(false)
const movingFolder = ref<Folder | null>(null)
const moveTargets = computed(() => {
  const f = movingFolder.value
  if (!f) return props.folders
  const skip = descendantIds(props.folders, f.id)
  return props.folders.filter((x) => x.id !== f.id && !skip.has(x.id))
})
function openMoveFolder(f: Folder) {
  movingFolder.value = f
  moveFolderDialog.value = true
}
function onPickParent(target: string | null) {
  if (movingFolder.value) void moveFolder(movingFolder.value.id, target)
}

// ---------- context menu & dialogs ----------
const menu = ref<InstanceType<typeof ContextMenu>>()
const ctxNode = ref<FolderNode | null>(null)
const menuItems = computed(() => [
  { label: t('folders.newSubfolder'), icon: 'pi pi-folder-plus', command: () => openCreate(ctxNode.value?.key ?? null), disabled: !props.canWrite },
  { label: t('folders.rename'), icon: 'pi pi-pencil', command: () => ctxNode.value && openRename(ctxNode.value.data), disabled: !props.canWrite },
  { label: t('folders.moveToFolder'), icon: 'pi pi-folder-open', command: () => ctxNode.value && openMoveFolder(ctxNode.value.data), disabled: !props.canWrite },
  { label: t('folders.moveToProject'), icon: 'pi pi-send', command: () => ctxNode.value && openMoveProject(ctxNode.value.data), disabled: !props.canWrite },
  { label: t('folders.moveToTop'), icon: 'pi pi-arrow-up', command: () => ctxNode.value && moveToRoot(ctxNode.value.data), disabled: !props.canWrite || !ctxNode.value?.data.parent_id },
  { separator: true },
  { label: t('folders.expandAll'), icon: 'pi pi-plus', command: () => (expanded.value = allKeys(nodes.value)) },
  { label: t('folders.collapseAll'), icon: 'pi pi-minus', command: () => (expanded.value = {}) },
  { separator: true },
  { label: t('folders.deleteEllipsis'), icon: 'pi pi-trash', class: 'p-error', command: () => ctxNode.value && openDelete(ctxNode.value.data), disabled: !props.canWrite },
])
function onContext(e: MouseEvent, node: FolderNode) {
  ctxNode.value = node
  menu.value?.show(e)
}
/** the same actions from a button, for keyboard and touch users */
const actionsMenu = ref<InstanceType<typeof Menu>>()
function findNode(list: FolderNode[], key: string): FolderNode | null {
  for (const n of list) {
    if (n.key === key) return n
    const c = findNode((n.children ?? []) as FolderNode[], key)
    if (c) return c
  }
  return null
}
const selectedNode = computed(() => (selected.value ? findNode(nodes.value, selected.value) : null))
function openActions(e: Event) {
  ctxNode.value = selectedNode.value
  actionsMenu.value?.toggle(e)
}

const nameDialog = ref(false)
const nameMode = ref<'create' | 'rename'>('create')
const nameForm = ref({ name: '', description: '', parent: null as string | null, id: '' })
const busy = ref(false)
const dialogError = ref('')

function openCreate(parent: string | null) {
  nameMode.value = 'create'
  nameForm.value = { name: '', description: '', parent, id: '' }
  dialogError.value = ''
  nameDialog.value = true
}
function openRename(f: Folder) {
  nameMode.value = 'rename'
  nameForm.value = { name: f.name, description: f.description, parent: f.parent_id, id: f.id }
  dialogError.value = ''
  nameDialog.value = true
}
async function saveName() {
  busy.value = true
  dialogError.value = ''
  try {
    if (nameMode.value === 'create') {
      const body = { kind: props.kind, name: nameForm.value.name, description: nameForm.value.description, parent_id: nameForm.value.parent }
      const path = `/projects/${props.projectId}/folders`
      const f = await api.post<Folder>(path, body)
      if (f?.id)
        recordUndo(
          toggleEntry(
            t('undo.a.folderCreate', { name: body.name }),
            true,
            f.id,
            async () => (await api.post<Folder>(path, body)).id,
            async (id) => void (await api.del(`/folders/${id}`)),
          ),
        )
      if (nameForm.value.parent) expanded.value[nameForm.value.parent] = true
      if (f?.id) selected.value = f.id
    } else {
      const old = props.folders.find((x) => x.id === nameForm.value.id)
      const after = { name: nameForm.value.name, description: nameForm.value.description }
      await api.patch(`/folders/${nameForm.value.id}`, after)
      if (old) recordUndo(patchEntry(t('undo.a.folderRename', { name: old.name }), `/folders/${old.id}`, { name: old.name, description: old.description }, after))
    }
    nameDialog.value = false
    emit('changed')
  } catch (e) {
    dialogError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

async function moveToRoot(f: Folder) {
  try {
    await api.post(`/folders/${f.id}/move`, { parent_id: null })
    recordUndo(entry(t('undo.a.folderMove', { name: f.name }), () => api.post(`/folders/${f.id}/move`, { parent_id: f.parent_id ?? null }), () => api.post(`/folders/${f.id}/move`, { parent_id: null })))
    emit('changed')
  } catch (e) {
    notify.fail(e)
  }
}

const deleteDialog = ref(false)
const deleting = ref<Folder | null>(null)
function openDelete(f: Folder) {
  deleting.value = f
  deleteDialog.value = true
}
async function doDelete(cascade: boolean) {
  if (!deleting.value) return
  busy.value = true
  try {
    await api.del(`/folders/${deleting.value.id}${cascade ? '?cascade=true' : ''}`)
    if (selected.value && (selected.value === deleting.value.id || cascade)) selected.value = null
    deleteDialog.value = false
    notify.ok(t('folders.deleted'))
    emit('changed')
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}

const moveDialog = ref(false)
const moving = ref<Folder | null>(null)
const projects = ref<Project[]>([])
const targetProject = ref<string | null>(null)
async function openMoveProject(f: Folder) {
  moving.value = f
  targetProject.value = null
  dialogError.value = ''
  moveDialog.value = true
  try {
    projects.value = asItems(await api.get<{ items: Project[] }>('/projects')).filter((p) => p.id !== props.projectId)
  } catch (e) {
    dialogError.value = errorMessage(e)
  }
}
async function doMoveProject() {
  if (!moving.value || !targetProject.value) return
  busy.value = true
  try {
    await api.post(`/folders/${moving.value.id}/move`, { parent_id: null, project_id: targetProject.value })
    moveDialog.value = false
    if (selected.value === moving.value.id) selected.value = null
    notify.ok(t('folders.movedToProject'), t('folders.movedToProjectDetail'))
    emit('changed')
  } catch (e) {
    dialogError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

// ---------- several folders at once ----------
const multi = ref(false)
const checkKeys = ref<Record<string, { checked: boolean; partialChecked: boolean }>>({})
/** the checked folders that are not inside another checked folder */
const bulkIds = computed(() => topMostFolders(props.folders, checkedIds(checkKeys.value)))
const bulkPicker = ref(false)
const bulkDeleteDialog = ref(false)
const bulkProjectDialog = ref(false)
const bulkBusy = ref(false)
const bulkTargets = computed(() => {
  const skip = new Set<string>()
  for (const id of bulkIds.value) for (const d of descendantIds(props.folders, id)) skip.add(d)
  return props.folders.filter((f) => !skip.has(f.id))
})
function toggleMulti() {
  multi.value = !multi.value
  checkKeys.value = {}
}
async function bulkPost(body: Record<string, unknown>, done: string) {
  bulkBusy.value = true
  dialogError.value = ''
  try {
    const res = await api.post<{ moved?: number; deleted?: number }>(`/projects/${props.projectId}/folders/bulk`, { ids: bulkIds.value, ...body })
    const n = res.moved ?? res.deleted ?? 0
    notify.ok(t(done, { n }, n))
    if (selected.value && checkedIds(checkKeys.value).includes(selected.value) && (body.action === 'delete' || body.project_id)) selected.value = null
    checkKeys.value = {}
    bulkPicker.value = bulkDeleteDialog.value = bulkProjectDialog.value = false
    emit('changed')
  } catch (e) {
    if (bulkProjectDialog.value) dialogError.value = errorMessage(e)
    else notify.fail(e)
  } finally {
    bulkBusy.value = false
  }
}
function onBulkPick(target: string | null) {
  void bulkPost({ action: 'move', parent_id: target ?? '' }, 'folders.bulk.moved')
}
async function openBulkProject() {
  targetProject.value = null
  dialogError.value = ''
  bulkProjectDialog.value = true
  try {
    projects.value = asItems(await api.get<{ items: Project[] }>('/projects')).filter((p) => p.id !== props.projectId)
  } catch (e) {
    dialogError.value = errorMessage(e)
  }
}

defineExpose({ openCreate })
</script>

<template>
  <div>
    <div class="row nowrap" style="justify-content: space-between; margin-bottom: 6px">
      <h2 v-if="!hideTitle" class="section-title" style="margin: 4px 0">{{ t('folders.title') }}</h2>
      <span v-else />
      <span class="row nowrap">
        <Button
          v-if="selectedNode && canWrite"
          icon="pi pi-ellipsis-v"
          text
          rounded
          size="small"
          severity="secondary"
          aria-haspopup="menu"
          v-tooltip="t('folders.actions', { name: selectedNode.label })"
          :aria-label="t('folders.actions', { name: selectedNode.label })"
          @click="openActions"
        />
        <Button icon="pi pi-plus" text rounded size="small" severity="secondary" v-tooltip="t('folders.expandAll')" :aria-label="t('folders.expandAll')" @click="expanded = allKeys(nodes)" />
        <Button icon="pi pi-minus" text rounded size="small" severity="secondary" v-tooltip="t('folders.collapseAll')" :aria-label="t('folders.collapseAll')" @click="expanded = {}" />
        <Button v-if="canWrite" icon="pi pi-folder-plus" text rounded size="small" v-tooltip="t('folders.newFolder')" :aria-label="t('folders.newFolder')" @click="openCreate(selected)" />
        <Button
          v-if="canWrite && folders.length"
          icon="pi pi-check-square"
          text
          rounded
          size="small"
          :severity="multi ? undefined : 'secondary'"
          :aria-pressed="multi"
          v-tooltip="t('folders.bulk.selectSeveral')"
          :aria-label="t('folders.bulk.selectSeveral')"
          @click="toggleMulti"
        />
      </span>
    </div>
    <div v-if="multi" class="bulkbar folder-bulk" role="toolbar" :aria-label="t('folders.bulk.toolbar')">
      <b>{{ t('folders.bulk.selected', { n: bulkIds.length }, bulkIds.length) }}</b>
      <template v-if="bulkIds.length">
        <Button :label="t('folders.bulk.moveInto')" icon="pi pi-folder-open" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="bulkPicker = true" />
        <Button :label="t('folders.moveToTop')" icon="pi pi-arrow-up" size="small" severity="secondary" outlined :loading="bulkBusy" @click="bulkPost({ action: 'move', parent_id: '' }, 'folders.bulk.moved')" />
        <Button :label="t('folders.moveToProject')" icon="pi pi-send" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openBulkProject" />
        <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined aria-haspopup="dialog" @click="bulkDeleteDialog = true" />
      </template>
      <span v-else class="small muted">{{ t('folders.bulk.hint') }}</span>
      <Button :label="t('folders.bulk.done')" size="small" text @click="toggleMulti" />
    </div>
    <button
      type="button"
      class="tree-root"
      :class="{ active: !selected, 'drop-target': dropTarget === null }"
      :aria-pressed="!selected"
      @click="selected = null"
      @dragover="onDragOver($event, null)"
      @dragleave="onDragLeave(null)"
      @drop="onDrop($event, null)"
    >
      <i class="pi pi-inbox" aria-hidden="true" /> {{ kind === 'requirement' ? t('workspace.allRequirements') : t('workspace.allTests') }}
    </button>
    <Tree
      v-model:expanded-keys="expanded"
      :selection-keys="multi ? checkKeys : selectionKeys"
      :value="nodes"
      :selection-mode="multi ? 'checkbox' : 'single'"
      :pt="{ rootChildren: { 'aria-multiselectable': multi ? 'true' : undefined }, pcNodeCheckbox: { input: { 'aria-hidden': 'true', tabindex: '-1' } } }"
      @update:selection-keys="(k: Record<string, { checked: boolean; partialChecked: boolean }>) => multi && (checkKeys = k)"
      :meta-key-selection="false"
      class="w-full"
      :aria-label="t('folders.title')"
      style="padding: 0; background: transparent; border: none"
      @node-select="(n: { key?: string | number }) => !multi && onSelect(n)"
    >
      <template #default="{ node }">
        <span
          class="tree-node"
          :class="{ 'drop-target': dropTarget === node.key }"
          :draggable="canWrite"
          :title="folderPath(folders, String(node.key))"
          @dragstart="onDragStart($event, node as FolderNode)"
          @dragend="onDragEnd"
          @dragover="onDragOver($event, String(node.key))"
          @dragleave="onDragLeave(String(node.key))"
          @drop="onDrop($event, String(node.key))"
          @contextmenu.prevent="onContext($event, node as FolderNode)"
        >
          <span class="ellipsis grow">{{ node.label }}</span>
        </span>
      </template>
    </Tree>
    <div v-if="!folders.length" class="muted small" style="padding: 8px 10px">
      {{ t('folders.none') }}<template v-if="canWrite">
        <i18n-t keypath="folders.createHint" tag="span"><template #icon><i class="pi pi-folder-plus" /></template></i18n-t>
      </template>
    </div>
    <div v-if="canWrite" class="muted small" style="padding: 8px 10px">{{ t('folders.dragHint') }} {{ t('folders.keyboardHint') }}</div>
    <ContextMenu ref="menu" :model="menuItems" />
    <Menu ref="actionsMenu" :model="menuItems" popup />
    <FolderPickerDialog v-model:visible="bulkPicker" :folders="bulkTargets" :header="t('folders.bulk.moveHeader', { n: bulkIds.length }, bulkIds.length)" @pick="onBulkPick" />
    <Dialog v-model:visible="bulkDeleteDialog" :header="t('folders.bulk.deleteHeader', { n: bulkIds.length }, bulkIds.length)" modal style="width: min(520px, 95vw)">
      <p>{{ t('folders.bulk.confirmDelete', { n: bulkIds.length }, bulkIds.length) }}</p>
      <p class="muted small">{{ kind === 'requirement' ? t('folders.deleteHintRequirements') : t('folders.deleteHintTests') }}</p>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="bulkDeleteDialog = false" />
        <Button :label="t('folders.deleteKeep')" severity="warn" outlined :loading="bulkBusy" @click="bulkPost({ action: 'delete', cascade: false }, 'folders.bulk.deleted')" />
        <Button :label="t('folders.deleteAll')" severity="danger" icon="pi pi-trash" :loading="bulkBusy" @click="bulkPost({ action: 'delete', cascade: true }, 'folders.bulk.deleted')" />
      </div>
    </Dialog>
    <Dialog v-model:visible="bulkProjectDialog" :header="t('folders.bulk.projectHeader', { n: bulkIds.length }, bulkIds.length)" modal style="width: min(460px, 95vw)">
      <div class="grid" style="gap: 12px">
        <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
        <p class="small" style="margin: 0">{{ t('folders.bulk.projectHint') }}</p>
        <Select
          v-model="targetProject"
          :options="projects"
          option-value="id"
          :option-label="(p: Project) => `${p.key} · ${p.name}`"
          :placeholder="t('folders.targetProject')"
          :aria-label="t('folders.targetProject')"
          filter
          class="w-full"
        />
        <div class="row" style="justify-content: flex-end">
          <Button :label="t('common.cancel')" text severity="secondary" @click="bulkProjectDialog = false" />
          <Button :label="t('common.move')" icon="pi pi-send" :disabled="!targetProject" :loading="bulkBusy" @click="bulkPost({ action: 'move', parent_id: '', project_id: targetProject }, 'folders.bulk.movedToProject')" />
        </div>
      </div>
    </Dialog>
    <FolderPickerDialog v-model:visible="moveFolderDialog" :folders="moveTargets" :header="t('folders.moveFolderTo', { name: movingFolder?.name ?? '' })" @pick="onPickParent" />

    <Dialog v-model:visible="nameDialog" :header="nameMode === 'create' ? t('folders.newFolder') : t('folders.renameFolder')" modal style="width: min(440px, 95vw)">
      <form class="grid" style="gap: 12px" @submit.prevent="saveName">
        <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
        <div v-if="nameMode === 'create'" class="small muted">
          {{ t('folders.parent', { path: nameForm.parent ? folderPath(folders, nameForm.parent) : t('folders.topLevel') }) }}
        </div>
        <div v-field class="field"><label>{{ t('common.name') }}</label><InputText v-model="nameForm.name" autofocus required /></div>
        <div v-field class="field"><label>{{ t('common.description') }}</label><Textarea v-model="nameForm.description" rows="2" auto-resize /></div>
        <div class="row" style="justify-content: flex-end">
          <Button :label="t('common.cancel')" text severity="secondary" @click="nameDialog = false" />
          <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="!nameForm.name.trim()" />
        </div>
      </form>
    </Dialog>

    <Dialog v-model:visible="deleteDialog" :header="t('folders.deleteFolder')" modal style="width: min(520px, 95vw)">
      <i18n-t keypath="folders.confirmDelete" tag="p"><template #name><b>{{ deleting?.name }}</b></template></i18n-t>
      <p class="muted small">{{ kind === 'requirement' ? t('folders.deleteHintRequirements') : t('folders.deleteHintTests') }}</p>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="deleteDialog = false" />
        <Button :label="t('folders.deleteKeep')" severity="warn" outlined :loading="busy" @click="doDelete(false)" />
        <Button :label="t('folders.deleteAll')" severity="danger" icon="pi pi-trash" :loading="busy" @click="doDelete(true)" />
      </div>
    </Dialog>

    <Dialog v-model:visible="moveDialog" :header="t('folders.moveFolderToProject')" modal style="width: min(460px, 95vw)">
      <div class="grid" style="gap: 12px">
        <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
        <i18n-t keypath="folders.moveHint" tag="div"><template #name><b>{{ moving?.name }}</b></template></i18n-t>
        <Select
          v-model="targetProject"
          :options="projects"
          option-value="id"
          :option-label="(p: Project) => `${p.key} · ${p.name}`"
          :placeholder="t('folders.targetProject')"
          :aria-label="t('folders.targetProject')"
          filter
          class="w-full"
        />
        <div class="row" style="justify-content: flex-end">
          <Button :label="t('common.cancel')" text severity="secondary" @click="moveDialog = false" />
          <Button :label="t('common.move')" icon="pi pi-send" :disabled="!targetProject" :loading="busy" @click="doMoveProject" />
        </div>
      </div>
    </Dialog>
  </div>
</template>

<style scoped>
.folder-bulk {
  padding: 6px 8px;
  gap: 6px;
}
</style>
