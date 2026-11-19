<script setup lang="ts">
import { computed } from 'vue'
import type { Category } from '@/lib/types'
import { readableTextColor, normalizeHex, defaultCategoryColor } from '@/lib/colors'

const props = defineProps<{ ids?: string[] | null; categories: Category[] }>()
const items = computed(() => {
  const byId = new Map(props.categories.map((c) => [c.id, c]))
  return (props.ids ?? []).map((id) => byId.get(id)).filter((c): c is Category => !!c)
})
function bg(c: Category) {
  return normalizeHex(c.color) ?? defaultCategoryColor(c.dimension)
}
</script>

<template>
  <span>
    <span
      v-for="c in items"
      :key="c.id"
      class="chip-cat"
      :style="{ background: bg(c), color: readableTextColor(bg(c)) }"
      :title="c.dimension + ' / ' + c.name"
      >{{ c.name }}</span
    >
  </span>
</template>
