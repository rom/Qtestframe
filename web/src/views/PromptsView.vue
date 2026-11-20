<script setup lang="ts">
// Prompts for AI assistants (software projects): built by the server from
// the chosen requirements or tests (POST /projects/{pid}/prompts), shown
// here to copy or download. Nothing is sent to an AI service.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AutoComplete from 'primevue/autocomplete'
import Button from 'primevue/button'
import Message from 'primevue/message'
import RadioButton from 'primevue/radiobutton'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import HelpLink from '@/guide/HelpLink.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import QueryInput from '@/components/QueryInput.vue'
import { api, errorMessage } from '@/lib/api'
import { saveBlob } from '@/lib/download'
import { P } from '@/lib/permissions'
import {
  FRAMEWORK_SUGGESTIONS,
  isPromptKind,
  isSoftware,
  PROMPT_KINDS,
  promptFileName,
  promptForRequirements,
  promptNeedsTestType,
  TEST_TYPES,
  type PromptKind,
} from '@/lib/projectTypes'
import { currentLang } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

interface PromptResult {
  kind: string
  lang: string
  title: string
  prompt: string
  items: number
}
interface Picked {
  id: string
  key: string
  title: string
}

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()

const software = computed(() => isSoftware(proj.project))
const kind = ref<PromptKind>(isPromptKind(route.query.kind) ? route.query.kind : 'implement')
const forReqs = computed(() => promptForRequirements(kind.value))
const presetIds = typeof route.query.ids === 'string' && route.query.ids ? route.query.ids.split(',') : []
const scope = ref<'all' | 'filter' | 'selected'>(presetIds.length ? 'selected' : 'all')
const q = ref('')
const picked = ref<Picked[]>([])
const testType = ref<string | null>(typeof route.query.test_type === 'string' ? route.query.test_type : null)
const framework = ref('')
const notes = ref('')
const lang = ref<string>(currentLang() === 'sv' ? 'sv' : 'en')
const result = ref<PromptResult | null>(null)
const error = ref('')
const busy = ref(false)
const pickerOpen = ref(false)

const canRead = computed(() => auth.can(forReqs.value ? P.requirementRead : P.testRead))
const groups = computed(() => [
  { id: 'req', label: t('prompts.groupRequirements'), kinds: PROMPT_KINDS.filter((k) => promptForRequirements(k)) },
  { id: 'test', label: t('prompts.groupTests'), kinds: PROMPT_KINDS.filter((k) => !promptForRequirements(k)) },
])
const scopeOptions = computed(() => [
  { value: 'all', label: forReqs.value ? t('prompts.allRequirements') : t('prompts.allTests') },
  { value: 'filter', label: t('prompts.byFilter') },
  { value: 'selected', label: t('prompts.selected') },
])
const typeOptions = computed(() => TEST_TYPES.map((v) => ({ value: v, label: t(`prompts.testType.${v}`) })))
const langOptions = [
  { value: 'en', label: 'English' },
  { value: 'sv', label: 'Svenska' },
]
const showTestType = computed(() => promptNeedsTestType(kind.value) || kind.value === 'automate_tests' || kind.value === 'review_tests')
const showFramework = computed(() => kind.value === 'automate_tests' || kind.value === 'test_strategy' || kind.value === 'implement')
const missingType = computed(() => promptNeedsTestType(kind.value) && !testType.value)
const canGenerate = computed(() => software.value && canRead.value && !missingType.value && !(scope.value === 'selected' && !picked.value.length))

// a different kind of item: the selection no longer fits
watch(forReqs, () => {
  picked.value = []
  q.value = ''
  if (scope.value === 'selected') scope.value = 'all'
})
watch(kind, () => {
  result.value = null
  router.replace({ query: { ...route.query, kind: kind.value, ids: undefined } })
})

// items preset from the editor or the list (?ids=…)
async function loadPreset() {
  if (!presetIds.length) return
  const path = forReqs.value ? 'requirements' : 'tests'
  const out: Picked[] = []
  for (const id of presetIds.slice(0, 200)) {
    try {
      const x = await api.get<Picked>(`/${path}/${id}`)
      if (x?.id) out.push({ id: x.id, key: x.key, title: x.title })
    } catch {
      /* skip what is gone */
    }
  }
  picked.value = out
}
void loadPreset()

function onPick(items: Picked[]) {
  const have = new Set(picked.value.map((p) => p.id))
  picked.value = [...picked.value, ...items.filter((i) => !have.has(i.id)).map((i) => ({ id: i.id, key: i.key, title: i.title }))]
}
const removePicked = (id: string) => (picked.value = picked.value.filter((p) => p.id !== id))

const frameworkSuggestions = ref<string[]>([])
function completeFramework(e: { query: string }) {
  const s = e.query.trim().toLowerCase()
  frameworkSuggestions.value = FRAMEWORK_SUGGESTIONS.filter((f) => !s || f.toLowerCase().includes(s))
}

async function generate() {
  busy.value = true
  error.value = ''
  try {
    const ids = scope.value === 'selected' ? picked.value.map((p) => p.id) : undefined
    result.value = await api.post<PromptResult>(`/projects/${proj.pid}/prompts`, {
      kind: kind.value,
      lang: lang.value,
      test_type: showTestType.value ? testType.value || undefined : undefined,
      framework: showFramework.value ? framework.value.trim() || undefined : undefined,
      notes: notes.value.trim() || undefined,
      q: scope.value === 'filter' ? q.value.trim() : undefined,
      ...(forReqs.value ? { requirement_ids: ids } : { test_ids: ids }),
    })
  } catch (e) {
    error.value = errorMessage(e)
    result.value = null
  } finally {
    busy.value = false
  }
}

const output = ref<{ $el?: HTMLElement } | null>(null)
async function copy() {
  if (!result.value) return
  try {
    await navigator.clipboard.writeText(result.value.prompt)
    notify.ok(t('prompts.copied'))
  } catch {
    // no clipboard access: select the text so the user can copy it
    const el = (output.value?.$el ?? null) as HTMLTextAreaElement | null
    el?.focus()
    el?.select()
    notify.info(t('prompts.copyManually'))
  }
}
function download() {
  if (!result.value) return
  saveBlob(new Blob([result.value.prompt], { type: 'text/markdown;charset=utf-8' }), promptFileName(proj.project?.key ?? '', result.value.kind))
}
const words = computed(() => (result.value ? result.value.prompt.split(/\s+/).filter(Boolean).length : 0))
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('prompts.title') }}</h1>
      <div class="sub">{{ t('prompts.intro') }}</div>
    </div>
    <div class="actions"><HelpLink page="requirements" :anchor="{ en: 'ai-prompts-software-projects', sv: 'ai-prompter-mjukvaruprojekt' }" /></div>
  </div>

  <Message v-if="!software" severity="info" :closable="false">
    {{ t('prompts.notSoftware') }}
    <RouterLink v-if="auth.can(P.projectUpdate)" :to="{ name: 'settings', params: { projectId: proj.pid }, query: { tab: 'project' } }">{{ t('prompts.openSettings') }}</RouterLink>
  </Message>

  <div v-else class="prompts-grid">
    <form class="card grid" style="gap: 16px; align-content: start" @submit.prevent="generate">
      <fieldset v-for="g in groups" :key="g.id" class="plain-fieldset prompt-kinds">
        <legend class="field-label">{{ g.label }}</legend>
        <div v-for="k in g.kinds" :key="k" class="prompt-kind" :class="{ on: kind === k }">
          <RadioButton v-model="kind" :input-id="`pk-${k}`" :value="k" name="prompt-kind" />
          <label :for="`pk-${k}`">
            <b>{{ t(`prompts.kind.${k}`) }}</b>
            <span class="small muted">{{ t(`prompts.kindHint.${k}`) }}</span>
          </label>
        </div>
      </fieldset>

      <div class="field">
        <span id="prompt-scope" class="field-label">{{ forReqs ? t('prompts.whichRequirements') : t('prompts.whichTests') }}</span>
        <SelectButton v-model="scope" :options="scopeOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="prompt-scope" />
        <QueryInput v-if="scope === 'filter'" v-model="q" :scope="forReqs ? 'requirement' : 'test'" :placeholder="t('prompts.filterPlaceholder')" />
        <div v-if="scope === 'selected'" class="grid" style="gap: 6px">
          <ul v-if="picked.length" class="plain-list picked-list" :aria-label="t('prompts.selected')">
            <li v-for="p in picked" :key="p.id" class="tag-chip">
              <span class="key">{{ p.key }}</span> <span class="ellipsis" style="max-width: 22ch">{{ p.title }}</span>
              <button type="button" class="tag-remove" :aria-label="t('prompts.removeItem', { key: p.key })" @click="removePicked(p.id)"><i class="pi pi-times small" aria-hidden="true" /></button>
            </li>
          </ul>
          <span v-else class="small muted">{{ t('prompts.noneSelected') }}</span>
          <div class="row"><Button :label="t('prompts.choose')" icon="pi pi-plus" size="small" outlined @click="pickerOpen = true" /></div>
        </div>
      </div>

      <div v-if="showTestType" class="field">
        <label for="prompt-type">{{ t('prompts.testTypeLabel') }}{{ promptNeedsTestType(kind) ? '' : ' ' + t('prompts.optional') }}</label>
        <Select v-model="testType" input-id="prompt-type" :options="typeOptions" option-label="label" option-value="value" :show-clear="!promptNeedsTestType(kind)" :placeholder="t('prompts.chooseType')" :invalid="missingType" />
        <span v-if="testType" class="small muted">{{ t(`prompts.testTypeHint.${testType}`) }}</span>
      </div>
      <div v-if="showFramework" class="field">
        <label for="prompt-fw">{{ t('prompts.framework') }} {{ t('prompts.optional') }}</label>
        <AutoComplete
          v-model="framework"
          input-id="prompt-fw"
          :suggestions="frameworkSuggestions"
          dropdown
          dropdown-mode="blank"
          :pt="{ dropdown: { 'aria-label': t('prompts.frameworkList') } }"
          :placeholder="t('prompts.frameworkPlaceholder')"
          maxlength="200"
          fluid
          @complete="completeFramework"
        />
      </div>
      <div class="field">
        <label for="prompt-lang">{{ t('prompts.language') }}</label>
        <Select v-model="lang" input-id="prompt-lang" :options="langOptions" option-label="label" option-value="value" />
      </div>
      <div class="field">
        <label for="prompt-notes">{{ t('prompts.notes') }} {{ t('prompts.optional') }}</label>
        <Textarea id="prompt-notes" v-model="notes" rows="3" auto-resize maxlength="8000" :placeholder="t('prompts.notesPlaceholder')" />
      </div>
      <Message v-if="!proj.project?.tech_context" severity="warn" :closable="false" class="small">
        {{ t('prompts.noTech') }}
        <RouterLink v-if="auth.can(P.projectUpdate)" :to="{ name: 'settings', params: { projectId: proj.pid }, query: { tab: 'project' } }">{{ t('prompts.openSettings') }}</RouterLink>
      </Message>
      <Message v-if="!canRead" severity="warn" :closable="false">{{ t('prompts.noPermission') }}</Message>
      <div class="row">
        <Button type="submit" :label="t('prompts.generate')" icon="pi pi-sparkles" :loading="busy" :disabled="!canGenerate" />
      </div>
    </form>

    <section class="card grid" style="gap: 10px; align-content: start" aria-live="polite" :aria-busy="busy">
      <h2 class="plain-heading">{{ t('prompts.result') }}</h2>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div v-else-if="!result" class="empty"><i class="pi pi-sparkles" aria-hidden="true" /><div>{{ t('prompts.empty') }}</div></div>
      <template v-else>
        <div class="row" style="justify-content: space-between; flex-wrap: wrap; gap: 8px">
          <div>
            <b>{{ result.title }}</b>
            <div class="small muted">{{ t('prompts.stats', { items: result.items, words, chars: result.prompt.length }) }}</div>
          </div>
          <div class="row nowrap">
            <Button :label="t('prompts.copy')" icon="pi pi-copy" size="small" @click="copy" />
            <Button :label="t('prompts.download')" icon="pi pi-download" size="small" outlined @click="download" />
          </div>
        </div>
        <Textarea ref="output" :model-value="result.prompt" readonly rows="26" class="mono prompt-output" :aria-label="result.title" />
        <p class="small muted" style="margin: 0">{{ t('prompts.privacy') }}</p>
      </template>
    </section>
  </div>

  <ItemPickerDialog
    v-if="software"
    v-model:visible="pickerOpen"
    :kind="forReqs ? 'requirement' : 'test'"
    :project-id="proj.pid"
    :exclude="picked.map((p) => p.id)"
    :header="forReqs ? t('prompts.pickRequirements') : t('prompts.pickTests')"
    :confirm-label="t('prompts.add')"
    @pick="onPick"
  />
</template>

<style>
.prompts-grid {
  display: grid;
  grid-template-columns: minmax(300px, 420px) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
@media (max-width: 1023px) {
  .prompts-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
.prompt-kinds {
  display: grid;
  gap: 6px;
}
.prompt-kind {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 8px 10px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
}
.prompt-kind.on {
  border-color: var(--p-primary-color);
}
.prompt-kind label {
  display: grid;
  gap: 2px;
  cursor: pointer;
}
.picked-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.prompt-output {
  width: 100%;
  font-size: 12.5px;
  line-height: 1.45;
}
</style>
