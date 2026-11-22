<script setup lang="ts">
// Code and builds: the CI builds whose results were uploaded, the commits and
// merge / pull requests that name requirements and tests, and the regression
// selection — the tests to run after changes, with a run made from them.
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import Tag from 'primevue/tag'
import ItemRef from '@/components/ItemRef.vue'
import StatusTag from '@/components/StatusTag.vue'
import VariantSelect from '@/components/VariantSelect.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { useRoute, useRouter } from 'vue-router'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import { buildLabel, changeRef, passRate, splitKeys, type Build, type CodeChange, type RegressionReason, type RegressionSelection } from '@/lib/variants'
import type { TestRun } from '@/lib/types'
import { useVariants } from '@/composables/variants'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { uid } from '@/lib/a11y'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const id = uid('codev')
const pid = computed(() => proj.pid)
const vs = useVariants(() => pid.value)
const canRunWrite = computed(() => auth.can(P.runWrite))
const canRead = computed(() => auth.can(P.runRead))

const tabs = ['regression', 'builds', 'changes'] as const
const tab = ref<string>(tabs.includes(route.query.tab as never) ? (route.query.tab as string) : 'regression')
watch(tab, (v) => router.replace({ query: { ...route.query, tab: v === 'regression' ? undefined : v } }))

// ---------- builds ----------
const builds = ref<Build[]>([])
async function loadBuilds() {
  if (!canRead.value) return
  try {
    builds.value = asItems(await api.get<{ items: Build[] }>(`/projects/${pid.value}/builds`))
  } catch (e) {
    notify.fail(e)
  }
}
async function removeBuild(b: Build) {
  if (!window.confirm(t('code.confirmDeleteBuild', { name: buildLabel(b) }))) return
  try {
    await api.del(`/builds/${b.id}`)
    notify.ok(t('code.buildDeleted'), buildLabel(b))
    await loadBuilds()
  } catch (e) {
    notify.fail(e)
  }
}
const buildOptions = computed(() => builds.value.map((b) => ({ label: `${buildLabel(b)}${b.branch ? ' · ' + b.branch : ''} · ${formatDateTime(b.created_at)}`, value: b.id })))

// ---------- changes ----------
const changes = ref<CodeChange[]>([])
const kind = ref<string | null>(null)
const kindOptions = computed(() => ['commit', 'merge_request'].map((k) => ({ label: t(`code.kind.${k}`), value: k })))
async function loadChanges() {
  try {
    changes.value = asItems(await api.get<{ items: CodeChange[] }>(`/projects/${pid.value}/code-changes`, { kind: kind.value || undefined }))
  } catch (e) {
    notify.fail(e)
  }
}
watch(kind, loadChanges)

// ---------- regression ----------
const reqKeys = ref((route.query.requirements as string) || '')
const sinceBuild = ref<string | null>(null)
const variant = ref<string | null>(null)
const related = ref(false)
const prerequisites = ref(true)
const selection = ref<RegressionSelection | null>(null)
const selError = ref('')
const busy = ref(false)
const runName = ref('')
const creating = ref(false)
const request = () => ({
  requirement_ids: splitKeys(reqKeys.value),
  since_build: sinceBuild.value || undefined,
  variant: variant.value || undefined,
  include_related: related.value,
  include_prerequisites: prerequisites.value,
})
const canPropose = computed(() => splitKeys(reqKeys.value).length > 0 || !!sinceBuild.value)
async function propose() {
  if (!canPropose.value) return
  busy.value = true
  selError.value = ''
  try {
    selection.value = await api.post<RegressionSelection>(`/projects/${pid.value}/regression`, request())
  } catch (e) {
    selError.value = errorMessage(e)
    selection.value = null
  } finally {
    busy.value = false
  }
}
async function createRun() {
  creating.value = true
  try {
    const res = await api.post<{ run: TestRun }>(`/projects/${pid.value}/regression/run`, { ...request(), name: runName.value.trim() || undefined })
    notify.ok(t('code.runCreated'), res.run.name)
    await router.push({ name: 'run', params: { projectId: pid.value, runId: res.run.id } })
  } catch (e) {
    notify.fail(e)
  } finally {
    creating.value = false
  }
}
function reasonText(r: RegressionReason) {
  return t(`code.reason.${r.kind}`, { ref: r.ref ?? '', type: r.type ? t(`code.reasonType.${r.type}`) : '' })
}

onMounted(async () => {
  await Promise.all([vs.load(), loadBuilds(), loadChanges()])
  if (reqKeys.value) await propose()
})
watch(pid, () => void Promise.all([vs.load(), loadBuilds(), loadChanges()]))
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('code.title') }}</h1>
      <div class="sub">{{ t('code.subtitle') }}</div>
    </div>
    <div class="actions"><HelpLink page="variants-and-code" :anchor="{ en: 'code-links', sv: 'kodlänkar' }" /></div>
  </div>

  <Tabs v-model:value="tab" class="editor-tabs">
    <TabList>
      <Tab value="regression"><i class="pi pi-replay" aria-hidden="true" /> {{ t('code.regression') }}</Tab>
      <Tab v-if="canRead" value="builds"><i class="pi pi-box" aria-hidden="true" /> {{ t('code.builds') }} <span class="count">{{ builds.length }}</span></Tab>
      <Tab value="changes"><i class="pi pi-code" aria-hidden="true" /> {{ t('code.changes') }} <span class="count">{{ changes.length }}</span></Tab>
    </TabList>
  </Tabs>

  <section v-if="tab === 'regression'" aria-labelledby="h-regression" style="margin-top: 12px">
    <h2 id="h-regression" class="sr-only">{{ t('code.regression') }}</h2>
    <form class="card grid" style="gap: 12px; max-width: 860px" @submit.prevent="propose">
      <p class="small muted" style="margin: 0">{{ t('code.regressionIntro') }}</p>
      <div class="field">
        <label :for="`${id}-reqs`">{{ t('code.changedRequirements') }}</label>
        <InputText :id="`${id}-reqs`" v-model="reqKeys" class="mono" :placeholder="`${proj.project?.key ?? 'P'}-REQ-1, ${proj.project?.key ?? 'P'}-REQ-4`" :aria-describedby="`${id}-reqs-help`" autocomplete="off" />
        <small :id="`${id}-reqs-help`" class="muted">{{ t('code.changedRequirementsHelp') }}</small>
      </div>
      <div class="form-grid">
        <div v-if="canRead" class="field">
          <label :for="`${id}-since`">{{ t('code.sinceBuild') }}</label>
          <Select v-model="sinceBuild" :input-id="`${id}-since`" :options="buildOptions" option-label="label" option-value="value" :placeholder="t('code.noBuild')" show-clear filter />
        </div>
        <div class="field">
          <label :for="`${id}-variant`">{{ t('variants.variant') }}</label>
          <VariantSelect v-model="variant" :input-id="`${id}-variant`" :variants="vs.variants.value" />
        </div>
      </div>
      <div class="row">
        <span class="row nowrap"><Checkbox v-model="related" binary :input-id="`${id}-rel`" /><label :for="`${id}-rel`">{{ t('code.includeRelated') }}</label></span>
        <span class="row nowrap"><Checkbox v-model="prerequisites" binary :input-id="`${id}-pre`" /><label :for="`${id}-pre`">{{ t('code.includePrerequisites') }}</label></span>
        <span class="grow" />
        <Button type="submit" :label="t('code.propose')" icon="pi pi-search" :loading="busy" :disabled="!canPropose" />
      </div>
      <Message v-if="selError" severity="error" :closable="false">{{ selError }}</Message>
    </form>

    <div v-if="selection" class="card" style="margin-top: 12px" aria-live="polite">
      <div class="row" style="justify-content: space-between">
        <h3 class="plain-heading" style="margin: 0">{{ t('code.proposedTests', { n: selection.tests.length }, selection.tests.length) }}</h3>
        <form v-if="canRunWrite && selection.tests.length" class="row nowrap" @submit.prevent="createRun">
          <label :for="`${id}-runname`" class="sr-only">{{ t('code.runName') }}</label>
          <InputText :id="`${id}-runname`" v-model="runName" :placeholder="t('code.runName')" maxlength="200" />
          <Button type="submit" :label="t('code.createRun')" icon="pi pi-play" :loading="creating" />
        </form>
      </div>
      <p class="small muted">
        {{ t('code.selectionSummary', { reqs: selection.requirements.length, changes: selection.changes.length }) }}
        <template v-if="selection.variant"> · {{ t('variants.variant') }}: {{ selection.variant.name }}</template>
      </p>
      <p v-if="!selection.tests.length" class="small muted">{{ t('code.noTests') }}</p>
      <div v-else class="table-scroll">
        <table class="code-table">
          <caption class="sr-only">{{ t('code.proposedCaption') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('nav.tests') }}</th>
              <th scope="col">{{ t('code.lastResult') }}</th>
              <th scope="col">{{ t('code.why') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="x in selection.tests" :key="x.id">
              <th scope="row"><ItemRef :to="{ name: 'tests', params: { projectId: pid }, query: { item: x.id } }" :item-key="x.key" :title="x.title" title-class="small muted ellipsis" /></th>
              <td><StatusTag :value="x.last_result" kind="result" /></td>
              <td class="small wrap">{{ x.reasons.map(reasonText).join('; ') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <details v-if="selection.requirements.length" style="margin-top: 10px">
        <summary class="small">{{ t('code.affectedRequirements', { n: selection.requirements.length }, selection.requirements.length) }}</summary>
        <ul class="plain-list">
          <li v-for="x in selection.requirements" :key="x.id" class="row nowrap small">
            <ItemRef :to="{ name: 'requirements', params: { projectId: pid }, query: { item: x.id } }" :item-key="x.key" :title="x.title" title-class="muted ellipsis" grow />
            <span class="muted">{{ x.reasons.map(reasonText).join('; ') }}</span>
          </li>
        </ul>
      </details>
    </div>
  </section>

  <section v-else-if="tab === 'builds'" aria-labelledby="h-builds" style="margin-top: 12px">
    <h2 id="h-builds" class="sr-only">{{ t('code.builds') }}</h2>
    <p v-if="!builds.length" class="card small muted">{{ t('code.noBuildsYet') }}</p>
    <div v-else class="table-scroll card" style="padding: 0">
      <table class="code-table">
        <caption class="sr-only">{{ t('code.builds') }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ t('code.build') }}</th>
            <th scope="col">{{ t('code.branch') }}</th>
            <th scope="col">{{ t('code.commit') }}</th>
            <th scope="col">{{ t('common.created') }}</th>
            <th scope="col">{{ t('code.runs') }}</th>
            <th scope="col">{{ t('code.passRate') }}</th>
            <th v-if="canRunWrite" scope="col"><span class="sr-only">{{ t('common.actions') }}</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="b in builds" :key="b.id">
            <th scope="row" class="mono">
              <a v-if="b.pipeline_url" :href="b.pipeline_url" target="_blank" rel="noopener noreferrer">{{ buildLabel(b) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
              <template v-else>{{ buildLabel(b) }}</template>
            </th>
            <td>{{ b.branch || '—' }}</td>
            <td class="mono">{{ b.commit_sha ? b.commit_sha.slice(0, 8) : '—' }}</td>
            <td>{{ formatDateTime(b.created_at) }}</td>
            <td>{{ b.runs }}</td>
            <td>
              <template v-if="passRate(b.summary) !== null">{{ passRate(b.summary) }} %</template><span v-else class="muted">—</span>
              <span v-if="b.summary?.failed" class="small" style="margin-inline-start: 6px"><StatusTag value="failed" kind="result" /> {{ b.summary.failed }}</span>
            </td>
            <td v-if="canRunWrite">
              <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('code.deleteBuild', { name: buildLabel(b) })" @click="removeBuild(b)" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <section v-else aria-labelledby="h-changes" style="margin-top: 12px">
    <h2 id="h-changes" class="sr-only">{{ t('code.changes') }}</h2>
    <div class="filterbar">
      <label :for="`${id}-kind`" class="small">{{ t('code.kindLabel') }}</label>
      <Select v-model="kind" :input-id="`${id}-kind`" :options="kindOptions" option-label="label" option-value="value" :placeholder="t('code.allKinds')" show-clear style="min-width: 180px" />
    </div>
    <p v-if="!changes.length" class="card small muted">{{ t('code.noChangesYet') }}</p>
    <ul v-else class="plain-list card" style="padding: 0 12px">
      <li v-for="c in changes" :key="c.id" class="change">
        <div class="row nowrap">
          <i :class="c.kind === 'commit' ? 'pi pi-code' : 'pi pi-arrow-right-arrow-left'" aria-hidden="true" class="muted" />
          <span class="sr-only">{{ t(`code.kind.${c.kind}`) }}:</span>
          <a v-if="c.url" :href="c.url" target="_blank" rel="noopener noreferrer" class="mono">{{ changeRef(c) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
          <span v-else class="mono">{{ changeRef(c) }}</span>
          <span class="grow ellipsis" :title="c.title">{{ c.title }}</span>
          <Tag v-if="c.kind === 'merge_request'" :value="t(`code.status.${c.status || 'open'}`)" :severity="c.status === 'merged' ? 'success' : c.status === 'closed' ? 'secondary' : 'info'" />
        </div>
        <div class="small muted meta">
          <span v-if="c.repository">{{ c.repository }}</span>
          <span v-if="c.branch">{{ c.branch }}</span>
          <span v-if="c.author">{{ c.author }}</span>
          <span>{{ formatDateTime(c.occurred_at) }}</span>
          <span v-for="b in c.builds ?? []" :key="b.id"><i class="pi pi-box" aria-hidden="true" /> {{ t('code.buildN', { n: buildLabel(b) }) }}</span>
        </div>
        <div v-if="c.items.length" class="row small meta">
          <ItemRef
            v-for="it in c.items"
            :key="it.item_type + it.item_id"
            :to="{ name: it.item_type === 'test' ? 'tests' : 'requirements', params: { projectId: pid }, query: { item: it.item_id } }"
            :item-key="it.key"
          />
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.table-scroll {
  overflow-x: auto;
}
.code-table {
  border-collapse: collapse;
  width: 100%;
}
.code-table th,
.code-table td {
  text-align: left;
  padding: 6px 10px;
  border-bottom: 1px solid var(--qtf-border);
  white-space: nowrap;
}
.code-table thead th {
  font-size: 12px;
  font-weight: 600;
}
.wrap {
  white-space: normal !important;
}
.change {
  padding: 8px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.change:last-child {
  border-bottom: 0;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 2px;
  padding-inline-start: 22px;
}
</style>
