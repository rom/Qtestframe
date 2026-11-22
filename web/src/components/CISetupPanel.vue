<script setup lang="ts">
// Settings → Project → CI setup: how to connect a GitLab (or GitHub)
// pipeline: the CI/CD variables to define, the API token to create and the
// ready-made pipeline templates with the server URL and project key filled
// in (GET /projects/{pid}/ci/setup), to copy or download.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { saveBlob } from '@/lib/download'
import type { CISetup, CITemplate } from '@/lib/ci'
import { useNotify } from '@/composables/notify'
import { useAuthStore } from '@/stores/auth'
import { P } from '@/lib/permissions'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string }>()
const { t, te } = useI18n()
const notify = useNotify()
const auth = useAuthStore()
const setup = ref<CISetup | null>(null)
const loadError = ref('')
const kind = ref<CITemplate['kind']>('gitlab')

async function load() {
  try {
    setup.value = await api.get<CISetup>(`/projects/${props.projectId}/ci/setup`)
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(load)

const kindOptions = computed(() =>
  (setup.value?.templates ?? []).map((x) => ({ value: x.kind, label: t(`ci.setup.kinds.${x.kind}`) })),
)
const current = computed(() => setup.value?.templates.find((x) => x.kind === kind.value) ?? null)
const includeSnippet = computed(() => (current.value?.url && current.value.kind === 'gitlab' ? `include:\n  - remote: '${current.value.url}'` : ''))

async function copy(text: string | undefined) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    notify.ok(t('common.copied'))
  } catch {
    // the text can be selected by hand
  }
}
function downloadTemplate() {
  if (!current.value) return
  saveBlob(new Blob([current.value.content], { type: 'text/yaml' }), current.value.name)
}
</script>

<template>
  <section class="card ci-setup" style="max-width: 900px; margin-top: 16px" aria-labelledby="h-ci-setup">
    <div class="row" style="justify-content: space-between">
      <h2 id="h-ci-setup" class="plain-heading" style="margin-bottom: 4px">{{ t('ci.setup.title') }}</h2>
      <HelpLink page="ci-cd" />
    </div>
    <p class="small muted" style="margin: 0 0 12px">{{ t('ci.setup.intro') }}</p>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <template v-else-if="setup">
      <ol class="ci-steps small">
        <li>
          {{ t('ci.setup.step1', { role: te(`enums.role.${setup.token_role}`) ? t(`enums.role.${setup.token_role}`) : setup.token_role }) }}
          <RouterLink v-if="auth.can(P.tokenManage)" :to="{ name: 'tokens' }">{{ t('ci.setup.tokensLink') }}</RouterLink>
        </li>
        <li>{{ t('ci.setup.step2') }}</li>
        <li>{{ t('ci.setup.step3') }}</li>
        <li>{{ t('ci.setup.step4') }}</li>
      </ol>
      <div class="table-scroll" tabindex="0" role="region" :aria-label="t('ci.setup.variables')">
        <table class="ci-vars small">
          <caption class="sr-only">{{ t('ci.setup.variables') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('ci.setup.varName') }}</th>
              <th scope="col">{{ t('ci.setup.varValue') }}</th>
              <th scope="col">{{ t('ci.setup.varNotes') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="v in setup.variables" :key="v.name">
              <th scope="row"><code>{{ v.name }}</code></th>
              <td>
                <code v-if="v.value">{{ v.value }}</code>
                <span v-else class="muted">{{ t(`ci.setup.values.${v.name}`) }}</span>
              </td>
              <td>
                <span class="row nowrap" style="gap: 4px; flex-wrap: wrap">
                  <Tag v-if="v.masked" severity="warn" :value="t('ci.setup.masked')" />
                  <Tag v-if="v.required" severity="info" :value="t('ci.setup.required')" />
                  <span v-else class="muted">{{ t('ci.setup.optional') }}</span>
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <Message severity="info" :closable="false" class="small" style="margin-top: 12px">{{ t('ci.setup.jobToken') }}</Message>
      <h3 id="h-ci-template" class="small" style="margin: 16px 0 6px">{{ t('ci.setup.template') }}</h3>
      <SelectButton v-model="kind" :options="kindOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="h-ci-template" />
      <template v-if="current">
        <p class="small muted" style="margin: 8px 0 4px">{{ t(`ci.setup.kindHelp.${current.kind}`, { file: current.file }) }}</p>
        <div class="row" style="margin-bottom: 6px">
          <Button :label="t('ci.setup.copy')" icon="pi pi-copy" size="small" severity="secondary" outlined @click="copy(current.content)" />
          <Button :label="t('ci.setup.download')" icon="pi pi-download" size="small" severity="secondary" outlined @click="downloadTemplate" />
        </div>
        <pre class="ci-template" tabindex="0" :aria-label="t('ci.setup.templateLabel', { file: current.file })">{{ current.content }}</pre>
        <template v-if="includeSnippet">
          <p class="small muted" style="margin: 8px 0 4px">{{ t('ci.setup.include') }}</p>
          <div class="row nowrap">
            <pre class="ci-template ci-include grow" tabindex="0" :aria-label="t('ci.setup.include')">{{ includeSnippet }}</pre>
            <Button icon="pi pi-copy" text :aria-label="t('ci.setup.copyInclude')" @click="copy(includeSnippet)" />
          </div>
        </template>
      </template>
    </template>
  </section>
</template>

<style scoped>
.ci-steps {
  margin: 0 0 12px;
  padding-left: 20px;
  display: grid;
  gap: 4px;
}
.ci-vars {
  border-collapse: collapse;
  width: 100%;
}
.ci-vars th,
.ci-vars td {
  text-align: left;
  padding: 4px 8px;
  border-bottom: 1px solid var(--p-content-border-color);
  vertical-align: top;
}
.ci-template {
  max-height: 360px;
  overflow: auto;
  margin: 0;
  padding: 10px;
  border: 1px solid var(--p-content-border-color);
  border-radius: 6px;
  background: var(--p-content-hover-background);
  color: var(--p-text-color);
  font-size: 12px;
  white-space: pre;
}
.ci-include {
  max-height: none;
}
.table-scroll {
  overflow-x: auto;
}
</style>
