<script setup lang="ts">
// The environment picker of the run, cycle and exploratory session dialogs:
// an environment item of the project (or a shared one) and, optionally, a
// booking of it for the item (from, to, purpose) with a capacity check.
import { computed, ref, watch } from 'vue'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import { api, asItems } from '@/lib/api'
import { bookable, defaultBookingWindow, environmentIcon, environmentSeverity, fits, fromLocalInput, validWindow, type Booking, type BookingInput, type Environment } from '@/lib/environments'

const props = defineProps<{
  environments: Environment[]
  /** unique prefix of the input ids */
  idPrefix: string
  /** offer the booking (when the caller may book) */
  canBook?: boolean
}>()
const envId = defineModel<string | null>({ default: null })
const booking = defineModel<BookingInput | null>('booking', { default: null })
const { t } = useI18n()

const options = computed(() => props.environments.filter((e) => bookable(e) || e.id === envId.value).map((e) => ({ label: e.name, value: e.id, env: e })))
const selected = computed(() => props.environments.find((e) => e.id === envId.value) ?? null)
const book = ref(false)
const win = ref(defaultBookingWindow(new Date()))
const purpose = ref('')
watch(
  [book, win, purpose, envId],
  () => {
    booking.value = book.value && envId.value ? { starts_at: fromLocalInput(win.value.from), ends_at: fromLocalInput(win.value.to), purpose: purpose.value } : null
  },
  { deep: true },
)
// the bookings of the selected environment around the window (capacity hint)
const existing = ref<Booking[]>([])
watch(
  () => [envId.value, book.value, win.value.from, win.value.to] as const,
  async ([id, on, from, to]) => {
    existing.value = []
    if (!id || !on || !validWindow(from, to)) return
    try {
      existing.value = asItems(await api.get<{ items: Booking[] }>(`/environments/${id}/bookings`, { from: fromLocalInput(from), to: fromLocalInput(to), status: 'pending,approved' }))
    } catch {
      existing.value = []
    }
  },
)
const windowOk = computed(() => validWindow(win.value.from, win.value.to))
const full = computed(() => !!selected.value && windowOk.value && !fits(selected.value.capacity, existing.value, fromLocalInput(win.value.from), fromLocalInput(win.value.to)))
const statusLabel = (s: string) => t(`environments.status.${s}`)
</script>

<template>
  <div class="env-field">
    <div class="field">
      <label :for="`${idPrefix}-env`">{{ t('environments.field') }}</label>
      <Select
        v-model="envId"
        :input-id="`${idPrefix}-env`"
        :options="options"
        option-label="label"
        option-value="value"
        filter
        show-clear
        :placeholder="t('environments.noneSelected')"
        :aria-describedby="`${idPrefix}-env-help`"
      >
        <template #option="{ option }">
          <span class="row nowrap" style="gap: 6px">
            <i :class="environmentIcon(option.env.type)" aria-hidden="true" />
            <span>{{ option.label }}</span>
            <Tag :value="statusLabel(option.env.status)" :severity="environmentSeverity(option.env.status)" />
            <span v-if="option.env.shared" class="small muted">{{ t('environments.sharedShort') }}</span>
          </span>
        </template>
      </Select>
      <small :id="`${idPrefix}-env-help`" class="muted">{{ t('environments.fieldHelp') }}</small>
    </div>
    <Message v-if="selected && (selected.status === 'broken' || selected.status === 'maintenance')" severity="warn" :closable="false">
      {{ t('environments.notReady', { name: selected.name, status: statusLabel(selected.status) }) }}<template v-if="selected.status_note"> · {{ selected.status_note }}</template>
    </Message>
    <template v-if="selected && canBook">
      <div class="row" style="gap: 8px">
        <Checkbox v-model="book" binary :input-id="`${idPrefix}-book`" />
        <label :for="`${idPrefix}-book`">{{ t('environments.bookIt') }}</label>
        <span v-if="selected.requires_approval" class="small muted">{{ t('environments.needsApproval') }}</span>
      </div>
      <div v-if="book" class="form-grid">
        <div class="field">
          <label :for="`${idPrefix}-from`">{{ t('environments.from') }}</label>
          <InputText :id="`${idPrefix}-from`" v-model="win.from" type="datetime-local" />
        </div>
        <div class="field">
          <label :for="`${idPrefix}-to`">{{ t('environments.to') }}</label>
          <InputText :id="`${idPrefix}-to`" v-model="win.to" type="datetime-local" :invalid="!windowOk" />
        </div>
        <div class="field full">
          <label :for="`${idPrefix}-purpose`">{{ t('environments.purpose') }}</label>
          <InputText :id="`${idPrefix}-purpose`" v-model="purpose" maxlength="2000" />
        </div>
      </div>
      <Message v-if="book && !windowOk" severity="error" :closable="false">{{ t('environments.badWindow') }}</Message>
      <Message v-else-if="book && full" severity="warn" :closable="false">{{ t('environments.fullHint', { n: selected.capacity }) }}</Message>
    </template>
  </div>
</template>

<style scoped>
.env-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
</style>
