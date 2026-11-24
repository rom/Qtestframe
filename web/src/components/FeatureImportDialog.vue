<script setup lang="ts">
// Import .feature files as Gherkin tests (POST /projects/{pid}/tests/
// features/import): one test per scenario or per feature, tags become
// tags and tags that name requirements become trace links. A preview (a
// dry run) shows what will be created before anything is.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import RadioButton from 'primevue/radiobutton'
import Select from 'primevue/select'
import FileDropZone from './FileDropZone.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { buildTree, flatten } from '@/lib/tree'
import type { Folder } from '@/lib/types'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; folders: Folder[]; folderId?: string | null }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ imported: [count: number] }>()
const { t } = useI18n()
const notify = useNotify()

interface ImportResult {
  tests: { id?: string; key?: string; title: string; file: string; tags: string[]; requirements: string[] }[]
  links: number
  errors: { file: string; line: number; message: string }[]
  dry_run: boolean
}
const files = ref<{ name: string; content: string }[]>([])
const mode = ref<'scenario' | 'feature'>('scenario')
const folder = ref<string | null>(props.folderId ?? null)
const preview = ref<ImportResult | null>(null)
const busy = ref(false)
const error = ref('')
watch(visible, (v) => {
  if (v) {
    files.value = []
    preview.value = null
    error.value = ''
    folder.value = props.folderId ?? null
  }
})
const folderOptions = computed(() => [
  { id: null as string | null, label: t('common.topLevelOption') },
  ...flatten(buildTree(props.folders)).map((f) => ({ id: f.id as string | null, label: '  '.repeat(f.depth) + f.label })),
])
async function onFiles(list: File[]) {
  const read = await Promise.all(list.map(async (f) => ({ name: f.name, content: await f.text() })))
  const names = new Set(read.map((f) => f.name))
  files.value = [...files.value.filter((f) => !names.has(f.name)), ...read]
  await run(true)
}
function removeFile(name: string) {
  files.value = files.value.filter((f) => f.name !== name)
  void run(true)
}
async function run(dryRun: boolean) {
  error.value = ''
  if (!files.value.length) {
    preview.value = null
    return
  }
  busy.value = true
  try {
    const r = await api.post<ImportResult>(`/projects/${props.projectId}/tests/features/import`, { files: files.value, mode: mode.value, folder_id: folder.value, dry_run: dryRun })
    if (dryRun) preview.value = r
    else {
      notify.ok(t('featureImport.done', { n: r.tests.length, links: r.links }, r.tests.length))
      emit('imported', r.tests.length)
      visible.value = false
    }
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
watch(mode, () => void run(true))
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('featureImport.title')" style="width: min(820px, 96vw)">
    <div class="grid" style="gap: 12px">
      <div class="row" style="justify-content: space-between; align-items: flex-start">
        <p class="small muted" style="margin: 0">{{ t('featureImport.intro', { at: '@' }) }}</p>
        <HelpLink page="tests" :anchor="{ en: 'import-and-export-feature-files', sv: 'import-och-export-av-feature-filer' }" />
      </div>
      <FileDropZone accept=".feature,text/plain" :label="t('featureImport.choose')" @files="onFiles" />
      <ul v-if="files.length" class="file-list" :aria-label="t('featureImport.files')">
        <li v-for="f in files" :key="f.name" class="row nowrap">
          <i class="pi pi-file" aria-hidden="true" /><span class="mono grow ellipsis">{{ f.name }}</span>
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('featureImport.removeFile', { name: f.name })" @click="removeFile(f.name)" />
        </li>
      </ul>
      <fieldset class="plain-fieldset">
        <legend class="block-label">{{ t('featureImport.mode') }}</legend>
        <div class="row">
          <RadioButton v-model="mode" input-id="fi-scenario" value="scenario" name="fi-mode" />
          <label for="fi-scenario">{{ t('featureImport.perScenario') }}</label>
          <RadioButton v-model="mode" input-id="fi-feature" value="feature" name="fi-mode" />
          <label for="fi-feature">{{ t('featureImport.perFeature') }}</label>
        </div>
      </fieldset>
      <div class="field">
        <label for="fi-folder" class="block-label">{{ t('common.folder') }}</label>
        <Select v-model="folder" input-id="fi-folder" :options="folderOptions" option-label="label" option-value="id" filter style="width: 100%" />
      </div>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <section v-if="preview" aria-labelledby="fi-preview-h" class="grid" style="gap: 6px">
        <h3 id="fi-preview-h" class="small" style="margin: 0">{{ t('featureImport.preview') }}</h3>
        <p class="small" role="status" style="margin: 0">{{ t('featureImport.summary', { n: preview.tests.length, links: preview.links }, preview.tests.length) }}</p>
        <ul class="preview-list">
          <li v-for="(x, i) in preview.tests.slice(0, 200)" :key="i">
            <b>{{ x.title }}</b> <span class="muted small mono">{{ x.file }}</span>
            <span v-for="tg in x.tags" :key="tg" class="tag-chip small">{{ tg }}</span>
            <span v-for="r in x.requirements" :key="r" class="tag-chip small"><i class="pi pi-link" aria-hidden="true" /> {{ r }}</span>
          </li>
        </ul>
        <div v-if="preview.errors.length" class="grid" style="gap: 2px">
          <b class="small">{{ t('featureImport.errors', { n: preview.errors.length }, preview.errors.length) }}</b>
          <span v-for="(e, i) in preview.errors" :key="i" class="small">{{ t('featureImport.errorLine', { file: e.file, line: e.line, message: e.message }) }}</span>
        </div>
      </section>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button :label="t('featureImport.import')" icon="pi pi-file-import" :loading="busy" :disabled="!preview?.tests.length" data-testid="feature-import" @click="run(false)" />
    </template>
  </Dialog>
</template>

<style scoped>
.file-list, .preview-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 4px; max-height: 240px; overflow: auto; }
.preview-list li { display: flex; flex-wrap: wrap; gap: 6px; align-items: baseline; }
.plain-fieldset { border: 0; padding: 0; margin: 0; }
</style>
