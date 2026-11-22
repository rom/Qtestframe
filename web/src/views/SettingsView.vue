<script setup lang="ts">
import HelpTip from '@/guide/HelpTip.vue'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
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
import Checkbox from 'primevue/checkbox'
import Select from 'primevue/select'
import { entry, recordUndo } from '@/lib/undoActions'
import { api, asItems, errorMessage } from '@/lib/api'
import AvatarPicker from '@/components/AvatarPicker.vue'
import ProjectColorPicker from '@/components/ProjectColorPicker.vue'
import ProjectTypeSelect from '@/components/ProjectTypeSelect.vue'
import DefectTrackerSettings from '@/components/DefectTrackerSettings.vue'
import WebhooksCard from '@/components/WebhooksCard.vue'
import VerificationSettingsCard from '@/components/VerificationSettingsCard.vue'
import ConfigurationsSettings from '@/components/ConfigurationsSettings.vue'
import WebTestSettings from '@/components/WebTestSettings.vue'
import CorrelationSettings from '@/components/CorrelationSettings.vue'
import SharingPanel from '@/components/SharingPanel.vue'
import ProjectLoadDialog from '@/components/ProjectLoadDialog.vue'
import ProjectDuplicateDialog from '@/components/ProjectDuplicateDialog.vue'
import { totalRows, type ProjectLoadResult } from '@/lib/projectArchive'
import { groupBy } from '@/lib/collections'
import { defaultCategoryColor, normalizeHex, readableTextColor } from '@/lib/colors'
import { P } from '@/lib/permissions'
import type { Category, ItemKind, ListResponse, Project } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { defectUrl, templateProblem } from '@/lib/defects'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
async function onSharingChanged() {
  try {
    await proj.refresh()
  } catch {
    router.push({ name: 'projects' })
  }
}
// kept in script: vue-tsc 3 mis-parses a literal {id} inside a static attribute
const defectPlaceholder = 'https://jira.example.com/browse/{id}'
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const route = useRoute()
const tab = ref<string>(['requirement', 'test', 'configurations', 'webtests', 'correlations', 'sharing', 'project'].includes(route.query.tab as string) ? (route.query.tab as string) : 'requirement')

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

// ---------- bulk ----------
const picked = ref<Record<ItemKind, string[]>>({ requirement: [], test: [] })
const pickedCats = (scope: ItemKind) => proj.categoriesFor(scope).filter((c) => picked.value[scope].includes(c.id))
const bulkScope = ref<ItemKind>('requirement')
const bulkDialog = ref<'' | 'move' | 'color' | 'merge'>('')
const bulkDimension = ref('')
const bulkColor = ref('#6366f1')
const bulkTarget = ref<string | null>(null)
const bulkBusy = ref(false)
const dimensionsOf = (scope: ItemKind) => [...new Set(proj.categoriesFor(scope).map((c) => c.dimension))].sort((a, b) => a.localeCompare(b))
function openBulk(scope: ItemKind, kind: 'move' | 'color' | 'merge') {
  bulkScope.value = scope
  bulkDimension.value = ''
  bulkColor.value = color(pickedCats(scope)[0])
  bulkTarget.value = pickedCats(scope)[0]?.id ?? null
  dialogError.value = ''
  bulkDialog.value = kind
}
async function bulkCall(scope: ItemKind, body: Record<string, unknown>, done: string) {
  const ids = [...picked.value[scope]]
  const before = body.action === 'color' ? pickedCats(scope).map((c) => ({ c: { ...c }, color: c.color })) : []
  bulkBusy.value = true
  dialogError.value = ''
  try {
    const res = await api.post<{ changed: number }>(`/projects/${proj.pid}/categories/bulk`, { ids, ...body })
    if (before.length) {
      recordUndo(
        entry(
          t('settings.bulk.undoColor', { n: before.length }),
          async () => {
            for (const b of before) await api.patch(`/categories/${b.c.id}`, { ...b.c, color: b.color })
            await proj.loadCategories()
          },
          async () => {
            await api.post(`/projects/${proj.pid}/categories/bulk`, { ids, ...body })
            await proj.loadCategories()
          },
        ),
      )
    }
    bulkDialog.value = ''
    picked.value[scope] = []
    notify.ok(t(done, { n: res.changed }, res.changed))
    await proj.loadCategories()
  } catch (e) {
    if (bulkDialog.value) dialogError.value = errorMessage(e)
    else notify.fail(e)
  } finally {
    bulkBusy.value = false
  }
}
function applyBulk() {
  const scope = bulkScope.value
  if (bulkDialog.value === 'move') void bulkCall(scope, { action: 'move', dimension: bulkDimension.value.trim() }, 'settings.bulk.moved')
  else if (bulkDialog.value === 'color') void bulkCall(scope, { action: 'color', color: bulkColor.value }, 'settings.bulk.colored')
  else if (bulkDialog.value === 'merge') void bulkCall(scope, { action: 'merge', target_id: bulkTarget.value }, 'settings.bulk.merged')
}
function bulkCopy(scope: ItemKind) {
  void bulkCall(scope, { action: 'copy' }, scope === 'requirement' ? 'settings.bulk.copiedToTests' : 'settings.bulk.copiedToRequirements')
}
function bulkDelete(scope: ItemKind) {
  const n = picked.value[scope].length
  notify.confirmDanger(t('settings.bulk.confirmDelete', { n }, n), () => bulkCall(scope, { action: 'delete' }, 'settings.bulk.deleted'))
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
const pform = ref({
  name: proj.project?.name ?? '',
  description: proj.project?.description ?? '',
  defect_url_template: proj.project?.defect_url_template ?? '',
  color: proj.project?.color ?? '',
  type: proj.project?.type || 'general',
  tech_context: proj.project?.tech_context ?? '',
  /** requirement sources, one per line */
  sources: (proj.project?.requirement_sources ?? []).join('\n'),
})
const sourceLines = () =>
  pform.value.sources
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
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
    const { sources, ...rest } = pform.value
    void sources
    const p = await api.patch<Project>(`/projects/${proj.pid}`, { ...rest, defect_url_template: pform.value.defect_url_template.trim(), requirement_sources: sourceLines() })
    if (proj.project && p) Object.assign(proj.project, p)
    if (p?.requirement_sources) pform.value.sources = p.requirement_sources.join('\n')
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
// save, duplicate and load (project archives)
const canSave = computed(() => auth.can([P.projectUpdate, P.export]))
const canDuplicate = computed(() => auth.can([P.projectCreate, P.projectUpdate, P.export]))
const takenKeys = ref<string[]>([])
const loadDialog = ref(false)
const duplicateDialog = ref(false)
const savingArchive = ref(false)
async function saveArchive() {
  const p = proj.project
  if (!p) return
  savingArchive.value = true
  try {
    const name = await api.download(`/projects/${p.id}/archive`, undefined, `${p.key.toLowerCase()}-project.zip`)
    notify.ok(t('projectArchive.saved'), name)
  } catch (e) {
    notify.fail(e, t('projectArchive.saveFailed'))
  } finally {
    savingArchive.value = false
  }
}
async function openArchiveDialog(which: 'load' | 'duplicate') {
  try {
    takenKeys.value = asItems(await api.get<ListResponse<Project>>('/projects')).map((p) => p.key)
  } catch {
    takenKeys.value = proj.project ? [proj.project.key] : []
  }
  if (which === 'load') loadDialog.value = true
  else duplicateDialog.value = true
}
function openArchived(p: Project, res: ProjectLoadResult, summary: string) {
  const skipped = totalRows(res.skipped)
  notify.ok(summary, `${p.key} · ${p.name}` + (skipped ? ` · ${t('projectArchive.skipped', { n: skipped }, skipped)}` : ''))
  router.push({ name: 'dashboard', params: { projectId: p.id } })
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
      <Tab value="configurations"><i class="pi pi-sliders-h" /> {{ t('configurations.tab') }}</Tab>
      <Tab value="webtests"><i class="pi pi-globe" /> {{ t('webTests.tab') }}</Tab>
      <Tab value="correlations"><i class="pi pi-share-alt" /> {{ t('correlations.settingsTab') }}</Tab>
      <Tab value="sharing"><i class="pi pi-share-alt" /> {{ t('sharing.title') }}</Tab>
      <Tab value="project"><i class="pi pi-cog" /> {{ t('settings.project') }}</Tab>
    </TabList>
    <TabPanels style="padding: 16px 0; background: transparent">
      <TabPanel v-for="scope in ['requirement', 'test'] as ItemKind[]" :key="scope" :value="scope">
        <div class="row" style="margin-bottom: 12px">
          <Button v-if="canEdit(scope)" :label="t('settings.newDimension')" icon="pi pi-plus" @click="openCreate(scope)" />
        </div>
        <div v-if="canEdit(scope) && picked[scope].length" class="bulkbar" role="toolbar" :aria-label="t('settings.bulk.toolbar')">
          <b>{{ t('common.selected', { count: picked[scope].length }) }}</b>
          <Button :label="t('settings.bulk.move')" icon="pi pi-arrow-right" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openBulk(scope, 'move')" />
          <Button :label="t('settings.bulk.color')" icon="pi pi-palette" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openBulk(scope, 'color')" />
          <Button v-if="picked[scope].length > 1" :label="t('settings.bulk.merge')" icon="pi pi-sitemap" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openBulk(scope, 'merge')" />
          <Button
            v-if="canEdit(scope === 'requirement' ? 'test' : 'requirement')"
            :label="scope === 'requirement' ? t('settings.bulk.copyToTests') : t('settings.bulk.copyToRequirements')"
            icon="pi pi-copy"
            size="small"
            severity="secondary"
            outlined
            @click="bulkCopy(scope)"
          />
          <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete(scope)" />
          <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="picked[scope] = []" />
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
              <Checkbox v-if="canEdit(scope)" v-model="picked[scope]" :value="c.id" :input-id="`cat-pick-${c.id}`" :aria-label="t('settings.bulk.selectOne', { dimension: g.key, name: c.name })" />
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
      <TabPanel value="configurations">
        <ConfigurationsSettings v-if="proj.pid && tab === 'configurations'" :project-id="proj.pid" :can-edit="auth.can(P.projectUpdate)" />
      </TabPanel>
      <TabPanel value="webtests">
        <WebTestSettings v-if="proj.pid && tab === 'webtests'" :project-id="proj.pid" :can-edit="auth.can(P.projectUpdate)" />
      </TabPanel>
      <TabPanel value="correlations">
        <CorrelationSettings v-if="proj.pid && tab === 'correlations'" :project-id="proj.pid" :can-edit="auth.can(P.projectUpdate)" />
      </TabPanel>
      <TabPanel value="sharing">
        <section v-if="proj.project && tab === 'sharing'" class="card" style="max-width: 860px" aria-labelledby="h-sharing">
          <h2 id="h-sharing" class="plain-heading" style="margin-top: 0">{{ t('sharing.title') }}</h2>
          <p class="small muted" style="margin-top: 0">{{ t('sharing.intro') }}</p>
          <SharingPanel :project="proj.project" :heading-level="3" @changed="onSharingChanged" />
        </section>
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
            <div class="field">
              <label for="ptype">{{ t('projectType.label') }}</label>
              <ProjectTypeSelect v-model="pform.type" input-id="ptype" :disabled="!auth.can(P.projectUpdate)" />
              <span class="small muted">{{ t('projectType.hint') }}</span>
            </div>
            <div v-if="pform.type === 'software'" class="field">
              <label for="ptech">{{ t('projectType.techContext') }}</label>
              <Textarea id="ptech" v-model="pform.tech_context" rows="5" auto-resize maxlength="8000" :placeholder="t('projectType.techPlaceholder')" :disabled="!auth.can(P.projectUpdate)" aria-describedby="ptech-hint" />
              <span id="ptech-hint" class="small muted">{{ t('projectType.techHint') }}</span>
            </div>
            <div class="field">
              <span id="pcolor-label" class="field-label">{{ t('projectColor.label') }}</span>
              <ProjectColorPicker v-model="pform.color" :disabled="!auth.can(P.projectUpdate)" aria-labelledby="pcolor-label" />
              <span class="small muted">{{ t('projectColor.hint') }}</span>
            </div>
            <div v-field class="field">
              <label>{{ t('settings.requirementSources') }}</label>
              <HelpTip id="settings.sources" :label="t('settings.requirementSources')" />
              <Textarea v-model="pform.sources" rows="5" auto-resize :disabled="!auth.can(P.projectUpdate)" aria-describedby="sources-hint" />
              <span id="sources-hint" class="small muted">
                {{ proj.project?.custom_sources ? t('settings.sourcesCustom') : t('settings.sourcesDefault') }}
                <Button v-if="proj.project?.custom_sources && auth.can(P.projectUpdate)" :label="t('settings.sourcesReset')" text size="small" @click="pform.sources = ''" />
              </span>
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
        <VerificationSettingsCard v-if="proj.pid" :project-id="proj.pid" :can-edit="auth.can(P.projectUpdate)" />
        <DefectTrackerSettings v-if="auth.can(P.projectUpdate) && proj.pid" :project-id="proj.pid" />
        <WebhooksCard v-if="auth.can(P.projectUpdate) && proj.pid" :project-id="proj.pid" />
        <section v-if="canSave || auth.can(P.projectCreate)" class="card project-archive" style="max-width: 640px; margin-top: 16px" aria-labelledby="h-archive">
          <div class="row nowrap" style="margin-bottom: 6px">
            <h2 id="h-archive" class="plain-heading" style="margin: 0">{{ t('projectArchive.sectionTitle') }}</h2>
            <HelpTip id="settings.archive" :label="t('projectArchive.sectionTitle')" />
          </div>
          <p class="small muted" style="margin-top: 0">{{ t('projectArchive.sectionText') }}</p>
          <div class="row">
            <Button v-if="canSave" :label="t('projectArchive.save')" icon="pi pi-download" :loading="savingArchive" @click="saveArchive" />
            <Button v-if="canDuplicate" :label="t('projectArchive.duplicate')" icon="pi pi-copy" severity="secondary" outlined @click="openArchiveDialog('duplicate')" />
            <Button v-if="auth.can(P.projectCreate)" :label="t('projectArchive.load')" icon="pi pi-upload" severity="secondary" outlined @click="openArchiveDialog('load')" />
          </div>
        </section>
        <ProjectLoadDialog v-model:visible="loadDialog" :taken-keys="takenKeys" @loaded="(p, res) => openArchived(p, res, t('projectArchive.loaded'))" />
        <ProjectDuplicateDialog v-model:visible="duplicateDialog" :project="proj.project" :taken-keys="takenKeys" @duplicated="(p, res) => openArchived(p, res, t('projectArchive.duplicated'))" />
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
  <Dialog :visible="!!bulkDialog" modal :header="bulkDialog ? t(`settings.bulk.header.${bulkDialog}`, { n: picked[bulkScope].length }, picked[bulkScope].length) : ''" :style="{ width: 'min(460px, 96vw)' }" @update:visible="(v: boolean) => !v && (bulkDialog = '')">
    <div class="grid" style="gap: 10px">
      <Message v-if="dialogError" severity="error" :closable="false">{{ dialogError }}</Message>
      <template v-if="bulkDialog === 'move'">
        <label for="bk-cat-dim" class="small" style="font-weight: 600">{{ t('settings.bulk.toDimension') }}</label>
        <Select v-model="bulkDimension" input-id="bk-cat-dim" :options="dimensionsOf(bulkScope)" editable fluid :placeholder="t('settings.bulk.dimensionPlaceholder')" aria-describedby="bk-cat-dim-help" />
        <small id="bk-cat-dim-help" class="muted">{{ t('settings.bulk.moveHelp') }}</small>
      </template>
      <template v-if="bulkDialog === 'color'">
        <label for="bk-cat-color" class="small" style="font-weight: 600">{{ t('settings.bulk.color') }}</label>
        <input id="bk-cat-color" v-model="bulkColor" type="color" style="width: 64px; height: 36px; border: none; background: none; padding: 0" />
      </template>
      <template v-if="bulkDialog === 'merge'">
        <label for="bk-cat-target" class="small" style="font-weight: 600">{{ t('settings.bulk.keep') }}</label>
        <Select v-model="bulkTarget" input-id="bk-cat-target" :options="pickedCats(bulkScope).map((c) => ({ label: `${c.dimension} / ${c.name}`, value: c.id }))" option-label="label" option-value="value" fluid aria-describedby="bk-cat-merge-help" />
        <small id="bk-cat-merge-help" class="muted">{{ t('settings.bulk.mergeHelp') }}</small>
      </template>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="bulkDialog = ''" />
      <Button :label="t('common.apply')" icon="pi pi-check" :loading="bulkBusy" :disabled="(bulkDialog === 'move' && !bulkDimension.trim()) || (bulkDialog === 'merge' && !bulkTarget)" @click="applyBulk" />
    </template>
  </Dialog>
</template>
