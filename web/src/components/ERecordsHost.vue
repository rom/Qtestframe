<script setup lang="ts">
// Electronic-records mode in the browser, mounted once in App.vue: when a
// request answers 428 (lib/api.ts), it asks for the reason for change or
// for the electronic signature (password, second factor, meaning), gets a
// signing ticket and lets the request be sent again.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Textarea from 'primevue/textarea'
import SignExtras from './SignExtras.vue'
import { api, ApiError, errorMessage, setChallengeHandler } from '@/lib/api'
import { browserTimeZone, challengeOf, encodeReason, type ERecordsChallenge } from '@/lib/erecords'
import { useAuthStore } from '@/stores/auth'
import { useERecordsStore } from '@/stores/erecords'

const { t, te } = useI18n()
const auth = useAuthStore()
const er = useERecordsStore()

const reasonOpen = ref(false)
const signOpen = ref(false)
const challenge = ref<ERecordsChallenge | null>(null)
const reason = ref('')
const password = ref('')
const mfaCode = ref('')
const meaning = ref('')
const comment = ref('')
const busy = ref(false)
const error = ref('')
let settle: ((h: Record<string, string> | null) => void) | null = null
// one dialog at a time: challenges of parallel requests wait their turn
let queue: Promise<unknown> = Promise.resolve()

const needsPassword = computed(() => (auth.user?.auth_provider ?? 'local') === 'local')
const editLabel = computed(() => {
  const c = challenge.value
  if (c?.kind !== 'reason') return ''
  return te(`erecords.edit.${c.editType}`) ? t(`erecords.edit.${c.editType}`) : c.editType
})
const actionLabel = computed(() => {
  const c = challenge.value
  if (c?.kind !== 'signature') return ''
  return te(`erecords.actionName.${c.action}`) ? t(`erecords.actionName.${c.action}`) : c.action
})
const meanings = computed(() => (challenge.value?.kind === 'signature' && challenge.value.meanings.length ? challenge.value.meanings : er.meanings))
const canSign = computed(() => !!meaning.value && (!needsPassword.value || !!password.value) && (!(er.enabled && auth.user?.mfa_enabled) || mfaCode.value.trim().length >= 6))

function ask(err: ApiError): Promise<Record<string, string> | null> {
  const c = challengeOf(err)
  if (!c) return Promise.resolve(null)
  const run = () =>
    new Promise<Record<string, string> | null>((resolve) => {
      challenge.value = c
      error.value = ''
      settle = resolve
      if (c.kind === 'reason') {
        reason.value = ''
        reasonOpen.value = true
      } else {
        void er.ensure(true)
        password.value = mfaCode.value = comment.value = ''
        meaning.value = c.meanings.length === 1 ? c.meanings[0] : ''
        signOpen.value = true
      }
    })
  const p = queue.then(run, run)
  queue = p.catch(() => null)
  return p
}

function finish(h: Record<string, string> | null) {
  const f = settle
  settle = null
  reasonOpen.value = signOpen.value = false
  password.value = mfaCode.value = ''
  f?.(h)
}

function giveReason() {
  const r = reason.value.trim()
  if (r.length < 3) return
  finish({ 'X-Change-Reason': encodeReason(r) })
}

async function sign() {
  const c = challenge.value
  if (c?.kind !== 'signature' || !canSign.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const res = await api.post<{ ticket: string }>('/esign/tickets', {
      action: c.action,
      password: password.value,
      mfa_code: mfaCode.value.trim(),
      meaning: meaning.value,
      comment: comment.value.trim(),
      time_zone: browserTimeZone(),
    })
    finish({ 'X-Signature-Ticket': res.ticket })
  } catch (e) {
    password.value = ''
    mfaCode.value = ''
    error.value = e instanceof ApiError && te(`erecords.err.${e.code}`) ? t(`erecords.err.${e.code}`) : errorMessage(e)
  } finally {
    busy.value = false
  }
}

const reasonVisible = computed({
  get: () => reasonOpen.value,
  set: (v: boolean) => {
    if (!v) finish(null)
  },
})
const signVisible = computed({
  get: () => signOpen.value,
  set: (v: boolean) => {
    if (!v) finish(null)
  },
})

onMounted(() => setChallengeHandler(ask))
onBeforeUnmount(() => setChallengeHandler(null))
</script>

<template>
  <Dialog v-model:visible="reasonVisible" modal :header="t('erecords.reason.title')" :style="{ width: 'min(480px, 96vw)' }">
    <form class="er-form" @submit.prevent="giveReason">
      <p class="small" style="margin-top: 0">{{ t('erecords.reason.explain', { edit: editLabel }) }}</p>
      <label for="er-reason">{{ t('erecords.reason.label') }}</label>
      <Textarea id="er-reason" v-model="reason" rows="3" auto-resize fluid autofocus aria-required="true" aria-describedby="er-reason-hint" />
      <span id="er-reason-hint" class="small muted">{{ t('erecords.reason.hint') }}</span>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="finish(null)" />
        <Button type="submit" :label="t('erecords.reason.continue')" icon="pi pi-check" :disabled="reason.trim().length < 3" />
      </div>
    </form>
  </Dialog>

  <Dialog v-model:visible="signVisible" modal :header="t('erecords.sign.title')" :style="{ width: 'min(500px, 96vw)' }">
    <form class="er-form" @submit.prevent="sign">
      <p class="small" style="margin-top: 0">{{ t('erecords.sign.explain', { action: actionLabel }) }}</p>
      <SignExtras v-model:meaning="meaning" v-model:mfa-code="mfaCode" id-prefix="er-sign" :meanings="meanings" />
      <label for="er-comment">{{ t('esign.comment') }}</label>
      <Textarea id="er-comment" v-model="comment" rows="2" auto-resize fluid />
      <template v-if="needsPassword">
        <label for="er-pw">{{ t('esign.password') }}</label>
        <Password v-model="password" input-id="er-pw" :feedback="false" toggle-mask fluid autocomplete="current-password" />
      </template>
      <p v-else class="small muted">{{ t('erecords.sign.ssoHint', { n: er.settings?.sso_reauth_minutes ?? 5 }) }}</p>
      <Message v-if="error" severity="error" :closable="false"><span role="alert">{{ error }}</span></Message>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="finish(null)" />
        <Button type="submit" :label="t('esign.sign')" icon="pi pi-pen-to-square" :disabled="!canSign" :loading="busy" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.er-form {
  display: grid;
  gap: 8px;
}
.er-form label {
  font-weight: 600;
  font-size: 13px;
}
</style>
