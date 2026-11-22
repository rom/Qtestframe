<script setup lang="ts">
// Picks one or more roles (a member holds the union of their permissions):
// a multi-select with the chosen roles as chips. The model is kept in
// role order (admin and the built-in roles first, rbac.NormalizeRoles) and
// never becomes empty: removing the last role is refused (the "change"
// event is not emitted), because a member always has at least one role.
import { computed } from 'vue'
import MultiSelect from 'primevue/multiselect'
import { normalizeRoles, sameRoles } from '@/lib/permissions'
import { enumLabel } from '@/i18n/labels'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  /** role names to offer */
  options: readonly string[]
  /** roles shown but not offered for adding or removing (e.g. ones the caller may not give) */
  locked?: readonly string[]
  inputId?: string
  ariaLabel?: string
  ariaLabelledby?: string
  ariaDescribedby?: string
  disabled?: boolean
  size?: 'small' | 'large'
  invalid?: boolean
}>()
const model = defineModel<string[]>({ default: () => [] })
const emit = defineEmits<{ change: [roles: string[]] }>()
const { t } = useI18n()

const opts = computed(() => {
  const names = normalizeRoles([...props.options, ...model.value])
  return names.map((n) => ({ label: enumLabel('role', n), value: n, disabled: (props.locked ?? []).includes(n) || !props.options.includes(n) }))
})

function update(v: string[] | null) {
  const next = normalizeRoles(v ?? [])
  if (!next.length || sameRoles(next, model.value)) return
  model.value = next
  emit('change', next)
}
</script>

<template>
  <MultiSelect
    :model-value="model"
    :input-id="inputId"
    :options="opts"
    option-label="label"
    option-value="value"
    option-disabled="disabled"
    display="chip"
    :show-toggle-all="false"
    :placeholder="t('roles.pick')"
    :aria-label="ariaLabel"
    :aria-labelledby="ariaLabelledby"
    :aria-describedby="ariaDescribedby"
    :disabled="disabled"
    :size="size"
    :invalid="invalid"
    :max-selected-labels="4"
    class="role-picker"
    @update:model-value="update"
  />
</template>

<style scoped>
.role-picker {
  width: 100%;
  min-width: 0;
}
</style>
