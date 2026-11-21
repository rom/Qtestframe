<script setup lang="ts">
// The project's web test settings (GET/PUT /projects/{pid}/web-tests): the
// base URL, a base URL per run environment and the test variables passed to
// every automated script. Secret values are write-only: the server only
// says whether one is saved.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import HelpTip from '@/guide/HelpTip.vue'
import { api, errorMessage } from '@/lib/api'
import { validateWebTests, variableEnvName, webTestsDirty, webTestsForm, webTestsPayload, type WebTestSettings } from '@/lib/webTests'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string; canEdit: boolean }>()
const { t } = useI18n()
const notify = useNotify()

const saved = ref<WebTestSettings | null>(null)
const form = ref(webTestsForm(null))
const loadError = ref('')
const saving = ref(false)
const urlExample = 'https://www.example.com'
const touched = ref(false)

async function load() {
  loadError.value = ''
  try {
    saved.value = await api.get<WebTestSettings>(`/projects/${props.projectId}/web-tests`)
    form.value = webTestsForm(saved.value)
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(load)

const problems = computed(() => validateWebTests(form.value))
const problem = (key: string) => (touched.value && problems.value[key] ? t(`webTests.invalid.${problems.value[key]}`) : '')
const dirty = computed(() => webTestsDirty(form.value, saved.value))
const example = computed(() => variableEnvName(form.value.variables.find((v) => v.name.trim())?.name || 'USERNAME'))

async function save() {
  touched.value = true
  if (Object.keys(problems.value).length) return
  saving.value = true
  try {
    saved.value = await api.put<WebTestSettings>(`/projects/${props.projectId}/web-tests`, webTestsPayload(form.value))
    form.value = webTestsForm(saved.value)
    touched.value = false
    notify.ok(t('webTests.saved'))
  } catch (e) {
    notify.fail(e, t('webTests.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="card web-settings" style="max-width: 860px" aria-labelledby="h-web-tests">
    <h2 id="h-web-tests" class="plain-heading" style="margin-bottom: 4px">
      {{ t('webTests.title') }} <HelpTip id="settings.webTests" :label="t('webTests.title')" />
    </h2>
    <p class="small muted" style="margin: 0 0 8px">{{ t('webTests.intro') }}</p>
    <Message v-if="saved && !saved.enabled" severity="info" :closable="false" style="margin-bottom: 12px">{{ t('webTests.serverOff') }}</Message>
    <p v-else-if="saved?.browser" class="small" style="margin: 0 0 12px">{{ t('webTests.serverOn', { browser: saved.browser }) }}</p>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <form v-else class="grid" style="gap: 16px" @submit.prevent="save">
      <div class="field">
        <label for="wt-base">{{ t('webTests.baseUrl') }}</label>
        <InputText id="wt-base" v-model="form.base_url" type="url" class="mono" placeholder="https://staging.example.com" :disabled="!canEdit" :invalid="!!problem('base_url')" aria-describedby="wt-base-hint" />
        <span id="wt-base-hint" class="small" :class="problem('base_url') ? 'warn-text' : 'muted'">{{ problem('base_url') || t('webTests.baseUrlHint') }}</span>
      </div>

      <fieldset class="plain-fieldset">
        <legend class="field-legend">{{ t('webTests.envUrls') }}</legend>
        <p class="small muted" style="margin: 0 0 6px">{{ t('webTests.envUrlsHint') }}</p>
        <p v-if="!form.environment_urls.length" class="small muted">{{ t('webTests.noEnvs') }}</p>
        <div v-for="(row, i) in form.environment_urls" :key="i" class="row-grid env-row">
          <div class="field">
            <InputText v-model="row.environment" :placeholder="t('webTests.environment')" :aria-label="t('webTests.environmentN', { n: i + 1 })" :disabled="!canEdit" :invalid="!!problem(`env.${i}.environment`)" />
            <span v-if="problem(`env.${i}.environment`)" class="small warn-text">{{ problem(`env.${i}.environment`) }}</span>
          </div>
          <div class="field">
            <InputText v-model="row.base_url" inputmode="url" class="mono" :placeholder="urlExample" :aria-label="t('webTests.urlN', { n: i + 1 })" :disabled="!canEdit" :invalid="!!problem(`env.${i}.base_url`)" />
            <span v-if="problem(`env.${i}.base_url`)" class="small warn-text">{{ problem(`env.${i}.base_url`) }}</span>
          </div>
          <Button v-if="canEdit" icon="pi pi-trash" text rounded severity="danger" size="small" :aria-label="t('webTests.removeEnvN', { n: i + 1 })" @click="form.environment_urls.splice(i, 1)" />
        </div>
        <Button v-if="canEdit" :label="t('webTests.addEnv')" icon="pi pi-plus" text size="small" @click="form.environment_urls.push({ environment: '', base_url: '' })" />
      </fieldset>

      <fieldset class="plain-fieldset">
        <legend class="field-legend">{{ t('webTests.variables') }}</legend>
        <p class="small muted" style="margin: 0 0 6px">{{ t('webTests.variablesHint', { example }) }} {{ t('webTests.secretHint') }}</p>
        <p v-if="!form.variables.length" class="small muted">{{ t('webTests.noVars') }}</p>
        <div v-for="(v, i) in form.variables" :key="i" class="row-grid var-row">
          <div class="field">
            <InputText v-model="v.name" class="mono" :placeholder="t('webTests.name')" :aria-label="t('webTests.nameN', { n: i + 1 })" :disabled="!canEdit" :invalid="!!problem(`var.${i}.name`)" />
            <span v-if="problem(`var.${i}.name`)" class="small warn-text">{{ problem(`var.${i}.name`) }}</span>
          </div>
          <div class="field">
            <Password
              v-if="v.secret"
              v-model="v.value"
              :feedback="false"
              toggle-mask
              fluid
              autocomplete="new-password"
              :placeholder="v.has_value ? t('webTests.secretKept') : t('webTests.value')"
              :input-props="{ 'aria-label': t('webTests.valueN', { n: i + 1 }) }"
              :disabled="!canEdit"
            />
            <InputText v-else v-model="v.value" :placeholder="t('webTests.value')" :aria-label="t('webTests.valueN', { n: i + 1 })" :disabled="!canEdit" :invalid="!!problem(`var.${i}.value`)" />
            <span v-if="problem(`var.${i}.value`)" class="small warn-text">{{ problem(`var.${i}.value`) }}</span>
          </div>
          <span class="row nowrap secret">
            <Checkbox v-model="v.secret" binary :input-id="`wt-secret-${i}`" :disabled="!canEdit" :aria-label="t('webTests.secretN', { n: i + 1 })" />
            <label :for="`wt-secret-${i}`" class="small">{{ t('webTests.secret') }}</label>
          </span>
          <Button v-if="canEdit" icon="pi pi-trash" text rounded severity="danger" size="small" :aria-label="t('webTests.removeVarN', { n: i + 1 })" @click="form.variables.splice(i, 1)" />
        </div>
        <Button v-if="canEdit" :label="t('webTests.addVar')" icon="pi pi-plus" text size="small" @click="form.variables.push({ name: '', value: '', secret: false })" />
      </fieldset>

      <div v-if="canEdit" class="row">
        <Button type="submit" :label="t('webTests.save')" icon="pi pi-check" :loading="saving" :disabled="!dirty" />
      </div>
    </form>
  </section>
</template>

<style scoped>
.plain-fieldset {
  border: none;
  margin: 0;
  padding: 0;
  min-width: 0;
}
.field-legend {
  font-weight: 600;
  padding: 0;
  margin-bottom: 4px;
}
.row-grid {
  display: grid;
  gap: 8px;
  align-items: start;
  margin-bottom: 8px;
}
.env-row {
  grid-template-columns: minmax(120px, 1fr) minmax(180px, 2fr) auto;
}
.var-row {
  grid-template-columns: minmax(120px, 1fr) minmax(160px, 2fr) auto auto;
}
.secret {
  gap: 6px;
  padding-top: 8px;
}
@media (max-width: 640px) {
  .env-row,
  .var-row {
    grid-template-columns: 1fr;
  }
}
</style>
