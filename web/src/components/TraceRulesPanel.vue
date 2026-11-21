<script setup lang="ts">
// Traceability rules of a project: evaluation results with the violating
// items, and (with project.update) creating, editing and deleting rules.
import ItemRef from './ItemRef.vue'
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import QueryInput from './QueryInput.vue'
import HelpTip from '@/guide/HelpTip.vue'
import { api, ApiError } from '@/lib/api'
import { P } from '@/lib/permissions'
import { RELATION_TYPES } from '@/lib/relations'
import { entry, recordUndo } from '@/lib/undoActions'
import {
  TRACE_DIRECTIONS,
  TRACE_RULE_PRESETS,
  TRACE_TARGET_TYPES,
  blankRule,
  changeScope,
  cleanRule,
  draftOf,
  failing,
  filterResults,
  kindOptions,
  kindsForScope,
  presetRule,
  resultCounts,
  ruleSummary,
  severityTone,
  targetKind,
  violationRoute,
  violationText,
} from '@/lib/traceRules'
import type { TraceRule, TraceRuleFields, TraceRuleResult, TraceRulesReport } from '@/lib/types'
import { relationText } from '@/i18n/labels'
import { useAuthStore } from '@/stores/auth'
import { useUndoStore } from '@/stores/undo'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string }>()
const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()
const canManage = computed(() => auth.can(P.projectUpdate))

const report = ref<TraceRulesReport | null>(null)
const loading = ref(false)
const onlyViolations = ref(false)
const search = ref('')
const open = ref<Record<string, boolean>>({})

async function load() {
  loading.value = true
  try {
    report.value = await api.get<TraceRulesReport>(`/projects/${props.projectId}/trace-rules/evaluate?all=1`)
  } catch (e) {
    notify.fail(e, t('traceRules.loadFailed'))
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => props.projectId, load)
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => void load(),
)

const counts = computed(() => resultCounts(report.value?.results))
const shown = computed(() => filterResults(report.value?.results, onlyViolations.value, search.value))
const relation = (type: string, direction: string) => relationText(type, direction)
const summary = (r: TraceRuleFields) => ruleSummary(r, t, relation)

// ---------- create / edit ----------
const dialog = ref(false)
const editing = ref<TraceRule | null>(null)
const form = ref<TraceRuleFields>(blankRule())
const preset = ref<string | null>(null)
const busy = ref(false)
const formError = ref<unknown>(null)
const selectorError = ref<unknown>(null)
const targetError = ref<unknown>(null)
const preview = ref<TraceRuleResult | null>(null)
const previewBusy = ref(false)

const presetOptions = computed(() => TRACE_RULE_PRESETS.map((p) => ({ value: p.id, label: t(`traceRules.preset.${p.id}.name`) })))
const severityOptions = computed(() => [
  { value: 'error', label: t('traceRules.severity.error') },
  { value: 'warning', label: t('traceRules.severity.warning') },
])
const scopeOptions = computed(() => [
  { value: 'requirement', label: t('traceRules.scope.requirement') },
  { value: 'test', label: t('traceRules.scope.test') },
])
const kindOptionsList = computed(() => kindsForScope(form.value.scope).map((k) => ({ value: k, label: t(`traceRules.kind.${k}`) })))
const relationOptions = computed(() => [{ value: '', label: t('traceRules.anyRelation') }, ...RELATION_TYPES.map((r) => ({ value: r, label: t(`traceRules.relationType.${r}`) }))])
const directionOptions = computed(() => TRACE_DIRECTIONS.map((d) => ({ value: d, label: d ? t(`traceRules.direction.${d}`) : t('traceRules.anyDirection') })))
const targetOptions = computed(() => [{ value: '', label: t('traceRules.anyTarget') }, ...TRACE_TARGET_TYPES.map((x) => ({ value: x, label: t(`traceRules.targetType.${x}`) }))])
const opts = computed(() => kindOptions(form.value.kind))
const otherKind = computed(() => targetKind(form.value))

function openCreate() {
  editing.value = null
  preset.value = null
  form.value = blankRule()
  resetErrors()
  dialog.value = true
}
function openEdit(r: TraceRule) {
  editing.value = r
  preset.value = null
  form.value = draftOf(r)
  resetErrors()
  dialog.value = true
}
function resetErrors() {
  formError.value = selectorError.value = targetError.value = null
  preview.value = null
}
watch(preset, (id) => {
  if (id) form.value = presetRule(id, t)
  preview.value = null
})
function setScope(scope: string) {
  if (scope) form.value = changeScope(form.value, scope)
}
/** Shows an error next to the query it belongs to (the API names it). */
function showError(e: unknown) {
  const msg = e instanceof ApiError ? e.message : String(e)
  if (e instanceof ApiError && msg.startsWith('selector')) selectorError.value = e
  else if (e instanceof ApiError && msg.startsWith('target_query')) targetError.value = e
  else formError.value = e
}
const errorText = (e: unknown) => (e instanceof ApiError || e instanceof Error ? e.message : String(e))

async function runPreview() {
  resetErrors()
  previewBusy.value = true
  try {
    preview.value = await api.post<TraceRuleResult>(`/projects/${props.projectId}/trace-rules/preview?limit=5`, cleanRule({ ...form.value, name: form.value.name.trim() || '-' }))
  } catch (e) {
    showError(e)
  } finally {
    previewBusy.value = false
  }
}

async function save() {
  resetErrors()
  busy.value = true
  const body = cleanRule(form.value)
  const pid = props.projectId
  try {
    if (editing.value) {
      const before = cleanRule(draftOf(editing.value))
      const id = editing.value.id
      await api.patch<TraceRule>(`/trace-rules/${id}`, body)
      recordUndo(entry(t('undo.a.edit', { key: body.name }), async () => void (await api.patch(`/trace-rules/${id}`, before)), async () => void (await api.patch(`/trace-rules/${id}`, body))))
    } else {
      let id = (await api.post<TraceRule>(`/projects/${pid}/trace-rules`, body)).id
      recordUndo(
        entry(
          t('undo.a.create', { key: body.name }),
          async () => void (await api.del(`/trace-rules/${id}`)),
          async () => void (id = (await api.post<TraceRule>(`/projects/${pid}/trace-rules`, body)).id),
        ),
      )
    }
    dialog.value = false
    notify.ok(t('traceRules.saved'))
    await load()
  } catch (e) {
    showError(e)
  } finally {
    busy.value = false
  }
}

async function toggleEnabled(r: TraceRule) {
  const id = r.id
  const next = !r.enabled
  try {
    await api.patch(`/trace-rules/${id}`, { enabled: next })
    recordUndo(entry(t('undo.a.edit', { key: r.name }), async () => void (await api.patch(`/trace-rules/${id}`, { enabled: !next })), async () => void (await api.patch(`/trace-rules/${id}`, { enabled: next }))))
    await load()
  } catch (e) {
    notify.fail(e, t('traceRules.saveFailed'))
  }
}

function remove(r: TraceRule) {
  const pid = props.projectId
  const body = cleanRule(draftOf(r))
  notify.confirmDanger(t('traceRules.confirmDelete', { name: r.name }), async () => {
    try {
      let id = r.id
      await api.del(`/trace-rules/${id}`)
      recordUndo(
        entry(
          t('undo.a.delete', { key: r.name }),
          async () => void (id = (await api.post<TraceRule>(`/projects/${pid}/trace-rules`, body)).id),
          async () => void (await api.del(`/trace-rules/${id}`)),
        ),
      )
      notify.ok(t('traceRules.deleted'))
      await load()
    } catch (e) {
      notify.fail(e, t('traceRules.saveFailed'))
    }
  })
}
</script>

<template>
  <section class="trace-rules" :aria-busy="loading" :aria-label="t('traceRules.title')">
    <div class="filterbar">
      <IconField>
        <InputIcon class="pi pi-search" aria-hidden="true" />
        <InputText v-model="search" :placeholder="t('traceRules.search')" :aria-label="t('traceRules.search')" />
      </IconField>
      <span class="row small nowrap"><ToggleSwitch v-model="onlyViolations" input-id="tr-only" /><label for="tr-only">{{ t('traceRules.onlyViolations') }}</label></span>
      <HelpTip id="trace.rules" :label="t('traceRules.title')" />
      <span class="grow" />
      <Button v-if="canManage" :label="t('traceRules.new')" icon="pi pi-plus" size="small" aria-haspopup="dialog" @click="openCreate" />
    </div>
    <div v-if="report" class="row small" role="status" style="margin-bottom: 10px">
      <Tag :value="counts.errors ? t('traceRules.failing') : t('traceRules.passing')" :severity="counts.errors ? 'danger' : counts.warnings ? 'warn' : 'success'" :icon="counts.errors ? 'pi pi-times' : 'pi pi-check'" />
      <span>{{ t('traceRules.countsLine', { passed: counts.passed, rules: counts.rules }) }}</span>
      <span v-if="counts.errors" class="muted">· {{ t('traceRules.errorsCount', { n: counts.errors }, counts.errors) }}</span>
      <span v-if="counts.warnings" class="muted">· {{ t('traceRules.warningsCount', { n: counts.warnings }, counts.warnings) }}</span>
      <span v-if="counts.disabled" class="muted">· {{ t('traceRules.disabledCount', { n: counts.disabled }, counts.disabled) }}</span>
    </div>

    <div v-if="loading && !report" class="empty"><ProgressSpinner style="width: 32px; height: 32px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="report && !report.results.length" class="card empty">
      <i class="pi pi-list-check" aria-hidden="true" />{{ t('traceRules.none') }}
      <span v-if="!canManage" class="small muted">{{ t('traceRules.noneReadOnly') }}</span>
    </div>
    <div v-else-if="report && !shown.length" class="card empty">{{ t('traceRules.noneShown') }}</div>
    <ul v-else-if="report" class="plain-list rule-list">
      <li v-for="res in shown" :key="res.rule.id" class="card rule" :class="{ fail: failing(res), off: !res.rule.enabled, warn: failing(res) && res.rule.severity === 'warning' }">
        <div class="row rule-head">
          <i :class="!res.rule.enabled ? 'pi pi-minus-circle' : res.passed ? 'pi pi-check-circle' : 'pi pi-times-circle'" class="rule-icon" aria-hidden="true" />
          <span class="sr-only">{{ !res.rule.enabled ? t('traceRules.disabled') : res.passed ? t('traceRules.passed') : t('traceRules.failed') }}:</span>
          <b class="grow rule-name">{{ res.rule.name }}</b>
          <Tag :value="t(`traceRules.severity.${res.rule.severity}`)" :severity="severityTone(res.rule.severity)" />
          <Tag v-if="!res.rule.enabled" :value="t('traceRules.disabled')" severity="secondary" />
          <span class="small mono nowrap">{{ t('traceRules.resultLine', { matched: res.matched }, res.violation_count) }}</span>
          <template v-if="canManage">
            <Button :icon="res.rule.enabled ? 'pi pi-eye-slash' : 'pi pi-eye'" text rounded size="small" severity="secondary" :aria-label="(res.rule.enabled ? t('traceRules.disable') : t('traceRules.enable')) + ': ' + res.rule.name" v-tooltip="res.rule.enabled ? t('traceRules.disable') : t('traceRules.enable')" @click="toggleEnabled(res.rule)" />
            <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('common.edit') + ': ' + res.rule.name" v-tooltip="t('common.edit')" aria-haspopup="dialog" @click="openEdit(res.rule)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('common.delete') + ': ' + res.rule.name" v-tooltip="t('common.delete')" @click="remove(res.rule)" />
          </template>
        </div>
        <div class="small muted rule-summary">{{ summary(res.rule) }}</div>
        <div v-if="res.rule.description" class="small">{{ res.rule.description }}</div>
        <Message v-if="res.error" severity="error" :closable="false" size="small">{{ t('traceRules.invalid', { error: res.error }) }}</Message>
        <template v-if="res.violation_count">
          <Button
            size="small"
            text
            :icon="open[res.rule.id] ? 'pi pi-chevron-down' : 'pi pi-chevron-right'"
            :label="t('traceRules.showViolations', { n: res.violation_count }, res.violation_count)"
            :aria-expanded="!!open[res.rule.id]"
            :aria-controls="'trv-' + res.rule.id"
            class="violations-toggle"
            @click="open[res.rule.id] = !open[res.rule.id]"
          />
          <ul v-if="open[res.rule.id]" :id="'trv-' + res.rule.id" class="plain-list violations" :aria-label="t('traceRules.violationsOf', { name: res.rule.name })">
            <li v-for="v in res.violations" :key="v.kind + v.id" class="row nowrap">
              <span class="sr-only">{{ t(`traceRules.scope.${v.kind === 'test' ? 'test' : 'requirement'}`) }}</span>
              <ItemRef :to="violationRoute(v, projectId)" :item-key="v.key" :title="v.title" :hint="v.title" title-class="ellipsis" grow />
              <span class="small muted nowrap">{{ violationText(v, t) }}</span>
            </li>
            <li v-if="res.violation_count > res.violations.length" class="small muted">{{ t('traceRules.more', { n: res.violation_count - res.violations.length }) }}</li>
          </ul>
        </template>
      </li>
    </ul>
  </section>

  <Dialog v-model:visible="dialog" :header="editing ? t('traceRules.editHeader') : t('traceRules.new')" modal style="width: min(680px, 96vw)">
    <form class="grid trace-rule-form" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ errorText(formError) }}</Message>
      <div v-if="!editing" v-field class="field">
        <label for="tr-preset">{{ t('traceRules.preset.label') }}</label>
        <Select v-model="preset" input-id="tr-preset" :options="presetOptions" option-label="label" option-value="value" :placeholder="t('traceRules.preset.placeholder')" show-clear />
      </div>
      <div class="form-grid">
        <div v-field class="field full"><label for="tr-name">{{ t('common.name') }}</label><InputText id="tr-name" v-model="form.name" required autofocus /></div>
        <div v-field class="field full"><label for="tr-desc">{{ t('common.description') }}</label><Textarea id="tr-desc" v-model="form.description" rows="2" auto-resize /></div>
        <div class="field">
          <span id="tr-sev-l" class="field-label">{{ t('traceRules.severityLabel') }}</span>
          <SelectButton v-model="form.severity" :options="severityOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="tr-sev-l" />
        </div>
        <div class="field">
          <span class="field-label">{{ t('traceRules.state') }}</span>
          <span class="row small nowrap"><ToggleSwitch v-model="form.enabled" input-id="tr-enabled" /><label for="tr-enabled">{{ t('traceRules.enabled') }}</label></span>
        </div>
        <div class="field full">
          <span id="tr-scope-l" class="field-label">{{ t('traceRules.appliesTo') }}</span>
          <SelectButton :model-value="form.scope" :options="scopeOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="tr-scope-l" @update:model-value="setScope" />
        </div>
        <div class="field full">
          <span class="field-label">{{ t('traceRules.selector') }}</span>
          <QueryInput v-model="form.selector" :scope="form.scope === 'test' ? 'test' : 'requirement'" :error="selectorError" :placeholder="t('traceRules.selectorPlaceholder')" />
          <small class="muted">{{ t('traceRules.selectorHint') }}</small>
        </div>
        <div v-field class="field">
          <label for="tr-kind">{{ t('traceRules.kindLabel') }}</label>
          <Select v-model="form.kind" input-id="tr-kind" :options="kindOptionsList" option-label="label" option-value="value" />
        </div>
        <div v-field class="field">
          <label for="tr-min">{{ t('traceRules.min') }}</label>
          <InputNumber v-model="form.min" input-id="tr-min" :min="form.every ? 0 : 1" :max="1000" show-buttons />
        </div>
        <div v-if="opts.relationType" v-field class="field">
          <label for="tr-rel">{{ t('traceRules.relationTypeLabel') }}</label>
          <Select v-model="form.relation_type" input-id="tr-rel" :options="relationOptions" option-label="label" option-value="value" />
        </div>
        <div v-if="opts.targetType" v-field class="field">
          <label for="tr-target">{{ t('traceRules.targetTypeLabel') }}</label>
          <Select v-model="form.target_type" input-id="tr-target" :options="targetOptions" option-label="label" option-value="value" />
        </div>
        <div v-if="opts.direction" v-field class="field">
          <label for="tr-dir">{{ t('traceRules.directionLabel') }}</label>
          <Select v-model="form.direction" input-id="tr-dir" :options="directionOptions" option-label="label" option-value="value" />
        </div>
        <div v-if="otherKind" class="field full">
          <span class="field-label">{{ t(`traceRules.targetQuery.${otherKind}`) }}</span>
          <QueryInput v-model="form.target_query" :scope="otherKind" :error="targetError" :placeholder="t('traceRules.targetQueryPlaceholder')" />
        </div>
        <div class="field full checks">
          <span v-if="opts.notSuspect" class="row small nowrap"><Checkbox v-model="form.not_suspect" binary input-id="tr-ns" /><label for="tr-ns">{{ t('traceRules.notSuspect') }}</label></span>
          <span v-if="opts.passed" class="row small nowrap"><Checkbox v-model="form.passed" binary input-id="tr-passed" /><label for="tr-passed">{{ t('traceRules.passedOnly') }}</label></span>
          <span class="row small nowrap"><Checkbox v-model="form.every" binary input-id="tr-every" /><label for="tr-every">{{ t('traceRules.every') }}</label></span>
        </div>
      </div>
      <div class="small muted" aria-live="polite">{{ summary(cleanRule(form)) }}</div>
      <div v-if="preview" class="small preview" role="status">
        <b>{{ t('traceRules.previewLine', { matched: preview.matched, violations: preview.violation_count }) }}</b>
        <span v-if="preview.violations.length" class="muted"> {{ preview.violations.map((v) => v.key).join(', ') }}{{ preview.violation_count > preview.violations.length ? ' …' : '' }}</span>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('traceRules.preview')" icon="pi pi-play" severity="secondary" outlined :loading="previewBusy" @click="runPreview" />
        <span class="grow" />
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.rule-list { display: grid; gap: 10px; }
.rule { border-left: 4px solid var(--p-green-500, #22c55e); display: grid; gap: 4px; }
.rule.fail { border-left-color: var(--p-red-500, #ef4444); }
.rule.warn { border-left-color: var(--p-amber-500, #f59e0b); }
.rule.off { border-left-color: var(--qtf-border); }
.rule-head { flex-wrap: wrap; }
.rule-icon { font-size: 16px; }
.rule.fail .rule-icon { color: var(--p-red-600, #dc2626); }
.rule.warn .rule-icon { color: var(--p-amber-600, #d97706); }
.rule-name { min-width: 0; overflow-wrap: anywhere; }
.violations { display: grid; gap: 4px; margin: 2px 0 0; padding-left: 8px; }
.violations-toggle { justify-self: start; }
.checks { flex-direction: row; flex-wrap: wrap; gap: 8px 18px; }
.preview { padding: 6px 10px; border-radius: 6px; background: var(--qtf-hover); }
</style>
