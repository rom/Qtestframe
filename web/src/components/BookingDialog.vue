<script setup lang="ts">
// A booking of a test environment: book it (from, to, purpose), or look at
// a booking and move, cancel, approve or reject it. A booking that does not
// fit the capacity answers with the overlapping bookings.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import { api, ApiError, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { formatDateTime } from '@/lib/format'
import { bookable, bookingSeverity, defaultBookingWindow, fromLocalInput, toLocalInput, validWindow, type Booking, type Environment } from '@/lib/environments'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  projectId: string
  environments: Environment[]
  /** the booking to show (null: a new one) */
  booking?: Booking | null
  /** the environment of a new booking */
  environmentId?: string | null
  /** start of a new booking (ISO) */
  start?: string | null
  /** the caller may approve the environment's bookings */
  canDecide?: (e: Environment) => boolean
}>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const auth = useAuthStore()
const id = uid('bk')

const envId = ref<string | null>(null)
const from = ref('')
const to = ref('')
const purpose = ref('')
const note = ref('')
const error = ref('')
const overlapping = ref<Booking[]>([])
const busy = ref(false)
const isNew = computed(() => !props.booking)
const env = computed(() => props.environments.find((e) => e.id === (props.booking?.environment_id ?? envId.value)) ?? null)
const active = computed(() => !props.booking || props.booking.status === 'pending' || props.booking.status === 'approved')
const mine = computed(() => !!props.booking && props.booking.user_id === auth.user?.id)
const decider = computed(() => !!env.value && !!props.canDecide?.(env.value))
const editable = computed(() => isNew.value || (active.value && !props.booking?.hidden && (mine.value || decider.value)))
const envOptions = computed(() => props.environments.filter(bookable).map((e) => ({ label: e.name, value: e.id })))

watch(visible, (open) => {
  if (!open) return
  error.value = ''
  note.value = ''
  overlapping.value = []
  const b = props.booking
  if (b) {
    envId.value = b.environment_id
    from.value = toLocalInput(b.starts_at)
    to.value = toLocalInput(b.ends_at)
    purpose.value = b.purpose
  } else {
    envId.value = props.environmentId ?? props.environments.find(bookable)?.id ?? null
    // a day picked in the calendar: from 09:00 that day
    const base = props.start ? new Date(props.start) : new Date()
    if (props.start) base.setHours(8, 0, 0, 0)
    const w = defaultBookingWindow(base)
    from.value = w.from
    to.value = w.to
    purpose.value = ''
  }
})

async function run(fn: () => Promise<unknown>) {
  busy.value = true
  error.value = ''
  overlapping.value = []
  try {
    await fn()
    visible.value = false
    emit('changed')
  } catch (e) {
    error.value = errorMessage(e)
    if (e instanceof ApiError && e.code === 'booking_conflict') {
      const body = e.body as { overlapping?: Booking[] } | undefined
      overlapping.value = body?.overlapping ?? []
    }
  } finally {
    busy.value = false
  }
}
const save = () =>
  run(() => {
    const body = { starts_at: fromLocalInput(from.value), ends_at: fromLocalInput(to.value), purpose: purpose.value }
    if (props.booking) return api.patch(`/bookings/${props.booking.id}`, body)
    return api.post(`/environments/${envId.value}/bookings`, { ...body, project_id: props.projectId })
  })
const decide = (decision: 'approved' | 'rejected') => run(() => api.post(`/bookings/${props.booking!.id}/decision`, { decision, note: note.value }))
const cancel = () => run(() => api.post(`/bookings/${props.booking!.id}/cancel`, {}))
const statusLabel = (s: string) => t(`environments.bookingStatus.${s}`)
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="isNew ? t('environments.bookHeader') : t('environments.bookingHeader', { name: booking?.environment_name ?? env?.name ?? '' })" style="width: min(560px, 96vw)">
    <div class="grid" style="gap: 10px">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div v-if="overlapping.length" class="small">
        <div class="strong">{{ t('environments.overlapping') }}</div>
        <ul style="margin: 4px 0 0; padding-left: 18px">
          <li v-for="o in overlapping" :key="o.id">{{ formatDateTime(o.starts_at) }} – {{ formatDateTime(o.ends_at) }} · {{ o.hidden ? t('environments.otherProject') : o.booked_by }} ({{ statusLabel(o.status) }})</li>
        </ul>
      </div>
      <div v-if="booking" class="row" style="gap: 8px">
        <Tag :value="statusLabel(booking.status)" :severity="bookingSeverity(booking.status)" />
        <span class="small muted">{{ booking.hidden ? t('environments.otherProject') : t('environments.bookedBy', { who: booking.booked_by }) }}</span>
        <span v-if="booking.decided_by" class="small muted">· {{ t('environments.decidedBy', { who: booking.decided_by }) }}</span>
      </div>
      <div v-if="isNew" class="field">
        <label :for="`${id}-env`">{{ t('environments.field') }}</label>
        <Select v-model="envId" :input-id="`${id}-env`" :options="envOptions" option-label="label" option-value="value" filter />
      </div>
      <Message v-if="isNew && env?.requires_approval" severity="info" :closable="false">{{ t('environments.needsApprovalLong') }}</Message>
      <div class="form-grid">
        <div class="field">
          <label :for="`${id}-from`">{{ t('environments.from') }}</label>
          <InputText :id="`${id}-from`" v-model="from" type="datetime-local" :disabled="!editable" />
        </div>
        <div class="field">
          <label :for="`${id}-to`">{{ t('environments.to') }}</label>
          <InputText :id="`${id}-to`" v-model="to" type="datetime-local" :disabled="!editable" :invalid="!validWindow(from, to)" />
        </div>
        <div class="field full">
          <label :for="`${id}-purpose`">{{ t('environments.purpose') }}</label>
          <InputText :id="`${id}-purpose`" v-model="purpose" maxlength="2000" :disabled="!editable" />
        </div>
      </div>
      <div v-if="booking && !booking.hidden && (booking.run_name || booking.cycle_name || booking.session_key)" class="small">
        <span v-if="booking.run_name">{{ t('environments.linkedRun', { name: booking.run_name }) }}</span>
        <span v-if="booking.cycle_name">{{ t('environments.linkedCycle', { name: booking.cycle_name }) }}</span>
        <span v-if="booking.session_key">{{ t('environments.linkedSession', { key: booking.session_key }) }}</span>
      </div>
      <div v-if="booking?.decision_note" class="small muted">{{ booking.decision_note }}</div>
      <div v-if="booking?.status === 'pending' && decider" class="field">
        <label :for="`${id}-note`">{{ t('environments.decisionNote') }}</label>
        <InputText :id="`${id}-note`" v-model="note" maxlength="2000" />
      </div>
    </div>
    <template #footer>
      <Button v-if="booking && active && (mine || decider) && !booking.hidden" :label="t('environments.cancelBooking')" severity="danger" text :loading="busy" @click="cancel" />
      <span class="grow" />
      <template v-if="booking?.status === 'pending' && decider">
        <Button :label="t('environments.reject')" icon="pi pi-times" severity="danger" outlined :loading="busy" @click="decide('rejected')" />
        <Button :label="t('environments.approve')" icon="pi pi-check" severity="success" :loading="busy" @click="decide('approved')" />
      </template>
      <Button :label="t('common.close')" text severity="secondary" @click="visible = false" />
      <Button v-if="editable" :label="isNew ? t('environments.book') : t('common.save')" icon="pi pi-calendar-plus" :loading="busy" :disabled="!envId || !validWindow(from, to)" @click="save" />
    </template>
  </Dialog>
</template>
