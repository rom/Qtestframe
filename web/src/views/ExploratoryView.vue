<script setup lang="ts">
// Exploratory testing (session based): the project's sessions with their
// charter, tester, time box and status, planning and changing sessions,
// and the metrics (sessions per area, bugs per session hour).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ItemRef from '@/components/ItemRef.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import { folderPath } from '@/lib/tree'
import {
  EXPLORATORY_STATUSES,
  TIMEBOXES,
  filterSessions,
  formatClock,
  outcomeSeverity,
  statusIcon,
  statusSeverity,
  type ExploratoryMetrics,
  type ExploratorySession,
  type ScopeItem,
} from '@/lib/exploratory'
import { useProjectStore } from '@/stores/project'
import { useProjectPeopleStore } from '@/stores/projectPeople'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'
import { useEnvironments } from '@/composables/environments'
import EnvironmentField from '@/components/EnvironmentField.vue'
import type { BookingInput } from '@/lib/environments'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const people = useProjectPeopleStore()
const auth = useAuthStore()
const prefs = usePrefsStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const canPlan = computed(() => auth.can(P.runWrite))

const statusLabel = (s: string) => t(`exploratory.status.${s}`)
const outcomeLabel = (s: string) => t(`exploratory.outcome.${s}`)

// ---------- data ----------
const sessions = ref<ExploratorySession[]>([])
const metrics = ref<ExploratoryMetrics | null>(null)
const loading = ref(false)
const error = ref('')
async function load() {
  loading.value = true
  error.value = ''
  try {
    const [list, m] = await Promise.all([
      api.get<{ items: ExploratorySession[] }>(`/projects/${proj.pid}/exploratory-sessions`),
      api.get<ExploratoryMetrics>(`/projects/${proj.pid}/exploratory-metrics`),
    ])
    sessions.value = asItems(list)
    metrics.value = m
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  void people.ensure(proj.pid)
  await load()
  if (route.query.new === '1' && canPlan.value) {
    openCreate()
    router.replace({ query: { ...route.query, new: undefined } })
  }
})

// ---------- filters ----------
const text = ref('')
const statuses = ref<string[]>([])
const area = ref('')
const mineOnly = ref(false)
const me = computed(() => (auth.user?.email ?? '').toLowerCase())
const filtered = computed(() =>
  filterSessions(sessions.value, {
    text: text.value,
    statuses: statuses.value,
    area: area.value,
    tester: mineOnly.value ? me.value : '',
  }),
)
const statusOptions = computed(() => EXPLORATORY_STATUSES.map((v) => ({ label: statusLabel(v), value: v })))
const allFolders = computed(() => [...proj.folders.requirement, ...proj.folders.test])
const areaLabel = (id: string) => {
  const req = proj.folders.requirement.some((f) => f.id === id)
  const path = folderPath(req ? proj.folders.requirement : proj.folders.test, id) || id
  return `${path} (${req ? t('exploratory.areaReq') : t('exploratory.areaTest')})`
}
const areaOptions = computed(() => allFolders.value.map((f) => ({ label: areaLabel(f.id), value: f.id })))
const areaFilterOptions = computed(() => [{ label: t('exploratory.anyArea'), value: '' }, ...areaOptions.value])
const hasFilter = computed(() => !!text.value.trim() || !!statuses.value.length || !!area.value || mineOnly.value)
function clearFilters() {
  text.value = ''
  statuses.value = []
  area.value = ''
  mineOnly.value = false
}
const areasOf = (x: ExploratorySession) => x.scope.filter((s) => s.type === 'folder')
const open = (x: ExploratorySession) =>
  router.push({
    name: 'exploratorySession',
    params: { projectId: proj.pid, sessionId: x.id },
  })

// ---------- metrics ----------
const pct = (v: number) => (v < 0 ? '–' : `${v}%`)
const num = (v: number) => (Math.round(v * 100) / 100).toLocaleString()

// ---------- create / edit ----------
interface Form {
  title: string
  mission: string
  tester: string
  timebox_minutes: number
  areas: string[]
  requirements: ScopeItem[]
  tests: ScopeItem[]
}
const blank = (): Form => ({
  title: '',
  mission: '',
  tester: me.value,
  timebox_minutes: 60,
  areas: [],
  requirements: [],
  tests: [],
})
const formOpen = ref(false)
const envs = useEnvironments(() => proj.pid)
const xsEnv = ref<string | null>(null)
const xsBooking = ref<BookingInput | null>(null)
const editing = ref<ExploratorySession | null>(null)
const form = ref<Form>(blank())
const formError = ref('')
const busy = ref(false)
const picker = ref<'' | 'requirement' | 'test'>('')
const timeboxOptions = computed(() => TIMEBOXES.map((n) => ({ label: t('exploratory.minutes', { n }), value: n })))
const testerOptions = computed(() => [
  { label: t('exploratory.unassigned'), value: '' },
  ...people.people
    .filter((x) => x.kind === 'user' && x.email)
    .map((x) => ({
      label: x.name && x.name !== x.email ? `${x.name} (${x.email})` : (x.email as string),
      value: (x.email as string).toLowerCase(),
    })),
])
function openCreate() {
  editing.value = null
  form.value = blank()
  xsEnv.value = null
  xsBooking.value = null
  void envs.load()
  formError.value = ''
  formOpen.value = true
}
function openEdit(x: ExploratorySession) {
  editing.value = x
  form.value = {
    title: x.title,
    mission: x.mission,
    tester: x.tester,
    timebox_minutes: x.timebox_minutes,
    areas: x.scope.filter((s) => s.type === 'folder').map((s) => s.id),
    requirements: x.scope.filter((s) => s.type === 'requirement'),
    tests: x.scope.filter((s) => s.type === 'test'),
  }
  xsEnv.value = x.environment_id ?? null
  xsBooking.value = null
  void envs.load()
  formError.value = ''
  formOpen.value = true
}
function addPicked(items: { id: string; key?: string; title?: string }[]) {
  const kind = picker.value
  if (!kind) return
  const list = kind === 'requirement' ? form.value.requirements : form.value.tests
  const have = new Set(list.map((s) => s.id))
  for (const i of items) if (!have.has(i.id)) list.push({ type: kind, id: i.id, key: i.key, title: i.title })
}
async function save() {
  busy.value = true
  formError.value = ''
  const f = form.value
  const body: Record<string, unknown> = {
    title: f.title.trim(),
    mission: f.mission,
    tester: f.tester,
    timebox_minutes: f.timebox_minutes,
    scope: [...f.areas.map((id) => ({ type: 'folder', id })), ...f.requirements.map((s) => ({ type: 'requirement', id: s.id })), ...f.tests.map((s) => ({ type: 'test', id: s.id }))],
  }
  if (envs.environments.value.length) {
    body.environment_id = xsEnv.value ?? ''
    if (xsEnv.value && xsBooking.value) body.booking = xsBooking.value
  }
  try {
    const x = editing.value
      ? await api.patch<ExploratorySession>(`/exploratory-sessions/${editing.value.id}`, body)
      : await api.post<ExploratorySession>(`/projects/${proj.pid}/exploratory-sessions`, body)
    notify.ok(editing.value ? t('exploratory.updated') : t('exploratory.created'), x.key)
    formOpen.value = false
    if (!editing.value) await open(x)
    else await load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
function remove(x: ExploratorySession) {
  notify.confirmDanger(t('exploratory.confirmDelete', { key: x.key, title: x.title }), async () => {
    try {
      await api.del(`/exploratory-sessions/${x.id}`)
      notify.ok(t('exploratory.deleted'), x.key)
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
watch(
  () => proj.pid,
  () => void people.ensure(proj.pid),
)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('exploratory.title') }}</h1>
      <div class="sub">{{ t('exploratory.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="exploratory-testing" />
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
      <Button v-if="canPlan" :label="t('exploratory.new')" icon="pi pi-plus" @click="openCreate" />
    </div>
  </div>

  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <template v-else>
    <section v-if="metrics" class="card" aria-labelledby="xm-title" style="margin-bottom: 12px">
      <h2 id="xm-title" class="plain-heading" style="margin-bottom: 8px">
        {{ t('exploratory.metrics') }}
      </h2>
      <dl class="xm-kpis">
        <div>
          <dt>{{ t('exploratory.kpi.sessions') }}</dt>
          <dd>{{ metrics.sessions }}</dd>
        </div>
        <div>
          <dt>{{ t('exploratory.kpi.hours') }}</dt>
          <dd>{{ num(metrics.hours) }}</dd>
        </div>
        <div>
          <dt>{{ t('exploratory.kpi.bugs') }}</dt>
          <dd>{{ metrics.bugs }}</dd>
        </div>
        <div>
          <dt>{{ t('exploratory.kpi.bugsPerHour') }}</dt>
          <dd>{{ num(metrics.bugs_per_hour) }}</dd>
        </div>
        <div>
          <dt>{{ t('exploratory.kpi.split') }}</dt>
          <dd class="small">{{ pct(metrics.avg_setup_pct) }} / {{ pct(metrics.avg_testing_pct) }} / {{ pct(metrics.avg_bug_pct) }}</dd>
        </div>
        <div>
          <dt>{{ t('exploratory.kpi.coverage') }}</dt>
          <dd>{{ pct(metrics.avg_coverage_pct) }}</dd>
        </div>
      </dl>
      <div class="xm-groups">
        <div>
          <h3 class="plain-heading small" style="margin: 10px 0 4px">
            {{ t('exploratory.byArea') }}
          </h3>
          <table class="plain-table small">
            <thead>
              <tr>
                <th scope="col">{{ t('exploratory.area') }}</th>
                <th scope="col">{{ t('exploratory.kpi.sessions') }}</th>
                <th scope="col">{{ t('exploratory.kpi.hours') }}</th>
                <th scope="col">{{ t('exploratory.kpi.bugsPerHour') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="g in metrics.by_area" :key="g.id">
                <td>{{ g.id ? areaLabel(g.id) : t('exploratory.noArea') }}</td>
                <td>{{ g.sessions }}</td>
                <td>{{ num(g.hours) }}</td>
                <td>{{ num(g.bugs_per_hour) }}</td>
              </tr>
              <tr v-if="!metrics.by_area.length">
                <td colspan="4" class="muted">—</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div>
          <h3 class="plain-heading small" style="margin: 10px 0 4px">
            {{ t('exploratory.byTester') }}
          </h3>
          <table class="plain-table small">
            <thead>
              <tr>
                <th scope="col">{{ t('exploratory.tester') }}</th>
                <th scope="col">{{ t('exploratory.kpi.sessions') }}</th>
                <th scope="col">{{ t('exploratory.kpi.hours') }}</th>
                <th scope="col">{{ t('exploratory.kpi.bugsPerHour') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="g in metrics.by_tester" :key="g.id">
                <td>{{ g.id || t('exploratory.unassigned') }}</td>
                <td>{{ g.sessions }}</td>
                <td>{{ num(g.hours) }}</td>
                <td>{{ num(g.bugs_per_hour) }}</td>
              </tr>
              <tr v-if="!metrics.by_tester.length">
                <td colspan="4" class="muted">—</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <section class="card" aria-labelledby="xs-list-title">
      <h2 id="xs-list-title" class="sr-only">{{ t('exploratory.list') }}</h2>
      <div class="filterbar">
        <InputText v-model="text" :placeholder="t('exploratory.searchPlaceholder')" :aria-label="t('exploratory.searchPlaceholder')" style="min-width: 200px" />
        <MultiSelect
          v-model="statuses"
          :options="statusOptions"
          option-label="label"
          option-value="value"
          :placeholder="t('common.status')"
          :aria-label="t('common.status')"
          show-clear
          style="min-width: 140px"
        />
        <Select v-model="area" :options="areaFilterOptions" option-label="label" option-value="value" :aria-label="t('exploratory.area')" filter style="min-width: 180px" />
        <Button :label="t('exploratory.mine')" size="small" :severity="mineOnly ? undefined : 'secondary'" :outlined="!mineOnly" :aria-pressed="mineOnly" @click="mineOnly = !mineOnly" />
        <Button v-if="hasFilter" :label="t('common.clear')" text size="small" severity="secondary" @click="clearFilters" />
        <span class="small muted" role="status">{{
          t('exploratory.shown', {
            count: filtered.length,
            total: sessions.length,
          })
        }}</span>
      </div>
      <DataTable v-stack :value="filtered" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(25)" :always-show-paginator="false" row-hover size="small">
        <Column field="key" :header="t('common.key')" sortable style="width: 120px"
          ><template #body="{ data }"
            ><span class="key">{{ data.key }}</span></template
          ></Column
        >
        <Column field="title" :header="t('exploratory.charter')" sortable body-class="stack-main" style="min-width: 220px">
          <template #body="{ data }">
            <RouterLink
              :to="{
                name: 'exploratorySession',
                params: { projectId: proj.pid, sessionId: data.id },
              }"
              style="font-weight: 600"
              :aria-label="t('exploratory.open', { key: data.key, title: data.title })"
              >{{ data.title }}</RouterLink
            >
            <div v-if="areasOf(data).length" class="small muted">
              {{
                areasOf(data)
                  .map((a: ScopeItem) => a.title)
                  .join(', ')
              }}
            </div>
          </template>
        </Column>
        <Column field="tester" :header="t('exploratory.tester')" sortable style="width: 180px">
          <template #body="{ data }"
            ><span class="small">{{ data.tester || t('exploratory.unassigned') }}</span></template
          >
        </Column>
        <Column field="status" :header="t('common.status')" sortable style="width: 140px">
          <template #body="{ data }">
            <Tag :value="statusLabel(data.status)" :severity="statusSeverity(data.status)" :icon="statusIcon(data.status)" />
            <Tag v-if="data.debrief" :value="outcomeLabel(data.debrief.outcome)" :severity="outcomeSeverity(data.debrief.outcome)" style="margin-left: 4px" />
          </template>
        </Column>
        <Column field="elapsed_seconds" :header="t('exploratory.time')" sortable style="width: 130px">
          <template #body="{ data }"
            ><span class="small mono">{{ formatClock(data.elapsed_seconds) }}</span> <span class="small muted">/ {{ data.timebox_minutes }}′</span></template
          >
        </Column>
        <Column field="note_count" :header="t('exploratory.notes')" sortable style="width: 90px" />
        <Column field="bug_count" :header="t('exploratory.bugs')" sortable style="width: 80px" />
        <Column field="created_at" :header="t('common.created')" sortable style="width: 160px"
          ><template #body="{ data }"
            ><span class="small">{{ formatDateTime(data.created_at) }}</span></template
          ></Column
        >
        <Column v-if="canPlan" :header="t('common.actions')" style="width: 100px">
          <template #body="{ data }">
            <Button
              v-if="data.status !== 'debriefed'"
              icon="pi pi-pencil"
              text
              rounded
              size="small"
              severity="secondary"
              :aria-label="t('exploratory.editOf', { key: data.key })"
              v-tooltip="t('common.edit')"
              @click="openEdit(data as ExploratorySession)"
            />
            <Button
              icon="pi pi-trash"
              text
              rounded
              size="small"
              severity="danger"
              :aria-label="t('exploratory.deleteOf', { key: data.key })"
              v-tooltip="t('common.delete')"
              @click="remove(data as ExploratorySession)"
            />
          </template>
        </Column>
        <template #empty>
          <div class="empty"><i class="pi pi-compass" aria-hidden="true" />{{ sessions.length ? t('exploratory.noneMatch') : t('exploratory.none') }}</div>
        </template>
      </DataTable>
    </section>
  </template>

  <Dialog
    v-model:visible="formOpen"
    :header="editing ? t('exploratory.editHeader', { key: editing.key }) : t('exploratory.new')"
    modal
    maximizable
    :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }"
    style="width: min(760px, 96vw)"
  >
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="form-grid">
        <div v-field class="field full">
          <label for="xs-title">{{ t('exploratory.charter') }}</label
          ><InputText id="xs-title" v-model="form.title" autofocus :placeholder="t('exploratory.charterPlaceholder')" />
        </div>
        <div v-field class="field full">
          <label for="xs-mission">{{ t('exploratory.mission') }}</label>
          <Textarea id="xs-mission" v-model="form.mission" rows="3" auto-resize aria-describedby="xs-mission-hint" />
          <small id="xs-mission-hint" class="muted">{{ t('exploratory.missionHint') }}</small>
        </div>
        <div v-field class="field">
          <label for="xs-tester">{{ t('exploratory.tester') }}</label>
          <Select v-model="form.tester" input-id="xs-tester" :options="testerOptions" option-label="label" option-value="value" filter editable />
        </div>
        <div v-field class="field">
          <label for="xs-box">{{ t('exploratory.timebox') }}</label>
          <div class="row nowrap" style="gap: 6px">
            <Select v-model="form.timebox_minutes" input-id="xs-box" :options="timeboxOptions" option-label="label" option-value="value" style="min-width: 120px" />
            <InputNumber v-model="form.timebox_minutes" :min="5" :max="480" :aria-label="t('exploratory.timeboxCustom')" suffix=" min" style="width: 110px" />
          </div>
        </div>
        <div v-if="envs.environments.value.length" class="field full">
          <EnvironmentField v-model="xsEnv" v-model:booking="xsBooking" :environments="envs.environments.value" id-prefix="xs" :can-book="auth.can(P.runExecute)" />
        </div>
        <div v-field class="field full">
          <label for="xs-areas">{{ t('exploratory.areas') }}</label>
          <MultiSelect
            v-model="form.areas"
            input-id="xs-areas"
            :options="areaOptions"
            option-label="label"
            option-value="value"
            filter
            display="chip"
            :placeholder="t('exploratory.areasPlaceholder')"
          />
        </div>
        <div v-for="kind in ['requirement', 'test'] as const" :key="kind" class="field full">
          <div class="section-title">
            <i :class="kind === 'requirement' ? 'pi pi-list-check' : 'pi pi-check-square'" aria-hidden="true" />
            {{
              kind === 'requirement'
                ? t('exploratory.scopeRequirements', {
                    count: form.requirements.length,
                  })
                : t('exploratory.scopeTests', { count: form.tests.length })
            }}
            <span class="grow" />
            <Button size="small" :label="kind === 'requirement' ? t('exploratory.addRequirements') : t('exploratory.addTests')" icon="pi pi-plus" text @click="picker = kind" />
          </div>
          <div v-for="s in kind === 'requirement' ? form.requirements : form.tests" :key="s.id" class="row nowrap" style="padding: 4px 0; border-bottom: 1px solid var(--qtf-border)">
            <ItemRef
              :to="{
                name: kind === 'requirement' ? 'requirements' : 'tests',
                params: { projectId: proj.pid },
                query: { item: s.id },
              }"
              :item-key="s.key ?? s.id.slice(0, 8)"
              :title="s.title ?? ''"
              title-class="ellipsis"
              grow
            />
            <Button
              icon="pi pi-times"
              text
              rounded
              size="small"
              severity="secondary"
              :aria-label="t('common.unlinkItem', { key: s.key ?? s.id })"
              @click="kind === 'requirement' ? (form.requirements = form.requirements.filter((x) => x.id !== s.id)) : (form.tests = form.tests.filter((x) => x.id !== s.id))"
            />
          </div>
        </div>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="formOpen = false" />
        <Button type="submit" :label="editing ? t('common.save') : t('exploratory.plan')" icon="pi pi-check" :loading="busy" :disabled="!form.title.trim()" />
      </div>
    </form>
  </Dialog>
  <ItemPickerDialog
    :visible="!!picker"
    :kind="picker || 'requirement'"
    :project-id="proj.pid"
    :exclude="(picker === 'test' ? form.tests : form.requirements).map((s) => s.id)"
    :header="picker === 'test' ? t('exploratory.addTests') : t('exploratory.addRequirements')"
    :confirm-label="t('common.add')"
    @update:visible="(v: boolean) => !v && (picker = '')"
    @pick="addPicked"
  />
</template>

<style scoped>
.xm-kpis {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 8px 16px;
  margin: 0;
}
.xm-kpis dt {
  color: var(--qtf-muted);
  font-size: 0.85em;
}
.xm-kpis dd {
  margin: 0;
  font-size: 1.3em;
  font-weight: 600;
}
.xm-groups {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 0 24px;
}
.plain-table {
  width: 100%;
  border-collapse: collapse;
}
.plain-table th,
.plain-table td {
  text-align: left;
  padding: 3px 6px;
  border-bottom: 1px solid var(--qtf-border);
}
</style>
