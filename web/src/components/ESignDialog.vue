<script setup lang="ts">
// An electronic signature: what is signed, an optional decision (approve
// or reject), a comment and the signer's password (or, for single sign-on
// accounts, a recent sign-in). Used for waiver decisions and for accepting
// residual risks; the parent sends the request and reports errors.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Password from 'primevue/password'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  header: string
  explain: string
  /** offer approve / reject */
  decisions?: boolean
  /** a comment is required for a rejection (decisions) or always */
  commentRequired?: 'reject' | 'always'
  confirmLabel?: string
  busy?: boolean
  error?: string
}>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ sign: [value: { decision: string; comment: string; password: string }] }>()
const { t } = useI18n()
const auth = useAuthStore()
const needsPassword = computed(() => (auth.user?.auth_provider ?? 'local') === 'local')
const decision = ref('approve')
const comment = ref('')
const password = ref('')
const decisionOptions = computed(() => [
  { label: t('esign.approve'), value: 'approve' },
  { label: t('esign.reject'), value: 'reject' },
])
watch(visible, (v) => {
  if (v) {
    decision.value = 'approve'
    comment.value = ''
    password.value = ''
  }
})
watch(
  () => props.busy,
  (b) => {
    if (!b) password.value = ''
  },
)
const needComment = computed(() => props.commentRequired === 'always' || (props.commentRequired === 'reject' && decision.value === 'reject'))
const canSign = computed(() => (!needsPassword.value || !!password.value) && (!needComment.value || comment.value.trim().length >= 3))
function sign() {
  if (!canSign.value || props.busy) return
  emit('sign', { decision: props.decisions ? decision.value : 'approve', comment: comment.value.trim(), password: password.value })
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="header" :style="{ width: 'min(500px, 96vw)' }">
    <form class="esign" @submit.prevent="sign">
      <p class="small" style="margin-top: 0">{{ explain }}</p>
      <template v-if="decisions">
        <span id="esign-decision" class="field-label">{{ t('esign.decision') }}</span>
        <SelectButton v-model="decision" :options="decisionOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="esign-decision" />
      </template>
      <label for="esign-comment">{{ needComment ? t('esign.commentRequired') : t('esign.comment') }}</label>
      <Textarea id="esign-comment" v-model="comment" rows="2" auto-resize fluid :aria-required="needComment" />
      <template v-if="needsPassword">
        <label for="esign-pw">{{ t('esign.password') }}</label>
        <Password v-model="password" input-id="esign-pw" :feedback="false" toggle-mask fluid autocomplete="current-password" />
      </template>
      <p v-else class="small muted">{{ t('esign.ssoHint') }}</p>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
        <Button type="submit" :label="confirmLabel || t('esign.sign')" icon="pi pi-pen-to-square" :disabled="!canSign" :loading="busy" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.esign {
  display: grid;
  gap: 8px;
}
</style>
