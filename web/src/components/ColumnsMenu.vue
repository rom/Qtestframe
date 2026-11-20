<script setup lang="ts">
// "Columns" button of a table: choose the optional columns and their order.
// Every column has a checkbox; visible ones can be moved up and down (the
// keyboard alternative to dragging the column headers).
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Popover from 'primevue/popover'
import { useI18n } from 'vue-i18n'
import { uid } from '@/lib/a11y'

const props = defineProps<{ columns: { field: string; header: string }[]; order: string[] }>()
const emit = defineEmits<{ toggle: [id: string, on: boolean]; move: [id: string, delta: number]; reset: [] }>()
const { t } = useI18n()
const pop = ref<InstanceType<typeof Popover>>()
const listId = uid('cols')
const label = (id: string) => props.columns.find((c) => c.field === id)?.header ?? id
/** visible columns in their order, then the hidden ones */
const rows = computed(() => [
  ...props.order.filter((id) => props.columns.some((c) => c.field === id)).map((id) => ({ id, on: true })),
  ...props.columns.filter((c) => !props.order.includes(c.field)).map((c) => ({ id: c.field, on: false })),
])
const status = ref('')
function move(id: string, delta: number) {
  emit('move', id, delta)
  const i = props.order.indexOf(id) + delta
  status.value = t('columns.moved', { column: label(id), position: Math.max(1, Math.min(props.order.length, i + 1)) })
}
</script>

<template>
  <Button
    :label="t('common.columns')"
    icon="pi pi-table"
    severity="secondary"
    outlined
    aria-haspopup="dialog"
    :aria-controls="listId"
    v-tooltip.bottom="t('columns.hint')"
    @click="pop?.toggle($event)"
  />
  <Popover ref="pop" :aria-label="t('columns.title')">
    <div :id="listId" class="columns-menu">
      <div class="row" style="justify-content: space-between; margin-bottom: 6px">
        <b>{{ t('columns.title') }}</b>
        <Button :label="t('columns.reset')" text size="small" severity="secondary" @click="emit('reset')" />
      </div>
      <p class="small muted" style="margin: 0 0 6px">{{ t('columns.hint') }}</p>
      <ul class="plain-list">
        <li v-for="(r, i) in rows" :key="r.id" class="row nowrap columns-row">
          <Checkbox :model-value="r.on" binary :input-id="`${listId}-${r.id}`" @update:model-value="(v: boolean) => emit('toggle', r.id, v)" />
          <label :for="`${listId}-${r.id}`" class="grow">{{ label(r.id) }}</label>
          <template v-if="r.on">
            <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :disabled="i === 0" :aria-label="t('columns.up', { column: label(r.id) })" @click="move(r.id, -1)" />
            <Button
              icon="pi pi-arrow-down"
              text
              rounded
              size="small"
              severity="secondary"
              :disabled="i === order.length - 1"
              :aria-label="t('columns.down', { column: label(r.id) })"
              @click="move(r.id, 1)"
            />
          </template>
        </li>
      </ul>
      <span class="sr-only" aria-live="polite">{{ status }}</span>
    </div>
  </Popover>
</template>

<style>
.columns-menu {
  min-width: 260px;
  max-height: min(70vh, 520px);
  overflow-y: auto;
}
.columns-row {
  gap: 6px;
  min-height: 34px;
}
</style>
