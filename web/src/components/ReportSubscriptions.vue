<script setup lang="ts">
// Scheduled report subscriptions of a project: saved reports e-mailed
// daily, weekly or monthly (see internal/api/reportmail.go).
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import { api, asItems, errorMessage } from '@/lib/api'
import { FORMAT_INFO, REPORT_FORMATS } from '@/lib/reports'
import {
  FREQUENCIES,
  MAX_MESSAGE,
  WEEKDAYS,
  blankSubscription,
  subscriptionBody,
  subscriptionForm,
  subscriptionProblems,
  subscriptionStatus,
  timeOptions,
  weekdayName,
  type SubscriptionForm,
} from '@/lib/email'
import { browserTimeZone, timeZones } from '@/lib/prefs'
import { formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { ReportDefinition, ReportRecipient, ReportSubscription } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { currentLang } from '@/i18n'

const props = defineProps<{ definitions: ReportDefinition[]; canSave: boolean }>()
const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const { t, locale } = useI18n()

const subs = ref<ReportSubscription[]>([])
const loading = ref(false)
const loadError = ref('')
const recipients = ref<ReportRecipient[]>([])
const addressesAllowed = ref(false)
const base = computed(() => `/projects/${proj.pid}/reports`)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    subs.value = asItems(await api.get<{ items: ReportSubscription[] }>(`${base.value}/subscriptions`))
  } catch (e) {
    loadError.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
async function loadRecipients() {
  try {
    const res = await api.get<{ items: ReportRecipient[]; addresses_allowed: boolean }>(`${base.value}/recipients`)
    recipients.value = asItems(res)
    addressesAllowed.value = !!res?.addresses_allowed
  } catch {
    recipients.value = []
  }
}
onMounted(load)
// a deleted saved report takes its subscriptions along
watch(
  () => props.definitions.length,
  () => void load(),
)

const canEdit = (s: ReportSubscription) => props.canSave && (s.owner_id === auth.user?.id || auth.can(P.projectUpdate))
function schedule(s: Pick<ReportSubscription, 'frequency' | 'weekday' | 'day_of_month' | 'time' | 'timezone'>): string {
  const freq = (FREQUENCIES as readonly string[]).includes(s.frequency) ? s.frequency : 'weekly'
  const day = freq === 'weekly' ? weekdayName(s.weekday, locale.value) : String(s.day_of_month)
  return t(`email.subs.every.${freq}`, { time: `${s.time} (${s.timezone})`, day })
}
const recipientCount = (s: ReportSubscription) => s.recipients.length + s.addresses.length
const statusSeverity = { error: 'danger', sent: 'success', paused: 'secondary', waiting: 'info' } as const

// ---------- dialog ----------
const open = ref(false)
const editing = ref<ReportSubscription | null>(null)
const form = ref<SubscriptionForm>(blankSubscription())
const showProblems = ref(false)
const saving = ref(false)
const formError = ref('')
const defaultZone = () => auth.user?.preferences?.timezone || browserTimeZone() || 'UTC'
const problems = computed(() => subscriptionProblems(form.value, addressesAllowed.value))
const defOptions = computed(() => props.definitions.map((d) => ({ label: d.name, value: d.id })))
const freqOptions = computed(() => FREQUENCIES.map((f) => ({ label: t(`email.subs.frequency.${f}`), value: f })))
const dayOptions = computed(() => WEEKDAYS.map((d) => ({ label: weekdayName(d, locale.value), value: d })))
const times = computed(() => {
  const list = timeOptions()
  return list.includes(form.value.time) ? list : [...list, form.value.time].sort()
})
const zoneOptions = computed(() => timeZones(form.value.timezone).map((z) => ({ label: z.replace(/_/g, ' '), value: z })))
const formatOptions = REPORT_FORMATS.map((f) => ({ label: FORMAT_INFO[f].label, value: f }))
const recipientOptions = computed(() => recipients.value.map((r) => ({ label: r.display_name && r.display_name !== r.email ? `${r.display_name} · ${r.email}` : r.email, value: r.user_id })))

function openNew() {
  editing.value = null
  form.value = blankSubscription(props.definitions[0]?.id ?? '', defaultZone())
  if (auth.user?.id) form.value.recipients = [auth.user.id]
  openDialog()
}
function openEdit(s: ReportSubscription) {
  editing.value = s
  form.value = subscriptionForm(s)
  openDialog()
}
function openDialog() {
  showProblems.value = false
  formError.value = ''
  open.value = true
  void loadRecipients()
}
async function save() {
  showProblems.value = true
  if (problems.value.length) return
  saving.value = true
  formError.value = ''
  try {
    if (editing.value) await api.patch(`/reports/subscriptions/${editing.value.id}`, subscriptionBody(form.value, false, currentLang()))
    else await api.post(`${base.value}/subscriptions`, subscriptionBody(form.value, true, currentLang()))
    open.value = false
    notify.ok(t('email.subs.saved'))
    await load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
async function setEnabled(s: ReportSubscription, enabled: boolean) {
  try {
    await api.patch(`/reports/subscriptions/${s.id}`, { enabled })
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
const sending = ref('')
async function sendNow(s: ReportSubscription) {
  sending.value = s.id
  try {
    const res = await api.post<ReportSubscription>(`/reports/subscriptions/${s.id}/send`)
    if (res?.last_error) notify.failText(t('email.subs.sendFailed'), res.last_error)
    else notify.ok(t('email.subs.sentNow'), s.definition_name)
    await load()
  } catch (e) {
    notify.fail(e, t('email.subs.sendFailed'))
  } finally {
    sending.value = ''
  }
}
function remove(s: ReportSubscription) {
  notify.confirmDanger(t('email.subs.confirmDelete', { name: s.definition_name ?? '' }), async () => {
    try {
      await api.del(`/reports/subscriptions/${s.id}`)
      notify.ok(t('email.subs.deleted'), s.definition_name)
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
</script>

<template>
  <section class="card subscriptions" style="margin-top: 20px" aria-labelledby="h-subs">
    <div class="row" style="margin-bottom: 6px">
      <h2 id="h-subs" class="plain-heading">{{ t('email.subs.title') }}</h2>
      <span class="grow" />
      <Button v-if="canSave" :label="t('email.subs.add')" icon="pi pi-calendar-plus" size="small" :disabled="!definitions.length" @click="openNew" />
    </div>
    <p class="small muted" style="margin: 0 0 10px">{{ t('email.subs.intro') }}</p>
    <p v-if="canSave && !definitions.length" class="small muted" style="margin: 0 0 10px">{{ t('email.subs.needDefinition') }}</p>
    <Message v-if="loadError" severity="warn" :closable="false">{{ loadError }}</Message>
    <DataTable v-else v-stack :value="subs" data-key="id" size="small" :loading="loading" :aria-busy="loading" aria-labelledby="h-subs">
      <Column :header="t('email.subs.report')" body-class="stack-main">
        <template #body="{ data }">
          <b>{{ data.definition_name }}</b>
          <div class="small muted">{{ t('email.subs.owner', { owner: data.owner_email || data.owner_id }) }}</div>
        </template>
      </Column>
      <Column :header="t('email.subs.schedule')">
        <template #body="{ data }"><span class="small">{{ schedule(data) }}</span> <Tag :value="FORMAT_INFO[data.format as keyof typeof FORMAT_INFO]?.label ?? data.format" severity="secondary" /></template>
      </Column>
      <Column :header="t('email.subs.recipients')" style="width: 130px">
        <template #body="{ data }"><span class="small">{{ t('email.subs.recipientsCount', { n: recipientCount(data) }, recipientCount(data)) }}</span></template>
      </Column>
      <Column :header="t('email.subs.lastSent')" style="width: 190px">
        <template #body="{ data }">
          <Tag :value="t(`email.subs.status.${subscriptionStatus(data)}`)" :severity="statusSeverity[subscriptionStatus(data)]" />
          <div class="small muted">{{ data.last_sent_at ? formatDateTime(data.last_sent_at) : t('email.subs.never') }}</div>
          <div v-if="data.last_error" class="small error-text" style="overflow-wrap: anywhere">{{ t('email.subs.lastError') }}: {{ data.last_error }}</div>
        </template>
      </Column>
      <Column :header="t('email.subs.nextRun')" style="width: 150px">
        <template #body="{ data }"><span class="small">{{ data.next_run_at ? formatDateTime(data.next_run_at) : t('email.subs.paused') }}</span></template>
      </Column>
      <Column :header="t('email.subs.active')" style="width: 80px">
        <template #body="{ data }"><ToggleSwitch :model-value="data.enabled" :disabled="!canEdit(data)" :aria-label="t('email.subs.activeOf', { name: data.definition_name })" @update:model-value="(v: boolean) => setEnabled(data, v)" /></template>
      </Column>
      <Column style="width: 130px">
        <template #body="{ data }">
          <template v-if="canEdit(data)">
            <Button icon="pi pi-send" text rounded size="small" severity="secondary" :loading="sending === data.id" :aria-label="t('email.subs.sendNowOf', { name: data.definition_name })" v-tooltip="t('email.subs.sendNow')" @click="sendNow(data)" />
            <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('email.subs.editOf', { name: data.definition_name })" v-tooltip="t('email.subs.edit')" @click="openEdit(data)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('email.subs.deleteOf', { name: data.definition_name })" v-tooltip="t('common.delete')" @click="remove(data)" />
          </template>
        </template>
      </Column>
      <template #empty><div class="muted small">{{ t('email.subs.none') }}</div></template>
    </DataTable>

    <Dialog v-model:visible="open" modal :header="editing ? t('email.subs.edit') : t('email.subs.new')" :style="{ width: 'min(560px, calc(100vw - 16px))' }" class="subscription-dialog">
      <form class="grid" style="gap: 12px" @submit.prevent="save">
        <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
        <div v-field class="field">
          <label for="sb-def">{{ t('email.subs.report') }}</label>
          <Select v-model="form.definition_id" input-id="sb-def" :options="defOptions" option-label="label" option-value="value" :disabled="!!editing" :invalid="showProblems && problems.includes('definition')" />
        </div>
        <div class="form-grid" style="gap: 12px">
          <div v-field class="field">
            <label for="sb-freq">{{ t('email.subs.frequencyLabel') }}</label>
            <Select v-model="form.frequency" input-id="sb-freq" :options="freqOptions" option-label="label" option-value="value" />
          </div>
          <div v-if="form.frequency === 'weekly'" v-field class="field">
            <label for="sb-wd">{{ t('email.subs.weekday') }}</label>
            <Select v-model="form.weekday" input-id="sb-wd" :options="dayOptions" option-label="label" option-value="value" />
          </div>
          <div v-if="form.frequency === 'monthly'" v-field class="field">
            <label for="sb-day">{{ t('email.subs.day') }}</label>
            <InputNumber v-model="form.day_of_month" input-id="sb-day" :min="1" :max="28" show-buttons :invalid="showProblems && problems.includes('day')" />
          </div>
          <div v-field class="field">
            <label for="sb-time">{{ t('email.subs.time') }}</label>
            <Select v-model="form.time" input-id="sb-time" :options="times" editable :invalid="showProblems && problems.includes('time')" />
          </div>
          <div v-field class="field">
            <label for="sb-tz">{{ t('email.subs.timezone') }}</label>
            <Select v-model="form.timezone" input-id="sb-tz" :options="zoneOptions" option-label="label" option-value="value" filter :virtual-scroller-options="{ itemSize: 38 }" />
          </div>
          <div v-field class="field">
            <label for="sb-format">{{ t('email.send.format') }}</label>
            <Select v-model="form.format" input-id="sb-format" :options="formatOptions" option-label="label" option-value="value" />
          </div>
        </div>
        <p class="small muted" style="margin: 0" aria-live="polite">{{ schedule({ ...form, time: form.time }) }}</p>
        <div v-field class="field">
          <label for="sb-to">{{ t('email.send.recipients') }}</label>
          <MultiSelect v-model="form.recipients" input-id="sb-to" :options="recipientOptions" option-label="label" option-value="value" filter display="chip" :placeholder="t('email.send.recipientsPlaceholder')" :invalid="showProblems && problems.includes('recipients')" class="w-full" />
        </div>
        <div v-if="addressesAllowed || form.addresses" v-field class="field">
          <label for="sb-addr">{{ t('email.send.addresses') }}</label>
          <Textarea id="sb-addr" v-model="form.addresses" rows="2" auto-resize :invalid="showProblems && problems.includes('addresses')" aria-describedby="sb-addr-hint" />
          <small id="sb-addr-hint" class="muted">{{ addressesAllowed ? t('email.send.addressesHint') : t('email.send.addressesNotAllowed') }}</small>
        </div>
        <div v-field class="field">
          <label for="sb-msg">{{ t('email.send.message') }}</label>
          <Textarea id="sb-msg" v-model="form.message" rows="2" auto-resize :maxlength="MAX_MESSAGE" />
        </div>
        <label class="row nowrap small" for="sb-on"><ToggleSwitch v-model="form.enabled" input-id="sb-on" /> {{ t('email.subs.active') }}</label>
        <Message v-if="showProblems && problems.length" severity="error" :closable="false"><span role="alert">{{ problems.map((p) => t(`email.subs.problem.${p}`)).join(' ') }}</span></Message>
        <div class="row" style="justify-content: flex-end">
          <Button :label="t('common.cancel')" text severity="secondary" @click="open = false" />
          <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" />
        </div>
      </form>
    </Dialog>
  </section>
</template>
