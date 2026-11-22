<script setup lang="ts">
// A week of bookings: one row per environment, a bar per booking (pending
// bookings striped). A bar opens its booking; an empty day of a row books
// the environment from that day. The same content is also given as a
// list for screen readers.
import { computed } from 'vue'
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/lib/format'
import { bookable, environmentIcon, timelineBars, timelineDays, type Booking, type Environment } from '@/lib/environments'

const props = defineProps<{ environments: Environment[]; bookings: Booking[]; weekStart: Date; canBook?: boolean }>()
const emit = defineEmits<{ open: [b: Booking]; book: [envId: string, day: string] }>()
const { t, locale } = useI18n()

const days = computed(() => timelineDays(props.weekStart))
const end = computed(() => {
  const d = new Date(props.weekStart)
  d.setDate(d.getDate() + 7)
  return d
})
const rows = computed(() =>
  props.environments.map((e) => {
    const { bars, lanes } = timelineBars(
      props.bookings.filter((b) => b.environment_id === e.id && (b.status === 'pending' || b.status === 'approved')),
      props.weekStart,
      end.value,
    )
    return { env: e, bars, lanes }
  }),
)
const dayLabel = (d: Date) => d.toLocaleDateString(locale.value, { weekday: 'short', day: 'numeric', month: 'numeric' })
const today = new Date().toDateString()
const barLabel = (b: Booking) =>
  t('environments.barLabel', {
    who: b.hidden ? t('environments.otherProject') : b.booked_by,
    from: formatDateTime(b.starts_at),
    to: formatDateTime(b.ends_at),
    status: t(`environments.bookingStatus.${b.status}`),
  })
</script>

<template>
  <div class="tl" role="group" :aria-label="t('environments.calendarLabel', { from: dayLabel(days[0]), to: dayLabel(days[6]) })">
    <div class="tl-head" aria-hidden="true">
      <div class="tl-name" />
      <div class="tl-days">
        <div v-for="d in days" :key="d.toISOString()" class="tl-day" :class="{ today: d.toDateString() === today }">{{ dayLabel(d) }}</div>
      </div>
    </div>
    <div v-for="r in rows" :key="r.env.id" class="tl-row">
      <div class="tl-name">
        <i :class="environmentIcon(r.env.type)" aria-hidden="true" />
        <span class="ellipsis">{{ r.env.name }}</span>
        <span class="small muted">×{{ r.env.capacity }}</span>
      </div>
      <div class="tl-track" :style="{ height: `${r.lanes * 26 + 6}px` }">
        <div class="tl-grid" aria-hidden="true">
          <button
            v-for="d in days"
            :key="d.toISOString()"
            type="button"
            class="tl-cell"
            :class="{ today: d.toDateString() === today }"
            :disabled="!canBook || !bookable(r.env)"
            tabindex="-1"
            @click="emit('book', r.env.id, d.toISOString())"
          />
        </div>
        <button
          v-for="b in r.bars"
          :key="b.booking.id"
          type="button"
          class="tl-bar"
          :class="[`st-${b.booking.status}`]"
          :style="{ left: `${b.left}%`, width: `${b.width}%`, top: `${b.lane * 26 + 3}px` }"
          :aria-label="barLabel(b.booking)"
          :title="barLabel(b.booking)"
          @click="emit('open', b.booking)"
        >
          <span class="ellipsis">{{ b.booking.hidden ? t('environments.otherProject') : b.booking.purpose || b.booking.booked_by }}</span>
        </button>
      </div>
      <Button v-if="canBook && bookable(r.env)" icon="pi pi-calendar-plus" text rounded size="small" :aria-label="t('environments.bookNamed', { name: r.env.name })" v-tooltip="t('environments.book')" @click="emit('book', r.env.id, days[0].toISOString())" />
    </div>
    <div v-if="!rows.length" class="muted small" style="padding: 8px">{{ t('environments.none') }}</div>
  </div>
</template>

<style scoped>
.tl {
  display: flex;
  flex-direction: column;
  gap: 2px;
  overflow-x: auto;
}
.tl-head,
.tl-row {
  display: grid;
  grid-template-columns: 180px minmax(560px, 1fr) 36px;
  align-items: center;
  gap: 6px;
}
.tl-head {
  grid-template-columns: 180px minmax(560px, 1fr) 36px;
}
.tl-days,
.tl-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
}
.tl-day {
  font-size: 0.8rem;
  color: var(--qtf-muted);
  padding: 2px 4px;
}
.tl-day.today {
  font-weight: 700;
  color: var(--qtf-text);
}
.tl-name {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.tl-track {
  position: relative;
  border: 1px solid var(--qtf-border);
  border-radius: 6px;
  min-height: 32px;
}
.tl-grid {
  position: absolute;
  inset: 0;
}
.tl-cell {
  border: 0;
  border-right: 1px dashed var(--qtf-border);
  background: transparent;
  cursor: pointer;
}
.tl-cell:disabled {
  cursor: default;
}
.tl-cell.today {
  background: color-mix(in srgb, var(--p-primary-color) 8%, transparent);
}
.tl-bar {
  position: absolute;
  height: 22px;
  border-radius: 4px;
  border: 1px solid var(--p-primary-color);
  background: color-mix(in srgb, var(--p-primary-color) 22%, var(--qtf-card));
  color: var(--qtf-text);
  font-size: 0.75rem;
  padding: 0 4px;
  text-align: left;
  overflow: hidden;
  cursor: pointer;
}
.tl-bar.st-pending {
  border-style: dashed;
  background: repeating-linear-gradient(45deg, transparent 0 6px, color-mix(in srgb, var(--p-primary-color) 15%, transparent) 6px 12px);
}
.tl-bar:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 1px;
}
</style>
