<script setup lang="ts">
// MITRE ATT&CK coverage of the catalogue (GET /catalog/attack): per matrix
// (Enterprise, ICS, Mobile, ATLAS) the tactics with the number of catalogue
// risks that list one of their techniques and of the requirements that cite
// one or mitigate such a risk, as a table and as a matrix of the techniques
// used. Each count opens the catalogue's risks filtered by the tactic or the
// technique.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import SelectButton from 'primevue/selectbutton'
import Select from 'primevue/select'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import type { CoverageMatrix } from '@/lib/attack'
import type { CatalogOverview } from '@/lib/catalog'
import { useProjectStore } from '@/stores/project'
import { useI18n } from 'vue-i18n'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const proj = useProjectStore()

const matrices = ref<CoverageMatrix[]>([])
const kinds = ref<{ label: string; value: string }[]>([])
const catalogue = ref<string>(typeof route.query.catalogue === 'string' ? route.query.catalogue : '')
const matrix = ref<string>(typeof route.query.matrix === 'string' ? route.query.matrix : 'enterprise')
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const r = await api.get<{ matrices: CoverageMatrix[] }>('/catalog/attack', catalogue.value ? { catalogue: catalogue.value } : {})
    matrices.value = r.matrices ?? []
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
async function loadKinds() {
  try {
    const ov = await api.get<CatalogOverview>('/catalog', { lang: String(locale.value).startsWith('sv') ? 'sv' : 'en' })
    kinds.value = [{ label: t('catalog.types.all'), value: '' }, ...(ov.catalogues ?? []).map((k) => ({ label: k.name, value: k.id }))]
  } catch {
    kinds.value = []
  }
}
onMounted(() => Promise.all([load(), loadKinds()]))
watch(catalogue, load)
watch([matrix, catalogue], () => router.replace({ query: { ...route.query, matrix: matrix.value, catalogue: catalogue.value || undefined } }))

const matrixOptions = computed(() => matrices.value.map((m) => ({ label: `${t(`attack.matrix.${m.id}`)} (${m.risks})`, value: m.id })))
const current = computed(() => matrices.value.find((m) => m.id === matrix.value) ?? matrices.value[0])
const maxRisks = computed(() => Math.max(1, ...(current.value?.tactics ?? []).flatMap((ta) => ta.techniques.map((x) => x.risks))))
/** Shade 0–4 of a technique cell by its number of risks. */
const shade = (n: number) => (n === 0 ? 0 : Math.min(4, 1 + Math.floor((3 * n) / maxRisks.value)))
const riskLink = (q: Record<string, string>) => ({ name: 'catalog', params: { projectId: proj.pid }, query: { tab: 'risks', ...q, ...(catalogue.value ? { catalogue: catalogue.value } : {}) } })
const totals = computed(() => {
  const ts = current.value?.tactics ?? []
  return { tactics: ts.filter((x) => x.risks > 0).length, all: ts.length }
})
</script>

<template>
  <div class="attack-page">
    <div class="page-header">
      <div>
        <h1 id="attack-title" tabindex="-1">{{ t('attack.coverage.title') }}</h1>
        <div class="sub">{{ t('attack.coverage.intro') }}</div>
      </div>
      <div class="actions">
        <HelpLink page="catalogue" anchor="mitre-attck" />
        <RouterLink class="p-button p-button-sm p-button-outlined p-button-secondary" :to="{ name: 'catalog', params: { projectId: proj.pid }, query: { tab: 'risks' } }">
          <i class="pi pi-arrow-left" aria-hidden="true" />&nbsp;{{ t('attack.coverage.back') }}
        </RouterLink>
        <span v-if="kinds.length" class="row nowrap" style="gap: 6px">
          <label for="atk-type" class="small">{{ t('catalog.types.label') }}</label>
          <Select v-model="catalogue" input-id="atk-type" :options="kinds" option-label="label" option-value="value" />
        </span>
      </div>
    </div>

    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-if="loading && !matrices.length" class="empty"><ProgressSpinner style="width: 32px; height: 32px" :aria-label="t('common.loading')" /></div>

    <template v-if="current">
      <div class="row matrix-row">
        <SelectButton v-model="matrix" :options="matrixOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('attack.coverage.matrix')" />
        <span class="small muted">
          {{ t('attack.coverage.summary', { risks: current.risks, tactics: totals.tactics, all: totals.all }) }} ·
          <a :href="current.url" target="_blank" rel="noopener noreferrer">{{ t('attack.coverage.onMitre', { name: current.name, version: current.version }) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
        </span>
      </div>

      <section class="card" aria-labelledby="atk-table-title">
        <h2 id="atk-table-title" class="pane-title">{{ t('attack.coverage.byTactic') }}</h2>
        <div class="table-scroll">
          <table class="atk-table">
            <thead>
              <tr>
                <th scope="col">{{ t('attack.tactic') }}</th>
                <th scope="col" class="num">{{ t('attack.coverage.risks') }}</th>
                <th scope="col" class="num">{{ t('attack.coverage.requirements') }}</th>
                <th scope="col" class="num">{{ t('attack.coverage.techniques') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="ta in current.tactics" :key="ta.id" :class="{ muted: !ta.risks && !ta.requirements }">
                <th scope="row">
                  <a :href="ta.url" target="_blank" rel="noopener noreferrer"><span class="key">{{ ta.id }}</span> {{ ta.name }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
                </th>
                <td class="num">
                  <RouterLink v-if="ta.risks" :to="riskLink({ tactic: ta.id })" :aria-label="t('attack.coverage.showTactic', { n: ta.risks, tactic: ta.name }, ta.risks)">{{ ta.risks }}</RouterLink>
                  <template v-else>0</template>
                </td>
                <td class="num">{{ ta.requirements }}</td>
                <td class="num">{{ ta.techniques.length }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="card" aria-labelledby="atk-matrix-title">
        <h2 id="atk-matrix-title" class="pane-title">{{ t('attack.coverage.matrixTitle') }}</h2>
        <p class="small muted">{{ t('attack.coverage.matrixHint') }}</p>
        <div class="atk-matrix" role="list">
          <div v-for="ta in current.tactics" :key="ta.id" class="atk-col" role="listitem" :aria-label="ta.name">
            <div class="atk-head">
              <div class="atk-tactic">{{ ta.name }}</div>
              <div class="small muted">{{ t('attack.coverage.cellCounts', { risks: ta.risks, requirements: ta.requirements }) }}</div>
            </div>
            <ul class="atk-cells">
              <li v-for="x in ta.techniques" :key="x.id">
                <RouterLink :to="riskLink({ attack: x.id })" class="atk-cell" :class="`s${shade(x.risks)}`" :title="`${x.id} ${x.name}`">
                  <span class="key">{{ x.id }}</span>
                  <span class="atk-name">{{ x.name }}</span>
                  <span class="small">{{ t('attack.coverage.cellCounts', { risks: x.risks, requirements: x.requirements }) }}</span>
                </RouterLink>
              </li>
              <li v-if="!ta.techniques.length" class="small muted atk-none">{{ t('attack.coverage.none') }}</li>
            </ul>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.matrix-row { gap: 12px; flex-wrap: wrap; align-items: center; margin-bottom: 12px; }
.card { margin-bottom: 14px; }
.pane-title { font-size: 15px; margin: 0 0 8px; font-weight: 650; }
.table-scroll { overflow-x: auto; }
.atk-table { border-collapse: collapse; width: 100%; font-size: 13px; }
.atk-table th, .atk-table td { text-align: left; padding: 5px 8px; border-bottom: 1px solid var(--qtf-border); }
.atk-table thead th { font-weight: 600; }
.atk-table th[scope='row'] { font-weight: 500; }
.atk-table .num { text-align: right; font-variant-numeric: tabular-nums; }
.atk-matrix { display: grid; grid-auto-flow: column; grid-auto-columns: minmax(150px, 1fr); gap: 8px; overflow-x: auto; padding-bottom: 6px; }
.atk-col { min-width: 0; display: flex; flex-direction: column; gap: 6px; }
.atk-head { border-bottom: 2px solid var(--qtf-border); padding-bottom: 4px; min-height: 48px; }
.atk-tactic { font-weight: 650; font-size: 13px; overflow-wrap: anywhere; }
.atk-cells { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.atk-cell { display: flex; flex-direction: column; gap: 1px; padding: 5px 7px; border-radius: 6px; border: 1px solid var(--qtf-border); color: inherit; text-decoration: none; font-size: 12px; }
.atk-cell:hover, .atk-cell:focus-visible { border-color: var(--p-primary-color); }
.atk-cell .atk-name { overflow-wrap: anywhere; }
.atk-cell.s1 { background: color-mix(in srgb, var(--p-primary-color) 8%, transparent); }
.atk-cell.s2 { background: color-mix(in srgb, var(--p-primary-color) 15%, transparent); }
.atk-cell.s3 { background: color-mix(in srgb, var(--p-primary-color) 22%, transparent); }
.atk-cell.s4 { background: color-mix(in srgb, var(--p-primary-color) 30%, transparent); border-left: 4px solid var(--p-primary-color); }
.atk-none { padding: 4px; }
tr.muted th a, tr.muted td { color: var(--qtf-muted); }
</style>
