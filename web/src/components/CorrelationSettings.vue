<script setup lang="ts">
// The project's change and defect correlation settings
// (GET/PUT /projects/{pid}/correlation-settings): along which relations a
// change travels, when results go stale, which defect severities block or
// warn, and what a regression does.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Select from 'primevue/select'
import { api, errorMessage } from '@/lib/api'
import { IMPACT_DIRECTIONS, REGRESSION_ACTIONS, RELATION_TYPES, SEVERITIES, setSeverityLevel, severityLevel, withDefaults, type CorrelationSettings } from '@/lib/correlations'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string; canEdit: boolean }>()
const { t } = useI18n()
const notify = useNotify()

const saved = ref<CorrelationSettings | null>(null)
const form = ref<CorrelationSettings>(withDefaults(null))
const loadError = ref('')
const saving = ref(false)
/** the age rule is on (stale_after_days > 0) */
const ageOn = ref(true)
const days = ref(90)

function fill(s: CorrelationSettings) {
  form.value = withDefaults(s)
  ageOn.value = form.value.stale_after_days > 0
  days.value = form.value.stale_after_days || 90
}
async function load() {
  loadError.value = ''
  try {
    saved.value = withDefaults(await api.get<CorrelationSettings>(`/projects/${props.projectId}/correlation-settings`))
    fill(saved.value)
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(load)

const payload = computed<CorrelationSettings>(() => ({ ...form.value, stale_after_days: ageOn.value ? Math.max(1, days.value || 1) : 0 }))
const dirty = computed(() => !!saved.value && JSON.stringify(payload.value) !== JSON.stringify(saved.value))

const directionOptions = computed(() => IMPACT_DIRECTIONS.map((d) => ({ value: d, label: t(`correlations.direction.${d}`) })))
const levelOptions = computed(() => ['block', 'warn', 'none'].map((l) => ({ value: l, label: t(`correlations.level.${l}`) })))
const actionOptions = computed(() => REGRESSION_ACTIONS.map((a) => ({ value: a, label: t(`correlations.action.${a}`) })))
function setLevel(sev: string, level: 'block' | 'warn' | 'none') {
  form.value = setSeverityLevel(form.value, sev, level)
}

async function save() {
  saving.value = true
  try {
    saved.value = withDefaults(await api.put<CorrelationSettings>(`/projects/${props.projectId}/correlation-settings`, payload.value))
    fill(saved.value)
    notify.ok(t('correlations.settingsSaved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="card corr-settings" style="max-width: 860px" aria-labelledby="h-corr">
    <h2 id="h-corr" class="plain-heading" style="margin-bottom: 4px">{{ t('correlations.settingsTitle') }}</h2>
    <p class="small muted" style="margin: 0 0 12px">{{ t('correlations.settingsIntro') }}</p>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <form v-else class="grid" style="gap: 18px" @submit.prevent="save">
      <fieldset class="plain-fieldset">
        <legend class="field-legend">{{ t('correlations.impactLegend') }}</legend>
        <p class="small muted" style="margin: 0 0 8px">{{ t('correlations.impactHint') }}</p>
        <div class="corr-grid">
          <template v-for="rt in RELATION_TYPES" :key="rt">
            <label :for="`corr-rel-${rt}`" class="small">{{ t(`correlations.relationType.${rt}`) }}</label>
            <Select :input-id="`corr-rel-${rt}`" v-model="form.relation_impact[rt]" :options="directionOptions" option-label="label" option-value="value" :disabled="!canEdit" />
          </template>
        </div>
        <div class="row" style="margin-top: 10px">
          <Checkbox v-model="form.flag_issues" binary input-id="corr-issues" :disabled="!canEdit" />
          <label for="corr-issues">{{ t('correlations.flagIssues') }}</label>
        </div>
      </fieldset>

      <fieldset class="plain-fieldset">
        <legend class="field-legend">{{ t('correlations.staleLegend') }}</legend>
        <p class="small muted" style="margin: 0 0 8px">{{ t('correlations.staleIntro') }}</p>
        <div class="row" style="gap: 8px">
          <Checkbox v-model="ageOn" binary input-id="corr-age" :disabled="!canEdit" />
          <label for="corr-age">{{ t('correlations.staleAge') }}</label>
          <InputNumber v-if="ageOn" v-model="days" :min="1" :max="3650" input-id="corr-days" :disabled="!canEdit" :aria-label="t('correlations.staleDays')" style="width: 110px" />
          <span v-if="ageOn" class="small">{{ t('correlations.days') }}</span>
        </div>
        <div class="row" style="gap: 8px; margin-top: 8px">
          <Checkbox v-model="form.stale_on_change" binary input-id="corr-change" :disabled="!canEdit" />
          <label for="corr-change">{{ t('correlations.staleOnChange') }}</label>
        </div>
      </fieldset>

      <fieldset class="plain-fieldset">
        <legend class="field-legend">{{ t('correlations.blockLegend') }}</legend>
        <p class="small muted" style="margin: 0 0 8px">{{ t('correlations.blockIntro') }}</p>
        <div class="corr-grid">
          <template v-for="sev in SEVERITIES" :key="sev">
            <label :for="`corr-sev-${sev}`" class="small">{{ t(`defectsView.severity.${sev}`) }}</label>
            <Select
              :input-id="`corr-sev-${sev}`"
              :model-value="severityLevel(form, sev)"
              :options="levelOptions"
              option-label="label"
              option-value="value"
              :disabled="!canEdit"
              @update:model-value="(v: 'block' | 'warn' | 'none') => setLevel(sev, v)"
            />
          </template>
        </div>
      </fieldset>

      <fieldset class="plain-fieldset">
        <legend class="field-legend">{{ t('correlations.regressionLegend') }}</legend>
        <p class="small muted" style="margin: 0 0 8px">{{ t('correlations.regressionIntro') }}</p>
        <div class="field" style="max-width: 360px">
          <label for="corr-action">{{ t('correlations.regressionAction') }}</label>
          <Select input-id="corr-action" v-model="form.regression_action" :options="actionOptions" option-label="label" option-value="value" :disabled="!canEdit" />
        </div>
      </fieldset>

      <div v-if="canEdit" class="row">
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!dirty" />
      </div>
    </form>
  </section>
</template>

<style scoped>
.plain-fieldset {
  border: none;
  margin: 0;
  padding: 0;
}
.corr-grid {
  display: grid;
  grid-template-columns: minmax(120px, max-content) minmax(0, 260px);
  gap: 6px 12px;
  align-items: center;
}
@media (max-width: 480px) {
  .corr-grid {
    grid-template-columns: 1fr;
  }
}
</style>
