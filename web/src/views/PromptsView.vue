<script setup lang="ts">
// Prompts for AI assistants (software projects): built by the server from
// the chosen requirements or tests (POST /projects/{pid}/prompts), shown
// here to copy or download. Solution prompts (architecture, design,
// implementation plan, full solution) start with the project's solution
// preamble and can be compact or split into parts. Nothing is sent to an
// AI service, unless the optional AI assistance is available and the user
// chooses "Send to AI assistance" (POST …/prompts/send, streamed); the
// answer can then be saved as a project document.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AutoComplete from 'primevue/autocomplete'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import RadioButton from 'primevue/radiobutton'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import HelpLink from '@/guide/HelpLink.vue'
import FolderPickerDialog from '@/components/FolderPickerDialog.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import QueryInput from '@/components/QueryInput.vue'
import VariantSelect from '@/components/VariantSelect.vue'
import { api, asItems, errorMessage, postDownload } from '@/lib/api'
import { aiStatus, streamPost, type AiStatus, type PromptAnswer } from '@/lib/ai'
import { upload } from '@/lib/upload'
import { saveBlob } from '@/lib/download'
import { P } from '@/lib/permissions'
import {
  FRAMEWORK_SUGGESTIONS,
  isPromptKind,
  isSoftware,
  isSolutionKind,
  PROMPT_KINDS,
  promptFileName,
  promptForRequirements,
  promptNeedsTestType,
  SOLUTION_KINDS,
  TEST_TYPES,
  type PromptKind,
} from '@/lib/projectTypes'
import {
  DEFAULT_PART_TOKENS,
  defaultFormats,
  estimateTokens,
  MIN_PART_TOKENS,
  promptPartFileName,
  SOLUTION_FORMATS,
  SPLITS,
  type PreambleView,
  type PromptPart,
  type SolutionFormat,
} from '@/lib/solution'
import { folderPath } from '@/lib/tree'
import type { Release } from '@/lib/types'
import { currentLang } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { useVariants } from '@/composables/variants'
import { useI18n } from 'vue-i18n'

interface PromptResult {
  kind: string
  lang: string
  title: string
  prompt: string
  items: number
  tokens?: number
  parts?: PromptPart[]
}
interface Picked {
  id: string
  key: string
  title: string
}
type Scope = 'all' | 'filter' | 'folder' | 'release' | 'variant' | 'selected'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()

const software = computed(() => isSoftware(proj.project))
const kind = ref<PromptKind>(isPromptKind(route.query.kind) ? route.query.kind : 'implement')
const forReqs = computed(() => promptForRequirements(kind.value))
const solution = computed(() => isSolutionKind(kind.value))
const presetIds = typeof route.query.ids === 'string' && route.query.ids ? route.query.ids.split(',') : []
const scope = ref<Scope>(presetIds.length ? 'selected' : 'all')
const q = ref('')
const picked = ref<Picked[]>([])
const folderId = ref<string | null>(null)
const releaseId = ref<string | null>(null)
const variant = ref<string | null>(null)
const testType = ref<string | null>(typeof route.query.test_type === 'string' ? route.query.test_type : null)
const framework = ref('')
const notes = ref('')
const lang = ref<string>(currentLang() === 'sv' ? 'sv' : 'en')
const compact = ref(false)
const split = ref<(typeof SPLITS)[number]>('none')
const maxTokens = ref<number | null>(null)
const formats = ref<SolutionFormat[]>(defaultFormats(kind.value))
const result = ref<PromptResult | null>(null)
const partIndex = ref(0)
const error = ref('')
const busy = ref(false)
const pickerOpen = ref(false)
const folderOpen = ref(false)

const canRead = computed(() => auth.can(forReqs.value ? P.requirementRead : P.testRead))
const groups = computed(() => [
  { id: 'req', label: t('prompts.groupRequirements'), kinds: PROMPT_KINDS.filter((k) => promptForRequirements(k) && !isSolutionKind(k)) },
  { id: 'test', label: t('prompts.groupTests'), kinds: PROMPT_KINDS.filter((k) => !promptForRequirements(k)) },
  { id: 'solution', label: t('prompts.groupSolution'), kinds: [...SOLUTION_KINDS] },
])
const scopeOptions = computed(() => [
  { value: 'all', label: forReqs.value ? t('prompts.allRequirements') : t('prompts.allTests') },
  { value: 'filter', label: t('prompts.byFilter') },
  ...(forReqs.value
    ? [
        { value: 'folder', label: t('prompts.byFolder') },
        { value: 'release', label: t('prompts.byRelease') },
        { value: 'variant', label: t('prompts.byVariant') },
      ]
    : []),
  { value: 'selected', label: t('prompts.selected') },
])
const typeOptions = computed(() => TEST_TYPES.map((v) => ({ value: v, label: t(`prompts.testType.${v}`) })))
const langOptions = [
  { value: 'en', label: 'English' },
  { value: 'sv', label: 'Svenska' },
]
const splitOptions = computed(() => SPLITS.map((s) => ({ value: s, label: t(`prompts.split.${s}`) })))
const showTestType = computed(() => promptNeedsTestType(kind.value) || kind.value === 'automate_tests' || kind.value === 'review_tests')
const showFramework = computed(() => kind.value === 'automate_tests' || kind.value === 'test_strategy' || kind.value === 'implement')
const missingType = computed(() => promptNeedsTestType(kind.value) && !testType.value)
const missingScope = computed(
  () =>
    (scope.value === 'selected' && !picked.value.length) ||
    (scope.value === 'folder' && folderId.value === null) ||
    (scope.value === 'release' && !releaseId.value) ||
    (scope.value === 'variant' && !variant.value),
)
const canGenerate = computed(() => software.value && canRead.value && !missingType.value && !missingScope.value)

// folders, releases and variants for the scope
const folders = computed(() => proj.folders.requirement)
const folderLabel = computed(() => (folderId.value === null ? '' : folderId.value === 'root' ? t('common.topLevel') : folderPath(folders.value, folderId.value)))
const releases = ref<Release[]>([])
const releaseOptions = computed(() => releases.value.map((r) => ({ value: r.id, label: r.version && r.version !== r.name ? `${r.name} (${r.version})` : r.name })))
const { variants, load: loadVariants } = useVariants(() => proj.pid)
let scopeDataLoaded = false
async function loadScopeData() {
  if (scopeDataLoaded || !proj.pid) return
  scopeDataLoaded = true
  void proj.loadFolders('requirement')
  void loadVariants()
  try {
    releases.value = asItems(await api.get<{ items: Release[] }>(`/projects/${proj.pid}/releases`))
  } catch {
    releases.value = []
  }
}
watch(scope, (s) => {
  if (s === 'folder' || s === 'release' || s === 'variant') void loadScopeData()
})

// the solution preamble, for its status next to the solution kinds
const preamble = ref<PreambleView | null>(null)
async function loadPreamble() {
  if (!proj.pid || preamble.value) return
  try {
    preamble.value = await api.get<PreambleView>(`/projects/${proj.pid}/solution-preamble`)
  } catch {
    preamble.value = null
  }
}
watch(
  solution,
  (on) => {
    if (on) void loadPreamble()
  },
  { immediate: true },
)
const settingsLink = (tab: string) => ({ name: 'settings', params: { projectId: proj.pid }, query: { tab } })

// a different kind of item: the selection no longer fits
watch(forReqs, () => {
  picked.value = []
  q.value = ''
  if (scope.value !== 'all' && scope.value !== 'filter') scope.value = 'all'
})
watch(kind, () => {
  result.value = null
  formats.value = defaultFormats(kind.value)
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
const onPickFolder = (id: string | null) => (folderId.value = id ?? 'root')

const frameworkSuggestions = ref<string[]>([])
function completeFramework(e: { query: string }) {
  const s = e.query.trim().toLowerCase()
  frameworkSuggestions.value = FRAMEWORK_SUGGESTIONS.filter((f) => !s || f.toLowerCase().includes(s))
}

function body(): Record<string, unknown> {
  const ids = scope.value === 'selected' ? picked.value.map((p) => p.id) : undefined
  return {
    kind: kind.value,
    lang: lang.value,
    test_type: showTestType.value ? testType.value || undefined : undefined,
    framework: showFramework.value ? framework.value.trim() || undefined : undefined,
    notes: notes.value.trim() || undefined,
    q: scope.value === 'filter' ? q.value.trim() : undefined,
    folder_id: scope.value === 'folder' ? folderId.value ?? undefined : undefined,
    release_id: scope.value === 'release' ? releaseId.value ?? undefined : undefined,
    variant: scope.value === 'variant' ? variant.value ?? undefined : undefined,
    ...(forReqs.value ? { requirement_ids: ids } : { test_ids: ids }),
    ...(solution.value
      ? { compact: compact.value, split: split.value, max_tokens: split.value !== 'none' ? maxTokens.value || (split.value === 'size' ? DEFAULT_PART_TOKENS : undefined) : undefined, formats: formats.value }
      : {}),
  }
}

async function generate() {
  busy.value = true
  error.value = ''
  try {
    result.value = await api.post<PromptResult>(`/projects/${proj.pid}/prompts`, body())
    partIndex.value = 0
  } catch (e) {
    error.value = errorMessage(e)
    result.value = null
  } finally {
    busy.value = false
  }
}

const parts = computed<PromptPart[]>(() => result.value?.parts ?? [])
const multi = computed(() => parts.value.length > 1)
const shown = computed(() => (multi.value ? parts.value[partIndex.value]?.prompt ?? '' : result.value?.prompt ?? ''))
const shownTitle = computed(() => (multi.value ? parts.value[partIndex.value]?.title ?? '' : result.value?.title ?? ''))
const shownTokens = computed(() => (multi.value ? parts.value[partIndex.value]?.tokens : result.value?.tokens) ?? estimateTokens(shown.value))
const partOptions = computed(() => parts.value.map((p, i) => ({ value: i, label: t('prompts.partN', { n: p.index }) + (p.label ? `: ${p.label}` : '') })))

const output = ref<{ $el?: HTMLElement } | null>(null)
async function copy() {
  if (!shown.value) return
  try {
    await navigator.clipboard.writeText(shown.value)
    notify.ok(multi.value ? t('prompts.copiedPart', { n: partIndex.value + 1 }) : t('prompts.copied'))
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
  const key = proj.project?.key ?? ''
  const name = multi.value ? promptPartFileName(key, result.value.kind, partIndex.value + 1, parts.value.length) : promptFileName(key, result.value.kind)
  saveBlob(new Blob([shown.value], { type: 'text/markdown;charset=utf-8' }), name)
}
const zipping = ref(false)
async function downloadZip() {
  zipping.value = true
  try {
    await postDownload(`/projects/${proj.pid}/prompts`, { ...body(), format: 'zip' }, promptPartFileName(proj.project?.key ?? '', kind.value, 1, 1).replace(/\.md$/, '.zip'))
  } catch (e) {
    notify.fail(e)
  } finally {
    zipping.value = false
  }
}
const words = computed(() => shown.value.split(/\s+/).filter(Boolean).length)

// the optional AI assistance: send the shown prompt (or part), stream the answer
const ai = ref<AiStatus | null>(null)
if (proj.pid)
  aiStatus(proj.pid, (path) => api.get<AiStatus>(path))
    .then((s) => (ai.value = s))
    .catch(() => (ai.value = null))
const aiAvailable = computed(() => !!ai.value?.available)
const answer = ref('')
const answering = ref(false)
const answerError = ref('')
const answerMeta = ref<PromptAnswer | null>(null)
let abort: AbortController | null = null
watch(result, () => {
  answer.value = ''
  answerError.value = ''
  answerMeta.value = null
})
async function send() {
  if (!result.value) return
  abort = new AbortController()
  answering.value = true
  answer.value = ''
  answerError.value = ''
  answerMeta.value = null
  try {
    const res = await streamPost<PromptAnswer>(`/projects/${proj.pid}/prompts/send`, { ...body(), part: multi.value ? partIndex.value + 1 : undefined }, {
      signal: abort.signal,
      onDelta: (text) => (answer.value += text),
    })
    answer.value = res.answer
    answerMeta.value = res
  } catch (e) {
    if ((e as Error)?.name !== 'AbortError') answerError.value = errorMessage(e)
  } finally {
    answering.value = false
    abort = null
  }
}
const stop = () => abort?.abort()
async function copyAnswer() {
  try {
    await navigator.clipboard.writeText(answer.value)
    notify.ok(t('prompts.answerCopied'))
  } catch {
    notify.info(t('prompts.copyManually'))
  }
}
const canSaveAnswer = computed(() => auth.can(P.requirementWrite) || auth.can(P.testWrite))
const savingAnswer = ref(false)
async function saveAnswer() {
  if (!result.value || !answer.value) return
  savingAnswer.value = true
  try {
    const key = proj.project?.key ?? ''
    const name = promptPartFileName(key, result.value.kind, partIndex.value + 1, multi.value ? parts.value.length : 1).replace(/\.md$/, '-answer.md')
    await upload(`/projects/${proj.pid}/documents`, new File([answer.value], name, { type: 'text/markdown' }), {
      fields: { description: t('prompts.answerDescription', { title: shownTitle.value }), tags: 'ai-answer' },
    })
    notify.ok(t('prompts.answerSaved'), name)
  } catch (e) {
    notify.fail(e)
  } finally {
    savingAnswer.value = false
  }
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('prompts.title') }}</h1>
      <div class="sub">{{ t('prompts.intro') }}</div>
    </div>
    <div class="actions">
      <HelpLink v-if="solution" page="requirements" :anchor="{ en: 'solution-prompts', sv: 'lösningsprompter' }" />
      <HelpLink v-else page="requirements" :anchor="{ en: 'ai-prompts-software-projects', sv: 'ai-prompter-mjukvaruprojekt' }" />
    </div>
  </div>

  <Message v-if="!software" severity="info" :closable="false">
    {{ t('prompts.notSoftware') }}
    <RouterLink v-if="auth.can(P.projectUpdate)" :to="settingsLink('project')">{{ t('prompts.openSettings') }}</RouterLink>
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

      <template v-if="solution">
        <Message v-if="preamble && preamble.filled === 0" severity="warn" :closable="false" class="small">
          {{ t('prompts.noPreamble') }}
          <RouterLink v-if="auth.can(P.projectUpdate)" :to="settingsLink('solution')">{{ t('prompts.editPreamble') }}</RouterLink>
        </Message>
        <div v-else-if="preamble" class="small muted preamble-status">
          <i class="pi pi-compass" aria-hidden="true" />
          {{ t('prompts.preambleStatus', { n: preamble.filled, version: preamble.preamble.version }) }}
          <RouterLink :to="settingsLink('solution')">{{ auth.can(P.projectUpdate) ? t('prompts.editPreamble') : t('prompts.viewPreamble') }}</RouterLink>
          <div v-if="preamble.warnings.length" class="warn-text"><i class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ t('prompts.preambleWarnings', { n: preamble.warnings.length }) }}</div>
        </div>
      </template>

      <div class="field">
        <span id="prompt-scope" class="field-label">{{ forReqs ? t('prompts.whichRequirements') : t('prompts.whichTests') }}</span>
        <SelectButton v-model="scope" :options="scopeOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="prompt-scope" class="scope-buttons" />
        <QueryInput v-if="scope === 'filter'" v-model="q" :scope="forReqs ? 'requirement' : 'test'" :placeholder="t('prompts.filterPlaceholder')" />
        <div v-if="scope === 'folder'" class="row" style="gap: 8px">
          <span class="small" :class="{ muted: folderId === null }" aria-live="polite">{{ folderId === null ? t('prompts.noFolderChosen') : folderLabel }}</span>
          <Button :label="t('prompts.chooseFolder')" icon="pi pi-folder-open" size="small" outlined @click="folderOpen = true" />
        </div>
        <div v-if="scope === 'release'" class="field">
          <label for="prompt-release" class="sr-only">{{ t('prompts.byRelease') }}</label>
          <Select v-model="releaseId" input-id="prompt-release" :options="releaseOptions" option-label="label" option-value="value" :placeholder="t('prompts.chooseRelease')" filter :empty-message="t('prompts.noReleases')" />
        </div>
        <div v-if="scope === 'variant'">
          <VariantSelect v-model="variant" :variants="variants" :aria-label="t('prompts.byVariant')" :placeholder="t('prompts.chooseVariant')" />
        </div>
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

      <fieldset v-if="solution" class="plain-fieldset grid solution-options" style="gap: 12px">
        <legend class="field-label">{{ t('prompts.size') }}</legend>
        <div class="row nowrap" style="gap: 8px; align-items: flex-start">
          <ToggleSwitch v-model="compact" input-id="prompt-compact" aria-describedby="prompt-compact-hint" />
          <div>
            <label for="prompt-compact"><b>{{ t('prompts.compact') }}</b></label>
            <div id="prompt-compact-hint" class="small muted">{{ t('prompts.compactHint') }}</div>
          </div>
        </div>
        <div class="field">
          <label for="prompt-split">{{ t('prompts.splitLabel') }}</label>
          <Select v-model="split" input-id="prompt-split" :options="splitOptions" option-label="label" option-value="value" aria-describedby="prompt-split-hint" />
          <span id="prompt-split-hint" class="small muted">{{ t(`prompts.splitHint.${split}`) }}</span>
        </div>
        <div v-if="split !== 'none'" class="field">
          <label for="prompt-max">{{ t('prompts.maxTokens') }}{{ split === 'folder' ? ' ' + t('prompts.optional') : '' }}</label>
          <InputNumber v-model="maxTokens" input-id="prompt-max" :min="MIN_PART_TOKENS" :max="1000000" :step="1000" :placeholder="split === 'size' ? String(DEFAULT_PART_TOKENS) : ''" aria-describedby="prompt-max-hint" />
          <span id="prompt-max-hint" class="small muted">{{ t('prompts.maxTokensHint') }}</span>
        </div>
        <div class="field">
          <span id="prompt-formats" class="field-label">{{ t('prompts.formats') }}</span>
          <div class="row" style="gap: 12px; flex-wrap: wrap" role="group" aria-labelledby="prompt-formats">
            <span v-for="f in SOLUTION_FORMATS" :key="f" class="row nowrap" style="gap: 6px">
              <Checkbox v-model="formats" :value="f" :input-id="`prompt-format-${f}`" />
              <label :for="`prompt-format-${f}`">{{ t(`prompts.format.${f}`) }}</label>
            </span>
          </div>
        </div>
      </fieldset>

      <div class="field">
        <label for="prompt-lang">{{ t('prompts.language') }}</label>
        <Select v-model="lang" input-id="prompt-lang" :options="langOptions" option-label="label" option-value="value" />
      </div>
      <div class="field">
        <label for="prompt-notes">{{ t('prompts.notes') }} {{ t('prompts.optional') }}</label>
        <Textarea id="prompt-notes" v-model="notes" rows="3" auto-resize maxlength="8000" :placeholder="t('prompts.notesPlaceholder')" />
      </div>
      <Message v-if="!solution && !proj.project?.tech_context" severity="warn" :closable="false" class="small">
        {{ t('prompts.noTech') }}
        <RouterLink v-if="auth.can(P.projectUpdate)" :to="settingsLink('project')">{{ t('prompts.openSettings') }}</RouterLink>
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
        <div v-if="multi" class="field">
          <span id="prompt-parts" class="field-label">{{ t('prompts.parts', { n: parts.length, tokens: result.tokens ?? 0 }) }}</span>
          <SelectButton v-model="partIndex" :options="partOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="prompt-parts" class="part-buttons" />
        </div>
        <div class="row" style="justify-content: space-between; flex-wrap: wrap; gap: 8px">
          <div>
            <b class="prompt-title">{{ shownTitle }}</b>
            <div class="small muted">
              {{ t('prompts.stats', { items: multi ? parts[partIndex]?.items ?? 0 : result.items, words, chars: shown.length }) }} ·
              <span :title="t('prompts.tokensHint')">{{ t('prompts.tokens', { n: shownTokens }) }}</span>
            </div>
          </div>
          <div class="row" style="gap: 6px; flex-wrap: wrap">
            <Button :label="multi ? t('prompts.copyPart') : t('prompts.copy')" icon="pi pi-copy" size="small" @click="copy" />
            <Button :label="multi ? t('prompts.downloadPart') : t('prompts.download')" icon="pi pi-download" size="small" outlined @click="download" />
            <Button v-if="multi" :label="t('prompts.downloadZip')" icon="pi pi-file-export" size="small" outlined :loading="zipping" @click="downloadZip" />
            <Button v-if="aiAvailable" :label="t('prompts.send')" icon="pi pi-send" size="small" severity="secondary" :loading="answering" :disabled="answering" data-test="prompt-send" @click="send" />
          </div>
        </div>
        <Textarea ref="output" :model-value="shown" readonly rows="26" class="mono prompt-output" :aria-label="shownTitle" />
        <p class="small muted" style="margin: 0">{{ t('prompts.privacy') }}</p>
        <section v-if="answering || answer || answerError" class="grid prompt-answer" style="gap: 8px" aria-labelledby="prompt-answer-h">
          <div class="row" style="justify-content: space-between; flex-wrap: wrap; gap: 8px">
            <h3 id="prompt-answer-h" class="plain-heading">{{ t('prompts.answer') }}</h3>
            <div class="row" style="gap: 6px; flex-wrap: wrap">
              <Button v-if="answering" :label="t('prompts.stop')" icon="pi pi-stop-circle" size="small" text severity="secondary" @click="stop" />
              <template v-else-if="answer">
                <Button :label="t('prompts.copyAnswer')" icon="pi pi-copy" size="small" outlined @click="copyAnswer" />
                <Button v-if="canSaveAnswer" :label="t('prompts.saveAnswer')" icon="pi pi-save" size="small" outlined :loading="savingAnswer" @click="saveAnswer" />
              </template>
            </div>
          </div>
          <Message v-if="answerError" severity="error" :closable="false">{{ answerError }}</Message>
          <Message v-if="answerMeta?.truncated" severity="warn" :closable="false" class="small">{{ t('prompts.answerTruncated') }}</Message>
          <Textarea v-if="answer || answering" :model-value="answer" readonly rows="16" class="mono prompt-output" :aria-label="t('prompts.answer')" :aria-busy="answering" />
          <p v-if="answerMeta" class="small muted" style="margin: 0">
            {{ t('prompts.answerMeta', { model: answerMeta.meta.model || answerMeta.meta.provider, input: answerMeta.meta.usage?.input_tokens ?? 0, output: answerMeta.meta.usage?.output_tokens ?? 0 }) }}
          </p>
        </section>
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
  <FolderPickerDialog v-if="software" v-model:visible="folderOpen" :folders="folders" :header="t('prompts.chooseFolder')" @pick="onPickFolder" />
</template>

<style>
.prompts-grid {
  display: grid;
  grid-template-columns: minmax(300px, 440px) minmax(0, 1fr);
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
.scope-buttons,
.part-buttons {
  flex-wrap: wrap;
}
.preamble-status {
  padding: 8px 10px;
  border: 1px dashed var(--qtf-border);
  border-radius: 8px;
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
