<script setup lang="ts">
// Create or edit a defect with its links to requirements, tests and
// executions. New defects can start from a draft (e.g. from a failed
// execution); links of an existing defect are saved one by one.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Message from 'primevue/message'
import ItemPickerDialog from './ItemPickerDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { DEFECT_PRIORITIES, DEFECT_SEVERITIES, DEFECT_STATUSES, diffLinks, groupLinks, linkBody, linkRoute, mergeLinks, sameLink, type DefectDraft } from '@/lib/defectTracker'
import { P } from '@/lib/permissions'
import type { Defect, DefectLink, Execution, TestRun } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ projectId: string; defect?: Defect | null; draft?: Partial<DefectDraft> | null }>()
const emit = defineEmits<{ saved: [d: Defect] }>()
const { t } = useI18n()
const auth = useAuthStore()
const canEdit = computed(() => auth.can(P.runExecute))
const isNew = computed(() => !props.defect?.id)

const empty = () => ({ title: '', description: '', status: 'open', severity: 'major', priority: 'medium', assignee: '', external_key: '', external_url: '' })
const form = ref(empty())
const links = ref<DefectLink[]>([])
const error = ref('')
const saving = ref(false)

watch(visible, (v) => {
  if (!v) return
  error.value = ''
  addingExecution.value = false
  const d = props.defect
  if (d?.id) {
    form.value = { title: d.title, description: d.description, status: d.status, severity: d.severity, priority: d.priority, assignee: d.assignee, external_key: d.external_key, external_url: d.external_url }
    links.value = [...(d.links ?? [])]
  } else {
    const x = props.draft ?? {}
    form.value = { ...empty(), title: x.title ?? '', description: x.description ?? '', severity: x.severity || 'major' }
    links.value = [...(x.links ?? [])]
  }
})

const opts = (group: 'status' | 'severity', values: readonly string[]) => values.map((v) => ({ value: v, label: t(`defectsView.${group}.${v}`) }))
const statusOptions = computed(() => opts('status', DEFECT_STATUSES))
const severityOptions = computed(() => opts('severity', DEFECT_SEVERITIES))
const priorityOptions = computed(() => DEFECT_PRIORITIES.map((v) => ({ value: v, label: enumLabel('priority', v) })))
const groups = computed(() => groupLinks(links.value))

// ---------- links ----------
const picker = ref<'requirement' | 'test' | null>(null)
/** the kind stays set while the picker closes (its items do not reload) */
const pickerKind = ref<'requirement' | 'test'>('requirement')
watch(picker, (k) => k && (pickerKind.value = k))
const pickerVisible = computed({ get: () => picker.value !== null, set: (v) => !v && (picker.value = null) })
function addPicked(items: { id: string; key?: string; title?: string }[]) {
  const type = pickerKind.value
  links.value = mergeLinks(links.value, items.map((i) => ({ entity_type: type, entity_id: i.id, key: i.key, title: i.title })))
}
function removeLink(l: DefectLink) {
  links.value = links.value.filter((x) => !sameLink(x, l))
}
const addingExecution = ref(false)
const runs = ref<TestRun[]>([])
const runId = ref<string | null>(null)
const executions = ref<Execution[]>([])
async function startExecution() {
  addingExecution.value = true
  runId.value = null
  executions.value = []
  try {
    runs.value = asItems(await api.get<{ items: TestRun[] }>(`/projects/${props.projectId}/runs`))
  } catch (e) {
    error.value = errorMessage(e)
  }
}
watch(runId, async (id) => {
  executions.value = []
  if (!id) return
  try {
    executions.value = asItems(await api.get<{ items: Execution[] }>(`/runs/${id}/executions`)).sort((a, b) => a.position - b.position)
  } catch (e) {
    error.value = errorMessage(e)
  }
})
const runOptions = computed(() => runs.value.map((r) => ({ value: r.id, label: r.name + (r.build ? ` (${r.build})` : '') })))
const executionOptions = computed(() =>
  executions.value.filter((e) => !links.value.some((l) => l.entity_type === 'execution' && l.entity_id === e.id)).map((e) => ({ value: e.id, label: `${e.test_key} ${e.test_title} · ${enumLabel('result', e.status)}` })),
)
function addExecution(id: string | null) {
  const e = executions.value.find((x) => x.id === id)
  if (!e) return
  links.value = mergeLinks(links.value, [{ entity_type: 'execution', entity_id: e.id, key: e.test_key, title: e.test_title }])
  addingExecution.value = false
}

// ---------- save ----------
async function save() {
  if (!form.value.title.trim()) return
  saving.value = true
  error.value = ''
  try {
    let d: Defect
    if (isNew.value) {
      d = await api.post<Defect>(`/projects/${props.projectId}/defects`, { ...form.value, title: form.value.title.trim(), links: links.value.map(linkBody) })
    } else {
      const id = props.defect!.id
      d = await api.patch<Defect>(`/defects/${id}`, { ...form.value, title: form.value.title.trim() })
      const diff = diffLinks(props.defect!.links ?? [], links.value)
      for (const l of diff.add) d = await api.post<Defect>(`/defects/${id}/links`, linkBody(l))
      for (const l of diff.remove) d = await api.del<Defect>(`/defects/${id}/links`, linkBody(l))
    }
    visible.value = false
    emit('saved', d)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" :header="isNew ? t('defectsView.newHeader') : t('defectsView.editHeader', { key: defect?.key ?? '' })" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(720px, 96vw)">
    <form id="defect-form" class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div v-field class="field"><label>{{ t('common.title') }}</label><InputText v-model="form.title" required autofocus :disabled="!canEdit" maxlength="300" /></div>
      <div class="form-grid">
        <div v-field class="field"><label>{{ t('common.status') }}</label><Select v-model="form.status" :options="statusOptions" option-label="label" option-value="value" :disabled="!canEdit" /></div>
        <div v-field class="field"><label>{{ t('defectsView.severityLabel') }}</label><Select v-model="form.severity" :options="severityOptions" option-label="label" option-value="value" :disabled="!canEdit" /></div>
        <div v-field class="field"><label>{{ t('common.priority') }}</label><Select v-model="form.priority" :options="priorityOptions" option-label="label" option-value="value" :disabled="!canEdit" /></div>
        <div v-field class="field"><label>{{ t('defectsView.assignee') }}</label><InputText v-model="form.assignee" :disabled="!canEdit" :placeholder="t('defectsView.assigneePlaceholder')" /></div>
      </div>
      <div v-field class="field"><label>{{ t('common.description') }}</label><Textarea v-model="form.description" rows="5" auto-resize :disabled="!canEdit" :placeholder="t('defectsView.descriptionPlaceholder')" /></div>

      <section aria-labelledby="defect-links-h">
        <h3 id="defect-links-h" class="section-title" style="margin: 0 0 6px">{{ t('defectsView.linkedItems') }}</h3>
        <div v-if="!links.length" class="small muted">{{ t('defectsView.noLinks') }}</div>
        <div v-for="g in groups" :key="g.type" style="margin-bottom: 6px">
          <div class="small muted">{{ t(`defectsView.linkType.${g.type}`) }}</div>
          <ul class="plain-list">
            <li v-for="l in g.links" :key="l.entity_type + l.entity_id" class="row nowrap" style="padding: 2px 0">
              <i :class="g.type === 'requirement' ? 'pi pi-list-check' : g.type === 'test' ? 'pi pi-check-square' : 'pi pi-play-circle'" aria-hidden="true" />
              <RouterLink v-if="linkRoute(l, projectId)" :to="linkRoute(l, projectId)!" class="key">{{ l.key || l.entity_id }}</RouterLink>
              <span v-else class="key">{{ l.key || l.entity_id }}</span>
              <span class="grow ellipsis">{{ l.title }}</span>
              <Button v-if="canEdit" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('common.removeItem', { key: l.key || l.entity_id })" @click="removeLink(l)" />
            </li>
          </ul>
        </div>
        <div v-if="canEdit" class="row" style="margin-top: 6px">
          <Button v-if="auth.can(P.requirementRead)" :label="t('defectsView.addRequirement')" icon="pi pi-list-check" size="small" severity="secondary" outlined @click="picker = 'requirement'" />
          <Button v-if="auth.can(P.testRead)" :label="t('defectsView.addTest')" icon="pi pi-check-square" size="small" severity="secondary" outlined @click="picker = 'test'" />
          <Button v-if="auth.can(P.runRead)" :label="t('defectsView.addExecution')" icon="pi pi-play-circle" size="small" severity="secondary" outlined :aria-expanded="addingExecution" @click="startExecution" />
        </div>
        <div v-if="addingExecution" class="form-grid" style="margin-top: 8px">
          <div v-field class="field"><label>{{ t('defectsView.run') }}</label><Select v-model="runId" :options="runOptions" option-label="label" option-value="value" filter :placeholder="t('defectsView.pickRun')" /></div>
          <div v-field class="field">
            <label>{{ t('defectsView.execution') }}</label>
            <Select :model-value="null" :options="executionOptions" option-label="label" option-value="value" filter :disabled="!runId" :placeholder="t('defectsView.pickExecution')" @update:model-value="addExecution" />
          </div>
        </div>
      </section>

      <details v-if="!isNew || form.external_key">
        <summary class="small">{{ t('defectsView.externalManual') }}</summary>
        <div class="form-grid" style="margin-top: 8px">
          <div v-field class="field"><label>{{ t('defectsView.externalKey') }}</label><InputText v-model="form.external_key" class="mono" :disabled="!canEdit" /></div>
          <div v-field class="field"><label>{{ t('defectsView.externalUrl') }}</label><InputText v-model="form.external_url" type="url" class="mono" :disabled="!canEdit" placeholder="https://" /></div>
        </div>
        <div class="small muted" style="margin-top: 4px">{{ t('defectsView.externalManualHint') }}</div>
      </details>
    </form>
    <template #footer>
      <Button :label="canEdit ? t('common.cancel') : t('common.close')" text severity="secondary" @click="visible = false" />
      <Button v-if="canEdit" type="submit" form="defect-form" :label="isNew ? t('defectsView.report') : t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!form.title.trim()" />
    </template>
  </Dialog>
  <!-- always mounted: the picker loads its items when it becomes visible -->
  <ItemPickerDialog
    v-model:visible="pickerVisible"
    :kind="pickerKind"
    :project-id="projectId"
    :exclude="links.filter((l) => l.entity_type === pickerKind).map((l) => l.entity_id)"
    @pick="addPicked"
  />
</template>
