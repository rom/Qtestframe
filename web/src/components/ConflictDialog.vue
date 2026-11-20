<script setup lang="ts">
// Someone saved the item after it was opened: show what they changed next
// to the unsaved changes, and let the user keep editing, load their version,
// or save their own changes on top of it.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { displayValue, type FieldConflict } from '@/lib/conflicts'
import { formatDateTime } from '@/lib/format'

const props = defineProps<{ conflicts: FieldConflict[]; changedBy: string; changedAt: string; saving?: boolean }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ reload: []; merge: [] }>()
const { t, te } = useI18n()

const when = computed(() => (props.changedAt ? formatDateTime(props.changedAt) : ''))
const label = (f: string) => (te(`fields.${f}`) ? t(`fields.${f}`) : f)
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('conflict.header')" :style="{ width: 'min(760px, 95vw)' }">
    <Message severity="warn" :closable="false">
      {{ changedBy ? t('conflict.byWhom', { name: changedBy, time: when }) : t('conflict.someone', { time: when }) }}
    </Message>
    <p class="small">{{ t('conflict.explain') }}</p>
    <div v-if="conflicts.length" class="guide-table" role="region" tabindex="0" :aria-label="t('conflict.caption')">
      <table class="conflict-table">
        <caption class="sr-only">{{ t('conflict.caption') }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ t('conflict.field') }}</th>
            <th scope="col">{{ t('conflict.theirs') }}</th>
            <th scope="col">{{ t('conflict.mine') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in conflicts" :key="c.field">
            <th scope="row">
              {{ label(c.field) }}
              <Tag v-if="c.both" :value="t('conflict.both')" severity="warn" class="conflict-both" />
            </th>
            <td>{{ displayValue(c.theirs) }}</td>
            <td>{{ displayValue(c.mine) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-else class="small muted">{{ t('conflict.noFields') }}</p>
    <p class="small muted">{{ t('conflict.mergeHint') }}</p>
    <template #footer>
      <Button :label="t('conflict.keep')" text severity="secondary" @click="visible = false" />
      <Button :label="t('conflict.reload')" icon="pi pi-refresh" severity="secondary" outlined :disabled="saving" @click="emit('reload')" />
      <Button :label="t('conflict.merge')" icon="pi pi-save" :loading="saving" @click="emit('merge')" />
    </template>
  </Dialog>
</template>

<style scoped>
.conflict-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.conflict-table th,
.conflict-table td {
  text-align: left;
  vertical-align: top;
  padding: 6px 8px;
  border-bottom: 1px solid var(--qtf-border);
  overflow-wrap: anywhere;
}
.conflict-both {
  margin-left: 6px;
}
</style>
