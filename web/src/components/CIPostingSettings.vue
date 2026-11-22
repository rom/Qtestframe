<script setup lang="ts">
// Settings → Project → Results back to GitLab: the server posts a commit
// status (qtestframe/tests, qtestframe/gate) and a merge request note when
// results with a commit arrive (PUT /projects/{pid}/ci-settings). The token
// is write-only: the server only says whether one is saved.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { ciForm, ciPayload, deliverySeverity, GITLAB_DEFAULT_API, shortSHA, validateCI, type CIDelivery, type CIField } from '@/lib/ci'
import type { CodeSettings } from '@/lib/variants'
import type { Release } from '@/lib/types'
import { formatDateTime } from '@/lib/format'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const notify = useNotify()
const cs = ref<CodeSettings | null>(null)
const form = ref(ciForm(null))
const releases = ref<Release[]>([])
const deliveries = ref<CIDelivery[]>([])
const loadError = ref('')
const saving = ref(false)
const touched = ref(false)

async function loadDeliveries() {
  try {
    deliveries.value = asItems(await api.get<{ items: CIDelivery[] }>(`/projects/${props.projectId}/ci/deliveries`, { limit: 10 }))
  } catch {
    deliveries.value = []
  }
}
async function load() {
  loadError.value = ''
  try {
    cs.value = await api.get<CodeSettings>(`/projects/${props.projectId}/code-settings`)
    form.value = ciForm(cs.value.ci)
    releases.value = asItems(await api.get<{ items: Release[] }>(`/projects/${props.projectId}/releases`))
  } catch (e) {
    loadError.value = errorMessage(e)
    return
  }
  await loadDeliveries()
}
onMounted(load)

const releaseOptions = computed(() => [{ value: '', label: t('ci.posting.noRelease') }, ...releases.value.map((r) => ({ value: r.id, label: r.name }))])
const problems = computed(() => validateCI(form.value, !!cs.value?.has_ci_token))
const problem = (f: CIField) => (touched.value && problems.value[f] ? t(`ci.posting.invalid.${problems.value[f]}`) : '')

async function save(clearToken = false) {
  touched.value = true
  if (!clearToken && Object.keys(problems.value).length) return
  saving.value = true
  try {
    cs.value = await api.put<CodeSettings>(`/projects/${props.projectId}/ci-settings`, ciPayload(form.value, clearToken))
    form.value = ciForm(cs.value.ci)
    touched.value = false
    notify.ok(clearToken ? t('ci.posting.tokenRemoved') : t('ci.posting.saved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="card ci-posting" style="max-width: 640px; margin-top: 16px" aria-labelledby="h-ci-posting">
    <div class="row" style="justify-content: space-between">
      <h2 id="h-ci-posting" class="plain-heading" style="margin-bottom: 4px">{{ t('ci.posting.title') }}</h2>
      <HelpLink page="ci-cd" :anchor="{ en: 'results-back-in-gitlab', sv: 'resultat-tillbaka-i-gitlab' }" />
    </div>
    <p class="small muted" style="margin: 0 0 12px">{{ t('ci.posting.intro') }}</p>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <form v-else-if="cs" class="grid" style="gap: 12px" @submit.prevent="save(false)">
      <div class="row nowrap">
        <Checkbox v-model="form.enabled" input-id="ci-enabled" binary />
        <label for="ci-enabled">{{ t('ci.posting.enabled') }}</label>
      </div>
      <template v-if="form.enabled">
        <div class="form-grid">
          <div v-field class="field">
            <label>{{ t('ci.posting.apiUrl') }}</label>
            <InputText v-model="form.api_url" type="url" class="mono" :placeholder="GITLAB_DEFAULT_API" :invalid="!!problem('api_url')" aria-describedby="ci-url-hint" />
            <span id="ci-url-hint" class="small" :class="problem('api_url') ? 'warn-text' : 'muted'">{{ problem('api_url') || t('ci.posting.apiUrlHint') }}</span>
          </div>
          <div v-field class="field">
            <label>{{ t('ci.posting.project') }}</label>
            <InputText v-model="form.project" class="mono" placeholder="group/project" :invalid="!!problem('project')" aria-describedby="ci-project-hint" />
            <span id="ci-project-hint" class="small" :class="problem('project') ? 'warn-text' : 'muted'">{{ problem('project') || t('ci.posting.projectHint') }}</span>
          </div>
        </div>
        <div class="field">
          <label for="ci-token">{{ t('ci.posting.token') }}</label>
          <Password
            v-model="form.token"
            input-id="ci-token"
            :feedback="false"
            toggle-mask
            fluid
            autocomplete="new-password"
            :placeholder="cs.has_ci_token ? t('ci.posting.tokenKept') : ''"
            :invalid="!!problem('token')"
            :input-props="{ 'aria-describedby': 'ci-token-hint' }"
          />
          <span id="ci-token-hint" class="small" :class="problem('token') ? 'warn-text' : 'muted'">{{ problem('token') || t('ci.posting.tokenHint') }}</span>
        </div>
        <fieldset class="plain-fieldset">
          <legend class="small">{{ t('ci.posting.what') }}</legend>
          <div class="row nowrap">
            <Checkbox v-model="form.post_status" input-id="ci-status" binary />
            <label for="ci-status">{{ t('ci.posting.postStatus') }}</label>
          </div>
          <div class="row nowrap">
            <Checkbox v-model="form.post_note" input-id="ci-note" binary />
            <label for="ci-note">{{ t('ci.posting.postNote') }}</label>
          </div>
        </fieldset>
        <div class="form-grid">
          <div v-field class="field">
            <label>{{ t('ci.posting.release') }}</label>
            <Select v-model="form.release_id" :options="releaseOptions" option-label="label" option-value="value" :placeholder="t('ci.posting.noRelease')" aria-describedby="ci-release-hint" />
            <span id="ci-release-hint" class="small muted">{{ t('ci.posting.releaseHint') }}</span>
          </div>
          <div v-field class="field">
            <label>{{ t('ci.posting.conditions') }}</label>
            <InputText v-model="form.conditions" class="mono" placeholder="must, no_failed, coverage>=80" aria-describedby="ci-cond-hint" />
            <span id="ci-cond-hint" class="small muted">{{ t('ci.posting.conditionsHint') }}</span>
          </div>
        </div>
      </template>
      <div class="row">
        <span class="small muted grow" role="status">
          {{ cs.ci?.provider ? t('ci.posting.active') : t('ci.posting.inactive') }}<template v-if="cs.has_ci_token"> · {{ t('ci.posting.hasToken') }}</template
          ><template v-else-if="cs.ci?.provider"> · {{ t('ci.posting.noToken') }}</template>
        </span>
        <Button v-if="cs.has_ci_token" type="button" :label="t('ci.posting.removeToken')" severity="danger" text :loading="saving" @click="save(true)" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" />
      </div>
    </form>
    <div v-if="deliveries.length" class="ci-deliveries" style="margin-top: 12px">
      <div class="row" style="justify-content: space-between">
        <h3 id="h-ci-deliveries" class="small" style="margin: 0">{{ t('ci.posting.deliveries') }}</h3>
        <Button icon="pi pi-refresh" text size="small" :aria-label="t('ci.posting.refresh')" @click="loadDeliveries" />
      </div>
      <ul class="plain-list" aria-labelledby="h-ci-deliveries">
        <li v-for="d in deliveries" :key="d.id" class="row nowrap small">
          <Tag :severity="deliverySeverity(d.status)" :value="t(`ci.posting.status.${d.status}`)" />
          <code>{{ shortSHA(d.commit_sha) }}</code>
          <span v-if="d.merge_request">!{{ d.merge_request }}</span>
          <span class="grow">{{ d.error || d.outcome }}</span>
          <time class="muted" :datetime="d.created_at">{{ formatDateTime(d.created_at) }}</time>
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.plain-fieldset {
  border: 0;
  padding: 0;
  margin: 0;
  display: grid;
  gap: 6px;
}
.plain-list {
  list-style: none;
  padding: 0;
  margin: 6px 0 0;
  display: grid;
  gap: 6px;
}
.ci-deliveries li span.grow {
  overflow-wrap: anywhere;
}
</style>
