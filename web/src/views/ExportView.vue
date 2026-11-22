<script setup lang="ts">
import HelpTip from '@/guide/HelpTip.vue'
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import Select from 'primevue/select'
import QueryInput from '@/components/QueryInput.vue'
import { api, asItems } from '@/lib/api'
import type { ListResponse } from '@/lib/types'
import { exportExtension, isReqIFFormat } from '@/lib/download'
import { P } from '@/lib/permissions'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()

const formats = computed(() => [
  { value: 'pdf', label: 'PDF', icon: 'pi pi-file-pdf', desc: t('export.formatPdf') },
  { value: 'docx', label: 'Word', icon: 'pi pi-file-word', desc: t('export.formatDocx') },
  { value: 'xlsx', label: 'Excel', icon: 'pi pi-file-excel', desc: t('export.formatXlsx') },
  { value: 'csv', label: 'CSV', icon: 'pi pi-file', desc: t('export.formatCsv') },
  { value: 'md', label: 'Markdown', icon: 'pi pi-hashtag', desc: t('export.formatMd') },
  { value: 'reqif', label: 'ReqIF', icon: 'pi pi-sitemap', desc: t('export.formatReqif') },
  { value: 'reqifz', label: t('export.reqifzLabel'), icon: 'pi pi-box', desc: t('export.formatReqifz') },
])
const aspects = computed(() => [
  { value: 'requirements', label: t('export.aspectRequirements') },
  { value: 'criteria', label: t('export.aspectCriteria') },
  { value: 'tests', label: t('export.aspectTests') },
  { value: 'test_data', label: t('testData.exportAspect') },
  { value: 'runs', label: t('export.aspectRuns') },
  { value: 'traceability', label: t('export.aspectTraceability') },
])
const format = ref('xlsx')
const selected = ref<string[]>(aspects.value.map((a) => a.value))
const q = ref('')
const busy = ref(false)
const error = ref<unknown>(null)
const reqif = computed(() => isReqIFFormat(format.value))
const willZip = computed(() => exportExtension(format.value, selected.value.length) === 'zip')
// ReqIF: test cases with their links, verification records, a baseline
const withTests = ref(false)
const withRecords = ref(false)
const baseline = ref('')
const baselines = ref<{ id: string; name: string }[]>([])
const baselineOptions = computed(() => [{ value: '', label: t('export.reqifCurrent') }, ...baselines.value.map((b) => ({ value: b.id, label: b.name }))])
watch(reqif, async (on) => {
  if (!on || baselines.value.length || !proj.pid) return
  try {
    baselines.value = asItems(await api.get<ListResponse<{ id: string; name: string }>>(`/projects/${proj.pid}/baselines`))
  } catch {
    baselines.value = []
  }
})
const include = computed(() => [withTests.value ? 'tests' : '', withRecords.value ? 'records' : ''].filter(Boolean).join(','))

async function download() {
  busy.value = true
  error.value = null
  try {
    const name = await api.download(
      `/projects/${proj.pid}/export`,
      {
        format: format.value,
        aspects: reqif.value || selected.value.length === aspects.value.length ? undefined : selected.value,
        q: reqif.value && baseline.value ? undefined : q.value.trim(),
        include: reqif.value && include.value ? include.value : undefined,
        baseline: reqif.value && baseline.value ? baseline.value : undefined,
      },
      `${proj.project?.key ?? 'export'}.${exportExtension(format.value, selected.value.length)}`,
    )
    notify.ok(t('export.downloaded'), name)
  } catch (e) {
    error.value = e
    notify.fail(e, t('export.failed'))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('export.title') }}</h1>
      <div class="sub">{{ t('export.subtitle') }}</div>
    </div>
  </div>
  <Message v-if="!auth.can(P.export)" severity="warn" :closable="false">{{ t('export.noPermission') }}</Message>
  <div v-else class="card" style="max-width: 820px">
    <div id="export-format" class="section-title" style="margin-top: 0">{{ t('export.format') }}</div>
    <div class="format-grid" role="group" aria-labelledby="export-format">
      <button
        v-for="f in formats"
        :key="f.value"
        type="button"
        class="card clickable"
        :style="{
          textAlign: 'left',
          borderColor: format === f.value ? 'var(--p-primary-color)' : undefined,
          boxShadow: format === f.value ? '0 0 0 1px var(--p-primary-color)' : undefined,
          color: 'inherit',
          font: 'inherit',
        }"
        :aria-pressed="format === f.value"
        @click="format = f.value"
      >
        <div class="row"><i :class="f.icon" style="font-size: 20px; color: var(--p-primary-color)" aria-hidden="true" /><b>{{ f.label }}</b></div>
        <div class="small muted" style="margin-top: 4px">{{ f.desc }}</div>
      </button>
    </div>
    <fieldset class="plain-fieldset" :disabled="reqif">
    <legend class="section-title">{{ t('export.aspects') }} <HelpTip id="export.aspects" :label="t('export.aspects')" /></legend>
    <p v-if="reqif" class="small muted" style="margin: 0 0 8px"><i class="pi pi-info-circle" aria-hidden="true" /> {{ t('export.reqifHint') }}</p>
    <div class="row" style="gap: 18px">
      <span v-for="a in aspects" :key="a.value" class="row nowrap">
        <Checkbox v-model="selected" :input-id="'a-' + a.value" :value="a.value" :disabled="reqif" />
        <label :for="'a-' + a.value">{{ a.label }}</label>
      </span>
    </div>
    </fieldset>
    <fieldset v-if="reqif" class="plain-fieldset">
      <legend class="section-title">{{ t('export.reqifContent') }}</legend>
      <div class="row wrap" style="gap: 18px">
        <span class="row nowrap"><Checkbox v-model="withTests" binary input-id="reqif-tests" /><label for="reqif-tests">{{ t('export.reqifTests') }}</label></span>
        <span class="row nowrap"><Checkbox v-model="withRecords" binary input-id="reqif-records" /><label for="reqif-records">{{ t('export.reqifRecords') }}</label></span>
      </div>
      <div v-field class="field" style="max-width: 360px; margin-top: 10px">
        <label for="reqif-baseline">{{ t('export.reqifBaseline') }}</label>
        <Select v-model="baseline" input-id="reqif-baseline" :options="baselineOptions" option-label="label" option-value="value" />
      </div>
    </fieldset>
    <div class="section-title">{{ t('export.filter') }} <HelpTip id="query.filter" :label="t('export.filter')" /></div>
    <p v-if="reqif && baseline" class="small muted" style="margin: 0 0 6px">{{ t('export.reqifBaselineHint') }}</p>
    <QueryInput v-else v-model="q" scope="requirement" :error="error" :placeholder="t('export.filterPlaceholder', { example: 'status:approved OR obligation:must' })" @submit="download" />
    <div class="row" style="margin-top: 18px">
      <span v-if="willZip" class="small muted"><i class="pi pi-info-circle" /> {{ t('export.zipHint') }}</span>
      <span class="grow" />
      <Button :label="t('export.download')" icon="pi pi-download" :loading="busy" :disabled="!reqif && !selected.length" @click="download" />
    </div>
  </div>
</template>
