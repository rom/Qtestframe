<script setup lang="ts">
import { useUndoStore } from '@/stores/undo'
import { recordUndo, toggleEntry } from '@/lib/undoActions'
import HelpTip from '@/guide/HelpTip.vue'
import { computed, onMounted, ref, watch } from 'vue'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import MultiSelect from 'primevue/multiselect'
import Button from 'primevue/button'
import ToggleSwitch from 'primevue/toggleswitch'
import ProgressSpinner from 'primevue/progressspinner'
import SelectButton from 'primevue/selectbutton'
import { useBreakpoint } from '@/lib/breakpoint'
import { readPref, writePref } from '@/stores/ui'
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

// matrix or one list per requirement (default on phones)
const phone = useBreakpoint('phone')
const storedView = readPref('qtf.trace.view')
const view = ref<'matrix' | 'list'>(storedView === 'matrix' || storedView === 'list' ? storedView : phone.value ? 'list' : 'matrix')
watch(view, (v) => writePref('qtf.trace.view', v))
const viewOptions = computed(() => [
  { value: 'matrix', label: t('traceability.viewMatrix'), icon: 'pi pi-th-large' },
  { value: 'list', label: t('traceability.viewList'), icon: 'pi pi-list' },
])
const showAll = ref<Record<string, boolean>>({})
const isLinked = (reqId: string, testId: string) => linkMap.value.has(linkKey(reqId, testId))
function linkedCols(reqId: string) {
  return cols.value.filter((tc) => isLinked(reqId, tc.id))
}
function cellLabel(rKey: string, tKey: string) {
  return t('traceability.linkCell', { req: rKey, test: tKey })
}

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

function matrixEntry(created: boolean, id: string, reqId: string, testId: string) {
  const pid = proj.pid
  const rk = data.value?.requirements.find((r) => r.id === reqId)?.key ?? ''
  const tk = data.value?.tests.find((x) => x.id === testId)?.key ?? ''
  return toggleEntry(
    t(created ? 'undo.a.link' : 'undo.a.unlink', { from: rk, to: tk }),
    created,
    id,
    async () => (await api.post<TraceLink>(`/projects/${pid}/links`, { requirement_id: reqId, test_case_id: testId })).id,
    async (x) => void (await api.del(`/links/${x}`)),
  )
}
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => void load(),
)

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
      recordUndo(matrixEntry(false, existing.id, reqId, testId))
    } else {
      const l = await api.post<TraceLink>(`/projects/${proj.pid}/links`, { requirement_id: reqId, test_case_id: testId })
      data.value.links = [...data.value.links, l ?? ({ id: '', requirement_id: reqId, test_case_id: testId } as TraceLink)]
      if (l?.id) recordUndo(matrixEntry(true, l.id, reqId, testId))
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
      <SelectButton v-model="view" :options="viewOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('traceability.view')">
        <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /><span>{{ option.label }}</span></template>
      </SelectButton>
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
    </div>
  </div>
  <div class="filterbar">
    <IconField>
      <InputIcon class="pi pi-filter" aria-hidden="true" />
      <InputText v-model="reqFilter" :placeholder="t('traceability.filterRequirements')" :aria-label="t('traceability.filterRequirements')" />
    </IconField>
    <IconField>
      <InputIcon class="pi pi-filter" aria-hidden="true" />
      <InputText v-model="testFilter" :placeholder="t('traceability.filterTests')" :aria-label="t('traceability.filterTests')" />
    </IconField>
    <MultiSelect v-model="coverage" :options="coverageOptions" option-label="label" option-value="value" :placeholder="t('traceability.coverage')" :aria-label="t('traceability.coverage')" style="min-width: 150px" show-clear />
    <span class="row small nowrap"><ToggleSwitch v-model="onlyLinkedTests" input-id="olt" /><label for="olt">{{ t('traceability.onlyLinked') }}</label><HelpTip id="trace.onlyLinked" :label="t('traceability.onlyLinked')" /></span>
    <HelpTip id="trace.coverage" :label="t('traceability.coverage')" />
    <ul class="row small muted plain-list trace-legend" :aria-label="t('traceability.legend')">
      <li v-for="(c, k) in COVERAGE_COLORS" :key="k" class="row nowrap" style="gap: 4px"><span aria-hidden="true" :style="{ width: '10px', height: '10px', borderRadius: '3px', background: c }" />{{ enumLabel('coverage', String(k)) }}</li>
    </ul>
  </div>
  <div v-if="loading && !data" class="empty" aria-busy="true"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
  <div v-else-if="data && (!data.requirements.length || !data.tests.length)" class="card empty">
    <i class="pi pi-th-large" aria-hidden="true" />{{ t('traceability.needsData') }}
  </div>

  <!-- list per requirement -->
  <ul v-else-if="data && view === 'list'" class="trace-list plain-list" :aria-busy="loading">
    <li v-for="r in rows" :key="r.id" class="card trace-item" :style="{ borderLeft: '4px solid ' + (COVERAGE_COLORS[r.coverage || 'none'] ?? '#94a3b8') }">
      <div class="row trace-item-head">
        <RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: r.id } }" class="key">{{ r.key }}</RouterLink>
        <span class="grow trace-title">{{ r.title }}</span>
        <StatusTag :value="r.obligation" kind="obligation" empty="" />
        <StatusTag :value="r.coverage || 'none'" kind="coverage" />
      </div>
      <div class="small muted" style="margin: 6px 0 4px">{{ t('traceability.linkedTests', linkedCols(r.id).length) }}</div>
      <div class="row" style="gap: 6px">
        <template v-for="tc in showAll[r.id] ? cols : linkedCols(r.id)" :key="tc.id">
          <button
            v-if="canEdit"
            type="button"
            class="trace-chip"
            :class="{ linked: isLinked(r.id, tc.id) }"
            :aria-pressed="isLinked(r.id, tc.id)"
            :aria-label="cellLabel(r.key, tc.key)"
            :title="tc.title"
            :disabled="!!busy"
            @click="toggle(r.id, tc.id)"
          >
            <i :class="busy === linkKey(r.id, tc.id) ? 'pi pi-spin pi-spinner' : isLinked(r.id, tc.id) ? 'pi pi-check' : 'pi pi-plus'" aria-hidden="true" />
            <span class="mono">{{ tc.key }}</span>
          </button>
          <RouterLink v-else :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: tc.id } }" class="trace-chip linked" :title="tc.title">
            <i class="pi pi-check" aria-hidden="true" /><span class="mono">{{ tc.key }}</span>
          </RouterLink>
        </template>
        <span v-if="!linkedCols(r.id).length && !showAll[r.id]" class="small muted">{{ t('traceability.noLinkedTests') }}</span>
      </div>
      <Button
        v-if="canEdit"
        size="small"
        text
        :label="showAll[r.id] ? t('traceability.showLinkedOnly') : t('traceability.showAllTests', { count: cols.length })"
        :icon="showAll[r.id] ? 'pi pi-eye-slash' : 'pi pi-plus-circle'"
        :aria-expanded="!!showAll[r.id]"
        style="margin-top: 6px"
        @click="showAll[r.id] = !showAll[r.id]"
      />
    </li>
  </ul>

  <!-- matrix -->
  <div v-else-if="data" class="matrix-wrap" tabindex="0" role="region" :aria-label="t('traceability.matrix')" :aria-busy="loading">
    <table class="matrix">
      <caption class="sr-only">{{ t('traceability.matrixCaption', { rows: rows.length, cols: cols.length }) }}</caption>
      <thead>
        <tr>
          <th scope="col" class="sticky-col req-cell" style="text-align: left; vertical-align: bottom">
            <span class="small muted">{{ t('traceability.size', { rows: rows.length, cols: cols.length }) }}</span>
          </th>
          <th scope="col" style="vertical-align: bottom; padding: 4px 8px" class="small">{{ t('traceability.coverage') }}</th>
          <th v-for="tc in cols" :key="tc.id" scope="col" class="vert" :title="`${tc.key} — ${tc.title}${tc.last_result ? ' (' + enumLabel('result', tc.last_result) + ')' : ''}`">
            <div>
              <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: tc.id } }" style="color: inherit">{{ tc.key }}</RouterLink>
            </div>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.id">
          <th scope="row" class="sticky-col req-cell" :style="{ borderLeft: '4px solid ' + (COVERAGE_COLORS[r.coverage || 'none'] ?? '#94a3b8') }">
            <div class="row nowrap">
              <RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: r.id } }" class="key">{{ r.key }}</RouterLink>
              <span class="ellipsis grow" :title="r.title">{{ r.title }}</span>
              <StatusTag :value="r.obligation" kind="obligation" empty="" />
            </div>
          </th>
          <td style="padding: 2px 8px; text-align: center"><StatusTag :value="r.coverage || 'none'" kind="coverage" /></td>
          <td v-for="tc in cols" :key="tc.id" class="cell" :class="{ linked: isLinked(r.id, tc.id), editable: canEdit }">
            <button
              v-if="canEdit"
              type="button"
              class="cell-button"
              :aria-pressed="isLinked(r.id, tc.id)"
              :aria-label="cellLabel(r.key, tc.key)"
              :title="`${r.key} ↔ ${tc.key}`"
              @click="toggle(r.id, tc.id)"
            >
              <i v-if="busy === linkKey(r.id, tc.id)" class="pi pi-spin pi-spinner" aria-hidden="true" />
              <i v-else-if="isLinked(r.id, tc.id)" class="pi pi-check" aria-hidden="true" />
            </button>
            <template v-else-if="isLinked(r.id, tc.id)">
              <i class="pi pi-check" aria-hidden="true" /><span class="sr-only">{{ t('traceability.linkedCell', { req: r.key, test: tc.key }) }}</span>
            </template>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
