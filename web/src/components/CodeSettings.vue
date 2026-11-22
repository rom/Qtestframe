<script setup lang="ts">
// Settings → Project → Code integration: the webhook URLs to enter in GitLab
// and GitHub and the project's shared webhook secret (write-only).
import { onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { randomSecret, type CodeSettings } from '@/lib/variants'
import { formatDateTime } from '@/lib/format'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const notify = useNotify()
const cs = ref<CodeSettings | null>(null)
const loadError = ref('')
const secret = ref('')
const busy = ref(false)
const shown = ref('')

async function load() {
  try {
    cs.value = await api.get<CodeSettings>(`/projects/${props.projectId}/code-settings`)
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(load)

async function save(clear = false) {
  busy.value = true
  try {
    cs.value = await api.put<CodeSettings>(`/projects/${props.projectId}/code-settings`, clear ? { clear_secret: true } : { secret: secret.value })
    shown.value = clear ? '' : secret.value
    secret.value = ''
    notify.ok(clear ? t('code.settings.cleared') : t('code.settings.saved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
function generate() {
  secret.value = randomSecret()
}
async function copy(v: string | undefined) {
  if (!v) return
  try {
    await navigator.clipboard.writeText(v)
    notify.ok(t('common.copied'))
  } catch {
    // the field can be selected by hand
  }
}
</script>

<template>
  <section class="card" style="max-width: 640px; margin-top: 16px" aria-labelledby="h-code-settings">
    <div class="row" style="justify-content: space-between">
      <h2 id="h-code-settings" class="plain-heading" style="margin-bottom: 4px">{{ t('code.settings.title') }}</h2>
      <HelpLink page="variants-and-code" :anchor="{ en: 'webhooks', sv: 'webhookar' }" />
    </div>
    <p class="small muted" style="margin: 0 0 12px">{{ t('code.settings.intro') }}</p>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <div v-else-if="cs" class="grid" style="gap: 12px">
      <div class="field">
        <label for="cs-gitlab">{{ t('code.settings.gitlabUrl') }}</label>
        <div class="row nowrap">
          <InputText id="cs-gitlab" :model-value="cs.gitlab_url" readonly class="mono grow" />
          <Button icon="pi pi-copy" severity="secondary" text :aria-label="t('code.settings.copyGitlab')" @click="copy(cs.gitlab_url)" />
        </div>
      </div>
      <div class="field">
        <label for="cs-github">{{ t('code.settings.githubUrl') }}</label>
        <div class="row nowrap">
          <InputText id="cs-github" :model-value="cs.github_url" readonly class="mono grow" />
          <Button icon="pi pi-copy" severity="secondary" text :aria-label="t('code.settings.copyGithub')" @click="copy(cs.github_url)" />
        </div>
      </div>
      <form class="field" @submit.prevent="save(false)">
        <label for="cs-secret">{{ t('code.settings.secret') }}</label>
        <div class="row nowrap">
          <InputText id="cs-secret" v-model="secret" class="mono grow" type="password" autocomplete="new-password" :placeholder="cs.has_secret ? t('code.settings.secretKept') : ''" aria-describedby="cs-secret-help" />
          <Button type="button" :label="t('code.settings.generate')" icon="pi pi-refresh" severity="secondary" outlined @click="generate" />
        </div>
        <small id="cs-secret-help" class="muted">{{ t('code.settings.secretHelp') }}</small>
        <div v-if="secret" class="small mono" style="word-break: break-all">{{ secret }}</div>
        <Message v-if="shown" severity="info" :closable="false">{{ t('code.settings.copyNow') }} <code style="word-break: break-all">{{ shown }}</code></Message>
        <div class="row" style="margin-top: 8px">
          <span class="small muted grow" role="status">{{ cs.has_secret ? t('code.settings.active') : t('code.settings.inactive') }}<template v-if="cs.updated_at"> · {{ formatDateTime(cs.updated_at) }}</template></span>
          <Button v-if="cs.has_secret" type="button" :label="t('code.settings.clear')" severity="danger" text :loading="busy" @click="save(true)" />
          <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="secret.length < 16" />
        </div>
      </form>
    </div>
  </section>
</template>
