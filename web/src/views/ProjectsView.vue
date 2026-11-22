<script setup lang="ts">
import ProjectColorPicker from '@/components/ProjectColorPicker.vue'
import ProjectTypeSelect from '@/components/ProjectTypeSelect.vue'
import { PROJECT_TYPE_ICONS, type ProjectType } from '@/lib/projectTypes'
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
import ProjectIcon from '@/components/ProjectIcon.vue'
import ProjectStarted from '@/components/ProjectStarted.vue'
import { projectCreator } from '@/lib/projectCreator'
import ProjectLoadDialog from '@/components/ProjectLoadDialog.vue'
import ProjectDuplicateDialog from '@/components/ProjectDuplicateDialog.vue'
import SharingDialog from '@/components/SharingDialog.vue'
import { canInProject, canShare, isRestricted } from '@/lib/sharing'
import { totalRows, type ProjectLoadResult } from '@/lib/projectArchive'
import { api, asItems, errorMessage } from '@/lib/api'
import { P } from '@/lib/permissions'
import { formatDate, relativeTime } from '@/lib/format'
import type { ListResponse, Project } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { readPref, writePref } from '@/stores/ui'
import { useI18n } from 'vue-i18n'

const auth = useAuthStore()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()

const projects = ref<Project[]>([])
const loading = ref(true)
const filter = ref('')
const layout = ref<'cards' | 'table'>((readPref('qtf.projects.layout') as 'cards' | 'table') || 'cards')
const dialog = ref(false)
const editing = ref<Project | null>(null)
const form = ref({ key: '', name: '', description: '', color: '', type: 'general', seed_categories: true })
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
    notify.fail(e, t('projects.loadFailed'))
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
  form.value = { key: '', name: '', description: '', color: '', type: 'general', seed_categories: true }
  formError.value = ''
  dialog.value = true
}
function openEdit(p: Project) {
  editing.value = p
  form.value = { key: p.key, name: p.name, description: p.description, color: p.color ?? '', type: p.type || 'general', seed_categories: false }
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
      await api.patch(`/projects/${editing.value.id}`, { name: form.value.name, description: form.value.description, color: form.value.color, type: form.value.type })
      notify.ok(t('projects.updated'))
    } else {
      const p = await api.post<Project>('/projects', form.value)
      notify.ok(t('projects.created'), p.name)
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
  notify.confirmDanger(t('projects.confirmDelete', { key: p.key, name: p.name }), async () => {
    try {
      await api.del(`/projects/${p.id}`)
      notify.ok(t('projects.deleted'))
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

function open(p: Project) {
  router.push({ name: 'dashboard', params: { projectId: p.id } })
}

// permissions in each project (its effective permissions: project sharing)
const canIn = (p: Project, perm: string | string[]) => canInProject(auth.me, p, perm)
const shareOpen = ref(false)
const sharing = ref<Project | null>(null)
function openShare(p: Project) {
  sharing.value = p
  shareOpen.value = true
}
// save, load and duplicate (project archives)
const canSave = (p: Project) => canIn(p, [P.projectUpdate, P.export])
const canDuplicate = (p: Project) => auth.can(P.projectCreate) && canIn(p, [P.projectUpdate, P.export])
const takenKeys = computed(() => projects.value.map((p) => p.key))
const loadDialog = ref(false)
const duplicateDialog = ref(false)
const duplicating = ref<Project | null>(null)
const savingId = ref('')
async function saveArchive(p: Project) {
  savingId.value = p.id
  try {
    const name = await api.download(`/projects/${p.id}/archive`, undefined, `${p.key.toLowerCase()}-project.zip`)
    notify.ok(t('projectArchive.saved'), name)
  } catch (e) {
    notify.fail(e, t('projectArchive.saveFailed'))
  } finally {
    savingId.value = ''
  }
}
function openDuplicate(p: Project) {
  duplicating.value = p
  duplicateDialog.value = true
}
function onLoaded(p: Project, res: ProjectLoadResult) {
  notify.ok(t('projectArchive.loaded'), loadedDetail(p, res))
  open(p)
}
function onDuplicated(p: Project, res: ProjectLoadResult) {
  notify.ok(t('projectArchive.duplicated'), loadedDetail(p, res))
  open(p)
}
function loadedDetail(p: Project, res: ProjectLoadResult) {
  const skipped = totalRows(res.skipped)
  return `${p.key} · ${p.name}` + (skipped ? ` · ${t('projectArchive.skipped', { n: skipped }, skipped)}` : '')
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('projects.title') }}</h1>
      <div class="sub">{{ auth.tenant?.name }}</div>
    </div>
    <div class="actions">
      <IconField>
        <InputIcon class="pi pi-filter" />
        <InputText v-model="filter" :placeholder="t('projects.filter')" :aria-label="t('projects.filter')" />
      </IconField>
      <SelectButton
        :model-value="layout"
        :options="[
          { v: 'cards', i: 'pi pi-th-large', l: t('projects.layoutCards') },
          { v: 'table', i: 'pi pi-list', l: t('projects.layoutTable') },
        ]"
        option-value="v"
        :allow-empty="false"
        :aria-label="t('projects.layout')"
        @update:model-value="setLayout"
      >
        <template #option="{ option }"><i :class="option.i" :title="option.l" aria-hidden="true" /><span class="sr-only">{{ option.l }}</span></template>
      </SelectButton>
      <Button v-if="auth.can(P.projectCreate)" :label="t('projectArchive.load')" icon="pi pi-upload" severity="secondary" outlined @click="loadDialog = true" />
      <Button v-if="auth.can(P.projectCreate)" :label="t('projects.new')" icon="pi pi-plus" @click="openCreate" />
    </div>
  </div>

  <div v-if="loading" class="charts" aria-busy="true" :aria-label="t('common.loading')">
    <Skeleton v-for="i in 3" :key="i" height="140px" />
  </div>
  <div v-else-if="!filtered.length" class="card empty">
    <i class="pi pi-folder-open" />
    <div v-if="projects.length">{{ t('projects.noMatch') }}</div>
    <div v-else>{{ t('projects.none') }}</div>
    <Button v-if="!projects.length && auth.can(P.projectCreate)" :label="t('projects.createFirst')" icon="pi pi-plus" style="margin-top: 12px" @click="openCreate" />
  </div>
  <ul v-else-if="layout === 'cards'" class="project-cards plain-list" :aria-label="t('projects.title')">
    <li v-for="p in filtered" :key="p.id" class="card clickable project-card" @click="open(p)">
      <div class="pc-head">
        <ProjectIcon :icon="p.icon" :project-key="p.key" :color="p.color" :size="40" />
        <div class="pc-title">
          <RouterLink :to="{ name: 'dashboard', params: { projectId: p.id } }" class="project-link pc-name" :title="p.name" @click.stop>{{ p.name }}</RouterLink>
          <div class="pc-badges">
            <span class="key">{{ p.key }}</span>
            <span v-if="p.type && p.type !== 'general'" class="type-badge small"><i :class="PROJECT_TYPE_ICONS[p.type as ProjectType]" aria-hidden="true" />{{ t(`projectType.${p.type}`) }}</span>
            <span v-if="isRestricted(p)" class="type-badge small restricted-badge" v-tooltip="t('sharing.restrictedTip')"><i class="pi pi-lock" aria-hidden="true" />{{ t('sharing.restricted') }}</span>
          </div>
        </div>
      </div>
      <div class="muted pc-desc">{{ p.description || t('common.noDescription') }}</div>
      <ProjectStarted :project="p" icon class="small muted pc-meta" />
      <div class="pc-foot">
        <span class="small muted pc-updated">{{ t('projects.updatedAgo', { time: relativeTime(p.updated_at || p.created_at) }) }}</span>
        <div class="pc-actions" @click.stop>
          <Button v-if="canSave(p)" icon="pi pi-download" text rounded size="small" severity="secondary" :loading="savingId === p.id" :aria-label="t('projectArchive.saveNamed', { name: p.name })" v-tooltip.top="t('projectArchive.save')" @click="saveArchive(p)" />
          <Button v-if="canDuplicate(p)" icon="pi pi-copy" text rounded size="small" severity="secondary" :aria-label="t('projectArchive.duplicateNamed', { name: p.name })" v-tooltip.top="t('projectArchive.duplicate')" @click="openDuplicate(p)" />
          <Button v-if="canShare(auth.me, p)" icon="pi pi-share-alt" text rounded size="small" severity="secondary" :aria-label="t('sharing.shareNamed', { name: p.name })" v-tooltip.top="t('sharing.share')" @click="openShare(p)" />
          <Button v-if="canIn(p, P.projectUpdate)" icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('projects.editNamed', { name: p.name })" v-tooltip.top="t('common.edit')" @click="openEdit(p)" />
          <Button v-if="canIn(p, P.projectDelete)" icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('projects.deleteNamed', { name: p.name })" v-tooltip.top="t('common.delete')" @click="remove(p)" />
        </div>
      </div>
    </li>
  </ul>
  <div v-else class="card" style="padding: 0">
    <DataTable v-stack :value="filtered" data-key="id" sort-field="key" :sort-order="1" row-hover @row-click="open($event.data)" class="clickable">
      <Column field="key" :header="t('common.key')" sortable><template #body="{ data }"><RouterLink :to="{ name: 'dashboard', params: { projectId: data.id } }" class="key-link" tabindex="-1" @click.stop><span class="key">{{ data.key }}</span></RouterLink></template></Column>
      <Column field="name" :header="t('common.name')" sortable body-class="stack-main">
        <template #body="{ data }"><span class="row nowrap" style="min-width: 0"><ProjectIcon :icon="data.icon" :project-key="data.key" :color="data.color" :size="24" /><RouterLink :to="{ name: 'dashboard', params: { projectId: data.id } }" class="project-link" @click.stop>{{ data.name }}</RouterLink><i v-if="isRestricted(data)" class="pi pi-lock muted small" v-tooltip="t('sharing.restrictedTip')" tabindex="0" role="img" :aria-label="t('sharing.restrictedTip')" /></span></template>
      </Column>
      <Column field="description" :header="t('common.description')" />
      <Column field="created_at" :header="t('common.created')" sortable><template #body="{ data }"><time :datetime="data.created_at" style="white-space: nowrap">{{ formatDate(data.created_at) }}</time></template></Column>
      <Column field="created_by_name" :header="t('projects.startedBy')" sortable>
        <template #body="{ data }"><span v-if="projectCreator(data)" :title="data.created_by_email && data.created_by_email !== projectCreator(data)?.name ? data.created_by_email : undefined">{{ projectCreator(data)?.name }}</span><span v-else class="muted">{{ t('projects.creatorUnknown') }}</span></template>
      </Column>
      <Column header="" style="width: 180px">
        <template #body="{ data }">
          <div class="row nowrap" @click.stop>
            <Button v-if="canSave(data)" icon="pi pi-download" text rounded severity="secondary" :loading="savingId === data.id" :aria-label="t('projectArchive.saveNamed', { name: data.name })" @click="saveArchive(data)" />
            <Button v-if="canDuplicate(data)" icon="pi pi-copy" text rounded severity="secondary" :aria-label="t('projectArchive.duplicateNamed', { name: data.name })" @click="openDuplicate(data)" />
            <Button v-if="canShare(auth.me, data)" icon="pi pi-share-alt" text rounded severity="secondary" :aria-label="t('sharing.shareNamed', { name: data.name })" @click="openShare(data)" />
            <Button v-if="canIn(data, P.projectUpdate)" icon="pi pi-pencil" text rounded severity="secondary" :aria-label="t('projects.editNamed', { name: data.name })" @click="openEdit(data)" />
            <Button v-if="canIn(data, P.projectDelete)" icon="pi pi-trash" text rounded severity="danger" :aria-label="t('projects.deleteNamed', { name: data.name })" @click="remove(data)" />
          </div>
        </template>
      </Column>
    </DataTable>
  </div>

  <SharingDialog v-model:visible="shareOpen" :project="sharing" @changed="load" />
  <ProjectLoadDialog v-model:visible="loadDialog" :taken-keys="takenKeys" @loaded="onLoaded" />
  <ProjectDuplicateDialog v-model:visible="duplicateDialog" :project="duplicating" :taken-keys="takenKeys" @duplicated="onDuplicated" />

  <Dialog v-model:visible="dialog" :header="editing ? t('projects.edit') : t('projects.new')" modal style="width: min(520px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div v-field class="field">
        <label>{{ t('common.name') }}</label>
        <InputText v-model="form.name" required autofocus @blur="suggestKey" />
      </div>
      <div v-field class="field">
        <label>{{ t('common.key') }}</label>
        <InputText v-model="form.key" :disabled="!!editing" required :placeholder="t('projects.keyPlaceholder')" style="text-transform: uppercase" />
        <small class="muted">{{ t('projects.keyHint', { example: (form.key || 'KEY') + '-REQ-1' }) }}</small>
      </div>
      <div v-field class="field">
        <label>{{ t('common.description') }}</label>
        <Textarea v-model="form.description" rows="3" auto-resize />
      </div>
      <div class="field">
        <label for="new-ptype">{{ t('projectType.label') }}</label>
        <ProjectTypeSelect v-model="form.type" input-id="new-ptype" />
      </div>
      <div class="field">
        <span id="new-pcolor" class="field-label">{{ t('projectColor.label') }}</span>
        <ProjectColorPicker v-model="form.color" aria-labelledby="new-pcolor" />
      </div>
      <div v-if="!editing" class="row">
        <Checkbox v-model="form.seed_categories" input-id="seed" binary />
        <label for="seed">{{ t('projects.seedCategories') }}</label>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" severity="secondary" text @click="dialog = false" />
        <Button type="submit" :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="saving" :disabled="!form.name || !form.key" />
      </div>
    </form>
  </Dialog>
</template>

<style>
.type-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 6px;
  border-radius: 999px;
  border: 1px solid var(--qtf-border);
  color: var(--qtf-muted);
}
</style>
