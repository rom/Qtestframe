<script setup lang="ts">
// The Gherkin editor of a test in the Gherkin format: a textarea with a
// highlighted overlay behind it (keywords, tags, strings, parameters,
// tables, doc strings), completion of keywords and of the project's step
// library (Ctrl+Space; ↑/↓, Enter or Tab, Escape), aligned tables
// (Alt+Shift+F), errors and undefined steps checked by the server as you
// type (POST /projects/{pid}/gherkin/check) and the latest result of each
// step (GET /tests/{id}/gherkin-results).
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { api, asItems, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import {
  applyCompletion,
  completions,
  formatTables,
  highlight,
  keywordFor,
  languageOf,
  lineCol,
  lineMarks,
  lineStart,
  nextIndent,
  undefinedSteps,
  type CompletionResult,
  type GherkinCheck,
  type GherkinResults,
  type GherkinStepInfo,
  type StepDefinition,
  type StepLibrary,
} from '@/lib/gherkin'
import { useNotify } from '@/composables/notify'

const props = defineProps<{
  projectId: string
  /** the test (for the latest results; null while new) */
  testId?: string | null
  disabled?: boolean
  label: string
}>()
const model = defineModel<string>({ default: '' })
const emit = defineEmits<{ check: [c: GherkinCheck | null]; library: [] }>()
const { t } = useI18n()
const notify = useNotify()
const id = uid('gherkin')
const area = ref<HTMLTextAreaElement | null>(null)
const overlay = ref<HTMLElement | null>(null)

// ---------- step library (for completion) ----------
const defs = ref<StepDefinition[]>([])
async function loadLibrary() {
  try {
    const lib = await api.get<StepLibrary>(`/projects/${props.projectId}/step-definitions`)
    defs.value = asItems(lib)
  } catch {
    defs.value = [] // completion of keywords still works
  }
}
watch(() => props.projectId, loadLibrary, { immediate: true })

// ---------- check as you type ----------
const check = ref<GherkinCheck | null>(null)
const checkError = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0
async function runCheck() {
  const n = ++seq
  if (!model.value.trim()) {
    check.value = null
    emit('check', null)
    return
  }
  try {
    const c = await api.post<GherkinCheck>(`/projects/${props.projectId}/gherkin/check`, { source: model.value })
    if (n !== seq) return
    check.value = c
    checkError.value = ''
    emit('check', c)
  } catch (e) {
    if (n === seq) checkError.value = errorMessage(e)
  }
}
watch(model, () => {
  clearTimeout(timer)
  timer = setTimeout(runCheck, 400)
})
onBeforeUnmount(() => clearTimeout(timer))
void runCheck()

// ---------- latest results ----------
const results = ref<GherkinResults | null>(null)
async function loadResults() {
  results.value = null
  if (!props.testId) return
  try {
    results.value = await api.get<GherkinResults>(`/tests/${props.testId}/gherkin-results`)
  } catch {
    results.value = null
  }
}
watch(() => props.testId, loadResults, { immediate: true })

const html = computed(() => highlight(model.value, lineMarks(check.value, results.value)) + '<div class="gh-line"> </div>')
const lang = computed(() => languageOf(model.value))
const errors = computed(() => check.value?.errors ?? [])
const undefinedList = computed(() => undefinedSteps(check.value))
const failedSteps = computed(() => (results.value?.steps ?? []).filter((s) => s.status === 'failed' || s.status === 'blocked'))
const lines = computed(() => model.value.split('\n'))

function syncScroll() {
  if (overlay.value && area.value) {
    overlay.value.scrollTop = area.value.scrollTop
    overlay.value.scrollLeft = area.value.scrollLeft
  }
}

// ---------- completion ----------
const comp = ref<CompletionResult | null>(null)
const active = ref(0)
const listId = `${id}-list`
const caret = ref({ line: 1, col: 1 })
function updateCaret() {
  const el = area.value
  if (el) caret.value = lineCol(model.value, el.selectionStart)
}
function openCompletion(explicit: boolean) {
  const el = area.value
  if (!el || props.disabled || el.selectionStart !== el.selectionEnd) {
    comp.value = null
    return
  }
  comp.value = completions(model.value, el.selectionStart, defs.value, lang.value, explicit)
  active.value = 0
  updateCaret()
}
function closeCompletion() {
  comp.value = null
}
async function accept(i = active.value) {
  const r = comp.value
  const el = area.value
  if (!r || !el) return
  const res = applyCompletion(model.value, r, r.items[i])
  model.value = res.text
  comp.value = null
  await nextTick()
  el.focus()
  el.setSelectionRange(res.selStart, res.selEnd)
}
const popupStyle = computed(() => {
  const el = area.value
  const top = caret.value.line * 1.5 - (el ? el.scrollTop / 13 / 1.5 : 0) * 1.5
  return { top: `calc(${Math.max(top, 1.5)}em + 10px)`, left: `calc(${Math.min(caret.value.col, 60)}ch + 12px)` }
})
const activeId = computed(() => (comp.value ? `${listId}-${active.value}` : undefined))

function onInput(e: Event) {
  const ev = e as InputEvent
  if (ev.inputType === 'insertText' || ev.inputType === 'deleteContentBackward') openCompletion(false)
  else closeCompletion()
}

async function onKeydown(e: KeyboardEvent) {
  const el = area.value
  if (!el) return
  if (comp.value) {
    const n = comp.value.items.length
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      active.value = (active.value + 1) % n
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      active.value = (active.value - 1 + n) % n
      return
    }
    if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      await accept()
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      closeCompletion()
      return
    }
  }
  if ((e.ctrlKey || e.metaKey) && (e.key === ' ' || e.code === 'Space')) {
    e.preventDefault()
    openCompletion(true)
    return
  }
  if (e.altKey && e.shiftKey && (e.key === 'F' || e.key === 'f' || e.code === 'KeyF')) {
    e.preventDefault()
    format()
    return
  }
  if (e.key === 'Enter' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey && !props.disabled) {
    // keep the indentation (one level deeper after a header)
    const pos = el.selectionStart
    const line = model.value.slice(model.value.lastIndexOf('\n', pos - 1) + 1, pos)
    const ind = nextIndent(line)
    if (!ind) return
    e.preventDefault()
    model.value = model.value.slice(0, pos) + '\n' + ind + model.value.slice(el.selectionEnd)
    await nextTick()
    el.setSelectionRange(pos + 1 + ind.length, pos + 1 + ind.length)
  }
}

// ---------- actions ----------
function format() {
  if (props.disabled) return
  const el = area.value
  const pos = el?.selectionStart ?? 0
  const { line } = lineCol(model.value, pos)
  const out = formatTables(model.value)
  if (out === model.value) {
    notify.info(t('gherkin.formatted'))
    return
  }
  model.value = out
  void nextTick(() => {
    if (el) {
      const p = lineStart(out, line)
      el.setSelectionRange(p, p)
    }
  })
}
async function goToLine(line: number) {
  const el = area.value
  if (!el) return
  el.focus()
  const start = lineStart(model.value, line)
  const end = model.value.indexOf('\n', start)
  el.setSelectionRange(start, end < 0 ? model.value.length : end)
  // scroll the line into view
  el.scrollTop = Math.max(0, (line - 3) * (el.scrollHeight / Math.max(lines.value.length, 1)))
  syncScroll()
}
const adding = ref<string | null>(null)
async function addToLibrary(s: GherkinStepInfo) {
  adding.value = s.text
  try {
    const kind = { Given: 'given', Givet: 'given', When: 'when', När: 'when', Then: 'then', Så: 'then' }[s.keyword] ?? ''
    await api.post(`/projects/${props.projectId}/step-definitions`, { pattern: s.suggestion || s.text, keyword: kind, description: '' })
    notify.ok(t('gherkin.addedToLibrary'), s.suggestion || s.text)
    await loadLibrary()
    await runCheck()
    emit('library')
  } catch (e) {
    notify.fail(e)
  } finally {
    adding.value = null
  }
}
defineExpose({ refresh: async () => (await loadLibrary(), await runCheck(), await loadResults()), format })
const keywordHint = computed(() => [keywordFor('given', lang.value), keywordFor('when', lang.value), keywordFor('then', lang.value)].join(', '))
</script>

<template>
  <div class="gherkin-editor">
    <div class="row gh-toolbar">
      <span class="small muted" :id="`${id}-help`">{{ t('gherkin.editorHelp', { keywords: keywordHint }) }}</span>
      <span class="grow" />
      <span v-if="check?.valid" class="small muted" role="status">{{ t('gherkin.scenarioCount', { n: check.scenarios.length }, check.scenarios.length) }}</span>
      <Button size="small" text icon="pi pi-align-left" :label="t('gherkin.format')" :disabled="disabled" v-tooltip.bottom="t('gherkin.formatHint')" @click="format" />
    </div>
    <div class="gh-frame" :class="{ disabled }">
      <div ref="overlay" class="gh-overlay" aria-hidden="true" v-html="html" />
      <textarea
        :id="id"
        ref="area"
        v-model="model"
        class="gh-input"
        spellcheck="false"
        autocapitalize="off"
        autocomplete="off"
        wrap="off"
        rows="16"
        :aria-label="label"
        :aria-describedby="`${id}-help ${id}-status`"
        :aria-invalid="errors.length ? 'true' : undefined"
        aria-autocomplete="list"
        :aria-controls="comp ? listId : undefined"
        :aria-activedescendant="activeId"
        :readonly="disabled"
        data-testid="gherkin-input"
        @input="onInput"
        @keydown="onKeydown"
        @scroll="syncScroll"
        @click="closeCompletion"
        @blur="closeCompletion"
      />
      <ul v-if="comp" :id="listId" class="gh-completions" role="listbox" :aria-label="t('gherkin.completions')" :style="popupStyle">
        <li
          v-for="(c, i) in comp.items"
          :id="`${listId}-${i}`"
          :key="c.insert + i"
          role="option"
          :aria-selected="i === active"
          :class="{ active: i === active }"
          @mousedown.prevent="accept(i)"
        >
          <i :class="c.kind === 'step' ? 'pi pi-book' : 'pi pi-hashtag'" aria-hidden="true" />
          <span class="mono">{{ c.label }}</span>
          <span v-if="c.detail" class="small muted ellipsis">{{ c.detail }}</span>
        </li>
      </ul>
    </div>
    <div :id="`${id}-status`" class="gh-status" aria-live="polite">
      <Message v-if="checkError" severity="warn" :closable="false">{{ checkError }}</Message>
      <div v-if="errors.length" class="gh-errors" role="group" :aria-label="t('gherkin.errorsTitle', { n: errors.length }, errors.length)">
        <div class="small"><i class="pi pi-times-circle gh-err-icon" aria-hidden="true" /> <b>{{ t('gherkin.errorsTitle', { n: errors.length }, errors.length) }}</b></div>
        <ul>
          <li v-for="(e, i) in errors" :key="i">
            <Button text size="small" class="gh-line-link" :label="t('gherkin.lineN', { n: e.line })" :aria-label="t('gherkin.goToLine', { n: e.line })" @click="goToLine(e.line)" />
            <span class="small">{{ e.message }}</span>
          </li>
        </ul>
      </div>
      <div v-if="undefinedList.length" class="gh-undefined-list" role="group" :aria-label="t('gherkin.undefinedTitle', { n: undefinedList.length }, undefinedList.length)">
        <div class="small"><i class="pi pi-question-circle" aria-hidden="true" /> <b>{{ t('gherkin.undefinedTitle', { n: undefinedList.length }, undefinedList.length) }}</b> <span class="muted">{{ t('gherkin.undefinedHint') }}</span></div>
        <ul>
          <li v-for="s in undefinedList.slice(0, 20)" :key="s.line">
            <Button text size="small" class="gh-line-link" :label="t('gherkin.lineN', { n: s.line })" :aria-label="t('gherkin.goToLine', { n: s.line })" @click="goToLine(s.line)" />
            <span class="mono small">{{ s.keyword }} {{ s.text }}</span>
            <Button
              v-if="!disabled"
              size="small"
              text
              icon="pi pi-plus"
              :label="t('gherkin.addToLibrary')"
              :aria-label="t('gherkin.addToLibraryAs', { pattern: s.suggestion || s.text })"
              :loading="adding === s.text"
              v-tooltip.bottom="s.suggestion"
              @click="addToLibrary(s)"
            />
          </li>
        </ul>
      </div>
      <details v-if="results?.steps.length" class="small gh-results">
        <summary>{{ failedSteps.length ? t('gherkin.lastRunFailed', { n: failedSteps.length }, failedSteps.length) : t('gherkin.lastRunPassed') }}</summary>
        <ul>
          <li v-for="r in results.steps" :key="r.line">
            <Button text size="small" class="gh-line-link" :label="t('gherkin.lineN', { n: r.line })" :aria-label="t('gherkin.goToLine', { n: r.line })" @click="goToLine(r.line)" />
            <span :class="`gh-res-text gh-res-${r.status}`">{{ t(`enums.result.${r.status}`) }}</span>
            <span class="muted">{{ lines[r.line - 1]?.trim() }}</span>
          </li>
        </ul>
      </details>
    </div>
  </div>
</template>

<style scoped>
.gherkin-editor { display: grid; gap: 6px; }
.gh-toolbar { gap: 8px; }
.gh-frame { position: relative; border: 1px solid var(--qtf-border); border-radius: 8px; background: var(--qtf-card); }
.gh-frame:focus-within { outline: 2px solid var(--qtf-focus); outline-offset: 1px; }
.gh-overlay, .gh-input {
  font-family: var(--qtf-mono); font-size: 13px; line-height: 1.5; padding: 10px 12px; margin: 0;
  white-space: pre; tab-size: 2; letter-spacing: 0; word-spacing: 0;
}
.gh-overlay { position: absolute; inset: 0; overflow: hidden; color: var(--qtf-text); pointer-events: none; }
.gh-input {
  position: relative; display: block; width: 100%; min-height: 18em; resize: vertical; border: 0; outline: 0; background: transparent;
  color: transparent; caret-color: var(--qtf-text); overflow: auto;
}
.gh-input::selection { background: color-mix(in srgb, var(--p-primary-color) 30%, transparent); color: transparent; }
.gh-completions {
  position: absolute; z-index: 5; list-style: none; margin: 0; padding: 4px; min-width: 260px; max-width: min(560px, 90%); max-height: 240px; overflow: auto;
  background: var(--qtf-card); border: 1px solid var(--qtf-border); border-radius: 8px; box-shadow: 0 8px 24px rgba(0, 0, 0, .18);
}
.gh-completions li { display: flex; gap: 8px; align-items: baseline; padding: 4px 8px; border-radius: 6px; cursor: pointer; color: var(--qtf-text); }
.gh-completions li.active { background: var(--p-primary-color); color: var(--p-primary-contrast-color); }
.gh-completions li.active .muted { color: inherit; }
.gh-status ul { margin: 4px 0 0; padding-left: 0; list-style: none; display: grid; gap: 2px; }
.gh-status li { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
.gh-errors, .gh-undefined-list { border-left: 3px solid var(--gh-err, #b91c1c); padding: 4px 8px; }
.gh-undefined-list { border-left-color: var(--gh-undef, #a16207); }
.gh-err-icon { color: var(--gh-err, #b91c1c); }
.gh-line-link { padding: 0 4px; font-family: var(--qtf-mono); }
.gh-results summary { cursor: pointer; }
.gh-res-text { font-weight: 600; }
.disabled .gh-input { cursor: default; }
</style>

<style>
/* the highlight colours (not scoped: the overlay is v-html) */
.gherkin-editor {
  --gh-kw: #6d28d9; --gh-step: #1d4ed8; --gh-tag: #0f766e; --gh-str: #9a3412; --gh-num: #a1490a; --gh-ph: #be185d;
  --gh-param: #0e7490; --gh-doc: #3f6212; --gh-err: #b91c1c; --gh-undef: #a16207; --gh-pass: #15803d;
}
.app-dark .gherkin-editor {
  --gh-kw: #c4b5fd; --gh-step: #93c5fd; --gh-tag: #5eead4; --gh-str: #fdba74; --gh-num: #fcd34d; --gh-ph: #f9a8d4;
  --gh-param: #67e8f9; --gh-doc: #bef264; --gh-err: #fca5a5; --gh-undef: #fde047; --gh-pass: #86efac;
}
:root[data-theme="high_contrast"] .gherkin-editor, :root[data-theme="high_contrast_light"] .gherkin-editor {
  --gh-kw: var(--qtf-text); --gh-step: var(--qtf-text); --gh-tag: var(--qtf-text); --gh-str: var(--qtf-text); --gh-num: var(--qtf-text);
  --gh-ph: var(--qtf-text); --gh-param: var(--qtf-text); --gh-doc: var(--qtf-text); --gh-err: var(--qtf-text); --gh-undef: var(--qtf-text); --gh-pass: var(--qtf-text);
}
:root[data-theme="high_contrast"] .gh-placeholder, :root[data-theme="high_contrast_light"] .gh-placeholder { text-decoration: underline; }
.gh-keyword { color: var(--gh-kw); font-weight: 700; }
.gh-step { color: var(--gh-step); font-weight: 700; }
.gh-tag { color: var(--gh-tag); }
.gh-string { color: var(--gh-str); }
.gh-number { color: var(--gh-num); }
.gh-placeholder { color: var(--gh-ph); font-style: italic; }
.gh-param { color: var(--gh-param); }
.gh-pipe { color: var(--qtf-muted); }
.gh-docstring { color: var(--gh-doc); }
.gh-comment { color: var(--qtf-muted); font-style: italic; }
.gh-line.gh-error { text-decoration: underline wavy var(--gh-err); text-decoration-skip-ink: none; background: color-mix(in srgb, var(--gh-err) 10%, transparent); }
.gh-line.gh-undefined { text-decoration: underline dotted var(--gh-undef); text-underline-offset: 3px; }
.gh-line.gh-result-passed { box-shadow: inset 3px 0 0 var(--gh-pass); }
.gh-line.gh-result-failed, .gh-line.gh-result-blocked { box-shadow: inset 3px 0 0 var(--gh-err); }
.gh-res-passed { color: var(--gh-pass); }
.gh-res-failed, .gh-res-blocked { color: var(--gh-err); }
</style>
