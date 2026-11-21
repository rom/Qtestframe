<script setup lang="ts">
// The project's connection to Jira, GitHub Issues or GitLab (GET/PUT
// /projects/{pid}/defect-tracker). The token is write-only: the server only
// says whether one is saved.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { api, errorMessage } from '@/lib/api'
import Checkbox from 'primevue/checkbox'
import { DEFAULT_REQUIREMENT_LABELS, GITHUB_API_URL, GITLAB_URL, trackerDirty, trackerError, trackerForm, trackerPayload, validateTracker, type TrackerField } from '@/lib/defectTracker'
import type { DefectTracker } from '@/lib/types'
import { previewLabels } from '@/lib/requirementIssues'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const notify = useNotify()

const saved = ref<DefectTracker | null>(null)
const form = ref(trackerForm(null))
const loadError = ref('')
const saving = ref(false)
const checking = ref(false)
const check = ref<{ ok: boolean; message: string } | null>(null)
const touched = ref(false)

async function load() {
  loadError.value = ''
  try {
    saved.value = await api.get<DefectTracker>(`/projects/${props.projectId}/defect-tracker`)
    form.value = trackerForm(saved.value)
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(load)

const typeOptions = computed(() => [
  { value: '', label: t('tracker.typeNone') },
  { value: 'jira', label: 'Jira' },
  { value: 'github', label: 'GitHub Issues' },
  { value: 'gitlab', label: 'GitLab' },
])
const tokenHint = computed(() => t(form.value.type === 'jira' ? 'tracker.tokenHintJira' : form.value.type === 'gitlab' ? 'tracker.tokenHintGitlab' : 'tracker.tokenHintGithub'))
const reqLabelsExample = computed(() => previewLabels(form.value.requirement_labels, { type: 'functional', priority: 'high', obligation: 'must', status: 'approved', tags: ['login'] }, DEFAULT_REQUIREMENT_LABELS).join(', ') || t('reqIssue.noLabels'))
const reqLabelsHint = computed(() => t('tracker.reqLabelsHint', { fields: '{type}, {priority}, {obligation}, {status}, {tags}', default: DEFAULT_REQUIREMENT_LABELS }))
const problems = computed(() => validateTracker(form.value))
const problem = (f: TrackerField) => (touched.value && problems.value[f] ? t(`tracker.invalid.${problems.value[f]}`) : '')
const dirty = computed(() => trackerDirty(form.value, saved.value))
const configured = computed(() => !!saved.value?.type)

async function put(clearToken = false): Promise<boolean> {
  touched.value = true
  if (Object.keys(problems.value).length) return false
  saving.value = true
  try {
    saved.value = await api.put<DefectTracker>(`/projects/${props.projectId}/defect-tracker`, trackerPayload(form.value, clearToken))
    form.value = trackerForm(saved.value)
    touched.value = false
    return true
  } catch (e) {
    notify.fail(e, t('tracker.saveFailed'))
    return false
  } finally {
    saving.value = false
  }
}
async function save() {
  check.value = null
  if (await put()) notify.ok(t('tracker.saved'))
}
async function removeToken() {
  form.value.token = ''
  if (await put(true)) notify.ok(t('tracker.tokenRemoved'))
}
async function checkConnection() {
  check.value = null
  if (dirty.value && !(await put())) return
  checking.value = true
  try {
    await api.post(`/projects/${props.projectId}/defect-tracker/check`)
    check.value = { ok: true, message: t('tracker.checkOk') }
  } catch (e) {
    check.value = { ok: false, message: trackerError(e) ?? errorMessage(e) }
  } finally {
    checking.value = false
  }
}
</script>

<template>
  <section class="card" style="max-width: 640px; margin-top: 16px" aria-labelledby="h-tracker">
    <h2 id="h-tracker" class="plain-heading" style="margin-bottom: 4px">{{ t('tracker.title') }}</h2>
    <p class="small muted" style="margin: 0 0 12px">{{ t('tracker.intro') }}</p>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <form v-else class="grid" style="gap: 12px" @submit.prevent="save">
      <div v-field class="field">
        <label>{{ t('tracker.type') }}</label>
        <Select v-model="form.type" :options="typeOptions" option-label="label" option-value="value" />
      </div>
      <template v-if="form.type === 'jira'">
        <div v-field class="field">
          <label>{{ t('tracker.jiraUrl') }}</label>
          <InputText v-model="form.url" type="url" class="mono" placeholder="https://example.atlassian.net" :invalid="!!problem('url')" aria-describedby="tr-url-hint" />
          <span id="tr-url-hint" class="small" :class="problem('url') ? 'warn-text' : 'muted'">{{ problem('url') || t('tracker.jiraUrlHint') }}</span>
        </div>
        <div class="form-grid">
          <div v-field class="field">
            <label>{{ t('tracker.projectKey') }}</label>
            <InputText v-model="form.project_key" class="mono" placeholder="SHOP" :invalid="!!problem('project_key')" aria-describedby="tr-key-hint" />
            <span id="tr-key-hint" class="small warn-text">{{ problem('project_key') }}</span>
          </div>
          <div v-field class="field">
            <label>{{ t('tracker.issueType') }}</label>
            <InputText v-model="form.issue_type" placeholder="Bug" />
          </div>
        </div>
        <div v-field class="field">
          <label>{{ t('tracker.user') }}</label>
          <InputText v-model="form.user" type="email" autocomplete="off" aria-describedby="tr-user-hint" />
          <span id="tr-user-hint" class="small muted">{{ t('tracker.userHint') }}</span>
        </div>
      </template>
      <template v-else-if="form.type === 'github'">
        <div v-field class="field">
          <label>{{ t('tracker.repo') }}</label>
          <InputText v-model="form.repo" class="mono" placeholder="owner/repository" :invalid="!!problem('repo')" aria-describedby="tr-repo-hint" />
          <span id="tr-repo-hint" class="small warn-text">{{ problem('repo') }}</span>
        </div>
        <div class="form-grid">
          <div v-field class="field">
            <label>{{ t('tracker.githubUrl') }}</label>
            <InputText v-model="form.url" type="url" class="mono" :placeholder="GITHUB_API_URL" :invalid="!!problem('url')" aria-describedby="tr-gurl-hint" />
            <span id="tr-gurl-hint" class="small" :class="problem('url') ? 'warn-text' : 'muted'">{{ problem('url') || t('tracker.githubUrlHint') }}</span>
          </div>
          <div v-field class="field">
            <label>{{ t('tracker.labels') }}</label>
            <InputText v-model="form.labels" placeholder="bug, qtestframe" aria-describedby="tr-labels-hint" />
            <span id="tr-labels-hint" class="small muted">{{ t('tracker.labelsHint') }}</span>
          </div>
        </div>
      </template>
      <template v-else-if="form.type === 'gitlab'">
        <div class="form-grid">
          <div v-field class="field">
            <label>{{ t('tracker.gitlabUrl') }}</label>
            <InputText v-model="form.url" type="url" class="mono" :placeholder="GITLAB_URL" :invalid="!!problem('url')" aria-describedby="tr-glurl-hint" />
            <span id="tr-glurl-hint" class="small" :class="problem('url') ? 'warn-text' : 'muted'">{{ problem('url') || t('tracker.gitlabUrlHint') }}</span>
          </div>
          <div v-field class="field">
            <label>{{ t('tracker.gitlabProject') }}</label>
            <InputText v-model="form.project" class="mono" placeholder="group/subgroup/project" :invalid="!!problem('project')" aria-describedby="tr-glproj-hint" />
            <span id="tr-glproj-hint" class="small" :class="problem('project') ? 'warn-text' : 'muted'">{{ problem('project') || t('tracker.gitlabProjectHint') }}</span>
          </div>
        </div>
      </template>
      <template v-if="form.type === 'github' || form.type === 'gitlab'">
        <div v-if="form.type === 'gitlab'" v-field class="field">
          <label>{{ t('tracker.labels') }}</label>
          <InputText v-model="form.labels" placeholder="qtestframe" aria-describedby="tr-gllabels-hint" />
          <span id="tr-gllabels-hint" class="small muted">{{ t('tracker.labelsHint') }}</span>
        </div>
        <div class="form-grid">
          <div v-field class="field">
            <label>{{ t('tracker.milestone') }} {{ t('prompts.optional') }}</label>
            <InputText v-model="form.milestone" aria-describedby="tr-ms-hint" />
            <span id="tr-ms-hint" class="small muted">{{ t('tracker.milestoneHint') }}</span>
          </div>
          <div v-field class="field">
            <label>{{ t('tracker.assignee') }} {{ t('prompts.optional') }}</label>
            <InputText v-model="form.assignee" autocomplete="off" aria-describedby="tr-as-hint" />
            <span id="tr-as-hint" class="small muted">{{ t('tracker.assigneeHint') }}</span>
          </div>
        </div>
      </template>
      <fieldset v-if="form.type" class="plain-fieldset grid" style="gap: 12px">
        <legend class="field-label">{{ t('tracker.reqSection') }}</legend>
        <div v-field class="field">
          <label>{{ t('tracker.reqLabels') }}</label>
          <InputText v-model="form.requirement_labels" class="mono" :placeholder="DEFAULT_REQUIREMENT_LABELS" aria-describedby="tr-rl-hint" />
          <span id="tr-rl-hint" class="small muted">{{ reqLabelsHint }} {{ t('tracker.reqLabelsExample', { labels: reqLabelsExample }) }}</span>
        </div>
        <div v-if="form.type === 'jira'" v-field class="field">
          <label>{{ t('tracker.reqIssueType') }}</label>
          <InputText v-model="form.requirement_issue_type" placeholder="Task" />
        </div>
        <span class="row nowrap"
          ><Checkbox v-model="form.comment_on_pass" binary input-id="tr-cop" /><label for="tr-cop">{{ t('tracker.commentOnPass') }}</label></span
        >
      </fieldset>
      <div v-if="form.type" class="field">
        <label for="tr-token">{{ t('tracker.token') }}</label>
        <Password
          v-model="form.token"
          input-id="tr-token"
          :feedback="false"
          toggle-mask
          fluid
          autocomplete="new-password"
          :placeholder="saved?.has_token ? t('tracker.tokenKeep') : ''"
          :input-props="{ 'aria-describedby': 'tr-token-hint' }"
        />
        <span id="tr-token-hint" class="small muted">{{ tokenHint }}</span>
        <span v-if="saved?.has_token && saved.type === form.type" class="row small">
          <Tag :value="t('tracker.tokenSaved')" severity="success" icon="pi pi-lock" />
          <Button :label="t('tracker.removeToken')" text size="small" severity="danger" :loading="saving" @click="removeToken" />
        </span>
      </div>
      <Message v-if="check" :severity="check.ok ? 'success' : 'error'" :closable="false" role="status">
        <b v-if="!check.ok">{{ t('tracker.checkFailed') }}</b> {{ check.message }}
      </Message>
      <div class="row">
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!dirty" />
        <Button v-if="form.type" :label="t('tracker.check')" icon="pi pi-wifi" severity="secondary" outlined :loading="checking" :disabled="!configured && !dirty" @click="checkConnection" />
      </div>
    </form>
  </section>
</template>
