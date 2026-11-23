<script setup lang="ts">
// Duplicates & conflicts of a project (GET /projects/{pid}/consistency/
// review): suspected duplicate requirements or tests and potentially
// conflicting requirements with their evidence, and what to do about them:
// mark as duplicate or as conflicting (relations; an item link between
// tests), merge (one survives, the other is obsoleted) or dismiss (not
// shown again; restorable).
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import RadioButton from 'primevue/radiobutton'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import ItemRef from '@/components/ItemRef.vue'
import StatusTag from '@/components/StatusTag.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { itemRoute } from '@/lib/consistency'
import { P } from '@/lib/permissions'
import {
  dismissCheck,
  dismissPair,
  markConflict,
  markDuplicate,
  mergePair,
  percent,
  suggestedSurvivor,
  type ConflictPair,
  type DuplicatePair,
  type Evidence,
  type ReviewResponse,
  type SimilarItem,
  type SimilarKind,
} from '@/lib/similarity'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const id = uid('dup')

const canTests = computed(() => auth.can(P.testRead))
const kind = ref<SimilarKind>(route.query.kind === 'test' && canTests.value ? 'test' : 'requirement')
watch(kind, (v) => void router.replace({ query: { ...route.query, kind: v === 'test' ? 'test' : undefined } }))
const kindOptions = computed(() => [
  { value: 'requirement', label: t('duplicates.requirements'), icon: 'pi pi-list-check' },
  ...(canTests.value ? [{ value: 'test', label: t('duplicates.tests'), icon: 'pi pi-check-square' }] : []),
])
const canWrite = computed(() => auth.can(kind.value === 'test' ? P.testWrite : P.requirementWrite))

const data = ref<ReviewResponse | null>(null)
const loading = ref(false)
const error = ref('')
const showDismissed = ref(false)
const threshold = ref<number | null>(null)
const confidence = ref<number | null>(null)
const busy = ref('')

async function load() {
  if (!proj.pid) return
  loading.value = true
  error.value = ''
  try {
    const q: Record<string, string> = { kind: kind.value }
    if (threshold.value) q.threshold = String(threshold.value)
    if (confidence.value) q.confidence = String(confidence.value)
    if (showDismissed.value) q.dismissed = 'true'
    data.value = await api.get<ReviewResponse>(`/projects/${proj.pid}/consistency/review`, q)
    threshold.value = data.value.threshold
    confidence.value = data.value.conflict_confidence
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch([() => proj.pid, kind, showDismissed], () => {
  if (kind.value !== data.value?.kind) threshold.value = null
  void load()
}, { immediate: true })

const pairKey = (p: { a: SimilarItem; b: SimilarItem }) => `${p.a.id}|${p.b.id}`

async function act(key: string, fn: () => Promise<unknown>, done: string) {
  busy.value = key
  try {
    await fn()
    notify.ok(done)
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = ''
  }
}

const markDup = (p: DuplicatePair) => act(`dup:${pairKey(p)}`, () => markDuplicate(kind.value, p.b, p.a), t('duplicates.markedDuplicate', { a: p.b.key, b: p.a.key }))
const markConf = (p: ConflictPair) => act(`conf:${pairKey(p)}`, () => markConflict(p.b, p.a), t('duplicates.markedConflict', { a: p.b.key, b: p.a.key }))
function dismiss(p: DuplicatePair | ConflictPair, conflict: boolean) {
  const check = dismissCheck(kind.value, conflict)
  return act(`dis:${pairKey(p)}`, () => dismissPair(proj.pid, check, p.a, p.b, !p.dismissed), p.dismissed ? t('duplicates.restored') : t('duplicates.dismissed'))
}

// merging: the user picks the item that survives
const mergeFor = ref<DuplicatePair | null>(null)
const survivor = ref('')
function openMerge(p: DuplicatePair) {
  mergeFor.value = p
  survivor.value = suggestedSurvivor(p).id
}
const mergeOther = computed(() => (mergeFor.value ? (survivor.value === mergeFor.value.a.id ? mergeFor.value.b : mergeFor.value.a) : null))
const mergeKeep = computed(() => (mergeFor.value ? (survivor.value === mergeFor.value.a.id ? mergeFor.value.a : mergeFor.value.b) : null))
async function doMerge() {
  const p = mergeFor.value
  if (!p || !mergeKeep.value || !mergeOther.value) return
  const keep = mergeKeep.value
  const other = mergeOther.value
  busy.value = 'merge'
  try {
    const res = await mergePair(proj.pid, kind.value, keep, other)
    notify.ok(t('duplicates.merged', { a: res.duplicate.key, b: res.survivor.key, links: res.moved.links }))
    mergeFor.value = null
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = ''
  }
}

const conflicts = computed(() => data.value?.conflicts ?? [])
const duplicates = computed(() => data.value?.duplicates ?? [])
const fieldLabel = (e: Evidence) => t(`duplicates.field.${e.field}`)
const obsoleteStatus = computed(() => (kind.value === 'test' ? 'deprecated' : 'obsolete'))
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('duplicates.title') }}</h1>
      <div class="sub">{{ t('duplicates.intro') }} <HelpLink page="duplicates-and-conflicts" /></div>
    </div>
    <div class="actions">
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" :loading="loading" @click="load" />
    </div>
  </div>

  <div class="card dup-controls">
    <SelectButton v-model="kind" :options="kindOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('duplicates.kind')">
      <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /> {{ option.label }}</template>
    </SelectButton>
    <div class="dup-field">
      <label :for="`${id}-threshold`">{{ t('duplicates.threshold') }}</label>
      <InputNumber v-model="threshold" :input-id="`${id}-threshold`" :min="0.3" :max="1" :step="0.05" :min-fraction-digits="2" :max-fraction-digits="2" show-buttons size="small" :aria-describedby="`${id}-threshold-h`" @update:model-value="load" />
      <small :id="`${id}-threshold-h`" class="muted">{{ t('duplicates.thresholdHint') }}</small>
    </div>
    <div v-if="kind === 'requirement'" class="dup-field">
      <label :for="`${id}-confidence`">{{ t('duplicates.confidence') }}</label>
      <InputNumber v-model="confidence" :input-id="`${id}-confidence`" :min="0.5" :max="1" :step="0.05" :min-fraction-digits="2" :max-fraction-digits="2" show-buttons size="small" :aria-describedby="`${id}-confidence-h`" @update:model-value="load" />
      <small :id="`${id}-confidence-h`" class="muted">{{ t('duplicates.confidenceHint') }}</small>
    </div>
    <div class="row dup-switch">
      <ToggleSwitch v-model="showDismissed" :input-id="`${id}-dismissed`" />
      <label :for="`${id}-dismissed`">{{ t('duplicates.showDismissed', { n: data?.dismissed ?? 0 }) }}</label>
    </div>
    <Tag v-if="data" :value="data.engine === 'embeddings' ? t('duplicates.engineEmbeddings') : t('duplicates.engineLocal')" severity="secondary" />
  </div>

  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="!data && loading" class="empty" aria-busy="true"><ProgressSpinner style="width: 32px; height: 32px" :aria-label="t('common.loading')" /></div>
  <template v-else-if="data">
    <p class="sr-only" role="status">{{ t('duplicates.summary', { d: duplicates.length, c: conflicts.length }) }}</p>
    <Message v-if="data.truncated" severity="info" :closable="false">{{ t('duplicates.truncated') }}</Message>

    <!-- potential conflicts (requirements) -->
    <section v-if="kind === 'requirement'" class="card dup-section" :aria-labelledby="`${id}-h-conf`" :aria-busy="loading">
      <h2 :id="`${id}-h-conf`">{{ t('duplicates.conflicts', { n: conflicts.length }) }}</h2>
      <p v-if="!conflicts.length" class="muted small">{{ t('duplicates.noConflicts') }}</p>
      <ul v-else class="plain-list pairs">
        <li v-for="p in conflicts" :key="pairKey(p)" class="pair" :class="{ dismissed: p.dismissed }">
          <div class="row pair-head">
            <Tag :value="t(`duplicates.conflictKind.${p.kind}`)" severity="warn" />
            <span class="small">{{ t('duplicates.confidenceValue', { pct: percent(p.confidence) }) }}</span>
            <span class="small muted ellipsis grow">{{ p.note }}</span>
            <Tag v-if="p.dismissed" :value="t('duplicates.dismissedTag')" severity="secondary" />
          </div>
          <div class="pair-items">
            <div v-for="(side, i) in [{ it: p.a, ev: p.evidence_a }, { it: p.b, ev: p.evidence_b }]" :key="i" class="pair-item">
              <div class="row" style="gap: 6px; min-width: 0">
                <ItemRef :to="itemRoute(side.it, proj.pid)" :item-key="side.it.key" :title="side.it.title" :hint="side.it.title" title-class="ellipsis" grow />
                <StatusTag :value="side.it.status" kind="status" />
              </div>
              <blockquote class="evidence" :aria-label="t('duplicates.evidenceOf', { key: side.it.key, field: fieldLabel(side.ev) })">
                <span class="small muted">{{ fieldLabel(side.ev) }}:</span>
                <template v-for="(part, k) in side.ev.parts" :key="k"><mark v-if="part.mark">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template>
              </blockquote>
            </div>
          </div>
          <div v-if="auth.can(P.requirementWrite)" class="row pair-actions">
            <Button v-if="!p.dismissed" :label="t('duplicates.markConflict')" icon="pi pi-bolt" size="small" outlined :loading="busy === `conf:${pairKey(p)}`" :disabled="!!busy" @click="markConf(p)" />
            <Button :label="p.dismissed ? t('duplicates.restore') : t('duplicates.dismiss')" :icon="p.dismissed ? 'pi pi-undo' : 'pi pi-eye-slash'" size="small" text :loading="busy === `dis:${pairKey(p)}`" :disabled="!!busy" @click="dismiss(p, true)" />
          </div>
        </li>
      </ul>
    </section>

    <!-- suspected duplicates -->
    <section class="card dup-section" :aria-labelledby="`${id}-h-dup`" :aria-busy="loading">
      <h2 :id="`${id}-h-dup`">{{ kind === 'test' ? t('duplicates.duplicateTests', { n: duplicates.length }) : t('duplicates.duplicateRequirements', { n: duplicates.length }) }}</h2>
      <p v-if="!duplicates.length" class="muted small">{{ t('duplicates.noDuplicates') }}</p>
      <ul v-else class="plain-list pairs">
        <li v-for="p in duplicates" :key="pairKey(p)" class="pair" :class="{ dismissed: p.dismissed }">
          <div class="row pair-head">
            <Tag :value="t('duplicates.similarValue', { pct: percent(p.score) })" :severity="p.score >= 0.9 ? 'danger' : 'warn'" />
            <span class="grow" />
            <Tag v-if="p.dismissed" :value="t('duplicates.dismissedTag')" severity="secondary" />
          </div>
          <div class="pair-items">
            <div v-for="(it, i) in [p.a, p.b]" :key="i" class="pair-item">
              <div class="row" style="gap: 6px; min-width: 0">
                <ItemRef :to="itemRoute(it, proj.pid)" :item-key="it.key" :title="it.title" :hint="it.title" title-class="ellipsis" grow />
                <StatusTag :value="it.status" kind="status" />
              </div>
              <span class="small muted">{{ t('duplicates.links', { n: it.links }, it.links) }}</span>
            </div>
          </div>
          <div v-if="canWrite" class="row pair-actions">
            <template v-if="!p.dismissed">
              <Button :label="t('duplicates.markDuplicate', { a: p.b.key, b: p.a.key })" icon="pi pi-link" size="small" outlined :loading="busy === `dup:${pairKey(p)}`" :disabled="!!busy" @click="markDup(p)" />
              <Button :label="t('duplicates.merge')" icon="pi pi-arrow-right-arrow-left" size="small" outlined :disabled="!!busy" @click="openMerge(p)" />
            </template>
            <Button :label="p.dismissed ? t('duplicates.restore') : t('duplicates.notDuplicate')" :icon="p.dismissed ? 'pi pi-undo' : 'pi pi-eye-slash'" size="small" text :loading="busy === `dis:${pairKey(p)}`" :disabled="!!busy" @click="dismiss(p, false)" />
          </div>
        </li>
      </ul>
    </section>
  </template>

  <Dialog :visible="!!mergeFor" modal :header="t('duplicates.mergeTitle')" style="width: min(560px, 96vw)" @update:visible="(v: boolean) => !v && (mergeFor = null)">
    <form v-if="mergeFor" :id="`${id}-merge`" class="grid" style="gap: 12px" @submit.prevent="doMerge">
      <fieldset class="plain-fieldset">
        <legend class="small" style="font-weight: 600">{{ t('duplicates.survivor') }}</legend>
        <div v-for="it in [mergeFor.a, mergeFor.b]" :key="it.id" class="row survivor">
          <RadioButton v-model="survivor" :input-id="`${id}-s-${it.id}`" :value="it.id" name="survivor" />
          <label :for="`${id}-s-${it.id}`"><span class="key">{{ it.key }}</span> {{ it.title }} <span class="small muted">({{ t('duplicates.links', { n: it.links }, it.links) }})</span></label>
        </div>
      </fieldset>
      <p v-if="mergeKeep && mergeOther" class="small" role="status">
        {{ t('duplicates.mergeExplain', { keep: mergeKeep.key, other: mergeOther.key, status: enumLabel('status', obsoleteStatus) }) }}
      </p>
    </form>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="mergeFor = null" />
      <Button type="submit" :form="`${id}-merge`" :label="t('duplicates.mergeConfirm')" icon="pi pi-check" :loading="busy === 'merge'" />
    </template>
  </Dialog>
</template>

<style scoped>
.dup-controls { display: flex; flex-wrap: wrap; gap: 12px 20px; align-items: flex-end; margin-bottom: 16px; }
.dup-field { display: grid; gap: 2px; max-width: 240px; }
.dup-field label { font-weight: 600; font-size: 13px; }
.dup-switch { gap: 8px; align-items: center; }
.dup-section { margin-bottom: 16px; }
.pairs { display: grid; gap: 10px; }
.pair { border: 1px solid var(--qtf-border); border-radius: 8px; padding: 8px 10px; display: grid; gap: 6px; min-width: 0; }
.pair.dismissed { opacity: 0.8; border-style: dashed; }
.pair-head { gap: 8px; flex-wrap: wrap; min-width: 0; }
.pair-items { display: grid; gap: 8px; grid-template-columns: repeat(auto-fit, minmax(min(280px, 100%), 1fr)); }
.pair-item { display: grid; gap: 4px; min-width: 0; }
.pair-actions { gap: 6px; flex-wrap: wrap; }
.evidence { margin: 0; padding: 4px 8px; border-left: 3px solid var(--qtf-border); font-size: 13px; overflow-wrap: anywhere; }
.evidence mark { background: var(--p-amber-200, #fde68a); color: #1f2937; border-radius: 2px; padding: 0 2px; font-weight: 600; }
.app-dark .evidence mark { background: var(--p-amber-300, #fcd34d); color: #111827; }
.survivor { gap: 8px; align-items: center; margin: 4px 0; }
</style>
