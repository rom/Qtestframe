<script setup lang="ts">
// "Send by e-mail" for a report: members and (with project.update) other
// addresses, a message and the format; the server renders and sends it.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import Textarea from 'primevue/textarea'
import Message from 'primevue/message'
import { api, asItems, errorMessage } from '@/lib/api'
import { FORMAT_INFO, type ReportFormat } from '@/lib/reports'
import { MAX_MESSAGE, reportMailBody, reportMailProblems, type ReportSource } from '@/lib/email'
import type { ReportRecipient } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ source: ReportSource | null; name: string; formats: readonly ReportFormat[]; format: ReportFormat }>()
const visible = defineModel<boolean>('visible', { required: true })
const proj = useProjectStore()
const notify = useNotify()
const { t } = useI18n()

const recipients = ref<ReportRecipient[]>([])
const addressesAllowed = ref(false)
const loadError = ref('')
const form = ref({ recipients: [] as string[], addresses: '', message: '', format: props.format as string })
const error = ref('')
const showProblems = ref(false)
const busy = ref(false)
const options = computed(() => recipients.value.map((r) => ({ label: r.display_name && r.display_name !== r.email ? `${r.display_name} · ${r.email}` : r.email, value: r.user_id })))
const formatOptions = computed(() => props.formats.map((f) => ({ label: FORMAT_INFO[f].label, value: f })))
const problems = computed(() => reportMailProblems(form.value, addressesAllowed.value))
const names = computed(() => new Map(recipients.value.map((r) => [r.user_id, r.email])))

async function load() {
  loadError.value = ''
  try {
    const res = await api.get<{ items: ReportRecipient[]; addresses_allowed: boolean }>(`/projects/${proj.pid}/reports/recipients`)
    recipients.value = asItems(res)
    addressesAllowed.value = !!res?.addresses_allowed
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
watch(visible, (v) => {
  if (!v) return
  form.value = { recipients: [], addresses: '', message: '', format: props.format }
  error.value = ''
  showProblems.value = false
  void load()
})

async function send() {
  showProblems.value = true
  if (problems.value.length || !props.source) return
  busy.value = true
  error.value = ''
  try {
    const res = await api.post<{ queued: number; skipped: { user_id: string; reason: string }[] }>(`/projects/${proj.pid}/reports/email`, reportMailBody(props.source, form.value, addressesAllowed.value))
    const skipped = res?.skipped ?? []
    const detail = skipped.length ? t('email.send.skipped', { list: skipped.map((s) => `${names.value.get(s.user_id) ?? s.user_id} (${t(`email.send.skippedReason.${s.reason}`)})`).join(', ') }) : t('email.send.later')
    notify.ok(t('email.send.queued', { n: res?.queued ?? 0 }, res?.queued ?? 0), detail)
    visible.value = false
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('email.send.title')" :style="{ width: 'min(520px, calc(100vw - 16px))' }" class="report-mail-dialog">
    <form class="grid" style="gap: 12px" @submit.prevent="send">
      <p class="small muted" style="margin: 0">{{ t('email.send.report') }}: <b>{{ name }}</b></p>
      <Message v-if="loadError" severity="warn" :closable="false">{{ loadError }}</Message>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div v-field class="field">
        <label for="rm-to">{{ t('email.send.recipients') }}</label>
        <MultiSelect v-model="form.recipients" input-id="rm-to" :options="options" option-label="label" option-value="value" filter display="chip" :placeholder="t('email.send.recipientsPlaceholder')" :invalid="showProblems && problems.includes('recipients')" :max-selected-labels="6" class="w-full" />
        <small class="muted">{{ t('email.send.recipientsHint') }}</small>
      </div>
      <div v-if="addressesAllowed" v-field class="field">
        <label for="rm-addr">{{ t('email.send.addresses') }}</label>
        <Textarea id="rm-addr" v-model="form.addresses" rows="2" auto-resize :invalid="showProblems && problems.includes('addresses')" aria-describedby="rm-addr-hint" />
        <small id="rm-addr-hint" class="muted">{{ t('email.send.addressesHint') }}</small>
      </div>
      <small v-else class="muted">{{ t('email.send.addressesNotAllowed') }}</small>
      <div v-field class="field">
        <label for="rm-msg">{{ t('email.send.message') }}</label>
        <Textarea id="rm-msg" v-model="form.message" rows="3" auto-resize :maxlength="MAX_MESSAGE" />
      </div>
      <div v-field class="field" style="max-width: 200px">
        <label for="rm-format">{{ t('email.send.format') }}</label>
        <Select v-model="form.format" input-id="rm-format" :options="formatOptions" option-label="label" option-value="value" />
      </div>
      <Message v-if="showProblems && problems.length" severity="error" :closable="false"><span role="alert">{{ problems.map((p) => t(`email.send.problem.${p}`)).join(' ') }}</span></Message>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
        <Button type="submit" :label="t('email.send.send')" icon="pi pi-send" :loading="busy" />
      </div>
    </form>
  </Dialog>
</template>
