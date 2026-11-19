<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Checkbox from 'primevue/checkbox'
import SelectButton from 'primevue/selectbutton'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Skeleton from 'primevue/skeleton'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Message from 'primevue/message'
import { api, asItems, errorMessage } from '@/lib/api'
import { P } from '@/lib/permissions'
import { formatDate, relativeTime } from '@/lib/format'
import type { ListResponse, Project } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { readPref, writePref } from '@/stores/ui'

const auth = useAuthStore()
const router = useRouter()
const notify = useNotify()

const projects = ref<Project[]>([])
const loading = ref(true)
const filter = ref('')
const layout = ref<'cards' | 'table'>((readPref('qtf.projects.layout') as 'cards' | 'table') || 'cards')
const dialog = ref(false)
const editing = ref<Project | null>(null)
const form = ref({ key: '', name: '', description: '', seed_categories: true })
const formError = ref('')
const saving = ref(false)

const filtered = computed(() => {
  const f = filter.value.trim().toLowerCase()
  if (!f) return projects.value
  return projects.value.filter((p) => [p.key, p.name, p.description].some((s) => s?.toLowerCase().includes(f)))
})

async function load() {
  if (!auth.tenant) {
    loading.value = false
    return
  }
  loading.value = true
  try {
    projects.value = asItems(await api.get<ListResponse<Project>>('/projects'))
  } catch (e) {
    notify.fail(e, 'Could not load projects')
  } finally {
    loading.value = false
  }
}
onMounted(load)

function setLayout(v: 'cards' | 'table') {
  layout.value = v
  writePref('qtf.projects.layout', v)
}

function openCreate() {
  editing.value = null
  form.value = { key: '', name: '', description: '', seed_categories: true }
  formError.value = ''
  dialog.value = true
}
function openEdit(p: Project) {
  editing.value = p
  form.value = { key: p.key, name: p.name, description: p.description, seed_categories: false }
  formError.value = ''
  dialog.value = true
}

function suggestKey() {
  if (editing.value || form.value.key) return
  const words = form.value.name.trim().split(/\s+/).filter(Boolean)
  const k = words.length > 1 ? words.map((w) => w[0]).join('') : (words[0] ?? '').slice(0, 4)
  form.value.key = k.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 8)
}

async function save() {
  saving.value = true
  formError.value = ''
  try {
    if (editing.value) {
      await api.patch(`/projects/${editing.value.id}`, { name: form.value.name, description: form.value.description })
      notify.ok('Project updated')
    } else {
      const p = await api.post<Project>('/projects', form.value)
      notify.ok('Project created', p.name)
    }
    dialog.value = false
    await load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

function remove(p: Project) {
  notify.confirmDanger(`Delete project ${p.key} "${p.name}" with all requirements, tests and runs? This cannot be undone.`, async () => {
    try {
      await api.del(`/projects/${p.id}`)
      notify.ok('Project deleted')
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

function open(p: Project) {
  router.push({ name: 'dashboard', params: { projectId: p.id } })
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>Projects</h1>
      <div class="sub">{{ auth.tenant?.name }}</div>
    </div>
    <div class="actions">
      <IconField>
        <InputIcon class="pi pi-filter" />
        <InputText v-model="filter" placeholder="Filter projects" />
      </IconField>
      <SelectButton
        :model-value="layout"
        :options="[
          { v: 'cards', i: 'pi pi-th-large' },
          { v: 'table', i: 'pi pi-list' },
        ]"
        option-value="v"
        :allow-empty="false"
        aria-label="Layout"
        @update:model-value="setLayout"
      >
        <template #option="{ option }"><i :class="option.i" /></template>
      </SelectButton>
      <Button v-if="auth.can(P.projectCreate)" label="New project" icon="pi pi-plus" @click="openCreate" />
    </div>
  </div>

  <div v-if="loading" class="charts">
    <Skeleton v-for="i in 3" :key="i" height="140px" />
  </div>
  <div v-else-if="!filtered.length" class="card empty">
    <i class="pi pi-folder-open" />
    <div v-if="projects.length">No project matches the filter.</div>
    <div v-else>No projects yet.</div>
    <Button v-if="!projects.length && auth.can(P.projectCreate)" label="Create the first project" icon="pi pi-plus" style="margin-top: 12px" @click="openCreate" />
  </div>
  <div v-else-if="layout === 'cards'" class="charts" style="grid-template-columns: repeat(auto-fill, minmax(280px, 1fr))">
    <div v-for="p in filtered" :key="p.id" class="card clickable" style="display: flex; flex-direction: column; gap: 8px" @click="open(p)">
      <div class="row nowrap">
        <div style="width: 40px; height: 40px; border-radius: 10px; background: color-mix(in srgb, var(--p-primary-color) 15%, transparent); color: var(--p-primary-color); display: grid; place-items: center; font-weight: 700; font-size: 12px; flex: none">
          {{ p.key.slice(0, 4) }}
        </div>
        <div class="grow">
          <div style="font-weight: 650" class="ellipsis">{{ p.name }}</div>
          <div class="key">{{ p.key }}</div>
        </div>
        <div class="row nowrap" @click.stop>
          <Button v-if="auth.can(P.projectUpdate)" icon="pi pi-pencil" text rounded severity="secondary" aria-label="Edit" @click="openEdit(p)" />
          <Button v-if="auth.can(P.projectDelete)" icon="pi pi-trash" text rounded severity="danger" aria-label="Delete" @click="remove(p)" />
        </div>
      </div>
      <div class="muted" style="min-height: 38px; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden">
        {{ p.description || 'No description' }}
      </div>
      <div class="small muted">Updated {{ relativeTime(p.updated_at || p.created_at) }}</div>
    </div>
  </div>
  <div v-else class="card" style="padding: 0">
    <DataTable :value="filtered" data-key="id" sort-field="key" :sort-order="1" row-hover @row-click="open($event.data)" class="clickable">
      <Column field="key" header="Key" sortable><template #body="{ data }"><span class="key">{{ data.key }}</span></template></Column>
      <Column field="name" header="Name" sortable />
      <Column field="description" header="Description" />
      <Column field="created_at" header="Created" sortable><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column header="" style="width: 100px">
        <template #body="{ data }">
          <div class="row nowrap" @click.stop>
            <Button v-if="auth.can(P.projectUpdate)" icon="pi pi-pencil" text rounded severity="secondary" aria-label="Edit" @click="openEdit(data)" />
            <Button v-if="auth.can(P.projectDelete)" icon="pi pi-trash" text rounded severity="danger" aria-label="Delete" @click="remove(data)" />
          </div>
        </template>
      </Column>
    </DataTable>
  </div>

  <Dialog v-model:visible="dialog" :header="editing ? 'Edit project' : 'New project'" modal style="width: min(520px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field">
        <label>Name</label>
        <InputText v-model="form.name" required autofocus @blur="suggestKey" />
      </div>
      <div class="field">
        <label>Key</label>
        <InputText v-model="form.key" :disabled="!!editing" required placeholder="e.g. PAY" style="text-transform: uppercase" />
        <small class="muted">Prefix for item keys like {{ form.key || 'KEY' }}-REQ-1. Cannot be changed later.</small>
      </div>
      <div class="field">
        <label>Description</label>
        <Textarea v-model="form.description" rows="3" auto-resize />
      </div>
      <div v-if="!editing" class="row">
        <Checkbox v-model="form.seed_categories" input-id="seed" binary />
        <label for="seed">Seed default categories (Polarity, Interface, Medium, Execution, Level, Area)</label>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" severity="secondary" text @click="dialog = false" />
        <Button type="submit" :label="editing ? 'Save' : 'Create'" icon="pi pi-check" :loading="saving" :disabled="!form.name || !form.key" />
      </div>
    </form>
  </Dialog>
</template>
