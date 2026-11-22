<script setup lang="ts">
// The rows of an exchange diff (ExchangeImportDialog): a filter by status
// and a table with the changes and problems of every row, and whether it
// is saved.
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Tag from 'primevue/tag'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import { useI18n } from 'vue-i18n'
import { uid } from '@/lib/a11y'
import { clip, ROW_STATUSES, savable, statusCount, visibleRows, type ExchangeDiff, type ExchangeRow, type RowStatus } from '@/lib/exchange'

const props = defineProps<{ diff: ExchangeDiff; opts: { overwrite: boolean; create: boolean }; excluded: ReadonlySet<number>; labelledby: string }>()
const emit = defineEmits<{ toggle: [index: number, on: boolean] }>()
const { t } = useI18n()

/** rows shown at once in the table */
const SHOWN = 200
const filter = ref<RowStatus[]>([])
const id = uid('ex-rows')
const shown = computed(() => visibleRows(props.diff.rows, filter.value).slice(0, SHOWN))
const filtered = computed(() => visibleRows(props.diff.rows, filter.value).length)

function toggleFilter(st: RowStatus) {
  filter.value = filter.value.includes(st) ? filter.value.filter((x) => x !== st) : [...filter.value, st]
}

const severity: Record<RowStatus, string> = { new: 'info', changed: 'success', conflict: 'warn', error: 'danger', unchanged: 'secondary' }
const icon: Record<RowStatus, string> = { new: 'pi pi-plus', changed: 'pi pi-pencil', conflict: 'pi pi-exclamation-triangle', error: 'pi pi-times-circle', unchanged: 'pi pi-check' }

function rowLabel(r: ExchangeRow) {
  return [r.key, r.title].filter(Boolean).join(' ')
}
</script>

<template>
  <div class="row wrap ex-filter" role="group" :aria-label="t('exchange.filterLabel')">
    <Button
      v-for="st in ROW_STATUSES"
      :key="st"
      size="small"
      :severity="filter.includes(st) ? undefined : 'secondary'"
      :outlined="!filter.includes(st)"
      :icon="icon[st]"
      :label="t(`exchange.status.${st}`) + ' ' + statusCount(diff.summary, st)"
      :aria-pressed="filter.includes(st)"
      @click="toggleFilter(st)"
    />
  </div>
  <p class="small muted" style="margin: 6px 0">
    {{ filtered > SHOWN ? t('exchange.showingFirst', { shown: SHOWN, total: filtered }) : t('exchange.showingAll', { total: filtered }) }}
  </p>
  <DataTable :value="shown" data-key="index" size="small" scrollable scroll-height="45vh" class="ex-table" :aria-labelledby="labelledby">
    <Column :header="t('exchange.include')" style="width: 70px">
      <template #body="{ data }">
        <Checkbox
          v-if="savable(data, opts)"
          :model-value="!excluded.has(data.index)"
          binary
          :input-id="`${id}-${data.index}`"
          :aria-label="t('exchange.includeRow', { item: rowLabel(data) })"
          @update:model-value="(v: boolean) => emit('toggle', data.index, v)"
        />
      </template>
    </Column>
    <Column field="location" :header="t('exchange.location')" style="width: 130px" />
    <Column :header="t('exchange.item')" style="min-width: 180px" body-class="stack-main">
      <template #body="{ data }">
        <span v-if="data.key" class="key">{{ data.key }}</span> {{ data.title }}
      </template>
    </Column>
    <Column :header="t('exchange.statusColumn')" style="width: 130px">
      <template #body="{ data }">
        <Tag :severity="severity[data.status as RowStatus]" :icon="icon[data.status as RowStatus]" :value="t(`exchange.status.${data.status}`)" style="white-space: nowrap" />
        <div v-if="data.status === 'conflict'" class="small muted">{{ t('exchange.versions', { base: data.base_version, now: data.version }) }}</div>
      </template>
    </Column>
    <Column :header="t('exchange.details')" style="min-width: 260px">
      <template #body="{ data }">
        <ul v-if="data.errors.length" class="ex-list small error-text">
          <li v-for="(e, i) in data.errors" :key="'e' + i"><span v-if="e.cell" class="mono">{{ e.cell }}</span> <b>{{ e.label }}:</b> {{ e.message }}</li>
        </ul>
        <ul v-if="data.changes.length" class="ex-list small">
          <li v-for="(c, i) in data.changes" :key="'c' + i">
            <b>{{ c.label }}:</b>
            <template v-if="data.status !== 'new'">
              <del :aria-label="t('exchange.oldValue', { value: c.old || t('exchange.empty') })">{{ clip(c.old) || t('exchange.empty') }}</del>
              <span aria-hidden="true"> → </span>
            </template>
            <ins :aria-label="t('exchange.newValue', { value: c.new || t('exchange.empty') })">{{ clip(c.new) || t('exchange.empty') }}</ins>
          </li>
        </ul>
        <ul v-if="data.warnings.length" class="ex-list small muted">
          <li v-for="(w, i) in data.warnings" :key="'w' + i">{{ w }}</li>
        </ul>
      </template>
    </Column>
    <template #empty><div class="empty">{{ t('exchange.none') }}</div></template>
  </DataTable>
</template>

<style scoped>
.ex-filter {
  gap: 6px;
}
.ex-list {
  margin: 2px 0 0;
  padding-left: 18px;
}
.ex-list del {
  text-decoration: line-through;
  opacity: 0.85;
}
.ex-list ins {
  text-decoration: underline;
}
</style>
