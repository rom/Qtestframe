<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import QueryInput from '@/components/QueryInput.vue'
import { api } from '@/lib/api'
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
  { value: 'xlsx', label: 'Excel', icon: 'pi pi-file-excel', desc: t('export.formatXlsx') },
  { value: 'csv', label: 'CSV', icon: 'pi pi-file', desc: t('export.formatCsv') },
  { value: 'pdf', label: 'PDF', icon: 'pi pi-file-pdf', desc: t('export.formatPdf') },
  { value: 'docx', label: 'Word', icon: 'pi pi-file-word', desc: t('export.formatDocx') },
])
const aspects = computed(() => [
  { value: 'requirements', label: t('export.aspectRequirements') },
  { value: 'criteria', label: t('export.aspectCriteria') },
  { value: 'tests', label: t('export.aspectTests') },
  { value: 'runs', label: t('export.aspectRuns') },
  { value: 'traceability', label: t('export.aspectTraceability') },
])
const format = ref('xlsx')
const selected = ref<string[]>(aspects.value.map((a) => a.value))
const q = ref('')
const busy = ref(false)
const error = ref<unknown>(null)
const willZip = computed(() => format.value === 'csv' && selected.value.length > 1)

async function download() {
  busy.value = true
  error.value = null
  try {
    const name = await api.download(
      `/projects/${proj.pid}/export`,
      { format: format.value, aspects: selected.value.length === aspects.value.length ? undefined : selected.value, q: q.value.trim() },
      `${proj.project?.key ?? 'export'}.${willZip.value ? 'zip' : format.value}`,
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
    <div class="section-title" style="margin-top: 0">{{ t('export.format') }}</div>
    <div class="charts" style="grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 10px">
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
        <div class="row"><i :class="f.icon" style="font-size: 20px; color: var(--p-primary-color)" /><b>{{ f.label }}</b></div>
        <div class="small muted" style="margin-top: 4px">{{ f.desc }}</div>
      </button>
    </div>
    <div class="section-title">{{ t('export.aspects') }}</div>
    <div class="row" style="gap: 18px">
      <span v-for="a in aspects" :key="a.value" class="row nowrap">
        <Checkbox v-model="selected" :input-id="'a-' + a.value" :value="a.value" />
        <label :for="'a-' + a.value">{{ a.label }}</label>
      </span>
    </div>
    <div class="section-title">{{ t('export.filter') }}</div>
    <QueryInput v-model="q" scope="requirement" :error="error" :placeholder="t('export.filterPlaceholder', { example: 'status:approved OR obligation:must' })" @submit="download" />
    <div class="row" style="margin-top: 18px">
      <span v-if="willZip" class="small muted"><i class="pi pi-info-circle" /> {{ t('export.zipHint') }}</span>
      <span class="grow" />
      <Button :label="t('export.download')" icon="pi pi-download" :loading="busy" :disabled="!selected.length" @click="download" />
    </div>
  </div>
</template>
