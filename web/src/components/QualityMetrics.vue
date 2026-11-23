<script setup lang="ts">
// Quality metrics of a project (GET analytics/quality): key figures and,
// per metric, a chart with its trend or distribution, a text alternative
// and a table view, plus the items behind it. Used on the analytics page
// and the dashboard's Quality tab.
import ItemRef from './ItemRef.vue'
import QualityChart from './QualityChart.vue'
import { computed, ref, watch } from 'vue'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import SelectButton from 'primevue/selectbutton'
import InputNumber from 'primevue/inputnumber'
import { api, errorMessage } from '@/lib/api'
import { itemRoute } from '@/lib/consistency'
import { QUALITY_RANGES, hoursText, qualityKpis, trendValues, type KpiValue, type QualityRange } from '@/lib/quality'
import { formatDate, formatNumber } from '@/lib/format'
import type { QualityItem, QualityReport } from '@/lib/types'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  projectId: string
  /** id prefix of the headings (two instances never share a page, but keep ids unique) */
  idPrefix?: string
  /** a dashboard widget: its frame shows the title */
  hideTitle?: boolean
}>()
const { t, te } = useI18n()
const range = ref<QualityRange>(90)
const staleDays = ref(30)
const report = ref<QualityReport | null>(null)
const loading = ref(false)
const error = ref('')
const pre = computed(() => props.idPrefix ?? 'q')

async function load() {
  loading.value = true
  error.value = ''
  try {
    report.value = await api.get<QualityReport>(`/projects/${props.projectId}/analytics/quality`, { range: range.value, stale_days: staleDays.value || 30 })
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch([() => props.projectId, range], load, { immediate: true })
function staleChanged() {
  if (staleDays.value >= 1) void load()
}
defineExpose({ reload: load })

const rangeOptions = computed(() => QUALITY_RANGES.map((n) => ({ value: n, label: t('quality.rangeDays', { n }) })))
const fmt = (n: number) => formatNumber(n)
const hours = (h: number) => hoursText(h, fmt, { h: t('quality.unitHours'), d: t('quality.unitDays') })
const kpiValue = (k: KpiValue) => (k.percent ? `${fmt(k.value)} %` : k.hours ? hours(k.value) : fmt(k.value))
const kpis = computed(() => (report.value ? qualityKpis(report.value) : []))
const periods = computed(() => (report.value?.periods ?? []).map((p) => formatDate(p)))
const periodHeader = computed(() => t(report.value?.period === 'month' ? 'quality.month' : 'quality.week'))
const r = computed(() => report.value!)

const ageLabels = computed(() => [...(report.value?.result_age.buckets ?? []).map((b) => t('quality.ageBucket', { range: b.value })), t('quality.never')])
const ageData = computed(() => [...(report.value?.result_age.buckets ?? []).map((b) => b.count), report.value?.result_age.never ?? 0])
const folderLabels = computed(() => (report.value?.defect_density.by_folder ?? []).slice(0, 12).map((g) => g.name || t('quality.rootFolder')))
const folderData = computed(() => (report.value?.defect_density.by_folder ?? []).slice(0, 12).map((g) => g.density))
const categoryLabels = computed(() => (report.value?.defect_density.by_category ?? []).slice(0, 12).map((g) => g.name))
const categoryData = computed(() => (report.value?.defect_density.by_category ?? []).slice(0, 12).map((g) => g.density))
const phaseLabel = (p: string) => (te(`quality.phase.${p}`) ? t(`quality.phase.${p}`) : p)
const ruleLabel = (rule: string) => (te(`analytics.rule.${rule}`) ? t(`analytics.rule.${rule}`) : rule)
const severityLabel = (s: string) => (te(`defectsView.severity.${s}`) ? t(`defectsView.severity.${s}`) : s)
const listRoute = (it: QualityItem) => itemRoute(it, props.projectId)
</script>

<template>
  <section class="quality" :aria-busy="loading" :aria-labelledby="hideTitle ? undefined : `${pre}-h`">
    <div class="row qhead">
      <h2 v-if="!hideTitle" :id="`${pre}-h`" class="plain-heading grow">{{ t('quality.title') }}</h2>
      <span v-else class="grow" />
      <SelectButton v-model="range" :options="rangeOptions" option-label="label" option-value="value" :allow-empty="false" size="small" :aria-label="t('quality.range')" />
      <span class="row small nowrap" style="gap: 6px">
        <label :for="`${pre}-stale`">{{ t('quality.staleDays') }}</label>
        <InputNumber v-model="staleDays" :input-id="`${pre}-stale`" :min="1" :max="3650" size="small" input-class="stale-input" @blur="staleChanged" @keydown.enter="staleChanged" />
      </span>
      <RouterLink :to="{ name: 'reports', params: { projectId } }" class="small">{{ t('quality.export') }}</RouterLink>
    </div>
    <p class="small muted" style="margin-top: 0">{{ t('quality.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="!report && loading" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <template v-else-if="report">
      <p class="small muted">{{ t('quality.rangeLine', { from: formatDate(report.from), to: formatDate(report.to) }) }}</p>
      <ul class="plain-list qkpis">
        <li v-for="k in kpis" :key="k.id" class="card qkpi">
          <div class="small muted">{{ t(`quality.kpi.${k.id}.label`) }}</div>
          <div class="value">{{ kpiValue(k) }}</div>
          <div class="small muted">{{ t(`quality.kpi.${k.id}.sub`, k.params) }}</div>
        </li>
      </ul>

      <div class="qgrid">
        <section class="card" :aria-labelledby="`${pre}-vol`">
          <h3 :id="`${pre}-vol`" class="plain-heading">{{ t('quality.volatility') }}</h3>
          <p class="small muted">{{ t('quality.volatilityHint') }}</p>
          <QualityChart :title="t('quality.volatility')" :label-header="periodHeader" :labels="periods" :series="[{ label: t('quality.changes'), data: trendValues(r.volatility.trend, true) }]" />
          <h4 class="plain-heading small">{{ t('quality.mostChanged') }}</h4>
          <ol v-if="r.volatility.most_changed.length" class="plain-list qitems">
            <li v-for="it in r.volatility.most_changed" :key="it.id" class="row nowrap">
              <ItemRef :to="listRoute(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
              <span class="small mono">{{ t('quality.changesCount', { n: it.count }, it.count) }}</span>
            </li>
          </ol>
          <p v-else class="small muted">{{ t('quality.noChanges') }}</p>
        </section>

        <section class="card" :aria-labelledby="`${pre}-dd`">
          <h3 :id="`${pre}-dd`" class="plain-heading">{{ t('quality.density') }}</h3>
          <p class="small muted">{{ t('quality.densityHint') }}</p>
          <QualityChart :title="t('quality.densityByFolder')" :label-header="t('quality.folder')" :labels="folderLabels" :series="[{ label: t('quality.defectsPerRequirement'), data: folderData }]" horizontal :height="`${Math.max(140, 40 + folderLabels.length * 28)}px`" />
          <QualityChart v-if="categoryLabels.length" :title="t('quality.densityByCategory')" :label-header="t('quality.category')" :labels="categoryLabels" :series="[{ label: t('quality.defectsPerRequirement'), data: categoryData }]" horizontal :height="`${Math.max(140, 40 + categoryLabels.length * 28)}px`" />
          <h4 class="plain-heading small">{{ t('quality.mostDefects') }}</h4>
          <ol v-if="r.defect_density.by_requirement.length" class="plain-list qitems">
            <li v-for="it in r.defect_density.by_requirement" :key="it.id" class="row nowrap">
              <ItemRef :to="listRoute(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
              <span class="small mono">{{ t('quality.defectsCount', { n: it.count }, it.count) }}</span>
            </li>
          </ol>
          <p v-else class="small muted">{{ t('quality.noDefects') }}</p>
        </section>

        <section class="card" :aria-labelledby="`${pre}-esc`">
          <h3 :id="`${pre}-esc`" class="plain-heading">{{ t('quality.escaped') }}</h3>
          <p class="small muted">{{ t('quality.escapedHint', { production: r.escaped.production, after: r.escaped.after_release }) }}</p>
          <QualityChart :title="t('quality.escaped')" :label-header="periodHeader" :labels="periods" :series="[{ label: t('quality.escapedDefects'), data: trendValues(r.escaped.trend, true) }]" />
          <h4 class="plain-heading small">{{ t('quality.byPhase') }}</h4>
          <ul v-if="r.escaped.by_phase.length" class="plain-list qitems">
            <li v-for="p in r.escaped.by_phase" :key="p.value" class="row nowrap">
              <span class="grow">{{ phaseLabel(p.value) }}</span><span class="small mono">{{ p.count }} ({{ fmt(p.percent) }} %)</span>
            </li>
          </ul>
          <ol v-if="r.escaped.items.length" class="plain-list qitems" :aria-label="t('quality.escapedDefects')">
            <li v-for="it in r.escaped.items.slice(0, 10)" :key="it.id" class="row nowrap">
              <ItemRef :to="listRoute(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
              <span class="small muted">{{ it.note === 'production' ? phaseLabel('production') : it.note }}</span>
            </li>
          </ol>
        </section>

        <section class="card" :aria-labelledby="`${pre}-eff`">
          <h3 :id="`${pre}-eff`" class="plain-heading">{{ t('quality.effectiveness') }}</h3>
          <p class="small muted">{{ t('quality.effectivenessHint', { tests: r.effectiveness.tests, executions: r.effectiveness.executions, hours: fmt(r.effectiveness.hours), untimed: fmt(r.effectiveness.untimed_percent) }) }}</p>
          <QualityChart :title="t('quality.effectiveness')" :label-header="periodHeader" :labels="periods" :series="[{ label: t('quality.defectsFound'), data: trendValues(r.effectiveness.trend, true) }]" />
          <h4 class="plain-heading small">{{ t('quality.topTests') }}</h4>
          <ol v-if="r.effectiveness.top_tests.length" class="plain-list qitems">
            <li v-for="it in r.effectiveness.top_tests" :key="it.id" class="row nowrap">
              <ItemRef :to="listRoute(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
              <span class="small mono">{{ t('quality.defectsCount', { n: it.count }, it.count) }}</span>
            </li>
          </ol>
          <p v-else class="small muted">{{ t('quality.noDefects') }}</p>
        </section>

        <section class="card" :aria-labelledby="`${pre}-age`">
          <h3 :id="`${pre}-age`" class="plain-heading">{{ t('quality.resultAge') }}</h3>
          <p class="small muted">{{ t('quality.resultAgeHint', { median: fmt(r.result_age.median_days), stale: r.result_age.stale, days: r.result_age.stale_days }) }}</p>
          <QualityChart :title="t('quality.resultAge')" :label-header="t('quality.age')" :labels="ageLabels" :series="[{ label: t('quality.tests'), data: ageData }]" />
          <h4 class="plain-heading small">{{ t('quality.oldest') }}</h4>
          <ol v-if="r.result_age.oldest.length" class="plain-list qitems">
            <li v-for="it in r.result_age.oldest" :key="it.id" class="row nowrap">
              <ItemRef :to="listRoute(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
              <span class="small mono">{{ t('quality.daysOld', { n: it.count }, it.count) }}</span>
            </li>
          </ol>
          <p v-else class="small muted">{{ t('quality.noResults') }}</p>
        </section>

        <section class="card" :aria-labelledby="`${pre}-rev`">
          <h3 :id="`${pre}-rev`" class="plain-heading">{{ t('quality.review') }}</h3>
          <p class="small muted">
            {{ t('quality.reviewHint', { count: r.review_turnaround.count, mean: hours(r.review_turnaround.mean_hours), p90: hours(r.review_turnaround.p90_hours), pending: r.review_turnaround.pending }) }}
          </p>
          <QualityChart :title="t('quality.review')" :label-header="periodHeader" :labels="periods" :series="[{ label: t('quality.medianHours'), data: trendValues(r.review_turnaround.trend) }]" type="line" :unit="' ' + t('quality.unitHours')" />
        </section>

        <section class="card" :aria-labelledby="`${pre}-fix`">
          <h3 :id="`${pre}-fix`" class="plain-heading">{{ t('quality.fix') }}</h3>
          <p class="small muted">{{ t('quality.fixHint', { count: r.fix_time.count, median: hours(r.fix_time.median_hours), open: r.fix_time.open }) }}</p>
          <QualityChart :title="t('quality.fix')" :label-header="periodHeader" :labels="periods" :series="[{ label: t('quality.meanHours'), data: trendValues(r.fix_time.trend) }]" type="line" :unit="' ' + t('quality.unitHours')" />
          <table v-if="r.fix_time.by_severity.length" class="qtable small">
            <caption class="sr-only">{{ t('quality.fixBySeverity') }}</caption>
            <thead>
              <tr>
                <th scope="col">{{ t('quality.severity') }}</th>
                <th scope="col" class="num">{{ t('quality.resolved') }}</th>
                <th scope="col" class="num">{{ t('quality.mean') }}</th>
                <th scope="col" class="num">{{ t('quality.median') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in r.fix_time.by_severity" :key="s.severity">
                <th scope="row">{{ severityLabel(s.severity) }}</th>
                <td class="num">{{ s.count }}</td>
                <td class="num">{{ hours(s.mean_hours) }}</td>
                <td class="num">{{ hours(s.median_hours) }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section class="card" :aria-labelledby="`${pre}-rq`">
          <h3 :id="`${pre}-rq`" class="plain-heading">{{ t('quality.requirementQuality') }}</h3>
          <p class="small muted">{{ t('quality.requirementQualityHint') }}</p>
          <QualityChart :title="t('quality.requirementQuality')" :label-header="periodHeader" :labels="periods" :series="[{ label: t('quality.shareWithWarnings'), data: trendValues(r.requirement_quality.trend) }]" type="line" unit=" %" />
          <h4 class="plain-heading small">{{ t('quality.byRule') }}</h4>
          <ul v-if="r.requirement_quality.by_rule.length" class="plain-list qitems">
            <li v-for="c in r.requirement_quality.by_rule.slice(0, 8)" :key="c.value" class="row nowrap">
              <span class="grow">{{ ruleLabel(c.value) }}</span><span class="small mono">{{ c.count }}</span>
            </li>
          </ul>
          <p v-else class="small muted">{{ t('quality.noWarnings') }}</p>
        </section>
      </div>
    </template>
  </section>
</template>

<style scoped>
.qhead { flex-wrap: wrap; gap: 8px 12px; margin-bottom: 4px; }
.qkpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 12px; margin-bottom: 16px; }
.qkpi .value { font-size: 24px; font-weight: 700; }
.qgrid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr)); gap: 16px; }
.qgrid > .card { display: grid; gap: 6px; align-content: start; min-width: 0; }
.qgrid h4 { margin: 6px 0 0; }
.qitems { display: grid; gap: 2px; }
.qitems li { gap: 8px; min-width: 0; }
.qtable { border-collapse: collapse; width: 100%; }
.qtable th, .qtable td { padding: 3px 8px; border-bottom: 1px solid var(--qtf-border); text-align: left; font-weight: normal; }
.qtable thead th { font-weight: 600; }
.qtable .num { text-align: right; }
:deep(.stale-input) { width: 5em; }
</style>
