<script setup lang="ts">
// MITRE D3FEND coverage of the catalogue (GET /catalog/d3fend/coverage):
// per defensive tactic the techniques that catalogue requirements and tests
// list, and how many catalogue risks have each ATT&CK technique countered
// by a mitigating requirement. Each technique opens the catalogue's
// requirements filtered by it.
import { computed, onMounted, ref, watch } from 'vue'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import { api, errorMessage } from '@/lib/api'
import { share, type D3fendCoverage } from '@/lib/frameworks'
import { useProjectStore } from '@/stores/project'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ catalogue: string }>()
const { t } = useI18n()
const proj = useProjectStore()

const cov = ref<D3fendCoverage | null>(null)
const loading = ref(false)
const error = ref('')
async function load() {
  loading.value = true
  error.value = ''
  try {
    cov.value = await api.get<D3fendCoverage>('/catalog/d3fend/coverage', props.catalogue ? { catalogue: props.catalogue } : {})
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => props.catalogue, load)

const maxReqs = computed(() => Math.max(1, ...(cov.value?.tactics ?? []).flatMap((ta) => ta.techniques.map((x) => x.requirements))))
const shade = (n: number) => (n === 0 ? 0 : Math.min(4, 1 + Math.floor((3 * n) / maxReqs.value)))
const reqLink = (id: string) => ({ name: 'catalog', params: { projectId: proj.pid }, query: { d3fend: id, ...(props.catalogue ? { catalogue: props.catalogue } : {}) } })
</script>

<template>
  <div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-if="loading && !cov" class="empty"><ProgressSpinner style="width: 32px; height: 32px" :aria-label="t('common.loading')" /></div>
    <template v-if="cov">
      <p class="small muted">
        {{ t('frameworks.coverage.d3fendSummary', { risks: cov.risks, covered: cov.covered, share: share(cov.covered, cov.risks), gaps: cov.gaps }) }} ·
        <a :href="cov.url" target="_blank" rel="noopener noreferrer">{{ t('frameworks.coverage.onSite', { name: 'MITRE D3FEND', version: cov.version }) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
      </p>
      <section class="card" aria-labelledby="d3-table-title">
        <h2 id="d3-table-title" class="pane-title">{{ t('frameworks.coverage.byTactic') }}</h2>
        <div class="table-scroll" tabindex="0" role="region" aria-labelledby="d3-table-title">
          <table class="fw-table">
            <thead>
              <tr>
                <th scope="col">{{ t('frameworks.d3fend.tactic') }}</th>
                <th scope="col" class="num">{{ t('frameworks.coverage.techniquesUsed') }}</th>
                <th scope="col" class="num">{{ t('frameworks.coverage.requirements') }}</th>
                <th scope="col" class="num">{{ t('frameworks.coverage.tests') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="ta in cov.tactics" :key="ta.id" :class="{ muted: !ta.used }">
                <th scope="row">
                  <a :href="ta.url" target="_blank" rel="noopener noreferrer">{{ ta.name }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
                </th>
                <td class="num">{{ t('frameworks.coverage.ofTotal', { n: ta.used, total: ta.total }) }}</td>
                <td class="num">{{ ta.requirements }}</td>
                <td class="num">{{ ta.tests }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section class="card" aria-labelledby="d3-matrix-title">
        <h2 id="d3-matrix-title" class="pane-title">{{ t('frameworks.coverage.matrixTitle') }}</h2>
        <p class="small muted">{{ t('frameworks.coverage.d3fendMatrixHint') }}</p>
        <div class="fw-matrix" role="list">
          <div v-for="ta in cov.tactics" :key="ta.id" class="fw-col" role="listitem" :aria-label="ta.name">
            <div class="fw-head">
              <div class="fw-tactic">{{ ta.name }}</div>
              <div class="small muted">{{ t('frameworks.coverage.counts', { requirements: ta.requirements, tests: ta.tests }) }}</div>
            </div>
            <ul class="fw-cells">
              <li v-for="x in ta.techniques" :key="x.id">
                <RouterLink :to="reqLink(x.id)" class="fw-cell" :class="`s${shade(x.requirements)}`" :title="`${x.id} ${x.name}`">
                  <span class="key">{{ x.id }}</span>
                  <span class="fw-name">{{ x.name }}</span>
                  <span class="small">{{ t('frameworks.coverage.counts', { requirements: x.requirements, tests: x.tests }) }}</span>
                </RouterLink>
              </li>
              <li v-if="!ta.techniques.length" class="small muted fw-none">{{ t('frameworks.coverage.none') }}</li>
            </ul>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.card { margin-bottom: 14px; }
.pane-title { font-size: 15px; margin: 0 0 8px; font-weight: 650; }
.table-scroll { overflow-x: auto; }
.fw-table { border-collapse: collapse; width: 100%; font-size: 13px; }
.fw-table th, .fw-table td { text-align: left; padding: 5px 8px; border-bottom: 1px solid var(--qtf-border); }
.fw-table thead th { font-weight: 600; }
.fw-table th[scope='row'] { font-weight: 500; }
.fw-table .num { text-align: right; font-variant-numeric: tabular-nums; }
.fw-matrix { display: grid; grid-auto-flow: column; grid-auto-columns: minmax(150px, 1fr); gap: 8px; overflow-x: auto; padding-bottom: 6px; }
.fw-col { min-width: 0; display: flex; flex-direction: column; gap: 6px; }
.fw-head { border-bottom: 2px solid var(--qtf-border); padding-bottom: 4px; min-height: 48px; }
.fw-tactic { font-weight: 650; font-size: 13px; overflow-wrap: anywhere; }
.fw-cells { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.fw-cell { display: flex; flex-direction: column; gap: 1px; padding: 5px 7px; border-radius: 6px; border: 1px solid var(--qtf-border); color: inherit; text-decoration: none; font-size: 12px; }
.fw-cell:hover, .fw-cell:focus-visible { border-color: var(--p-primary-color); }
.fw-cell .fw-name { overflow-wrap: anywhere; }
.fw-cell .key { color: inherit; font-weight: 650; }
.fw-cell.s1 { background: color-mix(in srgb, var(--p-primary-color) 6%, transparent); }
.fw-cell.s2 { background: color-mix(in srgb, var(--p-primary-color) 10%, transparent); border-left: 2px solid var(--p-primary-color); }
.fw-cell.s3 { background: color-mix(in srgb, var(--p-primary-color) 14%, transparent); border-left: 3px solid var(--p-primary-color); }
.fw-cell.s4 { background: color-mix(in srgb, var(--p-primary-color) 18%, transparent); border-left: 5px solid var(--p-primary-color); }
.fw-none { padding: 4px; }
tr.muted th a, tr.muted td { color: var(--qtf-muted); }
</style>
