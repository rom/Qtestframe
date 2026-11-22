<script setup lang="ts">
// The verification of a requirement (requirement editor, tab Verification):
// is it verified, and why — per criterion with its evidence (tests, test
// steps, verification records), the children it rolls up, the records, the
// criteria the linked tests are mapped to, and the system elements it is
// allocated to.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import ItemRef from './ItemRef.vue'
import StatusTag from './StatusTag.vue'
import VerificationRecordDialog from './VerificationRecordDialog.vue'
import LinkCriteriaDialog from './LinkCriteriaDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { useI18n } from 'vue-i18n'
import { api, asItems } from '@/lib/api'
import { P } from '@/lib/permissions'
import { formatDateTime } from '@/lib/format'
import { elementOptions, evidenceLabel, evidenceSeverity, reasonText } from '@/lib/verification'
import { enumLabel } from '@/i18n/labels'
import type { EvaluationCriterion, Evidence, RequirementVerification, SystemElement, TraceLink, VerificationRecord } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; requirementId: string; criteria: EvaluationCriterion[]; canWrite: boolean; stamp?: string | number }>()
const emit = defineEmits<{ changed: []; state: [state: string] }>()
const { t, te } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const canRecord = computed(() => auth.can(P.runExecute))
const canSign = computed(() => auth.can(P.requirementReview) || auth.can(P.runExecute))
const canMap = computed(() => auth.can(P.testWrite))

const v = ref<RequirementVerification | null>(null)
const records = ref<VerificationRecord[]>([])
const links = ref<TraceLink[]>([])
const elements = ref<SystemElement[]>([])
const allocated = ref<string[]>([])
const savedAllocation = ref<string[]>([])
const loading = ref(false)
const failed = ref(false)

async function load() {
  loading.value = true
  failed.value = false
  try {
    const [ver, recs, ls, els] = await Promise.all([
      api.get<RequirementVerification>(`/requirements/${props.requirementId}/verification`),
      api.get<{ items: VerificationRecord[] }>(`/requirements/${props.requirementId}/verification-records`),
      api.get<{ items: TraceLink[] }>(`/requirements/${props.requirementId}/links`),
      api.get<{ items: SystemElement[] }>(`/projects/${props.projectId}/elements`),
    ])
    v.value = ver
    emit('state', ver.state)
    records.value = asItems(recs)
    links.value = asItems(ls)
    elements.value = asItems(els)
    allocated.value = elements.value.filter((e) => e.requirement_ids.includes(props.requirementId)).map((e) => e.id)
    savedAllocation.value = [...allocated.value]
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
}
watch(() => [props.requirementId, props.stamp], load, { immediate: true })

function changed() {
  void load()
  emit('changed')
}

const reasons = computed(() => (v.value?.reasons ?? []).map((r) => reasonText(t, te, r)))
const inconsistencies = computed(() => (v.value?.inconsistencies ?? []).map((r) => reasonText(t, te, r)))
const plainEvidence = computed(() => v.value?.evidence ?? [])
const criterionText = (id: string | null) => {
  if (!id) return t('verification.wholeRequirement')
  const i = props.criteria.findIndex((c) => c.id === id)
  return i >= 0 ? `${i + 1}. ${props.criteria[i].description}` : t('requirements.criterionGeneric')
}
function noteText(e: Evidence) {
  if (!e.note) return ''
  return te(`verification.note_.${e.note}`) ? t(`verification.note_.${e.note}`) : e.note
}
function evidenceTo(e: Evidence) {
  return e.kind === 'test' ? { name: 'tests', params: { projectId: props.projectId }, query: { item: e.id } } : undefined
}

// ---------- records ----------
const dialog = ref(false)
const editing = ref<VerificationRecord | null>(null)
const forCriterion = ref<string | null>(null)
function addRecord(criterionId: string | null) {
  editing.value = null
  forCriterion.value = criterionId
  dialog.value = true
}
function editRecord(r: VerificationRecord) {
  editing.value = r
  dialog.value = true
}
function deleteRecord(r: VerificationRecord) {
  notify.confirmDanger(t('verification.deleteRecordConfirm'), async () => {
    try {
      await api.del(`/verification-records/${r.id}`)
      changed()
    } catch (e) {
      notify.fail(e)
    }
  })
}
async function sign(r: VerificationRecord, on: boolean) {
  try {
    if (on) await api.post(`/verification-records/${r.id}/signoff`, {})
    else await api.del(`/verification-records/${r.id}/signoff`)
    notify.ok(on ? t('verification.signed') : t('verification.unsigned'))
    void load()
  } catch (e) {
    notify.fail(e)
  }
}
const mine = (r: VerificationRecord) => r.created_by === auth.me?.user.email
const validitySeverity = (s: string) => (s === 'valid' ? 'success' : s === 'expired' ? 'warn' : 'danger')

// ---------- link criteria ----------
const mapOpen = ref(false)
const mapping = ref<TraceLink | null>(null)
function mapLink(l: TraceLink) {
  mapping.value = l
  mapOpen.value = true
}
function linkCriteriaText(l: TraceLink) {
  const cs = l.criteria ?? []
  if (!cs.length) return t('verification.plainLink')
  return cs
    .map((c) => {
      const i = props.criteria.findIndex((x) => x.id === c.criterion_id)
      return c.step_id ? t('verification.criterionStep', { n: i + 1 }) : t('verification.criterionShort', { n: i + 1 })
    })
    .join(', ')
}

// ---------- allocation ----------
const elementChoices = computed(() => elementOptions(elements.value))
const allocationDirty = computed(() => [...allocated.value].sort().join() !== [...savedAllocation.value].sort().join())
async function saveAllocation() {
  try {
    await api.put(`/requirements/${props.requirementId}/elements`, { element_ids: allocated.value })
    savedAllocation.value = [...allocated.value]
    notify.ok(t('verification.allocationSaved'))
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <div class="verification-panel">
    <div v-if="loading && !v" class="empty" aria-busy="true"><ProgressSpinner style="width: 28px; height: 28px" :aria-label="t('common.loading')" /></div>
    <Message v-else-if="failed" severity="warn" :closable="false">{{ t('verification.loadFailed') }}</Message>
    <template v-else-if="v">
      <section class="ver-summary" :aria-label="t('verification.summary')">
        <div class="row" style="gap: 8px; flex-wrap: wrap">
          <StatusTag :value="v.state" kind="verification" />
          <span v-if="v.criteria_total" class="small">{{ t('verification.criteriaVerified', { n: v.criteria_verified, total: v.criteria_total }) }}</span>
          <span v-if="v.children.length" class="small">· {{ t('verification.childrenVerified', { n: v.children_verified, total: v.children.length }) }}</span>
          <Tag v-if="v.strict" :value="t('verification.strict')" severity="secondary" icon="pi pi-lock" />
          <span class="grow" />
          <HelpLink page="traceability-and-coverage" :anchor="{ en: 'verification', sv: 'verifiering' }" />
        </div>
        <p v-if="v.state !== v.own && v.children.length" class="small muted" style="margin: 6px 0 0">
          {{ t('verification.ownState', { state: enumLabel('verification', v.own) }) }}
        </p>
        <div v-if="reasons.length" class="small" style="margin-top: 8px">
          <b>{{ t('verification.whyNot') }}</b>
          <ul class="reasons">
            <li v-for="(r, i) in reasons" :key="i">{{ r }}</li>
          </ul>
        </div>
        <Message v-if="inconsistencies.length" severity="warn" :closable="false" style="margin-top: 8px">
          <b>{{ t('verification.inconsistent') }}</b>
          <ul class="reasons">
            <li v-for="(r, i) in inconsistencies" :key="i">{{ r }}</li>
          </ul>
        </Message>
      </section>

      <section>
        <h3 class="section-title"><i class="pi pi-verified" aria-hidden="true" /> {{ t('verification.byCriterion') }}</h3>
        <p v-if="!v.criteria.length" class="small muted">{{ t('verification.noCriteria') }}</p>
        <ul class="plain-list crit-list">
          <li v-for="c in v.criteria" :key="c.criterion_id" class="crit">
            <div class="row nowrap" style="gap: 8px; align-items: flex-start">
              <span class="step-dot" aria-hidden="true">{{ c.position }}</span>
              <div class="grow" style="min-width: 0">
                <div>
                  <span class="sr-only">{{ t('verification.criterionShort', { n: c.position }) }}: </span>{{ c.description }}
                </div>
                <div class="small muted">{{ enumLabel('evaluationMethod', c.method) }}</div>
              </div>
              <StatusTag :value="c.state" kind="verification" />
            </div>
            <ul class="plain-list evidence">
              <li v-for="(e, i) in c.evidence" :key="i" class="row nowrap" :class="{ void: e.state === 'void' }">
                <i :class="e.kind === 'test' ? 'pi pi-check-square' : 'pi pi-file-check'" aria-hidden="true" />
                <RouterLink v-if="evidenceTo(e)" :to="evidenceTo(e)!" class="mono small">{{ evidenceLabel(t, e) }}</RouterLink>
                <span v-else class="small">{{ evidenceLabel(t, e) }}<template v-if="e.title"> · {{ e.title }}</template></span>
                <span v-if="e.plain" class="small muted">({{ t('verification.plainLink') }})</span>
                <span class="grow" />
                <span v-if="noteText(e)" class="small muted">{{ noteText(e) }}</span>
                <Tag :value="e.kind === 'test' ? enumLabel('result', e.result) : t(`verification.result.${e.result}`)" :severity="evidenceSeverity(e.state)" />
              </li>
              <li v-if="!c.evidence.length" class="small muted">{{ t('verification.noEvidence') }}</li>
            </ul>
            <Button v-if="canRecord" size="small" text icon="pi pi-plus" :label="t('verification.addRecordFor', { n: c.position })" @click="addRecord(c.criterion_id)" />
          </li>
        </ul>
        <div v-if="plainEvidence.length || !v.criteria.length" class="crit">
          <div class="small"><b>{{ t('verification.wholeRequirement') }}</b></div>
          <ul class="plain-list evidence">
            <li v-for="(e, i) in plainEvidence" :key="i" class="row nowrap" :class="{ void: e.state === 'void' }">
              <i :class="e.kind === 'test' ? 'pi pi-check-square' : 'pi pi-file-check'" aria-hidden="true" />
              <RouterLink v-if="evidenceTo(e)" :to="evidenceTo(e)!" class="mono small">{{ evidenceLabel(t, e) }}</RouterLink>
              <span v-else class="small">{{ evidenceLabel(t, e) }}<template v-if="e.title"> · {{ e.title }}</template></span>
              <span class="grow" />
              <span v-if="noteText(e)" class="small muted">{{ noteText(e) }}</span>
              <Tag :value="e.kind === 'test' ? enumLabel('result', e.result) : t(`verification.result.${e.result}`)" :severity="evidenceSeverity(e.state)" />
            </li>
            <li v-if="!plainEvidence.length" class="small muted">{{ t('verification.noEvidence') }}</li>
          </ul>
          <Button v-if="canRecord && !v.criteria.length" size="small" text icon="pi pi-plus" :label="t('verification.addRecord')" @click="addRecord(null)" />
        </div>
      </section>

      <section v-if="v.children.length">
        <h3 class="section-title"><i class="pi pi-sitemap" aria-hidden="true" /> {{ t('verification.children') }}</h3>
        <ul class="plain-list">
          <li v-for="c in v.children" :key="c.id" class="row nowrap list-row">
            <ItemRef :to="{ name: 'requirements', params: { projectId }, query: { item: c.id } }" :item-key="c.key" :title="c.title" title-class="ellipsis" grow />
            <StatusTag :value="c.status" kind="status" />
            <StatusTag :value="c.state" kind="verification" />
          </li>
        </ul>
      </section>

      <section>
        <h3 class="section-title">
          <i class="pi pi-file-check" aria-hidden="true" /> {{ t('verification.records', { n: records.length }, records.length) }}
          <span class="grow" />
          <Button v-if="canRecord" size="small" text icon="pi pi-plus" :label="t('verification.addRecord')" @click="addRecord(null)" />
        </h3>
        <p v-if="!records.length" class="small muted">{{ t('verification.noRecords') }}</p>
        <ul class="plain-list">
          <li v-for="r in records" :key="r.id" class="record">
            <div class="row" style="gap: 8px; flex-wrap: wrap">
              <b>{{ enumLabel('evaluationMethod', r.method) }}</b>
              <Tag :value="t(`verification.result.${r.result}`)" :severity="r.result === 'fail' ? 'danger' : r.result === 'pass' ? 'success' : 'warn'" />
              <Tag :value="t(`verification.validity.${r.validity}`)" :severity="validitySeverity(r.validity)" />
              <span class="small muted">{{ criterionText(r.criterion_id) }}</span>
            </div>
            <div class="small">
              {{ t('verification.performedLine', { by: r.performed_by || '—', on: r.performed_on }) }}
              <template v-if="r.valid_until"> · {{ t('verification.validUntilLine', { date: r.valid_until }) }}</template>
              <template v-if="r.until_change"> · {{ t('verification.untilChangeShort') }}</template>
            </div>
            <div v-if="r.invalidated_reason" class="small warn-text">{{ t('verification.invalidatedBy', { reason: r.invalidated_reason }) }}</div>
            <div v-if="r.note" class="small pre-wrap">{{ r.note }}</div>
            <div v-if="r.files.length" class="row small" style="gap: 6px; flex-wrap: wrap">
              <a v-for="f in r.files" :key="f.id" :href="`/api/v1/files/${f.id}/content`" target="_blank" rel="noopener" class="row nowrap" style="gap: 4px">
                <i class="pi pi-file" aria-hidden="true" />{{ f.name }}
              </a>
            </div>
            <div class="small">
              <template v-if="r.signoff"><i class="pi pi-check-circle" aria-hidden="true" /> {{ t('verification.signedBy', { name: r.signoff.name || r.signoff.by, date: formatDateTime(r.signoff.at) }) }}</template>
              <span v-else class="muted">{{ t('verification.notSigned') }}</span>
            </div>
            <div class="row" style="gap: 4px; flex-wrap: wrap">
              <Button v-if="canSign && !r.signoff && !mine(r)" size="small" text icon="pi pi-pen-to-square" :label="t('verification.signOff')" @click="sign(r, true)" />
              <Button v-if="canSign && r.signoff && r.signoff.by === auth.me?.user.email" size="small" text severity="secondary" :label="t('verification.removeSignoff')" @click="sign(r, false)" />
              <Button v-if="canRecord" size="small" text icon="pi pi-pencil" :label="t('common.edit')" @click="editRecord(r)" />
              <Button v-if="canRecord" size="small" text severity="danger" icon="pi pi-trash" :label="t('common.delete')" @click="deleteRecord(r)" />
            </div>
          </li>
        </ul>
      </section>

      <section v-if="links.length && criteria.length">
        <h3 class="section-title"><i class="pi pi-link" aria-hidden="true" /> {{ t('verification.linkCriteria') }}</h3>
        <ul class="plain-list">
          <li v-for="l in links" :key="l.id" class="row nowrap list-row">
            <span class="mono small">{{ l.test_case_key }}</span>
            <span class="grow small muted">{{ linkCriteriaText(l) }}</span>
            <Button v-if="canMap" size="small" text icon="pi pi-sliders-h" :label="t('verification.mapCriteria')" :aria-label="t('verification.mapCriteriaFor', { key: l.test_case_key ?? '' })" @click="mapLink(l)" />
          </li>
        </ul>
      </section>

      <section>
        <h3 class="section-title"><i class="pi pi-box" aria-hidden="true" /> {{ t('verification.elements') }}</h3>
        <p v-if="!elements.length" class="small muted">
          {{ t('verification.noElements') }}
          <RouterLink :to="{ name: 'verification', params: { projectId }, query: { tab: 'elements' } }">{{ t('verification.manageElements') }}</RouterLink>
        </p>
        <div v-else class="row" style="gap: 8px; flex-wrap: wrap">
          <MultiSelect
            v-model="allocated"
            :options="elementChoices"
            option-label="label"
            option-value="value"
            filter
            display="chip"
            :disabled="!canWrite"
            :placeholder="t('verification.noAllocation')"
            :aria-label="t('verification.elements')"
            style="min-width: 0; width: min(420px, 100%)"
          />
          <Button v-if="canWrite" size="small" :label="t('common.save')" icon="pi pi-check" :disabled="!allocationDirty" @click="saveAllocation" />
        </div>
      </section>
    </template>
    <VerificationRecordDialog
      v-model:visible="dialog"
      :project-id="projectId"
      :requirement-id="requirementId"
      :criteria="criteria"
      :record="editing"
      :criterion-id="forCriterion"
      @saved="changed"
    />
    <LinkCriteriaDialog v-model:visible="mapOpen" :link="mapping" :criteria="criteria" @saved="changed" />
  </div>
</template>

<style scoped>
.verification-panel {
  display: grid;
  gap: 16px;
}
.ver-summary {
  padding: 10px 12px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
}
.reasons {
  margin: 4px 0 0;
  padding-left: 18px;
}
.crit,
.record {
  padding: 8px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.record {
  display: grid;
  gap: 4px;
}
.evidence {
  margin: 6px 0 0 30px;
  display: grid;
  gap: 4px;
}
.evidence li {
  gap: 6px;
  min-width: 0;
}
.evidence li.void {
  opacity: 0.75;
  text-decoration: line-through;
}
.list-row {
  padding: 5px 0;
  border-bottom: 1px solid var(--qtf-border);
  gap: 8px;
}
.pre-wrap {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
@media (max-width: 640px) {
  .evidence {
    margin-left: 0;
  }
  .evidence li {
    flex-wrap: wrap;
  }
}
</style>

<style>
/* the verification badge in the requirement editor header (opens the tab) */
.verification-badge {
  background: none;
  border: 0;
  padding: 0;
  cursor: pointer;
  border-radius: 6px;
}
.verification-badge:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
</style>
