<script setup lang="ts">
// "Generate test code" of the tests list: code stubs of the selected tests
// (or of the tests the list shows) for a test framework, named after the
// test keys so the JUnit results map back (POST /projects/{pid}/tests/stubs,
// see internal/stubgen). Previews the files, downloads them as a zip (or
// the one file) and can set the tests' script to the stub path.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { saveBlob } from '@/lib/download'
import { STUB_FRAMEWORKS, stubsCommand, type StubFrameworkId, type StubRequest, type StubResponse } from '@/lib/stubs'
import { useNotify } from '@/composables/notify'
import { readPref, writePref } from '@/stores/ui'

const props = defineProps<{
  projectId: string
  /** the selected tests (ids); empty: the tests of the query */
  ids: string[]
  /** the query of the list (used without a selection) */
  query: string
  /** how many tests the list shows */
  shown: number
  canWrite: boolean
}>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ marked: [count: number] }>()
const { t } = useI18n()
const notify = useNotify()

const PREF = 'qtf.stubs.framework'
const framework = ref<StubFrameworkId>((STUB_FRAMEWORKS.find((f) => f.id === readPref(PREF))?.id ?? 'pytest') as StubFrameworkId)
const dir = ref('tests')
const pkg = ref('')
const markScript = ref(false)
const res = ref<StubResponse | null>(null)
const loadError = ref('')
const loading = ref(false)
const busy = ref(false)
const current = ref('')
const MAX_LISTED = 200

const fw = computed(() => STUB_FRAMEWORKS.find((f) => f.id === framework.value)!)
const frameworkOptions = STUB_FRAMEWORKS.map((f) => ({ value: f.id, label: f.name }))
const scopeText = computed(() =>
  props.ids.length ? t('stubs.scopeSelected', { count: props.ids.length }, props.ids.length) : t('stubs.scopeList', { count: props.shown }, props.shown),
)
const files = computed(() => res.value?.files ?? [])
const fileOptions = computed(() => files.value.slice(0, MAX_LISTED).map((f) => ({ value: f.path, label: `${f.path} (${f.key})` })))
const currentFile = computed(() => files.value.find((f) => f.path === current.value) ?? files.value[0] ?? null)
const command = computed(() =>
  stubsCommand({
    framework: framework.value,
    query: props.query,
    keys: props.ids.length ? files.value.map((f) => f.key) : undefined,
    dir: dir.value,
    pkg: fw.value.package ? pkg.value : '',
    markScript: markScript.value,
  }),
)

function request(extra: Partial<StubRequest>): StubRequest {
  const body: StubRequest = { framework: framework.value, format: 'json', dir: dir.value.trim(), ...extra }
  if (fw.value.package && pkg.value.trim()) body.package = pkg.value.trim()
  if (props.ids.length) body.ids = props.ids
  else body.query = props.query
  return body
}

let seq = 0
async function preview() {
  const mine = ++seq
  loading.value = true
  loadError.value = ''
  try {
    const out = await api.post<StubResponse>(`/projects/${props.projectId}/tests/stubs`, request({}))
    if (mine !== seq) return
    res.value = out
    if (!out.files.some((f) => f.path === current.value)) current.value = out.files[0]?.path ?? ''
  } catch (e) {
    if (mine === seq) {
      res.value = null
      loadError.value = errorMessage(e)
    }
  } finally {
    if (mine === seq) loading.value = false
  }
}

watch(
  visible,
  (v) => {
    if (!v) return
    markScript.value = false
    res.value = null
    void preview()
  },
  { immediate: true },
)
let timer: ReturnType<typeof setTimeout> | undefined
watch([framework, dir, pkg], () => {
  writePref(PREF, framework.value)
  if (!visible.value) return
  clearTimeout(timer)
  timer = setTimeout(() => void preview(), 250)
})

async function mark() {
  if (!markScript.value || !props.canWrite) return
  const out = await api.post<StubResponse>(`/projects/${props.projectId}/tests/stubs`, request({ mark_script: true }))
  notify.ok(t('stubs.marked', { count: out.marked }, out.marked))
  if (out.marked) emit('marked', out.marked)
}

async function downloadZip() {
  busy.value = true
  try {
    await mark()
    await api.postDownload(`/projects/${props.projectId}/tests/stubs`, request({ format: 'zip' }), `${framework.value}-stubs.zip`)
  } catch (e) {
    notify.fail(e, t('stubs.failed'))
  } finally {
    busy.value = false
  }
}

async function downloadFile() {
  const f = currentFile.value
  if (!f) return
  busy.value = true
  try {
    await mark()
    saveBlob(new Blob([f.content], { type: 'text/plain' }), f.path.split('/').pop() || f.path)
  } catch (e) {
    notify.fail(e, t('stubs.failed'))
  } finally {
    busy.value = false
  }
}

async function copy(text: string | undefined) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    notify.ok(t('common.copied'))
  } catch {
    // the text can be selected by hand
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('stubs.title')" style="width: min(900px, 96vw)" class="stubs-dialog">
    <div class="grid" style="gap: 12px">
      <div class="row" style="justify-content: space-between; align-items: flex-start">
        <p class="small muted" style="margin: 0">{{ t('stubs.intro') }}</p>
        <HelpLink page="ci-cd" :anchor="{ en: 'generate-test-code', sv: 'generera-testkod' }" />
      </div>
      <p class="small" style="margin: 0" data-testid="stubs-scope">
        <b>{{ scopeText }}</b>
        <span v-if="!ids.length && query" class="mono"> · {{ t('stubs.scopeQuery', { query }) }}</span>
      </p>
      <div class="stubs-options">
        <div class="field">
          <label for="stubs-framework" class="block-label">{{ t('stubs.framework') }}</label>
          <Select v-model="framework" input-id="stubs-framework" :options="frameworkOptions" option-label="label" option-value="value" style="width: 100%" />
        </div>
        <div class="field">
          <label for="stubs-dir" class="block-label">{{ t('stubs.dir') }}</label>
          <InputText id="stubs-dir" v-model="dir" aria-describedby="stubs-dir-hint" style="width: 100%" spellcheck="false" />
          <small id="stubs-dir-hint" class="muted">{{ t('stubs.dirHint') }}</small>
        </div>
        <div v-if="fw.package" class="field">
          <label for="stubs-package" class="block-label">{{ t('stubs.package') }}</label>
          <InputText id="stubs-package" v-model="pkg" aria-describedby="stubs-package-hint" style="width: 100%" spellcheck="false" />
          <small id="stubs-package-hint" class="muted">{{ fw.package === 'go' ? t('stubs.packageHintGo') : t('stubs.packageHintJava') }}</small>
        </div>
      </div>
      <div v-if="canWrite" class="row nowrap" style="align-items: flex-start">
        <Checkbox v-model="markScript" binary input-id="stubs-mark" aria-describedby="stubs-mark-hint" />
        <span>
          <label for="stubs-mark">{{ t('stubs.markScript') }}</label>
          <br /><small id="stubs-mark-hint" class="muted">{{ t('stubs.markScriptHint') }}</small>
        </span>
      </div>
      <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
      <div v-else-if="res && !files.length" role="status"><Message severity="info" :closable="false">{{ t('stubs.none') }}</Message></div>
      <section v-else-if="files.length" class="grid" style="gap: 6px" aria-labelledby="stubs-preview-h">
        <h3 id="stubs-preview-h" class="small" style="margin: 0">{{ t('stubs.preview') }}</h3>
        <p class="small muted" style="margin: 0" role="status">{{ t('stubs.fileCount', { count: files.length }, files.length) }}</p>
        <div class="row" style="flex-wrap: wrap">
          <label for="stubs-file" class="sr-only">{{ t('stubs.file') }}</label>
          <Select v-model="current" input-id="stubs-file" :options="fileOptions" option-label="label" option-value="value" class="grow" style="min-width: 240px" />
          <Button :label="t('stubs.copy')" icon="pi pi-copy" size="small" severity="secondary" outlined :disabled="!currentFile" @click="copy(currentFile?.content)" />
        </div>
        <pre v-if="currentFile" class="stubs-code" tabindex="0" :aria-label="t('stubs.fileLabel', { file: currentFile.path })" data-testid="stubs-code">{{ currentFile.content }}</pre>
      </section>
      <p v-else-if="loading" class="small muted" role="status">{{ t('common.loading') }}</p>
      <details class="small">
        <summary>{{ t('stubs.cli') }}</summary>
        <p class="muted" style="margin: 6px 0">{{ t('stubs.cliHint') }}</p>
        <div class="row nowrap">
          <pre class="stubs-code stubs-cmd grow" tabindex="0" :aria-label="t('stubs.cli')">{{ command }}</pre>
          <Button icon="pi pi-copy" text :aria-label="t('stubs.copyCommand')" @click="copy(command)" />
        </div>
      </details>
    </div>
    <template #footer>
      <Button :label="t('common.close')" severity="secondary" text @click="visible = false" />
      <Button v-if="files.length === 1" :label="t('stubs.downloadFile')" icon="pi pi-file" severity="secondary" outlined :disabled="busy" @click="downloadFile" />
      <Button :label="t('stubs.download')" icon="pi pi-download" :loading="busy" :disabled="busy || !files.length" @click="downloadZip" />
    </template>
  </Dialog>
</template>

<style scoped>
.stubs-options {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
}
.field {
  display: grid;
  gap: 4px;
}
.stubs-code {
  max-height: 360px;
  overflow: auto;
  margin: 0;
  padding: 10px;
  border: 1px solid var(--p-content-border-color);
  border-radius: 6px;
  background: var(--p-content-hover-background);
  font-size: 12px;
  white-space: pre;
}
.stubs-cmd {
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
