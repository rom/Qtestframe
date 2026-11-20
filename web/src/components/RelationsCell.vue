<script setup lang="ts">
// The Relations column of the requirement and test lists: related items
// grouped by relation, e.g. "depends on REQ-2, REQ-4 · verified by TC-1",
// each key linking to the item.
import { computed } from 'vue'
import { groupRefs } from '@/lib/itemRefs'
import { relationText } from '@/i18n/labels'
import type { ItemRef } from '@/lib/types'

const props = defineProps<{ refs: ItemRef[] | null | undefined; projectId: string; max?: number }>()
const groups = computed(() => groupRefs(props.refs))
const limit = computed(() => props.max ?? 4)
const shown = computed(() => groups.value.slice(0, limit.value))
const hidden = computed(() => groups.value.slice(limit.value).reduce((n, g) => n + g.items.length, 0))
const route = (kind: string, id: string) => ({ name: kind === 'test' ? 'tests' : 'requirements', params: { projectId: props.projectId }, query: { item: id } })
</script>

<template>
  <span v-if="groups.length" class="relations-cell small">
    <span v-for="g in shown" :key="g.type + g.direction" class="relation-group"
      ><span class="muted">{{ relationText(g.type, g.direction) }}</span>{{ ' '
      }}<template v-for="(r, i) in g.items" :key="r.id"
        ><RouterLink :to="route(g.kind, r.id)" class="key" :title="r.title || r.key" @click.stop>{{ r.key }}</RouterLink
        >{{ i < g.items.length - 1 ? ', ' : '' }}</template
      ></span
    >
    <span v-if="hidden" class="muted">+{{ hidden }}</span>
  </span>
  <span v-else class="muted small">–</span>
</template>

<style>
.relations-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.45;
}
.relation-group {
  display: block;
}
</style>
