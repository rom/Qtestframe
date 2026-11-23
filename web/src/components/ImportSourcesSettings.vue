<script setup lang="ts">
// The connections requirements are imported from (GET/PUT
// /projects/{pid}/import-sources): Jira (or the defect tracker's Jira
// connection) and Azure DevOps. Tokens are write-only: the server only says
// whether one is saved.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import { api, errorMessage } from '@/lib/api'
import { trackerError } from '@/lib/defectTracker'
import { importSourcesDirty, importSourcesForm, importSourcesPayload, validateImportSources, type ImportSourceId, type ImportSourcesField } from '@/lib/importSources'
import type { ImportSources } from '@/lib/types'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const notify = useNotify()

const saved = ref<ImportSources | null>(null)
const form = ref(importSourcesForm(null))
const loadError = ref('')
const saving = ref(false)
const touched = ref(false)
const checking = ref<ImportSourceId | ''>('')
const check = ref<{ source: ImportSourceId; ok: boolean; message: string } | null>(null)

async function load() {
  loadError.value = ''
  try {
    saved.value = await api.get<ImportSources>(`/projects/${props.projectId}/import-sources`)
    form.value = importSourcesForm(saved.value)
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(load)

const problems = computed(() => validateImportSources(form.value))
const problem = (f: ImportSourcesField) => (touched.value && problems.value[f] ? t(`importSources.invalid.${problems.value[f]}`) : '')
const dirty = computed(() => importSourcesDirty(form.value, saved.value))
const jiraUsable = computed(() => !!saved.value?.jira.url || !!saved.value?.jira_from_tracker)
const azureUsable = computed(() => !!saved.value?.azure_devops.url)

async function put(clear: { jira?: boolean; azure?: boolean } = {}): Promise<boolean> {
  touched.value = true
  if (Object.keys(problems.value).length) return false
  saving.value = true
  try {
    saved.value = await api.put<ImportSources>(`/projects/${props.projectId}/import-sources`, importSourcesPayload(form.value, clear))
    form.value = importSourcesForm(saved.value)
    touched.value = false
    return true
  } catch (e) {
    notify.fail(e, t('importSources.saveFailed'))
    return false
  } finally {
    saving.value = false
  }
}
async function save() {
  check.value = null
  if (await put()) notify.ok(t('importSources.saved'))
}
async function removeToken(which: 'jira' | 'azure') {
  if (which === 'jira') form.value.jiraToken = ''
  else form.value.azureToken = ''
  if (await put({ [which]: true })) notify.ok(t('tracker.tokenRemoved'))
}
async function checkSource(source: ImportSourceId) {
  check.value = null
  if (dirty.value && !(await put())) return
  checking.value = source
  try {
    await api.post(`/projects/${props.projectId}/import-sources/${source}/check`)
    check.value = { source, ok: true, message: t('importSources.checkOk') }
  } catch (e) {
    check.value = { source, ok: false, message: trackerError(e) ?? errorMessage(e) }
  } finally {
    checking.value = ''
  }
}
</script>

<template>
  <section class="card" style="max-width: 640px; margin-top: 16px" aria-labelledby="h-import-sources">
    <h2 id="h-import-sources" class="plain-heading" style="margin-bottom: 4px">{{ t('importSources.title') }}</h2>
    <p class="small muted" style="margin: 0 0 12px">
      {{ t('importSources.intro') }} <RouterLink :to="{ name: 'trackerSync' }">{{ t('importSources.syncLink') }}</RouterLink>
    </p>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <form v-else class="grid" style="gap: 16px" @submit.prevent="save">
      <fieldset class="plain-fieldset grid" style="gap: 12px">
        <legend class="field-label">Jira</legend>
        <p v-if="saved?.tracker_jira_url && !form.jiraUrl.trim()" class="small muted" style="margin: 0">
          {{ t('importSources.jiraFromTracker', { url: saved.tracker_jira_url, key: saved.tracker_jira_key }) }}
        </p>
        <div v-field class="field">
          <label>{{ t('tracker.jiraUrl') }} {{ saved?.tracker_jira_url ? t('prompts.optional') : '' }}</label>
          <InputText v-model="form.jiraUrl" type="url" class="mono" placeholder="https://example.atlassian.net" :invalid="!!problem('jiraUrl')" aria-describedby="is-jurl-hint" />
          <span id="is-jurl-hint" class="small" :class="problem('jiraUrl') ? 'warn-text' : 'muted'">{{ problem('jiraUrl') || t('importSources.jiraUrlHint') }}</span>
        </div>
        <template v-if="form.jiraUrl.trim()">
          <div v-field class="field">
            <label>{{ t('tracker.user') }}</label>
            <InputText v-model="form.jiraUser" type="email" autocomplete="off" aria-describedby="is-juser-hint" />
            <span id="is-juser-hint" class="small muted">{{ t('tracker.userHint') }}</span>
          </div>
          <div class="field">
            <label for="is-jtoken">{{ t('tracker.token') }}</label>
            <Password
              v-model="form.jiraToken"
              input-id="is-jtoken"
              :feedback="false"
              toggle-mask
              fluid
              autocomplete="new-password"
              :placeholder="saved?.jira.has_token ? t('tracker.tokenKeep') : ''"
              :input-props="{ 'aria-describedby': 'is-jtoken-hint' }"
            />
            <span id="is-jtoken-hint" class="small muted">{{ t('importSources.jiraTokenHint') }}</span>
            <span v-if="saved?.jira.has_token" class="row small">
              <Tag :value="t('tracker.tokenSaved')" severity="success" icon="pi pi-lock" />
              <Button :label="t('tracker.removeToken')" text size="small" severity="danger" :loading="saving" @click="removeToken('jira')" />
            </span>
          </div>
        </template>
        <div v-field class="field">
          <label>{{ t('importSources.jql') }} {{ t('prompts.optional') }}</label>
          <InputText v-model="form.jiraJql" class="mono" placeholder="project = SHOP AND issuetype = Story" aria-describedby="is-jql-hint" />
          <span id="is-jql-hint" class="small muted">{{ t('importSources.jqlHint') }}</span>
        </div>
        <div class="row">
          <Button
            :label="t('tracker.check')"
            icon="pi pi-wifi"
            severity="secondary"
            outlined
            size="small"
            :loading="checking === 'jira'"
            :disabled="!jiraUsable && !form.jiraUrl.trim()"
            @click="checkSource('jira')"
          />
        </div>
        <Message v-if="check?.source === 'jira'" :severity="check.ok ? 'success' : 'error'" :closable="false" role="status">
          <b v-if="!check.ok">{{ t('tracker.checkFailed') }}</b> {{ check.message }}
        </Message>
      </fieldset>

      <fieldset class="plain-fieldset grid" style="gap: 12px">
        <legend class="field-label">Azure DevOps</legend>
        <div class="form-grid">
          <div v-field class="field">
            <label>{{ t('importSources.azureUrl') }}</label>
            <InputText v-model="form.azureUrl" type="url" class="mono" placeholder="https://dev.azure.com/example" :invalid="!!problem('azureUrl')" aria-describedby="is-aurl-hint" />
            <span id="is-aurl-hint" class="small" :class="problem('azureUrl') ? 'warn-text' : 'muted'">{{ problem('azureUrl') || t('importSources.azureUrlHint') }}</span>
          </div>
          <div v-field class="field">
            <label>{{ t('importSources.azureProject') }}</label>
            <InputText v-model="form.azureProject" placeholder="Shop" :invalid="!!problem('azureProject')" aria-describedby="is-aproj-hint" />
            <span id="is-aproj-hint" class="small warn-text">{{ problem('azureProject') }}</span>
          </div>
        </div>
        <div class="form-grid">
          <div v-field class="field">
            <label>{{ t('importSources.areaPath') }} {{ t('prompts.optional') }}</label>
            <InputText v-model="form.azureAreaPath" class="mono" placeholder="Shop\Web" aria-describedby="is-area-hint" />
            <span id="is-area-hint" class="small muted">{{ t('importSources.areaPathHint') }}</span>
          </div>
          <div v-field class="field">
            <label>{{ t('importSources.workItemTypes') }} {{ t('prompts.optional') }}</label>
            <InputText v-model="form.azureTypes" placeholder="User Story, Requirement" aria-describedby="is-types-hint" />
            <span id="is-types-hint" class="small muted">{{ t('importSources.workItemTypesHint') }}</span>
          </div>
        </div>
        <div class="field">
          <label for="is-atoken">{{ t('importSources.pat') }}</label>
          <Password
            v-model="form.azureToken"
            input-id="is-atoken"
            :feedback="false"
            toggle-mask
            fluid
            autocomplete="new-password"
            :placeholder="saved?.azure_devops.has_token ? t('tracker.tokenKeep') : ''"
            :input-props="{ 'aria-describedby': 'is-atoken-hint' }"
          />
          <span id="is-atoken-hint" class="small muted">{{ t('importSources.patHint') }}</span>
          <span v-if="saved?.azure_devops.has_token" class="row small">
            <Tag :value="t('tracker.tokenSaved')" severity="success" icon="pi pi-lock" />
            <Button :label="t('tracker.removeToken')" text size="small" severity="danger" :loading="saving" @click="removeToken('azure')" />
          </span>
        </div>
        <div class="row">
          <Button
            :label="t('tracker.check')"
            icon="pi pi-wifi"
            severity="secondary"
            outlined
            size="small"
            :loading="checking === 'azure-devops'"
            :disabled="!azureUsable && !form.azureUrl.trim()"
            @click="checkSource('azure-devops')"
          />
        </div>
        <Message v-if="check?.source === 'azure-devops'" :severity="check.ok ? 'success' : 'error'" :closable="false" role="status">
          <b v-if="!check.ok">{{ t('tracker.checkFailed') }}</b> {{ check.message }}
        </Message>
      </fieldset>
      <div class="row">
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!dirty" />
      </div>
    </form>
  </section>
</template>
