<script setup lang="ts">
// The step library of a project (GET /projects/{pid}/step-definitions):
// reusable Gherkin steps as Cucumber expressions or regular expressions,
// how often and where each is used, the steps of the Gherkin tests that
// no definition matches (with an offer to add them), and renaming a step
// in every test that uses it.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import HelpLink from '@/guide/HelpLink.vue'
import ItemRef from './ItemRef.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import type { StepDefinition, StepLibrary, UndefinedStep } from '@/lib/gherkin'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; canWrite: boolean }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const notify = useNotify()

const lib = ref<StepLibrary | null>(null)
const loading = ref(false)
const error = ref('')
const filter = ref('')
async function load() {
  loading.value = true
  error.value = ''
  try {
    lib.value = await api.get<StepLibrary>(`/projects/${props.projectId}/step-definitions`)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(visible, (v) => v && load(), { immediate: true })

const items = computed(() => {
  const f = filter.value.trim().toLowerCase()
  return asItems(lib.value).filter((d) => !f || d.pattern.toLowerCase().includes(f) || d.description.toLowerCase().includes(f))
})
const undefinedSteps = computed(() => lib.value?.undefined ?? [])
const unparsed = computed(() => Object.keys(lib.value?.unparsed ?? {}).length)
const usesOf = (d: StepDefinition) => lib.value?.uses[d.id] ?? []

// ---------- add / edit ----------
const keywordOptions = computed(() => [
  { value: '', label: t('stepLibrary.anyKeyword') },
  { value: 'given', label: t('stepLibrary.keyword.given') },
  { value: 'when', label: t('stepLibrary.keyword.when') },
  { value: 'then', label: t('stepLibrary.keyword.then') },
])
const kindOptions = computed(() => [
  { value: 'cucumber', label: t('stepLibrary.kind.cucumber') },
  { value: 'regex', label: t('stepLibrary.kind.regex') },
])
const blank = () => ({ id: '', pattern: '', kind: 'cucumber' as StepDefinition['kind'], keyword: '' as StepDefinition['keyword'], description: '' })
const form = ref(blank())
const saving = ref(false)
const formError = ref('')
function edit(d: StepDefinition) {
  form.value = { id: d.id, pattern: d.pattern, kind: d.kind, keyword: d.keyword, description: d.description }
  formError.value = ''
  document.getElementById('step-pattern')?.focus()
}
function fromUndefined(u: UndefinedStep) {
  const k = { Given: 'given', Givet: 'given', When: 'when', När: 'when', Then: 'then', Så: 'then' }[u.keyword] as StepDefinition['keyword'] | undefined
  form.value = { ...blank(), pattern: u.suggestion || u.text, keyword: k ?? '' }
  formError.value = ''
  document.getElementById('step-pattern')?.focus()
}
async function save() {
  saving.value = true
  formError.value = ''
  const f = form.value
  const body = { pattern: f.pattern, kind: f.kind, keyword: f.keyword, description: f.description }
  try {
    if (f.id) await api.patch(`/step-definitions/${f.id}`, body)
    else await api.post(`/projects/${props.projectId}/step-definitions`, body)
    notify.ok(f.id ? t('stepLibrary.saved') : t('stepLibrary.added'), f.pattern)
    form.value = blank()
    await load()
    emit('changed')
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
function remove(d: StepDefinition) {
  notify.confirmDanger(t('stepLibrary.confirmDelete', { pattern: d.pattern }), async () => {
    try {
      await api.del(`/step-definitions/${d.id}`)
      notify.ok(t('stepLibrary.deleted'))
      await load()
      emit('changed')
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- rename in every test ----------
const renaming = ref<StepDefinition | null>(null)
const newPattern = ref('')
const renameError = ref('')
const renameBusy = ref(false)
function startRename(d: StepDefinition) {
  renaming.value = d
  newPattern.value = d.kind === 'cucumber' ? d.pattern : ''
  renameError.value = ''
}
async function rename() {
  const d = renaming.value
  if (!d) return
  renameBusy.value = true
  renameError.value = ''
  try {
    const r = await api.post<{ steps: number; tests: { key: string }[] }>(`/step-definitions/${d.id}/rename`, { pattern: newPattern.value })
    notify.ok(t('stepLibrary.renamed', { steps: r.steps, tests: r.tests.length }))
    renaming.value = null
    await load()
    emit('changed')
  } catch (e) {
    renameError.value = errorMessage(e)
  } finally {
    renameBusy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('stepLibrary.title')" style="width: min(980px, 96vw)" class="step-library">
    <div class="grid" style="gap: 12px">
      <div class="row" style="justify-content: space-between; align-items: flex-start">
        <p class="small muted" style="margin: 0">{{ t('stepLibrary.intro') }}</p>
        <HelpLink page="tests" :anchor="{ en: 'the-step-library', sv: 'stegbiblioteket' }" />
      </div>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

      <form v-if="canWrite" class="step-form" :aria-label="form.id ? t('stepLibrary.editTitle') : t('stepLibrary.addTitle')" @submit.prevent="save">
        <div class="field grow">
          <label for="step-pattern" class="block-label">{{ t('stepLibrary.pattern') }}</label>
          <InputText id="step-pattern" v-model="form.pattern" class="mono" style="width: 100%" spellcheck="false" aria-describedby="step-pattern-hint" required />
          <small id="step-pattern-hint" class="muted">{{ t('stepLibrary.patternHint') }}</small>
        </div>
        <div class="field">
          <label for="step-kind" class="block-label">{{ t('stepLibrary.kindLabel') }}</label>
          <Select v-model="form.kind" input-id="step-kind" :options="kindOptions" option-label="label" option-value="value" style="width: 100%" />
        </div>
        <div class="field">
          <label for="step-keyword" class="block-label">{{ t('stepLibrary.keywordLabel') }}</label>
          <Select v-model="form.keyword" input-id="step-keyword" :options="keywordOptions" option-label="label" option-value="value" style="width: 100%" />
        </div>
        <div class="field full">
          <label for="step-description" class="block-label">{{ t('common.description') }}</label>
          <InputText id="step-description" v-model="form.description" style="width: 100%" />
        </div>
        <Message v-if="formError" class="full" severity="error" :closable="false">{{ formError }}</Message>
        <div class="row full" style="justify-content: flex-end">
          <Button v-if="form.id" type="button" :label="t('common.cancel')" text severity="secondary" @click="form = blank()" />
          <Button type="submit" :label="form.id ? t('common.save') : t('stepLibrary.add')" icon="pi pi-check" :loading="saving" :disabled="!form.pattern.trim()" />
        </div>
      </form>

      <section aria-labelledby="step-list-h" class="grid" style="gap: 6px">
        <div class="row">
          <h3 id="step-list-h" class="small" style="margin: 0">{{ t('stepLibrary.steps', { n: asItems(lib).length }, asItems(lib).length) }}</h3>
          <span class="grow" />
          <label for="step-filter" class="sr-only">{{ t('stepLibrary.filter') }}</label>
          <InputText id="step-filter" v-model="filter" size="small" :placeholder="t('stepLibrary.filter')" />
        </div>
        <p v-if="!loading && !items.length" class="small muted" role="status">{{ t('stepLibrary.empty') }}</p>
        <ul class="step-list">
          <li v-for="d in items" :key="d.id" :data-testid="`step-def-${d.id}`">
            <div class="row nowrap">
              <span class="mono grow ellipsis" :title="d.pattern">{{ d.pattern }}</span>
              <span v-if="d.kind === 'regex'" class="tag-chip small">{{ t('stepLibrary.kind.regex') }}</span>
              <span v-if="d.keyword" class="tag-chip small">{{ t(`stepLibrary.keyword.${d.keyword}`) }}</span>
              <span class="small muted">{{ t('stepLibrary.usage', { n: d.usage_count }, d.usage_count) }}</span>
              <template v-if="canWrite">
                <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('stepLibrary.editStep', { pattern: d.pattern })" v-tooltip="t('common.edit')" @click="edit(d)" />
                <Button icon="pi pi-sync" text rounded size="small" severity="secondary" :aria-label="t('stepLibrary.renameStep', { pattern: d.pattern })" v-tooltip="t('stepLibrary.rename')" @click="startRename(d)" />
                <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('stepLibrary.deleteStep', { pattern: d.pattern })" v-tooltip="t('common.delete')" @click="remove(d)" />
              </template>
            </div>
            <div v-if="d.description" class="small muted">{{ d.description }}</div>
            <form v-if="renaming?.id === d.id" class="row rename-form" @submit.prevent="rename">
              <label :for="`rename-${d.id}`" class="sr-only">{{ t('stepLibrary.newPattern') }}</label>
              <InputText :id="`rename-${d.id}`" v-model="newPattern" class="mono grow" spellcheck="false" :aria-describedby="`rename-hint-${d.id}`" />
              <Button type="submit" size="small" :label="t('stepLibrary.renameApply')" icon="pi pi-check" :loading="renameBusy" :disabled="!newPattern.trim()" />
              <Button type="button" size="small" text severity="secondary" :label="t('common.cancel')" @click="renaming = null" />
              <small :id="`rename-hint-${d.id}`" class="muted full">{{ t('stepLibrary.renameHint', { n: d.usage_count }, d.usage_count) }}</small>
              <Message v-if="renameError" severity="error" :closable="false" class="full">{{ renameError }}</Message>
            </form>
            <details v-if="usesOf(d).length" class="small">
              <summary>{{ t('stepLibrary.whereUsed') }}</summary>
              <ul class="uses">
                <li v-for="(u, i) in usesOf(d)" :key="i">
                  <ItemRef :to="{ name: 'tests', params: { projectId }, query: { item: u.test_id } }" :item-key="u.test_key" :title="u.test_title" title-class="muted" />
                  <span class="muted">{{ t('gherkin.lineN', { n: u.line }) }}</span>
                  <span class="mono">{{ u.keyword }} {{ u.text }}</span>
                </li>
              </ul>
            </details>
          </li>
        </ul>
      </section>

      <section v-if="undefinedSteps.length" aria-labelledby="step-undef-h" class="grid" style="gap: 6px">
        <h3 id="step-undef-h" class="small" style="margin: 0">{{ t('stepLibrary.undefinedTitle', { n: undefinedSteps.length }, undefinedSteps.length) }}</h3>
        <p class="small muted" style="margin: 0">{{ t('stepLibrary.undefinedIntro') }}</p>
        <ul class="step-list">
          <li v-for="u in undefinedSteps" :key="u.text">
            <div class="row nowrap">
              <span class="mono grow ellipsis">{{ u.keyword }} {{ u.text }}</span>
              <span class="small muted">{{ t('stepLibrary.usage', { n: u.uses.length }, u.uses.length) }}</span>
              <Button v-if="canWrite" size="small" text icon="pi pi-plus" :label="t('gherkin.addToLibrary')" :aria-label="t('gherkin.addToLibraryAs', { pattern: u.suggestion })" @click="fromUndefined(u)" />
            </div>
            <details class="small">
              <summary>{{ t('stepLibrary.whereUsed') }}</summary>
              <ul class="uses">
                <li v-for="(x, i) in u.uses" :key="i">
                  <ItemRef :to="{ name: 'tests', params: { projectId }, query: { item: x.test_id } }" :item-key="x.test_key" :title="x.test_title" title-class="muted" />
                  <span class="muted">{{ t('gherkin.lineN', { n: x.line }) }}</span>
                </li>
              </ul>
            </details>
          </li>
        </ul>
      </section>
      <Message v-if="unparsed" severity="warn" :closable="false">{{ t('stepLibrary.unparsed', { n: unparsed }, unparsed) }}</Message>
    </div>
    <template #footer>
      <Button :label="t('common.close')" severity="secondary" text @click="visible = false" />
    </template>
  </Dialog>
</template>

<style scoped>
.step-form { display: grid; grid-template-columns: 1fr 180px 180px; gap: 10px; align-items: start; padding: 10px; border: 1px solid var(--qtf-border); border-radius: 8px; }
.step-form .full { grid-column: 1 / -1; }
.step-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 4px; }
.step-list > li { border-bottom: 1px solid var(--qtf-border); padding: 6px 0; display: grid; gap: 2px; }
.uses { list-style: none; padding-left: 12px; margin: 4px 0; display: grid; gap: 2px; }
.uses li { display: flex; gap: 8px; flex-wrap: wrap; align-items: baseline; }
.rename-form { flex-wrap: wrap; }
.rename-form .full { flex-basis: 100%; }
@media (max-width: 720px) { .step-form { grid-template-columns: 1fr; } }
</style>
