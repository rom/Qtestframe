<script setup lang="ts">
// "Coverage by configuration" of a requirement: the latest result of every
// linked test in every configuration it is planned in, and the coverage per
// configuration. Shown once a linked test was planned in a configuration.
import { computed, ref, watch } from 'vue'
import StatusTag from './StatusTag.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api } from '@/lib/api'
import { coverageInConfiguration, overallCoverage, resultIn, type CoverageTest } from '@/lib/configurations'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ requirementId: string; projectId: string; refresh?: unknown }>()
const { t } = useI18n()

interface CoverageResponse {
  coverage: string
  configurations: { id: string; name: string; coverage: string }[]
  tests: CoverageTest[]
}
const data = ref<CoverageResponse | null>(null)
async function load() {
  try {
    data.value = await api.get<CoverageResponse>(`/requirements/${props.requirementId}/coverage`)
  } catch {
    // without run.read results are not shown
    data.value = null
  }
}
watch(() => [props.requirementId, props.refresh], load, { immediate: true })

const configs = computed(() => data.value?.configurations ?? [])
const visible = computed(() => configs.value.some((c) => c.id))
const tests = computed(() => data.value?.tests ?? [])
const overall = computed(() => overallCoverage(tests.value))
const label = (c: { id: string; name: string }) => (c.id ? c.name : t('configurations.withoutShort'))
</script>

<template>
  <section v-if="visible" aria-labelledby="cov-cfg-heading" style="margin-top: 12px">
    <div class="section-title">
      <i class="pi pi-sliders-h" aria-hidden="true" /> <span id="cov-cfg-heading">{{ t('configurations.coverageTitle') }}</span>
      <span class="grow" />
      <span class="small muted row nowrap" style="gap: 4px">{{ t('configurations.overall') }} <StatusTag :value="overall" kind="coverage" /></span>
      <HelpLink page="traceability-and-coverage" :anchor="{ en: 'how-coverage-is-calculated', sv: 'så-beräknas-täckningen' }" />
    </div>
    <div class="table-scroll">
      <table class="cov-table">
        <caption class="sr-only">{{ t('configurations.coverageCaption') }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ t('configurations.test') }}</th>
            <th v-for="c in configs" :key="c.id" scope="col">{{ label(c) }}</th>
          </tr>
          <tr>
            <th scope="row" class="small muted">{{ t('traceability.coverage') }}</th>
            <td v-for="c in configs" :key="c.id"><StatusTag :value="coverageInConfiguration(tests, c.id)" kind="coverage" /></td>
          </tr>
        </thead>
        <tbody>
          <tr v-for="x in tests" :key="x.id">
            <th scope="row">
              <RouterLink :to="{ name: 'tests', params: { projectId }, query: { item: x.id } }" class="key">{{ x.key }}</RouterLink>
              <span class="small muted ellipsis" style="margin-left: 6px">{{ x.title }}</span>
            </th>
            <td v-for="c in configs" :key="c.id">
              <StatusTag v-if="resultIn(x, c.id)" :value="resultIn(x, c.id)" kind="result" />
              <span v-else class="muted" :title="t('configurations.notPlanned')">—<span class="sr-only">{{ t('configurations.notPlanned') }}</span></span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.table-scroll {
  overflow-x: auto;
}
.cov-table {
  border-collapse: collapse;
  width: 100%;
}
.cov-table th,
.cov-table td {
  text-align: left;
  padding: 4px 8px;
  border-bottom: 1px solid var(--qtf-border);
  white-space: nowrap;
}
.cov-table thead th {
  font-size: 12px;
  font-weight: 600;
}
</style>
