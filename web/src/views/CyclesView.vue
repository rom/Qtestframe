<script setup lang="ts">
// Test cycles: the list (with progress), a timeline of their dates and the
// recurring schedules that create cycles from a template.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import Textarea from 'primevue/textarea'
import QueryInput from '@/components/QueryInput.vue'
import SummaryBar from '@/components/SummaryBar.vue'
import VariantSelect from '@/components/VariantSelect.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { cycleSeverity, minutesText, overdue, progressSummary, scheduledName, scheduleText, timeline, CYCLE_STATUSES } from '@/lib/cycles'
import { formatDate, formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import { uid } from '@/lib/a11y'
import type { CycleSchedule, Release, TestCycle } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useProjectPeopleStore } from '@/stores/projectPeople'
import { useVariants } from '@/composables/variants'
import { useConfigurations } from '@/composables/configurations'
import { useEnvironments } from '@/composables/environments'
import EnvironmentField from '@/components/EnvironmentField.vue'
import type { BookingInput } from '@/lib/environments'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const people = useProjectPeopleStore()
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const id = uid('cyc')
const tabsId = uid('cyc-tabs')
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })
const pid = computed(() => proj.pid)
const envs = useEnvironments(() => proj.pid)
const cycleEnv = ref<string | null>(null)
const cycleBooking = ref<BookingInput | null>(null)
const canPlan = computed(() => auth.can(P.cyclePlan))
const vs = useVariants(() => pid.value)
const cfg = useConfigurations(() => pid.value)
const units = computed(() => ({ h: t('cycles.unitH'), min: t('cycles.unitMin') }))
const mins = (m: number) => minutesText(m, units.value)
const statusLabel = (s: string) => t(`cycles.status.${s}`)
const today = new Date().toISOString().slice(0, 10)

type TabId = 'list' | 'timeline' | 'schedules'
const TABS: TabId[] = ['list', 'timeline', 'schedules']
const tab = ref<TabId>(TABS.includes(route.query.tab as TabId) ? (route.query.tab as TabId) : 'list')
watch(tab, (v) => {
  router.replace({ query: { ...route.query, tab: v === 'list' ? undefined : v } })
  if (v === 'schedules') void loadSchedules()
})

// ---------- people, releases ----------
const testers = computed(() => people.people.filter((p) => p.kind === 'user' && p.user_id))
const personName = (uidv: string) => {
  if (!uidv) return ''
  const p = testers.value.find((x) => x.user_id === uidv)
  return p ? p.name || p.email || uidv : uidv === auth.user?.id ? auth.user?.display_name || auth.user?.email || '' : t('cycles.unknownPerson')
}
const peopleOptions = computed(() => testers.value.map((p) => ({ label: p.name && p.email ? `${p.name} (${p.email})` : p.name || p.email || '', value: p.user_id! })))
const releases = ref<Release[]>([])
const releaseOptions = computed(() => releases.value.map((r) => ({ label: r.version && r.version !== r.name ? `${r.name} (${r.version})` : r.name, value: r.id })))

// ---------- list ----------
const cycles = ref<TestCycle[]>([])
const allCycles = ref<TestCycle[]>([])
const loading = ref(false)
const q = ref((route.query.q as string) ?? '')
const queryError = ref<unknown>(null)
async function load() {
  if (!pid.value) return
  loading.value = true
  queryError.value = null
  try {
    cycles.value = asItems(await api.get<{ items: TestCycle[] }>(`/projects/${pid.value}/cycles`, { q: q.value || undefined }))
    allCycles.value = q.value ? asItems(await api.get<{ items: TestCycle[] }>(`/projects/${pid.value}/cycles`)) : cycles.value
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
const subtitle = computed(() => {
  const active = allCycles.value.filter((c) => c.status === 'active').length
  return t('cycles.subtitle', { n: allCycles.value.length, active }, allCycles.value.length)
})
const tl = computed(() => timeline(cycles.value))

// ---------- create ----------
const dialog = ref(false)
const busy = ref(false)
const formError = ref('')
const blank = () => ({
  name: '',
  goal: '',
  description: '',
  owner_id: auth.user?.id ?? '',
  release_id: null as string | null,
  variant_id: null as string | null,
  environment: '',
  configuration_ids: [] as string[],
  start_date: '',
  end_date: '',
  scheduled_start: '',
  copy_from: null as string | null,
})
const form = reactive(blank())
const copyOptions = computed(() => allCycles.value.map((c) => ({ label: c.name, value: c.id })))
function openCreate() {
  Object.assign(form, blank())
  cycleEnv.value = null
  cycleBooking.value = null
  formError.value = ''
  dialog.value = true
}
async function create() {
  busy.value = true
  formError.value = ''
  const body: Record<string, unknown> = {
    name: form.name,
    goal: form.goal,
    description: form.description,
    owner_id: form.owner_id,
    environment: form.environment,
    start_date: form.start_date,
    end_date: form.end_date,
  }
  if (form.release_id) body.release_id = form.release_id
  if (form.variant_id) body.variant_id = form.variant_id
  if (form.copy_from) body.copy_from = form.copy_from
  if (cycleEnv.value) body.environment_id = cycleEnv.value
  if (cycleEnv.value && cycleBooking.value) body.booking = cycleBooking.value
  else body.configuration_ids = form.configuration_ids
  if (form.scheduled_start) body.scheduled_start_at = new Date(form.scheduled_start).toISOString()
  try {
    const c = await api.post<TestCycle>(`/projects/${pid.value}/cycles`, body)
    dialog.value = false
    notify.ok(t('cycles.created'), c.name)
    router.push({ name: 'cycle', params: { projectId: pid.value, cycleId: c.id } })
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

// ---------- schedules ----------
const schedules = ref<CycleSchedule[]>([])
const schedLoaded = ref(false)
async function loadSchedules() {
  try {
    schedules.value = asItems(await api.get<{ items: CycleSchedule[] }>(`/projects/${pid.value}/cycle-schedules`))
  } catch (e) {
    notify.fail(e)
  } finally {
    schedLoaded.value = true
  }
}
const weekday = (d: number) => t(`cycles.weekday.${d}`)
const describeSchedule = (s: CycleSchedule) => scheduleText(s, t, weekday)
const schedDialog = ref(false)
const schedEditing = ref<CycleSchedule | null>(null)
const schedError = ref('')
const blankSched = () => ({
  name_pattern: '',
  template_cycle_id: null as string | null,
  query: '',
  owner_id: auth.user?.id ?? '',
  frequency: 'weekly',
  weekday: 1,
  day_of_month: 1,
  time: '06:00',
  timezone: auth.user?.preferences?.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
  duration_days: 5,
  auto_start: false,
  enabled: true,
})
const sched = reactive(blankSched())
const frequencyOptions = computed(() => ['daily', 'weekly', 'monthly'].map((f) => ({ label: t(`cycles.frequency.${f}`), value: f })))
const weekdayOptions = computed(() => [1, 2, 3, 4, 5, 6, 0].map((d) => ({ label: weekday(d), value: d })))
const namePreview = computed(() => scheduledName(sched.name_pattern || t('cycles.schedule.patternPlaceholder'), today, (schedEditing.value?.runs ?? 0) + 1))
function openSchedule(s: CycleSchedule | null) {
  schedEditing.value = s
  Object.assign(sched, blankSched(), s ? { ...s, template_cycle_id: s.template_cycle_id } : {})
  schedError.value = ''
  schedDialog.value = true
}
async function saveSchedule() {
  busy.value = true
  schedError.value = ''
  const body = { ...sched, template_cycle_id: sched.template_cycle_id ?? '' }
  try {
    if (schedEditing.value) await api.patch(`/cycle-schedules/${schedEditing.value.id}`, body)
    else await api.post(`/projects/${pid.value}/cycle-schedules`, body)
    schedDialog.value = false
    notify.ok(t('cycles.schedule.saved'))
    await loadSchedules()
  } catch (e) {
    schedError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
async function runSchedule(s: CycleSchedule) {
  try {
    const c = await api.post<TestCycle>(`/cycle-schedules/${s.id}/run`)
    notify.ok(t('cycles.created'), c.name)
    await Promise.all([loadSchedules(), load()])
  } catch (e) {
    notify.fail(e)
  }
}
async function removeSchedule(s: CycleSchedule) {
  if (!window.confirm(t('cycles.schedule.confirmDelete', { name: s.name_pattern }))) return
  try {
    await api.del(`/cycle-schedules/${s.id}`)
    notify.ok(t('cycles.schedule.deleted'), s.name_pattern)
    await loadSchedules()
  } catch (e) {
    notify.fail(e)
  }
}

onMounted(async () => {
  void people.ensure(pid.value)
  await Promise.all([
    load(),
    vs.load(),
    cfg.load(),
    envs.load(),
    (async () => {
      try {
        releases.value = asItems(await api.get<{ items: Release[] }>(`/projects/${pid.value}/releases`))
      } catch {
        releases.value = []
      }
    })(),
  ])
  if (tab.value === 'schedules') await loadSchedules()
  if (route.query.create === '1' && canPlan.value) openCreate()
})
watch(pid, () => void load())
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('cycles.title') }}</h1>
      <div class="sub">{{ subtitle }}</div>
    </div>
    <div class="actions">
      <HelpLink page="test-cycles" />
      <Button v-if="canPlan" :label="t('cycles.new')" icon="pi pi-plus" aria-haspopup="dialog" @click="openCreate" />
    </div>
  </div>

  <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs">
    <TabList :aria-label="t('cycles.sections')">
      <Tab value="list"><i class="pi pi-list" aria-hidden="true" /> {{ t('cycles.tabList') }}</Tab>
      <Tab value="timeline"><i class="pi pi-calendar" aria-hidden="true" /> {{ t('cycles.tabTimeline') }}</Tab>
      <Tab value="schedules"><i class="pi pi-replay" aria-hidden="true" /> {{ t('cycles.tabSchedules') }}</Tab>
    </TabList>
  </Tabs>

  <section v-if="tab === 'list'" v-bind="panel('list')" style="margin-top: 12px">
    <h2 class="sr-only">{{ t('cycles.tabList') }}</h2>
    <div class="filterbar">
      <QueryInput v-model="q" scope="cycle" :error="queryError" :placeholder="t('cycles.filterPlaceholder', { example: 'status:active assignee:anna' })" @submit="load" />
    </div>
    <div v-if="!loading && !allCycles.length" class="card empty-state">
      <p>{{ t('cycles.empty') }}</p>
      <Button v-if="canPlan" :label="t('cycles.new')" icon="pi pi-plus" @click="openCreate" />
    </div>
    <DataTable v-else :value="cycles" :loading="loading" data-key="id" size="small" class="card" striped-rows :aria-label="t('cycles.tableLabel')">
      <template #empty>{{ t('cycles.noMatch') }}</template>
      <Column field="name" :header="t('common.name')" sortable>
        <template #body="{ data }">
          <RouterLink :to="{ name: 'cycle', params: { projectId: pid, cycleId: data.id } }" class="strong">{{ data.name }}</RouterLink>
          <div v-if="data.goal" class="small muted ellipsis">{{ data.goal }}</div>
        </template>
      </Column>
      <Column field="status" :header="t('common.status')" sortable style="width: 120px">
        <template #body="{ data }">
          <Tag :value="statusLabel(data.status)" :severity="cycleSeverity(data.status)" />
          <Tag v-if="overdue(data, today)" :value="t('cycles.overdue')" severity="warn" class="ml-4" />
        </template>
      </Column>
      <Column :header="t('cycles.owner')" style="width: 160px">
        <template #body="{ data }"><span class="small">{{ personName(data.owner_id) || '—' }}</span></template>
      </Column>
      <Column field="start_date" :header="t('cycles.dates')" sortable style="width: 230px">
        <template #body="{ data }">
          <span class="small">{{ data.start_date ? formatDate(data.start_date) : '…' }} – {{ data.end_date ? formatDate(data.end_date) : '…' }}</span>
          <div v-if="data.scheduled_start_at" class="small muted"><i class="pi pi-clock" aria-hidden="true" /> {{ t('cycles.startsAt', { when: formatDateTime(data.scheduled_start_at) }) }}</div>
        </template>
      </Column>
      <Column :header="t('cycles.release')" style="width: 140px">
        <template #body="{ data }"><span class="small">{{ [data.release_name, data.variant_name].filter(Boolean).join(' · ') || '—' }}</span></template>
      </Column>
      <Column :header="t('cycles.progress')" style="min-width: 220px">
        <template #body="{ data }">
          <SummaryBar :summary="progressSummary(data.progress)" />
          <div class="small muted">{{ t('cycles.executedOf', { executed: data.progress.executed, planned: data.progress.planned, pct: data.progress.percent_executed }) }}</div>
        </template>
      </Column>
      <Column :header="t('cycles.time')" style="width: 150px">
        <template #body="{ data }">
          <span class="small">{{ t('cycles.remainingOf', { remaining: mins(data.progress.remaining_minutes), total: mins(data.progress.estimated_minutes) }) }}</span>
        </template>
      </Column>
    </DataTable>
  </section>

  <section v-else-if="tab === 'timeline'" v-bind="panel('timeline')" style="margin-top: 12px" aria-labelledby="cyc-tl-h">
    <h2 id="cyc-tl-h" class="sr-only">{{ t('cycles.tabTimeline') }}</h2>
    <div v-if="!tl.bars.length" class="card empty-state"><p>{{ t('cycles.noDates') }}</p></div>
    <div v-else class="card timeline">
      <div class="tl-range small muted">
        <span>{{ formatDate(tl.from) }}</span><span>{{ formatDate(tl.to) }}</span>
      </div>
      <ul class="plain-list">
        <li v-for="b in tl.bars" :key="b.id" class="tl-row">
          <RouterLink :to="{ name: 'cycle', params: { projectId: pid, cycleId: b.id } }" class="tl-name ellipsis">{{ b.name }}</RouterLink>
          <div class="tl-track">
            <span class="tl-bar" :class="`tl-${b.status}`" :style="{ insetInlineStart: `${b.left}%`, width: `${b.width}%` }">
              <span class="sr-only">{{ statusLabel(b.status) }}, </span>{{ formatDate(b.start) }} – {{ formatDate(b.end) }}
            </span>
          </div>
        </li>
      </ul>
      <div class="row small muted" style="gap: 12px; margin-top: 8px">
        <span v-for="s in CYCLE_STATUSES" :key="s" class="row nowrap" style="gap: 4px"><span class="tl-swatch" :class="`tl-${s}`" aria-hidden="true" />{{ statusLabel(s) }}</span>
      </div>
    </div>
  </section>

  <section v-else v-bind="panel('schedules')" style="margin-top: 12px" aria-labelledby="cyc-sched-h">
    <div class="row" style="justify-content: space-between">
      <h2 id="cyc-sched-h" style="margin: 0; font-size: 16px">{{ t('cycles.schedule.title') }}</h2>
      <Button v-if="canPlan" class="sched-new" :label="t('cycles.schedule.new')" icon="pi pi-plus" size="small" aria-haspopup="dialog" @click="openSchedule(null)" />
    </div>
    <p class="small muted">{{ t('cycles.schedule.intro') }}</p>
    <div v-if="schedLoaded && !schedules.length" class="card empty-state"><p>{{ t('cycles.schedule.empty') }}</p></div>
    <ul v-else class="plain-list card" style="padding: 0">
      <li v-for="s in schedules" :key="s.id" class="sched-row">
        <div class="grow" style="min-width: 0">
          <div class="row nowrap">
            <i class="pi pi-replay muted" aria-hidden="true" />
            <b class="ellipsis">{{ s.name_pattern }}</b>
            <Tag v-if="!s.enabled" :value="t('cycles.schedule.off')" severity="secondary" />
            <Tag v-if="s.auto_start" :value="t('cycles.schedule.autoStart')" severity="info" />
          </div>
          <div class="small muted wrap">
            {{ describeSchedule(s) }} · {{ t('cycles.schedule.lasts', { n: s.duration_days }, s.duration_days) }}
            <span v-if="s.template_name"> · {{ t('cycles.schedule.fromTemplate', { name: s.template_name }) }}</span>
            <span v-if="s.query"> · <code>{{ s.query }}</code></span>
          </div>
          <div class="small muted">
            <span v-if="s.next_run_at">{{ t('cycles.schedule.next', { when: formatDateTime(s.next_run_at) }) }}</span>
            <span v-if="s.last_run_at"> · {{ t('cycles.schedule.last', { when: formatDateTime(s.last_run_at), n: s.runs }) }}</span>
          </div>
          <Message v-if="s.last_error" severity="warn" :closable="false" size="small">{{ s.last_error }}</Message>
        </div>
        <span v-if="canPlan" class="row nowrap">
          <Button icon="pi pi-play" text rounded size="small" severity="secondary" :aria-label="t('cycles.schedule.runNow', { name: s.name_pattern })" v-tooltip="t('cycles.schedule.runNowHint')" @click="runSchedule(s)" />
          <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('cycles.schedule.edit', { name: s.name_pattern })" @click="openSchedule(s)" />
          <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('cycles.schedule.delete', { name: s.name_pattern })" @click="removeSchedule(s)" />
        </span>
      </li>
    </ul>
  </section>

  <Dialog v-model:visible="dialog" modal :header="t('cycles.newTitle')" style="width: min(680px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="create">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field">
        <label :for="`${id}-name`">{{ t('common.name') }}</label>
        <InputText :id="`${id}-name`" v-model="form.name" maxlength="200" required autofocus :placeholder="t('cycles.namePlaceholder')" />
      </div>
      <div class="field">
        <label :for="`${id}-goal`">{{ t('cycles.goal') }}</label>
        <InputText :id="`${id}-goal`" v-model="form.goal" maxlength="1000" />
      </div>
      <div class="field">
        <label :for="`${id}-copy`">{{ t('cycles.copyFrom') }}</label>
        <Select v-model="form.copy_from" :input-id="`${id}-copy`" :options="copyOptions" option-label="label" option-value="value" show-clear filter :placeholder="t('cycles.copyNone')" :aria-describedby="`${id}-copy-help`" />
        <small :id="`${id}-copy-help`" class="muted">{{ t('cycles.copyHelp') }}</small>
      </div>
      <div class="form-grid">
        <div class="field">
          <label :for="`${id}-owner`">{{ t('cycles.owner') }}</label>
          <Select v-model="form.owner_id" :input-id="`${id}-owner`" :options="peopleOptions" option-label="label" option-value="value" filter />
        </div>
        <div v-if="!envs.environments.value.length" class="field">
          <label :for="`${id}-env`">{{ t('cycles.environment') }}</label>
          <InputText :id="`${id}-env`" v-model="form.environment" maxlength="200" />
        </div>
        <div class="field">
          <label :for="`${id}-start`">{{ t('cycles.startDate') }}</label>
          <InputText :id="`${id}-start`" v-model="form.start_date" type="date" />
        </div>
        <div class="field">
          <label :for="`${id}-end`">{{ t('cycles.endDate') }}</label>
          <InputText :id="`${id}-end`" v-model="form.end_date" type="date" />
        </div>
      </div>
      <EnvironmentField v-if="envs.environments.value.length" v-model="cycleEnv" v-model:booking="cycleBooking" :environments="envs.environments.value" :id-prefix="`${id}-cy`" :can-book="auth.can(P.runExecute)" />
      <template v-if="!form.copy_from">
        <div class="form-grid">
          <div class="field">
            <label :for="`${id}-rel`">{{ t('cycles.release') }}</label>
            <Select v-model="form.release_id" :input-id="`${id}-rel`" :options="releaseOptions" option-label="label" option-value="value" show-clear :placeholder="t('cycles.none')" />
          </div>
          <div class="field">
            <label :id="`${id}-var-label`" :for="`${id}-var`">{{ t('cycles.variant') }}</label>
            <VariantSelect v-model="form.variant_id" :input-id="`${id}-var`" :labelledby="`${id}-var-label`" :variants="vs.variants.value" :placeholder="t('cycles.none')" />
          </div>
        </div>
        <div class="field">
          <label :for="`${id}-cfgs`">{{ t('cycles.configurations') }}</label>
          <MultiSelect v-model="form.configuration_ids" :input-id="`${id}-cfgs`" :options="cfg.options.value" option-label="label" option-value="value" display="chip" filter :placeholder="t('cycles.noConfigurations')" :aria-describedby="`${id}-cfgs-help`" />
          <small :id="`${id}-cfgs-help`" class="muted">{{ t('cycles.configurationsHelp') }}</small>
        </div>
      </template>
      <div class="field">
        <label :for="`${id}-sched`">{{ t('cycles.scheduledStart') }}</label>
        <InputText :id="`${id}-sched`" v-model="form.scheduled_start" type="datetime-local" :aria-describedby="`${id}-sched-help`" />
        <small :id="`${id}-sched-help`" class="muted">{{ t('cycles.scheduledStartHelp') }}</small>
      </div>
      <div class="field">
        <label :for="`${id}-desc`">{{ t('common.description') }}</label>
        <Textarea :id="`${id}-desc`" v-model="form.description" rows="2" auto-resize maxlength="20000" />
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="dialog = false" />
        <Button type="submit" :label="t('cycles.create')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>

  <Dialog v-model:visible="schedDialog" modal :header="schedEditing ? t('cycles.schedule.editTitle') : t('cycles.schedule.newTitle')" style="width: min(640px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="saveSchedule">
      <Message v-if="schedError" severity="error" :closable="false">{{ schedError }}</Message>
      <div class="field">
        <label :for="`${id}-sp`">{{ t('cycles.schedule.pattern') }}</label>
        <InputText :id="`${id}-sp`" v-model="sched.name_pattern" maxlength="180" required :placeholder="t('cycles.schedule.patternPlaceholder')" :aria-describedby="`${id}-sp-help`" />
        <small :id="`${id}-sp-help`" class="muted">{{ t('cycles.schedule.patternHelp', { example: namePreview }) }}</small>
      </div>
      <div class="field">
        <label :for="`${id}-tpl`">{{ t('cycles.schedule.template') }}</label>
        <Select v-model="sched.template_cycle_id" :input-id="`${id}-tpl`" :options="copyOptions" option-label="label" option-value="value" show-clear filter :placeholder="t('cycles.none')" />
      </div>
      <div class="field">
        <label :for="`${id}-sq`">{{ t('cycles.schedule.query') }}</label>
        <InputText :id="`${id}-sq`" v-model="sched.query" maxlength="2000" placeholder="tag:regression" :aria-describedby="`${id}-sq-help`" />
        <small :id="`${id}-sq-help`" class="muted">{{ t('cycles.schedule.queryHelp') }}</small>
      </div>
      <div class="form-grid">
        <div class="field">
          <label :for="`${id}-freq`">{{ t('cycles.schedule.frequencyLabel') }}</label>
          <Select v-model="sched.frequency" :input-id="`${id}-freq`" :options="frequencyOptions" option-label="label" option-value="value" />
        </div>
        <div v-if="sched.frequency === 'weekly'" class="field">
          <label :for="`${id}-wd`">{{ t('cycles.schedule.weekdayLabel') }}</label>
          <Select v-model="sched.weekday" :input-id="`${id}-wd`" :options="weekdayOptions" option-label="label" option-value="value" />
        </div>
        <div v-if="sched.frequency === 'monthly'" class="field">
          <label :for="`${id}-dom`">{{ t('cycles.schedule.dayOfMonth') }}</label>
          <InputNumber v-model="sched.day_of_month" :input-id="`${id}-dom`" :min="1" :max="28" show-buttons fluid />
        </div>
        <div class="field">
          <label :for="`${id}-time`">{{ t('cycles.schedule.time') }}</label>
          <InputText :id="`${id}-time`" v-model="sched.time" type="time" />
        </div>
        <div class="field">
          <label :for="`${id}-tz`">{{ t('cycles.schedule.timezone') }}</label>
          <InputText :id="`${id}-tz`" v-model="sched.timezone" maxlength="64" />
        </div>
        <div class="field">
          <label :for="`${id}-dur`">{{ t('cycles.schedule.duration') }}</label>
          <InputNumber v-model="sched.duration_days" :input-id="`${id}-dur`" :min="1" :max="366" show-buttons fluid />
        </div>
        <div class="field">
          <label :for="`${id}-sowner`">{{ t('cycles.owner') }}</label>
          <Select v-model="sched.owner_id" :input-id="`${id}-sowner`" :options="peopleOptions" option-label="label" option-value="value" filter />
        </div>
      </div>
      <div class="row">
        <Checkbox v-model="sched.auto_start" binary :input-id="`${id}-auto`" />
        <label :for="`${id}-auto`">{{ t('cycles.schedule.autoStartLabel') }}</label>
      </div>
      <div class="row">
        <Checkbox v-model="sched.enabled" binary :input-id="`${id}-en`" />
        <label :for="`${id}-en`">{{ t('cycles.schedule.enabled') }}</label>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="schedDialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="!sched.name_pattern.trim() || (!sched.template_cycle_id && !sched.query.trim())" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.empty-state {
  text-align: center;
  padding: 24px;
}
.ml-4 {
  margin-inline-start: 4px;
}
.strong {
  font-weight: 600;
}
.timeline {
  padding: 12px 16px;
}
.tl-range {
  display: flex;
  justify-content: space-between;
  margin-inline-start: 200px;
  margin-bottom: 6px;
}
.tl-row {
  display: grid;
  grid-template-columns: 190px 1fr;
  gap: 10px;
  align-items: center;
  padding: 4px 0;
}
.tl-track {
  position: relative;
  height: 26px;
  background: var(--qtf-bg);
  border-radius: 6px;
}
.tl-bar {
  position: absolute;
  top: 2px;
  bottom: 2px;
  border-radius: 5px;
  font-size: 11px;
  line-height: 22px;
  padding: 0 6px;
  overflow: hidden;
  white-space: nowrap;
  color: var(--qtf-text);
  border: 1px solid var(--qtf-border);
}
.tl-planned {
  background: color-mix(in srgb, var(--qtf-muted) 22%, var(--qtf-card));
}
.tl-active {
  background: color-mix(in srgb, var(--p-primary-color) 28%, var(--qtf-card));
}
.tl-closed {
  background: color-mix(in srgb, #22c55e 26%, var(--qtf-card));
}
.tl-swatch {
  display: inline-block;
  width: 12px;
  height: 12px;
  border-radius: 3px;
  border: 1px solid var(--qtf-border);
}
.sched-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--qtf-border);
}
.sched-row:last-child {
  border-bottom: 0;
}
.wrap {
  white-space: normal;
}
@media (max-width: 640px) {
  .tl-row {
    grid-template-columns: 1fr;
  }
  .tl-range {
    margin-inline-start: 0;
  }
}
</style>
