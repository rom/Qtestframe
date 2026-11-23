<script setup lang="ts">
// One system setting (Administration → System settings): its control, the
// source badge (default / file / environment / changed here), the lock and
// restart state, the file value and "Reset to file value".
import { computed } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useI18n } from 'vue-i18n'
import { uid } from '@/lib/a11y'
import { formatDateTime } from '@/lib/format'
import { displayValue, fieldId, sourceSeverity, type FormValue, type SettingView } from '@/lib/systemSettings'

const props = defineProps<{ setting: SettingView; modelValue: FormValue; removeSecret?: boolean; busy?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [v: FormValue]; 'update:removeSecret': [v: boolean]; reset: [] }>()
const { t } = useI18n()

const s = computed(() => props.setting)
const id = fieldId(props.setting.key)
const ids = { input: uid('sys'), label: uid('sysl'), hint: uid('sysh'), state: uid('syss') }
const label = computed(() => t(`sysSettings.fields.${id}.label`))
const hint = computed(() => t(`sysSettings.fields.${id}.hint`))
const describedBy = `${ids.hint} ${ids.state}`
const options = computed(() => (s.value.options ?? []).map((o) => ({ label: o, value: o })))

function show(v: unknown): string {
  if (typeof v === 'boolean') return v ? t('sysSettings.yes') : t('sysSettings.no')
  return displayValue(v) ?? t('sysSettings.empty')
}
const fileText = computed(() => {
  if (s.value.type === 'secret') return s.value.file_has_secret ? t('sysSettings.secretSet') : t('sysSettings.secretUnset')
  return show(s.value.file_value)
})
const lockText = computed(() => {
  if (!s.value.locked) return ''
  if (s.value.lock_reason === 'environment') return t('sysSettings.lock.environment', { env: s.value.env ?? '' })
  if (s.value.lock_reason === 'config') return t('sysSettings.lock.config')
  return t('sysSettings.lock.readonly')
})
const model = computed({
  get: () => props.modelValue,
  set: (v: FormValue) => emit('update:modelValue', v),
})
</script>

<template>
  <div class="sys-field" :class="{ 'is-locked': s.locked }" :data-key="s.key">
    <div class="sys-field-head">
      <label :id="ids.label" :for="ids.input" class="sys-label">{{ label }}</label>
      <span class="sys-badges">
        <Tag :value="t(`sysSettings.source.${s.source}`)" :severity="sourceSeverity(s.source)" class="sys-badge" />
        <Tag v-if="s.apply === 'restart'" :value="t('sysSettings.apply.restart')" severity="warn" class="sys-badge" icon="pi pi-refresh" />
        <Tag v-if="s.locked" :value="t(s.lock_reason === 'environment' ? 'sysSettings.apply.environment' : 'sysSettings.apply.readonly')" severity="secondary" class="sys-badge" icon="pi pi-lock" />
        <Tag v-if="s.pending_restart" :value="t('sysSettings.apply.pending')" severity="danger" class="sys-badge" icon="pi pi-clock" />
      </span>
    </div>

    <div class="sys-control">
      <template v-if="s.type === 'bool'">
        <ToggleSwitch v-model="model as boolean" :input-id="ids.input" :disabled="s.locked || busy" :aria-describedby="describedBy" />
        <span class="small" aria-hidden="true">{{ model ? t('sysSettings.yes') : t('sysSettings.no') }}</span>
      </template>
      <InputNumber v-else-if="s.type === 'int'" v-model="model as number" :input-id="ids.input" :min="s.min" :max="s.max" :use-grouping="false" :disabled="s.locked || busy" :aria-describedby="describedBy" fluid />
      <Select v-else-if="s.type === 'enum'" :model-value="model" :options="options" option-label="label" option-value="value" :label-id="ids.input" :aria-labelledby="ids.label" :disabled="s.locked || busy" :aria-describedby="describedBy" fluid @update:model-value="(v: string) => emit('update:modelValue', v)" />
      <template v-else-if="s.type === 'secret' && s.locked">
        <InputText :id="ids.input" :model-value="s.has_secret ? t('sysSettings.secretSet') : t('sysSettings.secretUnset')" disabled :aria-describedby="describedBy" fluid />
      </template>
      <template v-else-if="s.type === 'secret'">
        <InputText :id="ids.input" v-model="model as string" type="password" autocomplete="new-password" :placeholder="s.has_secret ? t('sysSettings.secretKeep') : ''" :disabled="s.locked || busy || removeSecret" :aria-describedby="describedBy" fluid />
      </template>
      <Textarea v-else-if="s.type === 'list'" :id="ids.input" v-model="model as string" :rows="Math.min(6, Math.max(2, String(model ?? '').split('\n').length + 1))" :disabled="s.locked || busy" spellcheck="false" class="mono" :aria-describedby="describedBy" fluid />
      <InputText v-else :id="ids.input" v-model="model as string" :disabled="s.locked || busy" spellcheck="false" :class="{ mono: s.type !== 'string' || s.key.endsWith('url') }" :aria-describedby="describedBy" fluid />
    </div>

    <small :id="ids.hint" class="muted sys-hint">{{ hint }}<template v-if="s.type === 'list' && !s.locked"> {{ t('sysSettings.listHint') }}</template></small>
    <div :id="ids.state" class="small sys-state">
      <span v-if="s.type === 'secret'"><i class="pi pi-key" aria-hidden="true" /> {{ s.has_secret ? t('sysSettings.secretSet') : t('sysSettings.secretUnset') }}</span>
      <span v-if="lockText" class="sys-lock"><i class="pi pi-lock" aria-hidden="true" /> {{ lockText }}</span>
      <span v-if="s.ignored" class="warn-text">{{ t('sysSettings.ignored') }}</span>
      <span v-if="s.source === 'database'" class="muted">{{ t('sysSettings.fileValue', { value: fileText }) }}</span>
      <span v-if="s.pending_restart" class="warn-text"><i class="pi pi-refresh" aria-hidden="true" /> {{ t('sysSettings.runningValue', { value: show(s.running_value) }) }}</span>
      <span v-if="s.stored && s.updated_by" class="muted">{{ t('sysSettings.changedBy', { who: s.updated_by, date: formatDateTime(s.updated_at) }) }}</span>
    </div>
    <div v-if="(s.type === 'secret' && s.has_secret && s.source === 'database' && !s.locked) || (s.stored && !s.locked)" class="row sys-actions">
      <div v-if="s.type === 'secret' && s.has_secret && s.source === 'database' && !s.locked" class="row nowrap" style="gap: 6px">
        <Checkbox :model-value="removeSecret" :input-id="`${ids.input}-rm`" binary :disabled="busy" @update:model-value="(v: boolean) => emit('update:removeSecret', v)" />
        <label :for="`${ids.input}-rm`" class="small">{{ t('sysSettings.secretRemove') }}</label>
      </div>
      <Button v-if="s.stored && !s.locked" :label="t('sysSettings.reset')" :aria-label="t('sysSettings.resetNamed', { name: label })" icon="pi pi-undo" size="small" text :disabled="busy" @click="emit('reset')" />
    </div>
  </div>
</template>

<style scoped>
.sys-field {
  display: grid;
  gap: 4px;
  padding: 12px 0;
  border-top: 1px solid var(--qtf-border);
}
.sys-field:first-child {
  border-top: 0;
}
.sys-field-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 10px;
}
.sys-label {
  font-weight: 600;
}
.sys-badges {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
}
.sys-badge {
  font-size: 11px;
}
.sys-control {
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 520px;
}
.sys-state {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 14px;
}
.sys-state:empty {
  display: none;
}
.sys-actions {
  gap: 12px;
}
</style>
