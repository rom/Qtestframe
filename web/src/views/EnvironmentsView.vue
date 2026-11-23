<script setup lang="ts">
// Test environments of the project (and those shared by the organisation):
// a status board, a filterable list and the booking calendar of a week.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import QueryInput from '@/components/QueryInput.vue'
import EnvironmentDialog from '@/components/EnvironmentDialog.vue'
import BookingDialog from '@/components/BookingDialog.vue'
import BookingTimeline from '@/components/BookingTimeline.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, ApiError, asItems } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import { uid } from '@/lib/a11y'
import { componentsText, environmentIcon, environmentSeverity, startOfWeek, type Booking, type Environment } from '@/lib/environments'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const pid = computed(() => proj.pid)
const tabsId = uid('env-tabs')
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })
type TabId = 'board' | 'list' | 'calendar'
const TABS: TabId[] = ['board', 'list', 'calendar']
const tab = ref<TabId>(TABS.includes(route.query.tab as TabId) ? (route.query.tab as TabId) : 'board')
watch(tab, (v) => {
  router.replace({ query: { ...route.query, tab: v === 'board' ? undefined : v } })
  if (v === 'calendar') void loadBookings()
})

const canManage = computed(() => auth.can(P.environmentManage))
// a shared environment needs environment.manage in the organisation
const canShare = computed(() => !!auth.me?.user?.super_admin || (auth.me?.permissions ?? []).includes(P.environmentManage))
const canBook = computed(() => auth.can(P.runExecute))
const canDecide = (e: Environment) => (e.owner_id !== '' && e.owner_id === auth.user?.id) || (e.shared ? canShare.value : canManage.value)

const all = ref<Environment[]>([])
const list = ref<Environment[]>([])
const loading = ref(false)
const q = ref((route.query.q as string) ?? '')
const queryError = ref<unknown>(null)
async function load() {
  loading.value = true
  queryError.value = null
  try {
    all.value = asItems(await api.get<{ items: Environment[] }>(`/projects/${pid.value}/environments`))
    list.value = q.value.trim() ? asItems(await api.get<{ items: Environment[] }>(`/projects/${pid.value}/environments`, { q: q.value })) : all.value
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
const subtitle = computed(() => {
  const avail = all.value.filter((e) => e.status === 'available').length
  return t('environments.subtitle', { n: all.value.length, available: avail }, all.value.length)
})
const statusLabel = (s: string) => t(`environments.status.${s}`)
const typeLabel = (s: string) => t(`environments.type.${s}`)
const healthLabel = (e: Environment) => (e.health_status ? t(`environments.healthStatus.${e.health_status}`) : '')
// the board: environments grouped by status (retired last)
const board = computed(() => [...all.value].sort((a, b) => Number(a.status === 'retired') - Number(b.status === 'retired') || a.name.localeCompare(b.name)))

// ---------- calendar ----------
const weekStart = ref(startOfWeek(new Date()))
const bookings = ref<Booking[]>([])
const weekEnd = computed(() => {
  const d = new Date(weekStart.value)
  d.setDate(d.getDate() + 7)
  return d
})
async function loadBookings() {
  try {
    bookings.value = asItems(await api.get<{ items: Booking[] }>(`/projects/${pid.value}/bookings`, { from: weekStart.value.toISOString(), to: weekEnd.value.toISOString() }))
  } catch (e) {
    notify.fail(e)
  }
}
function shiftWeek(n: number) {
  const d = new Date(weekStart.value)
  d.setDate(d.getDate() + 7 * n)
  weekStart.value = n === 0 ? startOfWeek(new Date()) : d
  void loadBookings()
}
const pending = computed(() => bookings.value.filter((b) => b.status === 'pending'))

// ---------- dialogs ----------
const envDialog = ref(false)
const bookDialog = ref(false)
const bookEnv = ref<string | null>(null)
const bookStart = ref<string | null>(null)
const openBooking = ref<Booking | null>(null)
function book(envId: string | null, day: string | null = null) {
  openBooking.value = null
  bookEnv.value = envId
  bookStart.value = day
  bookDialog.value = true
}
function show(b: Booking) {
  openBooking.value = b
  bookDialog.value = true
}
async function changed() {
  await Promise.all([load(), tab.value === 'calendar' ? loadBookings() : Promise.resolve()])
}
function created(e: Environment) {
  notify.ok(t('environments.created'), e.name)
  router.push({ name: 'environment', params: { projectId: pid.value, environmentId: e.id } })
}

onMounted(async () => {
  await load()
  if (tab.value === 'calendar') await loadBookings()
})
watch(pid, () => void load())
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('environments.title') }}</h1>
      <div class="sub">{{ subtitle }}</div>
    </div>
    <div class="actions">
      <HelpLink page="test-environments" />
      <Button :label="t('dataSets.title')" icon="pi pi-database" severity="secondary" text @click="router.push({ name: 'testData', params: { projectId: pid } })" />
      <Button v-if="canBook" :label="t('environments.book')" icon="pi pi-calendar-plus" severity="secondary" outlined aria-haspopup="dialog" :disabled="!all.length" @click="book(null)" />
      <Button v-if="canManage" :label="t('environments.new')" icon="pi pi-plus" aria-haspopup="dialog" @click="envDialog = true" />
    </div>
  </div>

  <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs">
    <TabList :aria-label="t('environments.sections')">
      <Tab value="board"><i class="pi pi-th-large" aria-hidden="true" /> {{ t('environments.tabBoard') }}</Tab>
      <Tab value="list"><i class="pi pi-list" aria-hidden="true" /> {{ t('environments.tabList') }}</Tab>
      <Tab value="calendar"><i class="pi pi-calendar" aria-hidden="true" /> {{ t('environments.tabCalendar') }}</Tab>
    </TabList>
  </Tabs>

  <section v-if="tab === 'board'" v-bind="panel('board')" style="margin-top: 12px">
    <h2 class="sr-only">{{ t('environments.tabBoard') }}</h2>
    <div v-if="!loading && !all.length" class="card empty-state">
      <p>{{ t('environments.empty') }}</p>
      <Button v-if="canManage" :label="t('environments.new')" icon="pi pi-plus" @click="envDialog = true" />
    </div>
    <ul v-else class="env-board" :aria-label="t('environments.tabBoard')">
      <li v-for="e in board" :key="e.id" class="card env-card" :class="{ retired: e.status === 'retired' }">
        <div class="row nowrap" style="gap: 8px">
          <i :class="environmentIcon(e.type)" aria-hidden="true" />
          <RouterLink :to="{ name: 'environment', params: { projectId: pid, environmentId: e.id } }" class="strong ellipsis">{{ e.name }}</RouterLink>
          <span class="grow" />
          <Tag :value="statusLabel(e.status)" :severity="environmentSeverity(e.status)" />
        </div>
        <div class="small muted">
          {{ typeLabel(e.type) }}<template v-if="e.shared"> · {{ t('environments.sharedShort') }}</template> · {{ t('environments.capacityShort', { n: e.capacity }) }}<template v-if="e.requires_approval"> · {{ t('environments.approvalShort') }}</template>
        </div>
        <div v-if="e.status_note" class="small">{{ e.status_note }}</div>
        <div v-if="e.components.length" class="small mono ellipsis" :title="componentsText(e.components)">{{ componentsText(e.components) }}</div>
        <div class="small">
          <span v-if="e.booked_now"><i class="pi pi-lock" aria-hidden="true" /> {{ t('environments.bookedNow', { n: e.booked_now, cap: e.capacity }) }}</span>
          <span v-else-if="e.next_booking" class="muted">{{ t('environments.nextBooking', { at: formatDateTime(e.next_booking) }) }}</span>
          <span v-else class="muted">{{ t('environments.noBookings') }}</span>
        </div>
        <div v-if="e.health_url" class="small">
          <i :class="e.health_status === 'down' ? 'pi pi-times-circle' : e.health_status === 'up' ? 'pi pi-check-circle' : 'pi pi-question-circle'" aria-hidden="true" />
          {{ t('environments.healthLine', { status: healthLabel(e) || t('environments.notChecked') }) }}<template v-if="e.health_checked_at"> · {{ formatDateTime(e.health_checked_at) }}</template>
        </div>
      </li>
    </ul>
  </section>

  <section v-else-if="tab === 'list'" v-bind="panel('list')" style="margin-top: 12px">
    <h2 class="sr-only">{{ t('environments.tabList') }}</h2>
    <div class="filterbar">
      <QueryInput v-model="q" scope="environment" :error="queryError" :placeholder="t('environments.filterPlaceholder', { example: 'status:available component:PostgreSQL' })" @submit="load" />
    </div>
    <DataTable :value="list" :loading="loading" data-key="id" size="small" class="card" striped-rows :aria-label="t('environments.tableLabel')">
      <template #empty>{{ t('environments.noMatch') }}</template>
      <Column field="name" :header="t('common.name')" sortable>
        <template #body="{ data }">
          <RouterLink :to="{ name: 'environment', params: { projectId: pid, environmentId: data.id } }" class="strong">{{ data.name }}</RouterLink>
          <span v-if="data.shared" class="small muted"> · {{ t('environments.sharedShort') }}</span>
        </template>
      </Column>
      <Column field="type" :header="t('environments.typeLabel')" sortable><template #body="{ data }">{{ typeLabel(data.type) }}</template></Column>
      <Column field="status" :header="t('common.status')" sortable><template #body="{ data }"><Tag :value="statusLabel(data.status)" :severity="environmentSeverity(data.status)" /></template></Column>
      <Column :header="t('environments.components')"><template #body="{ data }"><span class="small mono">{{ componentsText(data.components) }}</span></template></Column>
      <Column field="capacity" :header="t('environments.capacity')" sortable style="width: 110px" />
      <Column field="booked_now" :header="t('environments.bookedNowCol')" sortable style="width: 120px" />
      <Column field="run_count" :header="t('environments.runs')" sortable style="width: 90px" />
      <Column field="health_status" :header="t('environments.health')" sortable style="width: 110px"><template #body="{ data }">{{ healthLabel(data) }}</template></Column>
    </DataTable>
  </section>

  <section v-else v-bind="panel('calendar')" style="margin-top: 12px">
    <h2 class="sr-only">{{ t('environments.tabCalendar') }}</h2>
    <div class="card">
      <div class="row" style="gap: 6px; margin-bottom: 8px">
        <Button icon="pi pi-chevron-left" text rounded :aria-label="t('environments.prevWeek')" @click="shiftWeek(-1)" />
        <Button :label="t('environments.thisWeek')" text size="small" @click="shiftWeek(0)" />
        <Button icon="pi pi-chevron-right" text rounded :aria-label="t('environments.nextWeek')" @click="shiftWeek(1)" />
        <span class="small muted">{{ t('environments.calendarHelp') }}</span>
      </div>
      <BookingTimeline :environments="all" :bookings="bookings" :week-start="weekStart" :can-book="canBook" @open="show" @book="(id, day) => book(id, day)" />
    </div>
    <div class="card" style="margin-top: 12px">
      <h3 style="margin-top: 0">{{ t('environments.weekBookings', { n: bookings.length }) }}</h3>
      <p v-if="pending.length" class="small">{{ t('environments.pendingCount', { n: pending.length }, pending.length) }}</p>
      <ul class="plain-list">
        <li v-for="b in bookings" :key="b.id" class="row" style="gap: 8px">
          <Button text size="small" :label="`${b.environment_name} · ${formatDateTime(b.starts_at)} – ${formatDateTime(b.ends_at)}`" @click="show(b)" />
          <Tag :value="t(`environments.bookingStatus.${b.status}`)" :severity="b.status === 'pending' ? 'warn' : b.status === 'approved' ? 'success' : 'secondary'" />
          <span class="small muted">{{ b.hidden ? t('environments.otherProject') : `${b.booked_by}${b.purpose ? ': ' + b.purpose : ''}` }}</span>
        </li>
      </ul>
    </div>
  </section>

  <EnvironmentDialog v-model:visible="envDialog" :project-id="pid" :can-share="canShare" @saved="created" />
  <BookingDialog v-model:visible="bookDialog" :project-id="pid" :environments="all" :booking="openBooking" :environment-id="bookEnv" :start="bookStart" :can-decide="canDecide" @changed="changed" />
</template>

<style scoped>
.env-board {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
}
.env-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
}
.env-card.retired {
  opacity: 0.7;
}
.plain-list {
  list-style: none;
  padding: 0;
  margin: 0;
}
</style>
