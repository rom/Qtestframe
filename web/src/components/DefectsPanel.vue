<script setup lang="ts">
// Defects linked to a requirement, test or execution (GET …/{id}/defects):
// a requirement also sees the defects of its tests and their executions,
// a test those of its executions ("via"). Testers report a new defect with
// the item already linked, or link an existing one.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Select from 'primevue/select'
import Message from 'primevue/message'
import DefectTag from './DefectTag.vue'
import DefectFormDialog from './DefectFormDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { countOpen, sortDefects, viaKind, type DefectDraft } from '@/lib/defectTracker'
import { P } from '@/lib/permissions'
import type { Defect, LinkedDefect } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  entityType: 'requirement' | 'test' | 'execution'
  entityId: string
  projectId: string
  /** prefill of "Report defect" (default: just the link to this item) */
  draft?: () => Partial<DefectDraft>
}>()
/** count: open defects */
const emit = defineEmits<{ count: [n: number]; created: [d: Defect] }>()
const { t } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const canReport = computed(() => auth.can(P.runExecute))

const items = ref<LinkedDefect[]>([])
const loading = ref(false)
const error = ref('')
const base = computed(() => `/${props.entityType === 'requirement' ? 'requirements' : props.entityType === 'test' ? 'tests' : 'executions'}/${props.entityId}/defects`)

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = sortDefects(asItems(await api.get<{ items: LinkedDefect[] }>(base.value)))
    emit('count', countOpen(items.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.entityId, load, { immediate: true })

const via = (d: LinkedDefect) => viaKind(d.via, props.entityType)

// ---------- report ----------
const formVisible = ref(false)
const draft = ref<Partial<DefectDraft> | null>(null)
function report() {
  const own = { entity_type: props.entityType, entity_id: props.entityId }
  const d = props.draft?.() ?? {}
  draft.value = { ...d, links: d.links?.length ? d.links : [own] }
  formVisible.value = true
}
async function onCreated(d: Defect) {
  notify.ok(t('defectsView.created'), d.key)
  emit('created', d)
  await load()
}

// ---------- link existing ----------
const linkVisible = ref(false)
const candidates = ref<Defect[]>([])
const chosen = ref<string | null>(null)
const linkError = ref('')
const linking = ref(false)
async function openLink() {
  chosen.value = null
  linkError.value = ''
  linkVisible.value = true
  try {
    const have = new Set(items.value.filter((d) => via(d) === 'direct').map((d) => d.id))
    candidates.value = sortDefects(asItems(await api.get<{ items: Defect[] }>(`/projects/${props.projectId}/defects`)).filter((d) => !have.has(d.id)))
  } catch (e) {
    linkError.value = errorMessage(e)
  }
}
const candidateOptions = computed(() => candidates.value.map((d) => ({ value: d.id, label: `${d.key} ${d.title}` })))
async function link() {
  if (!chosen.value) return
  linking.value = true
  linkError.value = ''
  try {
    const d = await api.post<Defect>(`/defects/${chosen.value}/links`, { entity_type: props.entityType, entity_id: props.entityId })
    linkVisible.value = false
    notify.ok(t('defectsView.linked'), d?.key)
    await load()
  } catch (e) {
    linkError.value = errorMessage(e)
  } finally {
    linking.value = false
  }
}
async function unlink(d: LinkedDefect) {
  try {
    await api.del(`/defects/${d.id}/links`, { entity_type: props.entityType, entity_id: props.entityId })
    notify.ok(t('defectsView.unlinked'), d.key)
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
defineExpose({ report, reload: load })
</script>

<template>
  <div class="grid" style="gap: 10px" :aria-busy="loading">
    <div class="row" style="justify-content: space-between">
      <span class="small muted">{{ entityType === 'requirement' ? t('defectsView.panelHintRequirement') : entityType === 'test' ? t('defectsView.panelHintTest') : t('defectsView.panelHintExecution') }}</span>
      <span v-if="canReport" class="row">
        <Button :label="t('defectsView.linkExisting')" icon="pi pi-link" size="small" severity="secondary" outlined @click="openLink" />
        <Button :label="t('defectsView.report')" icon="pi pi-exclamation-circle" size="small" severity="danger" @click="report" />
      </span>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <ul v-else-if="items.length" class="plain-list defect-panel-list">
      <li v-for="d in items" :key="d.id" class="defect-panel-row">
        <div class="row nowrap" style="min-width: 0">
          <RouterLink :to="{ name: 'defects', params: { projectId }, query: { item: d.id } }" class="key">{{ d.key }}</RouterLink>
          <span class="grow ellipsis" :title="d.title">{{ d.title }}</span>
        </div>
        <div class="row small" style="gap: 6px">
          <DefectTag :value="d.status" />
          <DefectTag :value="d.severity" kind="severity" />
          <span class="muted">{{ via(d) === 'direct' ? t('defectsView.viaDirect') : via(d) === 'test' ? t('defectsView.viaTest') : t('defectsView.viaExecution') }}</span>
          <a v-if="d.external_url" :href="d.external_url" target="_blank" rel="noopener noreferrer" class="mono">
            {{ d.external_key }}<i class="pi pi-external-link" aria-hidden="true" style="font-size: 0.75em; margin-left: 2px" /><span class="sr-only"> ({{ t('links.newTab') }})</span>
          </a>
          <span v-else-if="d.external_key" class="mono">{{ d.external_key }}</span>
          <span v-if="d.external_status" class="muted">({{ d.external_status }})</span>
          <Button
            v-if="canReport && via(d) === 'direct'"
            icon="pi pi-times"
            text
            rounded
            size="small"
            severity="secondary"
            :aria-label="t('defectsView.unlinkNamed', { key: d.key })"
            v-tooltip="t('common.unlink')"
            @click="unlink(d)"
          />
        </div>
      </li>
    </ul>
    <div v-else-if="!loading" class="muted small">{{ t('defectsView.noneLinked') }}</div>
  </div>

  <DefectFormDialog v-model:visible="formVisible" :project-id="projectId" :draft="draft" @saved="onCreated" />
  <Dialog v-model:visible="linkVisible" :header="t('defectsView.linkExisting')" modal style="width: min(520px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="link">
      <Message v-if="linkError" severity="error" :closable="false">{{ linkError }}</Message>
      <div v-field class="field">
        <label>{{ t('defectsView.defect') }}</label>
        <Select v-model="chosen" :options="candidateOptions" option-label="label" option-value="value" filter :placeholder="t('defectsView.pickDefect')" :empty-message="t('defectsView.noCandidates')" />
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="linkVisible = false" />
        <Button type="submit" :label="t('common.link')" icon="pi pi-link" :loading="linking" :disabled="!chosen" />
      </div>
    </form>
  </Dialog>
</template>

<style>
.defect-panel-row {
  display: grid;
  gap: 4px;
  padding: 8px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.defect-panel-row:last-child {
  border-bottom: 0;
}
/* "2 open defects" next to the requirement's key (opens the defects tab) */
.open-defects-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 999px;
  border: 1px solid var(--p-red-300, #fca5a5);
  background: color-mix(in srgb, var(--p-red-500, #ef4444) 12%, transparent);
  color: var(--p-red-700, #b91c1c);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.app-dark .open-defects-badge {
  color: var(--p-red-300, #fca5a5);
}
.open-defects-badge:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
</style>
