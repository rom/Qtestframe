<script setup lang="ts">
// Electronic sign-offs of a baseline: who signed, what it means, whether
// the content still matches, and "Sign" (with password confirmation).
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { api, ApiError, asItems } from '@/lib/api'
import { P } from '@/lib/permissions'
import { shortHash } from '@/lib/notifications'
import { formatDateTime, formatFullDateTime } from '@/lib/format'
import SignExtras from './SignExtras.vue'
import ESignaturesPanel from './ESignaturesPanel.vue'
import { browserTimeZone, meaningLabel, signErrorKey } from '@/lib/erecords'
import { useAuthStore } from '@/stores/auth'
import { useERecordsStore } from '@/stores/erecords'
import { useNotify } from '@/composables/notify'
import type { Signoff } from '@/lib/types'

const props = defineProps<{ baselineId: string }>()
const emit = defineEmits<{ changed: [count: number] }>()
const { t } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const list = ref<Signoff[]>([])
const signing = ref(false)
const meaning = ref('approved')
const comment = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')

const canSign = computed(() => auth.can(P.requirementReview) || auth.can(P.testReview))
// electronic-records mode: the organisation's meanings and the second factor (SignExtras)
const er = useERecordsStore()
const mfaCode = ref('')
const meanings = computed(() =>
  er.enabled ? er.meanings.map((m) => ({ label: meaningLabel(t, m), value: m })) : ['authored', 'reviewed', 'approved', 'released'].map((m) => ({ label: t(`signoff.meaning.${m}`), value: m })),
)
const label = (m: string) => (['authored', 'reviewed', 'approved', 'released'].includes(m) ? t(`signoff.meaning.${m}`) : meaningLabel(t, m))
const needsMfa = computed(() => er.enabled && !!auth.user?.mfa_enabled)
const needsPassword = computed(() => (auth.user?.auth_provider ?? 'local') === 'local')

async function load() {
  try {
    list.value = asItems(await api.get<{ items: Signoff[] }>(`/baselines/${props.baselineId}/signoffs`))
    emit('changed', list.value.length)
  } catch {
    list.value = []
  }
}
onMounted(load)
watch(() => props.baselineId, load)

function openSign() {
  meaning.value = er.enabled ? (er.meanings.includes('approved') ? 'approved' : (er.meanings[0] ?? '')) : 'approved'
  mfaCode.value = ''
  comment.value = ''
  password.value = ''
  error.value = ''
  signing.value = true
}

async function sign() {
  busy.value = true
  error.value = ''
  try {
    await api.post(`/baselines/${props.baselineId}/signoffs`, {
      meaning: meaning.value,
      comment: comment.value,
      password: password.value,
      mfa_code: mfaCode.value.trim(),
      time_zone: browserTimeZone(),
    })
    notify.ok(t('signoff.signed'))
    signing.value = false
    await load()
  } catch (e) {
    if (e instanceof ApiError && (e.code === 'reauth_failed' || e.code === 'reauth_required')) error.value = t(`signoff.${e.code}`)
    else if (e instanceof ApiError && signErrorKey(e.code)) error.value = t(signErrorKey(e.code))
    else notify.fail(e)
  } finally {
    busy.value = false
    password.value = ''
    mfaCode.value = ''
  }
}
</script>

<template>
  <div class="signoffs">
    <div class="row head">
      <span class="grow small"><b><i class="pi pi-pen-to-square" aria-hidden="true" /> {{ t('signoff.title') }}</b></span>
      <Button v-if="canSign" :label="t('signoff.sign')" icon="pi pi-pencil" size="small" outlined @click="openSign" />
    </div>
    <p v-if="!list.length" class="small muted">{{ t('signoff.none') }}</p>
    <ul v-else class="list">
      <li v-for="s in list" :key="s.id" class="row">
        <Tag :value="label(s.meaning)" severity="secondary" />
        <span class="grow small">
          <b>{{ s.user_name || s.user_email }}</b>
          <span class="muted" :title="formatFullDateTime(s.signed_at)"> · {{ formatDateTime(s.signed_at) }}</span>
          <span v-if="s.comment" class="comment">{{ s.comment }}</span>
        </span>
        <span class="small mono" :title="s.content_hash">{{ shortHash(s.content_hash) }}</span>
        <Tag v-if="s.valid" :value="t('signoff.valid')" severity="success" icon="pi pi-check" />
        <Tag v-else :value="t('signoff.invalid')" severity="danger" icon="pi pi-exclamation-triangle" />
      </li>
    </ul>
    <ESignaturesPanel entity-type="baseline" :entity-id="baselineId" :stamp="list.length" />
    <Dialog v-model:visible="signing" modal :header="t('signoff.dialogTitle')" :style="{ width: 'min(480px, 96vw)' }">
      <p class="small">{{ t('signoff.explain') }}</p>
      <div class="form">
        <label for="so-meaning">{{ t('signoff.meaningLabel') }}</label>
        <Select v-model="meaning" input-id="so-meaning" :options="meanings" option-label="label" option-value="value" fluid />
        <label for="so-comment">{{ t('signoff.comment') }}</label>
        <Textarea id="so-comment" v-model="comment" rows="2" auto-resize fluid />
        <template v-if="needsPassword">
          <label for="so-pw">{{ t('signoff.password') }}</label>
          <Password v-model="password" input-id="so-pw" :feedback="false" toggle-mask fluid autocomplete="current-password" @keydown.enter="sign" />
        </template>
        <p v-else class="small muted">{{ t('signoff.ssoHint') }}</p>
        <SignExtras v-model:mfa-code="mfaCode" id-prefix="so" :with-meaning="false" />
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" severity="secondary" text @click="signing = false" />
        <Button :label="t('signoff.confirm')" icon="pi pi-check" :disabled="(needsPassword && !password) || !meaning || (needsMfa && mfaCode.trim().length < 6)" :loading="busy" @click="sign" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.signoffs {
  margin: 8px 0 12px;
  padding: 8px 12px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
}
.head {
  gap: 8px;
}
.list {
  list-style: none;
  margin: 4px 0 0;
  padding: 0;
}
.list li {
  gap: 8px;
  padding: 4px 0;
  flex-wrap: wrap;
}
.comment {
  display: block;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.form label {
  font-weight: 600;
  font-size: 13px;
  margin-top: 6px;
}
</style>
