<script setup lang="ts">
// One MFA policy (server or tenant): mode, roles, grace period and, for the
// server, "remember this device".
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import MultiSelect from 'primevue/multiselect'
import RadioButton from 'primevue/radiobutton'
import { useI18n } from 'vue-i18n'
import { MFA_MODES, type MfaMode, type MfaPolicy } from '@/lib/mfa'
import { formatDateTime } from '@/lib/format'

const props = defineProps<{
  policy: MfaPolicy
  /** id prefix of the controls (several forms on the page) */
  idp: string
  roleOptions: { label: string; value: string }[]
  showRemember?: boolean
  canEdit: boolean
  saving?: boolean
}>()
const emit = defineEmits<{ save: [req: { mode: MfaMode; roles: string[]; grace_days: number; remember_days: number }] }>()
const { t } = useI18n()

const toForm = (p: MfaPolicy) => ({ mode: p.mode, roles: [...(p.roles ?? [])], grace_days: p.grace_days ?? 0, remember_days: p.remember_days ?? 0 })
const form = ref(toForm(props.policy))
const saved = computed(() => JSON.stringify(toForm(props.policy)))
watch(saved, () => (form.value = toForm(props.policy)))
const dirty = computed(() => JSON.stringify(form.value) !== saved.value)
const invalid = computed(() => form.value.mode === 'roles' && form.value.roles.length === 0)
function submit() {
  if (invalid.value) return
  emit('save', { ...form.value, roles: form.value.mode === 'roles' ? form.value.roles : [] })
}
</script>

<template>
  <form class="grid" style="gap: 16px" @submit.prevent="submit">
    <fieldset class="plain-fieldset setting">
      <legend class="setting-title">{{ t('mfa.policy.requirement') }}</legend>
      <div class="grid" style="gap: 10px">
        <div v-for="m in MFA_MODES" :key="m" class="row nowrap" style="gap: 10px; align-items: flex-start">
          <RadioButton v-model="form.mode" :input-id="`${idp}-mode-${m}`" :name="`${idp}-mode`" :value="m" :disabled="!canEdit" :aria-describedby="`${idp}-mode-${m}-d`" />
          <div>
            <label :for="`${idp}-mode-${m}`"><b>{{ t(`mfa.policy.mode.${m}`) }}</b></label>
            <p :id="`${idp}-mode-${m}-d`" class="small muted" style="margin: 2px 0 0">{{ t(`mfa.policy.modeHint.${m}`) }}</p>
          </div>
        </div>
        <div v-if="form.mode === 'roles'" v-field class="field" style="max-width: 420px">
          <label>{{ t('mfa.policy.roles') }}</label>
          <MultiSelect v-model="form.roles" :options="roleOptions" option-label="label" option-value="value" display="chip" :disabled="!canEdit" :invalid="invalid" :aria-describedby="`${idp}-roles-hint`" />
          <small :id="`${idp}-roles-hint`" :class="invalid ? 'error-text' : 'muted'" :role="invalid ? 'alert' : undefined">{{ invalid ? t('mfa.policy.rolesNeeded') : t('mfa.policy.rolesHint') }}</small>
        </div>
      </div>
    </fieldset>
    <fieldset class="plain-fieldset setting" :disabled="form.mode === 'optional'">
      <legend class="setting-title">{{ t('mfa.policy.grace') }}</legend>
      <div v-field class="field" style="max-width: 260px">
        <label>{{ t('mfa.policy.graceDays') }}</label>
        <InputNumber v-model="form.grace_days" :min="0" :max="365" show-buttons :disabled="!canEdit || form.mode === 'optional'" :aria-describedby="`${idp}-grace-hint`" />
      </div>
      <p :id="`${idp}-grace-hint`" class="small muted" style="margin: 6px 0 0">{{ t('mfa.policy.graceHint') }}</p>
    </fieldset>
    <fieldset v-if="showRemember" class="plain-fieldset setting">
      <legend class="setting-title">{{ t('mfa.policy.rememberTitle') }}</legend>
      <div v-field class="field" style="max-width: 260px">
        <label>{{ t('mfa.policy.rememberDays') }}</label>
        <InputNumber v-model="form.remember_days" :min="0" :max="365" show-buttons :disabled="!canEdit" :aria-describedby="`${idp}-remember-hint`" />
      </div>
      <p :id="`${idp}-remember-hint`" class="small muted" style="margin: 6px 0 0">{{ t('mfa.policy.rememberHint') }}</p>
    </fieldset>
    <p v-if="policy.updated_at" class="small muted" style="margin: 0">{{ t('mfa.policy.updated', { when: formatDateTime(policy.updated_at), who: policy.updated_by || '—' }) }}</p>
    <div v-if="canEdit" class="row" style="justify-content: flex-end">
      <span v-if="dirty" class="small muted" style="margin-right: auto">{{ t('execSettings.unsaved') }}</span>
      <Button :label="t('common.cancel')" text severity="secondary" type="button" :disabled="!dirty" @click="form = toForm(policy)" />
      <Button :label="t('common.save')" icon="pi pi-check" type="submit" :loading="saving" :disabled="!dirty || invalid" />
    </div>
  </form>
</template>

<style scoped>
.setting {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 12px 14px;
}
.setting-title {
  font-weight: 700;
  padding: 0 4px;
}
</style>
