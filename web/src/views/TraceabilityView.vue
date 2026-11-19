<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import MultiSelect from 'primevue/multiselect'
import Button from 'primevue/button'
import ToggleSwitch from 'primevue/toggleswitch'
import ProgressSpinner from 'primevue/progressspinner'
import StatusTag from '@/components/StatusTag.vue'
import { api } from '@/lib/api'
import { linkKey } from '@/lib/collections'
import { COVERAGE_COLORS } from '@/lib/colors'
import { percent } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { TraceLink, Traceability } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'

const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()
const coverageOptions = computed(() => enumOptions('coverage', ['none', 'not_run', 'passed', 'failed', 'partial']))

const data = ref<Traceability | null>(null)
const loading = ref(false)
const reqFilter = ref('')
const testFilter = ref('')
const coverage = ref<string[]>([])
const onlyLinkedTests = ref(false)
const busy = ref<string | null>(null)
const canEdit = computed(() => auth.can(P.requirementWrite) || auth.can(P.testWrite))

async function load() {
  loading.value = true
  try {
    data.value = await api.get<Traceability>(`/projects/${proj.pid}/traceability`)
  } catch (e) {
    notify.fail(e, t('traceability.loadFailed'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

const linkMap = computed(() => {
  const m = new Map<string, TraceLink>()
  for (const l of data.value?.links ?? []) if (!m.has(linkKey(l.requirement_id, l.test_case_id))) m.set(linkKey(l.requirement_id, l.test_case_id), l)
  return m
})
const linkedTestIds = computed(() => new Set((data.value?.links ?? []).map((l) => l.test_case_id)))
const reqCounts = computed(() => {
  const m = new Map<string, number>()
  for (const l of data.value?.links ?? []) m.set(l.requirement_id, (m.get(l.requirement_id) ?? 0) + 1)
  return m
})

function matches(text: string, f: string) {
  return !f || text.toLowerCase().includes(f.toLowerCase())
}
const rows = computed(() =>
  (data.value?.requirements ?? []).filter(
    (r) => matches(`${r.key} ${r.title}`, reqFilter.value.trim()) && (!coverage.value.length || coverage.value.includes(r.coverage || 'none')),
  ),
)
const cols = computed(() =>
  (data.value?.tests ?? []).filter((t) => matches(`${t.key} ${t.title}`, testFilter.value.trim()) && (!onlyLinkedTests.value || linkedTestIds.value.has(t.id))),
)
const coveredCount = computed(() => (data.value?.requirements ?? []).filter((r) => (reqCounts.value.get(r.id) ?? 0) > 0).length)

async function toggle(reqId: string, testId: string) {
  if (!canEdit.value || !data.value) return
  const key = linkKey(reqId, testId)
  if (busy.value) return
  busy.value = key
  const existing = linkMap.value.get(key)
  try {
    if (existing) {
      await api.del(`/links/${existing.id}`)
      data.value.links = data.value.links.filter((l) => l.id !== existing.id)
    } else {
      const l = await api.post<TraceLink>(`/projects/${proj.pid}/links`, { requirement_id: reqId, test_case_id: testId })
      data.value.links = [...data.value.links, l ?? ({ id: '', requirement_id: reqId, test_case_id: testId } as TraceLink)]
      if (!l?.id) await load()
    }
  } catch (e) {
    notify.fail(e, t('traceability.linkFailed'))
  } finally {
    busy.value = null
  }
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('traceability.title') }}</h1>
      <div class="sub" v-if="data">
        {{ t('traceability.linked', { covered: coveredCount, total: data.requirements.length, pct: percent(coveredCount, data.requirements.length) }) }} ·
        {{ t('counts.links', data.links.length) }}
        <template v-if="canEdit">{{ t('traceability.toggleHint') }}</template>
      </div>
    </div>
    <div class="actions">
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
    </div>
  </div>
  <div class="filterbar">
    <IconField>
      <InputIcon class="pi pi-filter" />
      <InputText v-model="reqFilter" :placeholder="t('traceability.filterRequirements')" :aria-label="t('traceability.filterRequirements')" />
    </IconField>
    <IconField>
      <InputIcon class="pi pi-filter" />
      <InputText v-model="testFilter" :placeholder="t('traceability.filterTests')" :aria-label="t('traceability.filterTests')" />
    </IconField>
    <MultiSelect v-model="coverage" :options="coverageOptions" option-label="label" option-value="value" :placeholder="t('traceability.coverage')" style="min-width: 150px" show-clear />
    <span class="row small"><ToggleSwitch v-model="onlyLinkedTests" input-id="olt" /><label for="olt">{{ t('traceability.onlyLinked') }}</label></span>
    <span class="row small muted" style="margin-left: auto">
      <span v-for="(c, k) in COVERAGE_COLORS" :key="k" class="row nowrap" style="gap: 4px"><span :style="{ width: '10px', height: '10px', borderRadius: '3px', background: c }" />{{ enumLabel('coverage', String(k)) }}</span>
    </span>
  </div>
  <div v-if="loading && !data" class="empty"><ProgressSpinner style="width: 36px; height: 36px" /></div>
  <div v-else-if="data && (!data.requirements.length || !data.tests.length)" class="card empty">
    <i class="pi pi-th-large" />{{ t('traceability.needsData') }}
  </div>
  <div v-else-if="data" class="matrix-wrap">
    <table class="matrix">
      <thead>
        <tr>
          <th class="sticky-col req-cell" style="text-align: left; vertical-align: bottom">
            <span class="small muted">{{ t('traceability.size', { rows: rows.length, cols: cols.length }) }}</span>
          </th>
          <th style="vertical-align: bottom; padding: 4px 8px" class="small">{{ t('traceability.coverage') }}</th>
          <th v-for="tc in cols" :key="tc.id" class="vert" :title="`${tc.key} — ${tc.title}${tc.last_result ? ' (' + enumLabel('result', tc.last_result) + ')' : ''}`">
            <div>
              <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: tc.id } }" style="color: inherit">{{ tc.key }}</RouterLink>
            </div>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.id">
          <td class="sticky-col req-cell" :style="{ borderLeft: '4px solid ' + (COVERAGE_COLORS[r.coverage || 'none'] ?? '#94a3b8') }">
            <div class="row nowrap">
              <RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: r.id } }" class="key">{{ r.key }}</RouterLink>
              <span class="ellipsis grow" :title="r.title">{{ r.title }}</span>
              <StatusTag :value="r.obligation" kind="obligation" empty="" />
            </div>
          </td>
          <td style="padding: 2px 8px; text-align: center"><StatusTag :value="r.coverage || 'none'" kind="coverage" /></td>
          <td
            v-for="tc in cols"
            :key="tc.id"
            class="cell"
            :class="{ linked: linkMap.has(linkKey(r.id, tc.id)), editable: canEdit }"
            :title="`${r.key} ↔ ${tc.key}`"
            @click="toggle(r.id, tc.id)"
          >
            <i v-if="busy === linkKey(r.id, tc.id)" class="pi pi-spin pi-spinner" style="font-size: 11px" />
            <i v-else-if="linkMap.has(linkKey(r.id, tc.id))" class="pi pi-check" style="font-size: 11px; font-weight: 700" />
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
