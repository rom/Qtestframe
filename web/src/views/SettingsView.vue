<script setup lang="ts">
import HelpTip from '@/guide/HelpTip.vue'
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
import AvatarPicker from '@/components/AvatarPicker.vue'
import { groupBy } from '@/lib/collections'
import { defaultCategoryColor, normalizeHex, readableTextColor } from '@/lib/colors'
import { P } from '@/lib/permissions'
import type { Category, ItemKind, Project } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { defectUrl, templateProblem } from '@/lib/defects'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
// kept in script: vue-tsc 3 mis-parses a literal {id} inside a static attribute
const defectPlaceholder = 'https://jira.example.com/browse/{id}'
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
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
    notify.fail(e, t('settings.updateFailed'))
    proj.loadCategories()
  }
}
function remove(c: Category) {
  notify.confirmDanger(t('settings.confirmDeleteCategory', { dimension: c.dimension, name: c.name }), async () => {
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
const templateIssue = computed(() => templateProblem(pform.value.defect_url_template))
const templateHint = computed(() =>
  templateIssue.value === 'not_url'
    ? t('settings.templateNotUrl', { example: 'https://jira.example.com/browse/{id}' })
    : templateIssue.value === 'no_placeholder'
      ? t('settings.templateNoPlaceholder', { placeholder: '{id}' })
      : '',
)
const templateExample = computed(() => defectUrl('ABC-123', pform.value.defect_url_template))
async function saveProject() {
  try {
    const p = await api.patch<Project>(`/projects/${proj.pid}`, { ...pform.value, defect_url_template: pform.value.defect_url_template.trim() })
    if (proj.project) Object.assign(proj.project, p ?? pform.value)
    notify.ok(t('settings.projectSaved'))
  } catch (e) {
    notify.fail(e)
  }
}
async function saveIcon(icon: string) {
  const p = await api.patch<Project>(`/projects/${proj.pid}`, { icon })
  if (proj.project) Object.assign(proj.project, p ?? { icon })
}
function onIconUploaded(res: unknown) {
  const p = res as Project | null
  if (proj.project && p?.id) Object.assign(proj.project, p)
}
function deleteProject() {
  notify.confirmDanger(t('settings.confirmDeleteProject', { key: proj.project?.key ?? '' }), async () => {
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
      <h1>{{ t('settings.title') }}</h1>
      <div class="sub">{{ t('settings.subtitle') }}</div>
    </div>
  </div>
  <Tabs v-model:value="tab" scrollable>
    <TabList>
      <Tab value="requirement"><i class="pi pi-list-check" /> {{ t('settings.requirementCategories') }}</Tab>
      <Tab value="test"><i class="pi pi-check-square" /> {{ t('settings.testCategories') }}</Tab>
      <Tab value="project"><i class="pi pi-cog" /> {{ t('settings.project') }}</Tab>
    </TabList>
    <TabPanels style="padding: 16px 0; background: transparent">
      <TabPanel v-for="scope in ['requirement', 'test'] as ItemKind[]" :key="scope" :value="scope">
        <div class="row" style="margin-bottom: 12px">
          <Button v-if="canEdit(scope)" :label="t('settings.newDimension')" icon="pi pi-plus" @click="openCreate(scope)" />
        </div>
        <div v-if="!(scope === 'requirement' ? reqGroups : testGroups).length" class="card empty"><i class="pi pi-tags" />{{ t('settings.noCategories') }}</div>
        <div class="category-grid">
          <section v-for="g in scope === 'requirement' ? reqGroups : testGroups" :key="g.key" class="card" :aria-label="g.key">
            <div class="row" style="justify-content: space-between; margin-bottom: 8px">
              <h2 class="plain-heading">{{ g.key }}</h2>
              <span v-if="canEdit(scope)" class="row nowrap">
                <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" v-tooltip="t('settings.renameDimension')" :aria-label="t('settings.renameNamed', { name: g.key })" @click="openRename(scope, g.key)" />
                <Button icon="pi pi-plus" text rounded size="small" v-tooltip="t('settings.addValue')" :aria-label="t('settings.addValueTo', { dimension: g.key })" @click="openCreate(scope, g.key)" />
              </span>
            </div>
            <div v-for="c in g.items" :key="c.id" class="row nowrap" style="padding: 4px 0">
              <input
                type="color"
                :value="color(c)"
                :disabled="!canEdit(scope)"
                style="width: 28px; height: 28px; border: none; background: none; padding: 0; cursor: pointer"
                :aria-label="t('settings.colorOf', { name: c.name })"
                @change="(e) => patch(c, { color: (e.target as HTMLInputElement).value })"
              />
              <InputText
                :model-value="c.name"
                size="small"
                class="grow"
                :aria-label="t('settings.nameOf', { dimension: g.key })"
                :disabled="!canEdit(scope)"
                @change="(e: Event) => patch(c, { name: (e.target as HTMLInputElement).value })"
              />
              <span class="chip-cat" aria-hidden="true" :style="{ background: color(c), color: readableTextColor(color(c)) }">{{ c.name }}</span>
              <Button v-if="canEdit(scope)" icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('settings.deleteNamed', { name: c.name })" v-tooltip="t('common.delete')" @click="remove(c)" />
            </div>
          </section>
        </div>
      </TabPanel>
      <TabPanel value="project">
        <div class="card" style="max-width: 640px">
          <div class="grid" style="gap: 12px">
            <div v-field class="field"><label>{{ t('common.key') }}</label><InputText :model-value="proj.project?.key" disabled /></div>
            <div v-field class="field"><label>{{ t('common.name') }}</label><InputText v-model="pform.name" :disabled="!auth.can(P.projectUpdate)" /></div>
            <div v-field class="field"><label>{{ t('common.description') }}</label><Textarea v-model="pform.description" rows="3" auto-resize :disabled="!auth.can(P.projectUpdate)" /></div>
            <div v-field class="field">
              <label for="defect-tpl">{{ t('settings.defectTemplate') }}</label>
              <HelpTip id="settings.defectTemplate" :label="t('settings.defectTemplate')" />
              <InputText id="defect-tpl" v-model="pform.defect_url_template" class="mono" :placeholder="defectPlaceholder" :disabled="!auth.can(P.projectUpdate)" :invalid="templateIssue === 'not_url'" :aria-describedby="templateHint ? 'defect-tpl-hint defect-tpl-issue' : 'defect-tpl-hint'" />
              <span id="defect-tpl-hint" class="small muted">
                <i18n-t keypath="settings.defectTemplateHint" tag="span"><template #placeholder><code>{id}</code></template></i18n-t>
                <template v-if="templateExample"><br />{{ t('settings.example') }} ABC-123 → <a :href="templateExample" target="_blank" rel="noopener noreferrer">{{ templateExample }}</a></template>
              </span>
              <span v-if="templateHint" id="defect-tpl-issue" class="small warn-text" aria-live="polite">{{ templateHint }}</span>
            </div>
            <div class="row">
              <Button v-if="auth.can(P.projectUpdate)" :label="t('common.save')" icon="pi pi-check" @click="saveProject" />
              <span class="grow" />
              <Button v-if="auth.can(P.projectDelete)" :label="t('settings.deleteProject')" icon="pi pi-trash" severity="danger" outlined @click="deleteProject" />
            </div>
          </div>
        </div>
        <section class="card" style="max-width: 640px; margin-top: 16px" aria-labelledby="h-icon">
          <h2 id="h-icon" class="plain-heading" style="margin-bottom: 10px">{{ t('settings.projectIcon') }}</h2>
          <AvatarPicker kind="icon" :value="proj.project?.icon" :name="proj.project?.key ?? ''" :upload-path="`/projects/${proj.pid}/icon`" :save="saveIcon" :disabled="!auth.can(P.projectUpdate)" @uploaded="onIconUploaded" />
        </section>
      </TabPanel>
    </TabPanels>
  </Tabs>

  <Dialog v-model:visible="createDialog" :header="createForm.fixedDimension ? t('settings.addValueTo', { dimension: createForm.dimension }) : t('settings.newCategoryDimension')" modal style="width: min(460px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="create">
      <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
      <div v-if="!createForm.fixedDimension" v-field class="field"><label>{{ t('settings.dimension') }}</label><HelpTip id="settings.dimension" :label="t('settings.dimension')" /><InputText v-model="createForm.dimension" :placeholder="t('settings.dimensionPlaceholder')" autofocus /></div>
      <div v-field class="field"><label>{{ t('common.name') }}</label><InputText v-model="createForm.name" :placeholder="t('settings.namePlaceholder')" /></div>
      <div v-field class="field">
        <label>{{ t('settings.color') }}</label>
        <div class="row"><input v-model="createForm.color" :aria-label="t('settings.color')" type="color" style="width: 40px; height: 32px; border: none; background: none" /><span class="mono">{{ createForm.color }}</span></div>
      </div>
      <div v-field class="field"><label>{{ t('common.description') }}</label><Textarea v-model="createForm.description" rows="2" auto-resize /></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="createDialog = false" />
        <Button type="submit" :label="t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!createForm.dimension.trim() || !createForm.name.trim()" />
      </div>
    </form>
  </Dialog>
  <Dialog v-model:visible="renameDialog" :header="t('settings.renameDimension')" modal style="width: min(420px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="rename">
      <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
      <div class="small muted">{{ t('settings.renameHint', { dimension: renameForm.dimension }) }}</div>
      <div v-field class="field"><label>{{ t('settings.newName') }}</label><InputText v-model="renameForm.new_dimension" autofocus /></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="renameDialog = false" />
        <Button type="submit" :label="t('settings.rename')" icon="pi pi-check" :loading="busy" :disabled="!renameForm.new_dimension.trim() || renameForm.new_dimension === renameForm.dimension" />
      </div>
    </form>
  </Dialog>
</template>
