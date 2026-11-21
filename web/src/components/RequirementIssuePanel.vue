<script setup lang="ts">
// The requirement's issue in the project's tracker (GitLab, GitHub or
// Jira): create it from the requirement (with a preview), or, once it
// exists, open it, push the current text into it and refresh its state.
// GET/POST/PUT /requirements/{id}/issue and POST …/issue/sync. Nothing is
// shown when the project has no tracker.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { api, errorMessage } from '@/lib/api'
import { trackerError, trackerName } from '@/lib/defectTracker'
import { existingIssue, issueStateTag } from '@/lib/requirementIssues'
import { formatDateTime } from '@/lib/format'
import type { ItemLink, RequirementIssueInfo } from '@/lib/types'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ requirementId: string; canWrite: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const notify = useNotify()

const info = ref<RequirementIssueInfo | null>(null)
const loadError = ref('')
const busy = ref<'' | 'create' | 'update' | 'sync'>('')
const dialog = ref(false)
const dialogError = ref('')
/** the issue a create found (409 issue_exists) */
const found = ref<ItemLink | null>(null)

const tracker = computed(() => trackerName(info.value?.tracker))
const link = computed(() => info.value?.link ?? null)

async function load() {
  loadError.value = ''
  try {
    info.value = await api.get<RequirementIssueInfo>(`/requirements/${props.requirementId}/issue`)
  } catch (e) {
    info.value = null
    loadError.value = errorMessage(e)
  }
}
watch(() => props.requirementId, load, { immediate: true })

function fail(e: unknown) {
  notify.fail(e, t('reqIssue.failed'))
}

function openCreate() {
  dialogError.value = ''
  found.value = null
  dialog.value = true
  void load() // a fresh preview
}
async function create() {
  busy.value = 'create'
  dialogError.value = ''
  try {
    const res = await api.post<{ link: ItemLink }>(`/requirements/${props.requirementId}/issue`)
    dialog.value = false
    notify.ok(t('reqIssue.created', { key: res.link.issue?.key ?? '' }))
    await load()
    emit('changed')
  } catch (e) {
    const ex = existingIssue(e)
    if (ex) {
      found.value = ex
      await load()
    } else dialogError.value = trackerError(e) ?? errorMessage(e)
  } finally {
    busy.value = ''
  }
}
async function update() {
  busy.value = 'update'
  try {
    const res = await api.put<{ link: ItemLink }>(`/requirements/${props.requirementId}/issue`)
    dialog.value = false
    notify.ok(t('reqIssue.updated', { key: res.link.issue?.key ?? '' }))
    await load()
    emit('changed')
  } catch (e) {
    fail(e)
  } finally {
    busy.value = ''
  }
}
async function sync() {
  busy.value = 'sync'
  try {
    const res = await api.post<{ link: ItemLink; changed: boolean; commented: boolean }>(`/requirements/${props.requirementId}/issue/sync`)
    const state = res.link.issue?.state ?? ''
    notify.ok(res.changed ? t('reqIssue.stateChanged', { state: t(`reqIssue.state.${state}`) }) : t('reqIssue.syncedOk'), res.commented ? t('reqIssue.commented') : undefined)
    await load()
    emit('changed')
  } catch (e) {
    fail(e)
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <section v-if="info?.tracker" class="req-issue" :aria-label="t('reqIssue.title', { tracker })">
    <div class="row" style="justify-content: space-between; gap: 8px; flex-wrap: wrap">
      <h3 class="plain-heading small-heading">{{ t('reqIssue.title', { tracker }) }}</h3>
      <Button v-if="!link && canWrite" :label="t('reqIssue.create', { tracker })" icon="pi pi-ticket" size="small" outlined aria-haspopup="dialog" @click="openCreate" />
    </div>
    <p v-if="!link" class="small muted" style="margin: 4px 0 0">{{ t('reqIssue.none', { tracker }) }}</p>
    <div v-else class="row nowrap" style="gap: 8px; margin-top: 4px; flex-wrap: wrap">
      <a :href="link.url" target="_blank" rel="noopener noreferrer" class="link-text"
        >{{ link.issue?.key }} <i class="pi pi-external-link small" aria-hidden="true" /><span class="sr-only"> ({{ t('links.newTab') }})</span></a
      >
      <Tag :value="t(`reqIssue.state.${link.issue?.state || 'unknown'}`)" :severity="issueStateTag(link.issue?.state)" />
      <span v-if="link.issue?.status && link.issue.status !== link.issue.state" class="small muted">{{ link.issue.status }}</span>
      <span v-if="link.issue?.synced_at" class="small muted">{{ t('reqIssue.synced', { time: formatDateTime(link.issue.synced_at) }) }}</span>
      <span class="grow" />
      <template v-if="canWrite">
        <Button :label="t('reqIssue.update')" icon="pi pi-upload" size="small" text :loading="busy === 'update'" :disabled="!!busy" @click="update" />
        <Button :label="t('reqIssue.sync')" icon="pi pi-refresh" size="small" text :loading="busy === 'sync'" :disabled="!!busy" @click="sync" />
      </template>
    </div>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>

    <Dialog v-model:visible="dialog" modal :header="t('reqIssue.create', { tracker })" style="width: min(760px, 95vw)">
      <div class="grid" style="gap: 12px">
        <Message v-if="dialogError" severity="error" :closable="false" role="alert"><b>{{ t('reqIssue.failed') }}:</b> {{ dialogError }}</Message>
        <template v-if="found || link">
          <Message severity="warn" :closable="false">{{ t('reqIssue.exists', { key: (found ?? link)?.issue?.key ?? '' }) }}</Message>
          <p class="small muted" style="margin: 0">{{ t('reqIssue.updateHint') }}</p>
          <div class="row" style="justify-content: flex-end">
            <Button :label="t('common.cancel')" severity="secondary" text @click="dialog = false" />
            <a :href="(found ?? link)?.url" target="_blank" rel="noopener noreferrer" class="p-button p-button-secondary p-button-outlined" style="text-decoration: none"
              ><i class="pi pi-external-link" aria-hidden="true" />&nbsp;{{ t('reqIssue.open') }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a
            >
            <Button :label="t('reqIssue.update')" icon="pi pi-upload" :loading="busy === 'update'" @click="update" />
          </div>
        </template>
        <template v-else-if="info?.preview">
          <p class="small muted" style="margin: 0">{{ t('reqIssue.previewHint') }}</p>
          <div class="field">
            <span class="field-label">{{ t('reqIssue.previewTitle') }}</span>
            <span class="mono">{{ info.preview.title }}</span>
          </div>
          <div class="field">
            <span class="field-label">{{ t('reqIssue.previewLabels') }}</span>
            <span v-if="info.preview.labels.length" class="row" style="gap: 4px; flex-wrap: wrap">
              <Tag v-for="l in info.preview.labels" :key="l" :value="l" severity="secondary" />
            </span>
            <span v-else class="small muted">{{ t('reqIssue.noLabels') }}</span>
          </div>
          <div class="field">
            <span id="req-issue-desc" class="field-label">{{ t('reqIssue.previewDescription') }}</span>
            <pre class="req-issue-preview" tabindex="0" aria-labelledby="req-issue-desc">{{ info.preview.description }}</pre>
          </div>
          <div class="row" style="justify-content: flex-end">
            <Button :label="t('common.cancel')" severity="secondary" text @click="dialog = false" />
            <Button :label="t('reqIssue.createButton')" icon="pi pi-check" :loading="busy === 'create'" @click="create" />
          </div>
        </template>
      </div>
    </Dialog>
  </section>
</template>

<style>
.req-issue {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--p-content-border-color, #ddd);
}
.req-issue-preview {
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  margin: 0;
  padding: 8px;
  border: 1px solid var(--p-content-border-color, #ddd);
  border-radius: 6px;
}
</style>
