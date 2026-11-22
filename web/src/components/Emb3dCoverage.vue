<script setup lang="ts">
// MITRE EMB3D coverage of the catalogue (GET /catalog/emb3d/coverage): per
// threat category every threat with the catalogue risks that list it and
// the requirements and tests that list it or one of its mitigations. A
// threat no requirement addresses is a gap. The counts open the catalogue
// filtered by the threat.
import { computed, onMounted, ref, watch } from 'vue'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ToggleSwitch from 'primevue/toggleswitch'
import Tag from 'primevue/tag'
import { api, errorMessage } from '@/lib/api'
import { share, type Emb3dCoverage } from '@/lib/frameworks'
import { useProjectStore } from '@/stores/project'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ catalogue: string }>()
const { t } = useI18n()
const proj = useProjectStore()

const cov = ref<Emb3dCoverage | null>(null)
const loading = ref(false)
const error = ref('')
const gapsOnly = ref(false)
async function load() {
  loading.value = true
  error.value = ''
  try {
    cov.value = await api.get<Emb3dCoverage>('/catalog/emb3d/coverage', props.catalogue ? { catalogue: props.catalogue } : {})
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => props.catalogue, load)

const totals = computed(() => {
  const cs = cov.value?.categories ?? []
  return { addressed: cs.reduce((n, c) => n + c.addressed, 0), total: cs.reduce((n, c) => n + c.total, 0) }
})
const link = (tab: string, id: string) => ({ name: 'catalog', params: { projectId: proj.pid }, query: { ...(tab ? { tab } : {}), emb3d: id, ...(props.catalogue ? { catalogue: props.catalogue } : {}) } })
const slug = (id: string) => id.replace(/ /g, '_')
const categoryName = (id: string) => t(`frameworks.emb3d.category.${slug(id)}`)
</script>

<template>
  <div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-if="loading && !cov" class="empty"><ProgressSpinner style="width: 32px; height: 32px" :aria-label="t('common.loading')" /></div>
    <template v-if="cov">
      <p class="small muted">
        {{ t('frameworks.coverage.emb3dSummary', { addressed: totals.addressed, total: totals.total, share: share(totals.addressed, totals.total), mitigations: cov.mitigations }) }} ·
        <a :href="cov.url" target="_blank" rel="noopener noreferrer">{{ t('frameworks.coverage.onSite', { name: 'MITRE EMB3D', version: cov.version }) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
      </p>
      <section class="card" aria-labelledby="emb-table-title">
        <h2 id="emb-table-title" class="pane-title">{{ t('frameworks.coverage.byCategory') }}</h2>
        <div class="table-scroll" tabindex="0" role="region" aria-labelledby="emb-table-title">
          <table class="fw-table">
            <thead>
              <tr>
                <th scope="col">{{ t('frameworks.emb3d.categoryLabel') }}</th>
                <th scope="col" class="num">{{ t('frameworks.coverage.threatsAddressed') }}</th>
                <th scope="col" class="num">{{ t('frameworks.coverage.risks') }}</th>
                <th scope="col" class="num">{{ t('frameworks.coverage.requirements') }}</th>
                <th scope="col" class="num">{{ t('frameworks.coverage.tests') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="c in cov.categories" :key="c.id">
                <th scope="row">{{ categoryName(c.id) }}</th>
                <td class="num">{{ t('frameworks.coverage.ofTotal', { n: c.addressed, total: c.total }) }}</td>
                <td class="num">{{ c.risks }}</td>
                <td class="num">{{ c.requirements }}</td>
                <td class="num">{{ c.tests }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section class="card emb-threats" aria-labelledby="emb-threats-title">
        <div class="row emb-head">
          <h2 id="emb-threats-title" class="pane-title">{{ t('frameworks.coverage.threats') }}</h2>
          <span class="row small nowrap" style="gap: 6px">
            <ToggleSwitch v-model="gapsOnly" input-id="emb-gaps" />
            <label for="emb-gaps">{{ t('frameworks.coverage.gapsOnly') }}</label>
          </span>
        </div>
        <div v-for="c in cov.categories" :key="c.id" class="emb-cat">
          <h3 :id="`emb-cat-${slug(c.id)}`" class="emb-cat-title">{{ categoryName(c.id) }}</h3>
          <div class="table-scroll">
            <table class="fw-table" :aria-labelledby="`emb-cat-${slug(c.id)}`">
              <thead>
                <tr>
                  <th scope="col">{{ t('frameworks.emb3d.threat') }}</th>
                  <th scope="col" class="num">{{ t('frameworks.coverage.risks') }}</th>
                  <th scope="col" class="num">{{ t('frameworks.coverage.requirements') }}</th>
                  <th scope="col" class="num">{{ t('frameworks.coverage.tests') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="x in c.threats.filter((x) => !gapsOnly || !x.requirements)" :key="x.id">
                  <th scope="row">
                    <a :href="x.url" target="_blank" rel="noopener noreferrer"><span class="key">{{ x.id }}</span> {{ x.name }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
                    <Tag v-if="!x.requirements" :value="t('frameworks.defenses.gap')" severity="warn" class="small emb-gap" />
                  </th>
                  <td class="num">
                    <RouterLink v-if="x.risks" :to="link('risks', x.id)">{{ x.risks }}<span class="sr-only">: {{ t('frameworks.coverage.showRisks', { id: x.id }) }}</span></RouterLink>
                    <template v-else>0</template>
                  </td>
                  <td class="num">
                    <RouterLink v-if="x.requirements" :to="link('', x.id)">{{ x.requirements }}<span class="sr-only">: {{ t('frameworks.coverage.showRequirements', { id: x.id }) }}</span></RouterLink>
                    <template v-else>0</template>
                  </td>
                  <td class="num">
                    <RouterLink v-if="x.tests" :to="link('tests', x.id)">{{ x.tests }}<span class="sr-only">: {{ t('frameworks.coverage.showTests', { id: x.id }) }}</span></RouterLink>
                    <template v-else>0</template>
                  </td>
                </tr>
              </tbody>
            </table>
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
.fw-table .num { text-align: right; font-variant-numeric: tabular-nums; width: 8em; }
.emb-head { justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; }
.emb-cat { margin-top: 10px; }
.emb-cat-title { font-size: 13px; font-weight: 650; margin: 6px 0 4px; }
.emb-gap { margin-left: 6px; }
</style>
