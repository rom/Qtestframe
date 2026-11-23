<script setup lang="ts">
// Compliance with laws, regulations, standards and best practices
// (GET /projects/{pid}/analytics/compliance): per chosen source the
// adopted and missing catalogue requirements by obligation, their
// fulfilment by the latest test results, the compliance per clause and the
// gaps, with links to the catalogue and the project's requirements.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import ItemRef from '@/components/ItemRef.vue'
import SourceMultiSelect from '@/components/SourceMultiSelect.vue'
import StatusTag from '@/components/StatusTag.vue'
import { api, errorMessage } from '@/lib/api'
import type { CatalogSources } from '@/lib/catalog'
import { COMPLIANCE_STATES, complianceClass, obligationLabelKey, type ComplianceReport, type ComplianceSource } from '@/lib/compliance'
import { enumLabel } from '@/i18n/labels'
import { useUndoStore } from '@/stores/undo'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string; variant?: string | null }>()
const { t, te, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const undoStore = useUndoStore()

const lang = computed(() => (String(locale.value).startsWith('sv') ? 'sv' : 'en'))
const sources = ref<CatalogSources | null>(null)
const chosen = ref<string[]>(typeof route.query.compliance === 'string' && route.query.compliance ? route.query.compliance.split(',') : [])
const report = ref<ComplianceReport | null>(null)
const loading = ref(false)
const error = ref('')
const SHOWN = 10
const expanded = ref(new Set<string>())

async function load() {
  loading.value = true
  error.value = ''
  try {
    report.value = await api.get<ComplianceReport>(`/projects/${props.projectId}/analytics/compliance`, {
      lang: lang.value,
      ...(chosen.value.length ? { source: chosen.value.join(',') } : {}),
      variant: props.variant || undefined,
    })
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch([() => props.projectId, chosen, lang, () => props.variant], load, { immediate: true })
watch(() => undoStore.version, load)
watch(chosen, (v) => void router.replace({ query: { ...route.query, compliance: v.length ? v.join(',') : undefined } }))

const kindName = (k: string) => sources.value?.kinds.find((x) => x.id === k)?.name ?? k
const jurisdiction = (j?: string) => (!j ? '' : te(`catalog.sources.jurisdiction.${j}`) ? t(`catalog.sources.jurisdiction.${j}`) : j)
const oblLabel = (o: string) => (o === 'other' ? t(obligationLabelKey) : enumLabel('obligation', o))
const stateLabel = (s: string) => t(`analytics.compliance.state.${s}`)
const clauseLabel = (c: string) => c || t('analytics.compliance.general')
const fulfilmentText = (s: ComplianceSource) => COMPLIANCE_STATES.map((st) => `${stateLabel(st)} ${s.fulfilment[st]}`).join(', ')
const total = (s: ComplianceSource) => s.catalogue + s.own
const share = (n: number, of: number) => (of ? (n * 100) / of : 0)
const reqRoute = (id: string) => ({ name: 'requirements', params: { projectId: props.projectId }, query: { item: id } })
const catalogRoute = (s: ComplianceSource, item?: string) => ({ name: 'catalog', params: { projectId: props.projectId }, query: { source: s.id, catalogue: '', ...(item ? { item } : {}) } })
const toggle = (key: string) => {
  const x = new Set(expanded.value)
  if (x.has(key)) x.delete(key)
  else x.add(key)
  expanded.value = x
}
const limited = <T,>(key: string, list: T[]) => (expanded.value.has(key) ? list : list.slice(0, SHOWN))
// details of a source: open by default when the sources were chosen or there are few of them
const toggled = ref(new Set<string>())
watch([chosen, () => props.projectId], () => (toggled.value = new Set()))
const openByDefault = computed(() => chosen.value.length > 0 || (report.value?.sources.length ?? 0) <= 2)
const isOpen = (id: string) => openByDefault.value !== toggled.value.has(id)
const toggleOpen = (id: string) => {
  const x = new Set(toggled.value)
  if (x.has(id)) x.delete(id)
  else x.add(id)
  toggled.value = x
}
</script>

<template>
  <section class="card compliance" aria-labelledby="compliance-title" :aria-busy="loading">
    <div class="row" style="justify-content: space-between; flex-wrap: wrap; gap: 8px">
      <div>
        <h2 id="compliance-title" class="plain-heading">{{ t('analytics.compliance.title') }}</h2>
        <p class="small muted" style="margin: 0">{{ t('analytics.compliance.intro') }}</p>
      </div>
      <div class="row" style="gap: 8px; flex-wrap: wrap">
        <SourceMultiSelect v-model="chosen" :placeholder="t('analytics.compliance.usedSources')" :aria-label="t('analytics.compliance.choose')" @loaded="(d) => (sources = d)" />
        <Button v-if="chosen.length" :label="t('analytics.compliance.usedSources')" text severity="secondary" @click="chosen = []" />
      </div>
    </div>

    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="!report && loading" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <template v-else-if="report">
      <Message v-if="report.unknown?.length" severity="warn" :closable="false">{{ t('analytics.compliance.unknown', { ids: report.unknown.join(', ') }) }}</Message>
      <p v-if="!report.sources.length" class="muted">{{ chosen.length ? t('analytics.compliance.noneChosen') : t('analytics.compliance.noneUsed') }}</p>
      <p v-else class="small" role="status">
        {{ t('analytics.compliance.summary', { percent: report.summary.percent, sources: report.summary.sources, adopted: report.summary.adopted, catalogue: report.summary.catalogue, own: report.summary.own }) }}
      </p>

      <article v-for="s in report.sources" :key="s.id" class="cmp-source" :aria-labelledby="`cmp-${s.id}`">
        <header class="row" style="justify-content: space-between; gap: 12px; flex-wrap: wrap; align-items: flex-start">
          <div style="min-width: 0">
            <h3 :id="`cmp-${s.id}`" class="cmp-name">
              {{ s.name }}
              <Tag v-if="s.short && s.short !== s.name" :value="s.short" severity="secondary" />
            </h3>
            <div class="small muted">
              {{ kindName(s.kind) }}<template v-if="s.jurisdiction"> · {{ jurisdiction(s.jurisdiction) }}</template>
              <template v-if="s.url"> · <a :href="s.url" target="_blank" rel="noopener">{{ t('analytics.compliance.text') }}</a></template>
            </div>
          </div>
          <div class="cmp-figures">
            <div class="cmp-figure" :class="complianceClass(s.percent)">
              <span class="value">{{ s.percent }} %</span>
              <span class="small muted">{{ t('analytics.compliance.percent') }}</span>
            </div>
            <div class="cmp-figure">
              <span class="value">{{ s.adoption_percent }} %</span>
              <span class="small muted">{{ t('analytics.compliance.adoption', { adopted: s.adopted, catalogue: s.catalogue }) }}</span>
            </div>
            <div v-if="s.mandatory_percent >= 0" class="cmp-figure" :class="complianceClass(s.mandatory_percent)">
              <span class="value">{{ s.mandatory_percent }} %</span>
              <span class="small muted">{{ t('analytics.compliance.mandatory') }}</span>
            </div>
          </div>
        </header>
        <Button
          :label="isOpen(s.id) ? t('analytics.compliance.hideDetails') : t('analytics.compliance.showDetails')"
          :icon="isOpen(s.id) ? 'pi pi-chevron-up' : 'pi pi-chevron-down'"
          text
          size="small"
          class="cmp-toggle"
          :aria-expanded="isOpen(s.id)"
          :aria-controls="`cmp-body-${s.id}`"
          @click="toggleOpen(s.id)"
        />
        <div v-if="isOpen(s.id)" :id="`cmp-body-${s.id}`" class="cmp-body">

        <div class="small">{{ t('analytics.compliance.counts', { catalogue: s.catalogue, adopted: s.adopted, missing: s.missing, own: s.own }) }}</div>
        <div class="cmp-stack" role="img" :aria-label="`${t('analytics.compliance.fulfilment')}: ${fulfilmentText(s)}`">
          <span v-for="st in COMPLIANCE_STATES" :key="st" :class="`st-${st}`" :style="{ width: share(s.fulfilment[st], total(s)) + '%' }" />
          <span class="st-missing" :style="{ width: share(s.missing, total(s)) + '%' }" />
        </div>
        <div class="row small" style="gap: 12px; flex-wrap: wrap">
          <span v-for="st in COMPLIANCE_STATES" :key="st" class="row nowrap" style="gap: 4px"><span class="swatch" :class="`st-${st}`" aria-hidden="true" />{{ stateLabel(st) }} {{ s.fulfilment[st] }}</span>
          <span class="row nowrap" style="gap: 4px"><span class="swatch st-missing" aria-hidden="true" />{{ t('analytics.compliance.missing') }} {{ s.missing }}</span>
        </div>

        <div class="cmp-tables">
          <table class="cmp-table">
            <caption>{{ t('analytics.compliance.byObligation') }}</caption>
            <thead>
              <tr>
                <th scope="col">{{ t('requirements.obligation') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.catalogue') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.adopted') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.missing') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.own') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.fulfilled') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.percent') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="o in s.obligations" :key="o.obligation">
                <th scope="row">{{ oblLabel(o.obligation) }}</th>
                <td class="num">{{ o.catalogue }}</td>
                <td class="num">{{ o.adopted }}</td>
                <td class="num" :class="{ 'warn-text': o.missing && o.obligation === 'must' }">{{ o.missing }}</td>
                <td class="num">{{ o.own }}</td>
                <td class="num">{{ o.fulfilled }}</td>
                <td class="num">{{ o.percent }} %</td>
              </tr>
            </tbody>
          </table>
          <table v-if="s.clauses.length" class="cmp-table">
            <caption>{{ t('analytics.compliance.byClause') }}</caption>
            <thead>
              <tr>
                <th scope="col">{{ t('analytics.compliance.clause') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.requirements') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.adopted') }}</th>
                <th scope="col" class="num">{{ t('analytics.compliance.fulfilled') }}</th>
                <th scope="col">{{ t('analytics.compliance.percent') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="c in s.clauses" :key="c.clause">
                <th scope="row" class="mono">{{ clauseLabel(c.clause) }}</th>
                <td class="num">{{ c.catalogue + c.own }}</td>
                <td class="num">{{ c.adopted + c.own }}</td>
                <td class="num">{{ c.fulfilment.passed }}</td>
                <td class="bar-cell">
                  <span class="bar" :class="complianceClass(c.percent)" :style="{ width: Math.max(c.percent, 1) + '%' }" aria-hidden="true" />
                  <span class="small">{{ c.percent }} %</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="s.gaps_total" class="cmp-list">
          <div class="row" style="justify-content: space-between; flex-wrap: wrap; gap: 8px">
            <h4>{{ t('analytics.compliance.gaps', { n: s.gaps_total }, s.gaps_total) }}</h4>
            <RouterLink :to="catalogRoute(s)" class="p-button p-button-sm p-button-outlined">{{ t('analytics.compliance.addMissing') }}</RouterLink>
          </div>
          <ul class="plain-list">
            <li v-for="g in limited(`g:${s.id}`, s.gaps)" :key="g.catalog_id" class="row nowrap">
              <ItemRef :to="catalogRoute(s, g.catalog_id)" :item-key="g.catalog_id" :title="g.title" title-class="ellipsis" grow />
              <StatusTag :value="g.obligation" kind="obligation" />
              <span v-if="g.clauses.length" class="small muted mono">{{ g.clauses.map(clauseLabel).join(', ') }}</span>
              <span v-if="g.inactive?.length" class="small muted">{{ t('analytics.compliance.inactive', { keys: g.inactive.map((x) => x.key).join(', ') }) }}</span>
            </li>
          </ul>
          <Button
            v-if="s.gaps.length > SHOWN"
            :label="expanded.has(`g:${s.id}`) ? t('analytics.showFewer') : t('analytics.showAll', { n: s.gaps.length })"
            text
            size="small"
            :aria-expanded="expanded.has(`g:${s.id}`)"
            @click="toggle(`g:${s.id}`)"
          />
          <div v-if="s.gaps_total > s.gaps.length" class="small muted">{{ t('analytics.truncated', { shown: s.gaps.length, total: s.gaps_total }) }}</div>
        </div>

        <div v-if="s.items.some((x) => x.state !== 'passed')" class="cmp-list">
          <h4>{{ t('analytics.compliance.notFulfilled') }}</h4>
          <ul class="plain-list">
            <li v-for="it in limited(`i:${s.id}`, s.items.filter((x) => x.state !== 'passed'))" :key="it.id" class="row nowrap">
              <ItemRef :to="reqRoute(it.id)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
              <Tag :value="stateLabel(it.state)" :severity="it.state === 'failing' ? 'danger' : it.state === 'blocked' ? 'warn' : 'secondary'" />
              <span v-if="it.own" class="small muted">{{ t('analytics.compliance.ownItem') }}</span>
              <span v-if="it.clauses.length" class="small muted mono">{{ it.clauses.map(clauseLabel).join(', ') }}</span>
            </li>
          </ul>
          <Button
            v-if="s.items.filter((x) => x.state !== 'passed').length > SHOWN"
            :label="expanded.has(`i:${s.id}`) ? t('analytics.showFewer') : t('analytics.showAll', { n: s.items.filter((x) => x.state !== 'passed').length })"
            text
            size="small"
            :aria-expanded="expanded.has(`i:${s.id}`)"
            @click="toggle(`i:${s.id}`)"
          />
        </div>
        </div>
      </article>
    </template>
  </section>
</template>

<style scoped>
.compliance { margin-top: 16px; display: grid; gap: 10px; }
.cmp-source { border-top: 1px solid var(--qtf-border); padding-top: 12px; display: grid; gap: 8px; }
.cmp-body { display: grid; gap: 8px; }
.cmp-toggle { justify-self: start; }
.cmp-name { font-size: 16px; margin: 0; display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.cmp-figures { display: flex; gap: 16px; flex-wrap: wrap; }
.cmp-figure { display: grid; text-align: right; }
.cmp-figure .value { font-size: 22px; font-weight: 700; }
.cmp-figure.good .value { color: var(--p-green-700, #15803d); }
.cmp-figure.fair .value { color: var(--p-amber-700, #b45309); }
.cmp-figure.poor .value { color: var(--p-red-700, #b91c1c); }
.app-dark .cmp-figure.good .value { color: var(--p-green-300); }
.app-dark .cmp-figure.fair .value { color: var(--p-amber-300); }
.app-dark .cmp-figure.poor .value { color: var(--p-red-300); }
.cmp-stack { display: flex; height: 14px; border-radius: 4px; overflow: hidden; background: var(--qtf-hover); }
.swatch { width: 10px; height: 10px; border-radius: 2px; display: inline-block; }
.st-passed { background: var(--p-green-500, #22c55e); }
.st-failing { background: var(--p-red-500, #ef4444); }
.st-blocked { background: var(--p-amber-500, #f59e0b); }
.st-untested { background: var(--p-slate-400, #94a3b8); }
.st-waived { background: repeating-linear-gradient(45deg, var(--p-slate-500, #64748b) 0 4px, var(--p-slate-300, #cbd5e1) 4px 8px); }
.st-missing { background: repeating-linear-gradient(45deg, var(--qtf-border), var(--qtf-border) 3px, transparent 3px, transparent 6px); }
.cmp-tables { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 12px; align-items: start; }
.cmp-table th.mono { white-space: nowrap; }
.cmp-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.cmp-table caption { text-align: left; font-weight: 600; padding-bottom: 4px; }
.cmp-table th, .cmp-table td { padding: 3px 6px; text-align: left; font-weight: normal; border-bottom: 1px solid var(--qtf-border); }
.cmp-table thead th { font-weight: 600; }
.cmp-table .num { text-align: right; }
.cmp-table .bar-cell { width: 40%; white-space: nowrap; }
.cmp-table .bar { display: inline-block; height: 10px; border-radius: 3px; vertical-align: middle; margin-right: 6px; max-width: calc(100% - 4em); background: var(--p-primary-color); }
.cmp-table .bar.good { background: var(--p-green-500, #22c55e); }
.cmp-table .bar.fair { background: var(--p-amber-500, #f59e0b); }
.cmp-table .bar.poor { background: var(--p-red-500, #ef4444); }
.cmp-list h4 { margin: 4px 0; font-size: 14px; }
.cmp-list li { gap: 8px; padding: 2px 0; }
</style>
