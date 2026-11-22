<script setup lang="ts">
// One test environment: what it is, its components and versions, status
// and health check, credentials (reference and write-only secret), its
// booking calendar and bookings, the results recorded in it and its history.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import EnvironmentDialog from '@/components/EnvironmentDialog.vue'
import BookingDialog from '@/components/BookingDialog.vue'
import BookingTimeline from '@/components/BookingTimeline.vue'
import HistoryPanel from '@/components/HistoryPanel.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import { uid } from '@/lib/a11y'
import { bookable, bookingSeverity, environmentIcon, environmentSeverity, startOfWeek, type Booking, type Environment } from '@/lib/environments'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useProjectPeopleStore } from '@/stores/projectPeople'
import { useConfigurations } from '@/composables/configurations'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ environmentId: string }>()
const proj = useProjectStore()
const auth = useAuthStore()
const people = useProjectPeopleStore()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const pid = computed(() => proj.pid)
const cfg = useConfigurations(() => proj.pid)
const tabsId = uid('envd-tabs')
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })
const tab = ref<'bookings' | 'history'>('bookings')

const env = ref<Environment | null>(null)
const error = ref('')
const bookings = ref<Booking[]>([])
const weekStart = ref(startOfWeek(new Date()))
async function load() {
  error.value = ''
  try {
    env.value = await api.get<Environment>(`/environments/${props.environmentId}`)
    bookings.value = asItems(await api.get<{ items: Booking[] }>(`/environments/${props.environmentId}/bookings`))
  } catch (e) {
    error.value = errorMessage(e)
  }
}
const canShare = computed(() => !!auth.me?.user?.super_admin || (auth.me?.permissions ?? []).includes(P.environmentManage))
const canManage = computed(() => !!env.value && (env.value.shared ? canShare.value : auth.can(P.environmentManage)))
const canDecide = (e: Environment) => (e.owner_id !== '' && e.owner_id === auth.user?.id) || (e.shared ? canShare.value : auth.can(P.environmentManage))
const canBook = computed(() => auth.can(P.runExecute) && !!env.value && bookable(env.value))
const ownerName = computed(() => {
  const id = env.value?.owner_id
  if (!id) return ''
  const p = people.people.find((x) => x.user_id === id)
  return p ? p.name || p.email || id : id
})
const cfgNames = computed(() => (env.value?.configuration_ids ?? []).map((id) => cfg.names.value.get(id) ?? id))
const now = Date.now()
const upcoming = computed(() => bookings.value.filter((b) => Date.parse(b.ends_at) >= now).sort((a, b) => a.starts_at.localeCompare(b.starts_at)))
const past = computed(() => bookings.value.filter((b) => Date.parse(b.ends_at) < now).sort((a, b) => b.starts_at.localeCompare(a.starts_at)).slice(0, 20))

function shiftWeek(n: number) {
  const d = new Date(weekStart.value)
  d.setDate(d.getDate() + 7 * n)
  weekStart.value = n === 0 ? startOfWeek(new Date()) : d
}

// ---------- actions ----------
const editOpen = ref(false)
const bookOpen = ref(false)
const openBooking = ref<Booking | null>(null)
const bookStart = ref<string | null>(null)
function book(day: string | null = null) {
  openBooking.value = null
  bookStart.value = day
  bookOpen.value = true
}
function show(b: Booking) {
  openBooking.value = b
  bookOpen.value = true
}
const checking = ref(false)
async function check() {
  if (!env.value) return
  checking.value = true
  try {
    env.value = await api.post<Environment>(`/environments/${env.value.id}/check`, {})
    notify.ok(t('environments.checked'), t(`environments.healthStatus.${env.value.health_status || 'up'}`))
  } catch (e) {
    notify.fail(e)
  } finally {
    checking.value = false
  }
}
const secretOpen = ref(false)
const secret = ref('')
async function reveal() {
  if (!env.value) return
  try {
    secret.value = (await api.post<{ secret: string }>(`/environments/${env.value.id}/secret`, {})).secret
    secretOpen.value = true
  } catch (e) {
    notify.fail(e)
  }
}
watch(secretOpen, (v) => {
  if (!v) secret.value = ''
})
function remove() {
  const e = env.value
  if (!e) return
  notify.confirmDanger(t('environments.confirmDelete', { name: e.name }), async () => {
    try {
      await api.del(`/environments/${e.id}`)
      notify.ok(t('environments.deleted'), e.name)
      router.push({ name: 'environments', params: { projectId: pid.value } })
    } catch (err) {
      notify.fail(err)
    }
  })
}
const statusLabel = (s: string) => t(`environments.status.${s}`)

onMounted(() => {
  void people.ensure(pid.value)
  void cfg.load()
  void load()
})
watch(() => props.environmentId, () => void load())
</script>

<template>
  <nav class="row small" style="margin-bottom: 6px" :aria-label="t('nav.breadcrumb')">
    <RouterLink :to="{ name: 'environments', params: { projectId: pid } }">{{ t('environments.title') }}</RouterLink>
    <i class="pi pi-angle-right muted" aria-hidden="true" />
    <span aria-current="page">{{ env?.name }}</span>
  </nav>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <template v-else-if="env">
    <div class="page-header">
      <div>
        <h1 class="row" style="gap: 8px"><i :class="environmentIcon(env.type)" aria-hidden="true" />{{ env.name }} <Tag :value="statusLabel(env.status)" :severity="environmentSeverity(env.status)" /></h1>
        <div class="sub">
          {{ t(`environments.type.${env.type}`) }}<template v-if="env.shared"> · {{ t('environments.sharedLong') }}</template> · {{ t('environments.capacityShort', { n: env.capacity }) }}<template v-if="env.requires_approval"> · {{ t('environments.approvalShort') }}</template>
        </div>
      </div>
      <div class="actions">
        <HelpLink page="test-environments" />
        <Button v-if="canBook" :label="t('environments.book')" icon="pi pi-calendar-plus" aria-haspopup="dialog" @click="book()" />
        <Button v-if="canManage && env.health_url" :label="t('environments.checkNow')" icon="pi pi-heart" severity="secondary" outlined :loading="checking" @click="check" />
        <Button v-if="canManage" :label="t('common.edit')" icon="pi pi-pencil" severity="secondary" outlined aria-haspopup="dialog" @click="editOpen = true" />
        <Button v-if="canManage" icon="pi pi-trash" severity="danger" text rounded :aria-label="t('environments.deleteNamed', { name: env.name })" v-tooltip="t('common.delete')" @click="remove" />
      </div>
    </div>
    <Message v-if="env.status_note" :severity="env.status === 'broken' ? 'error' : env.status === 'maintenance' ? 'warn' : 'info'" :closable="false">{{ env.status_note }}</Message>

    <div class="env-grid">
      <section class="card" aria-labelledby="env-ov-h">
        <h2 id="env-ov-h" class="card-title">{{ t('environments.overview') }}</h2>
        <dl class="env-dl">
          <dt>{{ t('environments.owner') }}</dt>
          <dd>{{ ownerName || '—' }}</dd>
          <dt>{{ t('environments.urls') }}</dt>
          <dd>
            <template v-if="env.urls.length"><a v-for="u in env.urls" :key="u" :href="u" target="_blank" rel="noopener noreferrer" class="env-url">{{ u }}</a></template>
            <template v-else>—</template>
          </dd>
          <dt>{{ t('common.tags') }}</dt>
          <dd>{{ env.tags.join(', ') || '—' }}</dd>
          <template v-if="!env.shared">
            <dt>{{ t('environments.configurations') }}</dt>
            <dd>{{ cfgNames.join(', ') || '—' }}</dd>
          </template>
          <dt>{{ t('environments.runs') }}</dt>
          <dd>{{ env.run_count }}</dd>
          <dt>{{ t('common.description') }}</dt>
          <dd style="white-space: pre-wrap">{{ env.description || '—' }}</dd>
        </dl>
      </section>
      <section class="card" aria-labelledby="env-comp-h">
        <h2 id="env-comp-h" class="card-title">{{ t('environments.components') }}</h2>
        <table v-if="env.components.length" class="env-table">
          <thead>
            <tr>
              <th scope="col">{{ t('environments.component') }}</th>
              <th scope="col">{{ t('environments.version') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in env.components" :key="c.name">
              <td>{{ c.name }}</td>
              <td class="mono">{{ c.version }}</td>
            </tr>
          </tbody>
        </table>
        <p v-else class="muted small">{{ t('environments.noComponents') }}</p>
        <p class="small muted">{{ t('environments.componentsSnapshot') }}</p>
      </section>
      <section class="card" aria-labelledby="env-st-h">
        <h2 id="env-st-h" class="card-title">{{ t('environments.statusAndHealth') }}</h2>
        <dl class="env-dl">
          <dt>{{ t('common.status') }}</dt>
          <dd>
            {{ statusLabel(env.status) }}<template v-if="env.status_by_health"> ({{ t('environments.setByHealth') }})</template>
          </dd>
          <dt>{{ t('environments.bookedNowCol') }}</dt>
          <dd>{{ t('environments.bookedNow', { n: env.booked_now, cap: env.capacity }) }}</dd>
          <dt>{{ t('environments.healthUrl') }}</dt>
          <dd class="mono small">{{ env.health_url || t('environments.noHealthCheck') }}</dd>
          <template v-if="env.health_url">
            <dt>{{ t('environments.lastCheck') }}</dt>
            <dd>
              {{ env.health_status ? t(`environments.healthStatus.${env.health_status}`) : t('environments.notChecked') }}<template v-if="env.health_checked_at"> · {{ formatDateTime(env.health_checked_at) }}</template>
              <div v-if="env.health_error" class="small">{{ env.health_error }}</div>
            </dd>
          </template>
        </dl>
      </section>
      <section class="card" aria-labelledby="env-cr-h">
        <h2 id="env-cr-h" class="card-title">{{ t('environments.credentials') }}</h2>
        <dl class="env-dl">
          <dt>{{ t('environments.credentialsRef') }}</dt>
          <dd>{{ env.credentials_ref || '—' }}</dd>
          <dt>{{ t('environments.secret') }}</dt>
          <dd>
            {{ env.has_secret ? t('environments.secretStored') : t('environments.noSecret') }}
            <Button v-if="env.has_secret && canManage" :label="t('environments.reveal')" icon="pi pi-eye" text size="small" aria-haspopup="dialog" @click="reveal" />
          </dd>
        </dl>
        <p class="small muted">{{ t('environments.secretNote') }}</p>
      </section>
    </div>

    <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs" style="margin-top: 12px">
      <TabList :aria-label="t('environments.sections')">
        <Tab value="bookings"><i class="pi pi-calendar" aria-hidden="true" /> {{ t('environments.bookings') }}</Tab>
        <Tab value="history"><i class="pi pi-history" aria-hidden="true" /> {{ t('common.history') }}</Tab>
      </TabList>
    </Tabs>
    <section v-if="tab === 'bookings'" v-bind="panel('bookings')" class="card" style="margin-top: 8px">
      <h2 class="sr-only">{{ t('environments.bookings') }}</h2>
      <div class="row" style="gap: 6px; margin-bottom: 8px">
        <Button icon="pi pi-chevron-left" text rounded :aria-label="t('environments.prevWeek')" @click="shiftWeek(-1)" />
        <Button :label="t('environments.thisWeek')" text size="small" @click="shiftWeek(0)" />
        <Button icon="pi pi-chevron-right" text rounded :aria-label="t('environments.nextWeek')" @click="shiftWeek(1)" />
      </div>
      <BookingTimeline :environments="[env]" :bookings="bookings" :week-start="weekStart" :can-book="canBook" @open="show" @book="(_id, day) => book(day)" />
      <h3>{{ t('environments.upcoming') }}</h3>
      <p v-if="!upcoming.length" class="muted small">{{ t('environments.noBookings') }}</p>
      <ul class="plain-list">
        <li v-for="b in upcoming" :key="b.id" class="row" style="gap: 8px">
          <Button text size="small" :label="`${formatDateTime(b.starts_at)} – ${formatDateTime(b.ends_at)}`" aria-haspopup="dialog" @click="show(b)" />
          <Tag :value="t(`environments.bookingStatus.${b.status}`)" :severity="bookingSeverity(b.status)" />
          <span class="small">{{ b.hidden ? t('environments.otherProject') : b.booked_by }}</span>
          <span v-if="!b.hidden && b.purpose" class="small muted">{{ b.purpose }}</span>
          <span v-if="b.project_key && env.shared" class="key">{{ b.project_key }}</span>
        </li>
      </ul>
      <template v-if="past.length">
        <h3>{{ t('environments.past') }}</h3>
        <ul class="plain-list">
          <li v-for="b in past" :key="b.id" class="row small" style="gap: 8px">
            <span>{{ formatDateTime(b.starts_at) }} – {{ formatDateTime(b.ends_at) }}</span>
            <Tag :value="t(`environments.bookingStatus.${b.status}`)" :severity="bookingSeverity(b.status)" />
            <span class="muted">{{ b.hidden ? t('environments.otherProject') : b.booked_by }}</span>
          </li>
        </ul>
      </template>
    </section>
    <section v-else v-bind="panel('history')" class="card" style="margin-top: 8px">
      <h2 class="sr-only">{{ t('common.history') }}</h2>
      <HistoryPanel entity="environment" :entity-id="env.id" :can-restore="false" />
    </section>

    <EnvironmentDialog v-model:visible="editOpen" :project-id="pid" :environment="env" @saved="(e) => { env = e; notify.ok(t('environments.saved'), e.name) }" />
    <BookingDialog v-model:visible="bookOpen" :project-id="pid" :environments="[env]" :booking="openBooking" :environment-id="env.id" :start="bookStart" :can-decide="canDecide" @changed="load" />
    <Dialog v-model:visible="secretOpen" modal :header="t('environments.secretHeader', { name: env.name })" style="width: min(460px, 96vw)">
      <p class="small">{{ t('environments.secretAudited') }}</p>
      <code class="mono secret-box">{{ secret }}</code>
      <template #footer><Button :label="t('common.close')" @click="secretOpen = false" /></template>
    </Dialog>
  </template>
</template>

<style scoped>
.env-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 12px;
}
.env-grid .card {
  margin: 0;
}
.card-title {
  font-size: 1rem;
  margin: 0 0 8px;
}
.env-dl {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 12px;
  margin: 0;
}
.env-dl dt {
  font-weight: 600;
}
.env-dl dd {
  margin: 0;
  min-width: 0;
  overflow-wrap: anywhere;
}
.env-url {
  display: block;
}
.env-table {
  width: 100%;
  border-collapse: collapse;
}
.env-table th,
.env-table td {
  text-align: left;
  padding: 4px 6px;
  border-bottom: 1px solid var(--qtf-border);
}
.plain-list {
  list-style: none;
  padding: 0;
  margin: 0;
}
.secret-box {
  display: block;
  padding: 8px;
  border: 1px solid var(--qtf-border);
  border-radius: 6px;
  user-select: all;
  overflow-wrap: anywhere;
}
</style>
