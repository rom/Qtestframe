<script setup lang="ts">
// Requirement analytics (GET /projects/{pid}/analytics): the spread of
// obligation levels, consistent use of keywords, requirements without an
// obligation, tests or criteria, failing coverage, duplicates, conflicts
// and the language findings summed up per rule.
import ItemRef from '../components/ItemRef.vue'
import ComplianceSection from '@/components/ComplianceSection.vue'
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { FAMILY_COLORS, findingText, itemNote, languageNames, severityIcon, type AnalyticsFinding, type AnalyticsReport } from '@/lib/analytics'
import { enumLabel } from '@/i18n/labels'
import { useProjectStore } from '@/stores/project'
import { useUndoStore } from '@/stores/undo'
import { useI18n } from 'vue-i18n'

const { t, te } = useI18n()
const proj = useProjectStore()
const report = ref<AnalyticsReport | null>(null)
const loading = ref(false)
const error = ref('')
const lang = ref<'auto' | 'en' | 'sv'>('auto')
const severity = ref<'all' | 'problem' | 'warning' | 'info'>('all')
const category = ref<string | null>(null)
const expanded = ref(new Set<number>())
const SHOWN = 8

async function load() {
  loading.value = true
  error.value = ''
  try {
    report.value = await api.get<AnalyticsReport>(`/projects/${proj.pid}/analytics`, { lang: lang.value })
    expanded.value = new Set()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch([() => proj.pid, lang], load, { immediate: true })
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => void load(),
)

const severityOptions = computed(() => [
  { value: 'all', label: t('analytics.all') },
  { value: 'problem', label: t('analytics.problems', { n: report.value?.summary.problems ?? 0 }) },
  { value: 'warning', label: t('analytics.warnings', { n: report.value?.summary.warnings ?? 0 }) },
  { value: 'info', label: t('analytics.infos', { n: report.value?.summary.infos ?? 0 }) },
])
const categories = computed(() => [...new Set((report.value?.findings ?? []).map((f) => f.category))])
const categoryOptions = computed(() => categories.value.map((c) => ({ value: c, label: t(`analytics.category.${c}`) })))
const langOptions = computed(() => [
  { value: 'auto', label: t('analysis.languageAuto') },
  { value: 'en', label: t('analysis.languages.en') },
  { value: 'sv', label: t('analysis.languages.sv') },
])
const findings = computed(() =>
  (report.value?.findings ?? []).map((f, i) => ({ f, i })).filter(({ f }) => (severity.value === 'all' || f.severity === severity.value) && (!category.value || f.category === category.value)),
)
const text = (f: AnalyticsFinding) => findingText(f, t, te)
const obligationLabel = (v: string) => (v === 'none' ? t('analytics.noObligation') : enumLabel('obligation', v))
const toggle = (i: number) => {
  const s = new Set(expanded.value)
  if (s.has(i)) s.delete(i)
  else s.add(i)
  expanded.value = s
}
const itemRoute = (it: { kind: string; id: string }) => ({ name: it.kind === 'test' ? 'tests' : 'requirements', params: { projectId: proj.pid }, query: { item: it.id } })
const healthClass = computed(() => {
  const h = report.value?.summary.health ?? 100
  return h >= 80 ? 'good' : h >= 50 ? 'fair' : 'poor'
})
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('analytics.title') }}</h1>
      <div class="sub">{{ t('analytics.intro') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="traceability-and-coverage" :anchor="{ en: 'requirement-analytics', sv: 'kravanalys' }" />
      <Select v-model="lang" :options="langOptions" option-label="label" option-value="value" :aria-label="t('analysis.language')" />
      <Button icon="pi pi-refresh" :label="t('analytics.refresh')" severity="secondary" outlined :loading="loading" @click="load" />
    </div>
  </div>

  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="!report && loading" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
  <template v-else-if="report">
    <div v-if="!report.summary.requirements" class="card empty"><i class="pi pi-chart-bar" aria-hidden="true" /><div>{{ t('analytics.noRequirements') }}</div></div>
    <template v-else>
      <div class="analytics-cards">
        <div class="card stat" :class="healthClass">
          <div class="small muted">{{ t('analytics.health') }}</div>
          <div class="value">{{ report.summary.health }} %</div>
          <div class="small muted">{{ t('analytics.healthHint', { n: report.summary.with_findings, total: report.summary.requirements }) }}</div>
        </div>
        <div class="card stat">
          <div class="small muted">{{ t('analytics.requirements') }}</div>
          <div class="value">{{ report.summary.requirements }}</div>
          <div class="small muted">{{ t('analytics.testsCount', { n: report.summary.tests }) }}</div>
        </div>
        <div class="card stat">
          <div class="small muted">{{ t('analytics.coverage') }}</div>
          <div class="value">{{ report.summary.coverage_percent }} %</div>
          <div class="small muted">{{ t('analytics.coverageHint', { n: report.summary.covered }) }}</div>
        </div>
        <div class="card stat">
          <div class="small muted">{{ t('analytics.languageScore') }}</div>
          <div class="value">{{ report.summary.average_score }}</div>
          <div class="small muted">{{ t('analytics.languageScoreHint') }}</div>
        </div>
      </div>

      <div class="analytics-grid">
        <section class="card">
          <h2 class="plain-heading">{{ t('analytics.obligations') }}</h2>
          <table class="bars">
            <caption class="sr-only">{{ t('analytics.obligations') }}</caption>
            <thead class="sr-only">
              <tr>
                <th scope="col">{{ t('requirements.obligation') }}</th>
                <th scope="col">{{ t('analytics.count') }}</th>
                <th scope="col">{{ t('analytics.share') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="o in report.obligations" :key="o.value">
                <th scope="row" :class="{ 'warn-text': o.value === 'none' }">{{ obligationLabel(o.value) }}</th>
                <td class="num">{{ o.count }}</td>
                <td class="bar-cell">
                  <span class="bar" :style="{ width: Math.max(o.percent, 1) + '%' }" :class="'ob-' + o.value" aria-hidden="true" />
                  <span class="small">{{ o.percent }} %</span>
                </td>
              </tr>
            </tbody>
          </table>
        </section>

        <section class="card">
          <h2 class="plain-heading">{{ t('analytics.keywords') }}</h2>
          <p class="small muted" style="margin-top: 0">{{ t('analytics.keywordsHint') }}</p>
          <div v-if="!report.families.length" class="small muted">{{ t('analytics.noKeywords') }}</div>
          <div v-for="fam in report.families" :key="fam.family" class="family">
            <div class="row" style="justify-content: space-between">
              <b>{{ t(`analytics.family.${fam.family}`) }}</b>
              <span class="small muted">{{ t('analytics.familyTotal', { n: fam.total }, fam.total) }}</span>
            </div>
            <div class="stack" role="img" :aria-label="fam.keywords.map((k) => `${k.value} ${k.percent} %`).join(', ')">
              <span v-for="(k, i) in fam.keywords" :key="k.value" :style="{ width: k.percent + '%', background: FAMILY_COLORS[i % FAMILY_COLORS.length] }" />
            </div>
            <div class="row small" style="gap: 12px; flex-wrap: wrap">
              <span v-for="(k, i) in fam.keywords" :key="k.value" class="row nowrap" style="gap: 4px">
                <span class="swatch" :style="{ background: FAMILY_COLORS[i % FAMILY_COLORS.length] }" aria-hidden="true" />
                <b class="mono">{{ k.value }}</b> {{ k.count }} ({{ k.percent }} %)
              </span>
            </div>
          </div>
        </section>
      </div>

      <section v-if="report.languages?.length" class="card" style="margin-top: 16px">
        <h2 class="plain-heading">{{ t('analytics.languages') }}</h2>
        <p class="small muted" style="margin-top: 0">{{ t('analytics.languagesHint') }}</p>
        <div class="stack lang-stack" role="img" :aria-label="report.languages.map((l) => `${languageNames(l.value, t, te)} ${l.percent} %`).join(', ')">
          <span v-for="(l, i) in report.languages" :key="l.value" :style="{ width: l.percent + '%', background: FAMILY_COLORS[i % FAMILY_COLORS.length] }" />
        </div>
        <div class="row small" style="gap: 12px; flex-wrap: wrap; margin-top: 4px">
          <span v-for="(l, i) in report.languages" :key="l.value" class="row nowrap" style="gap: 4px">
            <span class="swatch" :style="{ background: FAMILY_COLORS[i % FAMILY_COLORS.length] }" aria-hidden="true" />
            <b>{{ languageNames(l.value, t, te) }}</b> {{ l.count }} ({{ l.percent }} %)
          </span>
        </div>
      </section>

      <section class="card" style="margin-top: 16px">
        <div class="row" style="justify-content: space-between; flex-wrap: wrap; gap: 8px">
          <h2 class="plain-heading">{{ t('analytics.findings') }}</h2>
          <div class="row" style="gap: 8px; flex-wrap: wrap">
            <SelectButton v-model="severity" :options="severityOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('analytics.severity')" />
            <Select v-model="category" :options="categoryOptions" option-label="label" option-value="value" show-clear :placeholder="t('analytics.allCategories')" :aria-label="t('analytics.categoryLabel')" />
          </div>
        </div>
        <p v-if="!findings.length" class="muted">{{ report.findings.length ? t('analytics.noneMatching') : t('analytics.noFindings') }}</p>
        <ul class="plain-list findings">
          <li v-for="{ f, i } in findings" :key="i" class="finding" :class="f.severity">
            <div class="row nowrap" style="align-items: flex-start; gap: 10px">
              <i :class="severityIcon(f.severity)" aria-hidden="true" class="sev-icon" />
              <div class="grow" style="min-width: 0">
                <div>
                  <span class="sr-only">{{ t(`analytics.severityName.${f.severity}`) }}: </span>
                  <b>{{ text(f).title }}</b>
                  <span class="small muted"> · {{ t(`analytics.category.${f.category}`) }}</span>
                </div>
                <div v-if="text(f).detail" class="small">{{ text(f).detail }}</div>
                <div v-if="text(f).fix" class="small"><b>{{ t('readiness.whatToDo') }}</b> {{ text(f).fix }}</div>
                <ul v-if="f.items.length" class="plain-list finding-items">
                  <li v-for="it in expanded.has(i) ? f.items : f.items.slice(0, SHOWN)" :key="it.id" class="row nowrap">
                    <ItemRef :to="itemRoute(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
                    <span v-if="it.note" class="small muted mono">{{ itemNote(f, it.note, t, te) }}</span>
                  </li>
                </ul>
                <Button
                  v-if="f.items.length > SHOWN"
                  :label="expanded.has(i) ? t('analytics.showFewer') : t('analytics.showAll', { n: f.items.length })"
                  text
                  size="small"
                  :aria-expanded="expanded.has(i)"
                  @click="toggle(i)"
                />
                <div v-if="f.total > f.items.length" class="small muted">{{ t('analytics.truncated', { shown: f.items.length, total: f.total }) }}</div>
              </div>
            </div>
          </li>
        </ul>
      </section>
    </template>
    <ComplianceSection :project-id="proj.pid" />
  </template>
</template>

<style>
.analytics-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.analytics-cards .stat .value {
  font-size: 26px;
  font-weight: 700;
}
.analytics-cards .stat.good .value {
  color: var(--p-green-700, #15803d);
}
.analytics-cards .stat.fair .value {
  color: var(--p-amber-700, #b45309);
}
.analytics-cards .stat.poor .value {
  color: var(--p-red-700, #b91c1c);
}
.app-dark .analytics-cards .stat.good .value {
  color: var(--p-green-300);
}
.app-dark .analytics-cards .stat.fair .value {
  color: var(--p-amber-300);
}
.app-dark .analytics-cards .stat.poor .value {
  color: var(--p-red-300);
}
.analytics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 16px;
}
.bars {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.bars th,
.bars td {
  padding: 4px 6px;
  text-align: left;
  font-weight: normal;
}
.bars .num {
  text-align: right;
  width: 3em;
}
.bars .bar-cell {
  width: 60%;
}
.bars .bar {
  display: inline-block;
  height: 12px;
  border-radius: 3px;
  background: var(--p-primary-color);
  vertical-align: middle;
  margin-right: 6px;
  max-width: calc(100% - 4em);
}
.bars .bar.ob-none {
  background: var(--p-red-500, #ef4444);
}
.family {
  display: grid;
  gap: 4px;
  margin-bottom: 12px;
}
.family .stack {
  display: flex;
  height: 14px;
  border-radius: 4px;
  overflow: hidden;
  background: var(--qtf-hover);
}
.lang-stack {
  display: flex;
  height: 14px;
  border-radius: 4px;
  overflow: hidden;
  background: var(--qtf-hover);
}
.family .swatch,
.lang-stack + .row .swatch {
  width: 10px;
  height: 10px;
  border-radius: 2px;
  display: inline-block;
}
.findings .finding {
  padding: 10px 0;
  border-top: 1px solid var(--qtf-border);
}
.finding .sev-icon {
  margin-top: 3px;
}
.finding.problem .sev-icon {
  color: var(--p-red-600, #dc2626);
}
.finding.warning .sev-icon {
  color: var(--p-amber-600, #d97706);
}
.finding.info .sev-icon {
  color: var(--p-primary-color);
}
.finding-items {
  margin-top: 4px;
}
.finding-items li {
  gap: 8px;
  padding: 2px 0;
}
</style>
