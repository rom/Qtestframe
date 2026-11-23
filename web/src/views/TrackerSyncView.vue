<script setup lang="ts">
// Two-way sync of requirements with Jira and Azure DevOps
// (internal/api/reqsync.go): the state of both sources with "Sync now" and
// a preview, the conflicts that wait for a decision (a field diff with
// base, Qtestframe and tracker values), the sync log and, for project
// managers, the settings: field mapping with directions and value maps,
// scopes, conflict policy, impact choice, comments and links, the webhook.
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import RadioButton from 'primevue/radiobutton'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import { folderPath } from '@/lib/tree'
import { trackerError } from '@/lib/defectTracker'
import type { Folder } from '@/lib/types'
import type { ImportSourceId } from '@/lib/importSources'
import {
  ENTRY_ICONS,
  MAPPED_VALUE_FIELDS,
  SYNC_DIRECTIONS,
  SYNC_FIELDS,
  SYNC_IMPACTS,
  SYNC_POLICIES,
  SYNC_SOURCE_NAMES,
  fullyChosen,
  nextField,
  randomSecret,
  resolveBody,
  runSeverity,
  summaryParts,
  syncForm,
  syncPayload,
  validateSync,
  type ReqSyncConfig,
  type ReqSyncConflict,
  type ReqSyncEntry,
  type ReqSyncRun,
  type SyncForm,
} from '@/lib/reqSync'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { uid } from '@/lib/a11y'

const { t } = useI18n()
const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const id = uid('rsync')
const tabsId = uid('rsync-tabs')
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })
const pid = computed(() => proj.pid)
const canRun = computed(() => auth.can(P.requirementWrite))
const canConfigure = computed(() => auth.can(P.projectUpdate))

const tabs = ['overview', 'conflicts', 'log', 'settings'] as const
const tab = ref<string>(tabs.includes(route.query.tab as never) ? (route.query.tab as string) : 'overview')
watch(tab, (v) => router.replace({ query: { ...route.query, tab: v === 'overview' ? undefined : v } }))

const configs = ref<ReqSyncConfig[]>([])
const loadError = ref('')
const folders = ref<Folder[]>([])
const sourceName = (s: string) => SYNC_SOURCE_NAMES[s as ImportSourceId] ?? s

async function loadConfigs() {
  try {
    configs.value = asItems(await api.get<{ items: ReqSyncConfig[] }>(`/projects/${pid.value}/req-sync`))
    loadError.value = ''
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}

// ---------- overview: sync now, preview ----------
const running = ref('')
const previews = ref<Record<string, ReqSyncRun | null>>({})
const results = ref<Record<string, string>>({})
async function runSync(c: ReqSyncConfig, dry: boolean) {
  running.value = c.system + (dry ? ':dry' : '')
  try {
    const run = await api.post<ReqSyncRun>(`/projects/${pid.value}/req-sync/${c.system}/run`, { dry_run: dry })
    if (dry) previews.value = { ...previews.value, [c.system]: run }
    else {
      previews.value = { ...previews.value, [c.system]: null }
      results.value = { ...results.value, [c.system]: run.error || summaryText(run) }
      if (run.status === 'error') notify.fail(new Error(run.error || ''), t('reqSync.runFailed'))
      else notify.ok(t('reqSync.ran', { source: sourceName(c.system) }))
      await Promise.all([loadConfigs(), loadRuns(), loadConflicts()])
    }
  } catch (e) {
    notify.fail(new Error(trackerError(e) ?? errorMessage(e)), t('reqSync.runFailed'))
  } finally {
    running.value = ''
  }
}
function summaryText(run: ReqSyncRun): string {
  const parts = summaryParts(run.summary).map(([k, n]) => t(`reqSync.count.${k}`, { n }, n))
  return parts.length ? parts.join(', ') : t('reqSync.nothing')
}

// ---------- conflicts ----------
const conflicts = ref<ReqSyncConflict[]>([])
const choices = ref<Record<string, Record<string, string>>>({})
const resolving = ref('')
async function loadConflicts() {
  try {
    conflicts.value = asItems(await api.get<{ items: ReqSyncConflict[] }>(`/projects/${pid.value}/req-sync/conflicts`))
    for (const c of conflicts.value) if (!choices.value[c.id]) choices.value[c.id] = {}
  } catch {
    conflicts.value = []
  }
}
async function resolve(c: ReqSyncConflict, all?: 'tracker' | 'qtestframe' | 'dismiss') {
  resolving.value = c.id
  try {
    await api.post(`/projects/${pid.value}/req-sync/conflicts/${c.id}/resolve`, resolveBody(c, choices.value[c.id] ?? {}, all))
    notify.ok(t('reqSync.resolved', { key: c.requirement_key }))
    await Promise.all([loadConflicts(), loadConfigs(), loadRuns()])
  } catch (e) {
    notify.fail(new Error(trackerError(e) ?? errorMessage(e)), t('reqSync.resolveFailed'))
  } finally {
    resolving.value = ''
  }
}
const fieldLabel = (f: string) => t(`reqSync.field.${f}`)

// ---------- log ----------
const runs = ref<ReqSyncRun[]>([])
const openRun = ref('')
const runEntries = ref<Record<string, ReqSyncEntry[]>>({})
async function loadRuns() {
  try {
    runs.value = asItems(await api.get<{ items: ReqSyncRun[] }>(`/projects/${pid.value}/req-sync/runs`, { limit: 50 }))
  } catch {
    runs.value = []
  }
}
async function toggleRun(r: ReqSyncRun) {
  if (openRun.value === r.id) {
    openRun.value = ''
    return
  }
  openRun.value = r.id
  if (!runEntries.value[r.id]) {
    try {
      const full = await api.get<ReqSyncRun>(`/projects/${pid.value}/req-sync/runs/${r.id}`)
      runEntries.value = { ...runEntries.value, [r.id]: full.entries ?? [] }
    } catch (e) {
      notify.fail(e)
    }
  }
}

// ---------- settings ----------
const source = ref<ImportSourceId>((route.query.source as ImportSourceId) === 'azure-devops' ? 'azure-devops' : 'jira')
const sourceOptions = computed(() => configs.value.map((c) => ({ value: c.system, label: sourceName(c.system) })))
const current = computed(() => configs.value.find((c) => c.system === source.value) ?? null)
const form = ref<SyncForm>(syncForm(null))
const touched = ref(false)
const saving = ref(false)
watch([current, source], () => {
  form.value = syncForm(current.value)
  touched.value = false
})
const problems = computed(() => validateSync(form.value))
const problem = (k: string) => (touched.value && problems.value[k] ? t(`reqSync.invalid.${problems.value[k]}`) : '')
const folderOptions = computed(() => [
  { value: '', label: t('reqSync.noFolder') },
  ...folders.value
    .filter((f) => f.kind === 'requirement')
    .map((f) => ({ value: f.id, label: folderPath(folders.value, f.id) }))
    .sort((a, b) => a.label.localeCompare(b.label)),
])
const policyOptions = computed(() => SYNC_POLICIES.map((p) => ({ value: p, label: t(`reqSync.policy.${p}`) })))
const impactOptions = computed(() => SYNC_IMPACTS.map((p) => ({ value: p, label: t(`reqSync.impact.${p}`) })))
const directionOptions = computed(() => SYNC_DIRECTIONS.map((d) => ({ value: d, label: t(`reqSync.direction.${d}`) })))
const fieldOptions = computed(() => SYNC_FIELDS.map((f) => ({ value: f, label: fieldLabel(f) })))
const isJira = computed(() => source.value === 'jira')

function addField() {
  const f = nextField(form.value)
  if (f) form.value.fields.push({ field: f, remote: '', direction: 'both', values: '', pushValues: '' })
}
function generateSecret() {
  form.value.secret = randomSecret()
}
async function copy(v: string) {
  try {
    await navigator.clipboard.writeText(v)
    notify.ok(t('reqSync.copied'))
  } catch {
    notify.fail(new Error(v))
  }
}
async function save(clearSecret = false) {
  touched.value = true
  if (!clearSecret && Object.keys(problems.value).length) return
  saving.value = true
  try {
    const saved = await api.put<ReqSyncConfig>(`/projects/${pid.value}/req-sync/${source.value}`, syncPayload(form.value, clearSecret))
    configs.value = configs.value.map((c) => (c.system === saved.system ? saved : c))
    notify.ok(clearSecret ? t('reqSync.secretRemoved') : t('reqSync.saved'))
  } catch (e) {
    notify.fail(e, t('reqSync.saveFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await loadConfigs()
  form.value = syncForm(current.value)
  try {
    folders.value = asItems(await api.get<{ items: Folder[] }>(`/projects/${pid.value}/folders`, { kind: 'requirement' }))
  } catch {
    folders.value = []
  }
  await Promise.all([loadConflicts(), loadRuns()])
})
const openConflicts = computed(() => conflicts.value.length)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('reqSync.title') }}</h1>
      <div class="sub">{{ t('reqSync.subtitle') }}</div>
    </div>
    <div class="actions"><HelpLink page="reports-and-export" :anchor="{ en: 'two-way-sync-with-jira-and-azure-devops', sv: 'tvåvägssynk-med-jira-och-azure-devops' }" /></div>
  </div>
  <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>

  <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs">
    <TabList>
      <Tab value="overview"><i class="pi pi-sync" aria-hidden="true" /> {{ t('reqSync.overview') }}</Tab>
      <Tab value="conflicts"><i class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ t('reqSync.conflicts') }} <span class="count">{{ openConflicts }}</span></Tab>
      <Tab value="log"><i class="pi pi-list" aria-hidden="true" /> {{ t('reqSync.log') }}</Tab>
      <Tab v-if="canConfigure" value="settings"><i class="pi pi-cog" aria-hidden="true" /> {{ t('reqSync.settings') }}</Tab>
    </TabList>
  </Tabs>

  <!-- overview -->
  <section v-if="tab === 'overview'" v-bind="panel('overview')" class="sync-overview" style="margin-top: 12px">
    <h2 class="sr-only">{{ t('reqSync.overview') }}</h2>
    <div class="sync-sources">
      <article v-for="c in configs" :key="c.system" class="card sync-source" :aria-labelledby="`${id}-h-${c.system}`">
        <div class="row" style="justify-content: space-between">
          <h3 :id="`${id}-h-${c.system}`" class="plain-heading" style="margin: 0">{{ sourceName(c.system) }}</h3>
          <Tag :value="c.enabled ? t('reqSync.on') : t('reqSync.off')" :severity="c.enabled ? 'success' : 'secondary'" />
        </div>
        <p v-if="!c.connected" class="small warn-text">
          {{ t('reqSync.notConnected') }}
          <RouterLink :to="{ name: 'settings', query: { tab: 'project' } }">{{ t('reqSync.toImportSources') }}</RouterLink>
        </p>
        <dl class="sync-facts small">
          <dt>{{ t('reqSync.linked') }}</dt>
          <dd>{{ c.linked }}</dd>
          <dt>{{ t('reqSync.lastRun') }}</dt>
          <dd>
            <template v-if="c.last_run_at">
              <time :datetime="c.last_run_at">{{ formatDateTime(c.last_run_at) }}</time>
              <Tag :value="t(`reqSync.status.${c.last_status || 'ok'}`)" :severity="runSeverity(c.last_status)" style="margin-left: 6px" />
            </template>
            <template v-else>{{ t('reqSync.never') }}</template>
          </dd>
          <dt>{{ t('reqSync.nextRun') }}</dt>
          <dd>
            <time v-if="c.enabled && c.next_run_at" :datetime="c.next_run_at">{{ formatDateTime(c.next_run_at) }}</time>
            <template v-else>{{ t('reqSync.noSchedule') }}</template>
          </dd>
          <dt>{{ t('reqSync.openConflicts') }}</dt>
          <dd>{{ c.open_conflicts }}</dd>
          <template v-if="c.run_as">
            <dt>{{ t('reqSync.runAs') }}</dt>
            <dd>{{ c.run_as }}</dd>
          </template>
        </dl>
        <p v-if="c.last_error" class="small warn-text sync-error">{{ c.last_error }}</p>
        <p v-if="results[c.system]" class="small muted" role="status">{{ results[c.system] }}</p>
        <div v-if="canRun" class="row">
          <Button :label="t('reqSync.syncNow')" icon="pi pi-sync" size="small" :loading="running === c.system" :disabled="!c.id || !c.connected || !!running" @click="runSync(c, false)" />
          <Button :label="t('reqSync.preview')" icon="pi pi-eye" size="small" severity="secondary" outlined :loading="running === c.system + ':dry'" :disabled="!c.id || !c.connected || !!running" @click="runSync(c, true)" />
          <span v-if="!c.id" class="small muted">{{ t('reqSync.saveFirst') }}</span>
        </div>
        <div v-if="previews[c.system]" class="sync-preview" aria-live="polite">
          <h4 class="plain-heading small">{{ t('reqSync.previewTitle') }} — {{ summaryText(previews[c.system]!) }}</h4>
          <ul v-if="previews[c.system]!.entries?.length" class="sync-entries">
            <li v-for="(x, i) in previews[c.system]!.entries" :key="i">
              <i :class="ENTRY_ICONS[x.action] ?? 'pi pi-circle'" aria-hidden="true" />
              <b>{{ t(`reqSync.action.${x.action}`) }}</b>
              <span class="mono">{{ x.requirement_key }}</span>
              <span v-if="x.external_id" class="mono">↔ {{ x.external_id }}</span>
              <span>{{ x.title }}</span>
              <span v-if="x.fields?.length" class="muted">({{ x.fields.map(fieldLabel).join(', ') }})</span>
              <span v-if="x.message" class="muted">{{ x.message }}</span>
            </li>
          </ul>
          <p v-else class="small muted">{{ t('reqSync.nothing') }}</p>
        </div>
      </article>
    </div>
  </section>

  <!-- conflicts -->
  <section v-if="tab === 'conflicts'" v-bind="panel('conflicts')" class="sync-conflicts" style="margin-top: 12px">
    <h2 class="sr-only">{{ t('reqSync.conflicts') }}</h2>
    <p class="small muted" style="max-width: 860px">{{ t('reqSync.conflictsIntro') }}</p>
    <p v-if="!conflicts.length" class="empty-state">{{ t('reqSync.noConflicts') }}</p>
    <article v-for="c in conflicts" :key="c.id" class="card sync-conflict" :aria-labelledby="`${id}-c-${c.id}`">
      <h3 :id="`${id}-c-${c.id}`" class="plain-heading" style="margin: 0 0 4px">
        <RouterLink :to="{ name: 'requirements', query: { item: c.requirement_id } }" class="mono">{{ c.requirement_key }}</RouterLink>
        {{ c.title }}
        <span class="muted">↔ {{ sourceName(c.system) }}</span>{{ ' ' }}
        <a v-if="c.external_url" :href="c.external_url" target="_blank" rel="noopener noreferrer" class="mono">{{ c.external_id }}</a>
        <span v-else class="mono">{{ c.external_id }}</span>
      </h3>
      <p class="small muted" style="margin: 0 0 8px">{{ t('reqSync.conflictSince', { time: formatDateTime(c.updated_at) }) }}</p>
      <div class="table-scroll">
        <table class="sync-diff">
          <caption class="sr-only">{{ t('reqSync.diffCaption', { key: c.requirement_key }) }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('reqSync.col.field') }}</th>
              <th scope="col">{{ t('reqSync.col.base') }}</th>
              <th scope="col">{{ t('reqSync.col.local') }}</th>
              <th scope="col">{{ t('reqSync.col.remote', { source: sourceName(c.system) }) }}</th>
              <th v-if="canRun" scope="col">{{ t('reqSync.col.decision') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="f in c.fields" :key="f.field">
              <th scope="row">{{ fieldLabel(f.field) }}</th>
              <td class="diff-base">{{ f.base || '—' }}</td>
              <td class="diff-local">{{ f.local || '—' }}</td>
              <td class="diff-remote">{{ f.remote || '—' }}</td>
              <td v-if="canRun">
                <fieldset class="plain-fieldset row nowrap">
                  <legend class="sr-only">{{ t('reqSync.decisionFor', { field: fieldLabel(f.field) }) }}</legend>
                  <span v-for="d in ['qtestframe', 'tracker', 'dismiss']" :key="d" class="row nowrap">
                    <RadioButton v-model="choices[c.id][f.field]" :input-id="`${id}-${c.id}-${f.field}-${d}`" :name="`${id}-${c.id}-${f.field}`" :value="d" />
                    <label :for="`${id}-${c.id}-${f.field}-${d}`" class="small">{{ t(`reqSync.take.${d}`, { source: sourceName(c.system) }) }}</label>
                  </span>
                </fieldset>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="canRun" class="row" style="margin-top: 8px">
        <Button :label="t('reqSync.applyDecisions')" icon="pi pi-check" size="small" :disabled="!fullyChosen(c, choices[c.id] ?? {})" :loading="resolving === c.id" @click="resolve(c)" />
        <Button :label="t('reqSync.allLocal')" size="small" severity="secondary" outlined :disabled="!!resolving" @click="resolve(c, 'qtestframe')" />
        <Button :label="t('reqSync.allRemote', { source: sourceName(c.system) })" size="small" severity="secondary" outlined :disabled="!!resolving" @click="resolve(c, 'tracker')" />
        <Button :label="t('reqSync.dismiss')" size="small" severity="secondary" text :disabled="!!resolving" @click="resolve(c, 'dismiss')" />
      </div>
    </article>
  </section>

  <!-- log -->
  <section v-if="tab === 'log'" v-bind="panel('log')" class="sync-log" style="margin-top: 12px">
    <h2 class="sr-only">{{ t('reqSync.log') }}</h2>
    <p v-if="!runs.length" class="empty-state">{{ t('reqSync.noRuns') }}</p>
    <div v-else class="table-scroll card">
      <table class="sync-runs">
        <caption class="sr-only">{{ t('reqSync.log') }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ t('reqSync.col.time') }}</th>
            <th scope="col">{{ t('reqSync.col.source') }}</th>
            <th scope="col">{{ t('reqSync.col.trigger') }}</th>
            <th scope="col">{{ t('reqSync.col.status') }}</th>
            <th scope="col">{{ t('reqSync.col.summary') }}</th>
            <th scope="col"><span class="sr-only">{{ t('reqSync.col.details') }}</span></th>
          </tr>
        </thead>
        <tbody>
          <template v-for="r in runs" :key="r.id">
            <tr>
              <td><time :datetime="r.started_at">{{ formatDateTime(r.started_at) }}</time></td>
              <td>{{ sourceName(r.system) }}</td>
              <td>{{ t(`reqSync.trigger.${r.trigger}`) }} <span class="muted small">{{ r.started_by }}</span></td>
              <td><Tag :value="t(`reqSync.status.${r.status}`)" :severity="runSeverity(r.status)" /></td>
              <td class="small">{{ r.error || summaryText(r) }}</td>
              <td>
                <Button
                  icon="pi pi-list"
                  text
                  size="small"
                  :aria-label="t('reqSync.showEntries')"
                  :aria-expanded="openRun === r.id"
                  :aria-controls="`${id}-run-${r.id}`"
                  v-tooltip.left="t('reqSync.showEntries')"
                  @click="toggleRun(r)"
                />
              </td>
            </tr>
            <tr v-if="openRun === r.id" :id="`${id}-run-${r.id}`">
              <td colspan="6">
                <ul v-if="runEntries[r.id]?.length" class="sync-entries small">
                  <li v-for="(x, i) in runEntries[r.id]" :key="i">
                    <i :class="ENTRY_ICONS[x.action] ?? 'pi pi-circle'" aria-hidden="true" />
                    <b>{{ t(`reqSync.action.${x.action}`) }}</b>
                    <span class="mono">{{ x.requirement_key }}</span>
                    <span v-if="x.external_id" class="mono">↔ {{ x.external_id }}</span>
                    <span>{{ x.title }}</span>
                    <span v-for="(v, k) in x.changes ?? {}" :key="k" class="muted">{{ fieldLabel(String(k)) }}: {{ v }}</span>
                    <span v-if="x.message" class="muted">{{ x.message }}</span>
                  </li>
                </ul>
                <p v-else class="small muted">{{ t('reqSync.nothing') }}</p>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  </section>

  <!-- settings -->
  <section v-if="tab === 'settings' && canConfigure" v-bind="panel('settings')" class="sync-settings" style="margin-top: 12px">
    <h2 :id="`${id}-h-settings`" class="sr-only">{{ t('reqSync.settings') }}</h2>
    <div class="field" style="max-width: 420px">
      <span :id="`${id}-src-label`" class="field-label">{{ t('reqSync.source') }}</span>
      <SelectButton v-model="source" :options="sourceOptions" option-label="label" option-value="value" :allow-empty="false" :aria-labelledby="`${id}-src-label`" />
    </div>
    <Message v-if="current && !current.connected" severity="warn" :closable="false" style="max-width: 860px">
      {{ t('reqSync.notConnected') }} <RouterLink :to="{ name: 'settings', query: { tab: 'project' } }">{{ t('reqSync.toImportSources') }}</RouterLink>
    </Message>
    <form v-if="current" class="card grid sync-form" style="gap: 16px; max-width: 860px; margin-top: 12px" :aria-labelledby="`${id}-h-settings`" @submit.prevent="save(false)">
      <div class="row nowrap">
        <Checkbox v-model="form.enabled" :input-id="`${id}-enabled`" binary />
        <label :for="`${id}-enabled`">{{ t('reqSync.enabled', { source: sourceName(source) }) }}</label>
      </div>
      <div class="form-grid">
        <div class="field">
          <label :for="`${id}-interval`">{{ t('reqSync.interval') }}</label>
          <InputNumber v-model="form.interval" :input-id="`${id}-interval`" :min="0" :max="10080" show-buttons :invalid="!!problem('interval')" :aria-describedby="`${id}-interval-hint`" />
          <span :id="`${id}-interval-hint`" class="small" :class="problem('interval') ? 'warn-text' : 'muted'">{{ problem('interval') || t('reqSync.intervalHint') }}</span>
        </div>
        <div class="field">
          <label :for="`${id}-writes`">{{ t('reqSync.maxWrites') }}</label>
          <InputNumber v-model="form.maxWrites" :input-id="`${id}-writes`" :min="1" :max="600" show-buttons :invalid="!!problem('maxWrites')" :aria-describedby="`${id}-writes-hint`" />
          <span :id="`${id}-writes-hint`" class="small" :class="problem('maxWrites') ? 'warn-text' : 'muted'">{{ problem('maxWrites') || t('reqSync.maxWritesHint') }}</span>
        </div>
      </div>

      <fieldset class="plain-fieldset grid" style="gap: 12px">
        <legend class="field-label">{{ t('reqSync.fromTracker', { source: sourceName(source) }) }}</legend>
        <div class="field">
          <label :for="`${id}-query`">{{ isJira ? t('reqSync.jql') : t('reqSync.wiql') }}</label>
          <InputText :id="`${id}-query`" v-model="form.query" class="mono" :placeholder="isJira ? 'project = SHOP AND issuetype = Story' : 'SELECT [System.Id] FROM WorkItems WHERE …'" :aria-describedby="`${id}-query-hint`" />
          <span :id="`${id}-query-hint`" class="small muted">{{ t('reqSync.queryHint') }}</span>
        </div>
        <div class="row nowrap">
          <Checkbox v-model="form.importNew" :input-id="`${id}-import`" binary />
          <label :for="`${id}-import`">{{ t('reqSync.importNew') }}</label>
        </div>
        <div v-if="form.importNew" class="field" style="max-width: 420px">
          <label :id="`${id}-folder-label`" :for="`${id}-folder`">{{ t('reqSync.folder') }}</label>
          <Select v-model="form.folderId" :input-id="`${id}-folder`" :aria-labelledby="`${id}-folder-label`" :options="folderOptions" option-label="label" option-value="value" :placeholder="t('reqSync.noFolder')" filter />
        </div>
      </fieldset>

      <fieldset class="plain-fieldset grid" style="gap: 12px">
        <legend class="field-label">{{ t('reqSync.toTracker', { source: sourceName(source) }) }}</legend>
        <div class="row nowrap">
          <Checkbox v-model="form.pushCreate" :input-id="`${id}-push`" binary />
          <label :for="`${id}-push`">{{ t('reqSync.pushCreate', { source: sourceName(source) }) }}</label>
        </div>
        <template v-if="form.pushCreate">
          <p class="small" :class="problem('pushScope') ? 'warn-text' : 'muted'" style="margin: 0" :id="`${id}-push-hint`">{{ problem('pushScope') || t('reqSync.pushScopeHint') }}</p>
          <div class="form-grid">
            <div class="field">
              <label :id="`${id}-pfolder-label`" :for="`${id}-pfolder`">{{ t('reqSync.pushFolder') }}</label>
              <Select v-model="form.pushFolderId" :input-id="`${id}-pfolder`" :aria-labelledby="`${id}-pfolder-label`" :options="folderOptions" option-label="label" option-value="value" :placeholder="t('reqSync.noFolder')" filter />
            </div>
            <div class="field">
              <label :for="`${id}-ptag`">{{ t('reqSync.pushTag') }}</label>
              <InputText :id="`${id}-ptag`" v-model="form.pushTag" placeholder="jira" :aria-describedby="`${id}-push-hint`" />
            </div>
          </div>
          <div class="field">
            <label :for="`${id}-pquery`">{{ t('reqSync.pushQuery') }}</label>
            <InputText :id="`${id}-pquery`" v-model="form.pushQuery" class="mono" placeholder="status = approved" :aria-describedby="`${id}-push-hint`" />
          </div>
          <div class="form-grid">
            <div class="field">
              <label :for="`${id}-ptype`">{{ isJira ? t('reqSync.issueType') : t('reqSync.workItemType') }}</label>
              <InputText :id="`${id}-ptype`" v-model="form.itemType" :placeholder="isJira ? 'Story' : 'User Story'" />
            </div>
            <div v-if="isJira" class="field">
              <label :for="`${id}-pkey`">{{ t('reqSync.projectKey') }}</label>
              <InputText :id="`${id}-pkey`" v-model="form.projectKey" class="mono" placeholder="SHOP" :aria-describedby="`${id}-pkey-hint`" />
              <span :id="`${id}-pkey-hint`" class="small muted">{{ t('reqSync.projectKeyHint') }}</span>
            </div>
          </div>
        </template>
      </fieldset>

      <fieldset class="plain-fieldset grid" style="gap: 12px">
        <legend class="field-label">{{ t('reqSync.fields') }}</legend>
        <p class="small muted" style="margin: 0">{{ t('reqSync.fieldsIntro') }}</p>
        <div class="table-scroll">
          <table class="sync-fields">
            <caption class="sr-only">{{ t('reqSync.fields') }}</caption>
            <thead>
              <tr>
                <th scope="col">{{ t('reqSync.col.qtfField') }}</th>
                <th scope="col">{{ t('reqSync.col.trackerField', { source: sourceName(source) }) }}</th>
                <th scope="col">{{ t('reqSync.col.direction') }}</th>
                <th scope="col"><span class="sr-only">{{ t('reqSync.col.actions') }}</span></th>
              </tr>
            </thead>
            <tbody>
              <template v-for="(f, i) in form.fields" :key="i">
                <tr>
                  <td>
                    <Select v-model="f.field" :options="fieldOptions" option-label="label" option-value="value" :aria-label="t('reqSync.col.qtfField')" :invalid="!!problem(`field${i}`)" />
                  </td>
                  <td>
                    <InputText v-model="f.remote" class="mono" :aria-label="t('reqSync.col.trackerField', { source: sourceName(source) })" :invalid="!!problem(`remote${i}`)" />
                  </td>
                  <td>
                    <Select v-model="f.direction" :options="directionOptions" option-label="label" option-value="value" :aria-label="t('reqSync.col.direction')" />
                  </td>
                  <td>
                    <Button icon="pi pi-trash" text severity="danger" size="small" :aria-label="t('reqSync.removeField', { field: fieldLabel(f.field) })" @click="form.fields.splice(i, 1)" />
                  </td>
                </tr>
                <tr v-if="MAPPED_VALUE_FIELDS.includes(f.field)" class="value-maps">
                  <td colspan="4">
                    <div class="form-grid">
                      <div class="field">
                        <label :for="`${id}-vals-${i}`">{{ t('reqSync.valuesPull', { field: fieldLabel(f.field), source: sourceName(source) }) }}</label>
                        <Textarea :id="`${id}-vals-${i}`" v-model="f.values" rows="4" class="mono" auto-resize :aria-describedby="`${id}-vals-hint`" />
                      </div>
                      <div class="field">
                        <label :for="`${id}-pvals-${i}`">{{ t('reqSync.valuesPush', { field: fieldLabel(f.field), source: sourceName(source) }) }}</label>
                        <Textarea :id="`${id}-pvals-${i}`" v-model="f.pushValues" rows="4" class="mono" auto-resize :aria-describedby="`${id}-vals-hint`" />
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
        <span :id="`${id}-vals-hint`" class="small muted">{{ t('reqSync.valuesHint') }}</span>
        <span v-if="Object.keys(problems).some((k) => k.startsWith('remote') || k.startsWith('field')) && touched" class="small warn-text" role="alert">{{ t('reqSync.invalid.fieldRows') }}</span>
        <div class="row">
          <Button :label="t('reqSync.addField')" icon="pi pi-plus" size="small" severity="secondary" outlined :disabled="!nextField(form)" @click="addField" />
        </div>
      </fieldset>

      <fieldset class="plain-fieldset grid" style="gap: 12px">
        <legend class="field-label">{{ t('reqSync.changes') }}</legend>
        <div class="form-grid">
          <div class="field">
            <label :id="`${id}-policy-label`" :for="`${id}-policy`">{{ t('reqSync.conflictPolicy') }}</label>
            <Select v-model="form.policy" :input-id="`${id}-policy`" :aria-labelledby="`${id}-policy-label`" :options="policyOptions" option-label="label" option-value="value" />
          </div>
          <div class="field">
            <label :id="`${id}-impact-label`" :for="`${id}-impact`">{{ t('reqSync.impactChoice') }}</label>
            <Select v-model="form.impact" :input-id="`${id}-impact`" :aria-labelledby="`${id}-impact-label`" :options="impactOptions" option-label="label" option-value="value" />
          </div>
        </div>
        <div class="row">
          <span class="row nowrap"><Checkbox v-model="form.comments" :input-id="`${id}-comments`" binary /><label :for="`${id}-comments`">{{ t('reqSync.comments') }}</label></span>
          <span class="row nowrap"><Checkbox v-model="form.links" :input-id="`${id}-links`" binary /><label :for="`${id}-links`">{{ t('reqSync.links') }}</label></span>
        </div>
      </fieldset>

      <fieldset class="plain-fieldset grid" style="gap: 12px">
        <legend class="field-label">{{ t('reqSync.webhook') }}</legend>
        <p class="small muted" style="margin: 0">{{ isJira ? t('reqSync.webhookJira') : t('reqSync.webhookAzure') }}</p>
        <div class="field">
          <label :for="`${id}-hookurl`">{{ t('reqSync.webhookUrl') }}</label>
          <div class="row nowrap">
            <InputText :id="`${id}-hookurl`" :model-value="current.webhook_url" class="mono grow" readonly />
            <Button icon="pi pi-copy" severity="secondary" text :aria-label="t('reqSync.copyUrl')" @click="copy(current.webhook_url)" />
          </div>
        </div>
        <div class="field">
          <label :for="`${id}-secret`">{{ t('reqSync.secret') }}</label>
          <div class="row nowrap">
            <Password
              v-model="form.secret"
              :input-id="`${id}-secret`"
              :feedback="false"
              toggle-mask
              fluid
              autocomplete="new-password"
              :placeholder="current.has_webhook_secret ? t('reqSync.secretKept') : ''"
              :invalid="!!problem('secret')"
              :input-props="{ 'aria-describedby': `${id}-secret-hint` }"
            />
            <Button :label="t('reqSync.generate')" icon="pi pi-key" severity="secondary" outlined size="small" @click="generateSecret" />
          </div>
          <span :id="`${id}-secret-hint`" class="small" :class="problem('secret') ? 'warn-text' : 'muted'">{{ problem('secret') || t('reqSync.secretHint') }}</span>
          <span v-if="current.has_webhook_secret" class="row small">
            <Tag :value="t('reqSync.secretSaved')" severity="success" icon="pi pi-lock" />
            <Button :label="t('reqSync.removeSecret')" text size="small" severity="danger" :loading="saving" @click="save(true)" />
          </span>
        </div>
      </fieldset>

      <p v-if="current.run_as" class="small muted" style="margin: 0">{{ t('reqSync.runAsHint', { user: current.run_as }) }}</p>
      <div class="row">
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" />
      </div>
    </form>
  </section>
</template>

<style scoped>
.sync-sources {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 12px;
}
.sync-facts {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 12px;
  margin: 8px 0;
}
.sync-facts dt {
  color: var(--p-text-muted-color);
}
.sync-facts dd {
  margin: 0;
}
.sync-entries {
  list-style: none;
  padding: 0;
  margin: 6px 0 0;
  display: grid;
  gap: 4px;
}
.sync-entries li {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: baseline;
}
.sync-conflict {
  margin-bottom: 12px;
  max-width: 1100px;
}
.sync-diff,
.sync-runs,
.sync-fields {
  width: 100%;
  border-collapse: collapse;
}
.sync-diff th,
.sync-diff td,
.sync-runs th,
.sync-runs td,
.sync-fields th,
.sync-fields td {
  text-align: left;
  vertical-align: top;
  padding: 6px 8px;
  border-bottom: 1px solid var(--p-content-border-color);
}
.sync-diff td {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-width: 320px;
}
.sync-error {
  overflow-wrap: anywhere;
}
.value-maps td {
  border-bottom-style: dashed;
}
</style>
