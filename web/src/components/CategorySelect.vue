<script setup lang="ts">
import { computed } from 'vue'
import MultiSelect from 'primevue/multiselect'
import type { Category } from '@/lib/types'
import { groupBy } from '@/lib/collections'
import { normalizeHex, defaultCategoryColor } from '@/lib/colors'

const model = defineModel<string[]>({ default: () => [] })
const props = defineProps<{ categories: Category[]; disabled?: boolean; placeholder?: string; valueKey?: 'id' | 'path' }>()

const groups = computed(() =>
  groupBy(
    [...props.categories].sort((a, b) => a.dimension.localeCompare(b.dimension) || a.name.localeCompare(b.name)),
    (c) => c.dimension,
  ).map((g) => ({
    label: g.key,
    items: g.items.map((c) => ({
      label: c.name,
      value: props.valueKey === 'path' ? `${c.dimension}/${c.name}` : c.id,
      color: normalizeHex(c.color) ?? defaultCategoryColor(c.dimension),
    })),
  })),
)
</script>

<template>
  <MultiSelect
    v-model="model"
    :options="groups"
    option-label="label"
    option-value="value"
    option-group-label="label"
    option-group-children="items"
    :placeholder="placeholder ?? 'Categories'"
    :disabled="disabled"
    display="chip"
    filter
    :max-selected-labels="4"
    class="w-full"
  >
    <template #optiongroup="{ option }">
      <span class="small" style="font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em">{{ option.label }}</span>
    </template>
    <template #option="{ option }">
      <span class="row nowrap"><span :style="{ width: '10px', height: '10px', borderRadius: '50%', background: option.color }" />{{ option.label }}</span>
    </template>
  </MultiSelect>
</template>
