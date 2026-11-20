<script setup lang="ts">
// The kind of a project. Software projects get extra features (prompt
// generation for requirements and tests).
import { computed } from 'vue'
import Select from 'primevue/select'
import { useI18n } from 'vue-i18n'
import { PROJECT_TYPE_ICONS, PROJECT_TYPES } from '@/lib/projectTypes'

defineProps<{ inputId?: string; disabled?: boolean }>()
const model = defineModel<string>({ default: 'general' })
const { t } = useI18n()
const options = computed(() => PROJECT_TYPES.map((v) => ({ value: v, label: t(`projectType.${v}`), hint: t(`projectType.${v}Hint`), icon: PROJECT_TYPE_ICONS[v] })))
</script>

<template>
  <Select v-model="model" :input-id="inputId" :options="options" option-label="label" option-value="value" :disabled="disabled" class="w-full">
    <template #value="{ value }">
      <span class="row nowrap"><i :class="options.find((o) => o.value === value)?.icon" aria-hidden="true" />{{ options.find((o) => o.value === value)?.label }}</span>
    </template>
    <template #option="{ option }">
      <div class="row nowrap" style="align-items: flex-start; gap: 8px">
        <i :class="option.icon" aria-hidden="true" style="margin-top: 3px" />
        <div>
          <div>{{ option.label }}</div>
          <div class="small muted" style="white-space: normal; max-width: 380px">{{ option.hint }}</div>
        </div>
      </div>
    </template>
  </Select>
</template>
