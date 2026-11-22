<script setup lang="ts">
// The proposed changes of one item of a change request: per field the
// current value and the proposed one (a word diff for texts).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { itemDiffs, type ChangeRequestItem } from '@/lib/changeRequests'

const props = defineProps<{ item: Pick<ChangeRequestItem, 'entity_type' | 'proposed' | 'current'> }>()
const { t, te } = useI18n()
const diffs = computed(() => itemDiffs(props.item))
const label = (f: string) => (te(`changeRequests.field.${f}`) ? t(`changeRequests.field.${f}`) : f)
</script>

<template>
  <dl class="cr-diff">
    <template v-for="d in diffs" :key="d.field">
      <dt>{{ label(d.field) }}</dt>
      <dd>
        <div v-if="d.segments" class="diff">
          <template v-for="(s, i) in d.segments" :key="i">
            <ins v-if="s.op === 'insert'">{{ s.text }}</ins>
            <del v-else-if="s.op === 'delete'">{{ s.text }}</del>
            <span v-else>{{ s.text }}</span>
          </template>
        </div>
        <span v-else>
          <span v-if="d.before" class="diff-before">{{ d.before }}</span>
          <span aria-hidden="true"> → </span><span class="sr-only">{{ t('changeRequests.becomes') }}</span>
          <span class="diff-after">{{ d.after || '—' }}</span>
        </span>
      </dd>
    </template>
  </dl>
</template>

<style scoped>
.cr-diff {
  display: grid;
  grid-template-columns: minmax(100px, max-content) minmax(0, 1fr);
  gap: 4px 12px;
  margin: 6px 0 0;
}
.cr-diff dt {
  font-weight: 600;
  color: var(--qtf-muted);
}
.cr-diff dd {
  margin: 0;
}
@media (max-width: 560px) {
  .cr-diff {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
