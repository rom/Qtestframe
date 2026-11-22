<script setup lang="ts">
// A multi-select of the project's variants with the hierarchy shown by
// indentation (the variants a release targets, the variants a waiver is
// limited to). Empty: none / every variant, as the placeholder says.
import { computed } from 'vue'
import MultiSelect from 'primevue/multiselect'
import { variantOptions, type Variant } from '@/lib/variants'

const props = defineProps<{ variants: Variant[]; inputId?: string; placeholder?: string; disabled?: boolean; ariaDescribedby?: string }>()
const model = defineModel<string[]>({ default: () => [] })
const options = computed(() => variantOptions(props.variants))
</script>

<template>
  <MultiSelect
    v-model="model"
    :input-id="inputId"
    :options="options"
    option-label="label"
    option-value="value"
    display="chip"
    filter
    :placeholder="placeholder"
    :disabled="disabled"
    :aria-describedby="ariaDescribedby"
    class="variant-multiselect"
  >
    <template #option="{ option }">
      <span :style="{ paddingInlineStart: `${option.depth * 14}px` }">{{ option.label }}</span>
    </template>
  </MultiSelect>
</template>

<style scoped>
.variant-multiselect {
  width: 100%;
}
</style>
