<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import { api, errorMessage } from '@/lib/api'
import { groupBy } from '@/lib/collections'
import { defaultCategoryColor, normalizeHex, readableTextColor } from '@/lib/colors'
import { P } from '@/lib/permissions'
import type { Category, ItemKind, Project } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { defectUrl, validateTemplate } from '@/lib/defects'

const proj = useProjectStore()
const auth = useAuthStore()
const router = useRouter()
const notify = useNotify()
const tab = ref<string>('requirement')

function canEdit(scope: ItemKind) {
  return auth.can(scope === 'requirement' ? P.requirementWrite : P.testWrite) || auth.can(P.projectUpdate)
}
function groups(scope: ItemKind) {
  return groupBy(
    proj.categoriesFor(scope).sort((a, b) => a.dimension.localeCompare(b.dimension) || a.name.localeCompare(b.name)),
    (c) => c.dimension,
  )
}
const reqGroups = computed(() => groups('requirement'))
const testGroups = computed(() => groups('test'))
const color = (c: Category) => normalizeHex(c.color) ?? defaultCategoryColor(c.dimension)

async function patch(c: Category, fields: Partial<Category>) {
  try {
    await api.patch(`/categories/${c.id}`, { scope: c.scope, dimension: c.dimension, name: c.name, color: c.color, description: c.description, ...fields })
    Object.assign(c, fields)
  } catch (e) {
    notify.fail(e, 'Could not update category')
    proj.loadCategories()
  }
}
function remove(c: Category) {
  notify.confirmDanger(`Delete category "${c.dimension} / ${c.name}"? It is removed from all items.`, async () => {
    try {
      await api.del(`/categories/${c.id}`)
      await proj.loadCategories()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- create ----------
const createDialog = ref(false)
const createForm = ref({ scope: 'requirement' as ItemKind, dimension: '', name: '', color: '#6366f1', description: '', fixedDimension: false })
const dialogError = ref('')
const busy = ref(false)
function openCreate(scope: ItemKind, dimension = '') {
  createForm.value = { scope, dimension, name: '', color: dimension ? defaultCategoryColor(dimension) : '#6366f1', description: '', fixedDimension: !!dimension }
  dialogError.value = ''
  createDialog.value = true
}
async function create() {
  busy.value = true
  dialogError.value = ''
  const f = createForm.value
  try {
    await api.post(`/projects/${proj.pid}/categories`, { scope: f.scope, dimension: f.dimension.trim(), name: f.name.trim(), color: f.color, description: f.description })
    createDialog.value = false
    await proj.loadCategories()
  } catch (e) {
    dialogError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

// ---------- rename dimension ----------
const renameDialog = ref(false)
const renameForm = ref({ scope: 'requirement' as ItemKind, dimension: '', new_dimension: '' })
function openRename(scope: ItemKind, dimension: string) {
  renameForm.value = { scope, dimension, new_dimension: dimension }
  dialogError.value = ''
  renameDialog.value = true
}
async function rename() {
  busy.value = true
  dialogError.value = ''
  try {
    await api.post(`/projects/${proj.pid}/categories/move`, { ...renameForm.value, new_dimension: renameForm.value.new_dimension.trim() })
    renameDialog.value = false
    await proj.loadCategories()
  } catch (e) {
    dialogError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

// ---------- project ----------
const pform = ref({ name: proj.project?.name ?? '', description: proj.project?.description ?? '', defect_url_template: proj.project?.defect_url_template ?? '' })
const templateHint = computed(() => validateTemplate(pform.value.defect_url_template))
const templateExample = computed(() => defectUrl('ABC-123', pform.value.defect_url_template))
async function saveProject() {
  try {
    const p = await api.patch<Project>(`/projects/${proj.pid}`, { ...pform.value, defect_url_template: pform.value.defect_url_template.trim() })
    if (proj.project) Object.assign(proj.project, p ?? pform.value)
    notify.ok('Project saved')
  } catch (e) {
    notify.fail(e)
  }
}
function deleteProject() {
  notify.confirmDanger(`Delete project ${proj.project?.key} with all its data? This cannot be undone.`, async () => {
    try {
      await api.del(`/projects/${proj.pid}`)
      proj.reset()
      router.push({ name: 'projects' })
    } catch (e) {
      notify.fail(e)
    }
  })
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>Categories &amp; settings</h1>
      <div class="sub">Classify requirements and tests along dimensions such as Polarity, Interface or Level.</div>
    </div>
  </div>
  <Tabs v-model:value="tab">
    <TabList>
      <Tab value="requirement"><i class="pi pi-list-check" /> Requirement categories</Tab>
      <Tab value="test"><i class="pi pi-check-square" /> Test categories</Tab>
      <Tab value="project"><i class="pi pi-cog" /> Project</Tab>
    </TabList>
    <TabPanels style="padding: 16px 0; background: transparent">
      <TabPanel v-for="scope in ['requirement', 'test'] as ItemKind[]" :key="scope" :value="scope">
        <div class="row" style="margin-bottom: 12px">
          <Button v-if="canEdit(scope)" label="New dimension" icon="pi pi-plus" @click="openCreate(scope)" />
        </div>
        <div v-if="!(scope === 'requirement' ? reqGroups : testGroups).length" class="card empty"><i class="pi pi-tags" />No categories yet.</div>
        <div class="charts" style="grid-template-columns: repeat(auto-fill, minmax(340px, 1fr))">
          <div v-for="g in scope === 'requirement' ? reqGroups : testGroups" :key="g.key" class="card">
            <div class="row" style="justify-content: space-between; margin-bottom: 8px">
              <h3 style="margin: 0">{{ g.key }}</h3>
              <span v-if="canEdit(scope)" class="row nowrap">
                <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" v-tooltip="'Rename dimension'" aria-label="Rename dimension" @click="openRename(scope, g.key)" />
                <Button icon="pi pi-plus" text rounded size="small" v-tooltip="'Add value'" aria-label="Add value" @click="openCreate(scope, g.key)" />
              </span>
            </div>
            <div v-for="c in g.items" :key="c.id" class="row nowrap" style="padding: 4px 0">
              <input
                type="color"
                :value="color(c)"
                :disabled="!canEdit(scope)"
                style="width: 28px; height: 28px; border: none; background: none; padding: 0; cursor: pointer"
                :aria-label="'Color of ' + c.name"
                @change="(e) => patch(c, { color: (e.target as HTMLInputElement).value })"
              />
              <InputText
                :model-value="c.name"
                size="small"
                class="grow"
                :disabled="!canEdit(scope)"
                @change="(e: Event) => patch(c, { name: (e.target as HTMLInputElement).value })"
              />
              <span class="chip-cat" :style="{ background: color(c), color: readableTextColor(color(c)) }">{{ c.name }}</span>
              <Button v-if="canEdit(scope)" icon="pi pi-trash" text rounded size="small" severity="danger" aria-label="Delete" @click="remove(c)" />
            </div>
          </div>
        </div>
      </TabPanel>
      <TabPanel value="project">
        <div class="card" style="max-width: 640px">
          <div class="grid" style="gap: 12px">
            <div class="field"><label>Key</label><InputText :model-value="proj.project?.key" disabled /></div>
            <div class="field"><label>Name</label><InputText v-model="pform.name" :disabled="!auth.can(P.projectUpdate)" /></div>
            <div class="field"><label>Description</label><Textarea v-model="pform.description" rows="3" auto-resize :disabled="!auth.can(P.projectUpdate)" /></div>
            <div class="field">
              <label for="defect-tpl">Defect tracker URL template</label>
              <InputText id="defect-tpl" v-model="pform.defect_url_template" class="mono" placeholder="https://jira.example.com/browse/{id}" :disabled="!auth.can(P.projectUpdate)" :invalid="!!templateHint && !templateHint.startsWith('Without')" />
              <span class="small muted">
                Defect IDs recorded on executions link to this URL; <code>{id}</code> is replaced by the ID. Values that are URLs are linked directly.
                <template v-if="templateExample"><br />Example: ABC-123 → <a :href="templateExample" target="_blank" rel="noopener noreferrer">{{ templateExample }}</a></template>
              </span>
              <span v-if="templateHint" class="small" style="color: var(--p-amber-600)">{{ templateHint }}</span>
            </div>
            <div class="row">
              <Button v-if="auth.can(P.projectUpdate)" label="Save" icon="pi pi-check" @click="saveProject" />
              <span class="grow" />
              <Button v-if="auth.can(P.projectDelete)" label="Delete project" icon="pi pi-trash" severity="danger" outlined @click="deleteProject" />
            </div>
          </div>
        </div>
      </TabPanel>
    </TabPanels>
  </Tabs>

  <Dialog v-model:visible="createDialog" :header="createForm.fixedDimension ? `Add value to ${createForm.dimension}` : 'New category dimension'" modal style="width: min(460px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="create">
      <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
      <div v-if="!createForm.fixedDimension" class="field"><label>Dimension</label><InputText v-model="createForm.dimension" placeholder="e.g. Polarity" autofocus /></div>
      <div class="field"><label>Name</label><InputText v-model="createForm.name" placeholder="e.g. Negative" /></div>
      <div class="field">
        <label>Color</label>
        <div class="row"><input v-model="createForm.color" type="color" style="width: 40px; height: 32px; border: none; background: none" /><span class="mono">{{ createForm.color }}</span></div>
      </div>
      <div class="field"><label>Description</label><Textarea v-model="createForm.description" rows="2" auto-resize /></div>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" text severity="secondary" @click="createDialog = false" />
        <Button type="submit" label="Create" icon="pi pi-check" :loading="busy" :disabled="!createForm.dimension.trim() || !createForm.name.trim()" />
      </div>
    </form>
  </Dialog>
  <Dialog v-model:visible="renameDialog" header="Rename dimension" modal style="width: min(420px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="rename">
      <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
      <div class="small muted">All categories of “{{ renameForm.dimension }}” move to the new dimension (merging if it exists).</div>
      <div class="field"><label>New name</label><InputText v-model="renameForm.new_dimension" autofocus /></div>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" text severity="secondary" @click="renameDialog = false" />
        <Button type="submit" label="Rename" icon="pi pi-check" :loading="busy" :disabled="!renameForm.new_dimension.trim() || renameForm.new_dimension === renameForm.dimension" />
      </div>
    </form>
  </Dialog>
</template>
