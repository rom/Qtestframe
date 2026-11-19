<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Tree from 'primevue/tree'
import ContextMenu from 'primevue/contextmenu'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Message from 'primevue/message'
import { api, asItems, errorMessage } from '@/lib/api'
import { allKeys, ancestorKeys, buildTree, canMoveFolder, folderPath, type FolderNode } from '@/lib/tree'
import type { Folder, ItemKind, Project } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { decodeIds, FOLDER_MIME, ITEMS_MIME } from '@/lib/dnd'


const props = defineProps<{ kind: ItemKind; projectId: string; folders: Folder[]; canWrite: boolean }>()
const selected = defineModel<string | null>('selected', { default: null })
const emit = defineEmits<{ changed: []; dropItems: [ids: string[], folderId: string | null] }>()
const notify = useNotify()

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
  if (!fid || !canMoveFolder(props.folders, fid, target)) return
  try {
    await api.post(`/folders/${fid}/move`, { parent_id: target })
    if (target) expanded.value[target] = true
    notify.ok('Folder moved')
    emit('changed')
  } catch (err) {
    notify.fail(err, 'Could not move folder')
  }
}

// ---------- context menu & dialogs ----------
const menu = ref<InstanceType<typeof ContextMenu>>()
const ctxNode = ref<FolderNode | null>(null)
const menuItems = computed(() => [
  { label: 'New subfolder', icon: 'pi pi-folder-plus', command: () => openCreate(ctxNode.value?.key ?? null), disabled: !props.canWrite },
  { label: 'Rename', icon: 'pi pi-pencil', command: () => ctxNode.value && openRename(ctxNode.value.data), disabled: !props.canWrite },
  { label: 'Move to project…', icon: 'pi pi-send', command: () => ctxNode.value && openMoveProject(ctxNode.value.data), disabled: !props.canWrite },
  { label: 'Move to top level', icon: 'pi pi-arrow-up', command: () => ctxNode.value && moveToRoot(ctxNode.value.data), disabled: !props.canWrite || !ctxNode.value?.data.parent_id },
  { separator: true },
  { label: 'Expand all', icon: 'pi pi-plus', command: () => (expanded.value = allKeys(nodes.value)) },
  { label: 'Collapse all', icon: 'pi pi-minus', command: () => (expanded.value = {}) },
  { separator: true },
  { label: 'Delete…', icon: 'pi pi-trash', class: 'p-error', command: () => ctxNode.value && openDelete(ctxNode.value.data), disabled: !props.canWrite },
])
function onContext(e: MouseEvent, node: FolderNode) {
  ctxNode.value = node
  menu.value?.show(e)
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
      const f = await api.post<Folder>(`/projects/${props.projectId}/folders`, {
        kind: props.kind,
        name: nameForm.value.name,
        description: nameForm.value.description,
        parent_id: nameForm.value.parent,
      })
      if (nameForm.value.parent) expanded.value[nameForm.value.parent] = true
      if (f?.id) selected.value = f.id
    } else {
      await api.patch(`/folders/${nameForm.value.id}`, { name: nameForm.value.name, description: nameForm.value.description })
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
    notify.ok('Folder deleted')
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
    notify.ok('Folder moved to project', 'Contained items were moved as well.')
    emit('changed')
  } catch (e) {
    dialogError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

defineExpose({ openCreate })
</script>

<template>
  <div>
    <div class="row nowrap" style="justify-content: space-between; margin-bottom: 6px">
      <span class="section-title" style="margin: 4px 0">Folders</span>
      <span class="row nowrap">
        <Button icon="pi pi-plus" text rounded size="small" severity="secondary" v-tooltip="'Expand all'" aria-label="Expand all" @click="expanded = allKeys(nodes)" />
        <Button icon="pi pi-minus" text rounded size="small" severity="secondary" v-tooltip="'Collapse all'" aria-label="Collapse all" @click="expanded = {}" />
        <Button v-if="canWrite" icon="pi pi-folder-plus" text rounded size="small" v-tooltip="'New folder'" aria-label="New folder" @click="openCreate(selected)" />
      </span>
    </div>
    <div
      class="tree-root"
      :class="{ active: !selected, 'drop-target': dropTarget === null }"
      @click="selected = null"
      @dragover="onDragOver($event, null)"
      @dragleave="onDragLeave(null)"
      @drop="onDrop($event, null)"
    >
      <i class="pi pi-inbox" /> All {{ kind === 'requirement' ? 'requirements' : 'tests' }}
    </div>
    <Tree
      v-model:expanded-keys="expanded"
      :selection-keys="selectionKeys"
      :value="nodes"
      selection-mode="single"
      :meta-key-selection="false"
      class="w-full"
      style="padding: 0; background: transparent; border: none"
      @node-select="onSelect"
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
      No folders yet.<template v-if="canWrite"> Use <i class="pi pi-folder-plus" /> to create one.</template>
    </div>
    <div v-if="canWrite" class="muted small" style="padding: 8px 10px">Drag folders to nest them; drag rows here to file them. Right-click for more.</div>
    <ContextMenu ref="menu" :model="menuItems" />

    <Dialog v-model:visible="nameDialog" :header="nameMode === 'create' ? 'New folder' : 'Rename folder'" modal style="width: min(440px, 95vw)">
      <form class="grid" style="gap: 12px" @submit.prevent="saveName">
        <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
        <div v-if="nameMode === 'create'" class="small muted">
          Parent: {{ nameForm.parent ? folderPath(folders, nameForm.parent) : 'top level' }}
        </div>
        <div class="field"><label>Name</label><InputText v-model="nameForm.name" autofocus required /></div>
        <div class="field"><label>Description</label><Textarea v-model="nameForm.description" rows="2" auto-resize /></div>
        <div class="row" style="justify-content: flex-end">
          <Button label="Cancel" text severity="secondary" @click="nameDialog = false" />
          <Button type="submit" label="Save" icon="pi pi-check" :loading="busy" :disabled="!nameForm.name.trim()" />
        </div>
      </form>
    </Dialog>

    <Dialog v-model:visible="deleteDialog" header="Delete folder" modal style="width: min(480px, 95vw)">
      <p>
        Delete <b>{{ deleting?.name }}</b>?
      </p>
      <p class="muted small">
        Keep content moves subfolders and items to the parent folder. Delete everything removes all subfolders and contained
        {{ kind === 'requirement' ? 'requirements' : 'tests' }}.
      </p>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" text severity="secondary" @click="deleteDialog = false" />
        <Button label="Delete, keep content" severity="warn" outlined :loading="busy" @click="doDelete(false)" />
        <Button label="Delete everything" severity="danger" icon="pi pi-trash" :loading="busy" @click="doDelete(true)" />
      </div>
    </Dialog>

    <Dialog v-model:visible="moveDialog" header="Move folder to project" modal style="width: min(460px, 95vw)">
      <div class="grid" style="gap: 12px">
        <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
        <div>
          Move <b>{{ moving?.name }}</b> including all subfolders and items to another project. Links to items that stay behind may
          be removed by the server.
        </div>
        <Select
          v-model="targetProject"
          :options="projects"
          option-value="id"
          :option-label="(p: Project) => `${p.key} · ${p.name}`"
          placeholder="Target project"
          filter
          class="w-full"
        />
        <div class="row" style="justify-content: flex-end">
          <Button label="Cancel" text severity="secondary" @click="moveDialog = false" />
          <Button label="Move" icon="pi pi-send" :disabled="!targetProject" :loading="busy" @click="doMoveProject" />
        </div>
      </div>
    </Dialog>
  </div>
</template>
