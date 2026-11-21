<script setup lang="ts">
// The code field of the second factor: a 6-digit authenticator code, or a
// recovery code. Labelled, autofilled by the platform (one-time-code) and
// tied to its error message, which is announced (role="alert").
import { computed, ref } from 'vue'
import InputText from 'primevue/inputtext'
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    id: string
    /** recovery code instead of an authenticator code */
    recovery?: boolean
    error?: string
    autofocus?: boolean
    /** the label (default: "Authentication code" / "Recovery code") */
    label?: string
  }>(),
  { recovery: false, error: '', autofocus: false, label: '' },
)
const model = defineModel<string>({ default: '' })
const { t } = useI18n()
const input = ref<{ $el: HTMLInputElement } | null>(null)
const hintId = computed(() => `${props.id}-hint`)
const errorId = computed(() => `${props.id}-error`)
const describedBy = computed(() => [hintId.value, props.error ? errorId.value : ''].filter(Boolean).join(' '))

function focus() {
  input.value?.$el?.focus()
}
defineExpose({ focus })
</script>

<template>
  <div v-field class="field mfa-code">
    <label :for="id">{{ label || (recovery ? t('mfa.recoveryCode') : t('mfa.code')) }}</label>
    <InputText
      :id="id"
      ref="input"
      v-model="model"
      :class="{ mono: true, recovery }"
      :inputmode="recovery ? 'text' : 'numeric'"
      :autocomplete="recovery ? 'off' : 'one-time-code'"
      :pattern="recovery ? undefined : '[0-9 ]*'"
      :maxlength="recovery ? 20 : 8"
      autocapitalize="off"
      spellcheck="false"
      required
      :autofocus="autofocus"
      :invalid="!!error"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="describedBy"
    />
    <span :id="hintId" class="small muted">{{ recovery ? t('mfa.recoveryHint') : t('mfa.codeHint') }}</span>
    <span v-if="error" :id="errorId" class="small error-text" role="alert">{{ error }}</span>
  </div>
</template>

<style scoped>
.mfa-code :deep(input) {
  font-size: 1.25em;
  letter-spacing: 0.2em;
  max-width: 14ch;
}
.mfa-code :deep(input.recovery) {
  max-width: 22ch;
  letter-spacing: 0.08em;
}
</style>
