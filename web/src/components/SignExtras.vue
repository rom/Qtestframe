<script setup lang="ts">
// The fields electronic-records mode adds to a signature dialog: the
// meaning of the signature (from the organisation's list, unless the dialog
// has its own), the second-factor code when the signer has one, and the
// note that the time is recorded in UTC with the signer's time zone.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from 'primevue/select'
import MfaCodeInput from './MfaCodeInput.vue'
import { browserTimeZone, meaningLabel } from '@/lib/erecords'
import { useAuthStore } from '@/stores/auth'
import { useERecordsStore } from '@/stores/erecords'

const props = withDefaults(defineProps<{ idPrefix: string; withMeaning?: boolean; meanings?: string[] }>(), { withMeaning: true, meanings: undefined })
const meaning = defineModel<string>('meaning', { default: '' })
const mfaCode = defineModel<string>('mfaCode', { default: '' })
const { t } = useI18n()
const auth = useAuthStore()
const er = useERecordsStore()
const options = computed(() => (props.meanings ?? er.meanings).map((m) => ({ label: meaningLabel(t, m), value: m })))
const needsMfa = computed(() => er.enabled && !!auth.user?.mfa_enabled)
const zone = browserTimeZone()
</script>

<template>
  <template v-if="er.enabled">
    <template v-if="withMeaning">
      <label :for="`${idPrefix}-meaning`">{{ t('erecords.sign.meaning') }}</label>
      <Select v-model="meaning" :input-id="`${idPrefix}-meaning`" :options="options" option-label="label" option-value="value" :placeholder="t('erecords.sign.meaningPlaceholder')" fluid aria-required="true" />
    </template>
    <MfaCodeInput v-if="needsMfa" :id="`${idPrefix}-mfa`" v-model="mfaCode" :label="t('erecords.sign.mfa')" />
    <p class="small muted" style="margin: 0">{{ t('erecords.sign.recorded', { zone }) }}</p>
  </template>
</template>
