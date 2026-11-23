<script setup lang="ts">
// The solution preamble of a software project (GET/PUT
// /projects/{pid}/solution-preamble): languages, stack, environments,
// constraints and conventions that the solution prompts start with.
// Templates prefill it; every field stays editable. Descriptions only:
// the server warns about text that looks like a credential (checked while
// typing) and masks it in prompts.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import { useConfirm } from 'primevue/useconfirm'
import { useI18n } from 'vue-i18n'
import HistoryPanel from './HistoryPanel.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import {
  applyTemplate,
  ARCHITECTURE_STYLES,
  normalizePreamble,
  PREAMBLE_FIELDS,
  PREAMBLE_GROUPS,
  preambleBody,
  preambleFilled,
  preambleLimit,
  type PreambleField,
  type PreambleTemplate,
  type PreambleView,
  type PreambleWarning,
  type SolutionPreamble,
} from '@/lib/solution'
import { currentLang } from '@/i18n'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; canEdit: boolean }>()
const { t } = useI18n()
const notify = useNotify()
const confirm = useConfirm()

const form = ref<SolutionPreamble>(normalizePreamble(null))
const saved = ref('')
const warnings = ref<PreambleWarning[]>([])
const templates = ref<PreambleTemplate[]>([])
const templateId = ref<string | null>(null)
const loading = ref(true)
const loadError = ref('')
const busy = ref(false)
const showHistory = ref(false)
const historyKey = ref(0)

const snapshot = (p: SolutionPreamble) => JSON.stringify(preambleBody(p))
const dirty = computed(() => saved.value !== '' && snapshot(form.value) !== saved.value)
const filled = computed(() => preambleFilled(form.value))
const lang = computed(() => (currentLang() === 'sv' ? 'sv' : 'en'))
const templateOptions = computed(() => templates.value.map((x) => ({ value: x.id, label: x.name[lang.value] ?? x.name.en ?? x.id })))
const styleOptions = computed(() => ARCHITECTURE_STYLES.map((s) => ({ value: s, label: t(`solution.style.${s}`) })))
const fieldLabels = computed(() => Object.fromEntries([...PREAMBLE_FIELDS, 'architecture_style', 'template'].map((f) => [f, t(`solution.field.${f}`)])))
const warningsOf = (f: string) => warnings.value.filter((w) => w.field === f)
const describedBy = (f: PreambleField) => [`sp-${f}-hint`, ...(warningsOf(f).length ? [`sp-${f}-warn`] : [])].join(' ')

function setFrom(v: PreambleView) {
  form.value = normalizePreamble(v.preamble)
  saved.value = snapshot(form.value)
  warnings.value = v.warnings ?? []
  templateId.value = form.value.template || null
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [v, tpl] = await Promise.all([
      api.get<PreambleView>(`/projects/${props.projectId}/solution-preamble`),
      api.get<{ items: PreambleTemplate[] }>(`/projects/${props.projectId}/solution-preamble/templates`),
    ])
    templates.value = asItems(tpl)
    setFrom(v)
  } catch (e) {
    loadError.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.projectId, load, { immediate: true })

// the credential warnings while typing (the server checks; nothing is saved)
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0
watch(
  form,
  () => {
    if (!dirty.value) return
    clearTimeout(timer)
    timer = setTimeout(async () => {
      const n = ++seq
      try {
        const v = await api.post<PreambleView>(`/projects/${props.projectId}/solution-preamble/check`, preambleBody(form.value))
        if (n === seq) warnings.value = v.warnings ?? []
      } catch {
        /* the save reports errors */
      }
    }, 500)
  },
  { deep: true },
)
onBeforeUnmount(() => clearTimeout(timer))

function useTemplate() {
  const tpl = templates.value.find((x) => x.id === templateId.value)
  if (!tpl) return
  const apply = () => {
    form.value = applyTemplate(form.value, tpl)
    notify.info(t('solution.templateApplied', { name: tpl.name[lang.value] ?? tpl.id }))
  }
  if (filled.value === 0) return apply()
  confirm.require({
    header: t('solution.templateConfirmHeader'),
    message: t('solution.templateConfirm', { name: tpl.name[lang.value] ?? tpl.id }),
    icon: 'pi pi-clone',
    rejectProps: { label: t('common.cancel'), severity: 'secondary', outlined: true },
    acceptProps: { label: t('solution.applyTemplate') },
    accept: apply,
  })
}

async function save() {
  busy.value = true
  try {
    const v = await api.put<PreambleView>(`/projects/${props.projectId}/solution-preamble`, preambleBody(form.value))
    setFrom(v)
    historyKey.value++
    if (v.warnings?.length) notify.info(t('solution.saved'), t('solution.savedWithWarnings', { n: v.warnings.length }))
    else notify.ok(t('solution.saved'))
  } catch (e) {
    notify.fail(e, t('solution.saveFailed'))
  } finally {
    busy.value = false
  }
}

function revert() {
  void load()
}
</script>

<template>
  <section class="card solution-preamble" aria-labelledby="sp-h">
    <div class="row" style="justify-content: space-between; gap: 8px; flex-wrap: wrap">
      <div>
        <h2 id="sp-h" class="plain-heading" style="margin-bottom: 4px"><i class="pi pi-compass" aria-hidden="true" /> {{ t('solution.title') }}</h2>
        <p class="small muted" style="margin: 0; max-width: 72ch">{{ t('solution.intro') }}</p>
      </div>
      <div class="row" style="gap: 12px">
        <RouterLink :to="{ name: 'prompts', params: { projectId }, query: { kind: 'architecture' } }" class="small"><i class="pi pi-sparkles" aria-hidden="true" /> {{ t('solution.openPrompts') }}</RouterLink>
        <HelpLink page="requirements" :anchor="{ en: 'solution-preamble', sv: 'lösningsbeskrivning' }" />
      </div>
    </div>

    <Message v-if="loadError" severity="error" :closable="false" style="margin-top: 12px">{{ loadError }}</Message>
    <div v-else-if="loading" class="muted small" style="margin-top: 12px" role="status">{{ t('common.loading') }}</div>
    <form v-else class="grid" style="gap: 16px; margin-top: 12px" @submit.prevent="save">
      <Message severity="info" :closable="false" class="small">
        <i class="pi pi-lock" aria-hidden="true" /> {{ t('solution.noSecrets') }}
      </Message>

      <div class="row" style="gap: 8px; align-items: flex-end; flex-wrap: wrap">
        <div class="field" style="min-width: 260px">
          <label for="sp-template">{{ t('solution.template') }}</label>
          <Select v-model="templateId" input-id="sp-template" :options="templateOptions" option-label="label" option-value="value" :placeholder="t('solution.chooseTemplate')" :disabled="!canEdit" />
        </div>
        <Button type="button" :label="t('solution.applyTemplate')" icon="pi pi-clone" outlined :disabled="!canEdit || !templateId" @click="useTemplate" />
        <span class="grow" />
        <span class="small muted" aria-live="polite">{{ t('solution.filled', { n: filled, total: PREAMBLE_FIELDS.length + 1 }) }}</span>
      </div>

      <div v-if="warnings.length" role="alert" class="p-message p-message-warn sp-warnings">
        <div class="p-message-content" style="display: block">
          <b><i class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ t('solution.warningsTitle', { n: warnings.length }) }}</b>
          <ul style="margin: 6px 0 0; padding-inline-start: 20px">
            <li v-for="(w, i) in warnings" :key="i">
              <a :href="`#sp-${w.field}`">{{ fieldLabels[w.field] }}</a>: {{ t(`solution.warning.${w.kind}`) }} <code class="small">{{ w.excerpt }}</code>
            </li>
          </ul>
        </div>
      </div>

      <fieldset v-for="g in PREAMBLE_GROUPS" :key="g.id" class="plain-fieldset sp-group">
        <legend class="field-label sp-legend">{{ t(`solution.group.${g.id}`) }}</legend>
        <div class="sp-fields">
          <template v-for="f in g.fields" :key="f">
            <div v-if="f === 'architecture_style'" class="field">
              <label for="sp-architecture_style">{{ fieldLabels.architecture_style }}</label>
              <Select
                v-model="form.architecture_style"
                input-id="sp-architecture_style"
                :options="styleOptions"
                option-label="label"
                option-value="value"
                :placeholder="t('solution.chooseStyle')"
                show-clear
                :disabled="!canEdit"
                aria-describedby="sp-architecture_style-hint"
              />
              <span id="sp-architecture_style-hint" class="small muted">{{ t('solution.hint.architecture_style') }}</span>
            </div>
            <div v-else class="field" :class="{ wide: f === 'additional' || f === 'databases' }">
              <label :for="`sp-${f}`">{{ fieldLabels[f] }}</label>
              <Textarea
                :id="`sp-${f}`"
                v-model="form[f]"
                :rows="f === 'additional' ? 4 : 2"
                auto-resize
                :maxlength="preambleLimit(f)"
                :placeholder="t(`solution.placeholder.${f}`)"
                :disabled="!canEdit"
                :invalid="warningsOf(f).length > 0"
                :aria-describedby="describedBy(f)"
              />
              <span :id="`sp-${f}-hint`" class="small muted">{{ t(`solution.hint.${f}`) }}</span>
              <span v-if="warningsOf(f).length" :id="`sp-${f}-warn`" class="small warn-text"><i class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ warningsOf(f).map((w) => t(`solution.warning.${w.kind}`)).join(' ') }}</span>
            </div>
          </template>
        </div>
      </fieldset>

      <div class="row" style="gap: 8px; flex-wrap: wrap">
        <Button v-if="canEdit" type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="!dirty" />
        <Button v-if="canEdit && dirty" type="button" :label="t('solution.discard')" text severity="secondary" @click="revert" />
        <span class="grow" />
        <span v-if="form.version" class="small muted">{{ t('solution.version', { version: form.version, by: form.updated_by || '—', at: form.updated_at ? formatDateTime(form.updated_at) : '—' }) }}</span>
        <Button type="button" :label="showHistory ? t('solution.hideHistory') : t('solution.showHistory')" icon="pi pi-history" text size="small" :aria-expanded="showHistory" aria-controls="sp-history" @click="showHistory = !showHistory" />
      </div>
      <div v-if="showHistory" id="sp-history">
        <HistoryPanel :key="historyKey" entity="solution_preamble" :entity-id="projectId" :can-restore="false" :field-labels="fieldLabels" />
      </div>
    </form>
  </section>
</template>

<style scoped>
.solution-preamble {
  max-width: 1100px;
}
.sp-group {
  display: grid;
  gap: 8px;
}
.sp-legend {
  font-weight: 600;
  margin-bottom: 4px;
}
.sp-fields {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr));
  gap: 12px 16px;
}
.sp-fields .wide {
  grid-column: 1 / -1;
}
.sp-warnings {
  padding: 10px 12px;
  border-radius: 8px;
}
</style>
