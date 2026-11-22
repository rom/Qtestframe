<script setup lang="ts">
// A select of the project's variants ("All variants" when empty), with the
// hierarchy shown by indentation. Used for "show for variant" in lists,
// release readiness, documents and runs.
import { computed } from 'vue'
import Select from 'primevue/select'
import { useI18n } from 'vue-i18n'
import { variantOptions, type Variant } from '@/lib/variants'

const props = defineProps<{ variants: Variant[]; inputId?: string; ariaLabel?: string; labelledby?: string; placeholder?: string; disabled?: boolean; size?: 'small' | 'large' }>()
const model = defineModel<string | null>({ default: null })
const { t } = useI18n()
const options = computed(() => variantOptions(props.variants))
</script>

<template>
  <Select
    v-model="model"
    :input-id="inputId"
    :options="options"
    option-label="label"
    option-value="value"
    :placeholder="placeholder ?? t('variants.allVariants')"
    :aria-label="labelledby ? undefined : ariaLabel ?? t('variants.showFor')"
    :aria-labelledby="labelledby"
    :disabled="disabled"
    :size="size"
    filter
    show-clear
    class="variant-select"
  >
    <template #option="{ option }">
      <span :style="{ paddingInlineStart: `${option.depth * 14}px` }">{{ option.label }}</span>
    </template>
  </Select>
</template>

<style scoped>
.variant-select {
  min-width: 160px;
  max-width: 100%;
}
</style>
